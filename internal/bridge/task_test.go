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
		t.Cleanup(func() { _ = hub.Flush() }) // what is written in the background (the error log too), before the folder goes
	}
	e.srv = NewServer(hub, testWeb)
	e.srv.Key = testKey
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
	return client{t: t, base: e.base, header: map[string]string{"authorization": "Bearer " + acc.Token}, modOnly: true}
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
// left uncommitted, merges the plain folder's branches into it (its repository and the branches stay), keeps the
// branches in the user's repository and deletes the advisor.
func hint(n int) *int { return &n }

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

	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(2), Title: "  build the thing  ",
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
	for _, want := range []string{"You are the lead of task T1. The user's task", "build the thing",
		"- alice-T1: folder " + plain + ", branch symphony/T1/alice, worktree " + a.Worktree, "- reviewer-T1: ",
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
	if text := e.srv.Hub.Chat("alice-task")[0].Text; !strings.Contains(text, "You are part of task T1; the lead is bob-T1.") {
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

	// the members are taken; an advisor in a running task can't be deleted
	if st, code := e.page.failure("POST", "/api/tasks", TaskRequest{Room: hint(3), Title: "x", Members: []TaskMemberRequest{{Account: aliceAcc.ID}, {Account: bobAcc.ID}}}); st != http.StatusConflict || code != "task_member_busy" {
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
	if got := taskState(e.srv.Hub, "T1"); got.Status != TaskEnded || got.Ended == nil || got.ErrorCode != "" {
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
	for _, l := range snap.Launches {
		if l.Task != "" {
			t.Fatalf("the task's launches should be gone: %+v", l)
		}
	}
	for _, m := range task.Members {
		if e.srv.Launcher.Terms.Get(m.Launch) != nil {
			t.Fatalf("%s's ended terminal should be forgotten", m.Name)
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
	// the plain folder holds everyone's work, merged on its base branch, clean; the repository and the branches stay
	if !isDir(filepath.Join(plain, ".git")) || exists(filepath.Join(plain, ".symphony")) || mustGit(t, plain, "status", "--porcelain") != "" {
		t.Fatal("the plain folder should keep its repository, with no .symphony left and nothing uncommitted")
	}
	if readFile(t, filepath.Join(plain, "alice.txt")) != "alice's\n" || readFile(t, filepath.Join(plain, "review.txt")) != "looks good\n" ||
		readFile(t, filepath.Join(plain, "notes.txt")) != "notes\n" {
		t.Fatal("the plain folder should hold the members' work")
	}
	if mustGit(t, plain, "show", "symphony/T1/alice:alice.txt") != "alice's" || mustGit(t, plain, "branch", "--list", "symphony/T1/reviewer") == "" ||
		strings.Count(mustGit(t, plain, "worktree", "list"), "\n") != 0 {
		t.Fatal("the plain folder's branches should stay, its worktrees go")
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
// same line make the merge fail: the task fails with the folder's repository and the branches left as they were (the
// first member's merge taken back), keeps its advisor and sessions, and dismissing it takes them out of the office.
func TestTaskMergeConflict(t *testing.T) {
	e := newTaskEnv(t, "")
	plain := e.folder(t, "plan")
	writeFile(t, filepath.Join(plain, "plan.txt"), "draft\n")
	bob, bobAcc := as(t, e.srv, e.base, "bob")
	_, carolAcc := as(t, e.srv, e.base, "carol")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: plain})
	e.srv.Hub.setLast(carolAcc.ID, LaunchRequest{Dir: plain})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(0), Title: "plan it", Lang: "zh-CN",
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Account: carolAcc.ID}, {Advisor: &AdvisorRequest{Name: "helper", Dir: plain}}}}))
	if task.Leader != "" {
		t.Fatalf("without leader the task has no lead: %+v", task)
	}
	helper := task.Members[2]
	e.modOf(t, helper.Account).ok("POST", "/api/event", Event{Session: "h", Kind: "session.start", Launch: helper.Launch, Task: task.ID})
	base := mustGit(t, plain, "rev-parse", "HEAD")
	bob.ok("POST", "/api/event", Event{Session: "b", Kind: "session.start", Launch: task.Members[0].Launch, Task: task.ID})
	if text := e.srv.Hub.Chat("b")[0].Text; !strings.HasPrefix(text, "你是任务 T1 的成员。") || !strings.Contains(text, "plan it") {
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
		tasks[0].ErrorParams["dir"] != plain || tasks[0].ErrorParams["branch"] != "symphony/T1/carol" ||
		!strings.Contains(tasks[0].Error, "CONFLICT (content): Merge conflict in plan.txt") || tasks[0].Ended == nil {
		t.Fatalf("the task should fail with task_merge_conflict, telling git's conflict: %+v", tasks)
	}
	// nothing is lost: the repository and the branches stay as they were (bob's merge is taken back), the checkout
	// clean, and the empty worktrees folder gone
	if !isDir(filepath.Join(plain, ".git")) || mustGit(t, plain, "branch", "--list", "symphony/T1/carol") == "" ||
		mustGit(t, plain, "rev-parse", "HEAD") != base || readFile(t, filepath.Join(plain, "plan.txt")) != "draft\n" ||
		mustGit(t, plain, "status", "--porcelain") != "" || exists(filepath.Join(plain, ".symphony")) {
		t.Fatal("after a failed merge the repository and the branches should stay as they were, the checkout clean")
	}
	if mustGit(t, plain, "show", "symphony/T1/bob:plan.txt") != "bob's plan" {
		t.Fatal("bob's branch should keep his work")
	}
	// the failed task keeps its advisor and its sessions for its window
	if e.srv.Hub.Account(helper.Account) == nil || session(t, e.srv, "h").Task != task.ID || len(e.srv.Hub.Snapshot().Launches) != 3 {
		t.Fatal("a failed task should keep its advisor, its sessions and its launches")
	}
	if st, code := e.page.failure("POST", "/api/tasks/T1/end", nil); st != http.StatusConflict || code != "task_ended" {
		t.Fatalf("ending a failed task: want 409 task_ended, got %d %q", st, code)
	}
	// its members stay its members until it is dismissed: no deleting them
	for _, id := range []string{carolAcc.ID, helper.Account} {
		if st, code := e.page.failure("DELETE", "/api/accounts/"+id, nil); st != http.StatusConflict || code != "account_busy" {
			t.Fatalf("deleting a member of a failed task: want 409 account_busy, got %d %q", st, code)
		}
	}
	e.page.ok("DELETE", "/api/tasks/T1", nil)
	if n := len(e.srv.Hub.Tasks()); n != 0 {
		t.Fatalf("a dismissed task leaves the office, %d left", n)
	}
	e.page.ok("DELETE", "/api/accounts/"+carolAcc.ID, nil)
	if snap := e.srv.Hub.Snapshot(); e.srv.Hub.Account(helper.Account) != nil || len(snap.Launches) != 0 ||
		slices.ContainsFunc(snap.Sessions, func(s *Session) bool { return s.Task != "" }) {
		t.Fatal("dismissing a failed task deletes its advisor and forgets its sessions and launches")
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
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(1), Title: "t", Leader: true,
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
	if got := taskState(e.srv.Hub, task.ID); got.Status != TaskEnded || exists(filepath.Join(plain, ".symphony")) || !exists(filepath.Join(plain, ".git")) {
		t.Fatalf("the task should have ended and the worktrees be gone: %+v", got)
	}
}

// TestTaskStop: a bridge on its way out kills the sessions, but leaves their task to the next bridge.
func TestTaskStop(t *testing.T) {
	e := newTaskEnv(t, "")
	plain := e.folder(t, "plain")
	_, bobAcc := as(t, e.srv, e.base, "bob")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: plain})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(1), Title: "t",
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
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(1), Title: "t",
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
	_, gone := as(t, e.srv, e.base, "gone")
	e.srv.Hub.setLast(bob.ID, LaunchRequest{Dir: plain})
	e.srv.Hub.setLast(alice.ID, LaunchRequest{Dir: plain})
	e.srv.Hub.setLast(gone.ID, LaunchRequest{Dir: filepath.Join(plain, "removed")})
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
		"a negative room":                   {TaskRequest{Room: hint(-1), Title: "t", Members: two}, 400, "launch_bad_option"},
		"an employee and an advisor in one": {TaskRequest{Title: "t", Members: []TaskMemberRequest{{Account: alice.ID, Advisor: &AdvisorRequest{Name: "x", Dir: plain}}, {Account: bob.ID}}}, 400, "launch_bad_option"},
		"an employee whose folder is gone":  {TaskRequest{Title: "t", Members: []TaskMemberRequest{{Account: bob.ID}, {Account: gone.ID}}}, 400, "task_dir_missing"},
	} {
		if st, code := e.page.failure("POST", "/api/tasks", tc.req); st != tc.status || code != tc.code {
			t.Errorf("%s: want %d %s, got %d %q", name, tc.status, tc.code, st, code)
		}
	}
	_, body := e.page.do("POST", "/api/tasks", TaskRequest{Title: "t", Members: []TaskMemberRequest{{Account: bob.ID}, {Account: nofolder.ID}}})
	if params, _ := body["params"].(map[string]any); params["name"] != "nofolder" {
		t.Errorf("task_bad_dir should name the employee: %v", body)
	}
	_, body = e.page.do("POST", "/api/tasks", TaskRequest{Title: "t", Members: []TaskMemberRequest{{Account: bob.ID}, {Account: gone.ID}}})
	if params, _ := body["params"].(map[string]any); params["name"] != "gone" || params["dir"] != filepath.Join(plain, "removed") {
		t.Errorf("task_dir_missing should name the employee and the folder: %v", body)
	}
	if e.starts.last() != nil || exists(filepath.Join(plain, ".git")) || len(e.srv.Hub.Snapshot().Accounts) != 4 {
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
	if params, _ := body["params"].(map[string]any); st != http.StatusBadRequest || body["code"] != "task_repo_no_commit" || params["dir"] != empty {
		t.Fatalf("want 400 task_repo_no_commit for the empty repository, got %d %v", st, body)
	}
	if exists(filepath.Join(plain, ".git")) || exists(filepath.Join(plain, ".symphony")) || len(e.srv.Hub.Snapshot().Accounts) != 4 || e.starts.last() != nil {
		t.Fatal("the failed task should take back the repository it made and the advisor's account")
	}

	// a session that won't start takes everything back too
	e.starts.failStart = true
	if st, code := e.page.failure("POST", "/api/tasks", TaskRequest{Title: "t", Members: append([]TaskMemberRequest{advisor("helper", plain)}, two...)}); st != http.StatusInternalServerError || code != "pty_start_failed" {
		t.Fatalf("want 500 pty_start_failed, got %d %q", st, code)
	}
	e.starts.failStart = false
	if exists(filepath.Join(plain, ".git")) || exists(filepath.Join(plain, ".symphony")) || len(e.srv.Hub.Snapshot().Accounts) != 4 || len(e.srv.Hub.Tasks()) != 0 {
		t.Fatal("the failed task should leave nothing behind")
	}
	// files kept from an earlier task's worktrees (.symphony/kept) stay when a start in that folder fails
	kept := filepath.Join(plain, ".symphony", "kept", "bob-T0", "out", "report.md")
	writeFile(t, kept, "report")
	e.starts.failStart = true
	if st, code := e.page.failure("POST", "/api/tasks", TaskRequest{Title: "t", Members: append([]TaskMemberRequest{advisor("helper", plain)}, two...)}); st != http.StatusInternalServerError || code != "pty_start_failed" {
		t.Fatalf("want 500 pty_start_failed, got %d %q", st, code)
	}
	e.starts.failStart = false
	if !exists(kept) || exists(filepath.Join(plain, ".git")) || exists(filepath.Join(plain, ".symphony", "worktrees")) {
		t.Fatal("the failed start should take back the repository and the worktrees it made, and leave the kept files")
	}
	if err := os.RemoveAll(filepath.Join(plain, ".symphony")); err != nil {
		t.Fatal(err)
	}

	// the tries that failed keep their numbers (messages may have been logged under them); an employee is in one task
	// at a time
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(4), Title: "t", Members: two}))
	if task.ID != "T4" {
		t.Fatalf("the first task made should be T4 (T1 to T3 went to the tries that failed), got %s", task.ID)
	}
	st, body = e.page.do("POST", "/api/tasks", TaskRequest{Room: hint(5), Title: "t", Members: []TaskMemberRequest{advisor("x", plain), {Account: bob.ID}}})
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
// the same way (worktrees gone, the work merged into the plain folder, the advisor deleted), drops advisors left over from a
// task that was never made, and goes on counting.
func TestTaskOpenEndsLeftovers(t *testing.T) {
	needGit(t)
	data := t.TempDir()
	e := newTaskEnv(t, data)
	plain := e.folder(t, "plain")
	writeFile(t, filepath.Join(plain, "notes.txt"), "notes\n")
	_, bobAcc := as(t, e.srv, e.base, "bob")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: plain})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(1), Title: "t", Leader: true,
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
	if exists(task.Members[0].Worktree) || exists(filepath.Join(plain, ".symphony")) || readFile(t, filepath.Join(plain, "help.txt")) != "helped\n" {
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

// TestTaskDetachedWork: commits a member made on a detached HEAD would go with its worktree: they get a branch of
// their own first; the members' branches are merged into the folder the bridge made a repository of, as always.
func TestTaskDetachedWork(t *testing.T) {
	e := newTaskEnv(t, "")
	plain := e.folder(t, "plain")
	writeFile(t, filepath.Join(plain, "notes.txt"), "notes\n")
	_, bobAcc := as(t, e.srv, e.base, "bob")
	_, aliceAcc := as(t, e.srv, e.base, "alice")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: plain})
	e.srv.Hub.setLast(aliceAcc.ID, LaunchRequest{Dir: plain})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(1), Title: "t",
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Account: aliceAcc.ID}}}))
	b, a := task.Members[0], task.Members[1]
	writeFile(t, filepath.Join(a.Worktree, "alice.txt"), "alice's\n")
	mustGit(t, b.Worktree, "checkout", "-q", "--detach")
	writeFile(t, filepath.Join(b.Worktree, "bob.txt"), "bob's\n")
	mustGit(t, b.Worktree, "add", "-A")
	mustGit(t, b.Worktree, append(gitCommitter, "commit", "-q", "-m", "detached")...)
	writeFile(t, filepath.Join(b.Worktree, "more.txt"), "more\n") // left uncommitted on the detached HEAD

	e.page.ok("POST", "/api/tasks/"+task.ID+"/end", nil)
	e.srv.Hub.Flush()
	if got := taskState(e.srv.Hub, task.ID); got.Status != TaskEnded {
		t.Fatalf("the task should have ended: %+v", got)
	}
	if exists(b.Worktree) || mustGit(t, plain, "show", b.Branch+"-detached:bob.txt") != "bob's" ||
		mustGit(t, plain, "show", b.Branch+"-detached:more.txt") != "more" {
		t.Fatal("bob's detached work should be on a branch of its own, his worktree gone")
	}
	if readFile(t, filepath.Join(plain, "alice.txt")) != "alice's\n" || exists(filepath.Join(plain, "bob.txt")) {
		t.Fatal("alice's branch should be merged into the folder")
	}
}

