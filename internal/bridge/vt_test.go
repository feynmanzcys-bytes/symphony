package bridge

import (
	"bytes"
	"strings"
	"testing"
)

// TestVTInkRedraw: Ink (Claude Code's renderer) redraws its frame by moving the cursor up and erasing the lines; text
// that was typed and then deleted, and a status line redrawn, leave nothing behind on the screen model, where the raw
// output laid out one redraw after another (plainText) keeps them.
func TestVTInkRedraw(t *testing.T) {
	frame := func(input string) string {
		return "╭──────╮\r\n│ > " + input + "\x1b[K\r\n╰──────╯\r\n  ⏵⏵ bypass permissions on (shift+tab to cycle)\x1b[K\r\n"
	}
	var raw strings.Builder
	raw.WriteString(frame(""))
	for _, typed := range []string{"要一个一个", "要一个一个;ugygtfrdrderd", "要一个一个;ugygtfrd", "要一个", ""} {
		raw.WriteString("\x1b[4A") // back up to the frame's first line
		raw.WriteString(frame(typed))
	}
	raw.WriteString("\x1b[4A\x1b[J" + `Try "refactor <filepath>"` + "\r\n\x1b[?25h\x1b(B")
	v := newVT(80, 24)
	v.feed([]byte(raw.String()))
	if got := strings.Join(v.lines(), "\n"); got != `Try "refactor <filepath>"` {
		t.Fatalf("the screen should show the last frame only:\n%q", got)
	}
	// plainText, meant for the trust prompt, still has every redraw
	if got := plainText([]byte(raw.String())); !strings.Contains(got, "ugygtfrdrderd") {
		t.Fatalf("plainText should lay the redraws out one after another: %q", got)
	}
}

