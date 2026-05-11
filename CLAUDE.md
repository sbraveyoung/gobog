# CLAUDE.md

本文件为 Claude Code（claude.ai/code）在本仓库中工作时提供指引。

## 项目概览

`gobog` 是一个用 Go 编写、只面向 Markdown 的小型博客（模块路径 `github.com/sbraveyoung/gobog`，Go 1.16）。它的设计目标是消费一个 Obsidian 风格的目录（通常是某个 Obsidian 仓库的子目录 —— 把 `[blog].source` 指向例如 `Vault/Blog/`），并以下列两种模式之一运行：

- **服务模式**（默认）：启动两个 `http.Server`（HTTP + HTTPS），在启动时扫描一次 `[blog].source`，用 `fsnotify` 递归监听目录，使用 `[blog].theme` 的模板按需渲染页面。Wikilinks 与图片嵌入通过扫描时构建的内存索引解析。
- **静态导出模式**（`-export <dir>`）：把整个站点渲染到 `<dir>` 目录后退出。输出与 gobog 的 HTTP 路由一一对应，可直接投递到 GitHub Pages。

驱动 gobog 的 Obsidian 插件（一键“发布我的 vault”）在另一个仓库 —— gobog 自身从不接触 git。它只消费一个目录并输出实时的 HTTP 响应或一棵静态文件树。

## 常用命令

```bash
make build                                  # 清理后执行: go build -o gobog src/main.go
make release                                # 构建后，把 gobog + conf/ + themes/ 整理到 release/
make start / make stop / make restart       # 守护进程化 / 终止 / 终止后再启动
go test ./src/...                           # 运行单元测试 (article, server)
go test -race ./src/...                     # 带 race detector 运行
go run src/main.go -config conf/config.toml # 默认参数
./gobog -config conf/config.toml -export ./dist  # 静态导出
```

绑定 `:80` / `:443`（`conf/config.toml` 中的默认端口）需要 root 或 `setcap`。本地开发时把 `[http].addr`/`addrs` 改成高端口。

## 架构

init 串联式启动链条仍在，但长驻 serve 现在是显式启动的：

1. `src/config.init()` —— 解析 `-config` 与 `-export`，把 TOML 反序列化到 `config.C`。通过 `.test` 后缀检测测试二进制并跳过文件 I/O，避免单元测试与 `flag` 包打架。
2. `src/blog.init()` —— 构造 `blog.Blog`，调用 `Blog.Reload()` 扫描一次 `[blog].source`，随后（仅在 serve 模式下）启动递归的 `fsnotify` 监听器，对每次 `.md` 改动以 300ms 的防抖再次 `Reload`。`BlogST` 用 `sync.RWMutex` 守护 `articles`、`byTag` 和 `wiki`；读操作走 `Articles`/`Groups`/`AllPosts`/`PostsByTag`/`Tags`/`FindByURL`/`Wiki`。`Reload` 始终递归遍历任意深度的 `<source>`；每个 `.md` 是一篇文章，每个子目录是一个分组。`<source>/about/` 是特例 —— 其中第一篇笔记会成为 `/about`。隐藏目录（`.obsidian/`、`.git/`）以及 `[blog].exclude_dirs` 中位于顶层的目录会被跳过。URL 生成由 `[blog].layout` 控制：`"vault"`（默认）使用 slug 化的路径；`"legacy"` 使用 gobog 原有的 `/post/<crc32(parent)>/<crc32(body)>` 方案。**缺失的 front-matter（id / url / title / create_time）会在扫描第一次见到笔记时自动补齐，并写回源 `.md` 文件**（这是 gobog 原版的产品设计），从而保证即便文件后续被移动或重命名，URL 也保持稳定。
3. `src/main.go` —— 根据 `config.ExportDir` 分支：为空 → `server.Run()`（通过 `gracehttp.Serve` 阻塞）；非空 → `server.Export(...)` 后退出。

### 请求处理（`src/server/server.go`）

`Server.newHandler()` 中注册的路由：

