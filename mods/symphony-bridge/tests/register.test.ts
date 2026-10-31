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
    reqs.push(r)
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
  return { reqs, toasts, runs, clock }
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
  expect(w.toasts).toEqual([])
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
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, () => {
    answered++
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
  expect(w.toasts).toEqual(['symphony: このアカウントには既にオンラインのセッションがあります。承認はターミナルで行います'])
})

test('a locale the mod lacks means English', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok', LANG: 'it_IT.UTF-8' }, () => ({ status: 409, text: '{}' }))

  await $.session.start(start)
  expect(w.toasts).toEqual(['symphony: this account already has a session online; approvals stay in the terminal'])
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
