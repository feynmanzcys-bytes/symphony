package bridge

import (
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
)

// ---------- collaborative tasks ----------

// TaskRequest is what the task dialog submits. Leader makes the first member the lead; Lang is the page's language,
// which the kickoff and the members' mods speak.
type TaskRequest struct {
	Room           *int                `json:"room,omitempty"` // the page's hint of the room it was started from, kept for the page
	Title          string              `json:"title"`
	Leader         bool                `json:"leader"`
	PermissionMode string              `json:"permissionMode"`
	Lang           string              `json:"lang,omitempty"`
	Members        []TaskMemberRequest `json:"members"`
}

// TaskMemberRequest is one member: an employee (Account) or an advisor, an account made for the task.
type TaskMemberRequest struct {
	Account string          `json:"account,omitempty"`
	Advisor *AdvisorRequest `json:"advisor,omitempty"`
	Model   string          `json:"model"`
	Effort  string          `json:"effort"`
}

type AdvisorRequest struct {
	Name string `json:"name"`
	Dir  string `json:"dir"`
}

var errBadOption = newError("launch_bad_option", "unknown drop-down value")

func (s *Server) tasks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"tasks": s.Hub.Tasks()})
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	if !s.launchReady(w, r) {
		return
	}
	var req TaskRequest
	if !decode(w, r, &req) {
		return
	}
	t, err := s.startTask(req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, t)
}

func (s *Server) endTask(w http.ResponseWriter, r *http.Request) {
	if err := s.Hub.EndTask(r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{})
}

func (s *Server) forgetTask(w http.ResponseWriter, r *http.Request) {
	if err := s.Hub.ForgetTask(r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{})
}

func (s *Server) sayTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &req) {
		return
	}
	m, err := s.Hub.SayTask(r.PathValue("id"), req.Text)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, m)
}

func (s *Server) taskChat(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"messages": s.Hub.TaskChat(r.PathValue("id"))})
}

