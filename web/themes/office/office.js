// The pixel office: one floor of a big company's building, drawn on a canvas, with the sessions as employees at their
// desks. The bridge's client (symphony.js) keeps the state; the employee window, the launch and task dialogs and the
// task window are the shared pieces (ui/*.js), which this theme places, names and draws figures for.
import { t, lang, errorText, fmtTime, applyI18n, langPicker } from '/i18n.js'
import { esc } from '/asks.js'
import { connect } from '/symphony.js'
import { GREEN, YELLOW, RED, GRAY, STATE_COLOR, STATE_MARK, GRAY_MARK, base, lower, dot, element, hash, build, buildText, statusText as statusOf, richText, setHTML, pointAt, deferWhilePressed, askKind } from '/ui/common.js'
import { connectionBanner } from '/ui/banner.js'
import { announce, announceAsks } from '/ui/a11y.js'
import { motion, stillSwitch } from '/ui/motion.js'
import { createChat } from '/ui/chat.js'
import { createLaunchDialog } from '/ui/launch.js'
import { createTaskUI } from '/ui/task.js'
import { themePicker } from '/ui/themes.js'

// the bridge's client (symphony.js) keeps the state; snapshot is its latest, taken before anything else looks at it
const sym = connect()
let snapshot = sym.state
sym.on('snapshot', s => { snapshot = s })

applyI18n()
connectionBanner(sym)
announceAsks(sym)
const prefs = document.querySelector('#hud .prefs'), langSel = langPicker()
prefs.appendChild(langSel)
themePicker(sym).then(sel => prefs.insertBefore(sel, langSel))

// ================= constants =================
const T = 16 // one tile is 16 pixels
const COLS = 6, COL_PITCH = 3 * T, ROW_PITCH = 4 * T, WALL_H = 4 * T + 8, AISLE = 24, LABEL_ROOM = 2 * T + 10
const DECOR = WALL_H - 48 // the back wall's windows and pictures hang below a plain band, where the HUD sits
const OFF = '#3b3e44' // laptop light of an empty desk

// Drawing the office at a finer pixel size, a line a pixel of the office thick (a highlight, an edge, a seam) comes out
// as thin as the screen shows: hair, in pixels of the office (0 while drawing anything else, or at the standard size)
let hair = 0
const rect = (g, x, y, w, h, c) => {
  g.fillStyle = c
  if (hair) { if (h === 1 && w >= 4) h = hair; else if (w === 1 && h >= 4) w = hair }
  g.fillRect(x, y, w, h)
}
const solid = (g, x, y, w, h, c) => { g.fillStyle = c; g.fillRect(x, y, w, h) } // never thinner: a body, an arm, a ring
// The finer steps' touches, a hair their pixel: a soft shadow on the floor below a thing (w wide from x, its bottom edge
// at y), a box with its corners cut, a run of dots (a reflection), a weave of dots over a floor.
function ground(g, x, y, w) {
  if (detail < 2 || !hair) return
  solid(g, x + 1, y, w - 2, 2 * hair, 'rgba(43,42,40,.16)'); solid(g, x + 2, y + 2 * hair, w - 4, 2 * hair, 'rgba(43,42,40,.07)')
}
function box(g, x, y, w, h, c) {
  if (detail < 2 || !hair || w < 3 || h < 3) return rect(g, x, y, w, h, c)
  solid(g, x + hair, y, w - 2 * hair, h, c); solid(g, x, y + hair, w, h - 2 * hair, c)
}
function dots(g, x, y, n, dx, dy, c) { for (let i = 0; i < n; i++) solid(g, x + i * dx, y + i * dy, hair, hair, c) }
function weave(g, x, y, w, h, c) {
  const p = document.createElement('canvas'), q = p.getContext('2d')
  p.width = p.height = 4; q.fillStyle = c; q.fillRect(0, 0, 1, 1); q.fillRect(2, 2, 1, 1)
  const pat = g.createPattern(p, 'repeat')
  pat.setTransform(new DOMMatrix([hair, 0, 0, hair, 0, 0]))
  g.fillStyle = pat; g.fillRect(x, y, w, h)
}

let launchOpts = null // /api/launch/options: whether the page can start sessions, and the drop-down choices
const sessionOf = acc => acc.session ? snapshot.sessions.find(s => s.id === acc.session) : null // the desk session
// the desk session the employee window shows: the one online, else the last one the bridge still has (its chat, its end
// and why)
const deskSessionOf = acc => sessionOf(acc) ?? snapshot.sessions.filter(s => s.account === acc.id && !s.task).sort((a, b) => Date.parse(b.updated) - Date.parse(a.updated))[0] ?? null
const sessionById = id => id ? snapshot.sessions.find(s => s.id === id) ?? null : null
const asking = sid => !!sid && snapshot.asks.some(a => a.session === sid) // the session waits for an approval or an answer
// tasks: a team of sessions; the snapshot has those running, ending or failed. The page seats each in a small meeting
// room (assignRooms): the bridge knows no rooms, only the hint of the one a task was started from
const taskById = id => snapshot.tasks.find(task => task.id === id) ?? null
const roomByTask = new Map() // task id -> the small room it sits in; a task with none has its members out of the building
const roomOf = task => roomByTask.get(task.id) ?? null
const roomTask = k => snapshot.tasks.find(task => roomOf(task)?.k === k) ?? null
const roomName = k => t('room.small.' + (k % 24 + 1)) + (k < 24 ? '' : ' ' + ['II', 'III', 'IV'][Math.min(2, Math.floor(k / 24) - 1)])
const taskRoom = task => { const r = roomOf(task); return r ? roomName(r.k) : t('task.noRoom') } // in this page's language
// Every task shown (running, ending, failed) takes a room, in id order so every browser agrees: the room it was started
// from when this layout has it and it's still free (its members may stand), else the first free room with seats enough,
// else the largest free one; a task that finds none is out of the building. A failed task keeps its room until dismissed.
function assignRooms() {
  roomByTask.clear()
  const free = new Set(smalls.map(r => r.k)), rest = []
  for (const task of [...snapshot.tasks].sort((a, b) => +a.id.slice(1) - +b.id.slice(1))) {
    const k = typeof task.room === 'number' ? task.room : -1
    if (free.has(k)) { roomByTask.set(task.id, smalls[k]); free.delete(k) } else rest.push(task)
  }
  for (const task of rest) {
    const rooms = [...free].map(k => smalls[k])
    const pick = rooms.find(r => r.seats >= task.members.length) ?? rooms.sort((a, b) => b.seats - a.seats)[0]
    if (pick) { roomByTask.set(task.id, pick); free.delete(pick.k) }
  }
}

const statusText = s => statusOf(sym, s) // a session in words (ui/common.js)

// ================= characters: size follows the model, muscles follow the effort =================
// A small person: hair on top, a face with dot eyes, a shirt with the arms in its color and the hands at their ends, two
// legs. Everyone has a look of their own (lookOf): the shirt, the skin and the hair come from palettes by seat, the
// shirts in blues, purples and a pink, away from the state colors, so the desk lights and the name tags still tell the
// state.
const EYE = '#2b2a28'
const SHIRTS = [['#4a78b5', '#3b6194'], ['#7d5ba6', '#644886'], ['#2f7d8c', '#246371'], ['#c25488', '#9f416e'],
  ['#34466e', '#283656'], ['#7fa9d6', '#6690bd'], ['#5a5fae', '#474b8f'], ['#a68fd0', '#8c75b6']] // [shirt, its shade]
// skin and hair in pairs that stand apart; long: the hair comes down the sides of the face
const HEADS = [{ skin:'#f3d2b0', hair:'#4a3020' }, { skin:'#e8b98f', hair:'#1f1f2a', long:true }, { skin:'#c98f62', hair:'#2b2420' },
  { skin:'#f6dcc4', hair:'#b48a52', long:true }, { skin:'#8d5a3b', hair:'#16161b' }, { skin:'#e8b98f', hair:'#7a3a24' },
  { skin:'#f3d2b0', hair:'#2b2420', long:true }, { skin:'#6b432c', hair:'#121216', long:true }, { skin:'#c98f62', hair:'#5b3a24' }]
const PANTS = ['#3a3f4b', '#2f4060', '#4a4038', '#2b2a28']
// off duty, or a subagent that's done: all gray
const GRAY_LOOK = { shirt:GRAY, shade:'#8f8a82', skin:'#cdc8bf', hair:'#7d7870', pants:'#8f8a82', shoe:'#7d7870', eye:'#5d5953' }
// an account's look: by its seat, so neighbors differ; an advisor (no seat) by its name
const looks = new Map()
function lookOf(acc) {
  const k = acc?.seat >= 0 ? acc.seat : hash(acc?.name ?? '')
  if (!looks.has(k)) {
    const [shirt, shade] = SHIRTS[k % SHIRTS.length], head = HEADS[(k * 4) % HEADS.length]
    looks.set(k, { shirt, shade, ...head, pants:PANTS[(k + Math.floor(k / COLS)) % PANTS.length], shoe:'#2b2a28', eye:EYE })
  }
  return looks.get(k)
}
const headRows = b => b.bh - (b.bh >= 9 ? 4 : 3) // the head's height in the body's box; the shirt is below it
// cx: the body's center line; by: the body's bottom edge (the top of the legs)
// pose: eyes 'open'|'down'|'closed'|'none' (none: facing away, the back of the head); armL/armR move the arms up or
// down; raiseL/raiseR raise a hand; legs draws the legs; step is the leg stepping forward
function drawPerson(g, cx, by, b, pose, look) {
  const left = cx - (b.bw >> 1), top = by - b.bh, right = left + b.bw, headH = headRows(b), shoulder = top + headH
  const headW = b.bw - 4, hl = left + 2, hr = hl + headW
  // the legs first: one stepping forward is lifted under the shirt, so only its shoe shows
  if (pose.legs) {
    const inset = b.bw >= 16 ? 3 : 2, lw = (b.bw - 2 * inset - 2) >> 1
    ;[left + inset, right - inset - lw].forEach((x, i) => {
      const y = by - (pose.step === i ? 1 : 0)
      solid(g, x, y, lw, 1, look.pants); solid(g, x, y + 1, lw, 1, look.shoe)
    })
  }
  // the shirt with a shade down its right side, the neck
  solid(g, left, shoulder, b.bw, b.bh - headH, look.shirt)
  solid(g, right - 1, shoulder + 1, 1, b.bh - headH - 1, look.shade)
  if (pose.eyes !== 'none') solid(g, cx - 1, shoulder, 2, 1, look.skin)
  // the head: the hair on top (two rows on a bigger head) and down the sides, the face below; from the back, all hair
  const hairH = headH >= 5 ? 2 : 1, eyeY = top + headH - 2, side = (look.long ? headH : eyeY - top + 1) - hairH
  solid(g, hl + 1, top, headW - 2, 1, look.hair)
  solid(g, hl, top + 1, headW, headH - 1, look.skin)
  if (pose.eyes === 'none') {
    solid(g, hl, top + 1, headW, headH - 2, look.hair)
    solid(g, hl, top + headH - 1, 1, 1, look.hair); solid(g, hr - 1, top + headH - 1, 1, 1, look.hair)
  } else {
    if (hairH > 1) solid(g, hl, top + 1, headW, hairH - 1, look.hair)
    solid(g, hl, top + hairH, 1, side, look.hair); solid(g, hr - 1, top + hairH, 1, side, look.hair)
    const ex1 = hl + Math.max(1, headW >> 2), ex2 = hr - 1 - Math.max(1, headW >> 2)
    if (pose.eyes === 'open') {
      solid(g, ex1, eyeY, 1, 1, look.eye); solid(g, ex2, eyeY, 1, 1, look.eye)
      if (detail && hair && hair < 1) { solid(g, ex1, eyeY, hair, hair, '#fffdf7'); solid(g, ex2, eyeY, hair, hair, '#fffdf7') } // a glint
    }
    else if (pose.eyes === 'down') { solid(g, ex1, eyeY + 1, 1, 1, look.eye); solid(g, ex2, eyeY + 1, 1, 1, look.eye) }
    else if (pose.eyes === 'closed') { solid(g, ex1 - 1, eyeY, 2, 1, look.eye); solid(g, ex2, eyeY, 2, 1, look.eye) }
  }
  if (detail >= 2 && hair) solid(g, hl + 1, top, headW - 2, hair, 'rgba(255,255,255,.3)') // light on the hair
  if (b.headband) {
    solid(g, hl, top + 1, headW, 1, RED)
    solid(g, hr, top + 1, 2, 1, RED); solid(g, hr + 1, top + 2, 2, 1, RED)
  }
  // the arms: the sleeve in the shirt's color, the hand at its end; a raised one reaches above the head
  const arm = (x, raised, dy) => {
    if (raised) { solid(g, x, top - 3, b.armW, headH + 4, look.shirt); solid(g, x, top - 3, b.armW, 1, look.skin) }
    else { solid(g, x, shoulder + dy, b.armW, b.armH - 1, look.shirt); solid(g, x, shoulder + dy + b.armH - 1, b.armW, 1, look.skin) }
  }
  arm(left - b.armW, pose.raiseL, pose.armL || 0)
  arm(right, pose.raiseR, pose.armR || 0)
  if (b.muscle >= 3) { // biceps
    if (!pose.raiseL) solid(g, left - b.armW - 1, shoulder + (pose.armL || 0), 1, b.armH - 1, look.shirt)
    if (!pose.raiseR) solid(g, right + b.armW, shoulder + (pose.armR || 0), 1, b.armH - 1, look.shirt)
  }
}

// a small person: a subagent beside its employee's desk; x, y is its top left, 8 by 7
const MINI_ROWS = ['..HHHH..', '.HHHHHH.', '.HEFFEH.', '..FFFF..', 'FSSSSSSF', '.SSSSSS.', '.PP..PP.']
function drawMiniPerson(g, x, y, look) {
  const c = { H:look.hair, E:look.eye, F:look.skin, S:look.shirt, P:look.pants }
  MINI_ROWS.forEach((row, r) => [...row].forEach((ch, k) => { if (ch !== '.') rect(g, x + k, y + r, 1, 1, c[ch]) }))
}

// symphony's mark: two beamed eighth notes, the beam in the accent color; x, y is its top left, 9 by 8
const MARK_ROWS = ['...BBBBBB', '...BBBBBB', '...N....N', '...N....N', '...N....N', '.NNN..NNN', 'NNNN.NNNN', '.NN...NN.']
function drawMark(g, x, y, note, beam) {
  MARK_ROWS.forEach((row, r) => [...row].forEach((ch, k) => { if (ch !== '.') rect(g, x + k, y + r, 1, 1, ch === 'B' ? beam : note) }))
}

// ---------- idle pastimes ----------
function drawPhone(g, cx, by, t) {
  rect(g, cx + 1, by - 7, 4, 6, '#2b2a28')
  rect(g, cx + 2, by - 6, 2, 3, Math.floor(t / 700) % 2 ? '#9fd3f0' : '#c4e6f8')
}
function drawConsole(g, cx, by, t) {
  rect(g, cx - 5, by - 6, 10, 5, '#5a5e66')
  rect(g, cx - 2, by - 5, 4, 3, Math.floor(t / 250) % 2 ? '#7ccf6b' : '#5fb04f')
  rect(g, cx - 4, by - 4, 1, 1, RED); rect(g, cx + 3, by - 5, 1, 1, '#4a90d9'); rect(g, cx + 3, by - 3, 1, 1, YELLOW)
}
function drawZ(g, x, y, c) { rect(g, x, y, 3, 1, c); rect(g, x + 1, y + 1, 1, 1, c); rect(g, x, y + 2, 3, 1, c) }
function drawThinking(g, cx, top, t) {
  const n = Math.floor(t / 350) % 4
  for (let i = 0; i < n; i++) rect(g, cx - 4 + i * 3, top - 5, 2, 2, '#fffdf7')
}
function drawAlert(g, x, y) {
  rect(g, x, y, 7, 9, '#2b2a28'); rect(g, x + 1, y + 1, 5, 7, '#fffdf7')
  rect(g, x + 3, y + 2, 1, 3, RED); rect(g, x + 3, y + 6, 1, 1, RED)
  rect(g, x + 1, y + 9, 2, 1, '#2b2a28')
}
function drawEnvelope(g, x, y) {
  rect(g, x, y, 9, 6, '#2b2a28'); rect(g, x + 1, y + 1, 7, 4, '#fffdf7')
  rect(g, x + 2, y + 2, 1, 1, '#b9b3a8'); rect(g, x + 3, y + 3, 3, 1, '#b9b3a8'); rect(g, x + 6, y + 2, 1, 1, '#b9b3a8')
  rect(g, x + 4, y + 2, 1, 1, RED)
}
// Zs floating up from the right of the head, clear of the name tag
function drawZzz(g, x, top, b, t) {
  for (let i = 0; i < 3; i++) {
    const p = ((t / 900) + i / 3) % 1
    drawZ(g, x + (b.bw >> 1) + 3 + Math.round(p * 6), top + 2 - Math.round(p * 10), `rgba(255,253,247,${(1 - p).toFixed(2)})`)
  }
}
// the right hand of a character standing at cx, by: where it holds a mug, a can or a cup
const handOf = (cx, by, b, armR = 0) => ({ x:cx - (b.bw >> 1) + b.bw + b.armW, y:by - b.bh + headRows(b) + 1 + armR })
function drawMug(g, h, t) {
  rect(g, h.x, h.y - 2, 3, 4, '#fbfbf8'); rect(g, h.x + 3, h.y - 1, 1, 2, '#fbfbf8'); rect(g, h.x, h.y - 2, 3, 1, '#6e5138')
  for (let i = 0; i < 2; i++) { // steam
    const p = ((t / 1300) + i / 2) % 1
    rect(g, h.x + i, h.y - 4 - Math.round(p * 6), 1, 1, `rgba(255,253,247,${(0.9 * (1 - p)).toFixed(2)})`)
  }
}
function drawCan(g, h) { rect(g, h.x, h.y - 2, 2, 4, RED); rect(g, h.x, h.y - 2, 2, 1, '#d9d5cc') }
function drawCup(g, h) { rect(g, h.x, h.y - 1, 3, 3, '#fbfbf8'); rect(g, h.x, h.y - 1, 3, 1, '#9cc7e0') }
function drawBook(g, cx, by, b) { // held open in front of the body
  const y = by - b.bh + Math.floor(b.bh / 2)
  rect(g, cx - 5, y, 10, 5, '#a65d4e'); rect(g, cx - 4, y, 8, 4, '#fbfbf8'); rect(g, cx, y, 1, 4, '#b9b3a8')
}
// a small speech bubble with an icon; x, y is the bottom left corner
const ICONS = {
  '!': ['..X..', '..X..', '..X..', '.....', '..X..'],
  '?': ['.XXX.', '....X', '..XX.', '.....', '..X..'],
  '♥': ['.X.X.', 'XXXXX', 'XXXXX', '.XXX.', '..X..'],
  '♪': ['..XX.', '..X.X', '..X..', 'XXX..', 'XX...'],
  '…': ['.....', '.....', '.....', '.....', 'X.X.X'],
}
const ICON_COLOR = { '!':RED, '?':'#4a90d9', '♥':RED, '♪':'#2b2a28', '…':'#6f6b64' }
function drawSay(g, x, y, icon) {
  rect(g, x, y - 9, 9, 9, '#2b2a28'); rect(g, x + 1, y - 8, 7, 7, '#fffdf7'); rect(g, x + 1, y, 2, 2, '#2b2a28')
  ICONS[icon].forEach((row, r) => [...row].forEach((c, k) => { if (c === 'X') rect(g, x + 2 + k, y - 7 + r, 1, 1, ICON_COLOR[icon]) }))
}
// the burst where a tag lands; p runs from 0 to 1
function drawPow(g, x, y, p) {
  const r = 2 + Math.round(p * 4), c = p < 0.5 ? YELLOW : '#fffdf7'
  rect(g, x - r, y, 2 * r + 1, 1, c); rect(g, x, y - r, 1, 2 * r + 1, c)
  for (const [dx, dy] of [[-1, -1], [1, -1], [-1, 1], [1, 1]]) rect(g, x + dx * (r - 1), y + dy * (r - 1), 1, 1, c)
}
// a blinking ring around the feet of an employee the search found
function drawRing(g, x, y, b, t) {
  const c = Math.floor(t / 250) % 2 ? YELLOW : '#fffdf7', hw = (b.bw >> 1) + b.armW + 3
  solid(g, x - hw + 2, y - 1, 2 * hw - 4, 1, c); solid(g, x - hw + 2, y + 3, 2 * hw - 4, 1, c)
  solid(g, x - hw, y, 2, 3, c); solid(g, x + hw - 2, y, 2, 3, c)
}

