# symphony

[English](README.md) | **简体中文** | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

把一群 Claude Code 会话放进一间像素办公室：谁在干活、谁在等你，一眼就看到；每个会话都能在自己的窗口里对话，权限审批和 AskUserQuestion 的提问也在那里点一下就处理了。

![办公室](docs/office.png)

## 安装

最快的办法：打开 Claude Code，粘贴这句话

> 帮我安装 https://github.com/feynmanzcys-bytes/symphony

Claude Code 会读这份 README，把下一节的事全部做完。想自己动手装，照着同样的步骤做就行。

### 给 AI agent 看的：安装手册

你读到这里，是因为用户让你安装 symphony。在 shell 里按顺序做完下面的步骤，最后按第 6 步向用户汇报。除非某一步要求，否则什么都别问用户；常规的选择自己定，在汇报里列出来。

symphony 分两部分，两部分都要装：

1. **`symphony` 程序**（bridge）：一个单独的可执行文件，是本机上的 web 服务器，地址 `http://127.0.0.1:7788/`，提供办公室页面、运行各个会话。它必须在用户的 `PATH` 里，名字就叫 `symphony`，因为会话找不到在跑的 bridge 时，mod 会用 `symphony -background` 把它拉起来。
2. **`symphony-bridge` 插件**（mod）：本仓库插件市场里的一个 Claude Code 插件，装在用户级别。它只管带着 `SYMPHONY_TOKEN` 启动的会话，别的会话里什么都不做。

#### 第 1 步：检查机器

```bash
uname -sm                 # Linux/macOS：据此选下面的压缩包（x86_64 → amd64；arm64/aarch64 → arm64）
claude --version          # 要 Claude Code 2.1.x（在 2.1.316 上验证过）
git --version             # 只有协作任务要用；没有的话在汇报里写明，不用停下
symphony -version         # 已经装了？会输出 "symphony v0.8.0 (darwin/arm64)"
curl -s http://127.0.0.1:7788/api/state | head -c 40   # 已经有 bridge 在跑？会输出 {"type":"snapshot",...
claude plugin list --json # mod 已经装了？找 "id":"symphony-bridge@symphony"
```

Windows 上用 PowerShell：`$env:PROCESSOR_ARCHITECTURE` 是 `AMD64` 或 `ARM64`，用 `Invoke-WebRequest http://127.0.0.1:7788/api/state` 代替 curl。

没有 `claude` 的话，停下来，告诉用户先装 Claude Code。程序和 mod 都已经装好的话，改去看下面的“更新”。

#### 第 2 步：拿到程序

用下面第一个能成功的办法：

- **A. 用 `gh`**（仓库还是私有的时候也能用，只要 `gh auth status` 显示已登录）：
  ```bash
  gh release download -R feynmanzcys-bytes/symphony --pattern 'symphony_<os>_<arch>.tar.gz' -D /tmp/symphony-dl
  ```
- **B. 用 `curl`**（仓库公开之后才行；返回 404 就改用 A 或 C）：
  ```bash
  curl -fL -o /tmp/symphony-dl/symphony_<os>_<arch>.tar.gz https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_<os>_<arch>.tar.gz
  ```
- **C. 从源码编译**（要 Go 1.27 以上；第 4 步还会用到这份 clone）：
  ```bash
  git clone https://github.com/feynmanzcys-bytes/symphony.git /tmp/symphony-src
  cd /tmp/symphony-src && go build -o symphony ./cmd/bridge
  ```
  要编出可执行文件；`go run` 不行，因为 `-autostart` 不认 `go run` 跑起来的程序。

| 系统 | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz)（Intel） | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz)（Apple 芯片） |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

每个压缩包里有程序（`symphony`，Windows 上是 `symphony.exe`）、`LICENSE` 和 `README.md`。Release 里的 `checksums.txt` 有每个压缩包的 SHA-256（`sha256sum -c --ignore-missing checksums.txt`，macOS 上用 `shasum -a 256 -c`）。

#### 第 3 步：放进 PATH

选一个已经在用户 `PATH` 里、不用 `sudo` 就能写的目录，按这个顺序找：`~/.local/bin`、`/usr/local/bin`、`/opt/homebrew/bin`。一个都没有的话，建一个 `~/.local/bin`，把它加进 shell 的 rc 文件（`~/.zshrc` 或 `~/.bashrc`），并在汇报里说明。