// startTask makes a task: it checks the request, makes the advisors' accounts, readies git for every member (a
// worktree on a branch of its own) and starts the members' sessions, which come online in the background as
// launches do. Whatever fails undoes what was done before it, and no session starts before git is ready for all.
func (s *Server) startTask(req TaskRequest) (*Task, error) {
	hub := s.Hub
	req.Title = strings.TrimSpace(req.Title)
	req.Lang = i18n.Match(req.Lang)
	switch {
	case req.Title == "":
		return nil, newError("task_empty_title", "the task needs a description")
	case len(req.Members) < taskMinMembers:
		return nil, newError("task_too_few", "a task needs at least "+strconv.Itoa(taskMinMembers)+" members", "min", strconv.Itoa(taskMinMembers))
	case len(req.Members) > taskMaxMembers:
		return nil, newError("task_too_many", "a task takes at most "+strconv.Itoa(taskMaxMembers)+" members", "max", strconv.Itoa(taskMaxMembers))
	case !validOption(launchPermissionModes, req.PermissionMode), req.Room != nil && *req.Room < 0:
		return nil, errBadOption
	}
	members := make([]*Member, 0, len(req.Members))
	var employees []string
	names := map[string]bool{}
	for _, mr := range req.Members {
		if !validOption(launchModels, mr.Model) || !validOption(launchEfforts, mr.Effort) || (mr.Advisor != nil && mr.Account != "") {
			return nil, errBadOption
		}
		m := &Member{Model: mr.Model, Effort: mr.Effort}
		if mr.Advisor != nil {
			name := strings.TrimSpace(mr.Advisor.Name)
			if !validName(name) {
				return nil, ErrBadName
			}
			if hub.nameTaken(name) {
				return nil, ErrNameTaken
			}
			dir, err := normalizeDir(mr.Advisor.Dir)
			if err != nil {
				return nil, err
			}
			m.Advisor, m.Name, m.Dir = true, name, dir // the name is the account's until the task id is known
		} else {
			acc, dir := hub.employee(strings.TrimSpace(mr.Account))
			if acc == nil || acc.Advisor {
				return nil, ErrUnknownAccount
			}
			if strings.TrimSpace(dir) == "" {
				return nil, newError("task_bad_dir", acc.Name+" has no folder to work in (no desk session, no start from the page)", "name", acc.Name, "dir", dir)
			}
			norm, err := normalizeDir(dir)
			if err != nil {
				return nil, newError("task_dir_missing", acc.Name+"'s folder is gone or not an absolute path: "+dir, "name", acc.Name, "dir", dir)
			}
			m.Account, m.Name, m.Dir = acc.ID, acc.Name, norm
			employees = append(employees, acc.ID)
		}
		// a folder in another task's worktree (where a desk session went, or an advisor's pick) would nest one task in
		// another's branch
		if inTaskWorktree(m.Dir) {
			return nil, newError("task_dir_in_worktree", m.Name+"'s folder "+m.Dir+" is in a task's git worktree; pick the project folder", "name", m.Name, "dir", m.Dir)
		}
		if names[m.Name] {
			return nil, newError("task_member_dup", m.Name+" is in the team twice", "name", m.Name)
		}
		names[m.Name] = true
		members = append(members, m)
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, ErrNoGit
	}
	id, err := hub.reserveTask(employees)
	if err != nil {
		return nil, err
	}

	undo := &taskUndo{}
	fail := func(err error) (*Task, error) {
		hub.dropTask(id) // first, so sessions of the task are refused from now on
		undo.run(hub, s.Launcher.Terms)
		return nil, err
	}
	for _, m := range members {
		if m.Advisor {
			acc, err := hub.CreateAdvisor(m.Name)
			if err != nil {
				return fail(err)
			}
			m.Account = acc.ID
			undo.advisors = append(undo.advisors, acc.ID)
		}
	}
	t := &Task{ID: id, Room: req.Room, Title: req.Title, PermissionMode: req.PermissionMode,
		Lang: req.Lang, Members: members, Repos: []*TaskRepo{}, Status: TaskRunning, Created: time.Now()}
	if req.Leader {
		t.Leader = members[0].Account
	}
	slugs := map[string]bool{}
	for _, m := range members {
		sl := slug(m.Name)
		for n := 2; slugs[sl]; n++ {
			sl = slug(m.Name) + "-" + strconv.Itoa(n)
		}
		slugs[sl] = true
		m.Name += "-" + id
		m.Branch = "symphony/" + id + "/" + sl
		m.Worktree = filepath.Join(m.Dir, symphonyDir, "worktrees", sl+"-"+id)
		m.Launch = newID()
	}

	var trees []string
	for _, m := range members {
		trees = append(trees, gitLockKey(m.Dir))
	}
	defer hub.gitLocks.lock(trees)()
	prefixes, err := s.prepareGit(t, undo)
	if err != nil {
		return fail(err)
	}
	hub.stageTask(t.copy()) // the members' sessions may report in as soon as they start

	var launches []*Launch
	for _, m := range t.Members {
		acc := hub.Account(m.Account)
		if acc == nil {
			return fail(ErrUnknownAccount)
		}
		// a folder deep in its repository: the session starts in the same place in its worktree
		dir := m.Worktree
		if p := prefixes[m.Dir]; p != "" {
			if sub := filepath.Join(m.Worktree, filepath.FromSlash(p)); isDir(sub) {
				dir = sub
			}
		}
		env := map[string]string{"SYMPHONY_BRIDGE": s.Launcher.BridgeURL, "SYMPHONY_LAUNCH": m.Launch, "SYMPHONY_TOKEN": acc.Token, "SYMPHONY_TASK": id}
		if t.Lang != "" {
			env["SYMPHONY_LANG"] = t.Lang
		}
		l := &Launch{ID: m.Launch, Name: m.Name, Dir: dir, Account: acc.ID, Model: m.Model, Effort: m.Effort,
			Status: LaunchStarting, Time: time.Now(), Task: id}
		lr := LaunchRequest{Account: acc.ID, Name: m.Name, Dir: dir, Model: m.Model, Effort: m.Effort, PermissionMode: t.PermissionMode, Lang: t.Lang}
		if err := s.Launcher.Start(l, lr, env); err != nil {
			return fail(err)
		}
		undo.launches = append(undo.launches, l.ID)
		launches = append(launches, l)
	}
	shown := hub.publishTask(id, launches)
	for _, l := range launches {
		go s.Launcher.watch(hub, l)
	}
	return shown, nil
}

