# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | **日本語** | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

Claude Code のセッションをチームにまとめて、ピクセルアートのオフィスで働かせましょう。誰が作業中で誰があなたを待っているのかがひと目でわかり、社員ごとのウィンドウで会話でき、権限の承認や AskUserQuestion の質問にもそこからワンクリックで対応できます。

![オフィス](docs/office.png)

## できること

- **セッションはチャットとして見える**：社員をクリックすると、ページの中央にそのウィンドウが開きます。上に社員の情報、下にセッションのチャット：入力したプロンプト、モデルの思考（折りたたみ、進行中はオレンジの「考え中…」）、流れてくる回答、ツール呼び出し、他のセッションからのメッセージ（紫）。下の入力欄に書けばそのセッションへのプロンプトになり、「停止」ボタンで中断できます。
- **ブラウザで承認**：権限の承認（コマンドの実行やファイルの編集）と AskUserQuestion の質問は、その社員のウィンドウの入力欄の上にカードとしてせり上がり、上部バーの赤い件数からそこへ飛べます。bridge が動いていないときや、bridge がアカウントを認識できないときは、いつもどおりターミナルでの操作に戻るので、セッションが止まったままになることはありません。
- **ピクセルアートのオフィス**：セッション 1 つが、決まった席に座る社員 1 人です。緑は待機中、黄色は作業中、赤はあなたの承認か回答を待っている状態、灰色は退勤です。手の空いた社員はまず席でスマホやゲームをして、そのうちオフィスをぶらつき始めます。ラウンジのソファでテレビ、テーブルサッカー、鬼ごっこ、丸テーブルでおしゃべり、コーヒー、窓の外を眺めたり。しばらくすると昼寝に行き、たいてい会議室に隠れて寝ています。キャラクターの体格はモデルで、筋肉は思考の強さで決まります。画面は大きな会社のワンフロアで、ウィンドウいっぱいに広がります。中央に執務エリア、左に名札つきの 3〜5 人用の小会議室、右に会議室とラウンジがあり、社員はエレベーターで出退勤します。上部のバーではピクセルの大きさも 4 段階で選べます。オフィスはそのままで、細かくするほど細かいピクセルと細部で描かれます。
- **すぐ見つけて呼び戻す**：手の空いた社員をクリックすると自分の席に戻り、次の仕事までそこで待ちます。上部のバーにある検索ボックスで、名前やフォルダから社員を探せます（`/` キーでジャンプ）。その横の状態の札（待機中・作業中・対応待ち・退勤）をクリックすると、その状態の社員がまとめて目立ちます。バーの下のつまみで、バーを上にしまったり戻したりできます。
- **セッション間のメッセージ**：セッション同士がメッセージを送り合う（SendMessage）と、オフィスの中を封筒が飛んでいきます。やり取りはメッセージ履歴に残ります。
- **ページからセッションを起動、ウィンドウは開かない**：空いている席の「+」をクリックすると新しい社員を採用でき、フォルダ、モデル、思考の強さ、権限モードを選べます（新しい社員は、すべて許可・Opus・最大の思考がデフォルト）。退勤した社員の「出勤」をクリックすると、同じ席に再出勤します。セッションは bridge の中の専用ターミナルで動くので、ウィンドウは開かず、フォーカスも奪われません。社員のウィンドウの「ターミナルを開く」でそのターミナルを表示でき、閉じてもセッションは続きます。
- **10 言語に対応**：English、简体中文、繁體中文、日本語、한국어、Español、Français、Deutsch、Português (Brasil)、Русский。ページはブラウザの言語で表示され、上部のバーで言語を切り替えられます。bridge のメッセージはシステムの言語（または `-lang`）に、mod のメッセージはそのセッションを起動したページの言語に合わせます。
- **ローカルで完結**：bridge は Go のプログラム 1 つ（標準ライブラリのみ）、ページは素の JavaScript で、外部のサービスには一切依存しません。

![ブラウザで承認](docs/approve.png)

## 必要なもの

- Claude Code 2.1.316（このバージョンで動作確認済み）。mod は Claude Code の function hooks を使っていますが、これはまだアーリーアクセスの API なので、Claude Code をアップグレードしたらエンドツーエンドテストを実行してください。
- bridge は Linux、macOS、Windows 用のビルド済みプログラムをダウンロードして使えます。Go 1.27 以降が必要なのは、ソースから実行する場合だけです。
- ほかには何も要りません。ページから起動したセッションは、どのプラットフォームでも bridge の中で動きます。

## インストール

### 1. mod をインストールする

Claude Code で次のように入力します：

