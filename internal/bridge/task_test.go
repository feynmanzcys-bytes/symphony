package bridge

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// taskEnv is a bridge on the fake terminal for tasks, and a folder for the test's repositories. Whatever task a test
// leaves running is ended (and git tidied up) before the folders go.
type taskEnv struct {
	srv    *Server
	base   string
	starts *fakeStarts
	page   client
	root   string
}

func newTaskEnv(t *testing.T, dataDir string) *taskEnv {
	t.Helper()
	needGit(t)
	e := &taskEnv{root: t.TempDir()} // made first, so it goes last
	hub := NewHub()
	if dataDir != "" {
		if err := hub.Open(dataDir); err != nil {
			t.Fatal(err)
		}
	}
	e.srv = NewServer(hub, testWeb)
	e.srv.WaitTimeout = 300 * time.Millisecond
	e.starts = &fakeStarts{}
	e.srv.SetLauncher(NewLauncher("claude", "/mods/sb", "http://127.0.0.1:7788", fakeTerminal(e.starts)))
	e.srv.Launcher.OnlineTimeout = 3 * time.Second
	ts := httptest.NewServer(e.srv)
	t.Cleanup(ts.Close)
	t.Cleanup(func() {
		for _, task := range hub.Tasks() {
			_ = hub.EndTask(task.ID)
		}
		hub.Flush()
		e.srv.Launcher.Terms.KillAll()
	})
	e.base = ts.URL
	e.page = client{t: t, base: ts.URL}
	return e
}

func (e *taskEnv) folder(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(e.root, filepath.FromSlash(name))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func (e *taskEnv) named(name string) *start {
	e.starts.mu.Lock()
	defer e.starts.mu.Unlock()
	for i := range e.starts.starts {
		if e.starts.starts[i].name == name {
			return &e.starts.starts[i]
		}
	}
	return nil
}

// modOf is a client with the account's token, as its sessions' mod.
func (e *taskEnv) modOf(t *testing.T, account string) client {
	acc := e.srv.Hub.Account(account)
	if acc == nil {
		t.Fatalf("no account %s", account)
	}
	return client{t: t, base: e.base, header: map[string]string{"authorization": "Bearer " + acc.Token}}
}

func asTask(t *testing.T, v map[string]any) *Task {
	t.Helper()
	data, _ := json.Marshal(v)
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		t.Fatal(err)
	}
	return &task
}

// taskState is the hub's task, wherever it is in its life.
func taskState(h *Hub, id string) *Task {
	h.mu.Lock()
	defer h.mu.Unlock()
	if t := h.taskLocked(id); t != nil {
		return t.copy()
	}
	return nil
}

