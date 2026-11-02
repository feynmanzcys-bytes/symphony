// The in-page terminal: the pseudo-terminal of a session started from the page (the bridge hosts it,
// internal/bridge/term.go), drawn in the page as rows of text in its colors with the cursor, and in an interactive view
// the keys typed into it going back to the session. A plain model of what a terminal does that a session's output uses
// (no front-end library), as internal/bridge/vt.go is for the exit notice: printing, line feeds and scrolling, the
// cursor moves, erasing, inserting and deleting characters and lines, a scroll region, saving the cursor, an East Asian
// wide rune in two cells; and what a viewer needs besides: colors and attributes (SGR), the cursor's visibility, the
// alternate screen, bracketed paste, the lines that scrolled off the top.
//
// The bridge streams the terminal's raw output (GET /api/terminal/<launch>/stream?sizes=1: the history first, then what
// comes), and the terminal's size in it, first and whenever it changes, as xterm reports one (CSI 8 ; rows ; cols t):
// a view that only watches never sizes the session's terminal, it follows it. The views of one launch in a page share
// one stream and one screen; a page hidden for a while lets the streams go and opens them again when it shows.
//
//   import { createTerminal } from '/ui/terminal.js'
//   const view = createTerminal(sym, { interactive, onLeave })
//   view.el               the element to put where it shows. An interactive view sizes the session's terminal to fit it
//                         (POST /api/terminal/<launch>/resize) and keeps what scrolled off above the screen; one that
//                         only watches scales the screen down to the element's width
//   view.open(launchId)   show that session's terminal (the same one again changes nothing); null lets it go
//   view.close()          let it go
//   view.focus()          (interactive) take the keys: they go to the session (POST .../input), Esc and Tab included,
//                         the browser's own shortcuts aside; Ctrl+] calls onLeave, as it detaches symphony attach
//   view.launch           the launch it shows, or null
import { t, errorText } from '/ui/common.js'

const COLS = 120, ROWS = 40 // the bridge's termCols and termRows: a terminal's size until a window sets one
const MAX_COLS = 500, MAX_ROWS = 300 // a bigger terminal is drawn this big (a page holds no more anyway)
const KEEP_BACK = 1000 // lines kept above the screen
const PENDING_MAX = 64 << 10 // an escape sequence still open after this much is dropped
const INPUT_MAX = 64 << 10 // what one request types at most (the bridge takes 1 MiB)
const RESIZE_WAIT = 120 // ms: a resize drag sends its last size only
const HIDDEN_CLOSE = 10000 // a page hidden this long lets its streams go, as symphony.js does its own

// ---------- pens: a cell's colors and attributes ----------
// A color is -1 (the default), 0-255 (the palette) or TRUE + 0xRRGGBB. A pen is made once per combination and shared,
// so the cells of a run drawn alike are told by identity.
const TRUE = 0x1000000
const BOLD = 1, DIM = 2, ITALIC = 4, UNDER = 8, INVERSE = 16, HIDDEN = 32, STRIKE = 64
const PLAIN = { fg: -1, bg: -1, fl: 0, css: '' }
const pens = new Map()
function penOf(fg, bg, fl) {
  if (fg === -1 && bg === -1 && fl === 0) return PLAIN
  const key = fg + ',' + bg + ',' + fl
  let p = pens.get(key)
  if (!p) {
    if (pens.size >= 4096) pens.clear() // the cells keep theirs; runs only break more often
    pens.set(key, p = { fg, bg, fl, css: null })
  }
  return p
}
const CUBE = [0, 95, 135, 175, 215, 255]
function colorCSS(c) {
  if (c < 16) return `var(--term-c${c})` // the theme's palette
  if (c >= TRUE) return '#' + (c - TRUE).toString(16).padStart(6, '0')
  if (c >= 232) { const v = 8 + (c - 232) * 10; return `rgb(${v},${v},${v})` }
  c -= 16
  return `rgb(${CUBE[Math.floor(c / 36)]},${CUBE[Math.floor(c / 6) % 6]},${CUBE[c % 6]})`
}
function penCSS(p) {
  if (p.css !== null) return p.css
  let fg = p.fg < 0 ? '' : colorCSS(p.fg), bg = p.bg < 0 ? '' : colorCSS(p.bg)
  if (p.fl & INVERSE) [fg, bg] = [bg || 'var(--term-bg)', fg || 'var(--term-fg)']
  let s = ''
  if (p.fl & HIDDEN) s += 'color:transparent;'
  else if (p.fl & DIM) s += `color:color-mix(in srgb,${fg || 'var(--term-fg)'} 55%,transparent);`
  else if (fg) s += `color:${fg};`
  if (bg) s += `background:${bg};`
  if (p.fl & BOLD) s += 'font-weight:700;'
  if (p.fl & ITALIC) s += 'font-style:italic;'
  if (p.fl & (UNDER | STRIKE)) s += `text-decoration:${p.fl & UNDER ? 'underline' : ''}${p.fl & UNDER && p.fl & STRIKE ? ' ' : ''}${p.fl & STRIKE ? 'line-through' : ''};`
  return (p.css = s)
}

// ---------- how many cells a character takes ----------
// internal/bridge/vt.go's wide(), range for range: East Asian Wide and Fullwidth (CJK, Hangul, kana, fullwidth forms)
// and the emoji blocks take two cells
const WIDE_SYMBOLS = new Set([0x231a, 0x231b, 0x23e9, 0x23ea, 0x23eb, 0x23ec, 0x23f0, 0x23f3, 0x25fd, 0x25fe, 0x2614, 0x2615,
  0x267f, 0x2693, 0x26a1, 0x26aa, 0x26ab, 0x26bd, 0x26be, 0x26c4, 0x26c5, 0x26ce, 0x26d4, 0x26ea, 0x26f2, 0x26f3, 0x26f5, 0x26fa,
  0x26fd, 0x2705, 0x270a, 0x270b, 0x2728, 0x274c, 0x274e, 0x2753, 0x2754, 0x2755, 0x2757, 0x2795, 0x2796, 0x2797, 0x27b0, 0x27bf,
  0x2b1b, 0x2b1c, 0x2b50, 0x2b55])
