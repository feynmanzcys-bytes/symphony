// The employee window (#chat), shared by every theme: who they are at the top (the name, the state dot and line, chips,
// buttons: the theme says what), below it the session as a chat (the prompts, the model's thinking folded and its
// answers as they stream, the tool calls, the messages between sessions, the asks and how they were settled), and the
// dock at the bottom: the input, over which the pending asks' cards rise until they're handled. The log comes from the
// bridge when the window shows a session; each change comes as a chat push.
//
//   const chat = createChat(sym, { askWho, onClose, onTaskChip })
//   chat.show({ name, state, line, chips, taskChip, buttons, session, online, offText })   draw it for a session (again on every change)
//   chat.setLine(text)      the status line alone (it follows what the employee does, every frame)
//   chat.close()            hide it (onClose is told)
//   chat.el                 the modal, for a theme that docks it beside another window
import { t, esc, errorText, richText, timeOf, STATE_COLOR, GRAY, button } from '/ui/common.js'
import { syncCards } from '/asks.js'

const MARKUP = `
<div class="modal" id="chat" hidden>
  <div class="panel chat">
    <div class="head">
      <span class="close">✕</span>
      <div class="who"><span class="dot"></span><h2></h2><span class="state"></span></div>
      <div class="meta"></div>
      <div class="btns"></div>
    </div>
    <div class="flow"></div>
    <div class="dock">
      <div id="asks"></div>
      <form class="say" autocomplete="off">
        <textarea rows="2"></textarea>
        <button type="submit" class="px primary"></button>
      </form>
      <div class="offline" hidden></div>
    </div>
  </div>
</div>`

