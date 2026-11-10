# dsh-novel-reader

DSH Web 里的小说面板：**左侧栏图标打开的书库 + 在线阅读器**，与 [LNReaderTUI](../README.md) 终端版**共用同一个数据目录**——也就是同一份 `library.json`、同一个 `books/` 书架、同一份阅读进度。在终端里读到 `第 3/37 章 · 〈露西与爸爸〉 49%`，在浏览器里打开就是同一章同一位置，反之亦然。

搜索 / 详情 / 下载 / 更新交给本机已安装的 `lnreadertui` 命令行完成（同一套站点抓取、限速、EPUB 组装与入库逻辑）；书库读写、EPUB/TXT 解析和进度落盘由插件在 Host 进程内直接完成，文件格式与终端版完全一致。

## 功能

| 区域 | 能力 |
|---|---|
| 书库 | 列出共用书架的每本书：标题、作者、章节数、**章节级进度**（`第 3/37 章 · 〈露西与爸爸〉 49%`）与进度条；按最近阅读排序；本地过滤；导入本地 `.epub` / `.txt`（文件或目录）；更新、重置进度、删除（含文件） |
| 搜索 | 输入即搜（500ms 去抖，过期请求丢弃）；结果显示作者/状态/评分/简介摘要与 `#小说ID`；直接「下载」或进入详情 |
| 详情 | 作者、状态、评分、标签、简介、卷/章结构（按卷折叠，显示每卷章节数与章节标题）；已下载的书显示「已在书库」并可直接阅读或更新 |
| 下载 / 更新 | 后台任务：实时进度（`12/85 · 第二卷 · Case-03`）、可取消、可查看 CLI 日志；完成后自动刷新书库 |
| 界面风格 | **NERV UI**（磷光终端风）：黑底、橙 `#FF9830` 主色，数据绿 `#50FF50` / 线缆青 `#20F0FF` / 警报红 `#FF3030`，等宽字体 + 大写加字距的标签、1px 细边框、警示斜纹、扫描线与网格底。整套配色只作用于 `.nr-page[data-skin="nerv"]`（覆盖 DSH 的 `--dsw-alias-*` 变量），不污染应用其它区域 |
| 阅读设置 | 顶栏 `Aa` 弹出：主题（NERV / 纸白 / 米黄 / 夜蓝）、字体（衬线 / 无衬线 / 楷体 / 等宽）、字号 12–32px、字色与背景色（预设色块 + 系统取色器）；`⛶` 或 `f` 全屏。设置存 `dsh-reader-settings.json`，与 `library.json` 分开，多个浏览器共享 |
| 插图 | 章节正文里的 `<img>` 按原始顺序作为**图块**渲染（`<figure>`），高度上限为一页，超出自动等比缩放；书库/详情用 EPUB 自带的封面；图片按需从书籍文件里读取并缓存（48 MiB LRU），不复制到 dataDir |
| 阅读器 | **纯翻页**：正文按栏排版，一页 = 一屏，不滚动、不会切出半行；左右 1/3 区域点击翻页、滚轮翻页、`←/→`（或 `h/l`、`j/k`、`PageUp/PageDown`）翻页、章末自动进下一章、`n/p` 切章、`s` 章节面板、`Esc` 返回；进度在翻页、切章、每 10s、离开页面时写回 |

阅读进度写入的字段与终端版一致：`chapterIndex`（章节下标）、`offset`（本章内的位置比例 0..1；终端版是滚动比例，这里换算成「第几页/总页数」）、`chapterTitle`、`lastReadAt`（RFC 3339 UTC）。因为章节下标来自与 `internal/epub/reader.go` 逐段对齐的解析实现，两边对「第几章」的理解完全相同。仓库里的 `test/fixtures/` 保存了终端版阅读器对真实书籍的参照输出（每章的标题、段落数与段落哈希），`test/parity.mjs` 会逐章比对。

## 安装

插件依赖本机的 `lnreadertui` 可执行文件（`brew install hhdtc/tap/lnreadertui`、`winget install hhdtc.LNReaderTUI`、发布页二进制或 `go build`）。找不到时面板仍可阅读与整理书库，只有搜索/下载会报错并提示。

**方式一：Web 界面安装（推荐）**

1. 侧边栏 **Plugins**（插件）页面 → 安装本地插件 → 选择本目录（`dsh-plugin/`），或直接粘贴路径。
2. 安装完成后刷新页面，左侧栏出现书本图标 **小说**。

**方式二：命令行**