// TestTaskKeepsIgnoredFiles: what git ignores in a member's worktree (build output, a .env) isn't on its branch; when
// the task ends it is moved to <folder>/.symphony/kept/<slug>-<task id> at the same paths, and the member's Kept names
// that folder. When something there is in the way, the move stops, the worktree stays with the rest and the task fails.
func TestTaskKeepsIgnoredFiles(t *testing.T) {
	e := newTaskEnv(t, "")
	repo := initRepo(t, e.folder(t, "repo"), map[string]string{"a.txt": "a\n", ".gitignore": "out/\n.env\n"})
	_, bobAcc := as(t, e.srv, e.base, "bob")
	_, aliceAcc := as(t, e.srv, e.base, "alice")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: repo})
	e.srv.Hub.setLast(aliceAcc.ID, LaunchRequest{Dir: repo})
	start := func() *Task {
		return asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(1), Title: "t",
			Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Account: aliceAcc.ID}}}))
	}
	ignored := map[string]string{"out/app.bin": "binary", "out/sub/x.o": "object", ".env": "TOKEN=secret", "src/.env": "nested"}

	task := start()
	b, a := task.Members[0], task.Members[1]
	for name, body := range ignored {
		writeFile(t, filepath.Join(b.Worktree, filepath.FromSlash(name)), body)
	}
	writeFile(t, filepath.Join(b.Worktree, "src", "main.go"), "package main\n")
	writeFile(t, filepath.Join(a.Worktree, "alice.txt"), "alice's\n") // alice leaves nothing ignored
	e.page.ok("POST", "/api/tasks/"+task.ID+"/end", nil)
	e.srv.Hub.Flush()
	kept := filepath.Join(repo, ".symphony", "kept", "bob-"+task.ID)
	if got := taskState(e.srv.Hub, task.ID); got.Status != TaskEnded || got.Members[0].Kept != kept || got.Members[1].Kept != "" {
		t.Fatalf("the task should have ended, bob's ignored files kept in %s: %+v %+v", kept, got, got.Members[0])
	}
	for name, body := range ignored {
		if got := readFile(t, filepath.Join(kept, filepath.FromSlash(name))); got != body {
			t.Fatalf("%s should be kept: %q", name, got)
		}
	}
	if exists(b.Worktree) || exists(a.Worktree) || exists(filepath.Join(repo, ".symphony", "kept", "alice-"+task.ID)) {
		t.Fatal("the worktrees should be gone, and alice had nothing to keep")
	}
	if mustGit(t, repo, "show", b.Branch+":src/main.go") != "package main" {
		t.Fatal("bob's branch should hold his work")
	}
	if _, err := runGit(repo, "show", b.Branch+":.env"); err == nil {
		t.Fatal("an ignored file must not be committed")
	}
	if st := mustGit(t, repo, "status", "--porcelain"); st != "" {
		t.Fatalf("the user's checkout should be as it was (.symphony is excluded): %q", st)
	}

	// next time something is in the way: .env is moved, out/ stays in the worktree, which stays too
	task = start()
	b = task.Members[0]
	for name, body := range ignored {
		writeFile(t, filepath.Join(b.Worktree, filepath.FromSlash(name)), body)
	}
	kept = filepath.Join(repo, ".symphony", "kept", "bob-"+task.ID)
	writeFile(t, filepath.Join(kept, "out"), "in the way")
	e.page.ok("POST", "/api/tasks/"+task.ID+"/end", nil)
	e.srv.Hub.Flush()
	got := taskState(e.srv.Hub, task.ID)
	if got.Status != TaskFailed || got.ErrorCode != "task_git_failed" || got.ErrorParams["dir"] != b.Worktree || got.Members[0].Kept != kept {
		t.Fatalf("the task should fail on bob's worktree, naming where .env went: %+v %+v", got, got.Members[0])
	}
	if readFile(t, filepath.Join(kept, ".env")) != "TOKEN=secret" || readFile(t, filepath.Join(kept, "out")) != "in the way" ||
		readFile(t, filepath.Join(b.Worktree, "out", "app.bin")) != "binary" {
		t.Fatal("what was in the way stays, and what couldn't be moved stays in the worktree")
	}
}

