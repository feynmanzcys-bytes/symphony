// Messenger: the sessions as coworkers in a messaging app. The sidebar lists them (a face whose motion is the state,
// the name, a title line, a progress line) and the tasks as group chats; the middle column holds the conversation
// (the shared employee window, or the task window with the team chat, embedded there); the right column the profile
// and the computer: what runs now, what ran, and "Take over" (the session's terminal in a window).
import { t, lang, errorText, applyI18n, langPicker } from '/i18n.js'
import { esc } from '/asks.js'
import { connect } from '/symphony.js'
import { base, lower, hash, buildText, statusText as statusOf, setHTML, deferWhilePressed } from '/ui/common.js'
import { connectionBanner } from '/ui/banner.js'
import { announceAsks } from '/ui/a11y.js'
import { createChat } from '/ui/chat.js'
import { createLaunchDialog } from '/ui/launch.js'
import { createTaskUI } from '/ui/task.js'
import { themePicker } from '/ui/themes.js'

const sym = connect()
let snapshot = sym.state
// the tab's title counts the asks: kept up with every snapshot, also while the page is hidden (no frames then)
sym.on('snapshot', s => { snapshot = s; document.title = (s.asks.length ? `(${s.asks.length}) ` : '') + t('msgr.title') })
const statusText = s => statusOf(sym, s)
applyI18n()
connectionBanner(sym)
announceAsks(sym)

// ---------- dark or light ----------
const DARK_STORE = 'symphony.messenger.dark'
const darkBtn = document.getElementById('dark')
darkBtn.setAttribute('aria-label', t('msgr.dark')) // a switch: its name stays, aria-pressed says whether it's on
function setDark(on, remember = true) {
  document.documentElement.toggleAttribute('data-dark', on)
  darkBtn.textContent = on ? '☀' : '☾'
  darkBtn.title = t(on ? 'msgr.light' : 'msgr.dark')
  darkBtn.setAttribute('aria-pressed', String(on))
  if (remember) try { localStorage.setItem(DARK_STORE, on ? '1' : '0') } catch {}
}
setDark((() => { try { const v = localStorage.getItem(DARK_STORE); if (v !== null) return v === '1' } catch {} return matchMedia('(prefers-color-scheme: dark)').matches })(), false)
darkBtn.onclick = () => { setDark(!document.documentElement.hasAttribute('data-dark')); render() }
const prefs = document.querySelector('.side .prefs'), langSel = langPicker()
prefs.appendChild(langSel)
themePicker(sym).then(sel => prefs.insertBefore(sel, langSel))

// ---------- figures ----------
const online = s => sym.isOnline(s)
const desks = () => snapshot.accounts.filter(a => !a.advisor)
const fmtUsd = n => '$' + (n >= 100 ? n.toFixed(0) : n >= 10 ? n.toFixed(1) : n.toFixed(2))
const fmtNum = new Intl.NumberFormat(lang, { notation: 'compact', maximumFractionDigits: 1 })
const fmtTime = when => new Date(when).toLocaleTimeString(lang, { hour: '2-digit', minute: '2-digit' })
const costOf = acc => snapshot.ledger.filter(r => r.account === acc.id).reduce((n, r) => n + r.usd, 0)
// an account's desk session: the one online, else the last one the bridge still has (off duty since when)
const deskOf = acc => sym.deskOf(acc)
const taskOf = acc => snapshot.tasks.find(k => k.members.some(m => m.account === acc.id && online(sym.sessionById(m.session)))) ?? null
const stateOf = s => !online(s) ? 'ended' : sym.asking(s.id) ? 'waiting' : s.state
const GROUP_ORDER = { waiting: 0, tool: 1, thinking: 1, idle: 2, ended: 3, done: 3 }
// the rate-limit windows: the ones known by name, any other as Claude Code calls it
const LIMIT_KEY = { five_hour: 'msgr.limit.fiveHour', seven_day: 'msgr.limit.sevenDay', spend_limit: 'msgr.limit.spend' }
const limitLabel = kind => LIMIT_KEY[kind] ? t(LIMIT_KEY[kind]) : String(kind).replace(/_/g, ' ')

