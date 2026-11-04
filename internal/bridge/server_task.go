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
	"unicode/utf8"

	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
)

// ---------- collaborative tasks ----------

// TaskRequest is what the task dialog submits. Name is what the user calls the task (shown instead of the id), Title
// the description (both may be empty: without a description the kickoff introduces the team and the user hands out
// the work in the task chat), Leader makes the first member the lead, Git gives every member a git worktree on a
// branch of its own (else each works in its folder as it is); Lang is the page's language, which the kickoff and the
// members' mods speak.
type TaskRequest struct {
	Name           string              `json:"name"`
	Title          string              `json:"title"`
	Leader         bool                `json:"leader"`
	Git            bool                `json:"git"`
	PermissionMode string              `json:"permissionMode"`
	Lang           string              `json:"lang,omitempty"`
	Members        []TaskMemberRequest `json:"members"`
}

// MembersRequest is what the task window submits to add members to a running task.
type MembersRequest struct {
	Members []TaskMemberRequest `json:"members"`
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

func (s *Server) addTaskMembers(w http.ResponseWriter, r *http.Request) {
	if !s.launchReady(w, r) {
		return
	}
	var req MembersRequest
	if !decode(w, r, &req) {
		return
	}
	t, err := s.addMembers(r.PathValue("id"), req.Members)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, t)
}

func (s *Server) kickTaskMember(w http.ResponseWriter, r *http.Request) {
	if err := s.Hub.KickMember(r.PathValue("id"), r.PathValue("account")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{})
}

