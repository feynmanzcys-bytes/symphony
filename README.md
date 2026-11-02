# symphony

**English** | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

symphony is an independent project. It is not affiliated with, endorsed by or sponsored by Anthropic. Claude and Claude Code are trademarks of Anthropic, PBC.

Put a team of Claude Code sessions in a pixel-art office: see at a glance who is working and who is waiting for you, talk to each one in its own window, and handle permission requests and AskUserQuestion questions there with one click.

![The pixel office: employees at their desks in the open office, small meeting rooms on the left, the meeting room and the lounge on the right](docs/office.png)

The page comes in themes: the pixel office above; the **HQ console**, a management console with a KPI strip, the roster, an approvals queue, a projects board and an activity timeline; **Coworkers**, the sessions as coworkers in a messaging app, their faces showing what they do, the conversation in the middle, the profile and the computer on the right; and **Mission Control**, a control room's wall for a second screen: a telemetry tile per session, the asks as alarms, the activity as a ticker. Pick one in the bar at the top; the bridge remembers it.

![The HQ console: key figures in a strip at the top, the employees and their work below](docs/hq.png)

![Coworkers: the sessions listed on the left with their faces, a conversation in the middle, the profile and the computer on the right](docs/coworkers.png)

![Mission Control: a dark wall with one telemetry tile per session and a ticker of activity at the bottom](docs/mission.png)

## Install

The quickest way: open Claude Code and paste

> Install https://github.com/feynmanzcys-bytes/symphony for me

Claude Code reads this README and does the steps in the next section; it asks you before any change that lasts beyond the install. To install by hand, follow the same steps.

### For AI agents: the install runbook

You are reading this because a user asked you to install symphony. Do the steps below in order, in a shell, and finish with the report in step 6. Make the routine choices yourself (the archive, the folder on `PATH`), but ask the user before any change that persists beyond this install: starting symphony at every login, editing a shell rc file, or changing the user's `PATH`. List all the choices in the report.

symphony has two parts and needs both:

1. **The `symphony` program** (the bridge): a single binary, a local web server at `http://127.0.0.1:7788/` that serves the office page and runs the sessions. It should be on the user's `PATH`, under the name `symphony`, because the mod starts it with `symphony -background` when a session finds no bridge running.
2. **The `symphony-bridge` plugin** (the mod): a Claude Code plugin from this repository's marketplace, installed at user scope. It only acts in sessions started with `SYMPHONY_TOKEN` and does nothing in any other session.

#### Step 1: check the machine

```bash
uname -sm                 # Linux/macOS: picks the archive below (x86_64 → amd64; arm64/aarch64 → arm64)
claude --version          # needs Claude Code 2.1.x (tested with 2.1.316)
git --version             # only for collaborative tasks; note it in the report if missing, don't stop
symphony -version         # already installed? prints "symphony v1.1.1 (darwin/arm64)"
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:7788/api/state   # a bridge running? 401: symphony 1.x; 200: an older one; 000: none
claude plugin list --json # the mod already installed? look for an entry with "id": "symphony-bridge@symphony"
brew --version            # Homebrew there (macOS, Linux)? step 2 can install with it
```

On Windows use PowerShell: `$env:PROCESSOR_ARCHITECTURE` is `AMD64` or `ARM64`, and `scoop --version` answers when Scoop is there.

If `claude` is missing, stop and tell the user to install Claude Code first. If the program and the mod are both in place, go to "Update" below instead. If only one of them is, do the steps for the other: steps 2 and 3 for the program (unless `symphony -version` already prints the latest release), step 4 for the mod, then step 5. A bridge that is already running keeps the program it started with: if one runs, you'll stop it before step 5.

#### Step 2: get the program

**With Homebrew or Scoop.** This repository is also a Homebrew tap and a Scoop bucket. If step 1 found Homebrew (macOS, Linux) or Scoop (Windows), you can install with it instead of the rest of this step and step 3: it downloads the newest release's archive, checks it against the SHA-256 in the formula or the manifest (written from the release's `checksums.txt` when the release came out) and puts `symphony` on `PATH`. Then go on with step 4, and say in the report which one installed it.

```bash
brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
brew install symphony
symphony -version          # prints the version and the platform
```

```powershell
scoop bucket add symphony https://github.com/feynmanzcys-bytes/symphony
scoop install symphony
symphony -version
```

Otherwise, download the archive for this machine and the release's `checksums.txt` into a new folder of its own, and check the archive before you install anything. The archives are `symphony_<os>_<arch>.tar.gz`, with `<os>` `linux` or `darwin` (macOS) and `<arch>` `amd64` or `arm64`, and `symphony_windows_<arch>.zip`.

On Linux and macOS:

```bash
d=$(mktemp -d)   # a new folder that only you can read
echo "$d"        # note it: step 3 installs from it
cd "$d"
curl -fsSLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_<os>_<arch>.tar.gz
curl -fsSLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/checksums.txt
if command -v sha256sum >/dev/null; then sha256sum -c --ignore-missing checksums.txt; else shasum -a 256 -c --ignore-missing checksums.txt; fi   # must print "symphony_<os>_<arch>.tar.gz: OK"
tar xzf symphony_<os>_<arch>.tar.gz              # the program is now "$d/symphony"
```