func exists(path string) bool { _, err := os.Lstat(path); return err == nil }

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestTaskLifecycle: two employees (one in the user's own repository, one in a plain folder) and an advisor. Each
// member's session starts in a worktree of its own with the task in its environment, reports in, gets the kickoff and
// goes by its session name; the desk sessions go on beside them. Ending the task stops the sessions, saves what they
// left uncommitted, merges the plain folder's branches and makes it plain again, keeps the branches in the user's
// repository and deletes the advisor.
func TestTaskLifecycle(t *testing.T) {
	e := newTaskEnv(t, "")
	repo := initRepo(t, e.folder(t, "repo"), map[string]string{"README.md": "hi\n"})
	repoBase, _ := gitBase(repo)
	plain := e.folder(t, "plain")
	writeFile(t, filepath.Join(plain, "notes.txt"), "notes\n")

	bob, bobAcc := as(t, e.srv, e.base, "bob")
	alice, aliceAcc := as(t, e.srv, e.base, "alice")
	// bob's folder is his desk session's; alice's the one she was last started in
	bob.ok("POST", "/api/event", Event{Session: "bob-desk", Kind: "session.start", Cwd: repo})
	e.srv.Hub.setLast(aliceAcc.ID, LaunchRequest{Dir: plain, Model: "sonnet"})

	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: 2, RoomName: "Sonata", Title: "  build the thing  ",
		Leader: true, PermissionMode: "bypassPermissions", Lang: "en", Members: []TaskMemberRequest{
			{Account: bobAcc.ID, Model: "opus", Effort: "max"},
			{Account: aliceAcc.ID, Model: "sonnet", Effort: "high"},
			{Advisor: &AdvisorRequest{Name: "reviewer", Dir: plain}, Model: "haiku"},
		}}))
	if task.ID != "T1" || task.Status != TaskRunning || task.Title != "build the thing" || task.Leader != bobAcc.ID || len(task.Members) != 3 {
		t.Fatalf("wrong task: %+v", task)
	}
	b, a, r := task.Members[0], task.Members[1], task.Members[2]
	if b.Name != "bob-T1" || a.Name != "alice-T1" || r.Name != "reviewer-T1" || b.Advisor || !r.Advisor ||
		b.Dir != repo || a.Dir != plain || r.Dir != plain || b.Branch != "symphony/T1/bob" || r.Branch != "symphony/T1/reviewer" ||
		b.Worktree != filepath.Join(repo, ".symphony", "worktrees", "bob-T1") || a.Worktree != filepath.Join(plain, ".symphony", "worktrees", "alice-T1") {
		t.Fatalf("wrong members: %+v %+v %+v", b, a, r)
	}
	if len(task.Repos) != 2 || task.repo(repo).Inited || task.repo(repo).Base != repoBase || !task.repo(plain).Inited {
		t.Fatalf("wrong repositories: %+v %+v", task.Repos[0], task.Repos[1])
	}
	// the advisor is an account without a seat; the employees keep theirs
	if v := view(t, e.srv, r.Account); !v.Advisor || v.Seat != -1 || v.Name != "reviewer" {
		t.Fatalf("the advisor should have no seat: %+v", v)
	}
	if view(t, e.srv, bobAcc.ID).Seat != 0 || view(t, e.srv, aliceAcc.ID).Seat != 1 {
		t.Fatal("the employees keep their seats")
	}
	// every member's session starts in its worktree, named after the member, with the task in its environment
	for _, m := range task.Members {
		st := e.named(m.Name)
		if st == nil || st.dir != m.Worktree || st.id != m.Launch || !isDir(m.Worktree) {
			t.Fatalf("%s: wrong start: %+v", m.Name, st)
		}
		env := envMap(st.env)
		if env["SYMPHONY_TASK"] != "T1" || env["SYMPHONY_LAUNCH"] != m.Launch || env["SYMPHONY_LANG"] != "en" ||
			env["SYMPHONY_TOKEN"] != e.srv.Hub.Account(m.Account).Token || env["SYMPHONY_BRIDGE"] != "http://127.0.0.1:7788" {
			t.Fatalf("%s: wrong environment: %v", m.Name, env)
		}
		if flagValue(st.argv, "--name") != m.Name || flagValue(st.argv, "--permission-mode") != "bypassPermissions" || flagValue(st.argv, "--model") != m.Model {
			t.Fatalf("%s: wrong arguments: %v", m.Name, st.argv)
		}
	}
	// the user's checkout doesn't see the worktrees; the plain folder became a repository
	if st := mustGit(t, repo, "status", "--porcelain"); st != "" || !isDir(filepath.Join(plain, ".git")) {
		t.Fatalf("status of the user's repository: %q", st)
	}
	snap := e.srv.Hub.Snapshot()
	if len(snap.Tasks) != 1 || snap.Tasks[0].ID != "T1" || len(snap.Launches) != 3 || snap.Launches[0].Task != "T1" {
		t.Fatalf("the snapshot should show the task and its launches: %+v %+v", snap.Tasks, snap.Launches)
	}
	// a task launch leaves the employee's last options alone, and worktrees are no recent folders
	if last := e.srv.Hub.Account(aliceAcc.ID).Last; last.Dir != plain || last.Model != "sonnet" {
		t.Fatalf("alice's last options changed: %+v", last)
	}
	for _, d := range e.srv.Hub.recentDirs(20) {
		if strings.Contains(d, ".symphony") {
			t.Fatalf("a worktree among the recent folders: %v", d)
		}
	}

	// bob's task session reports in: it takes its member's place and gets the kickoff as its first prompt, queued
	bob.ok("POST", "/api/event", Event{Session: "bob-task", Kind: "session.start", Cwd: b.Worktree, Launch: b.Launch, Task: "T1"})
	if m := taskState(e.srv.Hub, "T1").Members[0]; m.Session != "bob-task" {
		t.Fatalf("the member should have its session: %+v", m)
	}
	items := e.srv.Hub.Chat("bob-task")
	if len(items) != 1 || items[0].Kind != ChatUser || items[0].Origin != "task" || items[0].Status != "queued" {
		t.Fatalf("the kickoff should be the first prompt, queued: %+v", items)
	}
	kickoff := items[0].Text
	for _, want := range []string{"You are the lead of task T1 (meeting room Sonata)", "build the thing",
		"- alice-T1: repository " + plain + ", branch symphony/T1/alice, worktree " + a.Worktree, "- reviewer-T1: ",
		"You are bob-T1: you work in your own git worktree " + b.Worktree + " on branch symphony/T1/bob of " + repo + " (base branch " + repoBase + ")"} {
		if !strings.Contains(kickoff, want) {
			t.Fatalf("the kickoff lacks %q:\n%s", want, kickoff)
		}
	}
	if strings.Contains(kickoff, "- bob-T1:") {
		t.Fatalf("the team list names the others:\n%s", kickoff)
	}
	inbox, _ := bob.ok("GET", "/api/inbox?session=bob-task", nil)["items"].([]any)
	if it, _ := inbox[0].(map[string]any); len(inbox) != 1 || it["kind"] != "say" || it["text"] != kickoff {
		t.Fatalf("the kickoff should wait in the inbox: %v", inbox)
	}
	if s := session(t, e.srv, "bob-task"); s.Task != "T1" || s.Title != "bob-T1" || s.Model != "opus" || s.Launch != b.Launch {
		t.Fatalf("wrong task session: %+v", s)
	}
	// the account shows its desk session, which goes on
	if v := view(t, e.srv, bobAcc.ID); v.Session != "bob-desk" || session(t, e.srv, "bob-desk").State == StateEnded {
		t.Fatalf("bob's desk session should go on: %+v", v)
	}
	// one session per account and task: a second one of T1 is refused; so is one of a task bob isn't in
	if st, code := bob.failure("POST", "/api/event", Event{Session: "bob-task2", Kind: "session.start", Task: "T1"}); st != http.StatusConflict || code != "account_busy" {
		t.Fatalf("a second session of bob's in T1: want 409 account_busy, got %d %q", st, code)
	}
	if st, code := bob.failure("POST", "/api/event", Event{Session: "x", Kind: "session.start", Task: "T9"}); st != http.StatusConflict || code != "task_ended" {
		t.Fatalf("a session of a task that doesn't run: want 409 task_ended, got %d %q", st, code)
	}
	// the kickoff comes once
	bob.ok("POST", "/api/event", Event{Session: "bob-task", Kind: "session.start", Task: "T1"})
	if n := len(e.srv.Hub.Chat("bob-task")); n != 1 {
		t.Fatalf("the kickoff should come once, the chat has %d items", n)
	}

	// alice's: the kickoff of a member names the lead; her desk session can start next to it
	alice.ok("POST", "/api/event", Event{Session: "alice-task", Kind: "session.start", Launch: a.Launch, Task: "T1"})
	if text := e.srv.Hub.Chat("alice-task")[0].Text; !strings.Contains(text, "You are part of task T1 (meeting room Sonata); the lead is bob-T1.") {
		t.Fatalf("wrong kickoff for a member:\n%s", text)
	}
	alice.ok("POST", "/api/event", Event{Session: "alice-desk", Kind: "session.start", Cwd: plain})
	if v := view(t, e.srv, aliceAcc.ID); v.Session != "alice-desk" {
		t.Fatalf("alice's desk session should be accepted beside her task session: %+v", v)
	}
	e.modOf(t, r.Account).ok("POST", "/api/event", Event{Session: "rev-task", Kind: "session.start", Launch: r.Launch, Task: "T1"})
	waitFor(t, "the launches to come online", func() bool {
		for _, m := range task.Members {
			if e.srv.Hub.launchStatus(m.Launch) != LaunchOnline {
				return false
			}
		}
		return true
	})

	// members message each other by session name; the account's name is its desk session's, however busy the other
	bob.ok("POST", "/api/event", Event{Session: "bob-task", Kind: "message.send", To: "alice-T1", Text: "take the API", Delivered: true})
	msgs := e.srv.Hub.Snapshot().Messages
	if m := msgs[len(msgs)-1]; m.FromName != "bob-T1" || m.ToName != "alice-T1" || m.To != "alice-task" {
		t.Fatalf("a message between members should go by their session names: %+v", m)
	}
	time.Sleep(5 * time.Millisecond)
	alice.ok("POST", "/api/event", Event{Session: "alice-task", Kind: "turn.start", Text: "on it"})
	bob.ok("POST", "/api/event", Event{Session: "bob-desk", Kind: "message.send", To: "alice", Text: "lunch?", Delivered: true})
	msgs = e.srv.Hub.Snapshot().Messages
	if m := msgs[len(msgs)-1]; m.FromName != "bob" || m.To != "alice-desk" {
		t.Fatalf("a message to alice should reach her desk session: %+v", m)
	}

	// the room and the members are taken; an advisor in a running task can't be deleted
	if st, code := e.page.failure("POST", "/api/tasks", TaskRequest{Room: 2, Title: "x", Members: []TaskMemberRequest{{Account: aliceAcc.ID}, {Account: bobAcc.ID}}}); st != http.StatusConflict || code != "task_room_busy" {
		t.Fatalf("a second task in the room: want 409 task_room_busy, got %d %q", st, code)
	}
	if st, code := e.page.failure("POST", "/api/tasks", TaskRequest{Room: 3, Title: "x", Members: []TaskMemberRequest{{Account: aliceAcc.ID}, {Account: bobAcc.ID}}}); st != http.StatusConflict || code != "task_member_busy" {
		t.Fatalf("members in a running task: want 409 task_member_busy, got %d %q", st, code)
	}
	if st, code := e.page.failure("DELETE", "/api/accounts/"+r.Account, nil); st != http.StatusConflict || code != "account_busy" {
		t.Fatalf("deleting a member of a running task: want 409 account_busy, got %d %q", st, code)
	}

	// work left uncommitted in the worktrees, then the end
	writeFile(t, filepath.Join(b.Worktree, "bob.txt"), "bob's\n")
	writeFile(t, filepath.Join(a.Worktree, "alice.txt"), "alice's\n")
	writeFile(t, filepath.Join(r.Worktree, "review.txt"), "looks good\n")
	if res := e.page.ok("POST", "/api/tasks/T1/end", nil); len(res) != 0 {
		t.Fatalf("end answers {}: %v", res)
	}
	if st, code := e.page.failure("POST", "/api/tasks/T1/end", nil); st != http.StatusConflict || code != "task_ended" {
		t.Fatalf("ending twice: want 409 task_ended, got %d %q", st, code)
	}
	e.srv.Hub.Flush()
	if got := taskState(e.srv.Hub, "T1"); got.Status != TaskEnded || got.Ended.IsZero() || got.ErrorCode != "" {
		t.Fatalf("the task should have ended: %+v", got)
	}
	snap = e.srv.Hub.Snapshot()
	if len(snap.Tasks) != 0 {
		t.Fatalf("an ended task leaves the snapshot: %+v", snap.Tasks)
	}
	// the members' processes are gone, and their sessions with the task; the desk sessions go on
	for _, m := range task.Members {
		if e.srv.Launcher.Terms.Live(m.Launch) {
			t.Fatalf("%s's process should be killed", m.Name)
		}
	}
	for _, s := range snap.Sessions {
		if s.Task != "" {
			t.Fatalf("the task's sessions should be gone: %+v", s)
		}
	}
	if session(t, e.srv, "bob-desk").State == StateEnded || session(t, e.srv, "alice-desk").State == StateEnded {
		t.Fatal("the desk sessions go on")
	}
	if e.srv.Hub.Account(r.Account) != nil {
		t.Fatal("the advisor's account should be gone")
	}
	for _, m := range task.Members {
		if exists(m.Worktree) {
			t.Fatalf("%s's worktree should be gone", m.Name)
		}
	}
	// the plain folder is plain files again, holding everyone's work
	if exists(filepath.Join(plain, ".git")) || exists(filepath.Join(plain, ".symphony")) {
		t.Fatal("the plain folder should have no .git or .symphony left")
	}
	if readFile(t, filepath.Join(plain, "alice.txt")) != "alice's\n" || readFile(t, filepath.Join(plain, "review.txt")) != "looks good\n" ||
		readFile(t, filepath.Join(plain, "notes.txt")) != "notes\n" {
		t.Fatal("the plain folder should hold the members' work")
	}
	// the user's repository: the branch keeps bob's work, the checkout stays as it was
	if body := mustGit(t, repo, "show", "symphony/T1/bob:bob.txt"); body != "bob's" {
		t.Fatalf("bob's branch should hold his work: %q", body)
	}
	if msg := mustGit(t, repo, "log", "-1", "--format=%s", "symphony/T1/bob"); msg != "symphony: bob-T1's uncommitted work" {
		t.Fatalf("wrong commit message: %q", msg)
	}
	if exists(filepath.Join(repo, "bob.txt")) || exists(filepath.Join(repo, ".symphony")) || mustGit(t, repo, "status", "--porcelain") != "" ||
		strings.Count(mustGit(t, repo, "worktree", "list"), "\n") != 0 {
		t.Fatal("the user's checkout should be as it was, with no worktree left")
	}
}

