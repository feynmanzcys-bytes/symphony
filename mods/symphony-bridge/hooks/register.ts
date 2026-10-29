import type { EngineInterface, Register } from 'claude-code'

// 环境变量：
//   SYMPHONY_BRIDGE   bridge 地址，默认本机 7788
//   SYMPHONY_TOKEN    军团账号的 token：一个账号同时只给一个会话用，决定身份和座位
//   SYMPHONY_LAUNCH   从页面启动时 bridge 设的启动编号
// bridge 不通、不认 token 或账号已被占用时，一律退回 Claude Code 自己的终端交互，不会卡死会话。
const DEFAULT_BRIDGE = 'http://127.0.0.1:7788'

type Answer =
  | { decision: 'allow' | 'deny'; reason?: string }
  | { answers: Record<string, string> }

type Identity = { bridge: string; headers: Record<string, string>; login?: string }

let identity: Identity | undefined
let sessionId: string | undefined
let transcriptPath: string | undefined
// 每条事件都带上目录和启动编号：bridge 重启后不用等下一次启动就能认回这个会话
let sticky: { cwd?: string; launch?: string } = {}
// 上次报给 bridge 的模型和强度，变了才再报
let reported = { model: '', effort: '' }
let heartbeat: { cancel: () => void } | undefined
// 网页拒掉的调用。被拒的工具在 tool.call 里拿到的是一条报错结果，不是 { deny }，只能自己记着。
const deniedByWeb = new Set<string>()

// Claude 登录记录在 $CLAUDE_CONFIG_DIR/.claude.json，没设就在用户目录下（Windows 没有 HOME，用 USERPROFILE）。
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

async function ident($: EngineInterface) {
  if (identity) return identity
  const login = await loginEmail($)
  const token = await $.env.get('SYMPHONY_TOKEN')
  const headers: Record<string, string> = { 'content-type': 'application/json' }
  if (token) headers.authorization = `Bearer ${token}`
  identity = { bridge: (await $.env.get('SYMPHONY_BRIDGE')) || DEFAULT_BRIDGE, headers, login }
  return identity
}

async function sid($: EngineInterface) {
  return (sessionId ??= await $.session.id())
}

