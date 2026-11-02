// The mod's unit tests: claude plugin test mods/symphony-bridge
// The test's on() sits beneath the mod and answers for the engine and the bridge; every request the mod sends
// and every message it shows is recorded.
import type { On } from 'claude-code'
import { expect, mock, test } from 'claude-code/testing'
import { LANGS, message } from '../hooks/messages'
import { hmacSha256 } from '../hooks/register'

// answered: the request got an answer (not 'down' or 'hang'); proved: a hello the bridge answered with the right proof
type Req = { url: string; method?: string; auth?: string; body?: Record<string, unknown>; answered?: boolean; proved?: boolean }
// 'down' means the bridge isn't running and can't be reached; 'hang' that it never answers
type Reply = { status: number; text: string } | 'down' | 'hang'
// what running symphony does: exits with a code (saying something, maybe), or 'missing' when it isn't on PATH
type Ran = { exitCode: number; stdout?: string; stderr?: string } | 'missing'

const utf8 = (s: string) => new TextEncoder().encode(s)
const hex = (bytes: Uint8Array) => [...bytes].map(b => b.toString(16).padStart(2, '0')).join('')
const sha256hex = async (s: string) => hex(new Uint8Array(await crypto.subtle.digest('SHA-256', utf8(s))))

// the token the test bridge knows; it answers /api/hello for any other with a 401
const KNOWN = 'tok'

// The test bridge answers POST /api/hello as symphony's bridge does (unless it's down or hangs): the HMAC-SHA256 of
// "symphony-hello:" + nonce keyed with the token whose SHA-256 is the id. hello, when given, answers the hellos
// instead; undefined from it means as the bridge would.
function world(
  on: On,
  env: Record<string, string>,
  reply: (r: Req) => Reply | Promise<Reply> = () => ({ status: 200, text: '{}' }),
  run: (argv: readonly string[]) => Ran = () => 'missing',
  hello?: (r: Req) => Reply | undefined,
) {
  mock.env(on, env)
  const clock = mock.clock(on)
  const all: Req[] = [] // every request, in order
  const reqs: Req[] = []
  const polls: Req[] = [] // the inbox long polls, apart from the reports
  const hellos: Req[] = [] // the bridge's checks, apart from the reports
  const toasts: string[] = []
  const runs: string[][] = []
  const reads: string[] = []
  on('process.run', (_$, e) => {
    runs.push([...e.argv])
    const ran = run(e.argv)
    if (ran === 'missing') return { deny: `${e.argv[0]}: not found` }
    return { value: { exitCode: ran.exitCode, stdout: ran.stdout ?? '', stderr: ran.stderr ?? '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  const asBridge = async (r: Req): Promise<Reply> => {
    const answer = await reply(r)
    if (answer === 'down' || answer === 'hang') return answer
    if (r.body?.id !== (await sha256hex(KNOWN))) return { status: 401, text: '{"error":"no valid account token"}' }
    r.proved = true
    return { status: 200, text: JSON.stringify({ proof: hex(await hmacSha256(utf8(KNOWN), utf8(`symphony-hello:${r.body?.nonce}`))) }) }
  }
  on('http.fetch', async (_$, e) => {
    const headers = (e.init?.headers ?? {}) as Record<string, string>
    const body = typeof e.init?.body === 'string' ? JSON.parse(e.init.body) : undefined
    const r: Req = { url: e.url, method: e.init?.method, auth: headers.authorization, body }
    all.push(r)
    const isHello = r.url.endsWith('/api/hello')
    ;(isHello ? hellos : r.url.includes('/api/inbox') ? polls : reqs).push(r)
    const answer = isHello ? (hello?.(r) ?? (await asBridge(r))) : await reply(r)
    r.answered = answer !== 'down' && answer !== 'hang'
    if (answer === 'down') return { deny: 'connection refused' }
    if (answer === 'hang') return new Promise<never>(() => {})
    const { status, text } = answer
    return { value: { status, ok: status >= 200 && status < 300, headers: {}, text } }
  })
  on('ui.toast', (_$, e) => {
    toasts.push(e.text)
    return { value: undefined }
  })
  const session = { id: 'sess-1' } // what $.session.id() answers
  on('session.id', () => ({ value: session.id }))
  on('session.model', () => ({ value: 'claude-sonnet-5-5' }))
  on('fs.read', (_$, e) => {
    reads.push(e.path)
    return { deny: 'tests read no files' }
  })
  // the engine's side of session.start; a test may have it run the engine's own start hooks first, as the engine does
  const engine: { start?: () => Promise<void> } = {}
  on('session.start', async (_$, e) => {
    await engine.start?.()
    return { cwd: e.cwd }
  })
  return { all, reqs, polls, hellos, toasts, runs, reads, clock, session, engine }
}

// The token leaves the session only for a bridge that has just proved it knows it: every request that carries it
// comes after a hello with the right proof, with no request that got no answer in between.
function expectCheckedBeforeToken(all: Req[]) {
  let trusted = false
  for (const r of all) {
    if (r.url.endsWith('/api/hello')) {
      expect(r.auth).toBeUndefined()
      trusted = r.proved === true
      continue
    }
    if (r.auth !== undefined) expect({ url: r.url, trusted }).toEqual({ url: r.url, trusted: true })
    if (!r.answered) trusted = false
  }
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
  const w = world(on, { SYMPHONY_TOKEN: 'tok', SYMPHONY_BRIDGE: 'https://bridge.test', SYMPHONY_LAUNCH: 'L1' })

  await $.session.start(start)
  expect(w.reqs.length).toBe(1)
  expect(w.reqs[0]).toEqual(
    expect.objectContaining({ url: 'https://bridge.test/api/event', method: 'POST', auth: 'Bearer tok' }),
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
  const w = world(on, { SYMPHONY_TOKEN: 'tok', SYMPHONY_BRIDGE: 'https://bridge.test' }, () =>
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
  const w = world(on, { SYMPHONY_TOKEN: 'tok', SYMPHONY_BRIDGE: 'https://bridge.test' }, () =>
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
  const w = world(on, { SYMPHONY_TOKEN: 'tok', SYMPHONY_BRIDGE: 'https://bridge.test', SYMPHONY_LAUNCH: 'L2', SYMPHONY_TASK: 'T7' })

  await $.session.start(start)
  expect(w.reqs[0].body).toEqual(expect.objectContaining({ kind: 'session.start', launch: 'L2', task: 'T7' }))

  await w.clock.advance(15_000)
  await w.clock.settle()
  expect(w.reqs.at(-1)?.body).toEqual(expect.objectContaining({ kind: 'heartbeat', task: 'T7' }))
})

test('the bridge rejects the token at its check: one message, approvals stay in the terminal, and the token is never sent', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'bad' }, () => ({ status: 401, text: '{"error":"no valid account token"}' }))
  on('tool.check', () => ({ decision: 'ask' }))
  on('tool.call', () => ({ result: {} }))

  await $.session.start(start)
  expect(w.toasts).toEqual(["symphony: the bridge doesn't know this SYMPHONY_TOKEN; approvals stay in the terminal"])
  expect((await $.tool.check(bash)).decision).toBe('ask')
  await $.tool.call({ tool: 'Bash', command: 'ls' })
  await w.clock.advance(60_000)
  await w.clock.settle()
  expect(w.hellos.length).toBe(1) // the check the bridge refused, and nothing after it
  expect(w.all.length).toBe(1)
  expect(w.all.filter(r => r.auth !== undefined)).toEqual([])
})

test('a tool call is reported before and after, in order, without waiting for the bridge', async ($, on) => {
  let answered = 0
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, r => {
    if (r.url.endsWith('/api/event')) answered++
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
  expect(w.hellos.map(h => h.answered)).toEqual([false, true]) // nothing answered the first check; the started bridge did
  const starts = w.reqs.filter(r => r.body?.kind === 'session.start')
  expect(starts.length).toBe(1)
  expect(starts[0]).toEqual(expect.objectContaining({ url: 'http://localhost:7790/api/event', auth: 'Bearer tok' }))
  expect(w.toasts).toEqual(['symphony: started the bridge (http://localhost:7790) in the background; run symphony to open the office'])
  expectCheckedBeforeToken(w.all)
})

test('symphony fails to start the bridge without a word: the message names its exit status, and no second try', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, () => 'down', () => ({ exitCode: 1 }))

  await $.session.start(start)
  expect(w.runs.length).toBe(1)
  expect(w.hellos.length).toBe(1)
  expect(w.reqs).toEqual([]) // the token never left: nothing answered the check
  expect(w.toasts).toEqual(["symphony: couldn't start the bridge (http://127.0.0.1:7788): exit status 1; approvals stay in the terminal for now"])
})

test('symphony fails to start the bridge: the message says why, as symphony said it last, without the time or the page key', async ($, on) => {
  const stderr = [
    '2026/11/02 10:00:00 starting',
    "2026/11/02 10:00:06 the bridge doesn't answer at http://127.0.0.1:7788/?key=0123456789abcdef0123456789abcdef yet; its log: /data/bridge.log",
    '',
  ].join('\n')
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, () => 'down', () => ({ exitCode: 1, stderr }))

  await $.session.start(start)
  expect(w.toasts).toEqual([
    "symphony: couldn't start the bridge (http://127.0.0.1:7788): the bridge doesn't answer at http://127.0.0.1:7788/?key=… yet; its log: /data/bridge.log; approvals stay in the terminal for now",
  ])
})

