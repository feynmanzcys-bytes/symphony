# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | **繁體中文** | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

把一群 Claude Code 工作階段放進一間像素風辦公室：誰在工作、誰在等你，一眼就看得出來；每個員工都有自己的對話視窗，可以直接跟它交談，權限請求和 AskUserQuestion 的提問也在那裡按一下就能處理。

![辦公室](docs/office.png)

## 能做什麼

- **每個工作階段都是一個對話視窗**：按一下員工，頁面中央會打開它的視窗：上面是員工資訊，下面是整個工作階段的聊天流：輸入過的提示詞、模型的思考（預設摺疊，進行中顯示橘色「思考中…」）、逐字串流的回答、工具呼叫，以及其他工作階段傳來的訊息（紫色）。在底部輸入就是給這個工作階段送提示詞，「停止」按鈕能中斷它。
- **在瀏覽器裡核准**：權限請求（執行指令、編輯檔案）和 AskUserQuestion 的提問會以卡片形式從該員工視窗底部升起、蓋住輸入框，頂欄的紅色計數能直接帶你過去。bridge 沒在執行或不認得這個帳號時，工作階段會照常退回終端機處理，不會卡住。
- **像素風辦公室**：每個工作階段都是一位坐在固定座位上的員工。綠色是空閒，黃色是工作中，紅色是在等你核准或回答，灰色是下班。閒著的員工會先在座位上滑手機、打電動，過一陣子就滿辦公室閒晃：窩在休息區沙發看電視、玩手足球、追逐嬉鬧、在圓桌聊天、倒咖啡、看窗外，閒久了就去打盹，常常躲進會議室睡；角色的體型看模型，肌肉看思考強度。畫面是大公司的某一層樓，會填滿視窗：開放辦公區在正中央，左邊是掛著名牌的 3–5 人小會議室，右邊是大會議室和休息區，大家搭電梯上下班。頂部橫條裡還能調整像素大小，共四段：樓不變，越往細調畫得越細、細節越多。
- **一搜就找到，一點就回來**：點一下閒著的員工，他就會回到自己的座位，一直待到下次工作；頂部橫條裡的搜尋框可以用名字或資料夾找人（按 `/` 直接跳過去）；點它旁邊的狀態牌（空閒、工作中、待處理、下班），就會把這一類員工都標出來；橫條下方的小把手能把它收上去、再拉下來。
- **工作階段之間的訊息**：工作階段互傳訊息（SendMessage）時，會有一封信飛過辦公室，訊息紀錄也會把它們留下來。
- **從頁面啟動工作階段，不彈任何視窗**：按空座位上的「+」招募新員工，選好資料夾、模型、思考強度和權限模式（新員工預設全部放行、Opus、最高思考強度）；在下班員工的視窗裡按「上班」，他就會回到原本的座位。工作階段跑在 bridge 內部自己的終端機裡，不會彈出視窗、不會搶走你的注意力；想看終端機時按員工視窗裡的「開啟終端機」，關掉那個視窗也不影響工作階段。
- **在會議室裡組隊工作**：按一下空著的小會議室，寫下任務，挑 2–5 位成員：員工（每人另開一個只做這項任務的工作階段），或臨時找來的顧問。預設由第一位成員擔任組長，用 SendMessage 分派各部分；每位成員都在自己的 git worktree、自己的分支上工作，誰也不會動到別人的檔案。任務結束之前，成員們都坐在會議室裡。
- **10 種語言**：English、简体中文、繁體中文、日本語、한국어、Español、Français、Deutsch、Português (Brasil) 和 Русский。頁面會跟著瀏覽器的語言，頂部橫條裡也可以切換；bridge 的訊息跟著系統語言（或 `-lang`），mod 的訊息則跟著啟動這個工作階段的頁面。
- **只在本機執行**：bridge 是單一個 Go 程式（只用標準函式庫），頁面是原生 JavaScript，不依賴任何第三方服務。

![在瀏覽器裡核准](docs/approve.png)

## 環境需求

- Claude Code 2.1.316（在這個版本上測試過）。mod 用的是 Claude Code 的 function hooks，這個 API 還在搶先體驗階段，所以升級 Claude Code 之後，請先跑一次端對端測試。
- bridge 有 Linux、macOS、Windows 的現成程式，下載就能用；只有從原始碼執行才需要 Go 1.27 以上。
- `git`，只有協作任務需要：任務裡的每位成員都在一個 git worktree 裡工作。
- 不需要其他東西：從頁面啟動的工作階段在每個平台上都跑在 bridge 內部。