// TestTaskKeepsNestedCommits: a repository checked out in a member's worktree (one it made, or a submodule) whose commits
// no remote of its own has keeps the worktree: the task fails on it instead of deleting them. A submodule that is only
// checked out, with nothing new, doesn't stand in the way.
func TestTaskKeepsNestedCommits(t *testing.T) {
	e := newTaskEnv(t, "")
	sub := initRepo(t, e.folder(t, "subsrc"), map[string]string{"s.txt": "s\n"})
	repo := initRepo(t, e.folder(t, "repo"), map[string]string{"a.txt": "a\n"})
	mustGit(t, repo, "-c", "protocol.file.allow=always", "submodule", "add", "-q", sub, "sub")
	mustGit(t, repo, append(gitCommitter, "commit", "-q", "-m", "add sub")...)
	_, bobAcc := as(t, e.srv, e.base, "bob")
	_, aliceAcc := as(t, e.srv, e.base, "alice")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: repo})
	e.srv.Hub.setLast(aliceAcc.ID, LaunchRequest{Dir: repo})
	start := func() *Task {
		return asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(1), Title: "t",
			Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Account: aliceAcc.ID}}}))
	}
	end := func(task *Task) *Task {
		e.page.ok("POST", "/api/tasks/"+task.ID+"/end", nil)
		e.srv.Hub.Flush()
		return taskState(e.srv.Hub, task.ID)
	}
	initSub := func(worktree string) {
		mustGit(t, worktree, "-c", "protocol.file.allow=always", "submodule", "update", "--init", "-q")
	}

	// the submodule checked out, nothing new in it: the worktrees go
	task := start()
	initSub(task.Members[0].Worktree)
	if got := end(task); got.Status != TaskEnded || exists(task.Members[0].Worktree) {
		t.Fatalf("an untouched submodule shouldn't keep the worktree: %+v", got)
	}

	// commits in the submodule, and in a repository bob made: bob's worktree stays, with both
	task = start()
	b := task.Members[0]
	initSub(b.Worktree)
	writeFile(t, filepath.Join(b.Worktree, "sub", "new.txt"), "bob's\n")
	mustGit(t, filepath.Join(b.Worktree, "sub"), "add", "-A")
	mustGit(t, filepath.Join(b.Worktree, "sub"), append(gitCommitter, "commit", "-q", "-m", "sub work")...)
	lib := filepath.Join(b.Worktree, "lib")
	writeFile(t, filepath.Join(lib, "lib.go"), "package lib\n")
	mustGit(t, lib, "init", "-q")
	mustGit(t, lib, "add", "-A")
	mustGit(t, lib, append(gitCommitter, "commit", "-q", "-m", "lib work")...)
	got := end(task)
	if got.Status != TaskFailed || got.ErrorCode != "task_git_failed" || !strings.Contains(got.Error, "commits no remote has in lib, sub") {
		t.Fatalf("the task should fail, naming the repositories with new commits: %+v", got)
	}
	if readFile(t, filepath.Join(b.Worktree, "sub", "new.txt")) != "bob's\n" || mustGit(t, lib, "log", "--format=%s") != "lib work" {
		t.Fatal("bob's commits should still be there")
	}
	if exists(task.Members[1].Worktree) {
		t.Fatal("alice's worktree had nothing of the kind: it goes")
	}
}

