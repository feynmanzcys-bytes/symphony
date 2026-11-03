// Motion that goes on by itself (an animation that runs for good, the pixel office's people walking about) stops while
// the system asks for less motion (prefers-reduced-motion) or the page's switch says so (WCAG 2.1 2.2.2): html then
// has data-still, which the stylesheets (ui.css) and the office's drawing follow. The switch is remembered in this
// browser (symphony.still), for every theme.
import { t } from '/i18n.js'

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

// stillSwitch(button): the switch that pauses what moves by itself, pressed while it's paused; named by its title (a
// title that said what a click does would be read against its state); with the system asking for less motion, held on
// and saying why
export function stillSwitch(b) {
  b.setAttribute('aria-label', t('motion.pause'))
  const draw = () => {
    b.textContent = motion.still ? '▶' : '⏸'
    b.setAttribute('aria-pressed', String(motion.still))
    if (motion.system) b.setAttribute('aria-disabled', 'true')
    else b.removeAttribute('aria-disabled')
    b.title = t(motion.system ? 'motion.system' : 'motion.pause')
  }
  b.addEventListener('click', () => { if (!motion.system) motion.set(!chosen) })
  motion.on(draw)
  draw()
  return b
}
