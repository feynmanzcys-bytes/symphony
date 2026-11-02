//go:build darwin || linux

package bridge

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestTermRoundTrip runs a shell on a pseudo-terminal hosted by the bridge: its output reaches the history and an
// attached client, keystrokes reach it, and its end is noticed.
func TestTermRoundTrip(t *testing.T) {
	term, err := newTerm("t1", "alice", t.TempDir(), childEnv(map[string]string{"SYMPHONY_TOKEN": "x"}),
		[]string{"/bin/sh", "-c", `echo "hello $SYMPHONY_TOKEN"; read line; echo "got:$line"; stty size`})
	if err != nil {
		t.Fatal(err)
	}
	history, c, detach := term.Attach(100, 30)
	defer detach()
	collect := func(want string) string {
		got := string(history)
		deadline := time.After(5 * time.Second)
		for !strings.Contains(got, want) {
			select {
			case chunk := <-c.out:
				got += string(chunk)
			case <-deadline:
				t.Fatalf("waiting for %q, got %q", want, got)
			case <-c.gone:
				for len(c.out) > 0 {
					got += string(<-c.out)
				}
				if !strings.Contains(got, want) {
					t.Fatalf("the process ended before %q, got %q", want, got)
				}
			}
		}
		return got
	}
	collect("hello x")
	if err := term.Write([]byte("abc\r")); err != nil {
		t.Fatal(err)
	}
	collect("got:abc")
	out := collect("30 100") // the size the client attached with
	select {
	case <-term.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("the process should have exited")
	}
	if !term.Ended() || term.ExitErr() == nil || term.ExitErr().Error() != "exit 0" {
		t.Fatalf("ended=%v err=%v", term.Ended(), term.ExitErr())
	}
	if screen := term.Screen(3); !strings.Contains(screen, "got:abc") || strings.Contains(screen, "\x1b") {
		t.Fatalf("Screen should give plain text: %q (out %q)", screen, out)
	}
	if err := term.Write([]byte("x")); err != ErrTermEnded {
		t.Fatalf("writing to an ended terminal: %v", err)
	}
	// a client attaching now gets the history and is told it's over
	hist2, c2, detach2 := term.Attach(0, 0)
	defer detach2()
	if !strings.Contains(string(hist2), "got:abc") {
		t.Fatalf("history after the end: %q", hist2)
	}
	select {
	case <-c2.gone:
	default:
		t.Fatal("a client of an ended terminal should be told so at once")
	}
}

func TestTermTrustPromptAndFailure(t *testing.T) {
	hub := NewHub()
	pt := NewPTYTerminal()
	l := &Launch{ID: "p1", Name: "bob", Status: LaunchStarting}
	hub.addLaunch(l)
	// a fake claude that asks "trust this folder" with the cursor on No, moves it to Yes on Down, says which keys it got
	// once Enter comes, and exits
	fake := `echo "Do you trust this folder?"; echo "❯ No, exit"; echo "  Yes, I trust this folder"; stty raw -echo
k=$(dd bs=3 count=1 2>/dev/null); printf '\r\n  No, exit\r\n❯ Yes, I trust this folder\r\n'
e=$(dd bs=1 count=1 2>/dev/null); stty sane
[ "$k" = "$(printf '\033[B')" ] && [ "$e" = "$(printf '\r')" ] && echo "keys:down,enter"; exit 3`
	if err := pt.Start(l, t.TempDir(), nil, []string{"/bin/sh", "-c", fake}); err != nil {
		t.Fatal(err)
	}
	pt.Watch(hub, l, 10*time.Second)
	if st := hub.launchStatus(l.ID); st != LaunchFailed || l.ErrorCode != "launch_process_exited" {
		t.Fatalf("a process that exits before the session is online should fail the launch: %s %s %v", st, l.ErrorCode, l.ErrorParams)
	}
	if !strings.Contains(l.ErrorParams["screen"], "keys:") || !strings.Contains(l.ErrorParams["detail"], "exit status 3") {
		t.Fatalf("the failure should carry the screen and the exit status: %v", l.ErrorParams)
	}
	// Down, then Enter once the cursor was on Yes, reached the process
	if !strings.Contains(pt.Terms.Get(l.ID).Screen(5), "keys:down,enter") {
		t.Fatalf("screen: %q", pt.Terms.Get(l.ID).Screen(5))
	}
	if pt.Terms.Live(l.ID) {
		t.Fatal("the terminal should be marked ended")
	}
	pt.Terms.Prune() // ended just now: still kept
	if pt.Terms.Get(l.ID) == nil {
		t.Fatal("a terminal that ended a moment ago should still be readable")
	}
}

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

