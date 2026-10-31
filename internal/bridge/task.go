package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
)

// Tasks: a team of 2 to 5 sessions in a small meeting room, working together on one description the user wrote.
// The members are employees (each gets a second, task-only session; its desk session stays as it is) and advisors
// (accounts made for the task and deleted when it ends). Each member works in a git worktree of its own, on a branch
// of its own, of its folder's repository (git.go). The members talk among themselves with SendMessage; the bridge
// only names them predictably and sends each a kickoff prompt. When the task ends, the sessions stop, what they left
// uncommitted is committed to their branches and the worktrees go; in a repository the bridge made for the task, the
// branches are merged into the base branch, so the folder holds the results. The repositories and the branches stay.

// task states
const (
	TaskRunning = "running"
	TaskEnding  = "ending"
	TaskEnded   = "ended"
	TaskFailed  = "failed"
)

const (
	// keepTasks is how many tasks tasks.json keeps for the record
	keepTasks      = 200
	taskMinMembers = 2
	taskMaxMembers = 5
	// memberExitWait is how long ending a task waits for a member's killed process before touching its worktree
	memberExitWait = 10 * time.Second
)

var (
	ErrTaskUnknown = newError("task_unknown", "no such task")
	ErrTaskEnded   = newError("task_ended", "the task has ended or is ending")
	ErrTaskRunning = newError("task_running", "the task is still running; end it first")
	errRoomBusy    = newError("task_room_busy", "a task is already in this meeting room")
)

type Task struct {
	ID             string            `json:"id"`
	Room           int               `json:"room"`             // the small room's index on the page
	RoomName       string            `json:"roomName"`         // as the page named it, for the kickoff
	Title          string            `json:"title"`            // the user's task description
	Leader         string            `json:"leader,omitempty"` // the lead's account id; empty when everyone gets the task alike
	PermissionMode string            `json:"permissionMode"`
	Lang           string            `json:"lang,omitempty"`
	Members        []*Member         `json:"members"`
	Repos          []*TaskRepo       `json:"repos"`
	Status         string            `json:"status"`
	Error          string            `json:"error,omitempty"` // English; the page shows ErrorCode and ErrorParams
	ErrorCode      string            `json:"errorCode,omitempty"`
	ErrorParams    map[string]string `json:"errorParams,omitempty"`
	Created        time.Time         `json:"created"`
	Ended          *time.Time        `json:"ended,omitempty"`

	starting bool // still being made: out of sight, but in the way of other tasks wanting its room or its members
}

// TaskRepo is one member folder's repository.
type TaskRepo struct {
	Dir    string `json:"dir"`
	Inited bool   `json:"inited"` // the bridge made this repository: the members' branches are merged when the task ends
	Base   string `json:"base"`   // the branch the worktrees branched off ("HEAD" when detached)
	Root   string `json:"root"`   // the top of the work tree as git names it: folders in one repository share it
}

type Member struct {
	Account  string `json:"account"`
	Name     string `json:"name"` // the session name: <account name>-<task id>
	Advisor  bool   `json:"advisor,omitempty"`
	Dir      string `json:"dir"` // the folder: the employee's, or the advisor's
	Branch   string `json:"branch"`
	Worktree string `json:"worktree"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"effort,omitempty"`
	Launch   string `json:"launch,omitempty"`
	Session  string `json:"session,omitempty"` // set when the session reports in
}

func (t *Task) member(account string) *Member {
	for _, m := range t.Members {
		if m.Account == account {
			return m
		}
	}
	return nil
}

func (t *Task) lead() *Member {
	if t.Leader == "" {
		return nil
	}
	return t.member(t.Leader)
}

func (t *Task) repo(dir string) *TaskRepo {
	for _, r := range t.Repos {
		if r.Dir == dir {
			return r
		}
	}
	return nil
}

// open says whether the task still holds its room, its members and its repositories.
func (t *Task) open() bool { return t.Status == TaskRunning || t.Status == TaskEnding }

