# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`gobog` is a small Markdown-only blog written in Go (module `github.com/SmartBrave/gobog`, Go 1.16). It can run in two modes:

- **Server mode** (default): boots two `http.Server`s (HTTP + HTTPS), reads Markdown from `[blog].source` once at startup, watches the directory with `fsnotify` and renders pages on demand using templates from `[blog].theme`.
- **Static export mode** (`-export <dir>`): renders the entire site into `<dir>` and exits. The output mirrors the gobog HTTP routes one-for-one and is drop-in compatible with the GitHub Pages-style layout used by `sbraveyoung/sbraveyoung.github.io`.

## Common commands

```bash
make build                                  # cleans and runs: go build -o gobog src/main.go
make release                                # build, then stage gobog + conf/ + themes/ under release/
make start / make stop / make restart       # daemonize / kill / kill+start
go test ./src/...                           # run unit tests (article, server)
go test -race ./src/...                     # run with the race detector
go run src/main.go -config conf/config.toml # default flags
./gobog -config conf/config.toml -export ./dist  # static export
```

Binding `:80` / `:443` (defaults in `conf/config.toml`) requires root or `setcap`. For local dev, edit `[http].addr`/`addrs` to high ports.

## Architecture

The init-driven bootstrap chain is still in place but the long-running serve is now explicit:

1. `src/config.init()` — parses `-config` and `-export`, decodes the TOML into `config.C`. Detects test binaries via the `.test` suffix and skips file I/O so unit tests don't fight the `flag` package.
2. `src/blog.init()` — constructs `blog.Blog`, calls `Blog.Reload()` to scan `[blog].source` once, then (in serve mode only) starts an `fsnotify` watcher that re-runs `Reload` on every `.md` change with a 300 ms debounce. `BlogST` guards `articles` and `byTag` with a `sync.RWMutex`; reads go through `Articles`/`Groups`/`AllPosts`/`PostsByTag`/`Tags`/`FindByURL`.
3. `src/main.go` — branches on `config.ExportDir`: empty → `server.Run()` (blocks via `gracehttp.Serve`); non-empty → `server.Export(...)` and exit.

### Request handlers (`src/server/server.go`)

Routes wired in `Server.newHandler()`:

| Route | Behaviour |
| --- | --- |
| `/` | renders `theme/index.html` with the post group list |
| `/post/...` | matches the deepest article whose `URL` is a prefix of the request; group → renders sub-list with `index.html`, leaf → renders `post.html` |
| `/about` | first article from the `about` list, rendered through `post.html` |
| `/tag/` and `/tag/<name>` | tag index and per-tag listing (uses the precomputed `byTag` map) |
| `/search?query=...` | in-memory scoring (title 10, tag 5, body 1) |
| `/atom.xml`, `/sitemap.xml`, `/robots.txt` | feed + SEO endpoints (see `src/server/feed.go`) |
| `/healthz` | liveness probe, returns `ok` |
| `/image/`, `/css/`, `/js/` | static passthrough; `safeServeFile` enforces both URL-prefix and filesystem-root containment so `/image/../../etc/passwd` is rejected |
| `/bing_img` | proxies the Bing image-of-the-day API |

When `[http].redirect_tls = true` and a TLS keypair is configured, the plain-HTTP server is swapped for a 301 redirector that points clients at the HTTPS listener. With it false (or TLS not configured), HTTP and HTTPS share the same handler.

### Markdown rendering (`src/server/render.go`)

`mdRenderer` is a package-level `goldmark.Markdown` with `extension.GFM`, `html.WithXHTML()`, `html.WithUnsafe()` (raw HTML in posts is allowed because the post template uses `text/template`, not `html/template`).

`renderArticleHTML` is the only place that calls `goldmark.Convert`. It checks `Article.CachedHTML()` first; on miss it renders, then `Article.StoreHTML()`s the result into a `sync/atomic.Value`. The cache is per-`*Article` and is cleared automatically by hot reload because `BlogST.Reload()` replaces the whole article pointer.

Templates receive an `articleView` wrapper that embeds `*Article` and shadows the `Parse` field with a per-request value. **Don't reintroduce writes to `Article.Parse`** — concurrent requests on the same article would race. The wrapper also exposes `Canonical`, `Description`, `Domain` so theme templates can fill `<link rel="canonical">` and OG meta without reading `config.C` themselves.

