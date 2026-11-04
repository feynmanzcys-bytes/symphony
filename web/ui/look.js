// How the page reads: its size and its typeface, set in the settings and kept in this browser (ui/prefs.js).
//
//   look.scale → '1' | '1.15' | '1.3' | '1.5'   the whole page zoomed (html's --ui-scale, which ui.css puts on body)
//   look.font  → 'system' | 'serif' | 'rounded' | 'mono'   the typeface (html's data-font, which the theme's sheet reads)
//   look.set({ scale?, font? })
//   look.on(fn)                                 fn() after a change
//
// A zoom rather than a font size: the pages lay out in pixels, so a bigger root font alone would leave the boxes as
// they are. The typefaces are the system's (the content security policy loads none from elsewhere).
import { pref, setPref, onPref } from '/ui/prefs.js'

export const FONT_SCALE = 'symphony.fontScale', FONT = 'symphony.font'
export const SCALES = ['1', '1.15', '1.3', '1.5'], FONTS = ['system', 'serif', 'rounded', 'mono']
const heard = new Set()

// the layout's breakpoints go by the width the page has at its size (a zoomed page is that much narrower): html gets
// w-lt-<px> classes, which the sheets use in place of media queries (those only know the window)
const BREAKS = [1100, 700, 640, 520]
function fit() {
  const w = innerWidth / Number(look.scale)
  for (const bp of BREAKS) document.documentElement.classList.toggle('w-lt-' + bp, w <= bp)
}
function apply() {
  const root = document.documentElement, scale = look.scale, font = look.font
  root.style.setProperty('--ui-scale', scale)
  if (font === 'system') delete root.dataset.font; else root.dataset.font = font
  fit()
  for (const fn of heard) fn()
}
addEventListener('resize', fit)
export const look = {
  get scale() { const v = pref(FONT_SCALE, '1'); return SCALES.includes(v) ? v : '1' },
  get font() { const v = pref(FONT, 'system'); return FONTS.includes(v) ? v : 'system' },
  set({ scale, font }) {
    if (scale !== undefined) setPref(FONT_SCALE, scale === '1' ? null : scale)
    if (font !== undefined) setPref(FONT, font === 'system' ? null : font)
  },
  on(fn) { heard.add(fn) },
}
onPref(FONT_SCALE, apply)
onPref(FONT, apply)
apply()
