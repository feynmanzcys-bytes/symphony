# symphony-bridge

Unofficial. Not affiliated with Anthropic.

The Claude Code mod of [symphony](https://github.com/feynmanzcys-bytes/symphony), a pixel-art office in a web page where a team of Claude Code sessions sit as employees at their desks. With it, a session shows up in the office: its window there shows the session as a chat and takes prompts, and the session's permission requests and AskUserQuestion questions are answered in the page.

## What it needs

- The symphony bridge, the program that serves the office page. Get it from the [releases](https://github.com/feynmanzcys-bytes/symphony/releases/latest); the repository's README says how to install and start it. When the bridge on this machine isn't running as a session starts, the mod starts it with `symphony -background`, if `symphony` is on PATH.
- A session started with `SYMPHONY_TOKEN`, an office account's token (sessions started from the office page get one). In every other session the mod does nothing: it contacts no bridge, shows no message and leaves approvals alone.

## Install

```sh
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

Update it with `claude plugin update symphony-bridge@symphony`.

## What it sends, and where

- Only to the bridge named by `SYMPHONY_BRIDGE` (by default `http://127.0.0.1:7788`): plain http only on this machine, else https. Before the token leaves the session, the bridge has to prove it knows the token; anything else gets nothing.
- What the session does, for the page to show: its prompts, the model's thinking and text, tool calls, subagents, usage figures, the model and effort, the folder and the repository's name (never its remote), and the Claude login's email only where the organization's managed settings installed the mod.
- When the bridge can't be reached, refuses the token or the account is busy, the session goes on as usual and approvals stay in the terminal.

## License

MIT, see [LICENSE](LICENSE).