export function wide(r) {
  if (r < 0x1100) return false
  if (r <= 0x115f || (r >= 0x2e80 && r <= 0x303e) || (r >= 0x3041 && r <= 0x33ff) || (r >= 0x3400 && r <= 0x4dbf) ||
    (r >= 0x4e00 && r <= 0x9fff) || (r >= 0xa000 && r <= 0xa4cf) || (r >= 0xac00 && r <= 0xd7a3) || (r >= 0xf900 && r <= 0xfaff) ||
    (r >= 0xfe30 && r <= 0xfe4f) || (r >= 0xff00 && r <= 0xff60) || (r >= 0xffe0 && r <= 0xffe6) || r === 0x1f004 || r === 0x1f0cf ||
    (r >= 0x1f18e && r <= 0x1f19a) || (r >= 0x1f200 && r <= 0x1f251) || (r >= 0x1f300 && r <= 0x1f64f) || (r >= 0x1f680 && r <= 0x1f6ff) ||
    (r >= 0x1f7e0 && r <= 0x1f7eb) || (r >= 0x1f90c && r <= 0x1f9ff) || (r >= 0x1fa70 && r <= 0x1faff) || (r >= 0x20000 && r <= 0x3fffd)) return true
  return WIDE_SYMBOLS.has(r) || (r >= 0x2648 && r <= 0x2653) // the wide ones among the symbols and dingbats, the zodiac
}
// a combining mark, a joiner, a direction mark or a variation selector goes onto the cell before it
const ZERO_WIDTH = /^[\p{Mn}\p{Me}​-‏⁠﻿]$/u
// what 0x60-0x7e draw after ESC ( 0, the DEC line drawing set
const DEC_GRAPHICS = '◆▒␉␌␍␊°±␤␋┘┐┌└┼⎺⎻─⎼⎽├┤┴┬│≤≥π≠£·'

// ---------- the screen ----------
class Line {
  constructor(cols, pen = PLAIN) {
    this.chars = new Array(cols).fill(' ') // ' ' an empty cell, '' the second cell of a wide character
    this.pens = new Array(cols).fill(pen)
    this.ver = 0 // goes up with every change: a view draws the row again
  }
  // to a new width: cut (a wide character cut in half goes too), or filled with empty cells
  fit(cols) {
    const C = this.chars, P = this.pens, n = C.length
    if (cols === n) return
    if (cols < n) {
      if (cols > 0 && C[cols] === '') C[cols - 1] = ' '
      C.length = cols; P.length = cols
    } else for (let x = n; x < cols; x++) { C.push(' '); P.push(PLAIN) }
    this.ver++
  }
  get blank() { return this.chars.every(c => c === ' ') && this.pens.every(p => p === PLAIN) }
  get text() { return this.chars.join('').trimEnd() }
}

// Screen is the model: a grid of cols x rows cells fed with a terminal's output (write takes it in pieces, as it comes:
// a sequence cut at the end of one is finished by the next), the normal screen with what scrolled off its top (back)
// and the alternate one. Exported for the tests.
export class Screen {
  constructor(cols = COLS, rows = ROWS) {
    this.cols = Math.max(1, Math.min(cols, MAX_COLS)); this.rows = Math.max(1, Math.min(rows, MAX_ROWS))
    this.back = [] // the lines that scrolled off the normal screen's top, the oldest first
    this.backGone = 0 // how many lines have left the front of back (KEEP_BACK): back[i] is the (backGone + i)th kept
    this.backEpoch = 0 // goes up when back changes other than by lines added at its end or let go at its front
    this.pending = '' // an escape sequence cut off at the end of the last piece
    this.reset()
    // A screen that joins a session's output midway (the history keeps its last 256 KiB) assumes what Claude Code sets
    // as it starts, which the history may have lost: its own cursor drawn in the input box (the terminal's hidden), and
    // pastes in brackets. Its output says otherwise when it does.
    this.cursorOn = false
    this.paste = true
  }
  // the start, and a full reset (ESC c): the screens empty, the modes as a terminal starts
  reset() {
    this.normal = this.blankLines(this.rows); this.alt = null; this.lines = this.normal; this.altOn = false
    this.x = 0; this.y = 0; this.wrapNext = false
    this.top = 0; this.bot = this.rows - 1
    this.pen = PLAIN; this.charset = false; this.savedN = null; this.savedA = null
    this.cursorOn = true; this.appCursor = false; this.paste = false; this.autowrap = true; this.insert = false; this.origin = false
    this.last = ' '; this.lastW = 1 // the last character printed (REP repeats it)
  }
  blankLines(n, pen = PLAIN) { return Array.from({ length: n }, () => new Line(this.cols, pen)) }
  // the pen erased and scrolled-in cells get: the current background, as xterm does
  get erasePen() { return this.pen.bg < 0 ? PLAIN : penOf(-1, this.pen.bg, 0) }
  // the screen's text, a row a line (the tests read it)
  text() { return this.lines.map(l => l.text).join('\n') }

  write(data) {
    const s = this.pending ? this.pending + data : data
    this.pending = ''
    const n = s.length
    for (let i = 0; i < n;) {
      const c = s.charCodeAt(i)
      if (c >= 0x20 && c < 0x7f) { // the common case: printable ASCII
        this.print(this.charset && c >= 0x60 ? DEC_GRAPHICS[c - 0x60] : s[i], 1)
        i++
      } else if (c === 0x1b) {
        const end = this.escape(s, i)
        if (end < 0) { // cut off: the rest comes with the next piece
          if (n - i <= PENDING_MAX) this.pending = s.slice(i)
          break
        }
        i = end
      } else if (c < 0x20 || c === 0x7f || (c >= 0x80 && c < 0xa0)) {
        this.control(c)
        i++
      } else {
        const cp = s.codePointAt(i), len = cp > 0xffff ? 2 : 1, ch = s.slice(i, i + len)
        if (ZERO_WIDTH.test(ch)) this.combine(ch)
        else this.print(ch, wide(cp) ? 2 : 1)
        i += len
      }
    }
  }

  control(c) {
    switch (c) {
      case 0x08: // backspace
        if (this.wrapNext) this.wrapNext = false
        else if (this.x > 0) this.x--
        break
      case 0x09: this.tab(1); break
      case 0x0a: case 0x0b: case 0x0c: this.lineFeed(); break
      case 0x0d: this.x = 0; this.wrapNext = false; break
      // the bell, the shifts, the C1 controls and the rest: nothing
    }
  }
  tab(n) {
    for (; n > 0; n--) this.x = Math.min((Math.floor(this.x / 8) + 1) * 8, this.cols - 1)
    this.wrapNext = false
  }

