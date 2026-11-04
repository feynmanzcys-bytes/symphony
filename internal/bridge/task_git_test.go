package bridge

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
)

// warnings records what the hub writes to the bridge's log.
type warnings struct {
	mu    sync.Mutex
	lines []string
}

func (w *warnings) catch(h *Hub) { h.Warn = w.add }

func (w *warnings) add(line string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lines = append(w.lines, line)
}

func (w *warnings) all() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.lines)
}

// globalGitConfig makes a git config of the user's for the rest of the test (GIT_CONFIG_GLOBAL), with these lines.
func globalGitConfig(t *testing.T, lines ...string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gitconfig")
	writeFile(t, path, strings.Join(lines, "\n")+"\n")
	t.Setenv("GIT_CONFIG_GLOBAL", path)
}

// TestTaskMergesWhateverTheGitConfig: the user's own git config doesn't keep a task's branches from merging into the
// folder made a repository for it: not merge.ff=only (the second branch needs a merge commit), signatures to verify,
// nor hooks of a global core.hooksPath that reject every commit message (commit-msg, prepare-commit-msg, the merge's
// own pre-merge-commit) or every commit (pre-commit).
func TestTaskMergesWhateverTheGitConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hooks are shell scripts")
	}
	hooks := t.TempDir()
	for _, hook := range []string{"pre-commit", "commit-msg", "prepare-commit-msg", "pre-merge-commit"} {
		writeFile(t, filepath.Join(hooks, hook), "#!/bin/sh\necho \""+hook+": rejected\" >&2\nexit 1\n")
		if err := os.Chmod(filepath.Join(hooks, hook), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, config := range map[string][]string{
		"merge.ff=only, verifySignatures": {"[merge]", "\tff = only", "\tverifySignatures = true"},
		"hooks that reject":               {"[core]", "\thooksPath = " + filepath.ToSlash(hooks)},
		"no merge commits":                {"[merge]", "\tff = false"},
		"commits are signed":              {"[commit]", "\tgpgSign = true", "[gpg]", "\tprogram = false"},
	} {
		t.Run(name, func(t *testing.T) {
			globalGitConfig(t, config...)
			e := newTaskEnv(t, "")
			plain := e.folder(t, "plain")
			writeFile(t, filepath.Join(plain, "notes.txt"), "notes\n")
			_, bobAcc := as(t, e.srv, e.base, "bob")
			_, aliceAcc := as(t, e.srv, e.base, "alice")
			e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: plain})
			e.srv.Hub.setLast(aliceAcc.ID, LaunchRequest{Dir: plain})
			task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Git: true, Title: "t",
				Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Account: aliceAcc.ID}}}))
			writeFile(t, filepath.Join(task.Members[0].Worktree, "bob.txt"), "bob's\n")
			writeFile(t, filepath.Join(task.Members[1].Worktree, "alice.txt"), "alice's\n")
			e.page.ok("POST", "/api/tasks/"+task.ID+"/end", nil)
			e.srv.Hub.Flush()
			if got := taskState(e.srv.Hub, task.ID); got != nil && got.Status != TaskEnded {
				t.Fatalf("the task should have ended: %s %s", got.ErrorCode, got.Error)
			}
			if readFile(t, filepath.Join(plain, "bob.txt")) != "bob's\n" || readFile(t, filepath.Join(plain, "alice.txt")) != "alice's\n" ||
				mustGit(t, plain, "status", "--porcelain") != "" {
				t.Fatal("both branches should be merged into the folder, its checkout clean")
			}
		})
	}
}