// TestTaskFolderChecks: a member's folder in another task's worktree is refused (an employee whose desk session went
// there, an advisor's pick), and so are folders in or around a repository a running task made, and the home folder or
// one above it.
func TestTaskFolderChecks(t *testing.T) {
	e := newTaskEnv(t, "")
	outer := e.folder(t, "outer")
	plain := e.folder(t, "outer/plain")
	sub := e.folder(t, "outer/plain/sub")
	writeFile(t, filepath.Join(sub, "a.txt"), "a\n")
	bob, bobAcc := as(t, e.srv, e.base, "bob")
	_, aliceAcc := as(t, e.srv, e.base, "alice")
	_, carolAcc := as(t, e.srv, e.base, "carol")
	e.srv.Hub.setLast(aliceAcc.ID, LaunchRequest{Dir: plain})
	e.srv.Hub.setLast(carolAcc.ID, LaunchRequest{Dir: plain})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(1), Title: "t",
		Members: []TaskMemberRequest{{Account: aliceAcc.ID}, {Account: carolAcc.ID}}}))
	wt := task.Members[0].Worktree

	advisor := func(dir string) TaskRequest {
		return TaskRequest{Room: hint(2), Title: "t", Members: []TaskMemberRequest{{Advisor: &AdvisorRequest{Name: "x", Dir: dir}}, {Advisor: &AdvisorRequest{Name: "y", Dir: dir}}}}
	}
	for name, tc := range map[string]struct {
		dir    string
		status int
		code   string
	}{
		"a worktree":                    {wt, 400, "task_dir_in_worktree"},
		"a folder in a worktree":        {filepath.Join(wt, "sub"), 400, "task_dir_in_worktree"},
		"the folder a task made a repo": {plain, 409, "task_repo_in_use"},
		"a folder in it":                {sub, 409, "task_repo_in_use"},
		"a folder around it":            {outer, 409, "task_repo_in_use"},
	} {
		st, body := e.page.do("POST", "/api/tasks", advisor(tc.dir))
		if st != tc.status || body["code"] != tc.code {
			t.Errorf("%s: want %d %s, got %d %v", name, tc.status, tc.code, st, body)
		}
		if params, _ := body["params"].(map[string]any); tc.code == "task_repo_in_use" && params["task"] != task.ID {
			t.Errorf("%s: task_repo_in_use should name %s: %v", name, task.ID, body)
		}
	}
	// bob's desk session went into alice's worktree
	bob.ok("POST", "/api/event", Event{Session: "bob-desk", Kind: "session.start", Cwd: wt})
	st, body := e.page.do("POST", "/api/tasks", TaskRequest{Room: hint(2), Title: "t",
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Advisor: &AdvisorRequest{Name: "x", Dir: e.folder(t, "elsewhere")}}}})
	if params, _ := body["params"].(map[string]any); st != 400 || body["code"] != "task_dir_in_worktree" || params["name"] != "bob" || params["dir"] != wt {
		t.Fatalf("want 400 task_dir_in_worktree naming bob, got %d %v", st, body)
	}
	if exists(filepath.Join(outer, ".git")) || exists(filepath.Join(e.root, "elsewhere", ".git")) || len(e.srv.Hub.Tasks()) != 1 {
		t.Fatal("a refused task should make no repository")
	}

	// the home folder and the folders above it don't become repositories
	home := e.folder(t, "users/me")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, dir := range []string{home, filepath.Dir(home)} {
		st, body := e.page.do("POST", "/api/tasks", advisor(dir))
		if params, _ := body["params"].(map[string]any); st != 400 || body["code"] != "task_dir_home" || params["dir"] != dir {
			t.Errorf("%s: want 400 task_dir_home, got %d %v", dir, st, body)
		}
	}
	// nor when they are spelled another way: through a symlink, in another case where the file system ignores it
	link := filepath.Join(e.root, "home-link")
	above := filepath.Join(e.root, "users-link")
	if err := os.Symlink(home, link); err != nil {
		t.Skip("no symlinks here:", err)
	}
	if err := os.Symlink(filepath.Dir(home), above); err != nil {
		t.Fatal(err)
	}
	spellings := []string{link, above, filepath.Join(above, "me")}
	if upper := filepath.Join(filepath.Dir(home), "ME"); exists(upper) {
		spellings = append(spellings, upper, filepath.Join(filepath.Dir(filepath.Dir(home)), "USERS", "me"))
	}
	for _, dir := range spellings {
		st, body := e.page.do("POST", "/api/tasks", advisor(dir))
		if st != 400 || body["code"] != "task_dir_home" {
			t.Errorf("%s: want 400 task_dir_home, got %d %v", dir, st, body)
		}
	}
	if exists(filepath.Join(home, ".git")) || exists(filepath.Join(filepath.Dir(home), ".git")) {
		t.Fatal("no repository of the home folder")
	}
}

// TestHoldsHome: the home folder and the folders above it, by any path to them; not the folders in it or beside it.
func TestHoldsHome(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "Users", "me")
	for _, d := range []string{home, filepath.Join(home, "proj"), filepath.Join(root, "Users", "meow"), filepath.Join(root, "data", "Users", "me")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// a second way to the same folders, as macOS's /System/Volumes/Data/Users is to /Users
	if err := os.Symlink(filepath.Join(root, "Users"), filepath.Join(root, "mirror")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	for dir, want := range map[string]bool{
		home:                                true,
		filepath.Join(root, "Users"):        true,
		root:                                true,
		filepath.Join(root, "mirror"):       true,
		filepath.Join(root, "mirror", "me"): true,
		filepath.Join(home, "proj"):         false,
		filepath.Join(root, "mirror", "me", "proj"): false,
		filepath.Join(root, "Users", "meow"):        false,
		filepath.Join(root, "data"):                 false, // its Users/me is another folder
		filepath.Join(root, "data", "Users", "me"):  false,
	} {
		if got := holdsHome(dir, home); got != want {
			t.Errorf("holdsHome(%s): got %v, want %v", dir, got, want)
		}
	}
	// home itself through a symlink: the folders above the real one count
	if err := os.Symlink(home, filepath.Join(root, "home-link")); err != nil {
		t.Fatal(err)
	}
	if !holdsHome(filepath.Join(root, "Users"), filepath.Join(root, "home-link")) || holdsHome(filepath.Join(home, "proj"), filepath.Join(root, "home-link")) {
		t.Error("a home reached through a symlink should be resolved")
	}
	if holdsHome(home, "") {
		t.Error("no home, no guard")
	}
}

// TestTaskMemberNeverComes: a member whose session doesn't report in before the launch times out has its process
// killed, so it doesn't keep the task running; the task goes on with the others.
func TestTaskMemberNeverComes(t *testing.T) {
	e := newTaskEnv(t, "")
	e.srv.Launcher.OnlineTimeout = 300 * time.Millisecond
	plain := e.folder(t, "plain")
	bob, bobAcc := as(t, e.srv, e.base, "bob")
	_, aliceAcc := as(t, e.srv, e.base, "alice")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: plain})
	e.srv.Hub.setLast(aliceAcc.ID, LaunchRequest{Dir: plain})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(1), Title: "t",
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Account: aliceAcc.ID}}}))
	b, a := task.Members[0], task.Members[1]
	bob.ok("POST", "/api/event", Event{Session: "b", Kind: "session.start", Launch: b.Launch, Task: task.ID})
	waitFor(t, "alice's process to be killed", func() bool { return !e.srv.Launcher.Terms.Live(a.Launch) })
	if e.srv.Hub.launchStatus(a.Launch) != LaunchFailed || taskState(e.srv.Hub, task.ID).Status != TaskRunning {
		t.Fatalf("alice's launch should fail and the task go on with bob: %+v", taskState(e.srv.Hub, task.ID))
	}
	bob.ok("POST", "/api/event", Event{Session: "b", Kind: "session.end"})
	e.starts.proc(b.Launch).exit(nil)
	waitFor(t, "the task to end", func() bool { return len(e.srv.Hub.Tasks()) == 0 })
}