// TestTaskMergeConflict: without a lead everyone gets the task alike (in the page's language); two members changing the
// same line make the merge fail: the task fails with the folder's repository and the branches left in place, and
// dismissing it takes it out of the office.
func TestTaskMergeConflict(t *testing.T) {
	e := newTaskEnv(t, "")
	plain := e.folder(t, "plan")
	writeFile(t, filepath.Join(plain, "plan.txt"), "draft\n")
	bob, bobAcc := as(t, e.srv, e.base, "bob")
	_, carolAcc := as(t, e.srv, e.base, "carol")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: plain})
	e.srv.Hub.setLast(carolAcc.ID, LaunchRequest{Dir: plain})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: 0, Title: "plan it", Lang: "zh-CN",
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Account: carolAcc.ID}}}))
	if task.Leader != "" {
		t.Fatalf("without leader the task has no lead: %+v", task)
	}
	bob.ok("POST", "/api/event", Event{Session: "b", Kind: "session.start", Launch: task.Members[0].Launch, Task: task.ID})
	if text := e.srv.Hub.Chat("b")[0].Text; !strings.HasPrefix(text, "你是任务 T1（会议室 0）的成员。") || !strings.Contains(text, "plan it") {
		t.Fatalf("wrong flat kickoff:\n%s", text)
	}
	if st, code := e.page.failure("DELETE", "/api/tasks/T1", nil); st != http.StatusConflict || code != "task_running" {
		t.Fatalf("forgetting a running task: want 409 task_running, got %d %q", st, code)
	}

	writeFile(t, filepath.Join(task.Members[0].Worktree, "plan.txt"), "bob's plan\n")
	writeFile(t, filepath.Join(task.Members[1].Worktree, "plan.txt"), "carol's plan\n")
	e.page.ok("POST", "/api/tasks/T1/end", nil)
	e.srv.Hub.Flush()
	tasks := e.srv.Hub.Tasks()
	if len(tasks) != 1 || tasks[0].Status != TaskFailed || tasks[0].ErrorCode != "task_merge_conflict" ||
		tasks[0].ErrorParams["dir"] != plain || tasks[0].ErrorParams["branch"] != "symphony/T1/carol" || tasks[0].Error == "" {
		t.Fatalf("the task should fail with task_merge_conflict: %+v", tasks)
	}
	// nothing is lost: the repository and the branches stay, the folder holds the first member's work, clean
	if !isDir(filepath.Join(plain, ".git")) || mustGit(t, plain, "branch", "--list", "symphony/T1/carol") == "" ||
		readFile(t, filepath.Join(plain, "plan.txt")) != "bob's plan\n" || mustGit(t, plain, "status", "--porcelain") != "" {
		t.Fatal("after a failed merge the repository and the branches should stay, the checkout clean")
	}
	if st, code := e.page.failure("POST", "/api/tasks/T1/end", nil); st != http.StatusConflict || code != "task_ended" {
		t.Fatalf("ending a failed task: want 409 task_ended, got %d %q", st, code)
	}
	e.page.ok("DELETE", "/api/tasks/T1", nil)
	if n := len(e.srv.Hub.Tasks()); n != 0 {
		t.Fatalf("a dismissed task leaves the office, %d left", n)
	}
	if st, code := e.page.failure("DELETE", "/api/tasks/T1", nil); st != http.StatusNotFound || code != "task_unknown" {
		t.Fatalf("an unknown task: want 404 task_unknown, got %d %q", st, code)
	}
}