// copy is a copy the hub won't change behind its back (snapshots are encoded after the lock is let go).
func (t *Task) copy() *Task {
	cp := *t
	cp.Members = make([]*Member, len(t.Members))
	for i, m := range t.Members {
		mc := *m
		cp.Members[i] = &mc
	}
	cp.Repos = make([]*TaskRepo, len(t.Repos))
	for i, r := range t.Repos {
		rc := *r
		cp.Repos[i] = &rc
	}
	return &cp
}

func taskEvent(id string) map[string]any { return map[string]any{"type": "task", "id": id} }

func (h *Hub) taskLocked(id string) *Task {
	for _, t := range h.tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// tasksLocked is the tasks the page shows: running, ending and failed ones.
func (h *Hub) tasksLocked() []*Task {
	list := []*Task{}
	for _, t := range h.tasks {
		if !t.starting && t.Status != TaskEnded {
			list = append(list, t.copy())
		}
	}
	return list
}

// Tasks is the tasks the page shows.
func (h *Hub) Tasks() []*Task {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.tasksLocked()
}

// runningTaskLocked is the running task the account is a member of, if any.
func (h *Hub) runningTaskLocked(account string) *Task {
	for _, t := range h.tasks {
		if t.Status == TaskRunning && t.member(account) != nil {
			return t
		}
	}
	return nil
}

// ---------- making a task (the steps that need the hub; the rest is in server_task.go) ----------

// employee is an account and its folder: its online desk session's, else the last one started from the page, else
// the last one a desk session reported.
func (h *Hub) employee(id string) (*Account, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	a := h.accounts[id]
	if a == nil {
		return nil, ""
	}
	cp := *a
	if s := h.onlineLocked(id); s != nil && s.Cwd != "" {
		return &cp, s.Cwd
	}
	if a.Last != nil && a.Last.Dir != "" {
		return &cp, a.Last.Dir
	}
	return &cp, a.Dir
}

func (h *Hub) nameTaken(name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, a := range h.accounts {
		if a.Name == name {
			return true
		}
	}
	return false
}

// reserveTask checks that the room and the accounts are free and takes the next task id for them. Until it is
// published the task is out of sight, but it holds the room and the accounts.
func (h *Hub) reserveTask(room int, accounts []string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, t := range h.tasks {
		if t.Status == TaskEnded {
			continue
		}
		// a failed task stays in its room until it is dismissed
		if t.Room == room {
			return "", errRoomBusy
		}
		if !t.open() {
			continue
		}
		for _, a := range accounts {
			if t.member(a) != nil {
				name := a
				if acc := h.accounts[a]; acc != nil {
					name = acc.Name
				}
				return "", newError("task_member_busy", name+" is already in task "+t.ID, "name", name, "task", t.ID)
			}
		}
	}
	n := max(h.taskNext, 1)
	h.taskNext = n + 1
	t := &Task{ID: "T" + strconv.Itoa(n), Room: room, Status: TaskRunning, Created: h.now(), starting: true}
	for _, a := range accounts {
		t.Members = append(t.Members, &Member{Account: a})
	}
	h.tasks = append(h.tasks, t)
	h.trimTasksLocked()
	h.saveTasksLocked()
	return t.ID, nil
}

// repoMadeBy is the open task that made a repository holding dir (whose work tree's top is root), if any: no other
// task may use that repository while that task runs, as it merges into the folder's checkout when it ends. With
// around, a repository inside dir counts too (dir is about to be made one).
func (h *Hub) repoMadeBy(dir, root string, around bool) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, t := range h.tasks {
		if !t.open() {
			continue
		}
		for _, r := range t.Repos {
			if !r.Inited {
				continue
			}
			if within(dir, r.Dir) || (root != "" && within(root, r.Root)) || (around && (within(r.Dir, dir) || within(r.Root, dir))) {
				return t.ID
			}
		}
	}
	return ""
}

