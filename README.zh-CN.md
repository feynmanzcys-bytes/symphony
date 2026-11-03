# symphony

[English](README.md) | **简体中文** | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

symphony 是一个独立项目，不隶属于 Anthropic，也未经 Anthropic 认可或赞助。Claude 和 Claude Code 是 Anthropic, PBC 的商标。

把一群 Claude Code 会话放进一间像素办公室：谁在干活、谁在等你，一眼就看到；每个会话都能在自己的窗口里对话，权限审批和 AskUserQuestion 的提问也在那里点一下就处理了。

![像素办公室：员工坐在开放办公区的工位上，左边是小会议室，右边是大会议室和休息区](docs/office.png)

页面有两套主题：上面的像素办公室；以及**同事**——会话是聊天软件里的同事，头像的动作就是状态，中间是对话，右边是档案和电脑。在设置里选，bridge 会记住。

![同事：左边列着各个会话和它们的头像，中间是一段对话，右边是档案和电脑](docs/coworkers.png)

## 安装

最快的办法：打开 Claude Code，粘贴这句话

> 帮我安装 https://github.com/feynmanzcys-bytes/symphony

Claude Code 会读这份 README，照下一节的步骤做；凡是安装完还会留下来的改动，它都会先问你。想自己动手装，照着同样的步骤做就行。

### 给 AI agent 看的：安装手册

你读到这里，是因为用户让你安装 symphony。在 shell 里按顺序做下面的步骤，最后按第 6 步向用户汇报。常规的选择自己定（用哪个压缩包、放进 `PATH` 里的哪个目录），但凡是安装完还会留下来的改动，都要先问用户：每次登录都启动 symphony、改 shell 的 rc 文件、改用户的 `PATH`。所有选择都在汇报里列出来。

symphony 分两部分，两部分都要装：

1. **`symphony` 程序**（bridge）：一个单独的可执行文件，是本机上的 web 服务器，地址 `http://127.0.0.1:7788/`，提供办公室页面、运行各个会话。它应该在用户的 `PATH` 里，名字就叫 `symphony`，因为会话找不到在跑的 bridge 时，mod 会用 `symphony -background` 把它拉起来。
2. **`symphony-bridge` 插件**（mod）：本仓库插件市场里的一个 Claude Code 插件，装在用户级别。它只在带着 `SYMPHONY_TOKEN` 启动的会话里起作用，别的会话里什么都不做。

#### 第 1 步：检查机器

```bash
uname -sm                 # Linux/macOS：据此选下面的压缩包（x86_64 → amd64；arm64/aarch64 → arm64）
claude --version          # 要 Claude Code 2.1.x（在 2.1.316 上验证过）
git --version             # 只有协作任务要用；没有的话在汇报里写明，不用停下
symphony -version         # 已经装了？会输出 "symphony v1.3.0 (darwin/arm64)"
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:7788/api/state   # 有 bridge 在跑吗？401：symphony 1.x；200：旧版的；000：没有
claude plugin list --json # mod 已经装了？找有 "id": "symphony-bridge@symphony" 的那一项
brew --version            # 有 Homebrew 吗（macOS、Linux）？有的话第 2 步可以用它装
```

Windows 上用 PowerShell：`$env:PROCESSOR_ARCHITECTURE` 是 `AMD64` 或 `ARM64`；`scoop --version` 有回应的话，就是装了 Scoop。

没有 `claude` 的话，停下来，告诉用户先装 Claude Code。程序和 mod 都已经装好的话，改去看下面的“更新”。只装了其中一个的话，就补做另一个的步骤：程序是第 2、3 步（`symphony -version` 已经输出最新版本的话就不用做），mod 是第 4 步，然后做第 5 步。已经在跑的 bridge 用的一直是它启动时的那个程序：有 bridge 在跑的话，第 5 步之前要先把它停掉。

#### 第 2 步：拿到程序

**用 Homebrew 或 Scoop 装。** 这个仓库同时也是 Homebrew 的 tap 和 Scoop 的 bucket。第 1 步发现有 Homebrew（macOS、Linux）或 Scoop（Windows）的话，可以用它装，省掉这一步剩下的部分和第 3 步：它会下载最新 Release 的压缩包，拿 formula 或 manifest 里的 SHA-256 校验（这个值是 Release 发布时照它的 `checksums.txt` 写进去的），再把 `symphony` 放进 `PATH`。然后接着做第 4 步，并在汇报里写明是用哪个装的。

```bash
brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
brew install symphony
symphony -version          # 输出版本和平台
```

```powershell
scoop bucket add symphony https://github.com/feynmanzcys-bytes/symphony
scoop install symphony
symphony -version
```

否则，把这台机器对应的压缩包和这个 Release 的 `checksums.txt` 下载到一个新建的、专门的目录里，装任何东西之前先校验压缩包。压缩包叫 `symphony_<os>_<arch>.tar.gz`，`<os>` 是 `linux` 或 `darwin`（macOS），`<arch>` 是 `amd64` 或 `arm64`；Windows 的是 `symphony_windows_<arch>.zip`。

Linux 和 macOS 上：

```bash
d=$(mktemp -d)   # 一个只有你能读的新目录
echo "$d"        # 记下来：第 3 步要从这里安装
cd "$d"
curl -fsSLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_<os>_<arch>.tar.gz
curl -fsSLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/checksums.txt
if command -v sha256sum >/dev/null; then sha256sum -c --ignore-missing checksums.txt; else shasum -a 256 -c --ignore-missing checksums.txt; fi   # 必须输出 "symphony_<os>_<arch>.tar.gz: OK"
tar xzf symphony_<os>_<arch>.tar.gz              # 程序现在就是 "$d/symphony"
```

校验没有输出 `OK` 的话，就停下：别装这个程序，告诉用户。用 `gh` 代替 curl 的话，下载到 `$d` 里（目录还不存在时 gh 会自己建），然后在那里做同样的校验和 `tar`：

```bash
gh release download -R feynmanzcys-bytes/symphony -p 'symphony_<os>_<arch>.tar.gz' -p checksums.txt -D "$d"
```

Windows 上，在 PowerShell 里：

