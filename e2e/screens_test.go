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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// TestScreens is a screenshot tour of the office page, for looking at it rather than for checking behavior (no
// tokens: claude is a script that sits there, and the test reports the sessions in as the mod would). It sets up a
// team in every state, three tasks in the small meeting rooms (one of 3 members, seated; one of 5 in a room for 3, two
// of them standing; one failed on a merge conflict) and walks every screen: the office at every pixel size and at a
// small and a big window, the nameplates, the task dialog in each of its states, the task window running, flat and
// failed, the task chat (the members' messages, yours, the mention picker and its narrowed list, a refusal, a long chat
// that scrolls, alone and docked), the employee window of a member (docked to its task window), of a desk employee and
// of one away in a task, the HUD with the search box, the message and activity logs, the launch dialog and the /approve
// page; the task screens and the message log again in zh-CN and ja, with a task started in that language (its kickoff).
// The screenshots go to e2e/artifacts/screens/. It fails on a page error, a key a catalog lacks, or a raw key on screen.
func TestScreens(t *testing.T) {
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
	b.Launcher.Claude = sleeper(t) // claude sits there; the test reports its sessions in

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
	en, _ := clockTab(t, browser, b.URL+"/t/office/?lang=en", 0)
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

	// tasks: made from the page's own fetch (the bridge refuses writes from elsewhere), in its language as the task
	// dialog does, members reported in as the mod would, from their worktrees
	createTaskIn := func(s *screens, room int, title string, leader bool, members []map[string]any) *bridge.Task {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"room": room, "title": title, "leader": leader,
			"permissionMode": "bypassPermissions", "lang": s.lang, "members": members})
		res := s.fetch("POST", "/api/tasks", string(body))
		var task bridge.Task
		if err := json.Unmarshal([]byte(res), &task); err != nil || task.ID == "" {
			t.Fatalf("creating a task failed: %s", res)
		}
		return &task
	}
	createTask := func(room int, title string, leader bool, members []map[string]any) *bridge.Task {
		t.Helper()
		return createTaskIn(p, room, title, leader, members)
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
	if !p.is(`document.querySelector('#task [name=permissionMode]').value === '' && document.querySelector('#task .warn').hidden`) {
		t.Fatalf("a new task should start on Claude Code's own permission mode, without the warning: %q", p.js(`document.querySelector('#task [name=permissionMode]').value`))
	}
	p.shot("task-dialog-empty")
	p.check()
	p.run(`document.querySelector('#task [name=desc]').value = 'Add a CHANGELOG and a Makefile, and document the release steps in the README'`)
	p.tick("bob")
	p.tick("dave")
	p.shot("task-dialog-picked")
	p.check()
	p.bypass("#task")
	p.shot("task-dialog-bypass")
	p.check()
	p.run(`(s => { s.value = ''; s.dispatchEvent(new Event('change', { bubbles:true })) })(document.querySelector('#task [name=permissionMode]'))`)
	p.run(`document.querySelector('#task .add-adv').click()`)
	if !p.is(`(r => r.querySelector('[name=model]').value === '' && r.querySelector('[name=effort]').value === '')(document.querySelector('#task .adv'))`) {
		t.Fatal("a new advisor should start on Claude Code's own model and effort")
	}
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
	p.tick("erin")
	if !p.is(`document.querySelector('#task .count').classList.contains('over') && document.querySelector('#task .go').disabled`) {
		t.Fatalf("6 members should be too many (a task takes 5 at most): %s", p.js(`document.querySelector('#task .count').textContent`))
	}
	p.shot("task-dialog-over")
	p.check()
	p.tick("erin")
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

	// ---------- T2's chat: the members' messages, yours to one of them and to everyone, a mention, a name not in it ----------
	waitUntil(t, 10*time.Second, "T2's chat shows the members' three messages", func() bool {
		return p.is(`document.querySelectorAll('#taskwin .tmsg:not(.me)').length === 3`)
	}, func() string { return p.js(`document.querySelector('#taskwin .tlist').innerText`) })
	say := func(text string) string { // the answer's status, sent as the window's input sends it
		body, _ := json.Marshal(map[string]string{"text": text})
		return get(t, en, 20*time.Second, chromedp.Evaluate[string](fmt.Sprintf(
			`fetch('/api/tasks/%s/say', { method:'POST', headers:{ 'content-type':'application/json' }, body:%q }).then(r => String(r.status))`, t2.ID, body), chromedp.EvalAwaitPromise))
	}
	if st := say("@" + daveT2 + " Yes, cover the Windows build too, briefly"); st == "404" {
		t.Log("POST /api/tasks/{id}/say isn't there yet: the chat shows the members' messages only")
	} else {
		if st != "200" {
			t.Errorf("sending to %s from the task chat answered %s", daveT2, st)
		}
		if st := say("Commit before you tell bob it's done, please"); st != "200" {
			t.Errorf("sending to everyone from the task chat answered %s", st)
		}
		waitUntil(t, 10*time.Second, "T2's chat shows your two messages, one to dave and one to everyone", func() bool {
			return p.is(fmt.Sprintf(`(r => r.length === 2 && r[0].querySelector('.to')?.textContent === '@%s' && !r[1].querySelector('.to'))([...document.querySelectorAll('#taskwin .tmsg.me')])`, daveT2))
		}, func() string { return p.js(`document.querySelector('#taskwin .tlist').innerText`) })
	}
	p.run(`document.querySelector('#taskwin .tchat').scrollIntoView({ block:'end' })`)
	p.shot("taskwin-chat")
	p.check()
	// the mention picker: "@" lists the members, ArrowDown and Enter pick the second, "@r" and Tab add rev, and Enter
	// (the picker shut) sends to both
	box := `#taskwin .tsay textarea`
	draft := func() string { return p.js(`document.querySelector('#taskwin .tsay textarea').value`) }
	// keys the input handles itself go in as keydown events: a key sent through the browser would still type its
	// character after the handler stopped it (Enter: a new line)
	press := func(key string) {
		p.run(fmt.Sprintf(`document.querySelector('#taskwin .tsay textarea').dispatchEvent(new KeyboardEvent('keydown', { key:%q, bubbles:true, cancelable:true }))`, key))
	}
	do(t, en, 10*time.Second, chromedp.SendKeys(chromedp.CSS(box), "@"))
	waitUntil(t, 5*time.Second, "an @ opens the picker with T2's three members", func() bool {
		return p.is(`!document.querySelector('#taskwin .tpick').hidden && document.querySelectorAll('#taskwin .tpick .hit').length === 3`)
	})
	p.shot("taskwin-mention")
	p.check()
	press("ArrowDown")
	press("Enter")
	if got := draft(); got != "@"+daveT2+" " || !p.is(`document.querySelector('#taskwin .tpick').hidden`) {
		t.Errorf("ArrowDown and Enter in the picker should put in %q and close it, the input holds %q", "@"+daveT2+" ", got)
	}
	do(t, en, 10*time.Second, chromedp.SendKeys(chromedp.CSS(box), "@R"))
	if n := p.js(`[...document.querySelectorAll('#taskwin .tpick:not([hidden]) .hit')].map(h => h.dataset.name).join()`); n != revT2 {
		t.Errorf("@R should narrow the picker to %s, it lists %q", revT2, n)
	}
	p.shot("taskwin-mention-filtered")
	press("Tab")
	do(t, en, 10*time.Second, chromedp.SendKeys(chromedp.CSS(box), "please look at the Makefile"))
	if got, want := draft(), "@"+daveT2+" @"+revT2+" please look at the Makefile"; got != want {
		t.Errorf("Tab should put in the narrowed name: the input holds %q, want %q", got, want)
	}
	p.shot("taskwin-chat-mention")
	press("Enter")
	waitUntil(t, 10*time.Second, "the message to dave and rev shows with both names", func() bool {
		return p.is(fmt.Sprintf(`[...document.querySelectorAll('#taskwin .tmsg.me .to')].some(e => e.textContent === '@%s @%s') && document.querySelector('#taskwin .tsay textarea').value === ''`, daveT2, revT2))
	}, func() string {
		return p.js(`document.querySelector('#taskwin .tlist').innerText + ' | ' + document.querySelector('#taskwin .tstatus').textContent`)
	})
	// Esc closes the picker, not the window
	do(t, en, 10*time.Second, chromedp.SendKeys(chromedp.CSS(box), "@"))
	press("Escape")
	if !p.is(`document.querySelector('#taskwin .tpick').hidden && !document.getElementById('taskwin').hidden`) {
		t.Error("Esc in the picker should close the picker and leave the task window open")
	}
	p.run(`(b => { b.value = ''; b.dispatchEvent(new Event('input')) })(document.querySelector('#taskwin .tsay textarea'))`)
	// a name that reads like one of T2's members but isn't: refused, rather than sent to everyone
	nobody := "@nobody-" + t2.ID + " hello"
	p.run(fmt.Sprintf(`(b => { b.value = %q; b.closest('form').requestSubmit() })(document.querySelector('#taskwin .tsay textarea'))`, nobody))
	waitUntil(t, 10*time.Second, "the task chat says why it couldn't send, and keeps the draft", func() bool {
		return p.is(fmt.Sprintf(`document.querySelector('#taskwin .tstatus').textContent.length > 0 && document.querySelector('#taskwin .tsay textarea').value === %q`, nobody))
	}, func() string {
		return p.js(`document.querySelector('#taskwin .tstatus').textContent + ' | ' + document.querySelector('#taskwin .tsay textarea').value`)
	})
	p.shot("taskwin-chat-error")
	p.check()
	p.run(`(b => { b.value = ''; b.dispatchEvent(new Event('input')) })(document.querySelector('#taskwin .tsay textarea'))`)

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

	// ---------- employee windows: a member's (docked to its task window), a desk employee's, one away in a task ----------
	// a member's figure opens its task's window, not its own
	p.run(`document.getElementById('taskwin').hidden || document.querySelector('#taskwin .close').click()`)
	p.run(fmt.Sprintf(`document.querySelector('.emp-label.member[data-session="t-%s"]').click()`, bobT2))
	waitUntil(t, 10*time.Second, "a click on bob-T2's tag opens T2's window and no chat", func() bool {
		return p.is(fmt.Sprintf(`!document.getElementById('taskwin').hidden && document.querySelector('#taskwin h2').textContent.startsWith(%q) && document.getElementById('chat').hidden`, t2.ID+" "))
	})
	p.run(fmt.Sprintf(`[...document.querySelectorAll('#taskwin .mrow')].find(r => r.querySelector('b').textContent === %q).click()`, bobT2))
	waitUntil(t, 10*time.Second, "bob's task session's window docks beside the task window, with the kickoff and the task chip", func() bool {
		return p.is(fmt.Sprintf(`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === %q && !!document.querySelector('#chat .flow .it.user .from') && !!document.querySelector('#chat .meta [data-task]')`, bobT2))
	})
	time.Sleep(300 * time.Millisecond)
	// side by side: the chat's left edge on the task window's right one, the pair centered, bob's row lit
	if !p.is(fmt.Sprintf(`(() => {
		const a = document.querySelector('#taskwin > .panel').getBoundingClientRect(), b = document.querySelector('#chat .panel').getBoundingClientRect()
		const open = document.querySelector('#taskwin .mrow.open b')
		return document.getElementById('chat').classList.contains('docked') && document.getElementById('taskwin').classList.contains('with-chat') &&
			Math.abs(b.left - a.right) <= 3 && Math.abs((a.left + b.right) / 2 - innerWidth / 2) <= 3 && open?.textContent === %q
	})()`, bobT2)) {
		t.Errorf("bob-T2's window should dock to the right of T2's, the pair centered: %s", p.js(`JSON.stringify([document.querySelector('#taskwin > .panel').getBoundingClientRect(), document.querySelector('#chat .panel').getBoundingClientRect(), innerWidth])`))
	}
	p.shot("taskwin-member-docked")
	p.shot("chat-member")
	p.run(`document.querySelector('#chat .flow').scrollTop = 0`)
	p.shot("chat-member-top")
	p.check()
	// its ✕ closes it alone: the task window slides back to the middle
	p.run(`document.querySelector('#chat .close').click()`)
	if !p.is(`!document.getElementById('taskwin').hidden && !document.getElementById('taskwin').classList.contains('with-chat') && !document.querySelector('#taskwin .mrow.open')`) {
		t.Error("closing the docked window should leave the task window open, alone")
	}
	p.run(`document.querySelector('#taskwin .close').click()`)
	// the search box takes a member to its window the same way: dave-T2's, its question over the input
	do(t, en, 10*time.Second, chromedp.SendKeys(chromedp.CSS(`#find`), daveT2), chromedp.SendKeys(chromedp.CSS(`#find`), kb.Enter))
	waitUntil(t, 10*time.Second, "dave's task session's window docks beside T2's and shows its question", func() bool {
		return p.is(`!document.getElementById('chat').hidden && document.getElementById('chat').classList.contains('docked') && !document.getElementById('taskwin').hidden && document.querySelectorAll('#asks .card, #asks > *').length > 0`)
	})
	time.Sleep(time.Second) // the dock's rise
	p.shot("chat-member-ask")
	p.run(`document.querySelector('#taskwin .close').click()`)
	if !p.is(`document.getElementById('chat').hidden && document.getElementById('taskwin').hidden`) {
		t.Error("closing the task window should close the member's window docked to it")
	}
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
	if !p.is(`(f => f.elements.model.value === '' && f.elements.effort.value === '' && f.elements.permissionMode.value === '' && f.querySelector('.warn').hidden)(document.querySelector('#launch form'))`) {
		t.Fatalf("a new employee should start on Claude Code's own settings, without the warning: %s", p.js(`(f => [f.elements.model.value, f.elements.effort.value, f.elements.permissionMode.value].join())(document.querySelector('#launch form'))`))
	}
	if !p.is(`!document.querySelector('#launch .adv').open && document.querySelector('#launch .digest').textContent === "own folder · Claude Code's own settings"`) {
		t.Fatalf("the dialog should open folded, its line saying what the start will use: %s", p.js(`document.querySelector('#launch .digest').textContent`))
	}
	p.shot("launch-dialog")
	p.check()
	p.run(`document.querySelector('#launch .adv > summary').click()`)
	p.shot("launch-dialog-advanced")
	p.check()
	p.bypass("#launch")
	p.shot("launch-dialog-bypass")
	p.check()
	p.run(`document.querySelector('#launch .cancel').click()`)

	// ---------- a long team chat: T3's members talk it over, the list scrolls (after the logs, which it would crowd) ----------
	t3names := []string{"alice", "gina", "ana", "ben", "cyd"}
	for i := 0; i < 32; i++ {
		from, to := t3names[i%5], t3names[(i+2)%5]
		text := fmt.Sprintf("Step %d of the port: %s takes this one and reports back when it's on the branch.", i+1, to)
		if i%9 == 4 {
			text += "\n\n```\nnpm run build && npm test -- src/client.ts\n```"
		}
		message(from, from+"-"+t3.ID, to, to+"-"+t3.ID, text)
	}
	p.openPlate(roomT3)
	waitUntil(t, 10*time.Second, "T3's chat lists its 33 messages, scrolled to the end", func() bool {
		return p.is(`document.querySelectorAll('#taskwin .tmsg').length >= 33 && (l => l.scrollHeight > l.clientHeight + 100 && l.scrollTop + l.clientHeight >= l.scrollHeight - 2)(document.querySelector('#taskwin .tlist'))`)
	}, func() string { return p.js(`document.querySelectorAll('#taskwin .tmsg').length`) })
	p.shot("taskwin-chat-long")
	p.check()
	p.run(`document.querySelector('#taskwin .tlist').scrollTop = 0`)
	p.shot("taskwin-chat-long-top")
	p.run(fmt.Sprintf(`[...document.querySelectorAll('#taskwin .mrow')].find(r => r.querySelector('b').textContent === %q).click()`, "alice-"+t3.ID))
	waitUntil(t, 10*time.Second, "alice-T3's window docks beside the long chat", func() bool {
		return p.is(`!document.getElementById('chat').hidden && document.getElementById('chat').classList.contains('docked')`)
	})
	time.Sleep(300 * time.Millisecond)
	p.shot("taskwin-chat-long-docked")
	p.check()
	p.run(`document.querySelector('#taskwin .close').click()`)

	// the modals side by side: the task dialog and window should sit centered like the launch dialog and the chat window
	p.logModals()

	// ---------- pixel sizes and window sizes ----------
	for level := 0; level < 4; level++ {
		do(t, en, 10*time.Second, chromedp.Click(chromedp.CSS(`#open-settings`)), chromedp.Click(chromedp.CSS(fmt.Sprintf(`.pixel-steps button[data-level="%d"]`, level))),
			chromedp.Click(chromedp.CSS(`#settings .close`)))
		time.Sleep(1500 * time.Millisecond)
		p.shot(fmt.Sprintf("office-pixels-%d", level+1))
		if level > 0 {
			p.clipRoom(roomT2, fmt.Sprintf("room-T2-pixels-%d", level+1))
		}
	}
	do(t, en, 10*time.Second, chromedp.Click(chromedp.CSS(`#open-settings`)), chromedp.Click(chromedp.CSS(`.pixel-steps button[data-level="0"]`)),
		chromedp.Click(chromedp.CSS(`#settings .close`)))
	for _, size := range [][2]int64{{800, 600}, {1000, 700}, {1920, 1080}} {
		do(t, en, 10*time.Second, chromedp.EmulateViewport(size[0], size[1]))
		time.Sleep(3 * time.Second) // the figures find their new places
		p.shot(fmt.Sprintf("office-%dx%d", size[0], size[1]))
		p.openPlate(roomT2)
		p.shot(fmt.Sprintf("taskwin-running-%dx%d", size[0], size[1]))
		// a member's window: docked where there is room for both, over the task window where there isn't
		p.run(fmt.Sprintf(`[...document.querySelectorAll('#taskwin .mrow')].find(r => r.querySelector('b').textContent === %q).click()`, bobT2))
		docked := size[0] >= 1250
		waitUntil(t, 10*time.Second, fmt.Sprintf("bob-T2's window opens (docked: %v)", docked), func() bool {
			return p.is(fmt.Sprintf(`!document.getElementById('chat').hidden && document.getElementById('chat').classList.contains('docked') === %v && document.getElementById('chat').classList.contains('stacked') === %v`, docked, !docked))
		})
		p.shot(fmt.Sprintf("taskwin-member-%dx%d", size[0], size[1]))
		p.run(`document.querySelector('#chat .close').click()`)
		p.run(`document.querySelector('#taskwin .close').click()`)
		p.dialogOpen(roomFree)
		p.tick("bob")
		p.tick("carol")
		p.run(`document.querySelector('#task .add-adv').click()`)
		p.shot(fmt.Sprintf("task-dialog-%dx%d", size[0], size[1]))
		p.run(`document.querySelector('#task .cancel').click()`)
	}
	do(t, en, 10*time.Second, chromedp.EmulateViewport(1400, 860))

	// langTask starts a task in the tab's language, in the free room: its kickoff is in that language too. Two advisors
	// on the app repository (the others were made for T3); then one goes offline, and a message naming both reaches the
	// other only, which the team chat says. The task ends again after.
	langTask := func(q *screens) {
		t.Helper()
		lang := q.lang
		advisors := map[string][2]string{"en": {"kai", "noa"}, "zh-CN": {"lin", "mo"}, "ja": {"ken", "mio"}}[lang]
		tk := createTaskIn(q, roomFree, map[string]string{"en": "Write a CHANGELOG", "zh-CN": "写一个 CHANGELOG", "ja": "CHANGELOG を書く"}[lang], false, []map[string]any{
			{"advisor": map[string]string{"name": advisors[0], "dir": dir("app")}, "model": "sonnet", "effort": "high"},
			{"advisor": map[string]string{"name": advisors[1], "dir": dir("app")}, "model": "haiku", "effort": "low"}})
		accounts()
		reportIn(tk, advisors[0])
		reportIn(tk, advisors[1])
		q.openPlate(roomFree)
		lead := advisors[0] + "-" + tk.ID
		waitUntil(t, 10*time.Second, lang+": the new task's window shows its two members online", func() bool {
			return q.is(`document.querySelectorAll('#taskwin .mrow').length === 2 && !document.querySelector('#taskwin .mrow.wait')`)
		})
		q.run(fmt.Sprintf(`[...document.querySelectorAll('#taskwin .mrow')].find(r => r.querySelector('b').textContent === %q).click()`, lead))
		waitUntil(t, 10*time.Second, lang+": "+lead+"'s window shows its kickoff", func() bool {
			return q.is(`!document.getElementById('chat').hidden && !!document.querySelector('#chat .flow .it.user.kickoff')`)
		})
		q.run(`document.querySelector('#chat .flow .it.user.kickoff details').open = true`)
		q.shot("chat-kickoff")
		q.check()
		// the other one goes offline: a message naming both gets to the lead only, and the team chat says so
		other := advisors[1] + "-" + tk.ID
		event(advisors[1], "t-"+other, map[string]any{"kind": "session.end", "task": tk.ID})
		q.run(`document.querySelector('#chat .close').click()`)
		q.run(fmt.Sprintf(`(b => { b.value = %q; b.closest('form').requestSubmit() })(document.querySelector('#taskwin .tsay textarea'))`, "@"+lead+" @"+other+" "+map[string]string{"en": "tell me when it is written", "zh-CN": "写好了告诉我", "ja": "書けたら教えて"}[lang]))
		waitUntil(t, 10*time.Second, lang+": the team chat strikes "+other+" through and says it didn't get the message", func() bool {
			return q.is(fmt.Sprintf(`document.querySelector('#taskwin .tstatus').classList.contains('note') && [...document.querySelectorAll('#taskwin .tmsg.me .to s')].some(e => e.textContent === '@%s')`, other))
		}, func() string {
			return q.js(`document.querySelector('#taskwin .tstatus').textContent + ' | ' + document.querySelector('#taskwin .tlist').innerHTML`)
		})
		q.shot("taskwin-chat-missed")
		q.check()
		q.run(`document.querySelector('#taskwin .close').click()`)
		q.fetch("POST", "/api/tasks/"+tk.ID+"/end", "{}")
		waitUntil(t, 60*time.Second, lang+": the new task ends and leaves the free room", func() bool {
			return !slices.ContainsFunc(b.Hub.Tasks(), func(k *bridge.Task) bool { return k.ID == tk.ID }) && q.is(fmt.Sprintf(`document.querySelectorAll('.room-plate')[%d].dataset.light === 'free'`, roomFree))
		}, func() string { data, _ := json.Marshal(b.Hub.Tasks()); return string(data) })
	}
	langTask(p)

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
		ctx, closeTab := clockTab(t, browser, b.URL+"/t/office/?lang="+lang, 0)
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
		q.bypass("#task")
		q.shot("task-dialog-bypass")
		q.check()
		q.run(`document.querySelector('#task .panel').scrollTop = 0`)
		q.shot("task-dialog")
		q.run(`document.querySelector('#task .cancel').click()`)
		q.run(`document.getElementById('new-emp').click()`)
		waitUntil(t, 10*time.Second, lang+": the launch dialog opens", func() bool { return q.is(`!document.getElementById('launch').hidden`) })
		q.bypass("#launch")
		q.shot("launch-dialog-bypass")
		q.check()
		q.run(`document.querySelector('#launch .cancel').click()`)
		q.openPlate(roomT2)
		q.shot("taskwin-running")
		q.check()
		// the chat with the mention picker open, in this language
		q.run(`(b => { b.focus(); b.value = '@'; b.setSelectionRange(1, 1); b.dispatchEvent(new Event('input')) })(document.querySelector('#taskwin .tsay textarea'))`)
		waitUntil(t, 5*time.Second, lang+": the mention picker lists T2's members", func() bool {
			return q.is(`document.querySelectorAll('#taskwin .tpick:not([hidden]) .hit').length === 3`)
		})
		q.shot("taskwin-chat")
		q.run(`(b => { b.value = ''; b.dispatchEvent(new Event('input')); b.blur() })(document.querySelector('#taskwin .tsay textarea'))`)
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
		q.run(`document.querySelector('#taskwin .close').click()`)
		langTask(q)
		q.run(`document.getElementById('open-msgs').click()`)
		waitUntil(t, 5*time.Second, lang+": the message log opens", func() bool { return q.is(`!document.getElementById('msgs-panel').hidden`) })
		q.shot("messages")
		q.check()
		q.run(`document.querySelector('#msgs-panel .close').click()`)
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

// bypass picks bypassPermissions in a dialog (#launch, its Advanced settings unfolded, or #task) and checks that the
// warning shows under the select, in sight (not under the task dialog's buttons).
func (s *screens) bypass(dialog string) {
	s.t.Helper()
	if dialog == "#launch" { // (the select is in Advanced settings)
		s.run(`document.querySelector('#launch .adv').open = true`)
	}
	s.run(fmt.Sprintf(`(s => { s.value = 'bypassPermissions'; s.dispatchEvent(new Event('change', { bubbles:true })) })(document.querySelector('%s [name=permissionMode]'))`, dialog))
	if !s.is(fmt.Sprintf(`(w => { if (w.hidden || w.textContent.length < 20) return false; const r = w.getBoundingClientRect(); return w.contains(document.elementFromPoint(r.left + 4, r.top + r.height / 2)) })(document.querySelector('%s .warn'))`, dialog)) {
		s.t.Fatalf("%s: bypassPermissions should show the warning under the select", dialog)
	}
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
		const re = /\b(?:task|room|chat|launch|error|asks|log|office|search|pixels|common|msg|approve|msgr|settings|errlog)\.[a-z][A-Za-z0-9]*(?:\.[A-Za-z0-9]+)*/g
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
