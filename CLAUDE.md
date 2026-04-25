# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`gobog` is a small Markdown-only blog server written in Go (module `github.com/SmartBrave/gobog`, Go 1.16). It reads Markdown files from disk at startup, parses Jekyll-style front-matter, and serves rendered HTML using templates from a theme directory.

## Common commands

```bash
make build          # cleans and runs: go build -o gobog src/main.go
make release        # build, then stage gobog + conf/ + themes/ under release/
make start          # nohup ./gobog >debug.log 2>&1 &
make stop           # pkill the running gobog process
go run src/main.go  # run directly; uses ./conf/config.toml by default
./gobog -config /path/to/config.toml   # override config path
```

There is no test suite wired up.

Binding `:80` / `:443` (defaults in `conf/config.toml`) requires root or `setcap`. For local dev, edit `[http].addr`/`addrs` and provide cert paths, or comment out the TLS server in `src/server/server.go`.

## Architecture

The unusual thing about this codebase is that **`main.go` does no work** — it just `sync.WaitGroup.Wait()`s forever. All bootstrapping happens via package `init()` functions, chained by import order:

1. `src/config` — `init()` parses the `-config` flag and decodes TOML into the package-global `config.C`.
2. `src/blog` — `init()` walks `config.C.Blog.Source` for each entry in `BlogTypes` (`post`, `about`). Top-level files become `Article`s; subdirectories become "group" articles whose `SubArticle` list is the directory's `.md` files. Results are sorted and stored in `blog.Blog.Articles[type]`.
3. `src/server` — `init()` builds two `http.Server`s (HTTP + HTTPS) sharing one `mux`, then calls `gracehttp.Serve` (Facebook archive's graceful restart wrapper). This call **blocks**, which is what keeps the process alive; `main.go`'s WaitGroup is just defensive.

`src/main.go` imports `src/server` with a blank import (`_`) to trigger this chain. Adding a new top-level package means importing it from `main.go` (or transitively) so its `init()` runs.

### Request handlers (`src/server/server.go`)

Routes are wired in `Server.newHandler()`:

- `/` → renders `theme/index.html` with the `post` article list.
- `/post/...` → linear scan of `blog.Blog.Articles["post"]` matching `URL` prefix; if matched article has `SubArticle`s, recurse to find the leaf, then render `theme/post.html` (uses `text/template` here, not `html/template`, so article HTML isn't escaped). Otherwise renders the group with `index.html`.
- `/about` → first article in the `about` list, rendered via `post.html`.
- `/image/`, `/css/`, `/js/` → static file passthrough rooted at `config.C.Blog.Source` (images) or `config.C.Blog.Theme` (assets).
- `/bing_img` → proxies the Bing image-of-the-day API.

Markdown is rendered on each request via the package-level `mdRenderer` in `src/server/server.go` — a `goldmark.Markdown` configured with `extension.GFM` (tables, strikethrough, autolinks, task lists), `html.WithXHTML()`, and `html.WithUnsafe()` (raw HTML in posts is intentionally allowed because the post template uses `text/template`, not `html/template`). The shared `renderArticle` helper prepends `## <title>` before rendering. Results are stored back on `Article.Parse` but the entire article slice is shared across goroutines without locking, and the field is rewritten on every request — be careful introducing mutation, and prefer fixing the cache/race together rather than piling on more shared writes.

### Article model (`src/article/article.go`)

`NewArticle` reads a Markdown file and parses YAML-ish front-matter delimited by `---` lines (FIXME in source: don't use `---` as a horizontal rule inside post bodies — use `***`). Front-matter keys are mapped to `Meta` struct fields via `meta:"..."` tags using reflection.

If `Title`, `CreateTime`, `Id`, or `URL` are missing, defaults are filled in (`Id` is a CRC32 of body content, formatted as hex), and **the source `.md` file is rewritten in place** with the populated front-matter. Be aware of this side effect when pointing the server at a content directory.

`Articles` (slice) implements `sort.Interface`: groups (those with `SubArticle != nil`) come first, then leaf articles by `CreateTime` descending using layout `2006-01-02 15:04:05`. Times that fail to parse compare as equal.

## Configuration

`conf/config.toml` is the single source of runtime config. The Dockerfile uses `sed` to substitute `${YOUR_CERT_PATH}`, `${YOUR_SOURCE_PATH}`, and `${IMAGE_PATH}` placeholders at build time — keep those placeholder strings in sync between `conf/config.toml`, `script/export.sh`, and `dockerfile` if you rename them.

The blog content directory (`[blog].source`) is expected to contain subdirectories named after `BlogTypes` keys in `src/blog/blog.go` (currently `post/` and `about/`). Adding a new type means extending that map — startup `os.Exit(1)`s if the directory is missing.

## Static export

`script/export.sh` is a separate utility that crawls a running gobog instance over HTTP and writes static HTML into `./export/`. It greps for `tag="export"` markers in rendered pages to discover post URLs, so theme templates must keep that attribute on links you want crawled.