test('a bridge on another machine is never started from here', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok', SYMPHONY_BRIDGE: 'https://bridge.test' }, () => 'down', () => ({ exitCode: 0 }))

  await $.session.start(start)
  expect(w.runs).toEqual([])
  expect(w.toasts).toEqual(["symphony: can't reach the bridge (https://bridge.test); approvals stay in the terminal for now"])
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

// what the page can't decide leaves the engine's verdict: what the bridge answers to the ask and the wait for it
const created = { status: 200, text: '{"id":"a1"}' }
const answering = (wait: string): ((r: Req) => Reply) => r =>
  r.url.endsWith('/api/ask') ? created : r.url.endsWith('/api/wait/a1') ? { status: 200, text: wait } : { status: 200, text: '{}' }
const unanswered: [string, (r: Req) => Reply][] = [
  ['the bridge is down', () => 'down'],
  ['the ask gets a 500', r => (r.url.endsWith('/api/ask') ? { status: 500, text: 'oops' } : { status: 200, text: '{}' })],
  ['the ask gets an answer that is not JSON', r => (r.url.endsWith('/api/ask') ? { status: 200, text: '<html>' } : { status: 200, text: '{}' })],
  ['the wait gets a 500', r => (r.url.endsWith('/api/ask') ? created : r.url.endsWith('/api/wait/a1') ? { status: 500, text: '' } : { status: 200, text: '{}' })],
  ['the wait gets an answer that is not JSON', answering('done')],
  ['the answer is neither allow nor deny', answering('{"status":"done","answer":{"decision":"maybe"}}')],
  ['the answer is not an object', answering('{"status":"done","answer":"allow"}')],
  ['the answer is null', answering('{"status":"done","answer":null}')],
]
for (const [what, reply] of unanswered) {
  test(`a call that needs a person stays with the engine's ask when ${what}`, async ($, on) => {
    world(on, { SYMPHONY_TOKEN: 'tok' }, reply)
    on('tool.check', () => ({ decision: 'ask', reason: 'ask a person' }))

    await $.session.start(start)
    expect(await $.tool.check(bash)).toEqual(expect.objectContaining({ decision: 'ask', reason: 'ask a person' }))
  })
}

test("a call that needs a person stays with the engine's ask when the bridge doesn't take the ask in time", async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, r => (r.url.endsWith('/api/ask') ? 'hang' : { status: 200, text: '{}' }))
  on('tool.check', () => ({ decision: 'ask', reason: 'ask a person' }))

  await $.session.start(start)
  const verdict = $.tool.check(bash)
  await w.clock.settle()
  await w.clock.advance(10_000)
  expect(await verdict).toEqual(expect.objectContaining({ decision: 'ask', reason: 'ask a person' }))
})

// ---------- the bridge's check: the token goes only to a bridge that proves it knows it ----------

test('HMAC-SHA256 gives the RFC 4231 test vectors', async () => {
  const bytes = (b: number, n: number) => new Uint8Array(n).fill(b)
  const vectors: [Uint8Array, Uint8Array, string][] = [
    [bytes(0x0b, 20), utf8('Hi There'), 'b0344c61d8db38535ca8afceaf0bf12b881dc200c9833da726e9376c2e32cff7'],
    [utf8('Jefe'), utf8('what do ya want for nothing?'), '5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843'],
    [bytes(0xaa, 20), bytes(0xdd, 50), '773ea91e36800e46854db8ebd09181a72959098b3ef8c122d9635514ced565fe'],
    [Uint8Array.from({ length: 25 }, (_, i) => i + 1), bytes(0xcd, 50), '82558a389a443c0ea4cc819899f2083a85f0faa3e578f8077a2e3ff46729665b'],
    // case 5's output is truncated to 128 bits
    [bytes(0x0c, 20), utf8('Test With Truncation'), 'a3b6167473100ee06e0c796c2955552b'],
    // cases 6 and 7: a key longer than SHA-256's block is hashed first
    [bytes(0xaa, 131), utf8('Test Using Larger Than Block-Size Key - Hash Key First'), '60e431591ee0b67f0d8a26aacbf5b77f8e0bc6213728c5140546040f0ee37f54'],
    [
      bytes(0xaa, 131),
      utf8('This is a test using a larger than block-size key and a larger than block-size data. The key needs to be hashed before being used by the HMAC algorithm.'),
      '9b09ffa71b942fcb27635fbcd5b0e944bfdc63644f0713938a7f51535c3a35e2',
    ],
  ]
  for (const [key, data, mac] of vectors) expect(hex(await hmacSha256(key, data)).slice(0, mac.length)).toBe(mac)
})

test('the check carries the token\'s hash and a fresh nonce, never the token; once the bridge proves it, the reports carry the token', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok', SYMPHONY_BRIDGE: 'https://bridge.test' })

  await $.session.start(start)
  await w.clock.advance(15_000)
  await w.clock.settle()
  expect(w.hellos.length).toBe(1) // once, for as long as the bridge answers
  const hello = w.hellos[0]!
  expect(w.all[0]).toBe(hello) // before anything else
  expect(hello).toEqual(expect.objectContaining({ url: 'https://bridge.test/api/hello', method: 'POST', auth: undefined }))
  expect(Object.keys(hello.body ?? {}).sort()).toEqual(['id', 'nonce'])
  expect(hello.body?.id).toBe(await sha256hex('tok'))
  expect(String(hello.body?.nonce)).toMatch(/^[0-9a-f]{32}$/)
  expect(w.reqs.map(r => r.body?.kind)).toEqual(['session.start', 'heartbeat'])
  expect([...w.reqs, ...w.polls].every(r => r.auth === 'Bearer tok')).toBe(true)
  expectCheckedBeforeToken(w.all)
})

