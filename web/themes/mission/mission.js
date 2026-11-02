// Mission Control: a control room's wall. One tile per session online (desk sessions and task members alike), a dim
// one per account off duty: the name, the state in a word with its color, the model and effort, gauges for the context
// window's fill and the rate-limit windows, a sparkline of the tokens over the last minute, what runs now. A session
// waiting for you is an alarm: its tile flashes, the bar's alarm count goes red, a click opens its window (the shared
// employee window). The activity log runs along the bottom as a ticker. The shared pieces: the employee window, the
// launch dialog, the task dialog and window.
import { t, lang, errorText, applyI18n, langPicker } from '/i18n.js'
import { esc } from '/asks.js'
import { connect } from '/symphony.js'
import { STATE_COLOR, GRAY, base, lower, buildText, statusText as statusOf, deferWhilePressed } from '/ui/common.js'
import { connectionBanner } from '/ui/banner.js'
import { createChat } from '/ui/chat.js'
import { createLaunchDialog } from '/ui/launch.js'
import { createTaskUI } from '/ui/task.js'
import { themePicker } from '/ui/themes.js'

const sym = connect()
let snapshot = sym.state
// every snapshot, frames or not (a hidden page draws none): the alarm bell, and the tab's title counting the alarms
sym.on('snapshot', s => { snapshot = s; ring(s.asks.length); document.title = (s.asks.length ? `(${s.asks.length}) ` : '') + t('mc.title') })
const statusText = s => statusOf(sym, s)
applyI18n()
connectionBanner(sym)
const prefs = document.querySelector('.bar .prefs'), langSel = langPicker()
prefs.appendChild(langSel)
themePicker(sym).then(sel => prefs.insertBefore(sel, langSel))

// ---------- figures ----------
const online = s => sym.isOnline(s)
const fmtUsd = n => '$' + (n >= 100 ? n.toFixed(0) : n >= 10 ? n.toFixed(1) : n.toFixed(2))
const fmtNum = new Intl.NumberFormat(lang, { notation: 'compact', maximumFractionDigits: 1 })
const fmtTime = when => new Date(when).toLocaleTimeString(lang, { hour: '2-digit', minute: '2-digit', second: '2-digit' })
const stateOf = s => !online(s) ? 'ended' : sym.asking(s.id) ? 'waiting' : s.state
const ORDER = { waiting: 0, tool: 1, thinking: 1, idle: 2, ended: 3, done: 3 }
const STATE_WORD_SHORT = { idle: 'mc.state.idle', thinking: 'mc.state.thinking', tool: 'mc.state.tool', waiting: 'mc.state.waiting', ended: 'mc.state.off' }
// the rate-limit windows, short: the ones known by name, any other as Claude Code calls it
const LIMIT_KEY = { five_hour: 'mc.limit.fiveHour', seven_day: 'mc.limit.sevenDay', spend_limit: 'mc.limit.spend' }
const limitLabel = kind => LIMIT_KEY[kind] ? t(LIMIT_KEY[kind]) : String(kind).replace(/_/g, ' ').toUpperCase()
// the task colors: one per task, by its number
const TASK_HUES = [36, 200, 290, 150, 330, 60, 250, 100]
const taskColor = id => `hsl(${TASK_HUES[(+String(id).slice(1) || 0) % TASK_HUES.length]} 80% 60%)`

