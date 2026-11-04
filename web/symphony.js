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
//   sym.on('fault', fn)        the error log kept something (fn(msg): msg.fault, and msg.faults, the summary the state
//                              keeps as state.faults: how many, and when the newest happened) or was emptied
//   sym.on('change', fn)       once after any of the above that changed the state (snapshot, message, messages, usage, a session's last line)
//   sym.on('protocol', fn)     once, when the bridge's protocol isn't this client's, fn(bridge, page): the page says so and goes on
//   sym.on('connection', fn)   the stream came up (fn(true)) or was lost (fn(false)); it reconnects by itself
//   sym.on('resync', fn)       a full snapshot came after the first (the stream reconnected): pushes may have been missed,
//                              so what a page fetched on its own (a chat log, the activity) is worth fetching again
//   sym.on('pagekey', fn)      the bridge no longer takes this page's key (fn(true): it was made anew, the page is signed
//                              out and must be opened again from the symphony program), or takes it again (fn(false));
//                              sym.keyLost says which
//   sym.stayConnected(on)      keep the stream while the page is hidden (a page that must ring for a new ask)
//
// A page hidden for 10 seconds lets its stream go (a browser holds six connections per host, and every page would hold
// one) and asks for the state now and then instead; shown again, it opens the stream, whose first message is a full
// snapshot. The calls answer { ok, status, data }, data being the bridge's JSON ({ error, code, params } when !ok; the
// page turns a code into text with errorText() from i18n.js); a call that gets no answer in 15 seconds, or none at
// all, answers status 0 with the code 'unreachable'.
//
// The page's own errors (a script that throws, a promise nobody caught, what the content security policy blocks) go
// into the bridge's error log (POST /api/errors) as they happen, a few a minute at most; reportError(kind, text,
// detail) puts in one the page noticed itself (kind 'ui': an error it showed, once per page). The settings make a
// report file of the log.

export const PROTOCOL = 3 // the protocol this client speaks; the bridge's snapshot says its own

const KEEP_MESSAGES = 300 // of each task shown, and of the rest: as the bridge keeps them
const TIMEOUT = 15000 // a call without an answer in this long fails
const HIDDEN_CLOSE = 10000 // a page hidden this long lets its stream go...
const HIDDEN_POLL = 30000 // ...and asks for the state this often meanwhile (a tab's title counts the asks)
const RETRY = 3000 // after an answer that wasn't the stream (the browser gives up then), try again this soon
const KEY_RETRY = 10000 // a page whose key the bridge refuses tries again this rarely (another tab may sign in anew)

// request is one call to the bridge: { ok, status, data }. timeout 0 waits as long as it takes (a task's git work).
export async function request(method, path, body, { timeout = TIMEOUT } = {}) {
  const init = { method }
  if (body !== undefined) { init.headers = { 'content-type': 'application/json' }; init.body = JSON.stringify(body) }
  const ctl = new AbortController(), timer = timeout ? setTimeout(() => ctl.abort(), timeout) : 0
  init.signal = ctl.signal
  try {
    const r = await fetch(path, init)
    const data = await r.json().catch(() => null)
    return { ok: r.ok, status: r.status, data }
  } catch { // no answer: the bridge isn't there, or took too long
    return { ok: false, status: 0, data: { code: 'unreachable', error: "can't reach the bridge" } }
  } finally { clearTimeout(timer) }
}

