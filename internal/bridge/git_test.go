package bridge

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	if gitInWorkTree(dir) {
		t.Fatal("a plain folder is not in a work tree")
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
	if !gitInWorkTree(filepath.Join(dir, "src")) {
		t.Fatal("a folder inside a repository is in its work tree")
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
	if err := gitMerge(repo, "symphony/T1/bob"); err != nil {
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
	if err := gitMerge(repo, "symphony/T1/alice"); err != nil {
		t.Fatal(err)
	}
	if err := gitMerge(repo, "symphony/T1/carol"); err == nil {
		t.Fatal("both changed the same line: the merge should fail")
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