// TestTaskEndsWithItsLastMember: a member that goes leaves the others at it; when the last one's process is gone
// (without a word, as a killed one), its session goes off duty and the task ends by itself.
func TestTaskEndsWithItsLastMember(t *testing.T) {
	e := newTaskEnv(t, "")
	plain := e.folder(t, "plain")
	bob, bobAcc := as(t, e.srv, e.base, "bob")
	alice, aliceAcc := as(t, e.srv, e.base, "alice")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: plain})
	e.srv.Hub.setLast(aliceAcc.ID, LaunchRequest{Dir: plain})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: 1, Title: "t", Leader: true,
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Account: aliceAcc.ID}}}))
	b, a := task.Members[0], task.Members[1]
	bob.ok("POST", "/api/event", Event{Session: "b", Kind: "session.start", Launch: b.Launch, Task: task.ID})
	alice.ok("POST", "/api/event", Event{Session: "a", Kind: "session.start", Launch: a.Launch, Task: task.ID})

	bob.ok("POST", "/api/event", Event{Session: "b", Kind: "session.end"})
	e.starts.proc(b.Launch).exit(nil)
	waitFor(t, "bob's process to go", func() bool { return !e.srv.Launcher.Terms.Live(b.Launch) })
	time.Sleep(100 * time.Millisecond)
	if got := taskState(e.srv.Hub, task.ID); got.Status != TaskRunning {
		t.Fatalf("the task goes on while alice is at it: %+v", got)
	}

	e.starts.proc(a.Launch).exit(errors.New("signal: killed"))
	waitFor(t, "the task to end", func() bool { return len(e.srv.Hub.Tasks()) == 0 })
	e.srv.Hub.Flush()
	if got := taskState(e.srv.Hub, task.ID); got.Status != TaskEnded || exists(filepath.Join(plain, ".git")) {
		t.Fatalf("the task should have ended and the folder be plain again: %+v", got)
	}
}

