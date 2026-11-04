//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// TestTask walks through a collaborative task without spending tokens: claude is a script that just sits there,
// and the test reports the members in the way the mod would. It creates the task from Coworkers ("+ Group", the
// dialog: two employees and an advisor), checks the git side (a worktree and a branch for everyone, a plain folder
// made a repository), the kickoff prompts, the group in the sidebar's list with its members in its profile, the task
// window and a member's chat window from it, then ends the task and checks that everything is cleaned up.
func TestTask(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	requireBrowser(t)
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))
	// claude: a script that ignores its arguments and stays alive until the bridge kills it
	b.Launcher.Claude = sleeper(t) // claude sits there; the test reports its sessions in

	// bob's folder is a repository, alice's a plain folder; carol has never worked (no folder) and can't be picked
	work := t.TempDir()
	app, plain := filepath.Join(work, "app"), filepath.Join(work, "plain")
	for _, d := range []string{app, plain} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git(app, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(app, "README.md"), []byte("app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(app, "add", "-A")
	git(app, "commit", "-qm", "init")
	if err := os.WriteFile(filepath.Join(plain, "notes.txt"), []byte("notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{}
	for _, a := range []struct{ name, dir string }{{"bob", app}, {"alice", plain}, {"carol", ""}} { // in this order: the dialog lists them by seat
		name, dir := a.name, a.dir
		acc, err := b.Hub.CreateAccount(name)
		if err != nil {
			t.Fatal(err)
		}
		tokens[name] = acc.Token
		if dir != "" {
			b.post(t, acc.Token, "/api/event", map[string]any{"session": "s-" + name, "kind": "session.start", "cwd": dir, "model": "opus"})
		}
	}

	browser := newBrowser(t)
	page, _ := clockTab(t, browser, b.URL+"/t/messenger/?lang=en", 0) // collects the page's errors in window.__errors
	do(t, page, 10*time.Second, chromedp.EmulateViewport(1400, 900))
	js := func(expr string) string { // the value as JSON, strings without their quotes
		out := get(t, page, 5*time.Second, chromedp.Evaluate[string](fmt.Sprintf(`JSON.stringify((() => { return %s })())`, expr)))
		var str string
		if json.Unmarshal([]byte(out), &str) == nil {
			return str
		}
		return out
	}
	jsBool := func(expr string) bool { return js(expr) == "true" }
	run := func(stmt string) { // a statement in the page, for clicks and typing
		get(t, page, 5*time.Second, chromedp.Evaluate[bool](fmt.Sprintf(`(() => { %s; return true })()`, stmt)))
	}
	waitUntil(t, 20*time.Second, "Coworkers lists the three employees and offers a new group", func() bool {
		return jsBool(`document.querySelectorAll('#people .row').length === 3 && !document.getElementById('new-task').hidden`)
	})

	// "+ Group" opens the task dialog
	run(`document.getElementById('new-task').click()`)
	waitUntil(t, 5*time.Second, "the task dialog opens", func() bool { return jsBool(`!document.getElementById('task').hidden`) })
	if n := js(`document.querySelectorAll('#task .mem').length`); n != "3" {
		t.Fatalf("the dialog should list the 3 employees, got %s", n)
	}
	if !jsBool(`!!document.querySelector('#task .mem.off input[disabled]') && [...document.querySelectorAll('#task .mem.off .nm')].map(e => e.textContent).join() === 'carol'`) {
		t.Fatalf("carol, without a folder, should be the one employee that can't be picked: %s", js(`document.querySelector('#task .emps').innerHTML`))
	}
	if !jsBool(`document.querySelector('#task .go').disabled`) {
		t.Fatal("Start should be off before 2 members are picked")
	}
	if !jsBool(`!document.querySelector('#task [name=git]').checked && !document.getElementById('task-git-hint').hidden && document.getElementById('task-git-hint').textContent.length > 40`) {
		t.Fatal("the git box should start unticked, explained beside it")
	}
	// the description, git branches for everyone, bob then alice (bob leads), and an advisor on bob's repository
	run(`document.querySelector('#task [name=desc]').value = 'Add a CHANGELOG and a Makefile'; document.querySelector('#task [name=git]').click()`)
	for _, name := range []string{"bob", "alice"} {
		run(fmt.Sprintf(`[...document.querySelectorAll('#task .mem')].find(r => r.querySelector('.nm').textContent === %q).querySelector('input').click()`, name))
	}
	run(`document.querySelector('#task .add-adv').click()`)
	run(fmt.Sprintf(`(() => { const r = document.querySelector('#task .adv'); r.querySelector('[name=name]').value = 'rev'; r.querySelector('[name=dir]').value = %q })()`, app))
	if got := js(`document.querySelector('#task .count').textContent`); !strings.Contains(got, "3") || !strings.Contains(got, "20") {
		t.Fatalf("the count should read 3 of 20 members, got %q", got)
	}
	if !jsBool(`[...document.querySelectorAll('#task .mem.on, #task .adv')].map(r => r.querySelector('.ord').textContent).join() === '1,2,3' && document.querySelector('#task .mem.lead .nm').textContent === 'bob'`) {
		t.Fatalf("the members should be numbered in the order picked, bob leading: %s", js(`document.querySelector('#task form').innerHTML`))
	}
	if jsBool(`document.querySelector('#task .go').disabled`) {
		t.Fatal("Start should be on with 3 members")
	}
	run(`document.querySelector('#task .go').click()`)

	// the bridge made the task: a worktree on a branch for each member, the plain folder a repository
	var task *bridge.Task
	waitUntil(t, 30*time.Second, "the task is running", func() bool {
		for _, k := range b.Hub.Tasks() {
			if k.Status == "running" {
				task = k
				return true
			}
		}
		return false
	}, func() string { return js(`document.querySelector('#task .status')?.textContent`) })
	if task.ID != "T1" || task.Leader == "" || len(task.Members) != 3 || len(task.Repos) != 2 || !task.Git || task.Name != "" {
		t.Fatalf("unexpected task: %+v", task)
	}
	names := map[string]*bridge.Member{}
	for _, m := range task.Members {
		names[m.Name] = m
	}
	for _, name := range []string{"bob-T1", "alice-T1", "rev-T1"} {
		m := names[name]
		if m == nil {
			t.Fatalf("no member %s in %+v", name, task.Members)
		}
		if !isDirectory(m.Worktree) || m.Launch == "" {
			t.Fatalf("member %s has no worktree or launch: %+v", name, m)
		}
	}
	if names["rev-T1"].Dir != app || !names["rev-T1"].Advisor || names["bob-T1"].Account != task.Leader {
		t.Fatalf("the advisor should work in %s and bob lead: %+v", app, task.Members)
	}
	if got := git(app, "branch", "--list", "symphony/T1/*"); !strings.Contains(got, "symphony/T1/bob") || !strings.Contains(got, "symphony/T1/rev") {
		t.Fatalf("bob's and rev's branches should exist in app: %q", got)
	}
	if !isDirectory(filepath.Join(plain, ".git")) || git(plain, "status", "--porcelain") != "" {
		t.Fatal("the plain folder should be a repository now, with everything committed")
	}
	for _, r := range task.Repos {
		if (r.Dir == plain) != r.Inited {
			t.Fatalf("only the plain folder should be marked as made by the bridge: %+v", r)
		}
	}
	waitUntil(t, 10*time.Second, "the task window opens with the three members starting", func() bool {
		return jsBool(`!document.getElementById('taskwin').hidden && document.querySelectorAll('#taskwin .mrow').length === 3 && document.querySelectorAll('#taskwin .mrow.wait').length === 3`)
	}, func() string {
		return js(`'taskwin hidden: ' + document.getElementById('taskwin').hidden + '; dialog hidden: ' + document.getElementById('task').hidden + '; status: ' + document.querySelector('#task .status')?.textContent + '; members: ' + document.querySelector('#taskwin .members').innerHTML.slice(0, 600) + '; errors: ' + JSON.stringify(window.__errors)`)
	})
	if !jsBool(`document.querySelector('#taskwin h2').textContent.startsWith('T1') && !!document.querySelector('#taskwin .badge.lead') && !!document.querySelector('#taskwin .badge.adv') && !!document.querySelector('#taskwin .end')`) {
		t.Fatalf("the task window should show T1, a lead, an advisor and the End button: %s", js(`document.querySelector('#taskwin .panel').innerHTML`))
	}
	waitUntil(t, 10*time.Second, "the group is in the sidebar's list, picked", func() bool {
		return jsBool(`(r => !!r && r.classList.contains('on'))(document.querySelector('#groups .row[data-group="T1"]'))`)
	}, func() string { return js(`document.getElementById('groups').innerHTML`) })

	// the members report in, as the mod would, from their worktrees; the advisor's token is in accounts.json
	var accounts []struct{ Name, Token string }
	raw, err := os.ReadFile(filepath.Join(b.Data, "accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &accounts); err != nil {
		t.Fatal(err)
	}
	for _, a := range accounts {
		tokens[a.Name] = a.Token
	}
	for _, name := range []string{"bob", "alice", "rev"} {
		m := names[name+"-T1"]
		b.post(t, tokens[name], "/api/event", map[string]any{"session": "t-" + name, "kind": "session.start", "cwd": m.Worktree, "launch": m.Launch, "task": "T1", "model": "opus"})
		b.post(t, tokens[name], "/api/event", map[string]any{"session": "t-" + name, "kind": "session.title", "task": "T1", "title": name + "-T1"})
	}
	// each gets its kickoff: bob the lead's, alice a member's naming the lead
	kickoff := func(name string) string {
		req, _ := http.NewRequest("GET", b.URL+"/api/inbox?session=t-"+name, nil)
		req.Header.Set("Authorization", "Bearer "+tokens[name])
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var body struct{ Items []struct{ Kind, Text string } }
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil || len(body.Items) != 1 || body.Items[0].Kind != "say" {
			t.Fatalf("%s should have one prompt waiting, got %+v (%v)", name, body.Items, err)
		}
		return body.Items[0].Text
	}
	if text := kickoff("bob"); !strings.Contains(text, "lead of group T1") || !strings.Contains(text, "alice-T1") || !strings.Contains(text, "symphony/T1/bob") || !strings.Contains(text, "Add a CHANGELOG") {
		t.Fatalf("bob's kickoff should make him the lead and name the team:\n%s", text)
	}
	if text := kickoff("alice"); !strings.Contains(text, "the lead is bob-T1") || !strings.Contains(text, "symphony/T1/alice") {
		t.Fatalf("alice's kickoff should name the lead and her branch:\n%s", text)
	}
	waitUntil(t, 10*time.Second, "the members are online in the task window", func() bool {
		return jsBool(`document.querySelectorAll('#taskwin .mrow').length === 3 && document.querySelectorAll('#taskwin .mrow.wait').length === 0`)
	})
	// the group's profile lists the three members; the coworkers' rows stay as they were (a member's task session is
	// no row of its own)
	waitUntil(t, 10*time.Second, "the group's profile lists the members", func() bool {
		return jsBool(`document.querySelectorAll('#profile .list .row').length === 3 && document.querySelectorAll('#people .row').length === 3`)
	}, func() string { return js(`document.getElementById('profile').innerText`) })
	screenshot(t, page, "task-group.png")

	// a member's row opens its chat window: bob-T1's, with the kickoff as its first prompt
	run(`[...document.querySelectorAll('#taskwin .mrow')].find(r => r.querySelector('b').textContent === 'bob').click()`) // (shown short: bob, not bob-T1)
	waitUntil(t, 10*time.Second, "bob-T1's chat window opens with the kickoff, the way back to the group in the profile", func() bool {
		return jsBool(`!document.getElementById('chat').hidden && document.getElementById('taskwin').hidden && document.querySelector('#chat h2').textContent === 'bob' && document.querySelectorAll('#chat .flow .it.user').length === 1 && !!document.querySelector('#profile .link[data-group="T1"]')`)
	}, func() string {
		return js(`document.querySelector('#chat .head').innerHTML + document.getElementById('profile').innerHTML`)
	})
	screenshot(t, page, "task-chat.png")

	// bob's work on his branch survives the end; the end is a click in the task window
	if err := os.WriteFile(filepath.Join(names["bob-T1"].Worktree, "Makefile"), []byte("all:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(`document.querySelector('#profile .link[data-group="T1"]').click()`)
	waitUntil(t, 5*time.Second, "the task window opens again", func() bool {
		return jsBool(`!document.getElementById('taskwin').hidden && document.getElementById('chat').hidden && !!document.querySelector('#taskwin .end')`)
	})
	run(`window.confirm = () => true; document.querySelector('#taskwin .end').click()`)
	waitUntil(t, 60*time.Second, "the task ends and leaves the snapshot", func() bool {
		return len(b.Hub.Tasks()) == 0
	}, func() string { data, _ := json.Marshal(b.Hub.Tasks()); return string(data) })
	// git: the worktrees are gone, bob's uncommitted Makefile is on his branch, the plain folder is plain again
	for _, m := range task.Members {
		if isDirectory(m.Worktree) {
			t.Fatalf("worktree %s should be gone", m.Worktree)
		}
	}
	if got := git(app, "show", "symphony/T1/bob:Makefile"); got != "all:" {
		t.Fatalf("bob's Makefile should be committed on his branch, got %q", got)
	}
	// the plain folder stays a repository: alice's branch merged into its base, the branch kept, the .symphony folders gone
	if !isDirectory(filepath.Join(plain, ".git")) || isDirectory(filepath.Join(plain, ".symphony")) || isDirectory(filepath.Join(app, ".symphony")) {
		t.Fatal("the plain folder should keep its repository, and the .symphony folders should be gone")
	}
	if got := git(plain, "branch", "--list", "symphony/T1/alice"); !strings.Contains(got, "symphony/T1/alice") {
		t.Fatalf("alice's branch should stay in the plain folder's repository: %q", got)
	}
	if got := git(plain, "rev-list", "--count", "HEAD"); got == "" || git(plain, "status", "--porcelain") != "" {
		t.Fatal("the plain folder's base should hold the merged result, clean")
	}
	if _, err := os.Stat(filepath.Join(plain, "notes.txt")); err != nil {
		t.Fatal("the plain folder's files should still be there")
	}
	// the page: the group leaves the list, the advisor is gone, the employees stay; the task window stays, saying the
	// task ended (no End button, the chat input off), until it is closed; no errors
	waitUntil(t, 30*time.Second, "the group leaves the list and the employees stay", func() bool {
		return jsBool(`document.querySelectorAll('#groups .row').length === 0 && document.querySelectorAll('#people .row').length === 3`)
	}, func() string {
		return js(`document.getElementById('groups').innerHTML + document.getElementById('people').innerText`)
	})
	waitUntil(t, 5*time.Second, "the task window says the task ended", func() bool {
		return jsBool(`!document.getElementById('taskwin').hidden && document.querySelector('#taskwin .top .status').textContent.startsWith('Ended') && !document.querySelector('#taskwin .end') && document.querySelector('#taskwin textarea').disabled`)
	}, func() string { return js(`document.querySelector('#taskwin .panel').innerHTML`) })
	run(`document.querySelector('#taskwin .close').click()`)
	if !jsBool(`document.getElementById('taskwin').hidden`) {
		t.Fatal("the task window should close")
	}
	if b.account("rev") != nil {
		t.Fatal("the advisor's account should be gone with the task")
	}

	// ---------- a group the usual way: named, without a description, without git; members come and go ----------
	run(`document.getElementById('new-task').click()`)
	waitUntil(t, 5*time.Second, "the task dialog opens again", func() bool {
		return jsBool(`!document.getElementById('task').hidden && document.activeElement === document.getElementById('task-name')`)
	})
	run(`document.querySelector('#task [name=name]').value = 'Payments'`)
	for _, name := range []string{"bob", "alice"} {
		run(fmt.Sprintf(`[...document.querySelectorAll('#task .mem')].find(r => r.querySelector('.nm').textContent === %q).querySelector('input').click()`, name))
	}
	run(`document.querySelector('#task .go').click()`)
	var group *bridge.Task
	waitUntil(t, 30*time.Second, "the group is running", func() bool {
		for _, k := range b.Hub.Tasks() {
			if k.ID == "T2" && k.Status == "running" {
				group = k
				return true
			}
		}
		return false
	}, func() string { return js(`document.querySelector('#task .status')?.textContent`) })
	if group.Git || group.Name != "Payments" || group.Title != "" || len(group.Repos) != 0 || group.Members[0].Branch != "" || group.Members[0].Worktree != "" {
		t.Fatalf("a plain, untitled group: %+v %+v", group, group.Members[0])
	}
	gb, ga := group.Members[0], group.Members[1]
	// the members' sessions start in their folders as they are: no new branch, no .symphony
	if l := b.Hub.Snapshot().Launches; !slices.ContainsFunc(l, func(l *bridge.Launch) bool { return l.ID == gb.Launch && l.Dir == app }) || !slices.ContainsFunc(l, func(l *bridge.Launch) bool { return l.ID == ga.Launch && l.Dir == plain }) {
		t.Fatalf("bob should start in %s and alice in %s: %+v", app, plain, l)
	}
	if got := git(app, "branch", "--list", "symphony/T2/*"); got != "" || isDirectory(filepath.Join(app, ".symphony")) {
		t.Fatalf("a plain group makes no branch: %q", got)
	}
	// the window and the sidebar go by the name, the number beside it
	waitUntil(t, 10*time.Second, "the group window shows Payments with its number", func() bool {
		return jsBool(`!document.getElementById('taskwin').hidden && document.querySelector('#taskwin .top h2').textContent === 'Payments' && document.querySelector('#taskwin .tid').textContent === 'T2' && document.querySelectorAll('#taskwin .mitem').length === 2 && !!document.querySelector('#taskwin .actions .add')`)
	}, func() string { return js(`document.querySelector('#taskwin .top').innerHTML`) })
	waitUntil(t, 10*time.Second, "the sidebar lists Payments", func() bool {
		return jsBool(`(r => !!r && r.querySelector('.t').textContent === 'Payments' && r.querySelector('.badge').textContent.startsWith('T2'))(document.querySelector('#groups .row[data-group="T2"]'))`)
	}, func() string { return js(`document.getElementById('groups').innerHTML`) })
	for _, name := range []string{"bob", "alice"} {
		m := map[string]*bridge.Member{"bob": gb, "alice": ga}[name]
		b.post(t, tokens[name], "/api/event", map[string]any{"session": "g-" + name, "kind": "session.start", "cwd": m.Dir, "launch": m.Launch, "task": "T2", "model": "opus"})
	}
	// the kickoff of a plain, untitled group: the folder, no branch, and the user's word to come in the group chat
	gkick := func(name string) string {
		req, _ := http.NewRequest("GET", b.URL+"/api/inbox?session=g-"+name, nil)
		req.Header.Set("Authorization", "Bearer "+tokens[name])
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var body struct{ Items []struct{ Kind, Text string } }
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		var texts []string
		for _, it := range body.Items {
			if it.Kind == "say" {
				texts = append(texts, it.Text)
			}
		}
		return strings.Join(texts, "\n---\n")
	}
	if text := gkick("bob"); !strings.Contains(text, "lead of group T2 (Payments)") || !strings.Contains(text, "hasn't described a task yet") || !strings.Contains(text, "you work in "+app+" itself") || strings.Contains(text, "symphony/T2") {
		t.Fatalf("bob's kickoff should be the plain, untitled one:\n%s", text)
	}
	gkick("alice")
	// "+ Add members" opens the dialog for who joins: bob and alice are in already, carol has no folder; an advisor joins
	run(`document.querySelector('#taskwin .actions .add').click()`)
	waitUntil(t, 5*time.Second, "the dialog opens to add members", func() bool {
		return jsBool(`!document.getElementById('task').hidden && document.querySelector('#task form').classList.contains('adding') && [...document.querySelectorAll('#task .mem .nm')].map(e => e.textContent).join() === 'carol' && document.querySelector('#task .title').textContent.includes('Payments') && document.getElementById('task-name').getClientRects().length === 0`)
	}, func() string { return js(`document.querySelector('#task form').innerHTML.slice(0, 600)`) })
	run(`document.querySelector('#task .add-adv').click()`)
	run(fmt.Sprintf(`(() => { const r = document.querySelector('#task .adv'); r.querySelector('[name=name]').value = 'rev'; r.querySelector('[name=dir]').value = %q })()`, app))
	if got := js(`document.querySelector('#task .count').textContent`); !strings.Contains(got, "1") || !strings.Contains(got, "18") {
		t.Fatalf("the count should read 1 of 18 (two are in already), got %q", got)
	}
	run(`document.querySelector('#task .go').click()`)
	waitUntil(t, 30*time.Second, "the advisor joins: three member cards and the bridge's note in the team chat", func() bool {
		return jsBool(`document.getElementById('task').hidden && document.querySelectorAll('#taskwin .mitem').length === 3 && [...document.querySelectorAll('#taskwin .tmsg.sys')].some(m => m.textContent.includes('rev-T2'))`)
	}, func() string {
		return js(`document.querySelector('#task .status')?.textContent + ' | ' + document.querySelector('#taskwin .members').innerHTML`)
	})
	if text := gkick("bob"); !strings.Contains(text, "New in group T2 (Payments): rev-T2") {
		t.Fatalf("bob should be told rev joined:\n%s", text)
	}
	// alice is removed: her card says so, grayed, without buttons; bob is told
	run(`window.confirm = () => true; [...document.querySelectorAll('#taskwin .mitem')].find(c => c.querySelector('b').textContent === 'alice').querySelector('.acts .kick').click()`)
	waitUntil(t, 10*time.Second, "alice's card says she left", func() bool {
		return jsBool(`(c => !!c && c.classList.contains('left') && !c.querySelector('.acts'))([...document.querySelectorAll('#taskwin .mitem')].find(c => c.querySelector('b').textContent === 'alice'))`)
	}, func() string { return js(`document.querySelector('#taskwin .members').innerHTML`) })
	if text := gkick("bob"); !strings.Contains(text, "alice-T2 left group T2 (Payments)") {
		t.Fatalf("bob should be told alice left:\n%s", text)
	}
	// bob's process dies: his card offers to start him again; the new session gets the kickoff again
	if term := b.Launcher.Terms.Get(gb.Launch); term != nil {
		term.Kill()
	}
	waitUntil(t, 20*time.Second, "bob's card offers Start again", func() bool {
		return jsBool(`!![...document.querySelectorAll('#taskwin .mitem')].find(c => c.querySelector('b').textContent === 'bob')?.querySelector('.acts .start')`)
	}, func() string { return js(`document.querySelector('#taskwin .members').innerHTML`) })
	run(`[...document.querySelectorAll('#taskwin .mitem')].find(c => c.querySelector('b').textContent === 'bob').querySelector('.acts .start').click()`)
	var again *bridge.Member
	waitUntil(t, 20*time.Second, "bob has a new launch", func() bool {
		for _, k := range b.Hub.Tasks() {
			if k.ID == "T2" && k.Members[0].Launch != gb.Launch {
				again = k.Members[0]
				return true
			}
		}
		return false
	})
	b.post(t, tokens["bob"], "/api/event", map[string]any{"session": "g2-bob", "kind": "session.start", "cwd": app, "launch": again.Launch, "task": "T2", "model": "opus"})
	if text := gkick("bob"); text != "" {
		t.Fatalf("the old session gets nothing more: %s", text)
	}
	req, _ := http.NewRequest("GET", b.URL+"/api/inbox?session=g2-bob", nil)
	req.Header.Set("Authorization", "Bearer "+tokens["bob"])
	if res, err := http.DefaultClient.Do(req); err != nil {
		t.Fatal(err)
	} else {
		var body struct{ Items []struct{ Kind, Text string } }
		_ = json.NewDecoder(res.Body).Decode(&body)
		res.Body.Close()
		if len(body.Items) != 1 || !strings.Contains(body.Items[0].Text, "lead of group T2 (Payments)") {
			t.Fatalf("bob's new session should get the kickoff again: %+v", body.Items)
		}
	}
	waitUntil(t, 10*time.Second, "bob is online again in the window", func() bool {
		return jsBool(`(c => !!c && !c.querySelector('.acts .start'))([...document.querySelectorAll('#taskwin .mitem')].find(c => c.querySelector('b').textContent === 'bob'))`)
	})
	screenshot(t, page, "task-group-plain.png")
	// dissolved: the folders are left as they are, the advisor goes
	run(`document.querySelector('#taskwin .end').click()`)
	waitUntil(t, 60*time.Second, "the group ends", func() bool { return len(b.Hub.Tasks()) == 0 })
	if got := git(app, "branch", "--list", "symphony/T2/*"); got != "" || isDirectory(filepath.Join(app, ".symphony")) || b.account("rev") != nil {
		t.Fatalf("dissolving a plain group leaves git alone and deletes the advisor: %q", got)
	}
	if errs := js(`window.__errors`); errs != "[]" {
		t.Fatalf("page errors: %s", errs)
	}
}

func isDirectory(p string) bool { fi, err := os.Stat(p); return err == nil && fi.IsDir() }
