//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// TestScreens is a screenshot tour of Coworkers, for looking at it rather than for checking behavior (no tokens:
// claude is a script that sits there, and the test reports the sessions in as the mod would). It sets up a team in
// every state, three groups (one of 3 members with a lead and an advisor; one of 5, flat; one failed on a merge
// conflict) and walks every screen: the sidebar, the task dialog in each of its states, the group window running, flat
// and failed, the team chat (the members' messages, yours, the mention picker and its narrowed list, a refusal, a long
// chat that scrolls), a member's conversation from the group, a desk coworker's and one away in a group, the search
// box, the launch dialog, a few window sizes and the /approve page; the group screens again in zh-CN and ja, with a
// group started in that language (its kickoff). The screenshots go to e2e/artifacts/screens/. It fails on a page
// error, a key a catalog lacks, or a raw key on screen.
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
		{"i": 1, "kind": "text", "text": "I'll run the tests first:\n\n```sh\n$ make test -j4 $CI 2>&1 | tee out.log  # the whole suite\n```\n\nThen I'll look at **each failure** in turn, starting from the router:\n\n" +
			"```go\n// the router's handler\nfunc route(p string) (int, error) {\n\tif len(p) > 64 {\n\t\treturn 0, nil\n\t}\n\treturn strconv.Atoi(p)\n}\n```"}}})
	event("alice", "s-alice", map[string]any{"kind": "chat.stop", "turnId": "a1", "step": 0})
	event("alice", "s-alice", map[string]any{"kind": "tool.start", "tool": "Read", "summary": "src/router.js"})
	event("alice", "s-alice", map[string]any{"kind": "tool.end", "tool": "Read", "summary": "src/router.js"})
	event("alice", "s-alice", map[string]any{"kind": "tool.start", "tool": "Bash", "summary": "make test"})
	event("gina", "s-gina", map[string]any{"kind": "tool.start", "tool": "Grep", "summary": "TODO"})
	b.post(t, tokens["carol"], "/api/ask", map[string]any{"session": "s-carol", "kind": "permission", "tool": "Bash", "input": map[string]any{"command": "rm -rf build && npm run build"}})
	event("dave", "s-dave", map[string]any{"kind": "session.end"}) // off duty, and later away in a task
	event("erin", "s-erin", map[string]any{"kind": "session.end"})

	browser := newBrowser(t)
	en, _ := clockTab(t, browser, b.URL+"/t/messenger/?lang=en", 0)
	do(t, en, 10*time.Second, chromedp.EmulateViewport(1400, 860)) // the window's size less the browser's own bars
	p := &screens{t: t, ctx: en, lang: "en"}
	waitUntil(t, 20*time.Second, "the sidebar lists the eight coworkers, carol waiting", func() bool {
		return p.is(`document.querySelectorAll('#people .row').length === 8 && !!document.querySelector('#people .row .face[data-state=waiting]')`)
	})

	// tasks: made from the page's own fetch (the bridge refuses writes from elsewhere), in its language as the task
	// dialog does, with git (T1 fails on a merge), members reported in as the mod would, from their worktrees
	createTaskIn := func(s *screens, title string, leader bool, members []map[string]any) *bridge.Task {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"title": title, "leader": leader, "git": true, "permissionMode": "bypassPermissions", "lang": s.lang, "members": members})
		res := s.fetch("POST", "/api/tasks", string(body))
		var task bridge.Task
		if err := json.Unmarshal([]byte(res), &task); err != nil || task.ID == "" {
			t.Fatalf("creating a task failed: %s", res)
		}
		return &task
	}
	createTask := func(title string, leader bool, members []map[string]any) *bridge.Task {
		t.Helper()
		return createTaskIn(p, title, leader, members)
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
	t1 := createTask("Tidy up the notes", true, []map[string]any{
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
	p.dialogOpen()
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
	// past the cap: 21 members (every coworker and enough advisors)
	p.run(`for (let i = 0; i < 15; i++) document.querySelector('#task .add-adv').click(); document.activeElement?.blur()`)
	if !p.is(`document.querySelector('#task .count').classList.contains('over') && document.querySelector('#task .go').disabled`) {
		t.Fatalf("21 members should be too many (a task takes 20 at most): %s", p.js(`document.querySelector('#task .count').textContent`))
	}
	p.shot("task-dialog-over")
	p.check()
	p.run(`[...document.querySelectorAll('#task .adv')].slice(1).forEach(r => r.querySelector('.rm').click())`)
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

	// ---------- T3: five members, flat (nobody leads) ----------
	t3 := createTask("Port the API client to TypeScript and keep the old one working until every caller has moved", false, []map[string]any{
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

	groupsReady := func(q *screens) {
		t.Helper()
		waitUntil(t, 20*time.Second, q.lang+": the sidebar lists the three groups, T2 waiting for you", func() bool {
			return q.is(fmt.Sprintf(`document.querySelectorAll('#groups .row').length === 3 && !!document.querySelector('#groups .row[data-group=%q] .sub.hot')`, t2.ID))
		}, func() string { return q.js(`document.getElementById('groups').innerText`) })
	}
	groupsReady(p)

	// ---------- the sidebar ----------
	p.shot("sidebar")
	p.check()

	// ---------- the group windows: running with a lead and an advisor, flat, failed ----------
	p.openGroup(t2.ID)
	waitUntil(t, 10*time.Second, "T2's window shows its three members online", func() bool {
		return p.is(`document.querySelectorAll('#taskwin .mrow').length === 3 && !document.querySelector('#taskwin .mrow.wait')`)
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
	if st := say("@" + daveT2 + " Yes, cover the Windows build too, briefly"); st != "200" {
		t.Errorf("sending to %s from the task chat answered %s", daveT2, st)
	}
	if st := say("Commit before you tell bob it's done, please"); st != "200" {
		t.Errorf("sending to everyone from the task chat answered %s", st)
	}
	waitUntil(t, 10*time.Second, "T2's chat shows your two messages, one to dave and one to everyone", func() bool {
		return p.is(`(r => r.length === 2 && r[0].querySelector('.to')?.textContent === '@dave' && !r[1].querySelector('.to'))([...document.querySelectorAll('#taskwin .tmsg.me')])`)
	}, func() string { return p.js(`document.querySelector('#taskwin .tlist').innerText`) })
	p.run(`document.querySelector('#taskwin .tchat').scrollIntoView({ block:'end' })`)
	p.shot("taskwin-chat")
	p.check()
	// the mention picker: "@" lists the members, ArrowDown and Enter pick the second, "@r" and Tab add rev, and Enter
	// (the picker shut) sends to both; Coworkers names the members short (dave, not dave-T2)
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
	if got := draft(); got != "@dave " || !p.is(`document.querySelector('#taskwin .tpick').hidden`) {
		t.Errorf("ArrowDown and Enter in the picker should put in %q and close it, the input holds %q", "@dave ", got)
	}
	do(t, en, 10*time.Second, chromedp.SendKeys(chromedp.CSS(box), "@R"))
	if n := p.js(`[...document.querySelectorAll('#taskwin .tpick:not([hidden]) .hit')].map(h => h.dataset.name).join()`); n != revT2 {
		t.Errorf("@R should narrow the picker to %s, it lists %q", revT2, n)
	}
	p.shot("taskwin-mention-filtered")
	press("Tab")
	do(t, en, 10*time.Second, chromedp.SendKeys(chromedp.CSS(box), "please look at the Makefile"))
	if got, want := draft(), "@dave @rev please look at the Makefile"; got != want {
		t.Errorf("Tab should put in the narrowed name: the input holds %q, want %q", got, want)
	}
	p.shot("taskwin-chat-mention")
	press("Enter")
	waitUntil(t, 10*time.Second, "the message to dave and rev shows with both names", func() bool {
		return p.is(`[...document.querySelectorAll('#taskwin .tmsg.me .to')].some(e => e.textContent === '@dave @rev') && document.querySelector('#taskwin .tsay textarea').value === ''`)
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

	p.openGroup(t3.ID)
	waitUntil(t, 10*time.Second, "T3's window shows five members and no lead", func() bool {
		return p.is(`document.querySelectorAll('#taskwin .mrow').length === 5 && !document.querySelector('#taskwin .mode').hidden`)
	})
	p.shot("taskwin-flat")
	p.openGroup(t1.ID)
	waitUntil(t, 10*time.Second, "T1's window says why it failed and offers to dismiss it", func() bool {
		return p.is(`!document.querySelector('#taskwin .err').hidden && !!document.querySelector('#taskwin .dismiss')`)
	})
	p.shot("taskwin-failed")
	p.check()

	// ---------- conversations: a member's from its group, a desk coworker's, one away in a group ----------
	p.openGroup(t2.ID)
	p.openMember(bobT2)
	waitUntil(t, 10*time.Second, "bob's task session's conversation shows the kickoff, the task chip and the way back", func() bool {
		return p.is(`!!document.querySelector('#chat .flow .it.user .from') && !!document.querySelector('#chat .meta [data-task]') && !!document.querySelector('#profile .link[data-group]')`)
	})
	time.Sleep(300 * time.Millisecond)
	p.shot("chat-member")
	p.run(`document.querySelector('#chat .flow').scrollTop = 0`)
	p.shot("chat-member-top")
	p.check()
	// the way back leads to the group; dave's conversation from it, his question in its line
	p.run(`document.querySelector('#profile .link[data-group]').click()`)
	waitUntil(t, 10*time.Second, "back to T2's window", func() bool {
		return p.is(`!document.getElementById('taskwin').hidden && document.getElementById('chat').hidden`)
	})
	p.openMember(daveT2)
	waitUntil(t, 10*time.Second, "dave's task session's conversation shows his question", func() bool {
		return p.is(`document.querySelectorAll('#chat .flow .it.ask.carded .card').length === 1`)
	})
	time.Sleep(500 * time.Millisecond)
	p.shot("chat-member-ask")
	clickRow(t, en, accountID("alice"))
	waitUntil(t, 10*time.Second, "alice's desk conversation opens", func() bool {
		return p.is(`document.querySelector('#chat h2').textContent === 'alice'`)
	})
	time.Sleep(500 * time.Millisecond)
	p.shot("chat-employee")
	clickRow(t, en, accountID("dave"))
	waitUntil(t, 10*time.Second, "dave's desk conversation opens while he is away in T2", func() bool {
		return p.is(`document.querySelector('#chat h2').textContent === 'dave'`)
	})
	p.shot("chat-away-in-task")
	p.check()
	p.run(`document.querySelector('#chat .close').click()`)

	// ---------- the search box, the launch dialog ----------
	p.run(fmt.Sprintf(`(f => { f.value = %q; f.dispatchEvent(new Event('input')) })(document.getElementById('find'))`, t2.ID))
	waitUntil(t, 10*time.Second, "searching the task id leaves its group", func() bool {
		return p.is(fmt.Sprintf(`document.querySelectorAll('#groups .row').length === 1 && document.querySelector('#groups .row').dataset.group === %q`, t2.ID))
	})
	p.shot("search")
	p.run(`(f => { f.value = ''; f.dispatchEvent(new Event('input')); f.blur() })(document.getElementById('find'))`)
	p.run(`document.getElementById('new-emp').click()`)
	waitUntil(t, 10*time.Second, "the launch dialog opens", func() bool { return p.is(`!document.getElementById('launch').hidden`) })
	if !p.is(`(f => f.elements.model.value === '' && f.elements.effort.value === '' && f.elements.permissionMode.value === '' && f.querySelector('.warn').hidden)(document.querySelector('#launch form'))`) {
		t.Fatalf("a new employee should start on Claude Code's own settings, without the warning: %s", p.js(`(f => [f.elements.model.value, f.elements.effort.value, f.elements.permissionMode.value].join())(document.querySelector('#launch form'))`))
	}
	if !p.is(`!document.querySelector('#launch .adv').open && document.querySelector('#launch .digest').textContent === "own folder · Claude Code's own settings"`) {
		t.Fatalf("the dialog should open folded, its line saying what the start will use: %s", p.js(`document.querySelector('#launch .digest').textContent`))
	}
	if !p.is(`(r => !!r && !r.hidden && r.querySelectorAll('.look-pick input:checked').length === 2)(document.querySelector('#launch .look-row'))`) {
		t.Errorf("a new employee's dialog should offer a face, with a shape and a color picked: %s", p.js(`document.querySelector('#launch .look-row')?.outerHTML.slice(0, 200)`))
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

	// ---------- a long team chat: T3's members talk it over, the list scrolls ----------
	t3names := []string{"alice", "gina", "ana", "ben", "cyd"}
	for i := 0; i < 32; i++ {
		from, to := t3names[i%5], t3names[(i+2)%5]
		text := fmt.Sprintf("Step %d of the port: %s takes this one and reports back when it's on the branch.", i+1, to)
		if i%9 == 4 {
			text += "\n\n```\nnpm run build && npm test -- src/client.ts\n```"
		}
		message(from, from+"-"+t3.ID, to, to+"-"+t3.ID, text)
	}
	p.openGroup(t3.ID)
	waitUntil(t, 10*time.Second, "T3's chat lists its 33 messages, scrolled to the end", func() bool {
		return p.is(`document.querySelectorAll('#taskwin .tmsg').length >= 33 && (l => l.scrollHeight > l.clientHeight + 100 && l.scrollTop + l.clientHeight >= l.scrollHeight - 2)(document.querySelector('#taskwin .tlist'))`)
	}, func() string { return p.js(`document.querySelectorAll('#taskwin .tmsg').length`) })
	p.shot("taskwin-chat-long")
	p.check()
	p.run(`document.querySelector('#taskwin .tlist').scrollTop = 0`)
	p.shot("taskwin-chat-long-top")
	p.openMember("alice-" + t3.ID)
	time.Sleep(300 * time.Millisecond)
	p.shot("chat-member-flat")
	p.check()
	p.run(`document.querySelector('#chat .close').click()`)

	// the modals: the launch and task dialogs should sit centered
	p.logModals()

	// ---------- window sizes ----------
	for _, size := range [][2]int64{{800, 600}, {1000, 700}, {1920, 1080}} {
		do(t, en, 10*time.Second, chromedp.EmulateViewport(size[0], size[1]))
		time.Sleep(500 * time.Millisecond)
		p.openGroup(t2.ID)
		p.shot(fmt.Sprintf("taskwin-running-%dx%d", size[0], size[1]))
		p.openMember(bobT2)
		time.Sleep(300 * time.Millisecond)
		p.shot(fmt.Sprintf("chat-member-%dx%d", size[0], size[1]))
		p.run(`document.querySelector('#chat .close').click()`)
		p.dialogOpen()
		p.tick("bob")
		p.tick("carol")
		p.run(`document.querySelector('#task .add-adv').click()`)
		p.shot(fmt.Sprintf("task-dialog-%dx%d", size[0], size[1]))
		p.run(`document.querySelector('#task .cancel').click()`)
	}
	do(t, en, 10*time.Second, chromedp.EmulateViewport(1400, 860))

	// langTask starts a task in the tab's language: its kickoff is in that language too. Two advisors on the app
	// repository (the others were made for T3); then one goes offline, and a message naming both reaches the other
	// only, which the team chat says. The task ends again after.
	langTask := func(q *screens) {
		t.Helper()
		lang := q.lang
		advisors := map[string][2]string{"en": {"kai", "noa"}, "zh-CN": {"lin", "mo"}, "ja": {"ken", "mio"}}[lang]
		tk := createTaskIn(q, map[string]string{"en": "Write a CHANGELOG", "zh-CN": "写一个 CHANGELOG", "ja": "CHANGELOG を書く"}[lang], false, []map[string]any{
			{"advisor": map[string]string{"name": advisors[0], "dir": dir("app")}, "model": "sonnet", "effort": "high"},
			{"advisor": map[string]string{"name": advisors[1], "dir": dir("app")}, "model": "haiku", "effort": "low"}})
		accounts()
		reportIn(tk, advisors[0])
		reportIn(tk, advisors[1])
		q.openGroup(tk.ID)
		lead := advisors[0] + "-" + tk.ID
		waitUntil(t, 10*time.Second, lang+": the new task's window shows its two members online", func() bool {
			return q.is(`document.querySelectorAll('#taskwin .mrow').length === 2 && !document.querySelector('#taskwin .mrow.wait')`)
		})
		q.openMember(lead)
		waitUntil(t, 10*time.Second, lang+": "+lead+"'s conversation shows its kickoff", func() bool {
			return q.is(`!!document.querySelector('#chat .flow .it.user.kickoff')`)
		})
		q.run(`document.querySelector('#chat .flow .it.user.kickoff details').open = true`)
		q.shot("chat-kickoff")
		q.check()
		// the other one goes offline: a message naming both gets to the lead only, and the team chat says so
		other := advisors[1] + "-" + tk.ID
		event(advisors[1], "t-"+other, map[string]any{"kind": "session.end", "task": tk.ID})
		waitUntil(t, 10*time.Second, lang+": the profile offers the way back to the group", func() bool {
			return q.is(`!!document.querySelector('#profile .link[data-group]')`)
		}, func() string {
			return q.js(`JSON.stringify({ h2: document.querySelector('#chat h2').textContent, hidden: document.getElementById('profile').hidden, profile: document.getElementById('profile').innerText.slice(0, 300) })`)
		})
		q.run(`document.querySelector('#profile .link[data-group]').click()`)
		waitUntil(t, 10*time.Second, lang+": back to the group", func() bool { return q.is(`!document.getElementById('taskwin').hidden`) })
		q.run(fmt.Sprintf(`(b => { b.value = %q; b.closest('form').requestSubmit() })(document.querySelector('#taskwin .tsay textarea'))`, "@"+lead+" @"+other+" "+map[string]string{"en": "tell me when it is written", "zh-CN": "写好了告诉我", "ja": "書けたら教えて"}[lang]))
		waitUntil(t, 10*time.Second, lang+": the team chat strikes "+other+" through and says it didn't get the message", func() bool {
			return q.is(fmt.Sprintf(`document.querySelector('#taskwin .tstatus').classList.contains('note') && [...document.querySelectorAll('#taskwin .tmsg.me .to s')].some(e => e.textContent === '@%s')`, advisors[1]))
		}, func() string {
			return q.js(`document.querySelector('#taskwin .tstatus').textContent + ' | ' + document.querySelector('#taskwin .tlist').innerHTML`)
		})
		q.shot("taskwin-chat-missed")
		q.check()
		q.run(`document.querySelector('#taskwin .close').click()`)
		q.fetch("POST", "/api/tasks/"+tk.ID+"/end", "{}")
		waitUntil(t, 60*time.Second, lang+": the new task ends and leaves the sidebar", func() bool {
			return !slices.ContainsFunc(b.Hub.Tasks(), func(k *bridge.Task) bool { return k.ID == tk.ID }) && q.is(fmt.Sprintf(`!document.querySelector('#groups .row[data-group=%q]')`, tk.ID))
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

	// ---------- the group screens in zh-CN and ja ----------
	for _, lang := range []string{"zh-CN", "ja"} {
		ctx, closeTab := clockTab(t, browser, b.URL+"/t/messenger/?lang="+lang, 0)
		do(t, ctx, 10*time.Second, chromedp.EmulateViewport(1400, 860))
		q := &screens{t: t, ctx: ctx, lang: lang}
		waitUntil(t, 20*time.Second, lang+": the sidebar lists the team", func() bool {
			return q.is(`document.querySelectorAll('#people .row').length === 8`)
		})
		groupsReady(q)
		q.shot("sidebar")
		q.check()
		q.dialogOpen()
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
		q.openGroup(t2.ID)
		q.shot("taskwin-running")
		q.check()
		// the chat with the mention picker open, in this language
		q.run(`(b => { b.focus(); b.value = '@'; b.setSelectionRange(1, 1); b.dispatchEvent(new Event('input')) })(document.querySelector('#taskwin .tsay textarea'))`)
		waitUntil(t, 5*time.Second, lang+": the mention picker lists T2's members", func() bool {
			return q.is(`document.querySelectorAll('#taskwin .tpick:not([hidden]) .hit').length === 3`)
		})
		q.shot("taskwin-chat")
		q.run(`(b => { b.value = ''; b.dispatchEvent(new Event('input')); b.blur() })(document.querySelector('#taskwin .tsay textarea'))`)
		q.openGroup(t3.ID)
		q.shot("taskwin-flat")
		q.openGroup(t1.ID)
		q.shot("taskwin-failed")
		q.check()
		q.openGroup(t2.ID)
		q.openMember(bobT2)
		waitUntil(t, 10*time.Second, lang+": bob's task session's conversation opens", func() bool {
			return q.is(`!!document.querySelector('#chat .flow .it.user .from')`)
		})
		q.shot("chat-member")
		q.check()
		q.run(`document.querySelector('#chat .close').click()`)
		langTask(q)
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

// openGroup opens a group's window from its row in the sidebar (there once the snapshot with the task came down).
func (s *screens) openGroup(id string) {
	s.t.Helper()
	waitUntil(s.t, 10*time.Second, "the sidebar lists "+id, func() bool { return s.is(fmt.Sprintf(`!!document.querySelector('#groups .row[data-group=%q]')`, id)) })
	s.run(fmt.Sprintf(`document.querySelector('#groups .row[data-group=%q]').click()`, id))
	waitUntil(s.t, 10*time.Second, "the group window opens", func() bool {
		// the head reads the group's name, else its id (then beside the name)
		return s.is(fmt.Sprintf(`!document.getElementById('taskwin').hidden && (document.querySelector('#taskwin .top h2').textContent === %q || document.querySelector('#taskwin .tid').textContent === %q)`, id, id))
	})
}

// openMember opens a member's conversation from its row in the open group window.
func (s *screens) openMember(name string) {
	s.t.Helper()
	short := regexp.MustCompile(`-T\d+$`).ReplaceAllString(name, "") // shown short: bob, not bob-T2
	waitUntil(s.t, 10*time.Second, "the group window lists "+name, func() bool {
		return s.is(fmt.Sprintf(`[...document.querySelectorAll('#taskwin .mrow')].some(r => r.querySelector('b').textContent === %q)`, short))
	})
	s.run(fmt.Sprintf(`[...document.querySelectorAll('#taskwin .mrow')].find(r => r.querySelector('b').textContent === %q).click()`, short))
	waitUntil(s.t, 10*time.Second, name+"'s conversation opens", func() bool {
		return s.is(fmt.Sprintf(`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === %q`, short))
	})
}

// dialogOpen opens the task dialog from the sidebar's button.
func (s *screens) dialogOpen() {
	s.t.Helper()
	s.run(`document.getElementById('new-task').click()`)
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
		for (const id of ['launch', 'task']) {
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
		s.t.Errorf("%s: the page asked for keys its catalog lacks: %v", s.lang, miss)
	}
	if errs := get(s.t, s.ctx, 5*time.Second, chromedp.Evaluate[[]string](`window.__errors`)); len(errs) > 0 {
		s.t.Errorf("%s: the page threw: %v", s.lang, errs)
	}
}

func ptrTo[T any](v T) *T { return &v }
