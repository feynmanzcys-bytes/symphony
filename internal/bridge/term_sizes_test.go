package bridge

import (
	"bytes"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// streamReader reads a terminal stream in the background, for tests that look at what came so far.
type streamReader struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func readStream(t *testing.T, url string) *streamReader {
	t.Helper()
	resp, err := pageGet(url)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("GET %s: %s", url, resp.Status)
	}
	t.Cleanup(func() { resp.Body.Close() })
	s := &streamReader{}
	go func() {
		chunk := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(chunk)
			s.mu.Lock()
			s.buf.Write(chunk[:n])
			s.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	return s
}

func (s *streamReader) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// TestTermStreamSizes: a window that draws the screen itself (?sizes=1, the page's terminal view) is told the
// terminal's size before the history and whenever it changes, as xterm reports a size (CSI 8 ; rows ; cols t); its own
// size, when it gives one, comes after the size the history was laid out at. A window that doesn't ask (symphony
// attach) gets the output alone, and the history never holds a note.
func TestTermStreamSizes(t *testing.T) {
	srv, base, f := newLaunchBridge(t)
	f.screen = "hello"
	web := client{t: t, base: base}
	ann, _ := srv.Hub.CreateAccount("ann")
	id := web.ok("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: t.TempDir()})["id"].(string)
	term := srv.Launcher.Terms.Get(id)
	waitFor(t, "the process's first output", func() bool { h, _, d := term.Attach(0, 0); d(); return string(h) == "hello" })

	viewer := readStream(t, base+"/api/terminal/"+id+"/stream?sizes=1")
	plain := readStream(t, base+"/api/terminal/"+id+"/stream")
	waitFor(t, "the viewer's history, after the size it was laid out at", func() bool { return viewer.text() == "\x1b[8;40;120thello" })
	waitFor(t, "the plain window's history", func() bool { return plain.text() == "hello" })

	if st := web.status("POST", "/api/terminal/"+id+"/resize", map[string]int{"cols": 100, "rows": 30}); st != http.StatusNoContent {
		t.Fatalf("resize: %d", st)
	}
	waitFor(t, "the new size reaches the viewer", func() bool { return viewer.text() == "\x1b[8;40;120thello\x1b[8;30;100t" })
	// the same size again changes nothing, and tells nobody
	web.status("POST", "/api/terminal/"+id+"/resize", map[string]int{"cols": 100, "rows": 30})
	if _, err := f.proc(id).outW.Write([]byte(" world")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the output after the resize", func() bool {
		return strings.HasSuffix(viewer.text(), "world") && strings.HasSuffix(plain.text(), "world")
	})
	if got := viewer.text(); got != "\x1b[8;40;120thello\x1b[8;30;100t world" {
		t.Fatalf("the viewer's stream: %q", got)
	}
	if got := plain.text(); got != "hello world" {
		t.Fatalf("a window that didn't ask for sizes got: %q", got)
	}

	// a viewer that sizes the terminal itself: the size the history was laid out at, then its own
	sized := readStream(t, base+"/api/terminal/"+id+"/stream?sizes=1&cols=90&rows=20")
	waitFor(t, "the sized viewer's stream", func() bool {
		return sized.text() == "\x1b[8;30;100thello world\x1b[8;20;90t"
	})
	waitFor(t, "the other viewer hears of the new size too", func() bool { return strings.HasSuffix(viewer.text(), "world\x1b[8;20;90t") })
	if got := plain.text(); got != "hello world" {
		t.Fatalf("a window that didn't ask for sizes got: %q", got)
	}
	h, _, d := term.Attach(0, 0)
	d()
	if string(h) != "hello world" || strings.Contains(term.Screen(5), "8;") {
		t.Fatalf("the history and the screen must not hold the notes: %q / %q", h, term.Screen(5))
	}
	f.proc(id).mu.Lock()
	size := f.proc(id).size
	f.proc(id).mu.Unlock()
	if size != [2]int{90, 20} {
		t.Fatalf("the last size should reach the pty: %v", size)
	}
}