```bash
tar xzf /tmp/symphony-dl/symphony_<os>_<arch>.tar.gz -C /tmp/symphony-dl
install -m 755 /tmp/symphony-dl/symphony ~/.local/bin/symphony
xattr -d com.apple.quarantine ~/.local/bin/symphony 2>/dev/null || true   # macOS：程序没有签名
symphony -version          # 输出 "symphony v0.8.0 (linux/amd64)"：版本号和平台
```

Windows 上：`Expand-Archive symphony_windows_<arch>.zip -DestinationPath "$env:LOCALAPPDATA\Programs\symphony"`，然后把这个目录加进用户的 `Path`（`[Environment]::SetEnvironmentVariable('Path', "$([Environment]::GetEnvironmentVariable('Path','User'));$env:LOCALAPPDATA\Programs\symphony", 'User')`），再开一个新的 shell。第一次运行时 SmartScreen 可能会警告：点“更多信息”，再点“仍要运行”。

做完这些，你自己的 shell 还是找不到 `symphony` 的话（它的 `PATH` 是改之前读的），后面的步骤就用绝对路径。

#### 第 4 步：装 mod

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

这会把本仓库添加为插件市场 `symphony`，并把 mod 装在用户级别，中间没有任何提示。它用 git clone 仓库，所以仓库还是私有的时候，需要能用 git 访问 GitHub；要是因为这个失败了，就改用一份 clone 当插件市场（第 2 步 C 的那份，或者新 clone 一份）：`claude plugin install symphony-bridge --marketplace /absolute/path/to/symphony`。用 clone 当插件市场时，会话直接从那个目录读 mod，在那里 `git pull` 就能更新。

检查：

```bash
claude plugin list --json   # 其中一项有 "id":"symphony-bridge@symphony", "scope":"user", "enabled":true
```

- `"scope":"project"` 或 `"local"`：mod 只会在一个目录里加载。先运行 `claude plugin uninstall symphony-bridge@symphony`，再加上 `--scope user` 重新安装。
- `"enabled":false`：运行 `claude plugin enable symphony-bridge@symphony`。

#### 第 5 步：起 bridge

```bash
symphony -autostart on -open=false
```

这会马上启动 bridge，以后每次登录也会自动启动（Linux 上是 systemd 用户服务，macOS 上是 launchd 代理，Windows 上是注册表 Run 键里的一项），并输出

```
symphony now starts when you log in (<where>); it's running at http://127.0.0.1:7788/
```

如果用户说过不想随登录启动，就改用 `symphony -background -open=false`：它在后台启动 bridge，脱离终端，并输出 `the symphony bridge runs in the background at http://127.0.0.1:7788/; its log: <data folder>/bridge.log`。已经有 bridge 在跑时，两条命令都会回答 `a symphony bridge is already running at http://127.0.0.1:7788/`；这没问题，别再起一个。

用一个请求检查整条链路：

```bash
curl -s http://127.0.0.1:7788/api/launch/options
```

返回里有 `"enabled":true` 和 `"mod":{"ok":true,"id":"symphony-bridge@symphony","version":"0.8.0"}`：说明 bridge 在跑、找到了 `claude`，mod 也装在会话会加载的地方。如果是 `"enabled":false`，`mod.code` 说明哪里不对，`mod.fix` 给出修复的命令：`mod_not_installed` 或 `mod_project_scope`（回到第 4 步），`mod_disabled`（启用它），`mod_check_failed`（`claude plugin list --json` 失败了，多半是 Claude Code 太旧）。`"mod":null` 说明 bridge 在它的 `PATH` 里根本没找到 `claude`（日志里会写）：把它放进去，或者用 `-claude /path/to/claude` 启动 bridge。

bridge 的日志是数据目录里的 `bridge.log`（Linux 上是 `~/.config/symphony`，macOS 上是 `~/Library/Application Support/symphony`，Windows 上是 `%AppData%\symphony`）；随登录启动的话，Linux 上日志用 `journalctl --user -u symphony` 看，macOS 上在 `~/Library/Logs/symphony.log`。

#### 第 6 步：向用户汇报

在浏览器里打开办公室（macOS 上用 `open http://127.0.0.1:7788/`，Linux 上用 `xdg-open`，Windows 上用 `start`），然后告诉用户：

