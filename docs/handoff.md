# Handoff: what v1.2.0 leaves for later

**Status (2026-11-03):** v1.2.0 settles every item v1.1.1 left: the numbered known issues (item 30, macOS hiding its own programs' environment from `ps -E`, is a limitation the README states) and the later features. No numbered item is open. What follows is where things stand: what was checked on real machines and what wasn't, and what the accessibility review didn't cover. A new item goes here with what happens, where in the code, a way to fix it and how to test the fix; take items up as the repository's rules say ([CLAUDE.md](../CLAUDE.md), [CONTRIBUTING.md](../CONTRIBUTING.md)): a branch and worktree per change, the full tests before any commit, a release once a batch is done. When an item is done, remove it here in the same change.

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
- **Not yet checked on a real machine:**
  - Linux and Windows on ARM64, and macOS on Intel. The release builds them; nobody has run them.
  - v1.2.0's second batch on macOS (the Mac didn't get to its checklist). Most of it is the same code on every system and ran on Linux and Windows; three parts are macOS's own: the mod's download checking the hash with `shasum -a 256`, start at login handing the login variables over under launchd (`TestAutostart` checks it there), and the Homebrew formula.
  - On macOS, mouse reports after `symphony attach`: Claude Code never turned them on there.
  - Homebrew and Scoop installs. The release's packages job writes the formula and the manifest, so the first real install is v1.2.0's from the published tap and bucket; none ran from local archives either (the Windows machine has no Scoop).
- **Accessibility, checked by test, not by people:** contrast (WCAG 2.1 AA, every theme and scheme), the keyboard and Chrome's accessibility tree, windows down to 800 px (`/approve` 400). Not covered: a real screen reader (NVDA, JAWS, VoiceOver), hover states and animation frames, the 3:1 rule for control borders (about 1.2–1.5:1 in HQ, Coworkers and Mission), reflow at 320 px, the task chat's mention picker's semantics (the keyboard works).
- **How the real-machine checks were done.** A Claude Code session on that machine worked through a checklist and sent back each item's result, with the commands it ran and their output; on Linux the same checks ran here. Each check used:
  - a throwaway data folder and port 7811 (`-data`, `-addr`);
  - the repository's mod through `-mod` (the installed one turned off for those sessions only: `claude --settings '{"enabledPlugins":{"symphony-bridge@symphony":false}}'`, through a wrapper given as `-claude`);
  - a cheap model.

  It never touched the user's own office, plugins or start at login.

## Later features

None are left: the plan's deferred ones (approval cards inline in the chat, `prompt.compose` with the office's context, "remember this rule", MCP elicitation) shipped in v1.2.0, as did a terminal in the page, performance tests for the newer themes, Mission Control in light colors, the mod downloading the program, build provenance, and Homebrew and Scoop packages.