If the check doesn't print `OK`, stop: don't install the program, and tell the user. With `gh` instead of curl, download into `$d` (gh makes the folder when it doesn't exist yet), then run the same check and `tar` there:

```bash
gh release download -R feynmanzcys-bytes/symphony -p 'symphony_<os>_<arch>.tar.gz' -p checksums.txt -D "$d"
```

On Windows, in PowerShell:

```powershell
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$zip = "symphony_windows_$arch.zip"
$d = Join-Path $env:TEMP ("symphony-" + [guid]::NewGuid())   # a new folder
New-Item -ItemType Directory -Path $d | Out-Null
$url = 'https://github.com/feynmanzcys-bytes/symphony/releases/latest/download'
Invoke-WebRequest -UseBasicParsing -Uri "$url/$zip" -OutFile (Join-Path $d $zip)
Invoke-WebRequest -UseBasicParsing -Uri "$url/checksums.txt" -OutFile (Join-Path $d 'checksums.txt')
$want = ((Select-String -SimpleMatch -Path (Join-Path $d 'checksums.txt') -Pattern $zip).Line -split '\s+')[0]
$got = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $d $zip)).Hash
if ($got -ne $want) { throw "checksum mismatch: $zip" } else { 'checksum OK' }
```

Then, if `gh` is installed and signed in (`gh auth status` succeeds), check where the archive was built: every archive of a release carries GitHub's attestation that this repository's release workflow built it. Outside a terminal `gh` prints nothing when the check holds, so go by its exit status:

```bash
gh attestation verify "$d/symphony_<os>_<arch>.tar.gz" --repo feynmanzcys-bytes/symphony && echo 'attestation OK'
```

```powershell
gh attestation verify (Join-Path $d $zip) --repo feynmanzcys-bytes/symphony
if ($LASTEXITCODE -eq 0) { 'attestation OK' }
```

Releases before v1.2.0 have no attestation (`no attestations found`); for those the checksum is the check. Any other failure: stop, as for a wrong checksum. Without `gh`, or not signed in, skip this check and say so in the report.

**From source** on Linux or macOS, when the user prefers it (Go 1.27 or later). Build the binary; `go run` won't do, because `-autostart` refuses a `go run` build:

```bash
d=$(mktemp -d)
git clone https://github.com/feynmanzcys-bytes/symphony.git "$d/src"
cd "$d/src" && go build -o "$d/symphony" ./cmd/bridge   # the program is now "$d/symphony", as from an archive
```

| System | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz) (Intel) | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz) (Apple silicon) |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

Each archive holds the program (`symphony`, or `symphony.exe` on Windows), `LICENSE` and `README.md`. The release's `checksums.txt` holds the SHA-256 of every archive.

#### Step 3: put it on PATH

