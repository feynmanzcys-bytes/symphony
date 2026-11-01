# symphony

**English** | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

Put a team of Claude Code sessions in a pixel-art office: see at a glance who is working and who is waiting for you, talk to each one in its own window, and handle permission requests and AskUserQuestion questions there with one click.

![The office](docs/office.png)

## Install

The quickest way: open Claude Code and paste

> Install https://github.com/feynmanzcys-bytes/symphony for me

Claude Code reads this README and does everything in the next section. To install by hand, follow the same steps.

### For AI agents: the install runbook

You are reading this because a user asked you to install symphony. Do the steps below in order, in a shell, and finish with the report in step 6. Don't ask the user anything unless a step says so; make the routine choices yourself and list them in the report.

symphony has two parts and needs both:

1. **The `symphony` program** (the bridge): a single binary, a local web server at `http://127.0.0.1:7788/` that serves the office page and runs the sessions. It must be on the user's `PATH`, under the name `symphony`, because the mod starts it with `symphony -background` when a session finds no bridge running.
2. **The `symphony-bridge` plugin** (the mod): a Claude Code plugin from this repository's marketplace, installed at user scope. It only acts in sessions started with `SYMPHONY_TOKEN` and does nothing in any other session.

#### Step 1: check the machine

```bash
uname -sm                 # Linux/macOS: picks the archive below (x86_64 → amd64; arm64/aarch64 → arm64)
claude --version          # needs Claude Code 2.1.x (tested with 2.1.316)
git --version             # only for collaborative tasks; note it in the report if missing, don't stop
symphony -version         # already installed? prints "symphony v0.7.1 (darwin/arm64)"
curl -s http://127.0.0.1:7788/api/state | head -c 40   # a bridge already runs? prints {"type":"snapshot",...
claude plugin list --json # the mod already installed? look for "id":"symphony-bridge@symphony"
```

On Windows use PowerShell: `$env:PROCESSOR_ARCHITECTURE` is `AMD64` or `ARM64`, and `Invoke-WebRequest http://127.0.0.1:7788/api/state` instead of curl.

If `claude` is missing, stop and tell the user to install Claude Code first. If the program and the mod are both in place, go to "Update" below instead.

#### Step 2: get the program

Take the first option that works:

- **A. With `gh`** (works while this repository is private, as long as `gh auth status` is logged in):
  ```bash
  gh release download -R feynmanzcys-bytes/symphony --pattern 'symphony_<os>_<arch>.tar.gz' -D /tmp/symphony-dl
  ```
- **B. With `curl`** (only once the repository is public; a 404 means use A or C):
  ```bash
  curl -fL -o /tmp/symphony-dl/symphony_<os>_<arch>.tar.gz https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_<os>_<arch>.tar.gz
  ```
- **C. From source** (Go 1.27 or later; the clone is reused in step 4):
  ```bash
  git clone https://github.com/feynmanzcys-bytes/symphony.git /tmp/symphony-src
  cd /tmp/symphony-src && go build -o symphony ./cmd/bridge
  ```
  Build the binary; `go run` won't do, because `-autostart` refuses a `go run` build.

| System | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz) (Intel) | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz) (Apple silicon) |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

Each archive holds the program (`symphony`, or `symphony.exe` on Windows), `LICENSE` and `README.md`. The release's `checksums.txt` holds the SHA-256 of every archive (`sha256sum -c --ignore-missing checksums.txt`, or `shasum -a 256 -c` on macOS).

#### Step 3: put it on PATH

Pick a folder that is already on the user's `PATH` and writable without `sudo`: `~/.local/bin`, `/usr/local/bin` or `/opt/homebrew/bin`, in that order. If none exists, create `~/.local/bin`, add it to the shell's rc file (`~/.zshrc` or `~/.bashrc`) and say so in the report.