// ---------- furniture and decor ----------
function drawChair(g, x, y) {
  rect(g, x + 9, y - 15, 14, 13, '#5d6169')
  rect(g, x + 9, y - 15, 14, 1, '#7a7f89')
  if (detail >= 3 && hair) rect(g, x + 11, y - 8, 10, 1, '#666b74') // a seam in the backrest
  if (detail >= 2 && hair) { solid(g, x + 9, y - 15, hair, hair, '#cdd4dc'); solid(g, x + 23 - hair, y - 15, hair, hair, '#cdd4dc') }
}
function drawDesk(g, x, y, light) {
  rect(g, x, y, 32, 8, '#c9a477'); rect(g, x, y, 32, 1, '#dcbb90')
  rect(g, x, y + 8, 32, 6, '#a77d53'); rect(g, x, y + 13, 32, 1, '#87623f')
  rect(g, x + 1, y + 14, 2, 2, '#87623f'); rect(g, x + 29, y + 14, 2, 2, '#87623f')
  if (detail >= 3 && hair) { // wood grain, a drawer
    rect(g, x + 2, y + 3, 7, 1, '#bf9a6c'); rect(g, x + 22, y + 5, 8, 1, '#bf9a6c'); rect(g, x + 4, y + 6, 5, 1, '#d4b083')
    rect(g, x + 13, y + 10, 6, 1, '#87623f')
  }
  if (detail >= 2 && hair) { // rounded corners, on the carpet
    for (const [cx, cy] of [[x, y], [x + 32 - hair, y], [x, y + 14 - hair], [x + 32 - hair, y + 14 - hair]]) solid(g, cx, cy, hair, hair, '#cdd4dc')
  }
  // laptop: the screen faces the employee, the back faces us, and the light on the back shows the state color
  rect(g, x + 11, y, 10, 5, '#565a62'); rect(g, x + 11, y, 10, 1, '#6e737c')
  rect(g, x + 10, y + 5, 12, 1, '#3b3e44'); rect(g, x + 15, y + 2, 2, 2, light)
  if (detail >= 3 && hair) dots(g, x + 12, y + 4, 4, 2 * hair, -hair, '#6e737c') // a glint on the lid
  ground(g, x, y + 16, 32)
}
function drawPlant(g, x, y) {
  rect(g, x + 3, y + 12, 10, 2, '#c27b55'); rect(g, x + 4, y + 14, 8, 6, '#b06a45'); rect(g, x + 4, y + 19, 8, 1, '#8e5336')
  rect(g, x + 2, y + 4, 12, 8, '#5f9150'); rect(g, x + 4, y + 1, 8, 4, '#6fa35d')
  rect(g, x + 1, y + 7, 3, 3, '#4f7d42'); rect(g, x + 12, y + 6, 3, 4, '#4f7d42'); rect(g, x + 6, y + 2, 2, 2, '#86b873')
  if (detail >= 3 && hair) { dots(g, x + 4, y + 6, 3, hair, hair, '#9fd08a'); dots(g, x + 10, y + 4, 3, hair, hair, '#9fd08a'); dots(g, x + 7, y + 9, 2, hair, hair, '#9fd08a') }
  ground(g, x + 4, y + 20, 8)
}
function drawCooler(g, x, y) {
  rect(g, x + 2, y, 8, 9, '#9cc7e0'); rect(g, x + 3, y + 1, 2, 6, '#c3e0ef')
  rect(g, x, y + 9, 12, 19, '#e9e6df'); rect(g, x, y + 9, 12, 1, '#c9c3b8'); rect(g, x + 11, y + 10, 1, 18, '#c9c3b8')
  rect(g, x + 5, y + 13, 2, 2, '#5a8bb5'); rect(g, x + 2, y + 22, 8, 1, '#c9c3b8')
  ground(g, x, y + 28, 12)
}
function drawBookshelf(g, x, y) {
  box(g, x, y, 32, 32, '#8a6747'); rect(g, x + 2, y + 2, 28, 28, '#6e5138')
  const books = ['#a65d4e', '#5d7da6', '#6f9a63', '#c9a24e', '#8a6fa6', '#b5ada0']
  for (let shelf = 0; shelf < 3; shelf++) {
    const sy = y + 3 + shelf * 9
    let bx = x + 3, k = shelf * 2
    while (bx < x + 28) {
      const w = 2 + ((k * 7) % 3), h = 6 + ((k * 5) % 2)
      if (bx + w > x + 29) break
      rect(g, bx, sy + (7 - h), w, h, books[k % books.length])
      if (detail >= 3 && hair) { solid(g, bx, sy + (7 - h) + 1, w, hair, 'rgba(255,255,255,.3)'); solid(g, bx, sy + 5, w, hair, 'rgba(0,0,0,.18)') } // bands on the spine
      bx += w + 1; k++
    }
    rect(g, x + 2, sy + 7, 28, 2, '#8a6747')
  }
  ground(g, x, y + 32, 32)
}
function drawWindow(g, x, y) {
  box(g, x, y, 30, 22, '#8f8676'); rect(g, x + 2, y + 2, 12, 18, '#c6dfec'); rect(g, x + 16, y + 2, 12, 18, '#c6dfec')
  rect(g, x + 4, y + 4, 2, 6, '#e6f2f8'); rect(g, x + 18, y + 4, 2, 6, '#e6f2f8'); rect(g, x - 1, y + 22, 32, 2, '#b1a792')
  if (detail >= 3 && hair) { // the sky lighter at the top, a reflection across each pane
    rect(g, x + 2, y + 2, 12, 1, '#d9ebf4'); rect(g, x + 16, y + 2, 12, 1, '#d9ebf4')
    dots(g, x + 3, y + 18, 9, hair, -hair, '#eef6fa'); dots(g, x + 17, y + 18, 9, hair, -hair, '#eef6fa')
  }
}
function drawWhiteboard(g, x, y) {
  box(g, x, y, 42, 24, '#b9b3a8'); rect(g, x + 2, y + 2, 38, 19, '#fbfbf8')
  rect(g, x + 5, y + 6, 18, 1, '#5d7da6'); rect(g, x + 5, y + 10, 26, 1, '#5d7da6'); rect(g, x + 5, y + 14, 12, 1, '#a65d4e')
  rect(g, x + 4, y + 21, 34, 2, '#9b958a')
}
function drawClock(g, x, y) {
  rect(g, x + 2, y, 6, 10, '#2b2a28'); rect(g, x, y + 2, 10, 6, '#2b2a28')
  rect(g, x + 2, y + 1, 6, 8, '#fbfbf8'); rect(g, x + 1, y + 2, 8, 6, '#fbfbf8')
  rect(g, x + 4, y + 2, 1, 3, '#2b2a28'); rect(g, x + 5, y + 4, 2, 1, '#2b2a28')
}

// ---------- the trappings of a big company ----------
function drawTV(g, x, y) {
  box(g, x, y, 48, 28, '#2b2a28'); rect(g, x + 2, y + 2, 44, 22, '#2d4b6b')
  rect(g, x + 6, y + 15, 4, 7, '#7cc4e8'); rect(g, x + 12, y + 11, 4, 11, YELLOW); rect(g, x + 18, y + 7, 4, 15, '#7ccf6b')
  rect(g, x + 26, y + 6, 16, 1, '#c4e6f8'); rect(g, x + 26, y + 9, 12, 1, '#c4e6f8'); rect(g, x + 26, y + 12, 14, 1, '#c4e6f8')
  rect(g, x + 20, y + 28, 8, 2, '#55534f')
}
function drawSeatTop(g, x, y, c) { rect(g, x, y, 10, 8, c); rect(g, x, y, 10, 2, '#7a7f89') } // a chair seen from above
function drawMeetingTable(g, x, y, w, h) {
  ground(g, x, y + h, w)
  for (let i = 0; i < 4; i++) { drawSeatTop(g, x + 10 + i * 26, y - 9, '#5d6169'); drawSeatTop(g, x + 10 + i * 26, y + h + 1, '#5d6169') }
  drawSeatTop(g, x - 11, y + 11, '#5d6169'); drawSeatTop(g, x + w + 1, y + 11, '#5d6169')
  box(g, x, y, w, h, '#8a6747'); rect(g, x, y, w, 2, '#a5815e'); rect(g, x, y + h - 3, w, 3, '#6e5138')
  if (detail >= 3 && hair) { rect(g, x + 30, y + 5, 16, 1, '#7b5b3d'); rect(g, x + 66, y + 9, 22, 1, '#7b5b3d'); rect(g, x + 8, y + 20, 14, 1, '#7b5b3d') } // grain
  rect(g, x + 14, y + 8, 12, 9, '#fbfbf8'); rect(g, x + 50, y + 10, 10, 6, '#565a62'); rect(g, x + 84, y + 9, 4, 4, '#fbfbf8')
}
function drawGlassV(g, x, y, h, gapY, gapH) { // with a doorway from gapY down
  for (const [sy, sh] of [[y, gapY - y], [gapY + gapH, y + h - gapY - gapH]]) {
    rect(g, x, sy, 4, sh, '#cfe3ee'); rect(g, x, sy, 1, sh, '#7d8a94'); rect(g, x + 3, sy, 1, sh, '#7d8a94')
  }
  rect(g, x - 1, gapY - 2, 6, 2, '#7d8a94'); rect(g, x - 1, gapY + gapH, 6, 2, '#7d8a94')
}
function drawGlassH(g, x, y, w) {
  rect(g, x, y, w, 10, '#d7e8f1'); rect(g, x, y, w, 1, '#7d8a94'); rect(g, x, y + 9, w, 1, '#7d8a94')
  for (let mx = x; mx < x + w; mx += 24) rect(g, mx, y, 1, 10, '#7d8a94')
  rect(g, x + 3, y + 2, 6, 1, '#f2f8fb')
  if (detail >= 3 && hair) for (let mx = x + 8; mx + 8 < x + w; mx += 48) dots(g, mx, y + 8, 6, hair, -hair, '#f2f8fb') // reflections
}
function drawCounter(g, x, y, w, h) {
  rect(g, x, y, w, h, '#d6d1c7'); rect(g, x, y, 2, h, '#b9b3a8'); rect(g, x + w - 1, y, 1, h, '#a9a49b')
}
function drawSink(g, x, y) { rect(g, x, y, 14, 10, '#9aa7b1'); rect(g, x + 2, y + 2, 10, 6, '#7d8a94'); rect(g, x + 6, y, 2, 3, '#c9cfd4') }
function drawCoffeeMachine(g, x, y) {
  rect(g, x, y, 12, 13, '#2b2a28'); rect(g, x + 2, y + 2, 8, 3, '#55534f'); rect(g, x + 8, y + 6, 2, 2, RED)
  rect(g, x + 3, y + 9, 4, 3, '#fbfbf8')
}
function drawMicrowave(g, x, y) { rect(g, x, y, 14, 10, '#b9b3a8'); rect(g, x + 2, y + 2, 8, 6, '#2b2a28'); rect(g, x + 11, y + 3, 2, 4, '#55534f') }
function drawFridge(g, x, y) {
  box(g, x, y, 20, 32, '#f1efe9'); rect(g, x, y, 20, 1, '#c9c3b8'); rect(g, x + 19, y, 1, 32, '#c9c3b8')
  rect(g, x, y + 12, 20, 1, '#c9c3b8'); rect(g, x + 2, y + 4, 1, 6, '#9b958a'); rect(g, x + 2, y + 16, 1, 10, '#9b958a')
  ground(g, x, y + 32, 20)
}
function drawVending(g, x, y) {
  box(g, x, y, 22, 32, '#b5372e'); rect(g, x + 2, y + 3, 13, 22, '#2b2a28')
  const cans = ['#f2b705', '#4a90d9', '#7ccf6b', '#fbfbf8', RED]
  for (let r = 0; r < 4; r++) for (let c = 0; c < 3; c++) rect(g, x + 4 + c * 4, y + 5 + r * 5, 2, 3, cans[(r + c) % cans.length])
  rect(g, x + 17, y + 6, 3, 6, '#d9d5cc'); rect(g, x + 4, y + 27, 9, 3, '#2b2a28')
  if (detail >= 3 && hair) dots(g, x + 3, y + 23, 10, hair, -2 * hair, 'rgba(255,255,255,.3)') // a reflection on the glass
  ground(g, x, y + 32, 22)
}
function drawSofa(g, x, y, w, c, dark) { // facing north, backrest at the bottom
  rect(g, x - 3, y + 1, 3, 13, dark); rect(g, x + w, y + 1, 3, 13, dark)
  rect(g, x, y, w, 9, c); rect(g, x, y + 9, w, 5, dark)
  for (let cx = x + Math.floor(w / 3); cx < x + w - 2; cx += Math.floor(w / 3)) rect(g, cx, y + 1, 1, 7, dark)
  if (detail >= 3 && hair) solid(g, x, y + hair, w, hair, 'rgba(255,255,255,.22)') // light on the cushions
  ground(g, x - 3, y + 14, w + 6)
}
// the backrest again, over whoever sits on the sofa: they face away from us, so it hides their lower half
function drawSofaBack(g, s) { rect(g, s.x, s.y + 9, s.w, 5, s.dark) }
function drawRug(g, x, y, w, h, c1, c2) { rect(g, x, y, w, h, c1); rect(g, x + 3, y + 3, w - 6, h - 6, c2); rect(g, x + 5, y + 5, w - 10, h - 10, c1) }
function drawLoungeTV(g, x, y) { // on a low cabinet, the screen facing the sofa (and us)
  rect(g, x - 4, y + 20, 44, 6, '#6e5138'); rect(g, x - 4, y + 20, 44, 1, '#8a6747')
  rect(g, x + 2, y + 23, 14, 1, '#55402c'); rect(g, x + 20, y + 23, 14, 1, '#55402c')
  rect(g, x + 15, y + 18, 6, 2, '#2b2a28')
  box(g, x, y, 36, 19, '#2b2a28'); rect(g, x + 2, y + 2, 32, 15, '#1f2a33'); rect(g, x + 4, y + 3, 6, 1, '#34475a')
  ground(g, x - 4, y + 26, 44)
}
// what's on while someone watches: football, the countryside, a music show; the screen is 32x15 at x+2, y+2
function drawTVShow(g, x, y, t) {
  const sx = x + 2, sy = y + 2, tri = (period, span) => Math.round(Math.abs(((t / period) % 2) - 1) * span)
  switch (Math.floor(t / 7000) % 3) {
    case 0:
      rect(g, sx, sy, 32, 15, '#4f9a52'); rect(g, sx + 16, sy, 1, 15, '#e9f3e6'); rect(g, sx, sy + 5, 2, 5, '#e9f3e6'); rect(g, sx + 30, sy + 5, 2, 5, '#e9f3e6')
      rect(g, sx + 4 + tri(1500, 22), sy + 2 + tri(1100, 9), 2, 2, '#fbfbf8')
      rect(g, sx + 7 + tri(2300, 8), sy + 4 + tri(1700, 7), 2, 3, RED); rect(g, sx + 19 + tri(1900, 8), sy + 3 + tri(2100, 8), 2, 3, '#4a90d9')
      break
    case 1:
      rect(g, sx, sy, 32, 15, '#8fc3e8'); rect(g, sx + 24, sy + 2, 4, 4, YELLOW)
      const cloud = sx + ((t / 200) % 40 | 0) - 8 // drifting across, cut at the screen's edges
      rect(g, Math.max(cloud, sx), sy + 3, Math.min(cloud + 8, sx + 32) - Math.max(cloud, sx), 2, '#fbfbf8')
      rect(g, sx, sy + 10, 32, 5, '#6f9a63'); rect(g, sx + 4, sy + 8, 10, 2, '#6f9a63'); rect(g, sx + 18, sy + 9, 8, 1, '#5f8a53')
      break
    default:
      rect(g, sx, sy, 32, 15, '#2d4b6b')
      for (let i = 0; i < 4; i++) rect(g, sx + 3 + i * 8, sy + 2 + (i % 2) * 3, 1, 1, (Math.floor(t / 400) + i) % 2 ? '#fbfbf8' : '#2d4b6b')
      drawMark(g, sx + 12, sy + 6 - tri(500, 4), '#fbfbf8', YELLOW) // the notes dance
  }
  rect(g, sx, sy + (Math.floor(t / 60) % 15), 32, 1, 'rgba(255,255,255,.08)') // a faint scan line
}
// the ball while two play: it bounces around the field, x, y is the table's top left
function drawFoosBall(g, x, y, t) {
  const tri = (period, span) => Math.round(Math.abs(((t / period) % 2) - 1) * span)
  rect(g, x + 4 + tri(900, 37), y + 4 + tri(1300, 17), 2, 2, '#fbfbf8')
}
// a sheet coming out of the printer, x, y is the printer's top left
function drawPaper(g, x, y, t) { rect(g, x + 6, y + 14, 14, 3 + (Math.floor(t / 300) % 6), '#fbfbf8') }
function drawFoosball(g, x, y) {
  ground(g, x, y + 26, 46)
  box(g, x, y, 46, 26, '#6e5138'); rect(g, x + 3, y + 3, 40, 20, '#4f9a52')
  if (detail >= 3 && hair) for (let sx = x + 3; sx < x + 43; sx += 5) solid(g, sx, y + 3, hair, 20, '#55a058') // the pitch's stripes
  rect(g, x + 22, y + 3, 1, 20, '#e9f3e6'); rect(g, x + 3, y + 10, 2, 6, '#e9f3e6'); rect(g, x + 41, y + 10, 2, 6, '#e9f3e6')
  for (let i = 0; i < 6; i++) {
    const rx = x + 7 + i * 6.5 | 0
    rect(g, rx, y - 2, 1, 30, '#c9cfd4')
    for (let p = 0; p < 2; p++) rect(g, rx - 1, y + 7 + p * 9, 3, 3, i % 2 ? '#4a90d9' : RED)
  }
}
function drawBeanBag(g, x, y, c, dark) { // 14 x 10
  ground(g, x, y + 10, 14)
  rect(g, x + 2, y, 10, 2, c); rect(g, x, y + 2, 14, 6, c); rect(g, x + 1, y + 8, 12, 2, dark); rect(g, x + 3, y + 2, 4, 2, 'rgba(255,255,255,.35)')
}
function drawFloorLamp(g, x, y) { rect(g, x, y, 8, 6, '#f2e3b3'); rect(g, x, y + 5, 8, 1, '#c9b98a'); rect(g, x + 3, y + 6, 2, 18, '#55534f'); rect(g, x + 1, y + 24, 6, 2, '#55534f') }
function drawRoundTable(g, x, y) {
  ground(g, x, y + 12, 16)
  rect(g, x + 2, y, 12, 12, '#efece6'); rect(g, x, y + 2, 16, 8, '#efece6'); rect(g, x + 2, y + 11, 12, 1, '#c9c3b8')
  rect(g, x + 6, y + 4, 3, 3, '#5d7da6')
  for (const [sx, sy] of [[-7, 3], [19, 3], [5, 15]]) { rect(g, x + sx, y + sy, 6, 6, '#c27b55'); rect(g, x + sx, y + sy, 6, 1, '#d99a77') }
}
// a lift in the back wall, its bottom on the floor: closed, or open on the car
function drawLift(g, x, y, open) {
  rect(g, x, y, 22, 34, '#8f949a'); rect(g, x + 1, y + 1, 20, 1, '#b9bec4')
  rect(g, x + 7, y + 1, 8, 4, '#2b2a28'); rect(g, x + 9, y + 2, 4, 2, YELLOW) // the floor display
  if (open) { rect(g, x + 2, y + 6, 18, 28, '#4a4d52'); rect(g, x + 4, y + 8, 14, 2, '#f2e3b3'); rect(g, x + 2, y + 6, 2, 28, '#c9cfd4'); rect(g, x + 18, y + 6, 2, 28, '#c9cfd4') }
  else { rect(g, x + 2, y + 6, 18, 28, '#c9cfd4'); rect(g, x + 10, y + 6, 2, 28, '#9aa1a8'); rect(g, x + 3, y + 7, 6, 1, '#e3e7ea'); rect(g, x + 13, y + 7, 6, 1, '#e3e7ea') }
}
function drawLiftButtons(g, x, y) { rect(g, x, y, 4, 9, '#8f949a'); rect(g, x + 1, y + 1, 2, 2, '#e3e7ea'); rect(g, x + 1, y + 5, 2, 2, '#7ccf6b') }
function drawExitSign(g, x, y) { // the stairs: a green sign with a running man
  rect(g, x, y, 14, 8, '#2e9e5b'); rect(g, x + 1, y + 1, 12, 6, '#3fb46c')
  rect(g, x + 5, y + 2, 2, 2, '#fbfbf8'); rect(g, x + 4, y + 4, 3, 1, '#fbfbf8'); rect(g, x + 3, y + 5, 1, 1, '#fbfbf8'); rect(g, x + 7, y + 5, 1, 1, '#fbfbf8')
  rect(g, x + 9, y + 3, 3, 3, '#fbfbf8'); rect(g, x + 10, y + 4, 1, 1, '#3fb46c')
}
function drawPrinter(g, x, y) {
  box(g, x, y, 26, 20, '#d9d5cc'); rect(g, x, y, 26, 4, '#efece6'); rect(g, x + 3, y + 7, 9, 4, '#55534f')
  rect(g, x + 16, y + 7, 2, 2, GREEN); rect(g, x + 6, y + 14, 14, 3, '#fbfbf8')
  if (detail >= 3 && hair) dots(g, x + 19, y + 8, 3, 2 * hair, 0, '#9b958a') // buttons
  ground(g, x, y + 20, 26)
}
function drawCabinet(g, x, y) {
  box(g, x, y, 14, 26, '#9da3a8'); rect(g, x + 13, y, 1, 26, '#7d8389')
  for (let d = 0; d < 3; d++) { rect(g, x + 1, y + 8 + d * 8, 12, 1, '#7d8389'); rect(g, x + 5, y + 3 + d * 8, 4, 1, '#55534f') }
  ground(g, x, y + 26, 14)
}
function drawPainting(g, x, y, w, h, colors) {
  box(g, x, y, w, h, '#6e5138'); rect(g, x + 2, y + 2, w - 4, h - 4, colors[0])
  rect(g, x + 2, y + h - 8, w - 4, 6, colors[1]); rect(g, x + w - 8, y + 4, 3, 3, colors[2])
}
function drawSign(g, x, y) {
  box(g, x, y, 100, 20, '#2b2a28'); rect(g, x + 1, y + 1, 98, 1, '#55534f')
  drawMark(g, x + 7, y + 6, '#fffdf7', YELLOW)
}
function drawBigPlant(g, x, y) {
  rect(g, x + 4, y + 20, 10, 10, '#d9d5cc'); rect(g, x + 4, y + 20, 10, 2, '#efece6')
  rect(g, x + 2, y + 2, 14, 18, '#4f7d42'); rect(g, x + 5, y - 3, 8, 8, '#5f9150'); rect(g, x, y + 8, 4, 6, '#5f9150')
  rect(g, x + 14, y + 6, 4, 7, '#5f9150'); rect(g, x + 7, y + 4, 3, 3, '#86b873'); rect(g, x + 3, y + 12, 3, 2, '#86b873')
  if (detail >= 3 && hair) { dots(g, x + 4, y + 6, 4, hair, hair, '#9fd08a'); dots(g, x + 11, y + 10, 4, hair, hair, '#9fd08a'); dots(g, x + 7, y - 1, 3, hair, hair, '#9fd08a') }
  ground(g, x + 4, y + 30, 10)
}
function drawExtinguisher(g, x, y) { rect(g, x, y, 5, 10, RED); rect(g, x + 1, y - 2, 3, 2, '#2b2a28'); rect(g, x, y + 3, 5, 1, '#fbfbf8') }
// ---------- small meeting rooms ----------
// a glass wall like drawGlassH, with a doorway from gapX
function drawGlassHDoor(g, x, y, w, gapX, gapW) {
  drawGlassH(g, x, y, gapX - x); drawGlassH(g, gapX + gapW, y, x + w - gapX - gapW)
  rect(g, gapX - 2, y - 1, 2, 12, '#7d8a94'); rect(g, gapX + gapW, y - 1, 2, 12, '#7d8a94')
}
function drawPartition(g, x, y, h) { rect(g, x, y, 4, h, '#b9ad98'); rect(g, x, y, 1, h, '#cfc4b0') }
function drawWallH(g, x, y, w) { rect(g, x, y, w, 4, '#b9ad98'); rect(g, x, y, w, 1, '#cfc4b0') }
function drawWallScreen(g, x, y) {
  box(g, x, y, 34, 22, '#2b2a28'); rect(g, x + 2, y + 2, 30, 16, '#2d4b6b')
  rect(g, x + 5, y + 5, 12, 1, '#c4e6f8'); rect(g, x + 5, y + 8, 18, 1, '#c4e6f8'); rect(g, x + 5, y + 11, 9, 1, '#c4e6f8')
  rect(g, x + 22, y + 10, 6, 6, '#7ccf6b'); rect(g, x + 14, y + 22, 6, 2, '#55534f')
}
// a table around (cx, cy) for 3 (round: a chair on the far side and one at each end), 4 or 5 (two chairs a side, the
// fifth at the east end); far is -1 when the room's far wall is north of the table, 1 when it's south
// The chairs in the order a task's members take them: for 5 the head of the table first, then the far side, then the
// near side; x, y is a chair's top left (drawSeatTop), side which side of the table it stands on (n, s, w or e)
function roomChairs(cx, cy, seats, far) {
  if (seats === 3) return [{ x:cx - 5, y:far < 0 ? cy - 16 : cy + 8, side:far < 0 ? 'n' : 's' }, { x:cx - 23, y:cy - 4, side:'w' }, { x:cx + 13, y:cy - 4, side:'e' }]
  const w = seats === 4 ? 28 : 40, x = cx - (w >> 1), y = cy - 8, xs = [0.25, 0.75].map(f => Math.round(x + f * w) - 5)
  const north = xs.map(sx => ({ x:sx, y:y - 9, side:'n' })), south = xs.map(sx => ({ x:sx, y:y + 17, side:'s' }))
  return [...(seats === 5 ? [{ x:x + w + 1, y:y + 4, side:'e' }] : []), ...(far < 0 ? [...north, ...south] : [...south, ...north])]
}
function drawRoomTable(g, cx, cy, seats, far) {
  const chairs = () => roomChairs(cx, cy, seats, far).forEach(c => drawSeatTop(g, c.x, c.y, '#5d6169'))
  if (seats === 3) {
    chairs()
    rect(g, cx - 7, cy - 7, 14, 14, '#8a6747'); rect(g, cx - 10, cy - 4, 20, 8, '#8a6747')
    rect(g, cx - 7, cy - 7, 14, 2, '#a5815e'); rect(g, cx - 10, cy - 4, 3, 1, '#a5815e'); rect(g, cx + 7, cy - 4, 3, 1, '#a5815e')
    rect(g, cx - 7, cy + 5, 14, 2, '#6e5138'); rect(g, cx - 10, cy + 3, 3, 1, '#6e5138'); rect(g, cx + 7, cy + 3, 3, 1, '#6e5138')
    rect(g, cx - 3, cy - 2, 6, 4, '#fbfbf8') // papers
    return
  }
  const w = seats === 4 ? 28 : 40, x = cx - (w >> 1), y = cy - 8
  ground(g, x, y + 16, w)
  chairs()
  box(g, x, y, w, 16, '#8a6747'); rect(g, x, y, w, 2, '#a5815e'); rect(g, x, y + 13, w, 3, '#6e5138')
  rect(g, cx - 3, y + 5, 6, 4, '#3b3e44'); rect(g, cx - 1, y + 6, 2, 1, '#7ccf6b') // a conference phone
  rect(g, x + 3, y + 4, 7, 5, '#fbfbf8') // papers
}
const SMALL_CARPETS = [['#c9d3c4', '#c2ccbd'], ['#d8cdbf', '#d1c5b6'], ['#c8cfdb', '#c1c8d5'], ['#d5c8d2', '#cec0cb']]
// one small meeting room for 3 to 5: carpet, the table in the middle, a plant in a far corner, a whiteboard on the far
// wall (the first row's hang on the back wall) or a screen on a low cabinet; the glass wall with the door faces the
// hall, and the nameplate beside the door is a label (placeLabels)
function drawSmallRoom(g, r) {
  const [c1, c2] = SMALL_CARPETS[r.k % SMALL_CARPETS.length], cx = r.x + (r.w >> 1), cy = r.y + (r.h >> 1)
  rect(g, r.x, r.y, r.w, r.h, c1)
  for (let y = r.y + 2; y < r.y + r.h; y += 4) rect(g, r.x, y, r.w, 1, c2)
  if (detail >= 3 && hair) weave(g, r.x, r.y, r.w, r.h, c2)
  if (r.north && r.y !== WALL_H) { rect(g, cx - 14, r.y, 28, 5, '#b9b3a8'); rect(g, cx - 13, r.y, 26, 3, '#fbfbf8'); rect(g, cx - 10, r.y + 1, 9, 1, '#5d7da6') }
  if (!r.north) { rect(g, cx - 12, r.y + r.h - 7, 24, 6, '#6e5138'); rect(g, cx - 10, r.y + r.h - 9, 20, 3, '#2b2a28') }
  drawRoomTable(g, cx, cy, r.seats, r.north ? -1 : 1)
  drawPlant(g, r.x + r.w - 20, r.north ? r.y + 4 : r.y + r.h - 24)
  const door = smallDoor(r)
  drawGlassHDoor(g, r.x, r.north ? r.y + r.h : r.y - GLASS, r.w, door.x, door.w)
}
// the doorway in a small room's glass wall, and where its nameplate goes
const smallDoor = r => ({ x:r.x + 8, w:20, plateX:r.x + 32, plateY:(r.north ? r.y + r.h : r.y - GLASS) + GLASS / 2 })
// one small item per desk, fixed by seat number, so empty desks don't look bare
function drawClutter(g, x, y, seat) {
  switch (hash('seat' + seat) % 6) {
    case 0: rect(g, x + 25, y + 1, 5, 4, '#b06a45'); rect(g, x + 24, y - 3, 7, 5, '#5f9150'); break // small potted plant
    case 1: rect(g, x + 4, y + 2, 4, 4, '#fbfbf8'); rect(g, x + 8, y + 3, 1, 2, '#fbfbf8'); break // mug
    case 2: rect(g, x + 2, y + 1, 8, 6, '#fbfbf8'); rect(g, x + 3, y + 2, 6, 1, '#b9b3a8'); rect(g, x + 3, y + 4, 5, 1, '#b9b3a8'); break // papers
    case 3: rect(g, x + 25, y - 2, 6, 7, '#6e5138'); rect(g, x + 26, y - 1, 4, 4, '#9fd3f0'); break // photo frame
    case 4: rect(g, x + 26, y - 6, 1, 8, '#55534f'); rect(g, x + 24, y - 7, 5, 2, YELLOW); break // desk lamp
    default: break
  }
}

