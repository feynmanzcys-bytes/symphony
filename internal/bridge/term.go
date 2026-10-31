package bridge

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ptyProc is a child process on a pseudo-terminal, however the platform makes one (pty_*.go): its output is read
// from it, its input written to it.
type ptyProc interface {
	io.Reader
	io.Writer
	Resize(cols, rows int) error
	Wait() error  // waits for the process to exit
	Close() error // closes the output once the process is gone, which ends the reader
	Kill() error
}

const (
	// termHistory is how much of a terminal's output is kept, so a window that opens later shows the recent screen
	termHistory = 256 << 10
	// termChunk is the most one read from the pty takes
	termChunk = 4096
	// termEndedTTL is how long an ended terminal stays, so its last output can still be read
	termEndedTTL = 10 * time.Minute
	// termCols and termRows are a session's size until a window attaches with its own
	termCols, termRows = 120, 40
)

var (
	ErrTermUnknown = newError("terminal_unknown_session", "no terminal for this session")
	ErrTermEnded   = newError("terminal_ended", "the session's process has ended")
)

// Term is one session's terminal, hosted by the bridge: claude runs on a pseudo-terminal the bridge owns, its
// output goes to a history buffer and to every attached client (symphony attach), and their keystrokes go back.
// Nothing opens on the screen unless asked.
type Term struct {
	ID   string
	Name string // the employee's name, for the window title

	mu      sync.Mutex
	proc    ptyProc
	hist    []byte
	clients map[*termClient]struct{}
	cols    int
	rows    int
	exited  chan struct{}
	exitErr error
	ended   time.Time
}

// termClient is one attached window: output reaches it through out; a window that can't keep up is dropped
// (it reconnects and gets the history again).
type termClient struct {
	out  chan []byte
	gone chan struct{}
	once sync.Once
}

func (c *termClient) close() { c.once.Do(func() { close(c.gone) }) }

// newTerm starts argv in dir on a pseudo-terminal and follows its output in the background.
func newTerm(id, name, dir string, env []string, argv []string) (*Term, error) {
	proc, err := startPTY(dir, env, argv, termCols, termRows)
	if err != nil {
		return nil, err
	}
	return hostTerm(id, name, proc), nil
}

// hostTerm follows a started process's output in the background.
func hostTerm(id, name string, proc ptyProc) *Term {
	t := &Term{ID: id, Name: name, proc: proc, clients: map[*termClient]struct{}{}, cols: termCols, rows: termRows, exited: make(chan struct{})}
	go t.pump()
	return t
}

// pump copies the process's output to the history and the clients until it ends.
func (t *Term) pump() {
	buf := make([]byte, termChunk)
	for {
		n, err := t.proc.Read(buf)
		if n > 0 {
			t.push(buf[:n])
		}
		if err != nil {
			break
		}
	}
	err := t.proc.Wait()
	_ = t.proc.Close()
	t.mu.Lock()
	t.exitErr, t.ended = err, time.Now()
	if t.exitErr == nil {
		t.exitErr = errors.New("exit 0")
	}
	for c := range t.clients {
		c.close()
		delete(t.clients, c)
	}
	close(t.exited)
	t.mu.Unlock()
}

func (t *Term) push(b []byte) {
	chunk := append([]byte(nil), b...)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.hist = append(t.hist, chunk...)
	if len(t.hist) > termHistory {
		t.hist = t.hist[len(t.hist)-termHistory:]
	}
	for c := range t.clients {
		select {
		case c.out <- chunk:
		default:
			c.close()
			delete(t.clients, c)
		}
	}
}

// Attach registers a window of cols x rows: it gets the history so far and then every new chunk on out, until
// gone is closed (the process ended, or the window fell behind). Call detach when the window closes.
func (t *Term) Attach(cols, rows int) (history []byte, c *termClient, detach func()) {
	c = &termClient{out: make(chan []byte, 1024), gone: make(chan struct{})}
	t.mu.Lock()
	history = append([]byte(nil), t.hist...)
	if t.ended.IsZero() {
		t.clients[c] = struct{}{}
	} else {
		c.close()
	}
	t.mu.Unlock()
	if cols > 0 && rows > 0 {
		_ = t.Resize(cols, rows)
	}
	return history, c, func() {
		t.mu.Lock()
		delete(t.clients, c)
		t.mu.Unlock()
		c.close()
	}
}

// Write sends keystrokes to the process.
func (t *Term) Write(b []byte) error {
	if t.Ended() {
		return ErrTermEnded
	}
	_, err := t.proc.Write(b)
	return err
}

func (t *Term) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 || t.Ended() {
		return nil
	}
	t.mu.Lock()
	same := t.cols == cols && t.rows == rows
	t.cols, t.rows = cols, rows
	t.mu.Unlock()
	if same {
		return nil
	}
	return t.proc.Resize(cols, rows)
}

func (t *Term) Ended() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return !t.ended.IsZero()
}

// Exited is closed once the process is gone; ExitErr says how it ended.
func (t *Term) Exited() <-chan struct{} { return t.exited }
func (t *Term) ExitErr() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.exitErr
}

func (t *Term) Kill() { _ = t.proc.Kill() }

// Clients is how many windows are attached.
func (t *Term) Clients() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.clients)
}

var (
	ansi    = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]|[\x00-\x08\x0b-\x1f\x7f]`)
	forward = regexp.MustCompile(`\x1b\[(\d*)C`) // cursor forward: Ink draws the gaps between words with it
	spaces  = regexp.MustCompile(`\s+`)
)

// plainText is terminal output as text: cursor moves to the right become spaces, the other escape sequences go.
func plainText(raw []byte) string {
	moved := forward.ReplaceAllFunc(raw, func(m []byte) []byte {
		n, _ := strconv.Atoi(string(m[2 : len(m)-1]))
		return bytes.Repeat([]byte(" "), max(n, 1))
	})
	return strings.ReplaceAll(string(ansi.ReplaceAll(moved, nil)), "\r", "")
}

// Screen is the recent output as plain text: the last n non-empty lines.
func (t *Term) Screen(n int) string {
	t.mu.Lock()
	hist := append([]byte(nil), t.hist...)
	t.mu.Unlock()
	return lastLines(plainText(hist), n)
}

// Terms is the bridge's terminals by launch id.
type Terms struct {
	mu sync.Mutex
	m  map[string]*Term
}

func (ts *Terms) put(t *Term) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.m == nil {
		ts.m = map[string]*Term{}
	}
	ts.m[t.ID] = t
}

// Get is the terminal of a launch, or nil.
func (ts *Terms) Get(id string) *Term {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.m[id]
}

// Prune drops terminals whose process ended long ago.
func (ts *Terms) Prune() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	for id, t := range ts.m {
		t.mu.Lock()
		old := !t.ended.IsZero() && time.Since(t.ended) > termEndedTTL
		t.mu.Unlock()
		if old {
			delete(ts.m, id)
		}
	}
}

// Live says whether a launch's process is still running.
func (ts *Terms) Live(id string) bool {
	t := ts.Get(id)
	return t != nil && !t.Ended()
}

// KillAll ends every running process (the bridge is shutting down).
func (ts *Terms) KillAll() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	for _, t := range ts.m {
		if !t.Ended() {
			t.Kill()
		}
	}
}

// hasTrustPrompt says whether the recent output shows the "trust this folder" question (whatever the gaps between
// the words came out as).
func hasTrustPrompt(screen string) bool {
	return strings.Contains(spaces.ReplaceAllString(screen, ""), "trustthisfolder")
}
