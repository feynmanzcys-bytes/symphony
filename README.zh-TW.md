# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | **繁體中文** | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

symphony 是獨立專案，與 Anthropic 沒有隸屬關係，也未獲得 Anthropic 的認可或贊助。Claude 和 Claude Code 是 Anthropic, PBC 的商標。

把一群 Claude Code 工作階段放進一間像素風辦公室：誰在工作、誰在等你，一眼就看得出來；每個員工都有自己的對話視窗，可以直接跟它交談，權限請求和 AskUserQuestion 的提問也在那裡按一下就能處理。

![像素辦公室：員工坐在開放辦公區的座位上，左邊是小會議室，右邊是會議室和休息區](docs/office.png)

頁面有多套主題：上面的像素辦公室；**HQ 管理台**——一個管理後台：指標列、人員表、審批佇列、專案看板、活動時間軸；**同事**——工作階段是聊天軟體裡的同事，頭像的動作就是狀態，中間是對話，右邊是個人資料和電腦；以及**任務控制中心**——掛在副螢幕上的控制室大螢幕：每個工作階段一塊遙測面板，審批是警報，動態是底部跑馬燈。在頂欄裡選，bridge 會記住。

![HQ 管理台：上方一排關鍵數字，下面是員工和他們的工作](docs/hq.png)

![同事：左邊列出工作階段和它們的頭像，中間是對話，右邊是個人資料和電腦](docs/coworkers.png)

![任務控制中心：一面深色大螢幕，每個工作階段一塊遙測面板，底部是動態跑馬燈](docs/mission.png)

## 安裝

最快的方法：打開 Claude Code，貼上

> 幫我安裝 https://github.com/feynmanzcys-bytes/symphony

Claude Code 會讀這份 README，照著下一節的步驟做；凡是安裝完之後還會留著的改動，它都會先問過你。想自己動手安裝，照著同樣的步驟做就行。

### 給 AI agent：安裝手冊

你會讀到這裡，是因為使用者請你安裝 symphony。請在 shell 裡依序執行下面的步驟，最後完成第 6 步的回報。例行的選擇自己決定（用哪個壓縮檔、`PATH` 裡的哪個資料夾），但凡是安裝完之後還會留著的改動，都要先問使用者：每次登入都啟動 symphony、修改 shell 的 rc 檔，或是更改使用者的 `PATH`。所有的選擇都要列在回報裡。

symphony 由兩個部分組成，兩個都要裝：

1. **`symphony` 程式**（bridge）：單一個執行檔，是位於 `http://127.0.0.1:7788/` 的本機網頁伺服器，提供辦公室頁面並執行工作階段。它應該以 `symphony` 這個名字放在使用者的 `PATH` 裡，因為工作階段找不到正在執行的 bridge 時，mod 會用 `symphony -background` 啟動它。
2. **`symphony-bridge` 外掛**（mod）：來自這個儲存庫市集的 Claude Code 外掛，安裝在使用者範圍。它只在帶著 `SYMPHONY_TOKEN` 啟動的工作階段裡起作用，在其他工作階段裡什麼都不做。

#### 第 1 步：檢查這台電腦

```bash
uname -sm                 # Linux/macOS：決定下面用哪個壓縮檔（x86_64 → amd64；arm64/aarch64 → arm64）
claude --version          # 需要 Claude Code 2.1.x（在 2.1.316 上測試過）
git --version             # 只有協作任務需要；沒有的話在回報裡註明，不要停下來
symphony -version         # 已經裝了？會印出 "symphony v1.2.1 (darwin/arm64)"
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:7788/api/state   # 有 bridge 在執行嗎？401：symphony 1.x；200：比較舊的版本；000：沒有
claude plugin list --json # mod 已經裝了？找有 "id": "symphony-bridge@symphony" 的那一筆
brew --version            # 有 Homebrew 嗎（macOS、Linux）？有的話第 2 步可以用它安裝
```

在 Windows 上用 PowerShell：`$env:PROCESSOR_ARCHITECTURE` 的值是 `AMD64` 或 `ARM64`；`scoop --version` 有回應的話，就表示裝了 Scoop。

如果沒有 `claude`，就停下來，請使用者先安裝 Claude Code。如果程式和 mod 都已經就位，改看下面的「更新」。如果只有其中一個就位，就做另一個的步驟：程式是第 2 和第 3 步（`symphony -version` 印出的已經是最新版本的話就不用做），mod 是第 4 步，然後做第 5 步。已經在執行的 bridge 會一直用它啟動時的那個程式：如果有 bridge 在執行，你要在第 5 步之前把它停掉。

#### 第 2 步：取得程式

**用 Homebrew 或 Scoop 安裝。** 這個儲存庫同時也是 Homebrew 的 tap 和 Scoop 的 bucket。如果第 1 步發現有 Homebrew（macOS、Linux）或 Scoop（Windows），可以改用它安裝，省掉這一步其餘的部分和第 3 步：它會下載最新 Release 的壓縮檔，用 formula 或 manifest 裡的 SHA-256 檢查（這個值是 Release 發布時依它的 `checksums.txt` 寫進去的），再把 `symphony` 放進 `PATH`。接著做第 4 步，並在回報裡註明是用哪一個安裝的。

```bash
brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
brew install symphony
symphony -version          # 印出版本和平台
```

```powershell
scoop bucket add symphony https://github.com/feynmanzcys-bytes/symphony
scoop install symphony
symphony -version
```

否則，把這台電腦用的壓縮檔和 Release 裡的 `checksums.txt` 下載到一個專用的新資料夾，安裝任何東西之前先檢查壓縮檔。壓縮檔是 `symphony_<os>_<arch>.tar.gz`（`<os>` 是 `linux` 或 `darwin`（macOS），`<arch>` 是 `amd64` 或 `arm64`），以及 `symphony_windows_<arch>.zip`。

在 Linux 和 macOS 上：

```bash
d=$(mktemp -d)   # 一個只有你能讀取的新資料夾
echo "$d"        # 記下來：第 3 步要從這裡安裝
cd "$d"
curl -fsSLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_<os>_<arch>.tar.gz
curl -fsSLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/checksums.txt
if command -v sha256sum >/dev/null; then sha256sum -c --ignore-missing checksums.txt; else shasum -a 256 -c --ignore-missing checksums.txt; fi   # 必須印出 "symphony_<os>_<arch>.tar.gz: OK"
tar xzf symphony_<os>_<arch>.tar.gz              # 程式現在是 "$d/symphony"
```

如果檢查沒有印出 `OK`，就停下來：不要安裝這個程式，並告訴使用者。用 `gh` 代替 curl 的話，下載到 `$d`（資料夾還不存在時 gh 會自己建立），然後在那裡做同樣的檢查和 `tar`：

```bash
gh release download -R feynmanzcys-bytes/symphony -p 'symphony_<os>_<arch>.tar.gz' -p checksums.txt -D "$d"
```

