package bridge

import (
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
