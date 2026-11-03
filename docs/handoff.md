# Handoff: what v1.2.0 leaves for later

**Status (2026-11-03):** v1.2.0 settles every item v1.1.1 left: the numbered known issues (item 30, macOS hiding its own programs' environment from `ps -E`, is a limitation the README states) and the later features. After it, the gaps the accessibility review left are closed and tested (the edges of text fields and the focus indicators at 3:1, hover states and animation frames measured, the pages reflowing at 320 pixels, the mention picker's semantics), and the checks that need no Mac ran: the Homebrew formula on Linux, the Linux ARM64 build under emulation, and on Windows a Scoop install, start at login short of a real login, an archive downloaded with a browser and a real screen reader, NVDA (they found `-autostart on` failing for every Scoop install, and NVDA a few things a screen reader stumbles on: fixed since). One numbered item is open, 31. What follows is that item, then where things stand: what was checked on real machines and what wasn't, and what the accessibility checks don't cover. A new item goes here with what happens, where in the code, a way to fix it and how to test the fix; take items up as the repository's rules say ([CLAUDE.md](../CLAUDE.md), [CONTRIBUTING.md](../CONTRIBUTING.md)): a branch and worktree per change, the full tests before any commit, a release once a batch is done. When an item is done, remove it here in the same change.

## Open

31. **Moving and blinking content that can't be stopped** (WCAG 2.1 2.2.2, level A: what moves, blinks or scrolls by itself for more than five seconds next to other content needs a way to pause, stop or hide it). What moves: the pixel office's people and its TV (`office.js`'s frame loop), Coworkers' faces (`messenger.css`: `pulse`, `look`, `tick`, `ring`, `wave`, `blink`, all infinite), Mission Control's ticker (`.ticker`, which stops only under the pointer, out of the keyboard's reach) and its alarms (`flash`, `alarm`, `blink`), and the pending chip's `nudge` in the office's HUD. Nothing stops them, and no theme honors `prefers-reduced-motion`. A way to fix it: under `prefers-reduced-motion: reduce`, stop the CSS animations (an alarm keeps a steady color and its word), lay the ticker out as a still list and let the office's people stand at their spots; and give each theme a motion switch beside its dark one (remembered like it) for everyone else. How to test it: Chrome's `Emulation.setEmulatedMedia` with `prefers-reduced-motion: reduce`, then no infinite animation in `document.getAnimations()` and the office's figures standing still over a few seconds of the page's clock (`clockTab`); the same with the switch off. The contrast of what keeps moving is measured already (`TestContrast`'s frames pass).

## Where things stand

- **Checked on real machines.**
  - v1.1.0 ran on macOS 26.7 (Apple silicon) and on Windows 11 25H2 (x86-64).
  - v1.1.1's Windows fixes ran again on that Windows machine.
  - v1.1.1 ran again on that Mac:
    - tasks whose members start in new folders: the trust prompt answered, including once when the cursor jumped back to "No"; members joining and getting their kickoff with no nudge;
    - an off-duty window showing the exit notice;
    - `symphony attach` leaving the shell clean;
    - the dark-mode page tests.
  - v1.2.0 ran on Linux with real Claude Code sessions (2.1.316, Sonnet) doing real work:
    - the office context: a session started with it named the one already online when asked who else is in the office;
    - the token gone from the environment its Bash commands see, `SYMPHONY_LAUNCH` still there;
    - "Allow for this session": the next command with the same start ran without a card, a joined command (`&&`) still asked;
    - an MCP server's form (a stdio server sending `elicitation/create`): its card with the fields on the page, the session waiting, the form answered in the terminal, the card gone and the chat saying it was submitted, the server getting the answer;
    - `/reload-plugins` after the mod's code changed: the engine reloaded the module and ran `session.start` for it again; the session stayed online past the 45 s limit, kept the page's rule, took prompts from the page, and the activity had one start line;
    - a two-line bracketed paste through the terminal's input entering as one prompt; the in-page terminal drawing the real Claude Code screen and taking typed prompts;
    - the mod downloading the release of its version (a copy saying 1.1.1) when no `symphony` was on PATH: the archive and `checksums.txt` from GitHub, the hash checked, the program started, the session seated on it.
  - v1.2.0's first batch (the fixes to the terminal, the screen model, tasks, a session's lifecycle and the mod) ran on that Mac with real sessions:
    - what a page-started session leaves running: `ps -E` there reads the environment of programs like Homebrew's, not of Apple's own (`/bin/sleep`, the shells), so a leftover of those outlives the session (the README says so);
    - the exit notice after a `kill -9`: the screen's last lines, wide characters two cells wide, nothing typed and erased;
    - `symphony attach` detaching cleanly; the launch's attach command naming the program's full path;
    - start at login under launchd (`TestAutostart`, `ThrottleInterval` 60);
    - a second Ctrl-C stopping the bridge at once with exit status 1, its sessions ended;
    - a two-member task in Chinese: the kickoff, the window saying it ended until closed, the worktrees gone and the branches kept;
    - four prompts sent while a session was busy, each entering once, together.
  - v1.2.0 ran on that Windows machine (Claude Code 2.1.316, Sonnet):
    - the unit and mod tests, and the page tests in Edge: every theme, accessibility and contrast, narrow windows, the translations, the asks dock, reconnecting, the page key, a task, the screenshot tour;
    - the login variables read from a file (`-env-file`): `HTTPS_PROXY` set, `NODE_OPTIONS` not;
    - real sessions started from the page: the office context; the token gone from what Bash sees; "Allow for this session" (the next `node` command ran without a card, one joined with `&&` asked);
    - the in-page terminal through ConPTY: the real Claude Code screen, wide characters two cells wide, a typed prompt answered, Ctrl+] back to the chat;
    - the mod downloading the release (curl.exe, certutil, tar.exe) with Windows paths, starting it, the session seated on it;
    - opening the office through the http protocol's own browser command, on a Windows that asks which app opens .html files;
    - no subagents shown for the engine's own helpers; the office's activity panel showing the bridge's recent lines after a reload.
  - Everything also runs on Linux, where CI runs.
  - Real Claude Code sessions (2.1.316) ran collaborative tasks on all three.
