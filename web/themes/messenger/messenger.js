// Messenger: the sessions as coworkers in a messaging app. The sidebar lists them (a face whose motion is the state,
// the name, a title line, a progress line) and the tasks as group chats; the middle column holds the conversation
// (the shared employee window, or the task window with the team chat, embedded there); the right column the profile
// and the computer: what runs now, its screen (a page-started session's terminal, live and small; a click shows it in
// the conversation), what ran, and "Take over" (the session's terminal in a window).
import { t, lang, errorText, applyI18n } from '/i18n.js'
import { esc } from '/asks.js'
import { connect } from '/symphony.js'
import { base, lower, hash, modelText, statusText as statusOf, setHTML, deferWhilePressed, askKind, displayName } from '/ui/common.js'
import { connectionBanner } from '/ui/banner.js'
import { announceAsks } from '/ui/a11y.js'
import { createChat } from '/ui/chat.js'
import { createLaunchDialog } from '/ui/launch.js'
import { createTaskUI, team } from '/ui/task.js'
import { createTerminal } from '/ui/terminal.js'
import { createSettings, settingsButton } from '/ui/settings.js'
import '/ui/look.js' // the size and the typeface from the settings, applied as the page loads
import { faceHTML, avatarURL } from '/ui/faces.js'
import { showError } from '/ui/errors.js'

const sym = connect()
let snapshot = sym.state
// the tab's title counts the asks: kept up with every snapshot, also while the page is hidden (no frames then)
sym.on('snapshot', s => { snapshot = s; document.title = (s.asks.length ? `(${s.asks.length}) ` : '') + t('msgr.title') })
const statusText = s => statusOf(sym, s)
applyI18n()
connectionBanner(sym)
announceAsks(sym)

// ---------- dark or light: as the system is (it follows it), or as picked in the settings ----------
const DARK_STORE = 'symphony.messenger.dark' // '1' dark, '0' light, nothing: as the system is
const systemDark = matchMedia('(prefers-color-scheme: dark)')
const scheme = {
  get() { try { const v = localStorage.getItem(DARK_STORE); return v === '1' ? 'dark' : v === '0' ? 'light' : 'system' } catch { return 'system' } },
  set(v) { try { if (v === 'system') localStorage.removeItem(DARK_STORE); else localStorage.setItem(DARK_STORE, v === 'dark' ? '1' : '0') } catch {} applyScheme() },
}
function applyScheme(redraw = true) {
  const v = scheme.get(), dark = v === 'dark' || (v === 'system' && systemDark.matches)
  if (document.documentElement.hasAttribute('data-dark') === dark) return
  document.documentElement.toggleAttribute('data-dark', dark)
  if (redraw) render() // (the faces' colors depend on it)
}
systemDark.addEventListener('change', () => applyScheme())
applyScheme(false)
// the settings: light or dark is Coworkers' own; the sidebar's button opens them
const settings = createSettings(sym, { scheme })
document.querySelector('.side-actions').append(settingsButton(settings, { cls: 'px', icon: gearIcon() }))
settingsNav()
// the settings read like a page: their sections listed down the left of the dialog, a click scrolls to one and puts
// the focus on its heading, the one in sight marked as the body scrolls (the list's words come from the headings, so
// they follow the language; a section the theme hides is left out)
function settingsNav() {
  const panel = settings.el.querySelector('.panel'), body = panel.querySelector('.body')
  const sections = [...body.querySelectorAll(':scope > section')]
  const nav = document.createElement('nav')
  nav.className = 'set-nav'
  nav.setAttribute('aria-label', t('settings.title'))
  const buttons = sections.map(s => {
    const b = nav.appendChild(document.createElement('button'))
    b.type = 'button'
    const h = s.querySelector('h3')
    if (h) h.tabIndex = -1 // reachable from the list, so a screen reader lands on the section's name
    b.onclick = () => { s.scrollIntoView({ block: 'start' }); h?.focus({ preventScroll: true }); mark(s) }
    return b
  })
  panel.insertBefore(nav, body)
  function mark(cur) {
    buttons.forEach((b, i) => b.classList.toggle('on', sections[i] === cur))
  }
  function inSight() {
    const top = body.scrollTop + 48
    let cur = sections.find(s => !s.hidden) ?? null
    for (const s of sections) if (!s.hidden && s.offsetTop <= top) cur = s
    mark(cur)
  }
  function sync() {
    // (written only when changed: setting an attribute to what it is still wakes the observer, and that would loop)
    buttons.forEach((b, i) => { if (b.hidden !== sections[i].hidden) b.hidden = sections[i].hidden; const words = sections[i].querySelector('h3')?.textContent || ''; if (b.textContent !== words) b.textContent = words })
    inSight()
  }
  body.addEventListener('scroll', inSight, { passive: true })
  new MutationObserver(sync).observe(settings.el, { attributes: true, attributeFilter: ['hidden'], subtree: true })
  sync()
}
function gearIcon() {
  const tpl = document.createElement('template')
  tpl.innerHTML = `<svg class="gear" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" d="M12 15.2a3.2 3.2 0 1 0 0-6.4 3.2 3.2 0 0 0 0 6.4Z M19.4 13.5a7.6 7.6 0 0 0 0-3l2-1.5-2-3.4-2.3 1a7.4 7.4 0 0 0-2.6-1.5L14.2 2.6h-4l-.3 2.5a7.4 7.4 0 0 0-2.6 1.5l-2.3-1-2 3.4 2 1.5a7.6 7.6 0 0 0 0 3l-2 1.5 2 3.4 2.3-1a7.4 7.4 0 0 0 2.6 1.5l.3 2.5h4l.3-2.5a7.4 7.4 0 0 0 2.6-1.5l2.3 1 2-3.4Z"/></svg>`
  return tpl.content.firstElementChild
}