func (s *Server) startTaskMember(w http.ResponseWriter, r *http.Request) {
	if !s.launchReady(w, r) {
		return
	}
	t, err := s.startMember(r.PathValue("id"), r.PathValue("account"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, t)
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

// startTask makes a task: it checks the request, makes the advisors' accounts, with Git readies it for every member
// (a worktree on a branch of its own) and starts the members' sessions, which come online in the background as
// launches do. Whatever fails undoes what was done before it, and no session starts before git is ready for all.
func (s *Server) startTask(req TaskRequest) (*Task, error) {
	hub := s.Hub
	req.Name, req.Title = strings.TrimSpace(req.Name), strings.TrimSpace(req.Title)
	req.Lang = i18n.Match(req.Lang)
	switch {
	case utf8.RuneCountInString(req.Name) > taskNameMax:
		return nil, newError("task_name_long", "the task's name is too long (at most "+strconv.Itoa(taskNameMax)+" characters)", "max", strconv.Itoa(taskNameMax))
	case len(req.Members) < taskMinMembers:
		return nil, newError("task_too_few", "a task needs at least "+strconv.Itoa(taskMinMembers)+" members", "min", strconv.Itoa(taskMinMembers))
	case len(req.Members) > taskMaxMembers:
		return nil, newError("task_too_many", "a task takes at most "+strconv.Itoa(taskMaxMembers)+" members", "max", strconv.Itoa(taskMaxMembers))
	case !validOption(launchPermissionModes, req.PermissionMode):
		return nil, errBadOption
	}
	t := &Task{Name: req.Name, Title: req.Title, Git: req.Git, PermissionMode: req.PermissionMode,
		Lang: req.Lang, Repos: []*TaskRepo{}, Status: TaskRunning, Created: time.Now()}
	members, err := s.checkMembers(t, req.Members)
	if err != nil {
		return nil, err
	}
	if t.Git {
		if _, err := exec.LookPath("git"); err != nil {
			return nil, ErrNoGit
		}
		// past the numbers of the task branches the members' repositories already have
		hub.raiseTaskNext(usedTaskNumber(dirsOf(members)))
	}
	t.ID = hub.reserveTask()

	undo := &taskUndo{}
	fail := func(err error) (*Task, error) {
		hub.dropTask(t.ID) // first, so sessions of the task are refused from now on
		undo.run(hub, s.Launcher.Terms, t.ID)
		return nil, err
	}
	if err := s.makeAdvisors(members, undo); err != nil {
		return fail(err)
	}
	t.Members = members
	if req.Leader {
		t.Leader = members[0].Account
	}
	nameMembers(t, members)
	var prefixes map[string]string
	if t.Git {
		defer hub.gitLocks.lock(treesOf(members))()
		if prefixes, err = s.prepareGit(t, members, undo); err != nil {
			return fail(err)
		}
	}
	hub.stageTask(t.copy()) // the members' sessions may report in as soon as they start
	launches, err := s.launchMembers(t, members, prefixes, undo)
	if err != nil {
		return fail(err)
	}
	shown := hub.publishTask(t.ID, launches)
	for _, l := range launches {
		go s.Launcher.watch(hub, l)
	}
	return shown, nil
}

// addMembers puts more members into a running task, as startTask makes them: checked, advisors made, with Git a
// worktree each, their sessions started, then on the task's list (joinMembers), which tells the team. A member that
// had left comes back as itself. One change of a task's team at a time (taskLocks).
func (s *Server) addMembers(id string, reqs []TaskMemberRequest) (*Task, error) {
	hub := s.Hub
	defer hub.taskLocks.lock([]string{id})()
	t, err := hub.taskCopy(id)
	if err != nil {
		return nil, err
	}
	switch {
	case len(reqs) == 0:
		return nil, newError("task_add_empty", "nobody to add")
	case len(t.team())+len(reqs) > taskMaxMembers:
		return nil, newError("task_too_many", "a task takes at most "+strconv.Itoa(taskMaxMembers)+" members", "max", strconv.Itoa(taskMaxMembers))
	}
	members, err := s.checkMembers(t, reqs)
	if err != nil {
		return nil, err
	}
	if t.Git {
		if _, err := exec.LookPath("git"); err != nil {
			return nil, ErrNoGit
		}
	}
	undo := &taskUndo{}
	fail := func(err error) (*Task, error) {
		undo.run(hub, s.Launcher.Terms, id)
		return nil, err
	}
	if err := s.makeAdvisors(members, undo); err != nil {
		return fail(err)
	}
	nameMembers(t, members)
	var prefixes map[string]string
	var repos []*TaskRepo
	if t.Git {
		defer hub.gitLocks.lock(treesOf(members))()
		before := len(t.Repos)
		if prefixes, err = s.prepareGit(t, members, undo); err != nil {
			return fail(err)
		}
		repos = t.Repos[before:]
	}
	launches, err := s.launchMembers(t, members, prefixes, undo)
	if err != nil {
		return fail(err)
	}
	shown, err := hub.joinMembers(id, members, repos, launches)
	if err != nil {
		return fail(err)
	}
	for _, l := range launches {
		go s.Launcher.watch(hub, l)
	}
	return shown, nil
}

// startMember starts a member's session again, where it worked (its worktree, or its folder), when it is over: the
// member's process exited, its launch failed, or the bridge started anew. The kickoff comes again with its session
// (joinTaskLocked).
func (s *Server) startMember(id, account string) (*Task, error) {
	hub := s.Hub
	defer hub.taskLocks.lock([]string{id})()
	t, m, err := hub.memberToStart(id, account)
	if err != nil {
		return nil, err
	}
	dir := m.Dir
	if t.Git {
		dir = m.Worktree
		if _, prefix, err := gitRoot(m.Dir); err == nil && prefix != "" {
			if sub := filepath.Join(m.Worktree, filepath.FromSlash(prefix)); isDir(sub) {
				dir = sub
			}
		}
	}
	if !isDir(dir) {
		return nil, newError("task_dir_missing", m.Name+"'s folder is gone: "+dir, "name", m.Name, "dir", dir)
	}
	m.Launch = newID()
	l, err := s.launchMember(t, m, dir)
	if err != nil {
		return nil, err
	}
	if err := hub.startedMember(id, account, l); err != nil {
		(&taskUndo{launches: []string{l.ID}}).run(hub, s.Launcher.Terms, id)
		return nil, err
	}
	go s.Launcher.watch(hub, l)
	return hub.taskCopy(id)
}

// checkMembers checks what the request asks for and answers the members to make: an advisor with its name and folder,
// an employee with the folder it works in (hub.employee); none twice, none in the team already. One that left the
// task comes back as its old record (the same name, and with Git the same branch and worktree), with the model and
// effort asked for now.
func (s *Server) checkMembers(t *Task, reqs []TaskMemberRequest) ([]*Member, error) {
	hub := s.Hub
	members := make([]*Member, 0, len(reqs))
	names := map[string]bool{} // the account names in the request so far
	for _, mr := range reqs {
		if !validOption(launchModels, mr.Model) || !validOption(launchEfforts, mr.Effort) || (mr.Advisor != nil && mr.Account != "") {
			return nil, errBadOption
		}
		m := &Member{Model: mr.Model, Effort: mr.Effort}
		var back *Member // the member that left, coming back
		if mr.Advisor != nil {
			name := strings.TrimSpace(mr.Advisor.Name)
			if !validName(name) {
				return nil, ErrBadName
			}
			if old := t.leftAdvisor(name); old != nil {
				back = old
			} else if hub.nameTaken(name) {
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
			if t.active(acc.ID) != nil {
				return nil, newError("task_member_dup", acc.Name+" is in the team already", "name", acc.Name)
			}
			back = t.member(acc.ID)
			if strings.TrimSpace(dir) == "" {
				return nil, newError("task_bad_dir", acc.Name+" has no folder to work in (no desk session, no start from the page)", "name", acc.Name, "dir", dir)
			}
			norm, err := normalizeDir(dir)
			if err != nil {
				return nil, newError("task_dir_missing", acc.Name+"'s folder is gone or not an absolute path: "+dir, "name", acc.Name, "dir", dir)
			}
			m.Account, m.Name, m.Dir = acc.ID, acc.Name, norm
		}
		if names[m.Name] {
			return nil, newError("task_member_dup", m.Name+" is in the team twice", "name", m.Name)
		}
		names[m.Name] = true
		if back != nil {
			// its old record, back in the team: with Git in its worktree of its folder then, as its branch is there
			cp := *back
			cp.Left, cp.Launch, cp.Session, cp.Model, cp.Effort, cp.back = nil, "", "", m.Model, m.Effort, true
			if !t.Git {
				cp.Dir = m.Dir
			}
			m = &cp
		} else if t.Git && inTaskWorktree(m.Dir) {
			// a folder in another task's worktree (where a desk session went, or an advisor's pick) would nest one
			// task in another's branch
			return nil, newError("task_dir_in_worktree", m.Name+"'s folder "+m.Dir+" is in a task's git worktree; pick the project folder", "name", m.Name, "dir", m.Dir)
		}
		members = append(members, m)
	}
	return members, nil
}

// makeAdvisors makes the advisors' accounts (one coming back has its account still).
func (s *Server) makeAdvisors(members []*Member, undo *taskUndo) error {
	for _, m := range members {
		if m.Advisor && m.Account == "" {
			acc, err := s.Hub.CreateAdvisor(m.Name)
			if err != nil {
				return err
			}
			m.Account = acc.ID
			undo.advisors = append(undo.advisors, acc.ID)
		}
	}
	return nil
}

// nameMembers gives the members their launch ids, and new ones their session names (<account name>-<task id>) and,
// with Git, their branches and worktrees (the slug of the name; a second member with the same slug gets -2, -3, past
// those of the members already there, left ones included). A member coming back keeps its names.
func nameMembers(t *Task, members []*Member) {
	slugs := map[string]bool{}
	for _, m := range t.Members {
		if m.Branch != "" {
			slugs[strings.TrimPrefix(m.Branch, "symphony/"+t.ID+"/")] = true
		}
	}
	for _, m := range members {
		m.Launch = newID()
		if m.back {
			continue
		}
		if t.Git {
			sl := slug(m.Name)
			for n := 2; slugs[sl]; n++ {
				sl = slug(m.Name) + "-" + strconv.Itoa(n)
			}
			slugs[sl] = true
			m.Branch = "symphony/" + t.ID + "/" + sl
			m.Worktree = filepath.Join(m.Dir, symphonyDir, "worktrees", sl+"-"+t.ID)
		}
		m.Name += "-" + t.ID
	}
}

// launchMembers starts the members' sessions: with Git in their worktrees (a folder deep in its repository: at the
// same place in the worktree), else in their folders.
func (s *Server) launchMembers(t *Task, members []*Member, prefixes map[string]string, undo *taskUndo) ([]*Launch, error) {
	var launches []*Launch
	for _, m := range members {
		dir := m.Dir
		if t.Git {
			dir = m.Worktree
			if p := prefixes[m.Dir]; p != "" {
				if sub := filepath.Join(m.Worktree, filepath.FromSlash(p)); isDir(sub) {
					dir = sub
				}
			}
		}
		l, err := s.launchMember(t, m, dir)
		if err != nil {
			return nil, err
		}
		undo.launches = append(undo.launches, l.ID)
		launches = append(launches, l)
	}
	return launches, nil
}

// launchMember starts one member's session in dir, with the task in its environment, under m.Launch.
func (s *Server) launchMember(t *Task, m *Member, dir string) (*Launch, error) {
	acc := s.Hub.Account(m.Account)
	if acc == nil {
		return nil, ErrUnknownAccount
	}
	env := map[string]string{"SYMPHONY_BRIDGE": s.Launcher.BridgeURL, "SYMPHONY_LAUNCH": m.Launch, "SYMPHONY_TOKEN": acc.Token, "SYMPHONY_TASK": t.ID}
	if t.Lang != "" {
		env["SYMPHONY_LANG"] = t.Lang
	}
	l := &Launch{ID: m.Launch, Name: m.Name, Dir: dir, Account: acc.ID, Model: m.Model, Effort: m.Effort,
		Status: LaunchStarting, Time: time.Now(), Task: t.ID}
	lr := LaunchRequest{Account: acc.ID, Name: m.Name, Dir: dir, Model: m.Model, Effort: m.Effort, PermissionMode: t.PermissionMode, Lang: t.Lang}
	if err := s.Launcher.Start(l, lr, env); err != nil {
		return nil, err
	}
	return l, nil
}

func dirsOf(members []*Member) []string {
	dirs := make([]string, len(members))
	for i, m := range members {
		dirs[i] = m.Dir
	}
	return dirs
}

func treesOf(members []*Member) []string {
	var trees []string
	for _, m := range members {
		trees = append(trees, gitLockKey(m.Dir))
	}
	return trees
}

// prepareGit readies git for members of a task. Each folder's repository, when the task doesn't have it yet: when the
// folder isn't in one, it becomes one with everything in it in a first commit; its base branch; .symphony/ in its
// exclude file. Then a worktree on a branch of its own for each member (a member coming back has its branch, and its
// worktree unless it was removed by hand: it is added again then). It returns where each folder sits in its
// repository's work tree.
func (s *Server) prepareGit(t *Task, members []*Member, undo *taskUndo) (map[string]string, error) {
	var dirs []string
	for _, m := range members {
		if !slices.Contains(dirs, m.Dir) {
			dirs = append(dirs, m.Dir)
		}
	}
	// parents first: a folder inside another member's folder joins the repository made there
	slices.SortStableFunc(dirs, func(a, b string) int { return len(a) - len(b) })
	home, _ := os.UserHomeDir()
	prefixes := map[string]string{}
	for _, dir := range dirs {
		if t.repo(dir) != nil {
			_, prefix, err := gitRoot(dir)
			if err != nil {
				return nil, gitFailed(dir, err)
			}
			prefixes[dir] = prefix
			continue
		}
		r := &TaskRepo{Dir: dir}
		// a folder git can't tell about (a repository around it it can't read or won't trust) is an error, not plain
		plain, err := gitPlain(dir)
		if err != nil {
			return nil, gitFailed(dir, err)
		}
		if plain {
			// git add -A of a whole home folder (or a folder above it) or drive would take ages and hold everything
			if holdsHome(dir, home) || filepath.Dir(dir) == dir {
				return nil, newError("task_dir_home", "won't make a git repository of "+dir+" (the home folder, a folder above it or a drive's root); pick a project folder", "dir", dir)
			}
			// nor of a folder around a repository another task made
			if other := s.Hub.repoMadeBy(dir, "", true); other != "" && other != t.ID {
				return nil, repoInUse(dir, other)
			}
			undo.inited = append(undo.inited, dir)
			if err := gitInit(dir, "symphony: files before task "+t.ID); err != nil {
				return nil, gitFailed(dir, err)
			}
			r.Inited = true
		}
		root, prefix, err := gitRoot(dir)
		if err != nil {
			return nil, gitFailed(dir, err)
		}
		if other := s.Hub.repoMadeBy(dir, root, false); other != "" && other != t.ID {
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
	for _, m := range members {
		if m.back {
			// a member coming back: its branch is there; so is its worktree, unless it was removed by hand
			if isDir(m.Worktree) {
				continue
			}
			undo.readded = append(undo.readded, m)
			if err := gitReaddWorktree(m.Dir, m.Branch, m.Worktree); err != nil {
				return nil, gitFailed(m.Dir, err)
			}
			continue
		}
		// recorded before the call: a post-checkout hook that fails makes worktree add exit 1 after it made both the
		// worktree and the branch (the undo skips what isn't there). A branch of that name that is already there is
		// the user's (the task counter passed every symphony/T<n>/ branch, raiseTaskNext): the add fails on it, and
		// the undo must not delete it (the counter makes this near impossible, so no test forces it)
		if _, err := runGit(m.Dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+m.Branch); err == nil {
			return nil, gitFailed(m.Dir, errors.New("branch "+m.Branch+" already exists"))
		}
		undo.worktrees = append(undo.worktrees, m)
		if err := gitAddWorktree(m.Dir, m.Branch, m.Worktree, t.repo(m.Dir).Base); err != nil {
			return nil, gitFailed(m.Dir, err)
		}
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
	readded   []*Member // worktrees added again for members coming back (their branches stay)
	launches  []string  // sessions started
}

func (u *taskUndo) run(hub *Hub, terms *Terms, task string) {
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
	// what can't be taken back stays, and the log says what (the page shows why the task didn't start)
	var left []string
	gitLeft := func(dir string, err error) {
		if err != nil {
			left = append(left, gitFailed(dir, err).Text)
		}
	}
	var dirs []string
	for _, m := range u.readded {
		if _, err := os.Lstat(m.Worktree); err == nil {
			_, err := runGit(m.Dir, "worktree", "remove", "--force", m.Worktree)
			gitLeft(m.Dir, err)
		}
	}
	for _, m := range u.worktrees {
		// a member whose worktree add failed may have neither, or the branch alone
		if _, err := os.Lstat(m.Worktree); err == nil {
			_, err := runGit(m.Dir, "worktree", "remove", "--force", m.Worktree)
			gitLeft(m.Dir, err)
		}
		if _, err := runGit(m.Dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+m.Branch); err == nil {
			gitLeft(m.Dir, gitDeleteBranch(m.Dir, m.Branch))
		}
		if !slices.Contains(dirs, m.Dir) {
			dirs = append(dirs, m.Dir)
		}
	}
	for _, dir := range dirs {
		_, err := runGit(dir, "worktree", "prune")
		gitLeft(dir, err)
		removeIfEmpty(filepath.Join(dir, symphonyDir, "worktrees"), filepath.Join(dir, symphonyDir))
	}
	for _, dir := range u.inited {
		// only what this start made: files kept from earlier tasks' worktrees (.symphony/kept) stay
		for _, p := range []string{filepath.Join(dir, ".git"), filepath.Join(dir, symphonyDir, "worktrees")} {
			if err := os.RemoveAll(p); err != nil {
				left = append(left, err.Error())
			}
		}
		removeIfEmpty(filepath.Join(dir, symphonyDir))
	}
	for _, id := range u.advisors {
		if err := hub.DeleteAccount(id); err != nil {
			left = append(left, "deleting advisor "+id+": "+err.Error())
		}
	}
	for _, l := range left {
		hub.warn("log.task_undo_failed", "task", task, "reason", l)
	}
}