```powershell
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$zip = "symphony_windows_$arch.zip"
$d = Join-Path $env:TEMP ("symphony-" + [guid]::NewGuid())   # 一个新目录
New-Item -ItemType Directory -Path $d | Out-Null
$url = 'https://github.com/feynmanzcys-bytes/symphony/releases/latest/download'
Invoke-WebRequest -UseBasicParsing -Uri "$url/$zip" -OutFile (Join-Path $d $zip)
Invoke-WebRequest -UseBasicParsing -Uri "$url/checksums.txt" -OutFile (Join-Path $d 'checksums.txt')
$want = ((Select-String -SimpleMatch -Path (Join-Path $d 'checksums.txt') -Pattern $zip).Line -split '\s+')[0]
$got = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $d $zip)).Hash
if ($got -ne $want) { throw "checksum mismatch: $zip" } else { 'checksum OK' }
```

接下来，如果装了 `gh` 并且已经登录（`gh auth status` 成功），再查一下压缩包是在哪儿编出来的：每个 Release 的每个压缩包都带着 GitHub 的证明（attestation），证明它是这个仓库的发布 workflow 编出来的。不在终端里运行时，检查通过的话 `gh` 什么都不输出，所以看它的退出码：

```bash
gh attestation verify "$d/symphony_<os>_<arch>.tar.gz" --repo feynmanzcys-bytes/symphony && echo 'attestation OK'
```

```powershell
gh attestation verify (Join-Path $d $zip) --repo feynmanzcys-bytes/symphony
if ($LASTEXITCODE -eq 0) { 'attestation OK' }
```

v1.2.0 之前的 Release 没有证明（会说 `no attestations found`），这些版本以 SHA-256 校验为准。其他任何失败都要停下来，跟校验没通过时一样。没有 `gh` 或者没登录的话，跳过这项检查，并在汇报里写明。

**从源码编译**：在 Linux 或 macOS 上，用户想这样装的话（要 Go 1.27 以上）。要编出可执行文件；`go run` 不行，因为 `-autostart` 不认 `go run` 跑起来的程序：

```bash
d=$(mktemp -d)
git clone https://github.com/feynmanzcys-bytes/symphony.git "$d/src"
cd "$d/src" && go build -o "$d/symphony" ./cmd/bridge   # 程序现在就是 "$d/symphony"，跟从压缩包里解出来的一样
```

| 系统 | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz)（Intel） | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz)（Apple 芯片） |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

每个压缩包里有程序（`symphony`，Windows 上是 `symphony.exe`）、`LICENSE` 和 `README.md`。Release 里的 `checksums.txt` 有每个压缩包的 SHA-256。

#### 第 3 步：放进 PATH

安装第 2 步得到的程序 `$d/symphony`（你的 shell 在两条命令之间不保留变量的话，就把第 2 步输出的那个目录直接写出来）。选一个已经在用户 `PATH` 里、不用 `sudo` 就能写的目录，按这个顺序找：`~/.local/bin`、`/usr/local/bin`、`/opt/homebrew/bin`。

```bash
install -m 755 "$d/symphony" ~/.local/bin/symphony
xattr -d com.apple.quarantine ~/.local/bin/symphony 2>/dev/null || true   # macOS：程序没有签名
symphony -version          # 输出 "symphony v1.3.0 (linux/amd64)"：版本号和平台
```

这些目录一个都不在 `PATH` 里的话，动手改之前先问用户。用户同意，就建一个 `~/.local/bin`，把它加进 shell 的 rc 文件（`~/.zshrc` 或 `~/.bashrc`），并在汇报里说明；用户不同意，就把程序装到用户指定的地方，并告诉用户：`symphony` 不在他们 Claude Code 会话的 `PATH` 里时，mod 没法自己把 bridge 拉起来（第 5 步的随登录启动照样能用）。

Windows 上，把它解压到一个专门的目录（`$d` 和 `$zip` 来自第 2 步；换了新的 shell 就重新设一遍）：

```powershell
$dest = Join-Path $env:LOCALAPPDATA 'Programs\symphony'
Expand-Archive -LiteralPath (Join-Path $d $zip) -DestinationPath $dest -Force
& (Join-Path $dest 'symphony.exe') -version   # 输出 "symphony v1.3.0 (windows/amd64)"
```

把这个目录加进用户的 `Path` 是一项长期的改动：先问用户。用户同意，就运行下面这行，再开一个新的 shell；用户不同意，就用完整路径。用 `Expand-Archive` 解压出来的程序不带下载标记，SmartScreen 不会拦；用资源管理器“全部解压缩”解出来、再双击运行的，可能会被拦：点“更多信息”，再点“仍要运行”。

```powershell
[Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'User') + ";$dest", 'User')
```

做完这些，你自己的 shell 还是找不到 `symphony` 的话（它的 `PATH` 是改之前读的），后面的步骤就用绝对路径。

#### 第 4 步：装 mod

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

这会把本仓库添加为插件市场 `symphony`，并把 mod 装在用户级别，中间没有任何提示。也可以用仓库的一份本地 clone 当插件市场：`claude plugin install symphony-bridge --marketplace /absolute/path/to/symphony`。这样会话直接从那个目录读 mod，在那里 `git pull` 就能更新，所以这份 clone 要放在长期保留的地方，别放在临时目录里。

检查：

```bash
claude plugin list --json   # 其中一项有 "id": "symphony-bridge@symphony", "scope": "user", "enabled": true
```

- `"scope": "project"` 或 `"local"`：mod 只会在一个目录里加载。先运行 `claude plugin uninstall symphony-bridge@symphony`，再加上 `--scope user` 重新安装。
- `"enabled": false`：运行 `claude plugin enable symphony-bridge@symphony`。

#### 第 5 步：起 bridge

第 1 步发现有 bridge 在跑的话，先照下面“更新”里的办法把它停掉：它跑的是旧程序，新程序只会发现它已经在那里，然后自己退出。停掉它会结束它开的会话，所以先跟用户说一声。

问用户：symphony 要不要每次登录都启动。

要的话：

```bash
symphony -autostart on -open=false
```

这会马上启动 bridge，以后每次登录也会自动启动（Linux 上是 systemd 用户服务，macOS 上是 launchd 代理，Windows 上是注册表 Run 键里的一项），并在日期和时间后面输出：

```
symphony now starts when you log in (<where>); it's running at http://127.0.0.1:7788/?key=<key>
```

不要的话：

```bash
symphony -background -open=false
```

这会在后台启动 bridge，脱离终端（机器重启后它不会再启动），并在标准错误上输出（前面带着日期和时间）：

