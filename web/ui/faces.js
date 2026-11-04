// The faces: a body with two eyes and a mouth, in a shape and a color. An employee's is its own once picked (the
// account's look, {shape, hue}: picked when it is made, changed on the employees page); without one, the name picks
// a shape and a hue (so advisors, groups and names from before have faces too, the ones they always had). The markup
// is the same everywhere (the list, the profile, a group's members, the team chat); the theme's sheet draws the state
// as motion, each face a little out of step with the others (--d). The state is on the shape itself: a stroke in its
// color along the outline, which glows outward (a drop shadow of the shape, never a frame around it).
//
//   lookOf(name, look?)              → { shape, hue }
//   faceHTML(name, state, cls, look?) → the <div class="face"> markup
//   avatarURL(name, look?)           → a CSS url() of the face, for a background image
//   lookPicker(prefix)               → an element with two radio groups (the shape, the color); .value, .set(look), a change event
import { t, esc, hash } from '/ui/common.js'

export const SHAPES = ['circle', 'square', 'hex', 'diamond', 'drop', 'pill']
const BODY = {
  circle: '<circle class="body" cx="22" cy="22" r="20"/>',
  square: '<rect class="body" x="3" y="3" width="38" height="38" rx="10"/>',
  hex: '<polygon class="body" points="22,2 40,12 40,32 22,42 4,32 4,12"/>',
  diamond: '<path class="body" d="M22 3 L41 22 L22 41 L3 22 Z"/>',
  drop: '<path class="body" d="M22 2 C31 13 40 19 40 27 A18 18 0 0 1 4 27 C4 19 13 13 22 2 Z"/>',
  pill: '<rect class="body" x="5" y="3" width="34" height="38" rx="17"/>',
}
export const HUES = [4, 28, 46, 95, 150, 185, 212, 250, 285, 325] // ten colors round the wheel
export const HUE_NAMES = ['red', 'orange', 'yellow', 'green', 'teal', 'cyan', 'blue', 'indigo', 'purple', 'pink']

export function lookOf(name, look) {
  if (look && BODY[look.shape]) return { shape: look.shape, hue: ((Math.round(look.hue) % 360) + 360) % 360 }
  const h = hash(name)
  return { shape: SHAPES[(h >>> 8) % 4], hue: h % 360 } // (the four shapes the names picked from before: a face stays what it was)
}
export function faceHTML(name, state, cls = '', look) {
  const { shape, hue } = lookOf(name, look), mouth = (hash(name) >>> 12) % 2
  // (one lightness for both schemes: the faces sit on ivory or on charcoal, in the sidebar, the profile and the
  // group alike; --d puts the faces' motion out of step)
  return `<div class="face ${cls}" data-state="${esc(state)}" aria-hidden="true" style="--c:hsl(${hue} 50% 50%);--d:${(hue % 7) + (hash(name) % 3)}"><svg viewBox="0 0 44 44">` +
    `${BODY[shape]}` +
    `<ellipse class="eye" cx="15.5" cy="19" rx="3.1" ry="3.6"/><ellipse class="eye" cx="28.5" cy="19" rx="3.1" ry="3.6"/>` +
    `<circle class="pupil" cx="15.9" cy="19.4" r="1.6"/><circle class="pupil" cx="28.9" cy="19.4" r="1.6"/>` +
    (mouth ? `<path class="mouth" d="M15 29 Q22 34 29 29"/>` : `<path class="mouth" d="M16 30 L28 30"/>`) +
    `<rect class="hand" x="34" y="4" width="7" height="14" rx="3.5"/><path class="check" d="M30 36 L35 41 L43 31"/></svg></div>`
}
export function avatarURL(name, look) {
  const { shape, hue } = lookOf(name, look)
  const body = BODY[shape].replace(' class="body"', ` fill="hsl(${hue} 48% 46%)"`)
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 44 44">${body}<ellipse cx="15.5" cy="19" rx="3.1" ry="3.6" fill="#fff"/><ellipse cx="28.5" cy="19" rx="3.1" ry="3.6" fill="#fff"/>` +
    `<circle cx="15.9" cy="19.4" r="1.6" fill="#111b21"/><circle cx="28.9" cy="19.4" r="1.6" fill="#111b21"/><path d="M15 29 Q22 34 29 29" stroke="#fff" stroke-width="2" fill="none" stroke-linecap="round"/></svg>`
  return `url("data:image/svg+xml,${encodeURIComponent(svg)}")`
}

// the picker: the shapes as faces in the color picked, the colors as round swatches; each a radio, named for a screen
// reader; the element's value is the look, set() shows one, and a change bubbles up
export function lookPicker(prefix) {
  const el = document.createElement('div')
  el.className = 'look-pick'
  el.innerHTML = `<div class="row shapes" role="radiogroup" aria-label="${esc(t('launch.shape'))}">` +
    SHAPES.map(s => `<label title="${esc(t('launch.shape.' + s))}"><input type="radio" name="${prefix}-shape" value="${s}" aria-label="${esc(t('launch.shape.' + s))}">${faceHTML('', 'idle', '', { shape: s, hue: 0 })}</label>`).join('') + '</div>' +
    `<div class="row hues" role="radiogroup" aria-label="${esc(t('launch.color'))}">` +
    HUES.map((h, i) => `<label title="${esc(t('launch.hue.' + HUE_NAMES[i]))}"><input type="radio" name="${prefix}-hue" value="${h}" aria-label="${esc(t('launch.hue.' + HUE_NAMES[i]))}"><span class="swatch" style="--c:hsl(${h} 50% 50%)"></span></label>`).join('') + '</div>'
  const hueOf = () => Number(el.querySelector(`input[name="${prefix}-hue"]:checked`)?.value ?? HUES[0])
  const paint = () => { for (const f of el.querySelectorAll('.shapes .face')) f.style.setProperty('--c', `hsl(${hueOf()} 50% 50%)`) }
  el.addEventListener('change', paint)
  Object.defineProperty(el, 'value', { get: () => ({ shape: el.querySelector(`input[name="${prefix}-shape"]:checked`)?.value ?? SHAPES[0], hue: hueOf() }) })
  el.set = look => {
    const { shape, hue } = lookOf('', look)
    for (const r of el.querySelectorAll(`input[name="${prefix}-shape"]`)) r.checked = r.value === shape
    const near = HUES.reduce((a, b) => Math.min(Math.abs(a - hue), 360 - Math.abs(a - hue)) <= Math.min(Math.abs(b - hue), 360 - Math.abs(b - hue)) ? a : b)
    for (const r of el.querySelectorAll(`input[name="${prefix}-hue"]`)) r.checked = Number(r.value) === near
    paint()
  }
  el.set(null)
  return el
}