// ================= layout: one floor of a big company's building =================
// The open office in the middle: rows of 6 desks, at least 3 rows (a small team still gets full rows of desks, a big one
// more rows below), desks fixed by seat number. Left of it, small meeting rooms in rows on both sides of halls off the
// corridor beside the office; right of it, the meeting room and the lounge. People come and go by the lifts in the back
// wall; the printer and a bookshelf stand in the strip below the desks.
// The building fills the window. The scale is the biggest at which the smallest building for the team fits (in whole
// device pixels from 2 up, so pixels stay square). The room that leaves goes to more rows of desks (while a whole row
// fits), to both sides equally (the office stays in the middle), so to more small meeting rooms and a wider meeting room
// and lounge, and to a taller lounge, with a row of small meeting rooms below it at a time once it's tall.
const MIN_ROWS = 3
const OFFICE_W = COLS * COL_PITCH + 12           // the open office's carpet
const WCOR = 24, ECOR = 18                        // the corridors beside it, west and east
const RIGHT_MIN = 13 * T                          // the meeting room and the lounge at their narrowest
const MEET_H = 9 * T, GLASS = 10, HALL = 24      // the meeting room; a glass wall; a hall between rows of small meeting rooms
const LOUNGE_MIN = 9 * T, LOUNGE_MAX = 22 * T    // the lounge at its shortest, and as tall as it gets before rooms go below it
const ROOM_W = 88, ROOM_H = 72, ROOMS_MAX = 8    // the smallest small meeting room; at most this many side by side
const STRIP_MIN = 50                              // below the desks: the corridor, the printer and the bookshelf
const officeBottom = rows => WALL_H + AISLE + LABEL_ROOM + (rows - 1) * ROW_PITCH + 24
let W = 0, H = 0, rows = MIN_ROWS
let workX = 0, rightX = 0, rowsY = 0 // where the desks' columns start, where their first row is; where the meeting room and the lounge start
const door = { x:0, y:0 }  // where people come and go: in front of the lifts
const rooms = {}           // each room's area, for furniture and signs
let smalls = []            // the small meeting rooms: { x, y, w, h, north (door in the south wall), k (numbers them), seats }
let halls = []             // the halls between their rows: { x0, x1, y }
let walls = []             // walls between rows of rooms: { x0, x1, y }
let addSeat = -1           // the empty seat with the "+", -1 for none
let layoutKey = ''         // what the last layout was made for; the same again means nothing to do
const bgCanvas = document.createElement('canvas')
const buf = document.createElement('canvas')
const big = document.createElement('canvas') // the frame at a whole scale, for drawing it at a fractional one
const bgCtx = bgCanvas.getContext('2d'), bufCtx = buf.getContext('2d'), bigCtx = big.getContext('2d')

function deskOf(seat) {
  return { x:workX + (seat % COLS) * COL_PITCH + 8, y:rowsY + Math.floor(seat / COLS) * ROW_PITCH }
}
// an employee's desk and chair, by its seat
function seatEmployee(e) {
  const d = deskOf(e.acc.seat)
  e.desk = d
  e.seat = { x:d.x + 16, y:d.y - 2 }
}

// a row of small meeting rooms side by side from x0 to x1, a wall between each two
function roomRow(x0, x1, y, h, north) {
  const span = x1 - x0 + 4, n = Math.max(1, Math.min(ROOMS_MAX, Math.floor(span / (ROOM_W + 4))))
  return Array.from({ length:n }, (_, i) => {
    const x = x0 + Math.round(i * span / n)
    return { x, y, w:x0 + Math.round((i + 1) * span / n) - 4 - x, h, north }
  })
}

// layout lays the building out for the team and the window, and draws the background. Every snapshot calls it,
// but it only works when something it depends on changed: the seats, the "+", the window, the pixel size.
function layout() {
  const taken = new Set(snapshot.accounts.filter(a => a.seat >= 0).map(a => a.seat)) // advisors (seat -1) have no desk
  addSeat = -1
  if (launchOpts?.enabled) { addSeat = 0; while (taken.has(addSeat)) addSeat++ }
  const made = `${[...taken].sort((a, b) => a - b).join(',')}|${addSeat}|${innerWidth}x${innerHeight}@${devicePixelRatio}|${detail}`
  if (made === layoutKey) return
  layoutKey = made
  const seats = Math.max(addSeat + 1, ...[...taken].map(s => s + 1), 1)
  const team = Math.max(MIN_ROWS, Math.ceil(seats / COLS)), sideMin = ECOR + RIGHT_MIN + 4
  fitScreen(OFFICE_W + 2 * sideMin, Math.max(officeBottom(team) + STRIP_MIN + 6, WALL_H + MEET_H + GLASS + LOUNGE_MIN + 6))
  rows = Math.max(team, Math.floor((H - 6 - STRIP_MIN - officeBottom(1)) / ROW_PITCH) + 1)
  const workBottom = H - 6 - STRIP_MIN, side = Math.floor((W - OFFICE_W) / 2)
  workX = side + 6; rightX = side + OFFICE_W + ECOR
  rowsY = WALL_H + AISLE + LABEL_ROOM + ((workBottom - officeBottom(rows)) >> 1) // what's left over above and below the rows
  door.x = workX - 12; door.y = WALL_H + 10
  rooms.work = { x:side, y:WALL_H + AISLE + 6, w:OFFICE_W, h:workBottom - (WALL_H + AISLE + 6) }
  rooms.meeting = { x:rightX, y:WALL_H, w:W - 4 - rightX, h:MEET_H }
  // the lounge; once it's tall, a row of small meeting rooms below it at a time, each along a hall off the corridor
  const below = H - 6 - (WALL_H + MEET_H + GLASS), band = 4 + HALL + GLASS + ROOM_H
  const more = below > LOUNGE_MAX ? Math.min(Math.ceil((below - LOUNGE_MAX) / band), Math.floor((below - LOUNGE_MIN) / band)) : 0
  rooms.lounge = { x:rightX, y:WALL_H + MEET_H + GLASS, w:W - 4 - rightX, h:below - more * band }
  smalls = []; halls = []; walls = []
  for (let i = 0, y = rooms.lounge.y + rooms.lounge.h; i < more; i++, y += band) {
    walls.push({ x0:rightX, x1:W - 4, y })
    halls.push({ x0:rightX, x1:W - 4, y:y + 4 })
    smalls.push(...roomRow(rightX + 4, W - 4, y + 4 + HALL + GLASS, ROOM_H, false))
  }
  // left of the office: as many rows of small meeting rooms as fit, two to a hall (doors on the hall); the last one
  // takes what's left over
  const x0 = 4, x1 = side - WCOR - 4, avail = H - 6 - WALL_H
  const need = r => r * (ROOM_H + GLASS) + Math.ceil(r / 2) * (HALL + 4) - 4
  let r = 1
  while (need(r + 1) <= avail) r++
  const h = Math.floor((avail - need(r) + r * ROOM_H) / r), left = []
  for (let i = 0, y = WALL_H; i < r; i += 2) {
    left.push(...roomRow(x0, x1, y, i + 1 === r ? H - 6 - HALL - GLASS - y : h, true))
    y += left.at(-1).h + GLASS
    halls.push({ x0, x1, y }); y += HALL
    if (i + 1 < r) {
      left.push(...roomRow(x0, x1, y + GLASS, i + 2 === r ? H - 6 - y - GLASS : h, false))
      y += GLASS + left.at(-1).h
      if (i + 2 < r) { walls.push({ x0, x1, y }); y += 4 }
    }
  }
  smalls = [...left, ...smalls]
  smalls.forEach((r, k) => { r.k = k; r.seats = 3 + rnd('seats/' + k) % 3 })
  assignRooms()
  syncPlates()
  buildPlaces()
  const key = `${rows} ${W}x${H}`
  if (key !== navKey) { // the graph changed under everyone's feet: put them straight where they're going
    navKey = key
    for (const e of emps.values()) if (e.inside) e.fresh = true
    for (const f of figs.values()) { f.inside = true; f.fresh = true } // at the chair, or out if the room is gone
  }
  for (const e of emps.values()) seatEmployee(e)
  const [l] = toCss(rooms.work.x, 0), [rr] = toCss(rooms.work.x + rooms.work.w, 0)
  cv.dataset.office = `${l.toFixed(1)},${rr.toFixed(1)}` // for the tests and the curious: the open office, left to right
  drawBackground()
}

// the scale and the building's size, for the smallest building minW x minH: the building fills the whole window
function fitScreen(minW, minH) {
  dpr = devicePixelRatio || 1
  const cw = Math.round(innerWidth * dpr), ch = Math.round(innerHeight * dpr)
  if (cv.width !== cw || cv.height !== ch) { cv.width = cw; cv.height = ch }
  const fit = Math.min(cw / minW, ch / minH)
  S = fit >= 2 ? Math.floor(fit) : fit // on small screens a fractional scale beats a tiny office
  W = Math.ceil(cw / S); H = Math.ceil(ch / S)
  // the pixel size: the same building, drawn with D pixels to a pixel of the office. The standard size draws one; the
  // finer steps draw as many as the screen has (at most 5, and a whole share of them, so the drawing goes onto the screen
  // without resampling), each step with more detail; their hairlines are at least a pixel of the screen thick
  D = detail ? [5, 4, 3, 2].find(d => S >= 2 && S % d === 0) ?? 2 : 1
  hair = 0; drawHair = detail ? Math.max(1, Math.round(D / S)) / D : 0
  cv.dataset.detail = String(D) // for the tests and the curious
  offX = Math.round((cw - W * S) / 2); offY = Math.round((ch - H * S) / 2) // under a pixel of overhang, split between the sides
}

// ================= walkways and spots =================
// Employees walk on a small graph: an aisle beside each column of desks, a corridor along the back wall and one along
// the strip below the desks, the corridors on both sides of the office, and walkways in the meeting room and the lounge.
// A spot is a node where an idle employee spends a while: a chair, a sofa seat, a place to stand.
const MEET_DOOR = 95 // the meeting room's doorway is in its glass wall on the corridor, this far down from its top
let nav = { pts:[], adj:[], door:-1, seats:[] }, navKey = ''
const navCache = new Map() // target node -> the way there from every node
const spots = new Map()    // id -> { id, x, y, node, room, face (1 front, -1 back), sit, nap }
let geo = {}               // furniture that comes alive (TV, foosball, printer) or hides people (sofa backrests)