```
the symphony bridge runs in the background at http://127.0.0.1:7788/?key=<key>; its log: <data folder>/bridge.log
```

已经有这个版本的 bridge 在跑时，它（以同样的方式）输出的是 `a symphony bridge is already running at http://127.0.0.1:7788/?key=<key>`；这没问题，别再起一个。`<key>` 是页面密钥，浏览器靠它登录办公室：别把它贴到别的地方。如果程序回答 `127.0.0.1:7788 is taken: another program, or a symphony bridge with another data folder, answers there`，就告诉用户那里是什么在应答（旧版的 symphony bridge 也算：照“更新”里的办法把它停掉），没问过用户之前别换别的地址。

用一个请求检查整条链路，请求里出示数据目录里的页面密钥（经标准输入传进去，这样密钥不会出现在别的用户看得到的命令行上）：

```bash
printf 'X-Symphony-Key: %s\n' "$(cat "${SYMPHONY_DATA:-${XDG_CONFIG_HOME:-$HOME/.config}/symphony}/key")" | curl -s -H @- http://127.0.0.1:7788/api/launch/options      # Linux
printf 'X-Symphony-Key: %s\n' "$(cat "${SYMPHONY_DATA:-$HOME/Library/Application Support/symphony}/key")" | curl -s -H @- http://127.0.0.1:7788/api/launch/options   # macOS
```

```powershell
$data = if ($env:SYMPHONY_DATA) { $env:SYMPHONY_DATA } else { Join-Path $env:APPDATA 'symphony' }
(Invoke-WebRequest -UseBasicParsing -Headers @{ 'X-Symphony-Key' = (Get-Content (Join-Path $data 'key')) } -Uri http://127.0.0.1:7788/api/launch/options).Content
```

数据目录：Linux 上是 `~/.config/symphony`（设了 `$XDG_CONFIG_HOME` 时是 `$XDG_CONFIG_HOME/symphony`），macOS 上是 `~/Library/Application Support/symphony`，Windows 上是 `%AppData%\symphony`；设了 `SYMPHONY_DATA` 的话就是它。

返回里有 `"enabled":true` 和 `"mod":{"ok":true,"id":"symphony-bridge@symphony","version":"1.1.1"}`：说明 bridge 在跑、找到了 `claude`，mod 也装在会话会加载的地方。如果是 `"enabled":false`，`mod.code` 说明哪里不对，`mod.fix` 给出修复的命令：`mod_not_installed` 或 `mod_project_scope`（回到第 4 步），`mod_disabled`（启用它），`mod_check_failed`（`claude plugin list --json` 失败了，多半是 Claude Code 太旧）。`"mod":null` 说明 bridge 在它的 `PATH` 里根本没找到 `claude`（日志里会写）：把它放进去，或者用 `-claude /path/to/claude` 启动 bridge。返回里有 `"code":"page_key"` 的话，说明请求没出示密钥，或者出示的不对：检查 `cat` 有没有找到密钥文件（数据目录见上面）；找到了的话，就是正在跑的那个 bridge 是用另一个数据目录启动的。

bridge 的日志是数据目录里的 `bridge.log`；随登录启动的话，Linux 上日志用 `journalctl --user -u symphony` 看，macOS 上在 `~/Library/Logs/symphony.log`。

#### 第 6 步：向用户汇报

运行 `symphony` 打开办公室：bridge 在跑的话，它会在默认浏览器里打开已经登录好的办公室，然后退出。它还会输出带 `?key=` 的地址，本机上任何浏览器打开这个地址都能登录。

告诉用户：

- 打开办公室就运行 `symphony`（或者双击它）；在已经登录过的浏览器里，直接打开 `http://127.0.0.1:7788/` 也行。点空座位上的“+”招第一个员工：选一个目录（不填就给它新建一个专属目录），就会在那里启动一个 Claude Code 会话，跑在 bridge 内部；它在页面上的窗口就是它的对话。
- 新员工跟随用户的 Claude Code 设置（模型、思考强度、权限模式），除非在对话框里另选。`bypassPermissions` 执行所有命令和改动都不问，这时会话读到的内容就可能让它以用户的身份执行命令；选它时对话框会给出警告。
- 每个会话都花用户的 Claude 套餐额度或 API 额度，跟任何 Claude Code 会话一样；一个任务会同时跑 2 到 5 个会话。
- 只有从页面上开的会话（或者带着 `SYMPHONY_TOKEN` 手动开的，见下面的“开会话”）才会出现在办公室里。用户别的 Claude Code 会话，包括正在做这次安装的这个，都不会显示。
- 装了什么、装在哪：程序的路径，mod 装在用户级别，bridge 是随登录启动还是在后台启动，`PATH` 或 rc 文件有没有改动，你替用户做了哪些选择，没有 `git` 的话也要说。
- 怎么全部撤销：见下面的“卸载”。

规矩：bridge 只在本机监听（不是本机的地址它一律不接受；除非用户选了别的端口，否则保持默认的 `127.0.0.1:7788`），别用 `sudo` 运行它，也别在同一个地址、或者用同一个数据目录再起第二个 bridge（symphony 会拒绝，并说出正在跑的是哪一个）。

#### 更新

照第 2 步下载新版。正在跑的 bridge 重启之前还是旧程序（而且 Windows 上没法替换正在运行的程序），所以先用 `symphony -stop` 停掉它，按第 3 步替换程序，再照它原来的方式启动起来：随登录启动的用 `symphony -autostart on -open=false`（这也会马上启动它），否则用 `symphony -background -open=false`。如果 bridge 跑的时候用了非默认的 `-addr` 或 `-data`，这几条命令都要带上同样的参数：`-stop` 只停那个数据目录的 bridge（在 `-addr` 上，或者在那个目录的 bridge 实际运行的地方），没有 bridge 应答时会说 `nothing to stop`。停掉 bridge 会结束它开的会话，所以先跟用户说一声。（`-stop` 是 v1.1.0 才有的；要停 v1.0.0 的 bridge，就结束它的进程：Linux 和 macOS 上用 `pkill -x symphony`，Windows 上用 `Stop-Process -Name symphony`。）

用 Homebrew 或 Scoop 装的话，用 `brew update && brew upgrade symphony`（或 `scoop update; scoop update symphony`）代替下载和第 3 步，前后照样是 `-stop` 和重新启动。symphony 随登录启动的话，登录启动项里记的是 Homebrew 指向程序的链接（`bin/symphony`），升级后它会指向新版本，所以这里的 `-autostart on` 只是让它现在重新启动。