// ---------- figures ----------
const online = s => sym.isOnline(s)
const desks = () => snapshot.accounts.filter(a => !a.advisor)
const fmtUsd = n => '$' + (n >= 100 ? n.toFixed(0) : n >= 10 ? n.toFixed(1) : n.toFixed(2))
const fmtNum = new Intl.NumberFormat(lang, { notation: 'compact', maximumFractionDigits: 1 })
// one formatter for every time shown: toLocaleTimeString with options makes a new one on each call, and the sidebar
// shows a time on every row it draws
const timeFmt = new Intl.DateTimeFormat(lang, { hour: '2-digit', minute: '2-digit' })
const fmtTime = when => { const d = new Date(when); return isNaN(d) ? '' : timeFmt.format(d) }
const costOf = acc => snapshot.ledger.filter(r => r.account === acc.id).reduce((n, r) => n + r.usd, 0)
// an account's desk session: the one online, else the last one the bridge still has (off duty since when)
const deskOf = acc => sym.deskOf(acc)
const taskOf = acc => snapshot.tasks.find(k => k.members.some(m => m.account === acc.id && online(sym.sessionById(m.session)))) ?? null
const stateOf = s => !online(s) ? 'ended' : sym.asking(s.id) ? 'waiting' : s.state
const GROUP_ORDER = { waiting: 0, tool: 1, thinking: 1, idle: 2, ended: 3, done: 3 }
// the rate-limit windows: the ones known by name, any other as Claude Code calls it
const LIMIT_KEY = { five_hour: 'msgr.limit.fiveHour', seven_day: 'msgr.limit.sevenDay', spend_limit: 'msgr.limit.spend' }
// the model in a word with the effort: "opus · xhigh" (claude-opus-5-5 with the thinking effort xhigh)
const shortModel = s => [s.model ? s.model.replace(/^claude-/, '').replace(/-\d+-\d+$/, '') : '', s.effort || ''].filter(Boolean).join(' · ')
const limitLabel = kind => LIMIT_KEY[kind] ? t(LIMIT_KEY[kind]) : String(kind).replace(/_/g, ' ')

// a face: the shape and the hue from the name, the eyes, a mouth; the state is on the element, the motion in CSS
// a face (ui/faces.js): the account's own look, else what its name picks; the state is on the element, the motion in CSS
const face = (name, state, cls = '', look) => faceHTML(name, state, cls, look)