// stageTask puts the made task in place of its reservation: still out of sight, but its sessions can report in.
func (h *Hub) stageTask(t *Task) {
	h.mu.Lock()
	defer h.mu.Unlock()
	t.starting = true
	for i, old := range h.tasks {
		if old.ID == t.ID {
			h.tasks[i] = t
		}
	}
	h.saveTasksLocked()
}

// publishTask shows the task with its launches, and follows the members' processes.
func (h *Hub) publishTask(id string, launches []*Launch) *Task {
	h.mu.Lock()
	defer h.mu.Unlock()
	t := h.taskLocked(id)
	t.starting = false
	for _, l := range launches {
		h.launches[l.ID] = l
		// a session quick enough to report in before this has its launch online already
		for _, s := range h.sessions {
			if s.Launch == l.ID && s.Account == l.Account && s.State != StateEnded {
				l.Status, l.Session = LaunchOnline, s.ID
			}
		}
		if h.terms != nil {
			if term := h.terms.Get(l.ID); term != nil {
				go func(launch string) {
					<-term.Exited()
					h.memberExited(id, launch)
				}(l.ID)
			}
		}
	}
	h.saveTasksLocked()
	h.broadcastLocked(taskEvent(id))
	h.checkTaskLocked(id)
	return t.copy()
}

// dropTask forgets a task that couldn't be made; sessions that already reported in stop.
func (h *Hub) dropTask(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopTaskSessionsLocked(id)
	for sid, s := range h.sessions {
		if s.Task == id {
			h.dropSessionLocked(sid)
		}
	}
	h.tasks = slices.DeleteFunc(h.tasks, func(t *Task) bool { return t.ID == id })
	// its number goes back when no task took one since, so the ids people see have no gaps
	if n, err := strconv.Atoi(strings.TrimPrefix(id, "T")); err == nil && h.taskNext == n+1 {
		h.taskNext = n
	}
	h.saveTasksLocked()
	h.broadcastLocked(nil)
}

// ---------- the members' sessions ----------

// joinTaskLocked ties a task session that reports in to its member; the first time, it queues the kickoff. A session
// that starts afresh after the member's last one ended (/clear) gets it again, as its context is gone; one that comes
// back after being lost gets nothing again.
func (h *Hub) joinTaskLocked(s *Session, started bool) {
	t := h.taskLocked(s.Task)
	if t == nil || t.Status != TaskRunning {
		return
	}
	m := t.member(s.Account)
	if m == nil {
		return
	}
	if s.Title == "" {
		s.Title = m.Name // its --name, until the mod reports it
	}
	again := started && m.Session != ""
	if m.Session == s.ID && !again {
		return
	}
	first := m.Session == ""
	m.Session = s.ID
	if first || again {
		h.sayLocked(s.ID, kickoffText(t, m), "task")
	}
	h.saveTasksLocked()
	if !t.starting {
		h.broadcastLocked(taskEvent(t.ID))
	}
}

// kickoffText is a member's first prompt, in the task's language: the task, the team, and how they work together.
func kickoffText(t *Task, m *Member) string {
	var team []string
	for _, o := range t.Members {
		if o != m {
			team = append(team, i18n.TLang(t.Lang, "task.kickoff.teammate", map[string]string{"name": o.Name, "dir": o.Dir, "branch": o.Branch, "worktree": o.Worktree}))
		}
	}
	room := t.RoomName
	if room == "" {
		room = strconv.Itoa(t.Room)
	}
	base := ""
	if r := t.repo(m.Dir); r != nil {
		base = r.Base
	}
	p := map[string]string{"task": t.ID, "room": room, "title": t.Title, "team": strings.Join(team, "\n"),
		"name": m.Name, "dir": m.Dir, "branch": m.Branch, "worktree": m.Worktree, "base": base}
	lead := t.lead()
	switch {
	case lead == nil:
		return i18n.TLang(t.Lang, "task.kickoff.flat", p)
	case lead == m:
		return i18n.TLang(t.Lang, "task.kickoff.lead", p)
	}
	p["lead"] = lead.Name
	return i18n.TLang(t.Lang, "task.kickoff.member", p)
}