const notBridge = (bridge: string) =>
  `symphony: what answers at ${bridge} isn't a symphony bridge this session can trust, or it's older than this plugin (update symphony); approvals stay in the terminal`

const strangers: [string, Reply][] = [
  ['a wrong proof', { status: 200, text: JSON.stringify({ proof: '00'.repeat(32) }) }],
  ['no proof', { status: 200, text: '{}' }],
  ['a 404 (a bridge older than the mod)', { status: 404, text: '404 page not found' }],
  ['an answer that is not JSON', { status: 200, text: '<html>hello</html>' }],
  ['a 500', { status: 500, text: 'oops' }],
]
for (const [what, answer] of strangers) {
  test(`what answers the check with ${what} never gets the token: one message, approvals in the terminal, nothing more sent`, async ($, on) => {
    const w = world(on, { SYMPHONY_TOKEN: 'tok' }, () => ({ status: 200, text: '{}' }), () => ({ exitCode: 0 }), () => answer)
    on('tool.check', () => ({ decision: 'ask' }))
    on('tool.call', () => ({ result: {} }))

    await $.session.start(start)
    expect(w.toasts).toEqual([notBridge('http://127.0.0.1:7788')])
    expect((await $.tool.check(bash)).decision).toBe('ask')
    await $.tool.call({ tool: 'Bash', command: 'ls' })
    await w.clock.advance(60_000)
    await w.clock.settle()
    expect(w.all.length).toBe(1) // the check, and nothing after it
    expect(w.all.filter(r => r.auth !== undefined)).toEqual([])
    expect(w.runs).toEqual([]) // something answered: no bridge to start
  })
}

test('a proof for another nonce (a replayed answer) fails the check', async ($, on) => {
  const replayed = hex(await hmacSha256(utf8('tok'), utf8(`symphony-hello:${'00'.repeat(16)}`)))
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, undefined, undefined, () => ({ status: 200, text: JSON.stringify({ proof: replayed }) }))

  await $.session.start(start)
  expect(w.toasts).toEqual([notBridge('http://127.0.0.1:7788')])
  expect(w.all.filter(r => r.auth !== undefined)).toEqual([])
})

test('after a request that gets no answer, the bridge is checked again before the token goes out', async ($, on) => {
  let down = false
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, () => (down ? 'down' : { status: 200, text: '{}' }))

  await $.session.start(start)
  await w.clock.advance(15_000)
  await w.clock.settle()
  expect(w.hellos.length).toBe(1)

  down = true // the bridge stops
  await w.clock.advance(15_000)
  await w.clock.settle()
  down = false // and comes back
  const before = w.hellos.length
  await w.clock.advance(15_000)
  await w.clock.settle()
  expect(w.hellos.length).toBeGreaterThan(before)
  expect(w.reqs.at(-1)).toEqual(expect.objectContaining({ auth: 'Bearer tok', body: expect.objectContaining({ kind: 'heartbeat' }) }))
  expect(new Set(w.hellos.map(h => h.body?.nonce)).size).toBe(w.hellos.length) // a fresh nonce every time
  expectCheckedBeforeToken(w.all)
  expect(w.toasts).toEqual([])
})

test('something else takes the bridge\'s address while it is away: it never gets the token', async ($, on) => {
  let now: 'bridge' | 'down' | 'other' = 'bridge'
  const w = world(
    on,
    { SYMPHONY_TOKEN: 'tok' },
    () => (now === 'down' ? 'down' : { status: 200, text: '{}' }), // the other server takes anything
    undefined,
    () => (now === 'other' ? { status: 404, text: '404 page not found' } : undefined),
  )
  on('tool.check', () => ({ decision: 'ask' }))

  await $.session.start(start)
  await w.clock.advance(15_000)
  await w.clock.settle()
  now = 'down'
  await w.clock.advance(15_000)
  await w.clock.settle()
  now = 'other'
  const from = w.all.length
  await w.clock.advance(60_000)
  await w.clock.settle()
  expect((await $.tool.check(bash)).decision).toBe('ask')
  expect(w.all.slice(from).map(r => r.url)).toEqual(['http://127.0.0.1:7788/api/hello']) // the check it failed, then nothing
  expect(w.toasts).toEqual([notBridge('http://127.0.0.1:7788')])
  expectCheckedBeforeToken(w.all)
})

test('a session refused as busy checks the bridge again before each report: what takes the address in between never gets the token', async ($, on) => {
  let now: 'bridge' | 'other' = 'bridge'
  const w = world(
    on,
    { SYMPHONY_TOKEN: 'tok' },
    () => ({ status: 409, text: '{}' }), // the account is busy; the other server takes anything just the same
    undefined,
    () => (now === 'other' ? { status: 404, text: '404 page not found' } : undefined),
  )

  await $.session.start(start)
  await w.clock.advance(15_000)
  await w.clock.settle()
  expect(w.hellos.length).toBe(2) // one before each report: no inbox poll keeps an eye on the bridge
  now = 'other' // the bridge stops and something else takes its address, with no failed request in between
  const from = w.all.length
  await w.clock.advance(15_000)
  await w.clock.settle()
  expect(w.all.slice(from).filter(r => r.auth !== undefined)).toEqual([])
  expect(w.toasts).toEqual([
    'symphony: this account already has a session online; approvals go to the terminal for now',
    notBridge('http://127.0.0.1:7788'),
  ])
  expectCheckedBeforeToken(w.all)
})

test('an error from the bridge is still an answer: the check stands', async ($, on) => {
  let failing = false
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, r => (failing && r.url.endsWith('/api/event') ? { status: 500, text: '' } : { status: 200, text: '{}' }))

  await $.session.start(start)
  failing = true
  await w.clock.advance(15_000)
  await w.clock.settle()
  failing = false
  await w.clock.advance(15_000)
  await w.clock.settle()
  expect(w.reqs.filter(r => r.body?.kind === 'heartbeat').length).toBe(2)
  expect(w.hellos.length).toBe(1)
})

test('a bridge that never answers the check: the session goes on, approvals in the terminal, the token stays', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, () => 'hang')
  on('tool.check', () => ({ decision: 'ask' }))

  const started = $.session.start(start)
  await w.clock.settle()
  await w.clock.advance(5_000)
  await started
  expect(w.toasts).toEqual(["symphony: can't reach the bridge (http://127.0.0.1:7788); approvals stay in the terminal for now"])
  const verdict = $.tool.check(bash)
  await w.clock.settle()
  await w.clock.advance(5_000)
  expect((await verdict).decision).toBe('ask')
  expect(w.all.length).toBeGreaterThan(0)
  expect(w.all.every(r => r.url.endsWith('/api/hello'))).toBe(true)
})

// SYMPHONY_BRIDGE: plain http only on this machine (with a port), else https
const insecure = ['http://bridge.test:7788', 'http://192.168.1.20:7788', 'http://127.0.0.1:7788@evil.test', 'http://localhost.evil.test:7788', 'http://127.0.0.1', 'ftp://127.0.0.1:7788']
for (const bridge of insecure) {
  test(`SYMPHONY_BRIDGE=${bridge}: neither on this machine nor https, so the bridge is never contacted`, async ($, on) => {
    const w = world(on, { SYMPHONY_TOKEN: 'tok', SYMPHONY_BRIDGE: bridge }, undefined, () => ({ exitCode: 0 }))
    on('tool.check', () => ({ decision: 'ask' }))
    on('tool.call', () => ({ result: {} }))

    await $.session.start(start)
    expect(w.toasts).toEqual([`symphony: SYMPHONY_BRIDGE (${bridge}) must be on this machine or use https; approvals stay in the terminal`])
    expect((await $.tool.check(bash)).decision).toBe('ask')
    await $.tool.call({ tool: 'Bash', command: 'ls' })
    await w.clock.advance(60_000)
    await w.clock.settle()
    expect(w.all).toEqual([])
    expect(w.runs).toEqual([])
  })
}

