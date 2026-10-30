// Cards for pending asks (permission requests / AskUserQuestion), shared by the office popup and the
// approval page. A card submits its own answer to the bridge; the page styles .card and friends.

import { t, errorText } from '/i18n.js'

export const esc = s => String(s ?? '').replace(/[&<>"]/g, c => ({ '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;' }[c]))

async function answer(id, body) {
  const r = await fetch(`/api/answer/${id}`, { method:'POST', headers:{ 'content-type':'application/json' }, body:JSON.stringify(body) })
  if (!r.ok) alert(t('asks.submitFailed', { reason: errorText(await r.json().catch(() => null)) }))
}

// who: the HTML of the card's top line (who, and what kind of ask); the page decides how it reads
export function askCard(a, who) {
  const el = document.createElement('div')
  el.className = 'card'
  el.dataset.id = a.id
  el.dataset.session = a.session
  el.dataset.kind = a.kind
  if (a.kind === 'question') {
    el.innerHTML = who + (a.questions || []).map((q, i) => `
      <div class="q" data-i="${i}">
        <h3><span class="hd">${esc(q.header)}</span>${esc(q.question)}</h3>
        ${(q.options || []).map(o => `
          <label class="opt"><input type="${q.multiSelect ? 'checkbox' : 'radio'}" name="${a.id}-q${i}" value="${esc(o.label)}">
          ${esc(o.label)}<small>${esc(o.description)}</small></label>`).join('')}
        <input type="text" class="other" placeholder="${esc(t('asks.other'))}">
      </div>`).join('') +
      `<div class="row"><button class="submit">${esc(t('asks.submit'))}</button></div>`
    el.querySelector('.submit').onclick = () => {
      const answers = {}
      ;(a.questions || []).forEach((q, i) => {
        const box = el.querySelector(`.q[data-i="${i}"]`)
        const other = box.querySelector('.other').value.trim()
        const picked = [...box.querySelectorAll('input:checked')].map(x => x.value)
        answers[q.question] = other || picked.join(', ')
      })
      answer(a.id, { answers })
    }
  } else {
    el.innerHTML = `${who}
      <div class="tool">${esc(a.tool)}</div>
      ${a.reason ? `<div class="meta">${esc(a.reason)}</div>` : ''}
      <pre>${esc(JSON.stringify(a.input, null, 2))}</pre>
      <div class="row">
        <input type="text" class="reason" placeholder="${esc(t('asks.reason'))}">
        <button class="allow">${esc(t('asks.allow'))}</button><button class="deny">${esc(t('asks.deny'))}</button>
      </div>`
    const reason = el.querySelector('.reason')
    el.querySelector('.allow').onclick = () => answer(a.id, { decision:'allow', reason:reason.value })
    el.querySelector('.deny').onclick = () => answer(a.id, { decision:'deny', reason:reason.value })
  }
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
