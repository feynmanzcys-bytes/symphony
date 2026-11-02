package bridge

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// vt is a small model of a terminal's screen: a grid of cols x rows cells fed with a session's output, so that the
// last lines of the screen (an exit notice, a failed launch's error) are what the screen showed, not everything that
// was ever drawn over it: text that was typed and erased, a status line redrawn a hundred times. It knows what Ink
// (Claude Code's renderer) and a shell use: printing, line feeds and scrolling, the cursor moves, erasing in the line
// and the display, inserting and deleting characters and lines, a scroll region, saving the cursor. Colors, modes and
// strings (OSC) are read and ignored. An East Asian wide or fullwidth rune takes two cells, as the terminal gives it and
// Claude Code lays its lines out (a column move after 中文 lands two cells further).
type vt struct {
	cols, rows int
	cells      [][]rune // cells[y][x], 0 for an empty cell
	x, y       int      // the cursor
	top, bot   int      // the scroll region, rows top..bot inclusive
	savedX     int
	savedY     int
	wrapNext   bool // the cursor is past the last column: the next rune wraps
}

func newVT(cols, rows int) *vt {
	cols, rows = max(cols, 1), max(rows, 1)
	v := &vt{cols: cols, rows: rows, cells: make([][]rune, rows), bot: rows - 1}
	for i := range v.cells {
		v.cells[i] = make([]rune, cols)
	}
	return v
}

