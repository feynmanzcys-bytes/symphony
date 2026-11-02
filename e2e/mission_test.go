//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// TestMission walks through the Mission Control theme with made-up activity (no tokens): a tile per session with the
// state word, the gauges and the sparkline, a task's members with its stripe, the off-duty tiles dimmed; the room by
// day (the system's light scheme) and at night (the switch, remembered across a reload); the alarm in the bar opening
// the waiting session's window, where the ask is answered; a tile opening a window, a stripe the task window; the search
// box; the ticker; the grid fitting the window as the team grows; and the wall in zh-CN, by day and at night.
// Screenshots into e2e/artifacts/screens/mission-*. It fails on a page error, a key a catalog lacks, or a raw key on screen.
func TestMission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake claude is a shell script")
	}
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
	fake := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexec sleep 100000\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	b.Launcher.Claude = fake
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
		event(m.name, "s-"+m.name, map[string]any{"kind": "session.start", "cwd": dir(m.dir), "model": "claude-" + m.model + "-5-5", "effort": m.effort})
	}
	event("alice", "s-alice", map[string]any{"kind": "turn.start", "text": "Run the test suite and fix what fails"})
	event("alice", "s-alice", map[string]any{"kind": "tool.start", "tool": "Bash", "summary": "make test", "toolUseId": "tu1"})
	event("alice", "s-alice", map[string]any{"kind": "session.usage", "usage": map[string]any{"context": map[string]any{"tokens": 68000, "window": 200000, "percent": 34}, "rateLimits": []map[string]any{{"kind": "five_hour", "percentUsed": 12}, {"kind": "seven_day", "percentUsed": 41}}, "cost": map[string]any{"usd": 1.25}, "totals": map[string]any{"claude-opus-5-5": map[string]any{"input": 12000, "output": 800, "cacheRead": 0, "cacheWrite": 0}}}})
	event("bob", "s-bob", map[string]any{"kind": "session.usage", "usage": map[string]any{"context": map[string]any{"tokens": 150000, "window": 200000, "percent": 75}, "cost": map[string]any{"usd": 0.4}}})
	b.post(t, tokens["carol"], "/api/ask", map[string]any{"session": "s-carol", "kind": "permission", "tool": "Bash", "input": map[string]any{"command": "rm -rf build && npm run build"}})
	waitUntil(t, 10*time.Second, "the ask is pending", func() bool { return len(b.Hub.Snapshot().Asks) == 1 })
	askID := b.Hub.Snapshot().Asks[0].ID

	browser := newBrowser(t)
	en, _ := clockTab(t, browser, b.URL+"/t/mission/?lang=en", 0)
	do(t, en, 10*time.Second, chromedp.EmulateViewport(1400, 860))
	p := &screens{t: t, ctx: en, lang: "mission-en"}
	waitUntil(t, 20*time.Second, "the wall shows four tiles, the alarm first", func() bool {
		return p.is(`document.querySelectorAll('#wall .tile').length === 4 && document.querySelector('#wall .tile').classList.contains('waiting') && document.getElementById('stat-alarms').classList.contains('hot')`)
	})
	if got := p.js(`[...document.querySelectorAll('#wall .tile .nm')].map(e => e.textContent).join(',')`); got != "carol,alice,bob,dave" {
		t.Fatalf("tiles should sort alarm, working, idle, off: %s", got)
	}
	if !p.is(`(tile => tile.querySelectorAll('.gauge').length === 3 && tile.textContent.includes('34%') && tile.textContent.includes('5H') && !!tile.querySelector('.spark svg') && tile.textContent.includes('Bash'))([...document.querySelectorAll('#wall .tile')].find(x => x.textContent.includes('alice')))`) {
		t.Fatalf("alice's tile should show her gauges, sparkline and tool: %s", p.js(`[...document.querySelectorAll('#wall .tile')].find(x => x.textContent.includes('alice')).outerHTML + ' | wall ' + document.getElementById('wall').clientHeight + 'x' + document.getElementById('wall').clientWidth`))
	}
	if !p.is(`[...document.querySelectorAll('#wall .tile')].find(x => x.textContent.includes('dave')).classList.contains('off')`) {
		t.Fatal("dave's tile should be dimmed")
	}
	if !p.is(`/^T\+\d\d:\d\d:\d\d$/.test(document.getElementById('mission-time').textContent) && document.getElementById('stat-sessions').textContent === '3/4' && document.getElementById('stat-cost').textContent === '$1.65'`) {
		t.Fatalf("the bar: %s", p.js(`document.querySelector('.bar').innerText`))
	}
	waitUntil(t, 10*time.Second, "the ticker has lines", func() bool { return p.is(`document.querySelectorAll('#ticker .item').length >= 4`) })
	// by day: the browser's scheme is light (newBrowser), and nothing was picked yet
	if !p.is(`!document.documentElement.hasAttribute('data-dark') && document.getElementById('dark').getAttribute('aria-pressed') === 'false' && getComputedStyle(document.body).backgroundColor === 'rgb(223, 229, 236)'`) {
		t.Fatalf("a light system should get the room by day: %s", p.js(`getComputedStyle(document.body).backgroundColor`))
	}
	p.shot("wall")
	p.check()
	// at night: the switch, remembered across a reload
	p.run(`document.getElementById('dark').click()`)
	if !p.is(`document.documentElement.hasAttribute('data-dark') && localStorage.getItem('symphony.mission.dark') === '1' && document.getElementById('dark').getAttribute('aria-pressed') === 'true'`) {
		t.Fatal("the switch should turn the room dark, say so and remember it")
	}
	do(t, en, 30*time.Second, chromedp.Reload())
	waitUntil(t, 20*time.Second, "after a reload the room is still dark", func() bool {
		return p.is(`document.querySelectorAll('#wall .tile').length === 4 && document.documentElement.hasAttribute('data-dark') && getComputedStyle(document.body).backgroundColor === 'rgb(7, 11, 16)'`)
	})
	waitUntil(t, 10*time.Second, "the ticker has lines", func() bool { return p.is(`document.querySelectorAll('#ticker .item').length >= 4`) })
	p.shot("wall-dark")
	p.check()
	p.run(`document.getElementById('stat-alarms').click()`)
	waitUntil(t, 10*time.Second, "carol's window opens, dark", func() bool { return p.is(`!!document.querySelector('#chat #asks .card')`) })
	p.shot("alarm-dark")
	p.check()
	p.run(`document.querySelector('#chat .close').click(); document.getElementById('dark').click()`) // back to day

	// the alarm opens carol's window; the ask is answered there
	p.run(`document.getElementById('stat-alarms').click()`)
	waitUntil(t, 10*time.Second, "carol's window opens with the card", func() bool {
		return p.is(fmt.Sprintf(`!document.getElementById('chat').hidden && !!document.querySelector('#chat #asks .card[data-id="%s"]')`, askID))
	})
	p.shot("alarm")
	p.check()
	p.run(fmt.Sprintf(`document.querySelector('#chat #asks .card[data-id="%s"] .allow').click()`, askID))
	waitUntil(t, 10*time.Second, "the alarm is gone", func() bool {
		return !b.pending(askID) && p.is(`!document.getElementById('stat-alarms').classList.contains('hot') && !document.querySelector('#wall .tile').classList.contains('waiting')`)
	})
	p.run(`document.querySelector('#chat .close').click()`)

	// a task: its members' tiles carry its stripe; the stripe opens the task window, a member tile its window beside it
	var task bridge.Task
	body, _ := json.Marshal(map[string]any{"title": "Write the CHANGELOG for 0.12", "leader": true, "permissionMode": "bypassPermissions", "lang": "en",
		"members": []map[string]any{{"account": b.account("alice").ID, "model": "opus", "effort": "max"}, {"account": b.account("bob").ID, "model": "sonnet", "effort": "high"}}})
	if err := json.Unmarshal([]byte(p.fetch("POST", "/api/tasks", string(body))), &task); err != nil || task.ID == "" {
		t.Fatalf("creating a task failed: %v", err)
	}
	for _, m := range task.Members {
		name := strings.TrimSuffix(m.Name, "-"+task.ID)
		event(name, name+"-task", map[string]any{"kind": "session.start", "cwd": m.Worktree, "launch": m.Launch, "task": task.ID, "model": "claude-opus-5-5"})
	}
	waitUntil(t, 10*time.Second, "the members' tiles come with the stripe", func() bool {
		return p.is(`document.querySelectorAll('#wall .tile.member .stripe').length === 2 && document.querySelectorAll('#wall .tile').length === 6`)
	})
	p.shot("wall-task")
	p.check()
	p.run(`document.querySelector('#wall .tile.member .stripe').click()`)
	waitUntil(t, 10*time.Second, "the task window opens", func() bool {
		return p.is(`!document.getElementById('taskwin').hidden && document.getElementById('chat').hidden`)
	})
	p.run(`document.querySelector('#taskwin .close').click()`)
	p.run(`document.querySelector('#wall .tile.member').click()`)
	waitUntil(t, 10*time.Second, "the member's window opens with its task window", func() bool {
		return p.is(`!document.getElementById('taskwin').hidden && !document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent.endsWith('-T1')`)
	})
	p.shot("member")
	p.check()
	p.run(`document.querySelector('#chat .close').click(); document.querySelector('#taskwin .close').click()`)

	// the search box narrows the wall; the sound switch is remembered
	p.run(`const f = document.getElementById('find'); f.value = 'T1'; f.dispatchEvent(new Event('input'))`)
	if !p.is(`document.querySelectorAll('#wall .tile').length === 2`) {
		t.Fatal("the search should leave the task's two tiles")
	}
	p.run(`const f = document.getElementById('find'); f.value = ''; f.dispatchEvent(new Event('input'))`)
	p.run(`document.getElementById('sound').click()`)
	if !p.is(`localStorage.getItem('symphony.mission.sound') === '1'`) {
		t.Fatal("the sound switch should be remembered")
	}
	p.run(`document.getElementById('sound').click()`)
	p.done()

	// a bigger team still fits: 14 more accounts off duty, no scrolling, every tile on screen
	for i := 0; i < 14; i++ {
		if _, err := b.Hub.CreateAccount(fmt.Sprintf("op%02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	waitUntil(t, 10*time.Second, "twenty tiles on the wall", func() bool { return p.is(`document.querySelectorAll('#wall .tile').length === 20`) })
	if !p.is(`(w => w.scrollHeight <= w.clientHeight + 1 && [...w.querySelectorAll('.tile')].every(x => x.getBoundingClientRect().bottom <= innerHeight && x.getBoundingClientRect().right <= innerWidth) && document.querySelector('.bar').scrollWidth <= innerWidth)(document.getElementById('wall'))`) {
		t.Fatalf("every tile should fit on screen: %s", p.js(`(w => JSON.stringify({ wall:[w.scrollHeight, w.clientHeight, w.scrollWidth, w.clientWidth], inner:[innerWidth, innerHeight], bar:document.querySelector('.bar').scrollWidth, over:[...w.querySelectorAll('.tile')].filter(x => x.getBoundingClientRect().bottom > innerHeight || x.getBoundingClientRect().right > innerWidth).map(x => x.textContent.slice(0, 8) + JSON.stringify(x.getBoundingClientRect())) }))(document.getElementById('wall'))`))
	}
	p.shot("wall-20")
	p.check()

	// zh-CN
	zh, closeZh := clockTab(t, browser, b.URL+"/t/mission/?lang=zh-CN", 0)
	do(t, zh, 10*time.Second, chromedp.EmulateViewport(1400, 860))
	q := &screens{t: t, ctx: zh, lang: "mission-zh-CN"}
	waitUntil(t, 20*time.Second, "zh-CN: the wall", func() bool {
		return q.is(`document.querySelectorAll('#wall .tile').length === 20 && document.documentElement.lang === 'zh-CN'`)
	})
	q.shot("wall")
	q.check()
	q.run(`document.getElementById('dark').click()`)
	q.shot("wall-dark")
	q.check()
	q.run(`document.getElementById('dark').click()`)
	q.done()
	closeZh()
}
