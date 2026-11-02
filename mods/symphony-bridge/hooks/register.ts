import type { EngineInterface, HttpResponse, Register } from 'claude-code'
import { type Plan, download, targetOf } from './download'
import { type Lang, matchLang, message } from './messages'
import { type Rule, ruleAllows, sameRule, suggestRule } from './rules'

// Environment variables:
//   SYMPHONY_TOKEN    the office account's token: one account serves one session at a time and sets its identity and seat
//   SYMPHONY_BRIDGE   the bridge's address, by default http://127.0.0.1:7788; only http://127.0.0.1, http://localhost
//                     or http://[::1] with a port, or any https:// address (anything else: the bridge is never contacted)
//   SYMPHONY_LAUNCH   the launch id the bridge sets when the page starts a session
//   SYMPHONY_TASK     the collaborative task's id, when the bridge starts the session as a task member (in a worktree of its own)
//   SYMPHONY_LANG     the language of the mod's messages (the bridge passes the page's language); else LC_ALL / LC_MESSAGES / LANG
// The mod is usually installed at user scope, so every Claude Code session loads it, but only sessions started
// with SYMPHONY_TOKEN belong to the office. In any other session it does nothing: it doesn't contact the bridge,
// read login details, show messages or touch approvals.
// The token goes only to a bridge that has shown it knows the token: before the first request that carries it, the
// mod sends POST /api/hello with the token's SHA-256 (hex) as its id and a fresh random nonce, without the token, and
// the answer must be { proof: HMAC-SHA256 of "symphony-hello:" + nonce, keyed with the token (hex) }. A 401 means the
// bridge doesn't know the token; a wrong proof or any other answer means what answers there isn't a symphony bridge
// this session can trust (or one older than the mod), and nothing that carries the token is sent in this session.
// After a request that gets no answer (a network failure, a timeout) the check is redone before the next request
// that carries the token: the bridge may have stopped and something else taken its address.
// The login (the Claude account's email) is what $.session.identity() answers; it answers only a mod the
// organization's managed settings install, so elsewhere no login is reported.
// When a bridge on this machine doesn't answer as the session starts, the mod starts it with the symphony program
// (symphony -background, if it's on PATH; SYMPHONY_DATA, if set, tells it where the accounts are); when that fails,
// the message says why, as symphony does.
// When the bridge is unreachable, rejects the token, fails the check or the account is busy, everything falls back
// to Claude Code's own terminal interaction; the session never hangs.
// Reports (events, the heartbeat) go out one after another in the background: a hook never waits for the bridge,
// and a report that gets no answer in time is given up. A bridge that takes a request and doesn't answer in time is
// left alone for a while (everything meant for it in that time is dropped at once, so nothing piles up behind it),
// and the queue of reports is bounded. Only asks wait, for the page's answer, each long poll at most WAIT_TIMEOUT,
// and the terminal's Esc stops the wait at once and withdraws the card.
// After /clear (or a /resume inside the session) the session goes on under a new id: the mod reports the old one
// ended and the new one started, as at a start.
// The heartbeat carries what the start told (model, effort, engine, mod, repo, surface, login), so a bridge that
// restarted, or that first meets the session through its heartbeat, learns it; when the bridge says it didn't know
// the session (fresh), the mod tells it the tools, the surfaces, the name and the background work again.
// The employee window in the page shows the session as a chat: the mod streams the model's thinking and text
// (turn.step, in batches), reports the prompts that enter (prompt.submit) and the tool calls, and polls the bridge
// for what the page sends (its inbox): text to submit as the person's prompt, or a stop of the running turn.
// Besides the chat, the mod reports everything the engine tells it about the session, for the pages to show as they
// like: the engine's version and the repository at the start, the context window's fill, the rate limits and the cost
// (session.measure, coalesced), each response's tokens (the stop chunk of turn.step), why a turn ended, subagents by
// name, the agent team's tasks, notifications, compactions, model switches, folder changes, slash commands, tool
// durations and failures, the surfaces attached, the MCP servers connected and why the session ended.
const DEFAULT_BRIDGE = 'http://127.0.0.1:7788'
// how long a report, or putting an ask on the bridge, may take; a hanging bridge holds nothing up for longer
const REPORT_TIMEOUT = 5_000
const ASK_TIMEOUT = 10_000
// how long one wait for the page's answer may take: the bridge answers each long poll within 20 seconds (pending when
// nothing was decided), so no answer by then means it is stuck, and the terminal asks instead
const WAIT_TIMEOUT = 25_000
// how long a bridge that took a request and didn't answer in time is left alone
const STUCK_PAUSE = 10_000
// how many reports may wait to go out; beyond it the oldest that can be lost go first (OUTBOX_LOSSY), then the oldest
const OUTBOX_CAP = 500
const OUTBOX_LOSSY = new Set(['heartbeat', 'session.usage', 'chat.delta'])
// how often the pieces of a streamed response go to the bridge, at most
const STREAM_FLUSH = 150
// how often the usage figures go to the bridge, at most (the context fill moves on every response)
const USAGE_FLUSH = 2_000
// after a failed inbox poll, how long before the next
const INBOX_RETRY = 3_000

type Answer =
  | { decision: 'allow' | 'deny'; reason?: string; remember?: boolean }
  | { answers: Record<string, string> }

// insecure: SYMPHONY_BRIDGE is neither on this machine nor https, so the bridge is never contacted
type Identity = { bridge: string; token: string; headers: Record<string, string>; login?: string; lang: Lang; insecure: boolean }

// what the start tells the bridge of the session besides the event (the heartbeat carries it too)
type Known = {
  login?: string
  model: string
  surface?: string
  engine?: { version: string; base?: string }
  mod?: string
  repo?: { root: string; name?: string; internal: boolean }
}

// undefined: the environment hasn't been read yet; null: no SYMPHONY_TOKEN, not an office session
let identity: Identity | null | undefined
// the token came from the session's state, not the environment: this module was loaded into a session an earlier
// module of this process started (/reload-plugins, a worker respawn)
let revived = false
let sessionId: string | undefined
let transcriptPath: string | undefined
// every event carries the folder, the launch id and the task id, so a restarted bridge recognizes the session right away
let sticky: { cwd?: string; launch?: string; task?: string } = {}
// what session.start learned of the session, for a restart after /clear and for the heartbeat
let known: Known | undefined
let interactive = false
// withdraws an ask through the session.start hook's $, from a timer of its own: when the terminal interrupts the
// ask's hook, the engine cancels every request that dispatch starts afterwards ("user-cancel"), through any $; one
// started from the session's own timer goes out (checked with Claude Code 2.1.316)
let withdrawLater: ((card: string) => void) | undefined
// the model and effort last reported to the bridge; reported again only when they change
let reported = { model: '', effort: '' }
// the session's name last reported, told again to a bridge that didn't know the session
let title: string | undefined
let heartbeat: { cancel: () => void } | undefined
// the bridge doesn't know the token (401), what answers isn't a bridge this session can trust, or the address is
// insecure: nothing more goes to it in this session
let refused = false
// the bridge's check (POST /api/hello), shared by the requests that wait for it; dropped after a request that got no
// answer, so the next one checks again
let checked: Promise<boolean> | undefined
// until when the bridge is left alone: it took a request and didn't answer in time
let stuckUntil = 0
// reports waiting to go out, in order, one at a time; sending while they go
type Report = { kind: string; send: () => Promise<unknown> }
const outbox: Report[] = []
let sending: Promise<void> | undefined
// settled once session.start has told the bridge, or tried to: the engine runs its own start hooks inside session.start
// (SessionStart reports the session's name), and a report that went out first would seat the session before it named
// its launch and task: the bridge took a task member's session for a desk session
let startTold: Promise<void> | undefined
// the person was told once that a request waits in the office page
let toldWaiting = false
// calls the web page denied. A denied tool reaches tool.call as an error result, not { deny }, so we remember them.
const deniedByWeb = new Set<string>()
// the main loop's running turn, for a stop from the page
let mainTurn: string | undefined
// the session ended: the inbox poll stops, and nothing more is reported for it (a /clear starts the next one)
let ended = false
let polling = false
// which poll loop is the session's: a new session (after /clear) starts its own at once, while the old one's long
// poll may still be out; that loop ends when its poll comes back
let pollRound = 0
// the latest usage figures waiting to go out, and the timer that sends them
let usagePending: object | undefined
let usageTimer: { cancel: () => void } | undefined
// the background work last reported (classic.Stop comes at every turn's end; only a change is worth a report)
let background = { tasks: -1, crons: -1 }

