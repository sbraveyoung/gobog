# gobog

**[English](./README.md)** | [简体中文](./README.zh-CN.md)

A small, dependency-light Markdown blog server written in Go. Point it at an
[Obsidian](https://obsidian.md/) vault folder (or any directory of `.md`
files) and it does one of two things:

- **Serve mode** (default): boots HTTP / HTTPS, watches the source directory
  with `fsnotify`, renders pages on demand from goldmark + a theme.
- **Static export** (`-export <dir>`): renders the entire site into `<dir>`
  and exits. The output is drop-in compatible with GitHub Pages.

```
Obsidian vault            gobog                       reader
─────────────────  ────►  ───────────────  ────────►  HTML
   Blog/                  Live HTTP server            (or)
     Hello.md             OR                          GitHub Pages
     Tech/                gobog -export ./dist        (drag the dist into
       HTTP.md                                         <user>.github.io)
     about/me.md
```

## Quick start

```sh
git clone https://github.com/sbraveyoung/gobog
cd gobog
make build                                   # produces ./gobog
./gobog -config conf/config.toml             # serve mode
./gobog -config conf/config.toml -export ./dist   # static export
```

Binding `:80` / `:443` (defaults in `conf/config.toml`) needs root or
`setcap cap_net_bind_service=+ep ./gobog`. For local dev change
`[http].addr` to e.g. `:8080`.

## Vault layout

Point `[blog].source` at the folder you want to publish. With an
Obsidian-style vault you'd typically pick a subfolder (e.g. `Vault/Blog/`)
so the rest of your notes stay private.

```
<source>/
├── Hello.md                        → /post/hello
├── Tech/
│   └── HTTP.md                     → /post/tech/http
│   └── Networking/
│       └── TLS.md                  → /post/tech/networking/tls
├── about/
│   └── me.md                       → /about     (first .md inside about/)
├── attachments/
│   └── diagram.png                 → /image/diagram.png  (resolved by basename)
└── .obsidian/                      ignored (any dotfile / dotdir is skipped)
```

The URL path is derived from the relative file path, slugified per segment
(lowercase + ASCII; CJK-only segments fall back to a stable CRC32 hex so URLs
stay addressable).

`<source>/post/` enabling **legacy mode** is auto-detected; force the
behavior you want with `[blog].layout = "vault" | "legacy" | "auto"`.

## Front-matter

YAML between two `---` lines. All fields are optional. (FIXME: don't use
`---` as a horizontal rule inside post bodies; use `***`.)

```yaml
---
title: HTTP Protocol
description: A short summary used for OG meta + listing excerpt.
author: smart
create_time: 2026-04-25 09:00:00
tags: [networking, http]               # YAML list or "a, b, c"
cover: hero.png                        # bare name → /image/hero.png; URLs pass through
url: /post/tech/http                   # override the auto-generated slug
id: stable-id                          # used in legacy mode for /post/<id>
draft: true                            # excluded from listings unless include_drafts
hidden: true                           # like draft, semantically "temporarily off"
private: true                          # listed but body needs HTTP Basic auth
pin: true                              # sticks to the top of its containing listing
ai: claude                             # 🤖 badge with the model name; `ai: true` → "🤖 AI"
---

Body markdown here. **Bold**, *italics*, [links](https://example.com),
fenced code blocks, tables, math (`$E = mc^2$`) all work.

Obsidian wikilinks resolve against the source folder:
  [[Note Title]]            → /post/.../note-title
  [[Note Title|alt text]]   → custom anchor text
  [[Note Title#section]]    → fragment appended (slugified)
  ![[diagram.png]]          → <img src="/image/diagram.png">
  ![[diagram.png|caption]]  → with alt text

Unresolved [[...]] degrades to plain text — never broken anchors.
```

## URL surface

| Route | Behavior |
| --- | --- |
| `/` | Homepage; lists top-level posts + groups with summary, date, reading time, tags. |
| `/post/<slug>` | Leaf post (rendered with `post.html`). |
| `/post/<group>` | Group landing (rendered with `index.html`, lists sub-posts recursively). |
| `/about` | First `.md` under `<source>/about/`. |
| `/tag/` | List of all tags. |
| `/tag/<name>` | Posts tagged `<name>`. |
| `/search?query=<q>` | In-memory search (title 10 / tag 5 / body 1 weights). |
| `/atom.xml` | Atom feed (skips private posts). |
| `/sitemap.xml` | Sitemap. |
| `/robots.txt` | Robots + sitemap pointer. |
| `/healthz` | Liveness probe. |
| `/snippet`, `/snippet/<id>` | gist-style snippet share. POST needs auth. |
| `/image/<path>` | Tries `<source>/image/<path>` first, then resolves by basename via the wiki index. JPEG/PNG can be watermarked. |
| `/css/`, `/js/` | Theme passthrough. Path traversal is rejected. |
| `/bing_img` | Proxy for Bing image-of-the-day. |

