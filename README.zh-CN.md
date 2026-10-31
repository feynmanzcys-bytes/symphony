# symphony

[English](README.md) | **简体中文** | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

把一群 Claude Code 会话放进一间像素办公室：谁在干活、谁在等你批，一眼就看到；权限审批和 AskUserQuestion 的提问，在网页弹窗里点一下就处理了。

![办公室](docs/office.png)

## 能做什么

- **每个会话就是一个对话窗**：点一下员工，页面中间打开它的窗口：上面是员工信息，下面是整个会话的聊天流：输入过的提示词、模型的思考（默认折叠，进行中显示橘黄色“思考中…”）、逐字流出的回答、工具调用，以及其它会话发来的消息（紫色）。在底部输入就是给这个会话发提示词，“停止”按钮能打断它。
- **网页审批**：会话要执行命令、改文件时的权限审批，还有 AskUserQuestion 的提问，会以卡片形式从该员工窗口底部弹出、盖住输入框，顶栏的红色计数能直接跳过去。bridge 没开或者不认账号，就退回终端原样处理，不会卡住会话。
- **像素办公室**：每个会话是一个坐固定工位的员工。绿色空闲、黄色工作中、红色等你审批或回答、灰色下班；闲着的先在工位上玩手机、打游戏，过一会儿就满办公室溜达：窝在休息区沙发上看电视、玩桌上足球、追逐打闹、凑到圆桌聊天、接咖啡、看窗外，闲久了去打盹，多半躲进会议室睡；块头看模型，肌肉看思考强度。画面是大公司的某一层楼，铺满窗口：开放办公区在正中，左边是挂着铭牌的 3–5 人小会议室，右边是大会议室和休息区，大家坐电梯上下班。顶部横栏里还能调像素大小，分四档：楼不变，越往细调画得越细、细节越多。
- **一搜就找到，一点就回来**：点一下闲着的员工，它就回到自己的工位，一直待到下次干活；顶部横栏里的搜索框按名字或目录找人（按 `/` 直接跳过去）；点它旁边的状态牌（空闲、工作中、待处理、下班），这一类的人就都标出来了；横栏下面的小把手能把它收上去、再拉下来。
- **会话间消息**：会话之间互发消息（SendMessage）时有信封飞过去，还有消息记录。
- **在页面上开会话，不弹任何窗口**：点空座位上的“+”招新员工，选目录、模型、思考强度、权限模式（新员工默认全部放行、Opus、最高思考强度）；下班的员工点“上班”，回到原来的座位。会话跑在 bridge 内部自己的终端里，不会弹窗、不会抢走你的注意力；想看终端时点员工窗口里的“打开终端”，关掉那个窗口也不影响会话。
- **10 种语言**：English、简体中文、繁體中文、日本語、한국어、Español、Français、Deutsch、Português (Brasil)、Русский。页面跟着浏览器的语言走，顶部横栏里可以切换；bridge 的提示跟着系统语言走（也可以用 `-lang` 指定）；mod 的提示跟着启动这个会话的页面走。
- **全在本机**：bridge 是一个 Go 程序（只用标准库），网页是原生 JS，不依赖任何第三方平台。

![网页审批](docs/approve.png)

## 需要什么

- Claude Code 2.1.314（在这个版本上验证过）。mod 用的 function hooks 还是早期接口，升级 Claude Code 后最好先跑一遍端到端测试。
- bridge 有 Linux、macOS、Windows 的现成程序，下载就能用；只有从源码跑才要 Go 1.27 以上。
- 不需要别的：页面开的会话在每个平台上都跑在 bridge 内部。

## 安装

### 1. 装 mod

在 Claude Code 里输入：