在 Windows 上，用 PowerShell：

```powershell
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$zip = "symphony_windows_$arch.zip"
$d = Join-Path $env:TEMP ("symphony-" + [guid]::NewGuid())   # 一個新資料夾
New-Item -ItemType Directory -Path $d | Out-Null
$url = 'https://github.com/feynmanzcys-bytes/symphony/releases/latest/download'
Invoke-WebRequest -UseBasicParsing -Uri "$url/$zip" -OutFile (Join-Path $d $zip)
Invoke-WebRequest -UseBasicParsing -Uri "$url/checksums.txt" -OutFile (Join-Path $d 'checksums.txt')
$want = ((Select-String -SimpleMatch -Path (Join-Path $d 'checksums.txt') -Pattern $zip).Line -split '\s+')[0]
$got = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $d $zip)).Hash
if ($got -ne $want) { throw "checksum mismatch: $zip" } else { 'checksum OK' }
```

接著，如果裝了 `gh` 而且已經登入（`gh auth status` 成功），再檢查壓縮檔是在哪裡建置的：每個 Release 的每個壓縮檔都附有 GitHub 的證明（attestation），證明它是這個儲存庫的發布 workflow 建置的。不在終端機裡執行時，檢查通過的話 `gh` 什麼都不會印出，所以要看它的結束代碼：

```bash
gh attestation verify "$d/symphony_<os>_<arch>.tar.gz" --repo feynmanzcys-bytes/symphony && echo 'attestation OK'
```

```powershell
gh attestation verify (Join-Path $d $zip) --repo feynmanzcys-bytes/symphony
if ($LASTEXITCODE -eq 0) { 'attestation OK' }
```

v1.2.0 之前的 Release 沒有證明（會顯示 `no attestations found`），這些版本以 SHA-256 檢查為準。其他任何失敗都要停下來，和檢查沒通過時一樣。沒有 `gh` 或沒有登入的話，就跳過這項檢查，並在回報裡註明。

**從原始碼編譯**：在 Linux 或 macOS 上，使用者偏好這樣做時使用（需要 Go 1.27 以上）。要編譯出執行檔；`go run` 不行，因為 `-autostart` 不接受 `go run` 編出來的程式：

```bash
d=$(mktemp -d)
git clone https://github.com/feynmanzcys-bytes/symphony.git "$d/src"
cd "$d/src" && go build -o "$d/symphony" ./cmd/bridge   # 程式現在是 "$d/symphony"，和從壓縮檔取得的一樣
```

| 系統 | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz)（Intel） | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz)（Apple 晶片） |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

每個壓縮檔裡都有程式（`symphony`，Windows 上是 `symphony.exe`）、`LICENSE` 和 `README.md`。Release 裡的 `checksums.txt` 列出了每個壓縮檔的 SHA-256。

#### 第 3 步：放進 PATH

安裝第 2 步取得的程式 `$d/symphony`（如果你的 shell 在指令之間不保留變數，就直接寫出第 2 步印出的那個資料夾）。挑一個已經在使用者 `PATH` 裡、而且不用 `sudo` 就能寫入的資料夾，依序考慮 `~/.local/bin`、`/usr/local/bin`、`/opt/homebrew/bin`。

```bash
install -m 755 "$d/symphony" ~/.local/bin/symphony
xattr -d com.apple.quarantine ~/.local/bin/symphony 2>/dev/null || true   # macOS：這個執行檔沒有簽章
symphony -version          # 會印出 "symphony v1.2.1 (linux/amd64)"：版本和平台
```

如果這些資料夾都不在 `PATH` 裡，先問過使用者再做任何修改。使用者同意的話，就建立 `~/.local/bin`，把它加進 shell 的 rc 檔（`~/.zshrc` 或 `~/.bashrc`），並在回報裡說明；不同意的話，就把程式裝在使用者指定的地方，並告訴他們：只要 `symphony` 不在他們 Claude Code 工作階段的 `PATH` 裡，mod 就沒辦法自己啟動 bridge（第 5 步的隨登入啟動仍然可以用）。

在 Windows 上，把它解壓縮到一個專用的資料夾（`$d` 和 `$zip` 來自第 2 步；開了新的 shell 的話，要重新設定）：

```powershell
$dest = Join-Path $env:LOCALAPPDATA 'Programs\symphony'
Expand-Archive -LiteralPath (Join-Path $d $zip) -DestinationPath $dest -Force
& (Join-Path $dest 'symphony.exe') -version   # 會印出 "symphony v1.2.1 (windows/amd64)"
```

把那個資料夾加進使用者的 `Path` 是會一直留著的改動：要先問使用者。使用者同意的話，執行下面這行，再開一個新的 shell；不同意的話，就用完整路徑。用 `Expand-Archive` 解壓縮出來的程式不帶下載標記，SmartScreen 不會攔；用檔案總管「全部解壓縮」解出來、再按兩下執行的，可能會被攔：按「其他資訊」，再按「仍要執行」。

```powershell
[Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'User') + ";$dest", 'User')
```

如果做完這步，你自己的 shell 還是找不到 `symphony`（它的 `PATH` 是在你修改之前讀進來的），接下來的步驟就改用絕對路徑。

#### 第 4 步：安裝 mod

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

這會把這個儲存庫加入成名為 `symphony` 的外掛市集，並把 mod 安裝在使用者範圍，過程中不會詢問。也可以改用這個儲存庫在本機的一份 clone 當市集：`claude plugin install symphony-bridge --marketplace /absolute/path/to/symphony`。這樣工作階段會從那個資料夾讀取 mod，在那裡 `git pull` 就能更新，所以這份 clone 要放在會長期保留的地方，不要放在暫存資料夾裡。

確認：

```bash
claude plugin list --json   # 其中一筆是 "id": "symphony-bridge@symphony", "scope": "user", "enabled": true
```

- `"scope": "project"` 或 `"local"`：mod 只會在某一個資料夾裡載入。先執行 `claude plugin uninstall symphony-bridge@symphony`，再加上 `--scope user` 重新安裝。
- `"enabled": false`：執行 `claude plugin enable symphony-bridge@symphony`。

#### 第 5 步：啟動 bridge

如果第 1 步發現有 bridge 在執行，就先照下面「更新」裡的做法把它停掉：它跑的是舊程式，新程式只會發現它已經在那裡，然後就讓給它。停掉它會結束它啟動的工作階段，所以要先告訴使用者。

問使用者要不要每次登入都啟動 symphony。

要的話：

```bash
symphony -autostart on -open=false
```

這會立刻啟動 bridge，之後每次登入也會自動啟動（Linux 上是 systemd 使用者服務，macOS 上是 launchd 代理程式，Windows 上是登錄檔的 Run 機碼），並在日期和時間後面印出：