  print(ch, w) {
    if (this.wrapNext) {
      if (this.autowrap) { this.x = 0; this.lineFeed() } // (without autowrap the last column is drawn over)
      this.wrapNext = false
    }
    if (this.x + w > this.cols) {
      if (w > this.cols) return
      if (this.autowrap) { this.x = 0; this.lineFeed() } else this.x = this.cols - w
    }
    const line = this.lines[this.y], C = line.chars, P = line.pens, x = this.x, end = x + w
    if (this.insert) this.shift(line, x, w)
    // drawing over half of a wide character: its other half goes too
    if (C[x] === '' && x > 0) C[x - 1] = ' '
    if (end < this.cols && C[end] === '') C[end] = ' '
    C[x] = ch; P[x] = this.pen
    if (w === 2) { C[x + 1] = ''; P[x + 1] = this.pen }
    line.ver++
    this.last = ch; this.lastW = w
    if (end < this.cols) this.x = end
    else { this.x = this.cols - 1; this.wrapNext = true }
  }
  combine(mark) {
    let x = this.wrapNext ? this.x : this.x - 1
    if (x < 0) return
    const line = this.lines[this.y]
    if (line.chars[x] === '' && x > 0) x--
    line.chars[x] += mark
    line.ver++
  }

  lineFeed() {
    if (this.y === this.bot) this.scrollUp(1)
    else if (this.y < this.rows - 1) this.y++
  }
  reverseIndex() {
    if (this.y === this.top) this.scrollDown(1)
    else if (this.y > 0) this.y--
    this.wrapNext = false
  }
  // scrollUp moves rows top..bot up n: the top ones go (into back, when they leave the whole normal screen), empty ones
  // come in at the bottom
  scrollUp(n, top = this.top, keep = true) {
    n = Math.min(n, this.bot - top + 1)
    const L = this.lines
    for (let k = 0; k < n; k++) {
      const gone = L.splice(top, 1)[0]
      L.splice(this.bot, 0, new Line(this.cols, this.erasePen))
      if (keep && top === 0 && !this.altOn) this.keep(gone)
    }
  }
  scrollDown(n, top = this.top) {
    n = Math.min(n, this.bot - top + 1)
    const L = this.lines
    for (let k = 0; k < n; k++) {
      L.splice(this.bot, 1)
      L.splice(top, 0, new Line(this.cols, this.erasePen))
    }
  }
  keep(line) {
    this.back.push(line)
    if (this.back.length >= KEEP_BACK + 100) { // in batches, not one at a time
      const k = this.back.length - KEEP_BACK
      this.back.splice(0, k)
      this.backGone += k
    }
  }

  // clear empties cells from..to of a line (half a wide character at either end goes too)
  clear(line, from, to) {
    from = Math.max(from, 0); to = Math.min(to, this.cols)
    if (from >= to) return
    const C = line.chars, P = line.pens, pen = this.erasePen
    if (C[from] === '' && from > 0) C[from - 1] = ' '
    if (to < this.cols && C[to] === '') C[to] = ' '
    for (let x = from; x < to; x++) { C[x] = ' '; P[x] = pen }
    line.ver++
  }
  // shift moves the cells from x on k to the right (insert mode, CSI @): what passes the end goes
  shift(line, x, k) {
    const C = line.chars, P = line.pens, cols = this.cols
    k = Math.min(k, cols - x)
    if (C[x] === '' && x > 0) { C[x - 1] = ' '; C[x] = ' ' } // inside a wide character: it goes
    C.splice(x, 0, ...new Array(k).fill(' ')); P.splice(x, 0, ...new Array(k).fill(this.erasePen))
    if (C[cols] === '') C[cols - 1] = ' ' // a wide character pushed half past the end goes
    C.length = cols; P.length = cols
    line.ver++
  }
  deleteChars(k) {
    const line = this.lines[this.y], C = line.chars, P = line.pens, x = this.x, cols = this.cols
    k = Math.min(k, cols - x)
    if (C[x] === '' && x > 0) C[x - 1] = ' '
    C.splice(x, k); P.splice(x, k)
    for (let i = 0; i < k; i++) { C.push(' '); P.push(this.erasePen) }
    if (C[x] === '') C[x] = ' ' // the second half of a wide character whose first half went
    line.ver++
  }

  escape(s, i) {
    const n = s.length
    if (i + 1 >= n) return -1
    switch (s[i + 1]) {
      case '[': return this.csi(s, i)
      case ']': case 'P': case '^': case '_': case 'X': // OSC, DCS, PM, APC, SOS: a string up to BEL or ST, read and ignored
        for (let j = i + 2; j < n; j++) {
          const d = s.charCodeAt(j)
          if (d === 0x07) return j + 1
          if (d === 0x1b) return j + 1 >= n ? -1 : s[j + 1] === '\\' ? j + 2 : j // another sequence starts: the string ends here
        }
        return -1
      case '7': this.saveCursor(); return i + 2
      case '8': this.restoreCursor(); return i + 2
      case 'D': this.lineFeed(); this.wrapNext = false; return i + 2
      case 'M': this.reverseIndex(); return i + 2
      case 'E': this.x = 0; this.wrapNext = false; this.lineFeed(); return i + 2
      case 'c': // full reset: the screen and the modes; the size and what scrolled off stay
        this.reset()
        return i + 2
      case '(': // G0's character set: the line drawing one, or back to ASCII
        if (i + 2 >= n) return -1
        this.charset = s[i + 2] === '0'
        return i + 3
    }
    // ESC, intermediates (0x20-0x2f), a final byte: nothing drawn (ESC ) 0, ESC =, ESC >, ESC # 8 ...); a control
    // character breaks it off and goes on (a line feed after a lone ESC still feeds the line)
    let j = i + 1
    while (j < n && s.charCodeAt(j) >= 0x20 && s.charCodeAt(j) <= 0x2f) j++
    if (j >= n) return -1
    return s.charCodeAt(j) < 0x20 ? j : j + 1
  }

  // csi reads a control sequence (ESC [ parameters intermediates final) and acts on it
  csi(s, i) {
    const n = s.length
    let j = i + 2
    while (j < n && s.charCodeAt(j) >= 0x30 && s.charCodeAt(j) <= 0x3f) j++
    const params = s.slice(i + 2, j), k = j
    while (j < n && s.charCodeAt(j) >= 0x20 && s.charCodeAt(j) <= 0x2f) j++
    if (j >= n) return -1
    const f = s.charCodeAt(j)
    if (f < 0x40 || f > 0x7e) return j // broken off by a control character: dropped, the character goes on
    this.dispatch(params, s.slice(k, j), s[j])
    return j + 1
  }

