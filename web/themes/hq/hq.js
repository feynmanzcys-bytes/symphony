// HQ: a big company's management console. The sessions as a roster with their figures (the state, the model, the
// context window's fill, the tool calls, the turns, today's cost), the pending asks as a queue, the tasks as a board,
// the activity as a timeline, and an overview with the figures that matter. Everything comes from the bridge's client
// (symphony.js); the employee window, the launch dialog and the task dialog and window are the shared pieces (ui/*).
import { t, lang, errorText, applyI18n, langPicker } from '/i18n.js'
import { esc, syncCards } from '/asks.js'
import { connect } from '/symphony.js'
import { STATE_MARK, TASK_MARK, base, lower, dot, buildText, statusText as statusOf, setHTML, deferWhilePressed, askKind, askSummary } from '/ui/common.js'
import { connectionBanner } from '/ui/banner.js'
import { announceAsks } from '/ui/a11y.js'
import { createChat } from '/ui/chat.js'
import { createLaunchDialog } from '/ui/launch.js'
import { createTaskUI } from '/ui/task.js'
import { themePicker } from '/ui/themes.js'

const sym = connect()
let snapshot = sym.state
// the tab's title counts the asks: kept up with every snapshot, also while the page is hidden (no frames then)
sym.on('snapshot', s => { snapshot = s; document.title = (s.asks.length ? `(${s.asks.length}) ` : '') + t('hq.title') })
const statusText = s => statusOf(sym, s)
applyI18n()
connectionBanner(sym)
announceAsks(sym)

// ---------- dark or light: the system's choice, or the switch's, remembered ----------
const DARK_STORE = 'symphony.hq.dark'
const darkBtn = document.getElementById('dark')
darkBtn.setAttribute('aria-label', t('hq.dark')) // a switch: its name stays, aria-pressed says whether it's on
function setDark(on, remember = true) {
  document.documentElement.toggleAttribute('data-dark', on)
  darkBtn.textContent = on ? '☀' : '☾'
  darkBtn.title = t(on ? 'hq.light' : 'hq.dark')
  darkBtn.setAttribute('aria-pressed', String(on))
  if (remember) try { localStorage.setItem(DARK_STORE, on ? '1' : '0') } catch {}
}
setDark((() => { try { const v = localStorage.getItem(DARK_STORE); if (v !== null) return v === '1' } catch {} return matchMedia('(prefers-color-scheme: dark)').matches })(), false)
darkBtn.onclick = () => setDark(!document.documentElement.hasAttribute('data-dark'))
const prefs = document.querySelector('.top .prefs'), langSel = langPicker()
prefs.appendChild(langSel)
themePicker(sym).then(sel => prefs.insertBefore(sel, langSel))

// ---------- the tabs: the one in the address's hash, remembered ----------
const TABS = ['overview', 'people', 'approvals', 'projects', 'activity']
const tabsEl = document.getElementById('tabs')
let tab = TABS.includes(location.hash.slice(1)) ? location.hash.slice(1) : 'overview'
function showTab(name) {
  tab = name
  for (const b of tabsEl.querySelectorAll('button')) {
    b.classList.toggle('on', b.dataset.tab === name)
    if (b.dataset.tab === name) b.setAttribute('aria-current', 'true'); else b.removeAttribute('aria-current') // (an empty aria-current means false)
  }
  for (const s of document.querySelectorAll('main > section')) s.hidden = s.dataset.tab !== name
  if (location.hash !== '#' + name) history.replaceState(null, '', '#' + name)
  refreshActivity(true) // a tab that shows it: the log as it is now
  render()
}
tabsEl.addEventListener('click', ev => { const b = ev.target.closest('button[data-tab]'); if (b) showTab(b.dataset.tab) })
addEventListener('hashchange', () => { if (TABS.includes(location.hash.slice(1))) showTab(location.hash.slice(1)) })