// TestTaskStop: a bridge on its way out kills the sessions, but leaves their task to the next bridge.
func TestTaskStop(t *testing.T) {
	e := newTaskEnv(t, "")
	plain := e.folder(t, "plain")
	_, bobAcc := as(t, e.srv, e.base, "bob")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: plain})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: 1, Title: "t",
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Advisor: &AdvisorRequest{Name: "helper", Dir: plain}}}}))
	e.srv.Hub.Stop()
	e.srv.Launcher.Terms.KillAll()
	waitFor(t, "the processes to go", func() bool {
		return !e.srv.Launcher.Terms.Live(task.Members[0].Launch) && !e.srv.Launcher.Terms.Live(task.Members[1].Launch)
	})
	time.Sleep(100 * time.Millisecond)
	if got := taskState(e.srv.Hub, task.ID); got.Status != TaskRunning || !isDir(task.Members[0].Worktree) {
		t.Fatalf("a stopping bridge should leave the task as it is: %+v", got)
	}
}

// TestTaskSubfolder: a folder deep in a repository: the worktree checks the whole repository out and the session
// starts at the same place in it; two members in one repository share it.
func TestTaskSubfolder(t *testing.T) {
	e := newTaskEnv(t, "")
	repo := initRepo(t, e.folder(t, "mono"), map[string]string{"app/main.go": "package main\n", "lib/lib.go": "package lib\n"})
	app := filepath.Join(repo, "app")
	_, bobAcc := as(t, e.srv, e.base, "bob")
	_, aliceAcc := as(t, e.srv, e.base, "alice")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: app})
	e.srv.Hub.setLast(aliceAcc.ID, LaunchRequest{Dir: repo})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: 1, Title: "t",
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Account: aliceAcc.ID}}}))
	b, a := task.Members[0], task.Members[1]
	if b.Worktree != filepath.Join(app, ".symphony", "worktrees", "bob-T1") || e.named("bob-T1").dir != filepath.Join(b.Worktree, "app") ||
		e.named("alice-T1").dir != a.Worktree {
		t.Fatalf("bob should start in app/ of his worktree: %+v %+v", b, e.named("bob-T1"))
	}
	if len(task.Repos) != 2 || task.Repos[0].Root != task.Repos[1].Root || task.Repos[0].Inited || task.Repos[1].Inited {
		t.Fatalf("both folders are in the user's repository: %+v %+v", task.Repos[0], task.Repos[1])
	}
	if st := mustGit(t, repo, "status", "--porcelain"); st != "" {
		t.Fatalf("the worktrees must not show in the user's checkout: %q", st)
	}
	writeFile(t, filepath.Join(b.Worktree, "app", "new.go"), "package main\n")
	e.page.ok("POST", "/api/tasks/T1/end", nil)
	e.srv.Hub.Flush()
	if exists(filepath.Join(app, ".symphony")) || exists(filepath.Join(repo, ".symphony")) || mustGit(t, repo, "show", "symphony/T1/bob:app/new.go") != "package main" {
		t.Fatal("the worktrees should be gone and bob's branch hold his work")
	}
}

