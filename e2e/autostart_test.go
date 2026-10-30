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
)

// TestAutostart turns starting at login on and off for real on Linux, as a systemd user service with a name of its
// own, so a real symphony.service is never touched. It uses this machine's systemd, so it runs only with
// SYMPHONY_E2E=1 (no tokens): once enabled the bridge comes up at its address, and once disabled it's gone, unit file
// and all. macOS and Windows only have the unit tests in internal/autostart.
func TestAutostart(t *testing.T) {
	if os.Getenv("SYMPHONY_E2E") != "1" {
		t.Skip("uses this machine's systemd; set SYMPHONY_E2E=1")
	}
	if runtime.GOOS != "linux" {
		t.Skip("the systemd service is Linux's")
	}
	if err := exec.Command("systemctl", "--user", "show-environment").Run(); err != nil {
		t.Skipf("no systemd user manager here: %v", err)
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

	sys := autostart.Default(fmt.Sprintf("symphony-e2e-%d", time.Now().UnixNano()))
	unit := sys.Name + ".service"
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
		res, err := http.Get("http://" + addr + "/api/state")
		if err != nil {
			return false
		}
		res.Body.Close()
		return res.StatusCode == http.StatusOK
	}
	waitUntil(t, 20*time.Second, "systemd starts the bridge", answers)
	if out, _ := exec.Command("systemctl", "--user", "is-enabled", unit).Output(); strings.TrimSpace(string(out)) != "enabled" {
		t.Errorf("%s should be enabled; systemctl says %q", unit, out)
	}
	if out, _ := exec.Command("systemctl", "--user", "show", "-p", "ExecStart", unit).Output(); !strings.Contains(string(out), exe) || !strings.Contains(string(out), "-open=false") {
		t.Errorf("the service should run %s with -open=false: %s", exe, out)
	}

	if _, err := sys.Disable(); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 10*time.Second, "the bridge stops with the service", func() bool { return !answers() })
	if _, err := os.Stat(where); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the unit file is still there: %v", err)
	}
	if err := exec.Command("systemctl", "--user", "is-enabled", unit).Run(); err == nil {
		t.Errorf("systemd still knows %s", unit)
	}
}