// TestTaskMemberNamedLate: a mod before 1.1.1 could report a task member's session (its name, as the engine started it)
// before it named its launch and task: the bridge took it for a desk session. When its start names the task, it joins:
// it takes its member's place, gets the kickoff and binds its launch, an advisor's as an off-duty employee's, whose
// remembered folder stays out of the worktree and who has no desk session online.
func TestTaskMemberNamedLate(t *testing.T) {
	e := newTaskEnv(t, "")
	plain := e.folder(t, "plain")
	_, bobAcc := as(t, e.srv, e.base, "bob")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: plain})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(1), Title: "t", Leader: true,
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Advisor: &AdvisorRequest{Name: "helper", Dir: plain}}}}))
	for i, sid := range []string{"b1", "h1"} {
		m := task.Members[i]
		mod := e.modOf(t, m.Account)
		mod.ok("POST", "/api/event", Event{Session: sid, Kind: "session.title", Title: m.Name})
		if s := session(t, e.srv, sid); s.Task != "" {
			t.Fatalf("%s: a report that names no task is a desk session's: %q", m.Name, s.Task)
		}
		mod.ok("POST", "/api/event", Event{Session: sid, Kind: "session.start", Launch: m.Launch, Task: task.ID, Cwd: m.Worktree})
		mod.ok("POST", "/api/event", Event{Session: sid, Kind: "heartbeat", Task: task.ID, Cwd: m.Worktree})
		got := taskState(e.srv.Hub, task.ID).Members[i]
		kickoffs := 0
		for _, it := range e.srv.Hub.Chat(sid) {
			if it.Kind == ChatUser && it.Origin == "task" {
				kickoffs++
			}
		}
		if s := session(t, e.srv, sid); s.Task != task.ID || got.Session != sid || kickoffs != 1 || e.srv.Hub.launchStatus(m.Launch) != LaunchOnline {
			t.Fatalf("%s should join its task once its start names it: session task %q, member session %q, %d kickoffs, launch %s",
				m.Name, s.Task, got.Session, kickoffs, e.srv.Hub.launchStatus(m.Launch))
		}
	}
	if acc := e.srv.Hub.Account(bobAcc.ID); acc.Dir == task.Members[0].Worktree {
		t.Fatalf("bob's remembered folder should stay out of his worktree: %s", acc.Dir)
	}
	for _, a := range e.srv.Hub.Snapshot().Accounts {
		if a.ID == bobAcc.ID && a.Session != "" {
			t.Fatalf("bob has no desk session online: %s", a.Session)
		}
	}
}

// TestTaskMemberNamedLateReplaces: a session that names its task late takes the member's place from the account's
// session already in the task when that one runs in the same process (the same launch id: its end never came) or has
// been silent for StaleAfter; while another of the account's sessions works in the task, it stays a desk session.
func TestTaskMemberNamedLateReplaces(t *testing.T) {
	e := newTaskEnv(t, "")
	plain := e.folder(t, "plain")
	bob, bobAcc := as(t, e.srv, e.base, "bob")
	alice, aliceAcc := as(t, e.srv, e.base, "alice")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: plain})
	e.srv.Hub.setLast(aliceAcc.ID, LaunchRequest{Dir: plain})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(1), Title: "t", Leader: true,
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Account: aliceAcc.ID}}}))
	b, a := task.Members[0], task.Members[1]
	alice.ok("POST", "/api/event", Event{Session: "a1", Kind: "session.start", Launch: a.Launch, Task: task.ID, Cwd: a.Worktree})
	bob.ok("POST", "/api/event", Event{Session: "b1", Kind: "session.start", Launch: b.Launch, Task: task.ID, Cwd: b.Worktree})
	late := func(sid, launch string) *Session {
		bob.ok("POST", "/api/event", Event{Session: sid, Kind: "session.title", Title: b.Name}) // named no task yet
		bob.ok("POST", "/api/event", Event{Session: sid, Kind: "session.start", Launch: launch, Task: task.ID, Cwd: b.Worktree})
		return session(t, e.srv, sid)
	}
	// another process's session while b1 works: it stays a desk session, b1 keeps the place
	if s := late("b2", "elsewhere"); s.Task != "" || taskState(e.srv.Hub, task.ID).Members[0].Session != "b1" {
		t.Fatalf("a session of another process shouldn't take a working member's place: task %q, member %q", s.Task, taskState(e.srv.Hub, task.ID).Members[0].Session)
	}
	bob.ok("POST", "/api/event", Event{Session: "b2", Kind: "session.end"}) // the desk it stayed at
	// the same process (b1's end never came): it takes the place, b1 goes off duty
	if s := late("b3", b.Launch); s.Task != task.ID || taskState(e.srv.Hub, task.ID).Members[0].Session != "b3" {
		t.Fatalf("the same launch's session should take the member's place: task %q, member %q", s.Task, taskState(e.srv.Hub, task.ID).Members[0].Session)
	}
	for _, s := range e.srv.Hub.Snapshot().Sessions {
		if s.ID == "b1" && s.State != StateEnded {
			t.Fatalf("the session it replaced should be off duty or gone: %+v", s)
		}
	}
	// silent for StaleAfter: lost, and a late-named session takes the place
	bob.ok("POST", "/api/event", Event{Session: "b3", Kind: "heartbeat", Task: task.ID}) // stamped at the earlier time
	later := time.Now().Add(StaleAfter + time.Minute)
	e.srv.Hub.now = func() time.Time { return later }
	if s := late("b4", "elsewhere-too"); s.Task != task.ID || taskState(e.srv.Hub, task.ID).Members[0].Session != "b4" {
		t.Fatalf("a lost member's place should go to the late-named session: task %q, member %q", s.Task, taskState(e.srv.Hub, task.ID).Members[0].Session)
	}
}