  dispatch(params, inter, f) {
    const lead = params.charCodeAt(0)
    if (lead === 0x3f) { // ? : the private modes
      if (inter === '' && (f === 'h' || f === 'l')) for (const m of params.slice(1).split(';')) this.privateMode(parseInt(m, 10), f === 'h')
      return
    }
    if (lead >= 0x3c) return // < = > : keyboard and mouse modes, queries: nothing drawn
    if (inter !== '') { // DECSCUSR (" q"), DECRQM ("$p") and the like: nothing drawn; DECSTR resets the modes
      if (inter === '!' && f === 'p') this.softReset()
      return
    }
    const p = params === '' ? [] : params.split(';')
    const num = (k, def) => { const v = p[k]; if (v === undefined || v === '') return def; const x = parseInt(v, 10); return x >= 0 ? x : def }
    const n1 = () => Math.max(num(0, 1), 1)
    if (f !== 'm') this.wrapNext = false
    switch (f) {
      case 'A': this.y = Math.max(this.y - n1(), this.y >= this.top ? this.top : 0); break
      case 'B': case 'e': this.y = Math.min(this.y + n1(), this.y <= this.bot ? this.bot : this.rows - 1); break
      case 'C': case 'a': this.x = Math.min(this.x + n1(), this.cols - 1); break
      case 'D': this.x = Math.max(this.x - n1(), 0); break
      case 'E': this.x = 0; this.y = Math.min(this.y + n1(), this.rows - 1); break
      case 'F': this.x = 0; this.y = Math.max(this.y - n1(), 0); break
      case 'G': case '`': this.x = Math.min(n1(), this.cols) - 1; break
      case 'd': this.y = this.rowAt(n1()); break
      case 'H': case 'f': this.y = this.rowAt(n1()); this.x = Math.min(Math.max(num(1, 1), 1), this.cols) - 1; break
      case 'I': this.tab(n1()); break
      case 'Z': for (let k = n1(); k > 0 && this.x > 0; k--) this.x = (Math.ceil(this.x / 8) - 1) * 8; break
      case 'J': this.eraseDisplay(num(0, 0)); break
      case 'K': {
        const line = this.lines[this.y], m = num(0, 0)
        if (m === 0) this.clear(line, this.x, this.cols)
        else if (m === 1) this.clear(line, 0, this.x + 1)
        else if (m === 2) this.clear(line, 0, this.cols)
        break
      }
      case '@': this.shift(this.lines[this.y], this.x, n1()); break
      case 'P': this.deleteChars(n1()); break
      case 'X': this.clear(this.lines[this.y], this.x, this.x + n1()); break
      case 'L': if (this.y >= this.top && this.y <= this.bot) this.scrollDown(n1(), this.y); break
      case 'M': if (this.y >= this.top && this.y <= this.bot) this.scrollUp(n1(), this.y, false); break
      case 'S': this.scrollUp(n1()); break
      case 'T': if (p.length <= 1) this.scrollDown(n1()); break // (with more parameters: a mouse highlight)
      case 'b': for (let k = Math.min(n1(), 65535); k > 0; k--) this.print(this.last, this.lastW); break
      case 'r': this.margins(num(0, 1), num(1, this.rows)); break
      case 's': if (!p.length) this.saveCursor(); break
      case 'u': if (!p.length) this.restoreCursor(); break
      case 'm': this.sgr(p); break
      case 'h': case 'l': if (p.includes('4')) this.insert = f === 'h'; break
      case 't': if (num(0, 0) === 8) this.resize(num(2, 0) || this.cols, num(1, 0) || this.rows); break // the size, as the bridge reports it
      // DA, DSR and the other queries, tab stops, LEDs: nothing drawn
    }
  }
  rowAt(n) { return this.origin ? Math.min(this.top + n - 1, this.bot) : Math.min(n, this.rows) - 1 }
  margins(top, bot) {
    if (top < 1) top = 1
    if (bot > this.rows || bot < 1) bot = this.rows
    if (top < bot) { this.top = top - 1; this.bot = bot - 1; this.x = 0; this.y = this.origin ? this.top : 0 }
  }
  eraseDisplay(m) {
    const L = this.lines
    if (m === 0) {
      this.clear(L[this.y], this.x, this.cols)
      for (let y = this.y + 1; y < this.rows; y++) this.clear(L[y], 0, this.cols)
    } else if (m === 1) {
      for (let y = 0; y < this.y; y++) this.clear(L[y], 0, this.cols)
      this.clear(L[this.y], 0, this.x + 1)
    } else if (m === 2) {
      for (let y = 0; y < this.rows; y++) this.clear(L[y], 0, this.cols)
    } else if (m === 3) { // what scrolled off goes (xterm: only that)
      this.backGone += this.back.length
      this.back = []
      this.backEpoch++
    }
  }

  privateMode(m, on) {
    switch (m) {
      case 1: this.appCursor = on; break
      case 6: this.origin = on; this.x = 0; this.y = on ? this.top : 0; break
      case 7: this.autowrap = on; break
      case 25: this.cursorOn = on; break
      case 47: this.useAlt(on, false); break
      case 1047: if (!on && this.altOn) this.alt = null; this.useAlt(on, false); break
      case 1048: if (on) this.saveCursor(); else this.restoreCursor(); break
      case 1049:
        if (on) { if (!this.altOn) this.saveCursor(); this.useAlt(true, true) } else if (this.altOn) { this.useAlt(false); this.restoreCursor() }
        break
      case 2004: this.paste = on; break
      // the mouse, focus reports, synchronized output, the keyboard's modes...: nothing a viewer draws
    }
  }
  useAlt(on, fresh) {
    if (on) {
      if (!this.alt || fresh) this.alt = this.blankLines(this.rows)
      this.lines = this.alt
    } else this.lines = this.normal
    this.altOn = on
  }
  cursorState() { return { x: this.x, y: this.y, pen: this.pen, origin: this.origin, charset: this.charset } }
  saveCursor() { if (this.altOn) this.savedA = this.cursorState(); else this.savedN = this.cursorState() }
  restoreCursor() {
    const s = (this.altOn ? this.savedA : this.savedN) ?? { x: 0, y: 0, pen: PLAIN, origin: false, charset: false }
    this.x = Math.min(s.x, this.cols - 1); this.y = Math.min(s.y, this.rows - 1)
    this.pen = s.pen; this.origin = s.origin; this.charset = s.charset; this.wrapNext = false
  }
  softReset() {
    this.cursorOn = true; this.appCursor = false; this.autowrap = true; this.insert = false; this.origin = false
    this.top = 0; this.bot = this.rows - 1; this.pen = PLAIN; this.charset = false; this.savedN = this.savedA = null
  }