async function call($: EngineInterface, path: string, body?: unknown) {
  const { bridge, headers } = await ident($)
  return $.http.fetch(`${bridge}${path}`, {
    method: body === undefined ? 'GET' : 'POST',
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}

// 上报事件给办公室页面；失败就算了，不影响会话。
async function emit($: EngineInterface, kind: string, data: object = {}) {
  try {
    return await call($, '/api/event', { session: await sid($), kind, ...sticky, ...data })
  } catch {
    return undefined
  }
}

// 把一个待决事项挂到 bridge 上，长轮询等网页回复。
// hook 的 10 秒预算只算自己的代码时间，$.http.fetch 在途不计，所以可以一直等。
async function ask(
  $: EngineInterface,
  signal: AbortSignal,
  payload: object,
): Promise<Answer | undefined> {
  let id: string | undefined
  const create = async () => {
    const created = await call($, '/api/ask', { session: await sid($), ...payload })
    return created.ok ? (JSON.parse(created.text).id as string) : undefined
  }
  try {
    id = await create()
    if (!id) return undefined
    while (!signal.aborted) {
      const waited = await call($, `/api/wait/${id}`)
      // bridge 重启过，这件事丢了：重新挂上，接着等
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
  // 终端那边打断了（Esc 等），通知 bridge 撤掉卡片
  if (id) await call($, `/api/cancel/${id}`, {}).catch(() => {})
  return undefined
}

// 模型和思考强度决定办公室里小人的块头，变了才报。
async function reportModel($: EngineInterface, model: string, effort?: string | number) {
  const level = typeof effort === 'number' ? String(effort) : (effort ?? '')
  if (model === reported.model && level === reported.effort) return
  reported = { model, effort: level }
  await emit($, 'session.model', { model, effort: level || undefined })
}

// /rename 不带参数时名字是自动起的，命令返回里没有，只能去 transcript 里找最后一条 custom-title。
// 先直接读文件（各平台都行，上限 4 MiB）；太大读不了再用 grep（Windows 没有就算了，下次提交 prompt 时会补上）。
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
    const { login } = await ident($)
    // 从页面启动的会话带着 bridge 给的启动编号，bridge 靠它把会话和 tmux 对上
    sticky = { cwd: e.cwd, launch: (await $.env.get('SYMPHONY_LAUNCH')) || undefined }
    const model = await $.session.model()
    reported = { model, effort: '' }
    const res = await emit($, 'session.start', { interactive: e.isInteractive, login, model })
    if (res?.status === 401) $.ui.toast('symphony: 没有有效的军团账号（SYMPHONY_TOKEN），审批仍走终端')
    else if (res?.status === 409) $.ui.toast('symphony: 这个账号已经有会话在线，审批仍走终端')
    // 心跳：bridge 45 秒收不到任何消息就当这个会话下班了（比如直接关了终端）
    heartbeat?.cancel()
    heartbeat = $.clock.every(15_000, () => {
      void emit($, 'heartbeat')
    })
    return started
  })

  // 每次请求模型时看一眼实际用的模型和强度（只看主会话的）
  on('turn.step', async function* ($, e, next) {
    if (!('agentId' in e && e.agentId)) await reportModel($, e.model, e.effort)
    return yield* next(e)
  })

  // 会话名（--name 或 /rename 起的名字）：启动/恢复时、每次提交 prompt 时都带着当前名字，
  // /rename 本身再单独抓一次，改名立刻生效。
  on('classic.SessionStart', async ($, e, next) => {
    transcriptPath = e.transcript_path
    await emit($, 'session.title', { title: e.session_title })
    return next(e)
  })

  on('classic.UserPromptSubmit', async ($, e, next) => {
    transcriptPath = e.transcript_path
    await emit($, 'session.title', { title: e.session_title })
    return next(e)
  })

  on('command.run', { command: 'rename' }, async ($, e, next) => {
    const ran = await next(e)
    const title = e.args.trim() || (await titleFromTranscript($))
    await emit($, 'session.title', { title })
    return ran
  })

  // 会话间消息（SendMessage / agent team 邮箱）：发出和收到各报一次，bridge 负责配对。
  on('session.send', async ($, e, next) => {
    const sent = await next(e)
    await emit($, 'message.send', {
      to: e.to,
      text: e.text.slice(0, 2000),
      origin: e.origin.kind,
      agentId: e.agentId,
      delivered: sent.isDelivered,
    })
    return sent
  })

  on('session.receive', async ($, e, next) => {
    await emit($, 'message.receive', {
      text: e.text.slice(0, 2000),
      origin: e.origin,
      agentId: e.agentId,
    })
    return next(e)
  })

  on('session.end', async ($, e, next) => {
    heartbeat?.cancel()
    await emit($, 'session.end')
    return next(e)
  })

  on('turn.start', async ($, e, next) => {
    await emit($, 'turn.start', {
      turnId: e.turnId,
      text: e.text.slice(0, 200),
      agentId: 'agentId' in e ? e.agentId : undefined,
    })
    return next(e)
  })

  on('turn.complete', async ($, e, next) => {
    await emit($, 'turn.complete', {
      turnId: e.turnId,
      durationMs: e.durationMs,
      isAborted: e.isAborted,
      agentId: 'agentId' in e ? e.agentId : undefined,
    })
    return next(e)
  })

  // 权限审批：引擎判定为 ask 时，改由网页决定。
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
      reason: answer.reason || `symphony: 网页端${answer.decision === 'allow' ? '允许' : '拒绝'}`,
    }
  })

  // AskUserQuestion：不弹终端对话框，直接用网页上的回答作为工具结果。
  // 其它工具只做事件上报。
  on('tool.call', async ($, e, next) => {
    const call = { tool: e.tool, agentId: e.agentId }
    await emit($, 'tool.start', call)

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
    await emit($, 'tool.end', { ...call, denied, isError: ran.isError === true })
    return ran
  })
}
