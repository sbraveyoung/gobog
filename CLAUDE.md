# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`gobog` is a small Markdown-only blog written in Go (module `github.com/sbraveyoung/gobog`, Go 1.16). It is designed to consume an Obsidian-style folder (typically a subfolder of an Obsidian vault — point `[blog].source` at e.g. `Vault/Blog/`) and run in one of two modes:

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
2. `src/blog.init()` — constructs `blog.Blog`, calls `Blog.Reload()` to scan `[blog].source` once, then (in serve mode only) starts a recursive `fsnotify` watcher that re-runs `Reload` on every `.md` change with a 300 ms debounce. `BlogST` guards `articles`, `byTag`, and `wiki` with a `sync.RWMutex`; reads go through `Articles`/`Groups`/`AllPosts`/`PostsByTag`/`Tags`/`FindByURL`/`Wiki`. `Reload` always walks `<source>` recursively at any depth; each `.md` becomes a post and each subdirectory becomes a group. `<source>/about/` is special-cased so the first note inside it becomes `/about`. Hidden directories (`.obsidian/`, `.git/`) and `[blog].exclude_dirs` entries at the top level are skipped. URL generation is controlled by `[blog].layout`: `"vault"` (default) uses slugified path; `"legacy"` uses the original gobog `/post/<crc32(parent)>/<crc32(body)>` scheme. **Missing front-matter (id / url / title / create_time) is auto-filled and persisted back to the source `.md` file** the first time the scanner sees a note — original gobog product design — so URLs stay stable even when files are later moved or renamed.
3. `src/main.go` — branches on `config.ExportDir`: empty → `server.Run()` (blocks via `gracehttp.Serve`); non-empty → `server.Export(...)` and exit.

### Request handlers (`src/server/server.go`)

Routes wired in `Server.newHandler()`:

| Route | Behaviour |
| --- | --- |
| `/` | renders `theme/index.html` with the post group list |
| `/post/...` | matches the deepest article whose `URL` is a prefix of the request; group → renders `group.html` (falling back to `index.html`), leaf → renders `post.html` |
| `/<page>` | any path that doesn't hit a more specific route is matched against `<source>/pages/<page>.md` (`pages/about.md` → `/about`, etc.). Rendered through `post.html`. |
| `/tag/` and `/tag/<name>` | tag index and per-tag listing (uses the precomputed `byTag` map) |
| `/search?query=...` | in-memory scoring (title 10, tag 5, body 1) |
| `/atom.xml`, `/sitemap.xml`, `/robots.txt` | feed + SEO endpoints (see `src/server/feed.go`); private posts are filtered out |
| `/healthz` | liveness probe, returns `ok` |
| `/snippet`, `/snippet/<id>` | gist-style snippet sharing — POST creates (auth required), GET shows; rendered through the same goldmark pipeline as posts |
| `/css/`, `/js/` | static passthrough; `safeServeFile` enforces both URL-prefix and filesystem-root containment so `/css/../../etc/passwd` is rejected |
| `/image/<rest>` | first tries `<source>/image/<rest>` (legacy) and falls back to `WikiIndex.ResolveImage(basename)` so attachments scattered through the vault can still be served. JPEG / PNG responses go through `serveWatermarked` when `[image].watermark_text` is set; output is cached on disk under `<data>/wm-cache/` keyed by source path + mtime + watermark text + position |
| `/bing_img` | proxies the Bing image-of-the-day API |

`postHandler` now does an exact-URL walk via `findArticle`. The original implementation's prefix-then-fallback would render the deepest matching group when an unknown URL fell under it — clean 404 instead.

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

Two helpers:

- **`ParseFile(path)`** reads front-matter + body without touching the file. The blog scanner calls this first.
- **`RewriteFrontMatter(a)`** persists `a.Meta` back to `a.Source` (seek 0 + truncate + rewrite). The blog scanner calls this when defaults were filled in, so the user's vault gets `id` / `url` / `title` / `create_time` written once and never drifts.

Front-matter keys are mapped to `Meta` struct fields via `meta:"..."` tags using reflection. **All `Meta` fields must stay `string`-typed** because the reflective rewriter formats values as plain text. The parser strips `[ ]` from `tags:` so Obsidian's YAML-array form (`tags: [a, b]`) decodes into the same `TagsRaw` as the comma-separated form. Tags are also accepted with a leading `#`.

The blog scanner's `buildArticle` is responsible for filling missing meta and calling `RewriteFrontMatter`. The article package itself does not touch disk during reads.

`Article` derives `WordCount`, `ReadingTimeMin` (250 wpm), `Summary` (description if present, else a stripped-markdown excerpt up to 160 runes), and exposes a lazy `CachedHTML/StoreHTML` pair backed by `sync/atomic.Value` for goroutine-safe caching.

The visibility quartet is governed by a shared `truthy` helper (case-insensitive `true` / `1` / `yes` / `on`):

- `IsDraft()` — work-in-progress; excluded from listings unless `[blog].include_drafts`.
- `IsHidden()` — temporarily off; excluded from listings unless `[blog].include_hidden`. Mechanically the same as Draft today; the distinction is purely semantic for the author.
- `IsPrivate()` — listed in indexes but the body is gated by HTTP Basic auth (see `[auth]`). Filtered out of `atom.xml`, `sitemap.xml`, and the static export entirely (no auth on a static host).
- `IsPinned()` — sticks the article to the top of its containing listing, overriding the usual create_time descending order. The `Articles` sort considers the pinned bit before the date.
- `IsAI()` / `AILabel()` — `ai:` front-matter marks AI-collaborative content. Truthy values render a "🤖 AI 协作" badge; any other non-empty, non-falsy string is treated as a model name and shown verbatim ("🤖 claude", "🤖 GPT-4o"). Authors can write `ai: false` to mean "I wrote this myself".
- `Cover` — front-matter `cover:` value, surfaced as `articleView.CoverURL`. Bare names get prefixed with `/image/`; absolute URLs (`http(s)://`, `/...`) pass through. Renders as a hero image at the top of post.html and as `og:image` for social previews.

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