mod 用 `claude plugin update symphony-bridge@symphony` 跟着更新（用 clone 当插件市场的话，改成在那里 `git pull`）。

程序和 mod 用同一个版本号；每个版本改了什么，见 [CHANGELOG.md](CHANGELOG.md)。

#### 卸载

```bash
symphony -stop                                    # 停掉 bridge 和它的会话，不管它是怎么启动的（-addr 和 -data 不是默认值的话，要带上）
symphony -autostart off                           # 以后登录不再启动它（本来就没设的话会说明）
claude plugin uninstall symphony-bridge@symphony  # mod
claude plugin marketplace remove symphony         # 插件市场条目
rm ~/.local/bin/symphony                          # 程序，第 3 步放在哪就删哪
```

第 3 步把 `~/.local/bin` 加进了 shell 的 rc 文件的话，把那一行也删掉。Windows 上，在 PowerShell 里：

```powershell
symphony -stop                                               # 停掉 bridge 和它的会话（-addr 和 -data 不是默认值的话，要带上）
symphony -autostart off                                      # 以后登录不再启动它
claude plugin uninstall symphony-bridge@symphony
claude plugin marketplace remove symphony
$dest = Join-Path $env:LOCALAPPDATA 'Programs\symphony'
Remove-Item -Recurse -Force $dest                            # 程序
$p = ([Environment]::GetEnvironmentVariable('Path', 'User') -split ';' | Where-Object { $_ -and $_ -ne $dest }) -join ';'
[Environment]::SetEnvironmentVariable('Path', $p, 'User')    # 第 3 步加进去的那个 Path 条目
```

用 Homebrew 装的话，把 `rm` 那一行换成 `brew uninstall symphony` 和 `brew untap feynmanzcys-bytes/symphony`；用 Scoop 装的话，把 PowerShell 里最后四行换成 `scoop uninstall symphony` 和 `scoop bucket rm symphony`。

数据目录（账号和 token、页面密钥、消息记录、用量账本、任务，还有 `lock`、`bridge.addr` 或者被挪到一边的 `*.corrupt-<time>` 这类文件）会留着；想删得一干二净，就把它也删掉。员工的专属目录（主目录下的 `symphony-bots`，或设置里设的目录）和里面的成果也会留着。删掉它也就删掉了页面密钥：下次启动会生成一个新的，每个浏览器都得重新登录。Linux 上浏览器是 snap 或 flatpak 的话，`open.html` 在那个浏览器自己的目录里（`~/snap/<snap>/common` 或 `~/.var/app/<app id>`），也一起删掉。

## 能做什么

- **每个会话就是一个对话窗**：点一下员工，页面中间打开它的窗口：上面是员工信息，下面是整个会话的聊天流：输入过的提示词、模型的思考（默认折叠，进行中显示橘黄色“思考中…”）、逐字流出的回答、工具调用，以及其它会话发来的消息（紫色）。在底部输入就是给这个会话发提示词，“停止”按钮能打断它。模型写的内容按 Markdown 显示（标题、列表、表格、代码、链接）。两段回答之间的思考和工具调用收成一行：干活时是“思考中… 2分38秒”，做完是“思考了 2分38秒”，点一下展开看步骤。
- **网页审批**：会话要执行命令、改文件时的权限审批，还有 AskUserQuestion 的提问，会以卡片形式从该员工窗口底部弹出、盖住输入框（在“同事”主题里，卡片就留在对话中它出现的地方，你照样能打字），顶栏的红色计数能直接跳过去。权限审批卡片上的“本会话的……都允许”会让同类调用在会话结束前不再询问、直接放行：所有改动、开头相同的命令（`npm test`），或者某个工具。MCP 服务器的表单也会显示成卡片；它要到会话的终端里填写，卡片会在窗口里打开这个终端。bridge 没开、不认账号或者信不过，就退回终端原样处理，不会卡住会话。
- **像素办公室**：每个会话是一个坐固定工位的员工。绿色空闲、黄色工作中、红色等你审批或回答、灰色下班。闲着的先在工位上玩手机、打游戏，过一会儿就满办公室溜达：窝在休息区沙发上看电视、玩桌上足球、追逐打闹、凑到圆桌聊天、接咖啡、看窗外；闲久了去打盹，多半躲进会议室睡。块头看模型，肌肉看思考强度。画面是大公司的某一层楼，铺满窗口：开放办公区在正中，左边是挂着铭牌的 3–5 人小会议室，右边是大会议室和休息区，大家坐电梯上下班。设置里还能调像素大小，分四档：楼不变，越往细调画得越细、细节越多。
- **一搜就找到，一点就回来**：点一下闲着的员工，它就回到自己的工位，一直待到下次干活；顶部横栏里的搜索框按名字、会话名或目录找人（按 `/` 直接跳过去），点它旁边的状态牌（空闲、工作中、待处理、下班），这一类的人就都标出来了。横栏下面的小把手能把它收上去、再拉下来。
- **会话间消息**：会话之间互发消息（SendMessage）时有信封飞过去，还有消息记录。
- **在页面上开会话，不弹任何窗口**：点空座位上的“+”招新员工，选目录；目录不填的话，就在默认目录（主目录下的 `symphony-bots`，在设置 → 员工里可以改）里给它新建一个专属目录并初始化成 git 仓库，下次上班还回到那里。模型、思考强度和权限模式跟随你的 Claude Code 设置（“跟随设置”），除非你另选；选了 `bypassPermissions` 会显示一行红色警告，说明它放行了什么。勾选“告诉它办公室里还有谁”，会话的系统提示词里就会列出在线的其他人（每段对话一次），方便它用 SendMessage 找他们。下班的员工点“上班”，回到原来的座位，用的还是上次干活时的选择。会话跑在 bridge 内部自己的终端里，不会弹窗、不会抢走你的注意力；想看终端时点员工窗口里的“打开终端”，关掉那个窗口也不影响会话。
- **会议室里组队干活**：点一个空着的小会议室，写下任务，挑 2–5 个成员：员工（每人另开一个只做这个任务的会话），或者临时请来的顾问。默认第一个成员当组长，用 SendMessage 分派各部分；每个成员都在自己的 git worktree、自己的分支上干活，谁也不会踩到谁的文件。任务结束之前，成员们都坐在会议室里。
- **设置和报告问题**：“设置”按钮（同事主题里是个齿轮）放着不常改的东西：语言、主题、浅色或深色、像素大小、暂停动画、默认展开步骤。出了错（启动或任务失败、会话的 API 报错、页面里的错误）会自动写进错误日志；想报告问题时，在设置里点“我要报告问题”，把拿到的文件发邮件给我们，或者附到 GitHub 的 issue 里。文件里有这些错误、版本信息和 bridge 最近的日志，不含密钥、令牌和对话内容。
- **10 种语言**：English、简体中文、繁體中文、日本語、한국어、Español、Français、Deutsch、Português (Brasil)、Русский。页面跟着浏览器的语言走，设置里可以切换；bridge 的提示先看 `-lang`，没有就跟系统语言（`LANG` 之类），再没有就用英文；mod 的提示先看 `SYMPHONY_LANG`（页面开的会话就是页面的语言），没有就跟系统语言，再没有就用英文。
- **全在本机**：bridge 是一个 Go 程序（只用标准库；端到端测试另外用了 chromedp），只在本机监听，网页是原生 JS，不依赖任何第三方服务。

