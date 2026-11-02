// Accessibility shared by every theme and the approval page:
//
//   dialog(el, heading)   a window or dialog of the page (.modal, shown and hidden by its hidden attribute) for the
//                         keyboard and screen readers: named by its heading; over the page it's a modal dialog (focus
//                         moves into it as it opens, Tab and Shift+Tab stay in it, focus goes back to what opened it as
//                         it closes); one a theme lays into its page (Coworkers) or docks into another window is a region
//   announce(text)        says text politely, through a live region that's always there
//   announceAsks(sym)     says how many asks wait for you, whenever there are more
//
// and rows a click opens, marked role="button" tabindex="0", open with Enter or Space too. Esc stays the themes'
// business: they know which window is on top (and never close one while its asks are being answered).
import { t } from '/i18n.js'
import { keyFor } from '/ui/common.js'

// out of sight but read by screen readers (inline: the approval page has no ui.css)
export const SR_ONLY = 'position:absolute; width:1px; height:1px; margin:-1px; padding:0; border:0; overflow:hidden; clip-path:inset(50%); white-space:nowrap'

// ---------- the polite voice ----------
let voice = null
export function announce(text) {
  if (!voice) {
    voice = document.createElement('div')
    voice.id = 'announce'
    voice.setAttribute('role', 'status')
    voice.setAttribute('aria-live', 'polite')
    voice.setAttribute('aria-atomic', 'true')
    voice.style.cssText = SR_ONLY
    document.body.appendChild(voice)
  }
  // emptied first and filled a moment later, so the same words twice are read twice
  voice.textContent = ''
  setTimeout(() => { voice.textContent = text }, 100)
}

// the asks waiting for you: said when there are some as the page opens, and whenever there are more
export function announceAsks(sym) {
  let seen = null
  sym.on('snapshot', s => {
    const n = s.asks.length
    if (n > (seen ?? 0)) announce(t('common.asksPending', { n }))
    seen = n
  })
}

// ---------- rows a click opens ----------
// (a native button does this itself; a row is a div or a tr marked role="button")
addEventListener('keydown', ev => {
  if (ev.key !== 'Enter' && ev.key !== ' ') return
  const el = ev.target
  if (ev.defaultPrevented || ev.repeat || !(el instanceof Element) || el.getAttribute('role') !== 'button' || /^(BUTTON|INPUT|SELECT|TEXTAREA|A|SUMMARY)$/.test(el.tagName)) return
  ev.preventDefault()
  el.click()
})

// ---------- windows and dialogs ----------
const FOCUSABLE = 'a[href], button, input, select, textarea, summary, [tabindex]'
const watched = new Set() // every window and dialog
const openers = new WeakMap() // a window -> what opened it (had the focus, or was pressed), to go back to as it closes:
// { node, key }, key finding the element that took its place where it was drawn again (keyFor)
const stack = [] // the modal ones open, the last on top
// what the user last acted on: the element with the focus at a key, the focusable one pressed at a pointer, with the key
// that finds it again once its row is drawn anew (taken now: by the time the window it opens says so, the theme may
// have drawn it again already). A window that opens takes it as its opener.
let actor = null
const act = el => { actor = el && el !== document.body ? { node: el, key: keyFor(document.documentElement, el) } : null }
addEventListener('keydown', () => act(document.activeElement), true)
addEventListener('pointerdown', ev => act(ev.target instanceof Element ? ev.target.closest(FOCUSABLE) : null), true)

const usable = el => el instanceof HTMLElement && el.isConnected && !el.closest('[hidden], [inert]') && !el.disabled &&
  el.getClientRects().length > 0 && el.checkVisibility({ visibilityProperty: true })
const tabbable = root => [...root.querySelectorAll(FOCUSABLE)].filter(el => el.tabIndex >= 0 && usable(el))

// into a window: the first field or button the theme marked (autofocus), else its heading, which reads its name
function focusInto(el) {
  const target = el.querySelector('[autofocus]') ?? document.getElementById(el.getAttribute('aria-labelledby'))
  ;(usable(target) ? target : tabbable(el)[0])?.focus({ preventScroll: true })
}
// back to what opened a window that closed: what opened it, else (that went too) what opened the window it was in, else
// the window on top
function restore(opener) {
  for (let hops = 0; opener && hops < 8; hops++) {
    const n = opener.node.isConnected ? opener.node : opener.key && document.querySelector(opener.key)
    if (usable(n)) return n.focus({ preventScroll: true })
    const host = n && [...watched].filter(w => w.contains(n)).sort((a, b) => a.contains(b) ? 1 : -1)[0] // the innermost
    opener = host ? openers.get(host) : null
  }
  const top = stack[stack.length - 1]
  if (top) focusInto(top)
}

export function dialog(el, heading) {
  if (!heading.id) heading.id = el.id + '-title'
  heading.tabIndex = -1 // focusable from a script: a window with nothing else to focus starts on its name
  el.setAttribute('aria-labelledby', heading.id)
  for (const b of el.querySelectorAll('button.close')) b.setAttribute('aria-label', t('common.close'))
  watched.add(el)
  let shown = false
  const sync = () => {
    // a modal over the page, or a region: laid into the theme's page (not fixed), or docked into a modal window
    const modal = getComputedStyle(el).position === 'fixed' && !el.parentElement?.closest('[aria-modal="true"]')
    el.setAttribute('role', modal ? 'dialog' : 'region')
    if (modal) el.setAttribute('aria-modal', 'true')
    else el.removeAttribute('aria-modal')
    const open = !el.hidden
    if (open === shown) {
      if (open && !modal && stack.includes(el)) stack.splice(stack.indexOf(el), 1) // docked since it opened
      return
    }
    shown = open
    if (open) {
      openers.set(el, actor && !el.contains(actor.node) ? actor : null)
      if (!modal) return
      stack.push(el)
      if (!el.contains(document.activeElement)) focusInto(el)
    } else {
      if (stack.includes(el)) stack.splice(stack.indexOf(el), 1)
      const at = document.activeElement
      if (!at || at === document.body || el.contains(at)) restore(openers.get(el))
    }
  }
  new MutationObserver(sync).observe(el, { attributes: true, attributeFilter: ['hidden', 'class'] })
  sync()
}

// Tab and Shift+Tab go round the window on top, never out of it
addEventListener('keydown', ev => {
  if (ev.key !== 'Tab' || ev.defaultPrevented || ev.altKey || ev.ctrlKey || ev.metaKey) return
  const top = stack[stack.length - 1]
  if (!top || top.hidden) return
  const items = tabbable(top), at = document.activeElement
  if (!items.length) return ev.preventDefault()
  const inside = top.contains(at)
  const before = inside && items.some(x => x !== at && (x.compareDocumentPosition(at) & Node.DOCUMENT_POSITION_FOLLOWING))
  const after = inside && items.some(x => x !== at && (at.compareDocumentPosition(x) & Node.DOCUMENT_POSITION_FOLLOWING))
  if (!inside || (ev.shiftKey ? !before : !after)) {
    ev.preventDefault()
    ;(ev.shiftKey ? items[items.length - 1] : items[0]).focus()
  }
})