```
symphony now starts when you log in (<where>); it's running at http://127.0.0.1:7788/?key=<key>
```

不要的話：

```bash
symphony -background -open=false
```

這會在脫離終端機的背景啟動 bridge（電腦重新開機後不會再自動啟動），並在標準錯誤輸出上印出（前面帶著日期和時間）：

```
the symphony bridge runs in the background at http://127.0.0.1:7788/?key=<key>; its log: <data folder>/bridge.log
```

這個版本的 bridge 已經在執行時，它會（以同樣的方式）改印出 `a symphony bridge is already running at http://127.0.0.1:7788/?key=<key>`；這沒問題，不要再啟動另一個。`<key>` 是頁面金鑰，瀏覽器靠它登入辦公室：不要把它貼到別的地方。如果程式回答 `127.0.0.1:7788 is taken: another program, or a symphony bridge with another data folder, answers there`，就告訴使用者是什麼在那裡回應（舊版的 symphony bridge 也算：照「更新」裡的做法把它停掉），沒問過使用者之前不要改用其他位址。

用一個請求確認整條鏈路，請求裡出示資料目錄裡的頁面金鑰（經由標準輸入傳入，這樣金鑰就不會出現在其他使用者看得到的命令列上）：

```bash
printf 'X-Symphony-Key: %s\n' "$(cat "${SYMPHONY_DATA:-${XDG_CONFIG_HOME:-$HOME/.config}/symphony}/key")" | curl -s -H @- http://127.0.0.1:7788/api/launch/options      # Linux
printf 'X-Symphony-Key: %s\n' "$(cat "${SYMPHONY_DATA:-$HOME/Library/Application Support/symphony}/key")" | curl -s -H @- http://127.0.0.1:7788/api/launch/options   # macOS
```

```powershell
$data = if ($env:SYMPHONY_DATA) { $env:SYMPHONY_DATA } else { Join-Path $env:APPDATA 'symphony' }
(Invoke-WebRequest -UseBasicParsing -Headers @{ 'X-Symphony-Key' = (Get-Content (Join-Path $data 'key')) } -Uri http://127.0.0.1:7788/api/launch/options).Content
```

資料目錄在 Linux 上是 `~/.config/symphony`（有設定 `$XDG_CONFIG_HOME` 時是 `$XDG_CONFIG_HOME/symphony`），macOS 上是 `~/Library/Application Support/symphony`，Windows 上是 `%AppData%\symphony`；有設定 `SYMPHONY_DATA` 時則是它指定的地方。

回應裡會有 `"enabled":true` 和 `"mod":{"ok":true,"id":"symphony-bridge@symphony","version":"1.1.1"}`：表示 bridge 在執行、找到了 `claude`，而且 mod 安裝在工作階段會載入的地方。如果是 `"enabled":false`，`mod.code` 說明哪裡有問題，`mod.fix` 給出修正的指令：`mod_not_installed` 或 `mod_project_scope`（回到第 4 步）、`mod_disabled`（啟用它）、`mod_check_failed`（`claude plugin list --json` 執行失敗，多半是 Claude Code 版本太舊）。`"mod":null` 表示 bridge 在它的 `PATH` 裡完全找不到 `claude`（紀錄裡會寫）：把它放進去，或用 `-claude /path/to/claude` 啟動 bridge。回應是 `"code":"page_key"` 的話，表示請求沒有出示金鑰，或出示的金鑰不對：檢查 `cat` 有沒有找到金鑰檔（資料目錄見上面）；如果有找到，就表示正在執行的那個 bridge 是用另一個資料目錄啟動的。

bridge 的紀錄是資料目錄裡的 `bridge.log`；隨登入啟動時，Linux 上的紀錄在 `journalctl --user -u symphony`，macOS 上在 `~/Library/Logs/symphony.log`。

#### 第 6 步：回報給使用者

執行 `symphony` 開啟辦公室：bridge 在執行時，它會在預設瀏覽器裡開啟已經登入的辦公室，然後結束。它也會印出帶 `?key=` 的位址，這台電腦上的任何瀏覽器開啟它都能登入。

告訴使用者：

- 要開啟辦公室，就執行 `symphony`（或用滑鼠點兩下它）；已經登入的瀏覽器直接開 `http://127.0.0.1:7788/` 也可以。按空座位上的「+」招募第一位員工：選一個資料夾，就會在 bridge 內部、在那個資料夾裡啟動一個 Claude Code 工作階段；它在頁面上的視窗就是它的對話。
- 新員工沿用使用者的 Claude Code 設定（模型、思考強度、權限模式），除非在對話框裡另外選。`bypassPermissions` 會不經詢問直接執行所有指令和修改，工作階段讀到的內容就可能藉此讓它以使用者的身分執行指令；選了它時，對話框會發出警告。
- 每個工作階段都和一般的 Claude Code 工作階段一樣，消耗使用者的 Claude 方案或 API 額度；一項任務會同時執行 2–5 個工作階段。
- 只有從頁面啟動的工作階段（或帶著 `SYMPHONY_TOKEN` 手動啟動的，見下面的「啟動工作階段」）會出現在辦公室裡。使用者其他的 Claude Code 工作階段，包括正在做這次安裝的這一個，都不會顯示。
- 裝了什麼、裝在哪裡：程式的路徑、安裝在使用者範圍的 mod、bridge 是隨登入啟動還是在背景執行、對 `PATH` 或 rc 檔做的任何修改、你做的選擇，以及缺少 `git`（如果真的缺）。
- 怎麼全部復原：見下面的「解除安裝」。

規則：bridge 只在本機監聽（不是本機的位址它一律拒絕；除非使用者另外選了連接埠，否則維持預設的 `127.0.0.1:7788`），不要用 `sudo` 執行它，也不要在同一個位址、或用同一個資料目錄啟動第二個 bridge（symphony 會拒絕，並指出正在執行的是哪一個）。

#### 更新

照第 2 步下載新版。正在執行的 bridge 在重新啟動前跑的還是舊程式（而且在 Windows 上，執行中的程式沒辦法替換），所以要先用 `symphony -stop` 停掉它，照第 3 步替換程式，再照它原本的方式重新啟動：隨登入啟動的，就執行 `symphony -autostart on -open=false`（這也會立刻啟動它），不然就執行 `symphony -background -open=false`。如果 bridge 執行時用的 `-addr` 或 `-data` 不是預設值，這幾個指令都要加上同樣的參數：`-stop` 只會停掉那個資料目錄的 bridge（在 `-addr` 上，或在那個資料目錄的 bridge 實際執行的地方），沒有 bridge 回應時會說 `nothing to stop`。停掉 bridge 會結束它啟動的工作階段，所以要先告訴使用者。（`-stop` 是 v1.1.0 才有的；要停掉 v1.0.0 的 bridge，就結束它的程序：在 Linux 和 macOS 上用 `pkill -x symphony`，在 Windows 上用 `Stop-Process -Name symphony`。）