// ---------- the tokens over the last minute: from the usage pushes (the totals), per session ----------
const SPARK_SECS = 60, BIN = 5
const tokenLog = new Map() // session id -> [{ at, total }]
const totalOf = u => Object.values(u?.totals || {}).reduce((n, tk) => n + tk.input + tk.output, 0)
sym.on('usage', (sid, usage) => {
  const log = tokenLog.get(sid) ?? tokenLog.set(sid, []).get(sid)
  log.push({ at: Date.now(), total: totalOf(usage) })
  while (log.length && log[0].at < Date.now() - 2 * SPARK_SECS * 1000) log.shift()
})
function bins(sid) {
  const log = tokenLog.get(sid) || [], now = Date.now(), out = new Array(SPARK_SECS / BIN).fill(0)
  for (let i = 1; i < log.length; i++) {
    const k = out.length - 1 - Math.floor((now - log[i].at) / (BIN * 1000))
    if (k >= 0 && k < out.length) out[k] += Math.max(0, log[i].total - log[i - 1].total)
  }
  return out
}
function spark(values) {
  const W = 100, H = 30, max = Math.max(1, ...values), n = values.length
  const x = i => (i * W / (n - 1)).toFixed(1), y = v => (H - 2 - v * (H - 4) / max).toFixed(1)
  const pts = values.map((v, i) => `${x(i)},${y(v)}`).join(' ')
  return `<svg viewBox="0 0 ${W} ${H}" preserveAspectRatio="none"><polygon class="area" points="0,${H} ${pts} ${W},${H}"/><polyline class="line" points="${pts}"/></svg>` +
    `<span class="lbl">${esc(t('mc.tokPerMin', { n: fmtNum.format(values.reduce((a, b) => a + b, 0)) }))}</span>`
}

// ---------- the shared pieces ----------
let chatFor = null, chatSid = null
const askWho = a => `<div class="who"><span class="name">${esc(t(a.kind === 'question' ? 'asks.question' : 'asks.permission'))}</span>` + (a.agentId ? `<span class="kind">${esc(t('common.subagent'))}</span>` : '') + `</div>`
const chat = createChat(sym, { askWho, onClose: () => { chatFor = null; chatSid = null }, onTaskChip: id => tasks.openWindow(id) })
const launch = createLaunchDialog(sym, { options: loadOptions, onOnline: l => openChat(l.account), onOpen: () => chat.close(), log: () => {} })
const tasks = createTaskUI(sym, {
  options: loadOptions,
  roomLabel: task => task.title.split('\n')[0].slice(0, 48),
  memberAvatar: m => avatarURL(m?.name || '?'), youAvatar: () => avatarURL(t('task.you')),
  isMemberOpen: (task, m) => !chat.hidden && chatSid === m.session,
  onOpenMember: (task, m) => openChat(m.account, m.session, task.id),
  onWindowOpen: () => chat.close(), onWindowClose: () => chat.close(), onDialogOpen: () => { chat.close(); tasks.closeWindow() },
  log: () => {},
})
chat.el.classList.add('stacked')
function avatarURL(name) {
  const h = [...name].reduce((n, c) => (n * 31 + c.codePointAt(0)) >>> 0, 7), hue = h % 360
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 24"><rect x="6" y="1" width="36" height="22" rx="4" fill="hsl(${hue} 60% 45%)"/><text x="24" y="16.5" text-anchor="middle" font-family="monospace" font-size="12" font-weight="700" fill="#fff">${esc([...name].slice(0, 3).join('').toUpperCase())}</text></svg>`
  return `url("data:image/svg+xml,${encodeURIComponent(svg)}")`
}
let launchOpts = null
async function loadOptions() {
  launchOpts = await sym.launchOptions(true)
  document.getElementById('new-emp').hidden = !launchOpts?.enabled
  document.getElementById('new-task').hidden = !launchOpts?.enabled
  const note = document.getElementById('launch-note')
  note.hidden = !launchOpts || launchOpts.enabled
  if (!note.hidden) {
    const fix = launchOpts.mod?.fix, reason = launchOpts.mod ? errorText(launchOpts.mod) : t('error.launch_disabled')
    note.innerHTML = esc(t(fix ? 'launch.offWithFix' : 'launch.off', { reason })) + (fix ? `<code>${esc(fix)}</code>` : '')
  }
  render()
  return launchOpts
}
document.getElementById('new-emp').onclick = () => launch.open(null)
document.getElementById('new-task').onclick = () => tasks.openDialog({})