| 路由 | 行为 |
| --- | --- |
| `/` | 用 `theme/index.html` 渲染首页，列出文章分组 |
| `/post/...` | 匹配 `URL` 是请求路径前缀的最深一层文章；如果命中分组 → 渲染 `group.html`（缺失时回退到 `index.html`），如果命中叶子文章 → 渲染 `post.html` |
| `/<page>` | 任何未命中更具体路由的路径会去匹配 `<source>/pages/<page>.md`（`pages/about.md` → `/about`，依此类推），通过 `post.html` 渲染 |
| `/tag/` 与 `/tag/<name>` | 标签索引与单标签列表（使用预先计算的 `byTag`） |
| `/search?query=...` | 内存中的打分搜索（标题 10 分，标签 5 分，正文 1 分） |
| `/atom.xml`、`/sitemap.xml`、`/robots.txt` | feed + SEO 接口（见 `src/server/feed.go`）；私密文章会被过滤掉 |
| `/healthz` | 存活探针，返回 `ok` |
| `/snippet`、`/snippet/<id>` | gist 风格的代码片段分享 —— POST 创建（需鉴权），GET 显示；通过和文章一样的 goldmark 管线渲染 |
| `/css/`、`/js/` | 静态直通；`safeServeFile` 同时强制 URL 前缀以及文件系统根目录边界，所以 `/css/../../etc/passwd` 会被拒绝 |
| `/image/<rest>` | 先尝试 `<source>/image/<rest>`（旧路径），再回退到 `WikiIndex.ResolveImage(basename)`，这样散落在 vault 各处的附件也能被解析。当 `[image].watermark_text` 不为空时，JPEG / PNG 响应会走 `serveWatermarked`；输出以源路径 + mtime + 水印文本 + 位置作为 key 缓存到 `<data>/wm-cache/` |
| `/bing_img` | 代理 Bing 每日图片 API |

`postHandler` 现在通过 `findArticle` 做严格的 URL 全匹配。原实现使用“前缀匹配后回退”的方式，当未知 URL 落在某个分组下时会渲染出该分组中匹配最深的页 —— 现在改为干净的 404。

当 `[http].redirect_tls = true` 且配置了 TLS 密钥对时，明文 HTTP 服务会被替换为一个 301 跳转器，把客户端引向 HTTPS 监听端口。当该值为 false（或没有配置 TLS）时，HTTP 与 HTTPS 共用同一个 handler。

### Markdown 渲染（`src/server/render.go`）

`mdRenderer` 是一个包级 `goldmark.Markdown`，启用了 `extension.GFM`、`html.WithXHTML()`、`html.WithUnsafe()`（文章模板使用 `text/template` 而非 `html/template`，所以正文中的原始 HTML 被允许）。

`renderArticleHTML` 是唯一调用 `goldmark.Convert` 的地方。在交给 goldmark 之前，源字节会先经过 `expandWikilinks`，依据 `blog.Blog.Wiki()` 重写 Obsidian 语法：

- `[[Note Title]]` → `[Note Title](/post/...)`
- `[[Note Title|display text]]` → `[display text](/post/...)`
- `[[Note Title#section]]` → 在解析得到的 URL 后追加 `#section`（slug 化）
- `![[image.png]]` / `![[image.png|alt]]` → `<img src="/image/<rel-path>" alt="...">`
- 未解析的 `[[...]]` 回退为纯文本（如果提供了显示文本就用它，否则用目标名）—— 永远不会输出一个坏掉的锚点。

渲染完成后，结果通过 `Article.StoreHTML()` 缓存。这层缓存是按 `*Article` 维度的，并且在热重载时会被自动清空，因为 `BlogST.Reload()` 会把整个 article 指针整体替换。

模板收到的是一个 `articleView` wrapper，它内嵌 `*Article`，并以每次请求独立的值覆盖（shadow）了 `Parse` 字段。**不要重新引入对 `Article.Parse` 的写入** —— 那样同一篇文章的并发请求会出现 race。该 wrapper 还暴露了 `Canonical`、`Description`、`Domain`，主题模板就能填充 `<link rel="canonical">` 与 OG 元信息而无需自己去读 `config.C`。

