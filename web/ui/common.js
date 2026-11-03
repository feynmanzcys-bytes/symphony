// What the shared pieces of the office page have in common, for every theme: the state colors, a session's words
// (its status, its build from the model and effort), a little text markup, the launch form's drop-downs and the
// folder browser.
import { t, lang, errorText } from '/i18n.js'
import { esc } from '/asks.js'

export { esc, t, lang, errorText }

export const GREEN = '#2e9e5b', YELLOW = '#f2b705', RED = '#d64541', GRAY = '#a9a49b'
// three state colors: idle green, working yellow, waiting for an approval or an answer red; off duty gray (the drawing's
// colors, for a canvas)
export const STATE_COLOR = { idle: GREEN, thinking: YELLOW, tool: YELLOW, waiting: RED, ended: GRAY, done: GRAY }
export const TASK_COLOR = { running: YELLOW, ending: YELLOW, failed: RED, ended: GRAY }
// The same states for the marks on the page (dots, bands, lights): CSS variables each theme sets for its color scheme
// (ui.css: --st-idle, --st-busy, --st-wait, --st-off), dark enough on light paper to stand out (3:1), the drawing's
// colors where a page sets none. Not for a canvas, which takes no var().
const mark = (name, color) => `var(--st-${name}, ${color})`
export const GRAY_MARK = mark('off', GRAY)
export const STATE_MARK = { idle: mark('idle', GREEN), thinking: mark('busy', YELLOW), tool: mark('busy', YELLOW), waiting: mark('wait', RED), ended: GRAY_MARK, done: GRAY_MARK }
export const TASK_MARK = { running: mark('busy', YELLOW), ending: mark('busy', YELLOW), failed: mark('wait', RED), ended: GRAY_MARK }

export const base = p => (p || '').split(/[\\/]/).filter(Boolean).pop() || p || '?' // Windows paths use backslashes
export const lower = s => String(s ?? '').toLowerCase()
// a state's dot: beside words that say the state, hidden from screen readers; alone, it says it (label) as a picture and
// in its tooltip, so the state is never told by its color only
export const dot = (state, label) => `<span class="dot" ${label ? `role="img" aria-label="${esc(label)}" title="${esc(label)}"` : 'aria-hidden="true"'} style="background:${STATE_MARK[state] ?? GRAY_MARK}"></span>`
export const element = html => { const tpl = document.createElement('template'); tpl.innerHTML = html; return tpl.content.firstElementChild }
export const hash = s => [...String(s)].reduce((h, c) => (h * 31 + c.charCodeAt(0)) | 0, 7) >>> 0

// ---------- a session in words ----------
// what a session waits for: an approval or an answer (on the page), or an MCP server's form (in its terminal)
export function waitingText(sym, sessionId) {
  const kinds = new Set(sym.state.asks.filter(a => a.session === sessionId).map(a => a.kind))
  const approval = kinds.has('permission'), answer = kinds.has('question')
  if (!approval && !answer && kinds.has('elicitation')) return t('state.waitingForm')
  return t(approval && answer ? 'state.waitingBoth' : approval ? 'state.waitingApproval' : answer ? 'state.waitingAnswer' : 'state.waiting')
}
// an ask's kind in words: a permission, a question, an MCP server's form
export const askKind = kind => t(kind === 'question' ? 'asks.question' : kind === 'elicitation' ? 'asks.elicitation' : 'asks.permission')
// what an ask is about, in a line: the question, the form's server and message, the tool's main argument
export function askSummary(a) {
  if (a.kind === 'question') return a.questions?.[0]?.question || ''
  if (a.kind === 'elicitation') return [a.server, a.message].filter(Boolean).join(': ')
  return a.input?.command || a.input?.file_path || a.input?.pattern || a.input?.url || a.input?.description || ''
}
export function statusText(sym, s) {
  if (!s || s.state === 'ended') return t('state.ended')
  if (s.state === 'waiting') return waitingText(sym, s.id)
  if (s.state === 'tool' && s.tool) return t('state.working', { tool: s.tool })
  return t('state.' + s.state)
}