// TestMergeBranches: a merge that fails takes back the merges before it, keeping what is uncommitted in the folder
// (never reset --hard: the folder is the user's); a conflict is task_merge_conflict, anything else task_git_failed with
// git's words.
func TestMergeBranches(t *testing.T) {
	needGit(t)
	// alice and carol change the same line, bob adds a file; all from the same commit
	setup := func(t *testing.T) (repo, before string) {
		repo = userRepo(t, map[string]string{"a.txt": "one\n", "notes.txt": "notes\n"})
		base, _ := gitBase(repo)
		for who, file := range map[string]string{"alice": "a.txt", "carol": "a.txt", "bob": "bob.txt"} {
			wt := filepath.Join(t.TempDir(), who)
			if err := gitAddWorktree(repo, "symphony/T1/"+who, wt, base); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(wt, file), who+"\n")
			if _, err := gitCommitAll(wt, who); err != nil {
				t.Fatal(err)
			}
			if err := gitRemoveWorktree(repo, wt); err != nil {
				t.Fatal(err)
			}
		}
		return repo, mustGit(t, repo, "rev-parse", "HEAD")
	}

	t.Run("a conflict", func(t *testing.T) {
		repo, before := setup(t)
		writeFile(t, filepath.Join(repo, "notes.txt"), "the user's edit\n") // uncommitted, in a file no branch touches
		e := mergeBranches(repo, []string{"symphony/T1/alice", "symphony/T1/bob", "symphony/T1/carol"})
		if e == nil || e.Code != "task_merge_conflict" || e.Params["branch"] != "symphony/T1/carol" || !strings.Contains(e.Text, "CONFLICT") {
			t.Fatalf("carol's merge should fail on a conflict: %+v", e)
		}
		if mustGit(t, repo, "rev-parse", "HEAD") != before || readFile(t, filepath.Join(repo, "a.txt")) != "one\n" || exists(filepath.Join(repo, "bob.txt")) {
			t.Fatal("alice's and bob's merges should be taken back: nothing was merged")
		}
		if readFile(t, filepath.Join(repo, "notes.txt")) != "the user's edit\n" {
			t.Fatal("taking the merges back must keep the user's uncommitted edit")
		}
		if exists(filepath.Join(repo, ".git", "MERGE_HEAD")) {
			t.Fatal("no merge should be in progress")
		}
	})

	t.Run("not a conflict", func(t *testing.T) {
		repo, before := setup(t)
		writeFile(t, filepath.Join(repo, "bob.txt"), "the user's own bob.txt\n") // untracked, where bob's branch has one
		e := mergeBranches(repo, []string{"symphony/T1/alice", "symphony/T1/bob"})
		if e == nil || e.Code != "task_git_failed" || e.Params["dir"] != repo || !strings.Contains(e.Params["detail"], "symphony/T1/bob") ||
			!strings.Contains(e.Params["detail"], "untracked working tree files would be overwritten") {
			t.Fatalf("a merge git refuses is no conflict, and the error has git's words: %+v", e)
		}
		if mustGit(t, repo, "rev-parse", "HEAD") != before || readFile(t, filepath.Join(repo, "bob.txt")) != "the user's own bob.txt\n" {
			t.Fatal("alice's merge should be taken back, the user's file left alone")
		}
	})

	t.Run("all merge", func(t *testing.T) {
		repo, _ := setup(t)
		if e := mergeBranches(repo, []string{"symphony/T1/alice", "symphony/T1/bob"}); e != nil {
			t.Fatal(e)
		}
		if readFile(t, filepath.Join(repo, "a.txt")) != "alice\n" || readFile(t, filepath.Join(repo, "bob.txt")) != "bob\n" {
			t.Fatal("both branches should be merged")
		}
	})
}

// TestGitResetKeep: going back keeps what is uncommitted; where it can't (a file changed in the commits it goes back
// over), git refuses and the checkout and the edit stay as they are.
func TestGitResetKeep(t *testing.T) {
	needGit(t)
	repo := userRepo(t, map[string]string{"a.txt": "one\n", "b.txt": "b\n"})
	before := mustGit(t, repo, "rev-parse", "HEAD")
	writeFile(t, filepath.Join(repo, "a.txt"), "two\n")
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, append(gitCommitter, "commit", "-q", "-m", "two")...)
	after := mustGit(t, repo, "rev-parse", "HEAD")

	writeFile(t, filepath.Join(repo, "a.txt"), "the user's edit\n")
	if err := gitResetKeep(repo, before); err == nil {
		t.Fatal("a.txt changed since then and has an edit: going back should be refused")
	}
	if mustGit(t, repo, "rev-parse", "HEAD") != after || readFile(t, filepath.Join(repo, "a.txt")) != "the user's edit\n" {
		t.Fatal("a refused reset leaves the checkout and the edit")
	}
	mustGit(t, repo, "checkout", "-q", "--", "a.txt")
	writeFile(t, filepath.Join(repo, "b.txt"), "the user's edit\n")
	if err := gitResetKeep(repo, before); err != nil {
		t.Fatal(err)
	}
	if mustGit(t, repo, "rev-parse", "HEAD") != before || readFile(t, filepath.Join(repo, "a.txt")) != "one\n" ||
		readFile(t, filepath.Join(repo, "b.txt")) != "the user's edit\n" {
		t.Fatal("going back keeps the edit to a file the commits didn't change")
	}
}

