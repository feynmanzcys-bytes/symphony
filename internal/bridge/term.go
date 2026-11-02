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
	"unicode/utf8"
)

// ptyProc is a child process on a pseudo-terminal, however the platform makes one (pty_*.go): its output is read
// from it, its input written to it.
type ptyProc interface {
	io.Reader
	io.Writer
	Resize(cols, rows int) error
	Wait() error  // waits for the process to exit
	Close() error // closes the output once the process is gone, which ends the reader
	Kill() error  // ends the process and what it started, for good
}

// hanger is a process that can be asked to end before it is ended for good (pty_unix.go: a hangup, as closing its
// terminal would). Kill comes killGrace later, or once the process is gone.
type hanger interface {
	Hangup() error
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
	// termMaxSize is the most columns or rows a window may ask for: within a pty's uint16 and a Windows console's int16
	termMaxSize = 1<<15 - 1
	// maxForward is the most spaces one cursor move becomes in plain text, and forwardBudget all of them together: a
	// session's output can ask for any number, as often as it likes
	maxForward    = 1000
	forwardBudget = 64 << 10
	// screenMax is the most a screen's text may hold (Screen): it goes into a failed launch's error, in every snapshot
	screenMax = 8 << 10
)

// killGrace is how long a session asked to end (Kill) has to end what it started before all of it is killed; termDrain
// is how long the output may take to run dry once the process is gone (a pty's goes on while something the session
// started holds it). Variables, so tests can shorten them.
var (
	killGrace = 3 * time.Second
	termDrain = time.Second
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
	waited  chan struct{} // closed once the process is gone
	killing chan struct{} // made by Kill; closed once everything the session ran has been killed
	exited  chan struct{} // closed once the terminal is done with: the process gone, its output read, a kill finished
	exitErr error
	ended   time.Time
	grace   time.Duration // killGrace and termDrain as they were when it started: its goroutines outlive a test
	drain   time.Duration
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
	t := &Term{ID: id, Name: name, proc: proc, clients: map[*termClient]struct{}{}, cols: termCols, rows: termRows,
		waited: make(chan struct{}), exited: make(chan struct{}), grace: killGrace, drain: termDrain}
	go t.pump()
	return t
}

