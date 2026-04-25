# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`gobog` is a small Markdown-only blog written in Go (module `github.com/SmartBrave/gobog`, Go 1.16). It is designed to consume an Obsidian-style folder (typically a subfolder of an Obsidian vault — point `[blog].source` at e.g. `Vault/Blog/`) and run in one of two modes:

- **Server mode** (default): boots two `http.Server`s (HTTP + HTTPS), scans `[blog].source` once at startup, watches the directory recursively with `fsnotify`, and renders pages on demand using templates from `[blog].theme`. Wikilinks and image embeds are resolved through an in-memory index built during the scan.
- **Static export mode** (`-export <dir>`): renders the entire site into `<dir>` and exits. The output mirrors the gobog HTTP routes one-for-one and is drop-in compatible with GitHub Pages.

The Obsidian plugin that drives gobog (push-button "publish my vault") lives in a separate repository — gobog itself never touches git. It just consumes a folder and produces either live HTTP responses or a static tree.

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
2. `src/blog.init()` — constructs `blog.Blog`, calls `Blog.Reload()` to scan `[blog].source` once, then (in serve mode only) starts a recursive `fsnotify` watcher that re-runs `Reload` on every `.md` change with a 300 ms debounce. `BlogST` guards `articles`, `byTag`, and `wiki` with a `sync.RWMutex`; reads go through `Articles`/`Groups`/`AllPosts`/`PostsByTag`/`Tags`/`FindByURL`/`Wiki`. `Reload` switches between two layouts:
   - **Vault mode** (no `<source>/post/` directory): walks `<source>` recursively at any depth. Each `.md` becomes a post; each subdirectory becomes a group; URLs are derived from the relative path via `Slugify`. `<source>/about/` is special-cased so the first note inside it becomes `/about`. Hidden directories (`.obsidian/` etc.) are skipped.
   - **Legacy mode** (when `<source>/post/` exists): falls back to the original 1-level group convention with CRC32-hex IDs and in-place front-matter rewrites. Kept so existing deployments keep booting.
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
| `/css/`, `/js/` | static passthrough; `safeServeFile` enforces both URL-prefix and filesystem-root containment so `/css/../../etc/passwd` is rejected |
| `/image/<rest>` | first tries `<source>/image/<rest>` (legacy) and falls back to `WikiIndex.ResolveImage(basename)` so attachments scattered through the vault can still be served |
| `/bing_img` | proxies the Bing image-of-the-day API |

When `[http].redirect_tls = true` and a TLS keypair is configured, the plain-HTTP server is swapped for a 301 redirector that points clients at the HTTPS listener. With it false (or TLS not configured), HTTP and HTTPS share the same handler.

### Markdown rendering (`src/server/render.go`)

`mdRenderer` is a package-level `goldmark.Markdown` with `extension.GFM`, `html.WithXHTML()`, `html.WithUnsafe()` (raw HTML in posts is allowed because the post template uses `text/template`, not `html/template`).

`renderArticleHTML` is the only place that calls `goldmark.Convert`. Before invoking goldmark, the source bytes go through `expandWikilinks`, which rewrites Obsidian syntax against `blog.Blog.Wiki()`:

- `[[Note Title]]` → `[Note Title](/post/...)`
- `[[Note Title|display text]]` → `[display text](/post/...)`
- `[[Note Title#section]]` → appends `#section` (slugified) to the resolved URL
- `![[image.png]]` / `![[image.png|alt]]` → `<img src="/image/<rel-path>" alt="...">`
- Unresolved `[[...]]` falls back to plain text (display value if provided, else the target name) — never a broken anchor.

After rendering, the result is stored via `Article.StoreHTML()`. The cache is per-`*Article` and is cleared automatically by hot reload because `BlogST.Reload()` replaces the whole article pointer.

Templates receive an `articleView` wrapper that embeds `*Article` and shadows the `Parse` field with a per-request value. **Don't reintroduce writes to `Article.Parse`** — concurrent requests on the same article would race. The wrapper also exposes `Canonical`, `Description`, `Domain` so theme templates can fill `<link rel="canonical">` and OG meta without reading `config.C` themselves.

