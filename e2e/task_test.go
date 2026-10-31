//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
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

// TestTask walks through a collaborative task without spending tokens: claude is a script that just sits there,
// and the test reports the members in the way the mod would. It creates the task from the page (a click on a free
// small meeting room, the dialog: two employees and an advisor), checks the git side (a worktree and a branch for
// everyone, a plain folder made a repository), the kickoff prompts, the members seated in the room, the task
// window and a member's chat window, then ends the task and checks that everything is cleaned up.
func TestTask(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake claude is a shell script")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	requireBrowser(t)
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))
	// claude: a script that ignores its arguments and stays alive until the bridge kills it
	fake := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexec sleep 100000\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	b.Launcher.Claude = fake

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
	for name, dir := range map[string]string{"bob": app, "alice": plain, "carol": ""} {
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
	office, _ := clockTab(t, browser, b.URL+"/?lang=en", 0) // collects the page's errors in window.__errors
	js := func(expr string) string {                        // the value as JSON, strings without their quotes
		out := get(t, office, 5*time.Second, chromedp.Evaluate[string](fmt.Sprintf(`JSON.stringify((() => { return %s })())`, expr)))
		var str string
		if json.Unmarshal([]byte(out), &str) == nil {
			return str
		}
		return out
	}
	jsBool := func(expr string) bool { return js(expr) == "true" }
	run := func(stmt string) { // a statement in the page, for clicks and typing
		get(t, office, 5*time.Second, chromedp.Evaluate[bool](fmt.Sprintf(`(() => { %s; return true })()`, stmt)))
	}
	waitUntil(t, 20*time.Second, "the office shows the three employees and clickable room plates", func() bool {
		return jsBool(`document.querySelectorAll('.emp-label:not(.member)').length === 3 && document.querySelectorAll('.room-plate.click').length > 0`)
	})

	// a click on the first small room opens the task dialog for it
	run(`document.querySelectorAll('.room-plate')[0].click()`)
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
	seats := js(`document.querySelectorAll('.room-plate')[0].dataset.seats`)
	// the description, bob then alice (bob leads), and an advisor on bob's repository
	run(`document.querySelector('#task [name=desc]').value = 'Add a CHANGELOG and a Makefile'`)
	for _, name := range []string{"bob", "alice"} {
		run(fmt.Sprintf(`[...document.querySelectorAll('#task .mem')].find(r => r.querySelector('.nm').textContent === %q).querySelector('input').click()`, name))
	}
	run(`document.querySelector('#task .add-adv').click()`)
	run(fmt.Sprintf(`(() => { const r = document.querySelector('#task .adv'); r.querySelector('[name=name]').value = 'rev'; r.querySelector('[name=dir]').value = %q })()`, app))
	if got := js(`document.querySelector('#task .count').textContent`); !strings.Contains(got, "3") || !strings.Contains(got, seats) {
		t.Fatalf("the count should read 3 of %s seats, got %q", seats, got)
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
	if task.ID != "T1" || task.Leader == "" || len(task.Members) != 3 || len(task.Repos) != 2 {
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
	})
	if !jsBool(`document.querySelector('#taskwin h2').textContent.startsWith('T1') && !!document.querySelector('#taskwin .badge.lead') && !!document.querySelector('#taskwin .badge.adv') && !!document.querySelector('#taskwin .end')`) {
		t.Fatalf("the task window should show T1, a lead, an advisor and the End button: %s", js(`document.querySelector('#taskwin .panel').innerHTML`))
	}
	waitUntil(t, 10*time.Second, "the room's plate shows the task", func() bool {
		return jsBool(`(p => p.dataset.task === 'T1' && p.dataset.light === 'busy' && p.textContent.includes('T1'))(document.querySelectorAll('.room-plate')[0])`)
	})

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
	if text := kickoff("bob"); !strings.Contains(text, "lead of task T1") || !strings.Contains(text, "alice-T1") || !strings.Contains(text, "symphony/T1/bob") || !strings.Contains(text, "Add a CHANGELOG") {
		t.Fatalf("bob's kickoff should make him the lead and name the team:\n%s", text)
	}
	if text := kickoff("alice"); !strings.Contains(text, "the lead is bob-T1") || !strings.Contains(text, "symphony/T1/alice") {
		t.Fatalf("alice's kickoff should name the lead and her branch:\n%s", text)
	}
	waitUntil(t, 10*time.Second, "the members are online in the task window", func() bool {
		return jsBool(`document.querySelectorAll('#taskwin .mrow').length === 3 && document.querySelectorAll('#taskwin .mrow.wait').length === 0`)
	})
	// the three walk to the room and sit on its chairs (the spots room0.0 to room0.2 of the first small room); their
	// tags are named after the member sessions, and bob's and alice's desks keep their own tags
	waitUntil(t, 60*time.Second, "the members' figures sit down in the meeting room", func() bool {
		return jsBool(`(() => {
			const tags = [...document.querySelectorAll('.emp-label.member')]
			return tags.length === 3 && tags.every(l => l.dataset.task === 'T1' && !l.hidden && l.dataset.room === 'small' && l.dataset.moving === '0') &&
				tags.map(l => l.dataset.spot).sort().join() === 'room0.0,room0.1,room0.2' &&
				tags.map(l => l.querySelector('.nm').textContent).sort().join() === 'alice-T1,bob-T1,rev-T1' &&
				document.querySelectorAll('.emp-label:not(.member)').length === 3
		})()`)
	}, func() string {
		return js(`[...document.querySelectorAll('.emp-label')].map(l => l.querySelector('.nm').textContent + ' ' + JSON.stringify(l.dataset)).join('\n')`)
	})
	screenshot(t, office, "task-room.png")

	// a member's row opens its chat window: bob-T1's, with the kickoff as its first prompt
	run(`[...document.querySelectorAll('#taskwin .mrow')].find(r => r.querySelector('b').textContent === 'bob-T1').click()`)
	waitUntil(t, 10*time.Second, "bob-T1's chat window opens with the kickoff", func() bool {
		return jsBool(`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === 'bob-T1' && document.querySelectorAll('#chat .flow .it.user').length === 1 && !!document.querySelector('#chat .meta [data-task="T1"]')`)
	}, func() string { return js(`document.querySelector('#chat .head').innerHTML`) })
	screenshot(t, office, "task-chat.png")
	run(`document.querySelector('#chat .close').click()`)

	// bob's work on his branch survives the end; the end is a click in the task window
	if err := os.WriteFile(filepath.Join(names["bob-T1"].Worktree, "Makefile"), []byte("all:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(`document.querySelectorAll('.room-plate')[0].click()`)
	waitUntil(t, 5*time.Second, "the task window opens again", func() bool {
		return jsBool(`!document.getElementById('taskwin').hidden && !!document.querySelector('#taskwin .end')`)
	})
	run(`window.confirm = () => true; document.querySelector('#taskwin .end').click()`)
	waitUntil(t, 60*time.Second, "the task ends and leaves the office", func() bool {
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
	// the page: the room is free, the figures are gone, the advisor is gone, the employees are back; no errors
	waitUntil(t, 30*time.Second, "the room is free and the members are back at their desks", func() bool {
		return jsBool(`(p => p.dataset.light === 'free' && !p.dataset.task)(document.querySelectorAll('.room-plate')[0]) && document.querySelectorAll('.emp-label.member').length === 0 && document.querySelectorAll('.emp-label').length === 3 && document.getElementById('taskwin').hidden`)
	}, func() string {
		return js(`[...document.querySelectorAll('.emp-label')].map(l => l.querySelector('.nm').textContent + ' ' + JSON.stringify(l.dataset)).join('\n')`)
	})
	if b.account("rev") != nil {
		t.Fatal("the advisor's account should be gone with the task")
	}
	if errs := js(`window.__errors`); errs != "[]" {
		t.Fatalf("page errors: %s", errs)
	}
}

func isDirectory(p string) bool { fi, err := os.Stat(p); return err == nil && fi.IsDir() }