## Configuration (`conf/config.toml`)

```toml
[blog]
domain          = "https://example.com"  # used for canonical / atom / sitemap
title, subtitle, description, author     # site metadata
theme           = "themes/simple"
source          = "/path/to/your/Vault/Blog"
layout          = "auto"                 # auto | vault | legacy
exclude_dirs    = []                     # vault top-level dirs to skip
cname           = "example.com"          # written to <export>/CNAME
include_drafts  = false
include_hidden  = false

[http]
addr            = ":80"
addrs           = ":443"
cert            = ""                     # missing files → warn, fall back to HTTP
key             = ""
redirect_tls    = false                  # auto-disarmed if TLS isn't loaded

[auth]
username        = ""                     # required for /private posts + snippet POST
password_hash   = ""                     # printf 'pw' | sha256sum
realm           = "gobog"

[data]
dir             = "./gobog-data"         # views.json, snippets/, wm-cache/, backups/

[image]
watermark_text     = ""                  # set to enable; JPEG/PNG only
watermark_position = "bottom-right"      # top-left|top-right|bottom-left|bottom-right|center

[backup]
enabled         = false
interval        = "1h"                   # Go duration
dir             = ""                     # defaults to <data>/backups
keep            = 7                      # rotate, keep newest N
```

## Static export → GitHub Pages

```sh
./gobog -config conf/config.toml -export ./dist
```

Produces:

```
dist/
├── index.html
├── about/index.html
├── post/<slug-path>/index.html             # any depth
├── tag/<tag>/index.html
├── 404.html
├── atom.xml, sitemap.xml, robots.txt
├── CNAME                                    # if [blog].cname is set
├── css/, js/, image/<rel-path>
```