const accepted = [
  ['http://localhost:7790', 'http://localhost:7790'],
  ['http://[::1]:7790', 'http://[::1]:7790'],
  ['http://127.0.0.1:7790/', 'http://127.0.0.1:7790'],
  ['https://office.example.com:8443/symphony', 'https://office.example.com:8443/symphony'],
] as const
for (const [bridge, base] of accepted) {
  test(`SYMPHONY_BRIDGE=${bridge} is accepted: the check and then the reports go there`, async ($, on) => {
    const w = world(on, { SYMPHONY_TOKEN: 'tok', SYMPHONY_BRIDGE: bridge })

    await $.session.start(start)
    expect(w.hellos.map(r => r.url)).toEqual([`${base}/api/hello`])
    expect(w.reqs.map(r => r.url)).toEqual([`${base}/api/event`])
    expect(w.toasts).toEqual([])
  })
}

// ---------- the login ----------

test('the login is the email $.session.identity() tells', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('session.identity', () => ({ value: { kind: 'claude.ai', email: 'ada@example.com', isVerified: true } }))

  await $.session.start(start)
  expect(w.reqs[0]?.body).toEqual(expect.objectContaining({ kind: 'session.start', login: 'ada@example.com' }))
})

test('$.session.identity() answers { deny } (the organization did not install the mod): no login, and no config file is read for one', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok', HOME: '/home/ada', CLAUDE_CONFIG_DIR: '/home/ada/.claude' })
  on('session.identity', () => ({ value: { deny: 'not installed by managed settings' } }))

  await $.session.start(start)
  expect(w.reqs[0]?.body?.kind).toBe('session.start')
  expect(w.reqs[0]?.body?.login).toBeUndefined()
  expect(w.reads.filter(p => p.includes('.claude.json'))).toEqual([])
})

test('a session without an email (an API key) reports no login', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('session.identity', () => ({ value: { kind: 'api-key', isVerified: false } }))

  await $.session.start(start)
  expect(w.reqs[0]?.body?.kind).toBe('session.start')
  expect(w.reqs[0]?.body?.login).toBeUndefined()
})

test('an engine that does not answer $.session.identity(): no login', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('session.identity', () => ({ deny: 'not here' }))

  await $.session.start(start)
  expect(w.reqs[0]?.body?.kind).toBe('session.start')
  expect(w.reqs[0]?.body?.login).toBeUndefined()
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
  expect(w.toasts).toEqual(['symphony: wartet auf deine Antwort auf der Webseite des Büros (http://127.0.0.1:7788); Esc bricht ab'])
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

// ---------- everything else the engine tells the mod about the session ----------

const kinds = (w: { reqs: Req[] }) => w.reqs.map(r => r.body?.kind)
const find = (w: { reqs: Req[] }, kind: string) => w.reqs.find(r => r.body?.kind === kind)?.body

test('the start names the engine, the mod, the repository (not its remote) and the surface; the tools follow', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('session.version', () => ({ value: { version: '2.1.316', base: '2.1.316', builtAt: '2026-10-01T00:00:00Z' } }))
  on('session.repo', () => ({ value: { root: '/work', remote: 'https://user:secret@github.com/x/y.git', internal: false, name: 'y' } }))
  on('tool.list', () => ({
    value: [
      { name: 'Bash', description: '', mcp: false },
      { name: 'mcp__github__list_issues', description: '', mcp: true },
      { name: 'mcp__github__get_issue', description: '', mcp: true },
      { name: 'mcp__slack__send', description: '', mcp: true },
    ],
  }))

  await $.session.start(start)
  await w.clock.settle()
  expect(w.reqs[0].body).toEqual(
    expect.objectContaining({
      kind: 'session.start',
      surface: 'terminal',
      engine: { version: '2.1.316', base: '2.1.316' },
      repo: { root: '/work', name: 'y', internal: false },
    }),
  )
  expect(JSON.stringify(w.reqs[0]?.body)).not.toContain('github.com') // a remote's URL may carry credentials
  expect(w.reqs[0].body?.mod).toBeUndefined() // the tests read no files: the manifest's version stays unknown
  expect(find(w, 'session.tools')).toEqual(expect.objectContaining({ toolCount: 4, mcp: ['github', 'slack'] }))
})

test('without a repository or an engine that answers, the start goes out without them', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('session.repo', () => ({ value: null }))
  on('session.version', () => ({ deny: 'not here' }))

  await $.session.start(start)
  await w.clock.settle()
  expect(w.reqs[0].body?.repo).toBeUndefined()
  expect(w.reqs[0].body?.engine).toBeUndefined()
  expect(kinds(w)).toEqual(['session.start'])
})

test('the usage figures go out coalesced: the latest ones, at most every 2 seconds', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('session.measure', (_$, e) => ({ changed: e.changed }))
  const measure = (tokens: number, usd: number) =>
    $.session.measure({ context: { tokens, window: 200_000, percent: tokens / 2000 }, rateLimits: [{ kind: 'five_hour', percentUsed: 12 }], cost: { usd }, changed: ['context', 'cost'] })

  await $.session.start(start)
  await measure(1000, 0.1)
  await measure(2000, 0.2)
  await w.clock.advance(500)
  await w.clock.settle()
  expect(kinds(w)).toEqual(['session.start']) // not yet
  await measure(3000, 0.3)
  await w.clock.advance(2000)
  await w.clock.settle()
  const usage = w.reqs.filter(r => r.body?.kind === 'session.usage')
  expect(usage.length).toBe(1)
  expect(usage[0].body?.usage).toEqual({
    context: { tokens: 3000, window: 200_000, percent: 1.5 },
    rateLimits: [{ kind: 'five_hour', percentUsed: 12, resetsAt: undefined }],
    cost: { usd: 0.3 },
  })
})

test('the stop of a response carries its tokens and why the model stopped', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  const usage = { input_tokens: 100, output_tokens: 20, cache_read_input_tokens: 1000, cache_creation_input_tokens: 5, model: 'claude-opus-5-5' }
  on('turn.step', async function* () {
    yield { kind: 'text', index: 0, text: 'ok' }
    yield { kind: 'stop', stopReason: 'end_turn', usage }
    return { ...stepResult, answer: 'ok', usage }
  })

  await $.session.start(start)
  for await (const _ of $.turn.step(step)) {
    // consumed
  }
  await w.clock.advance(200)
  await w.clock.settle()
  expect(find(w, 'chat.stop')).toEqual(
    expect.objectContaining({
      turnId: 't1',
      step: 0,
      stopReason: 'end_turn',
      tokens: { input: 100, output: 20, cacheRead: 1000, cacheWrite: 5, model: 'claude-opus-5-5' },
    }),
  )
})