```
/plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

先按 `y` 添加插件市场，再选安装范围（用默认的用户级别）。也可以在命令行里装：

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

已经 clone 了仓库的话，也可以拿本地仓库当插件市场。这样 mod 和 bridge 是同一份代码，`git pull` 之后新开的会话就是新版：

```bash
claude plugin install symphony-bridge --marketplace /path/to/symphony
```

mod 装在用户级别，每个 Claude Code 会话都会加载它，但它只管带着 `SYMPHONY_TOKEN` 启动的会话，别的会话里什么都不做。

更新：`claude plugin update symphony-bridge@symphony`；卸载：`claude plugin uninstall symphony-bridge@symphony`。

### 2. 起 bridge

从[最新 Release](https://github.com/feynmanzcys-bytes/symphony/releases/latest) 下载你系统对应的压缩包，解压后运行里面的 `symphony`：

| 系统 | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz)（Intel） | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz)（Apple 芯片） |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

比如在 Linux 上：

```bash
curl -fLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz
tar xzf symphony_linux_amd64.tar.gz
./symphony
```

Windows 上解压后运行 `symphony.exe`；双击的话它会开一个自己的窗口，关掉窗口 bridge 就停了。

程序没有签名，第一次运行可能被拦。用浏览器下载的话，macOS 可能提示“无法验证开发者”：到“系统设置 → 隐私与安全性”里点“仍要打开”，或者运行一次 `xattr -d com.apple.quarantine symphony`。Windows 的 SmartScreen 也可能拦一下：点“更多信息”，再点“仍要运行”。Release 里的 `checksums.txt` 有每个压缩包的 SHA-256。

程序和 mod 用同一个版本号：`symphony -version` 看程序的，`claude plugin list` 看 mod 的。更新就是下载新版的压缩包，再运行 `claude plugin update symphony-bridge@symphony`。每个版本改了什么，见 [CHANGELOG.md](CHANGELOG.md)。

也可以从源码跑（要 Go 1.27 以上）：

```bash
git clone https://github.com/feynmanzcys-bytes/symphony.git
cd symphony
go run ./cmd/bridge
```

bridge 一启动就会在浏览器里打开办公室 <http://127.0.0.1:7788/>（不想要就加 `-open=false`）；<http://127.0.0.1:7788/approve> 是一个只管审批的简单页面。bridge 已经在跑时再启动一次，只会打开办公室。

| 参数 | 默认 | 说明 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | 监听地址。网页还没有登录，别绑到本机以外的地址 |
| `-data` | `$SYMPHONY_DATA`，没有就用系统配置目录下的 `symphony` | 放账号（含 token）和消息记录 |
| `-claude` | `claude` | claude 可执行文件 |
| `-lang` | 系统语言 | bridge 提示用的语言 |
| `-mod` | 空 | 开发用：用 `--plugin-dir` 带上这个目录里的 mod，盖过装好的版本 |
| `-web` | 空 | 开发用：直接读这个目录里的页面，改了刷新就生效 |
| `-open` | `true` | bridge 启动后在浏览器里打开办公室；`-addr` 上已经有 bridge 在跑的话，只打开它的办公室 |
| `-autostart` | — | `on`：以后每次登录都自动启动 bridge（带上一起给的参数），并且马上启动；`off`：取消 |
| `-background` | — | 在后台启动 bridge：脱离终端，日志写到数据目录的 `bridge.log`，等它能响应了就返回 |
| `-version` | — | 显示版本号后退出 |

**随登录启动**：`symphony -autostart on` 让 bridge 以后每次登录都自动启动（一起给的参数也会带上，比如 `-addr`），并且马上启动；`symphony -autostart off` 取消。Linux 上是 systemd 用户服务（日志用 `journalctl --user -u symphony` 看），macOS 上是 launchd 代理（日志在 `~/Library/Logs/symphony.log`），Windows 上是注册表 Run 键里的一项（bridge 跑在一个控制台窗口里，关掉窗口它就停到下次登录）。这要用 symphony 程序本身，`go run` 不行。

### 3. 开会话

**在页面上开**：点空座位上的“+”，填名字，选目录和参数。bridge 在自己内部的终端里跑 `claude`，不会弹出窗口。员工窗口以聊天形式展示会话；“打开终端”按钮会开一个连到该会话的终端窗口（在任何终端里执行 `symphony attach <编号>` 也一样；按 `Ctrl-]` 或关窗即断开，会话照常运行）。这些会话是 bridge 的子进程：停掉 bridge 会话也会结束。

**在命令行开**：先建一个账号拿 token，再带着 token 起 `claude`：

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"前端小王"}'   # 返回里有 token
SYMPHONY_TOKEN=<token> claude --name 前端小王 --permission-mode manual
```

权限模式建议用 `manual`：`auto` 模式下大部分操作不问人，审批也就到不了网页。bridge 不在默认地址的话，再加上 `SYMPHONY_BRIDGE=http://地址:端口`。`SYMPHONY_LANG` 决定 mod 提示用的语言。