用 Homebrew 或 Scoop 安裝的話，以 `brew update && brew upgrade symphony`（或 `scoop update; scoop update symphony`）取代下載和第 3 步，前後一樣是 `-stop` 和重新啟動。如果 symphony 隨登入啟動，登入啟動項記的是 Homebrew 指向程式的連結（`bin/symphony`），升級後會指向新版本，所以這裡的 `-autostart on` 只是讓它現在重新啟動。

mod 用 `claude plugin update symphony-bridge@symphony` 跟著更新（用 clone 當市集的話，改成在那裡 `git pull`）。

程式和 mod 使用同一個版本號；每個版本改了什麼，請見 [CHANGELOG.md](CHANGELOG.md)。

#### 解除安裝

```bash
symphony -stop                                    # 停掉 bridge 和它的工作階段，不管它是怎麼啟動的（不是預設值時，加上它的 -addr 和 -data）
symphony -autostart off                           # 之後登入不再自動啟動（原本就沒有設定的話，它會說明）
claude plugin uninstall symphony-bridge@symphony  # mod
claude plugin marketplace remove symphony         # 市集項目
rm ~/.local/bin/symphony                          # 程式，看第 3 步放在哪裡
```

如果第 3 步把 `~/.local/bin` 加進了 shell 的 rc 檔，也要把那一行刪掉。在 Windows 上，用 PowerShell：

```powershell
symphony -stop                                               # 停掉 bridge 和它的工作階段（不是預設值時，加上它的 -addr 和 -data）
symphony -autostart off                                      # 之後登入不再自動啟動
claude plugin uninstall symphony-bridge@symphony
claude plugin marketplace remove symphony
$dest = Join-Path $env:LOCALAPPDATA 'Programs\symphony'
Remove-Item -Recurse -Force $dest                            # 程式
$p = ([Environment]::GetEnvironmentVariable('Path', 'User') -split ';' | Where-Object { $_ -and $_ -ne $dest }) -join ';'
[Environment]::SetEnvironmentVariable('Path', $p, 'User')    # 第 3 步加進去的 Path 項目
```

用 Homebrew 安裝的話，把 `rm` 那一行換成 `brew uninstall symphony` 和 `brew untap feynmanzcys-bytes/symphony`；用 Scoop 安裝的話，把 PowerShell 的最後四行換成 `scoop uninstall symphony` 和 `scoop bucket rm symphony`。

資料目錄（帳號和它們的 token、頁面金鑰、訊息紀錄、用量帳本、任務，以及 `lock`、`bridge.addr` 或被移到一旁的 `*.corrupt-<time>` 這類檔案）會留下來；要全部移除的話，就把它刪掉。刪掉它也會刪掉頁面金鑰：下次啟動時會產生新的，每個瀏覽器都得重新登入。在 Linux 上，如果瀏覽器是 snap 或 flatpak，`open.html` 會在那個瀏覽器自己的資料夾裡（`~/snap/<snap>/common` 或 `~/.var/app/<app id>`），也要一起刪掉。

## 能做什麼

- **每個工作階段都是一個對話視窗**：按一下員工，頁面中央會打開它的視窗：上面是員工資訊，下面是整個工作階段的聊天流：輸入過的提示詞、模型的思考（預設摺疊，進行中顯示橘色「思考中…」）、逐字串流的回答、工具呼叫，以及其他工作階段傳來的訊息（紫色）。在底部輸入就是給這個工作階段送提示詞，「停止」按鈕能中斷它。
- **在瀏覽器裡核准**：權限請求（執行指令、編輯檔案）和 AskUserQuestion 的提問會以卡片形式從該員工視窗底部升起、蓋住輸入框（在「同事」主題裡，卡片會留在對話中它出現的地方，你照樣可以打字），頂欄的紅色計數能直接帶你過去。權限請求卡片上的「本工作階段的……都允許」會讓同類的呼叫在工作階段結束前不再詢問、直接放行：所有修改、開頭相同的指令（`npm test`），或某個工具。MCP 伺服器的表單也會以卡片顯示；要到工作階段的終端機裡填寫，卡片會在視窗裡打開這個終端機。bridge 沒在執行、不認得這個帳號或無法信任時，工作階段會照常退回終端機處理，不會卡住。
- **像素風辦公室**：每個工作階段都是一位坐在固定座位上的員工。綠色是空閒，黃色是工作中，紅色是在等你核准或回答，灰色是下班。閒著的員工會先在座位上滑手機、打電動，過一陣子就滿辦公室閒晃：窩在休息區沙發看電視、玩手足球、追逐嬉鬧、在圓桌聊天、倒咖啡、看窗外，閒久了就去打盹，常常躲進會議室睡。角色的體型看模型，肌肉看思考強度。畫面是大公司的某一層樓，會填滿視窗：開放辦公區在正中央，左邊是掛著名牌的 3–5 人小會議室，右邊是會議室和休息區，大家搭電梯上下班。頂部橫條裡還能調整像素大小，共四段：辦公室不變，越往細調畫得越細、細節越多。
- **一搜就找到，一點就回來**：點一下閒著的員工，他就會回到自己的座位（一直待到下次工作）；頂部橫條裡的搜尋框可以用名字、工作階段名稱或資料夾找人（按 `/` 直接跳過去），點它旁邊的狀態計數（空閒、工作中、待處理、下班），就會把這個狀態的員工都標出來。橫條下方的小把手能把它收上去、再拉下來。
- **工作階段之間的訊息**：工作階段互傳訊息（SendMessage）時，會有一封信飛過辦公室，訊息紀錄也會把它們留下來。
- **從頁面啟動工作階段，不彈任何視窗**：按空座位上的「+」招募新員工，選好資料夾。模型、思考強度和權限模式沿用你的 Claude Code 設定（「預設」），除非你另外選；選了 `bypassPermissions` 會顯示紅色警告，說明它允許做什麼。勾選「告訴它辦公室裡還有誰」，工作階段的系統提示詞就會列出在線上的其他人（每段對話一次），方便它用 SendMessage 找他們。在下班員工的視窗裡按「上班」，他就會回到原本的座位，沿用上次工作時的選擇。工作階段跑在 bridge 內部自己的終端機裡，不會彈出視窗、不會搶走你的注意力；想看終端機時按員工視窗裡的「開啟終端機」，關掉那個視窗也不影響工作階段。
- **在會議室裡組隊工作**：按一下空著的小會議室，寫下任務，挑 2–5 位成員：員工（每人另開一個只做這項任務的工作階段），或臨時找來的顧問。預設由第一位成員擔任組長，用 SendMessage 分派各部分；每位成員都在自己的 git worktree、自己的分支上工作，誰也不會動到別人的檔案。任務結束之前，成員們都坐在會議室裡。
- **10 種語言**：English、简体中文、繁體中文、日本語、한국어、Español、Français、Deutsch、Português (Brasil) 和 Русский。頁面會跟著瀏覽器的語言，頂部橫條裡也可以切換；bridge 的訊息跟著 `-lang`，沒設定就跟著系統語言（`LANG` 之類），再沒有就用英文；mod 的訊息跟著 `SYMPHONY_LANG`（從頁面啟動的工作階段就是頁面的語言），沒設定就跟著系統語言，再沒有就用英文。
- **只在本機執行**：bridge 是單一個 Go 程式（只用標準函式庫；端對端測試另外用了 chromedp），只在本機監聽；頁面是原生 JavaScript，不依賴任何第三方服務。

