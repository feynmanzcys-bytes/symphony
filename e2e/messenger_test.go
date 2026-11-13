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

// TestMessenger walks through the Messenger theme with made-up activity (no tokens): the coworkers in the sidebar
// with their faces and progress lines, a coworker's conversation in the middle with her ask's card in the flow, under
// its line, the input still usable, answered there, the profile and the computer on the right with the figures, a group chat (a task) with its team
// chat and a member's conversation from it, a new group from the button, the search box, dark mode, and the sidebar
// in zh-CN. Screenshots into e2e/artifacts/screens/messenger-*. It fails on a page error, a key a catalog lacks, or
// a raw key on screen.
func TestMessenger(t *testing.T) {
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

	tokens := map[string]string{}
	event := func(name, session string, body map[string]any) {
		t.Helper()
		body["session"] = session
		b.post(t, tokens[name], "/api/event", body)
	}
	team := []struct{ name, dir, model, effort string }{{"alice", "app", "opus", "max"}, {"bob", "web", "sonnet", "high"}, {"carol", "docs", "haiku", "low"}, {"erin", "docs", "sonnet", "medium"}, {"dave", "", "", ""}}
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
			"engine": map[string]any{"version": "2.1.316"}, "mod": "0.11.0", "surface": "terminal", "repo": map[string]any{"root": dir(m.dir), "name": m.dir}})
		event(m.name, "s-"+m.name, map[string]any{"kind": "session.title", "title": m.name + "-desk"})
	}
	event("alice", "s-alice", map[string]any{"kind": "chat.user", "text": "Run the test suite and fix what fails", "origin": "composer"})
	event("alice", "s-alice", map[string]any{"kind": "turn.start", "text": "Run the test suite and fix what fails"})
	event("alice", "s-alice", map[string]any{"kind": "chat.delta", "turnId": "a1", "step": 0, "blocks": []map[string]any{{"i": 0, "kind": "text", "text": "On it: running `make test` first."}}})
	event("alice", "s-alice", map[string]any{"kind": "chat.stop", "turnId": "a1", "step": 0, "tokens": map[string]any{"input": 12000, "output": 800, "cacheRead": 90000, "cacheWrite": 0, "model": "claude-opus-5-5"}})
	event("alice", "s-alice", map[string]any{"kind": "tool.start", "tool": "Bash", "summary": "make test", "toolUseId": "tu1"})
	event("alice", "s-alice", map[string]any{"kind": "session.usage", "usage": map[string]any{"context": map[string]any{"tokens": 68000, "window": 200000, "percent": 34}, "rateLimits": []map[string]any{{"kind": "five_hour", "percentUsed": 12}}, "cost": map[string]any{"usd": 1.25}}})
	event("alice", "s-alice", map[string]any{"kind": "session.tools", "toolCount": 30, "mcp": []string{"github"}})
	event("alice", "s-alice", map[string]any{"kind": "agent.start", "agentId": "ag1", "agentType": "Explore", "description": "find the failing tests", "model": "claude-haiku-5-5"})
	event("bob", "s-bob", map[string]any{"kind": "chat.user", "text": "Tidy the README", "origin": "composer"})
	event("bob", "s-bob", map[string]any{"kind": "turn.start", "text": "Tidy the README"})
	event("bob", "s-bob", map[string]any{"kind": "turn.complete", "turnId": "b1", "reason": "answer"})
	event("erin", "s-erin", map[string]any{"kind": "session.end", "reason": "exit"}) // off duty since now
	b.post(t, tokens["carol"], "/api/ask", map[string]any{"session": "s-carol", "kind": "question", "questions": []map[string]any{{"question": "Should the README cover the Windows build too?", "header": "Scope", "multiSelect": false, "options": []map[string]any{{"label": "Yes", "description": "briefly"}, {"label": "No", "description": "later"}}}}})
	waitUntil(t, 10*time.Second, "the ask is pending", func() bool { return len(b.Hub.Snapshot().Asks) == 1 })
	askID := b.Hub.Snapshot().Asks[0].ID

	browser := newBrowser(t)
	en, _ := clockTab(t, browser, b.URL+"/t/messenger/?lang=en", 0)
	do(t, en, 10*time.Second, chromedp.EmulateViewport(1400, 860))
	p := &screens{t: t, ctx: en, lang: "messenger-en"}
	waitUntil(t, 20*time.Second, "the sidebar lists the five coworkers, carol first", func() bool {
		return p.is(`document.querySelectorAll('#people .row').length === 5 && document.querySelector('#people .row .nm').textContent.trim() === 'carol'`)
	})
	// the theme is called Coworkers on the page (its folder and URLs stay messenger)
	if !p.is(`document.querySelector('h1[data-i18n="msgr.title"]').textContent === 'Symphony Coworkers' && document.title.endsWith('Symphony Coworkers')`) {
		t.Fatalf("the title should be Symphony Coworkers: %s | %s", p.js(`document.querySelector('h1[data-i18n="msgr.title"]').textContent`), p.js(`document.title`))
	}
	if !p.is(`document.querySelector('#people .row[data-account] .face').dataset.state === 'waiting' && document.querySelector('#people .row .sub.hot')?.textContent === 'Needs an answer'`) {
		t.Fatalf("carol's face should wave and her line say what she waits for: %s", p.js(`document.querySelector('#people .row .sub').textContent`))
	}
	if !p.is(`[...document.querySelectorAll('#people .row')].some(r => r.textContent.includes('erin') && r.querySelector('.face').dataset.state === 'ended' && /Off duty since \d/.test(r.querySelector('.sub').textContent))`) {
		t.Fatalf("erin's line should say since when she is off duty: %s", p.js(`[...document.querySelectorAll('#people .row')].map(r => r.textContent).join(' | ')`))
	}
	if !p.is(`[...document.querySelectorAll('#people .row')].some(r => r.textContent.includes('alice') && r.querySelector('.face').dataset.state === 'tool' && r.querySelector('.sub').textContent.startsWith('On it: running') && r.querySelector('.nm').textContent.includes('Working'))`) {
		t.Fatalf("alice's row should show the last line of her conversation, her state said for screen readers only: %s", p.js(`document.getElementById('people').innerText`))
	}
	p.shot("start")
	p.check()

	// carol: the conversation with her question's card in the flow, under its line, and the input still there; answer it
	p.run(`[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('carol')).click()`)
	card := fmt.Sprintf(`#chat .flow .it.ask.carded[data-ask="%s"] .card[data-id="%s"]`, askID, askID)
	waitUntil(t, 10*time.Second, "carol's conversation shows with the card in its line", func() bool {
		return p.is(fmt.Sprintf(`!document.getElementById('chat').hidden && !!document.querySelector('%s') && !document.getElementById('profile').hidden`, card))
	})
	if !p.is(`!document.querySelector('#chat .dock').classList.contains('asking') && getComputedStyle(document.querySelector('#chat .say')).display !== 'none' && !document.querySelector('#chat .say textarea').disabled`) {
		t.Fatal("the input should stay usable while the card waits in the flow")
	}
	p.shot("conversation-ask")
	p.check()
	p.run(fmt.Sprintf(`(c => { c.querySelector('input[type=radio]').click(); c.querySelector('.submit').click() })(document.querySelector('%s'))`, card))
	waitUntil(t, 10*time.Second, "the question is answered, its card gone from its line", func() bool {
		return !b.pending(askID) && p.is(fmt.Sprintf(`!document.querySelector('#chat .flow .card') && !document.querySelector('#chat .flow .it.ask[data-ask="%s"]')?.classList.contains('carded')`, askID))
	})

	// alice: the profile shows her figures and the computer what runs
	p.run(`[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('alice')).click()`)
	waitUntil(t, 10*time.Second, "alice's profile shows the figures", func() bool {
		return p.is(`document.getElementById('profile').textContent.includes('34%') && document.getElementById('profile').textContent.includes('$1.25') && document.getElementById('profile').textContent.includes('github') && document.getElementById('profile').textContent.includes('Explore') && document.querySelector('#profile .computer .now.on') !== null`)
	})
	waitUntil(t, 10*time.Second, "alice's conversation shows her answer (the log is fetched when the window opens)", func() bool {
		return p.is(`document.querySelector('#chat .flow').textContent.includes('make test')`)
	}, func() string { return p.js(`document.querySelector('#chat .flow').innerText`) })
	p.shot("conversation-alice")
	p.check()

	// a group: a task from the page, its members report in; the group chat in the middle, a member's conversation from it
	var task bridge.Task
	body, _ := json.Marshal(map[string]any{"title": "Write the CHANGELOG for 0.11", "leader": true, "permissionMode": "bypassPermissions", "lang": "en",
		"members": []map[string]any{{"account": b.account("alice").ID, "model": "opus", "effort": "max"}, {"account": b.account("bob").ID, "model": "sonnet", "effort": "high"}}})
	if err := json.Unmarshal([]byte(p.fetch("POST", "/api/tasks", string(body))), &task); err != nil || task.ID == "" {
		t.Fatalf("creating a task failed: %v", err)
	}
	for _, m := range task.Members {
		name := strings.TrimSuffix(m.Name, "-"+task.ID)
		event(name, name+"-task", map[string]any{"kind": "session.start", "cwd": m.Worktree, "launch": m.Launch, "task": task.ID, "model": "claude-opus-5-5"})
	}
	waitUntil(t, 10*time.Second, "the group shows in the sidebar with its members in", func() bool {
		return p.is(`document.querySelectorAll('#groups .row').length === 1`) && func() bool {
			for _, k := range b.Hub.Tasks() {
				if k.ID == task.ID && k.Members[0].Session != "" && k.Members[1].Session != "" {
					return true
				}
			}
			return false
		}()
	})
	p.run(`document.querySelector('#groups .row').click()`)
	waitUntil(t, 10*time.Second, "the group chat shows in the middle", func() bool {
		return p.is(`!document.getElementById('taskwin').hidden && document.getElementById('chat').hidden && document.querySelectorAll('#taskwin .mrow').length === 2 && document.querySelectorAll('#profile .list .row').length === 2`)
	})
	p.shot("group")
	p.check()
	p.run(`document.querySelector('#taskwin .mrow').click()`)
	waitUntil(t, 10*time.Second, "the member's conversation replaces it, with the way back", func() bool {
		return p.is(`!document.getElementById('chat').hidden && document.getElementById('taskwin').hidden && (h => h && !/-T\d+$/.test(h))(document.querySelector('#chat h2').textContent) && !!document.querySelector('#profile .link[data-group]')`)
	})
	p.shot("member")
	p.check()
	p.run(`document.querySelector('#profile .link[data-group]').click()`)
	waitUntil(t, 10*time.Second, "back to the group", func() bool { return p.is(`!document.getElementById('taskwin').hidden`) })

	// a new group from the button: carol and an advisor
	p.run(`document.getElementById('new-task').click()`)
	waitUntil(t, 10*time.Second, "the task dialog opens", func() bool { return p.is(`!document.getElementById('task').hidden`) })
	p.run(`[...document.querySelectorAll('#task .mem')].find(r => r.textContent.includes('carol')).querySelector('input').click()`)
	p.run(`document.querySelector('#task .add-adv').click()`)
	p.run(fmt.Sprintf(`(r => { r.querySelector('[name=name]').value = 'rev'; r.querySelector('[name=dir]').value = %q })(document.querySelector('#task .adv'))`, dir("docs")))
	p.run(`document.querySelector('#task [name=desc]').value = 'Review the docs'; document.querySelector('#task .go').click()`)
	waitUntil(t, 30*time.Second, "the second group shows", func() bool {
		return p.is(`document.getElementById('task').hidden && document.querySelectorAll('#groups .row').length === 2 && !document.getElementById('taskwin').hidden && document.getElementById('chat').hidden && document.querySelector('#groups .row.on')?.textContent.includes('T2')`)
	})
	p.shot("groups")

	// the search box narrows the sidebar; dark mode
	p.run(`const f = document.getElementById('find'); f.value = 'bo'; f.dispatchEvent(new Event('input'))`)
	if !p.is(`document.querySelectorAll('#people .row').length === 1 && document.querySelector('#people .row .nm').textContent.trim().startsWith('bob')`) {
		t.Fatalf("the search should leave bob:\n%s", p.js(`[...document.querySelectorAll('#people .row')].map(r => r.dataset.account + ' ' + r.querySelector('.nm').textContent).join('\n')`))
	}
	p.run(`const f = document.getElementById('find'); f.value = ''; f.dispatchEvent(new Event('input'))`)
	p.run(`document.getElementById('set-scheme-dark').click(); [...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('alice')).click()`)
	if !p.is(`document.documentElement.hasAttribute('data-dark')`) {
		t.Fatal("the settings should turn dark mode on")
	}
	p.shot("dark")
	p.check()
	p.done()

	// zh-CN
	zh, closeZh := clockTab(t, browser, b.URL+"/t/messenger/?lang=zh-CN", 0)
	do(t, zh, 10*time.Second, chromedp.EmulateViewport(1400, 860))
	q := &screens{t: t, ctx: zh, lang: "messenger-zh-CN"}
	waitUntil(t, 20*time.Second, "zh-CN: the sidebar", func() bool {
		return q.is(`document.querySelectorAll('#people .row').length === 5 && document.documentElement.lang === 'zh-CN'`)
	})
	if title := q.js(`document.querySelector('h1[data-i18n="msgr.title"]').textContent`); title != "Symphony 同事" {
		t.Fatalf("zh-CN: the title should be Symphony 同事: %s", title)
	}
	q.run(`[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('alice')).click()`)
	waitUntil(t, 10*time.Second, "zh-CN: alice's conversation", func() bool { return q.is(`!document.getElementById('chat').hidden`) })
	q.shot("conversation")
	// a coworker on duty: its conversation started over (/clear) and sent off duty from the window's head
	q.run(`window.confirm = () => true; [...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('bob')).click()`)
	waitUntil(t, 10*time.Second, "zh-CN: bob's head offers 清空 and 下班", func() bool {
		return q.is(`document.querySelector('#chat .btns .clear')?.textContent === '清空' && document.querySelector('#chat .btns .off-duty')?.textContent === '下班'`)
	}, func() string {
		return q.js(`[...document.querySelectorAll('#chat .btns button')].map(b => b.className + ' ' + b.textContent).join(' | ')`)
	})
	q.run(`document.querySelector('#chat .btns .clear').click()`)
	waitUntil(t, 10*time.Second, "zh-CN: a session whose mod is too old to be told says so", func() bool {
		return q.is(`document.getElementById('oops')?.hidden === false && document.querySelector('#oops h2').textContent === '没能重新开始对话' && document.querySelector('#oops .msg').textContent.includes('0.11.0')`)
	}, func() string { return q.js(`document.getElementById('oops')?.textContent || ''`) })
	q.run(`document.querySelector('#oops .actions button').click()`)
	event("bob", "s-bob", map[string]any{"kind": "heartbeat", "mod": "1.9.0"})
	commands := func() string {
		items, _ := b.Hub.Inbox("s-bob", b.account("bob").ID, -1, time.Millisecond, nil)
		var got []string
		for _, it := range items {
			got = append(got, it.Kind+" "+it.Command)
		}
		return strings.Join(got, ",")
	}
	q.run(`document.querySelector('#chat .btns .clear').click()`)
	waitUntil(t, 10*time.Second, "zh-CN: 清空 sends /clear to the session", func() bool { return commands() == "command clear" })
	q.run(`document.querySelector('#chat .btns .off-duty').click()`)
	waitUntil(t, 10*time.Second, "zh-CN: 下班 sends a hand-started session /exit", func() bool { return commands() == "command exit" })
	q.check()
	q.done()
	closeZh()
}