// memberAliveLocked says whether a member is still at it: its session is online, its process still runs (a session
// that ended may start again in it), or its launch is on the way. A launch that failed is over (its process is
// killed, failLaunch).
func (h *Hub) memberAliveLocked(m *Member) bool {
	if s := h.sessions[m.Session]; m.Session != "" && s != nil && s.State != StateEnded {
		return true
	}
	l := h.launches[m.Launch]
	if l != nil && l.Status == LaunchFailed {
		return false
	}
	if h.terms != nil && m.Launch != "" && h.terms.Live(m.Launch) {
		return true
	}
	return l != nil && l.Status == LaunchStarting
}

// checkTaskLocked ends a running task once no member is at it any more: the last one went off duty, or never came.
// A bridge on its way out leaves its tasks to the next one (Stop).
func (h *Hub) checkTaskLocked(id string) {
	t := h.taskLocked(id)
	if t == nil || t.starting || t.Status != TaskRunning || h.stopping {
		return
	}
	for _, m := range t.Members {
		if h.memberAliveLocked(m) {
			return
		}
	}
	h.endTaskLocked(t)
}

// memberExited: a member's process is gone. Its session is off duty at once (a killed process has no time to say
// so), and the task ends with the last member.
func (h *Hub) memberExited(task, launch string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	changed := false
	for _, s := range h.sessions {
		if s.Task == task && s.Launch == launch && s.State != StateEnded {
			h.endSessionLocked(s)
			changed = true
		}
	}
	if changed {
		h.broadcastLocked(nil)
	}
	h.checkTaskLocked(task)
}

// endSessionLocked marks a session off duty, as a session.end does.
func (h *Hub) endSessionLocked(s *Session) {
	before := s.State
	s.State, s.Tool = StateEnded, ""
	h.markStateLocked(s, before)
	h.chatNoticeLocked(s.ID, "ended")
}

// stopTaskSessionsLocked puts a task's sessions off duty and withdraws what they left pending.
func (h *Hub) stopTaskSessionsLocked(id string) {
	for _, s := range h.sessions {
		if s.Task == id && s.State != StateEnded {
			h.endSessionLocked(s)
		}
	}
	for _, a := range h.asks {
		if s := h.sessions[a.Session]; s != nil && s.Task == id && a.Status == AskPending {
			a.Status = AskCancelled
			close(a.done)
			h.chatSettledLocked(a)
		}
	}
}

// ---------- the task chat ----------

// splitMentions takes the leading "@name"s off text (separated by whitespace, a ":" or "," right after each dropped):
// the names in the order given, repeats left out, and the rest. Text that doesn't start with one gives no names.
func splitMentions(text string) (names []string, rest string) {
	rest = text
	for strings.HasPrefix(rest, "@") {
		end := strings.IndexFunc(rest, unicode.IsSpace)
		if end < 0 {
			end = len(rest)
		}
		name := rest[1:end]
		for _, p := range []string{":", ",", "：", "，"} {
			name = strings.TrimSuffix(name, p)
		}
		if name == "" {
			break
		}
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
		rest = strings.TrimSpace(rest[end:])
	}
	return names, rest
}