// ---------- figures ----------
const fmtNum = new Intl.NumberFormat(lang, { notation: 'compact', maximumFractionDigits: 1 })
const fmtUsd = n => '$' + (n >= 100 ? n.toFixed(0) : n >= 10 ? n.toFixed(1) : n.toFixed(2))
// one formatter for every time shown: toLocaleTimeString with options makes a new one on each call, which made the
// timeline's 300 rows cost a frame's time on every draw
const timeFmt = new Intl.DateTimeFormat(lang, { hour: '2-digit', minute: '2-digit' })
const fmtTime = when => { const d = new Date(when); return isNaN(d) ? '' : timeFmt.format(d) }
const uptime = s => { const m = Math.max(0, Math.round((Date.now() - Date.parse(s.started)) / 60000)); return t('hq.uptime', { h: Math.floor(m / 60), m: m % 60 }) }
const online = s => sym.isOnline(s)
const desks = () => snapshot.accounts.filter(a => !a.advisor)
const liveSessions = () => snapshot.sessions.filter(online)
const costToday = () => snapshot.ledger.reduce((n, r) => n + r.usd, 0)
const costOf = acc => snapshot.ledger.filter(r => r.account === acc.id).reduce((n, r) => n + r.usd, 0)
const groupOf = s => !online(s) ? 'off' : s.state === 'waiting' || sym.asking(s.id) ? 'waiting' : s.state === 'idle' ? 'idle' : 'busy'
// the task an account is away in: a member session online
const taskOf = acc => snapshot.tasks.find(k => k.members.some(m => m.account === acc.id && online(sym.sessionById(m.session)))) ?? null

// ---------- the KPI strip ----------
function renderKPIs() {
  const live = liveSessions(), waiting = snapshot.asks.length
  const working = live.filter(s => s.state === 'thinking' || s.state === 'tool').length
  const ctx = live.map(s => s.usage?.context?.percent).filter(p => typeof p === 'number')
  const avg = ctx.length ? Math.round(ctx.reduce((a, b) => a + b, 0) / ctx.length) : null
  const errors = live.filter(s => s.lastError).length
  const tiles = [
    ['headcount', String(desks().length), 'people'], ['online', String(live.length), 'people'], ['working', String(working), 'people'],
    ['waiting', String(waiting), 'approvals', waiting > 0], ['costToday', fmtUsd(costToday()), 'overview'],
    ['context', avg === null ? '—' : avg + '<small>%</small>', 'people'], ['errors', String(errors), 'activity', errors > 0],
  ]
  setHTML(document.getElementById('kpis'), tiles.map(([k, v, to, hot]) =>
    `<div class="kpi click${hot ? ' hot' : ''}" data-tab="${to}" role="button" tabindex="0"><div class="lbl">${esc(t('hq.kpi.' + k))}</div><div class="val">${v}</div></div>`).join(''))
  const count = (name, n, hot) => { const el = tabsEl.querySelector(`[data-tab=${name}] .count`); el.hidden = !n; if (el.textContent !== String(n)) el.textContent = n; el.classList.toggle('hot', !!hot) }
  count('approvals', waiting, waiting > 0)
  count('projects', snapshot.tasks.length, false)
}
document.getElementById('kpis').addEventListener('click', ev => { const k = ev.target.closest('.kpi'); if (k) showTab(k.dataset.tab) })

