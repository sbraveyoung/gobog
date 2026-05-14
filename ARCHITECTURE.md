# Architecture notes

Single document summarizing how the four repos hang together and where
the rough edges still are. Updated whenever the layout changes.

```
┌─ Obsidian vault (local) ────────┐    ┌─ sbraveyoung/blog ─────────────┐
│  Blog/                          │    │  .                             │
│    post/                        │    │  ├─ post/                      │
│    pages/                       │ ◄──┤  ├─ pages/                     │
│    resource/image/              │    │  └─ resource/image/            │
│  ┌────────────────────────────┐ │    └────────────────┬───────────────┘
│  │ gobog-obsidian plugin       │ │                    │
│  │   • pull / push markdown    │ │                    │ push to master
│  │   • auto front-matter       │ ├──►                 │ → repository_dispatch
│  │   • WeChat draft (optional) │ │                    │   (event=blog-update)
│  │   • deploy status echo      │ │                    ▼
│  └────────────────────────────┘ │    ┌─ sbraveyoung/sbraveyoung.github.io ┐
└─────────────────────────────────┘    │  .github/workflows/deploy.yml:     │
                                       │    1. checkout blog + gobog (pin)  │
                                       │    2. go build _gobog/src (cached) │
                                       │    3. gobog -export _dist          │
                                       │    4. pagefind --site _dist        │
                                       │    5. cp _dist → working tree      │
                                       │    6. commit + push                │
                                       │  .github/workflows/linkcheck.yml:  │
                                       │    weekly external-link sweep      │
                                       │  _gobog.json controls theme +      │
                                       │  comments + brand                  │
                                       └────────────────────────────────────┘
                                                    │
                                                    ▼  GitHub Pages
                                              https://sbrave.cn
```

## What lives where

| Repo                       | Role                                          | Owner    |
| -------------------------- | --------------------------------------------- | -------- |
| `sbraveyoung/blog`         | Markdown source (authoritative).              | human    |
| `sbraveyoung/gobog`        | Go renderer + themes.                         | human    |
| `sbraveyoung/gobog-obsidian` | Obsidian plugin: sync + auto front-matter + optional WeChat draft. | human |
| `sbraveyoung/sbraveyoung.github.io` | Rendered HTML for GitHub Pages.        | machine  |

The github.io repo is **machine-managed**: don't hand-edit its tracked
files (other than `_gobog.json` and `.github/`). The deploy workflow
overwrites the working tree.

## Source-of-truth rules

- **Content** flows: vault → blog repo → github.io repo. Always one
  direction in the deploy. The Obsidian plugin's pull lets you bring
  upstream blog-repo changes (e.g., a quick edit through the GitHub
  web UI) back to the vault.
- **URLs** are owned by `front-matter url:`. Once a note has it, gobog
  never rewrites it. Don't change a published note's URL casually —
  external links break.
- **Theme + brand** live in github.io's `_gobog.json`. Changing them is
  a commit on github.io, not blog.
- **Comments** are off by default (giscus). Configure in `_gobog.json`.

## Improvements still worth doing

1. ~~**No client-side search on the static export.**~~ **Done.** The
   github.io deploy runs `pagefind --site _dist` after rendering and
   `themes/minimal/index.html` pulls in the bundled UI on the home
   page. Other themes (sepia, ocean, letter, press, tufte) haven't
   been wired up — copy the `<link>` / `<script>` block from minimal
   when you adopt one.
2. **Atom feed item count is unbounded.** `buildAtomFeed` lists every
   post. With many years of writing this becomes a several-MB feed.
   Cap at e.g. 50 newest.
3. ~~**Image dedupe.**~~ **Guardrail in place.** The github.io deploy
   warns via `::warning::` when the same image basename exists in both
   `_blog/image/` (legacy) and `_blog/resource/image/` (canonical).
   The collision itself still silently overwrites — fixing it
   structurally means picking one canonical home and removing the
   other in the source.
4. ~~**No automated link checker.**~~ **Done.** Weekly
   `.github/workflows/linkcheck.yml` in github.io scans every external
   `http(s)://` in the rendered HTML and surfaces 4xx/5xx/timeout as
   workflow annotations (non-fatal by default; manual dispatch with
   `fatal=true` for stricter runs).
5. **WeChat IP whitelist friction.** Every machine the plugin runs from
   needs that egress IP whitelisted in the MP console. A tiny relay
   service on a VPS with a fixed IP would avoid that — but adds an
   ops burden. Skip until pain demands it.
6. **gobog dependency on beego logger.** Pulls in a fairly large
   transitive dep tree just for logging. Could be replaced with
   `log/slog` from the standard library. Cosmetic.
7. **Partial PR preview.** sbraveyoung/blog now has a
   `.github/workflows/preview.yml` that dry-renders the site for every
   PR and surfaces dead-link / orphan-resource warnings as annotations.
   Still missing: a published preview URL (would require pushing to a
   `gh-pages/<pr>` branch or an artifact deploy). Live with the
   annotations until that's worth building.
8. **GH_PAT single point of failure.** The deploy depends on a Fine-
   grained PAT tied to one user account. Expires / revoked / suspended
   → deploys break silently. See `docs/AUTH.md` in the github.io repo
   for the GitHub App migration path; do it when (a) you start using
   AI-coding agents that need a bot identity, (b) you want per-repo
   audit logs, or (c) you don't want deploys to break the day you
   take a sabbatical.

## Dead-link audit (one-time)

Done as part of the current refresh:

- `github.com/SmartBrave/*` URLs in old posts: rewritten to
  `github.com/sbraveyoung/*` (the username rename).
- `dockerfile` in the blog repo: removed — no longer in the deploy
  path.
- `smartbrave/gobog` docker image base in the gobog dockerfile:
  changed the maintainer email; the base image reference is in the
  gobog dockerfile (self-hosting only) and is fine.
- Two `file:///D:/...` Typora paths in
  `《高质量的C_C++编程指南》读书笔记.md`: already inside HTML
  comments, render as nothing — left alone.

Re-run periodically with:

```sh
grep -rln "github\.com/SmartBrave" post
grep -rln "file:///" post
```

(Source paths are at the repo root now — `post/`, `pages/`,
`resource/image/`. The historical `source/` prefix is gone.)