// TestTaskSessionStartsAgain: a member's session that starts afresh after its last one ended (/clear) gets the kickoff
// again; a mod that doesn't report the task is known by its launch; a running task has no end time.
func TestTaskSessionStartsAgain(t *testing.T) {
	e := newTaskEnv(t, "")
	plain := e.folder(t, "plain")
	bob, bobAcc := as(t, e.srv, e.base, "bob")
	alice, aliceAcc := as(t, e.srv, e.base, "alice")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: plain})
	e.srv.Hub.setLast(aliceAcc.ID, LaunchRequest{Dir: plain})
	res := e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(1), Title: "t", Leader: true,
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Account: aliceAcc.ID}}})
	if _, ok := res["ended"]; ok {
		t.Fatalf("a running task has no end time: %v", res)
	}
	task := asTask(t, res)
	b, a := task.Members[0], task.Members[1]
	kickoffs := func(session string) int {
		n := 0
		for _, it := range e.srv.Hub.Chat(session) {
			if it.Kind == ChatUser && it.Origin == "task" {
				n++
			}
		}
		return n
	}

	bob.ok("POST", "/api/event", Event{Session: "b1", Kind: "session.start", Launch: b.Launch, Task: task.ID})
	// lost and back: nothing again
	e.srv.Hub.mu.Lock()
	e.srv.Hub.sessions["b1"].State = StateEnded
	e.srv.Hub.mu.Unlock()
	bob.ok("POST", "/api/event", Event{Session: "b1", Kind: "heartbeat", Task: task.ID})
	if n := kickoffs("b1"); n != 1 {
		t.Fatalf("a session back after being lost gets no second kickoff, got %d", n)
	}
	// /clear: the session ends and a new one starts in the same process
	bob.ok("POST", "/api/event", Event{Session: "b1", Kind: "session.end"})
	bob.ok("POST", "/api/event", Event{Session: "b2", Kind: "session.start", Launch: b.Launch, Task: task.ID})
	if n := kickoffs("b2"); n != 1 || taskState(e.srv.Hub, task.ID).Members[0].Session != "b2" {
		t.Fatalf("the new session should get the kickoff again, got %d", n)
	}
	// the same session id starting again gets it again too
	bob.ok("POST", "/api/event", Event{Session: "b2", Kind: "session.end"})
	bob.ok("POST", "/api/event", Event{Session: "b2", Kind: "session.start", Launch: b.Launch, Task: task.ID})
	if n := kickoffs("b2"); n != 2 {
		t.Fatalf("the session starting again should get the kickoff again, got %d", n)
	}
	if got := taskState(e.srv.Hub, task.ID); got.Status != TaskRunning {
		t.Fatalf("the task goes on: %+v", got)
	}
	// /clear with the new session reporting in before the old one's end: the launch id proves it's the same process,
	// so the old one ends and the new one takes over (and gets the kickoff)
	bob.ok("POST", "/api/event", Event{Session: "b3", Kind: "session.start", Launch: b.Launch, Task: task.ID})
	if n := kickoffs("b3"); n != 1 || taskState(e.srv.Hub, task.ID).Members[0].Session != "b3" || session(t, e.srv, "b3").State == StateEnded {
		t.Fatalf("the new session should take over and get the kickoff, got %d", n)
	}
	if e.srv.Hub.Online(bobAcc.ID) || slices.ContainsFunc(e.srv.Hub.Snapshot().Sessions, func(s *Session) bool { return s.ID == "b2" }) {
		t.Fatal("the session it replaced should be gone")
	}
	bob.ok("POST", "/api/event", Event{Session: "b2", Kind: "session.end"}) // the old one's goodbye, late: nothing
	if session(t, e.srv, "b3").State == StateEnded {
		t.Fatal("the old session's late end shouldn't end the new one")
	}
	// another process (no launch id, or another one) is still refused
	for _, l := range []string{"", "other"} {
		if st, code := bob.failure("POST", "/api/event", Event{Session: "b4", Kind: "session.start", Launch: l, Task: task.ID}); st != http.StatusConflict || code != "account_busy" {
			t.Fatalf("a session of another process (launch %q): want 409 account_busy, got %d %q", l, st, code)
		}
	}
	// a member's session pruned while it was away, back with an ask first: it is the task's session again
	e.srv.Hub.mu.Lock()
	e.srv.Hub.dropSessionLocked("b3")
	e.srv.Hub.mu.Unlock()
	if _, err := e.srv.Hub.CreateAsk(AskRequest{Session: "b3", Kind: "permission", Tool: "Bash"}, e.srv.Hub.Account(bobAcc.ID)); err != nil {
		t.Fatal(err)
	}
	if s := session(t, e.srv, "b3"); s.Task != task.ID || s.State != StateWaiting || e.srv.Hub.Online(bobAcc.ID) {
		t.Fatalf("the pruned member session should come back as the task's, not the desk's: %+v", s)
	}

	// an old mod: no task in its events, but the launch id tells
	alice.ok("POST", "/api/event", Event{Session: "a1", Kind: "session.start", Launch: a.Launch})
	if s := session(t, e.srv, "a1"); s.Task != task.ID || kickoffs("a1") != 1 || taskState(e.srv.Hub, task.ID).Members[1].Session != "a1" {
		t.Fatalf("alice's session should be taken for her task session: %+v", s)
	}
}

