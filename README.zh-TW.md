# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | **繁體中文** | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

把一群 Claude Code 工作階段放進一間像素風辦公室：誰在工作、誰在等你，一眼就看得出來；每個員工都有自己的對話視窗，可以直接跟它交談，權限請求和 AskUserQuestion 的提問也在那裡按一下就能處理。

![辦公室](docs/office.png)

## 安裝

最快的方法：打開 Claude Code，貼上

> 幫我安裝 https://github.com/feynmanzcys-bytes/symphony

Claude Code 會讀這份 README，把下一節的步驟全部做完。想自己動手安裝，照著同樣的步驟做就行。

### 給 AI agent：安裝手冊

你會讀到這裡，是因為使用者請你安裝 symphony。請在 shell 裡依序執行下面的步驟，最後完成第 6 步的回報。除非步驟裡有交代，否則不要問使用者任何問題；例行的選擇自己決定，並在回報裡列出來。

symphony 由兩個部分組成，兩個都要裝：

1. **`symphony` 程式**（bridge）：單一個執行檔，是位於 `http://127.0.0.1:7788/` 的本機網頁伺服器，提供辦公室頁面並執行工作階段。它必須以 `symphony` 這個名字放在使用者的 `PATH` 裡，因為工作階段找不到正在執行的 bridge 時，mod 會用 `symphony -background` 啟動它。
2. **`symphony-bridge` 外掛**（mod）：來自這個儲存庫市集的 Claude Code 外掛，安裝在使用者範圍。它只在帶著 `SYMPHONY_TOKEN` 啟動的工作階段裡起作用，在其他工作階段裡什麼都不做。

#### 第 1 步：檢查這台電腦

```bash
uname -sm                 # Linux/macOS：決定下面用哪個壓縮檔（x86_64 → amd64；arm64/aarch64 → arm64）
claude --version          # 需要 Claude Code 2.1.x（在 2.1.316 上測試過）
git --version             # 只有協作任務需要；沒有的話在回報裡註明，不要停下來
symphony -version         # 已經裝了？會印出 "symphony v0.7.1 (darwin/arm64)"
curl -s http://127.0.0.1:7788/api/state | head -c 40   # 已經有 bridge 在執行？會印出 {"type":"snapshot",...
claude plugin list --json # mod 已經裝了？找 "id":"symphony-bridge@symphony"
```

在 Windows 上用 PowerShell：`$env:PROCESSOR_ARCHITECTURE` 的值是 `AMD64` 或 `ARM64`，並用 `Invoke-WebRequest http://127.0.0.1:7788/api/state` 取代 curl。

如果沒有 `claude`，就停下來，請使用者先安裝 Claude Code。如果程式和 mod 都已經就位，改看下面的「更新」。

#### 第 2 步：取得程式

用第一個行得通的方法：

- **A. 用 `gh`**（儲存庫還是私有的時候也能用，只要 `gh auth status` 顯示已登入）：
  ```bash
  gh release download -R feynmanzcys-bytes/symphony --pattern 'symphony_<os>_<arch>.tar.gz' -D /tmp/symphony-dl
  ```
- **B. 用 `curl`**（儲存庫公開之後才能用；出現 404 就改用 A 或 C）：
  ```bash
  curl -fL -o /tmp/symphony-dl/symphony_<os>_<arch>.tar.gz https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_<os>_<arch>.tar.gz
  ```
- **C. 從原始碼編譯**（需要 Go 1.27 以上；第 4 步會再用到這份 clone）：
  ```bash
  git clone https://github.com/feynmanzcys-bytes/symphony.git /tmp/symphony-src
  cd /tmp/symphony-src && go build -o symphony ./cmd/bridge
  ```
  要編譯出執行檔；`go run` 不行，因為 `-autostart` 不接受 `go run` 編出來的程式。

| 系統 | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz)（Intel） | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz)（Apple 晶片） |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

每個壓縮檔裡都有程式（`symphony`，Windows 上是 `symphony.exe`）、`LICENSE` 和 `README.md`。Release 裡的 `checksums.txt` 列出了每個壓縮檔的 SHA-256（`sha256sum -c --ignore-missing checksums.txt`，macOS 上用 `shasum -a 256 -c`）。

