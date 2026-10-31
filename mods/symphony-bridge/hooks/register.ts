import type { EngineInterface, Register } from 'claude-code'
import { type Lang, matchLang, message } from './messages'

// Environment variables:
//   SYMPHONY_TOKEN    the office account's token: one account serves one session at a time and sets its identity and seat
//   SYMPHONY_BRIDGE   the bridge's address, by default port 7788 on this machine
//   SYMPHONY_LAUNCH   the launch id the bridge sets when the page starts a session
//   SYMPHONY_LANG     the language of the mod's messages (the bridge passes the page's language); else LC_ALL / LC_MESSAGES / LANG
// The mod is usually installed at user scope, so every Claude Code session loads it, but only sessions started
// with SYMPHONY_TOKEN belong to the office. In any other session it does nothing: it doesn't contact the bridge,
// read login details, show messages or touch approvals.
// When a bridge on this machine doesn't answer as the session starts, the mod starts it with the symphony program
// (symphony -background, if it's on PATH; SYMPHONY_DATA, if set, tells it where the accounts are).
// When the bridge is unreachable, rejects the token or the account is busy, everything falls back to Claude Code's
// own terminal interaction; the session never hangs.
// Reports (events, the heartbeat) go out one after another in the background: a hook never waits for the bridge,
// and a report that gets no answer in time is given up. Only asks wait, for the page's answer.
const DEFAULT_BRIDGE = 'http://127.0.0.1:7788'
// how long a report, or putting an ask on the bridge, may take; a hanging bridge holds nothing up for longer
const REPORT_TIMEOUT = 5_000
const ASK_TIMEOUT = 10_000

type Answer =
  | { decision: 'allow' | 'deny'; reason?: string }
  | { answers: Record<string, string> }

type Identity = { bridge: string; headers: Record<string, string>; login?: string; lang: Lang }

// undefined: the environment hasn't been read yet; null: no SYMPHONY_TOKEN, not an office session
let identity: Identity | null | undefined
let sessionId: string | undefined
let transcriptPath: string | undefined
// every event carries the folder and the launch id, so a restarted bridge recognizes the session right away
let sticky: { cwd?: string; launch?: string } = {}
// the model and effort last reported to the bridge; reported again only when they change
let reported = { model: '', effort: '' }
let heartbeat: { cancel: () => void } | undefined
// the bridge doesn't know the token (401): nothing more goes to it in this session
let refused = false
// reports go out in this order, one at a time
let queue: Promise<unknown> = Promise.resolve()
// calls the web page denied. A denied tool reaches tool.call as an error result, not { deny }, so we remember them.
const deniedByWeb = new Set<string>()

