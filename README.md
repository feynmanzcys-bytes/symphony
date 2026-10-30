# symphony

**English** | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

Put a team of Claude Code sessions in a pixel-art office: see at a glance who is working and who is waiting for you, and handle permission requests and AskUserQuestion questions with one click in a web popup.

![The office](docs/office.png)

## What it does

- **Approvals in the browser**: permission requests (running commands, editing files) and AskUserQuestion questions go to the web page. When the bridge isn't running or doesn't know the account, the session falls back to the terminal as usual and never hangs.
- **A pixel-art office**: each session is an employee at a fixed desk. Green is idle, yellow is working, red is waiting for your approval or answer, gray is off duty. Idle employees play with their phones or games and fall asleep after a while; the character's size follows the model and its muscles follow the thinking effort.
- **Messages between sessions**: when sessions message each other (SendMessage), an envelope flies across the office, and a message log keeps them.
- **Start sessions from the page**: click the "+" on an empty desk to hire a new employee and pick the folder, model, thinking effort and permission mode; click "Start work" on an off-duty employee to bring them back to the same desk.
- **10 languages**: English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil) and Русский. The page follows your browser's language and has a language picker in its top-left corner; the bridge's messages follow your system language (or `-lang`), and the mod's messages follow the page that started the session.
- **Local only**: the bridge is a single Go program (standard library only), the pages are plain JavaScript, and nothing depends on a third-party service.

![Approvals in the browser](docs/approve.png)

## Requirements

- Claude Code 2.1.314 (tested with this version). The mod uses Claude Code's function hooks, which are still an early-access API, so run the end-to-end tests after upgrading Claude Code.
- Go 1.27 or later, to run the bridge.
- To start sessions from the page: tmux is best on Linux and macOS.

## Install

### 1. Install the mod

In Claude Code, type:

```
/plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

Press `y` to add the marketplace, then pick a scope (the default user scope). Or install it from a shell:

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

If you have cloned the repository, you can use your clone as the marketplace instead. Then the mod and the bridge are the same code, and after a `git pull` every new session gets the new version:

```bash
claude plugin install symphony-bridge --marketplace /path/to/symphony
```

Installed at user scope, the mod loads in every Claude Code session, but it only acts in sessions started with `SYMPHONY_TOKEN`; everywhere else it does nothing.

Update: `claude plugin update symphony-bridge@symphony`. Uninstall: `claude plugin uninstall symphony-bridge@symphony`.

### 2. Start the bridge

```bash
git clone https://github.com/feynmanzcys-bytes/symphony.git
cd symphony
go run ./cmd/bridge
```

Open <http://127.0.0.1:7788/> for the office. <http://127.0.0.1:7788/approve> is a simpler page for approvals only.

| Flag | Default | What it does |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | Address to listen on. The page has no login yet, so don't bind it to anything but this machine |
| `-data` | `symphony` in your config folder | Where accounts (with their tokens) and the message log are kept |
| `-terminal` | `auto` | Terminal for sessions started from the page: `auto`, `tmux` or `window` |
| `-claude` | `claude` | The claude executable |
| `-lang` | your system language | Language of the bridge's messages |
| `-mod` | empty | For development: give sessions this folder's mod with `--plugin-dir`, overriding the installed copy |
| `-web` | empty | For development: serve the pages from this folder, so edits show after a reload |

### 3. Start sessions

**From the page**: click the "+" on an empty desk, enter a name, and pick the folder and options. The bridge opens a terminal that runs `claude`; with tmux the session is called `sym-<id>`, and `tmux attach -t sym-<id>` takes you there.

**From a terminal**: create an account to get a token, then start `claude` with it:

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # the answer holds the token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

The `manual` permission mode is recommended: in `auto` mode most actions don't ask anyone, so few approvals reach the page. If the bridge isn't at the default address, also set `SYMPHONY_BRIDGE=http://host:port`. `SYMPHONY_LANG` picks the language of the mod's messages.

## Accounts and seats

- An identity in symphony is a name, a token and a fixed seat. An account serves one session at a time; a second session is refused and its approvals stay in the terminal.
- Claude logins are not managed here: all sessions may share one Claude login.
- Data lives in `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS) or `%AppData%\symphony` (Windows). `accounts.json` holds the tokens and is readable by its owner only (0600); on Windows the user folder's permissions protect it.

## Security

- The bridge only accepts requests addressed to this machine, and refuses writes that another website sends through your browser: the page can start sessions, which means running commands on this machine.
- The page has no login yet, so **don't** bind the bridge to an address other than this machine.

## Platforms

| System | Terminal for sessions started from the page | Tested |
| --- | --- | --- |
| Linux | tmux when installed, otherwise a graphical terminal | Full end-to-end tests |
| macOS | Terminal.app by default; `-terminal tmux` for tmux | Cross-compiled and unit-tested only |
| Windows | A new console window | Cross-compiled and unit-tested only |

Only tmux can answer "trust this folder" for you. With a terminal window, the first session in a new folder needs one click in that window.

## Development

```bash
go test -race ./...                         # bridge and translation unit tests
claude plugin test mods/symphony-bridge     # mod unit tests
claude plugin validate .                    # check the marketplace and the mod
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # end-to-end: starts real sessions and spends tokens
```

- Working on the mod: `go run ./cmd/bridge -mod mods/symphony-bridge`, and sessions started from the page get your copy of the mod with `--plugin-dir`; add `--plugin-dir mods/symphony-bridge` yourself to sessions you start by hand.
- Working on the pages: `go run ./cmd/bridge -web web`, and a reload shows your edits.
- Seeing the office without real sessions: start a second bridge and fill it with made-up activity: `go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo`, then `go run ./cmd/demo -base http://127.0.0.1:7799`.
- Translations: each language is one file, `web/locales/<lang>.json` for the pages and `internal/i18n/locales/<lang>.json` for the bridge, with the same keys as `en.json`; the mod's few messages are in `mods/symphony-bridge/hooks/messages.ts`. The unit tests check that every language has every key and the same `{placeholders}`, and the `TestI18n` end-to-end test opens the pages in every language.
- The end-to-end tests need claude, tmux and chromium, so they run on Linux and on macOS with tmux. They use this repository's mod by default; set `SYMPHONY_E2E_MOD=installed` to use the installed mod. Screenshots go to `e2e/artifacts/`.

| Folder | What it is |
| --- | --- |
| `mods/symphony-bridge/` | The Claude Code mod (TypeScript) |
| `.claude-plugin/marketplace.json` | The plugin marketplace that `/plugin install` reads |
| `cmd/bridge/`, `internal/bridge/` | The bridge |
| `internal/i18n/` | Language detection and the bridge's translations |
| `web/` | The office and approval pages and their translations, built into the bridge |
| `cmd/demo/` | Made-up activity for the office |
| `e2e/` | End-to-end tests |

## License

[MIT](LICENSE)