// TestTermResizeBounds: a window may ask for any size; one beyond what a pty or a console holds is ignored, as 0 is.
func TestTermResizeBounds(t *testing.T) {
	p := newFakeProc()
	tm := hostTerm("x", "x", p)
	defer p.exit(nil)
	for _, size := range [][2]int{{70000, 24}, {24, 70000}, {termMaxSize + 1, 30}, {0, 30}, {-1, 30}} {
		if err := tm.Resize(size[0], size[1]); err != nil {
			t.Fatal(err)
		}
		p.mu.Lock()
		got := p.size
		p.mu.Unlock()
		if got != [2]int{} {
			t.Fatalf("Resize(%d, %d) reached the pty: %v", size[0], size[1], got)
		}
	}
	if err := tm.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.size != [2]int{100, 30} {
		t.Fatalf("a good size should reach the pty: %v", p.size)
	}
}

func TestAttachScript(t *testing.T) {
	s := attachScript("it's bob", []string{"/opt/sym phony", "attach", "-addr", "127.0.0.1:7788", "abc"})
	for _, want := range []string{"#!/bin/sh\n", "rm -f -- \"$0\"\n", `'it'\''s bob'`, `exec '/opt/sym phony' attach -addr 127.0.0.1:7788 abc` + "\n"} {
		if !strings.Contains(s, want) {
			t.Fatalf("script lacks %q:\n%s", want, s)
		}
	}
}

// shorten sets a duration for one test.
func shorten(t *testing.T, d *time.Duration, to time.Duration) {
	old := *d
	*d = to
	t.Cleanup(func() { *d = old })
}

// processGone says whether pid has ended: no such process, or one that only waits to be reaped.
func processGone(pid int) bool {
	if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
		return true
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat") // Linux
	if err != nil {
		return false
	}
	f := strings.Fields(string(stat[bytes.LastIndexByte(stat, ')')+1:]))
	return len(f) > 0 && f[0] == "Z"
}

// TestSetsidHelper isn't a test but a session's tool, for TestTermKillEndsToolsOfTheirOwn: it runs in a session of its
// own, as Claude Code's tools do, ignores the hangup and waits to be killed.
func TestSetsidHelper(t *testing.T) {
	pidFile := os.Getenv("SYMPHONY_TEST_TOOL")
	if pidFile == "" {
		t.Skip("run by TestTermKillEndsToolsOfTheirOwn")
	}
	signal.Ignore(syscall.SIGHUP)
	if _, err := syscall.Setsid(); err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		os.Exit(2)
	}
	time.Sleep(time.Hour)
}

// TestTermKillEndsToolsOfTheirOwn: what a session started in a session of its own (Claude Code's tools), which no signal
// to the session's process group reaches, is killed too, even when it ignores the hangup.
func TestTermKillEndsToolsOfTheirOwn(t *testing.T) {
	shorten(t, &killGrace, 500*time.Millisecond)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(t.TempDir(), "pid")
	term, err := newTerm("k1", "kim", t.TempDir(), append(childEnv(nil), "SYMPHONY_TEST_TOOL="+pidFile),
		[]string{"/bin/sh", "-c", shellQuote(exe) + ` -test.run='^TestSetsidHelper$' </dev/null >/dev/null 2>&1 & exec sleep 1000`})
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	waitFor(t, "the tool to start", func() bool {
		data, err := os.ReadFile(pidFile)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		return err == nil && pid > 0
	})
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	term.Kill()
	select {
	case <-term.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("the session should be gone")
	}
	// gone by the time the session counts as gone: a task's worktrees are removed right after
	if !processGone(pid) {
		time.Sleep(200 * time.Millisecond) // the kill is sent; give it a moment to land
		if !processGone(pid) {
			t.Fatal("the tool in a session of its own should be killed with the session")
		}
	}
}