![员工窗口：会话以聊天的形式展示，审批卡片盖在输入框上](docs/approve.png)

## 需要什么

- Claude Code 2.1.316（在这个版本上验证过）。mod 用的 function hooks 还是早期接口，升级 Claude Code 后最好先跑一遍端到端测试。
- bridge 有 Linux、macOS、Windows 的现成程序；只有从源码编译才要 Go 1.27 以上。
- `git`，只有协作任务要用：任务里的每个成员都在一个 git worktree 里干活。
- 不需要别的：页面开的会话在每个平台上都跑在 bridge 内部。

## 运行 bridge

`symphony` 启动 bridge，并在浏览器里打开已经登录好的办公室（不想要就加 `-open=false`）。浏览器通过 `open.html` 拿到 bridge 的页面密钥，存成 cookie；`open.html` 是数据目录里一个只有你能读的文件，所以密钥从不出现在命令行上；Linux 上浏览器是 snap 或 flatpak 的话，这个文件改放到浏览器自己的目录里，因为这类浏览器读不到数据目录。在没登录的浏览器里打开 <http://127.0.0.1:7788/>，会看到一个页面，告诉你怎么进去：再运行一次 `symphony`，或者打开 bridge 在日志里打印的带 `?key=` 的地址。<http://127.0.0.1:7788/approve> 是一个只管审批的简单页面。`symphony` 已经在跑时再启动一次，只会打开办公室。Windows 上双击 `symphony.exe` 的话，它会开一个自己的窗口，关掉窗口 bridge 就停了。

| 参数 | 默认 | 说明 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | 监听地址。主机只能是 `127.0.0.1`、`localhost` 或 `::1`（例如 `[::1]:7788`），别的 bridge 一律不接受 |
| `-data` | `$SYMPHONY_DATA`，没有就用系统配置目录下的 `symphony` | 数据目录：账号和 token（`accounts.json`）、页面密钥（`key`）和 `open.html`、消息记录（`messages.json`）、用量账本（`ledger.json`）、任务（`tasks.json`）、bridge 的设置（`settings.json`）、错误日志（`errors.json`）、`-background` 的日志（`bridge.log`）、`lock` 和 `bridge.addr`（哪个 bridge 在用它、在哪个地址），还有损坏后被挪到一边的消息记录、用量账本或设置（`*.corrupt-<time>`） |
| `-claude` | `claude` | claude 可执行文件 |
| `-lang` | 系统语言，没有就是 `en` | bridge 提示用的语言（默认值看 `SYMPHONY_LANG`、`LC_ALL`、`LC_MESSAGES` 或 `LANG`） |
| `-mod` | 空 | 开发用：用 `--plugin-dir` 带上这个目录里的 mod，盖过装好的版本 |
| `-web` | 空 | 开发用：直接读这个目录里的页面，改了刷新就生效 |
| `-open` | `true` | bridge 启动后在浏览器里打开办公室；`-addr` 上已经有用这个数据目录的 bridge 在跑的话，只打开它的办公室 |
| `-autostart` | — | `on`：以后每次登录都自动启动 bridge（带上一起给的其它参数），并且马上启动；`off`：取消 |
| `-background` | — | 在后台启动 bridge：脱离终端，日志写到数据目录的 `bridge.log`，等它能响应了就返回 |
| `-stop` | — | 停掉用这个数据目录的 bridge（在 `-addr` 上，不在的话就在它实际运行的地方），不管它是怎么启动的（它的会话会结束），等它退出了才返回；它只向能证明自己有页面密钥的 bridge 出示这个密钥 |
| `-version` | — | 显示版本号后退出 |

`-addr` 上应答的是别的东西（别的程序、用另一个数据目录的 bridge，或者旧版的 symphony bridge）时，symphony 会说明情况，然后停下；它只向能证明自己有页面密钥的 bridge 出示这个密钥。一个数据目录同一时间只归一个 bridge 用：如果用的数据目录已经有 bridge 在别的地址上跑着，symphony 会说出它在哪儿跑、打开它的办公室，然后停下（要再起一个 bridge，就给它单独的 `-data`）。

**随登录启动**：`symphony -autostart on` 让 bridge 以后每次登录都自动启动（一起给的参数也会带上，比如 `-addr`），并且马上启动；`symphony -autostart off` 取消。Linux 上是 systemd 用户服务（日志用 `journalctl --user -u symphony` 看），macOS 上是 launchd 代理（日志在 `~/Library/Logs/symphony.log`），Windows 上是注册表 Run 键里的一项（bridge 跑在一个控制台窗口里，关掉窗口它就停到下次登录）。`symphony -stop` 能停掉 bridge，不管它是怎么启动的，一直停到它再被启动（随登录启动的，就是到下次登录）。这要用 symphony 程序本身，`go run` 不行。随登录启动的 bridge 从你的 shell 那里只拿到 `PATH`，所以 `-autostart on` 还会把会话可能要用的变量照你运行它时 shell 里的值存下来：`CLAUDE_CONFIG_DIR`、代理设置（`HTTP_PROXY`、`HTTPS_PROXY`、`NO_PROXY`、`ALL_PROXY`，大小写都认）、`NODE_EXTRA_CA_CERTS`、`SSL_CERT_FILE` 和 `SSL_CERT_DIR`，别的一概不存（API 密钥也不存），存在数据目录里的 `autostart-env.json`，这个文件只有你能读；这些变量改了之后，要再运行一次。一启动就失败的 bridge（数据目录正被别的 bridge 用着、`accounts.json` 坏了、地址被别的东西占了）会被重启几次，之后就不再管它：Linux 上是一分钟内试 5 次（原因用 `systemctl --user status symphony` 看），macOS 上是每分钟试一次（原因看日志）；把问题解决掉，再运行一次 `symphony -autostart on`，或者直接重新登录一次。