#### 第 3 步：放進 PATH

挑一個已經在使用者 `PATH` 裡、而且不用 `sudo` 就能寫入的資料夾，依序考慮 `~/.local/bin`、`/usr/local/bin`、`/opt/homebrew/bin`。如果一個都沒有，就建立 `~/.local/bin`，把它加進 shell 的 rc 檔（`~/.zshrc` 或 `~/.bashrc`），並在回報裡說明。

```bash
tar xzf /tmp/symphony-dl/symphony_<os>_<arch>.tar.gz -C /tmp/symphony-dl
install -m 755 /tmp/symphony-dl/symphony ~/.local/bin/symphony
xattr -d com.apple.quarantine ~/.local/bin/symphony 2>/dev/null || true   # macOS：這個執行檔沒有簽章
symphony -version          # 會印出 "symphony v0.7.1 (linux/amd64)"：版本和平台
```

在 Windows 上：`Expand-Archive symphony_windows_<arch>.zip -DestinationPath "$env:LOCALAPPDATA\Programs\symphony"`，然後把那個資料夾加進使用者的 `Path`（`[Environment]::SetEnvironmentVariable('Path', "$([Environment]::GetEnvironmentVariable('Path','User'));$env:LOCALAPPDATA\Programs\symphony", 'User')`），再開一個新的 shell。第一次執行時 SmartScreen 可能會跳出警告：按「其他資訊」，再按「仍要執行」。

如果做完這步，你自己的 shell 還是找不到 `symphony`（它的 `PATH` 是在你修改之前讀進來的），接下來的步驟就改用絕對路徑。

#### 第 4 步：安裝 mod

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

這會把這個儲存庫加入成名為 `symphony` 的外掛市集，並把 mod 安裝在使用者範圍，過程中不會詢問。它會用 git clone 這個儲存庫，所以儲存庫還是私有的時候，需要能用 git 存取 GitHub；如果因為這樣失敗，就改用一份 clone 當市集（第 2 步 C 的那份，或另外 clone 一份）：`claude plugin install symphony-bridge --marketplace /absolute/path/to/symphony`。用 clone 當市集時，工作階段會從那個資料夾讀取 mod，在那裡 `git pull` 就能更新。

確認：

```bash
claude plugin list --json   # 其中一筆是 "id":"symphony-bridge@symphony", "scope":"user", "enabled":true
```

- `"scope":"project"` 或 `"local"`：mod 只會在某一個資料夾裡載入。先執行 `claude plugin uninstall symphony-bridge@symphony`，再加上 `--scope user` 重新安裝。
- `"enabled":false`：執行 `claude plugin enable symphony-bridge@symphony`。

#### 第 5 步：啟動 bridge

```bash
symphony -autostart on -open=false
```

這會立刻啟動 bridge，之後每次登入也會自動啟動（Linux 上是 systemd 使用者服務，macOS 上是 launchd 代理程式，Windows 上是登錄檔的 Run 機碼），並印出

```
symphony now starts when you log in (<where>); it's running at http://127.0.0.1:7788/
```

如果使用者說不想隨登入啟動，就改執行 `symphony -background -open=false`：它會在脫離終端機的背景啟動 bridge，並印出 `the symphony bridge runs in the background at http://127.0.0.1:7788/; its log: <data folder>/bridge.log`。已經有 bridge 在執行時，兩個指令都會回答 `a symphony bridge is already running at http://127.0.0.1:7788/`；這沒問題，不要再啟動另一個。

用一個請求確認整條鏈路：

```bash
curl -s http://127.0.0.1:7788/api/launch/options
```

回應裡會有 `"enabled":true` 和 `"mod":{"ok":true,"id":"symphony-bridge@symphony","version":"0.7.1"}`：表示 bridge 在執行、找到了 `claude`，而且 mod 安裝在工作階段會載入的地方。如果是 `"enabled":false`，`mod.code` 說明哪裡有問題，`mod.fix` 給出修正的指令：`mod_not_installed` 或 `mod_project_scope`（回到第 4 步）、`mod_disabled`（啟用它）、`mod_check_failed`（`claude plugin list --json` 執行失敗，多半是 Claude Code 版本太舊）。`"mod":null` 表示 bridge 在它的 `PATH` 裡完全找不到 `claude`（紀錄裡會寫）：把它放進去，或用 `-claude /path/to/claude` 啟動 bridge。

