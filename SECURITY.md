# Security policy

symphony runs Claude Code sessions on your machine and lets a web page start them, prompt them and answer their permission requests. A flaw in it can let someone run commands as you, so reports are welcome.

## Supported versions

Only the latest release gets security fixes. The program (the bridge) and the mod share one version number: update both (see "Update" in the [README](README.md#update)).

## Reporting a vulnerability

Report it privately, not in a public issue: on GitHub, open this repository's **Security** tab and click **Report a vulnerability** (GitHub's private vulnerability reporting). If that button is missing, open an issue that only asks for a private channel, without any details of the problem. Please include:

- the version (`symphony -version`, and the mod's version from `claude plugin list --json`) and the operating system;
- what you did, what happened, and what an attacker gains;
- a proof of concept, if you have one.

## What to expect

- An acknowledgement in the report's thread.
- When the report is confirmed, a fix in a new release, with a line in [CHANGELOG.md](CHANGELOG.md).
- Credit in the advisory and the changelog, if you want it.

## Threat model

What symphony protects against:

- **Other people and programs that reach the address.** The office and its API need the page key: a cookie the browser gets through the link the `symphony` program opens. The key is in the `key` file in the data folder, readable only by you. The mod's endpoints take the account's token instead.
- **Other machines.** The bridge listens on this machine only (`-addr` must be `127.0.0.1`, `localhost` or `::1`; `localhost` means `127.0.0.1`) and refuses connections from any other address.
- **Websites you visit.** The bridge refuses requests addressed to any host but this machine (DNS rebinding) and writes that other websites send through your browser, and its API answers no request your browser says comes from another site or another port of this machine, not even a link. Other sites can't frame its pages. Its pages load only the bridge's own files (a Content Security Policy).
- **Something else at the bridge's address.** Before the mod sends its token, it checks that the bridge knows the token, without sending it (`POST /api/hello`; see [docs/protocol.md](docs/protocol.md)). It sends the token only over plain http on this machine or over https. The `symphony` program (and `symphony attach`) checks the same way that the bridge has the page key before it shows the key.

What it does not protect against:

- **Programs running as your own user.** They can read the page key and the tokens in the data folder, like any of your files.
- **Other users of this machine who run a server on 127.0.0.1.** Browsers send a site's cookies to every port of it, so such a server gets the page key's cookie when your browser opens it (a link, or a website that sends your browser there), or when it takes the bridge's port while the bridge is stopped and the office is still open in a tab. On a machine you share, close the office before you stop the bridge, and if the key may have leaked, delete the `key` file in the data folder: the next start makes a new one, and every browser signs in again.
- **Tampered downloads.** The programs aren't signed. Check the archive against the release's `checksums.txt` and its build attestation (`gh attestation verify <archive> --repo feynmanzcys-bytes/symphony`; the README's install steps show how). The archives are reproducible: building a release's commit on Linux with `.github/release.sh` gives the same checksums.

## Out of scope

- Anything that needs to run as the same operating system user as the bridge.
- Claude Code's own behavior in a session: what the model does, Claude Code's permission system and its "trust this folder" question. symphony answers that question for sessions started from the page; that is documented, not a flaw.
- `bypassPermissions` doing what it says: a session in that mode runs every command and edit without asking, and content it reads (a file, a web page, a tool's output) can make it run commands as you. symphony shows a warning when you pick it.