- 办公室在 `http://127.0.0.1:7788/`。点空座位上的“+”招第一个员工：选一个目录，就会在那里启动一个 Claude Code 会话，跑在 bridge 内部；它在页面上的窗口就是它的对话。
- 只有从页面上开的会话（或者带着 `SYMPHONY_TOKEN` 手动开的，见下面的“开会话”）才会出现在办公室里。用户别的 Claude Code 会话，包括正在做这次安装的这个，都不会显示。
- 装了什么、装在哪：程序的路径，mod 装在用户级别，bridge 是随登录启动还是在后台启动，`PATH` 有没有改动，没有 `git` 的话也要说。
- 怎么全部撤销：见下面的“卸载”。

规矩：bridge 只能绑本机（`-addr` 保持 `127.0.0.1:7788`；页面没有登录），别用 `sudo` 运行它，也别在同一个地址再起第二个 bridge。

#### 更新

照第 2 步下载新版，按第 3 步替换程序；mod 用 `claude plugin update symphony-bridge@symphony` 跟着更新（用 clone 当插件市场的话，改成在那里 `git pull`）。正在跑的 bridge 重启之前还是旧程序：先 `symphony -autostart off`，再 `symphony -autostart on -open=false`（或者按原来启动的方式停掉再启动）。停掉 bridge 会结束它开的会话，所以先跟用户说一声。程序和 mod 用同一个版本号；每个版本改了什么，见 [CHANGELOG.md](CHANGELOG.md)。

#### 卸载

```bash
symphony -autostart off                           # 停掉 bridge 和它的会话，以后登录也不再启动
claude plugin uninstall symphony-bridge@symphony  # mod
claude plugin marketplace remove symphony         # 插件市场条目
rm ~/.local/bin/symphony                          # 程序，第 3 步放在哪就删哪
```

数据目录（账号和 token、消息记录、任务）会留着；想删得一干二净，就把它也删掉。

## 能做什么

- **每个会话就是一个对话窗**：点一下员工，页面中间打开它的窗口：上面是员工信息，下面是整个会话的聊天流：输入过的提示词、模型的思考（默认折叠，进行中显示橘黄色“思考中…”）、逐字流出的回答、工具调用，以及其它会话发来的消息（紫色）。在底部输入就是给这个会话发提示词，“停止”按钮能打断它。
- **网页审批**：会话要执行命令、改文件时的权限审批，还有 AskUserQuestion 的提问，会以卡片形式从该员工窗口底部弹出、盖住输入框，顶栏的红色计数能直接跳过去。bridge 没开或者不认账号，就退回终端原样处理，不会卡住会话。
- **像素办公室**：每个会话是一个坐固定工位的员工。绿色空闲、黄色工作中、红色等你审批或回答、灰色下班。闲着的先在工位上玩手机、打游戏，过一会儿就满办公室溜达：窝在休息区沙发上看电视、玩桌上足球、追逐打闹、凑到圆桌聊天、接咖啡、看窗外；闲久了去打盹，多半躲进会议室睡。块头看模型，肌肉看思考强度。画面是大公司的某一层楼，铺满窗口：开放办公区在正中，左边是挂着铭牌的 3–5 人小会议室，右边是大会议室和休息区，大家坐电梯上下班。顶部横栏里还能调像素大小，分四档：楼不变，越往细调画得越细、细节越多。
- **一搜就找到，一点就回来**：点一下闲着的员工，它就回到自己的工位，一直待到下次干活；顶部横栏里的搜索框按名字、会话名或目录找人（按 `/` 直接跳过去），点它旁边的状态牌（空闲、工作中、待处理、下班），这一类的人就都标出来了。横栏下面的小把手能把它收上去、再拉下来。
- **会话间消息**：会话之间互发消息（SendMessage）时有信封飞过去，还有消息记录。
- **在页面上开会话，不弹任何窗口**：点空座位上的“+”招新员工，选目录、模型、思考强度、权限模式（新员工默认全部放行、Opus、最高思考强度）；下班的员工点“上班”，回到原来的座位。会话跑在 bridge 内部自己的终端里，不会弹窗、不会抢走你的注意力；想看终端时点员工窗口里的“打开终端”，关掉那个窗口也不影响会话。
- **会议室里组队干活**：点一个空着的小会议室，写下任务，挑 2–5 个成员：员工（每人另开一个只做这个任务的会话），或者临时请来的顾问。默认第一个成员当组长，用 SendMessage 分派各部分；每个成员都在自己的 git worktree、自己的分支上干活，谁也不会踩到谁的文件。任务结束之前，成员们都坐在会议室里。
- **10 种语言**：English、简体中文、繁體中文、日本語、한국어、Español、Français、Deutsch、Português (Brasil)、Русский。页面跟着浏览器的语言走，顶部横栏里可以切换；bridge 的提示先看 `-lang`，没有就跟系统语言（`LANG` 之类），再没有就用英文；mod 的提示先看 `SYMPHONY_LANG`（页面开的会话就是页面的语言），没有就跟系统语言，再没有就用英文。
- **全在本机**：bridge 是一个 Go 程序（只用标准库；端到端测试另外用了 chromedp），网页是原生 JS，不依赖任何第三方服务。

