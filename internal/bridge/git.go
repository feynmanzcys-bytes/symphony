package bridge

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

// Git for tasks: every member works in a worktree of its own, on a branch of its own, of its folder's repository.
// A folder that isn't in one becomes one for the task, and plain files again when it ends (tidyTask).

// gitCommitter is who the commits the bridge makes itself are by, so they work without a git identity set up; they
// are never signed (a signing prompt would hang) and skip the hooks (a pre-commit hook must not keep a member's
// work from being saved).
var gitCommitter = []string{"-c", "user.name=symphony", "-c", "user.email=symphony@localhost", "-c", "commit.gpgsign=false"}

// symphonyDir is where a folder's task worktrees live (<folder>/.symphony/worktrees/...), listed in the repository's
// own exclude file
const (
	symphonyDir     = ".symphony"
	symphonyExclude = ".symphony/"
)

var ErrNoGit = newError("task_no_git", "git is not on PATH; tasks need it")

// gitEnv is the bridge's environment without what would point git at another repository than the folder's.
func gitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_PREFIX":
			continue
		}
		env = append(env, kv)
	}
	return append(env, "GIT_TERMINAL_PROMPT=0")
}

// runGit runs git in dir and returns its output, trimmed; a failure carries git's own words.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = gitEnv()
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		sub := ""
		for _, a := range args {
			if !strings.HasPrefix(a, "-") && !strings.Contains(a, "=") {
				sub = a
				break
			}
		}
		return "", fmt.Errorf("git %s: %s", sub, msg)
	}
	return strings.TrimSpace(out.String()), nil
}

// gitFailed is a git command's failure as the page shows it.
func gitFailed(dir string, err error) *Error {
	return newError("task_git_failed", "git failed in "+dir+": "+err.Error(), "dir", dir, "detail", err.Error())
}

// gitInWorkTree says whether dir is inside a repository's work tree (a folder in a repository is not made one again).
func gitInWorkTree(dir string) bool {
	out, err := runGit(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// gitInit makes a plain folder a repository, with everything in it in a first commit.
func gitInit(dir, taskID string) error {
	if _, err := runGit(dir, "init", "-q"); err != nil {
		return err
	}
	if _, err := runGit(dir, "add", "-A"); err != nil {
		return err
	}
	_, err := runGit(dir, append(gitCommitter, "commit", "-q", "--no-verify", "--allow-empty", "-m", "symphony: files before task "+taskID)...)
	return err
}

// gitRoot is the top of dir's work tree, and where dir sits in it ("" at the top, else "sub/dir/").
func gitRoot(dir string) (root, prefix string, err error) {
	if root, err = runGit(dir, "rev-parse", "--show-toplevel"); err != nil {
		return "", "", err
	}
	prefix, err = runGit(dir, "rev-parse", "--show-prefix")
	return root, prefix, err
}

// gitBase is the branch dir's checkout is on ("HEAD" when detached); a repository without a commit has none.
func gitBase(dir string) (string, error) {
	if _, err := runGit(dir, "rev-parse", "--verify", "-q", "HEAD^{commit}"); err != nil {
		return "", errors.New("the repository has no commit yet")
	}
	if b, err := runGit(dir, "symbolic-ref", "--short", "-q", "HEAD"); err == nil && b != "" {
		return b, nil
	}
	return "HEAD", nil
}

// gitExclude lists .symphony/ in the repository's own exclude file (never in the user's .gitignore), once.
func gitExclude(dir string) error {
	path, err := runGit(dir, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == symphonyExclude {
			return nil
		}
	}
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		data = append(data, '\n')
	}
	data = append(data, symphonyExclude+"\n"...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// gitAddWorktree checks base out in a new worktree at path, on a new branch.
func gitAddWorktree(dir, branch, path, base string) error {
	_, err := runGit(dir, "worktree", "add", "-q", "-b", branch, path, base)
	return err
}

// gitCommitAll commits whatever a checkout has uncommitted, untracked files included; it says whether there was any.
func gitCommitAll(dir, msg string) (bool, error) {
	status, err := runGit(dir, "status", "--porcelain")
	if err != nil || status == "" {
		return false, err
	}
	if _, err := runGit(dir, "add", "-A"); err != nil {
		return false, err
	}
	if _, err := runGit(dir, append(gitCommitter, "commit", "-q", "--no-verify", "-m", msg)...); err != nil {
		return false, err
	}
	return true, nil
}

// gitRemoveWorktree removes a worktree (its branch stays) and prunes what git keeps about it. One that git won't
// remove as it is (submodules) is forced only when it has nothing uncommitted.
func gitRemoveWorktree(dir, path string) error {
	if isDir(path) {
		if _, err := runGit(dir, "worktree", "remove", path); err != nil {
			if status, serr := runGit(path, "status", "--porcelain"); serr != nil || status != "" {
				return err
			}
			if _, err := runGit(dir, "worktree", "remove", "--force", path); err != nil {
				return err
			}
		}
	}
	_, err := runGit(dir, "worktree", "prune")
	return err
}

// gitMerge merges a branch into dir's checkout; a merge that fails is aborted, leaving the checkout as it was.
func gitMerge(dir, branch string) error {
	if _, err := runGit(dir, append(gitCommitter, "merge", "-q", "--no-edit", branch)...); err != nil {
		_, _ = runGit(dir, "merge", "--abort")
		return err
	}
	return nil
}

func gitDeleteBranch(dir, branch string) error {
	_, err := runGit(dir, "branch", "-q", "-D", branch)
	return err
}

// slug makes a name fit for a branch and a folder: Unicode letters, digits, -, _ and . stay, every other run of
// characters becomes one -, and - and . go from the ends ("member" when nothing is left). As git refuses them in a
// branch, a run of dots becomes one and a final ".lock" becomes "-lock".
func slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	s := b.String()
	for strings.Contains(s, "..") {
		s = strings.ReplaceAll(s, "..", ".")
	}
	s = strings.Trim(s, "-.")
	if strings.HasSuffix(s, ".lock") {
		s = strings.TrimSuffix(s, ".lock") + "-lock"
	}
	if s == "" {
		return "member"
	}
	return s
}

// removeIfEmpty removes folders that are empty, innermost first (.symphony/worktrees, then .symphony).
func removeIfEmpty(dirs ...string) {
	for _, d := range dirs {
		_ = os.Remove(d) // fails, and leaves it, when something is in it
	}
}