![員工視窗：以聊天形式呈現的工作階段，核准卡片蓋在輸入框上](docs/approve.png)

## 環境需求

- Claude Code 2.1.316（在這個版本上測試過）。mod 用的是 Claude Code 的 function hooks，這個 API 還在搶先體驗階段，所以升級 Claude Code 之後，請先跑一次端對端測試。
- bridge 有 Linux、macOS、Windows 的現成程式；只有從原始碼編譯才需要 Go 1.27 以上。
- `git`，只有協作任務需要：任務裡的每位成員都在一個 git worktree 裡工作。
- 不需要其他東西：從頁面啟動的工作階段在每個平台上都跑在 bridge 內部。

## 執行 bridge

`symphony` 會啟動 bridge，並在瀏覽器開啟已經登入的辦公室（`-open=false` 可以關掉這個行為）。瀏覽器透過 `open.html` 以 cookie 的形式拿到 bridge 的頁面金鑰；這個檔案在資料目錄裡，只有你能讀取，所以金鑰從來不會出現在命令列上。在 Linux 上，如果你的瀏覽器是 snap 或 flatpak，這個檔案會改放在瀏覽器自己的資料夾裡，因為這類瀏覽器讀不到資料目錄。在還沒登入的瀏覽器裡開啟 <http://127.0.0.1:7788/>，會看到一個說明怎麼進去的頁面：再執行一次 `symphony`，或是開啟 bridge 在紀錄裡印出的帶 `?key=` 的位址。<http://127.0.0.1:7788/approve> 則是只處理核准的簡易頁面。`symphony` 已在執行時再啟動一次，只會開啟辦公室。在 Windows 上用滑鼠點兩下 `symphony.exe` 的話，它會在自己的視窗裡執行，關掉那個視窗 bridge 就會停止。

| 參數 | 預設值 | 用途 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | 監聽位址。主機部分必須是 `127.0.0.1`、`localhost` 或 `::1`（例如 `[::1]:7788`）；其他位址 bridge 一律拒絕 |
| `-data` | `$SYMPHONY_DATA`，沒有的話就用使用者設定資料夾裡的 `symphony` | 資料目錄：帳號和它們的 token（`accounts.json`）、頁面金鑰（`key`）和 `open.html`、訊息紀錄（`messages.json`）、用量帳本（`ledger.json`）、任務（`tasks.json`）、`-background` 的紀錄（`bridge.log`）、`lock` 和 `bridge.addr`（哪個 bridge 在用它、在哪個位址），以及損壞後被移到一旁的訊息紀錄或用量帳本（`*.corrupt-<time>`） |
| `-claude` | `claude` | claude 執行檔 |
| `-lang` | 系統語言，沒有的話就是 `en` | bridge 訊息使用的語言（預設值由 `SYMPHONY_LANG`、`LC_ALL`、`LC_MESSAGES` 或 `LANG` 決定） |
| `-mod` | 空白 | 開發用：透過 `--plugin-dir` 讓工作階段載入這個資料夾裡的 mod，覆蓋已安裝的版本 |
| `-web` | 空白 | 開發用：直接從這個資料夾提供頁面，修改後重新整理就會生效 |
| `-open` | `true` | bridge 啟動後在瀏覽器開啟辦公室；如果這個資料目錄的 bridge 已經在 `-addr` 上執行，就只開啟它的辦公室 |
| `-autostart` | — | `on`：之後每次登入都自動啟動 bridge（帶上一起給的參數），並且立刻啟動；`off`：取消 |
| `-background` | — | 在背景啟動 bridge：脫離終端機，紀錄寫到資料目錄裡的 `bridge.log`，等它有回應就返回 |
| `-stop` | — | 停掉這個資料目錄的 bridge（在 `-addr` 上，不在的話就在它實際執行的地方），不管它是怎麼啟動的（它的工作階段會結束），等它停止了才返回；它只會向能證明自己持有頁面金鑰的 bridge 出示這把金鑰 |
| `-version` | — | 顯示版本號後結束 |

如果 `-addr` 上回應的是別的東西（其他程式、用另一個資料目錄的 bridge，或是舊版的 symphony bridge），symphony 會說明這一點，然後停止；它只會向能證明自己持有頁面金鑰的 bridge 出示這把金鑰。一個資料目錄同一時間只屬於一個 bridge：如果指定的資料目錄已經有 bridge 在別的位址上執行，symphony 會說出它在哪裡執行、開啟它的辦公室，然後停止（要再啟動一個 bridge，就給它自己的 `-data`）。

**隨登入啟動**：`symphony -autostart on` 讓 bridge 之後每次登入都自動啟動（一起給的參數也會帶上，例如 `-addr`），並且立刻啟動；`symphony -autostart off` 取消。Linux 上是 systemd 使用者服務（用 `journalctl --user -u symphony` 查看紀錄），macOS 上是 launchd 代理程式（紀錄在 `~/Library/Logs/symphony.log`），Windows 上是登錄檔 Run 機碼裡的一個值（bridge 在主控台視窗裡執行，關掉視窗它就會停止，直到下次登入）。`symphony -stop` 不管 bridge 是怎麼啟動的都能停掉它，直到它再次被啟動（隨登入啟動的，就是到下次登入）。這需要 symphony 程式本身，`go run` 不行。隨登入啟動的 bridge 從你的 shell 只拿得到 `PATH`，所以 `-autostart on` 也會照你執行它時 shell 裡的值，把工作階段可能需要的變數存下來：`CLAUDE_CONFIG_DIR`、代理設定（`HTTP_PROXY`、`HTTPS_PROXY`、`NO_PROXY`、`ALL_PROXY`，大小寫皆可）、`NODE_EXTRA_CA_CERTS`、`SSL_CERT_FILE` 和 `SSL_CERT_DIR`，其他一概不存（API 金鑰也不存），存在資料目錄裡只有你能讀取的 `autostart-env.json`；這些變數改了之後，要再執行一次。一啟動就失敗的 bridge（資料目錄正被另一個 bridge 使用、`accounts.json` 損壞、它的位址被別的東西占用）會被重新啟動幾次，之後就不再理會：Linux 上是一分鐘內試 5 次（原因可以用 `systemctl --user status symphony` 查看），macOS 上是每分鐘試一次（原因看紀錄）；排除問題後再執行一次 `symphony -autostart on`，或是重新登入就好。

