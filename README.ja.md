# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | **日本語** | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

Claude Code のセッションをチームにまとめて、ピクセルアートのオフィスで働かせましょう。誰が作業中で誰があなたを待っているのかがひと目でわかり、権限の承認や AskUserQuestion の質問には Web のポップアップからワンクリックで対応できます。

![オフィス](docs/office.png)

## できること

- **ブラウザで承認**：権限の承認（コマンドの実行やファイルの編集）と AskUserQuestion の質問が Web ページに届きます。bridge が動いていないときや、bridge がアカウントを認識できないときは、いつもどおりターミナルでの操作に戻るので、セッションが止まったままになることはありません。
- **ピクセルアートのオフィス**：セッション 1 つが、決まった席に座る社員 1 人です。緑は待機中、黄色は作業中、赤はあなたの承認か回答を待っている状態、灰色は退勤です。手の空いた社員はまず席でスマホやゲームをして、そのうちオフィスをぶらつき始めます。ラウンジのソファでテレビ、テーブルサッカー、鬼ごっこ、丸テーブルでおしゃべり、コーヒー、窓の外を眺めたり。しばらくすると昼寝に行き、たいてい会議室に隠れて寝ています。キャラクターの体格はモデルで、筋肉は思考の強さで決まります。
- **すぐ見つけて呼び戻す**：手の空いた社員をクリックすると自分の席に戻り、次の仕事までそこで待ちます。タイトル横の検索ボックスで、名前やフォルダから社員を探せます（`/` キーでジャンプ）。タイトル下の状態の札（待機中・作業中・対応待ち・退勤）をクリックすると、その状態の社員がまとめて目立ちます。
- **セッション間のメッセージ**：セッション同士がメッセージを送り合う（SendMessage）と、オフィスの中を封筒が飛んでいきます。やり取りはメッセージ履歴に残ります。
- **ページからセッションを起動**：空いている席の「+」をクリックすると新しい社員を採用でき、フォルダ、モデル、思考の強さ、権限モードを選べます。退勤した社員の「出勤」をクリックすると、同じ席に再出勤します。
- **10 言語に対応**：English、简体中文、繁體中文、日本語、한국어、Español、Français、Deutsch、Português (Brasil)、Русский。ページはブラウザの言語で表示され、左上で言語を切り替えられます。bridge のメッセージはシステムの言語（または `-lang`）に、mod のメッセージはそのセッションを起動したページの言語に合わせます。
- **ローカルで完結**：bridge は Go のプログラム 1 つ（標準ライブラリのみ）、ページは素の JavaScript で、外部のサービスには一切依存しません。

![ブラウザで承認](docs/approve.png)

## 必要なもの

- Claude Code 2.1.314（このバージョンで動作確認済み）。mod は Claude Code の function hooks を使っていますが、これはまだアーリーアクセスの API なので、Claude Code をアップグレードしたらエンドツーエンドテストを実行してください。
- Go 1.27 以降（bridge の実行に使います）。
- ページからセッションを起動する場合、Linux と macOS では tmux がおすすめです。

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

```bash
git clone https://github.com/feynmanzcys-bytes/symphony.git
cd symphony
go run ./cmd/bridge
```

<http://127.0.0.1:7788/> を開くとオフィスが表示されます。<http://127.0.0.1:7788/approve> は承認だけを行うシンプルなページです。

| フラグ | デフォルト | 説明 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | 待ち受けるアドレス。ページにはまだログイン機能がないので、このマシン以外のアドレスにはバインドしないでください |
| `-data` | 設定フォルダ内の `symphony` | アカウント（token を含む）とメッセージ履歴の保存先 |
| `-terminal` | `auto` | ページから起動したセッションのターミナル：`auto`、`tmux`、`window` のいずれか |
| `-claude` | `claude` | claude の実行ファイル |
| `-lang` | システムの言語 | bridge のメッセージの言語 |
| `-mod` | なし | 開発用：このフォルダの mod を `--plugin-dir` でセッションに渡し、インストール済みのものより優先させる |
| `-web` | なし | 開発用：このフォルダのページを配信し、編集を再読み込みで反映させる |

### 3. セッションを起動する

**ページから**：空いている席の「+」をクリックし、名前を入力して、フォルダとオプションを選びます。bridge がターミナルを開いて `claude` を実行します。tmux の場合、セッション名は `sym-<id>` になり、`tmux attach -t sym-<id>` でそのセッションに入れます。

**ターミナルから**：アカウントを作って token を受け取り、その token を付けて `claude` を起動します：

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # レスポンスに token が含まれます
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

権限モードは `manual` がおすすめです。`auto` モードではほとんどの操作で誰にも確認しないので、ページに届く承認がほとんどなくなります。bridge がデフォルトのアドレスにない場合は、`SYMPHONY_BRIDGE=http://host:port` も設定してください。mod のメッセージの言語は `SYMPHONY_LANG` で選べます。

## アカウントと席

- symphony では、名前・token・固定の席の 3 つがそろって 1 つのアカウントになります。1 つのアカウントを同時に使えるセッションは 1 つだけです。2 つ目のセッションは拒否され、その承認はターミナルで行うことになります。
- Claude へのログインは symphony では管理しません。すべてのセッションで 1 つの Claude ログインを共有してかまいません。
- データは `~/.config/symphony`（Linux）、`~/Library/Application Support/symphony`（macOS）、`%AppData%\symphony`（Windows）に保存されます。`accounts.json` には token が入っているので、所有者だけが読めるようになっています（0600）。Windows ではユーザーフォルダのアクセス権で保護されます。

## セキュリティ

- bridge はこのマシン宛てのリクエストだけを受け付け、ほかの Web サイトがあなたのブラウザ経由で送ってくる書き込みリクエストは拒否します。ページからセッションを起動できる、つまりこのマシンでコマンドを実行できるからです。
- ページにはまだログイン機能がないので、bridge をこのマシン以外のアドレスに**バインドしないでください**。

## 対応プラットフォーム

| OS | ページから起動したセッションのターミナル | テスト状況 |
| --- | --- | --- |
| Linux | tmux があれば tmux、なければグラフィカルなターミナル | エンドツーエンドテストまで実施 |
| macOS | デフォルトは Terminal.app。`-terminal tmux` で tmux を使用 | クロスコンパイルと単体テストのみ |
| Windows | 新しいコンソールウィンドウ | クロスコンパイルと単体テストのみ |

「このフォルダを信頼する」にあなたの代わりに答えられるのは、tmux だけです。ターミナルウィンドウの場合、新しいフォルダで最初にセッションを起動するときは、そのウィンドウで 1 回クリックする必要があります。

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
- エンドツーエンドテストには claude、tmux、chromium が必要なので、Linux と、tmux を入れた macOS で実行できます。デフォルトではこのリポジトリの mod を使います。インストール済みの mod を使うには `SYMPHONY_E2E_MOD=installed` を設定してください。スクリーンショットは `e2e/artifacts/` に保存されます。

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