  // sgr sets the pen: colors (16, 256 and true, also in the ITU form with colons) and attributes
  sgr(f) {
    let { fg, bg, fl } = this.pen
    if (!f.length) f = ['0']
    const byte = v => Math.max(0, Math.min(255, parseInt(v, 10) || 0))
    for (let k = 0; k < f.length; k++) {
      if (f[k].includes(':')) { // 38:2::r:g:b, 38:2:r:g:b, 38:5:n, 4:3 (an underline's style)
        const sub = f[k].split(':'), code = parseInt(sub[0], 10)
        if (code === 38 || code === 48) {
          let c = null
          if (sub[1] === '5' && sub.length > 2) c = byte(sub[2])
          else if (sub[1] === '2') { const v = sub.length > 5 ? sub.slice(3, 6) : sub.slice(2, 5); if (v.length === 3) c = TRUE + (byte(v[0]) << 16 | byte(v[1]) << 8 | byte(v[2])) }
          if (c !== null) { if (code === 38) fg = c; else bg = c }
        } else if (code === 4) fl = sub[1] === '0' ? fl & ~UNDER : fl | UNDER
        continue
      }
      const c = f[k] === '' ? 0 : parseInt(f[k], 10)
      if (c === 0) { fg = -1; bg = -1; fl = 0 }
      else if (c === 1) fl |= BOLD
      else if (c === 2) fl |= DIM
      else if (c === 3) fl |= ITALIC
      else if (c === 4 || c === 21) fl |= UNDER
      else if (c === 7) fl |= INVERSE
      else if (c === 8) fl |= HIDDEN
      else if (c === 9) fl |= STRIKE
      else if (c === 22) fl &= ~(BOLD | DIM)
      else if (c === 23) fl &= ~ITALIC
      else if (c === 24) fl &= ~UNDER
      else if (c === 27) fl &= ~INVERSE
      else if (c === 28) fl &= ~HIDDEN
      else if (c === 29) fl &= ~STRIKE
      else if (c >= 30 && c <= 37) fg = c - 30
      else if (c === 39) fg = -1
      else if (c >= 40 && c <= 47) bg = c - 40
      else if (c === 49) bg = -1
      else if (c >= 90 && c <= 97) fg = c - 82
      else if (c >= 100 && c <= 107) bg = c - 92
      else if (c === 38 || c === 48 || c === 58) {
        let col = null
        if (f[k + 1] === '5') { if (k + 2 < f.length) col = byte(f[k + 2]); k += 2 }
        else if (f[k + 1] === '2') { if (k + 4 < f.length) col = TRUE + (byte(f[k + 2]) << 16 | byte(f[k + 3]) << 8 | byte(f[k + 4])); k += 4 }
        else k = f.length // a form we don't know: the rest can't be read
        if (col !== null && c !== 58) { if (c === 38) fg = col; else bg = col }
      }
      // blinking, fonts, overlines, the underline's color: nothing drawn
    }
    this.pen = penOf(fg, bg, fl)
  }

  // resize: the terminal's new size (the bridge's report), as xterm takes one. Rows are cut at a narrower width. At fewer
  // rows the empty ones below the cursor go first, then the top ones (into back), so the cursor's row stays; at more, the
  // lines kept above come back down first, so the screen's foot (Claude Code's input) stays at the bottom, then empty
  // rows come in below.
  resize(cols, rows) {
    cols = Math.max(1, Math.min(cols, MAX_COLS)); rows = Math.max(1, Math.min(rows, MAX_ROWS))
    if (cols === this.cols && rows === this.rows) return
    for (const buf of [this.normal, this.alt]) {
      if (!buf) continue
      for (const line of buf) line.fit(cols)
      const active = buf === this.lines, normal = buf === this.normal
      let excess = buf.length - rows
      while (excess > 0 && buf.length - 1 > (active ? this.y : -1) && buf[buf.length - 1].blank) { buf.pop(); excess-- }
      while (excess > 0) {
        const gone = buf.shift()
        if (normal) this.keep(gone)
        if (active) this.y--
        excess--
      }
      if (normal && buf.length < rows && this.back.length) this.backEpoch++ // (back changes at its end: views draw it again)
      while (normal && buf.length < rows && this.back.length) {
        const line = this.back.pop()
        line.fit(cols)
        buf.unshift(line)
        if (active) this.y++
      }
      while (buf.length < rows) buf.push(new Line(cols))
    }
    this.cols = cols; this.rows = rows
    this.top = 0; this.bot = rows - 1
    this.x = Math.min(this.x, cols - 1); this.y = Math.max(0, Math.min(this.y, rows - 1)); this.wrapNext = false
  }
}
const wideHead = (C, x) => x + 1 < C.length && C[x + 1] === ''

// ---------- the streams: one per launch, shared by the views of the page ----------
const feeds = new Map() // launch id -> Feed
const enc = new TextEncoder()