// the Claude login's email, when $.session.identity() tells it: { deny } (a mod the organization didn't install) or
// a session without an email means no login
async function loginEmail($: EngineInterface) {
  try {
    const email = (await $.session.identity()).email
    return typeof email === 'string' && email ? email : undefined
  } catch {
    return undefined
  }
}

// the mod's own version, from its manifest, for the pages to show (the bridge only keeps it)
async function modVersion($: EngineInterface) {
  try {
    const manifest = JSON.parse(await $.fs.read(`${$.plugin.root}/.claude-plugin/plugin.json`))
    return typeof manifest?.version === 'string' ? (manifest.version as string) : undefined
  } catch {
    return undefined
  }
}

// sessionLang is SYMPHONY_LANG, else the POSIX locale (the first of LC_ALL, LC_MESSAGES and LANG that is set), else English.
async function sessionLang($: EngineInterface): Promise<Lang> {
  const own = matchLang(await $.env.get('SYMPHONY_LANG'))
  if (own) return own
  const posix = (await $.env.get('LC_ALL')) || (await $.env.get('LC_MESSAGES')) || (await $.env.get('LANG'))
  return matchLang(posix) ?? 'en'
}

// the session's own record of its token, held by the engine for the session (a reloaded module finds it there): the
// token leaves the environment once the session started (hideToken), so the tools, scripts and servers it starts don't
// inherit it
const OFFICE = { plugin: 'symphony-bridge', key: 'office' } as const
// the rules the page gave for the rest of the session (rules.ts), held the same way through a reload
const RULES = { plugin: 'symphony-bridge', key: 'rules' } as const
let rules: Rule[] | undefined // undefined: not read from the session's state yet

async function rulesOf($: EngineInterface): Promise<Rule[]> {
  if (rules) return rules
  const kept = (await safe($.state.get(RULES)))?.value
  return (rules = Array.isArray(kept) ? (kept as Rule[]) : [])
}

// keepRule adds what the page allowed for the rest of the session, tells the bridge (the chat says so) and keeps it in
// the session's state
async function keepRule($: EngineInterface, rule: Rule) {
  const have = await rulesOf($)
  if (have.some(r => sameRule(r, rule))) return
  rules = [...have, rule]
  report($, 'session.rule', { rule })
  await safe($.state.set(RULES, rules))
}

async function ident($: EngineInterface) {
  if (identity !== undefined) return identity
  let token = await $.env.get('SYMPHONY_TOKEN')
  let bridgeVar = await $.env.get('SYMPHONY_BRIDGE')
  if (!token) {
    const kept = (await safe($.state.get(OFFICE)))?.value
    if (kept?.token) {
      token = kept.token
      bridgeVar ||= kept.bridge
      revived = true
    }
  }
  if (!token) return (identity = null)
  const bridge = (bridgeVar || DEFAULT_BRIDGE).replace(/\/+$/, '')
  identity = {
    bridge,
    token,
    headers: { 'content-type': 'application/json', authorization: `Bearer ${token}` },
    login: await loginEmail($),
    lang: await sessionLang($),
    // the token may cross the network only encrypted: plain http stays on this machine
    insecure: !LOCAL_BRIDGE.test(bridge) && !/^https:\/\/./i.test(bridge),
  }
  return identity
}

// hideToken takes SYMPHONY_TOKEN out of the session's environment, so what the session starts from now on (Bash
// commands, npm scripts, MCP servers) doesn't inherit it: they could call the mod's endpoints with it (empty the
// inbox, make up events or ask cards). The token stays in the module and in the session's state, which a module
// reloaded by /reload-plugins reads. When the state can't be written, the token stays where it is: a reload would
// lose the session otherwise.
async function hideToken($: EngineInterface, id: Identity) {
  try {
    const kept = await $.state.set(OFFICE, { token: id.token, bridge: id.bridge, session: await sid($) })
    if (!kept.isSet) return
    await $.env.set('SYMPHONY_TOKEN', undefined)
  } catch {
    // an engine without session state or env.set: the environment keeps it, as before
  }
}

// keepOffice changes what the session's state keeps besides the token (the session it is kept for, the office text)
async function keepOffice($: EngineInterface, change: { session?: string; context?: string }) {
  const kept = (await safe($.state.get(OFFICE)))?.value
  if (kept?.token) await safe($.state.set(OFFICE, { ...kept, ...change }))
}

async function sid($: EngineInterface) {
  return (sessionId ??= await $.session.id())
}

// office says whether this session reports to the bridge at all (a token, and not refused)
async function office($: EngineInterface) {
  return !refused && !!(await ident($))
}

// the value of p, or undefined when it fails (an engine op this build lacks, a folder without git)
async function safe<T>(p: Promise<T>): Promise<T | undefined> {
  try {
    return await p
  } catch {
    return undefined
  }
}

const utf8 = (s: string) => new TextEncoder().encode(s)
const hex = (bytes: Uint8Array) => [...bytes].map(b => b.toString(16).padStart(2, '0')).join('')
const sha256 = async (data: Uint8Array) => new Uint8Array(await crypto.subtle.digest('SHA-256', data))

// HMAC-SHA256 (RFC 2104), built on SHA-256 as the engine's crypto offers no HMAC of its own
export async function hmacSha256(key: Uint8Array, data: Uint8Array): Promise<Uint8Array> {
  const block = new Uint8Array(64)
  block.set(key.length > 64 ? await sha256(key) : key)
  const inner = new Uint8Array(64 + data.length)
  const outer = new Uint8Array(64 + 32)
  for (let i = 0; i < 64; i++) {
    inner[i] = block[i]! ^ 0x36
    outer[i] = block[i]! ^ 0x5c
  }
  inner.set(data, 64)
  outer.set(await sha256(inner), 64)
  return sha256(outer)
}

// what answers at the bridge's address refused the token or isn't a bridge this session can trust: nothing more goes
// to it in this session, and the person learns why once
function shut($: EngineInterface, id: Identity, key: 'badToken' | 'notBridge') {
  refused = true
  heartbeat?.cancel()
  $.ui.toast(message(id.lang, key, { bridge: id.bridge }))
}

