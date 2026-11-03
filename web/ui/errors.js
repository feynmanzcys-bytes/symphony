// Errors, for every theme: whatever went wrong is written to the bridge's error log by itself (the bridge keeps its own
// failures, the page reports what it shows: nobody copies anything), and the page says so; to report a problem one
// goes to the settings and takes the report file (ui/settings.js).
//
//   setText(el, text)          write text into el only when it changed: a selection in it outlives a redraw
//   errorBox(text, opts)       an error in a box: its text, and that the error log has it (opts.report false: the
//                              bridge kept it already, so the page doesn't report it again)
//   putErrorBox(el, text, opts)  fill el with an error box for text, or empty it; drawn again only when the text changed
//   showError(text, opts)      the same over the page, in place of alert(); opts.title says what failed
//   onBackdrop(el, fn)         fn when a click lands on a modal's backdrop, not when a drag that selects text ends there
import { t, lang } from '/i18n.js'
import { button } from '/ui/common.js'
import { dialog } from '/ui/a11y.js'
import { reportError } from '/symphony.js'

export function setText(el, text) {
  if (el.textContent !== text) el.textContent = text
}

// an error as a sentence: its first letter a capital (the bridge's words come as a reason, "no such task")
const sentence = text => String(text).replace(/^\p{Ll}/u, c => c.toLocaleUpperCase(lang))

// an error box: the text, then where it went (the error log) and how to report it
let boxes = 0
export function errorBox(text, { report = true } = {}) {
  if (report) reportError('ui', text, 'shown on ' + (document.documentElement.dataset.theme || 'the page'))
  const box = document.createElement('div')
  box.className = 'errbox'
  const msg = box.appendChild(document.createElement('div'))
  msg.className = 'msg'
  msg.id = 'errbox-' + ++boxes // (a dialog that shows it names it as its description)
  msg.textContent = sentence(text)
  const hint = box.appendChild(document.createElement('div'))
  hint.className = 'hint'
  hint.textContent = t('errlog.inLog')
  return box
}
// putErrorBox fills el with an error box for text, or empties it (no text): drawn again only when the text changed, so
// a selection in it stays and the error is reported once
export function putErrorBox(el, text, opts) {
  const msg = el.querySelector(':scope > .errbox > .msg')
  if (!text) { if (el.firstChild) el.textContent = ''; return }
  if (msg && msg.textContent === sentence(text)) return
  el.replaceChildren(errorBox(text, opts))
}

// ---------- over the page ----------
let shown = null
export function showError(text, { title = t('errlog.oops'), report = true } = {}) {
  if (!shown) {
    const el = document.createElement('div')
    el.className = 'modal pop'
    el.id = 'oops'
    el.hidden = true
    el.innerHTML = `<div class="panel"><h2></h2><div class="box"></div><div class="actions"></div></div>`
    document.body.appendChild(el)
    const h = el.querySelector('h2'), close = () => { el.hidden = true }
    const ok = button(t('common.ok'), 'primary', close)
    ok.type = 'button'
    ok.autofocus = true
    el.querySelector('.actions').append(ok)
    dialog(el, h)
    onBackdrop(el, close)
    el.addEventListener('keydown', ev => { if (ev.key === 'Escape') { ev.stopPropagation(); close() } })
    shown = { el, h, box: el.querySelector('.box') }
  }
  shown.h.textContent = title
  const box = errorBox(text, { report })
  shown.box.replaceChildren(box)
  shown.el.setAttribute('aria-describedby', box.querySelector('.msg').id) // (read with its name, as alert() was)
  shown.el.hidden = false
}

export function onBackdrop(el, fn) {
  let down = null
  el.addEventListener('pointerdown', ev => { down = ev.target })
  el.addEventListener('click', ev => {
    const from = down
    down = null
    if (ev.target === el && from === el) fn(ev)
  })
}