**從原始碼執行**，需要 Go 1.27 以上：`git clone https://github.com/feynmanzcys-bytes/symphony.git && cd symphony && go run ./cmd/bridge`。

## 啟動工作階段

**從頁面啟動**：按空座位上的「+」，輸入名字，再選好資料夾和選項。bridge 在自己內部的終端機裡執行 `claude`，不會彈出視窗。員工視窗以聊天的形式呈現工作階段；「開啟終端機」按鈕會開一個連到該工作階段的終端機視窗（在任何終端機裡執行 `symphony attach <id>` 也一樣，bridge 沒用預設值時要加上 `-addr` 和 `-data`；開不了終端機視窗時，頁面會顯示確切的指令。按 `Ctrl-]` 或關掉視窗即中斷連線，工作階段照常執行）。這些工作階段是 bridge 的子程序：停掉 bridge，工作階段也會結束。工作階段結束時（不論是從頁面停掉的、跟著 bridge 一起停止的，還是自己結束的），它留在背景執行的東西（例如工具用 `&` 放到背景的指令）也會跟著結束：在 Windows 上，由工作階段的工作物件（job object）把它們全部結束；在 Linux 和 macOS 上，bridge 依這些程序環境變數裡的啟動 ID 找出它們，但在 macOS 上，Apple 內建程式（`/bin/sleep`、`bash`、`zsh`、`perl`：系統不讓 `ps -E` 看到它們的環境變數）的環境變數它讀不到，所以留下來的 `node` 或 Homebrew 安裝的伺服器會結束，單純的 `sleep` 或 shell 指令碼則不會。

**從終端機啟動**：出示頁面金鑰建立一個帳號，拿到 token，再帶著 token 啟動 `claude`：

```bash
printf 'X-Symphony-Key: %s\n' "$(cat ~/.config/symphony/key)" | curl -H @- -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # 回應裡會有 token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

金鑰檔在 Linux 上是 `~/.config/symphony/key`，macOS 上是 `~/Library/Application Support/symphony/key`，Windows 上是 `%AppData%\symphony\key`；或是 `-data` / `SYMPHONY_DATA` 指定的資料夾裡的 `key`。

建議使用 `manual` 權限模式：在 `auto` 模式下，大部分動作都不會詢問任何人，能送到頁面的核准請求也就很少。如果 bridge 不在預設位址，還要設定 `SYMPHONY_BRIDGE`：本機上的 `http://127.0.0.1:<port>`、`http://localhost:<port>` 或 `http://[::1]:<port>`，或是一個 `https://` 位址；設成其他的話，mod 絕不會連線到 bridge。`SYMPHONY_LANG` 決定 mod 訊息使用的語言（沒設定就用系統語言，再沒有就用英文）。

工作階段把 token 送到任何地方之前，mod 會先確認那個位址上的 bridge 認得這個 token，確認時不會送出 token（`POST /api/hello`）。bridge 沒辦法接手這個工作階段時，工作階段會顯示下面其中一則訊息，核准請求留在終端機裡處理，不會卡住：

| 訊息（繁體中文／英文） | 意思 |
| --- | --- |
| `連不上 bridge` / `can't reach the bridge` | bridge 的位址上沒有任何回應，自己啟動 bridge 也沒用（或是這個位址不在本機）。 |
| `無法啟動 bridge（...）：<reason>` / `couldn't start the bridge ...: <reason>` | 沒有任何回應，`symphony -background` 也因為顯示的原因而失敗（例如這個資料目錄的 bridge 在別的位址上執行：請把 `SYMPHONY_BRIDGE` 設成那個位址）。 |
| `bridge 不認得這個 SYMPHONY_TOKEN` / `the bridge doesn't know this SYMPHONY_TOKEN` | bridge 沒有用這個 token 的帳號：帳號被刪掉了，或是 bridge 用的是另一個資料目錄。 |
| `不是這個工作階段能信任的 symphony bridge` / `isn't a symphony bridge this session can trust` | 這個位址上有東西回應，但無法證明它認得這個 token：可能是其他程式，或是比 mod 舊的 symphony（請更新它）。token 不會送出。 |
| `SYMPHONY_BRIDGE（...）必須在這台電腦上或使用 https` / `SYMPHONY_BRIDGE ... must be on this machine or use https` | 這個位址不是 mod 接受的寫法；完全不會連線到 bridge。 |
| `這個帳號已經有工作階段在線上` / `this account already has a session online` | 一個帳號同一時間只能給一個工作階段使用；請為這個工作階段另外建立一個帳號。 |
| `正在等你在辦公室網頁（...）上回覆` / `waiting for your answer in the office page` | 權限請求或提問正在辦公室網頁上等待，不在終端機裡；請到那裡回覆，或按 Esc 取消。每個工作階段只會顯示一次。 |

本機上 bridge 的位址沒有回應時，工作階段會自己用 `symphony -background -open=false -addr <that address>` 啟動 bridge（前提是 `symphony` 在 PATH 裡），並告訴你（`已在背景啟動 bridge` / `started the bridge ... in the background`）；工作階段結束後 bridge 仍會繼續執行。如果帳號不在預設的資料目錄，請設定 `SYMPHONY_DATA`。如果這個資料目錄的 bridge 已經在別的位址上執行，工作階段會說明這一點，然後退回終端機處理：請把 `SYMPHONY_BRIDGE` 設成那個位址。

## 協作任務

小會議室是 2–5 個工作階段組隊完成同一項任務的地方。任務需要 `git`。

**開始一項任務**：按一下空著的小會議室，寫清楚要做什麼。依你想要的順序勾選成員（2 人起，最多到會議室的座位數），替每個人選好模型和思考強度，再選全隊的權限模式（預設沿用 Claude Code 自己的設定；工作過的員工會帶著上次的模型和思考強度；選了 `bypassPermissions` 會顯示警告），然後按「開始任務」。成員分成兩種：

- **員工**：另開一個只做這項任務的工作階段，在這位員工的資料夾裡工作；他在座位上的工作階段維持原樣。一位員工同一時間只能參與一項任務；從沒工作過的員工還沒有資料夾，請先按一次「上班」。
- **顧問**：為這項任務臨時找來的成員，填的還是那幾個欄位（名字、資料夾、模型、思考強度）。顧問沒有座位，搭電梯上來，任務結束就離開。

**組長**：預設由第一位成員擔任組長：拆分任務、用 SendMessage 分派各部分、審查大家的成果、把其他人的分支合併到自己的分支，最後告訴你做了什麼。取消勾選「組長」那一項，每個人都會拿到整個任務，大家自己商量怎麼分。不管哪一種，bridge 都會給每位成員送出第一則提示詞：團隊裡有誰（成員之間用 SendMessage 依名字找人）、每個人的分支，以及規則。