### Wiki 索引（`src/blog/blog.go`）

`WikiIndex` 在 `Reload` 中构建，被渲染器与图片 handler 共享读取：

- `notes` 将每个合理的查找 key（小写文件名、相对路径、front-matter 标题）映射到规范 URL。多个 key 可以解析到同一篇文章。`ResolveNote(target)` 会先做小写化并去掉 `.md`，先用完整 key 查找，再用 basename 查找，因此 `[[Tech/Networking]]` 和 `[[Networking]]` 都能命中。
- `images` 将小写化的文件名映射到相对 `<source>` 的磁盘路径。图片 handler 在收到 `/image/<basename>` 请求时通过这里去定位 vault 任意位置的附件。旧的 `<source>/image/<rest>` 路径仍然优先尝试。

`NewWikiIndexForTesting(notes, images)` 对外暴露，方便测试构造确定性的索引而无需启动扫描器。

### Article 模型（`src/article/article.go`）

两个辅助函数：

- **`ParseFile(path)`** 读取 front-matter 与正文，不会改动文件。博客扫描器会先调用它。
- **`RewriteFrontMatter(a)`** 把 `a.Meta` 持久化回 `a.Source`（seek 0 → truncate → 重写）。当扫描器补全了默认值后会调用它，把 `id` / `url` / `title` / `create_time` 写入用户的 vault，且只写一次，从此不再漂移。

Front-matter 的 key 通过 `meta:"..."` tag 用反射映射到 `Meta` 结构体的字段。**`Meta` 的所有字段必须保持 `string` 类型**，因为反射式的 rewriter 把值当作纯文本来格式化。Parser 会去掉 `tags:` 上的 `[ ]`，这样 Obsidian 的 YAML 数组形式（`tags: [a, b]`）和逗号分隔形式都会被解码到同一个 `TagsRaw`。Tags 也接受前导 `#`。

博客扫描器中的 `buildArticle` 负责补全缺失的 meta 并调用 `RewriteFrontMatter`。article 包本身在读取阶段不接触磁盘。

`Article` 派生出 `WordCount`、`ReadingTimeMin`（250 字/分钟）、`Summary`（如果有 description 就用，否则用去掉 markdown 标记后最多 160 字的摘要），并通过 `sync/atomic.Value` 暴露一对懒加载、goroutine 安全的 `CachedHTML/StoreHTML` 缓存方法。

可见性四件套由共享的 `truthy` 辅助函数管理（大小写不敏感的 `true` / `1` / `yes` / `on`）：

- `IsDraft()` —— 草稿；在 `[blog].include_drafts` 未开时不进入列表。
- `IsHidden()` —— 暂时下线；在 `[blog].include_hidden` 未开时不进入列表。从机制上和 Draft 一样，只是语义上对作者来说不同。
- `IsPrivate()` —— 出现在索引中，但正文受 HTTP Basic auth 保护（见 `[auth]`）。在 `atom.xml`、`sitemap.xml` 与静态导出中完全过滤掉（静态托管无法做 auth）。
- `IsPinned()` —— 在所属列表中置顶，覆盖默认按 `create_time` 倒序的排序。`Articles` 的排序中 pinned 标志优先于日期。
- `IsAI()` / `AILabel()` —— `ai:` front-matter 标记 AI 参与度。词表借鉴 IPTC 的 digitalSourceType 三级体系，用中文表达：真值类关键词（`true` / `yes` / `1` / `on` / `assisted`）→ "🤖 AI 辅助"（人主导）；`generated` / `wrote` / `written` → "🤖 AI 生成"（AI 主导，人审校）；`edited` / `reviewed` / `polished` / `proofread` → "🤖 AI 校对"（AI 润色）。其它任何非空值（例如 `ai: claude`、`ai: gpt-4o`）被当成模型署名直接显示。作者可以写 `ai: false` 表示"这是我自己写的"。
- `Cover` —— front-matter 的 `cover:` 值，作为 `articleView.CoverURL` 暴露给模板。裸文件名会被加上 `/image/` 前缀；绝对 URL（`http(s)://`、`/...`）原样透传。会在 post.html 顶部渲染为 hero 图，同时作为 `og:image` 用于社交分享。