```bash
tar xzf /tmp/symphony-dl/symphony_<os>_<arch>.tar.gz -C /tmp/symphony-dl
install -m 755 /tmp/symphony-dl/symphony ~/.local/bin/symphony
xattr -d com.apple.quarantine ~/.local/bin/symphony 2>/dev/null || true   # macOS: the binary isn't signed
symphony -version          # prints "symphony v0.7.1 (linux/amd64)": the version and the platform
```

On Windows: `Expand-Archive symphony_windows_<arch>.zip -DestinationPath "$env:LOCALAPPDATA\Programs\symphony"`, then add that folder to the user's `Path` (`[Environment]::SetEnvironmentVariable('Path', "$([Environment]::GetEnvironmentVariable('Path','User'));$env:LOCALAPPDATA\Programs\symphony", 'User')`) and open a new shell. SmartScreen may warn on the first run: "More info", then "Run anyway".

If your own shell doesn't find `symphony` after this (its `PATH` was read before you changed it), use the absolute path for the rest of the steps.

#### Step 4: install the mod

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

This adds the repository as the plugin marketplace `symphony` and installs the mod at user scope, without prompts. It clones the repository with git, so while the repository is private it needs git access to GitHub; if it fails for that reason, use a clone as the marketplace instead (the one from step 2 C, or make one): `claude plugin install symphony-bridge --marketplace /absolute/path/to/symphony`. With a clone as the marketplace, sessions read the mod from that folder, and a `git pull` there updates it.

Verify:

```bash
claude plugin list --json   # one entry has "id":"symphony-bridge@symphony", "scope":"user", "enabled":true
```

- `"scope":"project"` or `"local"`: the mod would only load in one folder. Run `claude plugin uninstall symphony-bridge@symphony`, then install again with `--scope user`.
- `"enabled":false`: `claude plugin enable symphony-bridge@symphony`.

#### Step 5: start the bridge

```bash
symphony -autostart on -open=false
```

