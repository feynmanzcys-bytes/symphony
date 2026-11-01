// The bridge's client, for every theme of the office page: it connects to the bridge (the first snapshot, then the
// stream), keeps the state as the bridge pushes it (docs/protocol.md), and makes the calls a page makes. No DOM in
// here: a theme listens for what changes and draws it its own way.
//
//   import { connect } from '/symphony.js'
//   const sym = connect()
//   sym.state                  the latest snapshot (accounts, sessions, asks, launches, tasks, ledger, ...) plus the message log
//   sym.first                  true until the first snapshot was applied (a theme places people instead of walking them in)
//   sym.on('snapshot', fn)     a snapshot was applied (every change in the office), fn(state)
//   sym.on('message', fn)      a message between sessions came or changed, fn(message, isNew)
//   sym.on('messages', fn)     the whole log was fetched again (the page had fallen behind)
//   sym.on('event', fn)        a mod event the bridge relayed, fn(event, accountName)
//   sym.on('chat', fn)         a chat item or a piece of streamed text, fn(msg) (msg.session, msg.item or msg.delta)
//   sym.on('usage', fn)        a session's usage figures, fn(sessionId, usage); the state's session is updated first
//   sym.on('ask' | 'settled' | 'launch' | 'task', fn)   the bridge's own notices; a snapshot follows each
//   sym.on('change', fn)       once after any of the above that changed the state (snapshot, message, messages, usage)
//   sym.on('protocol', fn)     once, when the bridge's protocol isn't this client's, fn(bridge, page): the page says so and goes on
//
// The calls answer { ok, status, data }, data being the bridge's JSON ({ error, code, params } when !ok; the page
// turns a code into text with errorText() from i18n.js).

export const PROTOCOL = 2 // the protocol this client speaks; the bridge's snapshot says its own

const KEEP_MESSAGES = 300

