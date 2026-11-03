// The employee window (#chat), shared by every theme: who they are at the top (the name, the state dot and line, chips,
// buttons: the theme says what), below it the session as a chat (the prompts, the answers as they stream, the messages
// between sessions; what the model does between two pieces of its answer, its thinking, the tool calls, the commands,
// the asks once settled, folded into one line that unfolds: "the steps"), and the dock at the bottom: the input, over
// which the pending asks' cards rise until they're handled. The log comes from the
// bridge when the window shows a session; each change comes as a chat push. Hidden, the window lets the pushes go by
// and fetches the log again when it shows; it fetches it again too after a resync (pushes may have been missed), and
// keeps as many items as the bridge does.
//
//   const chat = createChat(sym, { askWho, onClose, onTaskChip, asks })
//   asks: 'overlay' (the cards rise over the input from the dock) or 'inline' (each card sits in the flow, under its own
//   line, and the input stays usable for a message that doesn't answer it: Coworkers)
//   chat.show({ name, state, line, chips, taskChip, buttons, session, online, offText, terminal, avatar })   draw it for a session (again on every change)
//     avatar: a CSS url() of the employee's picture before its name (the pixel office's figure), or none
//   chat.setLine(text)      the status line alone (it follows what the employee does, every frame)
//   chat.close()            hide it (onClose is told)
//   chat.el                 the modal, for a theme that docks it beside another window
//   chat.showTerminal(on)   show the session's terminal in the window instead of the chat (or the chat again)
//
// terminal is the launch id of a page-started session that is online: the window then offers its terminal in the page
// (ui/terminal.js), interactive, with a button of its own beside the theme's "terminal" button (the one that opens a
// window on this machine), or first; the button, or Ctrl+], brings the chat back, and so does another session.
import { t, esc, errorText, richText, timeOf, STATE_MARK, GRAY_MARK, button, askKind } from '/ui/common.js'
import { askCard, syncCards, onAskFailure } from '/asks.js'
import { createTerminal } from '/ui/terminal.js'
import { dialog } from '/ui/a11y.js'
import { motion } from '/ui/motion.js'
import { pref, onPref, STEPS_OPEN } from '/ui/prefs.js'
import { showError, onBackdrop } from '/ui/errors.js'
import { markdown } from '/ui/markdown.js'

onAskFailure(text => showError(text)) // a card's failed answer, over the page and into the error log

const KEEP_ITEMS = 400 // the bridge's keepChat
const TEXT_MAX = 64 << 10 // a streamed block keeps its last 64 KiB (the bridge's chatTextMax)
const LIVE_MARKUP = 8192 // a streaming block up to this long is drawn with its markup on every piece
// what the model wrote (its answers and thinking, its messages, the bridge's kickoff) is Markdown; a prompt typed by a
// person keeps the little markup it had (code and bold), as typed
const typed = it => it.kind === 'user' && it.origin !== 'task'
const bodyHTML = it => typed(it) ? richText(it.text) : markdown(it.text)
const bodyClass = it => typed(it) ? 'body' : 'body md'

// ---------- the steps ----------
// What the model does between two pieces of its answer, its thinking, the tools it calls, the commands that ran and the
// asks once they're settled, folds into one line: "Thinking… 2m 38s" while it goes on (the time counts up), "Thought
// for 2m 38s" once it's done; a click unfolds the steps. A pending ask stays out of the fold (its card is answered in its
// line), and so do the prompts, the answer's text, the messages and the notices: each of those ends a fold. A fold
// starts where the turn did when it follows a prompt or a message (the model's first words take a while too), else at
// its first step; done, it ends where the answer's next piece starts, else at its last step's end. The time is the
// model's: what the asks in it waited for you is left out, and while an ask waits after it, it doesn't count on. A step
// that failed or was denied shows on the line. With motion paused the count goes by the minute.
const foldable = it => it.kind === 'thinking' || it.kind === 'tool' || it.kind === 'command' || (it.kind === 'ask' && (it.status || 'pending') !== 'pending')
const BUSY = new Set(['thinking', 'tool'])
const OPENER_GAP = 120000 // a prompt whose first step came this much later waited for its turn: its fold starts at that step
// a duration in this page's language: 38s, 2m 38s, 1h 5m; by the minute (2m) when it may change only once a minute
export function duration(ms, minutes = false) {
  const s = Math.max(1, Math.round(ms / 1000))
  if (minutes) return s < 3600 ? t('chat.dur.m', { m: Math.floor(s / 60) }) : t('chat.dur.hm', { h: Math.floor(s / 3600), m: Math.floor(s / 60) % 60 })
  if (s < 60) return t('chat.dur.s', { s })
  if (s < 3600) return t('chat.dur.ms', { m: Math.floor(s / 60), s: s % 60 })
  return t('chat.dur.hm', { h: Math.floor(s / 3600), m: Math.floor(s / 60) % 60 })
}

