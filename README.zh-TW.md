# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | **繁體中文** | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

把一群 Claude Code 工作階段放進一間像素風辦公室：誰在工作、誰在等你，一眼就看得出來；權限請求和 AskUserQuestion 的提問，在網頁的彈出視窗裡按一下就能處理。

![辦公室](docs/office.png)

## 能做什麼

- **在瀏覽器裡核准**：權限請求（執行指令、編輯檔案）和 AskUserQuestion 的提問都會轉到網頁上。bridge 沒在執行或不認得這個帳號時，工作階段會照常退回終端機處理，不會卡住。
- **像素風辦公室**：每個工作階段都是一位坐在固定座位上的員工。綠色是空閒，黃色是工作中，紅色是在等你核准或回答，灰色是下班。閒著的員工會先在座位上滑手機、打電動，過一陣子就滿辦公室閒晃：窩在休息區沙發看電視、玩手足球、追逐嬉鬧、在圓桌聊天、倒咖啡、看窗外，閒久了就去打盹，常常躲進會議室睡；角色的體型看模型，肌肉看思考強度。畫面是大公司的某一層樓，會填滿視窗：開放辦公區在正中央，左邊是掛著名牌的 3–5 人小會議室，右邊是大會議室和休息區，大家搭電梯上下班。頂部橫條裡還能調整像素大小，共四段：樓不變，越往細調畫得越細、細節越多。
- **一搜就找到，一點就回來**：點一下閒著的員工，他就會回到自己的座位，一直待到下次工作；頂部橫條裡的搜尋框可以用名字或資料夾找人（按 `/` 直接跳過去）；點它旁邊的狀態牌（空閒、工作中、待處理、下班），就會把這一類員工都標出來；橫條下方的小把手能把它收上去、再拉下來。
- **工作階段之間的訊息**：工作階段互傳訊息（SendMessage）時，會有一封信飛過辦公室，訊息紀錄也會把它們留下來。
- **從頁面啟動工作階段**：按空座位上的「+」招募新員工，選好資料夾、模型、思考強度和權限模式；在下班員工的卡片上按「上班」，他就會回到原本的座位。
- **10 種語言**：English、简体中文、繁體中文、日本語、한국어、Español、Français、Deutsch、Português (Brasil) 和 Русский。頁面會跟著瀏覽器的語言，頂部橫條裡也可以切換；bridge 的訊息跟著系統語言（或 `-lang`），mod 的訊息則跟著啟動這個工作階段的頁面。
- **只在本機執行**：bridge 是單一個 Go 程式（只用標準函式庫），頁面是原生 JavaScript，不依賴任何第三方服務。

![在瀏覽器裡核准](docs/approve.png)

## 環境需求

- Claude Code 2.1.314（在這個版本上測試過）。mod 用的是 Claude Code 的 function hooks，這個 API 還在搶先體驗階段，所以升級 Claude Code 之後，請先跑一次端對端測試。
- bridge 有 Linux、macOS、Windows 的現成程式，下載就能用；只有從原始碼執行才需要 Go 1.27 以上。
- 想從頁面啟動工作階段的話：在 Linux 和 macOS 上最好安裝 tmux。

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

程式和 mod 使用同一個版本號：`symphony -version` 會顯示程式的版本，`claude plugin list` 會列出 mod 的版本。要更新時，下載新版的壓縮檔，再執行 `claude plugin update symphony-bridge@symphony`。

也可以從原始碼執行（需要 Go 1.27 以上）：

```bash
git clone https://github.com/feynmanzcys-bytes/symphony.git
cd symphony
go run ./cmd/bridge
```

開啟 <http://127.0.0.1:7788/> 就能看到辦公室。<http://127.0.0.1:7788/approve> 則是只處理核准的簡易頁面。

