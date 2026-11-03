package bridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

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
	memberExitWait = 20 * time.Second // a kill takes up to killGrace, plus listing what it started (ps on macOS)
)

var (
	ErrTaskUnknown = newError("task_unknown", "no such task")
	ErrTaskEnded   = newError("task_ended", "the task has ended or is ending")
	ErrTaskRunning = newError("task_running", "the task is still running; end it first")
)

type Task struct {
	ID             string            `json:"id"`
	Room           *int              `json:"room,omitempty"`   // the page's hint of the small room the task was started from; nil for none (the page seats tasks itself)
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
	// Kept is where what git ignored in the worktree (build output, .env ...) went when the task ended:
	// <folder>/.symphony/kept/<slug>-<task id>; empty when there was nothing
	Kept string `json:"kept,omitempty"`
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

// open says whether the task still holds its members and its repositories.
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

// shownTaskLocked is the task in the office (running, ending or failed, or still being made) the account is a member
// of, if any.
func (h *Hub) shownTaskLocked(account string) *Task {
	for _, t := range h.tasks {
		if t.Status != TaskEnded && t.member(account) != nil {
			return t
		}
	}
	return nil
}

// sessionTaskLocked is the task of a session the hub no longer has (pruned): the task whose member it is, or the task
// its launch was for.
func (h *Hub) sessionTaskLocked(session, account string) string {
	if session == "" {
		return ""
	}
	for _, t := range h.tasks {
		if t.Status == TaskEnded {
			continue
		}
		if m := t.member(account); m != nil && m.Session == session {
			return t.ID
		}
	}
	for _, l := range h.launches {
		if l.Session == session && l.Account == account && l.Task != "" {
			return l.Task
		}
	}
	return ""
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

// reserveTask checks that the accounts are free and takes the next task id for them. Until it is published the
// task is out of sight, but it holds the accounts.
func (h *Hub) reserveTask(accounts []string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, t := range h.tasks {
		if t.Status == TaskEnded {
			continue
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
	t := &Task{ID: "T" + strconv.Itoa(n), Status: TaskRunning, Created: h.now(), starting: true}
	for _, a := range accounts {
		t.Members = append(t.Members, &Member{Account: a})
	}
	h.tasks = append(h.tasks, t)
	h.trimTasksLocked()
	h.saveTasksLocked()
	return t.ID, nil
}

// raiseTaskNext makes sure task number n is never given out (again): the counter only goes up.
func (h *Hub) raiseTaskNext(n int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if n >= max(h.taskNext, 1) {
		h.taskNext = n + 1
		h.saveTasksLocked()
	}
}

// taskBranchRef is a task member's branch as git lists it: refs/heads/symphony/T<n>/<slug>.
var taskBranchRef = regexp.MustCompile(`^refs/heads/symphony/T([0-9]+)/`)

// usedTaskNumber is the largest task number among the symphony/T<n>/... branches of the repositories holding dirs
// (a folder in none has none). The branches stay after a task, and tasks.json needn't know them (another data
// folder's tasks, or one that was reset): given out again, a number they use would fail to make its worktrees.
func usedTaskNumber(dirs []string) int {
	used := 0
	seen := map[string]bool{}
	for _, dir := range dirs {
		if seen[dir] {
			continue
		}
		seen[dir] = true
		out, err := runGit(dir, "for-each-ref", "--format=%(refname)", "refs/heads/symphony/")
		if err != nil {
			continue
		}
		for _, ref := range strings.Split(out, "\n") {
			if m := taskBranchRef.FindStringSubmatch(ref); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil {
					used = max(used, n)
				}
			}
		}
	}
	return used
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
				h.launchOnlineLocked(l, s)
			}
		}
		if h.terms != nil {
			if term := h.terms.Get(l.ID); term != nil {
				go func(launch string) {
					<-term.Exited()
					h.memberExited(id, launch, term.ExitErr(), term.Screen(12))
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
	// its number is not given back: messages may have been logged under it, and a task taking it again would inherit them
	h.tasks = slices.DeleteFunc(h.tasks, func(t *Task) bool { return t.ID == id })
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
	base := ""
	if r := t.repo(m.Dir); r != nil {
		base = r.Base
	}
	p := map[string]string{"task": t.ID, "title": t.Title, "team": strings.Join(team, "\n"),
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
func (h *Hub) memberExited(task, launch string, exitErr error, screen string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopping {
		return // the bridge ends its sessions on its way out, and leaves the task to the next one
	}
	changed := false
	for _, s := range h.sessions {
		if s.Task == task && s.Launch == launch && s.State != StateEnded {
			h.exitedLocked(s, exitErr, screen)
			changed = true
		}
	}
	if changed {
		h.broadcastLocked(nil)
	}
	h.checkTaskLocked(task)
}

// stopTaskSessionsLocked puts a task's sessions off duty and withdraws what they left pending.
func (h *Hub) stopTaskSessionsLocked(id string) {
	for _, s := range h.sessions {
		if s.Task == id {
			h.offDutyLocked(s, "")
		}
	}
	for _, a := range h.asks {
		if s := h.sessions[a.Session]; s != nil && s.Task == id && a.Status == AskPending {
			h.cancelAskLocked(a)
		}
	}
}

// ---------- the task chat ----------

// mentionMarks are what may follow a mentioned name (besides whitespace); one right after it is dropped.
const mentionMarks = ":,：，"

// mentionEnds says whether a mentioned name may end where s begins: at the end of the text, whitespace or a mark.
func mentionEnds(s string) bool {
	r, size := utf8.DecodeRuneInString(s)
	return size == 0 || unicode.IsSpace(r) || strings.ContainsRune(mentionMarks, r)
}

// splitMentions takes the leading "@name"s off text, the names being the task's members': after each "@" the longest
// member name there that ends the text or is followed by whitespace or a mark (":", ",", "：", "，"; the mark is
// dropped, and the whitespace after). It gives the names in the order given, repeats left out, and the rest. An "@"
// that no member's name follows ends the mentions and is text; when what follows it reads like a member name of this
// task (up to the first "<suffix>", "-T7", that ends a name, on the same line), it is given as unknown instead, so a
// typo isn't sent to everyone.
func splitMentions(text string, members []string, suffix string) (names []string, unknown, rest string) {
	longest := slices.Clone(members)
	slices.SortStableFunc(longest, func(a, b string) int { return len(b) - len(a) })
	rest = text
	for strings.HasPrefix(rest, "@") {
		after := rest[1:]
		i := slices.IndexFunc(longest, func(n string) bool {
			return n != "" && strings.HasPrefix(after, n) && mentionEnds(after[len(n):])
		})
		if i < 0 {
			return names, unknownMention(after, suffix), rest
		}
		if name := longest[i]; !slices.Contains(names, name) {
			names = append(names, name)
		}
		rest = after[len(longest[i]):]
		if r, size := utf8.DecodeRuneInString(rest); size > 0 && strings.ContainsRune(mentionMarks, r) {
			rest = rest[size:]
		}
		rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
	}
	return names, "", rest
}

// unknownMention is the name an "@" was followed by when it reads like one of a task's session names: up to the first
// suffix (the task's "-T7") that ends a name, on the same line, with an account name (at most 40 characters) before it.
func unknownMention(after, suffix string) string {
	line, _, _ := strings.Cut(after, "\n")
	for from := 0; suffix != ""; {
		i := strings.Index(line[from:], suffix)
		if i < 0 {
			return ""
		}
		end := from + i + len(suffix)
		if n := utf8.RuneCountInString(line[:from+i]); n > 0 && n <= 40 && mentionEnds(line[end:]) {
			return line[:end]
		}
		from = end
	}
	return ""
}

// SayTask sends what the person wrote in a task's chat: "@<member name> ..." (one or more) to those members' sessions
// that are online, anything else to every member online. Each gets it as its next prompt from the page, as the
// employee window's input does, and the message log keeps one message for it (from the page, no sender; Missed names
// the members named that were offline).
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
	memberNames := make([]string, len(t.Members))
	for i, mb := range t.Members {
		memberNames[i] = mb.Name
	}
	names, unknown, rest := splitMentions(text, memberNames, "-"+id)
	if unknown != "" {
		return nil, newError("task_say_unknown_member", "no member named "+unknown+" in this task", "name", unknown)
	}
	var targets []*Session
	if len(names) > 0 {
		if rest == "" {
			return nil, newError("say_empty", "nothing to send")
		}
		for _, name := range names {
			mb := t.Members[slices.Index(memberNames, name)]
			if s := h.onlineTaskLocked(mb.Account, id); s != nil {
				targets = append(targets, s)
			} else {
				m.Missed = append(m.Missed, name)
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
	fails := tidyTask(t)
	unlock()
	// the page shows the first; the log has them all, the error log too (once, all of them: the task failed)
	var texts []string
	for _, e := range fails {
		texts = append(texts, e.Text)
		if h.Warn != nil {
			h.Warn(i18n.T("log.task_end_failed", "task", id, "reason", e.Text))
		}
	}
	if len(fails) > 0 {
		h.fault(Fault{Source: "task", Code: fails[0].Code, Text: strings.Join(texts, "\n"), Task: id})
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	task := h.taskLocked(id)
	if task == nil {
		return
	}
	now := h.now()
	task.Status, task.Ended = TaskEnded, &now
	for _, m := range t.Members { // tidyTask worked on a copy
		if mm := task.member(m.Account); mm != nil && m.Kept != "" {
			mm.Kept = m.Kept
		}
	}
	if len(fails) > 0 {
		task.Status, task.Error, task.ErrorCode, task.ErrorParams = TaskFailed, strings.Join(texts, "\n"), fails[0].Code, fails[0].Params
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
// or they would go with it; what git ignores in it is moved to <folder>/.symphony/kept/<slug>-<task id>, Member.Kept).
// In a repository the bridge made, uncommitted changes in the folder itself are committed too and every member's
// branch is merged into the base branch in member order; when a merge fails, the merges are taken back and the task
// fails. Nothing else is ever removed: the repositories, the bridge's own included, and the branches stay. It answers
// every failure, in the order they came; whatever one concerns stays as it is, so nothing is lost.
func tidyTask(t *Task) []*Error {
	var fails []*Error
	note := func(e *Error) { fails = append(fails, e) }
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
		// what git ignores isn't on the branch: it goes to .symphony/kept first, or the worktree stays
		if isDir(m.Worktree) {
			kept := filepath.Join(m.Dir, symphonyDir, "kept", filepath.Base(m.Worktree))
			moved, err := gitKeepIgnored(m.Worktree, kept)
			if moved {
				m.Kept = kept
			}
			if err != nil {
				note(gitFailed(m.Worktree, err))
				broken[rootOf[m.Dir]] = true
				continue
			}
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
		var branches []string
		for _, m := range t.Members {
			if rootOf[m.Dir] == r.Root && m.Branch != "" {
				branches = append(branches, m.Branch)
			}
		}
		if e := mergeBranches(r.Dir, branches); e != nil {
			note(e)
		}
	}
	for _, r := range t.Repos {
		removeIfEmpty(filepath.Join(r.Dir, symphonyDir, "worktrees"), filepath.Join(r.Dir, symphonyDir))
	}
	return fails
}

// mergeBranches merges the branches into dir's checkout, in order. When one doesn't merge, the merges before it are
// taken back, so nothing was merged and the branches stay as they were: task_merge_conflict when it conflicts,
// task_git_failed with git's words otherwise. Taking them back keeps what is uncommitted in the folder (reset --keep,
// never --hard: the folder is the user's); when git refuses, the checkout stays where it is, with the merges that went
// through, and the error says so.
func mergeBranches(dir string, branches []string) *Error {
	before, err := gitHead(dir)
	if err != nil {
		return gitFailed(dir, err)
	}
	for _, b := range branches {
		conflict, err := gitMerge(dir, b)
		if err == nil {
			continue
		}
		if uerr := gitResetKeep(dir, before); uerr != nil {
			return gitFailed(dir, fmt.Errorf("merging %s: %v; the merges before it couldn't be taken back, so the checkout was left where it is (at %s before the task's merges): %w", b, err, before, uerr))
		}
		if conflict {
			return newError("task_merge_conflict", fmt.Sprintf("merging %s in %s failed: %v", b, dir, err), "dir", dir, "branch", b)
		}
		return gitFailed(dir, fmt.Errorf("merging %s: %w", b, err))
	}
	return nil
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
	// out of the record first: its advisors can't be deleted while it is in the office
	h.tasks = slices.DeleteFunc(h.tasks, func(t *Task) bool { return t.ID == id })
	h.dropTaskPeopleLocked(t)
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
	path := filepath.Join(h.dataDir, "tasks.json")
	data, err := json.MarshalIndent(taskFile{Next: h.taskNext, Tasks: h.tasks}, "", "  ")
	if err != nil {
		h.taskSave.done(path, nil, err)
		return
	}
	h.taskSave.put(path, data)
}

// openTasksLocked reads tasks.json. A task found running or ending lost its sessions with the previous bridge: it is
// ended now, the same way. Advisors without such a task or a failed one are left over (the bridge stopped while
// making one): they go. A tasks.json that doesn't parse, empty too, stops the bridge (DamagedFile): it holds the task
// counter, and the tasks it names may have worktrees to tidy up.
func (h *Hub) openTasksLocked(dir string) error {
	path := filepath.Join(dir, "tasks.json")
	data, err := os.ReadFile(path)
	var f taskFile
	switch {
	case err == nil && len(bytes.TrimSpace(data)) == 0:
		// emptied by a crash (an older bridge didn't sync its writes): nothing in it to keep, and the task counter comes
		// back from the members' branches (raiseTaskNext); set aside like a damaged log
		aside := fmt.Sprintf("%s.corrupt-%d", path, h.now().Unix())
		if os.Rename(path, aside) != nil {
			return &DamagedFile{Path: path, Err: errors.New("empty")}
		}
		h.warn("log.data_set_aside", "file", path, "reason", "empty", "aside", aside)
	case err == nil:
		if err := json.Unmarshal(data, &f); err != nil {
			return &DamagedFile{Path: path, Err: err}
		}
	case !errors.Is(err, os.ErrNotExist):
		return err
	}
	f.Tasks = withoutNil(f.Tasks)
	for _, t := range f.Tasks {
		t.Members, t.Repos = withoutNil(t.Members), withoutNil(t.Repos)
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
