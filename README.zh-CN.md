# gobog

[English](./README.md) | **[简体中文](./README.zh-CN.md)**

一个用 Go 写的轻量 Markdown 博客服务器。把它指向一个
[Obsidian](https://obsidian.md/) vault 文件夹（或任何放 `.md` 文件的目录）就能用，
两种运行方式二选一：

- **服务模式**（默认）：起 HTTP / HTTPS 监听，用 `fsnotify` 监控源目录，按需用
  goldmark + 主题渲染页面。
- **静态导出**（`-export <dir>`）：把整站渲染成 `<dir>` 下的静态目录后退出，
  输出可直接丢给 GitHub Pages。

```
Obsidian vault              gobog                       读者
─────────────────────  ─►   ───────────────  ─────────► HTML
   Blog/                    动态 HTTP 服务              （或者）
     post/                  或者                        GitHub Pages
       Hello.md             gobog -export ./dist        （把 dist 推到
       Tech/HTTP.md                                        <user>.github.io）
     pages/about.md
     resource/image/foo.png
```

## 快速开始

```sh
git clone https://github.com/sbraveyoung/gobog
cd gobog
make build                                          # 产出 ./gobog
./gobog -config conf/config.toml                    # 服务模式
./gobog -config conf/config.toml -export ./dist     # 静态导出
```

绑定 `:80` / `:443`（`conf/config.toml` 默认值）需要 root 或
`setcap cap_net_bind_service=+ep ./gobog`。本地开发把 `[http].addr` 改成 `:8080`
之类的高端口即可。

## Vault 目录结构

把 `[blog].source` 指向你想发布的目录。Obsidian 用户一般指到 vault 的某个子目录
（比如 `Vault/Blog/`），这样别的笔记就不会被发布出去。

```
<source>/
├── post/                           文章都放这里
│   ├── Hello.md                    → /post/hello
│   ├── Tech/HTTP.md                → /post/tech/http
│   └── Tech/Networking/TLS.md      → /post/tech/networking/tls
├── pages/                          顶层独立页面
│   ├── about.md                    → /about
│   └── contact.md                  → /contact
├── resource/image/                 图片 / 附件
│   └── diagram.png                 → /image/diagram.png
└── .obsidian/                      跳过（任何点开头的文件 / 目录都跳过）
```

老版本的 `<source>/image/` 仍然作为兜底解析。`post/` 下空目录会被自动从
列表里剔除。

URL 路径来自相对文件路径，每段做 slug 化（小写 + ASCII；纯 CJK 段会回退到稳定的
CRC32 hex，保证 URL 始终可寻址）。

扫描永远是递归的——`[blog].layout` 只控制**新文章的 URL 生成策略**：
`"vault"`（默认）按 slug 化路径生成；`"legacy"` 复刻老 gobog 的
`/post/<crc32(parent)>/<crc32(body)>` 形式（保住已发布站点的 hex URL）。
扫到缺 `id` / `url` / `title` / `create_time` 的笔记时，扫描器会按既定规则
补齐并**写回 `.md` 文件**——这是 gobog 的产品设计，URL 钉死后即使你把文件
改名或移动也不会变。

## 主题

仓库自带三套主题，通过 `[blog].theme = "themes/<name>"` 选择：

| 主题               | 风格                                |
| ------------------ | ----------------------------------- |
| `themes/minimal`   | 默认主题，黑白极简                  |
| `themes/sepia`     | 暖色调，类似纸质阅读体验            |
| `themes/ocean`     | 冷蓝调，安静耐看                    |

三套主题共享 HTML 模板（`index.html`、`group.html`、`post.html`）和
JavaScript，只是 CSS 颜色 token 不同。每个主题都支持：

- 自动跟随系统深色 / 浅色，也支持手动切换（`localStorage` 持久化）
- 中文 / English 导航切换（同样持久化）
- 移动端友好的导航栏、阅读进度条、代码块复制按钮
- 桌面端（视口 ≥ 1100px）右侧目录侧边栏；窄屏自动隐藏，避免上滑时遮挡正文

## Front-matter

两条 `---` 之间的 YAML，所有字段都可选。（注意：正文里别用 `---` 作分隔线，
用 `***` 替代。）

```yaml
---
title: HTTP Protocol
description: 用作 OG 元信息和列表摘要的简短描述
author: smart
create_time: 2026-04-25 09:00:00
tags: [networking, http]               # YAML 列表或 "a, b, c" 形式都可以
cover: hero.png                        # 裸文件名 → /image/hero.png；URL 直通
url: /post/tech/http                   # 覆盖自动生成的 slug
id: stable-id                          # legacy 模式下用作 /post/<id>
draft: true                            # 排除在列表之外（除非 include_drafts）
hidden: true                           # 跟 draft 类似，语义上是"暂时下线"
private: true                          # 列出但正文需要 HTTP Basic 认证
pin: true                              # 置顶到所属列表的开头
ai: true                               # → "🤖 AI 辅助"   （truthy 默认）
                                       # ai: generated → "🤖 AI 生成"
                                       # ai: edited    → "🤖 AI 校对"
                                       # ai: claude    → "🤖 claude"   （模型署名，原样显示）
---

正文 markdown。**粗体**、*斜体*、[链接](https://example.com)、围栏代码块、表格、
公式（`$E = mc^2$`）都支持。

Obsidian wikilink 会基于源目录解析：
  [[Note Title]]            → /post/.../note-title
  [[Note Title|alt text]]   → 自定义锚点文本
  [[Note Title#section]]    → 追加 fragment（slug 化）
  ![[diagram.png]]          → <img src="/image/diagram.png">
  ![[diagram.png|caption]]  → 带 alt 文本

无法解析的 [[...]] 会降级为纯文本——永远不会留死链。
```

## URL 路由

| 路径 | 行为 |
| --- | --- |
| `/` | 首页；列出顶级 posts 和 group，带摘要 / 日期 / 阅读时长 / tags |
| `/post/<slug>` | 文章详情页（用 `post.html` 渲染） |
| `/post/<group>` | 分组着陆页（用 `group.html` 渲染，缺失时回落到 `index.html`） |
| `/<page>` | 顶层页面 (`pages/<page>.md`)。用 `post.html` 渲染 |
| `/tag/` | 所有标签列表 |
| `/tag/<name>` | 含该标签的文章列表 |
| `/search?query=<q>` | 内存搜索（标题权重 10 / 标签 5 / 正文 1） |
| `/atom.xml` | Atom 订阅（私密文章过滤掉） |
| `/sitemap.xml` | sitemap |
| `/robots.txt` | robots + sitemap 指针 |
| `/healthz` | 存活探针 |
| `/snippet`, `/snippet/<id>` | gist 风格代码段分享，POST 需登录 |
| `/image/<path>` | 先尝试 `<source>/image/<path>`，再按 basename 在 wiki 索引里找。JPEG/PNG 可加水印 |
| `/css/`, `/js/` | 主题静态资源直通，路径穿越被拒 |
| `/bing_img` | Bing 每日图片代理 |

## 配置（`conf/config.toml`）

```toml
[blog]
domain          = "https://example.com"  # 用于 canonical / atom / sitemap
title, subtitle, description, author     # 站点元信息
theme           = "themes/minimal"       # 或 themes/sepia | themes/ocean
source          = "/path/to/your/Vault/Blog"
layout          = "vault"                # vault（默认）| legacy — 只决定 URL 生成策略；扫描始终递归
exclude_dirs    = []                     # 顶级要跳过的目录名
cname           = "example.com"          # 写入 <export>/CNAME
include_drafts  = false
include_hidden  = false

[http]
addr            = ":80"
addrs           = ":443"
cert            = ""                     # 文件不存在时只警告，回退到 HTTP
key             = ""
redirect_tls    = false                  # TLS 没加载时自动失效

[auth]
username        = ""                     # 私密文章 + snippet POST 需要
password_hash   = ""                     # printf 'pw' | sha256sum
realm           = "gobog"

[data]
dir             = "./gobog-data"         # views.json, snippets/, wm-cache/, backups/

[image]
watermark_text     = ""                  # 设值后启用；只对 JPEG/PNG 生效
watermark_position = "bottom-right"      # top-left|top-right|bottom-left|bottom-right|center

[backup]
enabled         = false
interval        = "1h"                   # Go duration 格式
dir             = ""                     # 默认 <data>/backups
keep            = 7                      # 轮转保留份数
```

## 静态导出 → GitHub Pages

```sh
./gobog -config conf/config.toml -export ./dist
```

输出：

```
dist/
├── index.html
├── about/index.html
├── post/<slug-path>/index.html             # 任意深度
├── tag/<tag>/index.html
├── 404.html
├── atom.xml, sitemap.xml, robots.txt
├── CNAME                                    # 取自 [blog].cname
├── css/, js/, image/<rel-path>
```

这个布局跟 `<user>.github.io` 仓库的形态一致。把 `./dist` 推到你的 Pages 仓库
GitHub Pages 就直接 serve。Obsidian 插件
[`gobog-obsidian`](https://github.com/sbraveyoung/gobog-obsidian) 把
"导出 + git push" 包成命令面板里一条命令。

## 功能一览

- **Markdown**：goldmark + GFM（表格、删除线、autolink、任务列表）
- **Wikilink + 图片嵌入**：解析 Obsidian 风格 `[[...]]` 和 `![[...]]`
- **可见性**：`draft` / `hidden`（不进列表）/ `private`（列出但正文需 auth）/
  `pin`（置顶）
- **AI 标记**：`ai: true` 或 `ai: <model-name>`
- **Hero 图**：`cover: foo.png`，同时充当 `og:image`
- **阅读时长 / 字数 / 浏览量**：在 post meta 条和首页列表都展示
- **搜索**：`/search?query=...`，内存加权打分
- **Atom 订阅 / sitemap / robots / 404**：SEO 全套，静态导出也生成
- **热加载**：`fsnotify` 监控 `[blog].source`，300ms 防抖后整库重扫
- **Snippets**：`POST /snippet`（需 auth）创建 gist 风格分享，GET 走同一个 goldmark 管线
- **图片水印**：可选的 JPEG/PNG 文字水印，按 source mtime + 水印参数做磁盘缓存
- **周期备份**：tar.gz 整个 `[blog].source` 到 `<dir>/gobog-<utc>.tar.gz`，按 `keep` 轮转
- **TLS graceful**：cert 文件不存在时打 warn 日志，照常服务 HTTP；TLS 没加载时
  `redirect_tls` 自动失效
- **路径穿越防护**：`/image/`、`/css/`、`/js/` 同时校验 URL 前缀和文件系统根

## 开发

```sh
go test ./src/...           # 单元测试
go test -race ./src/...     # 必须过；多个 handler 共享状态
go run src/main.go -config conf/config.toml
make build                  # clean + go build -o gobog src/main.go
make release                # 打 release 包到 release/
```

测试和包放在一起：

| 包 | 覆盖 |
| --- | --- |
| `src/article` | front-matter 解析、原地重写稳定性、tag 切分、CJK + ASCII 字数、排序（group → pinned → 日期）、摘要提取、draft/hidden/private/pin/AI 语义、惰性 HTML 缓存、slugify + hex 回退 |
| `src/blog` | vault 三层嵌套扫描、`.obsidian` 跳过、按 basename 的图片索引、legacy 布局回退、`exclude_dirs`、`layout="vault"` 强制 |
| `src/server` | wikilink 展开（8 种）、`urlToFile` 映射、`withoutPrivate` 递归过滤、`safeServeFile` 穿越防护、`requireAuth` 决策矩阵、计数器持久化 + 并发增量、snippet POST/GET/auth、水印缓存命中、tar 内容 + 轮转、cover URL helper、`findArticle` 精确匹配（含 front-matter pinned 非前缀 URL）、TLS 缺证书优雅降级 |

测试二进制通过 `os.Args[0]` 后缀 `.test` 自识别，跳过 `config.init()`（不解析 flag、
不读 TOML）和 `blog.init()`（不扫盘、不起 fsnotify）。需要状态的测试自己设置
`config.C` 并调 `blog.Blog.SetForTesting`。

## 主题

`themes/simple/` 是默认主题，Go 模板 + CSS 的组合：

```
themes/simple/
├── index.html      首页 + 分组着陆页（遍历 *Article slice）
├── post.html       单篇文章页
├── login.html      预留（暂未接入）
├── resume.html     预留（暂未接入）
└── css/
    ├── home.css, main.css, prism.css, resume.css   （legacy）
    └── theme.css   （覆盖 + 暗色模式 + 新布局）
```

模板拿到的是嵌入了 `*Article` 的 `articleView` 包装，`Parse` 字段被请求局部覆盖
（避免并发请求改同一个 `Article.Parse`）。新主题可以直接用 `{{.Title}}`、
`{{.URL}}`、`{{.Tags}}`、`{{.Summary}}`、`{{.WordCount}}`、`{{.ReadingTimeMin}}`、
`{{.ViewCount}}`、`{{.Pinned}}`、`{{.Private}}`、`{{.AI}}`、`{{.AILabel}}`、
`{{.CoverURL}}`、`{{.Canonical}}`、`{{.Domain}}`。

`simple` 主题自带：

- 自定义语义化 header（去 Bootstrap、去 jQuery、去 Google Analytics）
- 暗色模式开关，状态写到 `localStorage`，默认尊重 `prefers-color-scheme`
- 阅读进度条 + 自动构建 TOC（≥3 个标题、≥1100px 视口）
- Hero 封面图、OG meta
- 代码块：hover 出 copy 按钮 + 超 20 行自动折叠
- Prism 惰性加载（仅当页面有代码）和 MathJax 惰性加载（仅当出现 `$` / `\(`）
- `@media print` 隐藏 nav / footer / TOC / copy / fold 按钮，展开折叠代码，强制黑白

## 项目结构

```
src/
├── main.go              入口 — 在 Run() 和 Export() 之间分流
├── config/              TOML 加载、flag 解析（-config / -export）
├── article/             Meta + Article + Articles 排序，ParseFile / NewArticle
└── server/
    ├── server.go        HTTP 路由 + Run 主循环 + handlers
    ├── render.go        goldmark + wikilink 预处理 + articleView 包装
    ├── feed.go          atom + sitemap + robots
    ├── export.go        静态导出，withoutPrivate 过滤
    ├── auth.go          HTTP Basic auth helper
    ├── views.go         atomic 浏览量计数 + JSON 持久化
    ├── snippets.go      gist 风格 POST + GET handlers
    ├── watermark.go     图片叠加 + 磁盘缓存
    └── backup.go        周期 tar.gz + 轮转

src/blog/                BlogST、WikiIndex、Reload、fsnotify watcher
themes/simple/           默认主题
conf/config.toml         配置示例
script/export.sh         遗留的 curl 风格导出（保留参考；正解走 Go 子命令）
dockerfile               容器构建（替换 ${YOUR_*} 占位符）
.github/workflows/       CI：master 分支推多架构镜像
```

## 架构说明

`main.go` 故意写得很短：

```go
if config.ExportDir != "" {
    server.Export(config.ExportDir)
} else {
    server.Run()
}
```

启动顺序由 Go 的包 init 链驱动：

1. `config.init` 解析 flag + TOML 写到 `config.C`（`go test` 下跳过）。
2. `blog.init` 构造 `blog.Blog`，调一次 `Reload()`；服务模式下还起 fsnotify watcher
   （`go test` 和 export 模式跳过）。
3. `server.Run()`（服务模式）加载 TLS、组装 mux、阻塞在 `gracehttp.Serve`。

读 `BlogST` 都走带 `RWMutex` 的访问器；`Reload()` 在写锁里整体替换
`articles`/`byTag`/`wiki` 三元组，请求处理只会看到旧或新的完整状态，绝不会看到中间态。

Markdown 渲染按 `*Article` 通过 `sync/atomic.Value` 缓存。热加载会构造新的
`*Article` 指针，所以底层文件改了缓存会自动失效。

## License

MIT（待补充 `LICENSE`；上游仓库目前没有 license 文件，欢迎以 MIT 协议贡献）。