- `[blog]`: `domain`, `title`, `subtitle`, `description`, `author`, `theme`, `source`, `cname`, `include_drafts`, `include_hidden`, `layout` (`vault` (default) / `legacy` — controls URL generation only; the scanner is always recursive), `exclude_dirs` (top-level vault dirs to skip, case-insensitive).
- `[http]`: `addr`, `addrs`, `cert`, `key`, `redirect_tls`. Missing cert/key files **don't** abort startup — the server logs a warning and serves plain HTTP only. `redirect_tls=true` is also disarmed when TLS isn't actually loaded, so users never bounce into a non-existent listener.
- `[auth]`: `username`, `password_hash` (hex sha256 of the plaintext password — generate via `printf 'pw' | sha256sum`), `realm`. When either field is empty, every auth-gated endpoint returns 503 (so private posts and snippet POSTs fail closed).
- `[data]`: `dir` — where the server keeps mutable state (`views.json`, `snippets/`, `wm-cache/`, `backups/`). Defaults to `./gobog-data`. Static export ignores this.
- `[image]`: `watermark_text` (turns watermarking on when non-empty), `watermark_position` (`top-left` / `top-right` / `bottom-left` / `bottom-right` (default) / `center`). Caches go under `<data>/wm-cache/`.
- `[backup]`: `enabled`, `interval` (Go duration like `1h`, `24h`; default `1h`), `dir` (defaults to `<data>/backups`), `keep` (rotation; default `7`). The worker runs once at startup and then on the ticker, writes `gobog-<utc-timestamp>.tar.gz`, and rotates by name (timestamps sort lexicographically).
- `[log]`: passthrough to beego's logger (currently unwired).

The Dockerfile uses `sed` to substitute `${YOUR_CERT_PATH}`, `${YOUR_SOURCE_PATH}`, and `${IMAGE_PATH}` placeholders at build time — keep those placeholder strings in sync between `conf/config.toml`, `script/export.sh`, and `dockerfile` if you rename them.

The blog content directory (`[blog].source`) follows a fixed three-folder contract:

- `<source>/post/...` — articles (recursive). Each sub-directory is a group.
- `<source>/pages/<name>.md` — top-level pages. Each file renders at `/<name>` (e.g. `pages/about.md` → `/about`). Sub-directories under `pages/` are ignored — useful for stashing drafts that aren't ready.
- `<source>/resource/image/...` — images and other binary attachments. Served under `/image/...`. Legacy `<source>/image/` is still honored for back-compat.

`BlogTypes` in `src/blog/blog.go` maps the logical name (`post`, `page`) to its on-disk folder.

## Testing

Unit tests live next to their packages:

- `src/article/article_test.go` — front-matter parsing, in-place rewrite stability, tag splitting, CJK + ASCII word counting, sort order (groups first, pinned next, then date desc), summary extraction, the visibility quartet (draft/hidden/private/pin), and the lazy HTML cache.
- `src/blog/blog_test.go` — Reload against an Obsidian-style fixture (vault layout, 3-level nesting, .obsidian skipped, image index by basename) and a legacy-layout fixture.
- `src/server/render_test.go` — `expandWikilinks` covering wikilinks, display text, fragments, path-style targets, missing notes, image embeds with and without alt text, unresolved images.
- `src/server/auth_test.go` — `requireAuth` decision matrix: missing config (503), missing creds (401 + WWW-Authenticate), wrong user, wrong password, valid creds (200). Plus `hashPassword` determinism + length.
- `src/server/views_test.go` — counter increment, persistence round-trip via temp-file rename, idempotent flush on clean state, concurrent increments under `-race`.
- `src/server/snippets_test.go` — POST without auth (401), POST with auth (200 + JSON), GET round-trip, bad-id 404, bad-id rejection of `..`/spaces, GET form for the empty path.
- `src/server/export_test.go` — `urlToFile` mapping, `withoutPrivate` recursive filter (drops private leaves AND empty-after-filter groups so the exported tree never links to a 404).
- `src/server/safefs_test.go` — `safeServeFile` rejects `/image/../../etc/passwd`-style traversal.
- `src/server/watermark_test.go` — disabled when text is empty, skipped on unsupported formats (SVG / GIF), correct PNG round-trip, on-disk cache hit returns identical bytes, every position constant produces a valid image.
- `src/server/backup_test.go` — `writeArchive` skips top-level `.obsidian` / `.git`, `rotateBackups` keeps newest N (and `keep <= 0` is a no-op), `backupOnce` round-trip, `archiveName` uses UTC.

The test binary detects itself via `os.Args[0]` ending in `.test` and short-circuits `config.init()` (no flag parsing, no TOML read) and `blog.init()` (no disk scan, no fsnotify). Tests that need state should set `config.C` directly and call `blog.Blog.SetForTesting`.

Run with the race detector — `views.go` uses lock-free atomics on a `sync.Map` and the post handler / view counter / wiki index are exercised concurrently in production. `go test -race ./src/...` is the supported gate.
