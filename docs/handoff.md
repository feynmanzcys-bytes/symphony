# Handoff: what v1.1.1 leaves for later

**Status (2026-11-02):** v1.1.1 is released. The batch after it (branch `handoff/all`, merged from `handoff/terminal`, `handoff/screen`, `handoff/tasks`, `handoff/lifecycle` and `handoff/mod`; mod 1.1.2) settled items 1 to 10, 12 to 14, 16 to 18 and 20 to 22 of the list v1.1.1 left; what is below is what remains. Each item says what happens, where in the code, a way to fix it and how to test the fix. Take items up as the repository's rules say ([CLAUDE.md](../CLAUDE.md), [CONTRIBUTING.md](../CONTRIBUTING.md)): a branch and worktree per change, the full tests before any commit, a release once a batch is done. When an item is done, remove it here in the same change.

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

## Tasks

11. **A session that names its task late joins it only when the account has no other session in that task** (`joinableLocked` in hub.go). Only an older mod's early report, together with a stale, crashed member, can meet this; the session then stays a desk session. It is fine as it is; this note is for whoever changes how sessions join tasks.

## The mod

29. **Development mode loads two mods.** `claude --plugin-dir <folder>` (what the bridge's `-mod` gives its sessions) loads that mod beside the installed `symphony-bridge@symphony` when there is one (`claude plugin list` shows both: `@symphony` and `@inline`). Each runs its own hooks: two inbox polls, every report twice, two asks for one permission. Seen on Windows as a prompt from the page entering a session twice (the installed 1.1.1 mod without acks took the item the 1.1.2 mod had been handed). The bridge now hands unacknowledged items to one poll per 3 s (`inboxRehand`), which hides that case, not the rest.
    - Fix: when `-mod` is given and the installed plugin is enabled, have the bridge warn at start (`claude plugin list --json` already runs for the install check) and tell how to disable it (`claude plugin disable symphony-bridge@symphony`); or pass a flag that disables it for the session, if Claude Code grows one. Until then, disable the installed mod while developing with `-mod`.
    - Test: a session with both; the bridge's log, or the session's `mod` field alternating between the two versions.

19. **The token stays in the session's environment.**
    - What happens: the session's Bash commands, npm scripts and MCP servers inherit `SYMPHONY_TOKEN`. With it they could call the mod's endpoints: empty the inbox, report made-up events, make ask cards. They could not answer the cards; that needs the page key. Since v1.1.0 the bridge no longer passes the token on.
    - Fix: `$.env.set('SYMPHONY_TOKEN', undefined)` once the mod has read it. First check that a module reloaded by `/reload-plugins` can still read it.

## Bridge lifecycle and platforms, continued

30. **On macOS, a session's leftover Apple programs aren't ended.** The launch-id kill (`procsWithEnv`, `ps -E`) can't read the environment of Apple's platform binaries (`/bin/sleep`, `bash`, `zsh`, `perl`: `KERN_PROCARGS2` hides it from a plain user on macOS 26.7), so a `nohup sleep &` left by a tool survives the session; `node`, Homebrew programs and anything Go-built are found. Seen on the Mac check of handoff/all. The README says so.
    - Fix ideas: snapshot the session's descendants (pid, start, pgid, sid) every second while it runs and, at the end, kill what still runs in those process groups or sessions (a tool's shell lives milliseconds, so this misses the quick `nohup x &` too); or have the mod's `tool.call` hook record `pgrep -P` of the tool's shell; or accept it.
    - Test: TestTermReapsWhatItLeftRunning on a Mac with `/bin/sleep` as the leftover.

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