// TestVTEditing: the cursor moves, erasing, inserting and deleting characters and lines, scrolling and the scroll region,
// saving the cursor, wide characters, a wrap at the last column.
func TestVTEditing(t *testing.T) {
	cases := []struct {
		name, raw, want string
	}{
		{"print and line feed", "ab\r\ncd", "ab\ncd"},
		{"overwrite", "hello\rJ", "Jello"},
		{"cursor moves", "abcdef\x1b[3DX\x1b[2CY", "abcXefY"},
		{"erase to end of line", "abcdef\x1b[3G\x1b[K", "ab"},
		{"erase to start of line", "abcdef\x1b[3G\x1b[1K", "   def"},
		{"erase line", "abcdef\x1b[2K", ""},
		{"erase below", "a\r\nb\r\nc\x1b[2;1H\x1b[J", "a"},
		{"erase above", "a\r\nb\r\nc\x1b[2;1H\x1b[1J", "\n\nc"},
		{"erase all", "a\r\nb\x1b[2J", ""},
		{"insert characters", "abcd\x1b[2G\x1b[2@", "a  bcd"},
		{"delete characters", "abcd\x1b[2G\x1b[2P", "ad"},
		{"erase characters", "abcd\x1b[2G\x1b[2X", "a  d"},
		{"insert line", "a\r\nb\r\nc\x1b[2;1H\x1b[L", "a\n\nb\nc"},
		{"delete line", "a\r\nb\r\nc\x1b[2;1H\x1b[M", "a\nc"},
		{"column and row", "\x1b[3;5Hx\x1b[2dy", "\n     y\n    x"},
		{"save and restore", "ab\x1b7cd\x1b8X", "abXd"},
		{"save and restore (CSI)", "ab\x1b[scd\x1b[uX", "abXd"},
		{"tab", "a\tb", "a       b"},
		{"backspace", "abc\bX", "abX"},
		{"wide characters", "你好\r\n世界", "你好\n世界"},
		{"wide characters take two cells", "\x1b[3G中文\x1b[9Gab", "  中文  ab"},
		{"a column move after wide characters", "结果是\x1b[8Gx", "结果是 x"},
		{"a wide character over half of another", "中文\x1b[2GX", " X文"},
		{"wide characters wrap whole", "abcdefghi中", "abcdefghi\n中"},
		{"wrap at the last column", strings.Repeat("x", 10) + "yz", strings.Repeat("x", 10) + "\nyz"},
		{"OSC and other escapes ignored", "\x1b]0;title\x07a\x1b(Bb\x1b=c\x1b[?25ld\x1b[38;2;1;2;3me", "abcde"},
		{"scroll off the bottom", "1\r\n2\r\n3\r\n4\r\n5\r\n6", "2\n3\n4\n5\n6"},
		{"scroll region", "1\r\n2\r\n3\r\n4\r\n5\x1b[2;4r\x1b[4;1H\r\nX", "1\n3\n4\nX\n5"},
		{"scroll up", "1\r\n2\r\n3\x1b[S", "2\n3"},
		{"reverse index at the top", "1\r\n2\x1b[1;1H\x1bM", "\n1\n2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := newVT(10, 5)
			v.feed([]byte(c.raw))
			if got := strings.Join(v.lines(), "\n"); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestVTCutOff: output cut in the middle of an escape sequence or a rune drops that bit and nothing else.
func TestVTCutOff(t *testing.T) {
	for _, raw := range []string{"ab\x1b[3", "ab\x1b]0;tit", "ab\xe4\xbd", "ab\x1b"} {
		v := newVT(10, 5)
		v.feed([]byte(raw))
		if got := strings.Join(v.lines(), "\n"); got != "ab" {
			t.Fatalf("%q: got %q", raw, got)
		}
	}
}

// (plainText is what the trust prompt is read from, on every platform)
func TestPlainText(t *testing.T) {
	raw := []byte("\x1b[1m❯\x1b[0m\x1b[2CNo,\x1b[1Cexit\r\n\x1b[3CYes,\x1b[CI\x1b[Ctrust\x1b[Cthis\x1b[Cfolder\x1b[K\r\n")
	if got := plainText(raw); got != "❯  No, exit\n   Yes, I trust this folder\n" {
		t.Fatalf("plainText: %q", got)
	}
	if !hasTrustPrompt(plainText(raw)) || hasTrustPrompt("trust me") {
		t.Fatal("the trust prompt should be found through cursor moves and nowhere else")
	}
	// Claude Code 2.1.316 draws the gaps with moves to a column (CHA): the words keep them
	status := "\x1b[3G\x1b[38;2;255;107;128m⏵⏵\x1b[6Gbypass\x1b[13Gpermissions\x1b[25Gon\x1b[38;2;153;153;153m (shift+tab\x1b[39Gto\x1b[42Gcycle)\x1b[49G·\x1b[51G←\x1b[53Gfor\x1b[57Gagents\x1b[104G○"
	if got, want := plainText([]byte(status)), "  ⏵⏵ bypass permissions on (shift+tab to cycle) · ← for agents"+strings.Repeat(" ", 41)+"○"; got != want {
		t.Fatalf("moves to a column:\n got %q\nwant %q", got, want)
	}
	trust := "\x1b[2G❯\x1b[4GNo,\x1b[8Gexit\r\n\x1b[4GYes,\x1b[9GI\x1b[11Gtrust\x1b[17Gthis\x1b[22Gfolder\r\n"
	if got := plainText([]byte(trust)); got != " ❯ No, exit\n   Yes, I trust this folder\n" || !hasTrustPrompt(got) {
		t.Fatalf("the trust prompt drawn with moves to a column: %q", got)
	}
	// two-byte escape sequences and ones with intermediates leave nothing behind: ESC ( B (a charset), ESC 7 and ESC 8
	// (save and restore the cursor), ESC = (the keypad)
	if got := plainText([]byte("a\x1b(Bb\x1b7c\x1b8d\x1b=e\x1b>f\x1b[?25lg")); got != "abcdefg" {
		t.Fatalf("escape sequences left bits: %q", got)
	}
	// a move back to a column already passed keeps the words apart; a carriage return starts the line's columns again
	if got := plainText([]byte("abc\x1b[2Gx\x1b[Gy\rz\x1b[3Gw")); got != "abc x yz w" {
		t.Fatalf("moves back: %q", got)
	}
	// a session's output may ask to move any distance: it becomes at most maxForward spaces, never a huge allocation
	for _, n := range []string{"99999999999999999999", "2000000000", "1002"} {
		for _, op := range []string{"C", "G"} {
			if got := plainText([]byte("a\x1b[" + n + op + "b")); got != "a"+strings.Repeat(" ", maxForward)+"b" {
				t.Fatalf("a move %s%s: %d characters", n, op, len(got))
			}
		}
	}
	// and all the moves together become at most forwardBudget spaces, then one each
	many := bytes.Repeat([]byte("x\x1b[1000C"), 32768)
	if got := len(plainText(many)); got > 2*32768+forwardBudget {
		t.Fatalf("32768 moves became %d characters", got)
	}
}