```bash
dsh plugin --profile web add link:"$PWD"
# 然后在 ~/.dsh/profiles/web/package.json 的 dsh.profile.bundles 里追加 "dsh-novel-reader"
# （或使用 Plugins 页面把它打开）
```

**方式三：手工启用（等价于插件自身的 bundle 层）**

`~/.dsh/profiles/web/cordis.patch.yml` 追加一行覆盖即可，包仍须能被 profile 解析（`dsh plugin --profile web add …`）：

```yaml
- id: ui-novel-reader
  disabled: false
```

配置了 HMR 的 profile（出厂 `web` 模板即是）会在保存后热重组；否则重启 `dsh web`。之后**刷新浏览器页面**即可看到入口。

## 配置（可选）

默认值基本够用；需要覆盖时，写在 profile 的 `cordis.patch.yml` 里（同样是 `ui-novel-reader` 这一行）：

```yaml
- id: ui-novel-reader
  name: dsh-novel-reader
  config:
    dataDir: /home/you/.local/share/lnreadertui   # 书库目录
    bin: /usr/local/bin/lnreadertui               # 命令行路径
```

环境变量优先级更高：`LNREADERTUI_DATA_DIR`、`LNREADERTUI_BIN`。

数据目录默认与终端版一致：`$XDG_DATA_HOME/lnreadertui`，Windows `%LOCALAPPDATA%\lnreadertui`，其余 `~/.local/share/lnreadertui`。所以**不需要任何迁移**：两边打开的是同一批书。

## 快捷键

| 键 | 作用 |
|---|---|
| `←` / `→`（或 `h` / `l`、`j` / `k`、`PageUp` / `PageDown`） | 上一页 / 下一页（到章末再翻就进下一章） |
| `n` / `p` | 下一章 / 上一章 |
| `s` | 展开或收起章节面板 |
| `f` | 进入 / 退出全屏（只放大阅读区） |
| `-` / `=` | 字号 −1 / +1 |
| `Esc` / `q` | 回到书库 |

阅读区鼠标：点击内容区左 1/3 = 上一页，右 1/3 = 下一页；滚轮也翻页。正文不滚动——每页刚好一屏，分栏在「行」边界断开；显示区被裁到正好一栏宽（`.nr-page-clip`），相邻页的栏不会从左右边缘漏出来，所以既不会只露出半行，也不会看到下一栏的字。

阅读外观与全屏：`Aa` 面板里选主题/字体/字号/字色/背景。字号与字体变化会重新分栏（`relayout` 依赖它们），字色/背景只换 CSS 变量、不触发重排；`⛶` 按钮（或 `f`）对 `.nr-reader` 调用 Fullscreen API，全屏下仍是一页一屏、翻页/章节跳转正常，`Esc` 退出。设置通过 `/reader-settings` 持久化，与终端版的库结构互不影响。

插图会参与分页：图片按 `--nr-page-height`（一页正文高度）限制最大高度，加载完成后自动重排一次（140 ms 防抖），并保持 `offset` 指定的比例不变——所以图片多的章节页数会变多，但进度不会漂移。

## 实现

```
dsh-plugin/
├── package.json           # dsh.bundle（bundle 层）+ dsh.client（浏览器入口）
├── cordis.patch.yml       # bundle 层：插入 ui-novel-reader 这一行
├── lib/
│   ├── index.js           # Host 插件：注册 /novel-reader/api 路由（connection 信任栅栏）
│   ├── routes.js          # HTTP 接口：书库/章节/进度/搜索/详情/任务
│   ├── library.js         # 与 terminal 版同格式的 library.json 读写（原子替换 + 串行化）
│   ├── book.js            # ZIP/EPUB/TXT 解析，逐段对齐 internal/epub/reader.go
│   ├── cli.js             # 驱动 lnreadertui：探测、同步调用、可取消的后台任务
│   └── client.js          # 浏览器半边：main 面板 + 侧边栏入口（ModuleLoader bundle）
└── test/                  # 章节对齐、HTTP/书库、jsdom 交互测试
```

Host 半边在 DSH 的 Web 监听器上注册 `/novel-reader/api` 前缀路由，请求先过 `ctx.connection.requestRejection`（与内置路由相同的信任栅栏，只服务本机/受信来源）。它不复制站点抓取逻辑：凡是访问 bilinovel.net 的动作都转发给 `lnreadertui`：

