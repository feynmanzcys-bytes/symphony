// The approval page (/approve): the pending asks as cards, kept up by the bridge's event stream.
import { t, applyI18n, langPicker } from '/i18n.js'
import { esc, syncCards } from '/asks.js'

applyI18n()
document.querySelector('header nav').before(langPicker())

const list = document.getElementById('list')
const statusEl = document.getElementById('status')
const onlySession = new URLSearchParams(location.search).get('session')
let sessions = new Map(), accounts = new Map()

if (onlySession) document.getElementById('filter').innerHTML = `${esc(t('approve.filtered'))} · <a href="/approve">${esc(t('approve.showAll'))}</a>`

const who = a => {
  // a task session goes by its member's name (bob-T7), so it isn't taken for the account's desk session
  const s = sessions.get(a.session), acc = accounts.get(a.account)?.name
  const name = s?.task ? s.title || `${acc ?? a.session.slice(0, 6)}-${s.task}` : acc ?? s?.title ?? a.session.slice(0, 6)
  return `<div class="who"><span class="name">${esc(name)}</span>
    <span class="kind">${a.agentId ? esc(t('common.subagent')) + ' ' + esc(a.agentId.slice(0, 6)) + ' · ' : ''}${esc(t(a.kind === 'question' ? 'asks.question' : 'asks.permission'))}</span></div>`
}

function render(snap) {
  sessions = new Map(snap.sessions.map(s => [s.id, s]))
  accounts = new Map(snap.accounts.map(a => [a.id, a]))
  const asks = snap.asks.filter(a => !onlySession || a.session === onlySession)
  syncCards(list, asks, who, `<div class="empty">${esc(t('approve.empty'))}</div>`)
  document.title = asks.length ? t('approve.titleCount', { n: asks.length }) : t('approve.title')
}

fetch('/api/state').then(r => r.json()).then(render).catch(() => {})
const es = new EventSource('/api/stream')
es.onopen = () => statusEl.textContent = t('approve.connected')
es.onerror = () => statusEl.textContent = t('approve.reconnecting')
es.onmessage = m => {
  const msg = JSON.parse(m.data)
  if (msg.type === 'snapshot') render(msg)
}
