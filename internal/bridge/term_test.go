//go:build darwin || linux

package bridge

import (
	"strings"
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
	// a fake claude that asks "trust this folder", then echoes the keys it gets and exits
	if err := pt.Start(l, t.TempDir(), nil, []string{"/bin/sh", "-c", `echo "Do you trust this folder?"; echo " > No, exit"; read -r k; echo "keys:$k"; exit 3`}); err != nil {
		t.Fatal(err)
	}
	pt.Watch(hub, l, 10*time.Second)
	if st := hub.launchStatus(l.ID); st != LaunchFailed || l.ErrorCode != "launch_process_exited" {
		t.Fatalf("a process that exits before the session is online should fail the launch: %s %s %v", st, l.ErrorCode, l.ErrorParams)
	}
	if !strings.Contains(l.ErrorParams["screen"], "keys:") || !strings.Contains(l.ErrorParams["detail"], "exit status 3") {
		t.Fatalf("the failure should carry the screen and the exit status: %v", l.ErrorParams)
	}
	// the Down and Enter keys reached the process (Down is an escape sequence; read -r keeps it)
	if !strings.Contains(pt.Terms.Get(l.ID).Screen(5), "keys:") {
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
}

func TestAttachScript(t *testing.T) {
	s := attachScript("it's bob", []string{"/opt/sym phony", "attach", "-addr", "127.0.0.1:7788", "abc"})
	for _, want := range []string{"#!/bin/sh\n", "rm -f -- \"$0\"\n", `'it'\''s bob'`, `exec '/opt/sym phony' attach -addr 127.0.0.1:7788 abc` + "\n"} {
		if !strings.Contains(s, want) {
			t.Fatalf("script lacks %q:\n%s", want, s)
		}
	}
}