function buildPlaces() {
  const pts = [], adj = []
  const node = (x, y) => (pts.push({ x, y }), adj.push([]), pts.length - 1)
  const link = (a, b) => { adj[a].push(b); adj[b].push(a) }
  const chain = ids => ids.reduce((a, b) => (link(a, b), b))
  const mark = (id, n, o) => spots.set(id, { id, x:pts[n].x, y:pts[n].y, node:n, face:1, sit:false, nap:false, ...o })
  const spot = (id, x, y, from, o) => { const n = node(x, y); link(n, from); mark(id, n, o) }
  spots.clear(); navCache.clear()
  const { work, meeting, lounge } = rooms
  const TOP = WALL_H + 14, BOT = work.y + work.h + 3, CX = (work.x + work.w + rightX) >> 1, WX = work.x - (WCOR >> 1)
  const top = new Map(), bot = new Map(), side = new Map(), wing = new Map() // corridor nodes, by x (by y for the side ones)
  const at = (m, k, x, y) => m.get(k) ?? m.set(k, node(x, y)).get(k)
  const onTop = x => at(top, x, x, TOP), onBot = x => at(bot, x, x, BOT), onSide = y => at(side, y, CX, y), onWest = y => at(wing, y, WX, y)
  top.set(CX, onSide(TOP)); bot.set(CX, onSide(BOT)); top.set(WX, onWest(TOP)); bot.set(WX, onWest(BOT))

  // the open office: each column's aisle runs from the back corridor to the front one, past its desks, and a cross
  // aisle between each two rows of desks joins the aisles to the corridors on both sides, so the way to a desk in a
  // middle row doesn't go round by the back wall or the front; the lifts and two windows on the back wall
  const seats = [], cross = Array.from({ length:rows - 1 }, () => [])
  for (let c = 0; c < COLS; c++) {
    const ax = deskOf(c).x + 40, aisle = [onTop(ax)]
    for (let r = 0; r < rows; r++) {
      const s = r * COLS + c, d = deskOf(s)
      aisle.push(node(ax, d.y - 2))
      seats[s] = node(d.x + 16, d.y - 2); link(seats[s], aisle.at(-1))
      if (r < rows - 1) { aisle.push(node(ax, d.y + 32)); cross[r].push(aisle.at(-1)) } // halfway to the next row's chairs
    }
    chain([...aisle, onBot(ax)])
  }
  cross.forEach((ids, r) => { const y = deskOf(r * COLS).y + 32; chain([onWest(y), ...ids, onSide(y)]) })
  const lift = node(door.x, door.y); link(lift, onTop(door.x))
  spot('win0', workX + 39, TOP - 4, onTop(workX + 39), { room:'work', face:-1 })
  spot('win1', workX + 259, TOP - 4, onTop(workX + 259), { room:'work', face:-1 })
  // the strip below the desks: the printer at its west end, the bookshelf at its east end
  const printer = { x:workX + 4, y:H - 28 }, shelf = { x:workX + 246, y:H - 40 }
  spot('print', printer.x + 13, printer.y - 6, onBot(printer.x + 13), { room:'work' })
  spot('books', shelf.x + 16, shelf.y - 4, onBot(shelf.x + 16), { room:'work' })

  // meeting room: a walkway around the table, ten chairs to nap on, the door on the corridor
  const tx = meeting.x + ((meeting.w - 112) >> 1), ty = meeting.y + 52, tw = 112, th = 30, chairX = i => tx + 15 + i * 26
  const mL = tx - 24, mR = tx + tw + 22, mT = ty - 16, mB = ty + th + 22, doorY = meeting.y + MEET_DOOR
  const tops = [0, 1, 2, 3].map(i => node(chairX(i), mT)), bottoms = [0, 1, 2, 3].map(i => node(chairX(i), mB))
  const TL = node(mL, mT), TR = node(mR, mT), BR = node(mR, mB), BL = node(mL, mB)
  const byLeft = node(mL, ty + 19), byRight = node(mR, ty + 19), inside = node(mL, doorY)
  chain([TL, ...tops, TR, byRight, BR, ...[...bottoms].reverse(), BL, inside, byLeft, TL])
  link(inside, onSide(doorY))
  tops.forEach((n, i) => spot('mt' + i, chairX(i), ty, n, { room:'meeting', sit:true, nap:true }))
  bottoms.forEach((n, i) => spot('mb' + i, chairX(i), ty + th + 9, n, { room:'meeting', sit:true, nap:true, face:-1 }))
  spot('ml', tx - 8, ty + 19, byLeft, { room:'meeting', sit:true, nap:true })
  spot('mr', tx + tw + 9, ty + 19, byRight, { room:'meeting', sit:true, nap:true })

  // lounge: a street across the middle, a loop around the foosball table (for tag), an aisle in front of the sofa
  const lx = lounge.x, ly = lounge.y, lr = lx + lounge.w, lb = ly + lounge.h, mx = lx + ((lounge.w - RIGHT_MIN) >> 1)
  const street = ly + 64, aisle = lb - 32, north = ly + 10
  const fx = mx + 60, fy = ly + 22
  const NW = node(fx - 12, north), NE = node(fx + 58, north), SE = node(fx + 58, street), SW = node(fx - 12, street)
  const fbN = node(fx + 17, north), fbS = node(fx + 29, street)
  chain([onSide(north), NW, fbN, NE, SE]); link(NW, SW)
  const west = node(mx + 38, street), east = node(mx + 130, street), vend = node(mx + 143, street), cafe = node(lr - 31, street)
  chain([onSide(street), west, SW, fbS, SE, east, vend, cafe])
  const sofa = { x:mx + 46, y:lb - 27, w:68, c:'#4f7d8a', dark:'#3d6370' }, seatX = [11, 33, 56].map(dx => sofa.x + dx)
  const coolA = node(lx + 28, aisle), westA = node(mx + 38, aisle), eastA = node(mx + 130, aisle), sofaA = seatX.map(x => node(x, aisle))
  link(west, westA); link(east, eastA)
  chain([coolA, westA, ...sofaA, eastA])
  spot('fb0', fx + 17, fy - 2, fbN, { room:'lounge' })
  spot('fb1', fx + 29, street - 4, fbS, { room:'lounge', face:-1 })
  mark('tag0', NW, { room:'lounge' }); mark('tag1', SE, { room:'lounge' })
  spot('vend', mx + 143, ly + 50, vend, { room:'lounge', face:-1 })
  spot('coffee', lr - 31, ly + 42, cafe, { room:'lounge' })
  seatX.forEach((x, i) => spot('ls' + i, x, lb - 17, sofaA[i], { room:'lounge', sit:true, nap:true, face:-1 }))
  spot('water', lx + 28, lb - 10, coolA, { room:'lounge' })
  spot('rt0', mx + 142, lb - 52, east, { room:'lounge', sit:true })
  spot('rt1', mx + 172, lb - 52, cafe, { room:'lounge', sit:true })

  // the small meeting rooms: a walkway along each hall from the corridor past the doors, and in each room one around the
  // table, with the chairs a task's members sit on (taskSeats; they aren't spots, so idle life keeps out of them)
  taskSeats.clear()
  const wingX = work.x - WCOR - 4, ways = halls.map(h => ({ h, y:h.y + (HALL >> 1), doors:new Map() }))
  for (const r of smalls) {
    const hy = r.north ? r.y + r.h + GLASS : r.y - GLASS - HALL
    const way = ways.find(w => w.h.y === hy && r.x >= w.h.x0 - 4 && r.x < w.h.x1)
    if (!way) continue
    const d = smallDoor(r), dx = d.x + (d.w >> 1)
    const outside = way.doors.get(dx) ?? way.doors.set(dx, node(dx, way.y)).get(dx), inside = node(dx, r.north ? r.y + r.h - 4 : r.y + 4)
    link(outside, inside)
    // a walkway around the table and its chairs, which the door joins on its side
    const cx = r.x + (r.w >> 1), cy = r.y + (r.h >> 1), chairs = roomChairs(cx, cy, r.seats, r.north ? -1 : 1), half = [10, 14, 20][r.seats - 3]
    const L = Math.max(r.x + 5, Math.min(cx - half, ...chairs.map(c => c.x)) - 5), R = Math.min(r.x + r.w - 5, Math.max(cx + half, ...chairs.map(c => c.x + 10)) + 4)
    const N = Math.max(r.y + 5, Math.min(cy - 8, ...chairs.map(c => c.y)) - 5), S = Math.min(r.y + r.h - 5, Math.max(cy + 8, ...chairs.map(c => c.y + 8)) + 4)
    const NW = node(L, N), NE = node(R, N), SE = node(R, S), SW = node(L, S), edge = { n:[NW, NE], s:[SW, SE], w:[NW, SW], e:[NE, SE] }
    chain([NW, NE, SE, SW, NW])
    edge[r.north ? 's' : 'n'].forEach(n => link(inside, n))
    // on a chair north of the table one faces us, south of it one shows its back; at the ends one faces us too, sitting a
    // little out from the table so it doesn't cover it
    const seats = chairs.map((c, i) => {
      const sx = c.x + 5 + (c.side === 'w' ? -2 : c.side === 'e' ? 2 : 0), sy = c.y + (c.side === 'n' ? 9 : 8)
      const a = node(c.side === 'w' ? L : c.side === 'e' ? R : sx, c.side === 'n' ? N : c.side === 's' ? S : sy), n = node(sx, sy)
      edge[c.side].forEach(m => link(a, m)); link(a, n)
      return { id:`room${r.k}.${i}`, x:sx, y:sy, node:n, room:'small', face:c.side === 's' ? -1 : 1, sit:true, nap:false, side:c.side }
    })
    // more members than chairs stand: in front of the table on the door's side when that side has no chairs, at an end
    // without a chair, then at the corners on the door's side (the plant is in a far corner), clear of the walls
    const has = side => chairs.some(c => c.side === side), near = r.north ? S : N, front = near + (r.north ? 4 : -4)
    const places = [...(has(r.north ? 's' : 'n') ? [] : [[cx - 14, front], [cx + 14, front]]), ...(has('w') ? [] : [[L, cy]]), ...(has('e') ? [] : [[R, cy]]), [L, near], [R, near]]
    const stand = places.map(([x, y], i) => {
      const n = node(Math.max(r.x + 12, Math.min(r.x + r.w - 12, x)), y)
      link(n, [NW, NE, SE, SW].reduce((a, b) => Math.hypot(pts[b].x - pts[n].x, pts[b].y - pts[n].y) < Math.hypot(pts[a].x - pts[n].x, pts[a].y - pts[n].y) ? b : a))
      return { id:`room${r.k}.stand${i}`, x:pts[n].x, y:pts[n].y, node:n, room:'small', face:1, sit:false, nap:false }
    })
    taskSeats.set(r.k, [...seats, ...stand])
  }
  for (const w of ways) { // the halls left of the office meet the west corridor at their east end, those below the lounge the east one
    const west = w.h.x1 === wingX
    chain([west ? onWest(w.y) : onSide(w.y), ...[...w.doors].sort((a, b) => west ? b[0] - a[0] : a[0] - b[0]).map(([, n]) => n)])
  }

  const sorted = m => [...m].sort((a, b) => a[0] - b[0]).map(([, n]) => n)
  chain(sorted(top)); chain(sorted(bot)); chain(sorted(side)); chain(sorted(wing))
  nav = { pts, adj, door:lift, seats }
  geo = { foos:{ x:fx, y:fy }, tv:{ x:mx + 62, y:lb - 72 }, sofa, printer, shelf, loop:[NW, NE, SE, SW], loungeMid:mx }
}

// the shortest way to a node from every node: dist[n] is how far, next[n] the node to go to next
function towards(target) {
  if (navCache.has(target)) return navCache.get(target)
  const { pts, adj } = nav, n = pts.length
  const dist = new Array(n).fill(Infinity), next = new Array(n).fill(-1), done = new Array(n).fill(false)
  dist[target] = 0
  for (;;) {
    let u = -1
    for (let i = 0; i < n; i++) if (!done[i] && dist[i] < Infinity && (u < 0 || dist[i] < dist[u])) u = i
    if (u < 0) break
    done[u] = true
    for (const v of adj[u]) {
      const d = dist[u] + Math.hypot(pts[u].x - pts[v].x, pts[u].y - pts[v].y)
      if (d < dist[v]) { dist[v] = d; next[v] = u }
    }
  }
  const way = { dist, next }
  navCache.set(target, way)
  return way
}
// for the tests: from every spot to every desk, how far it is along the walkways, and how far across and down
window.__routes = () => [...spots.values()].flatMap(s => nav.seats.map((n, seat) => ({ from:s.id, seat,
  walk:Math.round(towards(n).dist[s.node]), dx:Math.round(Math.abs(nav.pts[n].x - s.x)), dy:Math.round(Math.abs(nav.pts[n].y - s.y)) })))

function drawBackground() {
  bgCanvas.width = W * D; bgCanvas.height = H * D
  buf.width = W * D; buf.height = H * D
  const g = bgCtx
  g.setTransform(D, 0, 0, D, 0, 0)
  hair = drawHair
  // floors: light tiles in the corridors, each room its own
  for (let ty = 0; ty * T < H; ty++) for (let tx = 0; tx * T < W; tx++) {
    rect(g, tx * T, ty * T, T, T, (tx + ty) % 2 ? '#e0dcd3' : '#dcd8ce')
    if (detail >= 3 && hair) { solid(g, tx * T, ty * T, T, hair, '#e7e4dd'); solid(g, tx * T, ty * T, hair, T, '#e7e4dd') } // the joints
  }
  const { work, meeting, lounge } = rooms
  rect(g, work.x, work.y, work.w, work.h, '#cdd4dc') // office carpet
  for (let y = work.y; y < work.y + work.h; y += 4) rect(g, work.x, y, work.w, 1, '#c8cfd8')
  if (detail >= 3 && hair) weave(g, work.x, work.y, work.w, work.h, '#c3cad3')
  g.strokeStyle = '#a9b1bb'; g.lineWidth = 1; g.strokeRect(work.x + 0.5, work.y + 0.5, work.w - 1, work.h - 1)
  for (let y = meeting.y; y < meeting.y + meeting.h; y += 6) { // meeting room parquet
    rect(g, meeting.x, y, meeting.w, 6, (y / 6) % 2 ? '#c49c6d' : '#bd9566'); rect(g, meeting.x, y + 5, meeting.w, 1, '#a9845a')
    if (detail >= 3 && hair) for (let x = meeting.x + (y * 7) % 40; x < meeting.x + meeting.w; x += 40) rect(g, x, y, 1, 5, '#a9845a') // the planks' ends
  }
  for (let y = lounge.y; y < lounge.y + lounge.h; y += 8) for (let x = lounge.x; x < lounge.x + lounge.w; x += 8) {
    rect(g, x, y, 8, 8, ((x - lounge.x) / 8 + (y - lounge.y) / 8) % 2 ? '#ebe5d8' : '#e2dbcc') // lounge tiles
    if (detail >= 3 && hair) { solid(g, x, y, 8, hair, '#f3eee4'); solid(g, x, y, hair, 8, '#f3eee4') }
  }

  // back wall: a plain band with a molding on top, then the lifts, windows, the sign, paintings, a clock, the exit to the
  // stairs; a TV on the meeting room's stretch, a whiteboard or a screen on each small meeting room's
  rect(g, 0, 0, W, WALL_H, '#ebe5d9'); rect(g, 0, 0, W, 3, '#8f8676')
  rect(g, 0, DECOR - 6, W, 2, '#ddd5c6'); rect(g, 0, DECOR - 4, W, 1, '#f4efe6')
  rect(g, 0, WALL_H - 4, W, 4, '#b9ad98'); rect(g, 0, WALL_H - 4, W, 1, '#cfc4b0')
  if (detail >= 3 && hair) for (let x = 12; x < W; x += 24) solid(g, x, DECOR - 2, hair, WALL_H - 2 - DECOR, '#e2dbcd') // panels
  const signX = workX + (COLS * COL_PITCH) / 2 - 50
  drawSign(g, signX, DECOR + 12)
  drawLift(g, door.x - 25, WALL_H - 34, false); drawLift(g, door.x + 3, WALL_H - 34, false); drawLiftButtons(g, door.x - 2, WALL_H - 22)
  drawWindow(g, workX + 24, DECOR + 10); drawWindow(g, workX + 244, DECOR + 10)
  drawPainting(g, signX - 26, DECOR + 13, 18, 22, ['#c6dfec', '#6f9a63', '#f2b705'])
  drawPainting(g, signX + 108, DECOR + 13, 18, 22, ['#f0d9c4', '#a65d4e', '#5d7da6'])
  drawClock(g, workX + 280, DECOR + 10)
  drawExitSign(g, rightX - 16, WALL_H - 26)
  drawTV(g, meeting.x + meeting.w / 2 - 24, DECOR + 10)
  smalls.filter(r => r.north && r.y === WALL_H).forEach((r, i) => { // and windows beside it in a wide room
    const mid = r.x + (r.w >> 1)
    if (i % 2) drawWallScreen(g, mid - 17, DECOR + 12); else drawWhiteboard(g, mid - 21, DECOR + 11)
    if (r.w >= 176) { drawWindow(g, r.x + 10, DECOR + 10); drawWindow(g, r.x + r.w - 40, DECOR + 10) }
  })

  // meeting room: glass walls (the door faces the corridor), a long table, ten chairs, two big plants
  drawGlassV(g, rightX, WALL_H, meeting.h, meeting.y + MEET_DOOR - 13, 26)
  drawMeetingTable(g, meeting.x + ((meeting.w - 112) >> 1), meeting.y + 52, 112, 30)
  drawBigPlant(g, meeting.x + meeting.w - 22, meeting.y + 6)
  drawBigPlant(g, meeting.x + 8, meeting.y + meeting.h - 34)
  drawGlassH(g, rightX, meeting.y + meeting.h, meeting.w)

  // small meeting rooms along the halls, walls between them and between their rows; the left ones' wall on the corridor
  // opens onto each hall; a plant at each hall's closed end
  smalls.forEach(r => drawSmallRoom(g, r))
  for (const r of smalls) drawPartition(g, r.x - 4, r.north ? r.y : r.y - GLASS, r.h + GLASS)
  for (const w of walls) drawWallH(g, w.x0, w.y, w.x1 - w.x0)
  const wingX = work.x - WCOR - 4, openings = halls.filter(h => h.x1 === wingX).map(h => h.y).sort((a, b) => a - b)
  let wy = WALL_H
  for (const y of [...openings, H - 6]) { if (y > wy) drawPartition(g, wingX, wy, y - wy); wy = y + HALL }
  for (const h of halls) drawPlant(g, h.x1 === wingX ? h.x0 + 2 : h.x1 - 18, h.y + 2)

  // lounge: a counter (sink, coffee machine, microwave), a fridge, a vending machine, foosball, a TV in front of
  // the sofa, a small round table, a water cooler
  const lx = lounge.x, ly = lounge.y, lr = lounge.x + lounge.w, lb = ly + lounge.h, { sofa, printer, shelf, loungeMid:mx } = geo
  drawCounter(g, lr - 22, ly + 4, 22, 74)
  drawSink(g, lr - 18, ly + 8); drawCoffeeMachine(g, lr - 17, ly + 26); drawMicrowave(g, lr - 18, ly + 48)
  drawFridge(g, lr - 21, ly + 82)
  drawVending(g, mx + 132, ly + 6)
  drawFoosball(g, geo.foos.x, geo.foos.y)
  drawRug(g, mx + 34, lb - 52, 92, 46, '#c98b6b', '#d9a184')
  drawLoungeTV(g, geo.tv.x, geo.tv.y)
  drawSofa(g, sofa.x, sofa.y, sofa.w, sofa.c, sofa.dark)
  drawRoundTable(g, mx + 148, lb - 62)
  drawCooler(g, lx + 6, lb - 32)
  drawPlant(g, lr - 20, lb - 24)
  const gap = (lb - 76) - (ly + 74) // a tall lounge gets bean bags between the street and the TV, clear of the walkways
  if (gap >= 36) {
    const my = ly + 74 + (gap >> 1)
    drawRug(g, mx + 56, my - 14, 60, 28, '#9cb8a8', '#b3cbbd')
    drawBeanBag(g, mx + 60, my - 8, '#f2b705', '#c99a04'); drawBeanBag(g, mx + 79, my - 12, '#4a90d9', '#3a73ad'); drawBeanBag(g, mx + 98, my - 8, '#d64541', '#a83330')
    drawFloorLamp(g, mx + 116, my - 22)
  }

  // the strip below the desks: the printer and two filing cabinets, the bookshelf, plants at the corridors' ends, and a
  // fire extinguisher by the lifts
  drawPrinter(g, printer.x, printer.y); drawCabinet(g, printer.x + 32, H - 32); drawCabinet(g, printer.x + 48, H - 32)
  drawBookshelf(g, shelf.x, shelf.y); drawPlant(g, shelf.x - 20, H - 26)
  drawBigPlant(g, work.x - WCOR + 3, H - 36); drawBigPlant(g, work.x + work.w + 1, H - 36)
  drawExtinguisher(g, workX + 16, WALL_H - 10)

  // side walls and the south wall
  rect(g, 0, WALL_H, 4, H - WALL_H, '#b9ad98'); rect(g, W - 4, WALL_H, 4, H - WALL_H, '#b9ad98')
  rect(g, 0, H - 6, W, 6, '#b9ad98'); rect(g, 0, H - 6, W, 1, '#cfc4b0')
  hair = 0
}

// ================= employees (one per account) =================
const emps = new Map() // account id -> employee
const lift = { at:-1e9, no:0 } // when a lift last opened for someone, and which of the two
const liftRide = () => { lift.at = performance.now(); lift.no++ }
const WALK = 48, HURRY = 90, TAG_RUN = 70, TAG_CHASE = 84 // pixels per second

function makeEmployee(acc) {
  const label = document.createElement('div')
  label.className = 'emp-label'
  label.setAttribute('aria-hidden', 'true') // (the roster says it for screen readers)
  label.dataset.account = acc.id
  label.addEventListener('click', () => onEmployeeClick(acc.id))
  document.getElementById('labels').appendChild(label)
  return { key:acc.id, acc, s:null, label, x:0, y:0, path:[], node:-1, goal:null, want:null, inside:false, fresh:false, facing:1,
    blinkAt:performance.now() + 2000 + Math.random() * 4000, phase:hash(acc.id) % 1000 }
}

const isOnline = e => !!e?.s && e.s.state !== 'ended'

// task: the task the employee is away in (its figure sits in the meeting room); its desk then shows an empty chair, and its
// tag stays there with the desk session
function updateEmployee(e, acc, walkIn, task) {
  const was = isOnline(e)
  e.acc = acc
  e.s = sessionOf(acc)
  e.away = task ?? null
  const online = isOnline(e)
  if (online && !was && !e.inside && !task) { // on duty: in through the door, or straight to its spot when the page opens
    if (walkIn) { e.x = door.x; e.y = door.y; e.node = nav.door; e.path = []; e.goal = null; liftRide() }
    else e.fresh = true
    e.inside = true
  } // off duty: the planner walks it out
  const state = online ? e.s.state : 'ended'
  e.label.style.borderLeftColor = STATE_MARK[state] ?? GRAY_MARK
  e.label.classList.toggle('off', !online)
  e.label.dataset.state = state
  const status = task && !online ? t('task.inRoom', { room:taskRoom(task), id:task.id }) : statusText(online ? e.s : null)
  setLabel(e, acc.name, state, status)
  e.status = `${acc.name} · ${status}`
}
// A name tag's text, changed in place: the tag and its parts stay the elements they were, so a click pressed on one
// as the state changes still lands. Measured again next frame when it changed.
function setLabel(x, name, state, status) {
  const sig = `${name}|${state}|${status}`
  if (sig === x.labelSig) return
  x.labelSig = sig
  if (!x.label.firstChild) x.label.innerHTML = '<div class="nm"></div><div class="st"><span class="dot"></span><span class="tx"></span></div>'
  x.label.querySelector('.nm').textContent = name
  x.label.querySelector('.st .dot').style.background = STATE_MARK[state] ?? GRAY_MARK
  x.label.querySelector('.st .tx').textContent = status
  x.sized = false
}