// SayTask sends what the person wrote in a task's chat: "@<member name> ..." (one or more) to those members' sessions
// that are online, anything else to every member online. Each gets it as its next prompt from the page, as the
// employee window's input does, and the message log keeps one message for it (from the page, no sender).
func (h *Hub) SayTask(id, text string) (*Message, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	t := h.taskLocked(id)
	if t == nil || t.starting {
		return nil, ErrTaskUnknown
	}
	if t.Status != TaskRunning {
		return nil, ErrTaskEnded
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, newError("say_empty", "nothing to send")
	}
	m := &Message{ID: newID(), Text: text, Origin: "page", Task: id, Delivered: true, Received: true, Time: h.now()}
	var targets []*Session
	if names, rest := splitMentions(text); len(names) > 0 {
		var members []*Member
		for _, name := range names {
			i := slices.IndexFunc(t.Members, func(mb *Member) bool { return mb.Name == name })
			if i < 0 {
				return nil, newError("task_say_unknown_member", "no member named "+name+" in this task", "name", name)
			}
			members = append(members, t.Members[i])
		}
		if rest == "" {
			return nil, newError("say_empty", "nothing to send")
		}
		for _, mb := range members {
			if s := h.onlineTaskLocked(mb.Account, id); s != nil {
				targets = append(targets, s)
			}
		}
		if len(targets) == 0 {
			return nil, ErrSessionOffline
		}
		if len(names) == 1 {
			m.To = targets[0].ID
		}
		m.ToName, m.ToNames, m.Text = strings.Join(names, " "), names, rest
	} else {
		for _, mb := range t.Members {
			if s := h.onlineTaskLocked(mb.Account, id); s != nil {
				targets = append(targets, s)
			}
		}
		if len(targets) == 0 {
			return nil, ErrSessionOffline
		}
	}
	for _, s := range targets {
		h.sayLocked(s.ID, m.Text, "page")
	}
	h.pushMessageLocked(m)
	h.saveMessagesLocked()
	h.broadcastLocked(map[string]any{"type": "message", "message": m})
	cp := *m
	return &cp, nil
}

// TaskChat is the message log's messages of a task.
func (h *Hub) TaskChat(id string) []*Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := []*Message{}
	for _, m := range h.messages {
		if m.Task == id {
			cp := *m
			list = append(list, &cp)
		}
	}
	return list
}

// ---------- ending a task ----------

// EndTask ends a running task: its sessions stop now, and git is tidied up in the background (Flush waits for it).
func (h *Hub) EndTask(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	t := h.taskLocked(id)
	if t == nil || t.starting {
		return ErrTaskUnknown
	}
	if t.Status != TaskRunning {
		return ErrTaskEnded
	}
	h.endTaskLocked(t)
	return nil
}

func (h *Hub) endTaskLocked(t *Task) {
	t.Status = TaskEnding
	h.stopTaskSessionsLocked(t.ID)
	h.saveTasksLocked()
	h.broadcastLocked(taskEvent(t.ID))
	h.taskJobs.start()
	go h.finishTask(t.ID)
}

// finishTask does the rest of ending a task: the members' processes are killed, git is tidied up (tidyTask); then the
// task is ended and the advisors' accounts go, or it fails when something in git couldn't be done (and keeps its
// advisors and sessions for its window until it is dismissed).
func (h *Hub) finishTask(id string) {
	defer h.taskJobs.done()
	h.mu.Lock()
	t := h.taskLocked(id)
	if t == nil {
		h.mu.Unlock()
		return
	}
	t = t.copy()
	terms := h.terms
	h.mu.Unlock()

	if terms != nil {
		var gone []<-chan struct{}
		for _, m := range t.Members {
			if term := terms.Get(m.Launch); term != nil && !term.Ended() {
				term.Kill()
				gone = append(gone, term.Exited())
			}
		}
		deadline := time.After(memberExitWait)
		for _, ch := range gone {
			select {
			case <-ch:
			case <-deadline:
			}
		}
	}
	var trees []string
	for _, r := range t.Repos {
		trees = append(trees, gitLockKey(r.Dir))
	}
	unlock := h.gitLocks.lock(trees)
	fail := tidyTask(t)
	unlock()

	h.mu.Lock()
	defer h.mu.Unlock()
	task := h.taskLocked(id)
	if task == nil {
		return
	}
	now := h.now()
	task.Status, task.Ended = TaskEnded, &now
	if fail != nil {
		task.Status, task.Error, task.ErrorCode, task.ErrorParams = TaskFailed, fail.Text, fail.Code, fail.Params
	} else {
		// an ended task leaves the office, and its sessions and advisors with it; a failed one keeps them for its window
		h.dropTaskPeopleLocked(task)
	}
	h.saveTasksLocked()
	h.broadcastLocked(taskEvent(id))
}