**从源码跑**，要 Go 1.27 以上：`git clone https://github.com/feynmanzcys-bytes/symphony.git && cd symphony && go run ./cmd/bridge`。

## 开会话

**在页面上开**：点空座位上的“+”，填名字，选目录和参数。bridge 在自己内部的终端里跑 `claude`，不会弹出窗口。员工窗口以聊天形式展示会话；“打开终端”按钮会开一个连到该会话的终端窗口（在任何终端里执行 `symphony attach <id>` 也一样，bridge 不用默认值时要加上 `-addr` 和 `-data`；开不了终端窗口时，页面上会显示完整的命令。按 `Ctrl-]` 或关窗即断开，会话照常运行）。这些会话是 bridge 的子进程：停掉 bridge，会话也会结束。会话结束时（不管是在页面上停掉的、跟着 bridge 一起停的，还是自己退出的），它留在后台还在跑的东西（比如工具用 `&` 放到后台的命令）也会跟着结束：Windows 上由会话的作业对象（job object）把它们全部结束；Linux 和 macOS 上，bridge 按这些进程环境变量里的启动 ID 找到它们，但在 macOS 上，苹果自带程序（`/bin/sleep`、`bash`、`zsh`、`perl`：系统不让 `ps -E` 看到它们的环境变量）的环境变量它读不到，所以留下的 `node` 或 Homebrew 装的服务器会被结束，单纯的 `sleep` 或 shell 脚本则不会。

**在命令行开**：先建一个账号拿 token（请求里要出示页面密钥），再带着 token 起 `claude`：

```bash
printf 'X-Symphony-Key: %s\n' "$(cat ~/.config/symphony/key)" | curl -H @- -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # 返回里有 token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

密钥文件：Linux 上是 `~/.config/symphony/key`，macOS 上是 `~/Library/Application Support/symphony/key`，Windows 上是 `%AppData%\symphony\key`；用了 `-data` / `SYMPHONY_DATA` 的话，就是那个目录里的 `key`。

权限模式建议用 `manual`：`auto` 模式下大部分操作不问人，能到网页的审批就很少了。bridge 不在默认地址的话，再设上 `SYMPHONY_BRIDGE`：本机上用 `http://127.0.0.1:<port>`、`http://localhost:<port>` 或 `http://[::1]:<port>`，或者一个 `https://` 地址；别的写法，mod 一律不联系 bridge。`SYMPHONY_LANG` 决定 mod 提示用的语言（没有就跟系统语言，再没有就用英文）。

会话把 token 发往任何地方之前，mod 会先确认那个地址上的 bridge 认识这个 token，确认时不发出 token（`POST /api/hello`）。bridge 接不了这个会话时，会话会显示下面某一条消息，审批留在终端里，会话不会卡住：

| 消息 | 意思 |
| --- | --- |
| `连不上 bridge（...）` / `can't reach the bridge` | bridge 的地址上没有应答，拉起 bridge 也没用（或者这个地址不在本机）。 |
| `启动 bridge（...）失败：<reason>` / `couldn't start the bridge ...: <reason>` | 没有应答，`symphony -background` 也失败了，原因就是显示的那个（比如这个数据目录的 bridge 在别的地址上跑着：把 `SYMPHONY_BRIDGE` 设成那个地址）。 |
| `bridge 不认这个 SYMPHONY_TOKEN` / `the bridge doesn't know this SYMPHONY_TOKEN` | bridge 里没有带这个 token 的账号：账号被删了，或者 bridge 用的是另一个数据目录。 |
| `不是本会话能信任的 symphony bridge` / `isn't a symphony bridge this session can trust` | 那个地址上有东西应答，但证明不了它认识这个 token：是别的程序，或者是比 mod 旧的 symphony（更新它）。token 不会发出去。 |
| `SYMPHONY_BRIDGE（...）必须在本机或使用 https` / `SYMPHONY_BRIDGE ... must be on this machine or use https` | 这个地址 mod 不接受；bridge 根本不会被联系。 |
| `这个账号已经有会话在线` / `this account already has a session online` | 一个账号同一时间只能给一个会话用；给这个会话另建一个账号。 |
| `正在等你在办公室网页（...）上回复` / `waiting for your answer in the office page` | 权限审批或提问在办公室网页上等着，不在终端里；去那里回复，或者按 Esc 取消。每个会话只提示一次。 |

本机上 bridge 的地址没有应答时，会话会自己用 `symphony -background -open=false -addr <that address>` 把 bridge 拉起来（前提是 `symphony` 在 PATH 里），并告诉你（`已在后台启动 bridge（...）` / `started the bridge ... in the background`）；会话结束后 bridge 还会继续跑。账号不在默认数据目录的话，设好 `SYMPHONY_DATA`。这个数据目录的 bridge 已经在别的地址上跑着的话，会话会说明情况，然后退回终端：把 `SYMPHONY_BRIDGE` 设成那个地址。

## 协作任务

小会议室是 2–5 个会话组队做同一个任务的地方。任务要用到 `git`。

**开一个任务**：点一个空着的小会议室，写清楚要做什么。按你想要的顺序勾选成员（2 人起，最多到会议室的座位数），给每人选好模型和思考强度，再选全队的权限模式（默认跟随 Claude Code 自己的设置；干过活的员工一开始用的是它上次的模型和思考强度；选了 `bypassPermissions` 会显示那条警告），然后点“开始任务”。成员分两种：

- **员工**：另开一个只做这个任务的会话，在这个员工的目录里干活；它在工位上的会话原样不动。一个员工同一时间只能在一个任务里；从没干过活的员工还没有目录，先点一次“上班”。
- **顾问**：为这个任务临时请来的成员，填的还是那几项（名字、目录、模型、思考强度）。顾问没有工位，坐电梯上来，任务结束就走。