test('a turn that ends says why: a refusal with the API\'s reason, and an API error', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('turn.complete', () => ({ text: '' }))
  on('classic.StopFailure', () => ({}))

  await $.session.start(start)
  await $.turn.complete({ turnId: 't1', answer: '', durationMs: 10, isAborted: false, reason: 'refusal', refusal: { category: 'harm', explanation: null } })
  await $.classic.StopFailure({ error: 'rate_limit', error_details: 'try later' })
  await $.turn.complete({ turnId: 't2', answer: '', durationMs: 10, isAborted: false, reason: 'error' })
  await w.clock.settle()
  expect(find(w, 'turn.complete')).toEqual(expect.objectContaining({ reason: 'refusal', refusal: { category: 'harm' } }))
  expect(find(w, 'turn.error')).toEqual(expect.objectContaining({ text: 'rate_limit', details: 'try later' }))
  expect(kinds(w)).toEqual(['session.start', 'turn.complete', 'turn.error', 'turn.complete'])
})

test('a subagent is reported by name once spawned, and its end too', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('agent.spawn', () => ({ model: 'claude-haiku-5-5', agentId: 'agent-7' }))
  on('classic.SubagentStop', () => ({}))

  await $.session.start(start)
  await $.agent.spawn({ prompt: 'find the tests', description: 'Find the tests', subagentType: 'Explore' })
  await $.classic.SubagentStop({ stop_hook_active: false, agent_id: 'agent-7', agent_transcript_path: '/t', agent_type: 'Explore' })
  await w.clock.settle()
  expect(find(w, 'agent.start')).toEqual(
    expect.objectContaining({ agentId: 'agent-7', agentType: 'Explore', description: 'Find the tests', model: 'claude-haiku-5-5' }),
  )
  expect(find(w, 'agent.end')).toEqual(expect.objectContaining({ agentId: 'agent-7', agentType: 'Explore' }))
})

test('the classic hooks the office listens to: tasks, notifications, compaction, a model switch, a folder change, background work', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  for (const ev of ['TaskCreated', 'TaskCompleted', 'Notification', 'PostCompact', 'PostModelSwitch', 'CwdChanged', 'Stop'] as const) on(`classic.${ev}`, () => ({}))

  await $.session.start(start)
  await $.classic.TaskCreated({ task_id: '1', task_subject: 'write tests', teammate_name: 'bob' })
  await $.classic.TaskCompleted({ task_id: '1', task_subject: 'write tests' })
  await $.classic.Notification({ message: 'Claude is waiting for your input', notification_type: 'idle_prompt' })
  await $.classic.PostCompact({ trigger: 'auto', compact_summary: '...' })
  await $.classic.PostModelSwitch({
    from_model: 'claude-sonnet-5-5', to_model: 'claude-opus-5-5', requested_model: 'opus', source: 'command', context_tokens: 100,
    prompt_cache_warm: false, cache_ttl: '5m', estimated_cache_write_usd: 0, pricing: 'catalog',
  })
  await $.classic.CwdChanged({ old_cwd: '/work', new_cwd: '/work/sub' })
  await $.classic.Stop({ stop_hook_active: false, background_tasks: [{ id: 'b1', type: 'shell', status: 'running' }], session_crons: [] })
  await $.classic.Stop({ stop_hook_active: false, background_tasks: [{ id: 'b1', type: 'shell', status: 'running' }], session_crons: [] }) // unchanged: not reported again
  await w.clock.settle()
  expect(kinds(w)).toEqual([
    'session.start', 'todo.create', 'todo.done', 'session.notify', 'session.compact', 'session.model', 'session.cwd', 'session.background',
  ])
  expect(find(w, 'todo.create')).toEqual(expect.objectContaining({ todo: '1', subject: 'write tests', teammate: 'bob' }))
  expect(find(w, 'session.notify')).toEqual(expect.objectContaining({ notifyType: 'idle_prompt', text: 'Claude is waiting for your input' }))
  expect(find(w, 'session.compact')).toEqual(expect.objectContaining({ trigger: 'auto' }))
  expect(find(w, 'session.model')).toEqual(expect.objectContaining({ model: 'claude-opus-5-5', from: 'claude-sonnet-5-5', source: 'command' }))
  expect(find(w, 'session.cwd')).toEqual(expect.objectContaining({ cwd: '/work/sub' }))
  expect(w.reqs.at(-1)?.body).toEqual(expect.objectContaining({ kind: 'session.background', bgTasks: 1, bgCrons: 0, cwd: '/work/sub' }))
})

test('a slash command is reported with its arguments', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('command.run', () => ({ text: '' }))

  await $.session.start(start)
  await $.command.run({ command: 'compact', args: ' keep the API ', origin: { kind: 'composer' }, presentation: { fullscreen: false } })
  await w.clock.settle()
  expect(find(w, 'command.run')).toEqual(expect.objectContaining({ command: 'compact', args: 'keep the API' }))
})

test('a tool call reports how long it took, and a failure or a denial its reason', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('tool.call', async () => {
    await w.clock.advance(1234) // the call takes 1234 ms of the mocked clock
    return { deny: 'exit 1' }
  })
  on('classic.PostToolUseFailure', () => ({}))
  on('classic.PermissionDenied', () => ({}))

  await $.session.start(start)
  await $.tool.call({ tool: 'Bash', command: 'false', tool_use_id: 'tu-9' })
  await $.classic.PostToolUseFailure({ tool_name: 'Bash', tool_input: { command: 'false' }, tool_use_id: 'tu-9', error: 'exit 1' })
  await $.classic.PermissionDenied({ tool_name: 'Edit', tool_input: {}, tool_use_id: 'tu-10', reason: 'a deny rule' })
  await w.clock.settle()
  expect(find(w, 'tool.end')).toEqual(expect.objectContaining({ tool: 'Bash', toolUseId: 'tu-9', denied: true, durationMs: 1234 }))
  const fails = w.reqs.filter(r => r.body?.kind === 'tool.fail').map(r => r.body)
  expect(fails).toEqual([
    expect.objectContaining({ tool: 'Bash', toolUseId: 'tu-9', text: 'exit 1' }),
    expect.objectContaining({ tool: 'Edit', toolUseId: 'tu-10', text: 'a deny rule', denied: true }),
  ])
})

test('the end says why the session ended', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('session.end', (_$, e) => ({ sessionId: e.sessionId }))

  await $.session.start(start)
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: { id: 'sess-1' } })
  expect(w.reqs.at(-1)?.body).toEqual(expect.objectContaining({ kind: 'session.end', reason: 'clear' }))
})

test('without a token none of it is reported', async ($, on) => {
  const w = world(on, {})
  on('session.measure', (_$, e) => ({ changed: e.changed }))
  on('agent.spawn', () => ({ model: 'm', agentId: 'a' }))
  on('classic.PostCompact', () => ({}))

  await $.session.start(start)
  await $.session.measure({ context: { window: 1 }, rateLimits: [], changed: ['context'] })
  await $.agent.spawn({ prompt: 'x' })
  await $.classic.PostCompact({ trigger: 'manual', compact_summary: '' })
  await w.clock.advance(5000)
  await w.clock.settle()
  expect(w.reqs).toEqual([])
})

// ---------- the messages: every language has every message, with the same placeholders ----------

test('every language has the messages English has, with the same placeholders', async () => {
  const keys = ['unreachable', 'started', 'startFailed', 'badToken', 'notBridge', 'insecure', 'busy', 'waiting', 'allowed', 'denied'] as const
  const names = (text: string) => [...text.matchAll(/\{(\w+)\}/g)].map(m => m[1]).sort()
  for (const lang of LANGS) {
    for (const key of keys) {
      const text = message(lang, key)
      expect({ lang, key, ok: typeof text === 'string' && text.startsWith('symphony') }).toEqual({ lang, key, ok: true })
      expect({ lang, key, names: names(text) }).toEqual({ lang, key, names: names(message('en', key)) })
    }
  }
})