// prepareGit readies git for every member. Each folder's repository: when the folder isn't in one, it becomes one
// with everything in it in a first commit; its base branch; .symphony/ in its exclude file. Then a worktree on a
// branch of its own for each member. It returns where each folder sits in its repository's work tree.
func (s *Server) prepareGit(t *Task, undo *taskUndo) (map[string]string, error) {
	var dirs []string
	for _, m := range t.Members {
		if !slices.Contains(dirs, m.Dir) {
			dirs = append(dirs, m.Dir)
		}
	}
	// parents first: a folder inside another member's folder joins the repository made there
	slices.SortStableFunc(dirs, func(a, b string) int { return len(a) - len(b) })
	home, _ := os.UserHomeDir()
	prefixes := map[string]string{}
	for _, dir := range dirs {
		r := &TaskRepo{Dir: dir}
		if _, err := os.Lstat(filepath.Join(dir, ".git")); errors.Is(err, os.ErrNotExist) && !gitInWorkTree(dir) {
			// git add -A of a whole home folder (or a folder above it) or drive would take ages and hold everything
			if holdsHome(dir, home) || filepath.Dir(dir) == dir {
				return nil, newError("task_dir_home", "won't make a git repository of "+dir+" (the home folder, a folder above it or a drive's root); pick a project folder", "dir", dir)
			}
			// nor of a folder around a repository another task made
			if other := s.Hub.repoMadeBy(dir, "", true); other != "" {
				return nil, repoInUse(dir, other)
			}
			undo.inited = append(undo.inited, dir)
			if err := gitInit(dir, t.ID); err != nil {
				return nil, gitFailed(dir, err)
			}
			r.Inited = true
		}
		root, prefix, err := gitRoot(dir)
		if err != nil {
			return nil, gitFailed(dir, err)
		}
		if other := s.Hub.repoMadeBy(dir, root, false); other != "" {
			return nil, repoInUse(dir, other)
		}
		if r.Base, err = gitBase(dir); errors.Is(err, errNoCommit) {
			return nil, newError("task_repo_no_commit", "the repository of "+dir+" has no commit yet, so there is nothing to branch off", "dir", dir)
		} else if err != nil {
			return nil, gitFailed(dir, err)
		}
		if err := gitExclude(dir); err != nil {
			return nil, gitFailed(dir, err)
		}
		r.Root, prefixes[dir] = root, prefix
		t.Repos = append(t.Repos, r)
	}
	for _, m := range t.Members {
		if err := gitAddWorktree(m.Dir, m.Branch, m.Worktree, t.repo(m.Dir).Base); err != nil {
			return nil, gitFailed(m.Dir, err)
		}
		undo.worktrees = append(undo.worktrees, m)
	}
	return prefixes, nil
}

// holdsHome says whether dir is the home folder or a folder above it, however either is spelled: through a symlink, in
// another case where the file system ignores case, or by another path to the same folder (macOS's
// /System/Volumes/Data/Users/<me>). It compares folders, not names: walking up from home, dir is each folder on the
// way, or holds home where that folder does.
func holdsHome(dir, home string) bool {
	if home == "" {
		return false
	}
	if within(home, dir) {
		return true
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	if real, err := filepath.EvalSymlinks(home); err == nil {
		home = real
	}
	caseless := runtime.GOOS == "darwin" || runtime.GOOS == "windows"
	homeInfo, homeErr := os.Stat(home)
	dirInfo, dirErr := os.Stat(dir)
	for p := home; ; p = filepath.Dir(p) {
		if caseless && strings.EqualFold(p, dir) {
			return true
		}
		if pi, err := os.Stat(p); err == nil && dirErr == nil && os.SameFile(pi, dirInfo) {
			return true
		}
		if rel, err := filepath.Rel(p, home); err == nil && homeErr == nil {
			if fi, err := os.Stat(filepath.Join(dir, rel)); err == nil && os.SameFile(fi, homeInfo) {
				return true
			}
		}
		if filepath.Dir(p) == p {
			return false
		}
	}
}

func repoInUse(dir, task string) *Error {
	return newError("task_repo_in_use", "the repository of "+dir+" was made for task "+task+", which hasn't ended yet", "dir", dir, "task", task)
}

// taskUndo is what making a task has done so far, taken back when a later step fails.
type taskUndo struct {
	advisors  []string  // accounts made
	inited    []string  // folders made repositories
	worktrees []*Member // worktrees and branches added
	launches  []string  // sessions started
}

func (u *taskUndo) run(hub *Hub, terms *Terms) {
	if terms != nil {
		for _, id := range u.launches {
			if t := terms.Get(id); t != nil {
				t.Kill()
				select {
				case <-t.Exited():
				case <-time.After(memberExitWait):
				}
			}
		}
	}
	var dirs []string
	for _, m := range u.worktrees {
		_, _ = runGit(m.Dir, "worktree", "remove", "--force", m.Worktree)
		_ = gitDeleteBranch(m.Dir, m.Branch)
		if !slices.Contains(dirs, m.Dir) {
			dirs = append(dirs, m.Dir)
		}
	}
	for _, dir := range dirs {
		_, _ = runGit(dir, "worktree", "prune")
		removeIfEmpty(filepath.Join(dir, symphonyDir, "worktrees"), filepath.Join(dir, symphonyDir))
	}
	for _, dir := range u.inited {
		_ = os.RemoveAll(filepath.Join(dir, ".git"))
		_ = os.RemoveAll(filepath.Join(dir, symphonyDir))
	}
	for _, id := range u.advisors {
		_ = hub.DeleteAccount(id)
	}
}