- **Checked after v1.2.0, without a Mac:**
  - The Homebrew formula on Linux, in the `homebrew/brew` Docker image: the published tap (`brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony`), `brew install symphony`, `symphony -version`, `brew test`, `brew audit --strict --online` and `brew style` clean for the formula (its check of the repository's workflows couldn't run there: the image's shellcheck wants a newer glibc), the bridge started with `-background`, answering with its key and stopped with `-stop`, `brew uninstall` and `brew untap`.
  - Windows 11 (that machine, v1.2.0's release):
    - Scoop: installed the official way; `scoop bucket add symphony https://github.com/feynmanzcys-bytes/symphony`, `scoop install symphony/symphony` (the hash checked, the shim made, the notes shown: Scoop drops their leading spaces), `symphony -version`, `scoop status` and `checkver` at 1.2.0, `-background` and `-stop` through the shim, `scoop uninstall` and `scoop bucket rm`. `-autostart on` through the shim failed: Scoop runs the program through a folder junction (`apps\symphony\current`), and `filepath.EvalSymlinks` on that path answers "The system cannot find the path specified.", which ended the command. Fixed after v1.2.0 (`realExe`: on Windows the path as it runs, which an update keeps valid), and checked there through a junction made like Scoop's (`mklink /J`, the branch's program): `-autostart on` from `...\current\symphony.exe` wrote the Run key's value naming `current`, the bridge answered, the value started from `System32` as a login would; `-autostart off` said to close a window only while a bridge ran. Through Scoop's own shim it waits for a release.
    - Start at login, short of a real login (from the version folder, to get past the failure above): the Run key's value written, then started from `System32` the way a login starts it, a console window and the bridge answering; `-autostart off` taking the value away and leaving the others. It said to close the bridge's window when none ran: it says so now only when one answers (`bridgeRuns`).
    - A real screen reader: NVDA 2026.3 (portable, silent, its speech logged) in Edge, driven by real key presses. The office's search read as a combo box with the hit it points at, the employee window as a dialog named by the employee, the roster's buttons saying who and what they do, HQ's tabs and its dark switch. What it stumbled on is fixed: the focus fell out of the employee window when its Stop button went (`keepFocus`); the roster's buttons and the search's hits were read again at every redraw (`setHTML` now keeps them in place under the keyboard, `pointAt`); a hit's name and line ran together ("bobWorking"); HQ read "Approvals2"; the dark switches' tooltips said the opposite of their state; the chat's input was read as a form landmark. NVDA's browse mode keeps "/" from the page (NVDA+F2 passes the next key on): the search boxes say it, `aria-keyshortcuts`.
    - An archive downloaded with Edge carries the mark of the web (`Zone.Identifier`, `ZoneId=3`). `Expand-Archive`, the README's way, leaves the unpacked files unmarked, so neither a console nor `Start-Process` met SmartScreen; Explorer's "Extract all" marks them. Smart App Control was off there.
  - The Linux ARM64 archive under QEMU user emulation (`qemu-aarch64` from the `tonistiigi/binfmt` image, no binfmt registered), not on ARM64 hardware: its checksum, `-version` (`symphony v1.2.0 (linux/arm64)`), the bridge serving the pages and the API with its key (401 without), an account made, `-stop`. The unit tests built for arm64 pass under emulation but for the ones that run an arm64 program from the host (the bridge built by `cmd/bridge`'s tests, the test binary as a tool in three of `internal/bridge`'s terminal tests): exec format error, as nothing registers the emulator for them. That QEMU build takes the argument after the program as its argv[0]: give the program twice (`qemu-aarch64 ./symphony ./symphony -version`).
- **Not yet checked on a real machine:**
  - ARM64 hardware (Linux: emulated only, above; Windows: no machine), and macOS on Intel. The release builds them.
  - v1.2.0's second batch on macOS (the Mac didn't get to its checklist). Most of it is the same code on every system and ran on Linux and Windows; three parts are macOS's own: the mod's download checking the hash with `shasum -a 256`, start at login handing the login variables over under launchd (`TestAutostart` checks it there), and the Homebrew formula there (it installed on Linux, above).
  - On macOS, mouse reports after `symphony attach`: Claude Code never turned them on there.
  - The first run of an archive downloaded with a browser, let through the way the README says: on macOS "Open Anyway" or `xattr -d com.apple.quarantine`; on Windows SmartScreen's "More info", "Run anyway" at a double-click of a copy Explorer unpacked, and what Smart App Control does when it's on (it may block an unsigned program outright, with no way past). The mod's own download uses curl, which marks nothing, so it never meets this.
  - On Windows: `-autostart on` through Scoop's own shim, once a release has the fix (a junction made by hand passed); and a real sign-out and sign-in starting the bridge.
