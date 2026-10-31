# Changelog

What changed in each release of symphony. The program (the bridge) and the mod share one version number. A release's notes start with its section here: the release workflow stops before building when the section is missing.

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