// ---------- the employee window, the launch dialog, the tasks ----------
let chatFor = null, chatSid = null
const askWho = a => {
  const acc = sym.accountById(a.account)
  return `<div class="who"><span class="name">${esc(acc?.name || '?')}</span><span class="kind">${esc(askKind(a.kind))}` +
    (a.agentId ? ` · ${esc(t('common.subagent'))}` : '') + `</span><span class="open" data-session="${esc(a.session)}" role="button" tabindex="0">${esc(t('hq.btn.chat'))}</span></div>`
}
const askWhoInChat = a => `<div class="who"><span class="name">${esc(askKind(a.kind))}</span>` + (a.agentId ? `<span class="kind">${esc(t('common.subagent'))}</span>` : '') + `</div>`
const chat = createChat(sym, { askWho: askWhoInChat, onClose: () => { chatFor = null; chatSid = null }, onTaskChip: id => tasks.openWindow(id) })
const launch = createLaunchDialog(sym, { options: loadOptions, onOnline: l => openChat(l.account), onOpen: () => chat.close(), log: () => {} })
const tasks = createTaskUI(sym, {
  options: loadOptions,
  roomLabel: task => task.title.split('\n')[0].slice(0, 48),
  memberAvatar: m => avatar(m?.name || '?'), youAvatar: () => avatar(t('task.you'), true),
  isMemberOpen: (task, m) => !chat.hidden && chatSid === m.session,
  onOpenMember: (task, m) => openChat(m.account, m.session, task.id),
  onWindowOpen: () => chat.close(), onWindowClose: () => chat.close(), onDialogOpen: () => { chat.close(); tasks.closeWindow() },
  log: () => {},
})
chat.el.classList.add('stacked') // over the task window when both are open
// a figure for the team chat: a round badge with the first letter, colored by the name
function avatar(name, me = false) {
  const h = [...name].reduce((n, c) => (n * 31 + c.codePointAt(0)) >>> 0, 7), hue = me ? 220 : h % 360
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 24"><circle cx="24" cy="12" r="11" fill="hsl(${hue} 45% ${me ? 30 : 48}%)"/><text x="24" y="16.5" text-anchor="middle" font-family="sans-serif" font-size="13" font-weight="600" fill="#fff">${esc([...name][0].toUpperCase())}</text></svg>`
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

// the window for an account's desk session, or for one of its task sessions (sid, from the task window)
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
  const task = chatSid ? sym.taskOfSession(chatSid) : taskOf(acc)
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
    state: on ? (sym.asking(s.id) ? 'waiting' : s.state) : 'ended',
    line: statusText(on ? s : null),
    taskChip: task ? { id: task.id, title: task.title, label: `${task.id} · ${task.title.split('\n')[0].slice(0, 40)}` } : null,
    chips: [['model', on ? buildText(s) : ''], ['session', s?.title], ['dir', s?.cwd || (chatSid ? member?.worktree : acc.last?.dir || acc.dir)],
      ['subagents', on ? s.agentsTotal || Object.keys(s.agents || {}).length || '' : ''], ['tools', on ? s.tools : ''], ['login', s?.login]],
    buttons, buttonsKey: `${chatFor}|${chatSid}|${on}|${busy}|${on && !!s.launch}|${!!launchOpts?.enabled}|${task?.id}|${member?.session}`,
    terminal: on && s.launch ? s.launch : null, // its terminal in the window too
    session: chatSid || s?.id || null, online: on,
    offText: chatSid ? t('task.offline') : task && member ? t('chat.awayInTask', { id: task.id, name: member.name }) : t('chat.offline'),
  })
}

// ---------- overview ----------
function renderOverview() {
  // who waits for you, a click opens the window
  const waiting = snapshot.asks.map(a => ({ a, acc: sym.accountById(a.account), s: sym.sessionById(a.session) }))
  setHTML(document.getElementById('ov-waiting'), waiting.length ? waiting.map(({ a, acc, s }) =>
    `<div class="row click" data-session="${esc(a.session)}" data-account="${esc(a.account)}" role="button" tabindex="0">${dot('waiting')}<b>${esc(nameOf(s, acc))}</b><span class="txt">${esc(askKind(a.kind))}${esc(t('common.colon'))}${esc(a.tool ? a.tool + ' ' : '')}${esc(summaryOf(a))}</span><time>${esc(fmtTime(a.created))}</time></div>`).join('')
    : `<div class="empty">${esc(t('hq.overview.nothingWaiting'))}</div>`)
  // the activity of the last 30 minutes, events per minute
  const now = Date.now(), bins = new Array(30).fill(0)
  for (const a of activity) { const i = 29 - Math.floor((now - Date.parse(a.time)) / 60000); if (i >= 0 && i < 30) bins[i]++ }
  sparkline(document.getElementById('ov-spark'), bins, i => t('hq.events', { n: bins[i] }) + ' · ' + fmtTime(now - (29 - i) * 60000))
  // today's cost by employee, and the tokens by model over the sessions online
  const byAcc = desks().map(acc => [acc.name, costOf(acc)]).filter(([, n]) => n > 0).sort((a, b) => b[1] - a[1]).slice(0, 8)
  bars(document.getElementById('ov-cost'), byAcc, fmtUsd, t('hq.overview.noCost'))
  const byModel = new Map()
  for (const s of liveSessions()) for (const [model, tk] of Object.entries(s.usage?.totals || {})) byModel.set(model, (byModel.get(model) || 0) + tk.input + tk.output)
  bars(document.getElementById('ov-tokens'), [...byModel].sort((a, b) => b[1] - a[1]), n => fmtNum.format(n), t('hq.overview.noTokens'))
  // the last things that happened
  setHTML(document.getElementById('ov-recent'), activity.length ? activity.slice(-8).reverse().map(activityRow).join('') : `<div class="empty">${esc(t('hq.overview.noActivity'))}</div>`)
}
const nameOf = (s, acc) => (s?.task && s.title) ? s.title : acc?.name || s?.title || '?'
const summaryOf = askSummary
document.getElementById('ov-waiting').addEventListener('click', ev => { const r = ev.target.closest('.row'); if (r) openAsk(r.dataset.session, r.dataset.account) })
// an ask's session: a task member's window from the task window, else the employee's
function openAsk(sid, account) {
  const task = sym.taskOfSession(sid), m = task?.members.find(x => x.session === sid)
  if (m) { tasks.openWindow(task.id); openChat(m.account, m.session, task.id) } else openChat(account)
}