- **Accessibility, checked by test, not by people:** WCAG 2.1 AA's colors in every theme and scheme (text 4.5:1, 3:1 large; the state marks, the edges of text fields and selects and every focus indicator 3:1), as drawn, hovered and in every frame of what animates longer than 5 seconds (`TestContrast`); the keyboard and Chrome's accessibility tree, the task chat's mention picker included (`TestA11y`); windows down to 320 px, `/approve` too (`TestNarrow`). A real screen reader read it once, NVDA on Windows (above), before the fixes it led to; Chrome's tree checks those since (`TestA11y`). Not covered: JAWS and VoiceOver, and what item 31 says about motion.
- **How the real-machine checks were done.** A Claude Code session on that machine worked through a checklist and sent back each item's result, with the commands it ran and their output; on Linux the same checks ran here. Each check used:
  - a throwaway data folder and port 7811 (`-data`, `-addr`);
  - the repository's mod through `-mod` (the installed one turned off for those sessions only: `claude --settings '{"enabledPlugins":{"symphony-bridge@symphony":false}}'`, through a wrapper given as `-claude`);
  - a cheap model.

  It never touched the user's own office, plugins or start at login.

## Later features

None are left: the plan's deferred ones (approval cards inline in the chat, `prompt.compose` with the office's context, "remember this rule", MCP elicitation) shipped in v1.2.0, as did a terminal in the page, performance tests for the newer themes, Mission Control in light colors, the mod downloading the program, build provenance, and Homebrew and Scoop packages.
