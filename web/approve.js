// The approval page (/approve): the pending asks as cards, kept up by the bridge's client (symphony.js: the stream,
// let go while the page is hidden, and the connection banner while the bridge can't be reached).
import { t, applyI18n, langPicker } from '/i18n.js'
import { esc, syncCards } from '/asks.js'
import { connect } from '/symphony.js'
import { connectionBanner } from '/ui/banner.js'
import { announceAsks } from '/ui/a11y.js'
import { askKind } from '/ui/common.js'

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
    <span class="kind">${a.agentId ? esc(t('common.subagent')) + ' ' + esc(a.agentId.slice(0, 6)) + ' · ' : ''}${esc(askKind(a.kind))}</span></div>`
}

function render(snap) {
  sessions = new Map(snap.sessions.map(s => [s.id, s]))
  accounts = new Map(snap.accounts.map(a => [a.id, a]))
  const asks = snap.asks.filter(a => !onlySession || a.session === onlySession)
  syncCards(list, asks, who, `<div class="empty">${esc(t('approve.empty'))}</div>`)
  document.title = asks.length ? t('approve.titleCount', { n: asks.length }) : t('approve.title')
}

const sym = connect()
connectionBanner(sym)
announceAsks(sym)
sym.on('snapshot', render)
sym.on('connection', up => { statusEl.textContent = t(up ? 'approve.connected' : 'approve.reconnecting') })