![网页审批](docs/approve.png)

## 需要什么

- Claude Code 2.1.316（在这个版本上验证过）。mod 用的 function hooks 还是早期接口，升级 Claude Code 后最好先跑一遍端到端测试。
- bridge 有 Linux、macOS、Windows 的现成程序；只有从源码编译才要 Go 1.27 以上。
- `git`，只有协作任务要用：任务里的每个成员都在一个 git worktree 里干活。
- 不需要别的：页面开的会话在每个平台上都跑在 bridge 内部。

## 运行 bridge

`symphony` 启动 bridge，并在浏览器里打开办公室 <http://127.0.0.1:7788/>（不想要就加 `-open=false`）；<http://127.0.0.1:7788/approve> 是一个只管审批的简单页面。bridge 已经在跑时再启动一次，只会打开办公室。Windows 上双击 `symphony.exe` 的话，它会开一个自己的窗口，关掉窗口 bridge 就停了。

| 参数 | 默认 | 说明 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | 监听地址。网页还没有登录，别绑到本机以外的地址 |
| `-data` | `$SYMPHONY_DATA`，没有就用系统配置目录下的 `symphony` | 数据目录：账号和 token（`accounts.json`）、消息记录（`messages.json`）、任务（`tasks.json`），还有 `-background` 的日志（`bridge.log`） |
| `-claude` | `claude` | claude 可执行文件 |
| `-lang` | 系统语言，没有就是 `en` | bridge 提示用的语言（默认值看 `SYMPHONY_LANG`、`LC_ALL`、`LC_MESSAGES` 或 `LANG`） |
| `-mod` | 空 | 开发用：用 `--plugin-dir` 带上这个目录里的 mod，盖过装好的版本 |
| `-web` | 空 | 开发用：直接读这个目录里的页面，改了刷新就生效 |
| `-open` | `true` | bridge 启动后在浏览器里打开办公室；`-addr` 上已经有 bridge 在跑的话，只打开它的办公室 |
| `-autostart` | — | `on`：以后每次登录都自动启动 bridge（带上一起给的其它参数），并且马上启动；`off`：取消 |
| `-background` | — | 在后台启动 bridge：脱离终端，日志写到数据目录的 `bridge.log`，等它能响应了就返回 |
| `-version` | — | 显示版本号后退出 |

**随登录启动**：`symphony -autostart on` 让 bridge 以后每次登录都自动启动（一起给的参数也会带上，比如 `-addr`），并且马上启动；`symphony -autostart off` 取消。Linux 上是 systemd 用户服务（日志用 `journalctl --user -u symphony` 看），macOS 上是 launchd 代理（日志在 `~/Library/Logs/symphony.log`），Windows 上是注册表 Run 键里的一项（bridge 跑在一个控制台窗口里，关掉窗口它就停到下次登录）。这要用 symphony 程序本身，`go run` 不行。

**从源码跑**，要 Go 1.27 以上：`git clone https://github.com/feynmanzcys-bytes/symphony.git && cd symphony && go run ./cmd/bridge`。

## 开会话

**在页面上开**：点空座位上的“+”，填名字，选目录和参数。bridge 在自己内部的终端里跑 `claude`，不会弹出窗口。员工窗口以聊天形式展示会话；“打开终端”按钮会开一个连到该会话的终端窗口（在任何终端里执行 `symphony attach <id>` 也一样；按 `Ctrl-]` 或关窗即断开，会话照常运行）。这些会话是 bridge 的子进程：停掉 bridge，会话也会结束。