function openChat(id, sid = null, taskId = null) {
  if (!taskId) tasks.closeWindow()
  chatFor = id; chatSid = sid
  showChat()
  chat.focus()
}
function showChat() {
  if (!chatFor) return
  const acc = sym.accountById(chatFor)
  if (!acc) return chat.close()
  const s = chatSid ? sym.sessionById(chatSid) : sym.deskOf(acc), on = online(s)
  const task = chatSid ? sym.taskOfSession(chatSid) : snapshot.tasks.find(k => k.members.some(m => m.account === chatFor && online(sym.sessionById(m.session)))) ?? null
  const member = task?.members.find(m => chatSid ? m.session === chatSid : m.account === chatFor)
  const busy = on && (s.state === 'thinking' || s.state === 'tool' || s.state === 'waiting')
  const buttons = []
  if (on && s.launch) buttons.push({ key: 'terminal', text: t('chat.terminal'), cls: 'terminal', onclick: async () => { const r = await sym.openTerminal(s.launch); if (!r.ok) alert(errorText(r.data) + (r.data?.params?.command ? '\n' + r.data.params.command : '')) }, title: t('chat.terminalTitle') })
  if (busy) buttons.push({ key: 'stop', text: t('chat.stop'), cls: 'stop', onclick: async () => { const r = await sym.abort(s.id); if (!r.ok) alert(errorText(r.data)) }, title: t('chat.stopTitle') })
  if (!chatSid && task && member?.session) buttons.push({ key: 'to-task', text: t('task.openMember', { name: member.name }), cls: 'primary to-task', onclick: () => { tasks.openWindow(task.id); openChat(chatFor, member.session, task.id) } })
  if (!on && !chatSid && launchOpts?.enabled) buttons.push({ key: 'go-work', text: t('launch.go'), cls: 'primary go-work', onclick: () => launch.open(acc.id) })
  if (!on && !chatSid && !task) buttons.push({ key: 'delete', text: t('card.delete'), cls: '', onclick: async () => {
    if (!confirm(t('card.deleteConfirm', { name: acc.name }))) return
    const r = await sym.deleteAccount(acc.id)
    if (!r.ok) alert(t('card.deleteFailed', { reason: errorText(r.data) }))
    chat.close()
  } })
  chat.show({
    name: chatSid ? member?.name || s?.title || acc.name : acc.name,
    state: stateOf(s), line: statusText(on ? s : null),
    taskChip: task ? { id: task.id, title: task.title, label: `${task.id} · ${task.title.split('\n')[0].slice(0, 40)}` } : null,
    chips: [['model', on ? buildText(s) : ''], ['session', s?.title], ['dir', s?.cwd || (chatSid ? member?.worktree : acc.last?.dir || acc.dir)], ['tools', on ? s.tools : ''], ['login', s?.login]],
    buttons, buttonsKey: `${chatFor}|${chatSid}|${on}|${busy}|${on && !!s.launch}|${!!launchOpts?.enabled}|${task?.id}|${member?.session}`,
    session: chatSid || s?.id || null, online: on,
    offText: chatSid ? t('task.offline') : task && member ? t('chat.awayInTask', { id: task.id, name: member.name }) : t('chat.offline'),
  })
}

