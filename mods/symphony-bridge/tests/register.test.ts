// mod 的单元测试：claude plugin test mods/symphony-bridge
// 测试里的 on 挂在 mod 下面，替引擎和 bridge 回话；mod 发出的每个请求、弹的每条提示都记下来。
import type { On } from 'claude-code'
import { expect, mock, test } from 'claude-code/testing'

type Req = { url: string; method?: string; auth?: string; body?: Record<string, unknown> }
// 'down' 表示 bridge 没起，连不上
type Reply = { status: number; text: string } | 'down'

function world(on: On, env: Record<string, string>, reply: (r: Req) => Reply = () => ({ status: 200, text: '{}' })) {
  mock.env(on, env)
  const clock = mock.clock(on)
  const reqs: Req[] = []
  const toasts: string[] = []
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
  on('fs.read', () => ({ deny: '测试里不读文件' }))
  on('session.start', (_$, e) => ({ cwd: e.cwd }))
  return { reqs, toasts, clock }
}

const start = { cwd: '/work', surface: 'terminal', isInteractive: true } as const
const bash = { tool: 'Bash', input: { command: 'touch x' }, tool_use_id: 'tu-1' }

test('没带 SYMPHONY_TOKEN 的会话：不连 bridge、不弹提示、审批照旧', async ($, on) => {
  const w = world(on, { SYMPHONY_BRIDGE: 'http://bridge.test' })
  on('tool.check', () => ({ decision: 'ask', reason: '要问人' }))
  on('tool.call', () => ({ result: {} }))

  await $.session.start(start)
  expect((await $.tool.check(bash)).decision).toBe('ask')
  await $.tool.call({ tool: 'Bash', command: 'ls' })
  await w.clock.advance(60_000)

  expect(w.reqs).toEqual([])
  expect(w.toasts).toEqual([])
})

test('带 token 的会话：带着 token 报上线，之后每 15 秒一次心跳', async ($, on) => {
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

test('bridge 不认 token：弹一次提示，审批仍走终端', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'bad' }, () => ({ status: 401, text: '{"error":"不认识"}' }))
  on('tool.check', () => ({ decision: 'ask' }))

  await $.session.start(start)
  expect(w.toasts).toEqual(['symphony: bridge 不认这个 SYMPHONY_TOKEN，审批仍走终端'])
  expect((await $.tool.check(bash)).decision).toBe('ask')
})

test('连不上 bridge：提示一下，审批先走终端', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, () => 'down')
  on('tool.check', () => ({ decision: 'ask' }))

  await $.session.start(start)
  expect(w.toasts).toEqual(['symphony: 连不上 bridge（http://127.0.0.1:7788），审批先走终端'])
  expect((await $.tool.check(bash)).decision).toBe('ask')
})

test('带 token：要问人的调用挂到 bridge 上，网页批了就放行', async ($, on) => {
  const w = world(on, { SYMPHONY_TOKEN: 'tok' }, r =>
    r.url.endsWith('/api/ask')
      ? { status: 200, text: '{"id":"a1"}' }
      : r.url.endsWith('/api/wait/a1')
        ? { status: 200, text: '{"status":"done","answer":{"decision":"allow"}}' }
        : { status: 200, text: '{}' },
  )
  on('tool.check', () => ({ decision: 'ask', reason: '要问人' }))

  await $.session.start(start)
  expect((await $.tool.check(bash)).decision).toBe('allow')
  const ask = w.reqs.find(r => r.url.endsWith('/api/ask'))
  expect(ask?.body).toEqual(expect.objectContaining({ kind: 'permission', tool: 'Bash', session: 'sess-1' }))
})
