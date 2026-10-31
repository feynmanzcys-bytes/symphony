//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// TestInstall walks the proper install of the mod: in a throwaway Claude Code config folder (CLAUDE_CONFIG_DIR)
// it installs the mod with this repository as the plugin marketplace, as the office page tells you to, and
// checks that the bridge and the page notice. No sessions, no tokens, and the machine's real Claude Code
// config stays untouched.
func TestInstall(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude not found")
	}
	repo, _ := filepath.Abs("..")
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	claude := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("claude", args...).CombinedOutput(); err != nil {
			t.Fatalf("claude %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	var manifest struct{ Version string }
	data, err := os.ReadFile(filepath.Join(repo, "mods", "symphony-bridge", ".claude-plugin", "plugin.json"))
	if err == nil {
		err = json.Unmarshal(data, &manifest)
	}
	if err != nil {
		t.Fatal(err)
	}
	claude("plugin", "validate", repo) // the marketplace
	claude("plugin", "validate", filepath.Join(repo, "mods", "symphony-bridge"))

	// not installed: the bridge says how to install it, and so does the page
	b := startBridge(t, "")
	b.Launcher.Market = repo
	ctx := context.Background()
	m := b.Launcher.Mod(ctx)
	if m.OK || m.Fix != "claude plugin install symphony-bridge --marketplace "+repo {
		t.Fatalf("without the mod the bridge should offer the install command for the local repository: %+v", m)
	}
	office := tab(t, newBrowser(t), b.URL+"/")
	var note string
	waitUntil(t, 20*time.Second, "the office tells how to install the mod", func() bool {
		note = get(t, office, 5*time.Second, chromedp.Evaluate[string](
			`(n => n && !n.hidden ? n.textContent : '')(document.getElementById('launch-note'))`))
		return strings.Contains(note, m.Fix)
	}, func() string { return note })
	if get(t, office, 5*time.Second, chromedp.Evaluate[bool](`document.getElementById('new-emp').hidden`)) != true {
		t.Fatal(`without the mod there should be no "+ New employee"`)
	}
	screenshot(t, office, "install-missing.png")

	// install as told: no bridge restart, and after a reload the page can hire
	claude("plugin", "install", "symphony-bridge", "--marketplace", repo)
	// a fresh Launcher asks claude again (the bridge's own keeps a failed answer for 10 seconds; the page's reload
	// below waits that out)
	fresh := bridge.NewLauncher("claude", "", b.URL, nil)
	fresh.Market = repo
	if m = fresh.Mod(ctx); !m.OK || m.ID != "symphony-bridge@symphony" || m.Version != manifest.Version {
		t.Fatalf("once installed the bridge should see symphony-bridge@symphony %s: %+v", manifest.Version, m)
	}
	time.Sleep(11 * time.Second) // the bridge keeps its failed answer for 10 seconds; the page asks once per load
	do(t, office, 10*time.Second, chromedp.Reload())
	waitUntil(t, 20*time.Second, "after a reload the page can hire and the hint is gone", func() bool {
		return get(t, office, 5*time.Second, chromedp.Evaluate[bool](
			`!document.getElementById('new-emp').hidden && document.getElementById('launch-note').hidden`))
	})
	screenshot(t, office, "install-ok.png")

	// a disabled mod gets a clear hint too (a fresh Launcher, so the remembered answer above doesn't count)
	claude("plugin", "disable", "symphony-bridge@symphony")
	if m = bridge.NewLauncher("claude", "", b.URL, nil).Mod(ctx); m.OK || m.Fix != "claude plugin enable symphony-bridge@symphony" {
		t.Fatalf("a disabled mod should get the enable command: %+v", m)
	}
}