// TestTaskChat: messages between a task's members belong to the task, desk to desk to none; the task chat's input
// reaches one member with "@name", or every member online, and the message log keeps one message for it.
func TestTaskChat(t *testing.T) {
	e := newTaskEnv(t, "")
	plain := e.folder(t, "plain")
	bob, bobAcc := as(t, e.srv, e.base, "bob")
	alice, aliceAcc := as(t, e.srv, e.base, "alice")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: plain})
	e.srv.Hub.setLast(aliceAcc.ID, LaunchRequest{Dir: plain})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(1), Title: "t", Leader: true,
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Account: aliceAcc.ID}}}))
	b, a := task.Members[0], task.Members[1]
	bob.ok("POST", "/api/event", Event{Session: "b", Kind: "session.start", Launch: b.Launch, Task: task.ID})
	alice.ok("POST", "/api/event", Event{Session: "a", Kind: "session.start", Launch: a.Launch, Task: task.ID})
	bob.ok("POST", "/api/event", Event{Session: "bd", Kind: "session.start"})
	alice.ok("POST", "/api/event", Event{Session: "ad", Kind: "session.start"})
	wrap := func(from, text string) string {
		return "<cross-session-message from=\"uds:/x.sock\" from-name=\"" + from + "\" from-mode=\"prompting\">\n" + text + "\n</cross-session-message>"
	}
	// a pair between the members, sent first; a pair between the desks, received first
	bob.ok("POST", "/api/event", Event{Session: "b", Kind: "message.send", To: a.Name, Text: "take the tests", Delivered: true})
	alice.ok("POST", "/api/event", Event{Session: "a", Kind: "message.receive", Text: wrap(b.Name, "take the tests")})
	alice.ok("POST", "/api/event", Event{Session: "ad", Kind: "message.receive", Text: wrap("bob", "lunch?")})
	bob.ok("POST", "/api/event", Event{Session: "bd", Kind: "message.send", To: "alice", Text: "lunch?", Delivered: true})
	msgs := e.srv.Hub.Snapshot().Messages
	if len(msgs) != 2 || msgs[0].Task != task.ID || msgs[0].From != "b" || msgs[0].To != "a" || !msgs[0].Received ||
		msgs[1].Task != "" || msgs[1].From != "bd" || msgs[1].To != "ad" {
		t.Fatalf("the members' message belongs to the task, the desks' to none: %+v %+v", msgs[0], msgs[1])
	}

	// drain the kickoffs
	inbox := func(c client, session string) []string {
		var says []string
		for _, it := range c.ok("GET", "/api/inbox?session="+session, nil)["items"].([]any) {
			if it := it.(map[string]any); it["kind"] == "say" {
				says = append(says, it["text"].(string))
			}
		}
		return says
	}
	inbox(bob, "b")
	inbox(alice, "a")
	ch, stop := e.srv.Hub.Subscribe()
	defer stop()

	say := "/api/tasks/" + task.ID + "/say"
	m := e.page.ok("POST", say, map[string]any{"text": "  @" + a.Name + ": hello  "})
	if m["text"] != "hello" || m["to"] != "a" || m["toName"] != a.Name || m["task"] != task.ID || m["origin"] != "page" ||
		m["from"] != nil || m["fromName"] != "" || m["delivered"] != true || m["received"] != true || !slices.Equal(m["toNames"].([]any), []any{a.Name}) {
		t.Fatalf("say to a member: %v", m)
	}
	if got := inbox(alice, "a"); !slices.Equal(got, []string{"hello"}) {
		t.Fatalf("alice should get hello: %v", got)
	}
	if got := e.srv.Hub.Chat("a"); got[len(got)-1].Kind != ChatUser || got[len(got)-1].Text != "hello" || got[len(got)-1].Origin != "page" {
		t.Fatalf("alice's chat should show the prompt: %+v", got[len(got)-1])
	}
	// a member by its short name, as the page shows it ("alice" for "alice-T1"): the same member
	m = e.page.ok("POST", say, map[string]any{"text": "@" + strings.TrimSuffix(a.Name, "-"+task.ID) + " short"})
	if m["text"] != "short" || m["to"] != "a" || m["toName"] != a.Name || !slices.Equal(m["toNames"].([]any), []any{a.Name}) {
		t.Fatalf("say to a member by its short name: %v", m)
	}
	if got := inbox(alice, "a"); !slices.Equal(got, []string{"short"}) {
		t.Fatalf("alice should get short: %v", got)
	}
	waitFor(t, "the message event", func() bool {
		for {
			select {
			case p := <-ch:
				if ev, ok := p.Value.(map[string]any); ok && ev["type"] == "message" && ev["message"].(*Message).ID == m["id"] {
					return true
				}
			default:
				return false
			}
		}
	})

	m = e.page.ok("POST", say, map[string]any{"text": "status, everyone"})
	if m["text"] != "status, everyone" || m["to"] != nil || m["toName"] != "" || m["task"] != task.ID {
		t.Fatalf("say to everyone: %v", m)
	}
	if got := inbox(bob, "b"); !slices.Equal(got, []string{"status, everyone"}) {
		t.Fatalf("bob should get the broadcast: %v", got)
	}
	if got := inbox(alice, "a"); !slices.Equal(got, []string{"status, everyone"}) {
		t.Fatalf("alice should get the broadcast: %v", got)
	}
	if m["toNames"] != nil {
		t.Fatalf("a broadcast names nobody: %v", m)
	}
	// several mentions, one repeated: each named member once, in the order given
	m = e.page.ok("POST", say, map[string]any{"text": "@" + b.Name + ", @" + a.Name + "： @" + b.Name + " please sync"})
	if m["text"] != "please sync" || m["to"] != nil || m["toName"] != b.Name+" "+a.Name || !slices.Equal(m["toNames"].([]any), []any{b.Name, a.Name}) {
		t.Fatalf("say to several: %v", m)
	}
	if got := inbox(bob, "b"); !slices.Equal(got, []string{"please sync"}) {
		t.Fatalf("bob should get it once: %v", got)
	}
	if got := inbox(alice, "a"); !slices.Equal(got, []string{"please sync"}) {
		t.Fatalf("alice should get it: %v", got)
	}
	msgs = e.srv.Hub.Snapshot().Messages
	if len(msgs) != 6 || msgs[2].ToName != a.Name || msgs[3].Text != "short" || msgs[4].Text != "status, everyone" || msgs[5].Text != "please sync" {
		t.Fatalf("the log should keep one message per say: %+v", msgs)
	}
	if got := e.page.ok("GET", "/api/tasks/"+task.ID+"/chat", nil)["messages"].([]any); len(got) != 5 {
		t.Fatalf("the task's chat has the pair and the three says: %v", got)
	}

	// what goes wrong
	if st, out := e.page.do("POST", say, map[string]any{"text": "@" + a.Name + " @carol-" + task.ID + " @dave hi"}); st != http.StatusBadRequest ||
		out["code"] != "task_say_unknown_member" || out["params"].(map[string]any)["name"] != "carol-"+task.ID {
		t.Fatalf("unknown member: %d %v", st, out)
	}
	for _, text := range []string{"   ", "@" + a.Name + ","} {
		if st, code := e.page.failure("POST", say, map[string]any{"text": text}); st != http.StatusBadRequest || code != "say_empty" {
			t.Fatalf("empty %q: %d %s", text, st, code)
		}
	}
	if st, code := e.page.failure("POST", "/api/tasks/T999/say", map[string]any{"text": "hi"}); st != http.StatusNotFound || code != "task_unknown" {
		t.Fatalf("unknown task: %d %s", st, code)
	}
	cross := client{t: t, base: e.base, header: map[string]string{"origin": "http://evil.example"}}
	if st := cross.status("POST", say, map[string]any{"text": "hi"}); st != http.StatusForbidden {
		t.Fatalf("a cross-site say: %d", st)
	}
	alice.ok("POST", "/api/event", Event{Session: "a", Kind: "session.end"})
	if st, code := e.page.failure("POST", say, map[string]any{"text": "@" + a.Name + " hi"}); st != http.StatusConflict || code != "session_offline" {
		t.Fatalf("an offline member: %d %s", st, code)
	}
	e.page.ok("POST", say, map[string]any{"text": "just bob"})
	if got := inbox(bob, "b"); !slices.Equal(got, []string{"just bob"}) {
		t.Fatalf("bob alone should get it: %v", got)
	}
	m = e.page.ok("POST", say, map[string]any{"text": "@" + a.Name + " @" + b.Name + " bob again"})
	if got := inbox(bob, "b"); !slices.Equal(got, []string{"bob again"}) || m["toName"] != a.Name+" "+b.Name {
		t.Fatalf("of the members named, the online one gets it, and the message names both: %v %v", got, m)
	}
	bob.ok("POST", "/api/event", Event{Session: "b", Kind: "session.end"})
	if st, code := e.page.failure("POST", say, map[string]any{"text": "anyone?"}); st != http.StatusConflict || code != "session_offline" {
		t.Fatalf("nobody online: %d %s", st, code)
	}
	e.page.ok("POST", "/api/tasks/"+task.ID+"/end", nil)
	if st, code := e.page.failure("POST", say, map[string]any{"text": "hi"}); st != http.StatusConflict || code != "task_ended" {
		t.Fatalf("an ended task: %d %s", st, code)
	}
}

