# 从 Obsidian 到博客：写作与发布手册

本文描述当前实际使用的管线，适用于 `sbraveyoung/blog`、`gobog-obsidian`、`gobog` 和 `sbraveyoung.github.io`。

## 各部分负责什么

```mermaid
flowchart LR
    A[Obsidian blog：Markdown 与附件] -->|插件提交、推送| B[私有 blog 仓库]
    B -->|Notify：blog-update| C[公开站点 Deploy 工作流]
    C --> D[固定版本 gobog：过滤与渲染]
    D --> E[链接检查与 Pagefind 搜索索引]
    E --> F[GitHub Pages：sbrave.cn]
```

Markdown 是内容源。插件负责同步、补全新文章属性、目录开关和本地预览；实际线上渲染在 GitHub Actions 中由 gobog 完成。站点仓库保存配置、工作流和检查脚本；生成结果通过 Pages artifact 部署，不再提交回公开 Git 仓库。每次构建在 Actions Summary 中记录源码 SHA、渲染器 SHA 与主题。修改展示逻辑应该改渲染器、主题或 `_gobog.json`，不要直接修改生成的 HTML。

```text
blog/
├── _folder.md             整个博客的目录开关
├── BLOG_GUIDE.md           本手册；根目录说明不会发布
├── post/                  文章；递归扫描，目录形成分组
│   └── 某个系列/
│       ├── _folder.md     可选；控制这个系列及其子目录
│       └── 一篇文章.md
├── pages/                 独立页面；只扫描直接子文件
│   └── about.md           /about，参与导航与 sitemap
└── resource/image/        图片和附件
```

`.git` 只是指向本机外部 Git 元数据的指针，避免 iCloud 同步 Git 锁与 refs。不要将该指针移动到另一台机器后直接使用；其他机器应重新 clone 并配置自己的 Git 元数据。

## 文章属性

```yaml
---
title: "文章标题"
author: "sbraveyoung"
id: "固定标识"
url: "/post/固定标识"
create_time: "2026-10-12 09:00:00"
updated_time: "2026-10-12 09:00:00"
description: "可选简介；留空时从正文提取摘要"
category: "技术"
tags:
  - Obsidian
  - Go
draft: true
publish: false
hidden: false
private: false
pin: false
ai: false
cover: ""
---
```

| 属性 | 作用 |
| --- | --- |
| `draft` | 草稿开关；`true` 时即使 `publish: true` 也不会公开 |
| `publish` | 发表意图；显式 `false` 不发表；旧文章缺省时兼容历史行为 |
| `hidden` | 暂时下线单篇文章；不会留下可直接访问的公开页面 |
| `private` | 静态 Pages 构建完全排除；动态服务模式才支持 Basic Auth |
| `pin` | 列表置顶 |
| `ai` | `false` 无标记；`assisted` 辅助、`generated` 生成、`edited` 校对；也可填模型名 |
| `cover` | 可选封面图片 |
| `order` | 可选整数，控制系列章节顺序；未设置时也可使用数字文件名前缀 |
| `updated_time` | 作者维护的实际更新时间，用于 Atom 与 sitemap；普通保存不会自动改它 |

布尔字段建议使用不带引号的 `true`、`false`，标签统一为 YAML 列表。已有日期、正文、永久 URL 和自定义属性不会被整理工具改写成其他含义。缺失日期优先从文件 Git 历史补全，历史不可用时才用文件修改时间；历史时间反映仓库记录，不保证等于最初写作日期。保留已有时间的解释方式：上述无时区格式在 feeds 中按 UTC 解析；需要准确时区的项目应另行迁移日期规范。

准备发表时同时设置 `draft: false`、`publish: true`。它还必须满足 `hidden: false`、`private: false`，且没有被任一祖先目录隐藏。新建文章默认是草稿，所以同步到私有仓库不等于公开发表。已有 URL 应持续保留，重命名、移动文章时也不要重新生成。

## 隐藏整个目录

在要控制的文件夹里创建 `_folder.md`：

```markdown
---
hidden: true
---
这个系列暂不公开。
```

规则是：**任何父目录的 `hidden: true` 都是一个否决开关**。例如 `post/系列/_folder.md` 隐藏整个系列，子目录写 `hidden: false` 不能覆盖它。根目录的 `blog/_folder.md` 可以隐藏全部文章和独立页面。设回 `hidden: false` 或删除控制文件，只解除这一级否决，不会修改文章自己的草稿、发表、隐藏、私有属性。

