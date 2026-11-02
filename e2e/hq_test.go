//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// TestHQ walks through the HQ theme with made-up activity (no tokens: claude is a script that sits there, the sessions
// are reported in as the mod would): a team in every state with usage figures, a pending ask, a task with two members;
// then every tab in en (screenshots into e2e/artifacts/screens/hq-*), an ask answered from the approvals queue, a row
// of the roster opening the employee window, the board opening the task window, the search box, dark mode, and the
// overview and the roster again in zh-CN. It fails on a page error, a key a catalog lacks, or a raw key on screen.
func TestHQ(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	requireBrowser(t)
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))
	t.Cleanup(func() {
		for _, k := range b.Hub.Tasks() {
			_ = b.Hub.EndTask(k.ID)
		}
		b.Hub.Stop()
	})
	b.Launcher.Claude = sleeper(t) // claude sits there; the test reports its sessions in
	work := t.TempDir()
	dir := func(name string) string { return filepath.Join(work, name) }
	for _, d := range []string{"app", "web", "docs"} {
		if err := os.MkdirAll(dir(d), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir(d), "notes.txt"), []byte(d+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir("app"), "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// ---------- the team ----------
	tokens := map[string]string{}
	event := func(name, session string, body map[string]any) {
		t.Helper()
		body["session"] = session
		b.post(t, tokens[name], "/api/event", body)
	}
	team := []struct{ name, dir, model, effort string }{{"alice", "app", "opus", "max"}, {"bob", "web", "sonnet", "high"}, {"carol", "docs", "haiku", "low"}, {"dave", "", "", ""}}
	for _, m := range team {
		acc, err := b.Hub.CreateAccount(m.name)
		if err != nil {
			t.Fatal(err)
		}
		tokens[m.name] = acc.Token
		if m.dir == "" {
			continue
		}
		event(m.name, "s-"+m.name, map[string]any{"kind": "session.start", "cwd": dir(m.dir), "model": "claude-" + m.model + "-5-5", "effort": m.effort,
			"engine": map[string]any{"version": "2.1.316"}, "mod": "0.10.0", "surface": "terminal"})
		event(m.name, "s-"+m.name, map[string]any{"kind": "session.title", "title": m.name + "-desk"})
	}
	// alice works, with figures: tokens by model, the context window a third full, a cost; bob idles; carol asks
	event("alice", "s-alice", map[string]any{"kind": "chat.user", "text": "Run the test suite and fix what fails", "origin": "composer"})
	event("alice", "s-alice", map[string]any{"kind": "turn.start", "text": "Run the test suite and fix what fails"})
	event("alice", "s-alice", map[string]any{"kind": "chat.stop", "turnId": "a1", "step": 0, "tokens": map[string]any{"input": 12000, "output": 800, "cacheRead": 90000, "cacheWrite": 0, "model": "claude-opus-5-5"}})
	event("alice", "s-alice", map[string]any{"kind": "tool.start", "tool": "Bash", "summary": "make test", "toolUseId": "tu1"})
	event("alice", "s-alice", map[string]any{"kind": "session.usage", "usage": map[string]any{"context": map[string]any{"tokens": 68000, "window": 200000, "percent": 34}, "rateLimits": []map[string]any{{"kind": "five_hour", "percentUsed": 12}}, "cost": map[string]any{"usd": 1.25}}})
	event("bob", "s-bob", map[string]any{"kind": "session.usage", "usage": map[string]any{"context": map[string]any{"tokens": 150000, "window": 200000, "percent": 75}, "cost": map[string]any{"usd": 0.4}}})
	event("bob", "s-bob", map[string]any{"kind": "chat.stop", "turnId": "b1", "step": 0, "tokens": map[string]any{"input": 3000, "output": 400, "cacheRead": 0, "cacheWrite": 0, "model": "claude-sonnet-5-5"}})
	b.post(t, tokens["carol"], "/api/ask", map[string]any{"session": "s-carol", "kind": "permission", "tool": "Bash", "input": map[string]any{"command": "rm -rf build && npm run build"}})
	waitUntil(t, 10*time.Second, "the ask is pending", func() bool { return len(b.Hub.Snapshot().Asks) == 1 })
	askID := b.Hub.Snapshot().Asks[0].ID

	browser := newBrowser(t)
	en, _ := clockTab(t, browser, b.URL+"/t/hq/?lang=en", 0)
	do(t, en, 10*time.Second, chromedp.EmulateViewport(1400, 860))
	p := &screens{t: t, ctx: en, lang: "hq-en"}
	waitUntil(t, 20*time.Second, "HQ shows the KPI strip with the headcount", func() bool {
		return p.is(`document.querySelectorAll('#kpis .kpi').length === 7 && document.querySelector('#kpis .kpi .val').textContent === '4'`)
	})
	// a task with two members, from the page's own fetch, in the HQ's dialog's shape (no room)
	var task bridge.Task
	accOf := func(name string) *bridge.AccountView { return b.account(name) }
	body, _ := json.Marshal(map[string]any{"title": "Write the CHANGELOG for 0.10", "leader": true, "permissionMode": "bypassPermissions", "lang": "en",
		"members": []map[string]any{{"account": accOf("alice").ID, "model": "opus", "effort": "max"}, {"account": accOf("bob").ID, "model": "sonnet", "effort": "high"}}})
	if err := json.Unmarshal([]byte(p.fetch("POST", "/api/tasks", string(body))), &task); err != nil || task.ID == "" {
		t.Fatalf("creating a task failed: %v", err)
	}
	for _, m := range task.Members {
		name := strings.TrimSuffix(m.Name, "-"+task.ID)
		event(name, name+"-task", map[string]any{"kind": "session.start", "cwd": m.Worktree, "launch": m.Launch, "task": task.ID, "model": "claude-opus-5-5"})
	}
	waitUntil(t, 10*time.Second, "the members are in", func() bool {
		for _, k := range b.Hub.Tasks() {
			if k.ID == task.ID && k.Members[0].Session != "" && k.Members[1].Session != "" {
				return true
			}
		}
		return false
	})

	// ---------- overview ----------
	waitUntil(t, 10*time.Second, "the overview lists carol's ask and the figures", func() bool {
		return p.is(`document.querySelector('#ov-waiting .row') !== null && document.querySelectorAll('#ov-cost .bar').length === 2 && document.querySelectorAll('#ov-tokens .bar').length === 2 && document.querySelector('#ov-spark svg') !== null`)
	})
	if got := p.js(`[...document.querySelectorAll('#kpis .kpi .val')].map(e => e.textContent).join('|')`); !strings.HasPrefix(got, "4|") || !strings.Contains(got, "$1.65") {
		t.Fatalf("the KPI strip: %s", got)
	}
	p.shot("overview")
	p.check()
	// a hover on the sparkline shows the minute's count
	p.run(`document.querySelector('#ov-spark .hit:last-of-type').dispatchEvent(new MouseEvent('mousemove', { bubbles:true }))`)
	if !p.is(`!document.querySelector('#ov-spark .tip').hidden`) {
		t.Fatal("the sparkline should show a tooltip on hover")
	}
	// the waiting row opens carol's window with the card over the input
	p.run(`document.querySelector('#ov-waiting .row').click()`)
	waitUntil(t, 10*time.Second, "carol's window opens with her ask", func() bool {
		return p.is(fmt.Sprintf(`!document.getElementById('chat').hidden && !!document.querySelector('#chat #asks .card[data-id="%s"]')`, askID))
	})
	p.shot("chat-ask")
	p.check()
	p.run(`document.querySelector('#chat .close').click()`)

	// ---------- people ----------
	p.run(`document.querySelector('#tabs [data-tab=people]').click()`)
	waitUntil(t, 10*time.Second, "the roster lists the four", func() bool { return p.is(`document.querySelectorAll('#people tbody tr').length === 4`) })
	if got := p.js(`[...document.querySelectorAll('#people tbody tr td.name b')].map(e => e.textContent).join(',')`); got != "carol,alice,bob,dave" {
		t.Fatalf("the roster should sort waiting, working, idle, off duty: %s", got)
	}
	if !p.is(`document.querySelector('#people tbody tr:nth-child(2) .ctx') !== null && document.querySelector('#people tbody tr:nth-child(2)').textContent.includes('34%') && document.querySelector('#people tbody tr:nth-child(2)').textContent.includes('$1.25')`) {
		t.Fatalf("alice's row should show her context and cost: %s", p.js(`document.querySelector('#people tbody tr:nth-child(2)').textContent`))
	}
	if !p.is(`[...document.querySelectorAll('#people tbody tr')].some(tr => tr.textContent.includes('T1'))`) {
		t.Fatal("the roster should show the task alice is in")
	}
	p.shot("people")
	p.check()
	p.run(`document.querySelector('#people th[data-col=name]').click()`)
	if got := p.js(`[...document.querySelectorAll('#people tbody tr td.name b')].map(e => e.textContent).join(',')`); got != "alice,bob,carol,dave" {
		t.Fatalf("sorted by name: %s", got)
	}
	p.run(`document.querySelector('#chips .chip[data-group=off]').click()`)
	if !p.is(`document.querySelectorAll('#people tbody tr').length === 1`) {
		t.Fatal("the off-duty chip should leave dave alone")
	}
	p.run(`document.querySelector('#chips .chip[data-group=off]').click()`)
	// a row opens the window; dave's offers Start work
	p.run(`[...document.querySelectorAll('#people tbody tr')].find(tr => tr.textContent.includes('dave')).click()`)
	waitUntil(t, 10*time.Second, "dave's window opens with Start work", func() bool {
		return p.is(`!document.getElementById('chat').hidden && !!document.querySelector('#chat .btns .go-work')`)
	})
	p.shot("chat-off-duty")
	p.run(`document.querySelector('#chat .close').click()`)

	// ---------- approvals: the queue, answered there ----------
	p.run(`document.querySelector('#tabs [data-tab=approvals]').click()`)
	waitUntil(t, 10*time.Second, "the queue shows carol's card", func() bool {
		return p.is(fmt.Sprintf(`!!document.querySelector('.approvals #queue .card[data-id="%s"] .allow')`, askID))
	})
	p.shot("approvals")
	p.check()
	p.run(fmt.Sprintf(`document.querySelector('.approvals #queue .card[data-id="%s"] .allow').click()`, askID))
	waitUntil(t, 10*time.Second, "the ask is settled and the queue empty", func() bool {
		return !b.pending(askID) && p.is(`document.querySelectorAll('.approvals #queue .card').length === 0 && !!document.querySelector('.approvals #queue .empty')`)
	})

	// ---------- projects: the board, the task window ----------
	p.run(`document.querySelector('#tabs [data-tab=projects]').click()`)
	waitUntil(t, 10*time.Second, "the board shows the task running with its two members", func() bool {
		return p.is(`document.querySelectorAll('.board .task').length === 1 && document.querySelectorAll('.board .task .mem').length === 2`)
	})
	p.shot("projects")
	p.check()
	// a second task from the "New task" button: no room, carol and an advisor
	p.run(`document.getElementById('new-task').click()`)
	waitUntil(t, 10*time.Second, "the task dialog opens from the button", func() bool {
		return p.is(`!document.getElementById('task').hidden && document.querySelector('#task .title').textContent.length > 0 && !document.querySelector('#task .title').textContent.includes('(')`)
	})
	p.run(`[...document.querySelectorAll('#task .mem')].find(r => r.textContent.includes('carol')).querySelector('input').click()`)
	p.run(`document.querySelector('#task .add-adv').click()`)
	p.run(fmt.Sprintf(`(r => { r.querySelector('[name=name]').value = 'rev'; r.querySelector('[name=dir]').value = %q })(document.querySelector('#task .adv'))`, dir("docs")))
	p.run(`document.querySelector('#task [name=desc]').value = 'Review the docs'; document.querySelector('#task .go').click()`)
	waitUntil(t, 30*time.Second, "the board shows the second task", func() bool {
		return p.is(`document.getElementById('task').hidden && document.querySelectorAll('.board .task').length === 2`)
	})
	if n := len(b.Hub.Tasks()); n != 2 {
		t.Fatalf("two tasks expected, got %d", n)
	}
	if found := func() bool {
		for _, k := range b.Hub.Tasks() {
			if k.ID == "T2" && k.Room == nil {
				return true
			}
		}
		return false
	}(); !found {
		t.Fatal("a task from the button should carry no room hint")
	}
	p.run(`document.querySelector('#taskwin .close').click()`)
	p.run(`document.querySelector('.board .task').click()`)
	waitUntil(t, 10*time.Second, "the task window opens", func() bool { return p.is(`!document.getElementById('taskwin').hidden`) })
	p.run(`document.querySelector('#taskwin .mrow').click()`)
	waitUntil(t, 10*time.Second, "the member's window opens over it", func() bool {
		return p.is(`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent.endsWith('-T1')`)
	})
	p.shot("taskwin-member")
	p.check()
	p.run(`document.querySelector('#chat .close').click(); document.querySelector('#taskwin .close').click()`)

	// ---------- activity ----------
	p.run(`document.querySelector('#tabs [data-tab=activity]').click()`)
	waitUntil(t, 10*time.Second, "the timeline has rows", func() bool { return p.is(`document.querySelectorAll('#act-list .row').length >= 5`) })
	p.run(`const s = document.getElementById('act-kind'); s.value = 'tool'; s.dispatchEvent(new Event('change'))`)
	if !p.is(`[...document.querySelectorAll('#act-list .row .badge')].every(b => b.classList.contains('tool'))`) {
		t.Fatal("the kind filter should leave tool rows only")
	}
	p.run(`const s = document.getElementById('act-kind'); s.value = ''; s.dispatchEvent(new Event('change'))`)
	p.shot("activity")
	p.check()

	// ---------- the search box, dark mode ----------
	p.run(`const f = document.getElementById('find'); f.focus(); f.value = 'bo'; f.dispatchEvent(new Event('input'))`)
	waitUntil(t, 5*time.Second, "the search finds bob", func() bool {
		return p.is(`!document.getElementById('find-list').hidden && document.querySelector('#find-list .hit b')?.textContent === 'bob'`)
	})
	p.shot("search")
	p.run(`document.getElementById('find').dispatchEvent(new KeyboardEvent('keydown', { key:'Enter', bubbles:true }))`)
	waitUntil(t, 5*time.Second, "bob's window opens", func() bool {
		return p.is(`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === 'bob'`)
	})
	p.run(`document.querySelector('#chat .close').click()`)
	p.run(`document.getElementById('dark').click(); document.querySelector('#tabs [data-tab=overview]').click()`)
	if !p.is(`document.documentElement.hasAttribute('data-dark')`) {
		t.Fatal("the switch should turn dark mode on")
	}
	p.shot("overview-dark")
	p.check()
	p.done()

	// ---------- zh-CN ----------
	zh, closeZh := clockTab(t, browser, b.URL+"/t/hq/?lang=zh-CN#people", 0)
	do(t, zh, 10*time.Second, chromedp.EmulateViewport(1400, 860))
	q := &screens{t: t, ctx: zh, lang: "hq-zh-CN"}
	waitUntil(t, 20*time.Second, "zh-CN: the roster", func() bool {
		return q.is(`document.querySelectorAll('#people tbody tr').length === 4 && document.documentElement.lang === 'zh-CN'`)
	})
	q.shot("people")
	q.check()
	q.run(`document.querySelector('#tabs [data-tab=overview]').click()`)
	q.shot("overview")
	q.check()
	q.done()
	closeZh()
}
