package main

import (
	"bytes"
	"io"
	"regexp"
	"strconv"
)

// screen writes a session's output to this terminal, minus win32-input-mode (CSI ?9001h), which the Windows pseudo
// console asks of its host: a console that turned it on sends every key as a sequence, Ctrl-] too, and attach would
// never see its detach key. It notes the modes the output leaves on, so that reset can turn them off as attach ends:
// otherwise the shell attach ran in would get the session's mouse and focus reports, its keys in another encoding, or
// stay on the alternate screen.
type screen struct {
	w     io.Writer
	carry []byte // an escape sequence the last write cut off, written with the next
	alt   bool   // the output went to the alternate screen and didn't come back
	kitty int    // the keyboard modes the output pushed (CSI > flags u) and didn't pop (CSI < n u)
}

var (
	winInput = []byte("\x1b[?9001h")
	altMode  = regexp.MustCompile(`\x1b\[\?(?:1049|1047|47)([hl])`)
	kittyKey = regexp.MustCompile(`\x1b\[([<>])(\d*)u`)
)

func (s *screen) Write(p []byte) (int, error) {
	buf := append(s.carry, p...)
	s.carry = nil
	// an escape sequence still open at the end (no final byte yet) waits for the rest
	if i := bytes.LastIndexByte(buf, 0x1b); i >= 0 && len(buf)-i < 32 && !finished(buf[i:]) {
		s.carry = append([]byte(nil), buf[i:]...)
		buf = buf[:i]
	}
	for _, m := range altMode.FindAllSubmatch(buf, -1) {
		s.alt = m[1][0] == 'h'
	}
	for _, m := range kittyKey.FindAllSubmatch(buf, -1) {
		if m[1][0] == '>' {
			s.kitty++
		} else if n, _ := strconv.Atoi(string(m[2])); n > 1 {
			s.kitty -= n
		} else {
			s.kitty--
		}
		s.kitty = max(s.kitty, 0)
	}
	if _, err := s.w.Write(bytes.ReplaceAll(buf, winInput, nil)); err != nil {
		return 0, err
	}
	return len(p), nil
}

// finished says whether seq, which starts with ESC, is a whole escape sequence: a CSI with its final byte, an OSC with
// its terminator, or ESC and one more byte.
func finished(seq []byte) bool {
	if len(seq) < 2 {
		return false
	}
	switch seq[1] {
	case '[':
		for _, b := range seq[2:] {
			if b >= 0x40 && b <= 0x7e {
				return true
			}
		}
		return false
	case ']':
		return bytes.IndexByte(seq, 0x07) >= 0 || bytes.Contains(seq, []byte("\x1b\\"))
	}
	return true
}

// Flush writes what a cut-off sequence held back.
func (s *screen) Flush() {
	if len(s.carry) > 0 {
		_, _ = s.w.Write(bytes.ReplaceAll(s.carry, winInput, nil))
		s.carry = nil
	}
}

// reset turns off what the session's output may have left on in this terminal: mouse and focus reports, bracketed
// paste, color-scheme reports (CSI ?2031h: a terminal that supports them would put ^[[?997;1n into the shell at every
// switch between dark and light), win32-input-mode, xterm's modified keys, the keyboard modes it pushed; it shows the
// cursor and leaves the alternate screen when the output went there. Then it clears what's below the cursor (the line
// after it is drawn inside the session's last box otherwise, its bottom border showing after the message) and empties
// the window's title the banner set.
func (s *screen) reset() []byte {
	r := "\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1004l\x1b[?2004l\x1b[?2031l\x1b[?9001l\x1b[>4m\x1b[?25h"
	if s.kitty > 0 {
		r += "\x1b[<" + strconv.Itoa(s.kitty) + "u"
	}
	if s.alt {
		r += "\x1b[?1049l"
	}
	r += "\x1b[J\x1b]0;\x07"
	return []byte(r)
}
