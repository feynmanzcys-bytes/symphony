// The mod's unit tests: claude plugin test mods/symphony-bridge
// The test's on() sits beneath the mod and answers for the engine and the bridge; every request the mod sends
// and every message it shows is recorded.
import type { On } from 'claude-code'
import { expect, mock, test } from 'claude-code/testing'

type Req = { url: string; method?: string; auth?: string; body?: Record<string, unknown> }
// 'down' means the bridge isn't running and can't be reached
type Reply = { status: number; text: string } | 'down'
// what running symphony does: exits with a code, or 'missing' when it isn't on PATH
type Ran = { exitCode: number } | 'missing'

function world(
  on: On,
  env: Record<string, string>,
  reply: (r: Req) => Reply = () => ({ status: 200, text: '{}' }),
  run: (argv: readonly string[]) => Ran = () => 'missing',
) {
  mock.env(on, env)
  const clock = mock.clock(on)
  const reqs: Req[] = []
  const polls: Req[] = [] // the inbox long polls, apart from the reports
  const toasts: string[] = []
  const runs: string[][] = []
  on('process.run', (_$, e) => {
    runs.push([...e.argv])
    const ran = run(e.argv)
    if (ran === 'missing') return { deny: `${e.argv[0]}: not found` }
    return { value: { exitCode: ran.exitCode, stdout: '', stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  on('http.fetch', (_$, e) => {
    const headers = (e.init?.headers ?? {}) as Record<string, string>
    const body = typeof e.init?.body === 'string' ? JSON.parse(e.init.body) : undefined
    const r: Req = { url: e.url, method: e.init?.method, auth: headers.authorization, body }
    ;(r.url.includes('/api/inbox') ? polls : reqs).push(r)
    const answer = reply(r)
    if (answer === 'down') return { deny: 'connection refused' }
    const { status, text } = answer
    return { value: { status, ok: status >= 200 && status < 300, headers: {}, text } }
  })
  on('ui.toast', (_$, e) => {
    toasts.push(e.text)
    return { value: undefined }
  })
  on('session.id', () => ({ value: 'sess-1' }))
  on('session.model', () => ({ value: 'claude-sonnet-5-5' }))
  on('fs.read', () => ({ deny: 'tests read no files' }))
  on('session.start', (_$, e) => ({ cwd: e.cwd }))
  return { reqs, polls, toasts, runs, clock }
}

const start = { cwd: '/work', surface: 'terminal', isInteractive: true } as const
const bash = { tool: 'Bash', input: { command: 'touch x' }, tool_use_id: 'tu-1' }

test('a session without SYMPHONY_TOKEN: no bridge, no messages, approvals as usual', async ($, on) => {
  const w = world(on, { SYMPHONY_BRIDGE: 'http://bridge.test', SYMPHONY_LANG: 'zh-CN' })
  on('tool.check', () => ({ decision: 'ask', reason: 'ask a person' }))
  on('tool.call', () => ({ result: {} }))

  await $.session.start(start)
  expect((await $.tool.check(bash)).decision).toBe('ask')
  await $.tool.call({ tool: 'Bash', command: 'ls' })
  await w.clock.advance(60_000)

  expect(w.reqs).toEqual([])
  expect(w.toasts).toEqual([])
})

test('a session with a token: reports itself online with the token, then a heartbeat every 15 seconds', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok', SYMPHONY_BRIDGE: 'http://bridge.test', SYMPHONY_LAUNCH: 'L1' })

  await $.session.start(start)
  expect(w.reqs.length).toBe(1)
  expect(w.reqs[0]).toEqual(
    expect.objectContaining({ url: 'http://bridge.test/api/event', method: 'POST', auth: 'Bearer tok' }),
  )
  expect(w.reqs[0].body).toEqual(
    expect.objectContaining({ kind: 'session.start', session: 'sess-1', cwd: '/work', launch: 'L1', model: 'claude-sonnet-5-5' }),
  )

  await w.clock.advance(15_000)
  await w.clock.settle()
  expect(w.reqs.at(-1)?.body).toEqual(expect.objectContaining({ kind: 'heartbeat' }))
  expect(w.polls.length).toBeGreaterThan(0) // and it listens for what the page sends
  expect(w.toasts).toEqual([])
})

test('the account is busy at the start: once the bridge takes the heartbeat (the other session left), the inbox poll starts', async ($, on) => {
  let busy = true
  const w = world(on, { SYMPHONY_TOKEN: 'tok', SYMPHONY_BRIDGE: 'http://bridge.test' }, () =>
    busy ? { status: 409, text: '{}' } : { status: 200, text: '{}' },
  )

  await $.session.start(start)
  expect(w.toasts).toEqual(['symphony: this account already has a session online; approvals go to the terminal for now'])
  await w.clock.advance(15_000)
  await w.clock.settle()
  expect(w.polls.length).toBe(0) // nothing to listen for while the bridge refuses the session

  busy = false
  await w.clock.advance(15_000)
  await w.clock.settle()
  expect(w.reqs.at(-1)?.body).toEqual(expect.objectContaining({ kind: 'heartbeat' }))
  expect(w.polls.length).toBeGreaterThan(0) // the session is online now: what the page sends reaches it
})

test('the bridge rejects the token later (the account was deleted): nothing more is sent, not even heartbeats', async ($, on) => {
  let gone = false
  const w = world(on, { SYMPHONY_TOKEN: 'tok', SYMPHONY_BRIDGE: 'http://bridge.test' }, () =>
    gone ? { status: 401, text: '{}' } : { status: 200, text: '{}' },
  )

  await $.session.start(start)
  await w.clock.advance(15_000)
  await w.clock.settle()
  expect(w.reqs.at(-1)?.body).toEqual(expect.objectContaining({ kind: 'heartbeat' }))

  gone = true
  await w.clock.advance(15_000)
  await w.clock.settle()
  const sent = w.reqs.length // the heartbeat that got the 401
  await w.clock.advance(60_000)
  await w.clock.settle()
  expect(w.reqs.length).toBe(sent)
  expect(w.toasts).toEqual([])
})

test('a task member (SYMPHONY_TASK): every event carries the task id, so the bridge seats it in the meeting room', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok', SYMPHONY_BRIDGE: 'http://bridge.test', SYMPHONY_LAUNCH: 'L2', SYMPHONY_TASK: 'T7' })

  await $.session.start(start)
  expect(w.reqs[0].body).toEqual(expect.objectContaining({ kind: 'session.start', launch: 'L2', task: 'T7' }))

  await w.clock.advance(15_000)
  await w.clock.settle()
  expect(w.reqs.at(-1)?.body).toEqual(expect.objectContaining({ kind: 'heartbeat', task: 'T7' }))
})

test('the bridge rejects the token: one message, approvals stay in the terminal, and nothing more is sent', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'bad' }, () => ({ status: 401, text: '{"error":"no valid account token"}' }))
  on('tool.check', () => ({ decision: 'ask' }))
  on('tool.call', () => ({ result: {} }))

  await $.session.start(start)
  expect(w.toasts).toEqual(["symphony: the bridge doesn't know this SYMPHONY_TOKEN; approvals stay in the terminal"])
  expect((await $.tool.check(bash)).decision).toBe('ask')
  await $.tool.call({ tool: 'Bash', command: 'ls' })
  await w.clock.advance(60_000)
  await w.clock.settle()
  expect(w.reqs.length).toBe(1)
})