// ---------- /clear and /resume inside the session: it goes on under a new id ----------

test('/clear: the old session ends, the new one starts under its own id, and heartbeats and the inbox poll go on', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok', SYMPHONY_LAUNCH: 'L1', SYMPHONY_TASK: 'T7' })
  on('session.end', (_$, e) => ({ sessionId: e.sessionId }))
  on('classic.SessionStart', () => ({}))
  on('tool.list', () => ({ value: [{ name: 'Bash', description: '', mcp: false }] }))

  await $.session.start(start)
  await w.clock.advance(15_000)
  await w.clock.settle()
  w.session.id = 'sess-2' // the engine may be on the new id before the goodbye goes out: it still names the old one
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: { id: 'sess-1' } })
  const ended = w.reqs.length
  expect(w.reqs.at(-1)?.body).toEqual(expect.objectContaining({ kind: 'session.end', reason: 'clear', session: 'sess-1' }))

  await $.classic.SessionStart({ source: 'clear', session_id: 'sess-2', session_title: 'bob-T7', cwd: '/work' })
  await w.clock.settle()
  const after = w.reqs.slice(ended)
  // a start, as at the start: the bridge ends the old one by the launch id and gives a task member its kickoff again
  expect(after[0]?.body).toEqual(
    expect.objectContaining({ kind: 'session.start', session: 'sess-2', launch: 'L1', task: 'T7', cwd: '/work', model: 'claude-sonnet-5-5', interactive: true, surface: 'terminal' }),
  )
  expect(after.map(r => r.body?.kind)).toEqual(expect.arrayContaining(['session.start', 'session.title', 'session.tools']))
  const polled = w.polls.length
  await w.clock.advance(15_000)
  await w.clock.settle()
  expect(w.reqs.at(-1)?.body).toEqual(expect.objectContaining({ kind: 'heartbeat', session: 'sess-2' }))
  expect(w.reqs.slice(ended).every(r => r.body?.session === 'sess-2')).toBe(true)
  expect(w.polls.length).toBeGreaterThan(polled)
  expect(w.polls.at(-1)?.url).toContain('session=sess-2')
})

test('/resume inside the session: the same, under the resumed id', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('session.end', (_$, e) => ({ sessionId: e.sessionId }))
  on('classic.SessionStart', () => ({}))

  await $.session.start(start)
  await $.session.end({ reason: 'resume', sessionId: 'sess-1', resume: { id: 'sess-1' } })
  await $.classic.SessionStart({ source: 'resume', session_id: 'sess-0' })
  await w.clock.advance(15_000)
  await w.clock.settle()
  expect(w.reqs.map(r => [r.body?.kind, r.body?.session])).toEqual([
    ['session.start', 'sess-1'],
    ['session.end', 'sess-1'],
    ['session.start', 'sess-0'],
    ['session.title', 'sess-0'],
    ['heartbeat', 'sess-0'],
  ])
})

test('a report the engine makes while the session starts (its name) goes out after the start, naming the launch and the task', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok', SYMPHONY_LAUNCH: 'L1', SYMPHONY_TASK: 'T7' })
  on('classic.SessionStart', () => ({}))
  // the engine runs its own start hooks inside session.start: the mod's SessionStart reports the session's name then
  w.engine.start = async () => {
    await $.classic.SessionStart({ source: 'startup', session_id: 'sess-1', session_title: 'helper-T7', cwd: '/work' })
  }

  await $.session.start(start)
  await w.clock.settle()
  expect(kinds(w).slice(0, 2)).toEqual(['session.start', 'session.title'])
  expect(find(w, 'session.title')).toEqual(expect.objectContaining({ session: 'sess-1', title: 'helper-T7', launch: 'L1', task: 'T7' }))
})

test('claude --resume: the resume right after the start starts nothing again', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('classic.SessionStart', () => ({}))

  await $.session.start(start)
  await $.classic.SessionStart({ source: 'resume', session_id: 'sess-1', session_title: 'bob' })
  await w.clock.settle()
  expect(kinds(w)).toEqual(['session.start', 'session.title'])
})

test('once the session ended for good nothing more is reported (it would come back to life on the page)', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('session.end', (_$, e) => ({ sessionId: e.sessionId }))
  on('classic.SessionStart', () => ({}))
  on('tool.call', () => ({ result: {} }))

  await $.session.start(start)
  await $.session.end({ reason: 'prompt_input_exit', sessionId: 'sess-1', resume: { id: 'sess-1' } })
  await $.classic.SessionStart({ source: 'compact', session_id: 'sess-1' })
  await $.tool.call({ tool: 'Bash', command: 'ls' })
  await w.clock.advance(60_000)
  await w.clock.settle()
  expect(kinds(w)).toEqual(['session.start', 'session.end'])
})

// ---------- a bridge that restarted, or meets the session through its heartbeat ----------

test('the heartbeat carries what the start told, with the model and effort as last used', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' })
  on('session.version', () => ({ value: { version: '2.1.316', base: '2.1.316' } }))
  on('session.repo', () => ({ value: { root: '/work', remote: 'https://user:secret@github.com/x/y.git', internal: false, name: 'y' } }))
  on('session.identity', () => ({ value: { kind: 'claude.ai', email: 'ada@example.com', isVerified: true } }))
  on('turn.step', async function* () {
    yield { kind: 'text', index: 0, text: 'ok' }
    return stepResult
  })

  await $.session.start(start)
  for await (const _ of $.turn.step({ ...step, effort: 'high' })) {
    // consumed
  }
  await w.clock.advance(15_000)
  await w.clock.settle()
  const beat = w.reqs.filter(r => r.body?.kind === 'heartbeat').at(-1)
  expect(beat?.body).toEqual(
    expect.objectContaining({
      model: 'claude-opus-5-5',
      effort: 'high',
      engine: { version: '2.1.316', base: '2.1.316' },
      repo: { root: '/work', name: 'y', internal: false },
      surface: 'terminal',
      login: 'ada@example.com',
    }),
  )
  expect(JSON.stringify(beat?.body)).not.toContain('github.com')
})

test('a bridge that did not know the session (fresh) hears the tools, the surfaces, the name and the background work again', async ($, on) => {
  let restarted = false
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, r => {
    if (restarted && r.body?.kind === 'heartbeat') {
      restarted = false
      return { status: 200, text: '{"ok":true,"fresh":true}' }
    }
    return { status: 200, text: '{"ok":true}' }
  })
  on('tool.list', () => ({ value: [{ name: 'Bash', description: '', mcp: false }] }))
  on('session.surfaces', () => ({ value: ['terminal'] }))
  on('classic.UserPromptSubmit', () => ({}))
  on('classic.Stop', () => ({}))
  const stop = () => $.classic.Stop({ stop_hook_active: false, background_tasks: [], session_crons: [] })

  await $.session.start(start)
  await $.classic.UserPromptSubmit({ prompt: 'hi', session_title: 'bob' })
  await stop()
  await stop() // unchanged: not reported again
  await w.clock.settle()
  expect(kinds(w).sort()).toEqual(['session.background', 'session.start', 'session.title', 'session.tools'])

  restarted = true // the bridge restarted: it meets the session through the next heartbeat
  const from = w.reqs.length
  await w.clock.advance(15_000)
  await w.clock.settle()
  await stop()
  await w.clock.settle()
  expect(kinds({ reqs: w.reqs.slice(from) }).sort()).toEqual(['heartbeat', 'session.background', 'session.surfaces', 'session.title', 'session.tools'])
  expect(find({ reqs: w.reqs.slice(from) }, 'session.title')).toEqual(expect.objectContaining({ title: 'bob' }))
})