// ---------- the shared pieces, embedded in the middle column ----------
let picked = null // what the middle column shows: { account, sid?, task? } for a conversation, { group: taskId } for a group chat
const center = document.getElementById('center'), placeholder = document.getElementById('placeholder')
// a card's top line: an icon for the kind, the kind, the tool, the subagent it came from (by what it was told to do),
// and since when it waits
const ASK_ICON = {
  question: '<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M9.5 9.5a2.5 2.5 0 1 1 3.5 2.3c-.7.3-1 .9-1 1.7M12 17h.01"/></svg>',
  permission: '<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3 4 6v6c0 5 3.4 8.4 8 9 4.6-.6 8-4 8-9V6z"/></svg>',
  elicitation: '<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="4" y="3" width="16" height="18" rx="2"/><path d="M8 8h8M8 12h8M8 16h5"/></svg>',
}
const askWho = a => {
  const agent = a.agentId ? sym.sessionById(a.session)?.agents?.[a.agentId] : null
  const from = a.agentId ? (agent?.description ? t('msgr.askFromAgent', { what: agent.description }) : t('common.subagent')) : ''
  return `<div class="who">${ASK_ICON[a.kind] || ASK_ICON.permission}<span class="name">${esc(askKind(a.kind))}</span>` +
    (a.tool ? `<span class="kind">· ${esc(a.tool)}</span>` : '') + (from ? `<span class="kind from">· ${esc(from)}</span>` : '') +
    (a.created ? `<span class="since">${esc(t('msgr.askSince', { time: fmtTime(a.created) }))}</span>` : '') + `</div>`
}
const chat = createChat(sym, { askWho, onClose: () => { if (picked?.account) picked = null; showCenter() }, onTaskChip: id => openGroup(id) })
const launch = createLaunchDialog(sym, { options: loadOptions, onOnline: l => openCoworker(l.account) })
const tasks = createTaskUI(sym, {
  options: loadOptions,
  roomLabel: task => task.name || task.id,
  memberAvatar: m => avatarURL(m?.name || '?', sym.accountById(m?.account)?.look), youAvatar: () => avatarURL('you'),
  isMemberOpen: (task, m) => !!picked?.sid && picked.sid === m.session,
  onOpenMember: (task, m) => openCoworker(m.account, m.session, task.id),
  mentionName: (name, task) => displayName(name, task), // grace, not grace-T1 (the bridge takes both)
  onWindowOpen: id => { picked = { group: id }; chat.el.hidden = true; placeholder.hidden = true; renderProfile(); renderSide() }, // the one way into a group chat
  onWindowClose: () => { if (picked?.group) picked = null; showCenter() },
  onDialogOpen: () => {},
})
center.append(chat.el, tasks.win)
// what the middle column shows, for the narrow window's one column
const app = document.querySelector('.app')
const markPicked = () => app.classList.toggle('picked', !!(picked?.account || picked?.group))
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
document.getElementById('new-task').onclick = () => tasks.openDialog()

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
  const s = picked.sid ? sym.sessionById(picked.sid) : deskOf(acc), on = online(s)
  const task = picked.sid ? sym.taskOfSession(picked.sid) : taskOf(acc)
  const member = task?.members.find(m => picked.sid ? m.session === picked.sid : m.account === picked.account)
  const busy = on && (s.state === 'thinking' || s.state === 'tool' || s.state === 'waiting')
  const dir = s?.cwd || (picked.sid ? member?.worktree : acc.last?.dir || acc.dir) || ''
  const buttons = []
  if (busy) buttons.push({ key: 'stop', text: t('chat.stop'), cls: 'stop', onclick: async () => { const r = await sym.abort(s.id); if (!r.ok) showError(errorText(r.data), { title: t('errlog.oops.stop') }) }, title: t('chat.stopTitle') })
  if (!picked.sid && task && member?.session) buttons.push({ key: 'to-task', text: t('task.openMember', { name: displayName(member.name, task) }), cls: 'primary to-task', onclick: () => openCoworker(picked.account, member.session, task.id) })
  if (!on && !picked.sid && launchOpts?.enabled) buttons.push({ key: 'go-work', text: t('launch.go'), cls: 'primary go-work', onclick: () => launch.open(acc.id) })
  if (!on && !picked.sid && !task) buttons.push({ key: 'delete', text: t('card.delete'), cls: '', onclick: async () => {
    if (!confirm(t('card.deleteConfirm', { name: acc.name }))) return
    const r = await sym.deleteAccount(acc.id)
    if (!r.ok) showError(t('card.deleteFailed', { reason: errorText(r.data) }))
    picked = null; showCenter()
  } })
  chat.show({
    name: picked.sid ? displayName(member?.name, task) || s?.title || acc.name : acc.name,
    state: stateOf(s), line: statusText(on ? s : null),
    taskChip: task ? { id: task.id, title: task.title, label: task.name || task.id } : null,
    // the chips say little and tell the rest on hover: "opus · xhigh" (the model's full name), the folder's name (its path)
    chips: [['model', on ? shortModel(s) : '', on ? modelText(s) : ''], ['dir', base(picked.sid && member?.dir ? member.dir : dir), dir]], // (a member's folder by the repository's name, its worktree on hover: the worktree is named like the session)
    buttons, buttonsKey: `${picked.account}|${picked.sid}|${on}|${busy}|${!!launchOpts?.enabled}|${task?.id}|${member?.session}`,
    terminal: on && s.launch ? s.launch : null, // its terminal in the conversation, as the computer's screen shows it small
    session: picked.sid || s?.id || null, online: on,
    offText: picked.sid ? t('task.offline') : task && member ? t('chat.awayInTask', { id: task.name || task.id, name: displayName(member.name, task) }) : t('chat.offline'),
    // an off-duty coworker's empty conversation offers the start in the middle, where the eye lands
    emptyAction: !on && !picked.sid && !task && launchOpts?.enabled ? { text: t('launch.go'), onclick: () => launch.open(acc.id) } : null,
  })
}