`Slugify(s)` 返回 kebab-case 的 ASCII slug；`SlugOrHash(s)` 在 slug 为空时（例如纯 CJK 标题）回退为稳定的 CRC32 hex，所以 URL 永远可寻址。

`Articles`（切片）的排序：先列出分组（即 `SubArticle != nil`），再按 `CreateTime` 倒序列出叶子文章，使用 `2006-01-02 15:04:05` 的布局。

## 静态导出（`src/server/export.go`）

`server.Export(outDir)` 递归遍历 `blog.Blog`（任意深度）并写出：

```
outDir/
├── index.html
├── about/index.html
├── post/<slug-path>/index.html       （任意深度的每个叶子）
├── post/<slug-path>/index.html       （分组的落地页 —— 子列表用 index.html 渲染）
├── tag/<tag>/index.html
├── 404.html                          （通过同一个 notFound handler 渲染）
├── atom.xml                          （由 buildAtomFeed 构造）
├── sitemap.xml                       （由 buildSitemap 构造）
├── robots.txt
├── CNAME                             （来自 [blog].cname，非空时写入）
├── css/                              （从主题复制）
├── js/                               （从主题复制）
└── image/<rel-path>                  （`<source>` 中所有非 .md 文件，保留原结构）
```

`exportPosts` 是递归的，因此 vault 中嵌套 3 层及以上的目录在输出里都会得到对应的目录 + 落地页。资源拷贝同时覆盖旧版的 `<source>/image/` 和 vault 中任意路径的附件（后者从 `<source>` 起步遍历，复制到 `<outDir>/image/<rel-path>`，这与渲染器写出的 URL 一致）。

`urlToFile(outDir, url)` 是 URL → 文件系统路径的规范映射（由 `export_test.go` 覆盖）。旧脚本 `script/export.sh` 仅作参考；目前受支持的是 Go 子命令：它无需启动 server，遵循 `include_drafts`，并写入 SEO 相邻产物（`atom.xml`、`sitemap.xml`、`robots.txt`）。

## 配置

`conf/config.toml` 实际生效的 key：

- `[blog]`：`domain`、`title`、`subtitle`、`description`、`author`、`theme`、`source`、`cname`、`include_drafts`、`include_hidden`、`layout`（`vault`（默认）/ `legacy` —— 只控制 URL 生成；扫描器始终是递归的）、`exclude_dirs`（顶层要跳过的 vault 目录，大小写不敏感）。
- `[http]`：`addr`、`addrs`、`cert`、`key`、`redirect_tls`。证书/密钥文件缺失 **不会** 阻塞启动 —— server 会打一条 warning 并只服务 HTTP。如果 TLS 没能真正加载，`redirect_tls=true` 也会被卸除，避免把用户重定向到根本不存在的监听器。
- `[auth]`：`username`、`password_hash`（明文密码的 sha256 hex，可通过 `printf 'pw' | sha256sum` 生成）、`realm`。两者任一为空时，所有受 auth 保护的接口返回 503（这样私密文章和 snippet 的 POST 接口都是 fail-closed 的）。
- `[data]`：`dir` —— server 存放可变状态的目录（`views.json`、`snippets/`、`wm-cache/`、`backups/`）。默认 `./gobog-data`。静态导出会忽略该项。
- `[image]`：`watermark_text`（非空即开启水印），`watermark_position`（`top-left` / `top-right` / `bottom-left` / `bottom-right`（默认）/ `center`）。缓存放在 `<data>/wm-cache/`。
- `[backup]`：`enabled`、`interval`（Go duration，如 `1h`、`24h`；默认 `1h`）、`dir`（默认 `<data>/backups`）、`keep`（轮转数；默认 `7`）。worker 在启动时执行一次，然后按 ticker 触发，写出 `gobog-<utc-timestamp>.tar.gz`，按文件名做轮转（时间戳按字典序排序）。
- `[log]`：直通给 beego 的 logger（目前未接线）。