```
/plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

`y` を押してマーケットプレイスを追加し、スコープを選びます（デフォルトのユーザースコープで OK です）。シェルからインストールすることもできます：

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

リポジトリをクローンしてあるなら、そのクローンをマーケットプレイスとして使うこともできます。こうすると mod と bridge が同じコードになり、`git pull` のあとに起動したセッションはすべて新しいバージョンになります：

```bash
claude plugin install symphony-bridge --marketplace /path/to/symphony
```

ユーザースコープにインストールすると、mod はすべての Claude Code セッションで読み込まれますが、動作するのは `SYMPHONY_TOKEN` を付けて起動したセッションだけです。それ以外のセッションでは何もしません。

更新：`claude plugin update symphony-bridge@symphony`。アンインストール：`claude plugin uninstall symphony-bridge@symphony`。

### 2. bridge を起動する

[最新リリース](https://github.com/feynmanzcys-bytes/symphony/releases/latest)からお使いのシステム用のアーカイブをダウンロードして展開し、中の `symphony` を実行します。

| システム | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz)（Intel） | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz)（Apple シリコン） |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

たとえば Linux では：

```bash
curl -fLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz
tar xzf symphony_linux_amd64.tar.gz
./symphony
```

Windows では zip を展開して `symphony.exe` を実行します。ダブルクリックすると専用のウィンドウで動き、そのウィンドウを閉じると bridge も止まります。

プログラムには署名がないため、初回の実行がブロックされることがあります。ブラウザでダウンロードした場合、macOS は「開発元を検証できません」と表示することがあります。「システム設定 → プライバシーとセキュリティ」で「このまま開く」をクリックするか、`xattr -d com.apple.quarantine symphony` を一度実行してください。Windows でも SmartScreen が警告することがあります。「詳細情報」をクリックしてから「実行」を選んでください。リリースの `checksums.txt` には各アーカイブの SHA-256 が載っています。

プログラムと mod は同じバージョン番号を使います。プログラムのバージョンは `symphony -version`、mod のバージョンは `claude plugin list` で確認できます。更新するときは、新しいリリースをダウンロードしてから `claude plugin update symphony-bridge@symphony` を実行します。各リリースの変更点は [CHANGELOG.md](CHANGELOG.md) にあります。

ソースから実行することもできます（Go 1.27 以降）：

```bash
git clone https://github.com/feynmanzcys-bytes/symphony.git
cd symphony
go run ./cmd/bridge
```

起動するとブラウザでオフィス <http://127.0.0.1:7788/> が開きます（`-open=false` で無効にできます）。<http://127.0.0.1:7788/approve> は承認だけを行うシンプルなページです。bridge が動いているときにもう一度起動すると、オフィスを開くだけです。

| フラグ | デフォルト | 説明 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | 待ち受けるアドレス。ページにはまだログイン機能がないので、このマシン以外のアドレスにはバインドしないでください |
| `-data` | `$SYMPHONY_DATA`、なければ設定フォルダ内の `symphony` | アカウント（token を含む）とメッセージ履歴の保存先 |
| `-claude` | `claude` | claude の実行ファイル |
| `-lang` | システムの言語 | bridge のメッセージの言語 |
| `-mod` | なし | 開発用：このフォルダの mod を `--plugin-dir` でセッションに渡し、インストール済みのものより優先させる |
| `-web` | なし | 開発用：このフォルダのページを配信し、編集を再読み込みで反映させる |
| `-open` | `true` | bridge の起動後にブラウザでオフィスを開く。`-addr` で既に bridge が動いていれば、そのオフィスを開くだけ |
| `-autostart` | — | `on`：ログインのたびに bridge を自動で起動し（ほかに指定したフラグ付き）、今すぐ起動する。`off`：元に戻す |
| `-background` | — | bridge をバックグラウンドで起動する。ターミナルから切り離し、ログはデータフォルダの `bridge.log` に書き、応答するようになったら戻る |
| `-version` | — | バージョンを表示して終了する |

**ログイン時に起動**：`symphony -autostart on` を実行すると、以後ログインのたびに bridge が自動で起動し（一緒に指定したフラグ、たとえば `-addr` も引き継ぎます）、今すぐにも起動します。`symphony -autostart off` で元に戻せます。Linux では systemd のユーザーサービス（ログは `journalctl --user -u symphony` で見られます）、macOS では launchd エージェント（ログは `~/Library/Logs/symphony.log`）、Windows ではレジストリの Run キーの値です（bridge はコンソールウィンドウで動き、閉じると次のログインまで止まります）。`go run` ではなく、symphony プログラムそのものが必要です。

### 3. セッションを起動する

**ページから**：空いている席の「+」をクリックし、名前を入力して、フォルダとオプションを選びます。bridge は自分の中の専用ターミナルで `claude` を実行し、ウィンドウは開きません。社員のウィンドウにセッションがチャットとして表示され、「ターミナルを開く」ボタンでそのセッションに接続したターミナルウィンドウが開きます（どのターミナルからでも `symphony attach <id>` で同じことができます。`Ctrl-]` かウィンドウを閉じると切り離され、セッションは続きます）。セッションは bridge の子プロセスなので、bridge を止めるとセッションも終了します。

**ターミナルから**：アカウントを作って token を受け取り、その token を付けて `claude` を起動します：

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # レスポンスに token が含まれます
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

権限モードは `manual` がおすすめです。`auto` モードではほとんどの操作で誰にも確認しないので、ページに届く承認がほとんどなくなります。bridge がデフォルトのアドレスにない場合は、`SYMPHONY_BRIDGE=http://host:port` も設定してください。mod のメッセージの言語は `SYMPHONY_LANG` で選べます。

