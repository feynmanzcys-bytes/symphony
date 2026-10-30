# symphony

[English](README.md) | **简体中文** | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

把一群 Claude Code 会话放进一间像素办公室：谁在干活、谁在等你批，一眼就看到；权限审批和 AskUserQuestion 的提问，在网页弹窗里点一下就处理了。

![办公室](docs/office.png)

## 能做什么

- **网页审批**：会话要执行命令、改文件时的权限审批，还有 AskUserQuestion 的提问，都转到网页上处理。bridge 没开或者不认账号，就退回终端原样处理，不会卡住会话。
- **像素办公室**：每个会话是一个坐固定工位的员工。绿色空闲、黄色工作中、红色等你审批或回答、灰色下班；闲着的先在工位上玩手机、打游戏，过一会儿就满办公室溜达：窝在休息区沙发上看电视、玩桌上足球、追逐打闹、凑到圆桌聊天、接咖啡、看窗外，闲久了去打盹，多半躲进会议室睡；块头看模型，肌肉看思考强度。
- **一搜就找到，一点就回来**：点一下闲着的员工，它就回到自己的工位，一直待到下次干活；标题旁边的搜索框按名字或目录找人（按 `/` 直接跳过去）。
- **会话间消息**：会话之间互发消息（SendMessage）时有信封飞过去，还有消息记录。
- **在页面上开会话**：点空座位上的“+”招新员工，选目录、模型、思考强度、权限模式；下班的员工点“上班”，回到原来的座位。
- **10 种语言**：English、简体中文、繁體中文、日本語、한국어、Español、Français、Deutsch、Português (Brasil)、Русский。页面跟着浏览器的语言走，左上角可以切换；bridge 的提示跟着系统语言走（也可以用 `-lang` 指定）；mod 的提示跟着启动这个会话的页面走。
- **全在本机**：bridge 是一个 Go 程序（只用标准库），网页是原生 JS，不依赖任何第三方平台。

![网页审批](docs/approve.png)

## 需要什么

- Claude Code 2.1.314（在这个版本上验证过）。mod 用的 function hooks 还是早期接口，升级 Claude Code 后最好先跑一遍端到端测试。
- Go 1.27 以上，用来跑 bridge。
- 想在页面上开会话的话：Linux 和 macOS 最好装 tmux。

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

```bash
git clone https://github.com/feynmanzcys-bytes/symphony.git
cd symphony
go run ./cmd/bridge
```

打开 <http://127.0.0.1:7788/> 就是办公室。<http://127.0.0.1:7788/approve> 是一个只管审批的简单页面。

| 参数 | 默认 | 说明 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | 监听地址。网页还没有登录，别绑到本机以外的地址 |
| `-data` | 系统配置目录下的 `symphony` | 放账号（含 token）和消息记录 |
| `-terminal` | `auto` | 页面开会话用什么终端：`auto`、`tmux`、`window` |
| `-claude` | `claude` | claude 可执行文件 |
| `-lang` | 系统语言 | bridge 提示用的语言 |
| `-mod` | 空 | 开发用：用 `--plugin-dir` 带上这个目录里的 mod，盖过装好的版本 |
| `-web` | 空 | 开发用：直接读这个目录里的页面，改了刷新就生效 |

### 3. 开会话

**在页面上开**：点空座位上的“+”，填名字，选目录和参数。bridge 会开一个终端跑 `claude`；用 tmux 的话，会话名是 `sym-<编号>`，`tmux attach -t sym-<编号>` 就能进去看。

**在命令行开**：先建一个账号拿 token，再带着 token 起 `claude`：

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"前端小王"}'   # 返回里有 token
SYMPHONY_TOKEN=<token> claude --name 前端小王 --permission-mode manual
```

权限模式建议用 `manual`：`auto` 模式下大部分操作不问人，审批也就到不了网页。bridge 不在默认地址的话，再加上 `SYMPHONY_BRIDGE=http://地址:端口`。`SYMPHONY_LANG` 决定 mod 提示用的语言。

## 账号和座位

- 在 symphony 里，一个身份就是名字 + token + 固定座位。一个账号同一时间只能给一个会话用，第二个会话会被拒，审批退回终端。
- Claude 的登录不归它管，所有会话可以共用一个 Claude 账号。
- 数据放在 `~/.config/symphony`（Linux）、`~/Library/Application Support/symphony`（macOS）、`%AppData%\symphony`（Windows）。`accounts.json` 里有 token，只有自己能读（0600）；Windows 上靠用户目录的权限保护。

## 安全

- bridge 只认发给本机的请求，还会拒掉别的网站借你的浏览器发来的写请求，因为页面上能开会话，等于能在本机执行命令。
- 网页还没有登录功能，**不要**把 bridge 绑到本机以外的地址。

## 平台

| 系统 | 页面开会话用的终端 | 验证情况 |
| --- | --- | --- |
| Linux | 有 tmux 用 tmux，没有就开图形终端 | 跑过完整的端到端测试 |
| macOS | 默认 Terminal.app，`-terminal tmux` 改用 tmux | 只做了交叉编译和单测 |
| Windows | 新开一个控制台窗口 | 只做了交叉编译和单测 |

只有 tmux 能替你点“信任此目录”。弹窗口的话，第一次在新目录开会话，要你自己在窗口里点一下。

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
- 端到端测试需要 claude、tmux、chromium，所以只在 Linux 和装了 tmux 的 macOS 上跑。默认用仓库里的 mod；设了 `SYMPHONY_E2E_MOD=installed` 就改用装好的 mod。截图在 `e2e/artifacts/`。

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