// ---------- the page's own errors, into the bridge's error log ----------
const REPORTS_PER_MINUTE = 10, REPORT_AGAIN = 30000 // a page reports this many a minute at most, the same one this rarely
const reported = new Map() // what was reported -> when
let reports = []
export function reportError(kind, text, detail = '') {
  text = String(text ?? '').trim()
  if (!text) return
  const now = Date.now(), key = kind + '\n' + text
  // an error shown again (a window drawn anew) is the same one: once per page; a script's, every 30 s at most
  if (reported.has(key) && (kind === 'ui' || now - reported.get(key) < REPORT_AGAIN)) return
  reports = reports.filter(t => now - t < 60000)
  if (reports.length >= REPORTS_PER_MINUTE) return
  reports.push(now)
  reported.set(key, now)
  if (reported.size > 200) reported.delete(reported.keys().next().value)
  const page = location.pathname + location.search.replace(/([?&])key=[^&]*/g, '$1key=…')
  request('POST', '/api/errors', { kind, text: text.slice(0, 4000), detail: String(detail ?? '').slice(0, 6000), theme: document.documentElement.dataset.theme || '', page })
}
addEventListener('error', e => {
  if (!(e instanceof ErrorEvent)) return // (a resource that didn't load: its own request says why)
  reportError('js', e.message || String(e.error), [e.filename && `${e.filename}:${e.lineno}:${e.colno}`, e.error?.stack].filter(Boolean).join('\n'))
})
addEventListener('unhandledrejection', e => reportError('rejection', e.reason?.message ?? String(e.reason), e.reason?.stack || ''))
addEventListener('securitypolicyviolation', e => reportError('csp', `${e.violatedDirective} ${e.blockedURI || ''}`.trim(), e.sourceFile ? `${e.sourceFile}:${e.lineNumber}` : ''))