// pump copies the process's output to the history and the clients until it ends. It waits for the process, not for
// the output to end: on Windows the output ends only once Wait has closed the console, and on a pty only once nothing
// the session started holds it any more.
func (t *Term) pump() {
	read := make(chan struct{})
	go func() {
		defer close(read)
		buf := make([]byte, termChunk)
		for {
			n, err := t.proc.Read(buf)
			if n > 0 {
				t.push(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	err := t.proc.Wait()
	close(t.waited)
	t.mu.Lock()
	killing := t.killing
	t.mu.Unlock()
	if killing != nil {
		<-killing // what it started goes too before the session counts as gone (a task's worktrees are removed next)
	}
	select { // the last words, which a failed launch shows
	case <-read:
	case <-time.After(t.drain):
	}
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
	if cols <= 0 || rows <= 0 || cols > termMaxSize || rows > termMaxSize || t.Ended() {
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

// Exited is closed once the process is gone (after Kill, what it started too) and its output read; ExitErr says how it
// ended.
func (t *Term) Exited() <-chan struct{} { return t.exited }
func (t *Term) ExitErr() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.exitErr
}

// Kill ends the session without waiting: a hangup first, as closing its terminal would, so claude can end what it
// started; once it is gone, or after killGrace, the rest is killed (pty_unix.go: also what it started in sessions of
// their own, which no signal to its process group reaches). Exited closes once all of that is done.
func (t *Term) Kill() { t.kill() }

// kill starts ending the session, once, and returns a channel closed once everything is killed; nil when the
// process had already ended.
func (t *Term) kill() <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ended.IsZero() {
		return nil
	}
	if t.killing == nil {
		done := make(chan struct{})
		t.killing = done
		go func() {
			defer close(done)
			if h, ok := t.proc.(hanger); ok && h.Hangup() == nil {
				select {
				case <-t.waited:
				case <-time.After(t.grace):
				}
			}
			_ = t.proc.Kill()
		}()
	}
	return t.killing
}

// Clients is how many windows are attached.
func (t *Term) Clients() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.clients)
}

var (
	ansi   = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]|[\x00-\x08\x0b-\x1f\x7f]`)
	move   = regexp.MustCompile(`^\x1b\[(\d*)([CG])$`) // to the right (CUF) and to a column (CHA): Ink draws the gaps between words with both
	spaces = regexp.MustCompile(`\s+`)
)

// plainText is terminal output as text, as far as a stream of it tells: a move to the right or to a column further
// right becomes spaces, a move back to a column already passed one space (so words stay apart), and the other escape
// sequences and control characters go.
func plainText(raw []byte) string {
	budget := forwardBudget // once spent, a move is one space
	var b strings.Builder
	b.Grow(len(raw))
	col := 0 // the column the text has reached on its line
	pad := func(n int) {
		k := min(max(n, 1), maxForward, max(budget, 1))
		budget -= k
		b.WriteString(strings.Repeat(" ", k))
		col += k
	}
	text := func(p []byte) {
		if i := bytes.LastIndexByte(p, '\n'); i >= 0 {
			b.Write(p[:i+1])
			p, col = p[i+1:], 0
		}
		b.Write(p)
		col += utf8.RuneCount(p)
	}
	at := 0
	for _, m := range ansi.FindAllIndex(raw, -1) {
		text(raw[at:m[0]])
		seq := raw[m[0]:m[1]]
		at = m[1]
		if seq[0] == '\r' {
			col = 0
			continue
		}
		mv := move.FindSubmatch(seq)
		if mv == nil {
			continue
		}
		n, _ := strconv.Atoi(string(mv[1]))
		switch to := max(n, 1) - 1; {
		case mv[2][0] == 'C':
			pad(n)
		case to > col:
			pad(to - col)
		case to < col:
			pad(1)
		}
	}
	text(raw[at:])
	return b.String()
}

// Screen is what the terminal shows: the last n non-empty lines of a screen model (vt) fed with the whole history at the
// terminal's size, so text that was typed and erased, or drawn over, isn't in it. It goes into an exit notice and a
// failed launch's error.
func (t *Term) Screen(n int) string {
	t.mu.Lock()
	hist := append([]byte(nil), t.hist...)
	cols, rows := t.cols, t.rows
	t.mu.Unlock()
	v := newVT(cols, rows)
	v.feed(hist)
	return cutFront(lastLines(strings.Join(v.lines(), "\n"), n), screenMax)
}

// Recent is the recent output laid out as it came, the last n non-empty lines: every redraw one after another, with
// the gaps the cursor moves made (plainText). The trust prompt is read from it: whatever a console draws or leaves
// out, the words and the cursor's last position are in it.
func (t *Term) Recent(n int) string {
	t.mu.Lock()
	hist := append([]byte(nil), t.hist...)
	t.mu.Unlock()
	return cutFront(lastLines(plainText(hist), n), screenMax)
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

// Forget drops a launch's terminal once its process has ended.
func (ts *Terms) Forget(id string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if t := ts.m[id]; t != nil && t.Ended() {
		delete(ts.m, id)
	}
}

// Live says whether a launch's process is still running.
func (ts *Terms) Live(id string) bool {
	t := ts.Get(id)
	return t != nil && !t.Ended()
}

// KillAll ends every running session (the bridge is shutting down): all at once, so their grace periods run side by
// side, and it returns once they are killed, about killGrace at most.
func (ts *Terms) KillAll() {
	ts.mu.Lock()
	all := make([]*Term, 0, len(ts.m))
	for _, t := range ts.m {
		all = append(all, t)
	}
	ts.mu.Unlock()
	var killed []<-chan struct{}
	for _, t := range all {
		if k := t.kill(); k != nil {
			killed = append(killed, k)
		}
	}
	deadline := time.After(killGrace + 2*time.Second) // Kill itself may take a moment (pty_unix.go lists the processes)
	for _, k := range killed {
		select {
		case <-k:
		case <-deadline:
			return
		}
	}
}

// hasTrustPrompt says whether the recent output shows the "trust this folder" question (whatever the gaps between
// the words came out as).
func hasTrustPrompt(screen string) bool {
	return strings.Contains(spaces.ReplaceAllString(screen, ""), "trustthisfolder")
}

const (
	// trustDowns is how many times the trust prompt's answer presses Down while the cursor shows on "No, exit" before
	// it gives up (the launch then times out, its screen in the error)
	trustDowns = 8
	// trustSettle is how long the answer waits after the prompt showed before a Down when the screen doesn't say
	// where the cursor is
	trustSettle = time.Second
)

// trustCursor is the answer of the trust prompt the cursor is on, as the screen drew it last: "yes" or "no" from the
// words after the last ❯, "" when they don't tell.
func trustCursor(screen string) string {
	i := strings.LastIndex(screen, "❯")
	if i < 0 {
		return ""
	}
	switch rest := strings.TrimSpace(screen[i+len("❯"):]); {
	case strings.HasPrefix(rest, "Yes"):
		return "yes"
	case strings.HasPrefix(rest, "No"):
		return "no"
	}
	return ""
}