## 安裝

### 1. 安裝 mod

在 Claude Code 裡輸入：

```
/plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

按 `y` 加入市集，再選擇安裝範圍（選預設的使用者範圍就好）。也可以在命令列安裝：

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

如果你已經 clone 了這個儲存庫，也可以改用你本機的 clone 當作市集。這樣 mod 和 bridge 就是同一份程式碼，`git pull` 之後，每個新開的工作階段都會用到新版本：

```bash
claude plugin install symphony-bridge --marketplace /path/to/symphony
```

安裝在使用者範圍時，每個 Claude Code 工作階段都會載入這個 mod，但它只在帶著 `SYMPHONY_TOKEN` 啟動的工作階段裡起作用；在其他地方它什麼都不做。

更新：`claude plugin update symphony-bridge@symphony`。解除安裝：`claude plugin uninstall symphony-bridge@symphony`。

### 2. 啟動 bridge

從[最新 Release](https://github.com/feynmanzcys-bytes/symphony/releases/latest) 下載符合你系統的壓縮檔，解壓縮後執行裡面的 `symphony`：

| 系統 | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz)（Intel） | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz)（Apple 晶片） |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

例如在 Linux 上：

```bash
curl -fLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz
tar xzf symphony_linux_amd64.tar.gz
./symphony
```

在 Windows 上，解壓縮後執行 `symphony.exe`；用滑鼠點兩下的話，它會在自己的視窗裡執行，關掉視窗 bridge 就會停止。

這些程式沒有簽章，第一次執行時可能會被擋下。用瀏覽器下載的話，macOS 可能會顯示「無法驗證開發者」：請到「系統設定 → 隱私權與安全性」按「強制打開」，或執行一次 `xattr -d com.apple.quarantine symphony`。Windows 的 SmartScreen 也可能跳出警告：按「其他資訊」，再按「仍要執行」。Release 裡的 `checksums.txt` 列出了每個壓縮檔的 SHA-256。

程式和 mod 使用同一個版本號：`symphony -version` 會顯示程式的版本，`claude plugin list` 會列出 mod 的版本。要更新時，下載新版的壓縮檔，再執行 `claude plugin update symphony-bridge@symphony`。每個版本改了什麼，請見 [CHANGELOG.md](CHANGELOG.md)。

也可以從原始碼執行（需要 Go 1.27 以上）：

```bash
git clone https://github.com/feynmanzcys-bytes/symphony.git
cd symphony
go run ./cmd/bridge
```

bridge 一啟動就會在瀏覽器開啟辦公室 <http://127.0.0.1:7788/>（不需要的話加上 `-open=false`）；<http://127.0.0.1:7788/approve> 則是只處理核准的簡易頁面。bridge 已在執行時再啟動一次，只會開啟辦公室。

| 參數 | 預設值 | 用途 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | 監聽位址。頁面目前還沒有登入功能，所以不要綁定到本機以外的位址 |
| `-data` | `$SYMPHONY_DATA`，沒有的話就用使用者設定資料夾裡的 `symphony` | 存放帳號（含 token）和訊息紀錄的地方 |
| `-claude` | `claude` | claude 執行檔 |
| `-lang` | 系統語言 | bridge 訊息使用的語言 |
| `-mod` | 空白 | 開發用：透過 `--plugin-dir` 讓工作階段載入這個資料夾裡的 mod，覆蓋已安裝的版本 |
| `-web` | 空白 | 開發用：直接從這個資料夾提供頁面，修改後重新整理就會生效 |
| `-open` | `true` | bridge 啟動後在瀏覽器開啟辦公室；如果 `-addr` 上已有 bridge 在執行，就只開啟它的辦公室 |
| `-autostart` | — | `on`：之後每次登入都自動啟動 bridge（帶上一起給的參數），並且立刻啟動；`off`：取消 |
| `-background` | — | 在背景啟動 bridge：脫離終端機，紀錄寫到資料夾裡的 `bridge.log`，等它有回應就返回 |
| `-version` | — | 顯示版本號後結束 |

**隨登入啟動**：`symphony -autostart on` 讓 bridge 之後每次登入都自動啟動（一起給的參數也會帶上，例如 `-addr`），並且立刻啟動；`symphony -autostart off` 取消。Linux 上是 systemd 使用者服務（用 `journalctl --user -u symphony` 查看紀錄），macOS 上是 launchd 代理程式（紀錄在 `~/Library/Logs/symphony.log`），Windows 上是登錄檔 Run 機碼裡的一個值（bridge 在主控台視窗裡執行，關掉視窗它就會停止，直到下次登入）。這需要 symphony 程式本身，`go run` 不行。

### 3. 啟動工作階段

**從頁面啟動**：按空座位上的「+」，輸入名字，再選好資料夾和選項。bridge 在自己內部的終端機裡執行 `claude`，不會彈出視窗。員工視窗以聊天的形式呈現工作階段；「開啟終端機」按鈕會開一個連到該工作階段的終端機視窗（在任何終端機裡執行 `symphony attach <id>` 也一樣；按 `Ctrl-]` 或關掉視窗即中斷連線，工作階段照常執行）。這些工作階段是 bridge 的子程序：停掉 bridge，工作階段也會結束。

**從終端機啟動**：先建立一個帳號拿到 token，再帶著它啟動 `claude`：

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # 回應裡會有 token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

建議使用 `manual` 權限模式：在 `auto` 模式下，大部分動作都不會詢問任何人，能送到頁面的核准請求也就很少。如果 bridge 不在預設位址，還要設定 `SYMPHONY_BRIDGE=http://host:port`。`SYMPHONY_LANG` 決定 mod 訊息使用的語言。