| 參數 | 預設值 | 用途 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | 監聽位址。頁面目前還沒有登入功能，所以不要綁定到本機以外的位址 |
| `-data` | 使用者設定資料夾裡的 `symphony` | 存放帳號（含 token）和訊息紀錄的地方 |
| `-terminal` | `auto` | 從頁面啟動的工作階段要用的終端機：`auto`、`tmux` 或 `window` |
| `-claude` | `claude` | claude 執行檔 |
| `-lang` | 系統語言 | bridge 訊息使用的語言 |
| `-mod` | 空白 | 開發用：透過 `--plugin-dir` 讓工作階段載入這個資料夾裡的 mod，覆蓋已安裝的版本 |
| `-web` | 空白 | 開發用：直接從這個資料夾提供頁面，修改後重新整理就會生效 |
| `-version` | — | 顯示版本號後結束 |

### 3. 啟動工作階段

**從頁面啟動**：按空座位上的「+」，輸入名字，再選好資料夾和選項。bridge 會開一個終端機來執行 `claude`；用 tmux 的話，工作階段名稱是 `sym-<id>`，執行 `tmux attach -t sym-<id>` 就能連進去。

**從終端機啟動**：先建立一個帳號拿到 token，再帶著它啟動 `claude`：

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # 回應裡會有 token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

建議使用 `manual` 權限模式：在 `auto` 模式下，大部分動作都不會詢問任何人，能送到頁面的核准請求也就很少。如果 bridge 不在預設位址，還要設定 `SYMPHONY_BRIDGE=http://host:port`。`SYMPHONY_LANG` 決定 mod 訊息使用的語言。

## 帳號與座位

- 在 symphony 裡，一個身分就是一個名字、一個 token 加上一個固定座位。一個帳號同一時間只能給一個工作階段使用；第二個工作階段會被拒絕，它的核准請求會留在終端機裡處理。
- 這裡不管 Claude 的登入：所有工作階段可以共用同一個 Claude 登入。
- 資料存放在 `~/.config/symphony`（Linux）、`~/Library/Application Support/symphony`（macOS）或 `%AppData%\symphony`（Windows）。`accounts.json` 存有 token，只有檔案擁有者能讀取（0600）；在 Windows 上則靠使用者資料夾的權限來保護。

## 安全性

- bridge 只接受發給本機的請求，也會拒絕其他網站透過你的瀏覽器送來的寫入請求：因為頁面可以啟動工作階段，等於能在這台電腦上執行指令。
- 頁面目前還沒有登入功能，所以**不要**把 bridge 綁定到本機以外的位址。

## 平台

| 系統 | 從頁面啟動的工作階段使用的終端機 | 測試情況 |
| --- | --- | --- |
| Linux | 有裝 tmux 就用 tmux，否則用圖形介面的終端機 | 完整的端對端測試 |
| macOS | 預設用 Terminal.app；`-terminal tmux` 可改用 tmux | 只做過交叉編譯和單元測試 |
| Windows | 新開一個主控台視窗 | 只做過交叉編譯和單元測試 |

只有 tmux 能替你回答「信任此資料夾」。如果用的是終端機視窗，在新資料夾裡啟動第一個工作階段時，需要你在那個視窗裡按一下。

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
- 端對端測試需要 claude、tmux 和 chromium，所以只在 Linux 和裝了 tmux 的 macOS 上執行。預設使用這個儲存庫裡的 mod；設定 `SYMPHONY_E2E_MOD=installed` 就會改用已安裝的 mod。截圖會存到 `e2e/artifacts/`。
- 發布版本：在 main 的提交上建立版本標籤並推送（`git tag -a v0.3.0 -m v0.3.0 && git push origin v0.3.0`），GitHub Actions 就會為所有平台編譯程式並發布 Release（`.github/workflows/release.yml`）。標籤必須和 `plugin.json` 裡 mod 的版本號相同。`.github/release.sh v0.3.0 dist` 可以在本機打包出同樣的壓縮檔。

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