// ---------- the build: size follows the model, muscles follow the effort ----------
const MODEL_TIERS = [['haiku', 0], ['sonnet', 1], ['opus', 2], ['fable', 3]]
const EFFORT_TIERS = { low: 0, medium: 1, high: 2, xhigh: 3, max: 4 }
export function modelTier(model) {
  const m = String(model || '').toLowerCase()
  const hit = MODEL_TIERS.find(([k]) => m.includes(k))
  return hit ? hit[1] : 1
}
export function effortTier(effort) {
  if (effort in EFFORT_TIERS) return EFFORT_TIERS[effort]
  const n = Number(effort)
  return effort && Number.isFinite(n) ? Math.max(0, Math.min(4, Math.round(n / 25))) : 1
}
export function build(session) {
  const size = modelTier(session?.model), muscle = effortTier(session?.effort)
  return {
    size, muscle,
    bw: [10, 12, 14, 16][size], bh: [7, 8, 9, 10][size],
    armW: [1, 2, 2, 3, 3][muscle], armH: [2, 2, 3, 3, 4][muscle],
    headband: muscle >= 4, // at max effort it wears a red headband
  }
}
export const MODEL_NAME = ['Haiku', 'Sonnet', 'Opus', 'Fable']
export const EFFORT_NAME = ['low', 'medium', 'high', 'xhigh', 'max']
export function buildText(s) {
  if (!s?.model) return ''
  const b = build(s)
  return t('card.build', { model: s.model + (s.effort ? ' · ' + s.effort : ''), size: MODEL_NAME[b.size], muscle: EFFORT_NAME[b.muscle] })
}