This layout is the same shape as `<user>.github.io` repositories. Push
`./dist` to your Pages repo and Pages serves it as-is. The Obsidian plugin
[`gobog-obsidian`](https://github.com/sbraveyoung/gobog-obsidian) wraps
"export + git push" into a single command palette action.

## Features at a glance

- **Markdown**: goldmark + GFM (tables, strikethrough, autolinks, task lists).
- **Wikilinks + image embeds**: Obsidian-style `[[...]]` and `![[...]]`
  resolved against an in-memory index.
- **Visibility**: `draft`, `hidden` (excluded from listings), `private`
  (listed but body gated by HTTP Basic auth), `pin` (sorted to the top).
- **AI provenance badge**: `ai: true` or `ai: <model-name>`.
- **Hero image**: `cover: foo.png`; doubles as `og:image`.
- **Reading time / word count / view counter**: surfaced in the post meta bar
  and in listings.
- **Search**: `/search?query=...`, in-memory weighted scoring.
- **Atom feed + sitemap + robots + 404**: SEO siblings, also generated for
  static export.
- **Hot reload**: `fsnotify` watches `[blog].source`; 300ms debounced rescan.
- **Snippets**: `POST /snippet` (auth) creates a tiny gist-style share, GET
  renders through the same goldmark pipeline.
- **Image watermarking**: optional text overlay on JPEG/PNG with on-disk
  cache keyed by source mtime + watermark settings.
- **Periodic backup**: tar.gz `[blog].source` to `<dir>/gobog-<utc>.tar.gz`,
  rotated to `keep` archives.
- **TLS graceful**: missing cert files log a warning and serve plain HTTP;
  `redirect_tls` is automatically disarmed when no cert loaded.
- **Path-traversal safe**: `/image/`, `/css/`, `/js/` all enforce both URL
  prefix and filesystem-root containment.

## Development

```sh
go test ./src/...           # unit tests
go test -race ./src/...     # required gate; many handlers race on shared state
go run src/main.go -config conf/config.toml
make build                  # cleans and runs go build -o gobog src/main.go
make release                # stage gobog + conf/ + themes/ under release/
```

Tests live next to their packages:

| Package | What's covered |
| --- | --- |
| `src/article` | front-matter parsing, in-place rewrite stability, tag splitting, CJK + ASCII word counting, sort order (groups → pinned → date), summary extraction, draft/hidden/private/pin/AI semantics, lazy HTML cache, slugify + hex fallback |
| `src/blog` | vault-layout scan with 3-level nesting, `.obsidian` skipped, image index by basename, legacy-layout fallback, `exclude_dirs`, `layout="vault"` override |
| `src/server` | wikilink expansion (8 cases), `urlToFile` mapping, `withoutPrivate` recursive filter, `safeServeFile` traversal guard, `requireAuth` decision matrix, view counter persistence + concurrent increment, snippet POST/GET/auth, watermark cache hit, backup tar contents + rotation, cover URL helper, `findArticle` exact-match (incl. front-matter-pinned non-prefix URLs), TLS missing-cert graceful fallback |

The test binary detects itself via `os.Args[0]` ending in `.test` and
short-circuits `config.init()` (no flag parsing, no TOML read) and
`blog.init()` (no disk scan, no fsnotify). Tests that need state should set
`config.C` directly and call `blog.Blog.SetForTesting`.

## Theme

`themes/simple/` is the default theme. It's a small Go template pair plus
CSS:

```
themes/simple/
├── index.html      homepage + group landing (loops *Article slice)
├── post.html       single article page
├── login.html      reserved (not yet wired)
├── resume.html     reserved (not yet wired)
└── css/
    ├── home.css, main.css, prism.css, resume.css   (legacy)
    └── theme.css   (overrides + dark mode + new layout)
```

Templates receive an `articleView` wrapper that embeds `*Article` and
shadows `Parse` with a per-request rendered string. New themes can rely on
`{{.Title}}`, `{{.URL}}`, `{{.Tags}}`, `{{.Summary}}`, `{{.WordCount}}`,
`{{.ReadingTimeMin}}`, `{{.ViewCount}}`, `{{.Pinned}}`, `{{.Private}}`,
`{{.AI}}`, `{{.AILabel}}`, `{{.CoverURL}}`, `{{.Canonical}}`, `{{.Domain}}`.

The `simple` theme ships:

- Custom semantic header (no Bootstrap, no jQuery, no Google Analytics)
- Dark mode toggle, persisted to `localStorage`, respects `prefers-color-scheme`
- Reading progress bar + auto-built TOC (≥3 headings, ≥1100px viewport)
- Hero cover image, OG meta
- Code blocks: hover copy button + auto-fold for blocks > 20 lines
- Lazy-loaded Prism (only when code is present) and MathJax (only when `$`
  / `\(` is detected)
- `@media print` hides nav, footer, TOC, copy/fold buttons; expands folded
  code; forces black-on-white

## Project structure

```
src/
├── main.go              entry — dispatches Run() vs Export()
├── config/              TOML loader, flag parser (-config / -export)
├── article/             Meta + Article + Articles sort, ParseFile / NewArticle
└── server/
    ├── server.go        HTTP routing + Run loop + handlers
    ├── render.go        goldmark + wikilink preprocessor + articleView wrapper
    ├── feed.go          atom + sitemap + robots
    ├── export.go        static export, withoutPrivate filter
    ├── auth.go          HTTP Basic auth helper
    ├── views.go         atomic view counter + JSON persistence
    ├── snippets.go      gist-style POST + GET handlers
    ├── watermark.go     image overlay + on-disk cache
    └── backup.go        periodic tar.gz worker + rotation

src/blog/                BlogST, WikiIndex, Reload, fsnotify watcher
themes/simple/           default theme
conf/config.toml         sample config
script/export.sh         legacy curl-based export (kept for reference;
                         the Go subcommand is the supported path)
dockerfile               container build (substitutes ${YOUR_*} placeholders)
.github/workflows/       CI: pushes a multi-arch image on master
```

## Architecture notes

`main.go` is intentionally short:

```go
if config.ExportDir != "" {
    server.Export(config.ExportDir)
} else {
    server.Run()
}
```

Bootstrap order is driven by Go's package init chain:

1. `config.init` parses flags + TOML into `config.C` (skipped under `go test`).
2. `blog.init` constructs `blog.Blog` and calls `Reload()` once; in serve
   mode it also starts the fsnotify watcher (skipped under `go test` and
   for export mode).
3. `server.Run()` (server mode) loads TLS, builds the mux, and blocks on
   `gracehttp.Serve`.

Reads from `BlogST` go through accessor methods that hold an `RWMutex`;
`Reload()` swaps the entire `articles`/`byTag`/`wiki` triple under a write
lock, so request handlers see either the old state or the new state, never
a half-built one.

Markdown rendering caches per-`*Article` via a `sync/atomic.Value`. Hot
reload constructs new `*Article` pointers, so the cache invalidates
automatically when the underlying file changes.

## License

MIT (see `LICENSE` once added; the upstream repo doesn't currently ship one
but contributions are welcome under MIT).
