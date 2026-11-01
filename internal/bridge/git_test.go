package bridge

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := runGit(dir, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// userRepo is a repository of the user's own with these files in a first commit.
func userRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	return initRepo(t, t.TempDir(), files)
}

func initRepo(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	mustGit(t, dir, "init", "-q")
	for name, body := range files {
		writeFile(t, filepath.Join(dir, name), body)
	}
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, append(gitCommitter, "commit", "-q", "-m", "first")...)
	return dir
}

// TestGitInit: a plain folder becomes a repository with everything in it committed, on the branch git picks.
func TestGitInit(t *testing.T) {
	needGit(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "notes.txt"), "hello")
	writeFile(t, filepath.Join(dir, "src", "main.go"), "package main")
	if in, err := gitInWorkTree(dir); in || err != nil {
		t.Fatalf("a plain folder is not in a work tree: %v %v", in, err)
	}
	if err := gitInit(dir, "T3"); err != nil {
		t.Fatal(err)
	}
	if files := mustGit(t, dir, "ls-files"); files != "notes.txt\nsrc/main.go" {
		t.Fatalf("everything should be committed: %q", files)
	}
	if msg := mustGit(t, dir, "log", "-1", "--format=%s %an"); msg != "symphony: files before task T3 symphony" {
		t.Fatalf("wrong first commit: %q", msg)
	}
	base, err := gitBase(dir)
	if want := mustGit(t, dir, "branch", "--show-current"); err != nil || base != want || base == "" {
		t.Fatalf("base %q (%v), want the branch git chose, %q", base, err, want)
	}
	if in, err := gitInWorkTree(filepath.Join(dir, "src")); !in || err != nil {
		t.Fatalf("a folder inside a repository is in its work tree: %v %v", in, err)
	}
	root, prefix, err := gitRoot(filepath.Join(dir, "src"))
	if err != nil || prefix != "src/" || !sameDir(root, dir) {
		t.Fatalf("root %q, prefix %q (%v)", root, prefix, err)
	}

	// an empty folder works too; a repository without a commit has no base
	empty := t.TempDir()
	if err := gitInit(empty, "T4"); err != nil {
		t.Fatalf("an empty folder should get an empty first commit: %v", err)
	}
	bare := t.TempDir()
	mustGit(t, bare, "init", "-q")
	if _, err := gitBase(bare); err == nil {
		t.Fatal("a repository without a commit has no base")
	}
}

// TestGitInitLeavesSecretsOut: a folder made a repository doesn't commit secrets, keys, node_modules or .symphony: they
// stay in the folder, listed in the repository's own exclude file (each once), not in a .gitignore of the user's.
func TestGitInitLeavesSecretsOut(t *testing.T) {
	needGit(t)
	dir := t.TempDir()
	left := []string{".env", ".env.local", "server.key", "cert.pem", "store.p12", "id_ed25519", "id_rsa.pub", "sub/.npmrc", ".netrc",
		"node_modules/x/index.js", ".DS_Store", ".symphony/kept/bob-T1/out/app.bin"}
	for _, name := range append([]string{"main.go", "sub/notes.txt"}, left...) {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), "x")
	}
	if err := gitInit(dir, "T1"); err != nil {
		t.Fatal(err)
	}
	if files := mustGit(t, dir, "ls-files"); files != "main.go\nsub/notes.txt" {
		t.Fatalf("only the plain files should be committed: %q", files)
	}
	for _, name := range left {
		if !exists(filepath.Join(dir, filepath.FromSlash(name))) {
			t.Fatalf("%s should stay in the folder", name)
		}
	}
	if st := mustGit(t, dir, "status", "--porcelain"); st != "" || exists(filepath.Join(dir, ".gitignore")) {
		t.Fatalf("the left-out files are ignored through the exclude file: %q", st)
	}
	exclude := filepath.Join(dir, ".git", "info", "exclude")
	before := readFile(t, exclude)
	if err := gitExclude(dir); err != nil {
		t.Fatal(err)
	}
	after := readFile(t, exclude)
	lines := strings.Split(after, "\n")
	for _, p := range append([]string{".symphony/"}, initExcludes...) {
		if n := slices.Index(lines, p); n < 0 || slices.Index(lines[n+1:], p) >= 0 {
			t.Fatalf("%s should be in the exclude file once: %q", p, after)
		}
	}
	if after != before {
		t.Fatalf("listing .symphony/ again should change nothing: %q", after)
	}
}