// a face: the shape and the hue from the name, the eyes, a mouth; the state is on the element, the motion in CSS
const SHAPES = ['<circle class="body" cx="22" cy="22" r="20"/>', '<rect class="body" x="3" y="3" width="38" height="38" rx="10"/>', '<polygon class="body" points="22,2 40,12 40,32 22,42 4,32 4,12"/>', '<path class="body" d="M22 3 L41 22 L22 41 L3 22 Z" />']
function face(name, state, cls = '') {
  const h = hash(name), hue = h % 360, shape = SHAPES[(h >>> 8) % SHAPES.length], mouth = (h >>> 12) % 2
  return `<div class="face ${cls}" data-state="${esc(state)}" aria-hidden="true" style="--c:hsl(${hue} 48% ${document.documentElement.hasAttribute('data-dark') ? 52 : 44}%)"><svg viewBox="0 0 44 44">` +
    `<circle class="ring" cx="22" cy="22" r="21.5"/>${shape}` +
    `<ellipse class="eye" cx="15" cy="19" rx="4" ry="4.5"/><ellipse class="eye" cx="29" cy="19" rx="4" ry="4.5"/>` +
    `<circle class="pupil" cx="15.5" cy="19.5" r="2"/><circle class="pupil" cx="29.5" cy="19.5" r="2"/>` +
    (mouth ? `<path class="mouth" d="M15 29 Q22 34 29 29"/>` : `<path class="mouth" d="M16 30 L28 30"/>`) +
    `<rect class="hand" x="34" y="4" width="7" height="14" rx="3.5"/><path class="check" d="M30 36 L35 41 L43 31"/></svg></div>`
}