bridge 的紀錄是資料目錄裡的 `bridge.log`（Linux 上是 `~/.config/symphony`，macOS 上是 `~/Library/Application Support/symphony`，Windows 上是 `%AppData%\symphony`）；隨登入啟動時，Linux 上的紀錄在 `journalctl --user -u symphony`，macOS 上在 `~/Library/Logs/symphony.log`。

#### 第 6 步：回報給使用者

在瀏覽器開啟辦公室（macOS 上用 `open http://127.0.0.1:7788/`，Linux 上用 `xdg-open`，Windows 上用 `start`），然後告訴使用者：

- 辦公室在 `http://127.0.0.1:7788/`。按空座位上的「+」招募第一位員工：選一個資料夾，就會在 bridge 內部、在那個資料夾裡啟動一個 Claude Code 工作階段；它在頁面上的視窗就是它的對話。
- 只有從頁面啟動的工作階段（或帶著 `SYMPHONY_TOKEN` 手動啟動的，見下面的「啟動工作階段」）會出現在辦公室裡。使用者其他的 Claude Code 工作階段，包括正在做這次安裝的這一個，都不會顯示。
- 裝了什麼、裝在哪裡：程式的路徑、安裝在使用者範圍的 mod、bridge 是隨登入啟動還是在背景執行、對 `PATH` 做的任何修改，以及缺少 `git`（如果真的缺）。
- 怎麼全部復原：見下面的「解除安裝」。

規則：bridge 只能綁定在本機（`-addr` 維持 `127.0.0.1:7788`；頁面沒有登入功能），不要用 `sudo` 執行它，也不要在同一個位址啟動第二個 bridge。

#### 更新

照第 2 步下載新版，再照第 3 步替換程式；mod 用 `claude plugin update symphony-bridge@symphony` 跟著更新（用 clone 當市集的話，改成在那裡 `git pull`）。正在執行的 bridge 在重新啟動前跑的還是舊程式：先 `symphony -autostart off` 再 `symphony -autostart on -open=false`（或用當初啟動它的方式停掉再啟動）。停掉 bridge 會結束它啟動的工作階段，所以要先告訴使用者。程式和 mod 使用同一個版本號；每個版本改了什麼，請見 [CHANGELOG.md](CHANGELOG.md)。

#### 解除安裝

```bash
symphony -autostart off                           # 停掉 bridge 和它的工作階段，之後登入也不再自動啟動
claude plugin uninstall symphony-bridge@symphony  # mod
claude plugin marketplace remove symphony         # 市集項目
rm ~/.local/bin/symphony                          # 程式，看第 3 步放在哪裡
```

資料目錄（帳號和它們的 token、訊息紀錄、任務）會留下來；要全部移除的話，就把它刪掉。

## 能做什麼