**在命令行开**：先建一个账号拿 token，再带着 token 起 `claude`：

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # 返回里有 token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

权限模式建议用 `manual`：`auto` 模式下大部分操作不问人，能到网页的审批就很少了。bridge 不在默认地址的话，再设上 `SYMPHONY_BRIDGE=http://host:port`。`SYMPHONY_LANG` 决定 mod 提示用的语言（没有就跟系统语言，再没有就用英文）。

本机上 bridge 的地址没有响应时，会话会自己用 `symphony -background -open=false -addr <that address>` 把 bridge 拉起来（前提是 `symphony` 在 PATH 里），会话结束后 bridge 还会继续跑。账号不在默认数据目录的话，设好 `SYMPHONY_DATA`。

## 协作任务

小会议室是 2–5 个会话组队做同一个任务的地方。任务要用到 `git`，mod 也得是 0.7.0（`claude plugin update symphony-bridge@symphony`）。

**开一个任务**：点一个空着的小会议室，写清楚要做什么。按你想要的顺序勾选成员（2 人起，最多到会议室的座位数），给每人选好模型和思考强度，再选全队的权限模式（默认全部放行），然后点“开始任务”。成员分两种：

- **员工**：另开一个只做这个任务的会话，在这个员工的目录里干活；它在工位上的会话原样不动。一个员工同一时间只能在一个任务里；从没干过活的员工还没有目录，先点一次“上班”。
- **顾问**：为这个任务临时请来的成员，填的还是那几项（名字、目录、模型、思考强度）。顾问没有工位，坐电梯上来，任务结束就走。

**组长**：默认第一个成员当组长：拆分任务，用 SendMessage 分派各部分，审查大家的成果，把其他人的分支合并到自己的分支，最后告诉你做了什么。去掉“组长”那一项的勾，每人都拿到整个任务，大家自己商量着分。不管哪种，bridge 都会给每个成员发第一条提示词：队里有谁（成员之间用 SendMessage 按名字找人）、每人的分支，还有规矩。

**git 这边**：每个成员都在自己目录的一个独立 git worktree 里、自己的分支 `symphony/<task id>/<member>` 上干活（比如 `symphony/T1/bob`；同名的第二个成员会加上 `-2`），谁也不会踩到谁的文件，你自己的工作区也原样不动。worktree 放在目录下的 `.symphony/worktrees/`，靠 `.git/info/exclude` 让 git 忽略（不碰你的 `.gitignore`）。任务结束时，成员没提交的改动会提交到它的分支上，什么都不会丢，然后删掉 worktree。

- 在你自己的仓库里，什么都不合并：分支留着，等你来合并；有组长的话，组长的分支汇总了所有人的成果。
- 不是 git 仓库的目录，会为这个任务变成一个（`git init`，再把里面的东西全部提交一次），之后也一直是。任务结束时，成员的分支合并进来，目录里就是合并好的结果；`.git` 和分支都留着。
- 合并失败的话，这次合并会撤销，目录保持合并前的样子；分支都留着，上面的成果也在，任务窗口会说明是哪个分支冲突。

**办公室里**：任务期间成员们都坐在会议室的桌边，员工的工位上只剩一把空椅子。会议室的铭牌显示任务编号；铭牌上的灯空闲时是绿的，任务进行中变黄，有成员等你审批或回答时变红。点会议室打开任务窗口：任务内容、成员和各自的目录与分支、团队对话，还有“结束任务”。点会议室里的成员本人，打开的也是任务窗口；在任务窗口里点某个成员，它的对话窗就贴在任务窗口旁边打开，跟员工窗口一样，一次只开一个（点别的成员就换过去）。顶栏的红色计数和搜索框打开任务成员时也是这样。

**团队对话**：任务窗口里列着成员之间互发的消息，每条都带发送者的像素小人和名字，正文以 `@` 加收件人开头。下面有个输入框，你也能自己给团队发话：`@bob-T1 …` 只发给这个成员（写几个 `@names` 就发给这几个人；敲 `@` 会弹出成员列表，用方向键、Enter 或鼠标点选），不带 `@` 就发给任务里的所有人。Enter 发送，Shift+Enter 换行。