const MARKUP = `
<div class="modal" id="chat" hidden>
  <div class="panel chat">
    <div class="head">
      <button type="button" class="close"></button>
      <div class="who"><i class="av" aria-hidden="true" hidden></i><span class="dot" aria-hidden="true"></span><h2></h2><span class="state"></span></div>
      <div class="meta"></div>
      <div class="btns"></div>
    </div>
    <div class="termpane" hidden></div>
    <div class="flow" role="region"></div>
    <div class="dock">
      <div id="asks"></div>
      <form class="say" autocomplete="off" role="none">
        <textarea rows="2"></textarea>
        <button type="submit" class="px primary"></button>
      </form>
      <div class="offline" hidden></div>
    </div>
  </div>
</div>`

export function createChat(sym, { askWho, onClose = () => {}, onTaskChip = () => {}, asks: asksMode = 'overlay' } = {}) {
  const inline = asksMode === 'inline'
  const cards = new Map() // (inline) ask id -> its card: one element, moved into its line whenever the flow is drawn
  const tpl = document.createElement('template')
  tpl.innerHTML = MARKUP.trim()
  const el = tpl.content.firstElementChild
  document.body.appendChild(el)
  const head = el.querySelector('.head'), flow = el.querySelector('.flow'), dock = el.querySelector('.dock')
  const sayForm = el.querySelector('.say'), sayBox = sayForm.querySelector('textarea'), asksEl = el.querySelector('#asks')
  sayBox.placeholder = t('chat.placeholder')
  sayBox.setAttribute('aria-label', t('chat.placeholder'))
  dialog(el, head.querySelector('h2')) // a dialog over the page, a region where the theme lays it out (a11y.js)
  sayForm.querySelector('button').textContent = t('chat.send')

  let session = null // the session whose log the flow shows and pushes keep up (null while it is closed)
  let name = '' // who the window is for: the account's name, a member's name (bob-T7) for a task session
  const items = new Map() // item id -> item, of session
  const shown = { meta: '', btns: '', asks: '', avatar: '' }
  let inflight = null // the log's fetch on its way: { sid, again } (again: asked for once more meanwhile)

  el.querySelector('.close').onclick = () => chat.close()
  onBackdrop(el, () => chat.close()) // (not a drag that selects text and lets go outside the window)
  head.querySelector('.meta').addEventListener('click', ev => { const c = ev.target.closest('[data-task]'); if (c) onTaskChip(c.dataset.task) })

  // sending: Enter sends, Shift+Enter makes a new line; the bridge queues it for the session's mod, and the chat shows
  // it as queued until the mod has submitted it
  sayForm.addEventListener('submit', async ev => {
    ev.preventDefault()
    const text = sayBox.value.trim()
    if (!text || !session) return
    sayBox.value = ''
    const r = await sym.say(session, text)
    if (!r.ok) { showError(errorText(r.data), { title: t('errlog.oops.say') }); sayBox.value = text }
    sayBox.focus()
  })
  sayBox.addEventListener('keydown', ev => {
    if (ev.key === 'Enter' && !ev.shiftKey && !ev.isComposing) { ev.preventDefault(); sayForm.requestSubmit() }
  })

  // The log: one fetch at a time (asked for again meanwhile, it fetches once more after); what it brings is merged with
  // what pushes brought while it was on its way, the more complete version of an item staying
  async function load(sid) {
    if (inflight?.sid === sid) { inflight.again = true; return }
    const me = inflight = { sid, again: false }
    const r = await sym.get(`/api/chat/${sid}`)
    if (inflight === me) inflight = null
    if (sid !== session) return
    if (!r.ok) failed(errorText(r.data))
    else {
      for (const it of r.data?.items ?? []) { const had = items.get(it.id); if (!had || fuller(it, had)) items.set(it.id, it) }
      render()
    }
    if (me.again) load(sid)
  }
  // a fetched item against the one the pushes keep: the one done, else the one with more text (the fetch on a tie)
  const fuller = (a, b) => a.done !== b.done ? !!a.done : String(a.text ?? '').length >= String(b.text ?? '').length
  // the log couldn't be fetched: say so, unless there is a log to show already
  function failed(reason) {
    if (items.size) return
    flow.innerHTML = `<div class="empty err">${esc(t('chat.loadFailed', { reason }))}</div>`
  }
  function render() {
    const atEnd = flow.scrollTop + flow.clientHeight >= flow.scrollHeight - 40, top = flow.scrollTop
    // what was unfolded stays so: an item's own fold, and the steps by their first item
    const old = new Map([...flow.querySelectorAll('.it[data-id], .steps[data-id]')].map(n => [n.dataset.id, n]))
    trim()
    flow.innerHTML = ''
    const list = [...items.values()].sort((a, b) => a.seq - b.seq)
    if (!list.length) flow.innerHTML = `<div class="empty">${esc(t('chat.empty'))}</div>`
    let fold = null
    for (const it of list) {
      const node = itemEl(it, old.get(it.id))
      if (!foldable(it)) { fold = null; flow.appendChild(node); continue }
      if (!fold) fold = flow.appendChild(stepsEl(it.id, old.get('steps:' + it.id)))
      fold.querySelector('.list').appendChild(node)
    }
    labelSteps()
    ghost()
    if (inline) drawAsks()
    flow.scrollTop = atEnd ? flow.scrollHeight : top // it follows the end only if it was there
  }
  // as many items as the bridge keeps of a session (keepChat): the oldest go, and a fold they leave empty
  function trim() {
    if (items.size <= KEEP_ITEMS) return
    const list = [...items.values()].sort((a, b) => a.seq - b.seq)
    for (const it of list.slice(0, list.length - KEEP_ITEMS)) { items.delete(it.id); nodeOf(it.id)?.remove() }
    for (const f of flow.querySelectorAll('.steps')) if (!f.querySelector('.it')) f.remove()
  }
  const nodeOf = id => flow.querySelector(`.it[data-id="${CSS.escape(String(id))}"]`)

  // ---------- the steps (see foldable) ----------
  // a fold for the steps from the item first: unfolded as it was (old), else as the settings say
  function stepsEl(first, old) {
    const f = document.createElement('div')
    f.className = 'steps'
    f.dataset.id = 'steps:' + first
    const open = old ? !!old.querySelector(':scope > details')?.open : pref(STEPS_OPEN) === 'open'
    f.innerHTML = `<details${open ? ' open' : ''}><summary><span class="lbl"></span><span class="bad" hidden></span></summary><div class="list"></div></details>`
    return f
  }
  // the last thing in the flow: an item or a fold (not a card that waits for its line, nor the empty line)
  const lastBlock = () => { for (let n = flow.lastElementChild; n; n = n.previousElementSibling) if (n.matches('.it, .steps')) return n; return null }
  // a new item goes at the end: a step into the fold there (the one waiting for its first step too, or a new one),
  // anything else after it, which ends that fold
  function place(node, it) {
    const last = lastBlock()
    if (!foldable(it)) {
      if (last?.matches('.steps.ghost')) last.remove()
      flow.appendChild(node)
      if (last?.matches('.steps:not(.ghost)')) labelSteps(last)
      return
    }
    let fold = last?.matches('.steps:not(.ghost)') ? last : null
    if (!fold) {
      fold = stepsEl(it.id)
      if (last?.matches('.steps.ghost')) last.replaceWith(fold) // (the line that waited: a fold now, with its first step)
      else flow.appendChild(fold)
    }
    fold.querySelector('.list').appendChild(node)
    labelSteps(fold)
  }
  // Right after a prompt or a message, while the session works and nothing came yet: a line that counts from it, with
  // nothing to unfold yet; the first step makes it a fold (the answer's text takes it away).
  function ghost() {
    const last = lastBlock(), had = last?.matches('.steps.ghost') ? last : null
    const before = had ? (() => { for (let n = had.previousElementSibling; n; n = n.previousElementSibling) if (n.matches('.it, .steps')) return n; return null })() : last
    const want = busy() && !!before?.matches('.it.user:not(.queued):not(.dropped), .it.message')
    if (!want) { had?.remove(); return }
    let g = had
    if (!g) {
      g = flow.appendChild(document.createElement('div'))
      g.className = 'steps ghost running'
      g.innerHTML = `<div class="line"><span class="lbl"></span></div>`
    }
    labelSteps(g)
  }
  const itemOf = node => items.get(node?.matches('.steps') ? node.querySelector('.it')?.dataset.id : node?.dataset.id)
  const endOf = it => Date.parse(it.time) + (it.kind === 'tool' && it.done ? it.ms || 0 : it.kind === 'thinking' && it.done ? (it.secs || 0) * 1000 : 0)
  const busy = () => { const s = sym.sessionById(session); return sym.isOnline(s) && BUSY.has(s.state) }
  // a fold's line: whether it goes on (the last thing in the flow, but for asks waiting for you, while the session
  // works) and for how long; with no fold given, every fold's
  function labelSteps(fold) {
    if (!fold) { for (const f of flow.querySelectorAll('.steps')) labelSteps(f); return }
    const steps = [...fold.querySelectorAll('.list > .it')].map(itemOf).filter(Boolean)
    const prev = (() => { for (let n = fold.previousElementSibling; n; n = n.previousElementSibling) if (n.matches('.it, .steps')) return n; return null })()
    const opener = prev?.matches('.it.user, .it.message') ? itemOf(prev) : null
    if (!steps.length && !opener) return
    // what comes next ends the work: not a prompt that waits for its turn; an ask waiting for you doesn't either, but
    // the work stops counting while it waits
    let next = null, asking = false
    for (let n = fold.nextElementSibling; n && !next; n = n.nextElementSibling) {
      if (n.matches('.it.ask.pending')) asking = true
      else if (n.matches('.it, .steps') && !n.matches('.it.user.queued')) next = n
    }
    const first = steps.length ? Date.parse(steps[0].time) : NaN, from = opener ? Date.parse(opener.time) : first
    const start = opener && first - from > OPENER_GAP ? first : from
    const running = !next && !asking && busy()
    let end = steps.length ? Math.max(...steps.map(endOf)) : start
    if (next?.matches('.it.text')) end = Math.max(end, Date.parse(itemOf(next)?.time))
    const waited = steps.filter(s => s.kind === 'ask').reduce((n, s) => n + (s.ms || 0), 0) // (your time, not the model's)
    const took = Math.max(0, (running ? Date.now() : end) - start - waited)
    // (with motion paused: no time in its first minute, then whole minutes)
    const words = running && motion.still && took < 60000 ? t('chat.steps.runningName')
      : t(running ? 'chat.steps.running' : 'chat.steps.done', { time: duration(took, running && motion.still) })
    fold.classList.toggle('running', running)
    const lbl = fold.querySelector('.lbl'), sum = fold.querySelector('summary')
    if (lbl.textContent !== words) lbl.textContent = words
    if (!sum) return // (the line that waits for its first step: nothing to unfold, nothing failed)
    // while it counts up, a screen reader reads its name without the time (it would read the line again every second)
    const name = running ? t('chat.steps.runningName') : null
    if (sum.getAttribute('aria-label') !== name) name ? sum.setAttribute('aria-label', name) : sum.removeAttribute('aria-label')
    // a step that failed, or was denied, shows on the line
    const failed = steps.some(s => s.kind === 'tool' && s.status === 'error'), denied = steps.some(s => s.status === 'denied' || (s.kind === 'ask' && s.status === 'deny'))
    const bad = sum.querySelector('.bad'), said = [failed && t('chat.tool.error'), denied && t('chat.tool.denied')].filter(Boolean).join(' · ')
    bad.hidden = !said
    if (bad.textContent !== said) bad.textContent = said
  }
  // the fold at the end counts up once a second while the window shows (with motion paused, its words change once a
  // minute at most: they say whole minutes)
  setInterval(() => {
    if (el.hidden || !session) return
    const last = lastBlock()
    if (last?.matches('.steps.running') || (last?.matches('.steps') && busy())) labelSteps(last)
  }, 1000)
  // the session's state changed (show): the fold at the end may have finished, or one may wait for its first step
  function stepsFollow() {
    if (!session || el.hidden) return
    ghost()
    const last = lastBlock()
    if (last?.matches('.steps')) labelSteps(last)
  }
  // what the settings say goes for every fold
  onPref(STEPS_OPEN, v => { for (const d of flow.querySelectorAll('.steps > details')) d.open = v === 'open' })
  // closed (or hidden by the theme): the log goes, the pushes go by, and it is fetched again when the window shows
  function forget() {
    session = null
    items.clear()
    flow.innerHTML = ''
  }
  // a chat push: a whole item (new or changed), or text to append to a streaming one
  sym.on('chat', msg => {
    if (msg.session !== session) return
    if (el.hidden) return forget()
    const nearEnd = flow.scrollTop + flow.clientHeight >= flow.scrollHeight - 40
    if (msg.delta) {
      const it = items.get(msg.delta.id)
      if (!it) { load(session); return } // a block we missed: fetch the log again
      it.text = String(it.text ?? '') + msg.delta.text
      if (it.text.length > TEXT_MAX) it.text = it.text.slice(-TEXT_MAX) // the bridge keeps the end too
      const node = nodeOf(it.id), body = node?.querySelector('.body')
      if (body) grow(node, body, it, msg.delta.text)
    } else if (msg.item) {
      const it = msg.item, old = nodeOf(it.id), had = items.get(it.id)
      items.set(it.id, it)
      if (old && had && foldable(had) !== foldable(it)) render() // an ask settled: into the fold it goes, joining the steps around it
      else {
        const node = itemEl(it, old)
        if (old) {
          old.replaceWith(node)
          const fold = node.closest('.steps')
          if (fold) labelSteps(fold)
        }
        else { flow.querySelector(':scope > .empty')?.remove(); place(node, it); trim() }
      }
    }
    if (nearEnd) flow.scrollTop = flow.scrollHeight
  })
  // pushes may have been missed (the stream was lost, or closed while the page was hidden): the log again
  sym.on('resync', () => { if (session) el.hidden ? forget() : load(session) })
  // Streamed text: a short block is drawn again with its markup on every piece; a long one shows as plain text from then
  // on, taking each piece as it comes; a block folded away (a thinking block, the steps) nothing at all until it
  // unfolds. The push that ends the block draws it whole, with its markup.
  function grow(node, body, it, piece) {
    if (node.querySelector('details:not([open])') || node.parentElement.closest('details:not([open])')) body.dataset.stale = '1'
    else if (it.text.length <= LIVE_MARKUP) body.innerHTML = bodyHTML(it)
    else if (!body.classList.contains('raw')) { body.classList.add('raw'); body.textContent = it.text }
    else body.append(piece)
  }
  flow.addEventListener('toggle', ev => { // (toggle doesn't bubble: caught on its way down)
    if (!ev.target.open) return
    // what streamed while it was folded away is drawn now, where it shows (a thinking block of the steps folded on its own
    // still waits for its own unfolding)
    for (const body of ev.target.querySelectorAll('.body[data-stale]')) {
      const it = items.get(body.closest('.it')?.dataset.id)
      if (it && !body.parentElement.closest('details:not([open])')) { delete body.dataset.stale; body.classList.remove('raw'); body.innerHTML = bodyHTML(it) }
    }
  }, true)

  // ---------- the session's terminal, in the window ----------
  // In its place of the flow and the input (the asks' cards still rise over it); every key goes to the session, Esc
  // included, so the way back is the button (or Ctrl+], as symphony attach detaches). The window shows the chat again
  // when it hides, shows another session, or the session's terminal goes (show's terminal).
  const termPane = el.querySelector('.termpane')
  let term = null // the terminal view, made the first time it shows
  let termFor = null // the launch whose terminal the window offers (show's terminal)
  let termOn = false // the window shows the terminal instead of the chat
  let termBtn = null // the button that switches
  function showTerminal(on, focus = true) {
    on = !!on && !!termFor && !el.hidden
    if (on === termOn) return
    termOn = on
    el.querySelector('.panel').classList.toggle('term-on', on)
    termPane.hidden = !on
    if (on) {
      if (!term) { term = createTerminal(sym, { interactive: true, onLeave: () => showTerminal(false) }); termPane.appendChild(term.el) }
      term.open(termFor)
      if (focus) term.focus()
    } else {
      term?.close()
    }
    drawAsks() // the cards over the terminal, or back in their places
    if (!on && focus) chat.focus()
    labelTermBtn()
  }
  function labelTermBtn() {
    if (!termBtn) return
    termBtn.textContent = t(termOn ? 'chat.termBack' : 'chat.termShow')
    termBtn.title = t(termOn ? 'chat.termBackTitle' : 'chat.termShowTitle')
    termBtn.classList.toggle('on', termOn)
  }
  new MutationObserver(() => { if (el.hidden && termOn) showTerminal(false, false) }).observe(el, { attributes: true, attributeFilter: ['hidden'] })

  // the session's pending asks, as show() saw them last
  let pending = [], asking = false
  // an MCP server's form is answered in the session's terminal: its card has a button that shows it here, hidden while
  // the session has none (termButtons, on every show: the terminal may come after the card)
  const cardOpts = { onTerminal: () => showTerminal(true) }
  function termButtons() { for (const b of el.querySelectorAll('.card .to-term')) b.parentElement.hidden = !termFor }
  // Where the cards go. Overlay: all in the dock, which rises over the input. Inline: each in its line of the flow (at
  // the end while its line hasn't come), drawn again after the flow is (render). While the terminal shows, the dock
  // rises over it with the cards the page answers (inline: they move there, and back to their lines with the chat); an
  // MCP server's form, which is answered in the terminal itself, stays out of its way.
  function drawAsks() {
    if (!inline) syncCards(asksEl, pending, askWho, '', cardOpts)
    else {
      const ids = new Set(pending.map(a => a.id))
      for (const [id, card] of cards) if (!ids.has(id)) { card.remove(); cards.delete(id) }
      for (const a of pending) {
        let card = cards.get(a.id)
        if (!card) { card = askCard(a, askWho(a), cardOpts); cards.set(a.id, card) }
        const line = flow.querySelector(`.it.ask[data-ask="${CSS.escape(String(a.id))}"]`)
        if (termOn && a.kind !== 'elicitation') { if (card.parentNode !== asksEl) asksEl.appendChild(card) }
        else if (line) { if (card.parentNode !== line) { line.classList.add('carded'); line.appendChild(card) } }
        else if (card.parentNode !== flow) flow.appendChild(card)
      }
      for (const line of flow.querySelectorAll('.it.ask.carded')) if (!line.querySelector('.card')) line.classList.remove('carded')
    }
    const over = pending.filter(a => !termOn || a.kind !== 'elicitation')
    dock.classList.toggle('asking', over.length > 0 && (!inline || termOn))
    asking = pending.length > 0
    termButtons()
  }

  function itemEl(it, old) {
    const node = document.createElement('div')
    node.dataset.id = it.id
    const sub = it.agentId ? `<span class="sub">${esc(t('common.subagent'))}</span> ` : '' // (a space: read apart from what follows)
    switch (it.kind) {
      case 'user': {
        node.className = `it user ${it.status || ''}`
        // one the bridge couldn't hand the session (it ended first, say): failed, and why
        const why = it.status === 'dropped' ? `<div class="why">${esc(t('chat.dropped') + (it.summary ? t('common.colon') + it.summary : ''))}</div>` : ''
        if (it.origin === 'task') { // the bridge's kickoff of a task: long, so folded to its first line
          node.classList.add('kickoff')
          const open = old?.querySelector('details')?.open ? ' open' : '', peek = String(it.text ?? '').trim().split('\n')[0]
          node.innerHTML = `<details${open}><summary class="from"><b>${esc(t('chat.fromTask'))}</b><span class="peek">${esc(peek)}</span></summary>` +
            `<div class="${bodyClass(it)}">${bodyHTML(it)}</div></details>${why}${timeOf(it)}`
          break
        }
        // from the page
        node.innerHTML = `${it.origin === 'page' ? `<div class="from">${esc(t('task.you'))}</div>` : ''}<div class="body">${richText(it.text)}</div>${why}${timeOf(it)}`
        break
      }
      case 'thinking': {
        node.className = `it thinking${it.done ? ' done' : ''}`
        const open = old?.querySelector('details')?.open ? ' open' : '' // stays unfolded while it streams
        // without words (a model whose thinking is summarized) there is nothing to unfold: just that it thought, and how
        // long (it's a step of a fold, whose line says "Thought for …": this one says "Thinking · 12s")
        const label = !it.done ? t('chat.thinking') : t('chat.thinking') + ' · ' + duration((it.secs || 0) * 1000)
        node.innerHTML = it.text || !it.done
          ? `<details${open}><summary>${esc(label)}<span class="dots"></span></summary><div class="${bodyClass(it)}">${bodyHTML(it)}</div></details>`
          : `<div class="bare">${esc(label)}</div>`
        break
      }
      case 'text':
        node.className = `it text${it.done ? ' done' : ''}`
        node.innerHTML = `<div class="${bodyClass(it)}">${bodyHTML(it)}</div>${it.done ? timeOf(it) : ''}`
        break
      case 'tool': {
        node.className = `it tool ${it.status || ''}${it.done ? ' done' : ''}`
        // the outcome, and how long it took once it is done (the mod clocks the call); a failure's reason is in the tooltip
        const took = it.done && it.ms >= 1000 ? ` ${(it.ms / 1000).toFixed(it.ms < 10000 ? 1 : 0)}s` : ''
        node.innerHTML = `${sub}<b>${esc(it.tool)}</b> ${esc(it.summary || '')}` +
          (it.done ? `<span class="res">${esc(t('chat.tool.' + (it.status || 'ok')) + took)}</span>` : '')
        node.title = (it.summary || '') + (it.text ? '\n' + it.text : '')
        break
      }
      case 'command': // a slash command that ran, as a tool row
        node.className = 'it tool done'
        node.innerHTML = `<b>${esc(it.text)}</b> ${esc(it.summary || '')}`
        break
      case 'message': {
        const out = !!name && it.from === name // a task session sends as its member's name
        node.className = `it message${out ? ' out' : ''}`
        node.innerHTML = `<div class="from">${esc(out ? t('chat.msgTo', { name: it.to || '?' }) : t('chat.msgFrom', { name: it.from || '?' }))}</div><div class="${bodyClass(it)}">${bodyHTML(it)}</div>${timeOf(it)}`
        break
      }
      case 'ask': {
        node.className = `it ask ${it.status || ''}`
        node.dataset.ask = it.ask
        node.innerHTML = `${sub}${esc(askKind(it.askKind))}${esc(t('common.colon'))}` +
          `${esc(it.tool ? it.tool + ' ' : '')}${esc(it.summary || '')} · ${esc(t('chat.ask.' + (it.status || 'pending')))}${it.text ? esc(' · ' + it.text) : ''}`
        node.title = (it.summary || '') + (it.text ? '\n' + it.text : '')
        // (inline) a pending ask's card, under its line
        const card = inline && (it.status || 'pending') === 'pending' ? cards.get(it.ask) : null
        if (card) { node.classList.add('carded'); node.appendChild(card) }
        break
      }
      default: // a notice: the code translated, then the detail the bridge adds (an error's text, the models of a switch)
        node.className = 'it notice'
        node.textContent = t('chat.notice.' + it.text) + (it.summary ? `${t('common.colon')}${it.summary}` : '')
    }
    return node
  }

  // The focus a redraw took out of the window (the button it was on went: Stop once the turn ended; the input turned
  // off as the session went off duty) goes back to the same button drawn anew, else to the window's next control, else
  // its name: never to the page under the window, where a screen reader would land
  function keepFocus(had, hadKey) {
    if (!had || el.hidden || el.contains(document.activeElement)) return
    const usable = x => !!x && !x.disabled && x.getClientRects().length > 0
    const again = hadKey && [...head.querySelectorAll('.btns button')].find(b => (b.dataset.key || b.textContent) === hadKey)
    const next = [again, sayBox, ...head.querySelectorAll('.btns button'), head.querySelector('.close')].find(usable)
    ;(next ?? head.querySelector('h2'))?.focus({ preventScroll: true })
  }

  const chat = {
    el, dock, sayBox,
    get hidden() { return el.hidden },
    get session() { return session },
    get asking() { return asking },
    // the window for a session: the head as the theme describes it, the flow of that session, the dock
    show(p) {
      // what has the focus in the window: put back after the redraw if the redraw took it away (keepFocus)
      const had = el.contains(document.activeElement) ? document.activeElement : null
      const hadKey = had?.closest('.btns') ? had.dataset.key || had.textContent : null
      name = p.name
      head.querySelector('h2').textContent = p.name
      if ((p.avatar || '') !== shown.avatar) {
        const av = head.querySelector('.who .av')
        shown.avatar = p.avatar || ''
        av.style.backgroundImage = shown.avatar
        av.hidden = !shown.avatar
      }
      const said = t('chat.flow', { name: p.name }) // the conversation's name: the keyboard reaches it (it scrolls), and a screen reader would read it whole
      if (flow.getAttribute('aria-label') !== said) flow.setAttribute('aria-label', said)
      head.querySelector('.dot').style.background = STATE_MARK[p.state] ?? GRAY_MARK
      chat.setLine(p.line)
      const meta = (p.taskChip ? `<span class="task" data-task="${esc(p.taskChip.id)}" title="${esc(p.taskChip.title)}">${esc(p.taskChip.label)}</span>` : '') +
        (p.chips || []).filter(c => c[1] !== undefined && c[1] !== '' && c[1] !== null)
          .map(([k, v, tip]) => `<span title="${esc(tip || v)}"><b>${esc(t('card.' + k))}</b>${esc(v)}</span>`).join('')
      if (meta !== shown.meta) { shown.meta = meta; head.querySelector('.meta').innerHTML = meta }
      // the terminal: offered while the theme names the launch; another launch or session brings the chat back
      if ((p.terminal || null) !== termFor || (p.session || null) !== session) {
        termFor = p.terminal || null
        showTerminal(false, false)
      }
      // the buttons are rebuilt when their set changes, or whatever the theme says they depend on (buttonsKey)
      const btnKey = (p.buttons || []).map(b => b.key ?? b.text).join('|') + '|' + p.session + '|' + (p.buttonsKey ?? '') + '|' + (termFor ? 'terminal' : '')
      if (btnKey !== shown.btns) {
        shown.btns = btnKey
        const btns = head.querySelector('.btns')
        btns.innerHTML = ''
        termBtn = termFor ? button('', 'termview', () => showTerminal(!termOn)) : null
        for (const b of p.buttons || []) {
          const bt = btns.appendChild(button(b.text, b.cls, b.onclick, b.title))
          if (b.key) bt.dataset.key = b.key
          if (b.key === 'terminal' && termBtn) btns.appendChild(termBtn) // beside the one that opens a window
        }
        if (termBtn && !termBtn.parentNode) btns.prepend(termBtn)
        labelTermBtn()
      }
      // the chat follows the session the theme names (an employee off duty keeps the last one's log while the bridge has it)
      const sid = p.session || null
      if (sid !== session) {
        session = sid
        items.clear()
        flow.innerHTML = ''
        for (const c of cards.values()) c.remove() // (inline) another session's cards
        cards.clear()
        pending = []
        shown.asks = undefined
        if (sid) load(sid)
        else flow.innerHTML = `<div class="empty">${esc(t('chat.noSession'))}</div>`
      }
      const mine = sym.state.asks.filter(a => a.session === sid)
      const askKey = mine.map(a => a.id).join(',')
      if (askKey !== shown.asks) {
        shown.asks = askKey
        pending = mine
        drawAsks()
      } else termButtons()
      sayBox.disabled = !p.online
      sayForm.hidden = !p.online
      const off = dock.querySelector('.offline')
      off.hidden = p.online
      if (off.textContent !== p.offText) off.textContent = p.offText
      el.hidden = false
      stepsFollow()
      keepFocus(had, hadKey)
    },
    setLine(text) {
      const line = head.querySelector('.who .state')
      if (line.textContent !== text) line.textContent = text
    },
    // the input, or the first card's control while asks wait (the terminal while it shows)
    focus() {
      if (el.hidden) return
      if (termOn) term.focus()
      else if (dock.classList.contains('asking')) dock.querySelector('#asks input, #asks button')?.focus()
      else if (inline && asking) flow.querySelector('.card input, .card button')?.focus()
      else if (!sayBox.disabled) sayBox.focus()
    },
    close() {
      forget()
      if (el.hidden) return
      el.hidden = true
      onClose()
    },
    showTerminal(on = true) { showTerminal(on) },
    get terminalShown() { return termOn },
  }
  return chat
}