export function connect({ stream = true } = {}) {
  const listeners = new Map()
  const on = (type, fn) => { (listeners.get(type) ?? listeners.set(type, []).get(type)).push(fn); return () => off(type, fn) }
  const off = (type, fn) => { const l = listeners.get(type); if (l) l.splice(l.indexOf(fn) >>> 0, 1) }
  const emit = (type, ...args) => { for (const fn of listeners.get(type) ?? []) fn(...args) }

  const sym = {
    state: { protocol: 0, accounts: [], sessions: [], asks: [], messages: [], launches: [], tasks: [], ledger: [] },
    first: true,
    messagesVersion: 0, // goes up whenever the message log changes; panels redraw only then
    on, off,
    // ---------- lookups ----------
    sessionOf: acc => (acc?.session ? sym.state.sessions.find(s => s.id === acc.session) ?? null : null), // the desk session
    sessionById: id => (id ? sym.state.sessions.find(s => s.id === id) ?? null : null),
    accountById: id => sym.state.accounts.find(a => a.id === id) ?? null,
    taskById: id => sym.state.tasks.find(k => k.id === id) ?? null,
    isOnline: s => !!s && s.state !== 'ended',
    asking: sid => !!sid && sym.state.asks.some(a => a.session === sid), // the session waits for an approval or an answer
    // the task a session belongs to: by the session's own task, else the task that lists it as a member
    taskOfSession: sid => sym.taskById(sym.sessionById(sid)?.task) ?? sym.state.tasks.find(k => k.members.some(m => m.session === sid)) ?? null,
    // ---------- calls ----------
    post, get,
    say: (sid, text) => post(`/api/say/${sid}`, { text }),
    abort: sid => post(`/api/abort/${sid}`, {}),
    answer: (id, body) => post(`/api/answer/${id}`, body),
    createAccount: name => post('/api/accounts', { name }),
    deleteAccount: id => call('DELETE', `/api/accounts/${id}`),
    launch: req => post('/api/launch', req),
    dirs: path => get('/api/dirs?path=' + encodeURIComponent(path)),
    launchOptions,
    fetchChat: async sid => (await get(`/api/chat/${sid}`)).data?.items ?? [],
    fetchActivity: async () => (await get('/api/activity')).data?.items ?? [],
    fetchLedger: async () => (await get('/api/ledger')).data?.rows ?? [],
    openTerminal: launchId => post(`/api/terminal/${launchId}/open`),
    themes: () => get('/api/themes'),
    tasks: {
      create: body => post('/api/tasks', body),
      end: id => post(`/api/tasks/${id}/end`, {}),
      forget: id => call('DELETE', `/api/tasks/${id}`),
      say: (id, text) => post(`/api/tasks/${id}/say`, { text }),
      chat: async id => (await get(`/api/tasks/${id}/chat`)).data?.messages ?? [],
    },
  }

  async function call(method, path, body) {
    const init = { method }
    if (body !== undefined) { init.headers = { 'content-type': 'application/json' }; init.body = JSON.stringify(body) }
    let r
    try { r = await fetch(path, init) } catch { return { ok: false, status: 0, data: null } }
    const data = await r.json().catch(() => null)
    return { ok: r.ok, status: r.status, data }
  }
  function post(path, body = {}) { return call('POST', path, body) }
  function get(path) { return call('GET', path) }

  // /api/launch/options: whether the page can start sessions, and the drop-down choices; kept until asked again
  let opts = null
  async function launchOptions(refresh = false) {
    if (opts && !refresh) return opts
    const r = await get('/api/launch/options')
    opts = r.ok ? r.data : null
    return opts
  }

  // ---------- the message log ----------
  // A full snapshot (the first one, /api/state) carries it; the ones pushed on every change don't, and each new or
  // updated message comes as a "message" push instead. The snapshot names the latest message, so a page that missed
  // one (it fell behind) fetches the whole log again.
  const seen = new Set()
  function takeMessages(list) {
    sym.state.messages = list
    sym.messagesVersion++
    for (const m of list) noteMessage(m)
  }
  function noteMessage(m) {
    const isNew = !seen.has(m.id)
    seen.add(m.id)
    emit('message', m, isNew)
  }
  function applyMessage(m) {
    const list = sym.state.messages, i = list.findIndex(x => x.id === m.id)
    if (i >= 0) list[i] = m
    else { list.push(m); if (list.length > KEEP_MESSAGES) list.splice(0, list.length - KEEP_MESSAGES) }
    sym.messagesVersion++
    noteMessage(m)
    emit('change')
  }
  let catchingUp = false, warned = false
  async function catchUpMessages() {
    if (catchingUp) return
    catchingUp = true
    try {
      const r = await get('/api/state')
      if (r.ok && Array.isArray(r.data?.messages)) { takeMessages(r.data.messages); emit('messages'); emit('change') }
    } finally { catchingUp = false }
  }

  function applySnapshot(snap) {
    const full = Array.isArray(snap.messages) // the first snapshot and /api/state carry the log; the pushed ones don't
    const messages = full ? snap.messages : sym.state.messages
    sym.state = snap
    for (const k of ['accounts', 'sessions', 'asks', 'launches', 'tasks', 'ledger']) if (!Array.isArray(snap[k])) snap[k] = []
    snap.messages = messages
    if (full) takeMessages(messages)
    else if (snap.messageTail && !messages.some(m => m.id === snap.messageTail)) catchUpMessages()
    if (snap.protocol !== PROTOCOL && !warned) { warned = true; emit('protocol', snap.protocol, PROTOCOL) } // once: a bridge of another generation
    emit('snapshot', snap)
    sym.first = false
    emit('change')
  }

  function applyUsage(msg) {
    const s = sym.sessionById(msg.session)
    if (s) s.usage = msg.usage
    emit('usage', msg.session, msg.usage)
    emit('change')
  }

  function take(msg) {
    switch (msg.type) {
      case 'snapshot': return applySnapshot(msg)
      case 'event': return emit('event', msg.event, msg.title)
      case 'message': return applyMessage(msg.message)
      case 'chat': return emit('chat', msg)
      case 'usage': return applyUsage(msg)
      case 'ask': case 'settled': case 'launch': case 'task': return emit(msg.type, msg)
    }
  }
  sym.take = take // a theme or a test may feed messages by hand (made-up activity)

  // the first snapshot by request (it carries the message log), then the stream; the stream's own first message is a
  // full snapshot too, so whichever comes first wins and the other is just a newer state
  if (stream) {
    get('/api/state').then(r => { if (r.ok && r.data && sym.first) applySnapshot(r.data) })
    const es = new EventSource('/api/stream')
    es.onmessage = m => take(JSON.parse(m.data))
    sym.stream = es
  }
  return sym
}