// TestTaskErrors: what a task request can get wrong; a failure after git or the sessions started takes everything
// back; a room holds one task and an employee is in one task at a time.
func TestTaskErrors(t *testing.T) {
	e := newTaskEnv(t, "")
	plain := e.folder(t, "plain")
	_, bob := as(t, e.srv, e.base, "bob")
	_, alice := as(t, e.srv, e.base, "alice")
	nf, nofolder := as(t, e.srv, e.base, "nofolder")
	e.srv.Hub.setLast(bob.ID, LaunchRequest{Dir: plain})
	e.srv.Hub.setLast(alice.ID, LaunchRequest{Dir: plain})
	two := []TaskMemberRequest{{Account: bob.ID}, {Account: alice.ID}}
	advisor := func(name, dir string) TaskMemberRequest {
		return TaskMemberRequest{Advisor: &AdvisorRequest{Name: name, Dir: dir}}
	}
	var six []TaskMemberRequest
	for _, n := range []string{"a", "b", "c", "d", "e", "f"} {
		six = append(six, advisor(n, plain))
	}
	for name, tc := range map[string]struct {
		req    TaskRequest
		status int
		code   string
	}{
		"no description":                    {TaskRequest{Title: " ", Members: two}, 400, "task_empty_title"},
		"one member":                        {TaskRequest{Title: "t", Members: two[:1]}, 400, "task_too_few"},
		"six members":                       {TaskRequest{Title: "t", Members: six}, 400, "task_too_many"},
		"the same employee twice":           {TaskRequest{Title: "t", Members: []TaskMemberRequest{{Account: bob.ID}, {Account: bob.ID}}}, 400, "task_member_dup"},
		"two advisors of one name":          {TaskRequest{Title: "t", Members: []TaskMemberRequest{advisor("x", plain), advisor("x", plain)}}, 400, "task_member_dup"},
		"an employee without a folder":      {TaskRequest{Title: "t", Members: []TaskMemberRequest{{Account: bob.ID}, {Account: nofolder.ID}}}, 400, "task_bad_dir"},
		"an unknown account":                {TaskRequest{Title: "t", Members: []TaskMemberRequest{{Account: bob.ID}, {Account: "nope"}}}, 404, "account_unknown"},
		"an advisor named like an employee": {TaskRequest{Title: "t", Members: []TaskMemberRequest{{Account: bob.ID}, advisor("alice", plain)}}, 409, "account_name_taken"},
		"an advisor without a name":         {TaskRequest{Title: "t", Members: []TaskMemberRequest{{Account: bob.ID}, advisor(" ", plain)}}, 400, "account_bad_name"},
		"an advisor's relative folder":      {TaskRequest{Title: "t", Members: []TaskMemberRequest{{Account: bob.ID}, advisor("x", "rel/dir")}}, 400, "launch_dir_not_absolute"},
		"an advisor's missing folder":       {TaskRequest{Title: "t", Members: []TaskMemberRequest{{Account: bob.ID}, advisor("x", filepath.Join(plain, "nope"))}}, 400, "launch_dir_missing"},
		"an unknown model":                  {TaskRequest{Title: "t", Members: []TaskMemberRequest{{Account: bob.ID, Model: "gpt"}, {Account: alice.ID}}}, 400, "launch_bad_option"},
		"an unknown permission mode":        {TaskRequest{Title: "t", PermissionMode: "yolo", Members: two}, 400, "launch_bad_option"},
	} {
		if st, code := e.page.failure("POST", "/api/tasks", tc.req); st != tc.status || code != tc.code {
			t.Errorf("%s: want %d %s, got %d %q", name, tc.status, tc.code, st, code)
		}
	}
	_, body := e.page.do("POST", "/api/tasks", TaskRequest{Title: "t", Members: []TaskMemberRequest{{Account: bob.ID}, {Account: nofolder.ID}}})
	if params, _ := body["params"].(map[string]any); params["name"] != "nofolder" {
		t.Errorf("task_bad_dir should name the employee: %v", body)
	}
	if e.starts.last() != nil || exists(filepath.Join(plain, ".git")) || len(e.srv.Hub.Snapshot().Accounts) != 3 {
		t.Fatal("a refused request should start nothing, make no repository and no account")
	}
	// a desk session started in a terminal leaves its folder on the account, so once off duty the employee can join
	// a task there; a task session's folder (a worktree) is not remembered
	nf.ok("POST", "/api/event", Event{Session: "nf-desk", Kind: "session.start", Cwd: plain})
	nf.ok("POST", "/api/event", Event{Session: "nf-desk", Kind: "session.end"})
	if _, dir := e.srv.Hub.employee(nofolder.ID); dir != plain {
		t.Fatalf("the employee's folder should be the one its session reported, got %q", dir)
	}
	if v := e.srv.Hub.Snapshot().Accounts; !slices.ContainsFunc(v, func(a AccountView) bool { return a.ID == nofolder.ID && a.Dir == plain }) {
		t.Fatalf("the snapshot should carry the account's folder: %+v", v)
	}
	if st, code := e.page.failure("POST", "/api/tasks", TaskRequest{Title: "t", Members: []TaskMemberRequest{{Account: bob.ID}, {Account: nofolder.ID}, {Account: nofolder.ID}}}); st != 400 || code != "task_member_dup" {
		t.Fatalf("with a folder the employee gets past the folder check (the duplicate is refused instead): %d %s", st, code)
	}

	// git failing after the advisor's account and the plain folder's repository were made takes them back
	empty := e.folder(t, "deeper/than/plain/empty")
	mustGit(t, empty, "init", "-q") // a repository without a commit has no base to branch off
	st, body := e.page.do("POST", "/api/tasks", TaskRequest{Title: "t", Members: []TaskMemberRequest{{Account: bob.ID}, advisor("helper", empty)}})
	if params, _ := body["params"].(map[string]any); st != http.StatusInternalServerError || body["code"] != "task_git_failed" || params["dir"] != empty || params["detail"] == "" {
		t.Fatalf("want 500 task_git_failed for the empty repository, got %d %v", st, body)
	}
	if exists(filepath.Join(plain, ".git")) || exists(filepath.Join(plain, ".symphony")) || len(e.srv.Hub.Snapshot().Accounts) != 3 || e.starts.last() != nil {
		t.Fatal("the failed task should take back the repository it made and the advisor's account")
	}

	// a session that won't start takes everything back too
	e.starts.failStart = true
	if st, code := e.page.failure("POST", "/api/tasks", TaskRequest{Title: "t", Members: append([]TaskMemberRequest{advisor("helper", plain)}, two...)}); st != http.StatusInternalServerError || code != "pty_start_failed" {
		t.Fatalf("want 500 pty_start_failed, got %d %q", st, code)
	}
	e.starts.failStart = false
	if exists(filepath.Join(plain, ".git")) || exists(filepath.Join(plain, ".symphony")) || len(e.srv.Hub.Snapshot().Accounts) != 3 || len(e.srv.Hub.Tasks()) != 0 {
		t.Fatal("the failed task should leave nothing behind")
	}

	// the tries that failed gave their numbers back; a room holds one task, an employee is in one at a time
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: 4, Title: "t", Members: two}))
	if task.ID != "T1" {
		t.Fatalf("the first task made should be T1, got %s", task.ID)
	}
	if st, code := e.page.failure("POST", "/api/tasks", TaskRequest{Room: 4, Title: "t", Members: []TaskMemberRequest{advisor("x", plain), advisor("y", plain)}}); st != http.StatusConflict || code != "task_room_busy" {
		t.Fatalf("want 409 task_room_busy, got %d %q", st, code)
	}
	st, body = e.page.do("POST", "/api/tasks", TaskRequest{Room: 5, Title: "t", Members: []TaskMemberRequest{advisor("x", plain), {Account: bob.ID}}})
	if params, _ := body["params"].(map[string]any); st != http.StatusConflict || body["code"] != "task_member_busy" || params["name"] != "bob" {
		t.Fatalf("want 409 task_member_busy naming bob, got %d %v", st, body)
	}
	if st, code := e.page.failure("DELETE", "/api/accounts/"+bob.ID, nil); st != http.StatusConflict || code != "account_busy" {
		t.Fatalf("deleting an employee in a running task: want 409 account_busy, got %d %q", st, code)
	}
	// an advisor has no desk to be started at
	visitor, _ := e.srv.Hub.CreateAdvisor("visitor")
	if st, code := e.page.failure("POST", "/api/launch", LaunchRequest{Account: visitor.ID, Dir: plain}); st != http.StatusNotFound || code != "account_unknown" {
		t.Fatalf("starting an advisor at a desk: want 404 account_unknown, got %d %q", st, code)
	}
	if st, code := e.page.failure("POST", "/api/tasks/T9/end", nil); st != http.StatusNotFound || code != "task_unknown" {
		t.Fatalf("ending an unknown task: want 404 task_unknown, got %d %q", st, code)
	}
	if res := e.page.ok("GET", "/api/tasks", nil); len(res["tasks"].([]any)) != 1 {
		t.Fatalf("GET /api/tasks: %v", res)
	}
}