**git 這邊**：每位成員都在自己資料夾的一個獨立 git worktree 裡、自己的分支 `symphony/<task id>/<member>`（例如 `symphony/T1/bob`；同名的第二位成員會拿到 `-2`）上工作，誰也不會動到別人的檔案，你自己的工作目錄也維持原樣。worktree 放在資料夾裡的 `.symphony/worktrees/`，靠 `.git/info/exclude` 讓 git 忽略（不會動到你的 `.gitignore`）。任務結束時，成員還沒提交的變更會提交到他的分支上，他 worktree 裡被 git 忽略的檔案（建置產物、`.env`、`node_modules` 之類）會移到資料夾裡的 `.symphony/kept/<member>-<task id>/`，接著移除 worktree。symphony 從不刪除 `.symphony/kept/`；裡面的東西用不到了，請自己刪掉。

- 在你自己的儲存庫裡，什麼都不會合併：分支會留下來等你合併；有組長的話，組長的分支會匯集所有人的成果。
- 不是 git 儲存庫的資料夾，會為了這項任務變成一個，之後也會一直是：先 `git init`，再把裡面的東西全部提交一次，但常含有機密或體積龐大的檔案除外（`.env`、`.env.*`、`*.pem`、`*.key`、`*.p12`、`*.pfx`、`id_rsa*`、`id_ecdsa*`、`id_ed25519*`、`.npmrc`、`.netrc`、`.pypirc`、`node_modules/`、`.DS_Store`），這些會列進 `.git/info/exclude`。這些檔案還是留在資料夾裡，但不在成員的 worktree 裡，需要它們的成員得自己安裝或建立一份。任務結束時，成員的分支會合併進資料夾的分支，資料夾裡就是合併好的結果；`.git` 和分支都會保留。如果任務沒能啟動，資料夾會變回原本不是儲存庫的樣子。
- 合併失敗的話，這次合併會被復原，基底維持合併前的樣子；分支都留著，上面的成果也在，任務視窗會說明是哪個分支發生衝突。任務結束時如果還有其他步驟失敗，就維持失敗時的樣子，任務視窗會說明是什麼失敗了。

**辦公室裡**：任務期間成員們都坐在會議室的桌邊，員工的座位上只剩一張空椅子。會議室的名牌會顯示任務編號；名牌上的燈在空閒時是綠的，任務進行中變黃，有成員在等你核准或回答時變紅。按一下會議室就會打開任務視窗：任務內容、成員和各自的資料夾與分支、團隊對話，還有「結束任務」。按一下會議室裡的成員本人，打開的也是任務視窗；在任務視窗裡按一下某位成員，他的對話視窗就會貼在任務視窗旁邊打開，和員工視窗一樣，一次只開一位（按另一位成員就切換過去）。頂欄的紅色計數和搜尋框打開任務成員時也是這樣。

**團隊對話**：任務視窗裡列出成員之間互傳的訊息，每則都附上發送者的像素小人和名字，內文以 `@` 加收件人開頭。下方有個輸入框，你也可以自己傳話給團隊：`@bob-T1 …` 只傳給這位成員（寫幾個 `@names` 就傳給這幾位；輸入 `@` 會跳出成員清單，可用方向鍵、Enter 或滑鼠點選），不加 `@` 就傳給任務裡的所有人。Enter 送出，Shift+Enter 換行。

在任務視窗裡結束任務，或是最後一個工作階段結束時，任務就會結束：bridge 會停掉成員的工作階段，照上面說的把 git 這邊收拾好，再刪除顧問。bridge 停止時還在進行的任務，會在 bridge 下次啟動時用同樣的方式結束。

## 帳號與座位

- 在 symphony 裡，一個身分就是一個名字、一個 token 加上一個固定座位。一個帳號同一時間只能給一個工作階段使用（它參與的任務裡還能再有一個）；其他工作階段會被拒絕，它們的核准請求會留在終端機裡處理。
- 這裡不管 Claude 的登入：所有工作階段可以共用同一個 Claude 登入。
- 資料存放在 `~/.config/symphony`（Linux）、`~/Library/Application Support/symphony`（macOS）或 `%AppData%\symphony`（Windows），或是 `-data` / `SYMPHONY_DATA` 指定的地方。`accounts.json` 存有 token，`key` 存有頁面金鑰；兩者都只有檔案擁有者能讀取（0600）；在 Windows 上則靠使用者資料夾的權限來保護。

## 費用與權限

- symphony 啟動的每個工作階段都是 Claude Code 工作階段，和其他工作階段一樣消耗你的 Claude 方案或 API 額度。Opus 加上最高思考強度用得很快，而一項任務會同時執行 2–5 個工作階段。
- `bypassPermissions` 會不經詢問直接執行所有指令和修改。這時工作階段讀到的內容（檔案、網頁、工具的輸出）就可能讓它以你的身分執行指令。在啟動和任務對話框裡選它時，會顯示紅色警告；這兩個對話框預設沿用你的 Claude Code 設定。
- 從頁面啟動的工作階段，bridge 會替你回答 Claude Code 的「信任此資料夾」問題，所以只在你信任的資料夾裡啟動工作階段。
- 「本工作階段的……都允許」只到工作階段結束為止，而且只放行 Claude Code 本來就會詢問的操作，不越過你的權限設定：和其他指令串在一起的指令（`;`、`&&`、`|`、重新導向）一律不放行，也絕不越過拒絕規則或你所屬組織的設定。一條指令規則涵蓋所有開頭相同的指令（`npm test` 也涵蓋 `npm test -- --watch`）。

## 安全性

頁面可以啟動工作階段，等於能在這台電腦上執行指令，所以 bridge 會把關誰能連到它：