### Article model (`src/article/article.go`)

`NewArticle` reads a Markdown file and parses YAML-ish front-matter delimited by `---` lines (FIXME in source: don't use `---` as a horizontal rule inside post bodies — use `***`). Front-matter keys are mapped to `Meta` struct fields via `meta:"..."` tags using reflection. **All `Meta` fields must stay `string`-typed**: the same reflection loop is also used to rewrite the file in place, and it formats values as plain text. Multi-valued fields like `tags` are stored as a comma-separated `TagsRaw` string and split into `Tags []string` after the loop.

If `Title`, `CreateTime`, `Id`, or `URL` are missing, defaults are filled in (`Id` is a CRC32 of body content) and **the source `.md` file is rewritten in place** with the populated front-matter. The rewriter `Truncate(0)`s before writing so it is safe across multiple runs. Be aware of this side effect when pointing the server at a content directory.

`Article` also computes `WordCount`, `ReadingTimeMin` (250 wpm), and a `Summary` (description if present, else a stripped-markdown excerpt up to 160 runes). `IsDraft()` reads `Draft` (truthy values: `true`/`1`/`yes`); drafts are filtered out by `BlogST.Reload()` unless `[blog].include_drafts = true`.

`Articles` (slice) sort: groups (those with `SubArticle != nil`) first, then leaf articles by `CreateTime` descending using layout `2006-01-02 15:04:05`.

## Static export (`src/server/export.go`)

`server.Export(outDir)` walks `blog.Blog` and writes:

```
outDir/
├── index.html
├── about/index.html
├── post/<id>/index.html              (leaf post)
├── post/<id>/index.html              (group landing — sub-list rendered with index.html)
├── post/<id>/<sub>/index.html        (sub post)
├── tag/<tag>/index.html
├── 404.html                          (rendered via the same notFound handler)
├── atom.xml                          (built by buildAtomFeed)
├── sitemap.xml                       (built by buildSitemap)
├── robots.txt
├── CNAME                             (from [blog].cname, if non-empty)
├── css/                              (copied from theme)
├── js/                               (copied from theme)
└── image/                            (copied from <source>/image)
```

This layout is drop-in compatible with `sbraveyoung/sbraveyoung.github.io`. `urlToFile(outDir, url)` is the canonical URL → filesystem mapping (covered by `export_test.go`). The legacy `script/export.sh` is kept for reference but the Go subcommand is the supported path: it doesn't need a running server, respects `include_drafts`, and writes the SEO siblings (`atom.xml`, `sitemap.xml`, `robots.txt`).

## Configuration

`conf/config.toml` keys actually consumed:

- `[blog]`: `domain`, `title`, `subtitle`, `description`, `author`, `theme`, `source`, `cname`, `include_drafts`.
- `[http]`: `addr`, `addrs`, `cert`, `key`, `redirect_tls`.
- `[log]`: passthrough to beego's logger (currently unwired).

The Dockerfile uses `sed` to substitute `${YOUR_CERT_PATH}`, `${YOUR_SOURCE_PATH}`, and `${IMAGE_PATH}` placeholders at build time — keep those placeholder strings in sync between `conf/config.toml`, `script/export.sh`, and `dockerfile` if you rename them.

The blog content directory (`[blog].source`) is expected to contain subdirectories named after `BlogTypes` keys in `src/blog/blog.go` (currently `post/` and `about/`). Adding a new type means extending that map — `Reload` returns an error and startup aborts if a type's directory is missing.

## Testing

Unit tests live next to their packages:

- `src/article/article_test.go` covers front-matter parsing, in-place rewrite stability, tag splitting, CJK + ASCII word counting, sort order, summary extraction, draft detection, and the lazy HTML cache.
- `src/server/export_test.go`, `src/server/safefs_test.go` cover the URL → filesystem mapping and the path-traversal guard.

The test binary detects itself via `os.Args[0]` ending in `.test` and short-circuits `config.init()` (no flag parsing, no TOML read) and `blog.init()` (no disk scan, no fsnotify). Tests that need state should set `config.C` directly and call `blog.Blog.SetForTesting`.