本机上 bridge 的地址没有响应时，会话会自己用 `symphony -background` 把 bridge 拉起来（前提是 `symphony` 在 PATH 里），会话结束后 bridge 还会继续跑。账号不在默认数据目录的话，设好 `SYMPHONY_DATA`。

## 账号和座位

- 在 symphony 里，一个身份就是名字 + token + 固定座位。一个账号同一时间只能给一个会话用，第二个会话会被拒，审批退回终端。
- Claude 的登录不归它管，所有会话可以共用一个 Claude 账号。
- 数据放在 `~/.config/symphony`（Linux）、`~/Library/Application Support/symphony`（macOS）、`%AppData%\symphony`（Windows）。`accounts.json` 里有 token，只有自己能读（0600）；Windows 上靠用户目录的权限保护。

## 安全

- bridge 只认发给本机的请求，还会拒掉别的网站借你的浏览器发来的写请求，因为页面上能开会话，等于能在本机执行命令。
- 网页还没有登录功能，**不要**把 bridge 绑到本机以外的地址。

## 平台

| 系统 | 页面开的会话 | “打开终端” | 验证情况 |
| --- | --- | --- | --- |
| Linux | bridge 内部的伪终端 | 图形终端（x-terminal-emulator、gnome-terminal、konsole、xterm）运行 `symphony attach` | 跑过完整的端到端测试 |
| macOS | bridge 内部的伪终端 | Terminal.app 运行 `symphony attach` | 跑过完整的端到端测试 |
| Windows | bridge 内部的伪控制台（ConPTY） | 新开控制台窗口运行 `symphony attach` | 只做了交叉编译和单测 |

每个平台上都由 bridge 替你点“信任此目录”。

## 开发

```bash
go test -race ./...                         # bridge 和翻译的单测
claude plugin test mods/symphony-bridge     # mod 单测
claude plugin validate .                    # 校验插件市场和 mod
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # 端到端：起真会话，花 token
```

- 改 mod：`go run ./cmd/bridge -mod mods/symphony-bridge`，页面上开的会话会用 `--plugin-dir` 带上你手上这份 mod；命令行开的会话自己加 `--plugin-dir mods/symphony-bridge`。
- 改页面：`go run ./cmd/bridge -web web`，刷新就生效。
- 不开真会话看效果：另起一个 bridge 灌假数据，`go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo`，再 `go run ./cmd/demo -base http://127.0.0.1:7799`。
- 翻译：每种语言一个文件，页面的在 `web/locales/<语言>.json`，bridge 的在 `internal/i18n/locales/<语言>.json`，键和 `en.json` 一样；mod 那几句提示在 `mods/symphony-bridge/hooks/messages.ts`。单测会检查每种语言的键是不是齐全、`{占位符}` 对不对得上；端到端测试 `TestI18n` 会用每种语言打开页面。
- 端到端测试需要 claude、tmux（测试自己驱动的会话用）和 Chromium 或 Chrome，所以在 Linux 和 macOS 上跑。默认用仓库里的 mod；设了 `SYMPHONY_E2E_MOD=installed` 就改用装好的 mod。截图在 `e2e/artifacts/`。
- 发版：在 main 的提交上打版本标签并推送（`git tag -a v0.3.0 -m v0.3.0 && git push origin v0.3.0`），GitHub Actions 就会给所有平台编好程序、发布 Release（`.github/workflows/release.yml`）。标签必须和 `plugin.json` 里 mod 的版本号一致。`.github/release.sh v0.3.0 dist` 能在本地打出同样的包。每个版本都要在 `CHANGELOG.md` 里写一节，它会放在发布说明的最前面。

| 目录 | 是什么 |
| --- | --- |
| `mods/symphony-bridge/` | Claude Code mod（TypeScript） |
| `.claude-plugin/marketplace.json` | 插件市场，`/plugin install` 从这里找 mod |
| `cmd/bridge/`、`internal/bridge/` | bridge |
| `internal/i18n/` | 识别语言，以及 bridge 的翻译 |
| `web/` | 办公室和审批页面及其翻译，编译进 bridge |
| `cmd/demo/` | 灌假数据 |
| `e2e/` | 端到端测试 |

## 许可证

[MIT](LICENSE)