// TestTaskEndKeepsEveryFailure: when the git work of ending a task fails for several members, the task's error tells
// them all (the first is its code), and the log has every one.
func TestTaskEndKeepsEveryFailure(t *testing.T) {
	e := newTaskEnv(t, "")
	var w warnings
	w.catch(e.srv.Hub)
	one := initRepo(t, e.folder(t, "one"), map[string]string{"a.txt": "a\n"})
	two := initRepo(t, e.folder(t, "two"), map[string]string{"a.txt": "a\n"})
	_, bobAcc := as(t, e.srv, e.base, "bob")
	_, aliceAcc := as(t, e.srv, e.base, "alice")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: one})
	e.srv.Hub.setLast(aliceAcc.ID, LaunchRequest{Dir: two})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Git: true, Title: "t",
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Account: aliceAcc.ID}}}))
	for _, m := range task.Members {
		// work left uncommitted, and a lock left behind by a git that was killed: the work can't be saved
		writeFile(t, filepath.Join(m.Worktree, "work.txt"), m.Name+"\n")
		index := mustGit(t, m.Worktree, "rev-parse", "--git-path", "index")
		if !filepath.IsAbs(index) {
			index = filepath.Join(m.Worktree, index)
		}
		writeFile(t, index+".lock", "")
	}
	e.page.ok("POST", "/api/tasks/"+task.ID+"/end", nil)
	e.srv.Hub.Flush()
	got := taskState(e.srv.Hub, task.ID)
	if got == nil || got.Status != TaskFailed || got.ErrorCode != "task_git_failed" || got.ErrorParams["dir"] != task.Members[0].Worktree {
		t.Fatalf("the task should fail on bob's worktree first: %+v", got)
	}
	for _, m := range task.Members {
		if !strings.Contains(got.Error, m.Worktree) {
			t.Errorf("the task's error should tell %s's failure too:\n%s", m.Name, got.Error)
		}
		if !slices.ContainsFunc(w.all(), func(l string) bool { return strings.Contains(l, task.ID) && strings.Contains(l, m.Worktree) }) {
			t.Errorf("the log should tell %s's failure: %q", m.Name, w.all())
		}
		if !isDir(m.Worktree) {
			t.Errorf("%s's worktree should stay, with the work in it", m.Name)
		}
	}
}

// TestGitInWorkTree: only a folder with no .git around it is outside a repository; when git can't read or won't
// trust a repository around a folder, that is an error, not a plain folder.
func TestGitInWorkTree(t *testing.T) {
	needGit(t)
	plain := filepath.Join(t.TempDir(), "plain", "sub")
	repo := userRepo(t, map[string]string{"web/index.html": "hi\n"})
	broken := t.TempDir()
	writeFile(t, filepath.Join(broken, ".git"), "gitdir: "+filepath.Join(broken, "moved", ".git")+"\n") // its repository moved away
	writeFile(t, filepath.Join(broken, "web", "index.html"), "hi\n")
	for _, d := range []string{plain} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if in, err := gitInWorkTree(plain); in || err != nil {
		t.Fatalf("a plain folder: %v %v", in, err)
	}
	if in, err := gitInWorkTree(filepath.Join(repo, "web")); !in || err != nil {
		t.Fatalf("a folder in a repository: %v %v", in, err)
	}
	in, err := gitInWorkTree(filepath.Join(broken, "web"))
	if in || err == nil || !strings.Contains(err.Error(), filepath.Join(broken, ".git")) {
		t.Fatalf("a folder below a .git git can't read: want an error naming it, got %v %v", in, err)
	}
	if plain, err := gitPlain(filepath.Join(broken, "web")); plain || err == nil {
		t.Fatalf("a folder below a .git git can't read isn't plain: %v %v", plain, err)
	}

	// a repository git won't trust (someone else's, as git sees it): an error too
	t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")
	if _, err := runGit(filepath.Join(repo, "web"), "rev-parse", "--is-inside-work-tree"); err == nil {
		t.Skip("this git doesn't check who owns a repository")
	}
	in, err = gitInWorkTree(filepath.Join(repo, "web"))
	if in || err == nil || !strings.Contains(err.Error(), "dubious ownership") {
		t.Fatalf("a folder in a repository git won't trust: want git's words, got %v %v", in, err)
	}
}

// TestTaskInBrokenRepository: a member's folder inside a repository git can't read doesn't become a repository of its
// own: the task fails with git's words and makes nothing.
func TestTaskInBrokenRepository(t *testing.T) {
	e := newTaskEnv(t, "")
	project := e.folder(t, "project")
	web := e.folder(t, "project/web")
	writeFile(t, filepath.Join(project, ".git"), "gitdir: "+filepath.Join(e.root, "gone", ".git")+"\n")
	writeFile(t, filepath.Join(web, "index.html"), "hi\n")
	st, body := e.page.do("POST", "/api/tasks", TaskRequest{Git: true, Title: "t", Members: []TaskMemberRequest{
		{Advisor: &AdvisorRequest{Name: "x", Dir: web}}, {Advisor: &AdvisorRequest{Name: "y", Dir: web}}}})
	if params, _ := body["params"].(map[string]any); st != 500 || body["code"] != "task_git_failed" || params["dir"] != web ||
		!strings.Contains(params["detail"].(string), filepath.Join(project, ".git")) {
		t.Fatalf("want 500 task_git_failed naming the .git, got %d %v", st, body)
	}
	if exists(filepath.Join(web, ".git")) || exists(filepath.Join(web, symphonyDir)) || len(e.srv.Hub.Tasks()) != 0 {
		t.Fatal("no repository inside the broken one, and no task")
	}
}