export function createChat(sym, { askWho, onClose = () => {}, onTaskChip = () => {} } = {}) {
  const tpl = document.createElement('template')
  tpl.innerHTML = MARKUP.trim()
  const el = tpl.content.firstElementChild
  document.body.appendChild(el)
  const head = el.querySelector('.head'), flow = el.querySelector('.flow'), dock = el.querySelector('.dock')
  const sayForm = el.querySelector('.say'), sayBox = sayForm.querySelector('textarea'), asksEl = el.querySelector('#asks')
  sayBox.placeholder = t('chat.placeholder')
  sayForm.querySelector('button').textContent = t('chat.send')

  let session = null // the session whose log the flow shows
  let name = '' // who the window is for: the account's name, a member's name (bob-T7) for a task session
  const items = new Map() // item id -> item, of session
  const shown = { meta: '', btns: '', asks: '' }

  el.querySelector('.close').onclick = () => chat.close()
  el.addEventListener('click', ev => { if (ev.target === el) chat.close() })
  head.querySelector('.meta').addEventListener('click', ev => { const c = ev.target.closest('[data-task]'); if (c) onTaskChip(c.dataset.task) })

  // sending: Enter sends, Shift+Enter makes a new line; the bridge queues it for the session's mod, and the chat shows
  // it as queued until the mod has submitted it
  sayForm.addEventListener('submit', async ev => {
    ev.preventDefault()
    const text = sayBox.value.trim()
    if (!text || !session) return
    sayBox.value = ''
    const r = await sym.say(session, text)
    if (!r.ok) { alert(errorText(r.data)); sayBox.value = text }
    sayBox.focus()
  })
  sayBox.addEventListener('keydown', ev => {
    if (ev.key === 'Enter' && !ev.shiftKey && !ev.isComposing) { ev.preventDefault(); sayForm.requestSubmit() }
  })

  async function load(sid) {
    const list = await sym.fetchChat(sid)
    if (sid !== session) return
    for (const it of list) items.set(it.id, it)
    render()
  }
  function render() {
    flow.innerHTML = ''
    const list = [...items.values()].sort((a, b) => a.seq - b.seq)
    if (!list.length) flow.innerHTML = `<div class="empty">${esc(t('chat.empty'))}</div>`
    for (const it of list) flow.appendChild(itemEl(it))
    flow.scrollTop = flow.scrollHeight
  }
  // a chat push: a whole item (new or changed), or text to append to a streaming one
  sym.on('chat', msg => {
    if (msg.session !== session) return
    const nearEnd = flow.scrollTop + flow.clientHeight >= flow.scrollHeight - 40
    if (msg.delta) {
      const it = items.get(msg.delta.id)
      if (!it) { load(session); return } // a block we missed: fetch the log again
      it.text += msg.delta.text
      const body = flow.querySelector(`.it[data-id="${it.id}"] .body`)
      if (body) body.innerHTML = richText(it.text)
    } else if (msg.item) {
      const it = msg.item, old = flow.querySelector(`.it[data-id="${it.id}"]`)
      items.set(it.id, it)
      const node = itemEl(it, old)
      if (old) old.replaceWith(node)
      else { flow.querySelector('.empty')?.remove(); flow.appendChild(node) }
    }
    if (nearEnd) flow.scrollTop = flow.scrollHeight
  })

  function itemEl(it, old) {
    const node = document.createElement('div')
    node.dataset.id = it.id
    const sub = it.agentId ? `<span class="sub">${esc(t('common.subagent'))}</span>` : ''
    switch (it.kind) {
      case 'user':
        node.className = `it user ${it.status || ''}`
        if (it.origin === 'task') { // the bridge's kickoff of a task: long, so folded to its first line
          node.classList.add('kickoff')
          const open = old?.querySelector('details')?.open ? ' open' : '', peek = String(it.text ?? '').trim().split('\n')[0]
          node.innerHTML = `<details${open}><summary class="from"><b>${esc(t('chat.fromTask'))}</b><span class="peek">${esc(peek)}</span></summary>` +
            `<div class="body">${richText(it.text)}</div></details>${timeOf(it)}`
          break
        }
        // from the page
        node.innerHTML = `${it.origin === 'page' ? `<div class="from">${esc(t('task.you'))}</div>` : ''}<div class="body">${richText(it.text)}</div>${timeOf(it)}`
        break
      case 'thinking': {
        node.className = `it thinking${it.done ? ' done' : ''}`
        const open = old?.querySelector('details')?.open ? ' open' : '' // stays unfolded while it streams
        // without words (a model whose thinking is summarized) there is nothing to unfold: just that it thought, and how long
        const label = !it.done ? t('chat.thinking') : it.text ? t('chat.thoughtRead', { s: it.secs || 0 }) : t('chat.thoughtFor', { s: it.secs || 0 })
        node.innerHTML = it.text || !it.done
          ? `<details${open}><summary>${esc(label)}<span class="dots"></span></summary><div class="body">${richText(it.text)}</div></details>`
          : `<div class="bare">${esc(label)}</div>`
        break
      }
      case 'text':
        node.className = `it text${it.done ? ' done' : ''}`
        node.innerHTML = `<div class="body">${richText(it.text)}</div>${it.done ? timeOf(it) : ''}`
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
        node.innerHTML = `<div class="from">${esc(out ? t('chat.msgTo', { name: it.to || '?' }) : t('chat.msgFrom', { name: it.from || '?' }))}</div><div class="body">${richText(it.text)}</div>${timeOf(it)}`
        break
      }
      case 'ask':
        node.className = `it ask ${it.status || ''}`
        node.innerHTML = `${sub}${esc(t(it.askKind === 'question' ? 'asks.question' : 'asks.permission'))}${esc(t('common.colon'))}` +
          `${esc(it.tool ? it.tool + ' ' : '')}${esc(it.summary || '')} · ${esc(t('chat.ask.' + (it.status || 'pending')))}${it.text ? esc(' · ' + it.text) : ''}`
        node.title = (it.summary || '') + (it.text ? '\n' + it.text : '')
        break
      default: // a notice: the code translated, then the detail the bridge adds (an error's text, the models of a switch)
        node.className = 'it notice'
        node.textContent = t('chat.notice.' + it.text) + (it.summary ? `${t('common.colon')}${it.summary}` : '')
    }
    return node
  }

  const chat = {
    el, dock, sayBox,
    get hidden() { return el.hidden },
    get session() { return session },
    get asking() { return dock.classList.contains('asking') },
    // the window for a session: the head as the theme describes it, the flow of that session, the dock
    show(p) {
      name = p.name
      head.querySelector('h2').textContent = p.name
      head.querySelector('.dot').style.background = STATE_COLOR[p.state] ?? GRAY
      chat.setLine(p.line)
      const meta = (p.taskChip ? `<span class="task" data-task="${esc(p.taskChip.id)}" title="${esc(p.taskChip.title)}">${esc(p.taskChip.label)}</span>` : '') +
        (p.chips || []).filter(c => c[1] !== undefined && c[1] !== '' && c[1] !== null)
          .map(([k, v]) => `<span title="${esc(v)}"><b>${esc(t('card.' + k))}</b>${esc(v)}</span>`).join('')
      if (meta !== shown.meta) { shown.meta = meta; head.querySelector('.meta').innerHTML = meta }
      // the buttons are rebuilt when their set changes, or whatever the theme says they depend on (buttonsKey)
      const btnKey = (p.buttons || []).map(b => b.key ?? b.text).join('|') + '|' + p.session + '|' + (p.buttonsKey ?? '')
      if (btnKey !== shown.btns) {
        shown.btns = btnKey
        const btns = head.querySelector('.btns')
        btns.innerHTML = ''
        for (const b of p.buttons || []) btns.appendChild(button(b.text, b.cls, b.onclick, b.title))
      }
      // the chat follows the session the theme names (an employee off duty keeps the last one's log while the bridge has it)
      const sid = p.session || null
      if (sid !== session) {
        session = sid
        items.clear()
        flow.innerHTML = ''
        if (sid) load(sid)
        else flow.innerHTML = `<div class="empty">${esc(t('chat.noSession'))}</div>`
      }
      const mine = sym.state.asks.filter(a => a.session === sid)
      const askKey = mine.map(a => a.id).join(',')
      if (askKey !== shown.asks) {
        shown.asks = askKey
        syncCards(asksEl, mine, askWho)
        dock.classList.toggle('asking', mine.length > 0)
      }
      sayBox.disabled = !p.online
      sayForm.hidden = !p.online
      const off = dock.querySelector('.offline')
      off.hidden = p.online
      if (off.textContent !== p.offText) off.textContent = p.offText
      el.hidden = false
    },
    setLine(text) {
      const line = head.querySelector('.who .state')
      if (line.textContent !== text) line.textContent = text
    },
    // the input, or the first card's control while asks wait
    focus() {
      if (el.hidden) return
      if (dock.classList.contains('asking')) dock.querySelector('#asks input, #asks button')?.focus()
      else if (!sayBox.disabled) sayBox.focus()
    },
    close() {
      if (el.hidden) return
      el.hidden = true
      onClose()
    },
  }
  return chat
}