// ---------- the wall ----------
const wall = document.getElementById('wall'), findEl = document.getElementById('find')
findEl.addEventListener('input', () => renderWall()) // at once: the box narrows the wall as one types
// the tiles: every session online (a desk's, a task member's), then every account off duty
function tiles() {
  const q = lower(findEl.value.trim()), out = []
  for (const acc of snapshot.accounts.filter(a => !a.advisor)) {
    const s = sym.sessionOf(acc)
    if (online(s)) out.push({ key: acc.id, name: acc.name, acc, s, task: null })
  }
  for (const k of snapshot.tasks) for (const m of k.members) {
    const s = sym.sessionById(m.session)
    if (online(s)) out.push({ key: k.id + '/' + m.account, name: m.name, acc: sym.accountById(m.account), s, task: k, member: m })
  }
  for (const acc of snapshot.accounts.filter(a => !a.advisor)) if (!online(sym.sessionOf(acc))) out.push({ key: acc.id, name: acc.name, acc, s: null, task: null })
  return out.filter(x => !q || lower(x.name).includes(q) || lower(x.s?.title).includes(q) || lower(base(x.s?.cwd || '')).includes(q) || lower(x.task?.id).includes(q))
    .sort((a, b) => ORDER[stateOf(a.s)] - ORDER[stateOf(b.s)] || a.name.localeCompare(b.name, lang))
}
// the grid that fits: as many columns as keep the tiles nearest a 3:2 shape, and every row on screen
function fit(n) {
  const W = wall.clientWidth || innerWidth, H = wall.clientHeight || innerHeight - 100
  let best = 1, score = Infinity
  for (let cols = 1; cols <= Math.max(1, n); cols++) {
    const rows = Math.ceil(n / cols), w = W / cols, h = H / rows, d = Math.abs(Math.log((w / h) / 1.5))
    if (d < score) { score = d; best = cols }
  }
  const rows = Math.ceil(n / best)
  wall.style.gridTemplateColumns = `repeat(${best}, minmax(0, 1fr))`
  wall.style.gridTemplateRows = `repeat(${rows}, minmax(0, 1fr))`
  const h = H / rows
  return h < 90 ? 'tiny' : h < 140 ? 'small' : ''
}
// (not while a pointer is down: a tile replaced under a press loses its click)
const renderWall = deferWhilePressed(() => {
  const list = tiles()
  if (!list.length) { wall.style.gridTemplateColumns = '1fr'; wall.innerHTML = `<div class="empty">${esc(t('mc.noSessions'))}</div>`; return }
  const size = fit(list.length)
  const keep = new Set(list.map(x => x.key))
  for (const el of [...wall.children]) if (!keep.has(el.dataset.key)) el.remove()
  let prev = null
  for (const x of list) {
    const st = stateOf(x.s), s = x.s, on = online(s)
    const ctx = s?.usage?.context?.percent, limits = (s?.usage?.rateLimits || []).slice(0, 2)
    const now = !on ? t('state.ended') : st === 'waiting' ? t('mc.alarm', { what: askText(s) }) : s.state === 'tool' && s.tool ? s.tool + (s.prompt ? ' · ' + s.prompt : '') : s.state === 'thinking' ? t('state.thinking') + (s.prompt ? ' · ' + s.prompt : '') : s.prompt ? s.prompt : t('state.idle')
    const sig = `${st}|${size}|${on}|${s?.model}|${s?.effort}|${ctx}|${limits.map(l => l.kind + l.percentUsed).join()}|${now}|${x.task?.id}|${x.name}`
    let el = wall.querySelector(`[data-key="${CSS.escape(x.key)}"]`)
    if (!el || el.dataset.sig !== sig) {
      const fresh = document.createElement('div')
      fresh.className = `tile ${st === 'ended' ? 'off' : st} ${x.task ? 'member' : ''} ${size}`
      fresh.dataset.key = x.key; fresh.dataset.sig = sig; fresh.dataset.account = x.acc?.id || ''; fresh.dataset.session = s?.id || ''
      if (x.task) fresh.dataset.task = x.task.id
      fresh.innerHTML = (x.task ? `<div class="stripe" style="--c:${taskColor(x.task.id)}" title="${esc(x.task.title)}"><span>${esc(x.task.id)}</span></div>` : '') +
        `<div class="hd"><span class="nm" title="${esc(x.name)}">${esc(x.name)}</span><span class="st" style="--c:${STATE_COLOR[st] ?? GRAY}"><i></i>${esc(t(STATE_WORD_SHORT[st] ?? 'mc.state.off'))}</span></div>` +
        `<div class="model">${esc(on && s.model ? s.model.replace(/^claude-/, '') + (s.effort ? ' · ' + s.effort : '') : x.acc?.last?.model || '')}</div>` +
        (on ? `<div class="gauges">` +
          (typeof ctx === 'number' ? `<span class="lbl">${esc(t('mc.ctx'))}</span><span class="gauge${ctx >= 90 ? ' full' : ctx >= 70 ? ' high' : ''}"><i style="width:${Math.min(100, ctx)}%"></i></span><span class="val">${Math.round(ctx)}%</span>` : '') +
          limits.map(l => `<span class="lbl">${esc(limitLabel(l.kind))}</span><span class="gauge${l.percentUsed >= 90 ? ' full' : l.percentUsed >= 70 ? ' high' : ''}"><i style="width:${Math.min(100, l.percentUsed)}%"></i></span><span class="val">${Math.round(l.percentUsed)}%</span>`).join('') + `</div>` +
          `<div class="spark">${spark(bins(s.id))}</div>` : '') +
        `<div class="now" title="${esc(now)}">${esc(now)}</div>`
      if (el) el.replaceWith(fresh); else wall.appendChild(fresh)
      el = fresh
    } else if (on) { // only the minute's line moves
      const sp = el.querySelector('.spark')
      if (sp) sp.innerHTML = spark(bins(s.id))
    }
    if ((prev ? prev.nextSibling : wall.firstChild) !== el) { if (prev) prev.after(el); else wall.prepend(el) }
    prev = el
  }
})
const askText = s => { const a = snapshot.asks.find(a => a.session === s.id); return !a ? '' : a.kind === 'question' ? (a.questions?.[0]?.question || t('asks.question')) : [a.tool, a.input?.command || a.input?.file_path || ''].filter(Boolean).join(' ') }
wall.addEventListener('click', ev => {
  const stripe = ev.target.closest('.stripe')
  if (stripe) return tasks.openWindow(stripe.parentElement.dataset.task)
  const tile = ev.target.closest('.tile')
  if (!tile) return
  if (tile.dataset.task) { tasks.openWindow(tile.dataset.task); openChat(tile.dataset.account, tile.dataset.session, tile.dataset.task) } else openChat(tile.dataset.account)
})
addEventListener('resize', render)

