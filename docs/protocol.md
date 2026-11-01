# The bridge's protocol

What the bridge gives the pages and takes from them, and what the mod reports to it. Protocol version **2** (the snapshot's `protocol`). Additions keep the number; a removed or renamed field raises it. A page that finds a number it doesn't know shows `error.protocol_mismatch` and goes on as far as it can.

History: 1 (v0.8.0) the first numbered one; 2 (v0.9.0) `Task.roomName` removed, `Task.room` became an optional hint.

The pages are themes of one office page (`web/themes/<name>/`), all on the same client library, `web/symphony.js`, and the same shared pieces, `web/ui/*.js`. Everything below is what those use; a theme may use the library and never touch HTTP.

## 1. Transport

- `GET /api/state`: the full snapshot (with the message log), JSON.
- `GET /api/stream`: Server-Sent Events, one JSON message per `data:` line, a `: keepalive` comment every 15 s. The first message is a full snapshot; every change in the office then pushes a message and a snapshot without the log.
- Same origin only: the bridge refuses a `Host` that isn't this machine (DNS rebinding) and any write (`POST`, `DELETE`) whose `Origin` isn't its own. The mod sends no `Origin`.
- `GET /` serves the theme the browser picked (`?theme=<name>`, remembered in the `symphony.theme` cookie; else the cookie's; else `office`), `GET /t/<name>/` picks a theme and serves it, `GET /t/<name>/<file>` its own files, `GET /api/themes` lists them (`{themes: [Theme], current, protocol}`). The shared files are at the root: `/symphony.js`, `/i18n.js`, `/asks.js`, `/ui/<file>`, `/locales/<lang>.json`. `/approve` is the simple approval page.
- Errors: `{error: <English>, code, params}`; the page translates `error.<code>` with `params`. Statuses: 400 a bad request, 401 the mod's token is unknown, 403 another account's ask or a cross-site write, 404 unknown, 409 a conflict (an account busy, an ask settled, a name taken, a session offline, a task's member busy), 412 git missing, 500 a git or terminal failure.

## 2. Messages on the stream

By `type`:

| type | fields | when |
| --- | --- | --- |
| `snapshot` | the Snapshot below; `messages` only in the first one and `/api/state`, `null` in the pushed ones | every change |
| `event` | `event` (a mod Event, §6), `title` (the account's name) | the bridge relayed a mod event that changed the office |
| `message` | `message` (a Message) | a message between sessions came or changed (paired, delivered) |
| `chat` | `session`, and `item` (a whole ChatItem) or `delta` (`{id, seq, text}`: text to append to a streamed block) | a session's chat log changed; no snapshot follows |
| `usage` | `session`, `usage` (a Usage) | a session's usage figures came; no snapshot follows |
| `ask` | `id`, `session` | a permission request or question is pending |
| `settled` | `id`, `status` (`done` or `cancelled`) | an ask was answered or withdrawn |
| `launch` | `id` | a launch was made, came online or failed |
| `task` | `id` | a task's status changed |

Catch-up: every snapshot carries `messageTail`, the latest message's id; a page whose log lacks it fell behind and fetches `/api/state` again. `symphony.js` does this (the office before v0.9.0 never did: its check always passed).

## 3. The objects

Times are RFC 3339 strings; ids are opaque strings; fields marked optional are left out when empty.

**Snapshot**: `type` (`snapshot`), `protocol`, `started` (when the bridge started), `accounts` [AccountView], `sessions` [Session], `asks` [Ask] (pending only), `messages` [Message] or null, `launches` [Launch], `tasks` [Task] (running, ending and failed), `messageTail`, `ledger` [LedgerRow] (today's rows).

**AccountView**: `id`, `name`, `seat` (a fixed desk, from 0; -1 for an advisor), `advisor`?, `session`? (the online desk session's id), `last`? (LaunchRequest: how it was last started from the page), `dir`? (the folder its last desk session worked in).

**Session**: `id`, `account`, `login`?, `title` (the session name in Claude), `cwd`, `state` (`idle`, `thinking`, `tool`, `waiting`, `ended`), `tool`? (the running tool), `prompt`? (the turn's prompt, cut to 200), `agents` {id: Agent}, `tools` (tool calls so far), `model`?, `effort`?, `launch`? (the launch id of a page-started session), `attach`? (the command that shows its terminal), `task`? (the task it works on; empty for a desk session), `started`, `updated`, `stateSince`; what the mod reports besides (all optional, an older mod sends none): `surface` (`terminal`, `desktop`, `vscode`, `mobile`), `surfaces` [string], `engine` {version, base}, `mod` (its version), `repo` {root, remote, name, internal}, `usage` (Usage), `turns`, `compactions`, `todos` [Todo], `bgTasks`, `bgCrons`, `toolCount`, `mcp` [server names], `toolMs`, `lastError`, `endReason`, `notify` {type, title, text, time}.

**Agent** (a subagent): `id`, `state` (as a session's, plus `done`), `tool`?, `type`?, `description`?, `model`?, `background`?, `started`?.

**Usage**: `context`? {tokens, window, percent}, `rateLimits`? [{kind, percentUsed, resetsAt}], `cost`? {usd} (the session's running total), `totals`? {model: Tokens} (the responses' tokens summed by model), `updated`. **Tokens**: `input`, `output`, `cacheRead`, `cacheWrite`, `model`?.

**Todo** (the agent team's task list): `id`, `subject`, `teammate`?, `created`.

**Ask**: `id`, `session`, `account`, `kind` (`permission` or `question`), `tool`?, `input`? (the tool's input; strings over 4 KiB cut, `inputTruncated: true`), `reason`?, `questions`? (AskUserQuestion's), `agentId`?, `status` (`pending`, `done`, `cancelled`), `answer`?, `created`.

**Message** (between sessions, or from the page to a task's members): `id`, `from`? (the sender's session id), `fromName`, `to`?, `toName`, `toNames`? (a task chat's mentioned members), `missed`? (of those, the ones offline), `text`, `origin`? (`page` for one the page sent; else the engine's origin kind), `agentId`?, `task`? (the task it belongs to), `delivered`, `received`, `time`.

**Launch**: `id`, `attach`, `name`, `dir`, `account`, `model`?, `effort`?, `command`, `status` (`starting`, `online`, `failed`), `error`?/`errorCode`?/`errorParams`?, `session`?, `time`, `task`?.

**Task**: `id` (`T` + a number), `room`? (an integer hint: the small room the task was started from in the theme that made it; a theme seats tasks itself and may honor it), `title`, `leader`? (the lead's account id; absent for a flat task), `permissionMode`, `lang`?, `members` [Member], `repos` [TaskRepo], `status` (`running`, `ending`, `failed`; ended ones leave the snapshot), `error`?/`errorCode`?/`errorParams`?, `created`, `ended`?. **Member**: `account`, `name` (`<account name>-<task id>`), `advisor`?, `dir`, `branch`, `worktree`, `model`?, `effort`?, `launch`?, `session`? (once it reported in). **TaskRepo**: `dir`, `inited`, `base`, `root`.

**ChatItem**: `id`, `seq`, `kind` (`user`, `thinking`, `text`, `tool`, `command`, `message`, `ask`, `notice`), `text`?, `done`, `tool`?, `summary`?, `status`? (tool: `ok`, `error`, `denied`; user: `queued`, `sent`; ask: `pending`, `allow`, `deny`, `answered`, `cancelled`), `agentId`?, `origin`? (user: `composer`, `page`, `task`, or the engine's kind), `from`?/`to`? (message), `ask`?/`askKind`?, `secs`? (thinking), `ms`? (tool), `time`. A notice's `text` is a code the page translates (`chat.notice.<code>`: `ended`, `aborted`, `refused`, `error`, `compacted`, `modelSwitched`, `notify`) and `summary` a detail shown after it; a tool row's `text` is the reason it failed or was denied.

**LedgerRow**: `day` (YYYY-MM-DD, local), `account`, `usd`, `tokens` (Tokens), `turns`, `tools`. **Activity**: `time`, `kind` (`start`, `end`, `tool`, `ask`, `settled`, `message`, `error`, `launch`, `launchFailed`), `session`?, `account`?, `name`?, `task`?, `text`?.

**Theme**: `name`, `title` {lang: text}, `description`? {lang: text}, `version`?, `since`? (the protocol version it was written for), `icon`?.

## 4. What a page derives

- A session's display state: `waiting` while it has pending asks (the bridge sets it), else `state`; off duty is `ended`, and a session is kept 10 minutes after it ended (then it leaves the snapshot); 45 seconds without a heartbeat ends it.
- `stateSince` says how long a session has been in its state (the pixel office's idle life counts from it; a reload changes nothing).
- An employee away in a task: its account has a member session online in a task (`Task.members[].session` with a session not `ended`); its desk session is separate (`AccountView.session`).
- A task's room is the page's business (§3, `room`).
- The ledger's "today" is the bridge's local day; `/api/ledger` has every day kept (90).

## 5. The calls a page makes

| call | body | answer |
| --- | --- | --- |
| `POST /api/answer/{ask}` | `{decision: "allow"\|"deny", reason?}` or `{answers: {question: answer}}` | `{ok}` |
| `POST /api/say/{session}` | `{text}` | the queued ChatItem; `session_offline`, `say_empty` |
| `POST /api/abort/{session}` | | `{ok}`; `session_offline` |
| `GET /api/chat/{session}` | | `{items: [ChatItem]}` (the last 400) |
| `POST /api/accounts` | `{name}` | the Account with its `token` (for a session started by hand) |
| `DELETE /api/accounts/{id}` | | `{ok}`; `account_busy` while it has a session or is in a task |
| `GET /api/launch/options` | | `{enabled, mod, terminal, trustHint, models, efforts, permissionModes, resumes, home, recentDirs}` |
| `GET /api/dirs?path=` | | `{path, parent, dirs: [{name, path}], roots}` |
| `POST /api/launch` | LaunchRequest: `{account?, name?, dir, model, effort, permissionMode, resume, lang?}` | the Launch; the page follows its `status` in the snapshot |
| `POST /api/terminal/{launch}/open` | | `{ok}` or `window_*` with `params.command` |
| `GET /api/terminal/{launch}/stream`, `POST .../input`, `POST .../resize` | | the terminal, for `symphony attach` and an in-page terminal |
| `GET /api/tasks` | | `{tasks: [Task]}` |
| `POST /api/tasks` | `{room?, title, leader, permissionMode, lang?, members: [{account} \| {advisor: {name, dir}}, model, effort]}` (2 to 5 members) | the Task |
| `POST /api/tasks/{id}/end`, `DELETE /api/tasks/{id}` (a failed one) | | `{ok}` |
| `POST /api/tasks/{id}/say` | `{text}`, leading `@<member name>`s pick the members | `{ok, missed}` |
| `GET /api/tasks/{id}/chat` | | `{messages: [Message]}` |
| `GET /api/activity` | | `{items: [Activity]}` (the last 500) |
| `GET /api/ledger` | | `{rows: [LedgerRow]}` |
| `GET /api/themes` | | `{themes: [Theme], current, protocol}` |

## 6. The mod's side

With `Authorization: Bearer <account token>`:

- `POST /api/event`: an Event: `session`, `kind`, and the fields of that kind; every event may carry `cwd`, `launch`, `task`, which the bridge keeps. Kinds: `session.start` (`interactive`, `login`, `model`, `surface`, `engine`, `mod`, `repo`), `heartbeat` (every 15 s), `session.title`, `session.model` (`model`, `effort`, `from`?, `source`?), `session.cwd`, `session.surfaces`, `session.tools` (`toolCount`, `mcp`), `session.usage` (`usage`), `session.background` (`bgTasks`, `bgCrons`), `session.compact` (`trigger`), `session.notify` (`notifyType`, `title`, `text`), `session.end` (`reason`), `turn.start` (`turnId`, `text`, `agentId`?), `turn.complete` (`turnId`, `durationMs`, `isAborted`, `reason`, `refusal`?, `tokens`?, `agentId`?), `turn.error` (`text`, `details`), `tool.start` (`tool`, `toolUseId`, `summary`, `agentId`?), `tool.end` (+ `denied`, `isError`, `durationMs`), `tool.fail` (`toolUseId`, `text`, `denied`?), `agent.start` (`agentId`, `agentType`, `description`, `model`, `background`), `agent.end`, `todo.create` (`todo`, `subject`, `teammate`), `todo.done`, `command.run` (`command`, `args`), `message.send` (`to`, `text`, `origin`, `agentId`, `delivered`), `message.receive` (`text`, `origin`, `agentId`), `chat.user` (`text`, `origin`, `command`), `chat.delta` (`turnId`, `step`, `blocks: [{i, kind, text}]`), `chat.stop` (`turnId`, `step`, `tokens`?, `stopReason`?), `chat.sent` (`text`). Answers: 200 `{ok, account}`, 401 unknown token, 409 the account has another session online.
- `POST /api/ask` (`{session, kind, tool?, input?, reason?, questions?, agentId?}`) → `{id}`; `GET /api/wait/{id}` long-polls (20 s) → `{status, answer}`; `POST /api/cancel/{id}` withdraws.
- `GET /api/inbox?session=` long-polls for what the page sent: `{items: [{kind: "say", text} | {kind: "abort"}]}`; it counts as a heartbeat.

## 7. The theme contract

A theme is `web/themes/<name>/` with `theme.json` (§3 Theme, `name` the folder's) and `index.html` (`<html data-theme="<name>">`, which `i18n.js` reads to load the theme's own catalogs from `/t/<name>/locales/<lang>.json`, over the shared ones; a theme without catalogs gets two 404s and goes on). It uses `symphony.js` for the state and the calls, and the shared pieces for what every theme has to offer:

1. Every account (off duty included) and every online session, with its state and model; one click reaches a session's chat (`ui/chat.js`, `createChat`) and from it the asks.
2. The pending asks visible at a glance and reachable with one click from anywhere.
3. A new employee (`ui/launch.js`) and "Start work" for an off-duty one.
4. The running tasks and the task window (`ui/task.js`); a theme draws tasks its own way but opens the shared dialog and window. The bridge knows no rooms: `Task.room` is a hint for the theme that made it.
5. The message log and the activity log in some form.
6. `t()` for every string; its own catalog, when it has one, in all ten languages (`go test ./internal/i18n/` checks theme catalogs for parity).
7. `ui/ui.css` for the shared pieces' look, restyled through its variables.
8. For the tests: `<html data-theme>`, the shared pieces' ids (`#chat`, `#asks .card[data-id]`, `#launch`, `#task`, `#taskwin`), `window.__i18nMissing` empty; `TestThemesLoad` opens every theme and checks these.