// dropTaskPeopleLocked forgets a task's sessions, launches and the terminals whose process ended, and deletes its
// advisors' accounts.
func (h *Hub) dropTaskPeopleLocked(t *Task) {
	for sid, s := range h.sessions {
		if s.Task == t.ID {
			h.dropSessionLocked(sid)
		}
	}
	for _, m := range t.Members {
		if m.Launch != "" {
			delete(h.launches, m.Launch)
			if h.terms != nil {
				h.terms.Forget(m.Launch)
			}
		}
		if m.Advisor {
			_ = h.deleteAccountLocked(m.Account)
		}
	}
}

// tidyTask puts the members' folders in order after a task. What a member left uncommitted is committed to its
// branch and its worktree goes (commits on a detached HEAD that no ref holds get a branch first, <branch>-detached,
// or they would go with it). In a repository the bridge made, uncommitted changes in the folder itself are committed
// too and every member's branch is merged into the base branch in member order; when a merge fails, the merges are
// taken back and the task fails. Nothing else is ever removed: the repositories, the bridge's own included, and the
// branches stay. The first failure is the answer, and whatever it concerns stays as it is, so nothing is lost.
func tidyTask(t *Task) *Error {
	var fail *Error
	note := func(e *Error) {
		if fail == nil {
			fail = e
		}
	}
	rootOf := map[string]string{}
	for _, r := range t.Repos {
		rootOf[r.Dir] = r.Root
	}
	broken := map[string]bool{} // repositories where saving a member's work failed: they stay as they are
	for _, m := range t.Members {
		if m.Worktree == "" {
			continue
		}
		if isDir(m.Worktree) {
			if _, err := gitCommitAll(m.Worktree, "symphony: "+m.Name+"'s uncommitted work"); err != nil {
				note(gitFailed(m.Worktree, err))
				broken[rootOf[m.Dir]] = true
				continue
			}
			if gitLooseHead(m.Worktree) {
				if _, err := runGit(m.Worktree, "branch", m.Branch+"-detached", "HEAD"); err != nil {
					note(gitFailed(m.Worktree, err))
					broken[rootOf[m.Dir]] = true
					continue
				}
			}
		}
		if !isDir(m.Dir) {
			continue // the folder is gone; so is everything in it
		}
		if err := gitRemoveWorktree(m.Dir, m.Worktree); err != nil {
			note(gitFailed(m.Dir, err))
			broken[rootOf[m.Dir]] = true
		}
	}
	for _, r := range t.Repos {
		if !r.Inited || broken[r.Root] || !isDir(filepath.Join(r.Dir, ".git")) {
			continue
		}
		if _, err := gitCommitAll(r.Dir, "symphony: changes outside task "+t.ID); err != nil {
			note(gitFailed(r.Dir, err))
			continue
		}
		before, err := gitHead(r.Dir)
		if err != nil {
			note(gitFailed(r.Dir, err))
			continue
		}
		for _, m := range t.Members {
			if rootOf[m.Dir] != r.Root || m.Branch == "" {
				continue
			}
			if err := gitMerge(r.Dir, m.Branch); err != nil {
				note(newError("task_merge_conflict", fmt.Sprintf("merging %s in %s failed: %v", m.Branch, r.Dir, err), "dir", r.Dir, "branch", m.Branch))
				// the merges that went through are taken back: the folder and the branches are as the members left them
				if err := gitResetHard(r.Dir, before); err != nil {
					note(gitFailed(r.Dir, err))
				}
				break
			}
		}
	}
	for _, r := range t.Repos {
		removeIfEmpty(filepath.Join(r.Dir, symphonyDir, "worktrees"), filepath.Join(r.Dir, symphonyDir))
	}
	return fail
}