// ---------- what the page sends that doesn't enter ----------

const sayOnce = (text: string): ((r: Req) => Reply) => {
  let polls = 0
  return r => {
    if (r.url.includes('/api/inbox')) {
      polls++
      return { status: 200, text: polls === 1 ? JSON.stringify({ items: [{ kind: 'say', text }] }) : '{"items":[]}' }
    }
    return { status: 200, text: '{}' }
  }
}

test('a prompt from the page that a hook drops is reported dropped, with why, not sent', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, sayOnce('deploy it'))
  on('prompt.submit', () => ({ drop: 'blocked by a UserPromptSubmit hook: no deploys on Friday' }))

  await $.session.start(start)
  await w.clock.advance(100)
  await w.clock.settle()
  expect(find(w, 'chat.sent')).toBeUndefined()
  expect(find(w, 'chat.user')).toBeUndefined()
  expect(find(w, 'chat.dropped')).toEqual(
    expect.objectContaining({ text: 'deploy it', reason: 'blocked by a UserPromptSubmit hook: no deploys on Friday' }),
  )
})

test('a prompt from the page that fails to go in is reported dropped with the error', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, sayOnce('deploy it'))
  on('prompt.submit', () => {
    throw new Error('the prompt queue is full')
  })

  await $.session.start(start)
  await w.clock.advance(100)
  await w.clock.settle()
  expect(find(w, 'chat.sent')).toBeUndefined()
  const dropped = find(w, 'chat.dropped')
  expect(dropped?.text).toBe('deploy it')
  expect(typeof dropped?.reason === 'string' && dropped.reason.length > 0).toBe(true) // the engine's word for the failure
})

// ---------- waiting for the page: bounded, and the terminal can interrupt it ----------

test('a wait the bridge never answers: after 25 seconds the terminal asks', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, r =>
    r.url.endsWith('/api/ask') ? created : r.url.includes('/api/wait/') ? 'hang' : { status: 200, text: '{}' },
  )
  on('tool.check', () => ({ decision: 'ask', reason: 'ask a person' }))

  await $.session.start(start)
  let settled = false
  const verdict = $.tool.check(bash)
  void verdict.then(() => (settled = true))
  await w.clock.settle()
  await w.clock.advance(20_000)
  expect(settled).toBe(false) // the bridge answers a wait within 20 seconds: not yet
  await w.clock.advance(5_000)
  expect(await verdict).toEqual(expect.objectContaining({ decision: 'ask', reason: 'ask a person' }))
  // the card is withdrawn even though the bridge is now left alone for a while: nobody waits on it any more
  await w.clock.settle()
  expect(w.reqs.some(r => r.url.includes('/api/cancel/'))).toBe(true)
})

test('the page is told it waits: once, the first time a request goes there', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, answering('{"status":"done","answer":{"decision":"allow"}}'))
  on('tool.check', () => ({ decision: 'ask' }))

  await $.session.start(start)
  expect((await $.tool.check(bash)).decision).toBe('allow')
  expect((await $.tool.check({ ...bash, tool_use_id: 'tu-2' })).decision).toBe('allow')
  expect(w.toasts).toEqual(['symphony: waiting for your answer in the office page (http://127.0.0.1:7788); Esc cancels'])
})

test(
  'Esc while the page decides: the wait ends at once and the card is withdrawn',
  {
    plugins: [
      {
        // above the mod, as the terminal's interrupt: it settles first, and the mod's dispatch is abandoned
        name: 'esc',
        tier: 'prepend',
        register(on) {
          on('tool.check', async ($, e, next) => {
            void next(e).catch(() => undefined)
            await $.clock.sleep(1_000)
            return { decision: 'deny', reason: 'interrupted' }
          })
        },
      },
    ],
  },
  async ($, on) => {
    const w = world(on, { SYMPHONY_TOKEN: 'tok' }, r =>
      r.url.endsWith('/api/ask') ? created : r.url.includes('/api/wait/') ? 'hang' : { status: 200, text: '{}' },
    )
    on('tool.check', () => ({ decision: 'ask', reason: 'ask a person' }))

    await $.session.start(start)
    const verdict = $.tool.check(bash)
    await w.clock.settle()
    expect(w.reqs.some(r => r.url.endsWith('/api/wait/a1'))).toBe(true)
    await w.clock.advance(1_000)
    expect((await verdict).reason).toBe('interrupted')
    await w.clock.settle()
    expect(w.reqs.filter(r => r.url.endsWith('/api/cancel/a1')).length).toBe(1) // at once, not after the 20 s wait
  },
)

test('an ask the bridge takes only after the timeout is withdrawn when its answer comes', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, async r => {
    if (r.url.endsWith('/api/ask')) {
      await w.clock.sleep(12_000)
      return created
    }
    return { status: 200, text: '{}' }
  })
  on('tool.check', () => ({ decision: 'ask', reason: 'ask a person' }))

  await $.session.start(start)
  const verdict = $.tool.check(bash)
  await w.clock.settle()
  await w.clock.advance(10_000)
  expect(await verdict).toEqual(expect.objectContaining({ decision: 'ask', reason: 'ask a person' }))
  expect(w.reqs.filter(r => r.url.includes('/api/cancel/')).length).toBe(0)
  await w.clock.advance(2_000)
  await w.clock.settle()
  expect(w.reqs.filter(r => r.url.endsWith('/api/cancel/a1')).length).toBe(1)
})

test('the bridge restarted and lost the ask (the wait gets a 404): the ask is put back and the wait goes on', async ($, on) => {
  let asks = 0
  let waits = 0
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, r => {
    if (r.url.endsWith('/api/ask')) return { status: 200, text: JSON.stringify({ id: `a${++asks}` }) }
    if (r.url.includes('/api/wait/')) {
      waits++
      return waits === 1 ? { status: 404, text: '{"error":"no such ask"}' } : { status: 200, text: '{"status":"done","answer":{"decision":"allow"}}' }
    }
    return { status: 200, text: '{}' }
  })
  on('tool.check', () => ({ decision: 'ask' }))

  await $.session.start(start)
  expect((await $.tool.check(bash)).decision).toBe('allow')
  expect(w.reqs.filter(r => r.url.endsWith('/api/ask')).length).toBe(2)
  expect(w.reqs.filter(r => r.url.includes('/api/wait/')).map(r => r.url.split('/').at(-1))).toEqual(['a1', 'a2'])
  expect(w.reqs.filter(r => r.url.includes('/api/cancel/'))).toEqual([])
})

// ---------- what the page may decide ----------

test("the organization's ceiling holds: a page's allow can't pass it, a deny can", async ($, on) => {
  let answer = 'allow'
  world(on, { SYMPHONY_TOKEN: 'tok' }, r =>
    r.url.endsWith('/api/ask')
      ? created
      : r.url.endsWith('/api/wait/a1')
        ? { status: 200, text: JSON.stringify({ status: 'done', answer: { decision: answer } }) }
        : { status: 200, text: '{}' },
  )
  on('tool.check', () => ({ decision: 'ask', reason: 'ask a person', ceiling: 'ask' }))
  const capped = { ...bash, ceiling: 'ask' }

  await $.session.start(start)
  expect(await $.tool.check(capped)).toEqual(expect.objectContaining({ decision: 'ask', reason: 'ask a person' }))
  answer = 'deny'
  expect((await $.tool.check(capped)).decision).toBe('deny')
})