function dropEmployee(e) { e.label.remove(); e.bubble?.remove(); emps.delete(e.acc.id) }

// ================= task members =================
// While a member's task session is online, its figure sits at a chair of the task's small meeting room (chairs in member
// order): an employee gets up from wherever it is and walks there, or comes in by the lift when it's off duty; an advisor
// comes in by the lift and wears a visitor's badge. When the task session ends, the figure goes back to its desk (and is
// the employee again there) or out by the lift. Figures move and are drawn like employees; key: task id/account id.
const figs = new Map()
const taskSeats = new Map() // a small room's k -> where a task's members sit: its chairs in member order, then places to stand

function makeFigure(key) {
  const label = document.createElement('div')
  label.className = 'emp-label member'
  label.setAttribute('aria-hidden', 'true')
  label.addEventListener('click', () => onFigureClick(figs.get(key)))
  document.getElementById('labels').appendChild(label)
  return { key, label, x:0, y:0, path:[], node:-1, goal:null, want:null, inside:true, fresh:false, facing:1,
    blinkAt:performance.now() + 2000 + Math.random() * 4000, phase:hash(key) % 1000 }
}

// the accounts away in a task now: those with a figure, and those whose task session is online (account id -> task)
function awayInTasks() {
  const away = new Map()
  for (const f of figs.values()) away.set(f.acc.id, f.task)
  for (const task of snapshot.tasks) for (const m of task.members) if (isOnline({ s:sessionById(m.session) })) away.set(m.account, task)
  return away
}

function syncFigures(walkIn) {
  const live = new Map()
  for (const task of snapshot.tasks) task.members.forEach((m, i) => {
    const s = sessionById(m.session)
    if (s && s.state !== 'ended') live.set(task.id + '/' + m.account, { task, m, i, s })
  })
  for (const [key, w] of live) {
    let f = figs.get(key)
    if (!f) {
      f = makeFigure(key)
      const e = emps.get(w.m.account)
      if (e?.inside && !e.fresh) { // it gets up from wherever it is
        Object.assign(f, { x:e.x, y:e.y, node:e.node, path:e.path, facing:e.facing })
        Object.assign(e, { inside:false, path:[], goal:null, tag:null, there:false, atDesk:false })
      } else if (walkIn) { f.x = door.x; f.y = door.y; f.node = nav.door; liftRide() }
      else f.fresh = true
      figs.set(key, f)
    }
    const acc = snapshot.accounts.find(a => a.id === w.m.account) ?? f.acc ?? { id:w.m.account, name:w.m.name, seat:-1, advisor:!!w.m.advisor }
    Object.assign(f, { acc, s:w.s, task:w.task, member:w.m, order:w.i, leaving:false })
  }
  for (const [key, f] of figs) if (!live.has(key)) { // its task session ended (or the task did): back to the desk, or out
    f.leaving = true
    f.s = { ...(sessionById(f.s.id) ?? f.s), state:'ended' }
    f.task = taskById(f.task.id) ?? f.task
  }
  for (const f of figs.values()) labelFigure(f)
}

// a member's tag: the member's name (bob-T7) and its task session's state; a pending ask in its desk session turns it red too
function labelFigure(f) {
  const online = isOnline(f), state = !online ? 'ended' : asking(f.acc.session) ? 'waiting' : f.s.state
  f.label.style.borderLeftColor = STATE_MARK[state] ?? GRAY_MARK
  f.label.classList.toggle('off', !online)
  setData(f.label, 'state', state); setData(f.label, 'account', f.acc.id); setData(f.label, 'task', f.task.id); setData(f.label, 'session', f.s.id)
  const status = statusText(online ? f.s : null)
  setLabel(f, f.member.name, state, status)
  f.status = `${f.member.name} · ${status} · ${taskRoom(f.task)}`
}

// a figure that left its task: at its desk it becomes the employee again; out by the lift it's gone
function settleFigure(f) {
  const e = emps.get(f.acc.id)
  if (f.inside && !(f.goal?.id === 'home' && !f.path.length)) return
  if (f.inside && e) Object.assign(e, { x:f.x, y:f.y, node:f.node, path:[], goal:null, facing:f.facing, inside:true, fresh:false })
  f.label.remove(); f.bubble?.remove(); figs.delete(f.key)
  if (e) updateEmployee(e, e.acc, false, awayInTasks().get(e.acc.id))
}
function onFigureClick(f) { if (f) openTaskWin(f.task.id) } // a member's window opens from its task's window

// ================= idle life =================
// An idle employee stays at its desk a while (the next prompt may be on its way), then wanders off: TV on the
// lounge sofa, coffee, the vending machine, the water cooler, a window, the bookshelf, the printer, or with a
// colleague foosball, tag or a chat at the round table. After a while it naps (most hide in
// the meeting room) and now and then wakes up for another round. It all counts from when the bridge saw the
// session go idle, so reloading the page wakes nobody. Work or a pending ask sends it back to its desk; so does a
// click, and then it stays there until its next turn.
const SETTLE = 20000, FIRST_NAP = 150000, NAP = 240000, WAKE = 90000, SLOT = 30000, PAIR_SLOT = 40000
const SOLO = [['tv', 4], ['coffee', 2], ['vending', 2], ['water', 2], ['window', 2], ['books', 2], ['print', 2], ['desk', 1]]
const SOLO_SPOTS = { tv:['ls0', 'ls1', 'ls2'], coffee:['coffee'], vending:['vend'], water:['water'], window:['win0', 'win1'],
  books:['books'], print:['print'] }
const PAIR_SPOTS = { foosball:['fb0', 'fb1'], chase:['tag0', 'tag1'], chat:['rt0', 'rt1'] }
const NAP_SPOTS = [['mt0', 'mt1', 'mt2', 'mt3', 'mb0', 'mb1', 'mb2', 'mb3', 'ml', 'mr'], ['ls0', 'ls1', 'ls2']]

// a well-mixed hash for the choices (similar strings get similar numbers from hash())
function rnd(s) {
  let h = 2166136261
  for (const c of String(s)) { h ^= c.codePointAt(0); h = Math.imul(h, 16777619) }
  h ^= h >>> 16; h = Math.imul(h, 2246822507); h ^= h >>> 13; h = Math.imul(h, 3266489909); h ^= h >>> 16
  return h >>> 0
}
const idleSince = e => Date.parse(e.s?.stateSince || '') || Date.now()
const summoned = e => !!e.summon && e.s?.id === e.summon.session && e.s.stateSince === e.summon.since && e.s.state === 'idle'

// where an idle employee is in its rhythm at `at` (ms since 1970): settling at its desk, wandering, or napping;
// for a nap, also when it began
function rhythm(e, at) {
  if (motion.still) return { phase:'settle' } // motion paused: no wandering about, the desk it is
  const t = at - idleSince(e)
  if (t < SETTLE) return { phase:'settle' }
  if (t < FIRST_NAP) return { phase:'roam' }
  const k = (t - FIRST_NAP + rnd(e.acc.id + '/nap') % (NAP / 2)) % (NAP + WAKE) // each has its own
  return k < NAP ? { phase:'nap', since:at - k } : { phase:'roam' }
}

// at its desk: ready for work right after a click, then the phone or games (each has a preference, switching
// every half minute), asleep in the end
function deskPastime(e, at) {
  const called = summoned(e), t = at - (called ? e.summon.at : idleSince(e))
  if (e.want?.act === 'nap' || (called && t > FIRST_NAP)) return 'sleep'
  if (called && t < 60000) return 'ready'
  if (motion.still) return 'phone' // (motion paused: one pastime, not a change every half minute)
  return ['phone', 'game'][(hash(e.acc.id) + Math.floor(t / 30000)) % 2]
}

// Pairs form on a clock all pages share: in each PAIR_SLOT, the employees who were already wandering when it
// began may pair up (not every time) for foosball, tag or a chat. Foosball and tag need the same table.
let pairMemo = { slot:-1, pairs:[] }
function pairsAt(at, roamers) {
  const P = Math.floor(at / PAIR_SLOT)
  if (pairMemo.slot !== P) {
    const start = P * PAIR_SLOT
    const pool = roamers.filter(e => rhythm(e, start).phase === 'roam')
      .sort((a, b) => rnd(a.acc.id + '/' + P) - rnd(b.acc.id + '/' + P))
    const pairs = [], used = new Set(), kinds = ['foosball', 'chase', 'chat'], table = k => k === 'chase' ? 'foosball' : k
    for (let i = 0; i + 1 < pool.length && pairs.length < 2; i += 2) {
      if (rnd('pair/' + P + '/' + i) % 10 >= 7) continue
      const first = rnd('kind/' + P + '/' + i) % 3
      const kind = [0, 1, 2].map(k => kinds[(first + k) % 3]).find(k => !used.has(table(k)))
      if (!kind) break
      used.add(table(kind))
      pairs.push({ kind, key:P + '/' + i, ids:[pool[i].acc.id, pool[i + 1].acc.id] })
    }
    pairMemo = { slot:P, pairs }
  }
  return pairMemo.pairs
}

// a solo idea for each half minute, best first: a weighted pick that differs from the last one, then the rest
function soloIdeas(e, slot) {
  const total = SOLO.reduce((n, [, w]) => n + w, 0)
  const pick = k => { let r = rnd(e.acc.id + '/' + k) % total; return SOLO.find(([, w]) => (r -= w) < 0)[0] }
  let first = pick(slot)
  if (slot > 0 && first === pick(slot - 1)) first = pick(slot + 1e6)
  const rest = SOLO.map(([a]) => a).filter(a => a !== first), h = rnd(e.acc.id + '/order/' + slot)
  return [first, ...rest.map((_, i) => rest[(i + h) % rest.length])]
}

// where everyone in the building should be now, and what for: Map employee -> { act, spot, hurry, pair }
function planLife(at) {
  const plan = new Map(), taken = new Set()
  const desk = e => ({ id:'desk', x:e.seat.x, y:e.seat.y, node:nav.seats[e.acc.seat], room:'desk', face:1, sit:true })
  const free = (ids, turn) => ids.map((_, i) => spots.get(ids[(i + turn) % ids.length])).find(s => s && !taken.has(s.id))
  const claim = (e, act, s, more) => { taken.add(s.id); plan.set(e, { act, spot:s, ...more }) }
  const naps = [], roam = [], out = { id:'out', x:door.x, y:door.y, node:nav.door, room:'out', face:1 }
  // a task's members sit at their chairs for the whole task (out of the building when the room isn't in this layout);
  // one that left goes back to its desk if its desk session is online, else out
  for (const f of figs.values()) {
    if (!f.inside) continue
    const seats = taskSeats.get(roomOf(f.task)?.k), e = emps.get(f.acc.id)
    if (!f.leaving) plan.set(f, { act:'task', spot:seats ? seats[Math.min(f.order, seats.length - 1)] : out })
    else if (isOnline(e)) plan.set(f, { act:'back', spot:{ ...desk(e), id:'home' }, hurry:true })
    else plan.set(f, { act:'out', spot:out })
  }
  for (const e of emps.values()) {
    if (!e.inside || !e.desk) continue
    if (!isOnline(e)) plan.set(e, { act:'out', spot:out })
    else if (e.s.state !== 'idle') plan.set(e, { act:'work', spot:desk(e), hurry:true })
    else if (summoned(e)) plan.set(e, { act:'ready', spot:desk(e), hurry:true })
    else {
      const r = rhythm(e, at)
      if (r.phase === 'settle') plan.set(e, { act:'desk', spot:desk(e) })
      else if (r.phase === 'nap') naps.push([e, r.since])
      else roam.push(e)
    }
  }
  // nappers keep their spot for the whole nap (whoever fell asleep first chose first): most like the meeting
  // room, some the lounge sofa, some their own desk
  naps.sort((a, b) => a[1] - b[1] || (a[0].acc.id < b[0].acc.id ? -1 : 1))
  for (const [e] of naps) {
    const h = rnd(e.acc.id + '/bed'), k = h % 20
    const order = k < 13 ? [0, 1] : k < 17 ? [1, 0] : []
    const s = order.map(i => free(NAP_SPOTS[i], h)).find(Boolean)
    if (s) claim(e, 'nap', s); else plan.set(e, { act:'nap', spot:desk(e) })
  }
  // pairs
  const paired = new Set()
  for (const pair of pairsAt(at, roam)) {
    const two = pair.ids.map(id => emps.get(id)), ids = PAIR_SPOTS[pair.kind]
    if (!two.every(e => e && roam.includes(e)) || ids.some(id => taken.has(id))) continue
    two.forEach((e, i) => { claim(e, pair.kind, spots.get(ids[i]), { pair }); paired.add(e) })
  }
  // the rest wander on their own; whoever had its idea first gets the spot
  const solo = roam.filter(e => !paired.has(e)).map(e => {
    const t = at - idleSince(e) - SETTLE + rnd(e.acc.id + '/beat') % (SLOT / 2) // each its own beat, the first idea lasting 15 to 30 s
    return { e, slot:Math.floor(t / SLOT), since:at - t % SLOT }
  })
  solo.sort((a, b) => a.since - b.since || (a.e.acc.id < b.e.acc.id ? -1 : 1))
  for (const { e, slot } of solo) {
    const act = soloIdeas(e, slot).find(a => a === 'desk' || free(SOLO_SPOTS[a], 0))
    if (act === 'desk') plan.set(e, { act, spot:desk(e) })
    else claim(e, act, free(SOLO_SPOTS[act], rnd(e.acc.id + '/' + act)))
  }
  return plan
}

// every frame: follow the plan, walk, play
function lifeTick(at, now, dt) {
  const plan = planLife(at)
  for (const [e, w] of plan) {
    e.want = w
    if (e.fresh) { place(e, w.spot, now); e.fresh = false }
    else if (e.goal?.id !== w.spot.id) { if (motion.still) place(e, w.spot, now); else go(e, w.spot) } // (motion paused: there at once)
    else e.goal = w.spot // the same spot, maybe rebuilt by a new layout
    if (motion.still && (e.path.length || e.tag)) place(e, e.goal, now) // on the way as motion paused
  }
  playTag(plan, now, dt)
  for (const e of plan.keys()) e.moving = move(e, dt, now)
  for (const f of [...figs.values()]) if (f.leaving) settleFigure(f)
}

function place(e, spot, now) {
  e.x = spot.x; e.y = spot.y; e.node = spot.node; e.path = []; e.goal = spot; e.arrived = now; e.tag = null
  if (spot.id === 'out') e.inside = false
}

// sets off for a spot along the walkways, from the node it last passed or the one it was heading for (in a game
// of tag, the corner ahead), whichever makes the shorter trip
function go(e, spot) {
  const ends = [e.node, e.path[0]?.n ?? e.tag?.ahead]
  e.goal = spot; e.tag = null
  const { pts } = nav, { dist, next } = towards(spot.node)
  let from = -1, best = Infinity
  for (const n of ends) {
    if (!(n >= 0)) continue
    const d = Math.hypot(pts[n].x - e.x, pts[n].y - e.y) + dist[n]
    if (d < best) { from = n; best = d }
  }
  if (from < 0 || best === Infinity) return place(e, spot, performance.now())
  e.path = []
  for (let n = from; n >= 0; n = next[n]) e.path.push({ x:pts[n].x, y:pts[n].y, n })
}

// one frame of walking; true while on the way
function move(e, dt, now) {
  if (e.tag) return true // the game of tag moves it
  if (!e.path.length) return false
  let left = (e.want?.hurry ? HURRY : WALK) * dt
  while (left > 0 && e.path.length) {
    const p = e.path[0], dx = p.x - e.x, dy = p.y - e.y, d = Math.hypot(dx, dy)
    e.facing = dy < -0.01 ? -1 : 1 // walking up, we see the back
    if (d <= left) { e.x = p.x; e.y = p.y; e.node = p.n; e.path.shift(); left -= d }
    else { e.x += dx / d * left; e.y += dy / d * left; left = 0 }
  }
  if (!e.path.length) { e.arrived = now; if (e.goal?.id === 'out') { e.inside = false; liftRide() } }
  return true
}

// Tag around the foosball table: "it" runs a little faster than the other; a tag turns them both around,
// and the new "it" counts a second before giving chase.
const tagGames = new Map() // pair key -> { s:[distance along the loop of each], it, dir, wait, pow }
function playTag(plan, now, dt) {
  const loop = geo.loop.map(n => nav.pts[n]), lens = loop.map((p, i) => Math.hypot(loop[(i + 1) % 4].x - p.x, loop[(i + 1) % 4].y - p.y))
  const L = lens.reduce((a, b) => a + b)
  const where = s => {
    s = ((s % L) + L) % L
    let i = 0
    while (i < 3 && s > lens[i]) s -= lens[i++]
    const p = loop[i], q = loop[(i + 1) % 4], f = Math.min(1, s / lens[i])
    return { x:p.x + (q.x - p.x) * f, y:p.y + (q.y - p.y) * f, edge:i }
  }
  const live = new Set()
  for (const pair of pairMemo.pairs) {
    if (pair.kind !== 'chase') continue
    const two = pair.ids.map(id => emps.get(id))
    if (!two.every(e => e && plan.get(e)?.pair === pair)) continue
    if (!two.every(e => e.tag || (!e.path.length && e.goal?.id.startsWith('tag')))) continue // waiting for the other
    live.add(pair.key)
    let game = tagGames.get(pair.key)
    if (!game) tagGames.set(pair.key, game = { s:[0, L / 2], it:0, dir:1, wait:0, pow:null })
    const run = 1 - game.it
    game.s[run] += game.dir * TAG_RUN * dt
    if (game.wait > 0) game.wait -= dt
    else game.s[game.it] += game.dir * TAG_CHASE * dt
    if (game.wait <= 0 && (((game.s[run] - game.s[game.it]) * game.dir) % L + L) % L < 6) {
      const p = where(game.s[run])
      game.pow = { x:p.x, y:p.y - 6, at:now }
      game.it = run; game.dir = -game.dir; game.wait = 1
    }
    two.forEach((e, i) => {
      const p = where(game.s[i])
      e.x = p.x; e.y = p.y; e.node = geo.loop[p.edge]; e.path = []
      e.facing = (game.dir > 0 ? p.edge === 3 : p.edge === 1) ? -1 : 1
      e.tag = { it:game.it === i, wait:game.it === i && game.wait > 0, ahead:geo.loop[(p.edge + 1) % 4] }
    })
  }
  for (const key of tagGames.keys()) if (!live.has(key)) tagGames.delete(key)
}

// a chat at the round table: they take turns, a bubble each
function chatLine(e, pair, now) {
  const k = Math.floor(now / 1800)
  if (!pair || now % 1800 > 1400 || !pair.ids.every(id => emps.get(id)?.there)) return null
  if (pair.ids[rnd(pair.key + '/' + k) % 2] !== e.acc.id) return null
  return ['…', '!', '?', '♥', '♪'][rnd(pair.key + '/say/' + k) % 5]
}

// what an idle employee is doing, in words (for the card, the search list and the name tag's tooltip)
function doingText(e) {
  // the plan lags the state by a tick: a 'work' or 'out' plan on an idle employee has no text yet; a task's member has no
  // idle life
  if (e.member || !e.inside || !isOnline(e) || e.s.state !== 'idle' || !e.want || e.want.act === 'work' || e.want.act === 'out') return ''
  const w = e.want, room = e.goal?.room
  if (e.moving && !e.tag) return t(w.act === 'ready' ? 'idle.back' : 'idle.walk')
  const doing = t('idle.' + w.act)
  if (room && room !== 'desk') return t('idle.where', { doing, room:t('room.' + room) })
  return w.act === 'nap' ? t('idle.where', { doing, room:t('idle.desk') }) : doing
}
const stateLine = e => { const doing = doingText(e); return statusText(isOnline(e) ? e.s : null) + (doing ? ' · ' + doing : '') }

// the subagents beside a desk: at most four, picked the running ones first, then the newest (the bridge may keep only
// some of the finished ones; ended marks one that finished), and standing in the order they started
const agentDone = a => a.state === 'done' || !!a.ended
const agentAt = a => Date.parse(a.started) || 0
function agentsShown(s) {
  const list = Object.values(s.agents || {})
  const picked = list.length <= 4 ? list : [...list].sort((a, b) => agentDone(a) - agentDone(b) || agentAt(b) - agentAt(a)).slice(0, 4)
  return picked.sort((a, b) => agentAt(a) - agentAt(b))
}

// ================= rendering =================
const cv = document.getElementById('office'), ctx = cv.getContext('2d')
let S = 1, offX = 0, offY = 0, dpr = 1, D = 1, drawHair = 0 // D: the drawing's pixels to a pixel of the office