func TestSplitMentions(t *testing.T) {
	members := []string{"alice-T1", "bob-T1", "小 王-T1", "John Smith-T1", "al", "al ice"}
	for _, tc := range []struct {
		text    string
		names   []string
		unknown string
		rest    string
	}{
		{"hello", nil, "", "hello"},
		{"@alice-T1 hi", []string{"alice-T1"}, "", "hi"},
		{"@alice-T1", []string{"alice-T1"}, "", ""},
		{"@alice-T1：你好", []string{"alice-T1"}, "", "你好"},
		{"@alice-T1:x", []string{"alice-T1"}, "", "x"},
		{"@alice-T1,@bob-T1 x", []string{"alice-T1", "bob-T1"}, "", "x"},
		{"@alice-T1， @bob-T1: x", []string{"alice-T1", "bob-T1"}, "", "x"},
		{"@小 王-T1 你好", []string{"小 王-T1"}, "", "你好"},
		{"@John Smith-T1,\n@小 王-T1：hi", []string{"John Smith-T1", "小 王-T1"}, "", "hi"},
		{"@alice-T1 @bob-T1 @alice-T1 sync", []string{"alice-T1", "bob-T1"}, "", "sync"},
		// the longest name that ends there wins; one that doesn't end there doesn't count
		{"@al ice x", []string{"al ice"}, "", "x"},
		{"@al icy x", []string{"al"}, "", "icy x"},
		{"@alice-T1x hi", nil, "", "@alice-T1x hi"},
		// a bare @, or one no member follows, is text and ends the mentions
		{"@ hi", nil, "", "@ hi"},
		{"@", nil, "", "@"},
		{"@alice-T1 @dave hi", []string{"alice-T1"}, "", "@dave hi"},
		{"@bob-T1 mail me @ home", []string{"bob-T1"}, "", "mail me @ home"},
		// a name of the task's form that is no member's is unknown, wherever in the mentions
		{"@carol-T1 hi", nil, "carol-T1", "@carol-T1 hi"},
		{"@alice-T1 @carol-T1: hi", []string{"alice-T1"}, "carol-T1", "@carol-T1: hi"},
		{"@Jon Smith-T1 hi", nil, "Jon Smith-T1", "@Jon Smith-T1 hi"},
		{"@carol-T12 hi", nil, "", "@carol-T12 hi"},
		{"@see\nnotes-T1 hi", nil, "", "@see\nnotes-T1 hi"},
	} {
		names, unknown, rest := splitMentions(tc.text, members, "-T1")
		if !slices.Equal(names, tc.names) || unknown != tc.unknown || rest != tc.rest {
			t.Errorf("%q: got %q %q %q, want %q %q %q", tc.text, names, unknown, rest, tc.names, tc.unknown, tc.rest)
		}
	}
}

// TestTaskChatMentions: members whose names have spaces can be mentioned, marks right after a name work without a
// space, and a message that some of the members named didn't get says which.
func TestTaskChatMentions(t *testing.T) {
	e := newTaskEnv(t, "")
	plain := e.folder(t, "plain")
	bob, bobAcc := as(t, e.srv, e.base, "bob")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: plain})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Room: hint(1), Title: "t",
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Advisor: &AdvisorRequest{Name: "小 王", Dir: plain}}, {Advisor: &AdvisorRequest{Name: "John Smith", Dir: plain}}}}))
	b, w, j := task.Members[0], task.Members[1], task.Members[2]
	if w.Name != "小 王-"+task.ID || j.Name != "John Smith-"+task.ID {
		t.Fatalf("the members' names: %+v %+v", w, j)
	}
	wang := e.modOf(t, w.Account)
	bob.ok("POST", "/api/event", Event{Session: "b", Kind: "session.start", Launch: b.Launch, Task: task.ID})
	wang.ok("POST", "/api/event", Event{Session: "w", Kind: "session.start", Launch: w.Launch, Task: task.ID})
	inbox := func(c client, session string) []string {
		var says []string
		for _, it := range c.ok("GET", "/api/inbox?session="+session, nil)["items"].([]any) {
			if it := it.(map[string]any); it["kind"] == "say" && it["origin"] != "task" {
				says = append(says, it["text"].(string))
			}
		}
		return says
	}
	inbox(bob, "b")
	inbox(wang, "w")

	say := "/api/tasks/" + task.ID + "/say"
	m := e.page.ok("POST", say, map[string]any{"text": "@" + w.Name + "：你好"})
	if m["text"] != "你好" || m["to"] != "w" || !slices.Equal(m["toNames"].([]any), []any{w.Name}) || m["missed"] != nil {
		t.Fatalf("a name with a space and a mark right after it: %v", m)
	}
	if got := inbox(wang, "w"); !slices.Equal(got, []string{"你好"}) {
		t.Fatalf("小 王 should get it: %v", got)
	}
	// John Smith has no session online: the others named get it, and the message says who didn't
	m = e.page.ok("POST", say, map[string]any{"text": "@" + b.Name + ",@" + j.Name + " @" + w.Name + ": sync"})
	if m["text"] != "sync" || m["toName"] != b.Name+" "+j.Name+" "+w.Name || !slices.Equal(m["missed"].([]any), []any{j.Name}) {
		t.Fatalf("partial delivery: %v", m)
	}
	if got := inbox(bob, "b"); !slices.Equal(got, []string{"sync"}) {
		t.Fatalf("bob should get it: %v", got)
	}
	if got := inbox(wang, "w"); !slices.Equal(got, []string{"sync"}) {
		t.Fatalf("小 王 should get it: %v", got)
	}
	if msgs := e.srv.Hub.TaskChat(task.ID); !slices.Equal(msgs[len(msgs)-1].Missed, []string{j.Name}) {
		t.Fatalf("the logged message should name who missed it: %+v", msgs[len(msgs)-1])
	}
	// an @ no member follows is text: it ends the mentions
	m = e.page.ok("POST", say, map[string]any{"text": "@" + b.Name + " @everyone look"})
	if m["text"] != "@everyone look" || !slices.Equal(m["toNames"].([]any), []any{b.Name}) {
		t.Fatalf("an unmatched @ is text: %v", m)
	}
	if st, code := e.page.failure("POST", say, map[string]any{"text": "@" + j.Name + " hi"}); st != http.StatusConflict || code != "session_offline" {
		t.Fatalf("only offline members named: %d %s", st, code)
	}
}

// TestMessageLogKeepsTaskChats: the message log keeps keepMessages messages of each task the office shows besides as
// many others, so desk chatter doesn't push a team chat out; a task gone from the office counts with the others.
func TestMessageLogKeepsTaskChats(t *testing.T) {
	h := NewHub()
	h.tasks = []*Task{{ID: "T1", Status: TaskRunning}, {ID: "T2", Status: TaskFailed}, {ID: "T3", Status: TaskEnded}}
	add := func(task string, n int) {
		for range n {
			h.pushMessageLocked(&Message{ID: newID(), Task: task})
		}
	}
	count := func() map[string]int {
		c := map[string]int{}
		for _, m := range h.messages {
			c[m.Task]++
		}
		return c
	}
	add("T1", 10)
	add("T2", 5)
	add("T3", 50)
	add("", keepMessages)
	if c := count(); c["T1"] != 10 || c["T2"] != 5 || c["T3"] != 0 || c[""] != keepMessages {
		t.Fatalf("the shown tasks' messages stay; of the others the latest %d (the ended task's, older, go): %v", keepMessages, c)
	}
	add("T1", keepMessages+20)
	if c := count(); c["T1"] != keepMessages || c["T2"] != 5 || c[""] != keepMessages {
		t.Fatalf("a shown task keeps its latest %d: %v", keepMessages, c)
	}
	last := h.messages[len(h.messages)-1]
	if last.Task != "T1" || len(h.messages) != 2*keepMessages+5 {
		t.Fatalf("the log keeps its order and the latest message: %d", len(h.messages))
	}
	// once T1 is gone from the office, its messages count with the others
	h.tasks[0].Status = TaskEnded
	add("", 1)
	if c := count(); c["T1"]+c[""] != keepMessages || c["T2"] != 5 {
		t.Fatalf("an ended task's messages count with the others: %v", c)
	}
}