本機上 bridge 的位址沒有回應時，工作階段會自己用 `symphony -background` 啟動 bridge（前提是 `symphony` 在 PATH 裡），工作階段結束後 bridge 仍會繼續執行。如果帳號不在預設的資料夾，請設定 `SYMPHONY_DATA`。

## 協作任務

小會議室是 2–5 個工作階段組隊完成同一項任務的地方。任務需要 `git`，mod 也必須是 0.7.0（`claude plugin update symphony-bridge@symphony`）。

**開始一項任務**：按一下空著的小會議室，寫清楚要做什麼。依你想要的順序勾選成員（2 人起，最多到會議室的座位數），替每個人選好模型和思考強度，再選全隊的權限模式（預設全部放行），然後按「開始任務」。成員分成兩種：

- **員工**：另開一個只做這項任務的工作階段，在這位員工的資料夾裡工作；他在座位上的工作階段維持原樣。一位員工同一時間只能參與一項任務；從沒工作過的員工還沒有資料夾，請先按一次「上班」。
- **顧問**：為這項任務臨時找來的成員，填的還是那幾個欄位（名字、資料夾、模型、思考強度）。顧問沒有座位，搭電梯上來，任務結束就離開。

**組長**：預設由第一位成員擔任組長：拆分任務、用 SendMessage 分派各部分、審查大家的成果、把其他人的分支合併到自己的分支，最後告訴你做了什麼。取消勾選「組長」那一項，每個人都會拿到整個任務，大家自己商量怎麼分。不管哪一種，bridge 都會給每位成員送出第一則提示詞：團隊裡有誰（成員之間用 SendMessage 依名字找人）、每個人的分支，以及規則。

**git 這邊**：每位成員都在自己資料夾的一個獨立 git worktree 裡、自己的分支 `symphony/<任務>/<名字>` 上工作，誰也不會動到別人的檔案，你自己的工作目錄也維持原樣。worktree 放在資料夾裡的 `.symphony/worktrees/`，靠 `.git/info/exclude` 讓 git 忽略（不會動到你的 `.gitignore`）。任務結束時，成員還沒提交的變更會提交到他的分支上，什麼都不會遺失，接著移除 worktree。

- 在你自己的儲存庫裡，什麼都不會合併：分支會留下來等你合併；有組長的話，組長的分支會匯集所有人的成果。
- 不是 git 儲存庫的資料夾，會為了這項任務變成一個（`git init`，再把裡面的東西全部提交一次），之後也會一直是。任務結束時，成員的分支會合併進來，資料夾裡就是合併好的結果；`.git` 和分支都會保留。
- 合併失敗的話，這次合併會被復原，資料夾維持合併前的樣子；分支都留著，上面的成果也在，任務視窗會說明是哪個分支發生衝突。