- **每個工作階段都是一個對話視窗**：按一下員工，頁面中央會打開它的視窗：上面是員工資訊，下面是整個工作階段的聊天流：輸入過的提示詞、模型的思考（預設摺疊，進行中顯示橘色「思考中…」）、逐字串流的回答、工具呼叫，以及其他工作階段傳來的訊息（紫色）。在底部輸入就是給這個工作階段送提示詞，「停止」按鈕能中斷它。
- **在瀏覽器裡核准**：權限請求（執行指令、編輯檔案）和 AskUserQuestion 的提問會以卡片形式從該員工視窗底部升起、蓋住輸入框，頂欄的紅色計數能直接帶你過去。bridge 沒在執行或不認得這個帳號時，工作階段會照常退回終端機處理，不會卡住。
- **像素風辦公室**：每個工作階段都是一位坐在固定座位上的員工。綠色是空閒，黃色是工作中，紅色是在等你核准或回答，灰色是下班。閒著的員工會先在座位上滑手機、打電動，過一陣子就滿辦公室閒晃：窩在休息區沙發看電視、玩手足球、追逐嬉鬧、在圓桌聊天、倒咖啡、看窗外，閒久了就去打盹，常常躲進會議室睡。角色的體型看模型，肌肉看思考強度。畫面是大公司的某一層樓，會填滿視窗：開放辦公區在正中央，左邊是掛著名牌的 3–5 人小會議室，右邊是會議室和休息區，大家搭電梯上下班。頂部橫條裡還能調整像素大小，共四段：辦公室不變，越往細調畫得越細、細節越多。
- **一搜就找到，一點就回來**：點一下閒著的員工，他就會回到自己的座位（一直待到下次工作）；頂部橫條裡的搜尋框可以用名字、工作階段名稱或資料夾找人（按 `/` 直接跳過去），點它旁邊的狀態計數（空閒、工作中、待處理、下班），就會把這個狀態的員工都標出來。橫條下方的小把手能把它收上去、再拉下來。
- **工作階段之間的訊息**：工作階段互傳訊息（SendMessage）時，會有一封信飛過辦公室，訊息紀錄也會把它們留下來。
- **從頁面啟動工作階段，不彈任何視窗**：按空座位上的「+」招募新員工，選好資料夾、模型、思考強度和權限模式（新員工預設全部放行、Opus、最高思考強度）；在下班員工的視窗裡按「上班」，他就會回到原本的座位。工作階段跑在 bridge 內部自己的終端機裡，不會彈出視窗、不會搶走你的注意力；想看終端機時按員工視窗裡的「開啟終端機」，關掉那個視窗也不影響工作階段。
- **在會議室裡組隊工作**：按一下空著的小會議室，寫下任務，挑 2–5 位成員：員工（每人另開一個只做這項任務的工作階段），或臨時找來的顧問。預設由第一位成員擔任組長，用 SendMessage 分派各部分；每位成員都在自己的 git worktree、自己的分支上工作，誰也不會動到別人的檔案。任務結束之前，成員們都坐在會議室裡。
- **10 種語言**：English、简体中文、繁體中文、日本語、한국어、Español、Français、Deutsch、Português (Brasil) 和 Русский。頁面會跟著瀏覽器的語言，頂部橫條裡也可以切換；bridge 的訊息跟著 `-lang`，沒設定就跟著系統語言（`LANG` 之類），再沒有就用英文；mod 的訊息跟著 `SYMPHONY_LANG`（從頁面啟動的工作階段就是頁面的語言），沒設定就跟著系統語言，再沒有就用英文。
- **只在本機執行**：bridge 是單一個 Go 程式（只用標準函式庫；端對端測試另外用了 chromedp），頁面是原生 JavaScript，不依賴任何第三方服務。

![在瀏覽器裡核准](docs/approve.png)

## 環境需求

- Claude Code 2.1.316（在這個版本上測試過）。mod 用的是 Claude Code 的 function hooks，這個 API 還在搶先體驗階段，所以升級 Claude Code 之後，請先跑一次端對端測試。
- bridge 有 Linux、macOS、Windows 的現成程式；只有從原始碼編譯才需要 Go 1.27 以上。
- `git`，只有協作任務需要：任務裡的每位成員都在一個 git worktree 裡工作。
- 不需要其他東西：從頁面啟動的工作階段在每個平台上都跑在 bridge 內部。

## 執行 bridge

`symphony` 會啟動 bridge，並在瀏覽器開啟辦公室 <http://127.0.0.1:7788/>（`-open=false` 可以關掉這個行為）；<http://127.0.0.1:7788/approve> 則是只處理核准的簡易頁面。bridge 已在執行時再啟動一次，只會開啟辦公室。在 Windows 上用滑鼠點兩下 `symphony.exe` 的話，它會在自己的視窗裡執行，關掉那個視窗 bridge 就會停止。

