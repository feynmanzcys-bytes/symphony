# Handoff: what v1.1.1 leaves for later

**Status (2026-11-02):** v1.1.1 is released, and `dev` and `main` are at its last commit. Nothing below has been started. Each item says what happens, where in the code, a way to fix it and how to test the fix. Take items up as the repository's rules say ([CLAUDE.md](../CLAUDE.md), [CONTRIBUTING.md](../CONTRIBUTING.md)): a branch and worktree per change, the full tests before any commit, a release once a batch is done. When an item is done, remove it here in the same change.

## Where things stand

- **Checked on real machines.**
  - v1.1.0 ran on macOS 26.7 (Apple silicon) and on Windows 11 25H2 (x86-64).
  - v1.1.1's Windows fixes ran again on that Windows machine.
  - v1.1.1 ran again on that Mac:
    - tasks whose members start in new folders: the trust prompt answered, including once when the cursor jumped back to "No"; members joining and getting their kickoff with no nudge;
    - an off-duty window showing the exit notice;
    - `symphony attach` leaving the shell clean;
    - the dark-mode page tests.
  - Everything also runs on Linux, where CI runs.
  - Real Claude Code sessions (2.1.316) ran collaborative tasks on all three.
- **Not yet checked on a real machine:** Linux and Windows on ARM64, and macOS on Intel. The release builds them; nobody has run them. On macOS, mouse reports after `symphony attach` weren't exercised: Claude Code never turned them on there.
- **How the real-machine checks were done.** A Claude Code session on that machine worked through a checklist and sent back each item's result, with the commands it ran and their output. Each check used:
  - a throwaway data folder and port 7811 (`-data`, `-addr`);
  - the repository's mod through `-mod`;
  - a cheap model.

  It never touched the user's own office, plugins or start at login.

## Sessions and terminals

1. **A notice's screen lines can hold text that was typed and then deleted.**
   - What happens: the `exited` notice and a failed launch's error show the last lines of the session's screen. `plainText` (term.go) makes them from the raw output by laying every redraw out one after another, so text that was erased or overwritten on screen stays in. Seen on Windows: `…(shift+tab to cycle)要一个一个;ugygtfrdrderdTry "refactor <filepath>"`.
   - Fix: keep a small screen model, a grid of the terminal's size fed with the output (cursor moves, erasing in the line and the display, scrolling), and take the last lines from it. `plainText` stays for the trust prompt.
   - Test: replay a recorded Claude Code screen with typing and erasing, and compare the lines.
2. **On Linux and macOS, a process that a tool starts in the background with a plain `&` outlives the session.**
   - What happens: ending a session (`Term.Kill`) kills its process group, plus what it started that still descends from it (`descendants` in pty_unix.go). A process started with `&` is adopted by init once the tool's shell exits, so nothing finds it. Claude Code's own background mode keeps the shell, so its processes are found. On Windows the job object ends them all.
   - Fix idea: every process the session starts inherits `SYMPHONY_LAUNCH=<launch id>`. At Kill, also kill the user's processes whose environment holds it (`/proc/<pid>/environ` on Linux, `ps -E` on macOS).
   - Test: TestTermKillEndsToolsOfTheirOwn, with a shell that runs `sleep 1000 &` and exits.
3. **On Windows, a clean `/exit` also ends what the session left running.** The job closes with the terminal (`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`, pty_windows.go). Linux and macOS leave those processes alone unless the session is ended. Decide which behavior is right, then make the platforms agree, or say so in the README.

## Tasks

10. **A `git worktree add` that fails leaves the member's worktree and branch behind.**
    - What happens: a global `post-checkout` hook that exits non-zero makes the command exit 1, after git has already created both. The task start fails with `task_git_failed`. Its undo only knows the members whose worktree was added successfully (`undo.worktrees` in server_task.go), so this one stays.
    - Fix: record the member in the undo before the call. Running `worktree add` without hooks would also work, but changes behavior in the user's own repositories (LFS, husky).
    - Test: a repository whose global `core.hooksPath` holds a `post-checkout` that fails.
11. **A session that names its task late joins it only when the account has no other session in that task** (`joinableLocked` in hub.go). Only an older mod's early report, together with a stale, crashed member, can meet this; the session then stays a desk session. It is fine as it is; this note is for whoever changes how sessions join tasks.
12. **The kickoff in Chinese and Japanese lacks a space after the task id.** It reads "你是任务 T1的成员". Fix: in `task.kickoff.lead`, `task.kickoff.member` and `task.kickoff.flat` of internal/i18n/locales/zh-CN.json, zh-TW.json and ja.json, write `{task} 的` (and `{task} の`) for `{task}的` (and `{task}の`).
13. **The task window closes without saying the task ended.** After "End task" it shows "ending…", then closes once the cleanup is done, because an ended task leaves the snapshot. Only tasks.json says `ended`. Fix: before the window goes, show that the task ended (a line in the window, or a toast).

## Bridge lifecycle and platforms

14. **A start at login that fails for good restarts forever.**
    - What happens: systemd restarts it every 5 s (`Restart=on-failure`, `RestartSec=5`), and its default limit of 5 starts in 10 s is never reached. launchd restarts it every 10 s (`SuccessfulExit` false) and appends to `~/Library/Logs/symphony.log`, which nothing rotates. Permanent causes: the data folder used by another bridge (`data_in_use`), a damaged accounts.json or tasks.json, another program on the address.
    - Fix: in internal/autostart/autostart.go, put `StartLimitIntervalSec=60` and `StartLimitBurst=5` in the unit's `[Unit]` section, and `ThrottleInterval` 60 in the plist. Or exit 0 for the permanent causes. Have the README say how to look (`systemctl --user status symphony`).
