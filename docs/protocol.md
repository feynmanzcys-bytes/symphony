# The bridge's protocol

What the bridge gives the pages and takes from them, and what the mod reports to it. Protocol version **2** (the snapshot's `protocol`). Additions keep the number; a removed or renamed field raises it. A page that finds a number it doesn't know shows `error.protocol_mismatch` and goes on as far as it can.

History: 1 (v0.8.0) the first numbered one; 2 (v0.9.0) `Task.roomName` removed, `Task.room` became an optional hint.

The pages are themes of one office page (`web/themes/<name>/`), all on the same client library, `web/symphony.js`, and the same shared pieces, `web/ui/*.js`. Everything below is what those use; a theme may use the library and never touch HTTP.

## 1. Transport

- `GET /api/state`: the full snapshot (with the message log), JSON.
- `GET /api/stream`: Server-Sent Events, one JSON message per `data:` line, a `: keepalive` comment every 15 s. The first message is a full snapshot; every change in the office then pushes a message and a snapshot without the log. The stream starts with `retry: 1000`. A page that falls 1024 pushes behind (a frozen tab, a stalled connection) has its stream closed and its pending pushes dropped; its EventSource reconnects a second later and starts again from a full snapshot. A page hidden for 10 s closes its stream and fetches `/api/state` every 30 s; shown again it reopens it, and the first message (a full snapshot) resyncs it. `symphony.js` reopens a stream the browser gave up after 3 s; when the bridge then answers `/api/state` with 401 `page_key` (it made a new key, so the page is signed out), it emits `pagekey` (true; false once a request is taken again), sets `sym.keyLost` and tries every 10 s instead; a call without an answer in 15 s answers status 0, code `unreachable`.
- This machine only: the bridge listens on a loopback address (`-addr`'s host is `127.0.0.1`, `localhost` or `::1`; any other stops it) and answers 403 to a connection from any other address.
- Same origin only: the bridge refuses (403) a `Host` that isn't this machine (DNS rebinding) and any write (`POST`, `DELETE`) whose `Origin` isn't its own. The mod sends no `Origin`.
- Every answer carries `Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self'; font-src 'self'; media-src 'self' data: blob:; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'`, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff` and `Referrer-Policy: no-referrer`.
- `GET /` serves the theme the browser picked (`?theme=<name>`, remembered in the `symphony.theme` cookie; else the cookie's; else `office`), `GET /t/<name>/` picks a theme and serves it, `GET /t/<name>/<file>` its own files, `GET /api/themes` lists them (`{themes: [Theme], current, protocol}`). The shared files are at the root: `/symphony.js`, `/i18n.js`, `/asks.js`, `/ui/<file>`, `/locales/<lang>.json`. `/approve` is the simple approval page.
- Errors: `{error: <English>, code, params}`; the page translates `error.<code>` with `params`. Statuses: 400 a bad request, 401 no page key (`page_key`) or the mod's token is unknown (`token_invalid`), 403 another account's ask, a cross-site write, a foreign `Host` or another machine, 404 unknown, 409 a conflict (an account busy: online or a start from the page on its way, `account_busy`; the process of its last start still running with its session off duty, `launch_still_running` with `params.command`, the command that shows its terminal; an ask settled, a name taken, a session offline, a task's member busy), 412 git missing, 500 a git or terminal failure, or a data file that couldn't be written (`data_write_failed`).

### 1.1 The page key

Every request except the mod's (§6) must show the bridge's page key: 64 lowercase hex digits, kept in `key` in the bridge's data folder (mode 0600), made on the first start and kept across restarts.

- As the cookie `symphony_key_<port>` (the port of the request's `Host`; `HttpOnly`, `SameSite=Lax`, `Path=/`, `Max-Age` 400 days), or as the header `X-Symphony-Key: <key>` (for programs: `symphony attach`, the demo, curl).
- A `GET` or `HEAD` with `?key=<key>` sets the cookie and answers 303 to the same path and query without `key` (`?theme=`, `?lang=` and the like are kept). A wrong key counts as no key. The symphony program opens the office this way (through `open.html`, which only the user can read, so the key stays off command lines), and the bridge's log prints the address with `?key=`.
- `/api/*` (the mod's endpoints aside) answers 403 to a request whose `Sec-Fetch-Site` is `cross-site` or `same-site`: a link, redirect or fetch from another site or another port of this machine would bring the cookie along (SameSite=Lax lets it ride on a top-level GET). The pages send `same-origin`, an address typed in `none`, and programs none at all; the sign-in (`/?key=`) and the pages themselves stay reachable from anywhere. `/api/dirs`, which reads any folder it is given, also refuses `Sec-Fetch-Mode: navigate` (a link opened from anywhere, an address typed in): the pages fetch it.
- Without a valid key, `/api/*` answers 401 `{error, code: "page_key"}` (the page translates `error.page_key`); a `GET` or `HEAD` of `/`, `/t/<theme>/` or `/approve` answers 403 with an HTML page in the bridge's language that says how to get in (run symphony again, or open the `?key=` address from the log); any other file answers 401.
- The mod's endpoints don't take the key and keep their token check: `POST /api/event`, `POST /api/ask`, `GET /api/wait/{id}`, `POST /api/cancel/{id}`, `GET /api/inbox`, `POST /api/hello`.
- A page served by the bridge has the cookie already: `symphony.js` and the shared pieces need do nothing.
- A program that holds the key asks the bridge to prove it has the key before it shows it, so a program that took the bridge's address never gets it: `POST /api/hello` with `{"id": <the lowercase hex SHA-256 of the key>, "nonce": <16 to 128 hex digits>}` and no key; the bridge with that key answers 200 `{"proof": <lowercase hex HMAC-SHA256, keyed with the key's 64 characters, of "symphony-page:" + the nonce>}`, anything else is not that bridge (§6 has the same call for a token). The symphony program does it before it treats what answers at its address as its own bridge, and so do `symphony attach` and the demo.

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
| `settled` | `id`, `status` (`done` or `cancelled`) | an ask was answered or withdrawn (by the mod, or by the bridge when its session goes off duty) |
| `launch` | `id` | a launch was made, came online, failed or ended (its process exited) |
| `task` | `id` | a task's status changed |

Catch-up: every snapshot carries `messageTail`, the latest message's id; a page whose log lacks it fell behind and fetches `/api/state` again. `symphony.js` does this (the office before v0.9.0 never did: its check always passed). A full snapshot after the first (the stream reconnected) is a resync: `symphony.js` emits `resync`, and what a page fetched on its own (the chat log, the activity) is fetched again.

## 3. The objects

Times are RFC 3339 strings; ids are opaque strings; fields marked optional are left out when empty.

**Snapshot**: `type` (`snapshot`), `protocol`, `started` (when the bridge started), `accounts` [AccountView], `sessions` [Session], `asks` [Ask] (pending only), `messages` [Message] or null, `launches` [Launch], `tasks` [Task] (running, ending and failed), `messageTail`, `ledger` [LedgerRow] (today's rows).

**AccountView**: `id`, `name`, `seat` (a fixed desk, from 0; -1 for an advisor), `advisor`?, `session`? (the online desk session's id), `last`? (LaunchRequest: how it was last started from the page), `dir`? (the folder its last desk session worked in).

**Session**: `id`, `account`, `login`? (only what Claude Code's `$.session.identity()` tells the mod: usually empty), `title` (the session name in Claude), `cwd`, `state` (`idle`, `thinking`, `tool`, `waiting`, `ended`), `tool`? (the running tool), `prompt`? (the turn's prompt, cut to 200), `agents` {id: Agent} (the running subagents and the newest 8 finished ones), `agentsTotal`? (every subagent the session had, those dropped from `agents` too), `tools` (tool calls so far), `model`?, `effort`?, `launch`? (the launch id of a page-started session), `attach`? (the command that shows its terminal), `task`? (the task it works on; empty for a desk session), `started`, `updated`, `stateSince`; what the mod reports besides (all optional, an older mod sends none): `surface` (`terminal`, `desktop`, `vscode`, `mobile`), `surfaces` [string], `engine` {version, base}, `mod` (its version), `repo` {root, remote?, name, internal} (the mod doesn't send `remote`, as its URL may carry credentials; an older mod's has any `user[:password]@` removed by the bridge), `usage` (Usage), `turns`, `compactions`, `todos` [Todo], `bgTasks`, `bgCrons`, `toolCount`, `mcp` [server names], `toolMs`, `lastError`, `endReason` (the mod's reason, or the bridge's: `lost` when its heartbeat stopped, `exited` when its page-started process exited; cleared when it comes back), `notify` {type, title, text, time}.

**Agent** (a subagent): `id`, `state` (as a session's, plus `done`), `tool`?, `type`?, `description`?, `model`?, `background`?, `started`?, `ended`? (when agent.end came).

**Usage**: `context`? {tokens, window, percent}, `rateLimits`? [{kind, percentUsed, resetsAt}], `cost`? {usd} (the session's running total), `totals`? {model: Tokens} (the responses' tokens summed by model), `updated`. **Tokens**: `input`, `output`, `cacheRead`, `cacheWrite`, `model`?.

**Todo** (the agent team's task list): `id`, `subject`, `teammate`?, `created`.

**Ask**: `id`, `session`, `account`, `kind` (`permission` or `question`), `tool`?, `input`? (the tool's input; a string over 4096 characters keeps its first 3072 and last 1024 with ` … [+N] … ` between, `inputTruncated: true`; `GET /api/asks/{id}/input` has it whole), `reason`?, `questions`? (AskUserQuestion's), `agentId`?, `status` (`pending`, `done`, `cancelled`), `answer`?, `created`.

**Message** (between sessions, or from the page to a task's members): `id`, `from`? (the sender's session id), `fromName`, `to`?, `toName`, `toNames`? (a task chat's mentioned members), `missed`? (of those, the ones offline), `text`, `origin`? (`page` for one the page sent; else the engine's origin kind), `agentId`?, `task`? (the task it belongs to), `delivered`, `received`, `time`.

**Launch**: `id`, `attach`, `name`, `dir`, `account`, `model`?, `effort`?, `command`, `status` (`starting`, `online`, `failed`, `ended`: it was online and its process has exited), `error`?/`errorCode`?/`errorParams`?, `session`?, `time`, `task`?.

**Task**: `id` (`T` + a number), `room`? (an integer hint: the small room the task was started from in the theme that made it; a theme seats tasks itself and may honor it), `title`, `leader`? (the lead's account id; absent for a flat task), `permissionMode`, `lang`?, `members` [Member], `repos` [TaskRepo], `status` (`running`, `ending`, `failed`; ended ones leave the snapshot), `error`?/`errorCode`?/`errorParams`?, `created`, `ended`?. **Member**: `account`, `name` (`<account name>-<task id>`), `advisor`?, `dir`, `branch`, `worktree`, `model`?, `effort`?, `launch`?, `session`? (once it reported in), `kept`? (where what git ignored in its worktree went when the task ended: `<folder>/.symphony/kept/<slug>-<task id>`; empty when there was nothing; set on a failed task too, for what was moved before the failure). **TaskRepo**: `dir`, `inited`, `base`, `root`.

**ChatItem**: `id`, `seq`, `kind` (`user`, `thinking`, `text`, `tool`, `command`, `message`, `ask`, `notice`), `text`?, `done`, `tool`?, `summary`?, `status`? (tool: `ok`, `error`, `denied`; user: `queued`, `sent`, `dropped` (`summary` why it didn't enter: a hook's or the engine's reason); ask: `pending`, `allow`, `deny`, `answered`, `cancelled`), `agentId`?, `origin`? (user: `composer`, `page`, `task`, or the engine's kind), `from`?/`to`? (message), `ask`?/`askKind`?, `secs`? (thinking), `ms`? (tool), `time`. A notice's `text` is a code the page translates (`chat.notice.<code>`: `ended`, `aborted`, `refused`, `error`, `exited` (a page-started process that exited badly: the exit status and the last lines of its screen), `compacted`, `modelSwitched`, `notify`) and `summary` a detail shown after it; a tool row's `text` is the reason it failed or was denied.

**LedgerRow**: `day` (YYYY-MM-DD, local), `account`, `usd`, `tokens` (Tokens), `turns`, `tools`. **Activity**: `time`, `kind` (`start`, `end`, `tool`, `ask`, `settled`, `message`, `error`, `launch`, `launchFailed`), `session`?, `account`?, `name`?, `task`?, `text`?.

**Theme**: `name`, `title` {lang: text}, `description`? {lang: text}, `version`?, `since`? (the protocol version it was written for), `icon`?.

## 4. What a page derives

- A session's display state: `waiting` while it has pending asks (the bridge sets it), else `state`; off duty is `ended`, and a session is kept 10 minutes after it ended (then it leaves the snapshot); 45 seconds without a heartbeat ends it, and a page-started session whose process exits ends at once; a session that ends has its pending asks withdrawn (cancelled).
- `stateSince` says how long a session has been in its state (the pixel office's idle life counts from it; a reload changes nothing).
- An employee away in a task: its account has a member session online in a task (`Task.members[].session` with a session not `ended`); its desk session is separate (`AccountView.session`).
- A task's room is the page's business (§3, `room`).
- The ledger's "today" is the bridge's local day; `/api/ledger` has every day kept (90).

## 5. The calls a page makes

Every call shows the page key (§1.1).

| call | body | answer |
| --- | --- | --- |
| `POST /api/answer/{ask}` | `{decision: "allow"\|"deny", reason?}` or `{answers: {question: answer}}` | `{ok}` |
| `POST /api/say/{session}` | `{text}` | the queued ChatItem; `session_offline`, `say_empty` |
| `POST /api/abort/{session}` | | `{ok}`; `session_offline` |
| `GET /api/chat/{session}` | | `{items: [ChatItem]}` (the last 400) |
| `GET /api/asks/{id}/input` | | `{input}` (a pending ask's whole input); `ask_unknown` (404) once settled |
| `POST /api/accounts` | `{name}` | the Account with its `token` (for a session started by hand); `data_write_failed` (500) when accounts.json can't be written |
| `DELETE /api/accounts/{id}` | | `{ok}`; `account_busy` while it has a session or is in a task; `data_write_failed` (500) when accounts.json can't be written |
| `GET /api/launch/options` | | `{enabled, mod, terminal, trustHint, models, efforts, permissionModes, resumes, home, recentDirs}` |
| `GET /api/dirs?path=` | | `{path, parent, dirs: [{name, path}], roots}` |
| `POST /api/launch` | LaunchRequest: `{account?, name?, dir, model, effort, permissionMode, resume, lang?}` | the Launch; the page follows its `status` in the snapshot; 409 `account_busy` while the account is online or a start for it is on its way, `launch_still_running` while the process of its last start still runs |
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
| `POST /api/shutdown` | | 202 `{ok}`, then the bridge stops and its sessions end (`symphony -stop` sends it after the key's proof, §1.1) |

## 6. The mod's side

These take no page key. Before its first request with the token, and again after a request that got no answer, the mod checks that the bridge knows the token, without sending it:

- `POST /api/hello` with `{"id": <the lowercase hex SHA-256 of the token string>, "nonce": <16 to 128 hex digits, either case>}` (at most 4 KiB; no `Authorization`). The bridge answers 200 `{"proof": <lowercase hex HMAC-SHA256, the key the token string's bytes, the message "symphony-hello:" + the nonce exactly as sent>}`. An unknown id, a badly formed id or nonce, or a body that isn't one answers 401 `{code: "token_invalid"}`. The bridge compares the id with every account's in constant time. The same call with the page key's hash as `id` is the symphony program's (§1.1), its proof over `"symphony-page:"` + nonce, so neither proof stands for the other.
- The mod compares the proof in either case. A right proof: it goes on. 401: it shows `badToken` and sends nothing more. Any other answer (a wrong or missing proof, not JSON, a 404 from a bridge older than the mod, a 500): it shows `notBridge` and sends nothing with the token in that session. No answer: unreachable; at a session's start the mod starts a bridge on this machine (`symphony -background`) and checks again, unless what is there took the check and didn't answer within 5 s (then no bridge could start there). A request that gets no answer in time leaves the bridge alone for 10 s: the mod sends nothing in that time. Concurrent requests share one check; an HTTP error on a later request (409, 500) keeps it while the inbox poll runs; a session without one (refused as busy) checks again before its next report.
- `SYMPHONY_BRIDGE` must be `http://127.0.0.1:<port>`, `http://localhost:<port>` or `http://[::1]:<port>` (a trailing slash is ignored) or an `https://` address; with anything else the mod shows `insecure` and never contacts the bridge.

With `Authorization: Bearer <account token>`:

- `POST /api/event`: an Event: `session`, `kind`, and the fields of that kind; every event may carry `cwd`, `launch`, `task`, which the bridge keeps. Kinds: `session.start` (`interactive`, `login`, `model`, `surface`, `engine`, `mod`, `repo` {root, name, internal}), `heartbeat` (every 15 s; carries `login`, `model`, `effort`, `surface`, `engine`, `mod`, `repo` as `session.start` does, without `interactive`; the bridge keeps them and pushes only when they change), `session.title`, `session.model` (`model`, `effort`, `from`?, `source`?), `session.cwd`, `session.surfaces`, `session.tools` (`toolCount`, `mcp`), `session.usage` (`usage`), `session.background` (`bgTasks`, `bgCrons`), `session.compact` (`trigger`), `session.notify` (`notifyType`, `title`, `text`), `session.end` (`reason`), `turn.start` (`turnId`, `text`, `agentId`?), `turn.complete` (`turnId`, `durationMs`, `isAborted`, `reason`, `refusal`?, `tokens`?, `agentId`?), `turn.error` (`text`, `details`), `tool.start` (`tool`, `toolUseId`, `summary`, `agentId`?), `tool.end` (+ `denied`, `isError`, `durationMs`), `tool.fail` (`toolUseId`, `text`, `denied`?), `agent.start` (`agentId`, `agentType`, `description`, `model`, `background`), `agent.end`, `todo.create` (`todo`, `subject`, `teammate`), `todo.done`, `command.run` (`command`, `args`), `message.send` (`to`, `text`, `origin`, `agentId`, `delivered`), `message.receive` (`text`, `origin`, `agentId`), `chat.user` (`text`, `origin`, `command`), `chat.delta` (`turnId`, `step`, `blocks: [{i, kind, text}]`), `chat.stop` (`turnId`, `step`, `tokens`?, `stopReason`?), `chat.sent` (`text`), `chat.dropped` (`text`, `reason`). After /clear the mod sends `session.end` (`reason: clear`) for the old id, then `session.start` for the new one with the same `launch` and `task`. From mod 1.1.1 a session's first event is its `session.start` (the mod holds the others until it went out); a running session whose events named no `task` joins the task a later event names, when its account is a member of that running task and has no other session in it. Answers: 200 `{ok, account, fresh?}` (`fresh: true`: the event, other than `session.start`, made the bridge create the session's record; the mod then reports `session.tools`, `session.surfaces`, `session.title` and `session.background` again), 401 unknown token, 409 the account has another session online, 410 `session_gone` for an event naming the launch of a page-started session whose process has ended (a report that was on its way).
- `POST /api/ask` (`{session, kind, tool?, input?, reason?, questions?, agentId?}`) → `{id}`; `GET /api/wait/{id}` long-polls (20 s) → `{status, answer}`; `POST /api/cancel/{id}` withdraws. The mod gives each wait 25 s; it withdraws the ask when the terminal interrupts, a wait fails or isn't answered in time, or the ask was taken only after its 10 s timeout.
- `GET /api/inbox?session=&ack=` long-polls for what the page sent: `{items: [{id, kind: "say", text} | {id, kind: "abort"}]}`; it counts as a heartbeat. Ids count up per session. `ack` is the id of the last item the mod did (its previous poll's last): that one and the ones before it are dropped, the rest are handed out again, so a poll cut off on its way back loses nothing. Without `ack` (a mod before 1.1.2) each item is handed out once.

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
9. Only the bridge's own files, as the Content Security Policy (§1) allows: scripts in files (no inline `<script>` blocks, no inline event handlers such as `onclick="..."`, no `javascript:` addresses), nothing from other origins (scripts, styles, fonts, images, connections). Inline styles are fine; images may be `data:` or `blob:`. `TestThemesLoad` records every policy violation as a page error.
10. The connection banner (`ui/banner.js`, `connectionBanner(sym)`, `#conn`) while the bridge can't be reached; rows and tiles one clicks are redrawn only when they changed and not while a pointer is down (`setHTML`, `deferWhilePressed` in `ui/common.js`); the activity log fetched only while it shows.