class Feed {
  constructor(sym, id) {
    this.sym = sym; this.id = id
    this.screen = new Screen()
    this.views = new Set()
    this.status = 'connecting' // connecting, live, lost (trying again), ended (the process has), gone (no terminal), failed
    this.err = null // the bridge's error ({code, params}) for gone and failed, or a failed keystroke's
    this.ctl = null // the stream's AbortController while it is open or opening
    this.timer = 0 // the next try
    this.backoff = 0
    this.asleep = false // the page is hidden: the stream is let go until it shows
    this.want = null // [cols, rows]: the size an interactive view wants the terminal at
    this.sized = '' // the size last asked of the bridge
    this.resizeTimer = 0
    this.out = '' // keystrokes on their way, in order
    this.sending = false
  }
  get url() { return `/api/terminal/${encodeURIComponent(this.id)}` }
  add(view) {
    this.views.add(view)
    if (!this.ctl && !this.timer && !this.asleep && this.status !== 'ended' && this.status !== 'gone') this.connect()
  }
  remove(view) {
    this.views.delete(view)
    if (this.views.size) return
    this.stop()
    clearTimeout(this.resizeTimer)
    feeds.delete(this.id)
  }
  stop() {
    clearTimeout(this.timer); this.timer = 0
    const ctl = this.ctl
    this.ctl = null
    ctl?.abort()
  }
  notify() { for (const v of this.views) v.changed() }
  // whether the session's process still runs, as the snapshot tells
  live() {
    const l = this.sym?.state?.launches?.find(x => x.id === this.id)
    return !this.sym || !!l && (l.status === 'starting' || l.status === 'online')
  }
  async connect() {
    const ctl = this.ctl = new AbortController()
    const q = new URLSearchParams({ sizes: '1' })
    if (this.want) { q.set('cols', this.want[0]); q.set('rows', this.want[1]); this.sized = this.want.join('x') }
    let r
    try {
      r = await fetch(`${this.url}/stream?${q}`, { signal: ctl.signal, cache: 'no-store' })
    } catch { return this.lost(ctl) }
    if (ctl !== this.ctl) return
    if (!r.ok) return this.failed(ctl, r.status, await r.json().catch(() => null))
    const started = Date.now(), dec = new TextDecoder(), reader = r.body.getReader()
    let fresh = true
    for (;;) {
      let got
      try { got = await reader.read() } catch { return this.lost(ctl) }
      if (ctl !== this.ctl) return
      if (got.done) break
      if (fresh) { // a new stream starts from the history: a new screen
        fresh = false
        this.screen = new Screen()
        this.status = 'live'; this.err = null; this.backoff = 0
      }
      try {
        this.screen.write(dec.decode(got.value, { stream: true }))
      } catch (e) { setTimeout(() => { throw e }) } // the model's fault: the page's error, and the stream goes on
      this.notify()
    }
    this.ctl = null
    // the end: the process ended, or the bridge let this window go (it fell behind); a stream that lasted, of a
    // session still running, is opened again
    if (this.live() && Date.now() - started > 2000) return this.retry('lost')
    this.status = 'ended'
    this.notify()
  }
  lost(ctl) {
    if (ctl !== this.ctl) return
    this.ctl = null
    if (!this.live()) { this.status = 'ended'; return this.notify() }
    this.retry('lost')
  }
  failed(ctl, status, body) {
    if (ctl !== this.ctl) return
    this.ctl = null
    this.err = body ?? { code: 'unreachable' }
    if (status === 404 || !this.live()) { this.status = 'gone'; return this.notify() } // no terminal for this launch
    this.retry('failed')
  }
  retry(status) {
    this.status = status
    this.notify()
    this.backoff = Math.min((this.backoff || 500) * 2, 10000)
    clearTimeout(this.timer)
    this.timer = setTimeout(() => { this.timer = 0; if (this.views.size && !this.asleep) this.connect() }, this.backoff)
  }
  sleep() { this.asleep = true; this.stop() }
  wake() {
    if (!this.asleep) return
    this.asleep = false
    if (this.views.size && this.status !== 'ended' && this.status !== 'gone' && !this.ctl) this.connect()
  }

  // keystrokes go in order, one request at a time; what is typed meanwhile goes with the next
  send(text) {
    if (!text || this.status === 'ended' || this.status === 'gone') return
    this.out += text
    if (!this.sending) this.flush()
  }
  async flush() {
    this.sending = true
    try {
      while (this.out) {
        let cut = Math.min(this.out.length, INPUT_MAX)
        if (cut < this.out.length && /[\ud800-\udbff]/.test(this.out[cut - 1])) cut-- // not between a surrogate pair's halves
        const chunk = this.out.slice(0, cut)
        this.out = this.out.slice(cut)
        let res = null
        try {
          res = await fetch(`${this.url}/input`, { method: 'POST', headers: { 'content-type': 'application/octet-stream' }, body: enc.encode(chunk) })
        } catch {}
        if (!res?.ok) {
          this.out = ''
          this.err = res ? await res.json().catch(() => null) ?? { code: 'unknown' } : { code: 'unreachable' }
          this.inputFailed = Date.now()
          this.notify()
          setTimeout(() => this.notify(), 4000) // the note goes after a while
          break
        }
      }
    } finally { this.sending = false }
  }
  // resize asks the bridge for the size an interactive view fits (null: none wants one any more); the last of a drag
  resize(size) {
    this.want = size
    clearTimeout(this.resizeTimer)
    if (!size) { this.sized = ''; return } // a view that opens again asks again, whoever sized the terminal meanwhile
    this.resizeTimer = setTimeout(() => {
      const key = size.join('x')
      if (!this.ctl || key === this.sized) return // the next stream asks for it
      this.sized = key
      fetch(`${this.url}/resize`, { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ cols: size[0], rows: size[1] }) })
        .catch(() => { this.sized = '' })
    }, RESIZE_WAIT)
  }
}
function feedFor(sym, id) {
  let f = feeds.get(id)
  if (!f) feeds.set(id, f = new Feed(sym, id))
  return f
}
// a page hidden a while lets the streams go; shown again it opens them, and their screens come from the history again
let hiddenTimer = 0
document.addEventListener('visibilitychange', () => {
  clearTimeout(hiddenTimer)
  if (document.visibilityState === 'hidden') hiddenTimer = setTimeout(() => { for (const f of feeds.values()) f.sleep() }, HIDDEN_CLOSE)
  else for (const f of feeds.values()) f.wake()
})

// ---------- drawing ----------
const escHTML = s => s.replace(/[&<>]/g, c => c === '&' ? '&amp;' : c === '<' ? '&lt;' : '&gt;')
// A row as HTML: a run of cells drawn alike in one span (plain text needs none), every character but printable ASCII
// in a box one or two cells wide (fonts don't agree on their widths: a CJK glyph is rarely two Latin ones, and a symbol
// may come from another font), the cursor's cell on its own; the empty cells at the end are left out.
function rowHTML(line, cur) {
  const C = line.chars, P = line.pens, cols = C.length
  if (cur > 0 && C[cur] === '') cur-- // on a wide character's second half: the cursor covers all of it
  let end = cols
  while (end > 0 && C[end - 1] === ' ' && P[end - 1] === PLAIN) end--
  if (cur >= end) end = Math.min(cur + 1, cols)
  let html = ''
  for (let x = 0; x < end;) {
    const pen = P[x], css = penCSS(pen)
    let text = ''
    if (x === cur) {
      const two = wideHead(C, x)
      text = `<span class="cur${two ? ' w' : ''}">${escHTML(C[x] || ' ')}</span>`
      x += two ? 2 : 1
    } else {
      while (x < end && x !== cur && P[x] === pen) {
        const ch = C[x], code = ch.length === 1 ? ch.charCodeAt(0) : 0
        if (code >= 0x20 && code < 0x7f) { text += code === 0x26 ? '&amp;' : code === 0x3c ? '&lt;' : code === 0x3e ? '&gt;' : ch; x++ }
        else if (ch === '') { text += ' '; x++ } // a second half without its first: empty
        else if (wideHead(C, x)) { text += `<span class="w">${escHTML(ch)}</span>`; x += 2 }
        else { text += `<span class="n">${escHTML(ch)}</span>`; x++ }
      }
    }
    html += css ? `<span style="${css}">${text}</span>` : text
  }
  return html
}