15. **Starting at login passes only PATH.** `-autostart on` gives the bridge nothing but `PATH` (`autostart.Entry`). So `CLAUDE_CONFIG_DIR`, `HTTP(S)_PROXY`, `NO_PROXY` or `NODE_EXTRA_CA_CERTS` set in a shell never reach a bridge started at login, or its sessions: the mod looks uninstalled, or sessions sit at a login screen.
    - Fix: at `-autostart on`, capture those variables from a short allowlist and write them into the unit or plist.
    - Never capture `ANTHROPIC_*`: the unit file is 0644.
    - Have the README say what is captured.
16. **A second Ctrl-C while the bridge stops does nothing.** `stopAll` (cmd/bridge/main.go) runs once (`sync.Once`), so a slow stop can't be cut short: a task's git work, or a disk that hangs (writing the data is bounded at 10 s). Fix: on a second signal, run `KillAll` and exit at once.
17. **The goodbye can be lost on `/clear`.**
    - What happens: the mod queues `session.end` at the back of its outbox (register.ts), and the engine gives the session.end hook 1.5 s. With reports backed up, the goodbye is cut off. A session started by hand has no launch id to prove it is the same process, so its new id then gets 409 until the old one counts as lost, about 60 s.
    - Fix: put `session.end` at the front of the outbox, or send it at once.
18. **A damaged page key, with two sessions starting the bridge at once.**
    - What happens: each `symphony -background` writes a new key over the damaged `key` file. That path (`errBadKey` in key.go) renames over the file instead of using the `os.Link` that the first creation uses. The process whose key lost takes the running bridge for another program and says the address is taken, and its session falls back to the terminal.
    - It is rare: the key has to be damaged in the seconds after the first start, or by hand.
    - Fix: in `startBackground`, when the answer is "another", read the key again and retry if it changed.

## The mod

19. **The token stays in the session's environment.**
    - What happens: the session's Bash commands, npm scripts and MCP servers inherit `SYMPHONY_TOKEN`. With it they could call the mod's endpoints: empty the inbox, report made-up events, make ask cards. They could not answer the cards; that needs the page key. Since v1.1.0 the bridge no longer passes the token on.
    - Fix: `$.env.set('SYMPHONY_TOKEN', undefined)` once the mod has read it. First check that a module reloaded by `/reload-plugins` can still read it.
20. **The mod starts the bridge as a bare `symphony`, in the session's folder** (`$.process.run(['symphony', …])` without `cwd`, register.ts). On Windows, a `symphony.exe` in that folder might win over the one on PATH, depending on how the engine looks programs up. Fix: pass `cwd: $.plugin.root`.
21. **Windows with npm's `claude.cmd`: employee names go through cmd.exe.**
    - What happens: `CreateProcess` runs a `.cmd` file through `cmd.exe /c`, and `syscall.EscapeArg` doesn't escape for cmd.exe (pty_windows.go). `validName` in accounts.go only refuses control characters. So an employee named `R&D` or `50%x%` gives a broken command line: `--name R`, then a second command.
    - Fix: when the program is a `.cmd` or `.bat` file, either refuse arguments containing `% ^ & | < > ! "` with a coded error, or escape them for cmd.exe.
22. **A prompt from the page can be lost when the mod's connection drops.** `/api/inbox` takes the items before it writes the answer (chat.go, server_chat.go). A poll cut off in between loses them, and the page's bubble stays "queued". Fix: hand items out with an id, and drop them only when the next poll acknowledges that id.

## Code structure (no visible effect)

23. **The message log and the ledger are encoded under the hub's lock.** Each message's send and receive re-encode the whole message log: about 6 ms at 3,900 messages, which is 12 tasks with full logs. Each `chat.stop` re-encodes the ledger: about 1 ms at 2,700 rows. Fix: copy the values under the lock and encode them in the saver's goroutine (accounts.go, usage.go). Guard it with a benchmark.
24. **A launch's life is spread over several files**: server.go, server_task.go, hub.go, task.go, terminal.go. v1.1.0 gathered the desk rules (`reserveDesk`, `bindLaunchLocked`, `launchExited`). What remains is one place for "online" and "failed" (`launchOnlineLocked`, `failLaunchLocked`) that the task code calls too. Do it when a new way of starting sessions comes.

## Pages

25. **Not reviewed: accessibility, contrast, narrow windows.** That covers using the office (a canvas) by keyboard, screen readers, the contrast of the state colors and of HQ's dark mode, and windows narrower than about 1000 px. (The task window docks the member window beside it only from 1250 px.)
26. **A page whose key went stale doesn't say so.** When the `key` file is deleted, the bridge makes a new key at its next start. From then on, every API call from an open page answers 401 `page_key`, and the page just keeps reconnecting. Fix: say that the key changed, and that the office must be opened again from the `symphony` program.

## Tests

27. **The mod's tests sometimes fail under heavy parallel load** (six runs at once). The old code does the same. Run alone, and in CI, they pass.
28. **TestBackgroundLeavesTheSession runs on Linux only**, because it reads `/proc`. On macOS, `ps -E -p <pid>` and `lsof -a -p <pid> -d cwd` give the same information. For v1.1.0 it was checked by hand on a Mac.

## Later features

- From the status block of [the plan](plan-protocol-themes.md):
  - approval cards inline in the chat;
  - a terminal in the page (`terminal.js`);
  - `prompt.compose` with the office's context;
  - "remember this rule";
  - MCP elicitation;
  - performance tests for the new themes;
  - Mission Control in light colors.
- The mod downloads the `symphony` program when it is missing.
- Build provenance for the release archives.
- Homebrew and Scoop packages.