// The bridge's check, before the token leaves the session: what answers at the bridge's address gets the token's
// SHA-256 and a fresh nonce, never the token, and has to answer with the nonce's HMAC keyed with the token. True once it
// did; false when nothing answered (checked again before the next request; one that took the request and didn't answer
// in time is left alone for a while) or the check failed (shut).
async function hello($: EngineInterface, id: Identity): Promise<boolean> {
  const nonce = hex(crypto.getRandomValues(new Uint8Array(16)))
  const body = JSON.stringify({ id: hex(await sha256(utf8(id.token))), nonce })
  const sent = $.http.fetch(`${id.bridge}/api/hello`, { method: 'POST', headers: { 'content-type': 'application/json' }, body })
  const { value: res, late } = await within($, REPORT_TIMEOUT, sent)
  if (late) await stuck($, sent)
  if (!res) return false
  if (res.status === 401) {
    shut($, id, 'badToken')
    return false
  }
  let proof: unknown
  try {
    proof = JSON.parse(res.text)?.proof
  } catch {
    // not JSON: not a bridge
  }
  const expected = hex(await hmacSha256(utf8(id.token), utf8(`symphony-hello:${nonce}`)))
  if (res.status === 200 && typeof proof === 'string' && proof.toLowerCase() === expected) return true
  shut($, id, 'notBridge') // a wrong proof, a bridge older than the mod (404) or something else entirely
  return false
}

// The bridge took a request and didn't answer in time (stopped with Ctrl-Z, say, or locked up): it is left alone for
// STUCK_PAUSE, so the reports behind it are dropped at once instead of each waiting out its own timeout, and an ask
// goes to the terminal at once. The first request after the pause tries it again, and so does the next one once the
// late request gets its answer after all.
async function stuck($: EngineInterface, sent: Promise<unknown>) {
  stuckUntil = (await $.clock.now()) + STUCK_PAUSE
  sent.then(
    () => {
      stuckUntil = 0
    },
    () => {},
  )
}

async function isStuck($: EngineInterface) {
  return stuckUntil > 0 && (await $.clock.now()) < stuckUntil
}

