//go:build e2e

package e2e

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/feynmanzcys-bytes/symphony/internal/autostart"
	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// TestAutostart turns starting at login on and off for real, under a name of its own, so a real entry is never
// touched: a systemd user service on Linux, a launchd agent on macOS. It uses this machine's systemd or launchd, so it
// runs only with SYMPHONY_E2E=1 (no tokens): once enabled the bridge comes up at its address, and once disabled it's
// gone, entry and all. Windows only has the unit tests in internal/autostart.
func TestAutostart(t *testing.T) {
	if os.Getenv("SYMPHONY_E2E") != "1" {
		t.Skip("uses this machine's systemd or launchd; set SYMPHONY_E2E=1")
	}
	gui := fmt.Sprintf("gui/%d", os.Getuid()) // launchd's domain for the user's login session
	switch runtime.GOOS {
	case "linux":
		if err := exec.Command("systemctl", "--user", "show-environment").Run(); err != nil {
			t.Skipf("no systemd user manager here: %v", err)
		}
	case "darwin":
		if err := exec.Command("launchctl", "print", gui).Run(); err != nil {
			t.Skipf("no login session for launchd agents here (%s): %v", gui, err)
		}
	default:
		t.Skip("the systemd service is Linux's, the launchd agent macOS's")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "symphony")
	if out, err := exec.Command("go", "build", "-o", exe, "../cmd/bridge").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	// the page key, made first in the data folder the bridge will use (as -autostart on does)
	key, err := bridge.LoadKey(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}

	sys := autostart.Default(fmt.Sprintf("symphony-e2e-%d", time.Now().UnixNano()))
	where, err := sys.Enable(autostart.Entry{
		Exe:  exe,
		Args: []string{"-addr=" + addr, "-data=" + filepath.Join(dir, "data"), "-claude=" + filepath.Join(dir, "no-claude")},
		Path: os.Getenv("PATH"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sys.Disable() })
	answers := func() bool {
		req, _ := http.NewRequest("GET", "http://"+addr+"/api/state", nil)
		req.Header.Set("X-Symphony-Key", key)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		res.Body.Close()
		return res.StatusCode == http.StatusOK
	}
	waitUntil(t, 20*time.Second, "systemd or launchd starts the bridge", answers)

	var known func() bool // whether systemd or launchd still knows the entry
	switch runtime.GOOS {
	case "linux":
		unit := sys.Name + ".service"
		if out, _ := exec.Command("systemctl", "--user", "is-enabled", unit).Output(); strings.TrimSpace(string(out)) != "enabled" {
			t.Errorf("%s should be enabled; systemctl says %q", unit, out)
		}
		if out, _ := exec.Command("systemctl", "--user", "show", "-p", "ExecStart", unit).Output(); !strings.Contains(string(out), exe) || !strings.Contains(string(out), "-open=false") {
			t.Errorf("the service should run %s with -open=false: %s", exe, out)
		}
		known = func() bool { return exec.Command("systemctl", "--user", "is-enabled", unit).Run() == nil }
	case "darwin":
		job := gui + "/" + strings.TrimSuffix(filepath.Base(where), ".plist")
		logFile := filepath.Join(sys.Home, "Library", "Logs", sys.Name+".log")
		t.Cleanup(func() { os.Remove(logFile) })
		out, err := exec.Command("launchctl", "print", job).CombinedOutput()
		if err != nil {
			t.Fatalf("launchd doesn't know %s: %v\n%s", job, err, out)
		}
		if !strings.Contains(string(out), "program = "+exe) || !strings.Contains(string(out), "-open=false") {
			t.Errorf("the agent should run %s with -open=false:\n%s", exe, out)
		}
		// the bridge's log goes to ~/Library/Logs, from its first line on, which names the address
		if b, _ := os.ReadFile(logFile); !strings.Contains(string(b), addr) {
			t.Errorf("%s should hold the bridge's log:\n%s", logFile, b)
		}
		known = func() bool { return exec.Command("launchctl", "print", job).Run() == nil }
	}

	if _, err := sys.Disable(); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 10*time.Second, "the bridge stops with the entry", func() bool { return !answers() })
	if _, err := os.Stat(where); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the entry's file is still there: %v", err)
	}
	if known() {
		t.Errorf("%s is gone, but the service manager still knows it", where)
	}
}
