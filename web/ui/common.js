// What the shared pieces of the office page have in common, for every theme: the state colors, a session's words
// (its status, its build from the model and effort), a little text markup, the launch form's drop-downs and the
// folder browser.
import { t, lang, errorText } from '/i18n.js'
import { esc } from '/asks.js'

export { esc, t, lang, errorText }

export const GREEN = '#2e9e5b', YELLOW = '#f2b705', RED = '#d64541', GRAY = '#a9a49b'
// three state colors: idle green, working yellow, waiting for an approval or an answer red; off duty gray
export const STATE_COLOR = { idle: GREEN, thinking: YELLOW, tool: YELLOW, waiting: RED, ended: GRAY, done: GRAY }
export const TASK_COLOR = { running: YELLOW, ending: YELLOW, failed: RED }

export const base = p => (p || '').split(/[\\/]/).filter(Boolean).pop() || p || '?' // Windows paths use backslashes
export const lower = s => String(s ?? '').toLowerCase()
export const dot = state => `<span class="dot" style="background:${STATE_COLOR[state] ?? GRAY}"></span>`
export const element = html => { const tpl = document.createElement('template'); tpl.innerHTML = html; return tpl.content.firstElementChild }
export const hash = s => [...String(s)].reduce((h, c) => (h * 31 + c.charCodeAt(0)) | 0, 7) >>> 0

// ---------- a session in words ----------
// what a session waits for: an approval or an answer
export function waitingText(sym, sessionId) {
  const kinds = new Set(sym.state.asks.filter(a => a.session === sessionId).map(a => a.kind))
  const approval = kinds.has('permission'), answer = kinds.has('question')
  return t(approval && answer ? 'state.waitingBoth' : approval ? 'state.waitingApproval' : answer ? 'state.waitingAnswer' : 'state.waiting')
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
  ul.innerHTML = items.map(([label]) => `<li>${esc(label)}</li>`).join('') || `<li style="color:#8a867e;cursor:default">${esc(t('launch.noSubfolders'))}</li>`
  ul.querySelectorAll('li').forEach((li, i) => { if (items[i]) li.onclick = () => browse(sym, box, items[i][1], fail, home) })
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