const toCss = (x, y) => [(offX + x * S) / dpr, (offY + y * S) / dpr]

const flights = [] // envelopes in flight { from, to, t, toKey }
const mailAt = new Map() // an employee's or a member's key -> when mail arrived; it shows above the head for a while

const FRAME_MS = 1000 / 30 // frames a second: enough for pixel art, and half the work of a 60 Hz screen
function frame(now) {
  requestAnimationFrame(frame) // first, so one bad frame doesn't stop the office
  if (frame.last !== undefined && now - frame.last < FRAME_MS - 2) return // (a little under, so a 60 Hz screen gets every other frame)
  const dt = Math.min((now - (frame.last ?? now)) / 1000, 0.1)
  frame.last = now
  const at = Date.now()
  const g = bufCtx
  g.setTransform(1, 0, 0, 1, 0, 0)
  g.drawImage(bgCanvas, 0, 0)
  g.setTransform(D, 0, 0, D, 0, 0)
  hair = drawHair
  lifeTick(at, now, dt)

  const items = [] // sorted by the feet's y, so nearer things cover farther ones
  const taken = new Set(snapshot.accounts.filter(a => a.seat >= 0).map(a => a.seat))
  for (let seat = 0; seat < rows * COLS; seat++) { // empty desks are drawn too
    if (taken.has(seat)) continue
    const d = deskOf(seat)
    items.push({ y:d.y - 3, draw:() => drawChair(g, d.x, d.y) })
    items.push({ y:d.y + 14, draw:() => { drawDesk(g, d.x, d.y, OFF); drawClutter(g, d.x, d.y, seat) } })
  }
  const using = new Set() // spots someone has reached
  let watching = false
  for (const e of emps.values()) {
    if (!e.desk) continue
    const online = isOnline(e), st = online ? e.s.state : 'ended'
    items.push({ y:e.desk.y - 3, draw:() => drawChair(g, e.desk.x, e.desk.y) })
    items.push({ y:e.desk.y + 14, draw:() => { drawDesk(g, e.desk.x, e.desk.y, online ? STATE_COLOR[st] : OFF); drawClutter(g, e.desk.x, e.desk.y, e.acc.seat) } })
    if (e.inside) {
      items.push(employeeItem(g, e, now, at))
      if (e.there) { using.add(e.goal.id); watching ||= e.want?.act === 'tv' }
    } else e.there = e.atDesk = false
    // subagents: small ones standing in the aisles beside the desk, the running ones first, then the newest
    if (online) agentsShown(e.s).forEach((a, i) => {
      const ax = e.desk.x + (i % 2 ? 35 : -13), ay = e.desk.y + 2 + Math.floor(i / 2) * 8
      const busy = a.state === 'tool' || a.state === 'thinking'
      const bob = busy && !motion.still && Math.floor((now + i * 90) / 200) % 2 ? -1 : 0
      const look = agentDone(a) ? GRAY_LOOK : lookOf(e.acc)
      items.push({ y:ay + 5, draw:() => drawMiniPerson(g, ax + 1, ay - 2 + bob, look) })
    })
  }
  for (const f of figs.values()) { // a task's members, in its meeting room or on their way
    if (f.inside) items.push(employeeItem(g, f, now, at))
    else f.there = false
  }
  // furniture that comes alive while it's used, the sofa backrests over whoever sits there, and a tag's burst
  const { tv, foos, printer, sofa } = geo
  if (watching) items.push({ y:tv.y + 19, draw:() => drawTVShow(g, tv.x, tv.y, now) })
  if (using.has('fb0') && using.has('fb1')) items.push({ y:foos.y + 13, draw:() => drawFoosBall(g, foos.x, foos.y, now) })
  if (using.has('print')) items.push({ y:printer.y + 21, draw:() => drawPaper(g, printer.x, printer.y, now) })
  items.push({ y:sofa.y + 24, draw:() => drawSofaBack(g, sofa) })
  if (!motion.still && now - lift.at < 1600) items.push({ y:WALL_H - 1, draw:() => drawLift(g, door.x + (lift.no % 2 ? 3 : -25), WALL_H - 34, true) })
  for (const { pow } of tagGames.values()) {
    if (pow && now - pow.at < 500) items.push({ y:Infinity, draw:() => drawPow(g, Math.round(pow.x), Math.round(pow.y), (now - pow.at) / 500) })
  }
  items.sort((a, b) => a.y - b.y).forEach(it => it.draw())

  // envelopes in flight: an arc from the sender's head to the recipient's
  for (let i = flights.length - 1; i >= 0; i--) {
    const f = flights[i]
    f.t += motion.still ? 1 : dt / 1.6 // (motion paused: delivered at once)
    const p = Math.min(f.t, 1)
    const x = f.from.x + (f.to.x - f.from.x) * p, y = f.from.y + (f.to.y - f.from.y) * p - Math.sin(p * Math.PI) * 24
    drawEnvelope(g, Math.round(x) - 4, Math.round(y) - 3)
    if (f.t >= 1) { flights.splice(i, 1); mailAt.set(f.toKey, now) }
  }

  hair = 0
  ctx.fillStyle = '#2f2e2b'
  ctx.fillRect(0, 0, cv.width, cv.height)
  const bw = W * D, bh = H * D, k = S / D // screen pixels to a pixel of the drawing
  if (k === Math.floor(k)) {
    ctx.imageSmoothingEnabled = false
    ctx.drawImage(buf, 0, 0, bw, bh, offX, offY, W * S, H * S)
  } else if (k > 1) { // up by whole pixels first, then smoothly the rest of the way, so all pixels come out the same size
    const n = D === 1 ? Math.ceil(k) : Math.floor(k), bg = bigCtx // a drawing at D is big already
    if (big.width !== bw * n || big.height !== bh * n) { big.width = bw * n; big.height = bh * n }
    bg.imageSmoothingEnabled = false
    bg.drawImage(buf, 0, 0, bw * n, bh * n)
    ctx.imageSmoothingEnabled = true
    ctx.drawImage(big, 0, 0, bw * n, bh * n, offX, offY, W * S, H * S)
  } else { // finer than the screen: smoothly down
    ctx.imageSmoothingEnabled = true; ctx.imageSmoothingQuality = 'high'
    ctx.drawImage(buf, 0, 0, bw, bh, offX, offY, W * S, H * S)
  }
  placeLabels(now)
  if (!findList.hidden && now - (frame.listed || 0) > 500) { frame.listed = now; renderFind() } // what they're doing changes
  refreshChatLine()
}

// one employee in the building, as a depth-sorted item: its pose follows its state, where it is and what it does there
function employeeItem(g, e, now, at) {
  const online = isOnline(e), st = online ? e.s.state : 'ended', w = e.want || {}, spot = e.goal
  const t = (motion.still ? 0 : now) + e.phase, b = build(e.s) // the poses' clock: it stands while motion is paused
  e.there = !e.moving && !e.tag && !!spot && !e.path.length
  e.atDesk = e.there && spot.id === 'desk'
  const act = e.there ? w.act : null, pastime = e.atDesk && st === 'idle' ? deskPastime(e, at) : null
  const blink = !motion.still && now > e.blinkAt && now < e.blinkAt + 140
  if (now > e.blinkAt + 140) e.blinkAt = now + 2500 + Math.random() * 4000
  const x = Math.round(e.x * D) / D // at a finer pixel size, people move by finer steps
  let y = Math.round(e.y * D) / D, face = e.there ? spot.face : e.facing, extra = null
  const pose = { eyes:blink ? 'closed' : 'open', legs:!(e.there && spot.sit), step:-1, armL:0, armR:0 }
  const sip = Math.floor(t / 3000) % 3 === 0 // now and then a sip
  if (e.tag) {
    pose.step = Math.floor(t / 80) % 2
    if (e.tag.wait) pose.eyes = 'closed' // counting
    else if (e.tag.it) pose.raiseL = pose.raiseR = true
  } else if (e.moving) {
    pose.step = Math.floor(t / (w.hurry ? 90 : 150)) % 2
  } else if (e.atDesk) {
    const typing = st === 'tool'
    if (pastime === 'sleep') { y += 1; pose.eyes = 'closed' } else if (pastime && pastime !== 'ready' && !blink) pose.eyes = 'down'
    pose.armL = typing ? Math.floor(t / 140) % 2 : pastime === 'game' ? Math.floor(t / 120) % 2 : 0
    pose.armR = typing ? 1 - Math.floor(t / 140) % 2 : pastime === 'phone' ? -2 : pastime === 'game' ? Math.floor(t / 170) % 2 : 0
    pose.raiseL = st === 'waiting'
    extra = top => {
      if (pastime === 'phone') drawPhone(g, x, y, t)
      else if (pastime === 'game') drawConsole(g, x, y, t)
      else if (pastime === 'sleep') drawZzz(g, x, top, b, t)
      if (st === 'thinking') drawThinking(g, x, top, t)
      if (st === 'waiting') drawAlert(g, x + (b.bw >> 1) + b.armW + 2, top - 3 - (Math.floor(t / 400) % 2))
    }
  } else if (e.there) {
    switch (act) {
      case 'nap':
        if (face > 0) { y += 1; pose.eyes = 'closed' } // head down on the table
        extra = top => drawZzz(g, x, top, b, t)
        break
      case 'tv':
        if (Math.floor(t / 600) % 12 === 0) pose.raiseL = pose.raiseR = true // what a goal
        break
      case 'coffee': case 'water':
        pose.armR = sip ? -2 : 0
        extra = () => act === 'coffee' ? drawMug(g, handOf(x, y, b, pose.armR), t) : drawCup(g, handOf(x, y, b, pose.armR))
        break
      case 'vending':
        if (now - e.arrived < 4000) { face = -1; pose.armR = Math.floor(t / 250) % 2 ? -1 : 0 } // choosing, pressing buttons
        else { pose.armR = sip ? -2 : 0; extra = () => drawCan(g, handOf(x, y, b, pose.armR)) }
        break
      case 'books':
        if (!blink) pose.eyes = 'down'
        extra = () => drawBook(g, x, y, b)
        break
      case 'foosball':
        pose.armL = Math.floor(t / 110) % 2; pose.armR = 1 - Math.floor(t / 130) % 2
        if (Math.floor(t / 700) % 9 === 0) pose.raiseL = pose.raiseR = true // scored
        break
      case 'chat':
        extra = top => {
          const say = chatLine(e, w.pair, now)
          if (say) drawSay(g, spot.id === 'rt0' ? x - (b.bw >> 1) - b.armW - 10 : x + (b.bw >> 1) + b.armW + 1, top + 1, say)
        }
        break
      case 'task': { // at the meeting table: thinking, hands busy while it works, a hand up and the sign while either of its
        // sessions waits for you, now and then a look at its notes while it waits for the others; the hand and the sign go
        // up on the side away from the table, so the sign never lands on it (west of the table: the left)
        const red = st === 'waiting' || asking(e.acc.session), west = spot.side === 'w'
        if (st === 'tool') { pose.armL = Math.floor(t / 140) % 2; pose.armR = 1 - Math.floor(t / 140) % 2 }
        else if (st === 'idle' && !blink && Math.floor(t / 4000) % 3 === 0) pose.eyes = 'down'
        pose.raiseL = red && spot.side !== 'e'; pose.raiseR = red && spot.side === 'e'
        extra = top => {
          if (st === 'thinking') drawThinking(g, x, top, t)
          if (red) drawAlert(g, west ? x - (b.bw >> 1) - b.armW - 9 : x + (b.bw >> 1) + b.armW + 2, top - 3 - (Math.floor(t / 400) % 2))
        }
        break
      }
      // window, print, and tag (waiting at the corner for the other): just standing there
    }
  }
  if (face < 0) pose.eyes = 'none'
  const lit = online || e.goal?.id === 'home' // a member on its way back to its desk session
  const look = lit ? lookOf(e.acc) : GRAY_LOOK
  const dim = filtering() && !picked(e), ring = (filtering() && !dim) || now - (e.foundAt ?? -1e9) < 4000
  return { y:y + 10, draw:() => {
    if (dim) g.globalAlpha = 0.3
    if (detail && pose.legs && hair) solid(g, x - (b.bw >> 1) - 1, y + 1, b.bw + 2, 2 * hair, 'rgba(43,42,40,.18)') // a shadow
    if (ring) drawRing(g, x, y, b, now)
    drawPerson(g, x, y, b, pose, look)
    if (e.member?.advisor && face > 0) solid(g, x + 1, y - 3, 2, 2, '#fbfbf8') // an advisor's visitor badge, on the chest
    const top = y - b.bh
    extra?.(top)
    if (now - (e.popAt ?? -1e9) < 1200) drawSay(g, x + (b.bw >> 1) + b.armW + 1, top - 1, '!') // called back
    const got = mailAt.get(e.key)
    if (got && now - got < 4000) drawEnvelope(g, x - 4, top - 10 - (Math.floor(now / 300) % 2))
    g.globalAlpha = 1
  } }
}

// a member's tag at its task's table: above its head, else beside that to the left or the right, then a row up and the
// same again, then below its feet; always within the room's width, between its walls (a room on the back wall: up to
// the wall's pictures) and below the HUD. With no free place, where it covers the fewest other tags.
function tableTag(e, room, placed) {
  const [rl, rt] = toCss(room.x, room.north && room.y === WALL_H ? DECOR : room.y), [rr, rb] = toCss(room.x + room.w, room.y + room.h)
  const top = Math.max(rt, hudBottom) + 2 + e.lh, bottom = rb - 2, half = e.lw / 2, s = half + 2, step = e.lh + 1
  const inX = x => Math.max(rl + half + 1, Math.min(rr - half - 1, x))
  const ys = []
  for (let y = Math.min(e.ly, bottom); y >= top && ys.length < 4; y -= step) ys.push(y)
  for (let y = Math.max(toCss(0, e.y + 3)[1] + step, top); y <= bottom && ys.length < 8; y += step) ys.push(y)
  if (!ys.length) ys.push(Math.max(top, Math.min(e.ly, bottom)))
  ;[e.tx, e.ty] = bestPlace(e.lw, e.lh, ys.flatMap(y => [0, -s, s, -2 * s, 2 * s].map(dx => [inX(e.lx + dx), y])), placed)
}
// the first of the places (x the middle, y the bottom of a w x h box) that covers none of the boxes placed, else the one
// that covers the least of them; [x, y, how much it covers]
function bestPlace(w, h, places, placed) {
  let best = [...places[0], Infinity], least = Infinity
  for (const [x, y] of places) {
    const l = x - w / 2, r = x + w / 2, t = y - h
    let n = 0
    for (const p of placed) n += Math.max(0, Math.min(p.r, r) - Math.max(p.l, l)) * Math.max(0, Math.min(p.b, y) - Math.max(p.t, t))
    if (!n) return [x, y, 0]
    if (n < least) { best = [x, y, n]; least = n }
  }
  return best
}

function headOf(e) {
  const b = build(e.s)
  return e.inside ? { x:Math.round(e.x), y:Math.round(e.y) - b.bh - 4 } : { x:e.desk.x + 16, y:e.desk.y - 12 }
}

const setData = (el, key, value) => { if (el.dataset[key] !== value) el.dataset[key] = value }

// Name tags follow their employees. Away from its desk a tag shows just the name, and a tag that stays put
// (on a sofa, at the meeting table) moves up out of the way of the tags already there. A task's members have tags too
// (while they're in the building); at their table the tags go beside each other first, then up, inside the room. The
// tag of an employee away in a task stays at its desk.
function placeLabels(now) {
  const maxW = Math.max(60, COL_PITCH * S / dpr - 6), placed = [], loose = []
  const tight = maxW < 70 // a desk's tag with room for a few letters of the status only: the name alone
  const box = (e, x, y) => ({ l:x - e.lw / 2, r:x + e.lw / 2, t:y - e.lh, b:y })
  for (const f of figs.values()) if (f.label.hidden === f.inside) { f.label.hidden = !f.inside; f.sized = false }
  const tagged = [...emps.values(), ...figs.values()].filter(e => e.desk || e.inside)
  for (const e of tagged) { // what changes a tag's size, written first; the sizes are read below, all at once
    const el = e.label, away = e.inside && !e.atDesk, mw = e.member ? Math.max(90, maxW) : maxW // a member's tag has no desk to fit
    if (el.classList.contains('away') !== away) { el.classList.toggle('away', away); e.sized = false }
    if (e.maxW !== mw) { el.style.maxWidth = mw + 'px'; e.maxW = mw; e.sized = false }
    if (el.classList.contains('tight') !== (tight && !e.member)) { el.classList.toggle('tight', tight && !e.member); e.sized = false }
  }
  for (const e of tagged) {
    if (!e.sized) { e.lw = e.label.offsetWidth; e.lh = e.label.offsetHeight; e.sized = true }
    if (e.bubble && !e.bubbleSized) { e.bubW = e.bubble.offsetWidth; e.bubH = e.bubble.offsetHeight; e.bubbleSized = true }
  }
  for (const e of tagged) {
    const el = e.label, away = e.inside && !e.atDesk
    const h = headOf(e)
    ;[e.lx, e.ly] = toCss(h.x, h.y - 2)
    e.tx = e.lx; e.ty = e.ly
    if (!away) placed.push(box(e, e.lx, e.ly))
    else if (!e.moving) loose.push(e)
    // for the tests and the curious: what it's up to, and where
    setData(el, 'act', e.inside ? e.want?.act ?? '' : '')
    setData(el, 'room', e.inside ? e.goal?.room ?? '' : '')
    setData(el, 'spot', e.inside ? e.goal?.id ?? '' : '')
    setData(el, 'moving', e.inside && e.moving ? '1' : '0')
    const hit = filtering() && picked(e)
    el.classList.toggle('dim', filtering() && !hit)
    el.classList.toggle('match', hit)
    el.classList.toggle('found', now - (e.foundAt ?? -1e9) < 3200)
    const doing = doingText(e)
    const title = e.status + (doing ? ' · ' + doing : '') + (doing && away ? ' · ' + t('office.callBack') : '')
    if (el.title !== title) el.title = title
  }
  loose.sort((a, b) => b.ly - a.ly)
  for (const e of loose) {
    const room = e.member && e.want?.act === 'task' && roomOf(e.task)
    if (room) tableTag(e, room, placed)
    else for (let k = 0; k < 12; k++) {
      const me = box(e, e.tx, e.ty), hit = placed.find(p => p.l < me.r && me.l < p.r && p.t < me.b && me.t < p.b)
      if (!hit) break
      e.ty = hit.t - 1
    }
    placed.push(box(e, e.tx, e.ty))
  }
  // the rooms' nameplates, which the bubbles keep off too
  smalls.forEach((r, i) => {
    const d = smallDoor(r), [x, y] = toCss(d.plateX, d.plateY), maxW = Math.max(40, (r.x + r.w - 4 - d.plateX) * S / dpr) + 'px', p = plates[i]
    p.style.left = x + 'px'; p.style.top = y + 'px'
    if (p.style.maxWidth !== maxW) { p.style.maxWidth = maxW; fitPlate(p) }
    if (p.pw) placed.push({ l:x, r:x + p.pw, t:y - p.ph / 2, b:y + p.ph / 2 })
  })
  // message bubbles, after the tags, the newest first: above the sender's tag, else beside it or higher, else below the
  // figure; inside the window and below the HUD. Where there's no room for one (it would cover a quarter of itself or
  // more), it doesn't show: the envelope and the message log have the message
  for (const e of tagged.filter(e => e.bubble && e.bubbleSized).sort((a, b) => b.bubbleAt - a.bubbleAt)) {
    const w = e.bubW, h = e.bubH, s = w / 2 + 2, step = h + 2, top = hudBottom + 2 + h
    const inX = x => Math.max(w / 2 + 2, Math.min(innerWidth - w / 2 - 2, x)), ys = []
    for (let y = e.ty - e.lh - 2, k = 0; k < 4 && y >= top; k++, y -= step) ys.push(y)
    for (let y = Math.max(e.ly + e.lh + h + 24, top), k = 0; k < 2 && y <= innerHeight - 2; k++, y += step) ys.push(y)
    if (!ys.length) ys.push(top)
    const at = bestPlace(w, h, ys.flatMap(y => [0, -s, s].map(dx => [inX(e.tx + dx), y])), placed), shown = at[2] < w * h / 4
    ;[e.bx, e.by] = at
    const vis = shown ? '' : 'hidden'
    if (e.bubble.style.visibility !== vis) e.bubble.style.visibility = vis
    if (shown) placed.push({ l:at[0] - w / 2, r:at[0] + w / 2, t:at[1] - h, b:at[1] })
  }
  for (const e of tagged) {
    e.label.style.left = e.tx + 'px'; e.label.style.top = e.ty + 'px'
    if (e.bubble && e.bubbleSized) { e.bubble.style.left = e.bx + 'px'; e.bubble.style.top = e.by + 'px' }
  }
  for (const [key, el] of Object.entries(roomEls)) {
    const r = rooms[key]
    if (!r) continue
    // the open office's sign goes bottom left; top left would be hidden by the first row's name tags
    const [x, y] = key === 'sign' ? toCss(workX + (COLS * COL_PITCH) / 2 + 6, DECOR + 22)
      : key === 'work' ? toCss(r.x + 3, r.y + r.h - 12) : toCss(r.x + 3, r.y + 3)
    el.style.left = x + 'px'; el.style.top = y + 'px'
  }
  addBtn.hidden = addSeat < 0
  if (addSeat >= 0) {
    const d = deskOf(addSeat)
    const [ax, ay] = toCss(d.x + 16, d.y - 10)
    addBtn.style.left = ax + 'px'; addBtn.style.top = ay + 'px'
  }
}