// lines is the screen's text, each row trimmed on the right, the trailing empty rows dropped.
func (v *vt) lines() []string {
	out := make([]string, 0, v.rows)
	for _, row := range v.cells {
		var b strings.Builder
		for _, c := range row {
			switch c {
			case wideTail:
				continue
			case 0:
				c = ' '
			}
			b.WriteRune(c)
		}
		out = append(out, strings.TrimRight(b.String(), " "))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// feed reads output. It takes the whole output at once: a sequence cut at the end is dropped.
func (v *vt) feed(p []byte) {
	for len(p) > 0 {
		c := p[0]
		switch {
		case c == 0x1b:
			n := v.escape(p)
			if n == 0 {
				return // cut off at the end
			}
			p = p[n:]
			continue
		case c == '\r':
			v.x, v.wrapNext = 0, false
		case c == '\n', c == 0x0b, c == 0x0c:
			v.lineFeed()
		case c == '\b':
			if v.wrapNext {
				v.wrapNext = false
			} else if v.x > 0 {
				v.x--
			}
		case c == '\t':
			v.x = min((v.x/8+1)*8, v.cols-1)
			v.wrapNext = false
		case c < 0x20 || c == 0x7f:
			// other control characters: nothing
		default:
			r, size := utf8.DecodeRune(p)
			if r == utf8.RuneError && size <= 1 {
				if !utf8.FullRune(p) {
					return // cut off at the end
				}
			}
			v.put(r)
			p = p[size:]
			continue
		}
		p = p[1:]
	}
}

// wideTail marks the second cell of a wide rune.
const wideTail rune = -1

func (v *vt) put(r rune) {
	w := 1
	if wide(r) {
		w = 2
	}
	if v.wrapNext || v.x+w > v.cols {
		v.x, v.wrapNext = 0, false
		v.lineFeed()
	}
	row := v.cells[v.y]
	// drawing over half of a wide rune: the other half goes too
	if row[v.x] == wideTail && v.x > 0 {
		row[v.x-1] = 0
	}
	if w == 1 && v.x+1 < v.cols && row[v.x+1] == wideTail && wide(row[v.x]) {
		row[v.x+1] = 0
	}
	row[v.x] = r
	if w == 2 {
		row[v.x+1] = wideTail
	}
	if v.x+w < v.cols {
		v.x += w
	} else {
		v.x = v.cols - 1
		v.wrapNext = true
	}
}

// wide says whether a rune takes two cells: East Asian Wide and Fullwidth (CJK, Hangul, kana, fullwidth forms) and the
// emoji blocks, by range.
func wide(r rune) bool {
	switch {
	case r < 0x1100:
		return false
	case r <= 0x115f, // Hangul Jamo
		r >= 0x2e80 && r <= 0x303e, // CJK radicals, kangxi, punctuation
		r >= 0x3041 && r <= 0x33ff, // kana, bopomofo, CJK compatibility
		r >= 0x3400 && r <= 0x4dbf, // CJK extension A
		r >= 0x4e00 && r <= 0x9fff, // CJK unified
		r >= 0xa000 && r <= 0xa4cf, // Yi
		r >= 0xac00 && r <= 0xd7a3, // Hangul syllables
		r >= 0xf900 && r <= 0xfaff, // CJK compatibility ideographs
		r >= 0xfe30 && r <= 0xfe4f, // CJK compatibility forms
		r >= 0xff00 && r <= 0xff60, // fullwidth forms
		r >= 0xffe0 && r <= 0xffe6,
		r >= 0x1f300 && r <= 0x1f64f, // emoji
		r >= 0x1f900 && r <= 0x1f9ff,
		r >= 0x20000 && r <= 0x3fffd: // CJK extensions B and on
		return true
	}
	return false
}

func (v *vt) lineFeed() {
	if v.y == v.bot {
		v.scrollUp(1)
	} else if v.y < v.rows-1 {
		v.y++
	}
}

// scrollUp moves the scroll region up n rows: the top ones go, empty ones come in at the bottom.
func (v *vt) scrollUp(n int) {
	for ; n > 0; n-- {
		copy(v.cells[v.top:v.bot], v.cells[v.top+1:v.bot+1])
		v.cells[v.bot] = make([]rune, v.cols)
	}
}

func (v *vt) scrollDown(n int) {
	for ; n > 0; n-- {
		copy(v.cells[v.top+1:v.bot+1], v.cells[v.top:v.bot])
		v.cells[v.top] = make([]rune, v.cols)
	}
}

func (v *vt) clearRow(y, from, to int) {
	for x := max(from, 0); x < min(to, v.cols); x++ {
		v.cells[y][x] = 0
	}
}

// escape reads one escape sequence at the start of p (which begins with ESC) and returns its length, 0 when p ends
// before the sequence does.
func (v *vt) escape(p []byte) int {
	if len(p) < 2 {
		return 0
	}
	switch p[1] {
	case '[':
		return v.csi(p)
	case ']', 'P', '^', '_', 'X': // OSC, DCS, PM, APC, SOS: a string up to BEL or ST
		for i := 2; i < len(p); i++ {
			if p[i] == 0x07 {
				return i + 1
			}
			if p[i] == 0x1b {
				if i+1 < len(p) && p[i+1] == '\\' {
					return i + 2
				}
				return i // another sequence starts: the string ends here
			}
		}
		return 0
	case '7':
		v.savedX, v.savedY = v.x, v.y
		return 2
	case '8':
		v.x, v.y, v.wrapNext = v.savedX, v.savedY, false
		return 2
	case 'D':
		v.lineFeed()
		return 2
	case 'M': // reverse index
		if v.y == v.top {
			v.scrollDown(1)
		} else if v.y > 0 {
			v.y--
		}
		return 2
	case 'E':
		v.x = 0
		v.lineFeed()
		return 2
	case 'c': // full reset
		*v = *newVT(v.cols, v.rows)
		return 2
	}
	// ESC, intermediates (0x20-0x2f), a final byte (0x30-0x7e): nothing we model
	i := 1
	for i < len(p) && p[i] >= 0x20 && p[i] <= 0x2f {
		i++
	}
	if i >= len(p) {
		return 0
	}
	return i + 1
}

// csi reads a control sequence (ESC [ parameters intermediates final) and acts on it.
func (v *vt) csi(p []byte) int {
	i := 2
	for i < len(p) && p[i] >= 0x30 && p[i] <= 0x3f {
		i++
	}
	params := string(p[2:i])
	for i < len(p) && p[i] >= 0x20 && p[i] <= 0x2f {
		i++
	}
	if i >= len(p) {
		return 0
	}
	final := p[i]
	if len(params) > 0 && (params[0] == '?' || params[0] == '>' || params[0] == '<' || params[0] == '=') {
		// private modes (the alternate screen, the cursor's shape, mouse and keyboard reports): nothing drawn changes;
		// 1049h/l switch screens, which we don't keep apart: the alternate screen's frame simply draws over this one
		return i + 1
	}
	n := func(k, def int) int {
		fields := strings.Split(params, ";")
		if k >= len(fields) || fields[k] == "" {
			return def
		}
		x, err := strconv.Atoi(fields[k])
		if err != nil || x < 0 {
			return def
		}
		return x
	}
	v.wrapNext = false
	switch final {
	case 'A':
		v.y = max(v.y-max(n(0, 1), 1), 0)
	case 'B', 'e':
		v.y = min(v.y+max(n(0, 1), 1), v.rows-1)
	case 'C', 'a':
		v.x = min(v.x+max(n(0, 1), 1), v.cols-1)
	case 'D':
		v.x = max(v.x-max(n(0, 1), 1), 0)
	case 'E':
		v.x, v.y = 0, min(v.y+max(n(0, 1), 1), v.rows-1)
	case 'F':
		v.x, v.y = 0, max(v.y-max(n(0, 1), 1), 0)
	case 'G', '`':
		v.x = min(max(n(0, 1), 1), v.cols) - 1
	case 'd':
		v.y = min(max(n(0, 1), 1), v.rows) - 1
	case 'H', 'f':
		v.y = min(max(n(0, 1), 1), v.rows) - 1
		v.x = min(max(n(1, 1), 1), v.cols) - 1
	case 'J':
		switch n(0, 0) {
		case 0:
			v.clearRow(v.y, v.x, v.cols)
			for y := v.y + 1; y < v.rows; y++ {
				v.clearRow(y, 0, v.cols)
			}
		case 1:
			for y := 0; y < v.y; y++ {
				v.clearRow(y, 0, v.cols)
			}
			v.clearRow(v.y, 0, v.x+1)
		case 2, 3:
			for y := 0; y < v.rows; y++ {
				v.clearRow(y, 0, v.cols)
			}
		}
	case 'K':
		switch n(0, 0) {
		case 0:
			v.clearRow(v.y, v.x, v.cols)
		case 1:
			v.clearRow(v.y, 0, v.x+1)
		case 2:
			v.clearRow(v.y, 0, v.cols)
		}
	case '@': // insert blanks at the cursor
		k := min(max(n(0, 1), 1), v.cols-v.x)
		row := v.cells[v.y]
		copy(row[v.x+k:], row[v.x:v.cols-k])
		v.clearRow(v.y, v.x, v.x+k)
	case 'P': // delete characters at the cursor
		k := min(max(n(0, 1), 1), v.cols-v.x)
		row := v.cells[v.y]
		copy(row[v.x:], row[v.x+k:])
		v.clearRow(v.y, v.cols-k, v.cols)
	case 'X': // erase characters at the cursor
		v.clearRow(v.y, v.x, v.x+max(n(0, 1), 1))
	case 'L': // insert lines at the cursor, within the scroll region
		if v.y >= v.top && v.y <= v.bot {
			top := v.top
			v.top = v.y
			v.scrollDown(min(max(n(0, 1), 1), v.bot-v.y+1))
			v.top = top
		}
	case 'M': // delete lines at the cursor
		if v.y >= v.top && v.y <= v.bot {
			top := v.top
			v.top = v.y
			v.scrollUp(min(max(n(0, 1), 1), v.bot-v.y+1))
			v.top = top
		}
	case 'S':
		v.scrollUp(min(max(n(0, 1), 1), v.bot-v.top+1))
	case 'T':
		v.scrollDown(min(max(n(0, 1), 1), v.bot-v.top+1))
	case 'r': // the scroll region; the cursor goes home
		top, bot := n(0, 1), n(1, v.rows)
		if top < 1 {
			top = 1
		}
		if bot > v.rows || bot < 1 {
			bot = v.rows
		}
		if top < bot {
			v.top, v.bot = top-1, bot-1
			v.x, v.y = 0, 0
		}
	case 's':
		v.savedX, v.savedY = v.x, v.y
	case 'u':
		v.x, v.y = v.savedX, v.savedY
	}
	return i + 1
}
