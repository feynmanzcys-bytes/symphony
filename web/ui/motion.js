// Motion that goes on by itself (an animation that runs for good, the pixel office's people walking about) stops while
// the system asks for less motion (prefers-reduced-motion) or the page's switch says so (WCAG 2.1 2.2.2): html then
// has data-still, which the stylesheets (ui.css) and the office's drawing follow. The switch, "Pause motion" in the
// settings (ui/settings.js), is remembered in this browser (symphony.still), for every theme.

const STORE = 'symphony.still'
const asked = matchMedia('(prefers-reduced-motion: reduce)')
const heard = new Set()
let chosen = localStorage.getItem(STORE) === '1'

export const motion = {
  get system() { return asked.matches }, // the system asks for less motion: still, whatever the switch
  get still() { return asked.matches || chosen },
  set(on) { chosen = on; localStorage.setItem(STORE, on ? '1' : '0'); apply() },
  on(fn) { heard.add(fn) }, // fn(still) at every change
}
function apply() {
  document.documentElement.toggleAttribute('data-still', motion.still)
  for (const fn of heard) fn(motion.still)
}
asked.addEventListener('change', apply)
apply()