test('a tool call is reported before and after, in order, without waiting for the bridge', async ($, on) => {
  let answered = 0
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, r => {
    if (!r.url.includes('/api/inbox')) answered++
    return { status: 200, text: '{}' }
  })
  on('tool.call', () => ({ result: {} }))

  await $.session.start(start)
  await $.tool.call({ tool: 'Bash', command: 'ls' })
  await w.clock.settle()
  expect(w.reqs.map(r => r.body?.kind)).toEqual(['session.start', 'tool.start', 'tool.end'])
  expect(w.reqs[1].body).toEqual(expect.objectContaining({ tool: 'Bash', session: 'sess-1', cwd: '/work' }))
  expect(answered).toBe(3)
})

test('the bridge is unreachable and symphony is not installed: one message, approvals in the terminal for now', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, () => 'down')
  on('tool.check', () => ({ decision: 'ask' }))

  await $.session.start(start)
  expect(w.runs).toEqual([['symphony', '-background', '-open=false', '-addr', '127.0.0.1:7788']])
  expect(w.toasts).toEqual(["symphony: can't reach the bridge (http://127.0.0.1:7788); approvals stay in the terminal for now"])
  expect((await $.tool.check(bash)).decision).toBe('ask')
})

test('a bridge on this machine that is down: the mod starts it with symphony -background and takes its seat', async ($, on) => {
  let up = false
  const w = world(
    on,
    { SYMPHONY_TOKEN: 'tok', SYMPHONY_BRIDGE: 'http://localhost:7790' },
    () => (up ? { status: 200, text: '{}' } : 'down'),
    () => {
      up = true
      return { exitCode: 0 }
    },
  )

  await $.session.start(start)
  expect(w.runs).toEqual([['symphony', '-background', '-open=false', '-addr', 'localhost:7790']])
  const starts = w.reqs.filter(r => r.body?.kind === 'session.start')
  expect(starts.length).toBe(2)
  expect(starts[1]).toEqual(expect.objectContaining({ url: 'http://localhost:7790/api/event', auth: 'Bearer tok' }))
  expect(w.toasts).toEqual(['symphony: started the bridge (http://localhost:7790) in the background'])
})