// ================= search =================
// Finds employees by name (or session name, or folder): the matches light up in the office and show in a list
// under the box; picking one (a click, or Enter for the highlighted one) is the same as clicking that employee.
// It finds a task's members by their names too (bob-T7), and tasks by id, room or description (picking one opens it).
const findEl = document.getElementById('find'), findList = document.getElementById('find-list')
let findQ = '', findSel = 0
// a state chip in the HUD, clicked: its employees light up too ('idle' | 'busy' | 'waiting' | 'off'), until a second click
let chipOn = null
const groupOf = e => !isOnline(e) ? 'off' : e.s.state === 'idle' ? 'idle' : e.s.state === 'waiting' ? 'waiting' : 'busy'
// with a search or a chip on, the employees they pick out get a ring and the rest dims
const filtering = () => !!findQ || !!chipOn
const picked = e => (!findQ || matchesFind(e, findQ)) && (!chipOn || groupOf(e) === chipOn)
// a folder matches by its last part ("symphony"), or by its whole path once the query has a slash in it
const matchesFind = (e, q) => (e.member ? [e.member.name] : [e.acc.name, e.s?.title, ...[e.s?.cwd, e.acc.last?.dir, e.acc.dir].map(d => d && (/[\\/]/.test(q) ? d : base(d)))])
  .some(v => lower(v).includes(q))
const TASK_DOT = { running:'tool', ending:'tool', failed:'waiting' }
// the hits: { key, name, e } for an employee or a member, { key, name, task } for a task
function findHits() {
  if (!findQ) return []
  const rank = h => lower(h.name).startsWith(findQ) ? 0 : lower(h.name).includes(findQ) ? 1 : 2
  const people = [...emps.values(), ...[...figs.values()].filter(f => f.inside && !f.leaving)].filter(e => matchesFind(e, findQ))
    .map(e => ({ key:e.key, name:e.member ? e.member.name : e.acc.name, e }))
  const tasks = snapshot.tasks.filter(task => [task.id, taskRoom(task), task.title].some(v => lower(v).includes(findQ)))
    .map(task => ({ key:'task:' + task.id, name:`${task.id} · ${taskRoom(task)}`, task }))
  return [...people, ...tasks].sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name, lang)).slice(0, 8)
}
function renderFind() {
  findQ = lower(findEl.value.trim())
  const hits = findHits()
  findSel = Math.max(0, Math.min(findSel, hits.length - 1))
  findList.hidden = !findQ || document.activeElement !== findEl
  // (drawn in place, setHTML's keep: the hit the box points at stays the element it was, so a screen reader doesn't
  // read it again at every redraw; a space between the name and its line, so they aren't read as one word)
  setHTML(findList, hits.map((h, i) => `<div class="hit${i === findSel ? ' on' : ''}" data-key="${esc(h.key)}" role="option" id="find-hit-${i}" aria-selected="${i === findSel}">` +
    (h.task ? `${dot(TASK_DOT[h.task.status])}<b>${esc(h.name)}</b> <small>${esc(h.task.title)}</small></div>`
      : `${dot(isOnline(h.e) ? h.e.s.state : 'ended')}<b>${esc(h.name)}</b> <small>${esc(stateLine(h.e))}</small></div>`)).join('') ||
    `<div class="none" role="option" aria-disabled="true">${esc(t('search.none'))}</div>`, true)
  // for screen readers: the list open or not, and the hit the arrows are on
  findEl.setAttribute('aria-expanded', String(!findList.hidden))
  pointAt(findEl, !findList.hidden && hits.length ? 'find-hit-' + findSel : null)
}
function pickFound(h) {
  findEl.value = ''; findSel = 0; findEl.blur(); renderFind()
  if (!h) return
  if (h.task) return openTaskWin(h.task.id)
  h.e.foundAt = performance.now()
  if (h.e.member) openMember(h.e.task, h.e.member); else onEmployeeClick(h.e.acc.id)
}
findEl.addEventListener('input', () => { findSel = 0; renderFind() })
findEl.addEventListener('focus', renderFind)
findEl.addEventListener('blur', () => { findList.hidden = true; findEl.setAttribute('aria-expanded', 'false'); findEl.removeAttribute('aria-activedescendant') })
findEl.addEventListener('keydown', ev => {
  if (ev.key === 'ArrowDown' || ev.key === 'ArrowUp') { findSel += ev.key === 'ArrowDown' ? 1 : -1; renderFind() }
  else if (ev.key === 'Enter') pickFound(findHits()[findSel])
  else if (ev.key === 'Escape') { findEl.value = ''; renderFind(); findEl.blur() }
  else return
  ev.preventDefault()
})
findList.addEventListener('mousedown', ev => { // mousedown, so the box keeps its focus until the pick
  const row = ev.target.closest('.hit')
  if (!row) return
  ev.preventDefault()
  pickFound(findHits().find(h => h.key === row.dataset.key))
})
addEventListener('keydown', ev => { // "/" jumps to the search box; Esc lets go of a state chip
  if (ev.key === 'Escape' && chipOn) { chipOn = null; renderStats() }
  if (ev.key === 'Escape' && !chat.hidden) { if (!chat.asking) closeChat() } // the member's first, then its task
  else if (ev.key === 'Escape' && !tasks.win.hidden) closeTaskWin()
  if (ev.key === 'Escape' && !tasks.dialogHidden) tasks.closeDialog()
  if (ev.key === 'Escape' && !launch.hidden) launch.close()
  if (ev.key !== '/' || ev.ctrlKey || ev.metaKey || ev.altKey || !chat.hidden || !launch.hidden || !tasks.dialogHidden || !tasks.win.hidden) return
  if (/^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement?.tagName)) return
  ev.preventDefault()
  if (hud.classList.contains('up')) setHud(false)
  findEl.focus()
})

// ================= state chips =================
// The HUD's counts by state, one chip each in the state's color; the number stands out wherever the language puts it.
const numbered = (key, n) => t(key, { n:'\u0001' }).split('\u0001').map(esc).join(`<b>${n}</b>`)
const CHIPS = [['idle', 'idle', 'stats.idle'], ['busy', 'tool', 'stats.working'], ['waiting', 'waiting', 'stats.waiting'], ['off', 'ended', 'stats.offDuty']]
// While asks wait for you, the pending chip turns red and opens them. (Not while a pointer is down: a chip replaced under
// a press loses its click.)
const renderStats = deferWhilePressed(() => {
  const el = document.getElementById('stats'), badge = hudTab.querySelector('.badge')
  badge.hidden = !snapshot.asks.length
  badge.textContent = snapshot.asks.length
  labelTab()
  if (!snapshot.accounts.length) { chipOn = null; setHTML(el, esc(t('office.nobody'))); return }
  const n = { idle:0, busy:0, waiting:0, off:0 }
  for (const e of emps.values()) n[groupOf(e)]++
  for (const f of figs.values()) if (isOnline(f)) n[groupOf(f)]++ // task sessions count like desk sessions
  setHTML(el, CHIPS.map(([group, state, key]) => { // (the same chips stay the elements they were: a click pressed on one lands)
    const hot = group === 'waiting' && snapshot.asks.length > 0
    return `<button class="chip${hot ? ' hot' : ''}${chipOn === group ? ' on' : ''}" data-group="${group}" style="--c:${STATE_MARK[state]}"${hot ? '' : ` aria-pressed="${chipOn === group}"`} ` +
      `title="${esc(hot ? t('asks.title', { n:snapshot.asks.length }) : t('stats.focusTitle'))}">${numbered(key, n[group])}</button>`
  }).join(''))
})
document.getElementById('stats').addEventListener('click', ev => {
  const chip = ev.target.closest('.chip')
  if (!chip) return
  if (chip.classList.contains('hot')) return openAsks()
  chipOn = chipOn === chip.dataset.group ? null : chip.dataset.group
  renderStats()
})

// ================= the HUD's tab =================
// The HUD slides up out of the way and back down; the page remembers which, and the side panels start below it.
const hud = document.getElementById('hud'), hudTab = document.getElementById('hud-tab'), HUD_STORE = 'symphony.hud'
function setHud(up, remember = true) {
  hud.classList.toggle('up', up)
  hudTab.title = t(up ? 'hud.show' : 'hud.hide')
  hudTab.setAttribute('aria-expanded', String(!up))
  labelTab()
  if (remember) try { localStorage.setItem(HUD_STORE, up ? 'up' : 'down') } catch {}
  hudRoom()
}
// the tab's name for screen readers: what it does, and, while the HUD is away, how many asks wait (its badge)
function labelTab() {
  const up = hud.classList.contains('up'), n = snapshot.asks.length
  hudTab.setAttribute('aria-label', t(up ? 'hud.show' : 'hud.hide') + (up && n ? ' · ' + t('asks.title', { n }) : ''))
}
let hudBottom = 0 // where the HUD ends, in CSS pixels from the top; tags at a task's table keep below it
const hudRoom = () => { hudBottom = hud.classList.contains('up') ? 0 : hud.offsetHeight; document.documentElement.style.setProperty('--hud-h', hudBottom + 'px') }
hudTab.addEventListener('click', () => setHud(!hud.classList.contains('up')))
new ResizeObserver(hudRoom).observe(hud)

// ================= pixel size =================
// Four steps from the standard drawing to the finest: the same building, drawn with finer pixels and more detail (see
// fitScreen). The page remembers the step.
const DETAIL_STEPS = 4, DETAIL_STORE = 'symphony.pixels'
let detail = (() => {
  try { const n = Number(localStorage.getItem(DETAIL_STORE)); return Number.isInteger(n) && n >= 0 && n < DETAIL_STEPS ? n : 0 } catch { return 0 }
})()
const pixelsEl = document.createElement('div')
pixelsEl.className = 'group pixels'
pixelsEl.title = t('pixels.title')
pixelsEl.innerHTML = `<span class="lbl" id="pixels-label">${esc(t('pixels.label'))}</span><div class="steps" role="group" aria-labelledby="pixels-label">` +
  Array.from({ length:DETAIL_STEPS }, (_, i) => `<button data-level="${i}" title="${esc(t('pixels.' + (i + 1)))}" aria-label="${esc(t('pixels.' + (i + 1)))}"></button>`).join('') + '</div>'
function setDetail(level) {
  detail = level
  try { localStorage.setItem(DETAIL_STORE, String(level)) } catch {}
  markDetail()
  layout()
}
const markDetail = () => pixelsEl.querySelectorAll('button').forEach((b, i) => {
  b.classList.toggle('on', i === detail)
  b.setAttribute('aria-pressed', String(i === detail))
})
pixelsEl.addEventListener('click', ev => {
  const b = ev.target.closest('button[data-level]')
  if (b) setDetail(Number(b.dataset.level))
})
markDetail()

// ================= small meeting rooms' nameplates =================
// Each nameplate shows the room's name and a light: green while the room is free, yellow while a task runs in it (its id
// before the name, the id alone on a narrow plate), red while one of its members waits for you, blinking red with an "!"
// after the id when the task failed. A click on it, or on the room, opens the task window, or the task dialog for a free
// room when the page can start sessions.
const plates = []
function syncPlates() {
  while (plates.length > smalls.length) plates.pop().remove()
  while (plates.length < smalls.length) {
    const el = document.createElement('div')
    el.className = 'room-plate'
    el.setAttribute('aria-hidden', 'true') // (the roster says it)
    el.addEventListener('click', () => { const r = smalls[plates.indexOf(el)]; if (r) onRoomClick(r) })
    document.getElementById('labels').appendChild(el)
    plates.push(el)
  }
  smalls.forEach((r, i) => {
    const el = plates[i], name = roomName(r.k), task = roomTask(r.k), failed = task?.status === 'failed'
    const light = !task ? 'free' : failed || task.members.some(m => asking(m.session)) ? 'alert' : 'busy'
    const text = task ? `${task.id}${failed ? '!' : ''} · ${name}` : name, click = !!task || !!launchOpts?.enabled
    const title = `${name} · ${t('room.seats', { n:r.seats })} · ` +
      (task ? `${t(task.status === 'running' ? 'room.busy' : 'task.status.' + task.status)} · ${task.id}${t('common.colon')}${task.title}` : t('room.free'))
    const sig = `${text}|${r.seats}|${light}|${click}|${title}`
    if (el.dataset.sig === sig) return
    el.dataset.sig = sig; el.dataset.seats = r.seats; el.dataset.light = light
    if (task) el.dataset.task = task.id; else delete el.dataset.task
    el.className = `room-plate ${light}${failed ? ' failed' : ''}${click ? ' click' : ''}`
    el.innerHTML = task ? `<i></i><span class="id">${esc(task.id)}${failed ? '<b class="bang">!</b>' : ''}</span><span class="nm"> · ${esc(name)}</span>` : `<i></i>${esc(name)}`
    el.title = title
    if (el.style.maxWidth) fitPlate(el)
  })
  renderRoster()
}
// a plate too narrow for its text goes a step smaller; one with a task still too narrow for the id and the name shows the
// id alone. Its size is kept for the bubbles, which keep off the plates
function fitPlate(el) {
  el.classList.remove('narrow', 'small')
  const over = () => el.scrollWidth > el.clientWidth
  if (over()) el.classList.add('small')
  if (over() && el.dataset.task) { el.classList.replace('small', 'narrow'); if (over()) el.classList.add('small') }
  el.pw = el.offsetWidth; el.ph = el.offsetHeight
}
function onRoomClick(r) {
  const task = roomTask(r.k)
  if (task) openTaskWin(task.id)
  else if (launchOpts?.enabled) tasks.openDialog({ room:{ k:r.k, name:roomName(r.k) } })
}

// ================= the roster =================
// The office for the keyboard and screen readers, to whom the canvas says nothing: a button for every employee, in seat
// order, saying what it's doing (a click opens its window, as on the employee), and one for every small meeting room,
// saying what's in it (a click opens its task, or the task dialog). Out of sight until a key brings the focus into it;
// the employee whose button has the focus lights up in the office.
const roster = document.getElementById('roster')
function renderRoster() {
  const desks = [...emps.values()].sort((a, b) => a.acc.seat - b.acc.seat)
    .map(e => `<li><button type="button" data-account="${esc(e.acc.id)}">${esc(e.status ?? e.acc.name)}</button></li>`).join('')
  const rooms = smalls.map(r => {
    const task = roomTask(r.k), name = roomName(r.k)
    const text = task ? `${name} · ${task.id}${t('common.colon')}${task.title.split('\n')[0]} · ${t(task.status === 'running' ? 'room.busy' : 'task.status.' + task.status)} · ${task.members.map(m => m.name).join(', ')}`
      : `${name} · ${t('room.seats', { n:r.seats })} · ${t('room.free')}`
    return `<li><button type="button" data-room="${r.k}"${task || launchOpts?.enabled ? '' : ' aria-disabled="true"'}>${esc(text)}</button></li>`
  }).join('')
  setHTML(roster.querySelector('.desks'), desks)
  setHTML(roster.querySelector('.rooms'), rooms)
  // the employee whose button has the focus: its button changes its words in place, which a screen reader doesn't read
  // again by itself; a change of state is said (politely), not every tool call
  const e = emps.get(document.activeElement?.dataset?.account), level = e && levelOf(e)
  if (e && heard?.id === e.acc.id && heard.level !== level) announce(e.status ?? e.acc.name)
  heard = e ? { id:e.acc.id, level } : null
}
const levelOf = e => !isOnline(e) ? 'off' : asking(e.acc.session) ? 'waiting' : ['thinking', 'tool'].includes(e.s.state) ? 'working' : e.s.state
let heard = null // the employee whose roster button has the focus, and its state as last said
roster.addEventListener('click', ev => {
  const b = ev.target.closest('button')
  if (!b || b.getAttribute('aria-disabled') === 'true') return
  if (b.dataset.account) onEmployeeClick(b.dataset.account)
  else { const r = smalls.find(x => String(x.k) === b.dataset.room); if (r) onRoomClick(r) }
})
roster.addEventListener('focusin', ev => { const e = emps.get(ev.target.dataset?.account); if (e) { e.foundAt = performance.now(); heard = { id:e.acc.id, level:levelOf(e) } } })

// ================= data =================
// the activity log: hidden until its HUD button opens it (the page remembers), the last 200 lines
const logEl = document.getElementById('log'), logLines = logEl.querySelector('.lines'), LOG_STORE = 'symphony.log'
logLines.dataset.empty = t('log.empty')
for (const b of document.querySelectorAll('#log .close, #msgs-panel .close')) b.setAttribute('aria-label', t('common.close'))
// While the panel is hidden the lines only go into a list (no elements, no measuring: under load this runs 100 times a
// second); opening the panel puts them in
const logKept = [] // the last 200 lines
let logStale = false // the panel's lines are behind logKept
function log(text) {
  const line = `${new Date().toLocaleTimeString(lang)}  ${text}`
  logKept.push(line)
  if (logKept.length > 200) logKept.shift()
  if (logEl.hidden) { logStale = true; return }
  const d = document.createElement('div')
  d.textContent = line
  const atEnd = logLines.scrollTop + logLines.clientHeight >= logLines.scrollHeight - 4
  logLines.appendChild(d)
  while (logLines.children.length > 200) logLines.firstChild.remove()
  if (atEnd) logLines.scrollTop = logLines.scrollHeight
}
function setLog(open) {
  logEl.hidden = !open
  try { localStorage.setItem(LOG_STORE, open ? 'open' : 'closed') } catch {}
  if (open && logStale) drawLog()
  if (open) logLines.scrollTop = logLines.scrollHeight
}
function drawLog() {
  logStale = false
  logLines.replaceChildren(...logKept.map(line => { const d = document.createElement('div'); d.textContent = line; return d }))
}
// The bridge keeps the last lines of what happened (/api/activity): the panel starts with those of the kinds it shows
// (on and off duty, tool calls, messages), worded as it words them, so a reload doesn't empty it
const pageOpened = Date.now()
sym.fetchActivity().then(items => {
  const before = (items ?? []).filter(a => Date.parse(a.time) < pageOpened).map(a => {
    const at = `${new Date(a.time).toLocaleTimeString(lang)}  `, who = a.name || '?'
    switch (a.kind) {
      case 'start': return `${at}${who}  ${t('log.onDuty')}`
      case 'end': return `${at}${who}  ${t('log.offDuty')}`
      case 'tool': return `${at}${who}  ${String(a.text || '').split(' ')[0]}` // the tool, as a live line names it
      case 'message': return `${at}${a.text || ''}`
    }
    return null
  }).filter(Boolean)
  if (!before.length) return
  logKept.unshift(...before.slice(-200))
  while (logKept.length > 200) logKept.shift()
  if (logEl.hidden) logStale = true
  else { drawLog(); logLines.scrollTop = logLines.scrollHeight }
})
document.getElementById('open-log').onclick = () => setLog(logEl.hidden)
logEl.querySelector('.close').onclick = () => setLog(false)
setLog((() => { try { return localStorage.getItem(LOG_STORE) === 'open' } catch { return false } })())

// whoever is a session in the building: a task session's member figure, or the employee of a desk session
const empBySession = id => [...figs.values()].find(f => f.s?.id === id && f.inside) ?? [...emps.values()].find(e => e.s && e.s.id === id)

// messages between sessions: an envelope flies from sender to recipient, a bubble pops up over the sender,
// and the message log gets it
function showMessage(m) {
  const from = empBySession(m.from), to = empBySession(m.to)
  if (from && to) flights.push({ from:headOf(from), to:headOf(to), t:0, toKey:to.key })
  else if (to) mailAt.set(to.key, performance.now())
  if (!from) return
  from.bubble?.remove()
  const el = document.createElement('div')
  el.className = 'bubble'
  el.setAttribute('aria-hidden', 'true') // (the message log has it)
  el.style.visibility = 'hidden' // until placed
  el.innerHTML = `<b>→ ${esc(m.toName || '?')}</b>${esc(t('common.colon'))}${esc(m.text.length > 60 ? m.text.slice(0, 60) + '…' : m.text)}`
  document.getElementById('labels').appendChild(el)
  from.bubble = el; from.bubbleSized = false; from.bubbleAt = performance.now() // measured and placed next frame
  setTimeout(() => { if (from.bubble === el) from.bubble = null; el.remove() }, 6000)
}

// The message log lives in the client (symphony.js): a new message flies across the office and goes into the activity
// log, one that changed (paired, delivered) redraws the panels that show it
sym.on('message', (m, isNew) => {
  if (isNew && !sym.first) { showMessage(m); log(`${msgFrom(m)} → ${msgTo(m)}${t('common.colon')}${m.text}`) }
  if (!msgsPanel.hidden) renderMessages()
})
sym.on('messages', () => { if (!msgsPanel.hidden) renderMessages() })
// the names a message goes by; one you sent from a task's chat has no sender, and none to send to when it went to everyone
const fromYou = m => m.origin === 'page' && !m.fromName
const msgFrom = m => m.fromName || (fromYou(m) ? t('task.you') : '?')
// (a task's member is named for its task already: bob-T2; everyone in it, or a page-sent message, gets the task's id)
const msgTo = m => {
  const to = m.toName || (fromYou(m) && m.task ? t('task.chatAll') : '?')
  return m.task && !to.endsWith('-' + m.task) ? `${to} · ${m.task}` : to
}