// ---------- the bar: mission time, sessions, alarms, cost ----------
// The bell: two short tones when the alarms go up (not for the ones there when the page opens). With the sound on, the
// page keeps its stream while hidden, so the bell rings in a tab in the background too. A browser lets a page play
// sound only once it was clicked or typed in: until then the switch says so.
let alarmsSeen = 0, sound = false
const SOUND_STORE = 'symphony.mission.sound'
try { sound = localStorage.getItem(SOUND_STORE) === '1' } catch {}
const soundBtn = document.getElementById('sound')
let ac = null // the page's one AudioContext, made (and let play) by the first click or key
function wake() {
  try {
    ac ??= new AudioContext()
    ac.onstatechange = showSound
    if (ac.state === 'suspended') ac.resume().catch(() => {})
  } catch {}
  showSound()
}
addEventListener('pointerdown', wake, true)
addEventListener('keydown', wake, true)
const canPlay = () => ac?.state === 'running'
function showSound() {
  const blocked = sound && !canPlay()
  soundBtn.textContent = sound ? '🔔' : '🔕'
  soundBtn.classList.toggle('blocked', blocked)
  soundBtn.title = t(!sound ? 'mc.soundOff' : blocked ? 'mc.soundBlocked' : 'mc.soundOn')
}
function setSound(on) {
  sound = on
  showSound()
  sym.stayConnected(on)
  try { localStorage.setItem(SOUND_STORE, on ? '1' : '0') } catch {}
}
setSound(sound)
soundBtn.onclick = () => { wake(); setSound(!sound); if (sound) beep() }
function beep() { // two short tones, nothing loaded: the page's own
  if (!canPlay()) return
  try {
    const g = ac.createGain()
    g.gain.value = 0.08; g.connect(ac.destination)
    for (const [f, at] of [[880, 0], [1320, 0.16]]) { const o = ac.createOscillator(); o.type = 'square'; o.frequency.value = f; o.connect(g); o.start(ac.currentTime + at); o.stop(ac.currentTime + at + 0.12) }
    setTimeout(() => g.disconnect(), 600)
  } catch {}
}
function ring(alarms) {
  if (!sym.first && alarms > alarmsSeen && sound) beep()
  alarmsSeen = alarms
}
function renderBar() {
  // the desk sessions online over the headcount (a task's member sessions are the same people, and tiles of their own)
  const live = snapshot.accounts.filter(a => !a.advisor && online(sym.sessionOf(a))).length, head = snapshot.accounts.filter(a => !a.advisor).length
  document.getElementById('stat-sessions').textContent = `${live}/${head}`
  const alarms = snapshot.asks.length, el = document.getElementById('stat-alarms')
  el.querySelector('b').textContent = alarms
  el.classList.toggle('hot', alarms > 0)
  document.getElementById('stat-cost').textContent = fmtUsd(snapshot.ledger.reduce((n, r) => n + r.usd, 0))
}
document.getElementById('stat-alarms').onclick = () => {
  const a = snapshot.asks[0]
  if (!a) return
  const task = sym.taskOfSession(a.session), m = task?.members.find(x => x.session === a.session)
  if (m) { tasks.openWindow(task.id); openChat(m.account, m.session, task.id) } else openChat(a.account)
}
function tickTime() {
  const started = Date.parse(snapshot.started || '') || Date.now(), s = Math.max(0, Math.floor((Date.now() - started) / 1000))
  const p = n => String(n).padStart(2, '0')
  document.getElementById('mission-time').textContent = `T+${p(Math.floor(s / 3600))}:${p(Math.floor(s / 60) % 60)}:${p(s % 60)}`
}
setInterval(tickTime, 1000)