// ---------- the sidebar ----------
const findEl = document.getElementById('find')
findEl.setAttribute('aria-label', t('msgr.search'))
findEl.addEventListener('input', renderSide)
addEventListener('keydown', ev => {
  if (!settings.hidden) return // (the settings have Esc to themselves, and "/" doesn't reach behind them)
  if (ev.key === 'Escape' && !tasks.dialogHidden) tasks.closeDialog()
  if (ev.key === 'Escape' && !launch.hidden) launch.close()
  if (ev.key !== '/' || ev.ctrlKey || ev.metaKey || ev.altKey || !launch.hidden || !tasks.dialogHidden) return
  if (/^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement?.tagName)) return
  ev.preventDefault()
  findEl.focus()
})
// the row's line: waiting for you, what kind of answer it needs; off duty, since when; else the conversation's newest
// line (what it said or was told last; the ring around its face says the state), or its folder while there is none
function progress(acc, s) {
  if (!online(s)) return s ? t('msgr.offSince', { time: fmtTime(s.updated) }) : t('state.ended')
  const ask = snapshot.asks.find(a => a.session === s.id)
  if (ask) return t(ask.kind === 'question' ? 'msgr.needsAnswer' : ask.kind === 'elicitation' ? 'msgr.needsForm' : 'msgr.needsApproval')
  return s.last || s.prompt || base(s.cwd || acc.dir || '')
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
  markPicked()
  const q = lower(findEl.value.trim()), dark = document.documentElement.hasAttribute('data-dark')
  const rows = desks().map(acc => ({ acc, s: deskOf(acc) }))
    .filter(r => !q || [r.acc.name, r.s?.title, base(r.s?.cwd || r.acc.dir || '')].some(v => lower(v).includes(q)))
    .sort((a, b) => GROUP_ORDER[stateOf(a.s)] - GROUP_ORDER[stateOf(b.s)] || a.acc.name.localeCompare(b.acc.name, lang))
  // each row: the face, the name, one line saying what it waits for or does (its groups are in the list below); the ones
  // waiting for you first, under a red "Needs you" heading, the rest under a hairline (both plain elements between
  // the rows, keyed like them)
  const waiting = rows.filter(r => stateOf(r.s) === 'waiting').length
  const items = rows.map(({ acc, s }) => {
    const st = stateOf(s), on = online(s), hot = st === 'waiting', line = progress(acc, s)
    const picked_ = picked?.account === acc.id && !picked.sid
    // (the state is the ring around the face, and for a screen reader a hidden word after the name)
    const html = `<div class="row${picked_ ? ' on' : ''}${on ? '' : ' off'}" data-account="${esc(acc.id)}" role="button" tabindex="0"${picked_ ? ' aria-current="true"' : ''}>${face(acc.name, st, '', acc.look)}` +
      `<div class="nm">${esc(acc.name)}${hot || !on ? '' : `<span class="vh"> · ${esc(statusText(s))}</span>`}</div>` +
      `<div class="sub${hot ? ' hot' : ''}" title="${esc(line)}">${esc(line)}</div></div>`
    return { key: acc.id, sig: `${acc.name}|${st}|${line}|${picked_}|${dark}`, html }
  })
  if (waiting) {
    items.splice(waiting, 0, { key: '#rest', sig: 'rest', html: `<div class="sect rest" aria-hidden="true"></div>` })
    items.unshift({ key: '#needs', sig: `needs|${waiting}`, html: `<h3 class="sect" aria-label="${esc(t('msgr.needsYou'))} ${waiting}">${esc(t('msgr.needsYou'))} <span class="n">${waiting}</span></h3>` })
  }
  syncRows(document.getElementById('people'), items, t('msgr.nobody'))
  const groups = snapshot.tasks.filter(k => !q || lower(k.id).includes(q) || lower(k.title).includes(q))
  const gc = document.getElementById('groups-count')
  gc.hidden = !snapshot.tasks.length; gc.textContent = snapshot.tasks.length
  const heading = t('msgr.groups') + (snapshot.tasks.length ? ' ' + snapshot.tasks.length : '') // (said apart: a space before a count shown later stays collapsed in Edge's tree)
  if (gc.parentElement.getAttribute('aria-label') !== heading) gc.parentElement.setAttribute('aria-label', heading)
  syncRows(document.getElementById('groups'), groups.map(k => {
    const members = team(k), waiting = members.some(m => sym.asking(m.session)), st = k.status === 'failed' ? 'ended' : waiting ? 'waiting' : 'tool'
    const picked_ = picked?.group === k.id || picked?.task === k.id, line = k.name || k.id
    // the second line: what the group is on, or its state while it isn't just running
    const sub = k.status === 'running' && k.title ? k.title.split('\n')[0] : t('task.status.' + k.status)
    const html = `<div class="row${picked_ ? ' on' : ''}" data-group="${esc(k.id)}" role="button" tabindex="0"${picked_ ? ' aria-current="true"' : ''}>${face(k.id, st)}` +
      `<div class="nm"><span class="t">${esc(line)}</span> <span class="badge plain${waiting ? ' hot' : ''}" title="${esc(t('msgr.members', { n: members.length }))}">${k.name ? esc(k.id) + ' · ' : ''}${members.length}</span></div>` +
      `<div class="sub${waiting ? ' hot' : ''}" title="${esc(k.title)}">${esc(sub)}</div></div>`
    return { key: k.id, sig: `${st}|${k.status}|${members.length}|${waiting}|${picked_}|${line}|${sub}|${dark}`, html }
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
// The computer's screen: a page-started session's terminal, live and small. It only watches (the session's terminal
// keeps its size); a click shows it in the conversation, to work in. One element, moved into the profile's slot
// whenever the profile is drawn again.
const screen = createTerminal(sym)
screen.el.title = t('msgr.profile.screenTitle')
screen.el.addEventListener('click', () => chat.showTerminal(true))
function placeScreen(launch) {
  const slot = launch && profileEl.querySelector('.screen-slot')
  if (!slot) return screen.close()
  if (screen.el.parentNode !== slot) slot.appendChild(screen.el)
  screen.open(launch)
}
function renderProfile() {
  if (!picked) { profileEl.hidden = true; placeScreen(null); return }
  profileEl.hidden = false
  if (picked.group) {
    placeScreen(null)
    const k = sym.taskById(picked.group)
    if (!k) { profileEl.hidden = true; return }
    setHTML(profileEl, `<div class="top">${face(k.id, k.status === 'failed' ? 'ended' : 'tool', 'big')}<h2>${esc(k.name || k.id)}</h2><div class="st">${k.name ? esc(k.id) + ' · ' : ''}${esc(t('task.status.' + k.status))}</div></div>` +
      (k.title ? `<h3>${esc(t('msgr.profile.about'))}</h3><div class="wrap" style="font-size:13px;white-space:pre-wrap;overflow-wrap:anywhere">${esc(k.title)}</div>` : '') +
      `<h3>${esc(t('msgr.profile.members'))}</h3><div class="list">` + team(k).map(m => { const s = sym.sessionById(m.session); return `<div class="row" data-session="${esc(m.session || '')}" data-account="${esc(m.account)}"${m.session ? ' role="button" tabindex="0"' : ''}>${face(m.name, stateOf(s), '', sym.accountById(m.account)?.look)}<span class="txt"><b>${esc(displayName(m.name, k))}</b>${k.leader === m.account ? ` <span class="tag">${esc(t('task.lead'))}</span>` : ''}</span><span class="dim">${esc(!m.session ? t('task.memberStarting') : online(s) ? statusText(s) : t('task.memberEnded'))}</span></div>` }).join('') + '</div>')
    return
  }
  const acc = sym.accountById(picked.account)
  if (!acc) { profileEl.hidden = true; placeScreen(null); return }
  const s = picked.sid ? sym.sessionById(picked.sid) : deskOf(acc), on = online(s), st = stateOf(s)
  const task = picked.sid ? sym.taskOfSession(picked.sid) : taskOf(acc)
  const name = picked.sid ? displayName(task?.members.find(m => m.session === picked.sid)?.name, task) || acc.name : acc.name
  const u = s?.usage, ctx = u?.context?.percent
  const totals = u?.totals ? Object.values(u.totals).reduce((n, tk) => n + tk.input + tk.output, 0) : 0
  const recent = activity.filter(a => a.session === s?.id && (a.kind === 'tool' || a.kind === 'ask' || a.kind === 'error')).slice(-5).reverse()
  const drawn = setHTML(profileEl, `<div class="top">${face(name, st, 'big', acc.look)}<h2>${esc(name)}</h2><div class="st">${esc(on ? statusText(s) : t('state.ended'))}</div></div>` +
    (picked.task ? `<span class="link" data-group="${esc(picked.task)}" role="button" tabindex="0">← ${esc(t('msgr.profile.backToGroup', { id: picked.task }))}</span>` : task ? `<span class="link" data-group="${esc(task.id)}" role="button" tabindex="0">${esc(t('msgr.profile.inGroup', { id: task.id }))}</span>` : '') +
    `<h3>${esc(t('msgr.profile.computer'))}</h3><div class="computer">` +
    `<div class="now ${on && s.state === 'tool' ? 'on' : on ? '' : 'off'}"><i></i><span>${esc(on ? (s.state === 'tool' && s.tool ? s.tool : s.state === 'thinking' ? t('state.thinking') : t('msgr.profile.idle')) : t('state.ended'))}</span></div>` +
    (on && s.launch ? `<div class="screen-slot"></div><small class="hint">${esc(t('msgr.profile.screenHint'))}</small>` : '') +
    (recent.length ? `<div class="recent">${recent.map(a => `<div><time>${esc(fmtTime(a.time))}</time>${esc(a.text)}</div>`).join('')}</div>` : '') +
    (on && s.launch ? `<button class="px" id="take-over" title="${esc(t('chat.terminalTitle'))}">${esc(t('msgr.profile.takeOver'))}</button>` : '') + `</div>` +
    `<h3>${esc(t('msgr.profile.about'))}</h3><dl>` +
    row('model', on ? modelPicker(s) : esc(acc.last?.model || '')) + row('folder', esc(s?.cwd || acc.last?.dir || acc.dir || '')) +
    row('repo', s?.repo ? esc(s.repo.name || base(s.repo.root)) : '') + row('engine', s?.engine ? esc(s.engine.version + (s.mod ? ' · ' + t('msgr.profile.modVersion', { version: s.mod }) : '')) : '') +
    row('session', esc(s?.task ? displayName(s.title, task) : s?.title || '')) + row('login', esc(s?.login || '')) + `</dl>` +
    (on ? `<h3>${esc(t('msgr.profile.figures'))}</h3><dl>` +
      row('context', typeof ctx === 'number' ? `<span class="bar${ctx >= 90 ? ' full' : ctx >= 70 ? ' high' : ''}"><i style="width:${Math.min(100, ctx)}%"></i></span>${Math.round(ctx)}%` : '') +
      row('limits', (u?.rateLimits || []).map(r => esc(`${limitLabel(r.kind)} ${Math.round(r.percentUsed)}%`)).join(' · ')) +
      row('cost', (u?.cost ? esc(fmtUsd(u.cost.usd)) + ' · ' : '') + esc(t('msgr.profile.today', { usd: fmtUsd(costOf(acc)) }))) +
      row('tokens', totals ? esc(fmtNum.format(totals)) : '') + row('tools', s.tools ? `${s.tools}${s.toolMs ? ' · ' + esc(Math.round(s.toolMs / 1000) + 's') : ''}` : '') + row('turns', s.turns || '') +
      row('mcp', (s.mcp || []).map(m => `<span class="tag">${esc(m)}</span>`).join('')) +
      row('agents', Object.values(s.agents || {}).map(a => `<span class="tag" title="${esc(a.description || '')}">${esc(a.type || a.id.slice(0, 6))}${a.state === 'done' || a.ended ? ' ✓' : ''}</span>`).join('')) +
      row('todos', (s.todos || []).map(x => esc(x.subject)).join('<br>'), true) + `</dl>` : ''))
  if (drawn) for (const dd of profileEl.querySelectorAll('dd')) dd.title = dd.textContent // the tooltip: the row's text as shown
  placeScreen(on && s.launch ? s.launch : null)
}
// a running session's model and thinking effort: two drop-downs of the launch dialog's choices (the session's own
// value when it is none of them), a change runs /model or /effort in the session; what the engine did comes back as
// a command row in the chat and the model's chip
const shortModelId = m => (m || '').replace(/^claude-/, '').replace(/-\d+-\d+$/, '')
function modelPicker(s) {
  const options = (list, cur, label) => list.filter(o => o.value).map(o => `<option value="${esc(o.value)}"${o.value === cur ? ' selected' : ''}>${esc(o.label)}</option>`).join('') +
    (cur && !list.some(o => o.value === cur) ? `<option value="${esc(cur)}" selected>${esc(label || cur)}</option>` : cur ? '' : `<option value="" selected disabled>–</option>`)
  return `<select class="set-model" aria-label="${esc(t('msgr.profile.model'))}">${options(launchOpts?.models || [], shortModelId(s.model), s.model)}</select> ` +
    `<select class="set-effort" aria-label="${esc(t('msgr.profile.effort'))}">${options(launchOpts?.efforts || [], s.effort || '')}</select>`
}
profileEl.addEventListener('change', async ev => {
  const sel = ev.target.closest('select.set-model, select.set-effort')
  if (!sel || !picked?.account) return
  const acc = sym.accountById(picked.account), s = picked.sid ? sym.sessionById(picked.sid) : deskOf(acc)
  if (!online(s)) return
  const r = await sym.setModel(s.id, sel.classList.contains('set-model') ? { model: sel.value } : { effort: sel.value })
  if (!r.ok) showError(errorText(r.data), { title: t('errlog.oops.say') })
})
profileEl.addEventListener('click', async ev => {
  if (ev.target.id === 'take-over') {
    const acc = sym.accountById(picked.account), s = picked.sid ? sym.sessionById(picked.sid) : deskOf(acc)
    const r = await sym.openTerminal(s.launch)
    if (!r.ok) showError(errorText(r.data) + (r.data?.params?.command ? '\n' + r.data.params.command : ''), { title: t('errlog.oops.terminal') })
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
