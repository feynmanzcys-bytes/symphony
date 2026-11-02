package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestScreen: attach shows the session's output as it comes, minus win32-input-mode (the Windows pseudo console asks
// its host for it; a console that turned it on would send Ctrl-] as a sequence), also when a write cuts the sequence;
// as it ends, it turns off the modes the output left on, the alternate screen and the keyboard modes only when the
// output turned them on.
func TestScreen(t *testing.T) {
	var out bytes.Buffer
	s := &screen{w: &out}
	for _, chunk := range []string{"\x1b[?9001h\x1b[?1004h\x1b[31mred\x1b[0m a\x1b[?90", "01hb \x1b]0;bob\x07c\x1b"} {
		if n, err := s.Write([]byte(chunk)); n != len(chunk) || err != nil {
			t.Fatalf("write: %d %v", n, err)
		}
	}
	if got := out.String(); got != "\x1b[?1004h\x1b[31mred\x1b[0m ab \x1b]0;bob\x07c" {
		t.Fatalf("shown: %q", got)
	}
	s.Flush() // the stream ended in the middle of a sequence
	if got := out.String(); !strings.HasSuffix(got, "c\x1b") {
		t.Fatalf("a cut-off sequence should go out at the end: %q", got)
	}
	reset := string(s.reset())
	for _, mode := range []string{"\x1b[?1000l", "\x1b[?1004l", "\x1b[?2004l", "\x1b[?9001l", "\x1b[?25h"} {
		if !strings.Contains(reset, mode) {
			t.Fatalf("the reset lacks %q: %q", mode, reset)
		}
	}
	if strings.Contains(reset, "1049l") || strings.Contains(reset, "<") {
		t.Fatalf("the output never went to the alternate screen or pushed keyboard modes: %q", reset)
	}

	// the alternate screen and pushed keyboard modes, the sequences cut by the writes
	s = &screen{w: &out}
	for _, chunk := range []string{"\x1b[<u\x1b[?10", "49h\x1b[>1u x \x1b[>", "3u\x1b[<u"} {
		_, _ = s.Write([]byte(chunk))
	}
	if reset := string(s.reset()); !strings.HasSuffix(reset, "\x1b[<1u\x1b[?1049l") {
		t.Fatalf("one keyboard mode is still pushed and the output is on the alternate screen: %q", reset)
	}
	_, _ = s.Write([]byte("\x1b[?1049l\x1b[<3u"))
	if reset := string(s.reset()); strings.Contains(reset, "1049l") || strings.Contains(reset, "<") {
		t.Fatalf("the output came back and popped everything: %q", reset)
	}
	// a long title doesn't wait for its end
	out.Reset()
	s = &screen{w: &out}
	long := "\x1b]0;" + strings.Repeat("x", 100)
	_, _ = s.Write([]byte(long))
	if out.String() != long {
		t.Fatalf("a long sequence should go out at once: %d of %d bytes", out.Len(), len(long))
	}
}