const MARKUP = `<div class="term-scroll"><div class="term-back"></div><div class="term-screen"></div></div>` +
  `<div class="term-row term-probe" aria-hidden="true">0000000000000000000000000000000000000000</div><div class="term-note" hidden></div>`
const PROBE = 40

export function createTerminal(sym, { interactive = false, onLeave = () => {} } = {}) {
  const el = document.createElement('div')
  el.className = 'term ' + (interactive ? 'live' : 'peek')
  el.innerHTML = MARKUP
  el.setAttribute('role', 'region')
  el.setAttribute('aria-label', t('term.label'))
  const scroller = el.querySelector('.term-scroll'), backBox = el.querySelector('.term-back'), screenBox = el.querySelector('.term-screen')
  const probe = el.querySelector('.term-probe'), noteEl = el.querySelector('.term-note')
  let keys = null // (interactive) the textarea that takes the keys: hidden at the cursor, so an input method's window opens there
  let feed = null, launch = null
  let frame = 0
  let recs = new WeakMap() // line -> { el, ver, cur, html }: its row as last drawn
  let drawn = null // the screen drawn last: another one (a new stream) starts the rows over
  let backFrom = 0, backTo = 0 // the kept lines backBox shows, by their number among all kept (Screen.backGone)
  let cw = 0, rh = 0 // a cell's width and height, measured
  let natural = '' // the size the watching view was scaled for
  let parked = false // (watching) its element shows nowhere (a column a narrow window hides): the stream waits

  const view = {
    el,
    get launch() { return launch },
    changed() { if (!frame) frame = requestAnimationFrame(draw) },
    open(id) {
      id = id || null
      if (id === launch) return
      view.close()
      if (!id) return
      launch = id
      if (!parked) take()
    },
    close() {
      if (!launch) return
      letGo()
      launch = null
    },
    focus() { keys?.focus({ preventScroll: true }) },
  }
  function take() {
    feed = feedFor(sym, launch)
    if (interactive) fit()
    feed.add(view)
    view.changed()
  }
  function letGo() {
    if (!feed) return
    if (interactive) feed.resize(null) // its size isn't wanted any more
    feed.remove(view)
    feed = null; drawn = null
    backBox.textContent = ''; screenBox.textContent = ''
    noteEl.hidden = true
  }

  function metrics() {
    if (cw) return true
    const r = probe.getBoundingClientRect()
    if (!r.width) return false // not laid out (hidden)
    cw = r.width / PROBE; rh = r.height
    return true
  }
  // an interactive view sizes the session's terminal to the cells that fit in it
  function fit() {
    if (!interactive || !feed || !metrics()) return
    const cs = getComputedStyle(scroller)
    const w = scroller.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight)
    const h = scroller.clientHeight - parseFloat(cs.paddingTop) - parseFloat(cs.paddingBottom)
    const cols = Math.floor(w / cw), rows = Math.floor(h / rh)
    if (cols < 10 || rows < 3) return // not laid out yet, or too small to bother the session with
    feed.resize([Math.min(cols, MAX_COLS), Math.min(rows, MAX_ROWS)])
  }
  new ResizeObserver(() => {
    cw = 0; natural = ''
    // a watching view that shows nowhere lets its stream go (a page holds six connections to the bridge), and takes it
    // again when it shows; moved from one place to another at once (a profile drawn again) it notices nothing
    const nowhere = !interactive && !el.getClientRects().length
    if (nowhere !== parked) {
      parked = nowhere
      if (launch) parked ? letGo() : take()
    }
    fit()
    view.changed()
  }).observe(el)

  function rowEl(line, cur) {
    let r = recs.get(line)
    if (!r) {
      r = { el: document.createElement('div'), ver: -1, cur: -2, html: '' }
      r.el.className = 'term-row'
      recs.set(line, r)
    }
    if (r.ver !== line.ver || r.cur !== cur) {
      const html = rowHTML(line, cur)
      if (html !== r.html) { r.el.innerHTML = html; r.html = html }
      r.ver = line.ver; r.cur = cur
    }
    return r.el
  }
  // place makes box's children els, in order, moving as few as it can (a scrolled screen moves no row but the new one)
  function place(box, els) {
    const want = new Set(els)
    let node = box.firstChild
    for (const e of els) {
      while (node && !want.has(node)) { const next = node.nextSibling; node.remove(); node = next }
      if (node === e) node = node.nextSibling
      else box.insertBefore(e, node)
    }
    while (node) { const next = node.nextSibling; node.remove(); node = next }
  }
  // the kept lines: new ones go below (their rows move up from the screen), the oldest leave the top
  function syncBack(scr) {
    const total = scr.backGone + scr.back.length
    while (backFrom < scr.backGone && backBox.firstChild) { backBox.firstChild.remove(); backFrom++ }
    backFrom = Math.max(backFrom, scr.backGone); backTo = Math.max(backTo, backFrom)
    if (backTo < total) {
      const frag = document.createDocumentFragment()
      for (let k = backTo; k < total; k++) frag.appendChild(rowEl(scr.back[k - scr.backGone], -1))
      backBox.appendChild(frag)
      backTo = total
    }
  }
  let epoch = -1
  function draw() {
    frame = 0
    if (!feed || !metrics()) return
    const scr = feed.screen
    if (scr !== drawn) { // a new stream: its screen from the start
      drawn = scr; recs = new WeakMap(); natural = ''
      backBox.textContent = ''; screenBox.textContent = ''
      backFrom = backTo = scr.backGone; epoch = scr.backEpoch
    }
    const atEnd = scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - rh
    if (interactive) {
      if (scr.backEpoch !== epoch) { epoch = scr.backEpoch; backBox.textContent = ''; backFrom = backTo = scr.backGone }
      syncBack(scr)
    }
    backBox.hidden = scr.altOn || !interactive
    el.dataset.size = `${scr.cols}x${scr.rows}` // (the tests read it)
    const cy = scr.y, cx = scr.cursorOn ? scr.x : -1
    place(screenBox, scr.lines.map((line, y) => rowEl(line, y === cy ? cx : -1)))
    if (interactive) {
      keys.style.left = (screenBox.offsetLeft + scr.x * cw) + 'px'
      keys.style.top = (screenBox.offsetTop + cy * rh) + 'px'
      if (atEnd) scroller.scrollTop = scroller.scrollHeight
    } else scale(scr)
    note()
  }
  // a watching view: the screen at its own size, scaled down to the element's width
  function scale(scr) {
    const key = `${scr.cols}x${scr.rows}x${el.clientWidth}`
    if (key === natural) return
    natural = key
    const w = scr.cols * cw, h = scr.rows * rh, s = Math.min(1, el.clientWidth / w || 1)
    screenBox.style.width = w + 'px'
    screenBox.style.transform = s < 1 ? `scale(${s})` : ''
    scroller.style.height = Math.ceil(h * s) + 'px'
  }
  function note() {
    let text = ''
    switch (feed?.status) {
      case 'connecting': text = t('term.connecting'); break
      case 'lost': text = t('term.lost'); break
      case 'ended': text = t('term.ended'); break
      case 'gone': case 'failed': text = errorText(feed.err); break
    }
    if (feed?.inputFailed && Date.now() - feed.inputFailed < 4000) text = errorText(feed.err)
    noteEl.hidden = !text
    if (noteEl.textContent !== text) noteEl.textContent = text
  }

  if (interactive) {
    keys = document.createElement('textarea')
    keys.className = 'term-keys'
    keys.setAttribute('autocomplete', 'off'); keys.setAttribute('autocorrect', 'off'); keys.setAttribute('autocapitalize', 'off')
    keys.spellcheck = false
    keys.setAttribute('aria-label', t('term.keys'))
    scroller.appendChild(keys)
    el.tabIndex = -1 // a click on the rows (a selection) leaves the focus here, so the keys still come
    // a click focuses the keys; a selection stays (and Ctrl+C copies it)
    el.addEventListener('mouseup', () => { if (getSelection().isCollapsed) view.focus() })
    el.addEventListener('keydown', onKey)
    let composing = false
    keys.addEventListener('compositionstart', () => { composing = true })
    keys.addEventListener('compositionend', () => { composing = false; typed() })
    keys.addEventListener('input', ev => { if (!composing && !ev.isComposing) typed() })
    el.addEventListener('paste', ev => {
      if (!feed) return
      ev.preventDefault(); ev.stopPropagation()
      let text = ev.clipboardData?.getData('text/plain') ?? ''
      if (!text) return
      text = text.replace(/\r\n?|\n/g, '\r')
      if (feed.screen.paste) text = '\x1b[200~' + text.replace(/\x1b\[20[01]~/g, '') + '\x1b[201~'
      toEnd()
      feed.send(text)
    })
  }
  // what is typed goes where the session is: the screen, not the lines above it
  const toEnd = () => { scroller.scrollTop = scroller.scrollHeight }
  // text typed (a key, an input method's word, a dead key's letter): what the textarea got goes to the session
  function typed() {
    const v = keys.value.replace(/[\r\n]/g, '') // Enter went as a key
    keys.value = ''
    if (v && feed) feed.send(v)
  }
  function onKey(ev) {
    if (!feed || ev.isComposing || ev.keyCode === 229) return
    if (ev.ctrlKey && !ev.altKey && !ev.metaKey && (ev.code === 'BracketRight' || ev.key === ']')) { // as symphony attach detaches
      ev.preventDefault(); ev.stopPropagation()
      onLeave()
      return
    }
    const seq = keySeq(ev, feed.screen.appCursor, hasSelection)
    if (seq === null) return // the browser's
    ev.stopPropagation() // not the page's shortcuts (Esc closes windows there)
    if (document.activeElement !== keys) view.focus()
    toEnd()
    if (seq === true) return // text: the textarea takes it, and typed() sends it
    ev.preventDefault()
    feed.send(seq)
  }
  function hasSelection() {
    const s = getSelection()
    return !s.isCollapsed && el.contains(s.anchorNode)
  }
  return view
}