This starts the bridge now and at every login (a systemd user service on Linux, a launchd agent on macOS, the registry's Run key on Windows), and prints

```
symphony now starts when you log in (<where>); it's running at http://127.0.0.1:7788/
```

If the user said they don't want it started at login, run `symphony -background -open=false` instead: it starts the bridge detached from the terminal and prints `the symphony bridge runs in the background at http://127.0.0.1:7788/; its log: <data folder>/bridge.log`. Either command answers `a symphony bridge is already running at http://127.0.0.1:7788/` when one is; that is fine, don't start another.

Verify the whole chain with one request:

```bash
curl -s http://127.0.0.1:7788/api/launch/options
```

The answer holds `"enabled":true` and `"mod":{"ok":true,"id":"symphony-bridge@symphony","version":"0.7.1"}`: the bridge runs, it found `claude`, and the mod is installed where sessions will load it. With `"enabled":false`, `mod.code` says what is wrong and `mod.fix` the command that fixes it: `mod_not_installed` or `mod_project_scope` (back to step 4), `mod_disabled` (enable it), `mod_check_failed` (`claude plugin list --json` failed, so Claude Code is probably too old). `"mod":null` means the bridge didn't find `claude` on its `PATH` at all (its log says so): put it there, or start the bridge with `-claude /path/to/claude`.

The bridge's log is `bridge.log` in the data folder (`~/.config/symphony` on Linux, `~/Library/Application Support/symphony` on macOS, `%AppData%\symphony` on Windows); started at login, it logs to `journalctl --user -u symphony` on Linux and `~/Library/Logs/symphony.log` on macOS.

#### Step 6: report to the user

Open the office in the browser (`open http://127.0.0.1:7788/` on macOS, `xdg-open` on Linux, `start` on Windows) and tell the user:

- The office is at `http://127.0.0.1:7788/`. Click the "+" on an empty desk to hire the first employee: pick a folder, and a Claude Code session starts there, inside the bridge; its window on the page is its chat.
- Only sessions started from the page (or by hand with `SYMPHONY_TOKEN`, see "Start sessions" below) appear in the office. Their other Claude Code sessions, including the one doing this install, are not shown.
- What was installed where: the path of the program, the mod at user scope, the bridge started at login or in the background, any `PATH` change, and that `git` is missing if it is.
- How to undo it all: "Uninstall" below.

Rules: don't bind the bridge to anything but this machine (`-addr` stays `127.0.0.1:7788`; the page has no login), don't run it with `sudo`, and don't start a second bridge at the same address.

#### Update

Download the new release as in step 2 and replace the program in step 3; the mod follows with `claude plugin update symphony-bridge@symphony` (with a clone as the marketplace, `git pull` there instead). The running bridge keeps the old program until it restarts: `symphony -autostart off` then `symphony -autostart on -open=false` (or stop and start it the way it was started). Stopping the bridge ends the sessions it started, so tell the user before. The program and the mod share one version number; [CHANGELOG.md](CHANGELOG.md) lists what each release changed.

#### Uninstall

```bash
symphony -autostart off                           # stops the bridge and its sessions, and no longer starts it at login
claude plugin uninstall symphony-bridge@symphony  # the mod
claude plugin marketplace remove symphony         # the marketplace entry
rm ~/.local/bin/symphony                          # the program, wherever step 3 put it
```

The data folder (the accounts with their tokens, the message log, the tasks) stays; delete it to remove everything.

## What it does

- **Every session as a chat**: click an employee and its window opens in the middle of the page: who it is at the top, and below it the session as a chat: the prompts, the model's thinking (folded, "Thinking…" in orange while it goes on), its answers as they stream, the tool calls, and messages from other sessions in purple. Write at the bottom and it reaches the session as your prompt; a Stop button interrupts it.
- **Approvals in the browser**: permission requests (running commands, editing files) and AskUserQuestion questions rise over the input of that employee's window as cards, and the red count in the bar at the top takes you to them. When the bridge isn't running or doesn't know the account, the session falls back to the terminal as usual and never hangs.
- **A pixel-art office**: each session is an employee at a fixed desk. Green is idle, yellow is working, red is waiting for your approval or answer, gray is off duty. Idle employees first linger at their desks with their phones or games, then wander off: TV on the lounge sofa, foosball, tag, a chat at the round table, coffee, a look out of the window; after a while they nap, often hiding in the meeting room. The character's size follows the model and its muscles follow the thinking effort. The office is one floor of a big company's building and fills the window: the open office in the middle, small meeting rooms for 3 to 5 people with nameplates on its left, the meeting room and the lounge on its right, and lifts where people come and go. The bar at the top also sets the pixel size, in four steps: the same office, drawn in finer pixels and with more detail at each step.
- **Find anyone, call them back**: click an idle employee and it heads back to its desk (and stays there until its next turn); the search box in the bar at the top finds employees by name, session name or folder (press `/` to jump to it), and clicking a count beside it (idle, working, pending, off duty) lights up everyone in that state. The tab under the bar slides it up out of the way and back.
- **Messages between sessions**: when sessions message each other (SendMessage), an envelope flies across the office, and a message log keeps them.
- **Start sessions from the page, nothing pops up**: click the "+" on an empty desk to hire a new employee and pick the folder, model, thinking effort and permission mode (a new employee starts with everything allowed, Opus and the most thinking); click "Start work" on an off-duty employee to bring them back to the same desk. The session runs on a terminal of its own inside the bridge, so no window opens and nothing takes your focus; "Open terminal" in the employee's window shows that terminal when you want it, and closing the window changes nothing.
- **Teams in the meeting rooms**: click a free small meeting room, write a task and pick 2 to 5 members: employees, who each get a second session just for the task, or one-off advisors. The first member leads by default and hands out the parts with SendMessage; every member works in its own git worktree on its own branch, so nobody steps on anyone's files. The members sit in the meeting room until the task ends.
- **10 languages**: English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil) and Русский. The page follows your browser's language and has a language picker in the bar at the top; the bridge's messages follow `-lang`, else your locale (`LANG` and the like), else English; the mod's messages follow `SYMPHONY_LANG` (the page's language, for a session started from the page), else your locale, else English.
- **Local only**: the bridge is a single Go program (standard library only; the end-to-end tests also use chromedp), the pages are plain JavaScript, and nothing depends on a third-party service.