test('symphony fails to start the bridge: the usual message, and no second try', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, () => 'down', () => ({ exitCode: 1 }))

  await $.session.start(start)
  expect(w.runs.length).toBe(1)
  expect(w.reqs.filter(r => r.body?.kind === 'session.start').length).toBe(1)
  expect(w.toasts).toEqual(["symphony: can't reach the bridge (http://127.0.0.1:7788); approvals stay in the terminal for now"])
})

test('a bridge on another machine is never started from here', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok', SYMPHONY_BRIDGE: 'http://bridge.test' }, () => 'down', () => ({ exitCode: 0 }))

  await $.session.start(start)
  expect(w.runs).toEqual([])
  expect(w.toasts).toEqual(["symphony: can't reach the bridge (http://bridge.test); approvals stay in the terminal for now"])
})

test('with a token: a call that needs a person waits on the bridge and goes ahead once the page allows it', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, r =>
    r.url.endsWith('/api/ask')
      ? { status: 200, text: '{"id":"a1"}' }
      : r.url.endsWith('/api/wait/a1')
        ? { status: 200, text: '{"status":"done","answer":{"decision":"allow"}}' }
        : { status: 200, text: '{}' },
  )
  on('tool.check', () => ({ decision: 'ask', reason: 'ask a person' }))

  await $.session.start(start)
  expect((await $.tool.check(bash)).decision).toBe('allow')
  const ask = w.reqs.find(r => r.url.endsWith('/api/ask'))
  expect(ask?.body).toEqual(expect.objectContaining({ kind: 'permission', tool: 'Bash', session: 'sess-1' }))
})

// ---------- the language of the mod's messages ----------

test('SYMPHONY_LANG picks the language of the messages', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'bad', SYMPHONY_LANG: 'zh-CN', LANG: 'ja_JP.UTF-8' }, () => ({ status: 401, text: '{}' }))

  await $.session.start(start)
  expect(w.toasts).toEqual(['symphony: bridge 不认这个 SYMPHONY_TOKEN，审批仍走终端'])
})

test('without SYMPHONY_LANG the POSIX locale decides: the first of LC_ALL, LC_MESSAGES and LANG that is set', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok', LANG: 'de_DE.UTF-8', LC_MESSAGES: 'ja_JP.UTF-8' }, () => ({ status: 409, text: '{}' }))

  await $.session.start(start)
  expect(w.toasts).toEqual(['symphony: このアカウントには既にオンラインのセッションがあります。承認はひとまずターミナルで行います'])
})

test('a locale the mod lacks means English', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok', LANG: 'it_IT.UTF-8' }, () => ({ status: 409, text: '{}' }))

  await $.session.start(start)
  expect(w.toasts).toEqual(['symphony: this account already has a session online; approvals go to the terminal for now'])
})

test('the reason sent back to the model when the page decides is in the session language', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok', SYMPHONY_LANG: 'de' }, r =>
    r.url.endsWith('/api/ask')
      ? { status: 200, text: '{"id":"a1"}' }
      : r.url.endsWith('/api/wait/a1')
        ? { status: 200, text: '{"status":"done","answer":{"decision":"deny"}}' }
        : { status: 200, text: '{}' },
  )
  on('tool.check', () => ({ decision: 'ask' }))

  await $.session.start(start)
  const verdict = await $.tool.check(bash)
  expect(verdict).toEqual(expect.objectContaining({ decision: 'deny', reason: 'symphony: auf der Webseite abgelehnt' }))
  expect(w.toasts).toEqual([])
})

// ---------- the chat: what the employee window shows ----------

const step = { turnId: 't1', index: 0, model: 'claude-opus-5-5', messageCount: 3 } as const
const stepResult = { turnId: 't1', index: 0, answer: 'Hello there', toolUses: [], stopReason: 'end_turn', usage: null } as const

test('the response streams to the chat: thinking and text in batches, then a stop', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('turn.step', async function* () {
    yield { kind: 'thinking', index: 0, text: 'Let me ' }
    yield { kind: 'thinking', index: 0, text: 'think.' }
    yield { kind: 'text', index: 1, text: 'Hello ' }
    yield { kind: 'text', index: 1, text: 'there' }
    return stepResult
  })

  await $.session.start(start)
  const stream = $.turn.step(step)
  const seen: string[] = []
  let result
  for (;;) {
    const n = await stream.next()
    if (n.done) {
      result = n.value
      break
    }
    seen.push(n.value.kind)
  }
  expect(seen).toEqual(['thinking', 'thinking', 'text', 'text']) // every chunk passes through
  expect(result?.answer).toBe('Hello there')
  await w.clock.advance(200)
  await w.clock.settle()
  const kinds = w.reqs.map(r => r.body?.kind)
  expect(kinds).toEqual(['session.start', 'session.model', 'chat.delta', 'chat.stop'])
  expect(w.reqs[2].body).toEqual(
    expect.objectContaining({
      turnId: 't1',
      step: 0,
      blocks: [
        { i: 0, kind: 'thinking', text: 'Let me think.' },
        { i: 1, kind: 'text', text: 'Hello there' },
      ],
    }),
  )
  expect(w.reqs[3].body).toEqual(expect.objectContaining({ turnId: 't1', step: 0 }))
})