// ---------- text ----------
// text with a little shape: fenced code, `code`, **bold**; everything is escaped first
export function richText(text) {
  const parts = String(text ?? '').split(/(```[\s\S]*?(?:```|$))/)
  return parts.map((p, i) => i % 2
    ? `<pre>${esc(p.replace(/^```[^\n]*\n?/, '').replace(/\n?```$/, ''))}</pre>`
    // a blank line next to a fenced block would double the <pre>'s own margin: drop it
    : esc(p.replace(/\n+$/, i ? '' : '\n').replace(/^\n+/, i < parts.length - 1 ? '' : '\n')).replace(/`([^`\n]+)`/g, '<code>$1</code>').replace(/\*\*([^*\n]+)\*\*/g, '<b>$1</b>')).join('')
}
export const timeOf = it => `<time>${new Date(it.time).toLocaleTimeString(lang)}</time>`

// ---------- forms ----------
// a folder field shows the end of its path, the folder's own name, rather than its start
export const showEnd = el => { el.scrollLeft = el.scrollWidth }
// labels of the drop-downs, in this page's language; model names and effort levels stay as they are
export const optionLabel = {
  model: o => o.value ? o.label : t('option.default'),
  effort: o => o.value || t('option.default'),
  permissionMode: o => o.value ? o.value + t('common.colon') + t('option.permission.' + o.value) : t('option.default'),
  resume: o => t(o.value ? 'option.resume.' + o.value : 'option.resume.new'),
}
export const fill = (select, options, value) => {
  select.innerHTML = (options || []).map(o => `<option value="${esc(o.value)}">${esc(optionLabel[select.name](o))}</option>`).join('')
  select.value = value ?? ''
}

// the folder browser under a folder field (the launch dialog's, an advisor's in the task dialog): box is its .browser,
// fail shows why a folder can't be opened
export async function browse(sym, box, path, fail, home) {
  const r = await sym.dirs(path || home || '')
  if (!r.ok) return fail(t('launch.browseFailed', { reason: errorText(r.data) }))
  const data = r.data
  box.hidden = false
  box.dataset.path = data.path
  box.querySelector('.cur').textContent = data.path
  // the bridge builds the paths, so the page never deals with / or \; at the top it lists the drives (Windows)
  const up = data.parent ? [['⬆ ' + t('launch.up'), data.parent]] : (data.roots || []).filter(x => x !== data.path).map(x => ['💽 ' + x, x])
  const items = up.concat((data.dirs || []).map(d => [d.name + '/', d.path]))
  const ul = box.querySelector('ul')
  ul.innerHTML = items.map(([label]) => `<li><button type="button">${esc(label)}</button></li>`).join('') || `<li style="color:var(--muted);cursor:default">${esc(t('launch.noSubfolders'))}</li>`
  ul.querySelectorAll('li').forEach((li, i) => { if (items[i]) li.onclick = () => browse(sym, box, items[i][1], fail, home) })
}

// ---------- drawing what one clicks ----------
// setHTML puts html into el only when it differs from what it put there last (says whether it did): a row or a tile
// drawn again the same stays the element it was. While the keyboard's focus is in el, or with keep (a list an input
// points into with aria-activedescendant), the elements stay where the new html has them again (morph): only what
// changed in them changes, so a screen reader doesn't read the focused row anew at every redraw. What had the focus and
// was drawn anew has it again: the element with the same data-* attributes, in a parent with the same ones (keyFor).
const drawnHTML = new WeakMap()
export function setHTML(el, html, keep = false) {
  if (drawnHTML.get(el) === html) return false
  drawnHTML.set(el, html)
  const focused = keyFor(el, document.activeElement)
  if (focused || keep) morph(el, html)
  else el.innerHTML = html
  if (focused && !el.querySelector(focused)?.contains(document.activeElement)) el.querySelector(focused)?.focus({ preventScroll: true })
  return true
}
// pointAt says which option of its list an input points at for screen readers (aria-activedescendant), or none: set
// only when it changes, as setting it again may read the option again
export function pointAt(input, id) {
  if (!id) return input.removeAttribute('aria-activedescendant')
  if (input.getAttribute('aria-activedescendant') !== id) input.setAttribute('aria-activedescendant', id)
}
// morph makes el's children what html says, keeping each element that is drawn again in its place (the same tag and
// data-* attributes, data-sig aside): its other attributes and its text are set to the new ones
function morph(el, html) {
  const tpl = document.createElement('template')
  tpl.innerHTML = html
  sync(el, tpl.content)
}
const dataKey = n => [...n.attributes].filter(a => a.name.startsWith('data-') && a.name !== 'data-sig').map(a => a.name + '=' + a.value).join(' ')
function sync(into, from) {
  const fresh = [...from.childNodes]
  fresh.forEach((f, i) => {
    const same = n => !!n && n.nodeType === f.nodeType && (f.nodeType !== 1 || (n.tagName === f.tagName && dataKey(n) === dataKey(f)))
    let o = into.childNodes[i]
    if (!same(o) && f.nodeType === 1 && dataKey(f)) { // drawn again further on: brought here
      const later = [...into.childNodes].slice(i + 1).find(same)
      if (later) o = into.insertBefore(later, o ?? null)
    }
    if (!same(o)) { if (o) into.replaceChild(f, o); else into.appendChild(f); return } // (moved out of the template)
    if (f.nodeType !== 1) { if (o.nodeValue !== f.nodeValue) o.nodeValue = f.nodeValue; return }
    for (const a of [...o.attributes]) if (!f.hasAttribute(a.name)) o.removeAttribute(a.name)
    for (const a of [...f.attributes]) if (o.getAttribute(a.name) !== a.value) o.setAttribute(a.name, a.value)
    sync(o, f)
  })
  while (into.childNodes.length > fresh.length) into.lastChild.remove()
}
// a selector for at inside el that finds it again once el is drawn anew: its tag and data-* attributes, and its
// parents' up to el or up to one with an id (data-sig, which says how it was drawn, left out); null when at isn't in el
// or says nothing of itself
export function keyFor(el, at) {
  if (!at || at === el || !el.contains(at)) return null
  const parts = []
  for (let n = at; n && n !== el; n = n.parentElement) {
    const data = [...n.attributes].filter(a => a.name.startsWith('data-') && a.name !== 'data-sig').map(a => `[${a.name}="${CSS.escape(a.value)}"]`).join('')
    if (n.id) { parts.unshift('#' + CSS.escape(n.id) + data); break }
    if (data || n === at) parts.unshift(n.tagName.toLowerCase() + data)
  }
  return /[[#]/.test(parts[parts.length - 1]) ? parts.join(' ') : null
}
// A click is a press and a release on the same element: one a redraw replaces in between never gets it, so a busy
// session's row could hardly be clicked. deferWhilePressed(draw) gives a draw that, while a pointer is down, only notes
// that it's wanted and runs once the press is over, after the click it makes. A press longer than PRESS_HOLD lets the
// draws go on (a press that ends outside the window must not hold the page).
const PRESS_HOLD = 500
let pressedAt = 0
const wanted = new Set()
const pressed = () => pressedAt > 0 && performance.now() - pressedAt < PRESS_HOLD
function released() {
  pressedAt = 0
  setTimeout(() => { const fns = [...wanted]; wanted.clear(); for (const fn of fns) fn() }) // after the click
}
addEventListener('pointerdown', () => {
  const at = pressedAt = performance.now()
  setTimeout(() => { if (pressedAt === at) released() }, PRESS_HOLD)
}, true)
addEventListener('pointerup', released, true)
addEventListener('pointercancel', released, true)
addEventListener('blur', released)
export function deferWhilePressed(draw) {
  const run = (...args) => { if (pressed()) wanted.add(run); else draw(...args) }
  return run
}

// a button in the pixel style
export function button(text, cls, fn, title) {
  const b = document.createElement('button')
  b.className = 'px ' + cls
  b.textContent = text
  b.onclick = fn
  if (title) b.title = title
  return b
}
