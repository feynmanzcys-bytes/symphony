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

// TestInstall 走一遍正式安装 mod 的路：在临时的 Claude Code 配置目录（CLAUDE_CONFIG_DIR）里，
// 照着办公室页面给的命令，用本仓库当插件市场装 mod，看 bridge 和页面认不认。
// 不起会话、不花 token，也不碰本机真正的 Claude Code 配置。
func TestInstall(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("缺 claude")
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
	claude("plugin", "validate", repo) // 插件市场
	claude("plugin", "validate", filepath.Join(repo, "mods", "symphony-bridge"))

	// 没装：bridge 说清楚怎么装，页面上也摆出来
	b := startBridge(t, "")
	b.Launcher.Market = repo
	ctx := context.Background()
	m := b.Launcher.Mod(ctx)
	if m.OK || m.Fix != "claude plugin install symphony-bridge --marketplace "+repo {
		t.Fatalf("没装 mod 时应给出用本地仓库装的命令: %+v", m)
	}
	office := tab(t, newBrowser(t), b.URL+"/")
	var note string
	waitUntil(t, 20*time.Second, "办公室提示怎么装 mod", func() bool {
		note = get(t, office, 5*time.Second, chromedp.Evaluate[string](
			`(n => n && !n.hidden ? n.textContent : '')(document.getElementById('launch-note'))`))
		return strings.Contains(note, m.Fix)
	}, func() string { return note })
	if get(t, office, 5*time.Second, chromedp.Evaluate[bool](`document.getElementById('new-emp').hidden`)) != true {
		t.Fatal("没装 mod 时不该有“+ 新员工”")
	}
	screenshot(t, office, "install-missing.png")

	// 照着提示装上：bridge 不用重启，刷新页面就能招人
	claude("plugin", "install", "symphony-bridge", "--marketplace", repo)
	if m = b.Launcher.Mod(ctx); !m.OK || m.ID != "symphony-bridge@symphony" || m.Version != manifest.Version {
		t.Fatalf("装好后 bridge 应认出 symphony-bridge@symphony %s: %+v", manifest.Version, m)
	}
	do(t, office, 10*time.Second, chromedp.Reload())
	waitUntil(t, 20*time.Second, "刷新后能招人，提示消失", func() bool {
		return get(t, office, 5*time.Second, chromedp.Evaluate[bool](
			`!document.getElementById('new-emp').hidden && document.getElementById('launch-note').hidden`))
	})
	screenshot(t, office, "install-ok.png")

	// 停用了也要说清楚（换个新的 Launcher，不用上面记住的结果）
	claude("plugin", "disable", "symphony-bridge@symphony")
	if m = bridge.NewLauncher("claude", "", b.URL, nil).Mod(ctx); m.OK || m.Fix != "claude plugin enable symphony-bridge@symphony" {
		t.Fatalf("停用后应提示启用: %+v", m)
	}
}