// A request to the bridge with the token, only once the bridge has passed its check, and only for office sessions.
// Undefined when it isn't sent or gets no answer (within ms, when given); no answer drops the check, so the next
// request checks again (the bridge may have stopped and something else taken its address). late gets an answer that
// came after ms (the request can't be stopped, only no longer waited for).
async function call(
  $: EngineInterface,
  path: string,
  body?: unknown,
  ms?: number,
  late?: (res: HttpResponse) => void,
): Promise<HttpResponse | undefined> {
  const id = await ident($)
  // a withdrawal goes out even while the bridge is left alone: a card nobody waits on would stay on the page
  if (!id || id.insecure || refused || (!path.startsWith('/api/cancel/') && (await isStuck($)))) return undefined
  const check = (checked ??= hello($, id))
  let res: HttpResponse | undefined
  if (await check.catch(() => false)) {
    if (refused) return undefined
    const sent = $.http.fetch(`${id.bridge}${path}`, {
      method: body === undefined ? 'GET' : 'POST',
      headers: id.headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    if (ms === undefined) res = await sent.catch(() => undefined)
    else {
      const got = await within($, ms, sent)
      res = got.value
      if (got.late) {
        await stuck($, sent)
        if (late) sent.then(late, () => {})
      }
    }
  }
  if (!res && checked === check) checked = undefined
  return res
}

// p's value; late once ms have passed first (the request itself isn't stopped: the HTTP API has no way to, so it is
// just not waited for); neither when p fails. p's outcome is taken before the timer starts: when the timer can't (a
// module the engine unloaded, its code still running), p's failure is still handled and the answer is neither.
function within<T>($: EngineInterface, ms: number, p: Promise<T>): Promise<{ value?: T; late?: true }> {
  return new Promise(resolve => {
    let timer: { cancel: () => void } | undefined
    let done = false
    const settle = (r: { value?: T; late?: true }) => {
      if (done) return
      done = true
      timer?.cancel()
      resolve(r)
    }
    p.then(
      value => settle({ value }),
      () => settle({}),
    )
    try {
      timer = $.clock.after(ms, () => settle({ late: true }))
      if (done) timer.cancel()
    } catch {
      settle({})
    }
  })
}

// a bridge on this machine, the only one plain http may reach and one the mod may start itself: http://127.0.0.1:7788 and the like
const LOCAL_BRIDGE = /^http:\/\/(127\.0\.0\.1|localhost|\[::1\]):(\d+)\/?$/

// Nothing answers at the bridge's address. When it's on this machine, start the bridge with the symphony program, if
// it's on PATH: -background detaches it, so it keeps running after this session, and returns once it answers (a
// session starting it at the same moment finds it running and starts nothing). When symphony ran and failed, reason
// is what it said last (the log line of its fatal error, without the time or the page key), else its exit status.
async function startBridge(
  $: EngineInterface,
  bridge: string,
  program = 'symphony',
): Promise<{ ok: boolean; reason?: string; missing?: true }> {
  const local = LOCAL_BRIDGE.exec(bridge)
  if (!local) return { ok: false }
  let ran
  try {
    // in the mod's own folder, never the session's: a symphony.exe lying there could win over the one on PATH
    ran = await $.process.run([program, '-background', '-open=false', '-addr', `${local[1]}:${local[2]}`], {
      cwd: $.plugin.root,
      timeoutMs: 30_000,
    })
  } catch (err) {
    // no symphony on PATH, or it didn't finish in time
    return /time/i.test(err instanceof Error ? err.message : String(err)) ? { ok: false } : { ok: false, missing: true }
  }
  if (ran.exitCode === 0) return { ok: true }
  const last = (s: string) =>
    s
      .split(/\r?\n/)
      .map(l => l.trim())
      .filter(Boolean)
      .at(-1)
  const said = (last(ran.stderr) ?? last(ran.stdout))
    ?.replace(/^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2}(\.\d+)? /, '')
    .replace(/([?&]key=)[0-9a-fA-F]+/g, '$1…')
  return { ok: false, reason: (said || `exit status ${ran.exitCode}`).slice(0, 300) }
}

// downloadPlan says whether the mod gets the symphony program itself, it not being installed: the mod's own version
// (whose release it fetches), a system and processor the releases are built for, and SYMPHONY_DOWNLOAD not 0
async function downloadPlan($: EngineInterface): Promise<Plan | undefined> {
  if ((await $.env.get('SYMPHONY_DOWNLOAD')) === '0') return undefined
  const version = await modVersion($)
  if (!version || !/^\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$/.test(version)) return undefined
  const uname = await safe($.process.run(['uname', '-sm'], { timeoutMs: 5_000 }))
  const winArch = uname?.exitCode === 0 ? undefined : (await $.env.get('PROCESSOR_ARCHITEW6432')) || (await $.env.get('PROCESSOR_ARCHITECTURE'))
  const target = targetOf(uname?.exitCode === 0 ? uname.stdout : undefined, winArch)
  return target ? { version, target } : undefined
}

// once per process: a /clear's new session waits for the same download
let fetchingBridge: Promise<void> | undefined

// fetchBridge downloads the program (download.ts) in the background, starts the bridge with it and reports in again;
// until then the session goes on with the terminal's approvals (its heartbeat finds the bridge once it's up)
async function fetchBridge($: EngineInterface, id: Identity, plan: Plan) {
  const got = await download($, plan, `${$.plugin.root}/bin`)
  if (!got.exe) {
    $.ui.toast(message(id.lang, 'fetchFailed', { version: plan.version, reason: got.reason ?? '' }))
    return
  }
  const ran = await startBridge($, id.bridge, got.exe)
  if (!ran.ok) {
    $.ui.toast(message(id.lang, 'startFailed', { bridge: id.bridge, reason: ran.reason ?? `${got.exe} didn't run` }))
    return
  }
  checked = undefined // the bridge that answers now is the one just started: check it
  const res = await emit($, 'session.start', { interactive, ...knownNow() }, ASK_TIMEOUT)
  if (res?.ok) $.ui.toast(message(id.lang, 'started', { bridge: id.bridge }))
}

// reports an event to the office page and waits for the answer (at most timeout); a failure is ignored and never
// affects the session. Any report the bridge accepts means the session is online there (a session the bridge
// first refused as busy, or that started before the bridge, joins through its heartbeat), so the inbox poll starts
// here; a 401 ends everything (the token won't change while the session runs). A report refused otherwise, with no
// inbox poll running, drops the bridge's check: between two heartbeats the bridge may stop and something else take its
// address, and only a failed request would tell. An answer that says fresh (the bridge didn't know the session: it
// restarted, or the session joins through this report) makes the mod tell it again what it reports only now and then.
async function emit($: EngineInterface, kind: string, data: object = {}, timeout = REPORT_TIMEOUT, session?: string) {
  if (refused || !(await ident($))) return undefined
  if (kind !== 'session.start') await startTold
  if (refused) return undefined
  let res
  try {
    res = await call($, '/api/event', { session: session ?? (await sid($)), kind, ...sticky, ...data }, timeout)
  } catch {
    return undefined
  }
  if (res?.status === 401) {
    refused = true
    heartbeat?.cancel()
  } else if (res?.ok && !ended) {
    pollInbox($)
    // a reloaded module the engine gave no session.start: the session goes on, so does its heartbeat
    if (revived && !heartbeat) beat($)
    if (kind !== 'session.start' && fresh(res)) tellAgain($)
  }
  // refused (busy, say) and no inbox poll that would notice the bridge going away: check it again before the next one
  else if (!polling) checked = undefined
  return res
}

function fresh(res: HttpResponse) {
  try {
    return JSON.parse(res.text)?.fresh === true
  } catch {
    return false
  }
}

// what a bridge that didn't know the session can't have learned from the heartbeat: the tools, the surfaces, the
// session's name and the background work (reported only when it changes)
function tellAgain($: EngineInterface) {
  background = { tasks: -1, crons: -1 }
  void reportTools($)
  void reportSurfaces($)
  if (title !== undefined) report($, 'session.title', { title })
}

// report queues an event: the hook goes on at once, and the events reach the bridge in the order they happened, each
// for the session it happened in. Once the session ended nothing more is reported for it. The queue is bounded: past
// OUTBOX_CAP the oldest heartbeat, usage figures or streamed piece goes, else the oldest report but a start or an end.
// The end goes to the front of the queue.
function report($: EngineInterface, kind: string, data: object = {}) {
  if (ended && kind !== 'session.end') return
  const session = sessionId
  const r = { kind, send: () => emit($, kind, data, REPORT_TIMEOUT, session) }
  // the goodbye goes before whatever is still queued: the engine gives the session.end hook 1.5 s, and a backlog of
  // reports would cut it off; then the bridge keeps the session for a minute, and a session started by hand (no launch
  // id to prove the same process) gets 409 under its new id until then
  if (kind === 'session.end') outbox.unshift(r)
  else outbox.push(r)
  if (outbox.length > OUTBOX_CAP) {
    let i = outbox.findIndex(r => OUTBOX_LOSSY.has(r.kind))
    if (i < 0) i = outbox.findIndex(r => r.kind !== 'session.start' && r.kind !== 'session.end')
    if (i >= 0) outbox.splice(i, 1)
  }
  sending ??= drain()
}

async function drain() {
  for (let r = outbox.shift(); r; r = outbox.shift()) {
    try {
      await r.send()
    } catch {
      // a report is never worth more than the ones behind it
    }
  }
  sending = undefined
}

// puts a pending ask on the bridge and long-polls for the page's answer, each poll at most WAIT_TIMEOUT (a bridge that
// doesn't answer by then is stuck: the terminal asks). When the terminal interrupts (Esc and the like) the wait ends
// at once. Anything but the page's own answer withdraws the card, so nobody answers it in vain.
// A hook's 10-second budget counts only its own code; time inside $.http.fetch doesn't count, so it can wait as long as needed.
async function ask(
  $: EngineInterface,
  signal: AbortSignal,
  payload: object,
): Promise<Answer | undefined> {
  const id = await ident($)
  if (refused || !id) return undefined
  const interrupted = new Promise<undefined>(resolve => {
    if (signal.aborted) resolve(undefined)
    else signal.addEventListener('abort', () => resolve(undefined), { once: true })
  })
  const withdraw = (card: string) => (withdrawLater ? withdrawLater(card) : void call($, `/api/cancel/${card}`, {}, ASK_TIMEOUT))
  const withdrawCreated = (res: HttpResponse | undefined) => {
    try {
      const late = res?.ok ? JSON.parse(res.text)?.id : undefined
      if (typeof late === 'string') withdraw(late)
    } catch {
      // not an ask the bridge made
    }
  }
  const create = async () => {
    // an ask the bridge takes only after the timeout, or after an interrupt, is withdrawn as soon as it is known
    const sent = call($, '/api/ask', { session: await sid($), ...payload }, ASK_TIMEOUT, withdrawCreated)
    const created = await Promise.race([sent, interrupted])
    if (signal.aborted) {
      void sent.then(withdrawCreated)
      return undefined
    }
    return created?.ok ? (JSON.parse(created.text).id as string) : undefined
  }
  let card: string | undefined
  let settled = false
  try {
    card = await create()
    if (!card) return undefined
    if (!toldWaiting) {
      toldWaiting = true
      $.ui.toast(message(id.lang, 'waiting', { bridge: id.bridge }))
    }
    while (!signal.aborted) {
      const waited = await Promise.race([call($, `/api/wait/${card}`, undefined, WAIT_TIMEOUT), interrupted])
      if (signal.aborted) break
      // the bridge restarted and lost the ask: put it back and keep waiting
      if (waited?.status === 404) {
        card = await create()
        if (!card) return undefined
        continue
      }
      if (!waited?.ok) return undefined
      const body = JSON.parse(waited.text)
      if (body.status === 'done' || body.status === 'cancelled') {
        settled = true
        return body.status === 'done' && body.answer && typeof body.answer === 'object' ? (body.answer as Answer) : undefined
      }
    }
    return undefined
  } catch {
    return undefined
  } finally {
    if (card && !settled) withdraw(card)
  }
}

// the model and thinking effort set the character's size in the office; reported only when they change.
// A switch (/model, the picker, a fallback) also names the model it came from and what switched it.
function reportModel($: EngineInterface, model: string, effort?: string | number, extra: object = {}) {
  const level = typeof effort === 'number' ? String(effort) : (effort ?? '')
  if (model === reported.model && level === reported.effort) return
  reported = { model, effort: level }
  report($, 'session.model', { model, effort: level || undefined, ...extra })
}

// /rename without an argument makes up a name that the command's result doesn't include, so we look for the last
// custom-title in the transcript. Read the file directly first (works everywhere, up to 4 MiB); if it's too big, use grep
// (missing on Windows, in which case the next prompt submission catches up).
async function titleFromTranscript($: EngineInterface) {
  if (!transcriptPath) return undefined
  let lines: string[]
  try {
    lines = (await $.fs.read(transcriptPath)).split('\n').filter(l => l.includes('"type":"custom-title"'))
  } catch {
    try {
      lines = (await $.process.run(['grep', '-F', '"type":"custom-title"', transcriptPath])).stdout.trim().split('\n')
    } catch {
      return undefined
    }
  }
  const last = lines.at(-1)
  try {
    return last ? (JSON.parse(last).customTitle as string | undefined) : undefined
  } catch {
    return undefined
  }
}

// summary is a tool call's main argument, for the chat: the command, the file, the pattern, the prompt (the
// tool's input fields sit on the event itself).
function summary(input: unknown): string {
  if (!input || typeof input !== 'object') return ''
  const m = input as Record<string, unknown>
  for (const k of ['command', 'file_path', 'notebook_path', 'pattern', 'url', 'query', 'description', 'prompt', 'skill', 'path']) {
    const v = m[k]
    if (typeof v === 'string' && v.trim()) return v.trim().slice(0, 300)
  }
  return ''
}

// tokens is what a response or a turn cost, as the bridge keeps it
type ApiUsage = { input_tokens: number; output_tokens: number; cache_read_input_tokens: number; cache_creation_input_tokens: number; model?: string }
function tokens(u: ApiUsage | null | undefined) {
  if (!u) return undefined
  return { input: u.input_tokens, output: u.output_tokens, cacheRead: u.cache_read_input_tokens, cacheWrite: u.cache_creation_input_tokens, model: u.model }
}

type Stop = { stopReason: string | null; usage: ApiUsage | null }

// A step's response in pieces: thinking and text chunks are gathered by block and sent every STREAM_FLUSH ms,
// and end closes the blocks, with what the response cost and why the model stopped.
function stepStream($: EngineInterface, turnId: string, step: number) {
  const blocks = new Map<number, { kind: string; text: string }>()
  let timer: { cancel: () => void } | undefined
  const flush = () => {
    timer?.cancel()
    timer = undefined
    if (!blocks.size) return
    const list = [...blocks].map(([i, b]) => ({ i, kind: b.kind, text: b.text }))
    blocks.clear()
    report($, 'chat.delta', { turnId, step, blocks: list })
  }
  return {
    add(index: number, kind: 'thinking' | 'text', text: string) {
      // thinking may come without words (a model whose thinking is summarized): the block still says it thinks
      if (!text && kind !== 'thinking') return
      const b = blocks.get(index)
      if (b) b.text += text
      else blocks.set(index, { kind, text })
      timer ??= $.clock.after(STREAM_FLUSH, flush)
    },
    end(stop?: Stop) {
      flush()
      report($, 'chat.stop', { turnId, step, tokens: tokens(stop?.usage), stopReason: stop?.stopReason ?? undefined })
    },
  }
}

// the usage figures (the context window's fill, the rate limits, the cost) go out at most every USAGE_FLUSH ms, the
// latest ones: the fill moves on every response, and each report is one request through the queue
function reportUsage($: EngineInterface, usage: object) {
  usagePending = usage
  usageTimer ??= $.clock.after(USAGE_FLUSH, () => {
    usageTimer = undefined
    const u = usagePending
    usagePending = undefined
    if (u) report($, 'session.usage', { usage: u })
  })
}

// the tools the session has: how many, and the MCP servers among them (mcp__<server>__<tool>)
async function reportTools($: EngineInterface) {
  const list = await safe($.tool.list())
  if (!list) return
  const mcp = new Set<string>()
  for (const t of list) {
    const m = /^mcp__(.+?)__/.exec(t.name)
    if (t.mcp && m) mcp.add(m[1])
  }
  report($, 'session.tools', { toolCount: list.length, mcp: [...mcp].sort() })
}

// the surfaces the session draws on: the terminal, the desktop app, a phone
async function reportSurfaces($: EngineInterface) {
  const surfaces = await safe($.session.surfaces())
  if (surfaces) report($, 'session.surfaces', { surfaces: [...surfaces] })
}

// the session's name (from --name or /rename), kept for a bridge that didn't know the session
function reportTitle($: EngineInterface, name: string | undefined) {
  if (typeof name === 'string' && name) title = name
  report($, 'session.title', { title: name })
}

// the inbox: what the page sent to this session. One long poll at a time, for as long as the session runs (a session
// after /clear starts its own at once).
type InboxItem = ({ kind: 'say'; text: string } | { kind: 'abort' }) & { id?: number }
// the id of the last inbox item done, named in the next poll (ack): the bridge drops an item only then, so a poll cut
// off on its way back loses nothing (a bridge before 1.2.0 sends no ids and hands each item out once)
let inboxAck = 0
// the ids already submitted (the last 200): an item handed out twice (two polls of one session out at the same time,
// whatever brought that about) goes in once
const inboxDone = new Set<number>()

function pollInbox($: EngineInterface) {
  if (polling) return
  polling = true
  const round = pollRound
  const stop = () => {
    if (round === pollRound) polling = false
  }
  const again = (ms: number) => {
    $.clock.after(ms, () => {
      if (round !== pollRound) return // a newer session polls for itself
      polling = false
      pollInbox($)
    })
  }
  void (async () => {
    if (ended || refused || !(await ident($))) return stop()
    let items: InboxItem[] = []
    try {
      const res = await call($, `/api/inbox?session=${encodeURIComponent(await sid($))}&ack=${inboxAck}`)
      if (res?.status === 401) {
        refused = true
        return stop()
      }
      if (!res?.ok) return again(INBOX_RETRY)
      items = (JSON.parse(res.text).items ?? []) as InboxItem[]
    } catch {
      return again(INBOX_RETRY)
    }
    // a poll of an earlier round (a report before session.start had the loop going when begin started the session's
    // own): what it brought is the new loop's to do, which gets the same items (the bridge keeps them until acked);
    // submitting here too would put the prompt in twice
    if (round !== pollRound) return
    // prompt.submit resolves once the engine took the prompt (after the running turn): the says are submitted as they
    // come and the poll goes on without waiting for that, so several sent while the session is busy queue up in the
    // engine at once, as typed in the terminal, instead of one per poll (the submits are started in order, so they
    // enter in order)
    for (const it of items) {
      if (it.id) {
        inboxAck = it.id // the last one done; a bridge that restarted hands out larger ids, so it stays right
        if (inboxDone.has(it.id)) continue
        inboxDone.add(it.id)
        if (inboxDone.size > 200) inboxDone.delete(inboxDone.values().next().value as number)
      }
      if (it.kind === 'say' && it.text) {
        const text = it.text
        void (async () => {
          // a prompt that didn't enter (a hook refused it, the engine's own limit, a failure) is dropped, with why
          let dropped: string | undefined
          try {
            const entered = await $.prompt.submit({ text, asUser: true })
            if (entered.drop !== undefined) dropped = entered.drop
          } catch (err) {
            dropped = err instanceof Error ? err.message : String(err)
          }
          if (dropped === undefined) report($, 'chat.sent', { text: text.slice(0, 4000) })
          else report($, 'chat.dropped', { text: text.slice(0, 4000), reason: dropped.slice(0, 500) })
        })()
      } else if (it.kind === 'abort' && mainTurn) {
        try {
          await $.turn.abort({ turnId: mainTurn })
        } catch {
          // a turn already over: nothing to do
        }
      }
    }
    again(items.length ? 0 : 50)
  })()
}

// begin starts reporting a session: at the start, and again under the new id after /clear or a /resume inside it
// begin starts the mod's record of a session; carryOn: the same session in a reloaded module, which keeps the page's
// rules and the office text (read from the session's state)
function begin(id: string | undefined, model: string, carryOn = false) {
  sessionId = id
  ended = false
  // a new session (after /clear too) starts without the page's rules; the state forgets them at the next keep
  rules = carryOn ? undefined : []
  officeContext = undefined
  pollRound++
  polling = false
  inboxAck = 0
  mainTurn = undefined
  reported = { model, effort: '' }
  background = { tasks: -1, crons: -1 }
  usagePending = undefined
}

// what the start told, as the heartbeat carries it: the model and effort as last reported
function knownNow() {
  return known ? { ...known, model: reported.model || known.model, effort: reported.effort || undefined } : {}
}

// heartbeat: after 45 seconds without any message the bridge takes the session as off duty (say the terminal was closed)
function beat($: EngineInterface) {
  heartbeat?.cancel()
  heartbeat = $.clock.every(15_000, () => {
    report($, 'heartbeat', knownNow())
  })
}

// ---------- the office in the session's prompt (SYMPHONY_CONTEXT=1, a launch option) ----------
// Who else is in the office as the conversation begins, by the names SendMessage reaches them by: one section of the
// system prompt, written once per conversation (a changing one would cost the prompt cache on every turn).
let officeContext: Promise<string | undefined> | undefined

async function officeText($: EngineInterface): Promise<string | undefined> {
  const res = await call($, `/api/roster?session=${encodeURIComponent(await sid($))}`, undefined, REPORT_TIMEOUT)
  if (!res?.ok) return undefined
  let roster: { you?: string; others?: { name: string; folder?: string; task?: string }[] }
  try {
    roster = JSON.parse(res.text)
  } catch {
    return undefined
  }
  const others = (roster.others ?? []).slice(0, 30)
  const line = (o: { name: string; folder?: string; task?: string }) =>
    `- ${o.name}${o.folder ? `, working in ${o.folder}` : ''}${o.task ? `, a member of task ${o.task}` : ''}`
  return [
    `You are ${roster.you || 'a session'}, one of several Claude Code sessions on this machine that the symphony office (a web page) shows as a team.`,
    others.length ? 'The others online as this conversation began:' : 'No other session was online as this conversation began.',
    ...others.map(line),
    'You can reach one with SendMessage, by its name. They work on their own requests: message one only when it helps with yours.',
  ].join('\n')
}

// ---------- an MCP server's form (an elicitation) on the page ----------
// The engine asks the person in the terminal; the page shows the form as an ask card, which the terminal's answer
// settles (the mod can't answer one: the hook API has no way to). Keyed by the elicitation's id, else its server.
const elicitations = new Map<string, Promise<string | undefined>>()

type Schema = { properties?: Record<string, { type?: string; title?: string; description?: string; enum?: unknown[] }>; required?: string[] }

function fieldsOf(schema: unknown) {
  const s = (schema ?? {}) as Schema
  return Object.entries(s.properties ?? {})
    .slice(0, 50)
    .map(([name, p]) => ({
      name,
      title: typeof p?.title === 'string' ? p.title : undefined,
      description: typeof p?.description === 'string' ? p.description : undefined,
      type: typeof p?.type === 'string' ? p.type : undefined,
      required: s.required?.includes(name) || undefined,
      enum: Array.isArray(p?.enum) ? p.enum.map(String).slice(0, 50) : undefined,
    }))
}

async function elicitCard($: EngineInterface, e: { mcp_server_name: string; message: string; mode?: string; url?: string; requested_schema?: unknown }) {
  const res = await call($, '/api/ask', {
    session: await sid($),
    kind: 'elicitation',
    server: e.mcp_server_name,
    message: e.message,
    mode: e.mode,
    url: e.url,
    fields: fieldsOf(e.requested_schema),
  }, ASK_TIMEOUT)
  try {
    const id = res?.ok ? JSON.parse(res.text)?.id : undefined
    return typeof id === 'string' ? id : undefined
  } catch {
    return undefined
  }
}

// settleElicitation tells the bridge what the terminal answered (accept, decline, cancel)
async function settleElicitation($: EngineInterface, key: string, action: string) {
  const card = await elicitations.get(key)
  elicitations.delete(key)
  if (card) await call($, `/api/settle/${card}`, { action }, ASK_TIMEOUT)
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    let told = () => {}
    startTold = new Promise<void>(resolve => (told = resolve))
    try {
      const started = await next(e)
      const id = await ident($)
      if (!id) return started
      if (id.insecure) {
        refused = true // never contacted
        $.ui.toast(message(id.lang, 'insecure', { bridge: id.bridge }))
        return started
      }
      // A module loaded into a session an earlier module of this process started (/reload-plugins, a worker respawn)
      // gets session.start again. The session goes on: the page's rules and the office text stay, and the bridge hears
      // a heartbeat, not a second start (no start line in the activity; the inbox poll and the heartbeat start anew).
      const current = await safe($.session.id())
      const kept = revived ? (await safe($.state.get(OFFICE)))?.value : undefined
      const carryOn = kept?.session !== undefined && kept.session === current
      if (!carryOn) await hideToken($, id)
      withdrawLater = card => void $.clock.after(0, () => void call($, `/api/cancel/${card}`, {}, ASK_TIMEOUT))
      // sessions started from the page carry the bridge's launch id, which matches the session with its terminal in the bridge
      sticky = { cwd: e.cwd, launch: (await $.env.get('SYMPHONY_LAUNCH')) || undefined, task: (await $.env.get('SYMPHONY_TASK')) || undefined }
      const model = await $.session.model()
      begin(current, model, carryOn)
      if (carryOn && typeof kept?.context === 'string') officeContext = Promise.resolve(kept.context)
      // what the engine knows of itself and the folder: its version, the mod's, the repository, where it draws
      const [engine, mod, repo] = await Promise.all([safe($.session.version()), modVersion($), safe($.session.repo())])
      interactive = e.isInteractive
      known = {
        login: id.login,
        model,
        surface: e.surface ?? undefined,
        engine: engine ? { version: engine.version, base: engine.base } : undefined,
        mod,
        // not the remote: its URL may carry credentials
        repo: repo ? { root: repo.root, name: repo.name ?? undefined, internal: repo.internal } : undefined,
      }
      const kind = carryOn ? 'heartbeat' : 'session.start'
      const online = carryOn ? knownNow() : { interactive, ...knownNow() }
      if (carryOn) told() // the bridge knows the session: no report waits for a start
      // no answer and refused: the bridge's check failed, and said why. Something that took the check and never answered
      // holds the address: no bridge could start there.
      let res = await emit($, kind, online, ASK_TIMEOUT)
      let failed: string | undefined
      let fetching: Plan | undefined
      if (!res && !refused && !(await isStuck($))) {
        const ran = await startBridge($, id.bridge)
        if (ran.ok) {
          res = await emit($, kind, online, ASK_TIMEOUT)
          if (res?.ok) $.ui.toast(message(id.lang, 'started', { bridge: id.bridge }))
        } else if (ran.missing && (fetching = await downloadPlan($))) {
          const plan = fetching
          fetchingBridge ??= fetchBridge($, id, plan).catch(() => {})
        } else failed = ran.reason
      }
      if (!res) {
        if (refused) {
          // said already
        } else if (fetching) $.ui.toast(message(id.lang, 'fetching', { version: fetching.version }))
        else if (failed) $.ui.toast(message(id.lang, 'startFailed', { bridge: id.bridge, reason: failed }))
        else $.ui.toast(message(id.lang, 'unreachable', { bridge: id.bridge }))
      } else if (res.status === 401) {
        refused = true // the token won't change while the session runs: don't knock again
        $.ui.toast(message(id.lang, 'badToken'))
      } else if (res.status === 409) $.ui.toast(message(id.lang, 'busy'))
      heartbeat?.cancel()
      if (!refused) {
        beat($)
        void reportTools($)
      }
      return started
    } finally {
      told()
    }
  })

  // on every model request, note the model and effort actually used, and stream the response's thinking and
  // text to the chat (main session only; subagents pass through); the stop chunk says what the response cost
  on('turn.step', async function* ($, e, next) {
    const stream = next(e)
    if ('agentId' in e && e.agentId) return yield* stream
    reportModel($, e.model, e.effort)
    const chat = (await office($)) ? stepStream($, e.turnId, e.index) : undefined
    let stop: Stop | undefined
    try {
      for (;;) {
        const n = await stream.next()
        if (n.done) return n.value
        const c = n.value
        if (chat && (c.kind === 'thinking' || c.kind === 'text')) chat.add(c.index, c.kind, c.text)
        else if (c.kind === 'stop') stop = { stopReason: c.stopReason, usage: c.usage }
        yield c
      }
    } finally {
      chat?.end(stop)
    }
  })

  // the context window's fill, the rate limits and the cost, whenever one of them moves (coalesced)
  on('session.measure', async ($, e, next) => {
    if (await office($)) {
      reportUsage($, {
        context: { tokens: e.context.tokens, window: e.context.window, percent: e.context.percent },
        rateLimits: e.rateLimits.map(r => ({ kind: r.kind, percentUsed: r.percentUsed, resetsAt: r.resetsAt })),
        cost: e.cost ? { usd: e.cost.usd } : undefined,
      })
    }
    return next(e)
  })

  // a prompt that enters the session (typed, a command, a notification); messages from other sessions have
  // their own events, and a prompt this mod submits for the page is reported as chat.sent
  on('prompt.submit', async ($, e, next) => {
    const entered = await next(e)
    if (!('drop' in entered)) {
      report($, 'chat.user', { text: e.text.slice(0, 4000), origin: e.origin?.kind, command: e.command?.name })
    }
    return entered
  })

  // the session name (from --name or /rename): sent at start or resume and with every prompt;
  // /rename itself is caught too, so a new name shows right away.
  // After /clear (or a /resume or fork inside the session) the conversation goes on under a new id with no
  // session.start: its SessionStart, once the old one's end was seen, starts it as session.start does, so the bridge
  // ends the old session and seats the new one (a task member gets its kickoff again). claude --resume fires a resume
  // right after its session.start too: then nothing ended, and nothing starts again.
  on('classic.SessionStart', async ($, e, next) => {
    transcriptPath = e.transcript_path
    const goesOn = e.source === 'clear' || e.source === 'resume' || e.source === 'fork'
    if (ended && goesOn && known && (await office($))) {
      begin(e.session_id, (await safe($.session.model())) || reported.model || known.model)
      void keepOffice($, { session: e.session_id, context: undefined }) // a reload from now on is this session's
      if (e.cwd) sticky = { ...sticky, cwd: e.cwd }
      report($, 'session.start', { interactive, ...knownNow() })
      beat($)
      void reportTools($)
    }
    reportTitle($, e.session_title)
    return next(e)
  })

  on('classic.UserPromptSubmit', async ($, e, next) => {
    transcriptPath = e.transcript_path
    reportTitle($, e.session_title)
    return next(e)
  })

  // every slash command, for the chat; /rename also for the name
  on('command.run', async ($, e, next) => {
    report($, 'command.run', { command: e.command, args: e.args.trim().slice(0, 200) })
    return next(e)
  })

  on('command.run', { command: 'rename' }, async ($, e, next) => {
    const ran = await next(e)
    if (!(await office($))) return ran
    reportTitle($, e.args.trim() || (await titleFromTranscript($)))
    return ran
  })

  // messages between sessions (SendMessage / the agent team mailbox): sending and receiving are reported once each; the bridge pairs them.
  on('session.send', async ($, e, next) => {
    const sent = await next(e)
    report($, 'message.send', {
      to: e.to,
      text: e.text.slice(0, 2000),
      origin: e.origin.kind,
      agentId: e.agentId,
      delivered: sent.isDelivered,
    })
    return sent
  })

  on('session.receive', async ($, e, next) => {
    report($, 'message.receive', {
      text: e.text.slice(0, 2000),
      origin: e.origin,
      agentId: e.agentId,
    })
    return next(e)
  })

  on('session.end', async ($, e, next) => {
    heartbeat?.cancel()
    usageTimer?.cancel()
    usageTimer = undefined
    report($, 'session.end', { reason: e.reason })
    ended = true
    // /clear and a /resume inside the session go on under another id, which classic.SessionStart names
    if (e.reason === 'clear' || e.reason === 'resume') sessionId = undefined
    await sending // the goodbye, and whatever was still on its way, before the session goes
    return next(e)
  })

  on('turn.start', async ($, e, next) => {
    if (!('agentId' in e && e.agentId)) mainTurn = e.turnId
    report($, 'turn.start', {
      turnId: e.turnId,
      text: e.text.slice(0, 200),
      agentId: 'agentId' in e ? e.agentId : undefined,
    })
    return next(e)
  })

  // why the turn ended (answered, interrupted, refused, an API error) and what it cost
  on('turn.complete', async ($, e, next) => {
    if (!('agentId' in e && e.agentId) && mainTurn === e.turnId) {
      mainTurn = undefined
      // a form the turn left without a result (interrupted): its card goes
      for (const key of [...elicitations.keys()]) void settleElicitation($, key, 'cancel')
    }
    report($, 'turn.complete', {
      turnId: e.turnId,
      durationMs: e.durationMs,
      isAborted: e.isAborted,
      reason: e.reason,
      refusal: e.reason === 'refusal' ? { category: e.refusal.category ?? undefined, explanation: e.refusal.explanation ?? undefined } : undefined,
      tokens: tokens(e.usage),
      agentId: 'agentId' in e ? e.agentId : undefined,
    })
    return next(e)
  })

  // an API error ended the turn (retries exhausted, the context limit)
  on('classic.StopFailure', async ($, e, next) => {
    report($, 'turn.error', { text: String(e.error), details: e.error_details?.slice(0, 500) })
    return next(e)
  })

  // subagents by name: what they were asked, their type and model; the id is on the spawn's result
  on('agent.spawn', async ($, e, next) => {
    const spawned = await next(e)
    if (spawned.agentId) {
      report($, 'agent.start', {
        agentId: spawned.agentId,
        agentType: e.subagentType,
        description: (e.description ?? '').slice(0, 200),
        model: spawned.model,
        background: e.background,
      })
    }
    return spawned
  })

  on('classic.SubagentStop', async ($, e, next) => {
    report($, 'agent.end', { agentId: e.agent_id, agentType: e.agent_type })
    return next(e)
  })

  // the agent team's task list (TaskCreate / TaskUpdate): "todo" here, as "task" is the office's collaborative task
  on('classic.TaskCreated', async ($, e, next) => {
    report($, 'todo.create', { todo: e.task_id, subject: e.task_subject.slice(0, 200), teammate: e.teammate_name })
    return next(e)
  })

  on('classic.TaskCompleted', async ($, e, next) => {
    report($, 'todo.done', { todo: e.task_id, subject: e.task_subject.slice(0, 200) })
    return next(e)
  })

  // what the engine would show the person as a notification (idle, an error, a permission prompt in the terminal)
  on('classic.Notification', async ($, e, next) => {
    report($, 'session.notify', { notifyType: e.notification_type, title: e.title, text: e.message.slice(0, 500) })
    return next(e)
  })

  // the conversation was compacted (by hand or automatically): the context fill drops
  on('classic.PostCompact', async ($, e, next) => {
    report($, 'session.compact', { trigger: e.trigger })
    return next(e)
  })

  // the model changed (/model, the picker, a fallback): the character changes size, and the chat says so
  on('classic.PostModelSwitch', async ($, e, next) => {
    reportModel($, e.to_model, reported.effort || undefined, { from: e.from_model, source: e.source })
    return next(e)
  })

  // the session moved to another folder: every later event carries it
  on('classic.CwdChanged', async ($, e, next) => {
    sticky = { ...sticky, cwd: e.new_cwd }
    report($, 'session.cwd')
    return next(e)
  })

  // the surfaces the session draws on (a phone attached, the desktop app connected)
  on('session.attach', async ($, e, next) => {
    const attached = await next(e)
    void reportSurfaces($)
    return attached
  })

  on('session.detach', async ($, e, next) => {
    const detached = await next(e)
    void reportSurfaces($)
    return detached
  })

  // the tools changed (an MCP server added, a plugin enabled)
  on('classic.ConfigChange', async ($, e, next) => {
    void reportTools($)
    return next(e)
  })

  // the end of a turn says what runs on in the background: tasks and scheduled runs; reported when it changes
  on('classic.Stop', async ($, e, next) => {
    const tasks = (e.background_tasks ?? []).length
    const crons = (e.session_crons ?? []).length
    if (tasks !== background.tasks || crons !== background.crons) {
      background = { tasks, crons }
      report($, 'session.background', { bgTasks: tasks, bgCrons: crons })
    }
    return next(e)
  })

  // a tool call failed, or a permission was denied (in the terminal or by a rule): the reason, for the chat's tool row
  on('classic.PostToolUseFailure', async ($, e, next) => {
    report($, 'tool.fail', { tool: e.tool_name, toolUseId: e.tool_use_id, text: e.error.slice(0, 500) })
    return next(e)
  })

  on('classic.PermissionDenied', async ($, e, next) => {
    report($, 'tool.fail', { tool: e.tool_name, toolUseId: e.tool_use_id, text: e.reason.slice(0, 500), denied: true })
    return next(e)
  })

  // permissions: when the engine's verdict is ask, the web page decides instead.
  // the office in the prompt, once per conversation, when the launch asked for it
  on('prompt.compose', async ($, e, next) => {
    const composed = await next(e)
    if ((await $.env.get('SYMPHONY_CONTEXT')) !== '1' || !(await office($))) return composed
    officeContext ??= officeText($).then(
      text => {
        if (text !== undefined) void keepOffice($, { context: text }) // a reloaded module's prompt says the same
        return text
      },
      () => undefined,
    )
    const text = await officeContext
    return text ? { ...composed, sections: [...composed.sections, { id: 'symphony:office', text, scope: 'session' as const }] } : composed
  })

  // an MCP server's form: a card on the page while the terminal asks; the terminal's answer settles it
  on('classic.Elicitation', async ($, e, next) => {
    if (await office($)) {
      const key = e.elicitation_id ?? e.mcp_server_name
      elicitations.set(key, elicitCard($, e).catch(() => undefined))
    }
    return next(e)
  })
  on('classic.ElicitationResult', async ($, e, next) => {
    const key = e.elicitation_id ?? e.mcp_server_name
    if (elicitations.has(key)) void settleElicitation($, key, e.action)
    return next(e)
  })

  on('tool.check', async ($, e, next) => {
    const verdict = await next(e)
    if (verdict.decision !== 'ask' || e.tool_use_id === undefined) return verdict
    if (e.tool === 'AskUserQuestion') return verdict

    // what the page allowed for the rest of the session lets the engine's own ask through without a card, within the
    // organization's ceiling; an ask another plugin made stays a person's
    const allowed = e.ceiling === undefined || e.ceiling === 'allow'
    if (verdict.plugin === undefined && allowed && (await office($)) && ruleAllows(await rulesOf($), e.tool, e.input)) {
      return { decision: 'allow', reason: message(identity?.lang ?? 'en', 'remembered') }
    }
    // what an allow could be kept as, offered on the card ("Allow for this session")
    const remember = verdict.plugin === undefined && allowed ? suggestRule(e.tool, e.input) : undefined
    const answer = await ask($, next.signal, {
      kind: 'permission',
      tool: e.tool,
      input: e.input,
      reason: verdict.reason,
      agentId: e.agentId,
      remember,
    })
    // no answer, or one that is neither allow nor deny: the engine's verdict stands
    if (!answer || !('decision' in answer) || (answer.decision !== 'allow' && answer.decision !== 'deny')) return verdict
    if (answer.decision === 'allow' && e.ceiling !== undefined && e.ceiling !== 'allow') {
      return verdict
    }
    if (answer.decision === 'allow' && answer.remember === true && remember) await keepRule($, remember)
    if (answer.decision === 'deny') deniedByWeb.add(e.tool_use_id)
    return {
      decision: answer.decision,
      reason: (typeof answer.reason === 'string' && answer.reason) || message(identity?.lang ?? 'en', answer.decision === 'allow' ? 'allowed' : 'denied'),
    }
  })

  // AskUserQuestion: no terminal dialog; the answer from the web page becomes the tool result.
  // Other tools are only reported, with how long they took.
  on('tool.call', async ($, e, next) => {
    const call = { tool: e.tool, agentId: e.agentId, toolUseId: e.tool_use_id }
    report($, 'tool.start', { ...call, summary: summary(e) })
    const t0 = (await office($)) ? await $.clock.now() : undefined

    let ran: Awaited<ReturnType<typeof next>> | undefined
    if (e.tool === 'AskUserQuestion') {
      const answer = await ask($, next.signal, {
        kind: 'question',
        questions: e.questions,
        agentId: e.agentId,
      })
      if (answer && 'answers' in answer && answer.answers && typeof answer.answers === 'object') {
        ran = { result: { questions: e.questions, answers: answer.answers } }
      }
    }
    ran ??= await next(e)

    const denied = ran.deny !== undefined || (e.tool_use_id !== undefined && deniedByWeb.delete(e.tool_use_id))
    const durationMs = t0 === undefined ? undefined : Math.max(0, (await $.clock.now()) - t0)
    report($, 'tool.end', { ...call, denied, isError: ran.isError === true, durationMs })
    return ran
  })
}