// ---------- the shared pieces, embedded in the middle column ----------
let picked = null // what the middle column shows: { account, sid?, task? } for a conversation, { group: taskId } for a group chat
const center = document.getElementById('center'), placeholder = document.getElementById('placeholder')
const askWho = a => `<div class="who"><span class="name">${esc(t(a.kind === 'question' ? 'asks.question' : 'asks.permission'))}</span>` + (a.agentId ? `<span class="kind">${esc(t('common.subagent'))}</span>` : '') + `</div>`
const chat = createChat(sym, { askWho, onClose: () => { if (picked?.account) picked = null; showCenter() }, onTaskChip: id => openGroup(id) })
const launch = createLaunchDialog(sym, { options: loadOptions, onOnline: l => openCoworker(l.account), log: () => {} })
const tasks = createTaskUI(sym, {
  options: loadOptions,
  roomLabel: task => task.title.split('\n')[0].slice(0, 48),
  memberAvatar: m => avatarURL(m?.name || '?'), youAvatar: () => avatarURL(t('task.you')),
  isMemberOpen: (task, m) => !!picked?.sid && picked.sid === m.session,
  onOpenMember: (task, m) => openCoworker(m.account, m.session, task.id),
  onWindowOpen: id => { picked = { group: id }; chat.el.hidden = true; placeholder.hidden = true; renderProfile(); renderSide() }, // the one way into a group chat
  onWindowClose: () => { if (picked?.group) picked = null; showCenter() },
  onDialogOpen: () => {},
  log: () => {},
})
center.append(chat.el, tasks.win)
function avatarURL(name) {
  const h = hash(name), hue = h % 360
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><circle cx="16" cy="16" r="15" fill="hsl(${hue} 48% 46%)"/><ellipse cx="11" cy="14" rx="3" ry="3.4" fill="#fff"/><ellipse cx="21" cy="14" rx="3" ry="3.4" fill="#fff"/><circle cx="11.5" cy="14.5" r="1.5" fill="#111b21"/><circle cx="21.5" cy="14.5" r="1.5" fill="#111b21"/><path d="M11 22 Q16 26 21 22" stroke="#fff" stroke-width="1.6" fill="none" stroke-linecap="round"/></svg>`
  return `url("data:image/svg+xml,${encodeURIComponent(svg)}")`
}
function showCenter() {
  const conv = !!picked?.account, group = !!picked?.group
  if (!conv) chat.el.hidden = true
  if (!group) tasks.win.hidden = true
  placeholder.hidden = conv || group
  renderProfile()
  renderSide()
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

// a coworker's conversation: its desk session, or one of its task sessions (sid) from a group chat
function openCoworker(id, sid = null, taskId = null) {
  picked = { account: id, sid, task: taskId }
  tasks.win.hidden = true
  showChat()
  placeholder.hidden = true
  chat.focus()
  renderProfile(); renderSide()
  refreshActivity(true) // what its computer ran
}
const openGroup = id => tasks.openWindow(id) // onWindowOpen does the rest
function showChat() {
  if (!picked?.account) return
  const acc = sym.accountById(picked.account)
  if (!acc) { picked = null; return showCenter() }
  const s = picked.sid ? sym.sessionById(picked.sid) : sym.sessionOf(acc), on = online(s)
  const task = picked.sid ? sym.taskOfSession(picked.sid) : taskOf(acc)
  const member = task?.members.find(m => picked.sid ? m.session === picked.sid : m.account === picked.account)
  const busy = on && (s.state === 'thinking' || s.state === 'tool' || s.state === 'waiting')
  const buttons = []
  if (busy) buttons.push({ key: 'stop', text: t('chat.stop'), cls: 'stop', onclick: async () => { const r = await sym.abort(s.id); if (!r.ok) alert(errorText(r.data)) }, title: t('chat.stopTitle') })
  if (!picked.sid && task && member?.session) buttons.push({ key: 'to-task', text: t('task.openMember', { name: member.name }), cls: 'primary to-task', onclick: () => openCoworker(picked.account, member.session, task.id) })
  if (!on && !picked.sid && launchOpts?.enabled) buttons.push({ key: 'go-work', text: t('launch.go'), cls: 'primary go-work', onclick: () => launch.open(acc.id) })
  if (!on && !picked.sid && !task) buttons.push({ key: 'delete', text: t('card.delete'), cls: '', onclick: async () => {
    if (!confirm(t('card.deleteConfirm', { name: acc.name }))) return
    const r = await sym.deleteAccount(acc.id)
    if (!r.ok) alert(t('card.deleteFailed', { reason: errorText(r.data) }))
    picked = null; showCenter()
  } })
  chat.show({
    name: picked.sid ? member?.name || s?.title || acc.name : acc.name,
    state: stateOf(s), line: statusText(on ? s : null),
    taskChip: task ? { id: task.id, title: task.title, label: `${task.id} · ${task.title.split('\n')[0].slice(0, 40)}` } : null,
    chips: [['model', on ? buildText(s) : ''], ['dir', s?.cwd || (picked.sid ? member?.worktree : acc.last?.dir || acc.dir)]],
    buttons, buttonsKey: `${picked.account}|${picked.sid}|${on}|${busy}|${!!launchOpts?.enabled}|${task?.id}|${member?.session}`,
    session: picked.sid || s?.id || null, online: on,
    offText: picked.sid ? t('task.offline') : task && member ? t('chat.awayInTask', { id: task.id, name: member.name }) : t('chat.offline'),
  })
}

// ---------- the sidebar ----------
const findEl = document.getElementById('find')
findEl.setAttribute('aria-label', t('msgr.search'))
findEl.addEventListener('input', renderSide)
addEventListener('keydown', ev => {
  if (ev.key === 'Escape' && !tasks.dialogHidden) tasks.closeDialog()
  if (ev.key === 'Escape' && !launch.hidden) launch.close()
  if (ev.key !== '/' || ev.ctrlKey || ev.metaKey || ev.altKey || !launch.hidden || !tasks.dialogHidden) return
  if (/^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement?.tagName)) return
  ev.preventDefault()
  findEl.focus()
})
// the progress line: what the coworker is on, in a few words
function progress(acc, s) {
  if (!online(s)) return s ? t('msgr.offSince', { time: fmtTime(s.updated) }) : t('state.ended')
  const ask = snapshot.asks.find(a => a.session === s.id)
  if (ask) return t('msgr.waitingFor', { what: ask.kind === 'question' ? (ask.questions?.[0]?.question || t('asks.question')) : [ask.tool, ask.input?.command || ask.input?.file_path || ''].filter(Boolean).join(' ') })
  if (s.state === 'tool' && s.tool) return t('state.working', { tool: s.tool })
  if (s.state === 'thinking') return t('state.thinking')
  return s.prompt ? t('msgr.lastAsked', { text: s.prompt }) : t('state.idle')
}
// The rows are kept and replaced only when what they show changed (a face rebuilt would start its motion over), in
// the order the list wants; the empty line is put in when there is nothing
function syncRows(box, items, empty) {
  const keep = new Set(items.map(it => it.key))
  for (const el of [...box.children]) if (!el.dataset.key || !keep.has(el.dataset.key)) el.remove()
  let prev = null
  for (const it of items) {
    let el = box.querySelector(`[data-key="${CSS.escape(it.key)}"]`)
    if (!el || el.dataset.sig !== it.sig) {
      const tpl = document.createElement('template')
      tpl.innerHTML = it.html
      const fresh = tpl.content.firstElementChild
      fresh.dataset.key = it.key; fresh.dataset.sig = it.sig
      const focused = el?.contains(document.activeElement)
      if (el) el.replaceWith(fresh); else box.appendChild(fresh)
      if (focused) fresh.focus({ preventScroll: true })
      el = fresh
    }
    if ((prev ? prev.nextSibling : box.firstChild) !== el) { if (prev) prev.after(el); else box.prepend(el) }
    prev = el
  }
  if (!items.length) box.innerHTML = `<div class="empty">${esc(empty)}</div>`
}
function renderSide() {
  const q = lower(findEl.value.trim()), dark = document.documentElement.hasAttribute('data-dark')
  const rows = desks().map(acc => ({ acc, s: deskOf(acc), task: taskOf(acc) }))
    .filter(r => !q || [r.acc.name, r.s?.title, base(r.s?.cwd || r.acc.dir || '')].some(v => lower(v).includes(q)))
    .sort((a, b) => GROUP_ORDER[stateOf(a.s)] - GROUP_ORDER[stateOf(b.s)] || a.acc.name.localeCompare(b.acc.name, lang))
  syncRows(document.getElementById('people'), rows.map(({ acc, s, task }) => {
    const st = stateOf(s), on = online(s), hot = st === 'waiting', line = progress(acc, s), when = on ? fmtTime(s.updated) : ''
    const title = on ? [s.model ? s.model.replace(/^claude-/, '').replace(/-\d+-\d+$/, '') + (s.effort ? ' · ' + s.effort : '') : '', base(s.cwd || '')].filter(Boolean).join(' · ') : (acc.last?.model || '')
    const picked_ = picked?.account === acc.id && !picked.sid
    const html = `<div class="row${picked_ ? ' on' : ''}${on ? '' : ' off'}" data-account="${esc(acc.id)}" role="button" tabindex="0"${picked_ ? ' aria-current="true"' : ''}>${face(acc.name, st)}` +
      `<div class="nm">${esc(acc.name)}${task ? ` <span class="badge" title="${esc(task.title)}">${esc(task.id)}</span>` : ''}</div>` +
      `<div class="when">${esc(when)}</div>` +
      `<div class="sub${hot ? ' hot' : ''}" title="${esc(line)}">${esc(title ? title + ' — ' : '')}${esc(line)}</div></div>`
    return { key: acc.id, sig: `${acc.name}|${st}|${title}|${line}|${when}|${picked_}|${task?.id}|${dark}`, html }
  }), t('msgr.nobody'))
  const groups = snapshot.tasks.filter(k => !q || lower(k.id).includes(q) || lower(k.title).includes(q))
  const gc = document.getElementById('groups-count')
  gc.hidden = !snapshot.tasks.length; gc.textContent = snapshot.tasks.length
  syncRows(document.getElementById('groups'), groups.map(k => {
    const waiting = k.members.some(m => sym.asking(m.session)), st = k.status === 'failed' ? 'ended' : waiting ? 'waiting' : 'tool'
    const picked_ = picked?.group === k.id || picked?.task === k.id, line = k.title.split('\n')[0]
    const html = `<div class="row${picked_ ? ' on' : ''}" data-group="${esc(k.id)}" role="button" tabindex="0"${picked_ ? ' aria-current="true"' : ''}>${face(k.id, st)}` +
      `<div class="nm">${esc(k.id)} <span class="badge${waiting ? ' hot' : ''}">${esc(t('msgr.members', { n: k.members.length }))}</span></div><div class="when">${esc(t('task.status.' + k.status))}</div>` +
      `<div class="sub" title="${esc(k.title)}">${esc(line)}</div></div>`
    return { key: k.id, sig: `${st}|${k.status}|${k.members.length}|${waiting}|${picked_}|${line}|${dark}`, html }
  }), t('msgr.noGroups'))
}
document.getElementById('people').addEventListener('click', ev => { const r = ev.target.closest('.row'); if (r) openCoworker(r.dataset.account) })
document.getElementById('groups').addEventListener('click', ev => { const r = ev.target.closest('.row'); if (r) openGroup(r.dataset.group) })

// ---------- the profile and the computer ----------
// The activity log feeds the computer's last lines: fetched only while a coworker's profile shows, on a page that's
// seen, not more than every few seconds however many pushes come; again when one is picked, the page shows again or
// the stream comes back.
const ACT_GAP = 3000
let activity = [], actPending = null, actAt = 0
async function loadActivity() { actAt = Date.now(); const list = await sym.fetchActivity(); if (list) activity = list; renderProfile() }
function refreshActivity(now = false) {
  if (actPending || !picked?.account || document.visibilityState === 'hidden') return
  actPending = setTimeout(() => { actPending = null; loadActivity() }, Math.max(now ? 0 : 800, actAt + ACT_GAP - Date.now()))
}
for (const type of ['event', 'ask', 'settled']) sym.on(type, () => refreshActivity())
sym.on('resync', () => refreshActivity(true))
document.addEventListener('visibilitychange', () => refreshActivity(true))
const profileEl = document.getElementById('profile')
const row = (k, v, wrap = false) => v === undefined || v === null || v === '' ? '' : `<dt>${esc(t('msgr.profile.' + k))}</dt><dd class="${wrap ? 'wrap' : ''}">${v}</dd>`
function renderProfile() {
  if (!picked) { profileEl.hidden = true; return }
  profileEl.hidden = false
  if (picked.group) {
    const k = sym.taskById(picked.group)
    if (!k) { profileEl.hidden = true; return }
    setHTML(profileEl, `<div class="top">${face(k.id, k.status === 'failed' ? 'ended' : 'tool', 'big')}<h2>${esc(k.id)}</h2><div class="st">${esc(t('task.status.' + k.status))}</div></div>` +
      `<h3>${esc(t('msgr.profile.about'))}</h3><div class="wrap" style="font-size:13px;white-space:pre-wrap;overflow-wrap:anywhere">${esc(k.title)}</div>` +
      `<h3>${esc(t('msgr.profile.members'))}</h3><div class="list">` + k.members.map(m => { const s = sym.sessionById(m.session); return `<div class="row" data-session="${esc(m.session || '')}" data-account="${esc(m.account)}"${m.session ? ' role="button" tabindex="0"' : ''}>${face(m.name, stateOf(s))}<span class="txt"><b>${esc(m.name)}</b>${k.leader === m.account ? ` <span class="tag">${esc(t('task.lead'))}</span>` : ''}</span><span class="dim">${esc(!m.session ? t('task.memberStarting') : online(s) ? statusText(s) : t('task.memberEnded'))}</span></div>` }).join('') + '</div>')
    return
  }
  const acc = sym.accountById(picked.account)
  if (!acc) { profileEl.hidden = true; return }
  const s = picked.sid ? sym.sessionById(picked.sid) : sym.sessionOf(acc), on = online(s), st = stateOf(s)
  const task = picked.sid ? sym.taskOfSession(picked.sid) : taskOf(acc)
  const name = picked.sid ? task?.members.find(m => m.session === picked.sid)?.name || acc.name : acc.name
  const u = s?.usage, ctx = u?.context?.percent
  const totals = u?.totals ? Object.values(u.totals).reduce((n, tk) => n + tk.input + tk.output, 0) : 0
  const recent = activity.filter(a => a.session === s?.id && (a.kind === 'tool' || a.kind === 'ask' || a.kind === 'error')).slice(-5).reverse()
  const drawn = setHTML(profileEl, `<div class="top">${face(name, st, 'big')}<h2>${esc(name)}</h2><div class="st">${esc(on ? statusText(s) : t('state.ended'))}</div></div>` +
    (picked.task ? `<span class="link" data-group="${esc(picked.task)}" role="button" tabindex="0">← ${esc(t('msgr.profile.backToGroup', { id: picked.task }))}</span>` : task ? `<span class="link" data-group="${esc(task.id)}" role="button" tabindex="0">${esc(t('msgr.profile.inGroup', { id: task.id }))}</span>` : '') +
    `<h3>${esc(t('msgr.profile.computer'))}</h3><div class="computer">` +
    `<div class="now ${on && s.state === 'tool' ? 'on' : on ? '' : 'off'}"><i></i><span>${esc(on ? (s.state === 'tool' && s.tool ? s.tool : s.state === 'thinking' ? t('state.thinking') : t('msgr.profile.idle')) : t('state.ended'))}</span></div>` +
    (recent.length ? `<div class="recent">${recent.map(a => `<div><time>${esc(fmtTime(a.time))}</time>${esc(a.text)}</div>`).join('')}</div>` : '') +
    (on && s.launch ? `<button class="px" id="take-over" title="${esc(t('chat.terminalTitle'))}">${esc(t('msgr.profile.takeOver'))}</button>` : '') + `</div>` +
    `<h3>${esc(t('msgr.profile.about'))}</h3><dl>` +
    row('model', on ? esc(buildText(s)) : esc(acc.last?.model || '')) + row('folder', esc(s?.cwd || acc.last?.dir || acc.dir || '')) +
    row('repo', s?.repo ? esc(s.repo.name || base(s.repo.root)) : '') + row('engine', s?.engine ? esc(s.engine.version + (s.mod ? ' · ' + t('msgr.profile.modVersion', { version: s.mod }) : '')) : '') +
    row('session', esc(s?.title || '')) + row('login', esc(s?.login || '')) + `</dl>` +
    (on ? `<h3>${esc(t('msgr.profile.figures'))}</h3><dl>` +
      row('context', typeof ctx === 'number' ? `<span class="bar${ctx >= 90 ? ' full' : ctx >= 70 ? ' high' : ''}"><i style="width:${Math.min(100, ctx)}%"></i></span>${Math.round(ctx)}%` : '') +
      row('limits', (u?.rateLimits || []).map(r => esc(`${limitLabel(r.kind)} ${Math.round(r.percentUsed)}%`)).join(' · ')) +
      row('cost', (u?.cost ? esc(fmtUsd(u.cost.usd)) + ' · ' : '') + esc(t('msgr.profile.today', { usd: fmtUsd(costOf(acc)) }))) +
      row('tokens', totals ? esc(fmtNum.format(totals)) : '') + row('tools', s.tools ? `${s.tools}${s.toolMs ? ' · ' + esc(Math.round(s.toolMs / 1000) + 's') : ''}` : '') + row('turns', s.turns || '') +
      row('mcp', (s.mcp || []).map(m => `<span class="tag">${esc(m)}</span>`).join('')) +
      row('agents', Object.values(s.agents || {}).map(a => `<span class="tag" title="${esc(a.description || '')}">${esc(a.type || a.id.slice(0, 6))}${a.state === 'done' || a.ended ? ' ✓' : ''}</span>`).join('')) +
      row('todos', (s.todos || []).map(x => esc(x.subject)).join('<br>'), true) + `</dl>` : ''))
  if (drawn) for (const dd of profileEl.querySelectorAll('dd')) dd.title = dd.textContent // the tooltip: the row's text as shown
}
profileEl.addEventListener('click', async ev => {
  if (ev.target.id === 'take-over') {
    const acc = sym.accountById(picked.account), s = picked.sid ? sym.sessionById(picked.sid) : sym.sessionOf(acc)
    const r = await sym.openTerminal(s.launch)
    if (!r.ok) alert(errorText(r.data) + (r.data?.params?.command ? '\n' + r.data.params.command : ''))
    return
  }
  const link = ev.target.closest('[data-group]')
  if (link) return openGroup(link.dataset.group)
  const m = ev.target.closest('.row[data-session]')
  if (m && m.dataset.session) openCoworker(m.dataset.account, m.dataset.session, picked.group)
})

// ---------- every change, once per frame, and not while a pointer is down (a row replaced under a press loses its
// click) ----------
let drawing = false
const draw = deferWhilePressed(() => {
  renderSide()
  if (picked?.account) showChat()
  renderProfile()
})
function render() {
  if (drawing) return
  drawing = true
  requestAnimationFrame(() => { drawing = false; draw() })
}
sym.on('change', render)
sym.on('protocol', (bridge, page) => { const note = document.getElementById('launch-note'); note.hidden = false; note.textContent = t('error.protocol_mismatch', { bridge, page }) })
document.getElementById('placeholder-face').outerHTML = face('symphony', 'idle', 'big')
loadOptions()
render()