const IS_MAC = /Mac|iPhone|iPad/.test(navigator.userAgent)
// keySeq is what a key sends, as xterm does: a sequence, true for text (the textarea gets it, and an input method
// works), null for the browser's own (Cmd shortcuts, Ctrl+V's paste, Ctrl+C over a selection, Ctrl+Shift ones, the
// function keys)
function keySeq(ev, app, hasSelection) {
  const { key, code, ctrlKey: ctrl, altKey: alt, shiftKey: shift, metaKey: meta } = ev
  if (meta) return null
  const mod = 1 + (shift ? 1 : 0) + (alt ? 2 : 0) + (ctrl ? 4 : 0)
  const cursor = f => mod > 1 ? `\x1b[1;${mod}${f}` : (app ? '\x1bO' : '\x1b[') + f
  const tilde = n => `\x1b[${n}${mod > 1 ? ';' + mod : ''}~`
  switch (key) {
    case 'Enter': return alt || shift ? '\x1b\r' : ctrl ? '\n' : '\r' // Shift+Enter: a new line in Claude Code's input
    case 'Backspace': return (alt ? '\x1b' : '') + (ctrl ? '\x08' : '\x7f')
    case 'Tab': return ctrl || alt ? null : shift ? '\x1b[Z' : '\t'
    case 'Escape': return '\x1b'
    case 'ArrowUp': return cursor('A')
    case 'ArrowDown': return cursor('B')
    case 'ArrowRight': return cursor('C')
    case 'ArrowLeft': return cursor('D')
    case 'Home': return cursor('H')
    case 'End': return cursor('F')
    case 'Insert': return shift ? null : tilde(2) // (Shift+Insert pastes)
    case 'Delete': return tilde(3)
    case 'PageUp': return tilde(5)
    case 'PageDown': return tilde(6)
  }
  if ([...key].length !== 1) return null // F-keys, media keys, a modifier alone: the browser's
  const letter = /^Key[A-Z]$/.test(code) ? code[3] : '' // by its place, whatever the layout writes there
  if (ctrl && !alt) {
    if (shift) return null // Ctrl+Shift+...: the browser's (developer tools, tabs, a paste without formatting)
    if (letter) {
      if (letter === 'V') return null // paste: the paste event sends it
      if (letter === 'C' && hasSelection()) return null // copy what is selected
      return String.fromCharCode(letter.charCodeAt(0) - 64)
    }
    const ctl = { ' ': '\x00', '@': '\x00', '2': '\x00', '[': '\x1b', '\\': '\x1c', '^': '\x1e', '_': '\x1f', '/': '\x1f', '?': '\x7f' }[key]
    return ctl ?? null
  }
  if (alt && !ctrl && !IS_MAC) return '\x1b' + (letter ? (shift ? letter : letter.toLowerCase()) : key) // Alt is Meta: ESC first
  return true // text (also AltGr's, Ctrl+Alt on Windows, and a Mac's Option characters)
}
