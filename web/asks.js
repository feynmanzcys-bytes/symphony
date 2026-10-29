// 待处理事项（权限审批 / AskUserQuestion）的卡片：办公室弹窗和审批台共用。
// 卡片自己把回答提交给 bridge；样式由页面给 .card 等类名定。

export const esc = s => String(s ?? '').replace(/[&<>"]/g, c => ({ '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;' }[c]))

async function answer(id, body) {
  const r = await fetch(`/api/answer/${id}`, { method:'POST', headers:{ 'content-type':'application/json' }, body:JSON.stringify(body) })
  if (!r.ok) alert('提交失败：' + (await r.text()))
}

// who：卡片顶上一行的 HTML（谁、什么事），由页面决定怎么写
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
        <input type="text" class="other" placeholder="其它（自己写，填了就以这个为准）">
      </div>`).join('') +
      `<div class="row"><button class="submit">提交回答</button></div>`
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
        <input type="text" class="reason" placeholder="理由（可选，会回给模型）">
        <button class="allow">允许</button><button class="deny">拒绝</button>
      </div>`
    const reason = el.querySelector('.reason')
    el.querySelector('.allow').onclick = () => answer(a.id, { decision:'allow', reason:reason.value })
    el.querySelector('.deny').onclick = () => answer(a.id, { decision:'deny', reason:reason.value })
  }
  return el
}

// 一份卡片列表的增量渲染：已有的卡片不重绘，免得打断正在填的内容
export function syncCards(container, asks, who, emptyHTML) {
  const ids = new Set(asks.map(a => a.id))
  for (const el of [...container.querySelectorAll('.card')]) if (!ids.has(el.dataset.id)) el.remove()
  for (const a of asks) {
    if (!container.querySelector(`.card[data-id="${a.id}"]`)) container.appendChild(askCard(a, who(a)))
  }
  container.querySelector('.empty')?.remove()
  if (!asks.length && emptyHTML) container.insertAdjacentHTML('beforeend', emptyHTML)
}