// TestTermEndsWhilePTYHeld: a session that exits while something it started in a session of its own still holds the
// pty (so its output never ends by itself) counts as ended all the same, a moment later: closing the pty wakes the read.
func TestTermEndsWhilePTYHeld(t *testing.T) {
	shorten(t, &termDrain, 200*time.Millisecond)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(t.TempDir(), "pid")
	term, err := newTerm("k3", "kim", t.TempDir(), append(childEnv(nil), "SYMPHONY_TEST_TOOL="+pidFile),
		[]string{"/bin/sh", "-c", shellQuote(exe) + ` -test.run='^TestSetsidHelper$' & while [ ! -s ` + shellQuote(pidFile) + ` ]; do sleep 0.05; done; echo bye; exit 0`})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if data, err := os.ReadFile(pidFile); err == nil {
			if pid, _ := strconv.Atoi(strings.TrimSpace(string(data))); pid > 0 {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	select {
	case <-term.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("the session exited, but its terminal hasn't ended")
	}
	if err := term.ExitErr(); err == nil || err.Error() != "exit 0" || !strings.Contains(term.Screen(3), "bye") {
		t.Fatalf("exit %v, screen %q", err, term.Screen(3))
	}
}

// TestTermKillLetsItEnd: Kill hangs up first and gives the session time to end by itself (claude ends its tools then),
// and it isn't killed as long as it does so in time.
func TestTermKillLetsItEnd(t *testing.T) {
	shorten(t, &killGrace, 5*time.Second)
	term, err := newTerm("k2", "kim", t.TempDir(), childEnv(nil),
		[]string{"/bin/sh", "-c", `trap 'echo cleaning up; sleep 0.2; exit 7' HUP; echo ready; sleep 1000 & wait`})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the session to be ready", func() bool { return strings.Contains(term.Screen(5), "ready") })
	start := time.Now()
	term.Kill()
	select {
	case <-term.Exited():
	case <-time.After(10 * time.Second):
		t.Fatal("the session should be gone")
	}
	if err := term.ExitErr(); err == nil || err.Error() != "exit status 7" {
		t.Fatalf("the session should have ended by itself on the hangup, got %v", err)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("a session that ends on the hangup shouldn't wait out the grace period (%s)", took)
	}
	if !strings.Contains(term.Screen(5), "cleaning up") {
		t.Fatalf("screen: %q", term.Screen(5))
	}
}

// TestKillAllAtOnce: KillAll ends every session, the ones that ignore the hangup too, waiting out the grace period once
// for all of them, not once each.
func TestKillAllAtOnce(t *testing.T) {
	shorten(t, &killGrace, time.Second)
	ts := &Terms{}
	var terms []*Term
	for _, id := range []string{"a", "b", "c"} {
		term, err := newTerm(id, id, t.TempDir(), childEnv(nil), []string{"/bin/sh", "-c", `trap "" HUP; echo ready; exec sleep 1000`})
		if err != nil {
			t.Fatal(err)
		}
		ts.put(term)
		terms = append(terms, term)
	}
	for _, term := range terms {
		waitFor(t, "the session to be ready", func() bool { return strings.Contains(term.Screen(5), "ready") })
	}
	start := time.Now()
	ts.KillAll()
	if took := time.Since(start); took < killGrace || took > 2*killGrace {
		t.Fatalf("KillAll took %s; want the grace period (%s) once", took, killGrace)
	}
	for _, term := range terms {
		select {
		case <-term.Exited():
		case <-time.After(5 * time.Second):
			t.Fatalf("%s should be gone", term.ID)
		}
		if err := term.ExitErr(); err == nil || err.Error() != "signal: killed" {
			t.Fatalf("%s: %v", term.ID, err)
		}
	}
}

// heldProc is a process whose output stays open after it has exited: ConPTY's until Wait closes the console, a pty's
// while something the session started still holds it.
type heldProc struct {
	r      *io.PipeReader
	w      *io.PipeWriter
	exited chan struct{}
	once   sync.Once
}

func (p *heldProc) Read(b []byte) (int, error)  { return p.r.Read(b) }
func (p *heldProc) Write(b []byte) (int, error) { return len(b), nil }
func (p *heldProc) Resize(cols, rows int) error { return nil }
func (p *heldProc) Wait() error                 { <-p.exited; return nil }
func (p *heldProc) Close() error                { return p.r.Close() }
func (p *heldProc) Kill() error                 { p.once.Do(func() { close(p.exited) }); return nil }

// TestTermEndsWhileOutputHeld: a terminal ends when its process does, with its last words read, even when the output
// doesn't end by itself.
func TestTermEndsWhileOutputHeld(t *testing.T) {
	shorten(t, &termDrain, 200*time.Millisecond)
	r, w := io.Pipe()
	p := &heldProc{r: r, w: w, exited: make(chan struct{})}
	term := hostTerm("h1", "hal", p)
	if _, err := w.Write([]byte("last words\r\n")); err != nil {
		t.Fatal(err)
	}
	_ = p.Kill() // the process exits; its output stays open
	select {
	case <-term.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("the terminal should end with its process")
	}
	if !term.Ended() || !strings.Contains(term.Screen(3), "last words") {
		t.Fatalf("ended=%v screen=%q", term.Ended(), term.Screen(3))
	}
}