在任务窗口里结束任务，或者最后一个会话结束了，任务就结束：bridge 停掉成员的会话，按上面说的把 git 这边收拾好，再删掉顾问。bridge 停掉时还在进行的任务，会在 bridge 下次启动时照样结束掉。

## 账号和座位

- 在 symphony 里，一个身份就是名字 + token + 固定座位。一个账号同一时间只能给一个会话用（它所在的任务里还能再有一个），其它会话会被拒，审批退回终端。
- Claude 的登录不归它管，所有会话可以共用一个 Claude 账号。
- 数据放在 `~/.config/symphony`（Linux）、`~/Library/Application Support/symphony`（macOS）、`%AppData%\symphony`（Windows），或者 `-data` / `SYMPHONY_DATA` 指定的地方。`accounts.json` 里有 token，只有自己能读（0600）；Windows 上靠用户目录的权限保护。

## 安全

- bridge 只认发给本机的请求，还会拒掉别的网站借你的浏览器发来的写请求，因为页面上能开会话，等于能在本机执行命令。
- 网页还没有登录功能，**不要**把 bridge 绑到本机以外的地址。
- 程序没有签名。用浏览器下载的话，macOS 可能因为“无法验证开发者”而拒绝打开：到“系统设置 → 隐私与安全性”里点“仍要打开”，或者运行一次 `xattr -d com.apple.quarantine symphony`。Windows 的 SmartScreen 也可能警告：点“更多信息”，再点“仍要运行”。Release 里的 `checksums.txt` 有每个压缩包的 SHA-256。

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
claude plugin validate .                    # 校验插件市场
claude plugin validate mods/symphony-bridge  # 校验 mod
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # 端到端：起真会话，花 token
```

- 改 mod：`go run ./cmd/bridge -mod mods/symphony-bridge`，页面上开的会话会用 `--plugin-dir` 带上你手上这份 mod；命令行开的会话自己加 `--plugin-dir mods/symphony-bridge`。
- 改页面：`go run ./cmd/bridge -web web`，刷新就生效。
- 不开真会话看效果：另起一个 bridge 灌假数据，`go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo`，再 `go run ./cmd/demo -base http://127.0.0.1:7799`。
- 翻译：每种语言一个文件，页面的在 `web/locales/<lang>.json`，bridge 的在 `internal/i18n/locales/<lang>.json`，键和 `en.json` 一样；mod 那几句提示在 `mods/symphony-bridge/hooks/messages.ts`。单测会检查每种语言的键是不是齐全、`{placeholders}` 对不对得上；端到端测试 `TestI18n` 会用每种语言打开页面。
- 端到端测试需要 claude、tmux（测试自己驱动的会话用）和 Chromium 或 Chrome，所以在 Linux 和 macOS 上跑。默认用仓库里的 mod；设了 `SYMPHONY_E2E_MOD=installed` 就改用装好的 mod。截图在 `e2e/artifacts/`。
- 发版：把 `mods/symphony-bridge/.claude-plugin/plugin.json` 里的版本号改上去，在 `CHANGELOG.md` 里加一节 `## vX.Y.Z (date)`，把 dev 合到 main，然后在 main 的这个提交上打标签并推送（`git tag -a v0.7.0 -m v0.7.0 && git push origin v0.7.0`）；GitHub Actions 会检查标签是不是 `plugin.json` 里的版本号、那一节有没有写，给所有平台编好程序，再发布一个 Release，发布说明以那一节开头（`.github/workflows/release.yml`；带 `-rc.1` 这样的后缀就是预发布）。`.github/release.sh v0.7.0 dist` 能在本地打出同样的包。

| 目录 | 是什么 |
| --- | --- |
| `mods/symphony-bridge/` | Claude Code mod（TypeScript） |
| `.claude-plugin/marketplace.json` | 插件市场，`claude plugin install` 从这里读 |
| `cmd/bridge/`、`internal/bridge/` | bridge |
| `internal/i18n/` | 识别语言，以及 bridge 的翻译 |
| `web/` | 办公室和审批页面及其翻译，编译进 bridge |
| `cmd/demo/` | 给办公室灌假数据 |
| `e2e/` | 端到端测试 |

## 许可证

[MIT](LICENSE)