export function connect({ stream = true } = {}) {
  const listeners = new Map()
  const on = (type, fn) => { (listeners.get(type) ?? listeners.set(type, []).get(type)).push(fn); return () => off(type, fn) }
  const off = (type, fn) => { const l = listeners.get(type); if (l) l.splice(l.indexOf(fn) >>> 0, 1) }
  const emit = (type, ...args) => { for (const fn of listeners.get(type) ?? []) fn(...args) }

  const sym = {
    state: { protocol: 0, accounts: [], sessions: [], asks: [], messages: [], launches: [], tasks: [], ledger: [], faults: { count: 0, last: null } },
    first: true,
    messagesVersion: 0, // goes up whenever the message log changes; panels redraw only then
    on, off,
    // ---------- lookups ----------
    sessionOf: acc => (acc?.session ? sym.state.sessions.find(s => s.id === acc.session) ?? null : null), // the desk session
    // an account's desk session for its window: the one online, else the last one the bridge still has, so an employee
    // off duty shows its last session's chat, its end and why
    deskOf: acc => sym.sessionOf(acc) ?? (acc ? sym.state.sessions.filter(s => s.account === acc.id && !s.task).sort((a, b) => Date.parse(b.updated) - Date.parse(a.updated))[0] ?? null : null),
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
    setModel: (sid, change) => post(`/api/sessions/${sid}/model`, change), // {model?, effort?}: a running session's, through /model and /effort
    answer: (id, body) => post(`/api/answer/${id}`, body),
    createAccount: name => post('/api/accounts', { name }),
    deleteAccount: id => call('DELETE', `/api/accounts/${id}`),
    // the employees page: makes and changes employees in one go, all of it or none ({created, updated}; the entries:
    // {name, dir?, model?, effort?, permissionMode?, context?}, a name that isn't an employee's a new one)
    importStaff: employees => post('/api/accounts/import', { employees }),
    launch: req => post('/api/launch', req, { timeout: 60000 }), // the bridge checks the mod first (claude plugin list)
    dirs: path => get('/api/dirs?path=' + encodeURIComponent(path)),
    launchOptions,
    // the bridge's own settings: {botHome, botHomeDefault}; saving takes the fields to change (botHome '' is the default)
    settings: () => get('/api/settings'),
    saveSettings: changes => post('/api/settings', changes),
    // null when the bridge didn't answer with the list
    fetchChat: async sid => { const r = await get(`/api/chat/${sid}`); return r.ok ? r.data?.items ?? [] : null },
    fetchActivity: async () => { const r = await get('/api/activity'); return r.ok ? r.data?.items ?? [] : null },
    fetchLedger: async () => { const r = await get('/api/ledger'); return r.ok ? r.data?.rows ?? [] : null },
    openTerminal: launchId => post(`/api/terminal/${launchId}/open`),
    themes: () => get('/api/themes'),
    // the error log: what it keeps, what a report says besides (the bridge, its log's last lines), emptied
    errors: () => get('/api/errors'),
    diagnostics: () => get('/api/diagnostics', { timeout: 60000 }), // (whether it can start sessions: claude plugin list)
    clearErrors: () => call('DELETE', '/api/errors'),
    reportError,
    tasks: {
      create: body => post('/api/tasks', body, { timeout: 0 }), // git readies every member's worktree first
      end: id => post(`/api/tasks/${id}/end`, {}),
      forget: id => call('DELETE', `/api/tasks/${id}`),
      say: (id, text) => post(`/api/tasks/${id}/say`, { text }),
      chat: async id => (await get(`/api/tasks/${id}/chat`)).data?.messages ?? [],
    },
  }

  const call = request
  function post(path, body = {}, opts) { return call('POST', path, body, opts) }
  function get(path, opts) { return call('GET', path, undefined, opts) }

  // /api/launch/options: whether the page can start sessions, and the drop-down choices; kept until asked again
  let opts = null
  async function launchOptions(refresh = false) {
    if (opts && !refresh) return opts
    const r = await get('/api/launch/options', { timeout: 60000 })
    opts = r.ok ? r.data : null
    return opts
  }

  // ---------- the message log ----------
  // A full snapshot (the first one, /api/state) carries it; the ones pushed on every change don't, and each new or
  // updated message comes as a "message" push instead. The snapshot names the latest message, so a page that missed
  // one (it fell behind) fetches the whole log again.
  const seen = new Set() // the ids of the messages in the log: one not among them is new (forgotten as the log lets it go)
  function takeMessages(list) {
    sym.state.messages = list
    sym.messagesVersion++
    for (const m of list) noteMessage(m)
    if (seen.size > list.length) { const ids = new Set(list.map(m => m.id)); for (const id of seen) if (!ids.has(id)) seen.delete(id) }
  }
  function noteMessage(m) {
    const isNew = !seen.has(m.id)
    seen.add(m.id)
    emit('message', m, isNew)
  }
  function applyMessage(m) {
    const list = sym.state.messages, i = list.findIndex(x => x.id === m.id)
    if (i >= 0) list[i] = m
    else { list.push(m); trimMessages(list) }
    sym.messagesVersion++
    noteMessage(m)
    emit('change')
  }
  // as the bridge keeps the log: the newest KEEP_MESSAGES of each task shown (running, ending, failed: the snapshot's),
  // and as many of the rest together (between desks, of tasks gone); the oldest go first, the order stays
  function trimMessages(list) {
    if (list.length <= KEEP_MESSAGES) return
    const shown = new Set(sym.state.tasks.map(k => k.id)), count = new Map(), kept = []
    for (let i = list.length - 1; i >= 0; i--) {
      const group = shown.has(list[i].task) ? list[i].task : '', n = count.get(group) || 0
      if (n < KEEP_MESSAGES) { count.set(group, n + 1); kept.push(list[i]) }
    }
    if (kept.length < list.length) {
      const keep = new Set(kept.map(m => m.id))
      for (const m of list) if (!keep.has(m.id)) seen.delete(m.id)
      list.splice(0, list.length, ...kept.reverse())
    }
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

  // quiet: a full snapshot the page asked for while its stream was closed (no resync: the stream's will come)
  function applySnapshot(snap, quiet = false) {
    const full = Array.isArray(snap.messages) // the first snapshot and /api/state carry the log; the pushed ones don't
    const resync = full && !sym.first && !quiet
    const messages = full ? snap.messages : sym.state.messages
    sym.state = snap
    for (const k of ['accounts', 'sessions', 'asks', 'launches', 'tasks', 'ledger']) if (!Array.isArray(snap[k])) snap[k] = []
    if (!snap.faults) snap.faults = { count: 0, last: null } // (a bridge before the error log)
    snap.messages = messages
    if (full) takeMessages(messages)
    else if (snap.messageTail && !messages.some(m => m.id === snap.messageTail)) catchUpMessages()
    if (snap.protocol !== PROTOCOL && !warned) { warned = true; emit('protocol', snap.protocol, PROTOCOL) } // once: a bridge of another generation
    emit('snapshot', snap)
    if (resync) emit('resync')
    sym.first = false
    emit('change')
  }

  function applyUsage(msg) {
    const s = sym.sessionById(msg.session)
    if (s) s.usage = msg.usage
    emit('usage', msg.session, msg.usage)
    emit('change')
  }
  // the conversation's newest line (session.last), pushed without a snapshot like the usage figures
  function applyLast(msg) {
    const s = sym.sessionById(msg.session)
    if (s) s.last = msg.text
    emit('change')
  }

  function take(msg) {
    switch (msg.type) {
      case 'snapshot': return applySnapshot(msg)
      case 'event': return emit('event', msg.event, msg.title)
      case 'message': return applyMessage(msg.message)
      case 'chat': return emit('chat', msg)
      case 'usage': return applyUsage(msg)
      case 'last': return applyLast(msg)
      case 'ask': case 'settled': case 'launch': case 'task': return emit(msg.type, msg)
      case 'fault': { // (a summary older than the one the page has, a push overtaken by a snapshot, changes nothing; one
        // that says the log was emptied always does)
        const had = sym.state.faults, f = msg.faults
        if (f && (!f.count || !had?.last || !f.last || Date.parse(f.last) >= Date.parse(had.last))) sym.state.faults = f
        emit('fault', msg)
        return emit('change')
      }
    }
  }
  sym.take = take // a theme or a test may feed messages by hand (made-up activity)

  // ---------- the stream ----------
  // The first snapshot by request (it carries the message log), then the stream; the stream's own first message is a
  // full snapshot too, so whichever comes first wins and the other is just a newer state. The browser reconnects a
  // stream that broke off by itself (its first message: a full snapshot again); one that got an answer other than the
  // stream (the bridge restarting, a 401) it gives up, and this opens it again a little later.
  let es = null, up = null, retry = 0, hideTimer = 0, poll = 0, keep = false, gen = 0
  const setUp = v => { if (up !== v) { up = v; emit('connection', v) } }
  sym.keyLost = false
  const setKeyLost = v => { if (sym.keyLost !== v) { sym.keyLost = v; emit('pagekey', v) } }
  // an answer of the bridge's that says whether it takes this page's key: a 401 page_key means it doesn't (the key file
  // was deleted or damaged and the bridge made a new one), any answer of its own that isn't that means it does
  const noteKey = r => { if (r.status === 401 && r.data?.code === 'page_key') setKeyLost(true); else if (r.status) setKeyLost(false) }
  function open() {
    if (es) return
    clearTimeout(retry); retry = 0
    clearInterval(poll); poll = 0
    const src = es = sym.stream = new EventSource('/api/stream'), mine = ++gen
    src.onopen = () => { setUp(true); setKeyLost(false) }
    src.onmessage = m => take(JSON.parse(m.data))
    src.onerror = () => {
      if (src !== es) return
      setUp(false)
      if (src.readyState !== EventSource.CLOSED) return
      // the browser gives up on an answer that isn't the stream: the bridge restarting, or refusing the key; ask which
      es = sym.stream = null
      get('/api/state').then(r => {
        noteKey(r)
        if (mine === gen && !es && !poll && !retry) retry = setTimeout(open, sym.keyLost ? KEY_RETRY : RETRY)
      })
    }
  }
  // hidden a while: the stream goes (not a lost connection: no 'connection' false), the state comes now and then
  function sleep() {
    if (!es && !retry) return
    es?.close(); es = sym.stream = null
    clearTimeout(retry); retry = 0
    gen++
    poll = setInterval(async () => {
      const r = await get('/api/state')
      noteKey(r)
      if (poll && r.ok && r.data) applySnapshot(r.data, true)
    }, HIDDEN_POLL)
  }
  function visibility() {
    clearTimeout(hideTimer)
    if (document.visibilityState !== 'hidden') open()
    else if (keep) open()
    else hideTimer = setTimeout(sleep, HIDDEN_CLOSE)
  }
  sym.stayConnected = on => { keep = !!on; if (stream) visibility() }
  if (stream) {
    get('/api/state').then(r => { noteKey(r); if (r.ok && r.data && sym.first) applySnapshot(r.data) })
    open()
    document.addEventListener('visibilitychange', visibility)
    visibility()
  }
  return sym
}