也可以打开该文件夹中的任一笔记，在命令面板执行 Gobog Sync 的 `Hide active note’s folder (including subfolders)`；恢复使用 `Show active note’s folder (parent hiding still applies)`。插件通过 Obsidian 的文件 API 修改控制笔记，不批量改写文章。控制文件本身不会发布或被补全成文章，错误 YAML、缺少 `hidden` 或无效开关会令构建失败。

公开构建会在扫描阶段去掉隐藏目录，从而同时去掉文章页面、分组、导航、标签、归档、wiki 索引、Atom、sitemap 以及这些文章独占的附件。公开文章中的 `[[内部链接]]` 和普通 Markdown 链接指向已知未发表笔记时，会保留文字、去掉链接（含同域名的绝对 URL），因此隐藏一个系列不会因旧内链而卡住部署。真正不存在的 URL 仍会触发链接检查错误。手写 HTML 链接和公开正文里的引用文字不会被自动改写，前者若失效仍需修改。

隐藏对下一次**成功部署后的当前站点**生效，部署失败时线上仍保留上一版。已经公开的内容可能仍在公开 Git 历史、浏览器缓存或搜索引擎中；该机制不能撤销过去的公开记录。

## 日常操作

1. 在 `blog/post/` 写文章，图片放 `resource/image/`，先保持草稿。
2. 执行 `Preview blog locally` 查看实际主题渲染。预览使用临时源副本，并在仅本机可访问的 HTTP 服务中包含草稿和隐藏内容；`private: true` 仍不会导出。预览没有执行 Pagefind，所以完整搜索以线上构建为准。
3. 检查内容、永久链接与图片，维护 `updated_time`，把 `draft` 改成 `false`、`publish` 改成 `true`。
4. 执行 `Push blog repo`，查看差异并提交推送。当前机器 `autoPushOnSave` 关闭，保存本身不会触发推送；可在设置里开启自动同步。
5. 插件状态栏检查新的 Deploy。源码 push、Notify 成功、Pages Deploy 成功是三个不同阶段；以 Pages 成功及页面可访问为发表完成。

插件对自己的同步操作有互斥保护，但跨设备编辑仍可能造成 Git 冲突。先 pull，再编辑和 push；冲突时插件会停止并报告，不自动丢弃任一设备的内容。Token 保存在本机插件 `data.json`，不要提交该文件；它的权限和有效期仍需要账号持有人管理。

## 批量整理与检查

在 gobog 仓库中：

```sh
# 预览，不写入文章
go run ./cmd/frontmatter -source /path/to/blog -author sbraveyoung

# 应用；备份目录必须位于源目录之外、尚不存在
go run ./cmd/frontmatter -source /path/to/blog -author sbraveyoung \
  -apply -backup /path/to/new-backup -report /path/to/report.json

go test -race ./...
go vet ./...

# 导出目录必须为空；避免旧的隐藏页面残留
./gobog -config /path/to/gobog.toml -export /path/to/fresh-output
python3 /path/to/sbraveyoung.github.io/scripts/check-export.py /path/to/fresh-output
```

整理工具先解析所有文章再写入，保留正文原始字节、已有标识和链接、已有非空作者与日期、自定义 YAML 属性、注释；补全核心字段、规范开关与标签列表，并生成完整备份。重复运行应显示 `changed=0`。它只整理实际文章，不修改根目录说明、点文件、模板或 `_folder.md`。

站点通过 `_gobog.json` 的完整 `gobog_ref` 固定渲染器版本；更新 gobog 后需要同步更新这个 SHA，避免本地预览使用新功能而线上使用旧逻辑。源码 PR 预览也读取同一个配置。当前工作流不通过定时任务兜底，正常链路依赖 Notify 的有效 GitHub 凭据；遇到 Notify 失败可查看日志并在站点仓库手动触发 Deploy。

搜索构建只读取主路由的文章与独立页面正文，排除 `__themes` 副本及首页、标签、分组、归档列表，避免同一篇文章重复出现。模板使用 `data-pagefind-body` 标记正文范围，工作流使用 glob 排除主题副本，依据 [Pagefind 索引规则](https://pagefind.app/docs/indexing/) 与 [CLI 配置](https://pagefind.app/docs/config-options/)。
