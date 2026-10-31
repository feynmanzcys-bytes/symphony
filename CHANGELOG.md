# Changelog

What changed in each release of symphony. The program (the bridge) and the mod share one version number. A release's notes start with its section here: the release workflow stops before building when the section is missing.

## v0.7.0 (2026-10-31)

- **Collaborative tasks in the meeting rooms**: a click on a free small meeting room creates a task: write what has to be done, pick 2 to 5 members and start. Members are employees — each gets a second session just for the task, its own session stays as it was — and advisors, one-off participants made with the usual fields (name, folder, model, effort) and gone when the task ends. The first member picked leads by default (it splits the task, hands out the parts with SendMessage, reviews and merges); untick "lead" and everyone gets the whole task. The members sit in the meeting room for the duration; the room's nameplate shows the task; a click on the room opens the task window, where each member's chat is a click away and the task can be ended.
- **A worktree for everyone**: each member's session runs in its own git worktree on its own branch (`symphony/<task>/<name>`) of that member's folder, so members never step on each other's files and your checkout stays as it was. A folder that isn't a git repository becomes one for the task (`git init`, a commit of everything in it); when the task ends the members' branches are merged into it and the `.git` removed again, so the folder is plain files holding the results (a merge that fails leaves everything in place and says so). In your own repositories the branches stay for you to merge.
- An account remembers the folder its last session worked in, however that session was started, so an employee off duty can join a task there and "Start work" offers that folder.
- The mod is 0.7.0 (it reports the task a session belongs to): `claude plugin update symphony-bridge@symphony`.

## v0.6.1 (2026-10-31)

- Sessions started from the page no longer inherit the bridge's own Claude Code session (its messaging socket and session id): a page-started session is now a clean top-level session, as if launched from a plain terminal. Without this, when the bridge runs inside a Claude Code session (the mod can start it from one), the started sessions collided with it and SendMessage between them was never delivered.
- The employee window shows a model's thinking even when it carries no words (a model whose thinking is summarized): a folded "Thought for Ns" line instead of nothing, and the time it took once done.
- A finished answer bubble and tool rows no longer collapse to a sliver in the chat; blank lines around a fenced code block are trimmed; messages between sessions get a readable timestamp. The mod is 0.6.1: `claude plugin update symphony-bridge@symphony`.

## v0.6.0 (2026-10-31)

- **Every session as a chat**: a click on an employee opens its window in the middle of the page (it replaces the card on the right and the pending popup): the employee at the top, and below it the session as a chat: the prompts, the model's thinking (folded, "Thinking…" in orange with three dots while it goes on), its answers as they stream, the tool calls with their outcome, messages from other sessions in purple, and the asks with how they were settled. Write at the bottom and it reaches the session as your prompt (Enter sends, Shift+Enter breaks a line); Stop interrupts the running turn. The mod (0.6.0) streams the response, reports the prompts that enter and polls the bridge for what the page sends: `claude plugin update symphony-bridge@symphony`.
- **Asks in the employee's window**: permission requests and questions rise over the input of that employee's window as cards and must be handled before you can write; nothing pops up by itself any more. The red count in the bar at the top opens the first waiting employee's window.
- **No more windows, no tmux**: sessions started from the page run on a pseudo-terminal inside the bridge (`/dev/ptmx` on macOS and Linux, ConPTY on Windows): nothing opens on the screen and nothing takes your focus, and the bridge answers "trust this folder" on every platform. "Open terminal" in the employee's window opens a terminal window attached to the session (`symphony attach <id>` does the same anywhere; `Ctrl-]` or closing the window detaches, and the session goes on). The sessions are the bridge's children, so stopping the bridge ends them. `-terminal` is accepted and ignored.
- A new employee starts with everything allowed (`bypassPermissions`), Opus and the most thinking (`max`); an old one as it last worked.
- The activity log at the bottom left is a panel now, hidden until the bar's button opens it (the page remembers).
- Once a launch is online the new employee's window opens.

## v0.5.0 (2026-10-31)

- Lighter traffic between the bridge and the pages: a change pushes the message log no more (the first snapshot and `/api/state` carry it; new messages come as events, and a page that missed one fetches the log again), each push is encoded once for all pages, and a pending ask's long input strings reach the page cut to 4 KiB with a note. With 300 long messages a tool call used to send every page two snapshots of about 650 KiB.
- The office page does less per change: the building is laid out and its background drawn only when the seats, the window or the pixel size change (not on every event), the message panel and the employee card only when the messages change, name tags only when their text changes; it draws 30 frames a second, and a window drag lays out once per frame.
- The bridge keeps a failed mod check for 10 seconds, so opening the page doesn't run `claude plugin list` every time; the message log is written to disk in the background instead of while every other request waits; the pages' files answer with an ETag, so a reload downloads them only when they changed.
- The mod (0.5.0) reports events through a queue in the background, so a tool call no longer waits for two round trips to the bridge; a report that gets no answer in 5 seconds is given up, and after the bridge rejects the token nothing more is sent. `claude plugin update symphony-bridge@symphony`.

## v0.4.0 (2026-10-30)

- The office opens in your browser once the bridge is up (`-open=false` turns that off), and starting `symphony` again while it runs just opens the office.
- `symphony -autostart on` starts the bridge whenever you log in (a systemd user service on Linux, a launchd agent on macOS, the registry's Run key on Windows), and `symphony -autostart off` undoes it.
- A session started with `SYMPHONY_TOKEN` starts the bridge itself when nothing answers at its address on this machine (`symphony -background`, with `symphony` on your PATH), and the bridge keeps running after the session ends. This needs the mod at 0.4.0: `claude plugin update symphony-bridge@symphony`.
- `SYMPHONY_DATA` sets the default data folder.
- The pending popup hangs from the top of the window, so a new ask no longer moves the card you are about to click.
- The bar at the top no longer slides when the page opens with it put away.
- The archive names in the README and in the release notes link to their downloads.

## v0.3.0 (2026-10-30)

- First release: the bridge as a ready-made program, `symphony`, for Linux, macOS and Windows on x86-64 and ARM64, with `checksums.txt`. Go is now needed only to run the bridge from source.
- `symphony -version` prints the version, and the bridge's first log line shows it.