**组长**：默认第一个成员当组长：拆分任务，用 SendMessage 分派各部分，审查大家的成果，把其他人的分支合并到自己的分支，最后告诉你做了什么。去掉“组长”那一项的勾，每人都拿到整个任务，大家自己商量着分。不管哪种，bridge 都会给每个成员发第一条提示词：队里有谁（成员之间用 SendMessage 按名字找人）、每人的分支，还有规矩。

**git 这边**：每个成员都在自己目录的一个独立 git worktree 里、自己的分支 `symphony/<task id>/<member>` 上干活（比如 `symphony/T1/bob`；同名的第二个成员会加上 `-2`），谁也不会踩到谁的文件，你自己的工作区也原样不动。worktree 放在目录下的 `.symphony/worktrees/`，靠 `.git/info/exclude` 让 git 忽略（不碰你的 `.gitignore`）。任务结束时，成员没提交的改动会提交到它的分支上，它的 worktree 里 git 忽略的文件（编译产物、`.env`、`node_modules` 之类）会挪到目录下的 `.symphony/kept/<member>-<task id>/`，然后删掉 worktree。symphony 从来不删 `.symphony/kept/`；里面的东西不需要了，你自己删掉。

- 在你自己的仓库里，什么都不合并：分支留着，等你来合并；有组长的话，组长的分支汇总了所有人的成果。
- 不是 git 仓库的目录，会为这个任务变成一个，之后也一直是：先 `git init`，再把里面的东西提交一次，但常含机密信息或者体积大的文件不提交（`.env`、`.env.*`、`*.pem`、`*.key`、`*.p12`、`*.pfx`、`id_rsa*`、`id_ecdsa*`、`id_ed25519*`、`.npmrc`、`.netrc`、`.pypirc`、`node_modules/`、`.DS_Store`），这些规则写进 `.git/info/exclude`。这些文件还在目录里，只是不在成员的 worktree 里，成员要用的话得自己装或者自己建一份。任务结束时，成员的分支合并进这个目录的分支，目录里就是合并好的结果；`.git` 和分支都留着。任务没能启动的话，目录会变回普通目录。
- 合并失败的话，这次合并会撤销，目录保持合并前的样子；分支都留着，上面的成果也在，任务窗口会说明是哪个分支冲突。任务结束时别的环节出了错，就保持出错时的样子，任务窗口会说明是哪里出错。

**办公室里**：任务期间成员们都坐在会议室的桌边，员工的工位上只剩一把空椅子。会议室的铭牌显示任务编号；铭牌上的灯空闲时是绿的，任务进行中变黄，有成员等你审批或回答时变红。点会议室打开任务窗口：任务内容、成员和各自的目录与分支、团队对话，还有“结束任务”。点会议室里的成员本人，打开的也是任务窗口；在任务窗口里点某个成员，它的对话窗就贴在任务窗口旁边打开，跟员工窗口一样，一次只开一个（点别的成员就换过去）。顶栏的红色计数和搜索框打开任务成员时也是这样。

**团队对话**：任务窗口里列着成员之间互发的消息，每条都带发送者的像素小人和名字，正文以 `@` 加收件人开头。下面有个输入框，你也能自己给团队发话：`@bob-T1 …` 只发给这个成员（写几个 `@names` 就发给这几个人；敲 `@` 会弹出成员列表，用方向键、Enter 或鼠标点选），不带 `@` 就发给任务里的所有人。Enter 发送，Shift+Enter 换行。

在任务窗口里结束任务，或者最后一个会话结束了，任务就结束：bridge 停掉成员的会话，按上面说的把 git 这边收拾好，再删掉顾问。bridge 停掉时还在进行的任务，会在 bridge 下次启动时照样结束掉。

## 账号和座位

- 在 symphony 里，一个身份就是名字 + token + 固定座位。一个账号同一时间只能给一个会话用（它所在的任务里还能再有一个），其它会话会被拒，审批退回终端。
- Claude 的登录不归它管，所有会话可以共用一个 Claude 账号。
- 数据放在 `~/.config/symphony`（Linux）、`~/Library/Application Support/symphony`（macOS）、`%AppData%\symphony`（Windows），或者 `-data` / `SYMPHONY_DATA` 指定的地方。`accounts.json` 里有 token，`key` 是页面密钥；两者都只有所有者能读（0600）；Windows 上靠用户目录的权限保护它们。

## 费用和权限

- symphony 开的每个会话都是一个 Claude Code 会话，跟别的会话一样花你的 Claude 套餐额度或 API 额度。Opus 加最高思考强度用得很快，而一个任务会同时跑 2 到 5 个会话。
- `bypassPermissions` 执行所有命令和改动都不问。这时会话读到的内容（文件、网页、工具的输出）就可能让它以你的身份执行命令。开会话和开任务的对话框里选了它，会显示一行红色警告；这两个对话框默认跟随你的 Claude Code 设置。
- 页面开的会话，Claude Code 问“信任此目录”时由 bridge 替你回答，所以只在你信任的目录里开会话。
- “本会话的……都允许”只管到会话结束，而且只放行 Claude Code 本来会问你的操作，不越过你的权限设置：跟别的命令连在一起的命令（`;`、`&&`、`|`、重定向）一律不放行，也绝不越过拒绝规则或者你所在组织的设置。一条命令规则覆盖所有开头相同的命令（`npm test` 覆盖 `npm test -- --watch`）。

## 安全

页面上能开会话，等于能在本机执行命令，所以 bridge 会把好谁能连上它这道关：