![Approvals in the browser](docs/approve.png)

## Requirements

- Claude Code 2.1.316 (tested with this version). The mod uses Claude Code's function hooks, which are still an early-access API, so run the end-to-end tests after upgrading Claude Code.
- The bridge comes as a ready-made program for Linux, macOS and Windows; Go 1.27 or later is only needed to build it from source.
- `git`, for collaborative tasks only: every member of a task works in a git worktree.
- Nothing else: sessions started from the page run inside the bridge, on every platform.

## Running the bridge

`symphony` starts the bridge and opens the office in your browser at <http://127.0.0.1:7788/> (`-open=false` turns that off); <http://127.0.0.1:7788/approve> is a simpler page for approvals only. Starting the bridge again while it runs just opens the office. On Windows, double-clicked, `symphony.exe` runs in a window of its own, and closing that window stops the bridge.

| Flag | Default | What it does |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | Address to listen on. The page has no login yet, so don't bind it to anything but this machine |
| `-data` | `$SYMPHONY_DATA`, else `symphony` in your config folder | The data folder: the accounts with their tokens (`accounts.json`), the message log (`messages.json`), the tasks (`tasks.json`) and `-background`'s log (`bridge.log`) |
| `-claude` | `claude` | The claude executable |
| `-lang` | your locale, else `en` | Language of the bridge's messages (`SYMPHONY_LANG`, `LC_ALL`, `LC_MESSAGES` or `LANG` set the default) |
| `-mod` | empty | For development: give sessions this folder's mod with `--plugin-dir`, overriding the installed copy |
| `-web` | empty | For development: serve the pages from this folder, so edits show after a reload |
| `-open` | `true` | Open the office in the browser once the bridge is up; when a bridge already runs at `-addr`, just open its office |
| `-autostart` | — | `on`: start the bridge whenever you log in, with the other flags given, and start it now; `off`: undo that |
| `-background` | — | Start the bridge in the background, detached from the terminal and logging to `bridge.log` in the data folder, and return once it answers |
| `-version` | — | Print the version and exit |

**Start it when you log in**: `symphony -autostart on` makes the bridge start whenever you log in, with the flags you give alongside (say `-addr`), and starts it right away; `symphony -autostart off` undoes that. On Linux it's a systemd user service (`journalctl --user -u symphony` shows its log), on macOS a launchd agent (logging to `~/Library/Logs/symphony.log`), on Windows a value under the registry's Run key (the bridge runs in a console window; closing it stops the bridge until the next login). It needs the symphony program itself, not `go run`.

**From source**, with Go 1.27 or later: `git clone https://github.com/feynmanzcys-bytes/symphony.git && cd symphony && go run ./cmd/bridge`.

## Start sessions

**From the page**: click the "+" on an empty desk, enter a name, and pick the folder and options. The bridge runs `claude` on a terminal of its own, inside the bridge: no window opens. The employee's window shows the session as a chat, and its "Open terminal" button opens a terminal window attached to the session (`symphony attach <id>` does the same from any terminal; `Ctrl-]` or closing the window detaches, and the session goes on). The sessions are the bridge's children: stopping the bridge ends them.

**From a terminal**: create an account to get a token, then start `claude` with it:

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # the answer holds the token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

The `manual` permission mode is recommended: in `auto` mode most actions don't ask anyone, so few approvals reach the page. If the bridge isn't at the default address, also set `SYMPHONY_BRIDGE=http://host:port`. `SYMPHONY_LANG` picks the language of the mod's messages (else your locale, else English).