このマシン上の bridge のアドレスが応答しないときは、セッションが `symphony -background` で bridge を自分で起動します（`symphony` が PATH にある場合）。bridge はセッションが終わっても動き続けます。アカウントが既定のデータフォルダにない場合は `SYMPHONY_DATA` を設定してください。

## アカウントと席

- symphony では、名前・token・固定の席の 3 つがそろって 1 つのアカウントになります。1 つのアカウントを同時に使えるセッションは 1 つだけです。2 つ目のセッションは拒否され、その承認はターミナルで行うことになります。
- Claude へのログインは symphony では管理しません。すべてのセッションで 1 つの Claude ログインを共有してかまいません。
- データは `~/.config/symphony`（Linux）、`~/Library/Application Support/symphony`（macOS）、`%AppData%\symphony`（Windows）に保存されます。`accounts.json` には token が入っているので、所有者だけが読めるようになっています（0600）。Windows ではユーザーフォルダのアクセス権で保護されます。

## セキュリティ

- bridge はこのマシン宛てのリクエストだけを受け付け、ほかの Web サイトがあなたのブラウザ経由で送ってくる書き込みリクエストは拒否します。ページからセッションを起動できる、つまりこのマシンでコマンドを実行できるからです。
- ページにはまだログイン機能がないので、bridge をこのマシン以外のアドレスに**バインドしないでください**。

## 対応プラットフォーム

| OS | ページから起動したセッション | 「ターミナルを開く」 | テスト状況 |
| --- | --- | --- | --- |
| Linux | bridge の中の疑似ターミナル | グラフィカルなターミナル（x-terminal-emulator、gnome-terminal、konsole、xterm）で `symphony attach` を実行 | エンドツーエンドテストまで実施 |
| macOS | bridge の中の疑似ターミナル | Terminal.app で `symphony attach` を実行 | エンドツーエンドテストまで実施 |
| Windows | bridge の中の疑似コンソール（ConPTY） | 新しいコンソールウィンドウで `symphony attach` を実行 | クロスコンパイルと単体テストのみ |

「このフォルダを信頼する」には、どのプラットフォームでも bridge があなたの代わりに答えます。

## 開発

```bash
go test -race ./...                         # bridge と翻訳の単体テスト
claude plugin test mods/symphony-bridge     # mod の単体テスト
claude plugin validate .                    # マーケットプレイスと mod をチェック
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # エンドツーエンド：本物のセッションを起動し、token を消費します
```

- mod を開発するとき：`go run ./cmd/bridge -mod mods/symphony-bridge` で起動すると、ページから起動したセッションには手元の mod が `--plugin-dir` で渡されます。手動で起動するセッションには、自分で `--plugin-dir mods/symphony-bridge` を付けてください。
- ページを開発するとき：`go run ./cmd/bridge -web web` で起動すると、再読み込みするだけで編集が反映されます。
- 本物のセッションなしでオフィスを見るとき：2 つ目の bridge を起動して、架空のアクティビティを流し込みます。`go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo` を実行してから、`go run ./cmd/demo -base http://127.0.0.1:7799` を実行してください。
- 翻訳：1 言語につき 1 ファイルで、ページ用は `web/locales/<lang>.json`、bridge 用は `internal/i18n/locales/<lang>.json` です。キーは `en.json` と同じです。mod のわずかなメッセージは `mods/symphony-bridge/hooks/messages.ts` にあります。単体テストでは、すべての言語にすべてのキーがそろっていて、`{プレースホルダー}` も一致していることを確認します。エンドツーエンドテストの `TestI18n` は、すべての言語でページを開いて確かめます。
- エンドツーエンドテストには claude、tmux（テストが自分で動かすセッション用）、Chromium か Chrome が必要なので、Linux と macOS で実行できます。デフォルトではこのリポジトリの mod を使います。インストール済みの mod を使うには `SYMPHONY_E2E_MOD=installed` を設定してください。スクリーンショットは `e2e/artifacts/` に保存されます。
- リリース：main 上のコミットにバージョンタグを付けて push すると（`git tag -a v0.3.0 -m v0.3.0 && git push origin v0.3.0`）、GitHub Actions が全プラットフォーム向けにプログラムをビルドしてリリースを公開します（`.github/workflows/release.yml`）。タグは `plugin.json` にある mod のバージョンと一致している必要があります。`.github/release.sh v0.3.0 dist` で同じアーカイブを手元で作れます。各リリースには `CHANGELOG.md` の節が必要で、それがリリースノートの冒頭になります。

| フォルダ | 内容 |
| --- | --- |
| `mods/symphony-bridge/` | Claude Code の mod（TypeScript） |
| `.claude-plugin/marketplace.json` | `/plugin install` が読み込むプラグインマーケットプレイス |
| `cmd/bridge/`、`internal/bridge/` | bridge |
| `internal/i18n/` | 言語の判定と bridge の翻訳 |
| `web/` | オフィスと承認ページ、およびその翻訳（bridge に組み込まれます） |
| `cmd/demo/` | オフィス用の架空のアクティビティ |
| `e2e/` | エンドツーエンドテスト |

## ライセンス

[MIT](LICENSE)