**辦公室裡**：任務期間成員們都坐在會議室的桌邊，員工的座位上只剩一張空椅子。會議室的名牌會顯示任務編號；名牌上的燈在空閒時是綠的，任務進行中變黃，有成員在等你核准或回答時變紅。按一下會議室就會打開任務視窗：任務內容、成員和各自的資料夾與分支，還有「結束任務」。按一下那裡的成員，或是會議室裡的他本人，就會打開他的對話視窗，和員工視窗一樣。

在任務視窗裡結束任務，或是最後一個工作階段結束時，任務就會結束：bridge 會停掉成員的工作階段，照上面說的把 git 這邊收拾好，再刪除顧問。bridge 停止時還在進行的任務，會在 bridge 下次啟動時用同樣的方式結束。

## 帳號與座位

- 在 symphony 裡，一個身分就是一個名字、一個 token 加上一個固定座位。一個帳號同一時間只能給一個工作階段使用（它參與的任務裡還能再有一個）；其他工作階段會被拒絕，它們的核准請求會留在終端機裡處理。
- 這裡不管 Claude 的登入：所有工作階段可以共用同一個 Claude 登入。
- 資料存放在 `~/.config/symphony`（Linux）、`~/Library/Application Support/symphony`（macOS）或 `%AppData%\symphony`（Windows）。`accounts.json` 存有 token，只有檔案擁有者能讀取（0600）；在 Windows 上則靠使用者資料夾的權限來保護。

## 安全性

- bridge 只接受發給本機的請求，也會拒絕其他網站透過你的瀏覽器送來的寫入請求：因為頁面可以啟動工作階段，等於能在這台電腦上執行指令。
- 頁面目前還沒有登入功能，所以**不要**把 bridge 綁定到本機以外的位址。

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
claude plugin validate .                    # 檢查市集和 mod
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # 端對端測試：會啟動真的工作階段，並消耗 token
```

- 開發 mod：執行 `go run ./cmd/bridge -mod mods/symphony-bridge`，從頁面啟動的工作階段就會透過 `--plugin-dir` 載入你手上的這份 mod；自己手動啟動的工作階段，請自行加上 `--plugin-dir mods/symphony-bridge`。
- 開發頁面：執行 `go run ./cmd/bridge -web web`，重新整理就能看到你的修改。
- 不開真的工作階段也想看看辦公室：另外啟動第二個 bridge，並灌入模擬的活動：`go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo`，接著執行 `go run ./cmd/demo -base http://127.0.0.1:7799`。
- 翻譯：每種語言一個檔案，頁面的是 `web/locales/<lang>.json`，bridge 的是 `internal/i18n/locales/<lang>.json`，鍵和 `en.json` 相同；mod 的那幾則訊息在 `mods/symphony-bridge/hooks/messages.ts`。單元測試會檢查每種語言的鍵是否齊全、`{佔位符}` 是否和英文一致；端對端測試 `TestI18n` 會用每種語言開啟頁面。
- 端對端測試需要 claude、tmux（測試自己驅動的工作階段用）和 Chromium 或 Chrome，所以在 Linux 和 macOS 上執行。預設使用這個儲存庫裡的 mod；設定 `SYMPHONY_E2E_MOD=installed` 就會改用已安裝的 mod。截圖會存到 `e2e/artifacts/`。
- 發布版本：在 main 的提交上建立版本標籤並推送（`git tag -a v0.3.0 -m v0.3.0 && git push origin v0.3.0`），GitHub Actions 就會為所有平台編譯程式並發布 Release（`.github/workflows/release.yml`）。標籤必須和 `plugin.json` 裡 mod 的版本號相同。`.github/release.sh v0.3.0 dist` 可以在本機打包出同樣的壓縮檔。每個版本都要在 `CHANGELOG.md` 裡寫一節，它會放在發布說明的最前面。

| 資料夾 | 內容 |
| --- | --- |
| `mods/symphony-bridge/` | Claude Code mod（TypeScript） |
| `.claude-plugin/marketplace.json` | `/plugin install` 讀取的外掛市集 |
| `cmd/bridge/`、`internal/bridge/` | bridge |
| `internal/i18n/` | 語言偵測和 bridge 的翻譯 |
| `web/` | 辦公室頁面、核准頁面和它們的翻譯，會編譯進 bridge |
| `cmd/demo/` | 給辦公室用的模擬活動 |
| `e2e/` | 端對端測試 |

## 授權條款

[MIT](LICENSE)