When nothing answers at the bridge's address on this machine, the session starts the bridge itself with `symphony -background -open=false -addr <that address>` (if `symphony` is on your PATH), and the bridge keeps running after the session ends. Set `SYMPHONY_DATA` if your accounts aren't in the default data folder.

## Collaborative tasks

A small meeting room is where a team of 2 to 5 sessions works together on one task. Tasks need `git` and the mod at 0.7.0 (`claude plugin update symphony-bridge@symphony`).

**Start a task**: click a free small meeting room and describe what has to be done. Tick the members in the order you want them (2 up to the room's seats), pick each one's model and thinking effort and the team's permission mode (everything allowed by default), then click "Start the task". A member is one of:

- **An employee**: it gets a second session just for the task, working in the employee's folder; its own session at the desk is left as it was. An employee is in one task at a time, and one that has never worked has no folder yet: click "Start work" once first.
- **An advisor**: a one-off participant made for the task with the usual fields (name, folder, model, thinking effort). It has no desk, comes in by the lift, and is gone when the task ends.

**The lead**: by default the first member leads: it splits the task, hands out the parts with SendMessage, reviews the work, merges the others' branches into its own and tells you what was done. Untick the lead box and everyone gets the whole task and shares it out with the others. Either way the bridge gives each member its first prompt: who is on the team (the members reach each other by name with SendMessage), everyone's branch, and the rules.

**Git**: every member works in its own git worktree on its own branch, `symphony/<task id>/<member>` (say `symphony/T1/bob`; a second member with the same name gets `-2`), of its folder, so members never step on each other's files and your own checkout stays as it was. The worktrees are in the folder's `.symphony/worktrees/`, hidden from git by `.git/info/exclude` (your `.gitignore` is left alone). When the task ends, whatever a member hasn't committed is committed to its branch, so nothing is lost, and the worktrees are removed.

- In your own repositories nothing is merged: the branches stay for you to merge; with a lead, the lead's branch gathers everyone's work.
- A folder that isn't a git repository becomes one for the task (`git init` and a commit of everything in it) and stays one. When the task ends, the members' branches are merged into it, so the folder holds the result; the `.git` and the branches are kept.
- A merge that fails is undone and the base is left as it was; the branches stay, with the work on them, and the task window says which branch conflicts.

**In the office**: the members sit at the room's table for the whole task, and an employee's desk shows an empty chair meanwhile. The room's nameplate shows the task id; its light, green while the room is free, turns yellow while the task runs and red while a member waits for your approval or answer. Click the room for the task window: the task, the members with their folders and branches, the team chat and "End the task". A click on a member's figure in the room opens the task window too; a click on a member in the task window opens its chat window docked beside it, the same as an employee's window, one member at a time (click another member to switch). The red count in the bar at the top and the search box open a task member the same way.

**The team chat**: the task window lists the messages the members send each other, each with the sender's pixel figure and name, the text starting with `@` and who it went to. Below it is a box to write to the team yourself: `@bob-T1 …` goes to that member only (several `@names` go to all of them; typing `@` opens a list of the members to pick from with the arrow keys, Enter or a click), and a message without `@` goes to everyone in the task. Enter sends, Shift+Enter breaks a line.

A task ends when you end it in the task window or when its last session ends: the bridge stops the members' sessions, takes care of the git side as above and deletes the advisors. A task still running when the bridge stops is ended the same way when the bridge starts again.

## Accounts and seats

- An identity in symphony is a name, a token and a fixed seat. An account serves one session at a time, plus one in the task it's part of; any other session is refused and its approvals stay in the terminal.
- Claude logins are not managed here: all sessions may share one Claude login.
- Data lives in `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS) or `%AppData%\symphony` (Windows), or where `-data` / `SYMPHONY_DATA` says. `accounts.json` holds the tokens and is readable by its owner only (0600); on Windows the user folder's permissions protect it.

## Security

- The bridge only accepts requests addressed to this machine, and refuses writes that another website sends through your browser: the page can start sessions, which means running commands on this machine.
- The page has no login yet, so **don't** bind the bridge to an address other than this machine.
- The programs aren't signed. macOS may refuse a copy downloaded with a browser because it can't verify the developer: click "Open Anyway" under System Settings → Privacy & Security, or run `xattr -d com.apple.quarantine symphony` once. Windows SmartScreen may warn you too: click "More info", then "Run anyway". The release's `checksums.txt` holds the SHA-256 of every archive.

## Platforms

| System | Sessions started from the page | "Open terminal" | Tested |
| --- | --- | --- | --- |
| Linux | A pseudo-terminal inside the bridge | A graphical terminal (x-terminal-emulator, gnome-terminal, konsole, xterm) running `symphony attach` | Full end-to-end tests |
| macOS | A pseudo-terminal inside the bridge | Terminal.app running `symphony attach` | Full end-to-end tests |
| Windows | A pseudo-console (ConPTY) inside the bridge | A new console window running `symphony attach` | Cross-compiled and unit-tested only |

The bridge answers "trust this folder" for you on every platform.

## Development

```bash
go test -race ./...                         # bridge and translation unit tests
claude plugin test mods/symphony-bridge     # mod unit tests
claude plugin validate .                    # check the marketplace
claude plugin validate mods/symphony-bridge  # check the mod
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # end-to-end: starts real sessions and spends tokens
```

- Working on the mod: `go run ./cmd/bridge -mod mods/symphony-bridge`, and sessions started from the page get your copy of the mod with `--plugin-dir`; add `--plugin-dir mods/symphony-bridge` yourself to sessions you start by hand.
- Working on the pages: `go run ./cmd/bridge -web web`, and a reload shows your edits.
- Seeing the office without real sessions: start a second bridge and fill it with made-up activity: `go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo`, then `go run ./cmd/demo -base http://127.0.0.1:7799`.
- Translations: each language is one file, `web/locales/<lang>.json` for the pages and `internal/i18n/locales/<lang>.json` for the bridge, with the same keys as `en.json`; the mod's few messages are in `mods/symphony-bridge/hooks/messages.ts`. The unit tests check that every language has every key and the same `{placeholders}`, and the `TestI18n` end-to-end test opens the pages in every language.
- The end-to-end tests need claude, tmux (for the sessions the tests drive themselves) and Chromium or Chrome, so they run on Linux and on macOS. They use this repository's mod by default; set `SYMPHONY_E2E_MOD=installed` to use the installed mod. Screenshots go to `e2e/artifacts/`.
- Releases: bump the version in `mods/symphony-bridge/.claude-plugin/plugin.json`, add a `## vX.Y.Z (date)` section to `CHANGELOG.md`, bring dev to main, then tag that commit on main and push the tag (`git tag -a v0.7.0 -m v0.7.0 && git push origin v0.7.0`); GitHub Actions checks that the tag is `plugin.json`'s version and that the section exists, builds the program for every platform and publishes a release whose notes start with that section (`.github/workflows/release.yml`; a suffix like `-rc.1` makes a pre-release). `.github/release.sh v0.7.0 dist` builds the same archives locally.

| Folder | What it is |
| --- | --- |
| `mods/symphony-bridge/` | The Claude Code mod (TypeScript) |
| `.claude-plugin/marketplace.json` | The plugin marketplace that `claude plugin install` reads |
| `cmd/bridge/`, `internal/bridge/` | The bridge |
| `internal/i18n/` | Language detection and the bridge's translations |
| `web/` | The office and approval pages and their translations, built into the bridge |
| `cmd/demo/` | Made-up activity for the office |
| `e2e/` | End-to-end tests |

## License

[MIT](LICENSE)