| 动作 | 实现 |
|---|---|
| 搜索 / 详情 | `lnreadertui -data-dir <dir> search <q> --json` / `detail <q> --json` |
| 下载 / 更新 | `lnreadertui -data-dir <dir> download <q> --json` / `update <id|title> --json`（后台任务，解析 stderr 的 `[12/85] 卷 · 章` 进度行，`SIGINT` 干净取消） |
| 导入 | `lnreadertui -data-dir <dir> import <path> --json` |
| 书库列表、进度、重置、删除 | 进程内直接读写同一份 `library.json` 与 `books/`（原子写入，字段与 `internal/model/library.go` 一致） |
| 章节正文 | 进程内解析 EPUB/TXT（结果按 mtime/size 缓存最近 4 本） |
| 插图 / 封面 | 进程内从 EPUB 解压读取（索引按 mtime/size 缓存，48 MiB 图片 LRU），不复制到 dataDir |
| 阅读外观设置 | 进程内读写 `dsh-reader-settings.json`（原子写），与终端版的 `library.json` 完全分开 |

HTTP 接口（全部同源、`no-store`、JSON）：

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/novel-reader/api/status` | 数据目录、CLI 可用性与版本、书数 |
| GET | `/novel-reader/api/books` | 书库（含 `present`、`sizeBytes`、进度摘要） |
| GET | `/novel-reader/api/books/:id/chapters` | 章节目录（解析后的真实章数） |
| GET | `/novel-reader/api/books/:id/chapter/:index` | 一章正文：`paragraphs`（与终端版一致的纯文本）+ `blocks`（阅读顺序，含图块） |
| GET | `/novel-reader/api/books/:id/image?path=<entry>` | 从书籍文件读出插图；`..`/绝对路径 400，非图片或缺失 404 |
| GET | `/novel-reader/api/books/:id/cover` | 书籍自带封面（EPUB `cover-image`） |
| GET · POST | `/novel-reader/api/reader-settings` | 阅读外观设置：GET 返回当前值；POST 合并持久化（非法字体/主题/颜色丢弃，字号夹到 12–32） |
| POST | `/novel-reader/api/books/:id/progress` | 写进度 `{chapterIndex, offset, chapterTitle}` |
| POST | `/novel-reader/api/books/:id/reset` · `/delete` | 重置进度 / 删除书与文件 |
| POST | `/novel-reader/api/import` | `{path}` 导入 |
| GET | `/novel-reader/api/search?q=` · `/detail?q=` | 转发 CLI |
| POST | `/novel-reader/api/download` · `/update` | `{query}` → 任务 |
| GET | `/novel-reader/api/jobs` · `/jobs/:id`；POST `/jobs/:id/cancel` | 任务状态与取消 |

## 测试

```bash
node test/parity.mjs   # 与终端版阅读器逐章逐段比对真实书籍（用本机书库 + test/fixtures）
node test/plugin.mjs   # 书库格式、HTTP 接口、浏览器 bundle 装载与注册
node test/ui.mjs       # 真实 React + jsdom 交互：书库→阅读→翻页→进度→搜索→下载→详情
node test/pagination.mjs  # 真实浏览器分栏校验：页数/栏宽、无纵向溢出、整页无半行、翻页位移、进度换算
```

`test/pagination.mjs` 会启动一个本地 harness（`test/pagination.html`）并尝试拉起 Chromium；没有浏览器时会打印地址，用任意浏览器打开即可，脚本等待页面回报测量结果。

`test/ui.mjs` 需要一次性的开发依赖，不随插件发布：

```bash
npm install --no-save --prefix .reacttest react@18.3.1 react-dom@18.3.1 jsdom@25
node test/ui.mjs
```

## 已知边界

- 面板会显示插图；**终端版仍然是纯文字**（`[图]` 占位），因为终端里画不出位图。
- 站点已从 `bilinovel.com` 迁到 `bilinovel.net`；早期版本的 `lnreadertui` 因为按旧域名跑搜索验证而只能拿到空结果，本仓库的构建已改为跟随实际应答的主机（`internal/site` 的 `guardBase`），搜索/下载均以新域名为准。
- `offset` 是「本章进度比例」：终端版按滚动位置、面板按页码换算，语义一致但像素位置不必相同（视口大小不同，页数也不同）。
- 搜索走站点搜索接口；站点改版或搜索被限流时可能返回 0 结果，此时用小说 ID（如 `4649`）或小说链接仍然可用。
- 进度、书库由终端版与插件共同写入；两者同时运行时最后写入的一方生效（与两个终端版进程并发的行为一致）。