| 參數 | 預設值 | 用途 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | 監聽位址。頁面目前還沒有登入功能，所以不要綁定到本機以外的位址 |
| `-data` | `$SYMPHONY_DATA`，沒有的話就用使用者設定資料夾裡的 `symphony` | 資料目錄：帳號和它們的 token（`accounts.json`）、訊息紀錄（`messages.json`）、任務（`tasks.json`），以及 `-background` 的紀錄（`bridge.log`） |
| `-claude` | `claude` | claude 執行檔 |
| `-lang` | 系統語言，沒有的話就是 `en` | bridge 訊息使用的語言（預設值由 `SYMPHONY_LANG`、`LC_ALL`、`LC_MESSAGES` 或 `LANG` 決定） |
| `-mod` | 空白 | 開發用：透過 `--plugin-dir` 讓工作階段載入這個資料夾裡的 mod，覆蓋已安裝的版本 |
| `-web` | 空白 | 開發用：直接從這個資料夾提供頁面，修改後重新整理就會生效 |
| `-open` | `true` | bridge 啟動後在瀏覽器開啟辦公室；如果 `-addr` 上已有 bridge 在執行，就只開啟它的辦公室 |
| `-autostart` | — | `on`：之後每次登入都自動啟動 bridge（帶上一起給的參數），並且立刻啟動；`off`：取消 |
| `-background` | — | 在背景啟動 bridge：脫離終端機，紀錄寫到資料目錄裡的 `bridge.log`，等它有回應就返回 |
| `-version` | — | 顯示版本號後結束 |

**隨登入啟動**：`symphony -autostart on` 讓 bridge 之後每次登入都自動啟動（一起給的參數也會帶上，例如 `-addr`），並且立刻啟動；`symphony -autostart off` 取消。Linux 上是 systemd 使用者服務（用 `journalctl --user -u symphony` 查看紀錄），macOS 上是 launchd 代理程式（紀錄在 `~/Library/Logs/symphony.log`），Windows 上是登錄檔 Run 機碼裡的一個值（bridge 在主控台視窗裡執行，關掉視窗它就會停止，直到下次登入）。這需要 symphony 程式本身，`go run` 不行。

**從原始碼執行**，需要 Go 1.27 以上：`git clone https://github.com/feynmanzcys-bytes/symphony.git && cd symphony && go run ./cmd/bridge`。

## 啟動工作階段

**從頁面啟動**：按空座位上的「+」，輸入名字，再選好資料夾和選項。bridge 在自己內部的終端機裡執行 `claude`，不會彈出視窗。員工視窗以聊天的形式呈現工作階段；「開啟終端機」按鈕會開一個連到該工作階段的終端機視窗（在任何終端機裡執行 `symphony attach <id>` 也一樣；按 `Ctrl-]` 或關掉視窗即中斷連線，工作階段照常執行）。這些工作階段是 bridge 的子程序：停掉 bridge，工作階段也會結束。

**從終端機啟動**：先建立一個帳號拿到 token，再帶著它啟動 `claude`：

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # 回應裡會有 token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

建議使用 `manual` 權限模式：在 `auto` 模式下，大部分動作都不會詢問任何人，能送到頁面的核准請求也就很少。如果 bridge 不在預設位址，還要設定 `SYMPHONY_BRIDGE=http://host:port`。`SYMPHONY_LANG` 決定 mod 訊息使用的語言（沒設定就用系統語言，再沒有就用英文）。

本機上 bridge 的位址沒有回應時，工作階段會自己用 `symphony -background -open=false -addr <that address>` 啟動 bridge（前提是 `symphony` 在 PATH 裡），工作階段結束後 bridge 仍會繼續執行。如果帳號不在預設的資料目錄，請設定 `SYMPHONY_DATA`。

## 協作任務

小會議室是 2–5 個工作階段組隊完成同一項任務的地方。任務需要 `git`，mod 也必須是 0.7.0（`claude plugin update symphony-bridge@symphony`）。

**開始一項任務**：按一下空著的小會議室，寫清楚要做什麼。依你想要的順序勾選成員（2 人起，最多到會議室的座位數），替每個人選好模型和思考強度，再選全隊的權限模式（預設全部放行），然後按「開始任務」。成員分成兩種：

- **員工**：另開一個只做這項任務的工作階段，在這位員工的資料夾裡工作；他在座位上的工作階段維持原樣。一位員工同一時間只能參與一項任務；從沒工作過的員工還沒有資料夾，請先按一次「上班」。
- **顧問**：為這項任務臨時找來的成員，填的還是那幾個欄位（名字、資料夾、模型、思考強度）。顧問沒有座位，搭電梯上來，任務結束就離開。