### Wiki index (`src/blog/blog.go`)

`WikiIndex` is built during `Reload` and read by the renderer + image handler:

- `notes` maps every plausible lookup key (lowercased basename, relative path, front-matter title) to the canonical URL. Multiple keys can resolve to the same article. `ResolveNote(target)` lowercases + strips `.md` and tries the full key first, then the basename so `[[Tech/Networking]]` and `[[Networking]]` both work.
- `images` maps a lowercased filename to its disk path relative to `<source>`. The image handler uses this to serve attachments anywhere in the vault when a request comes in for `/image/<basename>`. The legacy `<source>/image/<rest>` path is still honored first.

`NewWikiIndexForTesting(notes, images)` is exposed so tests can build a deterministic index without spinning up the scanner.

### Article model (`src/article/article.go`)

Two constructors:

- **`ParseFile(path)`** is the **read-only** path used by the vault scanner. It parses front-matter and body but never touches the file on disk. URLs / IDs / titles are derived by the caller from the disk path; the article model itself stays pristine.
- **`NewArticle(path, type, fatherURL)`** is the legacy constructor that mirrors the original gobog behavior: parses, fills missing meta defaults (`Id` = CRC32 of body), and **rewrites the source `.md` in place** to persist them. Only used when `<source>/post/` exists (legacy layout).

Front-matter keys are mapped to `Meta` struct fields via `meta:"..."` tags using reflection. **All `Meta` fields must stay `string`-typed** because the reflective rewriter formats values as plain text. The parser strips `[ ]` from `tags:` so Obsidian's YAML-array form (`tags: [a, b]`) decodes into the same `TagsRaw` as the comma-separated form. Tags are also accepted with a leading `#`.

`Article` derives `WordCount`, `ReadingTimeMin` (250 wpm), `Summary` (description if present, else a stripped-markdown excerpt up to 160 runes), and exposes a lazy `CachedHTML/StoreHTML` pair backed by `sync/atomic.Value` for goroutine-safe caching. `IsDraft()` reads `Draft` (truthy: `true`/`1`/`yes`).

`Slugify(s)` returns a kebab-case ASCII slug; `SlugOrHash(s)` falls back to a stable CRC32 hex when the slug is empty (e.g. CJK-only titles), so URLs are always addressable.

`Articles` (slice) sort: groups (those with `SubArticle != nil`) first, then leaf articles by `CreateTime` descending using layout `2006-01-02 15:04:05`.

## Static export (`src/server/export.go`)

`server.Export(outDir)` walks `blog.Blog` recursively (any depth) and writes:

```
outDir/
├── index.html
├── about/index.html
├── post/<slug-path>/index.html       (every leaf at any depth)
├── post/<slug-path>/index.html       (group landing — sub-list rendered with index.html)
├── tag/<tag>/index.html
├── 404.html                          (rendered via the same notFound handler)
├── atom.xml                          (built by buildAtomFeed)
├── sitemap.xml                       (built by buildSitemap)
├── robots.txt
├── CNAME                             (from [blog].cname, if non-empty)
├── css/                              (copied from theme)
├── js/                               (copied from theme)
└── image/<rel-path>                  (every non-.md file in <source>, preserving its layout)
```

`exportPosts` is recursive, so vault folders nested 3+ levels deep all get a directory + landing page in the output. Asset copying covers both legacy `<source>/image/` and arbitrary attachment paths in the vault (the latter walked from `<source>` and copied to `<outDir>/image/<rel-path>`, matching the URLs the renderer emits).

`urlToFile(outDir, url)` is the canonical URL → filesystem mapping (covered by `export_test.go`). The legacy `script/export.sh` is kept for reference but the Go subcommand is the supported path: it doesn't need a running server, respects `include_drafts`, and writes the SEO siblings (`atom.xml`, `sitemap.xml`, `robots.txt`).

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