// TestTaskOpenEndsLeftovers: a bridge that stopped with a task running leaves it in tasks.json; the next one ends it
// the same way (worktrees gone, the plain folder plain again, the advisor deleted), drops advisors left over from a
// task that was never made, and goes on counting.
func TestTaskOpenEndsLeftovers(t *testing.T) {
	needGit(t)
	data := t.TempDir()
	e := newTaskEnv(t, data)
	plain := e.folder(t, "plain")
	writeFile(t, filepath.Join(plain, "notes.txt"), "notes\n")
	_, bobAcc := as(t, e.srv, e.base, "bob")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: plain})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: 1, Title: "t", Leader: true,
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Advisor: &AdvisorRequest{Name: "helper", Dir: plain}}}}))
	writeFile(t, filepath.Join(task.Members[1].Worktree, "help.txt"), "helped\n")
	if _, err := e.srv.Hub.CreateAdvisor("ghost"); err != nil {
		t.Fatal(err)
	}
	e.srv.Hub.Flush()
	// the bridge dies: it forgets the task without ending it
	e.srv.Hub.mu.Lock()
	e.srv.Hub.tasks = nil
	e.srv.Hub.mu.Unlock()
	e.srv.Launcher.Terms.KillAll()

	again := NewHub()
	if err := again.Open(data); err != nil {
		t.Fatal(err)
	}
	again.Flush()
	if got := taskState(again, task.ID); got == nil || got.Status != TaskEnded || len(again.Tasks()) != 0 {
		t.Fatalf("the task left running should be ended: %+v", got)
	}
	if exists(task.Members[0].Worktree) || exists(filepath.Join(plain, ".git")) || readFile(t, filepath.Join(plain, "help.txt")) != "helped\n" {
		t.Fatal("the leftover task's git should be tidied up")
	}
	if accs := again.Snapshot().Accounts; len(accs) != 1 || accs[0].Name != "bob" {
		t.Fatalf("only bob should be left (the advisors go): %+v", accs)
	}
	var file taskFile
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(data, "tasks.json"))), &file); err != nil || file.Next != 2 || len(file.Tasks) != 1 || file.Tasks[0].Status != TaskEnded {
		t.Fatalf("tasks.json should keep the ended task and the counter: %+v (%v)", file, err)
	}
}