// every snapshot: the employees for the accounts, the building for the seats, the figures for the tasks, the panels
sym.on('snapshot', snap => {
  const first = sym.first // the first snapshot places everyone; later ones walk them in
  const desks = snap.accounts.filter(a => !a.advisor) // an advisor has no desk: it's in the building as a task's member only
  const live = new Set(desks.map(a => a.id))
  for (const [id, e] of emps) if (!live.has(id)) dropEmployee(e)
  for (const acc of desks) {
    if (!emps.has(acc.id)) { const e = makeEmployee(acc); emps.set(acc.id, e); seatEmployee(e) }
    emps.get(acc.id).acc = acc
  }
  layout()
  assignRooms() // the tasks may have changed while the building didn't
  const away = awayInTasks()
  for (const acc of desks) updateEmployee(emps.get(acc.id), acc, !first, away.get(acc.id))
  syncFigures(!first)

  syncPlates() // (and the roster)
  renderStats()
  if (chatFor) showChat(chatFor)
  if (!msgsPanel.hidden) renderMessages()
  if (!findList.hidden) renderFind()
})

function applyEvent(ev, name) {
  // a task session goes by its member's name (bob-T7)
  const who = (name ? name + (ev.task ? '-' + ev.task : '') : ev.session.slice(0, 6)) + (ev.agentId ? ' / ' + t('common.subagent') : '')
  if (ev.kind === 'tool.start') log(`${who}  ${ev.tool}`)
  else if (ev.kind === 'session.start') log(`${who}  ${t('log.onDuty')}`)
  else if (ev.kind === 'session.end') log(`${who}  ${t('log.offDuty')}`)
}

// ================= pending asks =================
// They are handled in the employee window: the HUD's red chip opens the window of the first employee waiting.
// the card's top line: inside the employee's own window the name would only repeat, so it's the kind alone
function askWho(a) {
  return `<div class="who"><span class="name">${esc(askKind(a.kind))}</span>` +
    (a.agentId ? `<span class="kind">${esc(t('common.subagent'))}</span>` : '') + `</div>`
}
function openAsks(session = null) {
  const a = snapshot.asks.find(x => !session || x.session === session) || snapshot.asks[0]
  if (!a) return
  // a task session's ask: its member's window, beside the task window
  const task = taskById(sessionById(a.session)?.task) ?? snapshot.tasks.find(k => k.members.some(m => m.session === a.session))
  const member = task?.members.find(m => m.session === a.session)
  if (member) openMember(task, member)
  else openChat(a.account)
}

// ================= message log =================
const msgsPanel = document.getElementById('msgs-panel')
function msgHTML(m) {
  return `<div class="msg"><div class="meta">${fmtTime(m.time)} · <b>${esc(msgFrom(m))}</b> → <b>${esc(msgTo(m))}</b>
    ${m.delivered ? '' : `<span class="lost">${esc(t('msg.undelivered'))}</span>`}</div><div class="txt">${richText(m.text)}</div></div>`
}
// The panel shows the log newest first and is kept up to date in place: a new message is one element put in, an
// updated one (paired, delivered) is replaced, the rest stay (300 long messages make 600 KB of HTML; rebuilding
// them all on every message took a good part of a second)
let msgsRendered = -1 // the sym.messagesVersion the panel shows
const msgEls = new Map() // message id -> { el, sig }: its element, and what it was drawn from
const msgSig = m => `${m.fromName}|${m.toName}|${m.delivered}|${m.received}|${m.text.length}`
function renderMessages() {
  if (msgsRendered === sym.messagesVersion) return
  msgsRendered = sym.messagesVersion
  const box = msgsPanel.querySelector('.msgs'), list = snapshot.messages, ids = new Set(list.map(m => m.id))
  for (const [id, r] of msgEls) if (!ids.has(id)) { r.el.remove(); msgEls.delete(id) }
  box.querySelector('.none')?.remove()
  let prev = null
  for (let i = list.length - 1; i >= 0; i--) {
    const m = list[i], sig = msgSig(m)
    let r = msgEls.get(m.id)
    if (!r) { r = { el:element(msgHTML(m)), sig }; msgEls.set(m.id, r) }
    else if (r.sig !== sig) { const el = element(msgHTML(m)); r.el.replaceWith(el); r.el = el; r.sig = sig }
    if ((prev ? prev.nextSibling : box.firstChild) !== r.el) { if (prev) prev.after(r.el); else box.prepend(r.el) }
    prev = r.el
  }
  if (!list.length) box.insertAdjacentHTML('beforeend', `<div class="msg none">${esc(t('msg.none'))}</div>`)
}
document.getElementById('open-msgs').onclick = () => { renderMessages(); msgsPanel.hidden = false }
msgsPanel.querySelector('.close').onclick = () => { msgsPanel.hidden = true }

// ================= the employee window =================
// The window itself is shared (ui/chat.js): who they are at the top, the session as a chat below, the input at the
// bottom with the asks' cards over it. The office says what the head shows for an employee or a task member, keeps the
// window's place (alone in the middle, or docked beside its task's window) and the status line that follows what the
// employee does.
const chat = createChat(sym, { askWho, onClose: () => closeChat(), onTaskChip: id => openTaskWin(id) })
const chatEl = chat.el
let chatFor = null // the account whose window is open
let chatSid = null // the task session it shows; null: the account's desk session, whichever is current
function closeChat() {
  chatEl.hidden = true; chatFor = null
  if (!chatTask) return
  chatTask = null
  placeChat()
  if (tasks.windowFor) tasks.render() // its row lets go, and the task window slides back to the middle
}
// A task member's window belongs to its task's window: docked to its right, the two centered as a pair, or over it
// where the screen is too narrow for both. The element moves into the task window for that and back when it's alone.
let chatTask = null // the task whose window the member's window goes with; null: a window of its own
const chatHome = document.createComment('chat')
chatEl.before(chatHome)
const DOCK_MIN = 1250 // narrower than this, the member's window opens over the task window
function placeChat() {
  const taskWin = tasks.win, docked = !!chatTask && innerWidth >= DOCK_MIN
  if (docked ? chatEl.parentElement !== taskWin : chatHome.nextSibling !== chatEl) (docked ? taskWin.append(chatEl) : chatHome.after(chatEl))
  chatEl.classList.toggle('docked', docked)
  chatEl.classList.toggle('stacked', !!chatTask && !docked)
  taskWin.classList.toggle('with-chat', docked && !chatEl.hidden)
  if (!taskWin.hidden) tasks.growBox() // a new width: the placeholder may take more lines, or fewer
}
// the one way to a task member's window: the task window, with the member's beside it
function openMember(task, m) {
  if (!m?.session) return openTaskWin(task.id)
  if (tasks.windowFor !== task.id || tasks.win.hidden) openTaskWin(task.id)
  openChat(m.account, m.session, task.id)
}

// a click on an employee opens its window, and an idle one goes back to its desk and waits there for its next turn
function onEmployeeClick(id) {
  const acc = snapshot.accounts.find(a => a.id === id)
  const s = acc && sessionOf(acc), e = emps.get(id)
  if (e?.inside && s?.state === 'idle') {
    e.summon = { session:s.id, since:s.stateSince, at:Date.now() }
    e.popAt = performance.now()
  }
  openChat(id)
}
// sid: a task session to show; without one the window shows the desk session. task: the task window it goes with
// (openMember); without one it's a window of its own, and the task window closes
function openChat(id, sid = null, task = null) {
  if (!task) closeTaskWin()
  chatSid = sid; chatTask = task
  showChat(id)
  if (task) tasks.render()
  chat.focus()
}

function showChat(id) {
  const acc = snapshot.accounts.find(a => a.id === id)
  if (!acc || (!chatSid && !emps.has(id))) { closeChat(); return }
  chatFor = id
  const s = chatSid ? sessionById(chatSid) : deskSessionOf(acc), online = !!s && s.state !== 'ended'
  // a task session's window: the member's name, the task (a click opens it), no seat; it can't start work or be deleted.
  // The desk window of an employee away in a task shows that task too, and a button to its window there; while it's away
  // it can't be deleted (the bridge refuses)
  const away = chatSid ? null : emps.get(id)?.away ?? null
  const task = chatSid ? taskById(s?.task) ?? snapshot.tasks.find(k => k.members.some(m => m.session === chatSid)) ?? null : away
  const member = chatSid ? task?.members.find(m => m.session === chatSid) : away?.members.find(m => m.account === id)
  const busy = online && (s.state === 'thinking' || s.state === 'tool' || s.state === 'waiting')
  const buttons = []
  if (online && s.launch) buttons.push({ key:'terminal', text:t('chat.terminal'), cls:'terminal', onclick:() => openTerminal(s), title:t('chat.terminalTitle') })
  if (busy) buttons.push({ key:'stop', text:t('chat.stop'), cls:'stop', onclick:() => post(`/api/abort/${s.id}`, {}), title:t('chat.stopTitle') })
  if (away && member?.session) buttons.push({ key:'to-task', text:t('task.openMember', { name:member.name }), cls:'primary to-task', onclick:() => openMember(away, member),
    title:t('task.openMemberTitle', { id:away.id, room:taskRoom(away) }) })
  if (!online && !chatSid && launchOpts?.enabled) buttons.push({ key:'go-work', text:t('launch.go'), cls:(away ? '' : 'primary ') + 'go-work', onclick:() => openLaunch(acc.id) })
  if (!online && !chatSid && !away) buttons.push({ key:'delete', text:t('card.delete'), cls:'', onclick:async () => {
    if (!confirm(t('card.deleteConfirm', { name:acc.name }))) return
    const r = await sym.deleteAccount(acc.id)
    if (!r.ok) alert(t('card.deleteFailed', { reason:errorText(r.data) }))
    closeChat()
  } })
  chat.show({
    name:chatSid ? member?.name || s?.title || acc.name : acc.name,
    state:online ? s.state : 'ended',
    line:chatStateLine(),
    taskChip:task ? { id:task.id, title:task.title, label:t('task.chip', { id:task.id, room:taskRoom(task) }) } : null,
    chips:[['seat', chatSid ? '' : t('card.seatNo', { n:acc.seat + 1 })], ['model', online ? buildText(s) : ''], ['session', s?.title],
      ['dir', s?.cwd || (chatSid ? member?.worktree : acc.last?.dir || acc.dir)], ['subagents', online ? s.agentsTotal || Object.keys(s.agents || {}).length || '' : ''], ['tools', online ? s.tools : ''],
      ['login', s?.login]],
    buttons, buttonsKey:`${id}|${chatSid}|${online}|${busy}|${online && !!s.launch}|${!!launchOpts?.enabled}|${away?.id}|${member?.session}`,
    terminal:online && s.launch ? s.launch : null, // its terminal in the window too, beside the one that opens a window
    // the chat follows the account's current desk session (an employee off duty keeps the last one's log while the bridge
    // has it), or shows the one task session
    session:chatSid || s?.id || null,
    online,
    offText:chatSid ? t('task.offline') : away && member ? t('chat.awayInTask', { id:away.id, name:member.name }) : t('chat.offline'),
  })
  placeChat()
}
// the status line follows what the employee (or the member) is doing
function chatStateLine() {
  const e = chatSid ? [...figs.values()].find(f => f.s?.id === chatSid) : emps.get(chatFor)
  // off duty at its desk but away in a task: where it is, as its name tag says
  return e?.away && !isOnline(e) ? t('task.inRoom', { room:taskRoom(e.away), id:e.away.id }) : e ? stateLine(e) : statusText(sessionById(chatSid))
}
function refreshChatLine() {
  if (!chatFor || chat.hidden) return
  chat.setLine(chatStateLine())
}

async function post(path, body) {
  const r = await sym.post(path, body)
  if (!r.ok) alert(errorText(r.data))
  return r.ok
}
// the session's terminal: the bridge opens a window on this machine attached to it (closing it changes nothing)
async function openTerminal(s) {
  const r = await sym.openTerminal(s.launch)
  if (!r.ok) alert(errorText(r.data) + (r.data?.params?.command ? '\n' + r.data.params.command : ''))
}

// a click on a character, or else on a desk, is a click on that employee (on a task's member: the window of its task
// session); a click on a small meeting room opens its task, or the task dialog
cv.addEventListener('click', ev => {
  const x = (ev.clientX * dpr - offX) / S, y = (ev.clientY * dpr - offY) / S
  const on = e => {
    const b = build(e.s)
    return e.inside && x >= e.x - b.bw / 2 - 3 && x < e.x + b.bw / 2 + 3 && y >= e.y - b.bh - 4 && y < e.y + 2
  }
  const fig = [...figs.values()].find(on)
  if (fig) return onFigureClick(fig)
  const list = [...emps.values()].filter(e => e.desk)
  const hit = list.find(on) || list.find(e => x >= e.desk.x && x < e.desk.x + 32 && y >= e.desk.y - 15 && y < e.desk.y + 16)
  if (hit) return onEmployeeClick(hit.acc.id)
  const room = smalls.find(r => x >= r.x && x < r.x + r.w && y >= r.y && y < r.y + r.h)
  if (room) onRoomClick(room)
})

// ================= starting sessions from the page =================
// The launch dialog is shared (ui/launch.js); the office loads the options, says in the HUD why starting is off, and
// opens the new employee's window once the launch is online
const launch = createLaunchDialog(sym, { options:loadLaunchOptions, onOnline:l => openChat(l.account), onOpen:closeChat, log })
async function loadLaunchOptions() {
  launchOpts = await sym.launchOptions(true)
  document.getElementById('new-emp').hidden = !launchOpts?.enabled
  document.getElementById('new-task').hidden = !launchOpts?.enabled
  // when starting isn't possible, say why; for a missing mod show the install command (reload after installing)
  const note = document.getElementById('launch-note')
  note.hidden = !launchOpts || launchOpts.enabled
  if (!note.hidden) {
    const fix = launchOpts.mod?.fix
    note.innerHTML = esc(t(fix ? 'launch.offWithFix' : 'launch.off', { reason:launchOffReason() })) + (fix ? `<code>${esc(fix)}</code>` : '')
  }
  layout()
  syncPlates() // a free room takes clicks while the page can start sessions
  return launchOpts
}
// why the page can't start sessions: the mod's problem, or no launcher at all
const launchOffReason = () => launchOpts?.mod ? errorText(launchOpts.mod) : t('error.launch_disabled')
const openLaunch = accountId => launch.open(accountId)
document.getElementById('new-emp').onclick = () => openLaunch(null)
document.getElementById('new-task').onclick = () => tasks.openDialog({})

// ================= tasks =================
// The task dialog and the task window are shared (ui/task.js): a click on a free small meeting room opens the dialog for
// it, a click on a room with a task opens the window. The office names a task's room, draws the members' figures for the
// team chat, keeps a member's window beside the task window and writes the activity log's lines.
// the figures in the team chat: a member as it looks in the office, and you
const avatars = new Map() // what a figure is drawn from -> its picture, a CSS url()
function avatar(key, draw, w = 24, h = 12) {
  let a = avatars.get(key)
  if (!a) { const was = hair; hair = 0; a = pixelArt(w, h, draw); hair = was; avatars.set(key, a) }
  return a
}
// a member as it looks in the office: its look, its size from the model, its arms from the effort, an advisor's visitor badge
function memberAvatar(m) {
  const s = m && sessionById(m.session), b = build(s?.model ? s : m)
  const acc = m && (snapshot.accounts.find(a => a.id === m.account) ?? { name:m.name, seat:-1 }), look = lookOf(acc)
  return avatar(`${b.size}|${b.muscle}|${!!m?.advisor}|${Object.values(look).join()}`, g => {
    drawPerson(g, 12, 10, b, { eyes:'open', legs:true }, look)
    if (m?.advisor) solid(g, 13, 7, 2, 2, '#fbfbf8')
  })
}
// you: a person in a dark suit with a yellow tie, on a smaller canvas, so it shows as big as a member
const YOU = ['...HHHHHH...', '...FFFFFF...', '...FEFFEF...', '...FFFFFF...', '....FFFF....', '.XXXXWWXXXX.', 'XXXXXYYXXXXX', 'XXXXXYYXXXXX']
const YOU_COLOR = { H:'#2b2a28', F:'#f0c8a0', E:EYE, X:'#2b2a28', W:'#fffdf7', Y:YELLOW }
const youAvatar = () => avatar('you', g => YOU.forEach((row, y) => [...row].forEach((c, x) => { if (c !== '.') rect(g, 2 + x, y, 1, 1, YOU_COLOR[c]) })), 16, 8)
const tasks = createTaskUI(sym, {
  options:loadLaunchOptions, roomLabel:taskRoom, memberAvatar, youAvatar,
  isMemberOpen:(task, m) => chatTask === task.id && chatSid === m.session && !chat.hidden, // its window is beside this one
  onOpenMember:openMember,
  onWindowOpen:id => { if (chatTask !== id) closeChat() }, // a member's window of this task stays beside it
  onWindowClose:() => { if (chatTask) closeChat() },
  onDialogOpen:() => { closeChat(); closeTaskWin() },
  log,
})
const openTaskWin = (id, data = null) => tasks.openWindow(id, data)
const closeTaskWin = () => tasks.closeWindow()


// ================= start-up =================
// room signs and the company sign's lettering, drawn with web fonts (clearer than pixel fonts in every script)
const roomEls = {}
for (const [key, text] of [['work', t('room.work')], ['meeting', t('room.meeting')], ['lounge', t('room.lounge')], ['sign', 'SYMPHONY']]) {
  const el = document.createElement('div')
  el.className = key === 'sign' ? 'sign-text' : 'room-label'
  el.setAttribute('aria-hidden', 'true')
  el.textContent = text
  document.getElementById('labels').appendChild(el)
  roomEls[key] = el
}
rooms.sign = {}
// the HUD's pixel art, drawn with the office's own helpers: the logo from the sign, a magnifier, an envelope
drawMark(document.querySelector('#hud .logo').getContext('2d'), 0, 0, '#2b2a28', YELLOW)
const pixelArt = (w, h, draw) => { const c = document.createElement('canvas'); c.width = w; c.height = h; draw(c.getContext('2d')); return `url(${c.toDataURL()})` }
const MAGNIFIER = ['.XXX...', 'X.o.X..', 'X...X..', 'X...X..', '.XXX...', '....XX.', '.....XX']
document.documentElement.style.setProperty('--ico-find', pixelArt(7, 7, g => MAGNIFIER.forEach((row, y) => [...row].forEach((c, x) => {
  if (c !== '.') rect(g, x, y, 1, 1, c === 'X' ? '#2b2a28' : '#9fd3f0')
}))))
document.documentElement.style.setProperty('--ico-mail', pixelArt(9, 6, g => drawEnvelope(g, 0, 0)))
const CHEVRON = ['...X...', '..XXX..', '.XX.XX.', 'XX...XX']
const chevron = rows => pixelArt(7, 4, g => rows.forEach((row, y) => [...row].forEach((c, x) => { if (c === 'X') rect(g, x, y, 1, 1, '#2b2a28') })))
document.documentElement.style.setProperty('--ico-up', chevron(CHEVRON))
document.documentElement.style.setProperty('--ico-down', chevron([...CHEVRON].reverse()))
setHud((() => { try { return localStorage.getItem(HUD_STORE) === 'up' } catch { return false } })(), false)
pixelsEl.querySelectorAll('button').forEach((b, i) => { // each a finer grid
  const cell = [6, 4, 3, 2][i]
  b.style.backgroundImage = pixelArt(12, 12, g => {
    for (let y = 0; y < 12; y++) for (let x = 0; x < 12; x++) rect(g, x, y, 1, 1, (((x / cell) | 0) + ((y / cell) | 0)) % 2 ? '#d6d1c6' : '#2b2a28')
  })
})
document.querySelector('#hud .prefs').prepend(pixelsEl)
pixelsEl.after(stillSwitch(Object.assign(document.createElement('button'), { type:'button', className:'px', id:'still' })))
// It slides from now on, not when the page opens: the remembered position takes effect here, before the slide is
// turned on. Without this, a browser that hadn't worked out the HUD's style yet would do so only in the next frame,
// after the slide is on, and slide it up as the page opens.
hud.getBoundingClientRect()
requestAnimationFrame(() => hud.classList.add('anim'))
const addBtn = document.createElement('button')
addBtn.className = 'seat-add'
addBtn.textContent = '+'
addBtn.title = t('office.addSeatTitle')
addBtn.setAttribute('aria-label', t('office.addSeatTitle'))
addBtn.hidden = true
addBtn.addEventListener('click', () => openLaunch(null))
document.getElementById('labels').appendChild(addBtn)

layout()
loadLaunchOptions()
sym.on('event', applyEvent)
sym.on('protocol', (bridge, page) => { // a bridge of another generation: say so in the HUD's note, and go on as far as it goes
  const note = document.getElementById('launch-note')
  note.hidden = false
  note.textContent = t('error.protocol_mismatch', { bridge, page })
})
let resizing = false
addEventListener('resize', () => { // one layout per frame, however many events a drag sends
  if (resizing) return
  resizing = true
  requestAnimationFrame(() => { resizing = false; layout(); placeChat() })
})
requestAnimationFrame(frame)