// TestTaskUndoLogs: what can't be taken back when a task fails to start is in the log, not dropped.
func TestTaskUndoLogs(t *testing.T) {
	needGit(t)
	hub := NewHub()
	var w warnings
	w.catch(hub)
	repo := userRepo(t, map[string]string{"a.txt": "a\n"})
	base, _ := gitBase(repo)
	ok := &Member{Dir: repo, Branch: "symphony/T9/bob", Worktree: filepath.Join(repo, symphonyDir, "worktrees", "bob-T9")}
	if err := gitAddWorktree(repo, ok.Branch, ok.Worktree, base); err != nil {
		t.Fatal(err)
	}
	plain := t.TempDir() // no repository: its worktree can't be removed
	gone := &Member{Dir: plain, Branch: "symphony/T9/alice", Worktree: filepath.Join(plain, symphonyDir, "worktrees", "alice-T9")}
	if err := os.MkdirAll(gone.Worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	// a member whose worktree add failed before it made anything: nothing to take back, nothing to log
	never := &Member{Dir: repo, Branch: "symphony/T9/carol", Worktree: filepath.Join(repo, symphonyDir, "worktrees", "carol-T9")}
	u := &taskUndo{worktrees: []*Member{ok, gone, never}, advisors: []string{"nobody"}}
	u.run(hub, nil, "T9")
	if exists(ok.Worktree) || mustGit(t, repo, "branch", "--list", ok.Branch) != "" {
		t.Fatal("bob's worktree and branch should be taken back")
	}
	lines := w.all()
	for _, want := range []string{"git failed in " + plain, "deleting advisor nobody"} {
		if !slices.ContainsFunc(lines, func(l string) bool { return strings.Contains(l, "T9") && strings.Contains(l, want) }) {
			t.Errorf("the log should say %q: %q", want, lines)
		}
	}
	if slices.ContainsFunc(lines, func(l string) bool { return strings.Contains(l, repo+":") }) {
		t.Errorf("what was taken back isn't in the log: %q", lines)
	}
}

// TestTaskUndoAfterFailedWorktreeAdd: a global post-checkout hook that fails makes git worktree add exit 1 after it
// made the worktree and the branch; the task doesn't start, and neither stays behind.
func TestTaskUndoAfterFailedWorktreeAdd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hooks are shell scripts")
	}
	hooks := t.TempDir()
	writeFile(t, filepath.Join(hooks, "post-checkout"), "#!/bin/sh\necho \"post-checkout: rejected\" >&2\nexit 1\n")
	if err := os.Chmod(filepath.Join(hooks, "post-checkout"), 0o755); err != nil {
		t.Fatal(err)
	}
	globalGitConfig(t, "[core]", "\thooksPath = "+filepath.ToSlash(hooks))
	e := newTaskEnv(t, "")
	repo := userRepo(t, map[string]string{"a.txt": "a\n"})
	_, bobAcc := as(t, e.srv, e.base, "bob")
	_, aliceAcc := as(t, e.srv, e.base, "alice")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: repo})
	e.srv.Hub.setLast(aliceAcc.ID, LaunchRequest{Dir: repo})
	st, body := e.page.do("POST", "/api/tasks", TaskRequest{Git: true, Title: "t",
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Account: aliceAcc.ID}}})
	if st != 500 || body["code"] != "task_git_failed" || !strings.Contains(fmt.Sprint(body["params"]), "post-checkout") {
		t.Fatalf("want 500 task_git_failed from the hook, got %d %v", st, body)
	}
	if got := mustGit(t, repo, "branch", "--list", "symphony/T1/*"); got != "" {
		t.Fatalf("the members' branches should be gone: %q", got)
	}
	if got := mustGit(t, repo, "worktree", "list", "--porcelain"); strings.Contains(got, symphonyDir) {
		t.Fatalf("the members' worktrees should be gone: %q", got)
	}
	if exists(filepath.Join(repo, symphonyDir)) {
		t.Fatal("the .symphony folder should be gone")
	}
}
