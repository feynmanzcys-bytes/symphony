//go:build e2e

package e2e

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// TestModStartsBridge starts a real session with a token while no bridge runs: the mod starts the bridge with the
// symphony program on PATH (built here; SYMPHONY_DATA points it at the accounts made below), the session takes its
// account's seat there, and the bridge keeps running after the session ends. A session start without a prompt, but a
// real one, so it needs SYMPHONY_E2E=1 like TestOffice.
func TestModStartsBridge(t *testing.T) {
	if os.Getenv("SYMPHONY_E2E") != "1" {
		t.Skip("starts a real Claude session; set SYMPHONY_E2E=1 to run")
	}
	if os.Getenv("SYMPHONY_E2E_MOD") == "installed" {
		t.Skip("tests this repository's mod")
	}
	for _, bin := range []string{"claude", "tmux", "pkill"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not found", bin)
		}
	}
	repo, _ := filepath.Abs("..")
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	exe := filepath.Join(bin, "symphony")
	if out, err := exec.Command("go", "build", "-o", exe, "../cmd/bridge").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	// the bridge the mod starts, whatever happens to the test
	t.Cleanup(func() { exec.Command("pkill", "-f", "--", exe).Run() })

	// the account exists before any bridge runs, as after a restart of the computer
	data := filepath.Join(dir, "data")
	hub := bridge.NewHub()
	if err := hub.Open(data); err != nil {
		t.Fatal(err)
	}
	name := "solo-" + time.Now().Format("150405")
	acc, err := hub.CreateAccount(name)
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	model := os.Getenv("SYMPHONY_E2E_MODEL")
	if model == "" {
		model = "sonnet"
	}
	env := map[string]string{
		"SYMPHONY_TOKEN": acc.Token, "SYMPHONY_BRIDGE": "http://" + addr, "SYMPHONY_LANG": "en", "SYMPHONY_DATA": data,
		"PATH": bin + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	pane := startClaude(t, "sym-e2e-"+name, workDir(t, "solo"), env,
		"--model", model, "--permission-mode", "manual", "--plugin-dir", filepath.Join(repo, "mods", "symphony-bridge"))

	online := func() bool {
		res, err := http.Get("http://" + addr + "/api/state")
		if err != nil {
			return false
		}
		defer res.Body.Close()
		var snap struct {
			Accounts []struct{ Name, Session string }
		}
		if json.NewDecoder(res.Body).Decode(&snap) != nil {
			return false
		}
		for _, a := range snap.Accounts {
			if a.Name == name && a.Session != "" {
				return true
			}
		}
		return false
	}
	waitUntil(t, 60*time.Second, "the mod starts the bridge and the session takes its seat", online, pane.dump)
	if log, err := os.ReadFile(filepath.Join(data, "bridge.log")); err != nil || !strings.Contains(string(log), "http://"+addr) {
		t.Errorf("the bridge the mod started should log to bridge.log in SYMPHONY_DATA: %v\n%s", err, log)
	}

	pane.kill()
	time.Sleep(2 * time.Second)
	if res, err := http.Get("http://" + addr + "/api/state"); err != nil {
		t.Fatalf("the bridge should keep running after the session that started it ends: %v", err)
	} else {
		res.Body.Close()
	}
}