- **頁面金鑰**：辦公室和它的 API 都需要頁面金鑰，這是瀏覽器透過 `symphony` 開啟的連結拿到的 cookie。金鑰就是資料目錄裡的 `key` 檔，只有你能讀取；程式則在 `X-Symphony-Key` 標頭裡出示它。
- **只限這台電腦**：bridge 只在本機監聽，也只接受來自本機的連線。它會拒絕發給其他主機的請求（DNS rebinding），也會拒絕其他網站透過你的瀏覽器送來的寫入請求；只要你的瀏覽器表明請求來自其他網站或其他連接埠，它的 API 一律不回應，就算只是點開一個連結也一樣。其他網站也不能用框架嵌入它的頁面。頁面只載入 bridge 自己的檔案（Content Security Policy）。
- **mod 會先確認 bridge**，確認過才送出 token；`symphony` 程式也會先確認過才出示頁面金鑰，所以在 bridge 的位址上監聽的其他東西兩樣都拿不到。
- **防不了的事**：以你自己的使用者身分執行的程式，可以讀取資料目錄裡的頁面金鑰和 token，就像它們能讀你的任何檔案一樣。另外，瀏覽器會把一個網站的 cookie 送給它的每一個連接埠：這台電腦上另一位使用者在 `127.0.0.1` 上執行的伺服器，只要你的瀏覽器開啟了它，或是在 bridge 停止、辦公室卻還開在某個分頁裡時占用了 bridge 的連接埠，就會拿到頁面金鑰的 cookie。在和別人共用的電腦上，請先關掉辦公室再停止 bridge；如果金鑰可能已經外洩，就刪掉資料目錄裡的 `key` 檔（下次啟動時會產生新的）。
- 這些程式沒有簽章。用瀏覽器下載的話，macOS 可能因為無法驗證開發者而拒絕執行：請到「系統設定 → 隱私權與安全性」按「強制打開」，或執行一次 `xattr -d com.apple.quarantine symphony`。在 Windows 上，用檔案總管「全部解壓縮」解出來的程式，按兩下時 SmartScreen 可能會攔：按「其他資訊」，再按「仍要執行」（安裝步驟用的 `Expand-Archive` 不會在它身上留下下載標記）。Release 裡的 `checksums.txt` 列出了每個壓縮檔的 SHA-256，每個壓縮檔也都附有 GitHub 的證明（attestation），說明它是在哪裡建置的；安裝的第 2 步兩者都會檢查。

要回報安全漏洞，請見 [SECURITY.md](SECURITY.md)。

## 平台

| 系統 | 從頁面啟動的工作階段 | 「開啟終端機」 | 測試情況 |
| --- | --- | --- | --- |
| Linux | bridge 內部的偽終端機 | 圖形終端機（x-terminal-emulator、gnome-terminal、konsole、xterm）執行 `symphony attach` | 完整的端對端測試 |
| macOS | bridge 內部的偽終端機 | Terminal.app 執行 `symphony attach` | 完整的端對端測試 |
| Windows | bridge 內部的偽主控台（ConPTY） | 新開主控台視窗執行 `symphony attach` | 單元測試和不需要 tmux 的端到端測試；真實工作階段在 Windows 11 上驗證過 |

每個平台上都由 bridge 替你回答「信任此資料夾」。

## 開發

```bash
go test -race ./...                         # bridge 和翻譯的單元測試
claude plugin test mods/symphony-bridge     # mod 的單元測試
claude plugin validate .                    # 檢查市集
claude plugin validate mods/symphony-bridge  # 檢查 mod
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # 端對端測試：會啟動真的工作階段，並消耗 token
```

- 參與貢獻：[CONTRIBUTING.md](CONTRIBUTING.md) 說明環境設定、發 pull request 之前要跑的檢查，以及規則；[CLAUDE.md](CLAUDE.md) 是程式碼的詳細地圖。
- 開發 mod：執行 `go run ./cmd/bridge -mod mods/symphony-bridge`，從頁面啟動的工作階段就會透過 `--plugin-dir` 載入你手上的這份 mod；自己手動啟動的工作階段，請自行加上 `--plugin-dir mods/symphony-bridge`。
- 開發頁面：執行 `go run ./cmd/bridge -web web`，重新整理就能看到你的修改。
- 不開真的工作階段也想看看辦公室：另外啟動第二個 bridge，給它一個自己的資料目錄，並灌入模擬的活動：`D=$(mktemp -d)`，`go run ./cmd/bridge -addr 127.0.0.1:7799 -data "$D"`，接著在另一個終端機裡（用同一個 `D`）執行 `go run ./cmd/demo -base http://127.0.0.1:7799 -data "$D"`（demo 會從那個資料目錄讀取 bridge 的頁面金鑰）。
- 這份 README 裡的截圖：`SYMPHONY_E2E=1 SYMPHONY_E2E_DOCS=1 go test -tags e2e -run TestDocsScreens ./e2e/` 會用模擬的活動重新拍一次（不消耗 token）。
- 翻譯：每種語言一個檔案，頁面的是 `web/locales/<lang>.json`，bridge 的是 `internal/i18n/locales/<lang>.json`，鍵和 `en.json` 相同；mod 的那幾則訊息在 `mods/symphony-bridge/hooks/messages.ts`。單元測試會檢查每種語言的鍵是否齊全、`{placeholders}` 是否和英文一致；端對端測試 `TestI18n` 會用每種語言開啟頁面。
- 端對端測試需要 claude、tmux（測試自己驅動的工作階段用）和 Chromium 或 Chrome，所以在 Linux 和 macOS 上執行。`TestOffice` 和 `TestModStartsBridge` 會啟動真的工作階段並消耗 token，其他測試不會。預設使用這個儲存庫裡的 mod；設定 `SYMPHONY_E2E_MOD=installed` 就會改用已安裝的 mod。截圖會存到 `e2e/artifacts/`。
- 發布版本：調高 `mods/symphony-bridge/.claude-plugin/plugin.json` 裡的版本號，在 `CHANGELOG.md` 加一節 `## vX.Y.Z (date)`，把 dev 合併進 main，然後在 main 的那個提交上建立標籤並推送（`git tag -a v1.0.0 -m v1.0.0 && git push origin v1.0.0`）；GitHub Actions 會檢查標籤和 `plugin.json` 的版本是否相同、那一節是否存在，接著為所有平台編譯程式，為這些壓縮檔附上它們是在哪裡建置的證明，並發布一個 Release，發布說明以那一節開頭（`.github/workflows/release.yml`；加上 `-rc.1` 這類後綴就會變成預先發行版）；不是預先發行版的話，之後還會把 Homebrew 的 formula 和 Scoop 的 manifest 提交到 main（dev 可以快轉的話，也提交到 dev），所以發布後兩個分支都要 pull 一下。`.github/release.sh v1.0.0 dist` 可以在本機打包出同樣的壓縮檔，在 Linux 上連每個位元組都相同。

| 資料夾 | 內容 |
| --- | --- |
| `mods/symphony-bridge/` | Claude Code mod（TypeScript） |
| `.claude-plugin/marketplace.json` | `claude plugin install` 讀取的外掛市集 |
| `cmd/bridge/`、`internal/bridge/` | bridge |
| `internal/i18n/` | 語言偵測和 bridge 的翻譯 |
| `web/` | 辦公室頁面、核准頁面和它們的翻譯，會編譯進 bridge |
| `cmd/demo/` | 給辦公室用的模擬活動 |
| `e2e/` | 端對端測試 |
| `docs/` | bridge 和頁面之間的協定，以及截圖 |

## 授權條款

[MIT](LICENSE)

symphony 是獨立專案，與 Anthropic 沒有隸屬關係，也未獲得 Anthropic 的認可或贊助。Claude 和 Claude Code 是 Anthropic, PBC 的商標。
