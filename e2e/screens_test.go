//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// TestScreens is a screenshot tour of the office page, for looking at it rather than for checking behavior (no
// tokens: claude is a script that sits there, and the test reports the sessions in as the mod would). It sets up a
// team in every state, three tasks in the small meeting rooms (one of 3 members, seated; one of 5 in a room for 3, two
// of them standing; one failed on a merge conflict) and walks every screen: the office at every pixel size and at a
// small and a big window, the nameplates, the task dialog in each of its states, the task window running, flat and
// failed, the employee window of a member, of a desk employee and of one away in a task, the HUD with the search box,
// the message and activity logs, the launch dialog and the /approve page; the task screens again in zh-CN and ja. The
// screenshots go to e2e/artifacts/screens/. It fails on a page error, a key a catalog lacks, or a raw key on screen.
func TestScreens(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake claude is a shell script")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	requireBrowser(t)
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))
	t.Cleanup(func() { // the tasks end and their git is tidied up before the folders go
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

	// the folders: app is a repository, the others plain folders (made repositories for the tasks)
	work := t.TempDir()
	dir := func(name string) string { return filepath.Join(work, name) }
	for _, d := range []string{"app", "plain", "web", "api", "docs", "design"} {
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

	// ---------- the team: working, idle, waiting, off duty ----------
	tokens := map[string]string{}
	event := func(name, session string, body map[string]any) {
		t.Helper()
		body["session"] = session
		b.post(t, tokens[name], "/api/event", body)
	}
	team := []struct{ name, dir, model, effort string }{
		{"alice", "web", "opus", "max"}, {"bob", "app", "opus", "high"}, {"carol", "api", "sonnet", "medium"}, {"dave", "docs", "haiku", "low"},
		{"erin", "plain", "opus", "max"}, {"frank", "plain", "sonnet", "high"}, {"gina", "api", "fable", "max"}, {"hal", "", "", ""},
	}
	for _, m := range team {
		acc, err := b.Hub.CreateAccount(m.name)
		if err != nil {
			t.Fatal(err)
		}
		tokens[m.name] = acc.Token
		if m.dir == "" {
			continue // hal has never worked: off duty, no folder
		}
		event(m.name, "s-"+m.name, map[string]any{"kind": "session.start", "cwd": dir(m.dir), "model": m.model, "effort": m.effort})
		event(m.name, "s-"+m.name, map[string]any{"kind": "session.title", "title": m.name + "-desk"})
	}
	event("alice", "s-alice", map[string]any{"kind": "chat.user", "text": "Run the test suite and fix what fails", "origin": "composer"})
	event("alice", "s-alice", map[string]any{"kind": "turn.start", "text": "Run the test suite and fix what fails"})
	event("alice", "s-alice", map[string]any{"kind": "chat.delta", "turnId": "a1", "step": 0, "blocks": []map[string]any{
		{"i": 0, "kind": "thinking", "text": "The suite is in `make test`; let me run it first and look at the failures."},
		{"i": 1, "kind": "text", "text": "I'll run the tests first:\n\n```\nmake test\n```\n\nThen I'll look at **each failure** in turn."}}})
	event("alice", "s-alice", map[string]any{"kind": "chat.stop", "turnId": "a1", "step": 0})
	event("alice", "s-alice", map[string]any{"kind": "tool.start", "tool": "Read", "summary": "src/router.js"})
	event("alice", "s-alice", map[string]any{"kind": "tool.end", "tool": "Read", "summary": "src/router.js"})
	event("alice", "s-alice", map[string]any{"kind": "tool.start", "tool": "Bash", "summary": "make test"})
	event("gina", "s-gina", map[string]any{"kind": "tool.start", "tool": "Grep", "summary": "TODO"})
	b.post(t, tokens["carol"], "/api/ask", map[string]any{"session": "s-carol", "kind": "permission", "tool": "Bash", "input": map[string]any{"command": "rm -rf build && npm run build"}})
	event("dave", "s-dave", map[string]any{"kind": "session.end"}) // off duty, and later away in a task
	event("erin", "s-erin", map[string]any{"kind": "session.end"})

	browser := newBrowser(t)
	en, _ := clockTab(t, browser, b.URL+"/?lang=en", 0)
	do(t, en, 10*time.Second, chromedp.EmulateViewport(1400, 860)) // the window's size less the browser's own bars
	p := &screens{t: t, ctx: en, lang: "en"}
	waitUntil(t, 20*time.Second, "the office shows the eight employees and the clickable nameplates", func() bool {
		return p.is(`document.querySelectorAll('.emp-label').length === 8 && document.querySelectorAll('.room-plate.click').length >= 4 && !!document.querySelector('#stats .chip.hot')`)
	})
	// the rooms: the first three for 3 hold the tasks (the one of 5 stands two in a room for 3), the next is free
	var plates []struct{ Name, Seats string }
	if err := json.Unmarshal([]byte(p.js(`[...document.querySelectorAll('.room-plate')].map(p => ({ Name:p.textContent, Seats:p.dataset.seats }))`)), &plates); err != nil {
		t.Fatal(err)
	}
	var threes, others []int
	for i, pl := range plates {
		if pl.Seats == "3" {
			threes = append(threes, i)
		} else {
			others = append(others, i)
		}
	}
	if len(threes) < 3 || len(others) < 1 {
		t.Fatalf("the tour needs three rooms for 3 and one more: %+v", plates)
	}
	roomT2, roomT3, roomT1, roomFree := threes[0], threes[1], threes[2], others[0]
	t.Logf("rooms: %+v; T1 (failed) in %d, T2 (3) in %d, T3 (5) in %d, free %d", plates, roomT1, roomT2, roomT3, roomFree)

	// tasks: made from the page's own fetch (the bridge refuses writes from elsewhere), members reported in as the mod
	// would, from their worktrees
	createTask := func(room int, title string, leader bool, members []map[string]any) *bridge.Task {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"room": room, "roomName": plates[room].Name, "title": title, "leader": leader,
			"permissionMode": "bypassPermissions", "lang": "en", "members": members})
		res := p.fetch("POST", "/api/tasks", string(body))
		var task bridge.Task
		if err := json.Unmarshal([]byte(res), &task); err != nil || task.ID == "" {
			t.Fatalf("creating a task failed: %s", res)
		}
		return &task
	}
	accounts := func() {
		var list []struct{ Name, Token string }
		raw, err := os.ReadFile(filepath.Join(b.Data, "accounts.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			t.Fatal(err)
		}
		for _, a := range list {
			tokens[a.Name] = a.Token
		}
	}
	// a member reports in, takes its kickoff and is sent it, as the mod does
	reportIn := func(task *bridge.Task, account string) string {
		t.Helper()
		name := account + "-" + task.ID
		var m *bridge.Member
		for _, x := range task.Members {
			if x.Name == name {
				m = x
			}
		}
		if m == nil {
			t.Fatalf("no member %s in %s", name, task.ID)
		}
		sid := "t-" + name
		event(account, sid, map[string]any{"kind": "session.start", "cwd": m.Worktree, "launch": m.Launch, "task": task.ID, "model": m.Model, "effort": m.Effort})
		event(account, sid, map[string]any{"kind": "session.title", "task": task.ID, "title": name})
		req, _ := http.NewRequest("GET", b.URL+"/api/inbox?session="+sid, nil)
		req.Header.Set("Authorization", "Bearer "+tokens[account])
		res, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var inbox struct{ Items []struct{ Kind, Text string } }
		err = json.NewDecoder(res.Body).Decode(&inbox)
		res.Body.Close()
		if err != nil || len(inbox.Items) != 1 {
			t.Fatalf("%s should have its kickoff waiting: %+v (%v)", name, inbox.Items, err)
		}
		event(account, sid, map[string]any{"kind": "chat.sent", "task": task.ID, "text": inbox.Items[0].Text})
		return sid
	}
	message := func(fromAcc, from, toAcc, to, text string) {
		wrapped := fmt.Sprintf("<cross-session-message from=\"uds:/tmp/x.sock\" from-name=%q from-mode=\"prompting\">\n%s\n</cross-session-message>", from, text)
		event(toAcc, "t-"+to, map[string]any{"kind": "message.receive", "text": wrapped, "origin": map[string]any{"kind": "peer"}})
		event(fromAcc, "t-"+from, map[string]any{"kind": "message.send", "to": to, "text": text, "origin": "model", "delivered": true})
	}
	accountID := func(name string) string { return b.account(name).ID }

	// ---------- T1: a task that fails, on a merge conflict in the plain folder erin and frank share ----------
	t1 := createTask(roomT1, "Tidy up the notes", true, []map[string]any{
		{"account": accountID("erin"), "model": "opus", "effort": "max"}, {"account": accountID("frank"), "model": "sonnet", "effort": "high"}})
	reportIn(t1, "erin")
	reportIn(t1, "frank")
	for i, m := range t1.Members {
		if err := os.WriteFile(filepath.Join(m.Worktree, "notes.txt"), []byte(fmt.Sprintf("version %d by %s\n", i, m.Name)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p.fetch("POST", "/api/tasks/"+t1.ID+"/end", "{}")
	waitUntil(t, 60*time.Second, "T1 fails on the merge", func() bool {
		for _, k := range b.Hub.Tasks() {
			if k.ID == t1.ID {
				return k.Status == bridge.TaskFailed
			}
		}
		return false
	}, func() string { data, _ := json.Marshal(b.Hub.Tasks()); return string(data) })

	// ---------- the task dialog, in each of its states, making T2 ----------
	p.dialogOpen(roomT2)
	p.shot("task-dialog-empty")
	p.check()
	p.run(`document.querySelector('#task [name=desc]').value = 'Add a CHANGELOG and a Makefile, and document the release steps in the README'`)
	p.tick("bob")
	p.tick("dave")
	p.shot("task-dialog-picked")
	p.check()
	p.run(`document.querySelector('#task .add-adv').click()`)
	p.run(fmt.Sprintf(`(r => { r.querySelector('[name=name]').value = 'rev'; r.querySelector('[name=dir]').value = %q; r.querySelector('.browse').click() })(document.querySelector('#task .adv'))`, work))
	waitUntil(t, 10*time.Second, "the advisor's folder browser lists the work folder", func() bool {
		return p.is(`(b => !b.hidden && b.querySelectorAll('li').length >= 5)(document.querySelector('#task .adv .browser'))`)
	})
	p.shot("task-dialog-advisor-browser")
	p.check()
	p.run(fmt.Sprintf(`(r => { r.querySelector('.browser').hidden = true; r.querySelector('[name=dir]').value = %q })(document.querySelector('#task .adv'))`, dir("app")))
	p.shot("task-dialog-advisor")
	p.run(`document.querySelector('#task [name=leader]').click()`)
	if !p.is(`!document.querySelector('#task .lead')`) {
		t.Fatal("unticking the lead box should take the lead tag away")
	}
	p.shot("task-dialog-no-lead")
	p.run(`document.querySelector('#task [name=leader]').click()`)
	p.tick("alice")
	p.tick("carol")
	if !p.is(`document.querySelector('#task .count').classList.contains('over') && document.querySelector('#task .go').disabled`) {
		t.Fatalf("5 members in a room for 3 should be too many: %s", p.js(`document.querySelector('#task .count').textContent`))
	}
	p.shot("task-dialog-over")
	p.check()
	p.tick("alice")
	p.tick("carol")
	// the bridge refuses an advisor named like an employee: the error shows in the dialog's status line
	p.run(`document.querySelector('#task .adv [name=name]').value = 'carol'; document.querySelector('#task .go').click()`)
	waitUntil(t, 10*time.Second, "the dialog shows the bridge's refusal", func() bool {
		return p.is(`document.querySelector('#task .status').classList.contains('err') && document.querySelector('#task .status').textContent.length > 0`)
	})
	p.shot("task-dialog-error")
	p.check()
	p.run(`document.querySelector('#task .adv [name=name]').value = 'rev'; document.querySelector('#task .go').click()`)
	waitUntil(t, 30*time.Second, "T2 starts and its window opens with its members starting", func() bool {
		return p.is(`!document.getElementById('taskwin').hidden && document.querySelectorAll('#taskwin .mrow.wait').length === 3`)
	}, func() string { return p.js(`document.querySelector('#task .status').textContent`) })
	p.shot("taskwin-starting")
	p.run(`document.querySelector('#taskwin .close').click()`)
	var t2 *bridge.Task
	for _, k := range b.Hub.Tasks() {
		if k.Status == bridge.TaskRunning {
			t2 = k
		}
	}
	accounts()
	for _, a := range []string{"bob", "dave", "rev"} {
		reportIn(t2, a)
	}
	bobT2, daveT2, revT2 := "bob-"+t2.ID, "dave-"+t2.ID, "rev-"+t2.ID
	event("bob", "t-"+bobT2, map[string]any{"kind": "turn.start", "task": t2.ID, "text": "kickoff"})
	event("bob", "t-"+bobT2, map[string]any{"kind": "chat.delta", "task": t2.ID, "turnId": "b1", "step": 0, "blocks": []map[string]any{
		{"i": 0, "kind": "thinking", "text": "Three of us: I split the work, dave writes the docs, rev reviews."},
		{"i": 1, "kind": "text", "text": "I'll split it: **dave-" + t2.ID + "** takes the README, **rev-" + t2.ID + "** reviews; I write the Makefile."}}})
	event("bob", "t-"+bobT2, map[string]any{"kind": "chat.stop", "task": t2.ID, "turnId": "b1", "step": 0})
	event("bob", "t-"+bobT2, map[string]any{"kind": "tool.start", "task": t2.ID, "tool": "SendMessage", "summary": daveT2})
	event("bob", "t-"+bobT2, map[string]any{"kind": "tool.end", "task": t2.ID, "tool": "SendMessage", "summary": daveT2})
	message("bob", bobT2, "dave", daveT2, "Please write the release steps in the README, on your branch; tell me when it's committed.")
	message("dave", daveT2, "bob", bobT2, "On it. Do we document the Windows build as well?")
	message("rev", revT2, "bob", bobT2, "I'll review the Makefile once it's on your branch.")
	event("bob", "t-"+bobT2, map[string]any{"kind": "tool.start", "task": t2.ID, "tool": "Write", "summary": "Makefile"})
	event("rev", "t-"+revT2, map[string]any{"kind": "turn.start", "task": t2.ID, "text": "kickoff"})
	b.post(t, tokens["dave"], "/api/ask", map[string]any{"session": "t-" + daveT2, "kind": "question", "questions": []map[string]any{{
		"question": "Should the README cover the Windows build too?", "header": "Scope", "multiSelect": false,
		"options": []map[string]string{{"label": "Yes", "description": "Document all three platforms"}, {"label": "No", "description": "Linux and macOS only"}}}}})

	// ---------- T3: five members, flat (nobody leads), in a room for 3 ----------
	t3 := createTask(roomT3, "Port the API client to TypeScript and keep the old one working until every caller has moved", false, []map[string]any{
		{"account": accountID("alice"), "model": "opus", "effort": "max"}, {"account": accountID("gina"), "model": "fable", "effort": "max"},
		{"advisor": map[string]string{"name": "ana", "dir": dir("web")}, "model": "sonnet", "effort": "high"},
		{"advisor": map[string]string{"name": "ben", "dir": dir("api")}, "model": "haiku", "effort": "low"},
		{"advisor": map[string]string{"name": "cyd", "dir": dir("design")}, "model": "opus", "effort": "medium"}})
	accounts()
	for _, a := range []string{"alice", "gina", "ana", "ben", "cyd"} {
		reportIn(t3, a)
	}
	event("alice", "t-alice-"+t3.ID, map[string]any{"kind": "tool.start", "task": t3.ID, "tool": "Edit", "summary": "src/client.ts"})
	event("ana", "t-ana-"+t3.ID, map[string]any{"kind": "turn.start", "task": t3.ID, "text": "kickoff"})
	event("cyd", "t-cyd-"+t3.ID, map[string]any{"kind": "tool.start", "task": t3.ID, "tool": "Read", "summary": "docs/api.md"})
	message("ana", "ana-"+t3.ID, "alice", "alice-"+t3.ID, "I'll take the request builders; you keep the transport?")

	waitUntil(t, 90*time.Second, "the members of T2 and T3 sit (or stand) in their rooms", func() bool {
		return p.is(fmt.Sprintf(`(() => {
			const tags = [...document.querySelectorAll('.emp-label.member')]
			const at = id => tags.filter(l => l.dataset.task === id && l.dataset.moving === '0' && (l.dataset.spot || '').startsWith('room'))
			return at(%q).length === 3 && at(%q).length === 5
		})()`, t2.ID, t3.ID))
	}, func() string {
		return p.js(`[...document.querySelectorAll('.emp-label.member')].map(l => l.querySelector('.nm').textContent + ' ' + JSON.stringify(l.dataset)).join('\n')`)
	})
	if n := p.js(fmt.Sprintf(`[...document.querySelectorAll('.emp-label.member')].filter(l => l.dataset.task === %q && l.dataset.spot.includes('stand')).length`, t3.ID)); n != "2" {
		t.Errorf("two of T3's five should stand in a room for 3, %s do", n)
	}
	p.waitPlates(roomT1, "alert", roomT2, "alert", roomT3, "busy", roomFree, "free")
	time.Sleep(time.Second) // tags settle

	// ---------- the office and its nameplates ----------
	p.shot("office")
	p.check()
	for _, c := range []struct {
		room int
		name string
	}{{roomFree, "free"}, {roomT3, "busy-standing"}, {roomT2, "alert-ask-seated"}, {roomT1, "alert-failed"}} {
		p.clipRoom(c.room, "room-"+c.name)
	}
	p.clip("#hud", 12, 2, "hud-pending")
	// the figures at the tables, without the tags in front of them
	p.run(`document.getElementById('labels').style.visibility = 'hidden'`)
	p.clipTable(roomT2, "table-seated")
	p.clipTable(roomT3, "table-standing")
	p.run(`document.getElementById('labels').style.visibility = ''`)

	// ---------- the task windows: running with a lead and an advisor, flat, failed ----------
	p.openPlate(roomT2)
	waitUntil(t, 10*time.Second, "T2's window shows its three members online", func() bool {
		return p.is(`!document.getElementById('taskwin').hidden && document.querySelectorAll('#taskwin .mrow').length === 3 && !document.querySelector('#taskwin .mrow.wait')`)
	})
	p.shot("taskwin-running")
	p.check()
	p.openPlate(roomT3)
	waitUntil(t, 10*time.Second, "T3's window shows five members and no lead", func() bool {
		return p.is(`document.querySelectorAll('#taskwin .mrow').length === 5 && !document.querySelector('#taskwin .mode').hidden`)
	})
	p.shot("taskwin-flat")
	p.openPlate(roomT1)
	waitUntil(t, 10*time.Second, "T1's window says why it failed and offers to dismiss it", func() bool {
		return p.is(`!document.querySelector('#taskwin .err').hidden && !!document.querySelector('#taskwin .dismiss')`)
	})
	p.shot("taskwin-failed")
	p.check()

	// ---------- employee windows: a member's (from the task window), a desk employee's, one away in a task ----------
	p.openPlate(roomT2)
	p.run(fmt.Sprintf(`[...document.querySelectorAll('#taskwin .mrow')].find(r => r.querySelector('b').textContent === %q).click()`, bobT2))
	waitUntil(t, 10*time.Second, "bob's task session's window shows the kickoff and the task chip", func() bool {
		return p.is(fmt.Sprintf(`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === %q && !!document.querySelector('#chat .flow .it.user .from') && !!document.querySelector('#chat .meta [data-task]')`, bobT2))
	})
	p.shot("chat-member")
	p.run(`document.querySelector('#chat .flow').scrollTop = 0`)
	p.shot("chat-member-top")
	p.check()
	p.run(`document.querySelector('#chat .close').click()`)
	p.openPlate(roomT2)
	p.run(fmt.Sprintf(`[...document.querySelectorAll('#taskwin .mrow')].find(r => r.querySelector('b').textContent === %q).click()`, daveT2))
	waitUntil(t, 10*time.Second, "dave's task session's window shows its question", func() bool {
		return p.is(`!document.getElementById('chat').hidden && document.querySelectorAll('#asks .card, #asks > *').length > 0`)
	})
	time.Sleep(time.Second) // the dock's rise
	p.shot("chat-member-ask")
	p.run(`document.querySelector('#chat .close').click()`)
	clickLabel(t, en, accountID("alice"))
	waitUntil(t, 10*time.Second, "alice's desk window opens", func() bool {
		return p.is(`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === 'alice'`)
	})
	time.Sleep(500 * time.Millisecond)
	p.shot("chat-employee")
	p.run(`document.querySelector('#chat .close').click()`)
	p.run(fmt.Sprintf(`document.querySelector('.emp-label:not(.member)[data-account=%q]').click()`, accountID("dave")))
	waitUntil(t, 10*time.Second, "dave's desk window opens while he is away in T2", func() bool {
		return p.is(`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === 'dave'`)
	})
	p.shot("chat-away-in-task")
	p.check()
	p.run(`document.querySelector('#chat .close').click()`)

	// ---------- the HUD: the search box, the message log, the activity log, the launch dialog ----------
	do(t, en, 10*time.Second, chromedp.SendKeys(chromedp.CSS(`#find`), t2.ID))
	waitUntil(t, 10*time.Second, "searching the task id lists its members and the task", func() bool {
		return p.is(`document.querySelectorAll('#find-list:not([hidden]) .hit').length >= 4`)
	})
	p.shot("search")
	p.run(`(f => { f.value = ''; f.dispatchEvent(new Event('input')); f.blur() })(document.getElementById('find'))`)
	p.run(`document.getElementById('open-msgs').click()`)
	waitUntil(t, 5*time.Second, "the message log opens", func() bool { return p.is(`!document.getElementById('msgs-panel').hidden`) })
	p.shot("messages")
	p.run(`document.querySelector('#msgs-panel .close').click(); document.getElementById('open-log').click()`)
	waitUntil(t, 5*time.Second, "the activity log opens with the tasks' lines", func() bool {
		return p.is(fmt.Sprintf(`!document.getElementById('log').hidden && [...document.querySelectorAll('#log .lines div')].some(d => d.textContent.includes(%q))`, t1.ID))
	})
	p.shot("activity-log")
	p.run(`document.querySelector('#log .close').click()`)
	p.run(`document.getElementById('new-emp').click()`)
	waitUntil(t, 10*time.Second, "the launch dialog opens", func() bool { return p.is(`!document.getElementById('launch').hidden`) })
	p.shot("launch-dialog")
	p.check()
	p.run(`document.querySelector('#launch .cancel').click()`)

	// the modals side by side: the task dialog and window should sit centered like the launch dialog and the chat window
	p.logModals()

	// ---------- pixel sizes and window sizes ----------
	for level := 0; level < 4; level++ {
		do(t, en, 10*time.Second, chromedp.Click(chromedp.CSS(fmt.Sprintf(`#hud .steps button[data-level="%d"]`, level))))
		time.Sleep(1500 * time.Millisecond)
		p.shot(fmt.Sprintf("office-pixels-%d", level+1))
		if level > 0 {
			p.clipRoom(roomT2, fmt.Sprintf("room-T2-pixels-%d", level+1))
		}
	}
	do(t, en, 10*time.Second, chromedp.Click(chromedp.CSS(`#hud .steps button[data-level="0"]`)))
	for _, size := range [][2]int64{{1000, 700}, {1920, 1080}} {
		do(t, en, 10*time.Second, chromedp.EmulateViewport(size[0], size[1]))
		time.Sleep(3 * time.Second) // the figures find their new places
		p.shot(fmt.Sprintf("office-%dx%d", size[0], size[1]))
		p.openPlate(roomT2)
		p.shot(fmt.Sprintf("taskwin-running-%dx%d", size[0], size[1]))
		p.run(`document.querySelector('#taskwin .close').click()`)
		p.dialogOpen(roomFree)
		p.tick("bob")
		p.tick("carol")
		p.run(`document.querySelector('#task .add-adv').click()`)
		p.shot(fmt.Sprintf("task-dialog-%dx%d", size[0], size[1]))
		p.run(`document.querySelector('#task .cancel').click()`)
	}
	do(t, en, 10*time.Second, chromedp.EmulateViewport(1400, 860))

	// ---------- the /approve page, for comparison ----------
	approve, closeApprove := openTab(t, browser, b.URL+"/approve?lang=en")
	do(t, approve, 10*time.Second, chromedp.EmulateViewport(1400, 860))
	waitUntil(t, 10*time.Second, "the approve page shows the pending asks", func() bool {
		return get(t, approve, 5*time.Second, chromedp.Evaluate[bool](`document.body.innerText.includes('npm run build')`))
	})
	(&screens{t: t, ctx: approve, lang: "en"}).shot("approve")
	closeApprove()
	p.done()

	// ---------- the task screens in zh-CN and ja ----------
	for _, lang := range []string{"zh-CN", "ja"} {
		ctx, closeTab := clockTab(t, browser, b.URL+"/?lang="+lang, 0)
		do(t, ctx, 10*time.Second, chromedp.EmulateViewport(1400, 860))
		q := &screens{t: t, ctx: ctx, lang: lang}
		waitUntil(t, 20*time.Second, lang+": the office shows the team and the tasks", func() bool {
			return q.is(`document.querySelectorAll('.emp-label.member').length === 8 && document.querySelectorAll('.room-plate[data-task]').length === 3`)
		})
		q.waitPlates(roomT1, "alert", roomT2, "alert", roomT3, "busy", roomFree, "free")
		time.Sleep(3 * time.Second) // the figures sit down
		q.shot("office")
		q.check()
		q.dialogOpen(roomFree)
		q.run(`document.querySelector('#task [name=desc]').value = '写一个 CHANGELOG'`)
		q.tick("bob")
		q.tick("carol")
		q.run(`document.querySelector('#task .add-adv').click()`)
		q.tick("alice")
		q.tick("frank")
		q.shot("task-dialog")
		q.check()
		q.run(`document.querySelector('#task .cancel').click()`)
		q.openPlate(roomT2)
		q.shot("taskwin-running")
		q.check()
		q.openPlate(roomT3)
		q.shot("taskwin-flat")
		q.openPlate(roomT1)
		q.shot("taskwin-failed")
		q.check()
		q.openPlate(roomT2)
		q.run(fmt.Sprintf(`[...document.querySelectorAll('#taskwin .mrow')].find(r => r.querySelector('b').textContent === %q).click()`, bobT2))
		waitUntil(t, 10*time.Second, lang+": bob's task session's window opens", func() bool {
			return q.is(`!document.getElementById('chat').hidden && !!document.querySelector('#chat .flow .it.user .from')`)
		})
		q.shot("chat-member")
		q.check()
		q.run(`document.querySelector('#chat .close').click()`)
		q.clip("#hud", 12, 2, "hud-pending")
		q.done()
		closeTab()
	}
}

// screens drives one tab of the tour.
type screens struct {
	t    *testing.T
	ctx  context.Context
	lang string
}

// js evaluates expr in the page and gives its value as JSON, strings without their quotes.
func (s *screens) js(expr string) string {
	s.t.Helper()
	out := get(s.t, s.ctx, 10*time.Second, chromedp.Evaluate[string](fmt.Sprintf(`JSON.stringify((() => { return %s })())`, expr)))
	var str string
	if json.Unmarshal([]byte(out), &str) == nil {
		return str
	}
	return out
}

func (s *screens) is(expr string) bool { s.t.Helper(); return s.js(expr) == "true" }

// run runs statements in the page: clicks and typing.
func (s *screens) run(stmt string) {
	s.t.Helper()
	get(s.t, s.ctx, 10*time.Second, chromedp.Evaluate[bool](fmt.Sprintf(`(() => { %s; return true })()`, stmt)))
}

// fetch sends a request from the page and gives the answer's body.
func (s *screens) fetch(method, path, body string) string {
	s.t.Helper()
	return get(s.t, s.ctx, 60*time.Second, chromedp.Evaluate[string](fmt.Sprintf(
		`fetch(%q, { method:%q, headers:{ 'content-type':'application/json' }, body:%q }).then(r => r.text())`, path, method, body), chromedp.EvalAwaitPromise))
}

func (s *screens) path(name string) string {
	dir := filepath.Join("artifacts", "screens")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, s.lang+"-"+name+".png")
}

func (s *screens) save(name string, buf []byte) {
	s.t.Helper()
	path := s.path(name)
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		s.t.Fatal(err)
	}
	abs, _ := filepath.Abs(path)
	s.t.Logf("screenshot: %s", abs)
}

// shot saves the whole window, once a frame or two has been drawn.
func (s *screens) shot(name string) {
	s.t.Helper()
	time.Sleep(250 * time.Millisecond)
	s.save(name, get(s.t, s.ctx, 20*time.Second, chromedp.CaptureScreenshot()))
}

// clip saves a part of the window, scale times as big: the box of sel grown by pad pixels.
func (s *screens) clip(sel string, pad, scale float64, name string) {
	s.t.Helper()
	var r struct{ X, Y, W, H float64 }
	if err := json.Unmarshal([]byte(s.js(fmt.Sprintf(`(b => ({ X:b.left, Y:b.top, W:b.width, H:b.height }))(document.querySelector(%q).getBoundingClientRect())`, sel))), &r); err != nil {
		s.t.Fatal(err)
	}
	s.clipRect(r.X-pad, r.Y-pad, r.W+2*pad, r.H+2*pad, scale, name)
}

func (s *screens) clipRect(x, y, w, h, scale float64, name string) {
	s.t.Helper()
	x, y = math.Max(0, math.Round(x)), math.Max(0, math.Round(y))
	time.Sleep(250 * time.Millisecond)
	buf := get(s.t, s.ctx, 20*time.Second, chromedp.Action[[]byte](func(ctx context.Context, tg *chromedp.Target) ([]byte, error) {
		res, err := cdp.Call(ctx, tg, page.CaptureScreenshot, page.CaptureScreenshotParams{Format: page.CaptureScreenshotFormatPng,
			FromSurface: ptrTo(true), Clip: &page.Viewport{X: x, Y: y, Width: math.Round(w), Height: math.Round(h), Scale: scale}})
		if err != nil {
			return nil, err
		}
		return res.Data, nil
	}))
	s.save(name, buf)
}

// clipRoom saves a small meeting room with its nameplate, twice as big: the plate hangs at the room's door, the room
// above or below it.
func (s *screens) clipRoom(room int, name string) {
	s.t.Helper()
	var r struct{ X, Y, W, H float64 }
	if err := json.Unmarshal([]byte(s.js(fmt.Sprintf(`(b => ({ X:b.left, Y:b.top, W:b.width, H:b.height }))(document.querySelectorAll('.room-plate')[%d].getBoundingClientRect())`, room))), &r); err != nil {
		s.t.Fatal(err)
	}
	s.clipRect(r.X-70, r.Y-150, r.W+140, r.H+300, 2, name)
}

// clipTable saves the table of a room north of its plate, five times as big.
func (s *screens) clipTable(room int, name string) {
	s.t.Helper()
	var r struct{ X, Y float64 }
	if err := json.Unmarshal([]byte(s.js(fmt.Sprintf(`(b => ({ X:b.left, Y:b.top }))(document.querySelectorAll('.room-plate')[%d].getBoundingClientRect())`, room))), &r); err != nil {
		s.t.Fatal(err)
	}
	s.clipRect(r.X-45, r.Y-115, 160, 95, 5, name)
}

func (s *screens) openPlate(room int) {
	s.t.Helper()
	s.run(`document.getElementById('chat').hidden || document.querySelector('#chat .close').click()`)
	s.run(fmt.Sprintf(`document.querySelectorAll('.room-plate')[%d].click()`, room))
	waitUntil(s.t, 10*time.Second, "the task window opens", func() bool { return s.is(`!document.getElementById('taskwin').hidden`) })
}

func (s *screens) dialogOpen(room int) {
	s.t.Helper()
	s.run(fmt.Sprintf(`document.querySelectorAll('.room-plate')[%d].click()`, room))
	waitUntil(s.t, 10*time.Second, "the task dialog opens", func() bool {
		return s.is(`!document.getElementById('task').hidden && document.querySelectorAll('#task .mem').length === 8`)
	})
}

// tick ticks (or unticks) an employee in the task dialog.
func (s *screens) tick(name string) {
	s.t.Helper()
	s.run(fmt.Sprintf(`[...document.querySelectorAll('#task .mem')].find(r => r.querySelector('.nm').textContent === %q).querySelector('input').click()`, name))
}

// waitPlates waits for the nameplates' lights: pairs of a room and its light.
func (s *screens) waitPlates(pairs ...any) {
	s.t.Helper()
	var conds []string
	for i := 0; i < len(pairs); i += 2 {
		conds = append(conds, fmt.Sprintf(`document.querySelectorAll('.room-plate')[%d].dataset.light === %q`, pairs[i], pairs[i+1]))
	}
	waitUntil(s.t, 20*time.Second, "the nameplates' lights", func() bool { return s.is(strings.Join(conds, " && ")) },
		func() string {
			return s.js(`[...document.querySelectorAll('.room-plate')].map(p => p.textContent + ' ' + p.dataset.light).join('\n')`)
		})
}

// check fails the test on a raw catalog key on screen, and logs what spills out of its box in the open modals.
func (s *screens) check() {
	s.t.Helper()
	raw := s.js(`(() => {
		const re = /\b(?:task|room|chat|launch|error|asks|log|office|search|pixels|common|msg|approve)\.[a-z][A-Za-z0-9]*(?:\.[A-Za-z0-9]+)*/g
		const text = document.body.innerText + '\n' + [...document.querySelectorAll('[title], [placeholder]')].map(e => (e.title || '') + ' ' + (e.placeholder || '')).join('\n')
		return [...new Set(text.match(re) || [])].join(', ')
	})()`)
	if raw != "" {
		s.t.Errorf("%s: raw catalog keys on screen: %s", s.lang, raw)
	}
	spill := s.js(`(() => {
		const out = []
		for (const m of document.querySelectorAll('.modal:not([hidden]) .panel')) {
			const p = m.getBoundingClientRect()
			if (p.left < 0 || p.top < 0 || p.right > innerWidth || p.bottom > innerHeight) out.push(m.parentElement.id + ' panel off screen ' + JSON.stringify(p))
			for (const e of m.querySelectorAll('*')) {
				if (!e.offsetParent || getComputedStyle(e).overflow !== 'visible' || ['SELECT', 'TEXTAREA', 'INPUT', 'PRE'].includes(e.tagName)) continue
				if (e.scrollWidth > e.clientWidth + 1 && e.clientWidth > 0) out.push(m.parentElement.id + ' ' + e.tagName.toLowerCase() + '.' + e.className + ' ' + e.scrollWidth + '>' + e.clientWidth + ' "' + e.textContent.slice(0, 40) + '"')
			}
		}
		return out.join('\n')
	})()`)
	if spill != "" {
		s.t.Logf("%s: overflow:\n%s", s.lang, spill)
	}
}

// logModals logs where the modals sit and how big they are, opened one after the other.
func (s *screens) logModals() {
	s.t.Helper()
	s.t.Logf("%s: modals: %s", s.lang, s.js(`(() => {
		const out = {}
		for (const id of ['chat', 'launch', 'task', 'taskwin']) {
			const m = document.getElementById(id), was = m.hidden
			m.hidden = false
			const p = m.querySelector('.panel').getBoundingClientRect(), cs = getComputedStyle(m.querySelector('.panel'))
			out[id] = { left:Math.round(p.left), top:Math.round(p.top), width:Math.round(p.width), height:Math.round(p.height),
				border:cs.borderTopWidth + ' ' + cs.borderTopColor, shadow:cs.boxShadow, bg:cs.backgroundColor, font:cs.fontFamily.slice(0, 40) }
			m.hidden = was
		}
		return out
	})()`))
}

// done fails the test on page errors and on keys a catalog lacks.
func (s *screens) done() {
	s.t.Helper()
	if miss := get(s.t, s.ctx, 5*time.Second, chromedp.Evaluate[[]string](`window.__i18nMissing || []`)); len(miss) > 0 {
		s.t.Errorf("%s: the office asked for keys its catalog lacks: %v", s.lang, miss)
	}
	if errs := get(s.t, s.ctx, 5*time.Second, chromedp.Evaluate[[]string](`window.__errors`)); len(errs) > 0 {
		s.t.Errorf("%s: the page threw: %v", s.lang, errs)
	}
}

func ptrTo[T any](v T) *T { return &v }
