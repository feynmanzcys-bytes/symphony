# Contributing to symphony

Thanks for helping. Bug reports and pull requests are welcome; report security problems privately, as [SECURITY.md](SECURITY.md) says.

## Set up

- Go 1.27 or later, for the bridge.
- Claude Code 2.1.316, for the mod: its function hooks are an early-access API, and this is the version it is tested with.
- For the end-to-end tests: Chromium or Google Chrome (`SYMPHONY_E2E_BROWSER` picks one), and `tmux`. They run on Linux and macOS.

[CLAUDE.md](CLAUDE.md) is the detailed map of the code: what each file does, the conventions, and every command. The README's "Development" section has the short version.

## Before a pull request

Run these from the repository root; CI runs the same on every pull request.

```bash
gofmt -l .                                    # prints nothing when the Go code is formatted
go vet ./... && go vet -tags e2e ./e2e/
go test -race ./...                           # the bridge and the translations
for os in windows darwin linux; do GOOS=$os go vet ./internal/... ./cmd/... && GOOS=$os go build -o /dev/null ./cmd/bridge; done
claude plugin validate .
claude plugin validate mods/symphony-bridge
claude plugin test mods/symphony-bridge       # the mod's unit tests
go test -tags e2e -count=1 -v -run 'TestI18n|TestIdle|TestLayout|TestAsksDock|TestTask$|TestScreens|TestThemesLoad|TestMessenger|TestReconnect|TestTerminalView|TestContrast|TestNarrow|TestA11y|TestAskKinds|TestMotion|TestChatSteps|TestSettings|TestMarkdown' ./e2e/   # the pages in a browser, no tokens
```

`TestOffice` and `TestModStartsBridge` start real Claude sessions and spend tokens, so CI doesn't run them; they need `SYMPHONY_E2E=1`. Run them when you change the mod or how sessions start: `SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/` runs everything.

## Rules

- Pull requests go to the `dev` branch.
- Commit messages and code comments are in English. Commit messages carry no `Claude-Session:` line: the History check fails the pull request on one (drop it with `git commit --amend`, or rebase).
- User-visible text never sits in code: it goes through the translations, in all 10 languages (`web/locales/`, the themes' `locales/`, `internal/i18n/locales/`, and `mods/symphony-bridge/hooks/messages.ts`). `go test ./internal/i18n/` checks that the pages' and the bridge's catalogs have the same keys and placeholders in every language.
- When the mod's behavior changes, bump the version in `mods/symphony-bridge/.claude-plugin/plugin.json`; it is the version of the whole project.
- Add a line to the next version's section of [CHANGELOG.md](CHANGELOG.md) for each change people will notice.