test('a call the page denied ends denied, though the call comes back only as an error', async ($, on) => {
  let answer = 'deny'
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, r =>
    r.url.endsWith('/api/ask')
      ? created
      : r.url.endsWith('/api/wait/a1')
        ? { status: 200, text: JSON.stringify({ status: 'done', answer: { decision: answer, reason: 'not now' } }) }
        : { status: 200, text: '{}' },
  )
  on('tool.check', () => ({ decision: 'ask' }))
  let ran = 0
  on('tool.call', () => {
    ran++
    return { result: 'exit 1', isError: true }
  })

  await $.session.start(start)
  // the engine asks the check inside the call; a deny comes back as an errored call, not { deny }
  const denied = await $.tool.call({ tool: 'Bash', command: 'touch x', tool_use_id: 'tu-1' })
  expect(denied.deny).toBeUndefined()
  expect(ran).toBe(0)
  answer = 'allow' // and a call the page allowed that fails on its own is no denial
  await $.tool.call({ tool: 'Bash', command: 'touch x', tool_use_id: 'tu-2' })
  expect(ran).toBe(1)
  await w.clock.settle()
  const ends = w.reqs.filter(r => r.body?.kind === 'tool.end').map(r => r.body)
  expect(ends).toEqual([
    expect.objectContaining({ toolUseId: 'tu-1', denied: true, isError: true }),
    expect.objectContaining({ toolUseId: 'tu-2', denied: false, isError: true }),
  ])
})

const questions = [{ question: 'Which colour?', header: 'Colour', options: [{ label: 'red', description: '' }, { label: 'blue', description: '' }], multiSelect: false }]

test('AskUserQuestion: the page answers it, and the answers become the tool result without the terminal dialog', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, answering('{"status":"done","answer":{"answers":{"Which colour?":"blue"}}}'))
  let dialog = 0
  on('tool.call', () => {
    dialog++
    return { result: { questions, answers: {} } }
  })
  on('tool.check', () => ({ decision: 'ask' }))

  await $.session.start(start)
  // the check passes it by: the question itself is the ask
  expect((await $.tool.check({ tool: 'AskUserQuestion', input: { questions }, tool_use_id: 'tu-q' })).decision).toBe('ask')
  expect(w.reqs.filter(r => r.url.endsWith('/api/ask')).length).toBe(0)
  const ran = await $.tool.call({ tool: 'AskUserQuestion', questions, tool_use_id: 'tu-q' })
  expect(ran.result).toEqual({ questions, answers: { 'Which colour?': 'blue' } })
  expect(dialog).toBe(0)
  expect(w.reqs.find(r => r.url.endsWith('/api/ask'))?.body).toEqual(expect.objectContaining({ kind: 'question', questions }))
})

const badAnswers: [string, string][] = [
  ['answers that are not an object', '{"status":"done","answer":{"answers":"blue"}}'],
  ['no answers', '{"status":"done","answer":{"decision":"allow"}}'],
  ['the card withdrawn', '{"status":"cancelled"}'],
]
for (const [what, wait] of badAnswers) {
  test(`AskUserQuestion with ${what}: the terminal's own dialog asks`, async ($, on) => {
    world(on, { SYMPHONY_TOKEN: 'tok' }, answering(wait))
    let dialog = 0
    on('tool.call', () => {
      dialog++
      return { result: { questions, answers: { 'Which colour?': 'red' } } }
    })

    await $.session.start(start)
    const ran = await $.tool.call({ tool: 'AskUserQuestion', questions, tool_use_id: 'tu-q' })
    expect(dialog).toBe(1)
    expect(ran.result).toEqual({ questions, answers: { 'Which colour?': 'red' } })
  })
}

// ---------- a bridge that takes requests and doesn't answer ----------

test('a bridge that takes requests and never answers: the reports behind are dropped at once, not each given its own timeout', async ($, on) => {
  let hung = false
  const w = world(
    on,
    { SYMPHONY_TOKEN: 'tok' },
    () => (hung ? 'hang' : { status: 200, text: '{}' }),
    undefined,
    () => (hung ? 'hang' : undefined),
  )
  on('tool.call', () => ({ result: {} }))

  await $.session.start(start)
  await w.clock.settle()
  hung = true // stopped with Ctrl-Z, say
  const from = w.all.length
  for (let i = 0; i < 100; i++) await $.tool.call({ tool: 'Bash', command: `echo ${i}` })
  await w.clock.advance(60_000)
  await w.clock.settle()
  // one report that hung, then a probe now and then (the heartbeat), not a check and a request per report
  expect(w.all.length - from).toBeLessThan(8)

  hung = false // and it goes on
  await w.clock.advance(15_000)
  await w.clock.settle()
  await $.tool.call({ tool: 'Bash', command: 'echo back' })
  await w.clock.settle()
  expect(w.reqs.filter(r => r.body?.kind === 'tool.start').at(-1)?.body).toEqual(expect.objectContaining({ summary: 'echo back' }))
})

test('a bridge that never answers the check: no symphony is started in its place', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, () => 'hang', () => ({ exitCode: 1 }))

  const started = $.session.start(start)
  await w.clock.settle()
  await w.clock.advance(5_000)
  await started
  expect(w.runs).toEqual([]) // something holds the address: no bridge could start there
  expect(w.toasts).toEqual(["symphony: can't reach the bridge (http://127.0.0.1:7788); approvals stay in the terminal for now"])
})

test(
  'the queue of reports is bounded: past 500, the oldest go',
  { timeoutMs: 30_000 },
  async ($, on) => {
    let open: (() => void) | undefined
    const gate = new Promise<void>(resolve => (open = resolve))
    const w = world(on, { SYMPHONY_TOKEN: 'tok' }, async r => {
      if (r.body?.kind === 'session.notify' && r.body?.text === 'n0') await gate // the bridge is slow with the first one
      return { status: 200, text: '{}' }
    })
    on('classic.Notification', () => ({}))

    await $.session.start(start)
    for (let i = 0; i < 600; i++) await $.classic.Notification({ message: `n${i}`, notification_type: 'idle_prompt' })
    open?.()
    await w.clock.settle()
    const notes = w.reqs.filter(r => r.body?.kind === 'session.notify').map(r => r.body?.text)
    expect(notes.length).toBe(501) // the one on its way, and the newest 500
    expect(notes[0]).toBe('n0')
    expect(notes[1]).toBe('n100')
    expect(notes.at(-1)).toBe('n599')
  },
)

test('/clear while the old session\'s inbox poll is still out: the new session polls for itself at once', async ($, on) => {
  let held = false
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, r => {
    // the bridge holds an ended session's long poll until its time is up
    if (held && r.url.includes('/api/inbox?session=sess-1')) return 'hang'
    return { status: 200, text: '{}' }
  })
  on('session.end', (_$, e) => ({ sessionId: e.sessionId }))
  on('classic.SessionStart', () => ({}))

  await $.session.start(start)
  held = true
  await w.clock.advance(100) // the next poll goes out and is held
  await w.clock.settle()
  expect(w.polls.at(-1)?.url).toContain('session=sess-1')
  await $.session.end({ reason: 'clear', sessionId: 'sess-1', resume: { id: 'sess-1' } })
  await $.classic.SessionStart({ source: 'clear', session_id: 'sess-2' })
  await w.clock.settle()
  expect(w.polls.at(-1)?.url).toContain('session=sess-2') // a kickoff the page queued for it comes at once
})