// ---------- the ticker: the bridge's activity log, newest last, and every push that adds to it ----------
// fetched on a page that's seen, not more than every few seconds however many pushes come; again when the page shows
// again or the stream comes back
const ACT_GAP = 3000
let activity = [], actPending = null, actAt = 0
async function loadActivity() { actAt = Date.now(); const list = await sym.fetchActivity(); if (list) activity = list; renderTicker() }
function refreshActivity(now = false) {
  if (actPending || document.visibilityState === 'hidden') return
  actPending = setTimeout(() => { actPending = null; loadActivity() }, Math.max(now ? 0 : 800, actAt + ACT_GAP - Date.now()))
}
for (const type of ['event', 'message', 'ask', 'settled', 'launch']) sym.on(type, () => refreshActivity())
sym.on('resync', () => refreshActivity(true))
document.addEventListener('visibilitychange', () => refreshActivity(true))
let tickerShown = ''
function renderTicker() {
  const track = document.getElementById('ticker-track'), items = activity.slice(-40)
  const html = items.length ? items.map(a => `<span class="item ${esc(a.kind)}"><time>${esc(fmtTime(a.time))}</time><b>${esc(a.name || '—')}</b><span class="k">${esc(t('mc.kind.' + a.kind))}</span>${esc(a.text || '')}</span>`).join('') : `<span class="empty">${esc(t('mc.noActivity'))}</span>`
  if (html === tickerShown) return
  tickerShown = html
  track.innerHTML = html
  track.classList.toggle('still', items.length < 4) // a few lines sit still and readable; more of them scroll
  track.style.setProperty('--dur', Math.max(20, items.length * 4) + 's')
}

// ---------- every change, once per frame, and not while a pointer is down ----------
let drawing = false
const draw = deferWhilePressed(() => { renderBar(); renderWall(); if (chatFor) showChat() })
function render() {
  if (drawing) return
  drawing = true
  requestAnimationFrame(() => { drawing = false; draw() })
}
sym.on('change', render)
sym.on('protocol', (bridge, page) => { const note = document.getElementById('launch-note'); note.hidden = false; note.textContent = t('error.protocol_mismatch', { bridge, page }) })
setInterval(() => renderWall(), 5000) // the sparklines move on with the minute
loadOptions()
loadActivity()
tickTime()
render()