**組長**：預設由第一位成員擔任組長：拆分任務、用 SendMessage 分派各部分、審查大家的成果、把其他人的分支合併到自己的分支，最後告訴你做了什麼。取消勾選「組長」那一項，每個人都會拿到整個任務，大家自己商量怎麼分。不管哪一種，bridge 都會給每位成員送出第一則提示詞：團隊裡有誰（成員之間用 SendMessage 依名字找人）、每個人的分支，以及規則。

**git 這邊**：每位成員都在自己資料夾的一個獨立 git worktree 裡、自己的分支 `symphony/<task id>/<member>`（例如 `symphony/T1/bob`；同名的第二位成員會拿到 `-2`）上工作，誰也不會動到別人的檔案，你自己的工作目錄也維持原樣。worktree 放在資料夾裡的 `.symphony/worktrees/`，靠 `.git/info/exclude` 讓 git 忽略（不會動到你的 `.gitignore`）。任務結束時，成員還沒提交的變更會提交到他的分支上，什麼都不會遺失，接著移除 worktree。

- 在你自己的儲存庫裡，什麼都不會合併：分支會留下來等你合併；有組長的話，組長的分支會匯集所有人的成果。
- 不是 git 儲存庫的資料夾，會為了這項任務變成一個（`git init`，再把裡面的東西全部提交一次），之後也會一直是。任務結束時，成員的分支會合併進來，資料夾裡就是合併好的結果；`.git` 和分支都會保留。
- 合併失敗的話，這次合併會被復原，基底維持合併前的樣子；分支都留著，上面的成果也在，任務視窗會說明是哪個分支發生衝突。

**辦公室裡**：任務期間成員們都坐在會議室的桌邊，員工的座位上只剩一張空椅子。會議室的名牌會顯示任務編號；名牌上的燈在空閒時是綠的，任務進行中變黃，有成員在等你核准或回答時變紅。按一下會議室就會打開任務視窗：任務內容、成員和各自的資料夾與分支、團隊對話，還有「結束任務」。按一下會議室裡的成員本人，打開的也是任務視窗；在任務視窗裡按一下某位成員，他的對話視窗就會貼在任務視窗旁邊打開，和員工視窗一樣，一次只開一位（按另一位成員就切換過去）。頂欄的紅色計數和搜尋框打開任務成員時也是這樣。

**團隊對話**：任務視窗裡列出成員之間互傳的訊息，每則都附上發送者的像素小人和名字，內文以 `@` 加收件人開頭。下方有個輸入框，你也可以自己傳話給團隊：`@bob-T1 …` 只傳給這位成員（寫幾個 `@names` 就傳給這幾位；輸入 `@` 會跳出成員清單，可用方向鍵、Enter 或滑鼠點選），不加 `@` 就傳給任務裡的所有人。Enter 送出，Shift+Enter 換行。

在任務視窗裡結束任務，或是最後一個工作階段結束時，任務就會結束：bridge 會停掉成員的工作階段，照上面說的把 git 這邊收拾好，再刪除顧問。bridge 停止時還在進行的任務，會在 bridge 下次啟動時用同樣的方式結束。

## 帳號與座位

- 在 symphony 裡，一個身分就是一個名字、一個 token 加上一個固定座位。一個帳號同一時間只能給一個工作階段使用（它參與的任務裡還能再有一個）；其他工作階段會被拒絕，它們的核准請求會留在終端機裡處理。
- 這裡不管 Claude 的登入：所有工作階段可以共用同一個 Claude 登入。
- 資料存放在 `~/.config/symphony`（Linux）、`~/Library/Application Support/symphony`（macOS）或 `%AppData%\symphony`（Windows），或是 `-data` / `SYMPHONY_DATA` 指定的地方。`accounts.json` 存有 token，只有檔案擁有者能讀取（0600）；在 Windows 上則靠使用者資料夾的權限來保護。

## 安全性

- bridge 只接受發給本機的請求，也會拒絕其他網站透過你的瀏覽器送來的寫入請求：因為頁面可以啟動工作階段，等於能在這台電腦上執行指令。
- 頁面目前還沒有登入功能，所以**不要**把 bridge 綁定到本機以外的位址。
- 這些程式沒有簽章。用瀏覽器下載的話，macOS 可能因為無法驗證開發者而拒絕執行：請到「系統設定 → 隱私權與安全性」按「強制打開」，或執行一次 `xattr -d com.apple.quarantine symphony`。Windows 的 SmartScreen 也可能跳出警告：按「其他資訊」，再按「仍要執行」。Release 裡的 `checksums.txt` 列出了每個壓縮檔的 SHA-256。

