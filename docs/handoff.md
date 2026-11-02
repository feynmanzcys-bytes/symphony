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
- **Accessibility, checked by test, not by people:** contrast (WCAG 2.1 AA, every theme and scheme), the keyboard and Chrome's accessibility tree, windows down to 800 px (`/approve` 400). Not covered: a real screen reader (NVDA, JAWS, VoiceOver), hover states and animation frames, the 3:1 rule for control borders (about 1.2–1.5:1 in HQ, Coworkers and Mission), reflow at 320 px, the task chat's mention picker's semantics (the keyboard works).
- **How the real-machine checks were done.** A Claude Code session on that machine worked through a checklist and sent back each item's result, with the commands it ran and their output. Each check used:
  - a throwaway data folder and port 7811 (`-data`, `-addr`);
  - the repository's mod through `-mod`;
  - a cheap model.

  It never touched the user's own office, plugins or start at login.

## Later features

- From the status block of [the plan](plan-protocol-themes.md):
  - approval cards inline in the chat;
  - `prompt.compose` with the office's context;
  - "remember this rule";
  - MCP elicitation;