func sameDir(a, b string) bool {
	ra, _ := filepath.EvalSymlinks(a)
	rb, _ := filepath.EvalSymlinks(b)
	return ra == rb
}

// TestGitWorktreeLifecycle: a worktree on a branch of its own, .symphony/ excluded, uncommitted work committed, the
// worktree removed with the branch kept, branches merged, and a conflicting merge aborted.
func TestGitWorktreeLifecycle(t *testing.T) {
	needGit(t)
	repo := userRepo(t, map[string]string{"a.txt": "one\n", ".gitignore": "*.log\n"})
	base, _ := gitBase(repo)

	for range 2 {
		if err := gitExclude(repo); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if strings.Count(string(data), ".symphony/\n") != 1 {
		t.Fatalf(".symphony/ should be in the exclude file once: %q", data)
	}
	if ignore, _ := os.ReadFile(filepath.Join(repo, ".gitignore")); string(ignore) != "*.log\n" {
		t.Fatalf("the user's .gitignore must stay as it was: %q", ignore)
	}

	wt := filepath.Join(repo, symphonyDir, "worktrees", "bob-T1")
	if err := gitAddWorktree(repo, "symphony/T1/bob", wt, base); err != nil {
		t.Fatal(err)
	}
	if st := mustGit(t, repo, "status", "--porcelain"); st != "" {
		t.Fatalf("the worktrees must not show in the user's checkout: %q", st)
	}
	if committed, err := gitCommitAll(wt, "nothing"); committed || err != nil {
		t.Fatalf("a clean worktree has nothing to commit: %v %v", committed, err)
	}
	writeFile(t, filepath.Join(wt, "b.txt"), "bob's\n")
	writeFile(t, filepath.Join(wt, "build.log"), "ignored")
	if committed, err := gitCommitAll(wt, "symphony: bob-T1's uncommitted work"); !committed || err != nil {
		t.Fatalf("the new file should be committed: %v %v", committed, err)
	}
	if err := gitRemoveWorktree(repo, wt); err != nil {
		t.Fatal(err)
	}
	if isDir(wt) || strings.Contains(mustGit(t, repo, "worktree", "list"), "bob-T1") {
		t.Fatal("the worktree should be gone")
	}
	if body := mustGit(t, repo, "show", "symphony/T1/bob:b.txt"); body != "bob's" {
		t.Fatalf("the branch keeps the work: %q", body)
	}
	removeIfEmpty(filepath.Join(repo, symphonyDir, "worktrees"), filepath.Join(repo, symphonyDir))
	if _, err := os.Stat(filepath.Join(repo, symphonyDir)); !os.IsNotExist(err) {
		t.Fatal(".symphony should go once empty")
	}

	// merges: the branch comes in; a conflict is aborted and leaves the checkout clean
	if _, err := gitMerge(repo, "symphony/T1/bob"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "b.txt")); err != nil {
		t.Fatal("the merge should bring b.txt")
	}
	for _, who := range []string{"alice", "carol"} { // both from the same commit, both changing the same line
		wt := filepath.Join(repo, symphonyDir, "worktrees", who)
		if err := gitAddWorktree(repo, "symphony/T1/"+who, wt, base); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(wt, "a.txt"), who+"\n")
		if _, err := gitCommitAll(wt, "edit"); err != nil {
			t.Fatal(err)
		}
		if err := gitRemoveWorktree(repo, wt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := gitMerge(repo, "symphony/T1/alice"); err != nil {
		t.Fatal(err)
	}
	if conflict, err := gitMerge(repo, "symphony/T1/carol"); err == nil || !conflict {
		t.Fatalf("both changed the same line: the merge should fail on a conflict: %v %v", conflict, err)
	}
	if st := mustGit(t, repo, "status", "--porcelain"); st != "" {
		t.Fatalf("a failed merge should be aborted: %q", st)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "MERGE_HEAD")); !os.IsNotExist(err) {
		t.Fatal("no merge should be in progress")
	}
}

