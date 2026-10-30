# Changelog

What changed in each release of symphony. The program (the bridge) and the mod share one version number. A release's notes start with its section here: the release workflow stops before building when the section is missing.

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