test('thinking without words still reaches the chat as a block', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('turn.step', async function* () {
    yield { kind: 'thinking', index: 0, text: '' }
    yield { kind: 'text', index: 1, text: '391' }
    return { ...stepResult, answer: '391' }
  })

  await $.session.start(start)
  for await (const _ of $.turn.step(step)) {
    // consumed
  }
  await w.clock.advance(200)
  await w.clock.settle()
  const delta = w.reqs.find(r => r.body?.kind === 'chat.delta')
  expect(delta?.body?.blocks).toEqual([
    { i: 0, kind: 'thinking', text: '' },
    { i: 1, kind: 'text', text: '391' },
  ])
})

test('a subagent\'s steps pass through without reaching the chat', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('turn.step', async function* () {
    yield { kind: 'text', index: 0, text: 'sub' }
    return { ...stepResult, answer: 'sub' }
  })

  await $.session.start(start)
  for await (const _ of $.turn.step({ ...step, agentId: 'agent-1' })) {
    // consumed
  }
  await w.clock.advance(200)
  await w.clock.settle()
  expect(w.reqs.map(r => r.body?.kind)).toEqual(['session.start'])
})

test('without a token nothing streams', async ($, on) => {
  const w = world(on, {})
  on('turn.step', async function* () {
    yield { kind: 'text', index: 0, text: 'x' }
    return stepResult
  })

  await $.session.start(start)
  for await (const _ of $.turn.step(step)) {
    // consumed
  }
  await w.clock.advance(200)
  expect(w.reqs).toEqual([])
})

test('a prompt that enters is reported to the chat with where it came from', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('prompt.submit', (_$, e) => ({ text: e.text }))

  await $.session.start(start)
  await $.prompt.submit({ text: 'fix the tests' })
  await w.clock.settle()
  const user = w.reqs.find(r => r.body?.kind === 'chat.user')
  expect(user?.body).toEqual(expect.objectContaining({ text: 'fix the tests', session: 'sess-1' }))
})

test('the page sends text: the mod polls the inbox, submits it as the person\'s prompt and reports it sent', async ($, on) => {
  let polls = 0
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, r => {
    if (r.url.includes('/api/inbox?session=sess-1')) {
      polls++
      return { status: 200, text: polls === 1 ? '{"items":[{"kind":"say","text":"run the build"}]}' : '{"items":[]}' }
    }
    return { status: 200, text: '{}' }
  })
  const submitted: Record<string, unknown>[] = []
  on('prompt.submit', (_$, e) => {
    submitted.push({ text: e.text, origin: e.origin })
    return { text: e.text }
  })

  await $.session.start(start)
  await w.clock.advance(100)
  await w.clock.settle()
  expect(submitted.length).toBe(1)
  expect(submitted[0].text).toBe('run the build')
  const sent = w.reqs.find(r => r.body?.kind === 'chat.sent')
  expect(sent?.body).toEqual(expect.objectContaining({ text: 'run the build' }))
  expect(polls).toBeGreaterThan(1) // the poll goes on after an item
  expect(w.polls[0]).toEqual(expect.objectContaining({ auth: 'Bearer tok' }))
})

test('the page stops the running turn: the mod aborts the main turn', async ($, on) => {
  let polls = 0
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, r => {
    if (r.url.includes('/api/inbox')) {
      polls++
      return { status: 200, text: polls === 2 ? '{"items":[{"kind":"abort"}]}' : '{"items":[]}' }
    }
    return { status: 200, text: '{}' }
  })
  const aborted: string[] = []
  on('turn.abort', (_$, e) => {
    aborted.push(e.turnId)
    return { value: undefined }
  })
  on('turn.start', (_$, e) => ({ turnId: e.turnId }))

  await $.session.start(start)
  await $.turn.start({ turnId: 'turn-9', text: 'go' })
  await w.clock.advance(200)
  await w.clock.settle()
  expect(aborted).toEqual(['turn-9'])
})

test('a tool call carries its main argument for the chat', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('tool.call', () => ({ result: {} }))

  await $.session.start(start)
  await $.tool.call({ tool: 'Bash', command: 'go test ./...' })
  await w.clock.settle()
  expect(w.reqs[1].body).toEqual(expect.objectContaining({ kind: 'tool.start', tool: 'Bash', summary: 'go test ./...' }))
})
