// Cards for pending asks (permission requests / AskUserQuestion / an MCP server's form), shared by the office popup and
// the approval page. A card submits its own answer to the bridge; the page styles .card and friends.
//
// A permission the mod could keep for the rest of the session (a.remember, the rule it proposes) gets a third button,
// "Allow for this session", which answers { decision: 'allow', remember: true }. An MCP server's form (kind
// elicitation) can't be answered here: its card shows what the server asks and says to answer it in the session's
// terminal, with a button that opens it when the page offers one (opts.onTerminal); the terminal's answer takes it away.
//
// A card's buttons are off while its answer is on its way (back on if it failed, which onAskFailure's handler says). A question
// can't be answered with nothing: every question needs a pick or words of its own. A permission whose input the bridge
// cut (inputTruncated) says so as a warning and can't be allowed before the whole input was shown ("Show all"); a
// bridge that can't show it (an older one) says that, and Allow is on again. Deny always works.

import { t, errorText } from '/i18n.js'
import { request } from '/symphony.js'

export const esc = s => String(s ?? '').replace(/[&<>"]/g, c => ({ '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;' }[c]))

// how a failure is shown: alert() where nothing else is set (the approval page); the themes show it over the page and
// write it to the error log (ui/errors.js)
let showFailure = text => alert(text)
export function onAskFailure(fn) { showFailure = fn }

// the answer, with the card's buttons off meanwhile; says whether the bridge took it
async function answer(el, id, body) {
  const buttons = [...el.querySelectorAll('button')], was = buttons.map(b => b.disabled)
  buttons.forEach(b => { b.disabled = true })
  const r = await request('POST', `/api/answer/${id}`, body)
  if (r.ok) return true // the card goes with the next snapshot
  buttons.forEach((b, i) => { b.disabled = was[i] })
  showFailure(t('asks.submitFailed', { reason: errorText(r.data) }))
  return false
}

// a permission's main argument, shown first: a command (as a command line), else a file, a pattern or an address
function mainOf(input) {
  if (!input || typeof input !== 'object') return null
  if (typeof input.command === 'string' && input.command) return { cls: 'cmd', text: input.command }
  for (const k of ['file_path', 'notebook_path', 'pattern', 'url', 'path']) if (typeof input[k] === 'string' && input[k]) return { cls: 'path', text: input[k] }
  return null
}

// the words of a rule on its button: every edit, the commands that start with a prefix, or a tool
export function ruleText(r) {
  if (!r) return ''
  if (r.tool === 'Edit') return t('asks.allowSessionEdits')
  if (r.tool === 'Bash' && r.prefix) return t('asks.allowSessionCmd', { prefix: r.prefix })
  return t('asks.allowSessionTool', { tool: r.tool })
}

// who: the HTML of the card's top line (who, and what kind of ask); the page decides how it reads. For screen readers
// a card is a group named by that line, and each question a group named by itself. opts.onTerminal: (an MCP server's
// form) the page can show the session's terminal, where it is answered.
export function askCard(a, who, opts = {}) {
  const el = document.createElement('div')
  el.className = 'card'
  el.dataset.id = a.id
  el.dataset.session = a.session
  el.dataset.kind = a.kind
  el.setAttribute('role', 'group')
  const id = 'ask-' + String(a.id).replace(/[^\w-]/g, '_')
  if (a.kind === 'question') {
    el.innerHTML = who + (a.questions || []).map((q, i) => `
      <div class="q" data-i="${i}" role="group" aria-labelledby="${id}-q${i}">
        <h3 id="${id}-q${i}"><span class="hd">${esc(q.header)}</span>${esc(q.question)}</h3>
        ${(q.options || []).map(o => `
          <label class="opt"><input type="${q.multiSelect ? 'checkbox' : 'radio'}" name="${a.id}-q${i}" value="${esc(o.label)}">
          ${esc(o.label)}<small>${esc(o.description)}</small></label>`).join('')}
        <input type="text" class="other" placeholder="${esc(t('asks.other'))}" aria-label="${esc(t('asks.other'))}">
        <div class="need" hidden>${esc(t('asks.answerRequired'))}</div>
      </div>`).join('') +
      `<div class="row"><button class="submit">${esc(t('asks.submit'))}</button></div>`
    // a question answered (a pick, or words of its own) lets go of its mark
    el.addEventListener('input', ev => { const q = ev.target.closest('.q'); if (q) { q.classList.remove('missing'); q.querySelector('.need').hidden = true } })
    el.querySelector('.submit').onclick = () => {
      const answers = {}
      let missing = null
      ;(a.questions || []).forEach((q, i) => {
        const box = el.querySelector(`.q[data-i="${i}"]`)
        const other = box.querySelector('.other').value.trim()
        const picked = [...box.querySelectorAll('input:checked')].map(x => x.value)
        answers[q.question] = other || picked.join(', ')
        const none = !answers[q.question]
        box.classList.toggle('missing', none)
        box.querySelector('.need').hidden = !none
        if (none && !missing) missing = box
      })
      if (missing) return (missing.querySelector('input:not([type=text])') ?? missing.querySelector('.other')).focus()
      answer(el, a.id, { answers })
    }
  } else if (a.kind === 'elicitation') {
    const fields = (a.fields || []).map(f => `<li><b>${esc(f.title || f.name)}</b>${f.required ? ` <span class="req">${esc(t('asks.elicitRequired'))}</span>` : ''}` +
      `${f.type ? ` <small>${esc(f.enum?.length ? f.enum.join(' / ') : f.type)}</small>` : ''}${f.description ? `<div class="meta">${esc(f.description)}</div>` : ''}</li>`).join('')
    el.innerHTML = `${who}
      <div class="tool">${esc(t('asks.elicitFrom', { server: a.server || '?' }))}</div>
      <div class="msg">${esc(a.message)}</div>
      ${fields ? `<ul class="fields">${fields}</ul>` : ''}
      ${a.mode === 'url' && /^https?:\/\//i.test(a.url || '') ? `<div class="meta"><a href="${esc(a.url)}" target="_blank" rel="noopener noreferrer">${esc(a.url)}</a></div>` : ''}
      <div class="meta">${esc(t('asks.elicitTerminal'))}</div>
      ${opts.onTerminal ? `<div class="row"><button class="to-term">${esc(t('asks.elicitOpen'))}</button></div>` : ''}`
    el.querySelector('.to-term')?.addEventListener('click', () => opts.onTerminal(a))
  } else {
    // the main argument first, in plain sight (a command as a command line, a file by its path), the description as
    // a sentence, the whole input folded under them (open when there is nothing else to show); the reason folded too
    const main = mainOf(a.input)
    el.innerHTML = `${who}
      <div class="tool">${esc(a.tool)}</div>
      ${a.reason ? `<div class="meta">${esc(a.reason)}</div>` : ''}
      ${main ? `<div class="main ${main.cls}">${esc(main.text)}</div>` : ''}
      ${a.input?.description && main ? `<div class="meta desc">${esc(a.input.description)}</div>` : ''}
      <details class="all"${main ? '' : ' open'}><summary>${esc(t('asks.allInput'))}</summary><pre>${esc(JSON.stringify(a.input, null, 2))}</pre></details>
      ${a.inputTruncated ? `<div class="cut"><span class="warn">${esc(t('asks.truncatedWarn'))}</span><button class="show-all">${esc(t('asks.showAll'))}</button></div>` : ''}
      <details class="why"><summary>${esc(t('asks.addReason'))}</summary><input type="text" class="reason" placeholder="${esc(t('asks.reason'))}" aria-label="${esc(t('asks.reason'))}"></details>
      <div class="row">
        <button class="allow">${esc(t('asks.allow'))}</button>${a.remember ? `<button class="allow-session" title="${esc(t('asks.allowSessionTitle'))}">${esc(ruleText(a.remember))}</button>` : ''}<button class="deny">${esc(t('asks.deny'))}</button>
      </div>`
    const reason = el.querySelector('.reason'), allow = el.querySelector('.allow'), session = el.querySelector('.allow-session')
    allow.onclick = () => answer(el, a.id, { decision:'allow', reason:reason.value })
    if (session) session.onclick = () => answer(el, a.id, { decision:'allow', remember:true, reason:reason.value })
    el.querySelector('.deny').onclick = () => answer(el, a.id, { decision:'deny', reason:reason.value })
    if (a.inputTruncated) {
      // part of the input isn't shown: not to be allowed unseen
      allow.disabled = true
      allow.title = t('asks.allowAfterAll')
      if (session) session.disabled = true
      const cut = el.querySelector('.cut'), more = cut.querySelector('.show-all')
      more.onclick = async () => {
        more.disabled = true
        const r = await request('GET', `/api/asks/${encodeURIComponent(a.id)}/input`)
        if (r.ok && r.data && 'input' in r.data) {
          el.querySelector('details.all pre').textContent = JSON.stringify(r.data.input, null, 2)
          const whole = mainOf(r.data.input), shown = el.querySelector('.main')
          if (whole && shown) shown.textContent = whole.text
          cut.remove()
        } else if (r.status === 404 && !r.data?.code) {
          // a bridge without the call (its plain 404, no error code): what was cut can't be seen here; the warning
          // stays, and Allow is up to you
          cut.querySelector('.warn').textContent = t('asks.truncatedNoFull')
          more.remove()
        } else {
          more.disabled = false
          return showFailure(t('asks.showAllFailed', { reason: errorText(r.data) }))
        }
        allow.disabled = false
        allow.title = ''
        if (session) session.disabled = false
      }
    }
  }
  const top = el.querySelector('.who')
  if (top) { top.id = id + '-who'; el.setAttribute('aria-labelledby', top.id) }
  return el
}

// renders a list of cards incrementally: existing cards are left alone, so nothing interrupts typing. opts go to every
// card (askCard: onTerminal).
export function syncCards(container, asks, who, emptyHTML, opts = {}) {
  const ids = new Set(asks.map(a => a.id))
  for (const el of [...container.querySelectorAll('.card')]) if (!ids.has(el.dataset.id)) el.remove()
  for (const a of asks) {
    if (!container.querySelector(`.card[data-id="${a.id}"]`)) container.appendChild(askCard(a, who(a), opts))
  }
  container.querySelector('.empty')?.remove()
  if (!asks.length && emptyHTML) container.insertAdjacentHTML('beforeend', emptyHTML)
}
