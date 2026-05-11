# gobog themes — letter / tufte / press

三套可直接用作 gobog 主题的目录。每个目录就是一个独立主题。

## 结构

```
gobog-themes/
├── letter/        # 极简文人风（奶油纸 + Newsreader serif）
│   ├── index.html
│   ├── post.html
│   └── css/theme.css
├── tufte/         # 学术笔记风（侧栏 marginalia + IBM Plex Serif）
│   ├── index.html
│   ├── post.html
│   └── css/theme.css
└── press/         # 杂志编辑风（DM Serif Display + 双栏正文）
    ├── index.html
    ├── post.html
    └── css/theme.css
```

## 用法（gobog）

1. 把你想用的主题目录拷到你 gobog 项目的 `themes/` 下，例如：
   ```
   cp -r letter /path/to/your-blog/themes/letter
   ```
2. 在 gobog 配置中把 `theme` 改成对应名字（`letter` / `tufte` / `press`）。
3. 重新生成站点。CSS 在 `<theme>/css/theme.css`，按 gobog 的资源拷贝规则会被发布到站点根 `/css/theme.css`。

## 模板里用到的字段

模板按你贴的 gobog 风格写（Go `text/template`，`{{.Field}}`）。常用字段：

**列表页 (`index.html`)**

- `.Site.Title` `.Site.Author` `.Site.Description` `.Site.Domain`
- `.Articles` — 文章数组；每篇有 `.Title .URL .Summary .CreateTime .Tags .ReadingTimeMin .WordCount .Pinned .AI .AILabel .CoverURL`
- `.Tag` / `.Group` — 在 tag 或分组列表页时
- press 主题还会用 `.Feature`（头条）、`.Digest`（次条 3 篇）、`.Rest`（剩余）、`.Year`、`.Now` —— 如果你的 gobog 没有这些字段，把对应模板段简化为遍历 `.Articles` 即可。

**文章页 (`post.html`)**

- `.Title .Summary .Parse`（渲染好的 HTML 正文）
- `.CreateTime .ReadingTimeMin .WordCount .ViewCount .Tags .CoverURL .Canonical`
- `.AI .AILabel`（如果你启用了 AI 摘要标记）

字段名与你给的 gobog 文档对齐；如果实际名字不同（比如 `.Content` 而不是 `.Parse`），全局替换一下即可。

## 字体

所有主题都从 Google Fonts 拉字体（Newsreader / IBM Plex Serif / DM Serif Display / JetBrains Mono / Noto Serif SC 等），并预连了 `fonts.googleapis.com`。需要离线时把字体下到 `<theme>/fonts/` 并改 `@font-face` 即可。

## 暗色模式

三套主题都用 `@media (prefers-color-scheme: dark)` 自动切换，无需 JS。

## 预览

仓库里的 `Themes Canvas.html` 是 5 个方向的 design canvas 概览，可点 ⤢ 进任意一张全屏对比。`letter / tufte / press` 三个目录是其中三个方向落地后的 gobog 模板代码。