## 平台

| 系統 | 從頁面啟動的工作階段 | 「開啟終端機」 | 測試情況 |
| --- | --- | --- | --- |
| Linux | bridge 內部的偽終端機 | 圖形終端機（x-terminal-emulator、gnome-terminal、konsole、xterm）執行 `symphony attach` | 完整的端對端測試 |
| macOS | bridge 內部的偽終端機 | Terminal.app 執行 `symphony attach` | 完整的端對端測試 |
| Windows | bridge 內部的偽主控台（ConPTY） | 新開主控台視窗執行 `symphony attach` | 只做過交叉編譯和單元測試 |

每個平台上都由 bridge 替你回答「信任此資料夾」。

## 開發

```bash
go test -race ./...                         # bridge 和翻譯的單元測試
claude plugin test mods/symphony-bridge     # mod 的單元測試
claude plugin validate .                    # 檢查市集
claude plugin validate mods/symphony-bridge  # 檢查 mod
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # 端對端測試：會啟動真的工作階段，並消耗 token
```

- 開發 mod：執行 `go run ./cmd/bridge -mod mods/symphony-bridge`，從頁面啟動的工作階段就會透過 `--plugin-dir` 載入你手上的這份 mod；自己手動啟動的工作階段，請自行加上 `--plugin-dir mods/symphony-bridge`。
- 開發頁面：執行 `go run ./cmd/bridge -web web`，重新整理就能看到你的修改。
- 不開真的工作階段也想看看辦公室：另外啟動第二個 bridge，並灌入模擬的活動：`go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo`，接著執行 `go run ./cmd/demo -base http://127.0.0.1:7799`。
- 翻譯：每種語言一個檔案，頁面的是 `web/locales/<lang>.json`，bridge 的是 `internal/i18n/locales/<lang>.json`，鍵和 `en.json` 相同；mod 的那幾則訊息在 `mods/symphony-bridge/hooks/messages.ts`。單元測試會檢查每種語言的鍵是否齊全、`{placeholders}` 是否和英文一致；端對端測試 `TestI18n` 會用每種語言開啟頁面。
- 端對端測試需要 claude、tmux（測試自己驅動的工作階段用）和 Chromium 或 Chrome，所以在 Linux 和 macOS 上執行。預設使用這個儲存庫裡的 mod；設定 `SYMPHONY_E2E_MOD=installed` 就會改用已安裝的 mod。截圖會存到 `e2e/artifacts/`。
- 發布版本：調高 `mods/symphony-bridge/.claude-plugin/plugin.json` 裡的版本號，在 `CHANGELOG.md` 加一節 `## vX.Y.Z (date)`，把 dev 合併進 main，然後在 main 的那個提交上建立標籤並推送（`git tag -a v0.7.0 -m v0.7.0 && git push origin v0.7.0`）；GitHub Actions 會檢查標籤和 `plugin.json` 的版本是否相同、那一節是否存在，接著為所有平台編譯程式，並發布一個 Release，發布說明以那一節開頭（`.github/workflows/release.yml`；加上 `-rc.1` 這類後綴就會變成預先發行版）。`.github/release.sh v0.7.0 dist` 可以在本機打包出同樣的壓縮檔。

| 資料夾 | 內容 |
| --- | --- |
| `mods/symphony-bridge/` | Claude Code mod（TypeScript） |
| `.claude-plugin/marketplace.json` | `claude plugin install` 讀取的外掛市集 |
| `cmd/bridge/`、`internal/bridge/` | bridge |
| `internal/i18n/` | 語言偵測和 bridge 的翻譯 |
| `web/` | 辦公室頁面、核准頁面和它們的翻譯，會編譯進 bridge |
| `cmd/demo/` | 給辦公室用的模擬活動 |
| `e2e/` | 端對端測試 |

## 授權條款

[MIT](LICENSE)
