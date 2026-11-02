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
  - Everything also runs on Linux, where CI runs.
  - Real Claude Code sessions (2.1.316) ran collaborative tasks on all three.
- **Not yet checked on a real machine:** Linux and Windows on ARM64, and macOS on Intel. The release builds them; nobody has run them. On macOS, mouse reports after `symphony attach` weren't exercised: Claude Code never turned them on there. Homebrew and Scoop installs from the published tap and bucket: the formula and the manifest are written by the release's packages job, so the first real install is v1.2.0's.
- **Accessibility, checked by test, not by people:** contrast (WCAG 2.1 AA, every theme and scheme), the keyboard and Chrome's accessibility tree, windows down to 800 px (`/approve` 400). Not covered: a real screen reader (NVDA, JAWS, VoiceOver), hover states and animation frames, the 3:1 rule for control borders (about 1.2–1.5:1 in HQ, Coworkers and Mission), reflow at 320 px, the task chat's mention picker's semantics (the keyboard works).
- **How the real-machine checks were done.** A Claude Code session on that machine worked through a checklist and sent back each item's result, with the commands it ran and their output; on Linux the same checks ran here. Each check used:
  - a throwaway data folder and port 7811 (`-data`, `-addr`);
  - the repository's mod through `-mod` (the installed one turned off for those sessions only: `claude --settings '{"enabledPlugins":{"symphony-bridge@symphony":false}}'`, through a wrapper given as `-claude`);
  - a cheap model.

  It never touched the user's own office, plugins or start at login.

## Later features

None are left: the plan's deferred ones (approval cards inline in the chat, `prompt.compose` with the office's context, "remember this rule", MCP elicitation) shipped in v1.2.0, as did a terminal in the page, performance tests for the newer themes, Mission Control in light colors, the mod downloading the program, build provenance, and Homebrew and Scoop packages.