Dockerfile 在构建时用 `sed` 替换 `${YOUR_CERT_PATH}`、`${YOUR_SOURCE_PATH}` 与 `${IMAGE_PATH}` 占位符 —— 如果重命名了这些占位符，要在 `conf/config.toml`、`script/export.sh` 与 `dockerfile` 之间保持一致。

博客内容目录（`[blog].source`）遵循固定的三级目录约定：

- `<source>/post/...` —— 文章（递归）。每个子目录是一个分组。
- `<source>/pages/<name>.md` —— 顶层页面。每个文件渲染到 `/<name>`（例如 `pages/about.md` → `/about`）。`pages/` 下的子目录被忽略 —— 适合存放还没准备好的草稿。
- `<source>/resource/image/...` —— 图片与其它二进制附件。在 `/image/...` 下提供。出于兼容，旧的 `<source>/image/` 也仍被识别。

`src/blog/blog.go` 中的 `BlogTypes` 把逻辑名（`post`、`page`）映射到磁盘目录。

## 测试

单元测试与各包并排放置：

- `src/article/article_test.go` —— front-matter 解析、原地重写的稳定性、tag 拆分、CJK + ASCII 字数统计、排序规则（分组优先，置顶其次，然后日期倒序）、摘要抽取、可见性四件套（draft/hidden/private/pin），以及懒加载的 HTML 缓存。
- `src/blog/blog_test.go` —— 针对 Obsidian 风格 fixture（vault 布局、三层嵌套、跳过 `.obsidian`、按 basename 建索引）以及 legacy 布局 fixture 的 Reload 测试。
- `src/server/render_test.go` —— `expandWikilinks` 覆盖 wikilinks、显示文本、片段、路径式目标、缺失笔记、带与不带 alt 的图片嵌入、未解析图片。
- `src/server/auth_test.go` —— `requireAuth` 的决策矩阵：缺配置（503）、缺凭证（401 + WWW-Authenticate）、用户名错、密码错、合法凭证（200），以及 `hashPassword` 的确定性与长度。
- `src/server/views_test.go` —— 计数器自增、通过临时文件 rename 的持久化往返、干净状态下幂等的 flush、`-race` 下的并发自增。
- `src/server/snippets_test.go` —— 未鉴权 POST（401）、鉴权后 POST（200 + JSON）、GET 往返、坏 id 404、坏 id 拒绝 `..`/空格、空路径 GET 返回表单。
- `src/server/export_test.go` —— `urlToFile` 映射、`withoutPrivate` 的递归过滤（同时丢弃私密叶子与过滤后空掉的分组，确保导出树不会指向 404）。
- `src/server/safefs_test.go` —— `safeServeFile` 拒绝 `/image/../../etc/passwd` 风格的路径穿越。
- `src/server/watermark_test.go` —— 文本为空时关闭、对不支持的格式（SVG / GIF）跳过、正确的 PNG 往返、命中磁盘缓存时返回完全相同的字节、每个位置常量都能产出有效图像。
- `src/server/backup_test.go` —— `writeArchive` 跳过顶层的 `.obsidian` / `.git`、`rotateBackups` 保留最新 N 份（`keep <= 0` 时是 no-op）、`backupOnce` 往返、`archiveName` 使用 UTC。

测试二进制通过 `os.Args[0]` 是否以 `.test` 结尾来识别自己，进而短路 `config.init()`（不解析 flag，不读 TOML）与 `blog.init()`（不扫盘，不挂 fsnotify）。需要状态的测试应直接设置 `config.C` 并调用 `blog.Blog.SetForTesting`。

请带 race detector 运行 —— `views.go` 在 `sync.Map` 上使用无锁的 atomics，post handler / 阅读计数 / wiki index 在生产里都被并发使用。`go test -race ./src/...` 是受支持的把关命令。