// a sparkline: one series, a line over a faint area, a value on hover; drawn in the box's own pixels
function sparkline(el, values, tipOf) {
  const W = el.clientWidth || 300, H = el.clientHeight || 150, pad = 16, max = Math.max(1, ...values), n = values.length
  const x = i => pad + i * (W - 2 * pad) / (n - 1), y = v => H - pad - v * (H - 2 * pad) / max
  const pts = values.map((v, i) => `${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(' ')
  const step = (W - 2 * pad) / (n - 1)
  const drawn = setHTML(el, `<svg viewBox="0 0 ${W} ${H}" width="${W}" height="${H}">
    <line class="axis" x1="${pad}" y1="${H - pad}" x2="${W - pad}" y2="${H - pad}"/>
    <line class="grid" x1="${pad}" y1="${y(max)}" x2="${W - pad}" y2="${y(max)}"/>
    <text x="${pad}" y="${y(max) - 4}">${max}</text>
    <polygon class="area" points="${x(0)},${H - pad} ${pts} ${x(n - 1)},${H - pad}"/>
    <polyline class="line" points="${pts}"/>
    ${values.map((v, i) => `<rect class="hit" data-i="${i}" x="${(x(i) - step / 2).toFixed(1)}" y="0" width="${step.toFixed(1)}" height="${H}"/>`).join('')}
    <line class="cross" x1="0" y1="${pad}" x2="0" y2="${H - pad}" visibility="hidden"/></svg><div class="tip" hidden></div>`)
  if (!drawn) return // the same as before, with its hover already
  const svg = el.querySelector('svg'), cross = el.querySelector('.cross'), tip = el.querySelector('.tip')
  svg.addEventListener('mousemove', ev => {
    const hit = ev.target.closest('.hit')
    if (!hit) return
    const i = +hit.dataset.i
    cross.setAttribute('x1', x(i)); cross.setAttribute('x2', x(i)); cross.setAttribute('visibility', 'visible')
    tip.hidden = false; tip.textContent = tipOf(i)
    tip.style.left = x(i) + 'px'; tip.style.top = (y(values[i]) - 6) + 'px'
  })
  svg.addEventListener('mouseleave', () => { cross.setAttribute('visibility', 'hidden'); tip.hidden = true })
}
// horizontal bars, one hue, the value in text at the end of each; drawn in the box's own pixels
function bars(el, rows, fmt, emptyText) {
  if (!rows.length) { el.style.height = ''; setHTML(el, `<div class="none">${esc(emptyText)}</div>`); return }
  const W = el.clientWidth || 300, rowH = 24, H = rows.length * rowH + 8, labelW = Math.min(150, Math.round(W * 0.36)), max = Math.max(...rows.map(r => r[1])) || 1
  el.style.height = Math.max(60, H) + 'px'
  const cut = s => { s = String(s); return s.length > 18 ? s.slice(0, 17) + '…' : s }
  setHTML(el, `<svg viewBox="0 0 ${W} ${H}" width="${W}" height="${H}">` + rows.map(([name, v], i) => {
    const y = 4 + i * rowH, w = Math.max(2, (v / max) * (W - labelW - 70))
    return `<text x="${labelW - 8}" y="${y + 15}" text-anchor="end"><title>${esc(String(name))}</title>${esc(cut(name))}</text>` +
      `<rect class="bar" x="${labelW}" y="${y + 4}" width="${w.toFixed(1)}" height="${rowH - 10}" rx="3"><title>${esc(name + ': ' + fmt(v))}</title></rect>` +
      `<text class="val" x="${labelW + w + 6}" y="${y + 15}">${esc(fmt(v))}</text>`
  }).join('') + '</svg>')
}

// ---------- people ----------
const COLS = ['name', 'state', 'model', 'folder', 'project', 'context', 'tools', 'turns', 'cost', 'uptime', 'actions']
let sortBy = 'state', sortAsc = true, chipOn = null
const thead = document.querySelector('#people thead tr')
thead.innerHTML = COLS.map(c => `<th data-col="${c}" scope="col">${c === 'actions' ? '' : `<button type="button" class="sort">${esc(t('hq.col.' + c))}</button>`}</th>`).join('')
thead.addEventListener('click', ev => {
  const th = ev.target.closest('th'), col = th?.dataset.col
  if (!col || col === 'actions') return
  if (sortBy === col) sortAsc = !sortAsc; else { sortBy = col; sortAsc = true }
  renderPeople()
})
const GROUP_ORDER = { waiting: 0, busy: 1, idle: 2, off: 3 }
function peopleRows() {
  return desks().map(acc => {
    const s = sym.sessionOf(acc), on = online(s), task = taskOf(acc), g = s ? groupOf(s) : 'off'
    return { acc, s, on, task, g, cost: costOf(acc), ctx: s?.usage?.context?.percent ?? -1 }
  })
}
function renderPeople() {
  const rows = peopleRows()
  const n = { idle: 0, busy: 0, waiting: 0, off: 0 }
  for (const r of rows) n[r.g]++
  setHTML(document.getElementById('chips'), [['idle', 'idle', 'stats.idle'], ['busy', 'tool', 'stats.working'], ['waiting', 'waiting', 'stats.waiting'], ['off', 'ended', 'stats.offDuty']]
    .map(([g, st, key]) => `<button class="chip${chipOn === g ? ' on' : ''}${g === 'waiting' && n.waiting ? ' hot' : ''}" data-group="${g}" style="--c:${STATE_MARK[st]}" aria-pressed="${chipOn === g}"><i aria-hidden="true"></i>${esc(t(key, { n: n[g] }))}</button>`).join(''))
  const q = lower(findEl.value.trim())
  const shown = rows.filter(r => (!chipOn || r.g === chipOn) && (!q || matches(r, q)))
  const key = r => ({
    name: lower(r.acc.name), state: GROUP_ORDER[r.g] + (r.s?.state || ''), model: lower(r.s?.model || '~'), folder: lower(base(r.s?.cwd || r.acc.dir || '')),
    project: r.task?.id || '~', context: r.ctx, tools: r.s?.tools || 0, turns: r.s?.turns || 0, cost: r.cost, uptime: r.on ? Date.parse(r.s.started) : Infinity,
  })[sortBy]
  shown.sort((a, b) => { const x = key(a), y = key(b); return (x < y ? -1 : x > y ? 1 : 0) * (sortAsc ? 1 : -1) || a.acc.name.localeCompare(b.acc.name, lang) })
  for (const th of thead.children) {
    th.classList.toggle('on', th.dataset.col === sortBy); th.classList.toggle('asc', th.dataset.col === sortBy && sortAsc)
    if (th.dataset.col === sortBy) th.setAttribute('aria-sort', sortAsc ? 'ascending' : 'descending'); else th.removeAttribute('aria-sort')
  }
  setHTML(document.querySelector('#people tbody'), shown.map(r => {
    const { acc, s, on, task } = r, state = on ? (sym.asking(s.id) ? 'waiting' : s.state) : 'ended'
    const ctx = r.ctx >= 0 ? `<span class="ctx${r.ctx >= 90 ? ' full' : r.ctx >= 70 ? ' high' : ''}" title="${esc(fmtNum.format(s.usage.context.tokens || 0) + ' / ' + fmtNum.format(s.usage.context.window))}"><i style="width:${Math.min(100, r.ctx)}%"></i></span>${Math.round(r.ctx)}%` : '<span class="dim">—</span>'
    const acts = [on && s.launch ? `<button class="px" data-act="terminal" title="${esc(t('chat.terminalTitle'))}">${esc(t('hq.btn.terminal'))}</button>` : '',
      !on && launchOpts?.enabled ? `<button class="px primary" data-act="start">${esc(t('launch.go'))}</button>` : '',
      `<button class="px" data-act="open">${esc(t('hq.btn.open'))}</button>`].join('')
    return `<tr data-account="${esc(acc.id)}" class="${on ? '' : 'off'}">` +
      `<td class="name">${dot(state)}<b>${esc(acc.name)}</b>${s?.title && s.title !== acc.name ? `<small>${esc(s.title)}</small>` : ''}</td>` +
      `<td>${esc(on ? statusText(s) : t('state.ended'))}</td>` +
      `<td class="dim">${esc(on && s.model ? s.model + (s.effort ? ' · ' + s.effort : '') : acc.last?.model ? acc.last.model + (acc.last.effort ? ' · ' + acc.last.effort : '') : '')}</td>` +
      `<td class="dir" title="${esc(s?.cwd || acc.last?.dir || acc.dir || '')}">${esc(base(s?.cwd || acc.last?.dir || acc.dir || '') === '?' ? '' : base(s?.cwd || acc.last?.dir || acc.dir))}</td>` +
      `<td>${task ? `<span class="badge" title="${esc(task.title)}">${esc(task.id)}</span>` : ''}</td>` +
      `<td>${ctx}</td><td>${on ? s.tools : ''}</td><td>${on ? s.turns || 0 : ''}</td><td>${r.cost ? fmtUsd(r.cost) : '<span class="dim">—</span>'}</td>` +
      `<td class="dim">${on ? esc(uptime(s)) : ''}</td><td class="acts">${acts}</td></tr>`
  }).join(''))
  document.getElementById('people-empty').hidden = shown.length > 0
}
document.getElementById('chips').addEventListener('click', ev => { const c = ev.target.closest('.chip'); if (!c) return; chipOn = chipOn === c.dataset.group ? null : c.dataset.group; renderPeople() })
document.querySelector('#people tbody').addEventListener('click', async ev => {
  const tr = ev.target.closest('tr'), act = ev.target.closest('button')?.dataset.act
  if (!tr) return
  const acc = sym.accountById(tr.dataset.account), s = acc && sym.sessionOf(acc)
  if (act === 'terminal' && s?.launch) { const r = await sym.openTerminal(s.launch); if (!r.ok) alert(errorText(r.data)); return }
  if (act === 'start') return launch.open(acc.id)
  openChat(acc.id)
})
const matches = (r, q) => [r.acc.name, r.s?.title, base(r.s?.cwd || r.acc.dir || ''), r.task?.id].some(v => lower(v).includes(q))

// ---------- the search box: employees by name, session name or folder; tasks by id or title ----------
const findEl = document.getElementById('find'), findList = document.getElementById('find-list')
findEl.setAttribute('aria-label', t('hq.search'))
let findSel = 0
function findHits() {
  const q = lower(findEl.value.trim())
  if (!q) return []
  const people = peopleRows().filter(r => matches(r, q)).map(r => ({ key: r.acc.id, name: r.acc.name, state: r.on ? (sym.asking(r.s.id) ? 'waiting' : r.s.state) : 'ended', sub: r.on ? statusText(r.s) : t('state.ended'), r }))
  const ts = snapshot.tasks.filter(k => lower(k.id).includes(q) || lower(k.title).includes(q)).map(k => ({ key: 'task:' + k.id, name: k.id, state: k.status === 'failed' ? 'waiting' : 'tool', sub: k.title.split('\n')[0], task: k }))
  return [...people, ...ts].slice(0, 8)
}
function renderFind() {
  const hits = findHits()
  findSel = Math.max(0, Math.min(findSel, hits.length - 1))
  findList.hidden = !findEl.value.trim() || document.activeElement !== findEl
  findList.innerHTML = hits.map((h, i) => `<div class="hit${i === findSel ? ' on' : ''}" data-key="${esc(h.key)}" role="option" id="find-hit-${i}" aria-selected="${i === findSel}">${dot(h.state)}<b>${esc(h.name)}</b><small>${esc(h.sub)}</small></div>`).join('') || `<div class="none" role="option" aria-disabled="true">${esc(t('search.none'))}</div>`
  findEl.setAttribute('aria-expanded', String(!findList.hidden))
  if (!findList.hidden && hits.length) findEl.setAttribute('aria-activedescendant', 'find-hit-' + findSel)
  else findEl.removeAttribute('aria-activedescendant')
  if (tab === 'people') renderPeople()
}
function pickFound(h) {
  findEl.value = ''; findSel = 0; findEl.blur(); renderFind()
  if (!h) return
  if (h.task) tasks.openWindow(h.task.id); else openChat(h.r.acc.id)
}
findEl.addEventListener('input', () => { findSel = 0; renderFind() })
findEl.addEventListener('focus', renderFind)
findEl.addEventListener('blur', () => { findList.hidden = true; findEl.setAttribute('aria-expanded', 'false'); findEl.removeAttribute('aria-activedescendant') })
findEl.addEventListener('keydown', ev => {
  if (ev.key === 'ArrowDown' || ev.key === 'ArrowUp') { findSel += ev.key === 'ArrowDown' ? 1 : -1; renderFind() }
  else if (ev.key === 'Enter') pickFound(findHits()[findSel])
  else if (ev.key === 'Escape') { findEl.value = ''; renderFind(); findEl.blur() }
  else return
  ev.preventDefault()
})
findList.addEventListener('mousedown', ev => { const row = ev.target.closest('.hit'); if (!row) return; ev.preventDefault(); pickFound(findHits().find(h => h.key === row.dataset.key)) })
addEventListener('keydown', ev => {
  if (ev.key === 'Escape') { if (!chat.hidden) { if (!chat.asking) chat.close() } else if (!tasks.win.hidden) tasks.closeWindow(); if (!tasks.dialogHidden) tasks.closeDialog(); if (!launch.hidden) launch.close() }
  if (ev.key !== '/' || ev.ctrlKey || ev.metaKey || ev.altKey || !chat.hidden || !launch.hidden || !tasks.dialogHidden || !tasks.win.hidden) return
  if (/^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement?.tagName)) return
  ev.preventDefault()
  findEl.focus()
})

// ---------- approvals: the queue, the shared cards ----------
let asksShown = ''
function renderApprovals() {
  const key = snapshot.asks.map(a => a.id).join(',')
  if (key === asksShown) return
  asksShown = key
  syncCards(document.getElementById('queue'), snapshot.asks, askWho, `<div class="empty">${esc(t('hq.empty.approvals'))}</div>`)
}
document.getElementById('queue').addEventListener('click', ev => { const o = ev.target.closest('.open'); if (o) openAsk(o.dataset.session, snapshot.asks.find(a => a.session === o.dataset.session)?.account) })

// ---------- projects: the board ----------
function renderProjects() {
  const cols = [['running', TASK_MARK.running], ['ending', TASK_MARK.ending], ['failed', TASK_MARK.failed]]
  setHTML(document.getElementById('board'), cols.map(([st, c]) => {
    const list = snapshot.tasks.filter(k => k.status === st)
    return `<div class="col"><h2><i style="--c:${c}"></i>${esc(t('hq.projects.' + st))}<span class="badge">${list.length}</span></h2>` + (list.map(task =>
      `<div class="task" data-task="${esc(task.id)}" role="button" tabindex="0"><div class="hd"><b>${esc(task.id)}</b>${task.leader ? `<span class="lead">${esc(t('task.lead'))}${esc(t('common.colon'))}${esc(task.members.find(m => m.account === task.leader)?.name || '')}</span>` : `<span class="lead">${esc(t('task.flat'))}</span>`}</div>` +
        `<div class="title">${esc(task.title)}</div>` + task.members.map(m => { const s = sym.sessionById(m.session), on = online(s); return `<div class="mem">${dot(!on ? 'ended' : sym.asking(m.session) ? 'waiting' : s.state)}<b>${esc(m.name)}</b><span class="dim">${esc(m.branch)}</span><span class="st">${esc(!m.session ? t('task.memberStarting') : on ? statusText(s) : t('task.memberEnded'))}</span></div>` }).join('') +
        (task.status === 'failed' ? `<div class="err">${esc(errorText({ code: task.errorCode, params: task.errorParams, error: task.error }))}</div>` : '') + `</div>`).join('') || `<div class="empty">${esc(t('hq.empty.projects'))}</div>`) + '</div>'
  }).join(''))
}
document.getElementById('board').addEventListener('click', ev => { const k = ev.target.closest('.task'); if (k) tasks.openWindow(k.dataset.task) })

// ---------- activity: the bridge's log (/api/activity), then every push that adds to it ----------
// Fetched only for a tab that shows it (the overview, the timeline), on a page that's seen, and not more than every
// few seconds however many pushes come; again when such a tab opens, the page shows again or the stream comes back.
const ACT_GAP = 3000
let activity = [], actPending = null, actAt = 0
async function loadActivity() {
  actAt = Date.now()
  const list = await sym.fetchActivity()
  if (list) activity = list
  if (tab === 'activity' || tab === 'overview') render()
}
function refreshActivity(now = false) {
  if (actPending || (tab !== 'activity' && tab !== 'overview') || document.visibilityState === 'hidden') return
  actPending = setTimeout(() => { actPending = null; loadActivity() }, Math.max(now ? 0 : 800, actAt + ACT_GAP - Date.now()))
}
for (const type of ['event', 'message', 'ask', 'settled', 'launch']) sym.on(type, () => refreshActivity())
sym.on('resync', () => refreshActivity(true))
document.addEventListener('visibilitychange', () => refreshActivity(true))
const actWho = document.getElementById('act-who'), actKind = document.getElementById('act-kind')
const KINDS = ['start', 'end', 'tool', 'ask', 'settled', 'message', 'error', 'launch', 'launchFailed']
function activityRow(a) {
  return `<div class="row"><time>${esc(fmtTime(a.time))}</time><b title="${esc(a.name)}">${esc(a.name || '')}</b><span class="badge ${esc(a.kind)}">${esc(t('hq.activity.kind.' + a.kind))}</span><span class="txt" title="${esc(a.text)}">${esc(a.text || '')}</span>${a.task ? `<span class="dim">${esc(a.task)}</span>` : ''}</div>`
}
function renderActivity() {
  const names = [...new Set(activity.map(a => a.name).filter(Boolean))].sort((a, b) => a.localeCompare(b, lang))
  const who = actWho.value, kind = actKind.value
  // (the options are put in again only when they change: that would close a list someone has open)
  const opts = (sel, list, all) => { const v = sel.value; if (setHTML(sel, `<option value="">${esc(all)}</option>` + list.map(([val, label]) => `<option value="${esc(val)}">${esc(label)}</option>`).join(''))) sel.value = list.some(([val]) => val === v) ? v : '' }
  opts(actWho, names.map(n => [n, n]), t('hq.activity.everyone'))
  opts(actKind, KINDS.map(k => [k, t('hq.activity.kind.' + k)]), t('hq.activity.everything'))
  const shown = activity.filter(a => (!who || a.name === who) && (!kind || a.kind === kind)).slice(-300).reverse()
  setHTML(document.getElementById('act-list'), shown.length ? shown.map(activityRow).join('') : `<div class="empty">${esc(t('hq.empty.activity'))}</div>`)
}
actWho.onchange = actKind.onchange = renderActivity

// ---------- every change: the strip and the tab shown, once per frame however many pushes come, and not while a
// pointer is down (a row replaced under a press loses its click); the windows are the shared pieces' business ----------
let drawing = false
function render() {
  if (drawing) return
  drawing = true
  requestAnimationFrame(() => { drawing = false; draw() })
}
const draw = deferWhilePressed(() => {
  renderKPIs()
  if (tab === 'overview') renderOverview()
  else if (tab === 'people') renderPeople()
  else if (tab === 'approvals') renderApprovals()
  else if (tab === 'projects') renderProjects()
  else if (tab === 'activity') renderActivity()
  if (chatFor) showChat()
})
sym.on('change', render)
sym.on('protocol', (bridge, page) => { const note = document.getElementById('launch-note'); note.hidden = false; note.textContent = t('error.protocol_mismatch', { bridge, page }) })
setInterval(() => { if (tab === 'people' || tab === 'overview') render() }, 30000) // uptimes and the minutes move
showTab(tab)
loadOptions()