// The Claude login is in $CLAUDE_CONFIG_DIR/.claude.json, or in the home folder when that isn't set (Windows has no HOME; use USERPROFILE).
async function loginEmail($: EngineInterface) {
  const dir =
    (await $.env.get('CLAUDE_CONFIG_DIR')) ?? (await $.env.get('HOME')) ?? (await $.env.get('USERPROFILE'))
  if (!dir) return undefined
  try {
    const config = JSON.parse(await $.fs.read(`${dir}/.claude.json`))
    const email = config?.oauthAccount?.emailAddress
    return typeof email === 'string' && email ? email : undefined
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

async function ident($: EngineInterface) {
  if (identity !== undefined) return identity
  const token = await $.env.get('SYMPHONY_TOKEN')
  if (!token) return (identity = null)
  identity = {
    bridge: (await $.env.get('SYMPHONY_BRIDGE')) || DEFAULT_BRIDGE,
    headers: { 'content-type': 'application/json', authorization: `Bearer ${token}` },
    login: await loginEmail($),
    lang: await sessionLang($),
  }
  return identity
}

async function sid($: EngineInterface) {
  return (sessionId ??= await $.session.id())
}

// only called for office sessions (emit and ask check ident first)
async function call($: EngineInterface, path: string, body?: unknown) {
  const { bridge, headers } = (await ident($))!
  return $.http.fetch(`${bridge}${path}`, {
    method: body === undefined ? 'GET' : 'POST',
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}

// the value of p, or undefined once ms have passed or p fails (the request itself isn't stopped, just not waited for)
function within<T>($: EngineInterface, ms: number, p: Promise<T>): Promise<T | undefined> {
  return new Promise(resolve => {
    const timer = $.clock.after(ms, () => resolve(undefined))
    p.then(
      v => {
        timer.cancel()
        resolve(v)
      },
      () => {
        timer.cancel()
        resolve(undefined)
      },
    )
  })
}

// a bridge on this machine, which the mod may start itself: http://127.0.0.1:7788 and the like
const LOCAL_BRIDGE = /^http:\/\/(127\.0\.0\.1|localhost|\[::1\]):(\d+)\/?$/

// Nothing answers at the bridge's address. When it's on this machine, start the bridge with the symphony program, if
// it's on PATH: -background detaches it, so it keeps running after this session, and returns once it answers (a
// session starting it at the same moment finds it running and starts nothing).
async function startBridge($: EngineInterface, bridge: string) {
  const local = LOCAL_BRIDGE.exec(bridge)
  if (!local) return false
  try {
    const ran = await $.process.run(['symphony', '-background', '-open=false', '-addr', `${local[1]}:${local[2]}`], {
      timeoutMs: 30_000,
    })
    return ran.exitCode === 0
  } catch {
    return false // no symphony on PATH, or it didn't finish in time
  }
}

// reports an event to the office page and waits for the answer (at most timeout); a failure is ignored and never
// affects the session.
async function emit($: EngineInterface, kind: string, data: object = {}, timeout = REPORT_TIMEOUT) {
  if (refused || !(await ident($))) return undefined
  try {
    return await within($, timeout, call($, '/api/event', { session: await sid($), kind, ...sticky, ...data }))
  } catch {
    return undefined
  }
}

// report queues an event: the hook goes on at once, and the events reach the bridge in the order they happened.
function report($: EngineInterface, kind: string, data: object = {}) {
  queue = queue.then(() => emit($, kind, data))
}

// puts a pending ask on the bridge and long-polls for the page's answer.
// A hook's 10-second budget counts only its own code; time inside $.http.fetch doesn't count, so it can wait as long as needed.
async function ask(
  $: EngineInterface,
  signal: AbortSignal,
  payload: object,
): Promise<Answer | undefined> {
  if (refused || !(await ident($))) return undefined
  let id: string | undefined
  const create = async () => {
    const created = await within($, ASK_TIMEOUT, call($, '/api/ask', { session: await sid($), ...payload }))
    return created?.ok ? (JSON.parse(created.text).id as string) : undefined
  }
  try {
    id = await create()
    if (!id) return undefined
    while (!signal.aborted) {
      const waited = await call($, `/api/wait/${id}`)
      // the bridge restarted and lost the ask: put it back and keep waiting
      if (waited.status === 404) {
        id = await create()
        if (!id) return undefined
        continue
      }
      if (!waited.ok) return undefined
      const body = JSON.parse(waited.text)
      if (body.status === 'done') return body.answer as Answer
      if (body.status === 'cancelled') return undefined
    }
  } catch {
    return undefined
  }
  // the terminal interrupted (Esc and the like): tell the bridge to withdraw the card
  if (id) await within($, ASK_TIMEOUT, call($, `/api/cancel/${id}`, {}))
  return undefined
}

// the model and thinking effort set the character's size in the office; reported only when they change.
function reportModel($: EngineInterface, model: string, effort?: string | number) {
  const level = typeof effort === 'number' ? String(effort) : (effort ?? '')
  if (model === reported.model && level === reported.effort) return
  reported = { model, effort: level }
  report($, 'session.model', { model, effort: level || undefined })
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

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    const started = await next(e)
    const id = await ident($)
    if (!id) return started
    // sessions started from the page carry the bridge's launch id, which matches the session with its tmux window
    sticky = { cwd: e.cwd, launch: (await $.env.get('SYMPHONY_LAUNCH')) || undefined }
    const model = await $.session.model()
    reported = { model, effort: '' }
    const online = { interactive: e.isInteractive, login: id.login, model }
    let res = await emit($, 'session.start', online, ASK_TIMEOUT)
    if (!res && (await startBridge($, id.bridge))) {
      res = await emit($, 'session.start', online, ASK_TIMEOUT)
      if (res?.ok) $.ui.toast(message(id.lang, 'started', { bridge: id.bridge }))
    }
    if (!res) $.ui.toast(message(id.lang, 'unreachable', { bridge: id.bridge }))
    else if (res.status === 401) {
      refused = true // the token won't change while the session runs: don't knock again
      $.ui.toast(message(id.lang, 'badToken'))
    } else if (res.status === 409) $.ui.toast(message(id.lang, 'busy'))
    // heartbeat: after 45 seconds without any message the bridge takes the session as off duty (say the terminal was closed)
    heartbeat?.cancel()
    if (!refused) {
      heartbeat = $.clock.every(15_000, () => {
        report($, 'heartbeat')
      })
    }
    return started
  })

  // on every model request, note the model and effort actually used (main session only)
  on('turn.step', async function* ($, e, next) {
    if (!('agentId' in e && e.agentId)) reportModel($, e.model, e.effort)
    return yield* next(e)
  })

  // the session name (from --name or /rename): sent at start or resume and with every prompt;
  // /rename itself is caught too, so a new name shows right away.
  on('classic.SessionStart', async ($, e, next) => {
    transcriptPath = e.transcript_path
    report($, 'session.title', { title: e.session_title })
    return next(e)
  })

  on('classic.UserPromptSubmit', async ($, e, next) => {
    transcriptPath = e.transcript_path
    report($, 'session.title', { title: e.session_title })
    return next(e)
  })

  on('command.run', { command: 'rename' }, async ($, e, next) => {
    const ran = await next(e)
    if (refused || !(await ident($))) return ran
    const title = e.args.trim() || (await titleFromTranscript($))
    report($, 'session.title', { title })
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
    report($, 'session.end')
    await queue // the goodbye, and whatever was still on its way, before the session goes
    return next(e)
  })

  on('turn.start', async ($, e, next) => {
    report($, 'turn.start', {
      turnId: e.turnId,
      text: e.text.slice(0, 200),
      agentId: 'agentId' in e ? e.agentId : undefined,
    })
    return next(e)
  })

  on('turn.complete', async ($, e, next) => {
    report($, 'turn.complete', {
      turnId: e.turnId,
      durationMs: e.durationMs,
      isAborted: e.isAborted,
      agentId: 'agentId' in e ? e.agentId : undefined,
    })
    return next(e)
  })

  // permissions: when the engine's verdict is ask, the web page decides instead.
  on('tool.check', async ($, e, next) => {
    const verdict = await next(e)
    if (verdict.decision !== 'ask' || e.tool_use_id === undefined) return verdict
    if (e.tool === 'AskUserQuestion') return verdict

    const answer = await ask($, next.signal, {
      kind: 'permission',
      tool: e.tool,
      input: e.input,
      reason: verdict.reason,
      agentId: e.agentId,
    })
    if (!answer || !('decision' in answer)) return verdict
    if (answer.decision === 'allow' && e.ceiling !== undefined && e.ceiling !== 'allow') {
      return verdict
    }
    if (answer.decision === 'deny') deniedByWeb.add(e.tool_use_id)
    return {
      decision: answer.decision,
      reason: answer.reason || message(identity?.lang ?? 'en', answer.decision === 'allow' ? 'allowed' : 'denied'),
    }
  })

  // AskUserQuestion: no terminal dialog; the answer from the web page becomes the tool result.
  // Other tools are only reported.
  on('tool.call', async ($, e, next) => {
    const call = { tool: e.tool, agentId: e.agentId }
    report($, 'tool.start', call)

    let ran: Awaited<ReturnType<typeof next>> | undefined
    if (e.tool === 'AskUserQuestion') {
      const answer = await ask($, next.signal, {
        kind: 'question',
        questions: e.questions,
        agentId: e.agentId,
      })
      if (answer && 'answers' in answer) {
        ran = { result: { questions: e.questions, answers: answer.answers } }
      }
    }
    ran ??= await next(e)

    const denied = ran.deny !== undefined || (e.tool_use_id !== undefined && deniedByWeb.delete(e.tool_use_id))
    report($, 'tool.end', { ...call, denied, isError: ran.isError === true })
    return ran
  })
}