func TestSlug(t *testing.T) {
	for name, want := range map[string]string{
		"bob": "bob", "前端小王": "前端小王", "Ana María": "Ana-María", "a  /  b": "a-b", "--x--": "x",
		"v1.2_beta": "v1.2_beta", "...": "member", "": "member", "!!!": "member", "a..b": "a.b", "x.lock": "x-lock",
		"réviseur 2": "réviseur-2",
	} {
		if got := slug(name); got != want {
			t.Errorf("slug(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestTreeLocks: git work on unrelated folder trees goes on side by side; on the same tree, or one inside another,
// it waits its turn.
func TestTreeLocks(t *testing.T) {
	var l treeLocks
	unlockA := l.lock([]string{"/w/a"})
	done := make(chan struct{})
	go func() { l.lock([]string{"/w/b", "/w/c"})(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("an unrelated tree should not wait")
	}
	for _, dirs := range [][]string{{"/w/a"}, {"/w/a/sub"}, {"/w"}, {"/w/x", "/w/a"}} {
		got := make(chan struct{})
		go func() { l.lock(dirs)(); close(got) }()
		select {
		case <-got:
			t.Fatalf("%v should wait for /w/a", dirs)
		case <-time.After(50 * time.Millisecond):
		}
		unlockA()
		select {
		case <-got:
		case <-time.After(2 * time.Second):
			t.Fatalf("%v should go once /w/a is free", dirs)
		}
		unlockA = l.lock([]string{"/w/a"})
	}
	unlockA()
}

// TestGitTimeout: a git command that takes too long is killed, and says so.
// TestGitWorktreeOptionBranch: a repository whose branch is named like an option (git allows it, and a clone takes the
// upstream's default branch name) still gets a plain worktree on the task's branch: the name isn't read as an option.
func TestGitWorktreeOptionBranch(t *testing.T) {
	for _, name := range []string{"--lock", "-bfoo", "--no-checkout"} {
		dir := initRepo(t, t.TempDir(), map[string]string{"a.txt": "a\n"})
		mustGit(t, dir, "update-ref", "refs/heads/"+name, "HEAD")
		mustGit(t, dir, "symbolic-ref", "HEAD", "refs/heads/"+name)
		base, err := gitBase(dir)
		if err != nil || base != name {
			t.Fatalf("%s: base %q %v", name, base, err)
		}
		wt := filepath.Join(t.TempDir(), "wt")
		if err := gitAddWorktree(dir, "symphony/T1/bob", wt, base); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if list := mustGit(t, dir, "worktree", "list", "--porcelain"); strings.Contains(list, "locked") {
			t.Fatalf("%s: the worktree is locked:\n%s", name, list)
		}
		if readFile(t, filepath.Join(wt, "a.txt")) != "a\n" || mustGit(t, wt, "symbolic-ref", "--short", "HEAD") != "symphony/T1/bob" {
			t.Fatalf("%s: the worktree should have the files, on the task's branch", name)
		}
		if mustGit(t, dir, "symbolic-ref", "HEAD") != "refs/heads/"+name {
			t.Fatalf("%s: the user's checkout moved", name)
		}
	}
}

func TestGitTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake git is a shell script")
	}
	bin := t.TempDir()
	writeFile(t, filepath.Join(bin, "git"), "#!/bin/sh\nexec sleep 30\n")
	if err := os.Chmod(filepath.Join(bin, "git"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	old := gitTimeout
	gitTimeout = 200 * time.Millisecond
	t.Cleanup(func() { gitTimeout = old })
	start := time.Now()
	_, err := runGit(t.TempDir(), "add", "-A")
	if err == nil || err.Error() != "git add: timed out after 200ms" || time.Since(start) > 5*time.Second {
		t.Fatalf("want a timeout, got %v after %v", err, time.Since(start))
	}
	if e := gitFailed("/x", err); e.Code != "task_git_failed" || !strings.Contains(e.Params["detail"], "timed out") {
		t.Fatalf("a timeout is a git failure: %+v", e)
	}
}

// TestGitEnvWithoutSession: git runs without what points it at another repository and without a session's variables
// (a bridge started by hand inside an office session has its token).
func TestGitEnvWithoutSession(t *testing.T) {
	t.Setenv("GIT_DIR", "/elsewhere")
	t.Setenv("SYMPHONY_TOKEN", "starter-token")
	t.Setenv("SYMPHONY_TASK", "T9")
	t.Setenv("SYMPHONY_DATA", "/data")
	env := strings.Join(gitEnv(), "\n")
	for _, gone := range []string{"GIT_DIR=", "SYMPHONY_TOKEN=", "SYMPHONY_TASK="} {
		if strings.Contains(env, gone) {
			t.Errorf("git's environment has %s", gone)
		}
	}
	if !strings.Contains(env, "SYMPHONY_DATA=/data") || !strings.Contains(env, "GIT_TERMINAL_PROMPT=0") {
		t.Errorf("git's environment lost what it keeps:\n%s", env)
	}
}