// Stop is for a bridge on its way out, before it kills the sessions: from then on no task ends by itself (that would
// start tidying git up just as the bridge exits; the next bridge ends them, see Open), and it waits for the tasks
// already being ended.
func (h *Hub) Stop() {
	h.mu.Lock()
	h.stopping = true
	h.mu.Unlock()
	h.Flush()
}

// ForgetTask takes a failed task out of the office (and out of the record).
func (h *Hub) ForgetTask(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	t := h.taskLocked(id)
	if t == nil || t.starting {
		return ErrTaskUnknown
	}
	if t.Status != TaskFailed {
		return ErrTaskRunning
	}
	h.dropTaskPeopleLocked(t)
	h.tasks = slices.DeleteFunc(h.tasks, func(t *Task) bool { return t.ID == id })
	h.saveTasksLocked()
	h.broadcastLocked(taskEvent(id))
	return nil
}

// ---------- tasks.json ----------

// taskFile is tasks.json: the task counter and the last keepTasks tasks.
type taskFile struct {
	Next  int     `json:"next"`
	Tasks []*Task `json:"tasks"`
}

// trimTasksLocked keeps the record to keepTasks tasks, dropping the oldest ended ones.
func (h *Hub) trimTasksLocked() {
	for over := len(h.tasks) - keepTasks; over > 0; over-- {
		i := slices.IndexFunc(h.tasks, func(t *Task) bool { return t.Status == TaskEnded })
		if i < 0 {
			return
		}
		h.tasks = slices.Delete(h.tasks, i, i+1)
	}
}

// saveTasksLocked hands the tasks to the background writer.
func (h *Hub) saveTasksLocked() {
	if h.dataDir == "" {
		return
	}
	data, err := json.MarshalIndent(taskFile{Next: h.taskNext, Tasks: h.tasks}, "", "  ")
	if err != nil {
		return
	}
	h.taskSave.put(filepath.Join(h.dataDir, "tasks.json"), data)
}

// openTasksLocked reads tasks.json. A task found running or ending lost its sessions with the previous bridge: it is
// ended now, the same way. Advisors without such a task or a failed one are left over (the bridge stopped while
// making one): they go.
func (h *Hub) openTasksLocked(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, "tasks.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var f taskFile
	if len(data) > 0 {
		if err := json.Unmarshal(data, &f); err != nil {
			return fmt.Errorf("tasks.json: %w", err)
		}
	}
	h.taskNext, h.tasks = f.Next, f.Tasks
	var stale []string
	keep := map[string]bool{}
	for _, t := range h.tasks {
		if t.open() {
			t.Status = TaskEnding
			stale = append(stale, t.ID)
		}
		if t.open() || t.Status == TaskFailed {
			for _, m := range t.Members {
				keep[m.Account] = true
			}
		}
	}
	gone := false
	for id, a := range h.accounts {
		if a.Advisor && !keep[id] {
			delete(h.accounts, id)
			gone = true
		}
	}
	if gone {
		if err := h.saveLocked(); err != nil {
			return err
		}
	}
	for _, id := range stale {
		h.taskJobs.start()
		go h.finishTask(id)
	}
	return nil
}

// ---------- background work Flush waits for ----------

// jobs counts work running in the background (tasks being ended); wait returns once there is none.
type jobs struct {
	mu   sync.Mutex
	n    int
	idle chan struct{}
}

func (j *jobs) start() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.n == 0 {
		j.idle = make(chan struct{})
	}
	j.n++
}

func (j *jobs) done() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.n--; j.n == 0 {
		close(j.idle)
	}
}

func (j *jobs) wait() {
	j.mu.Lock()
	idle := j.idle
	busy := j.n > 0
	j.mu.Unlock()
	if busy {
		<-idle
	}
}