- **页面密钥**：办公室和它的 API 都要页面密钥，这是一个 cookie，浏览器打开 `symphony` 给的那个链接时拿到。密钥就是数据目录里的 `key` 文件，只有你能读；程序在请求头 `X-Symphony-Key` 里出示它。
- **只限本机**：bridge 只在本机监听，也只接受本机的连接。发给别的主机名的请求（DNS rebinding），还有别的网站借你的浏览器发来的写请求，它都会拒掉；浏览器标明来自别的网站或别的端口的请求，它的 API 一概不回答，哪怕只是点开一个链接。别的网站也没法把它的页面嵌进框架里。页面只加载 bridge 自己的文件（Content Security Policy）。
- **mod 先验证 bridge**，再发出 token；`symphony` 程序也是先验证 bridge，再出示页面密钥。所以在 bridge 地址上监听的别的东西，token 和密钥哪个都拿不到。
- **防不住的**：以你自己的用户身份跑的程序，能读到数据目录里的页面密钥和 token，跟你的其它文件一样。另外，浏览器会把一个站点的 cookie 发给这个站点的每一个端口：这台机器的其他用户在 `127.0.0.1` 上跑的服务器，在你的浏览器打开它时，或者在 bridge 停着、办公室还开在某个标签页里时占了 bridge 的端口，就能拿到页面密钥的 cookie。在跟别人共用的机器上，先关掉办公室再停 bridge；密钥可能已经泄露的话，删掉数据目录里的 `key` 文件（下次启动会生成一个新的）。
- 程序没有签名。用浏览器下载的话，macOS 可能因为“无法验证开发者”而拒绝打开：到“系统设置 → 隐私与安全性”里点“仍要打开”，或者运行一次 `xattr -d com.apple.quarantine symphony`。Windows 上，用资源管理器“全部解压缩”解出来的程序，双击时 SmartScreen 可能会拦：点“更多信息”，再点“仍要运行”（安装步骤用的 `Expand-Archive` 不会给它留下下载标记）。Release 里的 `checksums.txt` 有每个压缩包的 SHA-256，每个压缩包还带着 GitHub 的证明（attestation），说明它是在哪儿编出来的；安装的第 2 步两样都会检查。

要报告漏洞，见 [SECURITY.md](SECURITY.md)。

## 平台

| 系统 | 页面开的会话 | “打开终端” | 验证情况 |
| --- | --- | --- | --- |
| Linux | bridge 内部的伪终端 | 图形终端（x-terminal-emulator、gnome-terminal、konsole、xterm）运行 `symphony attach` | 跑过完整的端到端测试 |
| macOS | bridge 内部的伪终端 | Terminal.app 运行 `symphony attach` | 跑过完整的端到端测试 |
| Windows | bridge 内部的伪控制台（ConPTY） | 新开控制台窗口运行 `symphony attach` | 单测和不需要 tmux 的端到端测试；真会话在 Windows 11 上验过 |

每个平台上都由 bridge 替你点“信任此目录”。

## 开发

```bash
go test -race ./...                         # bridge 和翻译的单测
claude plugin test mods/symphony-bridge     # mod 单测
claude plugin validate .                    # 校验插件市场
claude plugin validate mods/symphony-bridge  # 校验 mod
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # 端到端：起真会话，花 token
```

- 参与贡献：[CONTRIBUTING.md](CONTRIBUTING.md) 里有环境搭建、提 pull request 之前要跑的检查和规矩；[CLAUDE.md](CLAUDE.md) 是代码的详细地图。
- 改 mod：`go run ./cmd/bridge -mod mods/symphony-bridge`，页面上开的会话会用 `--plugin-dir` 带上你手上这份 mod；命令行开的会话自己加 `--plugin-dir mods/symphony-bridge`。
- 改页面：`go run ./cmd/bridge -web web`，刷新就生效。
- 不开真会话看效果：另起一个用自己数据目录的 bridge 灌假数据，`D=$(mktemp -d)`，`go run ./cmd/bridge -addr 127.0.0.1:7799 -data "$D"`，再在另一个终端里（`D` 设成同一个目录）`go run ./cmd/demo -base http://127.0.0.1:7799 -data "$D"`（demo 从那个数据目录里读 bridge 的页面密钥）。
- 这份 README 里的截图：`SYMPHONY_E2E=1 SYMPHONY_E2E_DOCS=1 go test -tags e2e -run TestDocsScreens ./e2e/` 会用假数据重新截一遍（不花 token）。
- 翻译：每种语言一个文件，页面的在 `web/locales/<lang>.json`，bridge 的在 `internal/i18n/locales/<lang>.json`，键和 `en.json` 一样；mod 那几句提示在 `mods/symphony-bridge/hooks/messages.ts`。单测会检查每种语言的键是不是齐全、`{placeholders}` 对不对得上；端到端测试 `TestI18n` 会用每种语言打开页面。
- 端到端测试需要 claude、tmux（测试自己驱动的会话用）和 Chromium 或 Chrome，所以在 Linux 和 macOS 上跑。`TestOffice` 和 `TestModStartsBridge` 会起真会话、花 token，其它的不花。默认用仓库里的 mod；设了 `SYMPHONY_E2E_MOD=installed` 就改用装好的 mod。截图在 `e2e/artifacts/`。
- 发版：把 `mods/symphony-bridge/.claude-plugin/plugin.json` 里的版本号改上去，在 `CHANGELOG.md` 里加一节 `## vX.Y.Z (date)`，把 dev 合到 main，然后在 main 的这个提交上打标签并推送（`git tag -a v1.0.0 -m v1.0.0 && git push origin v1.0.0`）；GitHub Actions 会检查标签是不是 `plugin.json` 里的版本号、那一节有没有写，给所有平台编好程序，给这些压缩包附上它们是在哪儿编出来的证明，再发布一个 Release，发布说明以那一节开头（`.github/workflows/release.yml`；带 `-rc.1` 这样的后缀就是预发布）；不是预发布的话，接着还会把 Homebrew 的 formula 和 Scoop 的 manifest 提交到 main（dev 能快进的话，也提交到 dev），所以发完之后两个分支都要 pull 一下。`.github/release.sh v1.0.0 dist` 能在本地打出同样的包，在 Linux 上连每个字节都一样。

| 目录 | 是什么 |
| --- | --- |
| `mods/symphony-bridge/` | Claude Code mod（TypeScript） |
| `.claude-plugin/marketplace.json` | 插件市场，`claude plugin install` 从这里读 |
| `cmd/bridge/`、`internal/bridge/` | bridge |
| `internal/i18n/` | 识别语言，以及 bridge 的翻译 |
| `web/` | 办公室和审批页面及其翻译，编译进 bridge |
| `cmd/demo/` | 给办公室灌假数据 |
| `e2e/` | 端到端测试 |
| `docs/` | bridge 和页面之间的协议，以及截图 |

## 许可证

[MIT](LICENSE)

symphony 是一个独立项目，不隶属于 Anthropic，也未经 Anthropic 认可或赞助。Claude 和 Claude Code 是 Anthropic, PBC 的商标。
