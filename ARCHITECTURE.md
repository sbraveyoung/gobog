# Architecture notes

Single document summarizing how the four repos hang together and where
the rough edges still are. Updated whenever the layout changes.

```
┌─ Obsidian vault (local) ────────┐    ┌─ sbraveyoung/blog ─────────────┐
│  Blog/                          │    │  source/                       │
│    post/                        │    │    post/                       │
│    pages/                       │ ◄──┤    pages/                      │
│    resource/image/              │    │    resource/image/             │
│  ┌────────────────────────────┐ │    └────────────────┬───────────────┘
│  │ gobog-obsidian plugin       │ │                    │
│  │   • pull / push markdown    │ │                    │ push to master
│  │   • auto front-matter       │ ├──►                 │ → repository_dispatch
│  │   • WeChat draft (optional) │ │                    │   (event=blog-update)
│  └────────────────────────────┘ │                    ▼
└─────────────────────────────────┘    ┌─ sbraveyoung/sbraveyoung.github.io ┐
                                       │  .github/workflows/deploy.yml:     │
                                       │    1. checkout blog + gobog        │
                                       │    2. go build _gobog/src          │
                                       │    3. gobog -export _dist          │
                                       │    4. cp _dist → working tree      │
                                       │    5. commit + push                │
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

1. **No client-side search on the static export.** Server mode has
   `/search`; static mode doesn't (would need a JS lunr or pagefind
   index emitted at build time). Punt for now — the blog isn't huge.
2. **Atom feed item count is unbounded.** `buildAtomFeed` lists every
   post. With many years of writing this becomes a several-MB feed.
   Cap at e.g. 50 newest.
3. **Image dedupe.** The legacy `<source>/image/` and the canonical
   `<source>/resource/image/` are both copied to `<dist>/image/`. If
   the same basename exists in both, the second overwrites the first.
   In practice we only have one of them populated at a time; still
   worth a guardrail.
4. **No automated link checker.** Posts with `[]()` markdown links to
   external sites can rot silently. A nightly workflow that runs a
   simple HTTP HEAD across the export would catch them.
5. **WeChat IP whitelist friction.** Every machine the plugin runs from
   needs that egress IP whitelisted in the MP console. A tiny relay
   service on a VPS with a fixed IP would avoid that — but adds an
   ops burden. Skip until pain demands it.
6. **gobog dependency on beego logger.** Pulls in a fairly large
   transitive dep tree just for logging. Could be replaced with
   `log/slog` from the standard library. Cosmetic.
7. **No preview-before-deploy on github.io.** The workflow renders
   straight to main. For a one-author blog this is fine; for a team a
   PR-preview workflow that publishes to a branch would help.
8. **`cert/` in the blog repo.** Holds TLS keys for the legacy
   self-hosted era. They aren't deployed anywhere by the current
   pipeline, but they sit in the repo's history. Consider rotating
   the certs out of git (move to a vault / 1Password) before the next
   major commit. Treat anything currently in `cert/` as compromised
   for hygiene.

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
grep -rln "github\.com/SmartBrave" source/post
grep -rln "file:///" source/post
```
