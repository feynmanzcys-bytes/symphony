# Plan: everything the mod can report, a page protocol, and themes

**Status (2026-11-01): shipped.** v0.8.0 (the mod reports everything, the ledger and the activity log), v0.9.0 (the protocol, `symphony.js`, `ui/*`, themes, tasks loose from the rooms), v0.10.0 (HQ), v0.11.0 (Messenger, shown to users as Coworkers since v1.0.0) and v0.12.0 (Mission Control) are in `dev`. Deferred, each noted where it belongs below: the asks' cards inline in the chat flow (`chat.js` `asks: 'inline'`, §3.2), an in-page terminal view (`terminal.js`, §2.3 and §3.2), `prompt.compose` office context and "remember this allow" (§1.4), MCP Elicitation as a third ask kind (§1.4), a light mode for Mission Control (§3.3). The pixel office's keys stay in the shared catalogs (§2.4).

Written as a draft for approval, 2026-11-01. Design only; no code changes come with it. Implementation follows the repository's rules: a branch and worktree per change, full tests before any commit, the mod's version bumped (it is the project's version) and a `CHANGELOG.md` section per release.

Three deliverables, in this order, each its own release:

| Release | What | Why first |
| --- | --- | --- |
| v0.8.0 | The mod reports everything the Claude Code mod API offers that describes a session; the bridge keeps it and pushes it, even where no page shows it yet | Data before pages: the themes below are designed on this data, and it needs real sessions to validate |
| v0.9.0 | The bridge-to-page protocol written down and versioned; the page's state and transport pulled out of `office.html` into a client library and shared UI modules; themes as a folder each, switchable; the pixel office moved onto it with no visible change | The refactor is the risky part; doing it alone, with the existing e2e suite as the net, keeps a theme bug and a refactor bug apart |
| v0.10.0, v0.11.0, v0.12.0 | Three new themes, one per release: **HQ** (an enterprise management console), **Messenger** (the sessions as coworkers in an agent-messaging app; shown to users as **Coworkers**), **Mission Control** (a telemetry wall) | Each theme is small once the framework exists; shipping them one at a time keeps each release testable |

Decisions made before this design: the Messenger theme follows the agent-messaging-app layout (a sidebar of named coworkers with a title line and a progress line, the avatar's motion as its status, cards in the chat, layered oversight), drawn in symphony's own style; the fourth theme is Mission Control; this document is English and lives in `docs/`.

## 1. v0.8.0: everything the mod can report

### 1.1 Where we stand

Claude Code 2.1.316's function-hook API (early access) has these events: `session.start/end/send/receive/append/compact/attach/detach/measure`, `turn.start/step/complete`, `tool.call/check/describe`, `prompt.submit/fill/suggest/edit/change/autocomplete/section/context/compose/attachment/mention`, `agent.spawn/offer`, `command.run/describe`, `config.set/describe`, `ui.render/resolve/press/input/select/message/fault/scroll/focus`, `skill.prompt`, `attribution.text`, `plugin.register`, `engine.create`, plus every classic hook as `classic.<Name>` (SessionStart, UserPromptSubmit, PreToolUse, PostToolUse, PostToolUseFailure, PostToolBatch, PermissionRequest, PermissionDenied, Notification, Stop, StopFailure, SubagentStart, SubagentStop, TaskCreated, TaskCompleted, TeammateIdle, PreCompact, PostCompact, PreModelSwitch, PostModelSwitch, CwdChanged, DirectoryAdded, FileChanged, WorktreeCreate, WorktreeRemove, ConfigChange, InstructionsLoaded, Elicitation, ElicitationResult, MessageDisplay, UserPromptExpansion, Setup, SessionEnd). The nouns on `$`: `session` (id, cwd, root, model, turns, messages, repo, usage, version, surfaces, surface, compact, send, append, authorize, identity), `turn`, `prompt`, `tool`, `command`, `config`, `agent` (spawn, list, register), `model`, `mcp`, `ui`, `audio`, `fs`, `store`, `state`, `clock`, `http`, `process`, `settings`, `env`, `telemetry`, `plugin`.

The mod uses today: `session.start/end/send/receive`, `turn.start/step/complete`, `prompt.submit`, `tool.check/call`, `command.run` (rename only), `classic.SessionStart/UserPromptSubmit`; and `$.session.id/model`, `$.env`, `$.fs`, `$.process`, `$.http`, `$.clock`, `$.ui.toast`, `$.prompt.submit`, `$.turn.abort`.

### 1.2 New reports

Every new field is optional: a bridge with an older mod, or an older mod with a new bridge, keeps working. `kind` names below are the bridge event kinds (`POST /api/event`); the right column says where the bridge keeps it (a `Session` field, a chat item, or a push only).

| Engine source | Event / payload to the bridge | Kept as |
| --- | --- | --- |
| `session.start` + `$.session.version()`, `$.session.repo()`, `$.session.surfaces()`, `e.surface`, the plugin's own version (`plugin.json`, read with `$.fs` from `$.plugin.root`) | `session.start {..., engine: {version, base}, mod: <version>, surface, repo: {root, remote, name, internal}}` | `Session.Engine`, `Session.Mod`, `Session.Surface`, `Session.Repo`; the bridge can now say "mod outdated" per session |
| `session.measure` (fires when the context fill, a rate-limit window or the cost moves; observe-only) | `session.usage {context: {tokens, window, percent}, rateLimits: [{kind, percentUsed, resetsAt}], cost: {usd}, totals: {tokens by model}}` — the mod coalesces it (the 150 ms batcher pattern of `chat.delta`, here one report per 2 s at most: the fill moves on every response, and each report is one HTTP POST through the queue) | `Session.Usage`; pushed as a `usage` message without a snapshot (like `chat`); the snapshot carries the latest, so a page that opens late has it |
| `turn.step` stop chunk (`usage: TurnUsage`, `stopReason`) | folded into the existing `chat.stop {turnId, step, usage: {input, output, cacheRead, cacheWrite, model}, stopReason}` | stays a `chat` push with no snapshot (v0.5.0's `chatOnly` rule holds): the page adds the step's tokens to its own per-session figures (the sparklines); the session's totals travel on the coalesced `usage` push; the `thinking`/`text` chat items are unchanged |
| `turn.complete` (`reason`, `refusal`, `usage`, `answer`) | `turn.complete {..., reason, refusal: {category, explanation}, usage}` | `reason` other than `answer` becomes a chat `notice` (aborted exists; add `refused`, `error`); `Session.Turns++` |
| `classic.StopFailure` (`error`, `error_details`) | `turn.error {error, details}` | chat `notice` with the error; `Session.LastError` |
| `agent.spawn` (`description`, `subagentType`, `model`, `background`, `tool_use_id`; the agent's id is on the spawn's *result*, so the hook reports after `await next(e)`) and `classic.SubagentStop` (`agent_type`, `last_assistant_message`) | `agent.start {agentId, type, description, model, background}`, `agent.end {agentId, type}` | `Agent` gains `Type`, `Description`, `Model`, `Background`, `Started` (the chat's `tool` row for `Agent` already shows the description, through `summary()`) |
| `classic.TaskCreated` / `TaskCompleted` (the agent-team task list; `task_id`, `task_subject`, `teammate_name`) | `todo.create {id, subject, teammate}`, `todo.done {id}` ("todo", not "task": `task` is the collaborative task) | `Session.Todos` (open subjects, capped at 50) |
| `classic.Notification` (`message`, `title`, `notification_type`) | `session.notify {type, title, message}` | chat `notice`; the page may show a toast in a theme that has them |
| `session.compact` / `classic.PostCompact` (`trigger`) | `session.compact {trigger, tokensBefore, tokensAfter}` | chat `notice` ("context compacted"); `Session.Compactions++` |
| `classic.PostModelSwitch` (`from_model`, `to_model`, `source`) | folded into `session.model {model, effort, from, source}` | `Session.Model` as today; a chat `notice` when `from` is set |
| `classic.CwdChanged` (`new_cwd`) | `session.cwd {cwd}` | `Session.Cwd` (today only `sticky.cwd` from the start) |
| `classic.PermissionDenied` (`tool_name`, `reason`) | folded into `tool.end {..., denied: true, reason}` | the chat `tool` row's tooltip |
| `tool.call` itself (the mod clocks `next(e)` with `$.clock`; no classic hook needed) and `classic.PostToolUseFailure` (`error`) | folded into `tool.end {..., durationMs, error}` | `ChatItem.DurationMs`; `Session.ToolTime` (sum) |
| `command.run` (every slash command, not just `/rename`) | `command.run {name, args}` (args cut to 200) | chat item kind `command` (a dim monospace line like a tool row) |
| `classic.Stop` (`background_tasks`, `session_crons`) | `session.background {tasks: [{id, kind, status}], crons: n}` | `Session.Background` (count and kinds) |
| `session.attach` / `session.detach` (`surface`, `clientId`) | `session.surfaces {surfaces: [...]}` | `Session.Surfaces` (a phone attached shows in the HQ roster) |
| `session.end` (`reason`, the classic SessionEnd's reasons) | `session.end {reason}` | the `ended` chat notice names the reason |
| `$.tool.list()` at start (MCP servers: `mcp__<server>__<tool>`) | `session.tools {mcp: [server names], count}` | `Session.MCP`; refreshed when `classic.ConfigChange` fires |
| `prompt.submit` (`origin.kind`: composer, bridge, peer, task-notification, scheduled-trigger, channel...) | already sent as `origin`; the bridge keeps the whole set | `ChatItem.Origin` (today the page shows only typed/page/task) |

Payload budget: a `session.usage` push is under 300 bytes; `chat.stop` grows by about 100 bytes; `agent.start` is one per subagent. `TestPerfTasks` measures push sizes and will show the difference.

### 1.3 Bridge changes

- `Event` gains the fields above; `Session` gains `Engine`, `Mod`, `Surface`, `Surfaces`, `Repo`, `Usage` (with the totals), `Turns`, `Compactions`, `Todos`, `Background`, `MCP`, `ToolTime`, `LastError`; `Agent` gains `Type`, `Description`, `Model`, `Background`, `Started`.
- A new push type `usage` (no snapshot, as `chat`): the snapshot carries `Session.Usage` so a page that opens late has it. `chat.stop` stays a `chat` push and never triggers a snapshot.
- **A small ledger, because the themes need history the bridge doesn't keep today**: ended sessions vanish after 10 minutes, the activity log is built page-side from the events it saw, and nothing carries the bridge's start time, so "cost today", "mission time" and a late-opened page's activity log would be unbacked. The bridge adds: `started` (its own start time) to the snapshot; a ledger, one row per account and calendar day with the cost (the difference between the session's running total and its last report), the tokens, the turns and the tool calls (persisted in `ledger.json` in the data folder, 90 days kept): today's rows ride in the snapshot (`ledger`), every day kept at `GET /api/ledger`; `GET /api/activity` with the last 500 activity lines (sessions coming and going, tool calls, asks and their outcomes, messages, errors, launches) the bridge logs as it goes, so every page sees the same timeline. The per-step sparklines stay page-side (the last 60 seconds of what the page saw).
- New chat item kinds: `command`, and new `notice` codes (`refused`, `error`, `compacted`, `modelSwitched`, `notify`); the page translates them (`chat.notice.<code>` keys).
- `/api/state` and the snapshot carry `protocol: 1` from now on (see §2.1), so pages can refuse a bridge they don't understand.
- Nothing changes in approvals or launches.

### 1.4 Considered and not done now

- `$.session.identity()`: answers `{deny}` to any mod the organization's managed settings didn't install, so the `.claude.json` read stays for the login email. *Changed in v1.0.0: the mod no longer reads `.claude.json`; the login is only what `identity()` answers, usually nothing.*
- `ui.render`, `$.ui.open`, bands and status lines: symphony's UI is the page; the mod stays invisible in the terminal beyond its toasts.
- `prompt.compose` (injecting "who else is in the office" into the system prompt) and `tool.check` honoring a "remember this allow for the session" rule from the page: both wanted for the Messenger (Coworkers) theme's approval cards; they change what the model does, so they get their own design later.
- `classic.Elicitation` (an MCP server asks the person a form): a third ask kind for the page, later.
- `classic.FileChanged`: too chatty; at most a count per turn, later if a theme wants it.
- `classic.MessageDisplay`: duplicates `turn.step` text.
- `$.session.messages()` (the transcript): the chat log is enough; the whole transcript stays in Claude.
- `$.session.usage({breakdown})`: one API request per category; the free call is enough.

### 1.5 Tests

Mod unit tests for every new hook (the fake engine in `tests/`); hub tests for the new fields, the `usage` coalescing and the snapshot; `TestOffice` checks that a real session reports `engine`, `repo`, `usage` and a subagent's description; `TestPerf` with made-up `usage` pushes.

## 2. v0.9.0: the protocol and the theme framework

### 2.1 `docs/protocol.md`

A written, versioned description of what the bridge gives a page, mostly what exists today:

- **Transport**: `GET /api/stream` (SSE, one JSON message per `data:` line, a keepalive comment every 15 s), `GET /api/state` (the full snapshot with the message log). Same-origin only: the bridge refuses a foreign `Host` and writes with a foreign `Origin`.
- **Messages** on the stream, by `type`: `snapshot` (`protocol`, `accounts`, `sessions`, `asks`, `launches`, `tasks`, `messageTail`, `messages` only in the first), `event` (`event`, `title`), `message`, `chat` (a whole `ChatItem` or `{delta}`), `usage` (new), `ask`, `settled`, `launch`, `task`.
- **Objects**: `AccountView`, `Session`, `Agent`, `Ask` (and `inputTruncated`), `Message`, `Launch`, `Task`, `Member`, `TaskRepo`, `ChatItem`, `ChatDelta`, `Usage`; each field with its meaning, as the Go structs have them today.
- **Derived state the page is expected to compute**: a session's display state from `state` + pending asks; `stateSince` for idling; off duty after `StaleAfter`; an ended session kept `endedTTL`; `messageTail` catch-up (`/api/state` when a message id is unknown); the "a member is away in a task" rule (an account's desk session vs its task sessions).
- **Commands**: `POST /api/answer/{id}`, `/api/say/{session}`, `/api/abort/{session}`, `/api/accounts` (+ `DELETE`), `/api/launch` (+ `GET /api/launch/options`, `/api/dirs`), `/api/terminal/{id}/{stream,input,resize,open}`, `/api/tasks` and `/api/tasks/{id}/{end,say,chat}` (+ `DELETE`), `GET /api/chat/{session}`; request and response bodies; the error envelope `{error, code, params}` and the code list (`errors.go`); `409`, `401`, `404` meanings.
- **Internationalisation**: the pages' catalogs (`/locales/<lang>.json`), `error.<code>` and `chat.notice.<code>` keys, the ten languages.
- **Version**: `protocol: 1`. Additions keep the number; a removed or renamed field raises it. A page compares and shows `error.protocol_mismatch`.
- **Tasks come loose from the meeting rooms** (a bridge change in this release, the protocol's one semantic change): today `Task.Room`/`RoomName` are required, the member cap is "the room's seats", creation is a click on a free room, the kickoff names the room and `task_room_busy` is a bridge error; the three new themes have no rooms, and the pixel office's room count depends on the window's height, so a room index isn't even the same in two browsers. From v0.9.0: `POST /api/tasks` takes `room` only as an optional hint (the room the task was started from, kept on the task for the theme that made it; `roomName` is gone, hence protocol 2); the cap is 2 to 5 members bridge-side; the kickoff drops its room phrase; `task_room_busy` goes. The pixel office seats running tasks in its rooms client-side, deterministically (tasks in id order into free rooms, by seat count; a task with more members than any free room gets the largest and the extra members stand), so every browser agrees; a click on a free room still opens the task dialog there, as does "New task" in the HUD. `TestTask` and `TestScreens` click a room today; they get the room-less path too.

The mod-to-bridge side (`/api/event`, `/api/ask`, `/api/wait`, `/api/inbox`) is documented too, in a second section, as the bridge is the only consumer.

### 2.2 `web/symphony.js`: the client library

One ES module every theme imports; no DOM in it. It is the part of `office.html` that is not drawing (`applySnapshot`, `applyEvent`, `applyMessage`, `applyChat`, `takeMessages`, `catchUpMessages`, the chat log fetch, `post`, the launch option load), extracted as is.

```js
import { connect } from '/symphony.js'
const sym = connect()                  // EventSource + /api/state, reconnects, catches up
sym.state                              // the latest snapshot plus the message log
sym.on('snapshot' | 'event' | 'message' | 'chat' | 'usage' | 'ask' | 'settled' | 'launch' | 'task', fn)
sym.on('change', fn)                   // once per push, after the state is updated
sym.chat(sessionId)                    // the log, fetched once, kept up by chat pushes; .items, .on('item' | 'delta')
sym.say(sessionId, text); sym.abort(sessionId); sym.answer(askId, body)
sym.launch(req); sym.launchOptions(); sym.dirs(path); sym.deleteAccount(id)
sym.tasks.create(req); sym.tasks.end(id); sym.tasks.forget(id); sym.tasks.say(id, text); sym.tasks.chat(id)
sym.terminal(launchId)                 // stream, input, resize, open
// derived helpers, pure functions:
import { viewState, isOffDuty, awayInTask, memberOf } from '/symphony.js'
```

Errors come back as `{code, params}`; the theme turns them into text with `errorText` from `i18n.js`.

### 2.3 Shared UI modules, `web/ui/`

Pieces every theme needs and none should rewrite; each a module that renders into an element the theme gives it and uses `symphony.js` for the calls. They carry their own CSS variables so a theme restyles them with a stylesheet, not a fork:

- `asks.js` (exists): the ask cards.
- `chat.js`: the employee window's flow (the chat log renderer with its item kinds, `richText`, the streaming blocks, the dock with the textarea and the asks overlay). The window frame (head, chips, buttons) stays the theme's; the flow is shared.
- `launch.js`: the launch dialog (folder browser, model/effort/permission/resume selects, the preview, the status watch).
- `task.js`: the task dialog and the task window (members, team chat with the `@` picker).
- `terminal.js`: the "open terminal" call and, later, an in-page terminal view (the Messenger theme's preview layer wants one; `xterm.js` is a front-end library and out, so this is a plain monospace renderer of the stream, cursor and colors only).
- `i18n.js` (exists): the language, `t()`, the picker; it loads the shared catalog and the theme's own (`/t/<name>/locales/<lang>.json`), the theme's keys over the shared ones.

The pixel office keeps using these the way it does now; `office.html` shrinks to the canvas, the HUD and the window frames.

### 2.4 Themes on disk and in the bridge

```
web/
  symphony.js  i18n.js  ui/*.js  locales/<lang>.json      shared
  approve.html                                             stays as it is
  themes/
    office/     theme.json  index.html  office.css  office.js  locales/<lang>.json
    hq/         ...
    messenger/  ...
    mission/    ...
```

- `theme.json`: `{name, title (an i18n key), description (a key), version, since: <protocol>, icon}`.
- Routing: `GET /` serves the remembered theme (a cookie `symphony.theme`, else `office`); `GET /t/<name>/` serves a theme and sets the cookie; `?theme=<name>` on `/` does the same. A theme's files are under `/t/<name>/`, the shared ones at the root as today. `-web web` serves it all from disk.
- The theme picker is a shared module too (`ui/themes.js`): a small menu listing `themes/*/theme.json`, served as `GET /api/themes`; each theme puts it where its HUD is.
- `embed.go` embeds `themes/*`; the bridge lists themes at start and logs them.
- `/approve` is untouched (it already is a minimal theme over `asks.js`).

### 2.5 What a theme must do (the theme contract)

Written into `docs/protocol.md` so a third party can write one:

1. Show every account (off duty included) and every online session, with its display state and model; one click reaches a session's chat (shared `chat.js`) and from it the asks.
2. Make pending asks visible within one glance and reachable within one click from anywhere (the pixel office's red employee and the HUD chip).
3. Offer a new employee (the launch dialog) and "Start work" for an off-duty one.
4. Show running tasks and open the task window; a theme may draw tasks its own way (the pixel office's meeting rooms, HQ's board) but must use `task.js` for the dialog and the window.
5. Show the message log and the activity log in some form.
6. Use `t()` for every string, with its own catalog in all ten languages (`go test ./internal/i18n/` checks theme catalogs for parity too).
7. Degrade when the bridge's `protocol` is newer or older: show `error.protocol_mismatch`, don't break.
8. Mark the elements the e2e tests look for: `data-session=<id>` on a session's clickable representation, `data-ask=<id>` on a card, `data-theme=<name>` on `<html>`; the shared modules already carry their own ids.

### 2.6 Tests

- `TestScreens` runs its tour per theme (a `themes` list; the pixel-office tour stays the long one, the others start with: office with made-up activity, a session window, an ask, a launch dialog, a task window, in en/zh-CN/ja), failing on a page error, a missing key or a raw key.
- `TestPerf` and `TestPerfTasks` run against every theme (frame gaps, per-frame cost).
- `TestAsksDock` and `TestIdle` stay pixel-office tests (idle life is a pixel-office feature).
- A `TestThemes` checks routing: `/` default, `/t/hq/` sets the cookie, `?theme=`, an unknown theme is 404, `/api/themes` lists four.
- Unit tests for `symphony.js`'s pure helpers through chromedp (no Node in the repo).
- Acceptance of v0.9.0 itself: the pixel office looks and behaves exactly as v0.8.0 (the `TestScreens` screenshots compared by eye).

## 3. The themes

Common ground: all three are DOM + CSS with SVG or Canvas where a chart or a figure is needed (no front-end libraries, as the rest of the project); light and dark follow `prefers-color-scheme`, with a switch remembered per theme; the session states map the same way everywhere: `idle` green, `thinking`/`tool` yellow, `waiting` red, off duty gray, a task member marked by its task id.

### 3.1 HQ: the enterprise management console (v0.10.0)

The office as the management page of a large company's internal tooling: dense, tabular, numbers first. The place the v0.8.0 data pays off.

```
┌ Symphony HQ  ▸ Overview  People  Approvals  Projects  Activity  ·  🔍  ⚙ ───────────┐
│ Headcount 12  Online 9  Working 4  Waiting 2 ●  Today $4.21  Context avg 38 %  ⚠ 1  │
├───────────────────────────────────────────────────────────────────────────────────────┤
│ People                                                                   Approvals ● │
│ ┌ name  ┬ state ┬ model/effort ┬ folder ┬ task ┬ ctx ┬ tools ┬ $  ┬ uptime ┬ ▶ ┐  ┌ bob: Bash  rm -rf …  [Allow][Deny] ┐ │
│ │ bob   │ ● tool│ opus / max   │ ~/app  │ T7   │ ▓▓░ 62%│ 143 │ 1.20│ 2h10  │ ▶ │  │ ann: Question  Which DB?  [Open]   │ │
│ │ ann   │ ● wait│ sonnet / high│ ~/site │  —   │ ▓░░ 21%│  38 │ 0.33│ 0h41  │ ▶ │  └────────────────────────────────────┘ │
│ │ …                                                                               Projects (tasks)   T7 running  3 members│
├ Activity ─────────────────────────────────────────────────────────────────────────────┤
│ 14:02 bob  Bash  go test ./...  ✓ 4.1s    14:02 ann asked: Which DB?    14:01 T7 ann → bob: done with the API            │
```

- **Overview**: KPI tiles (headcount, online, working, waiting, cost today, average context fill, errors), a sparkline of activity per minute (Canvas), a bar of cost by model.
- **People**: the roster table, sortable and filterable (state, model, folder, task); a row expands into the session's window (`chat.js`) in a right drawer; off-duty rows have "Start work"; a row's "▶" opens the terminal. Context fill is a tiny bar; rate limits a badge when over 80 %.
- **Approvals**: the pending asks as a queue, oldest first, with the shared cards; a count in the tab.
- **Projects**: the collaborative tasks as a board (running / ending / failed) with members, branches and the team chat (`task.js`); "New project" is the task dialog (no room: §2.1).
- **Activity**: `/api/activity` and the message log as one timeline, filterable by person; "Today $…" and the per-account cost columns come from the ledger (§1.3).
- New strings: about 60 keys × 10 languages. Charts follow the `dataviz` skill when the time comes (one palette, no color-only meaning).

### 3.2 Messenger, shown as Coworkers: the coworker chat (v0.11.0)

The sessions as coworkers in a messaging app: identity and status in one avatar, a conversation per coworker, cards in the chat, oversight in layers.

```
┌ Symphony ──────────────┬───────────────────────────────────────────┬──────────────────┐
│ 🔍 Search              │ ◉ bob  ·  opus / max  ·  ~/app  ·  T7     │ Computer   ▣ ▢ ⤢ │
│ ◉ bob   backend eng.   │                                           │ ┌──────────────┐ │
│   ● running go test…   │   ▢ You: add the API tests                │ │ $ go test …  │ │
│ ◎ ann   frontend       │   ◉ Thinking… (folded)                    │ │ ok  4.1s     │ │
│   ● waiting: Which DB? │   ◉ I'll start with the handlers…         │ │              │ │
│ ○ cid   off duty       │   ▸ Bash  go test ./...  ✓ 4.1s           │ └──────────────┘ │
│ ─ Group chats ──────── │   ┌ Permission: Bash  rm -rf build ┐      │ Routines         │
│ ⬢ T7  API rewrite  3   │   │ [Allow] [Deny]                  │      │  (later)         │
│                        │   └─────────────────────────────────┘      │                  │
│ + New coworker         │ ✎ Message bob…                     [Send] │                  │
└────────────────────────┴───────────────────────────────────────────┴──────────────────┘
```

- **Sidebar**: one row per account: the avatar, the name, a title line (the model and effort read as a job title: "opus / max"; the folder's last segment), a progress line (the running tool's summary, the last answer's first line, "waiting: <question>", "off duty since 10:12"). Sorted waiting first, then working, idle, off duty; a search box over names, session names and folders (as the HUD's).
- **Avatar as status**: a simple geometric face per account (drawn from the account id, Canvas or SVG, our own design), whose motion is the state: still when idle, a slow pulse when thinking, a quick tick when a tool runs, a raised hand and a red ring when waiting, gray and eyes closed off duty, a check mark on a task member whose task ended. Hover shows the current action in words. No separate status dots.
- **Chat**: the shared `chat.js` flow, styled as bubbles; the asks come as cards inline in the flow (the overlay form is the pixel office's; here a card sits where it happened, and the input stays usable for a message that doesn't answer it) — this is a `chat.js` option (`asks: 'overlay' | 'inline'`). *Shipped in v0.11.0 with the overlay; the inline option is still to do.*
- **Oversight in three layers**: a small icon in the header while a tool runs (the status), a docked right panel with the in-page terminal view read-only (the preview, `terminal.js`), and "Take over" which opens the full terminal (`/api/terminal/<id>/open`, or the in-page view with input when it exists). *Shipped in v0.11.0 as the status (the computer block's pulsing dot and the running tool), the last tool rows as the preview, and Take over opening the terminal window; `terminal.js` is still to do.*
- **Group chats**: the collaborative tasks, listed under the coworkers; a group's chat is the task window's team chat (`task.js`) with the members' avatars; "New group" is the task dialog.
- **Right panel**: the coworker's profile: folder, repository, engine version, context fill, cost, MCP servers, subagents; "Routines" is a placeholder for scheduled runs, later.
- New strings: about 40 keys.

### 3.3 Mission Control: the telemetry wall (v0.12.0)

A NASA-style control room: the whole screen a wall of panels, one per session, with gauges, a ticker of events and alarms for asks. Made for a second monitor; readable from across the room.

```
┌ SYMPHONY MISSION CONTROL          T+02:14:07   SESSIONS 9/12   ALARMS 2 ●   COST $4.21 ┐
├──────────────┬──────────────┬──────────────┬──────────────┬──────────────┬─────────────┤
│ BOB   ● TOOL │ ANN  ● WAIT  │ CID   ○ OFF  │ DAN  ● IDLE  │ T7-BOB ● TOOL│ T7-ANN ● THK│
│ opus/max     │ sonnet/high  │              │ haiku        │ opus/max     │ sonnet      │
│ CTX ▓▓▓▓▓░░░ │ CTX ▓▓░░░░░░ │              │ CTX ▓░░░░░░░ │ CTX ▓▓▓▓░░░░ │ CTX ▓▓░░░░░░│
│ 62%  5h 41%  │ 21%  5h 41%  │              │  9%          │ 51%          │ 30%         │
│ ▁▂▃▅▇▅▃▂▁▂▃  │ ▁▁▂▂▁▁▁▁▁▁▁  │              │ ▁▁▁▁▁▁▁▁▁▁▁  │ ▂▃▅▇▇▅▃▂▁▂▃  │ ▁▂▂▃▂▁▁▂▂▁▁ │
│ go test ./…  │ ⚠ Which DB?  │              │              │ Edit api.go  │ thinking    │
├──────────────┴──────────────┴──────────────┴──────────────┴──────────────┴─────────────┤
│ ▶ 14:02:07 BOB  Bash go test ./...  OK 4.1s   ▶ 14:02:01 ANN  ASK question   ▶ 14:01:55 T7 ANN→BOB …  │
└───────────────────────────────────────────────────────────────────────────────────────────┘
```

- **Panels**: a grid that fits the window (the `fitScreen` idea without pixels: the biggest tile size at which every session fits), one tile per online session and per task member, a dim tile per off-duty account at the end. Each tile: the name, the state in a word with its color, the model and effort, a context-fill gauge, the rate-limit windows as small gauges when present, a 60-second sparkline of tokens per step (Canvas, from `chat.stop`'s usage), the current tool's summary or the last notice.
- **Alarms**: a waiting session's tile flashes its border and the alarm count in the header goes red; a click on a tile opens the session window (`chat.js`) as an overlay; the alarm chip opens the first waiting one (as the HUD's pending chip).
- **Ticker**: the activity log and the messages as one line that scrolls at the bottom; a pause on hover.
- **Header**: mission time (since the bridge's `started`), sessions online / headcount, alarms, cost today (the ledger), the theme picker and the language picker.
- **Tasks**: a task's members' tiles share a colored stripe with the task id; the stripe opens the task window; "New employee" and "New task" are buttons in the header.
- An optional sound on a new alarm (off by default; `audio` is the page's, not the mod's).
- *Shipped in v0.12.0 dark only (the room's look); the light mode the common ground promises is still to do.*
- New strings: about 35 keys. Big type, high contrast, color plus a word for every state (color-blind safe).

## 4. Risks and open points

- The mod API is early access: v0.8.0 touches many more events, so each Claude Code upgrade needs the e2e run; a hook that fails is skipped by the engine and must never affect the session (the queue and `within` already guarantee that; the new hooks keep the pattern).
- Push volume: `usage` and `chat.stop`'s growth are measured by `TestPerfTasks` before release; coalescing is the lever.
- The refactor of `office.html` (3,300 lines) is the largest single change in the project; v0.9.0 ships nothing else.
- Translation load: three themes add about 135 keys × 10 languages. Shared keys first (`state.*`, `chat.*`, `launch.*`, `task.*` already exist); theme catalogs hold only what is theirs.
- In-page terminal (`terminal.js`): a plain renderer of the stream with no `xterm.js` is a project in itself (ANSI colors, cursor movement, alternate screen); v0.11.0 may ship the preview layer as the last lines of the screen, as the launch error already shows them, and leave a full renderer for later.
- `prompt.compose` and "remember this allow" are wanted by Messenger's cards and deferred; the cards ship with Allow / Deny.

## 5. Order of work and acceptance

1. v0.8.0: mod hooks and bridge fields; `claude plugin validate`, `claude plugin test`, `go test -race ./...`, cross-compile, `TestOffice` with real sessions showing the new fields in `/api/state`.
2. v0.9.0: `docs/protocol.md`, `symphony.js`, `ui/*`, themes folder and routing, the pixel office moved; the full e2e suite; the pixel office compared screen by screen with v0.8.0.
3. v0.10.0 HQ, v0.11.0 Messenger (Coworkers), v0.12.0 Mission Control: each with its `TestScreens` tour, `TestPerf`, its ten catalogs, a README section with a screenshot, and the mod unchanged unless a theme needs a new field.
