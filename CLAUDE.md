# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 硬性规定

1. 非必要情况下，用精炼的简体中文大白话。
2. 没有完整的测试验收，不许提交，不许推送。
3. 凡是涉及代码的改动，必须在本地新开 git branch + worktree 里做；得到用户确认后，才能在本地合并回 `dev`。

## 项目是什么

symphony：用 Claude Code 对标 grok bot 的试验仓库。独立开发，不参考其它仓库。

## 代码现状

bridge 用 Go（只用标准库，e2e 另用 chromedp），mod 是 TypeScript（Claude Code 规定），网页是原生 JS，办公室用 Canvas 2D 画像素图，不依赖任何前端库。

- **账号**：军团里的身份 = 名字 + token + 固定座位，一个账号同一时间只给一个会话用（第二个会话 409，退回终端）。Claude 登录不管，所有会话可以共用。账号存在数据目录 `accounts.json`（含 token，权限 0600），消息记录存 `messages.json`。
- `mods/symphony-bridge/`：Claude Code mod（function hooks，2.1.314 上验证过；API 标着 EARLY ACCESS，升级 Claude Code 要先跑 e2e）。凭 `SYMPHONY_TOKEN` 认账号，没带 token 的会话里什么都不做（不连 bridge、不弹提示、不碰审批）；`tool.check` 把权限审批转给 bridge，`tool.call` 拦 AskUserQuestion；上报会话事件、会话名、会话间消息、模型和思考强度（`turn.step`），每 15 秒一次心跳。bridge 不通、不认 token 或账号被占用，就弹个提示、退回终端原生交互。单测在 `tests/`。
- 正式安装：仓库根目录的 `.claude-plugin/marketplace.json` 是插件市场（名字 `symphony`），`claude plugin install symphony-bridge --marketplace <本地仓库路径 或 feynmanzcys-bytes/symphony>`，装在用户级别，每个会话都加载。本地路径当市场时，会话直接读那个目录（改了 mod，下个会话或 `/reload-plugins` 就生效）；从 GitHub 装的是安装时的拷贝，要改 `plugin.json` 的版本号再 `claude plugin update`。改了 mod 的行为就升版本号。
- `internal/bridge/`：`hub.go` 状态中枢（45 秒没心跳算下班）、`accounts.go` 账号和座位、`server.go` HTTP（长轮询给 mod，SSE 推给网页）、`launch.go` 起会话、`terminal*.go` 各平台怎么开终端。
- 三端：Windows（新控制台窗口直接起 claude，`terminal_windows.go`）、macOS（默认 Terminal.app，跑一个先删掉自己的一次性脚本；`-terminal tmux` 可改用 tmux）、Linux（有 tmux 用 tmux，没有就开图形终端）。只有 tmux 能替人点“信任此目录”，弹窗口的后端第一次进新目录要人自己点。数据目录用 `os.UserConfigDir()`：`~/.config/symphony`、`~/Library/Application Support/symphony`、`%AppData%\symphony`（Windows 上文件权限 0600 不起作用，token 靠用户目录的权限保护）。
- `web/`：`office.html`（`/`，像素办公室）、`approve.html`（`/approve`，备用审批台）、`asks.js`（两边共用的审批卡片），都编译进 bridge 二进制。
- 办公室：正规大公司布局——开放办公区（6 列一排，至少 3 排，人少也摆满桌子，人多往下加排）、玻璃会议室、休息区（吧台、冰箱、售货机、沙发、桌上足球）、前台；每个账号一个固定工位；状态色空闲绿、工作中黄、待审批/待回答红、下班灰；空闲时玩手机/打游戏，闲 2 分钟睡觉；块头看模型（haiku < sonnet < opus < fable），肌肉看思考强度（max 系红头带）；会话间消息有飞信封 + 气泡 + 消息记录；待处理事项自动弹窗，直接在弹窗里处理。
- 页面启动会话：空座位的“+”招新员工，下班员工的卡片上点“上班”（带出上次的目录和参数，坐回原座位）；bridge 在那个目录的 tmux（`sym-<编号>`）里起 claude，带上这个账号的 token 和 bridge 地址（mod 用装好的，`-mod` 开发模式才加 `--plugin-dir`）；新目录的“信任此目录”由 bridge 代点。bridge 用 `claude plugin list --json` 查 mod 装没装（`launch.go` 的 `Mod`），没装就关掉页面启动，页面左上角给出安装命令，装好刷新就行，不用重启 bridge。
- bridge 只认本机 Host，并拒绝别的网站借浏览器发来的写请求（启动接口能在本机执行命令，要防跨站）。网页本身还没有登录，不要绑到非本机地址。
- `cmd/bridge`、`cmd/demo`（建 demo 账号灌假数据）。`e2e/`：真起 Claude 会话的端到端验收。

## 常用命令

- 起 bridge（在仓库根目录）：`go run ./cmd/bridge`（`-addr`、`-data`、`-claude`、`-terminal auto|tmux|window`；改页面时加 `-web web` 直接读磁盘，刷新就生效；改 mod 时加 `-mod mods/symphony-bridge`，页面起的会话用 `--plugin-dir` 带上手上这份，盖过装好的；缺 claude 或终端时页面启动关闭）
- 命令行手动起会话：先 `curl -X POST localhost:7788/api/accounts -d '{"name":"某某"}'` 拿 token，再 `SYMPHONY_TOKEN=<token> claude --name 某某 --permission-mode manual`（mod 要先装好，改 mod 时加 `--plugin-dir mods/symphony-bridge`；auto 模式大部分不问人，审批到不了网页）
- 校验：`claude plugin validate .`（插件市场）、`claude plugin validate mods/symphony-bridge`；mod 单测：`claude plugin test mods/symphony-bridge`
- 单测：`go test -race ./...`
- 三端交叉编译检查（改了 Go 代码都要跑）：`for os in windows darwin linux; do GOOS=$os go vet ./internal/... ./cmd/... && GOOS=$os go build -o /dev/null ./cmd/bridge; done`
- 端到端（起真会话、花 token，约半分钟到一分钟）：`SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/`，截图在 `e2e/artifacts/`；需要 `claude`、`tmux`、`chromium`；e2e 固定用 tmux 后端，只在 Linux / 装了 tmux 的 macOS 上跑，Windows 上会跳过。`TestLegion` 默认用 `--plugin-dir` 带仓库里的 mod，`SYMPHONY_E2E_MOD=installed` 改用装好的 mod（要先装）；`TestInstall` 在临时的 `CLAUDE_CONFIG_DIR` 里走一遍安装，不花 token、不碰本机配置
- 假数据看办公室：另起一个 bridge（`-addr 127.0.0.1:7799 -data /tmp/x`），`go run ./cmd/demo -base http://127.0.0.1:7799`

## 分支

`dev` 是开发主线，代码改动按硬性规定第 3 条走。
