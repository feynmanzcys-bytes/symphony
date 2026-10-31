import type { EngineInterface, Register } from 'claude-code'
import { type Lang, matchLang, message } from './messages'

// Environment variables:
//   SYMPHONY_TOKEN    the office account's token: one account serves one session at a time and sets its identity and seat
//   SYMPHONY_BRIDGE   the bridge's address, by default port 7788 on this machine
//   SYMPHONY_LAUNCH   the launch id the bridge sets when the page starts a session
//   SYMPHONY_TASK     the collaborative task's id, when the bridge starts the session as a task member (in a worktree of its own)
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
// The employee window in the page shows the session as a chat: the mod streams the model's thinking and text
// (turn.step, in batches), reports the prompts that enter (prompt.submit) and the tool calls, and polls the bridge
// for what the page sends (its inbox): text to submit as the person's prompt, or a stop of the running turn.
const DEFAULT_BRIDGE = 'http://127.0.0.1:7788'
// how long a report, or putting an ask on the bridge, may take; a hanging bridge holds nothing up for longer
const REPORT_TIMEOUT = 5_000
const ASK_TIMEOUT = 10_000
// how often the pieces of a streamed response go to the bridge, at most
const STREAM_FLUSH = 150
// after a failed inbox poll, how long before the next
const INBOX_RETRY = 3_000

type Answer =
  | { decision: 'allow' | 'deny'; reason?: string }
  | { answers: Record<string, string> }

type Identity = { bridge: string; headers: Record<string, string>; login?: string; lang: Lang }

// undefined: the environment hasn't been read yet; null: no SYMPHONY_TOKEN, not an office session
let identity: Identity | null | undefined
let sessionId: string | undefined
let transcriptPath: string | undefined
// every event carries the folder, the launch id and the task id, so a restarted bridge recognizes the session right away
let sticky: { cwd?: string; launch?: string; task?: string } = {}
// the model and effort last reported to the bridge; reported again only when they change
let reported = { model: '', effort: '' }
let heartbeat: { cancel: () => void } | undefined
// the bridge doesn't know the token (401): nothing more goes to it in this session
let refused = false
// reports go out in this order, one at a time
let queue: Promise<unknown> = Promise.resolve()
// calls the web page denied. A denied tool reaches tool.call as an error result, not { deny }, so we remember them.
const deniedByWeb = new Set<string>()
// the main loop's running turn, for a stop from the page
let mainTurn: string | undefined
// the session ended: the inbox poll stops
let ended = false
let polling = false

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

// A step's response in pieces: thinking and text chunks are gathered by block and sent every STREAM_FLUSH ms,
// and end closes the blocks.
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
    end() {
      flush()
      report($, 'chat.stop', { turnId, step })
    },
  }
}

// the inbox: what the page sent to this session. One long poll at a time, for as long as the session runs.
type InboxItem = { kind: 'say'; text: string } | { kind: 'abort' }

function pollInbox($: EngineInterface) {
  if (polling) return
  polling = true
  const again = (ms: number) => {
    $.clock.after(ms, () => {
      polling = false
      pollInbox($)
    })
  }
  void (async () => {
    if (ended || refused || !(await ident($))) {
      polling = false
      return
    }
    let items: InboxItem[] = []
    try {
      const res = await call($, `/api/inbox?session=${encodeURIComponent(await sid($))}`)
      if (res.status === 401) {
        refused = true
        polling = false
        return
      }
      if (!res.ok) return again(INBOX_RETRY)
      items = (JSON.parse(res.text).items ?? []) as InboxItem[]
    } catch {
      return again(INBOX_RETRY)
    }
    for (const it of items) {
      try {
        if (it.kind === 'say' && it.text) {
          await $.prompt.submit({ text: it.text, asUser: true })
          report($, 'chat.sent', { text: it.text.slice(0, 4000) })
        } else if (it.kind === 'abort' && mainTurn) {
          await $.turn.abort({ turnId: mainTurn })
        }
      } catch {
        // a prompt that couldn't enter, or a turn already over: nothing to do
      }
    }
    again(items.length ? 0 : 50)
  })()
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    const started = await next(e)
    const id = await ident($)
    if (!id) return started
    // sessions started from the page carry the bridge's launch id, which matches the session with its tmux window
    sticky = { cwd: e.cwd, launch: (await $.env.get('SYMPHONY_LAUNCH')) || undefined, task: (await $.env.get('SYMPHONY_TASK')) || undefined }
    const model = await $.session.model()
    reported = { model, effort: '' }
    ended = false
    mainTurn = undefined
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
      if (res?.ok) pollInbox($)
    }
    return started
  })

  // on every model request, note the model and effort actually used, and stream the response's thinking and
  // text to the chat (main session only; subagents pass through)
  on('turn.step', async function* ($, e, next) {
    const stream = next(e)
    if ('agentId' in e && e.agentId) return yield* stream
    reportModel($, e.model, e.effort)
    const office = !refused && (await ident($))
    const chat = office ? stepStream($, e.turnId, e.index) : undefined
    try {
      for (;;) {
        const n = await stream.next()
        if (n.done) return n.value
        const c = n.value
        if (chat && (c.kind === 'thinking' || c.kind === 'text')) chat.add(c.index, c.kind, c.text)
        yield c
      }
    } finally {
      chat?.end()
    }
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
    ended = true
    report($, 'session.end')
    await queue // the goodbye, and whatever was still on its way, before the session goes
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

  on('turn.complete', async ($, e, next) => {
    if (!('agentId' in e && e.agentId) && mainTurn === e.turnId) mainTurn = undefined
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
    const call = { tool: e.tool, agentId: e.agentId, toolUseId: e.tool_use_id }
    report($, 'tool.start', { ...call, summary: summary(e) })

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
