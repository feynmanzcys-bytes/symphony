// The connection banner, shared by every theme and the approval page: a line at the top while the page can't reach the
// bridge (it stopped, or restarts), so what the page shows isn't taken for the office as it is now. It waits a moment
// before it shows, so a stream that drops and comes straight back doesn't flash it. When the bridge answers but no
// longer takes the page's key (it made a new one), it says so at once: the page must be opened again from the symphony
// program. Styled inline (the approval page has no ui.css); a theme may restyle #conn with its own rules and !important.
//
//   connectionBanner(sym)    sym: connect()'s client, whose 'connection' and 'pagekey' it follows
import { t } from '/i18n.js'

const GRACE = 1500

export function connectionBanner(sym) {
  const el = document.createElement('div')
  el.id = 'conn'
  el.setAttribute('role', 'alert')
  el.hidden = true
  el.textContent = t('error.unreachable')
  el.style.cssText = 'position:fixed; top:8px; left:50%; transform:translateX(-50%); z-index:1000; pointer-events:none; ' +
    'padding:3px 12px; background:var(--danger, #d64541); color:#fff; border:2px solid var(--danger-dark, #7a1f1c); ' +
    'box-shadow:2px 2px 0 rgba(0,0,0,.3); font:700 13px/1.5 system-ui, sans-serif; max-width:calc(100vw - 24px); text-align:center'
  document.body.appendChild(el)
  let timer = 0, down = false, keyLost = !!sym.keyLost
  const draw = () => {
    el.textContent = t(keyLost ? 'error.page_key_changed' : 'error.unreachable')
    el.hidden = !(keyLost || down)
  }
  sym.on('connection', up => {
    clearTimeout(timer)
    if (up) { down = false; draw() } else timer = setTimeout(() => { down = true; draw() }, GRACE)
  })
  sym.on('pagekey', lost => { keyLost = lost; draw() })
  draw()
  return el
}