Install the program from step 2, `$d/symphony` (if your shell doesn't keep variables between commands, write out the folder step 2 printed). Use a folder that is already on the user's `PATH` and writable without `sudo`: `~/.local/bin`, `/usr/local/bin` or `/opt/homebrew/bin`, in that order.

```bash
install -m 755 "$d/symphony" ~/.local/bin/symphony
xattr -d com.apple.quarantine ~/.local/bin/symphony 2>/dev/null || true   # macOS: the binary isn't signed
symphony -version          # prints "symphony v1.1.1 (linux/amd64)": the version and the platform
```

If none of these folders is on `PATH`, ask the user before you change anything. With their yes, create `~/.local/bin`, add it to the shell's rc file (`~/.zshrc` or `~/.bashrc`) and say so in the report; with their no, install the program where they say and tell them that the mod can't start the bridge by itself while `symphony` isn't on the `PATH` of their Claude Code sessions (starting at login, in step 5, still works).

On Windows, unpack it into a folder of its own (`$d` and `$zip` are from step 2; in a new shell, set them again):

```powershell
$dest = Join-Path $env:LOCALAPPDATA 'Programs\symphony'
Expand-Archive -LiteralPath (Join-Path $d $zip) -DestinationPath $dest -Force
& (Join-Path $dest 'symphony.exe') -version   # prints "symphony v1.1.1 (windows/amd64)"
```

Adding that folder to the user's `Path` is a lasting change: ask the user first. With their yes, run the line below and open a new shell; with their no, use the full path. SmartScreen may warn on the first run: "More info", then "Run anyway".

```powershell
[Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'User') + ";$dest", 'User')
```

If your own shell doesn't find `symphony` after this (its `PATH` was read before you changed it), use the absolute path for the rest of the steps.

#### Step 4: install the mod

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

This adds the repository as the plugin marketplace `symphony` and installs the mod at user scope, without prompts. A local clone of the repository can be the marketplace instead: `claude plugin install symphony-bridge --marketplace /absolute/path/to/symphony`. Sessions then read the mod from that folder, and a `git pull` there updates it, so keep such a clone in a lasting place, not in a temporary folder.

Verify:

```bash
claude plugin list --json   # one entry has "id": "symphony-bridge@symphony", "scope": "user", "enabled": true
```

- `"scope": "project"` or `"local"`: the mod would only load in one folder. Run `claude plugin uninstall symphony-bridge@symphony`, then install again with `--scope user`.
- `"enabled": false`: `claude plugin enable symphony-bridge@symphony`.

#### Step 5: start the bridge

If step 1 found a bridge running, stop it first, as in "Update" below: it runs the old program, and the new one would only find it there and step aside. Stopping it ends the sessions it started, so tell the user before.

Ask the user whether symphony should start at every login.

If yes:

```bash
symphony -autostart on -open=false
```

This starts the bridge now and at every login (a systemd user service on Linux, a launchd agent on macOS, the registry's Run key on Windows), and prints, after the date and time:

```
symphony now starts when you log in (<where>); it's running at http://127.0.0.1:7788/?key=<key>
```

If no:

```bash
symphony -background -open=false
```

This starts the bridge detached from the terminal (it doesn't start again after a restart of the machine), and prints, on standard error after the date and time:

```
the symphony bridge runs in the background at http://127.0.0.1:7788/?key=<key>; its log: <data folder>/bridge.log
```

When this version's bridge already runs, it prints (the same way) `a symphony bridge is already running at http://127.0.0.1:7788/?key=<key>` instead; that is fine, don't start another. `<key>` is the page key, which signs a browser in to the office: don't paste it anywhere else. If the program answers `127.0.0.1:7788 is taken: another program, or a symphony bridge with another data folder, answers there`, tell the user what answers there (an older symphony bridge counts too: stop it as in "Update") and don't pick another address without asking.

Verify the whole chain with one request, which shows the page key from the data folder (through standard input, so the key isn't on a command line other users could see):

```bash
printf 'X-Symphony-Key: %s\n' "$(cat "${SYMPHONY_DATA:-${XDG_CONFIG_HOME:-$HOME/.config}/symphony}/key")" | curl -s -H @- http://127.0.0.1:7788/api/launch/options      # Linux
printf 'X-Symphony-Key: %s\n' "$(cat "${SYMPHONY_DATA:-$HOME/Library/Application Support/symphony}/key")" | curl -s -H @- http://127.0.0.1:7788/api/launch/options   # macOS
```

```powershell
$data = if ($env:SYMPHONY_DATA) { $env:SYMPHONY_DATA } else { Join-Path $env:APPDATA 'symphony' }
(Invoke-WebRequest -UseBasicParsing -Headers @{ 'X-Symphony-Key' = (Get-Content (Join-Path $data 'key')) } -Uri http://127.0.0.1:7788/api/launch/options).Content
```

The data folder is `~/.config/symphony` on Linux (`$XDG_CONFIG_HOME/symphony` when that is set), `~/Library/Application Support/symphony` on macOS and `%AppData%\symphony` on Windows, or `SYMPHONY_DATA` when that is set.

The answer holds `"enabled":true` and `"mod":{"ok":true,"id":"symphony-bridge@symphony","version":"1.1.1"}`: the bridge runs, it found `claude`, and the mod is installed where sessions will load it. With `"enabled":false`, `mod.code` says what is wrong and `mod.fix` the command that fixes it: `mod_not_installed` or `mod_project_scope` (back to step 4), `mod_disabled` (enable it), `mod_check_failed` (`claude plugin list --json` failed, so Claude Code is probably too old). `"mod":null` means the bridge didn't find `claude` on its `PATH` at all (its log says so): put it there, or start the bridge with `-claude /path/to/claude`. An answer with `"code":"page_key"` means the request showed no key or the wrong one: check that `cat` found the key file (the data folder above); if it did, the running bridge was started with another data folder.

The bridge's log is `bridge.log` in the data folder; started at login, it logs to `journalctl --user -u symphony` on Linux and `~/Library/Logs/symphony.log` on macOS.

#### Step 6: report to the user

Open the office by running `symphony`: with the bridge running, it opens the office in the default browser, already signed in, and exits. It also prints the address with `?key=`, which signs in any browser on this machine.

Tell the user:

- To open the office, run `symphony` (or double-click it); `http://127.0.0.1:7788/` alone works in a browser that is already signed in. Click the "+" on an empty desk to hire the first employee: pick a folder, and a Claude Code session starts there, inside the bridge; its window on the page is its chat.
- New employees follow the user's Claude Code settings (model, thinking effort, permission mode) unless they pick others in the dialog. `bypassPermissions` runs every command and edit without asking, and what a session reads can then make it run commands as the user; the dialog warns when it's picked.
- Every session spends the user's Claude plan or API credit, like any Claude Code session; a task runs 2 to 5 sessions at once.
- Only sessions started from the page (or by hand with `SYMPHONY_TOKEN`, see "Start sessions" below) appear in the office. Their other Claude Code sessions, including the one doing this install, are not shown.
- What was installed where: the path of the program, the mod at user scope, the bridge started at login or in the background, any `PATH` or rc file change, the choices you made, and that `git` is missing if it is.
- How to undo it all: "Uninstall" below.

Rules: the bridge listens only on this machine (it refuses any address that isn't; keep the default `127.0.0.1:7788` unless the user picks another port), don't run it with `sudo`, and don't start a second bridge at the same address, or on the same data folder (symphony refuses that and names the running one).

#### Update

Download the new release as in step 2. The running bridge keeps the old program until it restarts (and on Windows a running program can't be replaced), so stop it with `symphony -stop`, replace the program as in step 3, and start it again the way it ran before: `symphony -autostart on -open=false` if it starts at login (this starts it now too), else `symphony -background -open=false`. If the bridge runs with an `-addr` or `-data` other than the defaults, give the same ones to each of these commands: `-stop` stops only the bridge of that data folder (at `-addr`, or wherever that folder's bridge runs), and says `nothing to stop` when none answers. Stopping the bridge ends the sessions it started, so tell the user before. (`-stop` came with v1.1.0; to stop a v1.0.0 bridge, end its process: `pkill -x symphony` on Linux and macOS, `Stop-Process -Name symphony` on Windows.)

Installed with Homebrew or Scoop, `brew update && brew upgrade symphony` (or `scoop update; scoop update symphony`) takes the place of the download and step 3, between the same `-stop` and new start. If symphony starts at login, the login entry names Homebrew's link to the program (`bin/symphony`), which the upgrade moves to the new version, so `-autostart on` there only starts it again now.

The mod follows with `claude plugin update symphony-bridge@symphony` (with a clone as the marketplace, `git pull` there instead).

The program and the mod share one version number; [CHANGELOG.md](CHANGELOG.md) lists what each release changed.

#### Uninstall

```bash
symphony -stop                                    # stops the bridge and its sessions, however it was started (add its -addr and -data if they aren't the defaults)
symphony -autostart off                           # no longer starts it at login (says so when it didn't)
claude plugin uninstall symphony-bridge@symphony  # the mod
claude plugin marketplace remove symphony         # the marketplace entry
rm ~/.local/bin/symphony                          # the program, wherever step 3 put it
```

If step 3 added `~/.local/bin` to a shell rc file, remove that line too. On Windows, in PowerShell:

```powershell
symphony -stop                                               # stops the bridge and its sessions (add its -addr and -data if they aren't the defaults)
symphony -autostart off                                      # no longer starts it at login
claude plugin uninstall symphony-bridge@symphony
claude plugin marketplace remove symphony
$dest = Join-Path $env:LOCALAPPDATA 'Programs\symphony'
Remove-Item -Recurse -Force $dest                            # the program
$p = ([Environment]::GetEnvironmentVariable('Path', 'User') -split ';' | Where-Object { $_ -and $_ -ne $dest }) -join ';'
[Environment]::SetEnvironmentVariable('Path', $p, 'User')    # the Path entry step 3 added
```

Installed with Homebrew, run `brew uninstall symphony` and `brew untap feynmanzcys-bytes/symphony` in place of the `rm` line; with Scoop, `scoop uninstall symphony` and `scoop bucket rm symphony` in place of the last four PowerShell lines.

The data folder (the accounts with their tokens, the page key, the message log, the ledger, the tasks, and files such as `lock`, `bridge.addr` or a `*.corrupt-<time>` set aside) stays; delete it to remove everything. Deleting it also deletes the page key: the next start makes a new one, and every browser has to sign in again. On Linux with a snap or flatpak browser, `open.html` is in that browser's own folder (`~/snap/<snap>/common` or `~/.var/app/<app id>`); delete it too.

## What it does

- **Every session as a chat**: click an employee and its window opens in the middle of the page: who it is at the top, and below it the session as a chat: the prompts, the model's thinking (folded, "Thinking…" in orange while it goes on), its answers as they stream, the tool calls, and messages from other sessions in purple. Write at the bottom and it reaches the session as your prompt; a Stop button interrupts it.
- **Approvals in the browser**: permission requests (running commands, editing files) and AskUserQuestion questions rise over the input of that employee's window as cards, and the red count in the bar at the top takes you to them. When the bridge isn't running, doesn't know the account or can't be trusted, the session falls back to the terminal as usual and never hangs.
- **A pixel-art office**: each session is an employee at a fixed desk. Green is idle, yellow is working, red is waiting for your approval or answer, gray is off duty. Idle employees first linger at their desks with their phones or games, then wander off: TV on the lounge sofa, foosball, tag, a chat at the round table, coffee, a look out of the window; after a while they nap, often hiding in the meeting room. The character's size follows the model and its muscles follow the thinking effort. The office is one floor of a big company's building and fills the window: the open office in the middle, small meeting rooms for 3 to 5 people with nameplates on its left, the meeting room and the lounge on its right, and lifts where people come and go. The bar at the top also sets the pixel size, in four steps: the same office, drawn in finer pixels and with more detail at each step.
- **Find anyone, call them back**: click an idle employee and it heads back to its desk (and stays there until its next turn); the search box in the bar at the top finds employees by name, session name or folder (press `/` to jump to it), and clicking a count beside it (idle, working, pending, off duty) lights up everyone in that state. The tab under the bar slides it up out of the way and back.
- **Messages between sessions**: when sessions message each other (SendMessage), an envelope flies across the office, and a message log keeps them.
- **Start sessions from the page, nothing pops up**: click the "+" on an empty desk to hire a new employee and pick the folder. The model, thinking effort and permission mode follow your Claude Code settings ("Default") unless you pick others; picking `bypassPermissions` shows a red warning of what it allows. Click "Start work" on an off-duty employee to bring them back to the same desk, with the choices they last worked with. The session runs on a terminal of its own inside the bridge, so no window opens and nothing takes your focus; "Open terminal" in the employee's window shows that terminal when you want it, and closing the window changes nothing.
- **Teams in the meeting rooms**: click a free small meeting room, write a task and pick 2 to 5 members: employees, who each get a second session just for the task, or one-off advisors. The first member leads by default and hands out the parts with SendMessage; every member works in its own git worktree on its own branch, so nobody steps on anyone's files. The members sit in the meeting room until the task ends.
- **10 languages**: English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil) and Русский. The page follows your browser's language and has a language picker in the bar at the top; the bridge's messages follow `-lang`, else your locale (`LANG` and the like), else English; the mod's messages follow `SYMPHONY_LANG` (the page's language, for a session started from the page), else your locale, else English.
- **Local only**: the bridge is a single Go program (standard library only; the end-to-end tests also use chromedp) that listens on this machine only, the pages are plain JavaScript, and nothing depends on a third-party service.

![An employee's window: the session as a chat, with approval cards over its input](docs/approve.png)

## Requirements

- Claude Code 2.1.316 (tested with this version). The mod uses Claude Code's function hooks, which are still an early-access API, so run the end-to-end tests after upgrading Claude Code.
- The bridge comes as a ready-made program for Linux, macOS and Windows; Go 1.27 or later is only needed to build it from source.
- `git`, for collaborative tasks only: every member of a task works in a git worktree.
- Nothing else: sessions started from the page run inside the bridge, on every platform.

## Running the bridge

`symphony` starts the bridge and opens the office in your browser, already signed in (`-open=false` turns that off). The browser gets the bridge's page key as a cookie through `open.html`, a file in the data folder that only you can read, so the key never shows on a command line; on Linux, when your browser is a snap or a flatpak, that file goes into the browser's own folder instead, because such browsers can't read the data folder. Opening <http://127.0.0.1:7788/> in a browser that isn't signed in shows a page that says how to get in: run `symphony` again, or open the address with `?key=` that the bridge printed in its log. <http://127.0.0.1:7788/approve> is a simpler page for approvals only. Starting `symphony` again while it runs just opens the office. On Windows, double-clicked, `symphony.exe` runs in a window of its own, and closing that window stops the bridge.

| Flag | Default | What it does |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | Address to listen on. Its host must be `127.0.0.1`, `localhost` or `::1` (as in `[::1]:7788`); the bridge refuses any other |
| `-data` | `$SYMPHONY_DATA`, else `symphony` in your config folder | The data folder: the accounts with their tokens (`accounts.json`), the page key (`key`) and `open.html`, the message log (`messages.json`), the ledger (`ledger.json`), the tasks (`tasks.json`), `-background`'s log (`bridge.log`), `lock` and `bridge.addr` (which bridge uses it, and where), and a damaged message log or ledger set aside as `*.corrupt-<time>` |
| `-claude` | `claude` | The claude executable |
| `-lang` | your locale, else `en` | Language of the bridge's messages (`SYMPHONY_LANG`, `LC_ALL`, `LC_MESSAGES` or `LANG` set the default) |
| `-mod` | empty | For development: give sessions this folder's mod with `--plugin-dir`, overriding the installed copy |
| `-web` | empty | For development: serve the pages from this folder, so edits show after a reload |
| `-open` | `true` | Open the office in the browser once the bridge is up; when this data folder's bridge already runs at `-addr`, just open its office |
| `-autostart` | — | `on`: start the bridge whenever you log in, with the other flags given, and start it now; `off`: undo that |
| `-background` | — | Start the bridge in the background, detached from the terminal and logging to `bridge.log` in the data folder, and return once it answers |
| `-stop` | — | Stop this data folder's bridge (at `-addr`, else wherever it runs), however it was started (its sessions end), and return once it's gone; it shows the key only to a bridge that proves it has it |
| `-version` | — | Print the version and exit |

When something else answers at `-addr` (another program, a bridge with another data folder, or an older symphony bridge), symphony says so and stops; it shows the page key only to a bridge that proves it has it. A data folder is one bridge's at a time: started with a data folder whose bridge already runs at another address, symphony says where it runs, opens its office and stops (give a second bridge its own `-data`).

**Start it when you log in**: `symphony -autostart on` makes the bridge start whenever you log in, with the flags you give alongside (say `-addr`), and starts it right away; `symphony -autostart off` undoes that. On Linux it's a systemd user service (`journalctl --user -u symphony` shows its log), on macOS a launchd agent (logging to `~/Library/Logs/symphony.log`), on Windows a value under the registry's Run key (the bridge runs in a console window; closing it stops the bridge until the next login). `symphony -stop` stops a bridge however it was started, until it is started again (at the next login, for one started at login). It needs the symphony program itself, not `go run`. The bridge started at login gets nothing from your shell but `PATH`, so `-autostart on` also keeps the variables its sessions may need, as your shell has them when you run it: `CLAUDE_CONFIG_DIR`, the proxy (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`, `ALL_PROXY`, in either case), `NODE_EXTRA_CA_CERTS`, `SSL_CERT_FILE` and `SSL_CERT_DIR`, and nothing else (no API keys), in `autostart-env.json` in the data folder, which only you can read; run it again after changing them. A bridge that fails as it starts (its data folder used by another bridge, a damaged `accounts.json`, something else on its address) is restarted a few times, then left alone: 5 tries in a minute on Linux (`systemctl --user status symphony` says why), one try a minute on macOS (the log says why); fix the cause and run `symphony -autostart on` again, or just log in again.

**From source**, with Go 1.27 or later: `git clone https://github.com/feynmanzcys-bytes/symphony.git && cd symphony && go run ./cmd/bridge`.

## Start sessions

**From the page**: click the "+" on an empty desk, enter a name, and pick the folder and options. The bridge runs `claude` on a terminal of its own, inside the bridge: no window opens. The employee's window shows the session as a chat, and its "Open terminal" button opens a terminal window attached to the session (`symphony attach <id>` does the same from any terminal, with `-addr` and `-data` when the bridge doesn't use the defaults; when no terminal window can open, the page shows the exact command. `Ctrl-]` or closing the window detaches, and the session goes on). The sessions are the bridge's children: stopping the bridge ends them. When a session ends — stopped from the page, with the bridge, or exiting on its own — whatever it left running in the background (a command a tool put in the background with `&`, say) ends with it: on Windows the session's job object ends them all; on Linux and macOS the bridge finds those processes by the launch id in their environment, which on macOS it can't read for Apple's own programs (`/bin/sleep`, `bash`, `zsh`, `perl`: the system hides their environment from `ps -E`), so a `node` or Homebrew server left behind ends, a plain `sleep` or shell script doesn't.

**From a terminal**: create an account to get a token, showing the page key, then start `claude` with the token:

```bash
printf 'X-Symphony-Key: %s\n' "$(cat ~/.config/symphony/key)" | curl -H @- -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # the answer holds the token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

The key file is `~/.config/symphony/key` on Linux, `~/Library/Application Support/symphony/key` on macOS and `%AppData%\symphony\key` on Windows, or `key` in the folder `-data` / `SYMPHONY_DATA` names.

The `manual` permission mode is recommended: in `auto` mode most actions don't ask anyone, so few approvals reach the page. If the bridge isn't at the default address, also set `SYMPHONY_BRIDGE`: `http://127.0.0.1:<port>`, `http://localhost:<port>` or `http://[::1]:<port>` on this machine, or an `https://` address; with anything else the mod never contacts the bridge. `SYMPHONY_LANG` picks the language of the mod's messages (else your locale, else English).

Before the session sends its token anywhere, the mod checks that the bridge at that address knows the token, without sending it (`POST /api/hello`). When the bridge can't take the session, the session shows one of these messages, its approvals stay in the terminal, and it never hangs:

| Message | What it means |
| --- | --- |
| `can't reach the bridge` | Nothing answers at the bridge's address, and starting the bridge didn't help (or the address isn't on this machine). |
| `couldn't start the bridge ...: <reason>` | Nothing answered, and `symphony -background` failed for the reason shown (say, this data folder's bridge runs at another address: set `SYMPHONY_BRIDGE` to it). |
| `the bridge doesn't know this SYMPHONY_TOKEN` | The bridge has no account with this token: it was deleted, or the bridge uses another data folder. |
| `isn't a symphony bridge this session can trust` | Something answers at the address but can't prove it knows the token: another program, or a symphony older than the mod (update it). The token isn't sent. |
| `SYMPHONY_BRIDGE ... must be on this machine or use https` | The address isn't one the mod accepts; the bridge is never contacted. |
| `this account already has a session online` | An account serves one session at a time; create another account for this session. |
| `waiting for your answer in the office page` | A permission request or question waits in the office's page, not in the terminal; answer it there, or press Esc to cancel it. Shown once per session. |

When nothing answers at the bridge's address on this machine, the session starts the bridge itself with `symphony -background -open=false -addr <that address>` (if `symphony` is on your PATH) and says so (`started the bridge ... in the background`); the bridge keeps running after the session ends. Set `SYMPHONY_DATA` if your accounts aren't in the default data folder. If this data folder's bridge already runs at another address, it says so and the session falls back to the terminal: set `SYMPHONY_BRIDGE` to that address.

## Collaborative tasks

A small meeting room is where a team of 2 to 5 sessions works together on one task. Tasks need `git`.

**Start a task**: click a free small meeting room and describe what has to be done. Tick the members in the order you want them (2 up to the room's seats), pick each one's model and thinking effort and the team's permission mode (Claude Code's own settings by default; an employee that has worked before starts with its last model and effort; picking `bypassPermissions` shows the warning), then click "Start the task". A member is one of:

- **An employee**: it gets a second session just for the task, working in the employee's folder; its own session at the desk is left as it was. An employee is in one task at a time, and one that has never worked has no folder yet: click "Start work" once first.
- **An advisor**: a one-off participant made for the task with the usual fields (name, folder, model, thinking effort). It has no desk, comes in by the lift, and is gone when the task ends.

**The lead**: by default the first member leads: it splits the task, hands out the parts with SendMessage, reviews the work, merges the others' branches into its own and tells you what was done. Untick the lead box and everyone gets the whole task and shares it out with the others. Either way the bridge gives each member its first prompt: who is on the team (the members reach each other by name with SendMessage), everyone's branch, and the rules.

**Git**: every member works in its own git worktree on its own branch, `symphony/<task id>/<member>` (say `symphony/T1/bob`; a second member with the same name gets `-2`), of its folder, so members never step on each other's files and your own checkout stays as it was. The worktrees are in the folder's `.symphony/worktrees/`, hidden from git by `.git/info/exclude` (your `.gitignore` is left alone). When the task ends, whatever a member hasn't committed is committed to its branch, the files git ignores in its worktree (build output, `.env`, `node_modules` and the like) are moved to `.symphony/kept/<member>-<task id>/` in the folder, and the worktrees are removed. symphony never deletes `.symphony/kept/`; delete it when you no longer need what is in it.

- In your own repositories nothing is merged: the branches stay for you to merge; with a lead, the lead's branch gathers everyone's work.
- A folder that isn't a git repository becomes one for the task and stays one: `git init`, then a commit of everything in it except files that often hold secrets or are bulky (`.env`, `.env.*`, `*.pem`, `*.key`, `*.p12`, `*.pfx`, `id_rsa*`, `id_ecdsa*`, `id_ed25519*`, `.npmrc`, `.netrc`, `.pypirc`, `node_modules/`, `.DS_Store`), which it lists in `.git/info/exclude`. Those files stay in the folder but aren't in the members' worktrees, so a member that needs them installs or creates its own. When the task ends, the members' branches are merged into the folder's branch, so the folder holds the result; the `.git` and the branches are kept. If the task fails to start, the folder is plain again.
- A merge that fails is undone and the base is left as it was; the branches stay, with the work on them, and the task window says which branch conflicts. Whatever else fails while a task ends stays as it is, and the task window says what failed.

**In the office**: the members sit at the room's table for the whole task, and an employee's desk shows an empty chair meanwhile. The room's nameplate shows the task id; its light, green while the room is free, turns yellow while the task runs and red while a member waits for your approval or answer. Click the room for the task window: the task, the members with their folders and branches, the team chat and "End the task". A click on a member's figure in the room opens the task window too; a click on a member in the task window opens its chat window docked beside it, the same as an employee's window, one member at a time (click another member to switch). The red count in the bar at the top and the search box open a task member the same way.

**The team chat**: the task window lists the messages the members send each other, each with the sender's pixel figure and name, the text starting with `@` and who it went to. Below it is a box to write to the team yourself: `@bob-T1 …` goes to that member only (several `@names` go to all of them; typing `@` opens a list of the members to pick from with the arrow keys, Enter or a click), and a message without `@` goes to everyone in the task. Enter sends, Shift+Enter breaks a line.

A task ends when you end it in the task window or when its last session ends: the bridge stops the members' sessions, takes care of the git side as above and deletes the advisors. A task still running when the bridge stops is ended the same way when the bridge starts again.

## Accounts and seats

- An identity in symphony is a name, a token and a fixed seat. An account serves one session at a time, plus one in the task it's part of; any other session is refused and its approvals stay in the terminal.
- Claude logins are not managed here: all sessions may share one Claude login.
- Data lives in `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS) or `%AppData%\symphony` (Windows), or where `-data` / `SYMPHONY_DATA` says. `accounts.json` holds the tokens and `key` the page key; both are readable by their owner only (0600); on Windows the user folder's permissions protect them.

## Cost and permissions

- Every session symphony starts is a Claude Code session and uses your Claude plan or API credit like any other. Opus at max effort uses it up fast, and a task runs 2 to 5 sessions at once.
- `bypassPermissions` runs every command and edit without asking. Content a session reads (a file, a web page, a tool's output) can then make it run commands as you. The launch and task dialogs show a red warning when you pick it; by default they follow your Claude Code settings.
- For sessions started from the page, the bridge answers Claude Code's "trust this folder" question for you, so start sessions only in folders you trust.

## Security

The page can start sessions, which means running commands on this machine, so the bridge guards who reaches it:

- **The page key**: the office and its API need the page key, a cookie your browser gets through the link `symphony` opens. The key is the `key` file in the data folder, readable only by you; programs show it in the `X-Symphony-Key` header.
- **This machine only**: the bridge listens on and accepts connections from this machine only. It refuses requests addressed to any other host (DNS rebinding) and writes that other websites send through your browser, and its API answers no request your browser says comes from another site or another port, not even a link. Other sites can't frame its pages. The pages load only the bridge's own files (a Content Security Policy).
- **The mod checks the bridge** before it sends its token, and the `symphony` program before it shows the page key, so something else listening at the bridge's address gets neither.
- **What it doesn't protect against**: programs running as your own user can read the page key and the tokens in the data folder, like any of your files. And browsers send a site's cookies to every port of it: a server another user of this machine runs on `127.0.0.1` gets the page key's cookie when your browser opens it, or when it takes the bridge's port while the bridge is stopped and the office is still open in a tab. On a machine you share, close the office before you stop the bridge, and if the key may have leaked, delete the `key` file in the data folder (the next start makes a new one).
- The programs aren't signed. macOS may refuse a copy downloaded with a browser because it can't verify the developer: click "Open Anyway" under System Settings → Privacy & Security, or run `xattr -d com.apple.quarantine symphony` once. Windows SmartScreen may warn you too: click "More info", then "Run anyway". The release's `checksums.txt` holds the SHA-256 of every archive, and every archive carries GitHub's attestation of where it was built; step 2 of the install checks both.

To report a vulnerability, see [SECURITY.md](SECURITY.md).

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

- Contributing: [CONTRIBUTING.md](CONTRIBUTING.md) has the setup, the checks to run before a pull request and the rules; [CLAUDE.md](CLAUDE.md) is the detailed map of the code.
- Working on the mod: `go run ./cmd/bridge -mod mods/symphony-bridge`, and sessions started from the page get your copy of the mod with `--plugin-dir`; add `--plugin-dir mods/symphony-bridge` yourself to sessions you start by hand.
- Working on the pages: `go run ./cmd/bridge -web web`, and a reload shows your edits.
- Seeing the office without real sessions: start a second bridge with a data folder of its own and fill it with made-up activity: `D=$(mktemp -d)`, `go run ./cmd/bridge -addr 127.0.0.1:7799 -data "$D"`, then in another terminal (with the same `D`) `go run ./cmd/demo -base http://127.0.0.1:7799 -data "$D"` (the demo reads the bridge's page key from that data folder).
- The screenshots in this README: `SYMPHONY_E2E=1 SYMPHONY_E2E_DOCS=1 go test -tags e2e -run TestDocsScreens ./e2e/` takes them again with made-up activity (no tokens).
- Translations: each language is one file, `web/locales/<lang>.json` for the pages and `internal/i18n/locales/<lang>.json` for the bridge, with the same keys as `en.json`; the mod's few messages are in `mods/symphony-bridge/hooks/messages.ts`. The unit tests check that every language has every key and the same `{placeholders}`, and the `TestI18n` end-to-end test opens the pages in every language.
- The end-to-end tests need claude, tmux (for the sessions the tests drive themselves) and Chromium or Chrome, so they run on Linux and on macOS. `TestOffice` and `TestModStartsBridge` start real sessions and spend tokens; the others don't. They use this repository's mod by default; set `SYMPHONY_E2E_MOD=installed` to use the installed mod. Screenshots go to `e2e/artifacts/`.
- Releases: bump the version in `mods/symphony-bridge/.claude-plugin/plugin.json`, add a `## vX.Y.Z (date)` section to `CHANGELOG.md`, bring dev to main, then tag that commit on main and push the tag (`git tag -a v1.0.0 -m v1.0.0 && git push origin v1.0.0`); GitHub Actions checks that the tag is `plugin.json`'s version and that the section exists, builds the program for every platform, attests where the archives were built and publishes a release whose notes start with that section (`.github/workflows/release.yml`; a suffix like `-rc.1` makes a pre-release); for a release that isn't a pre-release it then commits the Homebrew formula and the Scoop manifest to main (and to dev when that is a fast-forward), so pull both afterwards. `.github/release.sh v1.0.0 dist` builds the same archives locally, byte for byte on Linux.

| Folder | What it is |
| --- | --- |
| `mods/symphony-bridge/` | The Claude Code mod (TypeScript) |
| `.claude-plugin/marketplace.json` | The plugin marketplace that `claude plugin install` reads |
| `cmd/bridge/`, `internal/bridge/` | The bridge |
| `internal/i18n/` | Language detection and the bridge's translations |
| `web/` | The office and approval pages and their translations, built into the bridge |
| `cmd/demo/` | Made-up activity for the office |
| `e2e/` | End-to-end tests |
| `docs/` | The protocol between the bridge and the pages, and the screenshots |

## License

[MIT](LICENSE)

symphony is an independent project. It is not affiliated with, endorsed by or sponsored by Anthropic. Claude and Claude Code are trademarks of Anthropic, PBC.
