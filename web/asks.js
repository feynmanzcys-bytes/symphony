// Cards for pending asks (permission requests / AskUserQuestion), shared by the office popup and the
// approval page. A card submits its own answer to the bridge; the page styles .card and friends.
//
// A card's buttons are off while its answer is on its way (back on if it failed, which an alert says). A question
// can't be answered with nothing: every question needs a pick or words of its own. A permission whose input the bridge
// cut (inputTruncated) says so as a warning and can't be allowed before the whole input was shown ("Show all"); a
// bridge that can't show it (an older one) says that, and Allow is on again. Deny always works.

import { t, errorText } from '/i18n.js'
import { request } from '/symphony.js'

export const esc = s => String(s ?? '').replace(/[&<>"]/g, c => ({ '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;' }[c]))

// the answer, with the card's buttons off meanwhile; says whether the bridge took it
async function answer(el, id, body) {
  const buttons = [...el.querySelectorAll('button')], was = buttons.map(b => b.disabled)
  buttons.forEach(b => { b.disabled = true })
  const r = await request('POST', `/api/answer/${id}`, body)
  if (r.ok) return true // the card goes with the next snapshot
  buttons.forEach((b, i) => { b.disabled = was[i] })
  alert(t('asks.submitFailed', { reason: errorText(r.data) }))
  return false
}

// who: the HTML of the card's top line (who, and what kind of ask); the page decides how it reads. For screen readers
// a card is a group named by that line, and each question a group named by itself.
export function askCard(a, who) {
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
  } else {
    el.innerHTML = `${who}
      <div class="tool">${esc(a.tool)}</div>
      ${a.reason ? `<div class="meta">${esc(a.reason)}</div>` : ''}
      <pre>${esc(JSON.stringify(a.input, null, 2))}</pre>
      ${a.inputTruncated ? `<div class="cut"><span class="warn">${esc(t('asks.truncatedWarn'))}</span><button class="show-all">${esc(t('asks.showAll'))}</button></div>` : ''}
      <div class="row">
        <input type="text" class="reason" placeholder="${esc(t('asks.reason'))}" aria-label="${esc(t('asks.reason'))}">
        <button class="allow">${esc(t('asks.allow'))}</button><button class="deny">${esc(t('asks.deny'))}</button>
      </div>`
    const reason = el.querySelector('.reason'), allow = el.querySelector('.allow')
    allow.onclick = () => answer(el, a.id, { decision:'allow', reason:reason.value })
    el.querySelector('.deny').onclick = () => answer(el, a.id, { decision:'deny', reason:reason.value })
    if (a.inputTruncated) {
      // part of the input isn't shown: not to be allowed unseen
      allow.disabled = true
      allow.title = t('asks.allowAfterAll')
      const cut = el.querySelector('.cut'), more = cut.querySelector('.show-all')
      more.onclick = async () => {
        more.disabled = true
        const r = await request('GET', `/api/asks/${encodeURIComponent(a.id)}/input`)
        if (r.ok && r.data && 'input' in r.data) {
          el.querySelector('pre').textContent = JSON.stringify(r.data.input, null, 2)
          cut.remove()
        } else if (r.status === 404 && !r.data?.code) {
          // a bridge without the call (its plain 404, no error code): what was cut can't be seen here; the warning
          // stays, and Allow is up to you
          cut.querySelector('.warn').textContent = t('asks.truncatedNoFull')
          more.remove()
        } else {
          more.disabled = false
          return alert(t('asks.showAllFailed', { reason: errorText(r.data) }))
        }
        allow.disabled = false
        allow.title = ''
      }
    }
  }
  const top = el.querySelector('.who')
  if (top) { top.id = id + '-who'; el.setAttribute('aria-labelledby', top.id) }
  return el
}

// renders a list of cards incrementally: existing cards are left alone, so nothing interrupts typing
export function syncCards(container, asks, who, emptyHTML) {
  const ids = new Set(asks.map(a => a.id))
  for (const el of [...container.querySelectorAll('.card')]) if (!ids.has(el.dataset.id)) el.remove()
  for (const a of asks) {
    if (!container.querySelector(`.card[data-id="${a.id}"]`)) container.appendChild(askCard(a, who(a)))
  }
  container.querySelector('.empty')?.remove()
  if (!asks.length && emptyHTML) container.insertAdjacentHTML('beforeend', emptyHTML)
}
