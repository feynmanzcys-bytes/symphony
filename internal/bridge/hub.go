// Package bridge is the hub of symphony: it takes the session events and pending asks the mod
// reports and pushes them to the approval page and the office page. Accounts live in a file, the rest
// in memory, all under one lock.
package bridge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
)

// session states
const (
	StateIdle     = "idle"
	StateThinking = "thinking"
	StateTool     = "tool"
	StateWaiting  = "waiting"
	StateEnded    = "ended"
	StateDone     = "done" // a subagent finished
)

// ask states
const (
	AskPending   = "pending"
	AskDone      = "done"
	AskCancelled = "cancelled"
)

const (
	keepMessages = 300
	// inputPreview: a string longer than this in a pending ask's input reaches the page cut to this length, its first
	// inputHead runes and its last ones, where a long command ends (a Write of a whole file would otherwise ride along
	// with every snapshot until the ask is settled); GET /api/asks/<id>/input has it whole
	inputPreview = 4096
	inputHead    = 3072
	// keepDoneAgents: how many finished subagents a session keeps, the newest (the office draws up to 4 by a desk)
	keepDoneAgents = 8
	// subBuffer: how many pushes a page may fall behind before the hub lets it go (deliverLocked); the busiest office
	// the tests make (42 sessions streaming) pushes about 500 a second, so this is a couple of seconds of a stalled
	// connection, on top of what the sockets hold
	subBuffer = 1024
	// how long an ended session is kept (accounts and seats stay)
	endedTTL = 10 * time.Minute
	// a sent and a received message pair up by content within this window
	pairWindow = time.Minute
	// StaleAfter: a session silent this long (the mod sends a heartbeat every 15 seconds) is lost and off duty
	StaleAfter = 45 * time.Second
)

var (
	ErrUnknownAsk     = newError("ask_unknown", "unknown ask")
	ErrSettled        = newError("ask_settled", "ask already settled")
	ErrWrongAccount   = newError("ask_wrong_account", "belongs to another account")
	ErrNotModSettled  = newError("ask_not_mod_settled", "only the page answers this ask")
	ErrNotPageAnswer  = newError("ask_not_page_answered", "this ask is answered in the session's terminal")
	ErrSessionOffline = newError("session_offline", "the session is not online")
)

type Agent struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Tool  string `json:"tool,omitempty"`
	// from agent.start: what the subagent was asked, its type and model (an older mod reports none of it)
	Type        string     `json:"type,omitempty"`
	Description string     `json:"description,omitempty"`
	Model       string     `json:"model,omitempty"`
	Background  bool       `json:"background,omitempty"`
	Started     *time.Time `json:"started,omitempty"`
	Ended       *time.Time `json:"ended,omitempty"` // agent.end
}

type Session struct {
	ID      string            `json:"id"`
	Account string            `json:"account"`
	Login   string            `json:"login,omitempty"` // the Claude login the mod reports; for display only
	Title   string            `json:"title"`           // the session name in Claude (--name / /rename)
	Cwd     string            `json:"cwd"`
	State   string            `json:"state"`
	Tool    string            `json:"tool,omitempty"`
	Prompt  string            `json:"prompt,omitempty"`
	Agents  map[string]*Agent `json:"agents"` // the running subagents and the newest finished ones (keepDoneAgents)
	Tools   int               `json:"tools"`
	Model   string            `json:"model,omitempty"`  // the model in use; sets the character's size
	Effort  string            `json:"effort,omitempty"` // the thinking effort; sets the character's muscles
	Launch  string            `json:"launch,omitempty"` // only for sessions started from the page
	Attach  string            `json:"attach,omitempty"` // where to find its terminal
	Task    string            `json:"task,omitempty"`   // the task this session works on; empty for a desk session
	Started time.Time         `json:"started"`
	Updated time.Time         `json:"updated"`
	// StateSince is when the current state began (the page uses it to tell how long a session idles; long idlers fall asleep)
	StateSince time.Time `json:"stateSince"`
	// what the mod reports besides the state and the chat (usage.go); every field is optional, an older mod sends none
	Surface     string      `json:"surface,omitempty"`  // where the session draws: terminal, desktop, vscode, mobile
	Surfaces    []string    `json:"surfaces,omitempty"` // every surface attached (a phone, the desktop app)
	Engine      *EngineInfo `json:"engine,omitempty"`
	Mod         string      `json:"mod,omitempty"` // the mod's version
	Repo        *RepoInfo   `json:"repo,omitempty"`
	Usage       *Usage      `json:"usage,omitempty"`
	Turns       int         `json:"turns"`                 // main-loop turns completed
	Compactions int         `json:"compactions,omitempty"` // times the conversation was compacted
	AgentsTotal int         `json:"agentsTotal,omitempty"` // every subagent the session had, those no longer in Agents too
	Todos       []*Todo     `json:"todos,omitempty"`       // the agent team's open tasks
	BgTasks     int         `json:"bgTasks,omitempty"`     // background tasks running at the last turn's end
	BgCrons     int         `json:"bgCrons,omitempty"`     // scheduled runs
	ToolCount   int         `json:"toolCount,omitempty"`   // tools the session has
	MCP         []string    `json:"mcp,omitempty"`         // the MCP servers among them
	ToolMs      float64     `json:"toolMs,omitempty"`      // time spent in tool calls, milliseconds
	LastError   string      `json:"lastError,omitempty"`   // the last API error that ended a turn
	EndReason   string      `json:"endReason,omitempty"`   // why the session ended (clear, logout, ...)
	Notify      *Notify     `json:"notify,omitempty"`      // the last notification
	Rules       []Rule      `json:"rules,omitempty"`       // what the page allowed for the rest of the session (session.rule)
	costSeen    float64     // the session's cost at the last report, for the ledger's difference
}

// Rule is what a person allowed on the page for the rest of a session: a tool (every file edit for "Edit"), and for
// Bash the commands that start with Prefix. The mod proposes it with an ask (Ask.Remember) and keeps it; the page only
// says yes.
type Rule struct {
	Tool   string `json:"tool"`
	Prefix string `json:"prefix,omitempty"`
}

// AskField is one field of an MCP server's form (an elicitation), as the page shows it: what the server asks for, never
// answered from the page (the session's terminal answers it).
type AskField struct {
	Name        string   `json:"name"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type,omitempty"` // string | number | integer | boolean
	Required    bool     `json:"required,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type Ask struct {
	ID        string         `json:"id"`
	Session   string         `json:"session"`
	Account   string         `json:"account"`
	Kind      string         `json:"kind"` // permission | question | elicitation
	Tool      string         `json:"tool,omitempty"`
	Input     any            `json:"input,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	Questions any            `json:"questions,omitempty"`
	AgentID   string         `json:"agentId,omitempty"`
	Remember  *Rule          `json:"remember,omitempty"` // what an allow could be kept as for the session (a permission's)
	Server    string         `json:"server,omitempty"`   // an elicitation's MCP server
	Message   string         `json:"message,omitempty"`  // an elicitation's question
	Mode      string         `json:"mode,omitempty"`     // an elicitation's: form | url
	URL       string         `json:"url,omitempty"`      // an elicitation's page to open (mode url)
	Fields    []AskField     `json:"fields,omitempty"`   // an elicitation's form
	Status    string         `json:"status"`
	Answer    map[string]any `json:"answer,omitempty"`
	Created   time.Time      `json:"created"`
	// InputTruncated says that the Input the page gets is a preview: long strings in it are cut (inputPreview); GET
	// /api/asks/<id>/input has it whole
	InputTruncated bool `json:"inputTruncated,omitempty"`

	preview any // Input as the page gets it
	done    chan struct{}
}

// Message is one internal message between sessions (SendMessage / the agent team mailbox).
type Message struct {
	ID        string    `json:"id"`
	From      string    `json:"from,omitempty"` // the sender's session id; empty when unknown
	FromName  string    `json:"fromName"`
	To        string    `json:"to,omitempty"`
	ToName    string    `json:"toName"`
	ToNames   []string  `json:"toNames,omitempty"` // the task chat's mentioned members, when the page sent it to some
	Missed    []string  `json:"missed,omitempty"`  // of those, the ones that were offline and didn't get it
	Text      string    `json:"text"`
	Origin    string    `json:"origin,omitempty"`
	AgentID   string    `json:"agentId,omitempty"`
	Task      string    `json:"task,omitempty"` // the task it belongs to: the sender's, else the receiver's; empty between desks
	Delivered bool      `json:"delivered"`
	Received  bool      `json:"received"`
	Time      time.Time `json:"time"`
}

// The largest duration and cost a report may carry: the hub adds them up, and a sum that ran off to infinity couldn't be
// encoded for the pages any more.
const (
	maxReportedMs  = 1e12 // about 30 years
	maxReportedUSD = 1e9
)

// saneNumbers drops a duration or a cost no session can mean: negative, past the maximum, or not a number at all.
func (ev *Event) saneNumbers() {
	if !(ev.DurationMs >= 0 && ev.DurationMs <= maxReportedMs) {
		ev.DurationMs = 0
	}
	if u := ev.Usage; u != nil && u.Cost != nil && !(u.Cost.USD >= 0 && u.Cost.USD <= maxReportedUSD) {
		cp := *u
		cp.Cost = nil
		ev.Usage = &cp
	}
}

// Event is one event the mod reports; which fields matter depends on kind.
type Event struct {
	Session     string  `json:"session"`
	Kind        string  `json:"kind"`
	Login       string  `json:"login,omitempty"` // the Claude login the mod reports
	Model       string  `json:"model,omitempty"`
	Effort      string  `json:"effort,omitempty"`
	Launch      string  `json:"launch,omitempty"` // the id the bridge gave when the page started the session
	Task        string  `json:"task,omitempty"`   // the task id of a session started for a task (SYMPHONY_TASK)
	Cwd         string  `json:"cwd,omitempty"`
	Title       string  `json:"title,omitempty"`
	Args        string  `json:"args,omitempty"`
	Text        string  `json:"text,omitempty"`
	Tool        string  `json:"tool,omitempty"`
	AgentID     string  `json:"agentId,omitempty"`
	To          string  `json:"to,omitempty"`
	Origin      any     `json:"origin,omitempty"`
	Delivered   bool    `json:"delivered,omitempty"`
	Denied      bool    `json:"denied,omitempty"`
	IsError     bool    `json:"isError,omitempty"`
	DurationMs  float64 `json:"durationMs,omitempty"`
	IsAborted   bool    `json:"isAborted,omitempty"`
	TurnID      string  `json:"turnId,omitempty"`
	ToolUseID   string  `json:"toolUseId,omitempty"`
	Interactive bool    `json:"interactive,omitempty"`
	// the chat: a tool call's main argument (tool.start), and the model's response in pieces (chat.delta / chat.stop)
	Summary string      `json:"summary,omitempty"`
	Step    int         `json:"step,omitempty"`
	Blocks  []ChatDelta `json:"blocks,omitempty"`
	// what else the mod reports (usage.go): all optional
	Surface     string      `json:"surface,omitempty"`    // session.start
	Surfaces    []string    `json:"surfaces,omitempty"`   // session.surfaces
	Engine      *EngineInfo `json:"engine,omitempty"`     // session.start
	Mod         string      `json:"mod,omitempty"`        // session.start: the mod's version
	Repo        *RepoInfo   `json:"repo,omitempty"`       // session.start
	Usage       *Usage      `json:"usage,omitempty"`      // session.usage: context, rateLimits, cost
	Tokens      *Tokens     `json:"tokens,omitempty"`     // chat.stop: the response's; turn.complete: the turn's
	StopReason  string      `json:"stopReason,omitempty"` // chat.stop
	Reason      string      `json:"reason,omitempty"`     // turn.complete: answer | aborted | refusal | error; session.end: why
	Refusal     *Refusal    `json:"refusal,omitempty"`    // turn.complete
	Details     string      `json:"details,omitempty"`    // turn.error
	AgentType   string      `json:"agentType,omitempty"`  // agent.start / agent.end
	Description string      `json:"description,omitempty"`
	Background  bool        `json:"background,omitempty"`
	Todo        string      `json:"todo,omitempty"` // todo.create / todo.done: the item's id
	Subject     string      `json:"subject,omitempty"`
	Teammate    string      `json:"teammate,omitempty"`
	NotifyType  string      `json:"notifyType,omitempty"` // session.notify
	Trigger     string      `json:"trigger,omitempty"`    // session.compact: manual | auto
	From        string      `json:"from,omitempty"`       // session.model: the model switched from
	Source      string      `json:"source,omitempty"`     // session.model: what switched it
	Command     string      `json:"command,omitempty"`    // command.run: the slash command's name
	BgTasks     int         `json:"bgTasks,omitempty"`    // session.background
	BgCrons     int         `json:"bgCrons,omitempty"`
	ToolCount   int         `json:"toolCount,omitempty"` // session.tools
	MCP         []string    `json:"mcp,omitempty"`
	Rule        *Rule       `json:"rule,omitempty"` // session.rule: what the page allowed for the rest of the session
}

// Snapshot is the whole state as the pages see it. Messages is the message log in a full snapshot (the first one a
// subscriber gets, and /api/state) and null in the ones pushed on every change: the page keeps its own log and gets
// each new or updated message as a "message" event, so a tool call doesn't carry 300 messages along.
type Snapshot struct {
	Type     string        `json:"type"`
	Protocol int           `json:"protocol"` // Protocol: what the pages may expect of the bridge
	Started  time.Time     `json:"started"`  // when the bridge started
	Accounts []AccountView `json:"accounts"`
	Sessions []*Session    `json:"sessions"`
	Asks     []*Ask        `json:"asks"`
	Messages []*Message    `json:"messages"`
	Launches []*Launch     `json:"launches"`
	Tasks    []*Task       `json:"tasks"` // running, ending and failed ones
	// MessageTail is the latest message's id, in every snapshot: a page that doesn't have it fell behind and
	// fetches the log again (/api/state)
	MessageTail string `json:"messageTail,omitempty"`
	// Ledger is today's rows: what each account's sessions cost today (/api/ledger has every day kept)
	Ledger []*LedgerRow `json:"ledger"`
}

// Push is one message for the subscribers: an event or a snapshot, encoded once for all of them when it is made, under
// the hub's lock: what it holds (a chat item, a message, a session's usage) is the hub's own and keeps changing.
type Push struct {
	// Value is what was encoded, for Go code in this process (tests); it may share what the hub changes later, so read
	// only what never changes (an id, a kind), or JSON
	Value any
	data  []byte
}

// pushLocked encodes v into a push; the hub's lock must be held, so the push is the state of this moment. A value that
// doesn't encode (a number that isn't finite, which saneNumbers keeps out) gives nil, and the caller sends nothing: an
// empty message would break the page, and the log says why (Warn, at most once a minute).
func (h *Hub) pushLocked(v any) *Push {
	data, err := json.Marshal(v)
	if err != nil {
		if now := h.now(); h.Warn != nil && now.Sub(h.warned) >= time.Minute {
			h.warned = now
			h.Warn(i18n.T("log.push_failed", "reason", err.Error()))
		}
		return nil
	}
	return &Push{Value: v, data: data}
}

// JSON is the message encoded, the same bytes for every subscriber.
func (p *Push) JSON() []byte { return p.data }

// Hub holds all the state.
type Hub struct {
	mu         sync.Mutex
	accounts   map[string]*Account
	dataDir    string // accounts and the message log are stored here; empty means nothing goes to disk (tests)
	sessions   map[string]*Session
	asks       map[string]*Ask
	messages   []*Message
	launches   map[string]*Launch
	chats      map[string]*chatLog // by session id
	inboxes    map[string]*inbox   // by session id
	subs       map[chan *Push]struct{}
	now        func() time.Time
	msgSave    saver   // writes the message log in the background
	tasks      []*Task // the record, oldest first (tasks.json)
	taskNext   int     // the next task's number
	taskSave   saver   // writes tasks.json in the background
	taskJobs   jobs    // tasks being ended
	stopping   bool    // the bridge is on its way out (Stop)
	started    time.Time
	ledger     []*LedgerRow // cost and tokens by account and day (ledger.json)
	ledgerSave saver
	activity   []*Activity // the last keepActivity things that happened
	// gitLocks let one task at a time make or tidy up the repositories of a folder tree (tasks may share a folder)
	gitLocks treeLocks
	// AttachHint tells, by launch id, where to find a session's terminal; the current terminal backend provides it
	AttachHint func(launchID string) string
	// Warn writes a line to the bridge's log (the program sets it; nothing without it)
	Warn   func(line string)
	warned time.Time // when Warn last said a push couldn't be encoded
	// behind is when Warn last said a page fell behind
	behind time.Time
	// terms are the sessions' terminals when the bridge hosts them (ending a task kills its members' processes)
	terms *Terms
	// reserved are the accounts a start from the page is being made for (reserveDesk)
	reserved map[string]bool
}

func NewHub() *Hub {
	return &Hub{
		accounts: map[string]*Account{},
		sessions: map[string]*Session{},
		asks:     map[string]*Ask{},
		launches: map[string]*Launch{},
		chats:    map[string]*chatLog{},
		inboxes:  map[string]*inbox{},
		subs:     map[chan *Push]struct{}{},
		now:      time.Now,
		started:  time.Now(),
	}
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------- subscriptions ----------

// Subscribe returns a channel that first gets a full snapshot, then an event and a new snapshot (without the
// message log) on every change. The hub closes it when the subscriber falls too far behind (deliverLocked); a new
// subscription starts again from a full snapshot.
func (h *Hub) Subscribe() (<-chan *Push, func()) {
	ch := make(chan *Push, subBuffer)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	if p := h.pushLocked(h.snapshotLocked(true)); p != nil {
		ch <- p
	}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch) // only deliverLocked closes it, as it lets it go
		h.mu.Unlock()
	}
}

// deliverLocked hands a push to every subscriber. One whose channel is full has stopped reading (a frozen tab, a
// stalled connection): rather than go on with a hole it can't see (a piece of a streamed answer, an ask), it is let
// go: what it has queued is dropped (it comes back for a full snapshot, and a frozen tab would hold on to it) and its
// channel closed, so its stream ends and the page's EventSource comes back. The log says so, once a minute at most.
func (h *Hub) deliverLocked(p *Push) {
	for ch := range h.subs {
		select {
		case ch <- p:
		default:
			delete(h.subs, ch)
			for len(ch) > 0 {
				select {
				case <-ch:
				default: // the stream took the last one meanwhile
				}
			}
			close(ch)
			if now := h.now(); h.Warn != nil && now.Sub(h.behind) >= time.Minute {
				h.behind = now
				h.Warn(i18n.T("log.page_behind"))
			}
		}
	}
}

// broadcastLocked pushes an event (optional), then the latest snapshot. Each is encoded once, however many
// subscribers there are.
func (h *Hub) broadcastLocked(extra any) {
	if len(h.subs) == 0 {
		return
	}
	if extra != nil {
		if ev := h.pushLocked(extra); ev != nil {
			h.deliverLocked(ev)
		}
	}
	if snap := h.pushLocked(h.snapshotLocked(false)); snap != nil {
		h.deliverLocked(snap)
	}
}

// Snapshot is the whole state, message log included. It is read or encoded once the lock is let go (/api/state), so
// unlike the pushes it gets copies of what the hub changes in place too: the usage totals and the todos.
func (h *Hub) Snapshot() Snapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	snap := h.snapshotLocked(true)
	for _, s := range snap.Sessions { // copies of the hub's sessions
		s.Usage = s.Usage.copy()
		if s.Todos != nil {
			todos := make([]*Todo, len(s.Todos))
			for i, t := range s.Todos {
				tc := *t
				todos[i] = &tc
			}
			s.Todos = todos
		}
	}
	return snap
}

// snapshotLocked builds a snapshot; a full one carries the message log, the others leave it null.
func (h *Hub) snapshotLocked(full bool) Snapshot {
	snap := Snapshot{Type: "snapshot", Protocol: Protocol, Started: h.started, Accounts: []AccountView{}, Sessions: []*Session{}, Asks: []*Ask{}, Launches: []*Launch{}, Tasks: h.tasksLocked(), Ledger: h.ledgerTodayLocked()}
	for _, a := range h.accounts {
		v := AccountView{ID: a.ID, Name: a.Name, Seat: a.Seat, Advisor: a.Advisor, Last: a.Last, Dir: a.Dir}
		if s := h.onlineLocked(a.ID); s != nil {
			v.Session = s.ID
		}
		snap.Accounts = append(snap.Accounts, v)
	}
	sort.Slice(snap.Accounts, func(i, j int) bool { return snap.Accounts[i].Seat < snap.Accounts[j].Seat })
	for _, s := range h.sessions {
		cp := *s
		cp.Agents = make(map[string]*Agent, len(s.Agents))
		for k, v := range s.Agents {
			a := *v
			cp.Agents[k] = &a
		}
		if s.Usage != nil {
			u := *s.Usage
			cp.Usage = &u
		}
		snap.Sessions = append(snap.Sessions, &cp)
	}
	sort.Slice(snap.Sessions, func(i, j int) bool { return snap.Sessions[i].Started.Before(snap.Sessions[j].Started) })
	for _, a := range h.asks {
		if a.Status == AskPending {
			cp := *a
			cp.Input = a.preview
			snap.Asks = append(snap.Asks, &cp)
		}
	}
	sort.Slice(snap.Asks, func(i, j int) bool { return snap.Asks[i].Created.Before(snap.Asks[j].Created) })
	if n := len(h.messages); n > 0 {
		snap.MessageTail = h.messages[n-1].ID
	}
	if full {
		snap.Messages = make([]*Message, 0, len(h.messages))
		for _, m := range h.messages {
			cp := *m
			snap.Messages = append(snap.Messages, &cp)
		}
	}
	for _, l := range h.launches {
		cp := *l
		snap.Launches = append(snap.Launches, &cp)
	}
	sort.Slice(snap.Launches, func(i, j int) bool { return snap.Launches[i].Time.Before(snap.Launches[j].Time) })
	return snap
}

// ---------- sessions ----------

// joinableLocked says whether s, a session online that named no task, may become task's, and makes its place free: the
// task runs, s's account is a member, and no other session of the account works in it, or the one that does gives way
// as a new session of the task would make it: it runs in the same page-started process (its launch id is the one s or
// this report names: a member's session after /clear whose old session's end never came), or it has been silent for
// StaleAfter (lost).
func (h *Hub) joinableLocked(s *Session, task, launch string) bool {
	t := h.taskLocked(task)
	if t == nil || t.Status != TaskRunning || t.member(s.Account) == nil {
		return false
	}
	switch other := h.onlineTaskLocked(s.Account, task); {
	case other == nil || other.ID == s.ID:
		return true
	case other.Launch != "" && (other.Launch == s.Launch || other.Launch == launch):
		h.offDutyLocked(other, "") // the launch id proves it is the same process
		return true
	case h.now().Sub(other.Updated) >= StaleAfter:
		h.offDutyLocked(other, "lost")
		return true
	}
	return false
}

// sessionLocked returns the session, creating it if needed. An account serves one desk session at a time, and one
// session per task it is in: while it has a session online that isn't lost on the same desk or task, a new one is
// refused (ErrAccountBusy), unless it starts in the same process (restart, the launch id of a session.start, is the
// online one's): a fresh session after /clear may report in before the one it replaces says goodbye, which then ends.
// A task session only comes (back) online while its task runs (ErrTaskEnded).
func (h *Hub) sessionLocked(id string, acc *Account, task, restart, launch string) (s *Session, fresh bool, err error) {
	s = h.sessions[id]
	if s != nil && s.Account != acc.ID {
		return nil, false, ErrWrongAccount
	}
	if s != nil && s.State != StateEnded {
		// a session that named no task and now names one is a task member whose first report went out before it said
		// so (a mod before 1.1.1 reported the session's name as the engine started it): it joins its task now, as a
		// session new to it would
		if s.Task == "" && task != "" && h.joinableLocked(s, task, launch) {
			for sid, old := range h.sessions {
				if old.Account == acc.ID && old.Task == task && sid != id {
					h.dropSessionLocked(sid)
				}
			}
			s.Task = task
			return s, true, nil
		}
		return s, false, nil
	}
	// a report that was on its way when the session's process exited (it names that process's launch) doesn't bring
	// the session back; one from another launch (claude --continue from the page) or from a session started by hand
	// (no launch) does
	if s != nil && s.Launch != "" && launch == s.Launch && h.terms != nil {
		if t := h.terms.Get(s.Launch); t != nil && t.Ended() {
			return nil, false, ErrSessionGone
		}
	}
	if task != "" {
		if t := h.taskLocked(task); t == nil || t.Status != TaskRunning || t.member(acc.ID) == nil {
			return nil, false, ErrTaskEnded
		}
	}
	if other := h.onlineTaskLocked(acc.ID, task); other != nil && other.ID != id {
		switch {
		case restart != "" && other.Launch == restart:
			h.offDutyLocked(other, "") // the launch id proves it is the same process
		case h.now().Sub(other.Updated) < StaleAfter:
			return nil, false, ErrAccountBusy
		default:
			h.offDutyLocked(other, "lost") // that one was lost long ago; make way
		}
	}
	// an account keeps the record of its current desk session only, and of one session per task
	for sid, old := range h.sessions {
		if old.Account == acc.ID && old.Task == task && sid != id {
			h.dropSessionLocked(sid)
		}
	}
	now := h.now()
	if s == nil {
		s = &Session{ID: id, Account: acc.ID, Agents: map[string]*Agent{}, Started: now}
		h.sessions[id] = s
	}
	s.Task = task
	s.State, s.Updated, s.StateSince = StateIdle, now, now // lost and back again; carry on
	s.EndReason = ""
	return s, true, nil
}

// Apply handles one mod event. acc is the account its token belongs to.
func (h *Hub) Apply(ev Event, acc *Account) error { return h.apply(ev, acc, nil) }

// Report is Apply, and says whether the event made the hub create the session's record: an event other than
// session.start from a session it didn't know (the bridge restarted, or forgot the session long after it was lost).
// The mod then reports again what it tells only now and then (its tools).
func (h *Hub) Report(ev Event, acc *Account) (fresh bool, err error) {
	err = h.apply(ev, acc, &fresh)
	return fresh, err
}

// sessionInfo is what the mod tells of a session besides what it does; the heartbeats carry it too, so a restarted
// bridge learns it again, and a heartbeat pushes only when it changed some of it.
type sessionInfo struct {
	login, model, effort, cwd, mod, surface string
	engine                                  EngineInfo
	repo                                    RepoInfo
}

func (s *Session) info() sessionInfo {
	in := sessionInfo{login: s.Login, model: s.Model, effort: s.Effort, cwd: s.Cwd, mod: s.Mod, surface: s.Surface}
	if s.Engine != nil {
		in.engine = *s.Engine
	}
	if s.Repo != nil {
		in.repo = *s.Repo
	}
	return in
}

// apply handles one mod event; created, when given, is set as Report says.
func (h *Hub) apply(ev Event, acc *Account, created *bool) error {
	ev.saneNumbers()
	h.mu.Lock()
	defer h.mu.Unlock()

	if ev.Kind == "session.end" {
		// an unknown session going off duty (it started before the bridge restarted); no need to create it
		if s := h.sessions[ev.Session]; s == nil || s.Account != acc.ID || s.State == StateEnded {
			return nil
		}
	}
	task := ev.Task
	if s := h.sessions[ev.Session]; s != nil && task == "" {
		task = s.Task
	}
	// a mod too old to report the task: a session started for one is known by its launch
	if l := h.launches[ev.Launch]; task == "" && ev.Launch != "" && l != nil && l.Account == acc.ID {
		task = l.Task
	}
	if task == "" && h.sessions[ev.Session] == nil {
		task = h.sessionTaskLocked(ev.Session, acc.ID) // a member's session pruned while it was away
	}
	restart := ""
	if ev.Kind == "session.start" {
		restart = ev.Launch
	}
	known := h.sessions[ev.Session] != nil
	s, fresh, err := h.sessionLocked(ev.Session, acc, task, restart, ev.Launch)
	if err != nil {
		return err
	}
	if created != nil {
		*created = !known && ev.Kind != "session.start"
	}
	info := s.info()
	if ev.Login != "" {
		s.Login = ev.Login
	}
	// the model and effort are the session's; a subagent's start names its own (kept on the agent, detailsLocked)
	if ev.Model != "" && ev.Kind != "agent.start" {
		s.Model = ev.Model
	}
	if ev.Effort != "" && ev.Kind != "agent.start" {
		s.Effort = ev.Effort
	}
	// every mod event carries the folder and the launch id, so a restarted bridge can restore them
	if ev.Cwd != "" {
		s.Cwd = ev.Cwd
		// the account remembers where its desk session works (however it was started), so the employee can join a
		// task there once off duty
		if a := h.accounts[acc.ID]; s.Task == "" && a != nil && a.Dir != ev.Cwd {
			a.Dir = ev.Cwd
			_ = h.saveLocked()
		}
	}
	// what the mod knows of the engine and the folder at the start; a session that moves reports its folder again
	if ev.Engine != nil {
		s.Engine = ev.Engine
	}
	if ev.Mod != "" {
		s.Mod = ev.Mod
	}
	if ev.Repo != nil {
		// an older mod reports the remote as git has it, credentials and all: neither the session nor the event pushed
		// to the pages keeps them
		repo := *ev.Repo
		repo.Remote = withoutUserinfo(repo.Remote)
		ev.Repo, s.Repo = &repo, &repo
	}
	if ev.Surface != "" {
		s.Surface = ev.Surface
	}
	// a session.start names the launch it runs in, also for a session that ran in another before (claude --continue
	// goes on under the same session id)
	if ev.Launch != "" && (s.Launch == "" || (ev.Kind == "session.start" && s.Launch != ev.Launch)) {
		s.Launch = ev.Launch
		if h.AttachHint != nil {
			s.Attach = h.AttachHint(ev.Launch) // the launch record is gone after a restart; the terminal backend derives it from the id
		}
		if l := h.launches[ev.Launch]; l != nil && l.Account == acc.ID {
			h.launchOnlineLocked(l, s)
		}
	}
	s.Updated = h.now()
	if fresh && s.Task != "" {
		h.joinTaskLocked(s, ev.Kind == "session.start")
	}
	if ev.Kind == "heartbeat" && !fresh && s.info() == info {
		return nil // a heartbeat only keeps the session alive; nothing to tell the page
	}
	// the chat's own events change nothing in the office: they go to the pages as chat events, without a snapshot;
	// so do the usage figures (a "usage" push), as they move on every response
	chatOnly := true
	switch ev.Kind {
	case "chat.delta":
		h.chatStreamLocked(s, ev.TurnID, ev.Step, ev.Blocks)
	case "chat.stop":
		h.chatStopLocked(s, ev.TurnID, ev.Step)
		h.countTokensLocked(s, ev.Tokens)
	case "chat.user":
		h.chatUserLocked(s, ev)
	case "chat.sent":
		h.chatSentLocked(s, ev.Text)
	case "chat.dropped":
		h.chatDroppedLocked(s, ev.Text, ev.Reason)
	case "session.usage":
		if ev.Usage != nil {
			h.usageLocked(s, ev.Usage)
		}
	case "command.run":
		h.chatCommandLocked(s, ev)
	case "tool.fail":
		h.chatToolFailLocked(s, ev)
	default:
		chatOnly = false
	}
	if chatOnly && !fresh {
		return nil
	}

	before := s.State

	var target *Agent
	if ev.AgentID != "" {
		target = s.Agents[ev.AgentID]
		// the end of an agent the session never showed is no subagent to show or count: the engine ends its own helpers
		// as subagents too (the prompt suggestion it makes after a turn), and one let go already stays gone
		if target == nil && ev.Kind != "agent.end" {
			target = &Agent{ID: ev.AgentID, State: StateIdle}
			s.Agents[ev.AgentID] = target
			s.AgentsTotal++
		}
	}
	setState := func(state, tool string) {
		if target != nil {
			target.State, target.Tool = state, tool
			return
		}
		s.State, s.Tool = state, tool
	}

	var extra any = map[string]any{"type": "event", "event": ev, "title": acc.Name}
	switch ev.Kind {
	case "session.start":
		h.noteLocked(s, "start", "")
	case "session.end":
		h.offDutyLocked(s, ev.Reason)
	case "session.model":
		// a switch (/model, the picker, a fallback) is worth a line in the chat; the model itself was set above
		if ev.From != "" && ev.From != ev.Model {
			h.chatNoticeDetailLocked(s.ID, "modelSwitched", ev.From+" → "+ev.Model)
		}
	case "session.cwd", "session.surfaces", "session.tools", "session.background", "session.compact", "session.notify", "session.rule", "todo.create", "todo.done", "agent.start", "agent.end", "turn.error":
		h.detailsLocked(s, ev, target)
	case "session.title":
		// a new session's SessionStart carries no name; don't clear the known one
		title := strings.TrimSpace(ev.Title)
		if title == "" {
			title = strings.TrimSpace(ev.Args)
		}
		if title != "" {
			s.Title = title
		}
	case "turn.start":
		setState(StateThinking, "")
		if target == nil && ev.Text != "" {
			s.Prompt = ev.Text
		}
	case "turn.complete":
		if target != nil {
			setState(StateDone, "")
		} else {
			setState(StateIdle, "")
			s.Turns++
			h.ledgerRowLocked(s.Account).Turns++
			h.saveLedgerLocked()
			switch {
			case ev.IsAborted || ev.Reason == "aborted":
				h.chatNoticeLocked(s.ID, "aborted")
			case ev.Reason == "refusal":
				detail := ""
				if ev.Refusal != nil {
					detail = strings.TrimSpace(ev.Refusal.Category + " " + ev.Refusal.Explanation)
				}
				h.chatNoticeDetailLocked(s.ID, "refused", detail)
				// an API error (reason: error) gets its notice from turn.error, which names it
			}
		}
	case "tool.start":
		setState(StateTool, ev.Tool)
		s.Tools++
		if target == nil {
			h.ledgerRowLocked(s.Account).Tools++
		}
		h.chatToolLocked(s, ev, false)
		h.noteLocked(s, "tool", strings.TrimSpace(ev.Tool+" "+ev.Summary))
	case "tool.end":
		setState(StateThinking, "")
		s.ToolMs += ev.DurationMs
		h.chatToolLocked(s, ev, true)
	case "message.receive":
		if m := h.receiveLocked(s, ev); m != nil {
			extra = map[string]any{"type": "message", "message": m}
			h.saveMessagesLocked()
			h.chatMessageLocked(s.ID, m)
		}
	case "message.send":
		if m := h.sendLocked(s, ev); m != nil {
			extra = map[string]any{"type": "message", "message": m}
			h.saveMessagesLocked()
			h.chatMessageLocked(s.ID, m)
		}
	}
	h.refreshWaitingLocked(s)
	h.markStateLocked(s, before)
	h.broadcastLocked(extra)
	if ev.Kind == "session.end" && s.Task != "" {
		h.checkTaskLocked(s.Task) // the last member gone ends the task
	}
	return nil
}

// detailsLocked takes the events that only fill in what the pages may show of a session: its folder (already set
// above), the surfaces attached, the tools, the background work, a compaction, a notification, the agent team's
// tasks, a subagent's name, an API error.
func (h *Hub) detailsLocked(s *Session, ev Event, target *Agent) {
	switch ev.Kind {
	case "session.surfaces":
		s.Surfaces = ev.Surfaces
	case "session.tools":
		s.ToolCount, s.MCP = ev.ToolCount, ev.MCP
	case "session.background":
		s.BgTasks, s.BgCrons = ev.BgTasks, ev.BgCrons
	case "session.compact":
		s.Compactions++
		h.chatNoticeLocked(s.ID, "compacted")
	case "session.rule":
		if ev.Rule != nil && ev.Rule.Tool != "" {
			r := Rule{Tool: cutRunes(ev.Rule.Tool, 200), Prefix: cutRunes(ev.Rule.Prefix, 200)}
			if !slices.Contains(s.Rules, r) && len(s.Rules) < 100 {
				s.Rules = append(s.Rules, r)
				h.chatNoticeDetailLocked(s.ID, "remembered", strings.TrimSpace(r.Tool+" "+r.Prefix))
			}
		}
	case "session.notify":
		s.Notify = &Notify{Type: ev.NotifyType, Title: ev.Title, Text: ev.Text, Time: h.now()}
		if ev.NotifyType != "idle_prompt" { // "waiting for your input" every minute is no news for the chat
			h.chatNoticeDetailLocked(s.ID, "notify", strings.TrimSpace(ev.Title+" "+ev.Text))
		}
	case "todo.create":
		h.todoLocked(s, ev, false)
	case "todo.done":
		h.todoLocked(s, ev, true)
	case "agent.start":
		if target != nil {
			now := h.now()
			target.Type, target.Description, target.Model, target.Background, target.Started = ev.AgentType, ev.Description, ev.Model, ev.Background, &now
		}
	case "agent.end":
		if target != nil {
			now := h.now()
			target.State, target.Tool, target.Ended = StateDone, "", &now
			dropOldAgents(s, target)
		}
	case "turn.error":
		s.LastError = strings.TrimSpace(ev.Text + " " + ev.Details)
		h.chatNoticeDetailLocked(s.ID, "error", s.LastError)
		h.noteLocked(s, "error", s.LastError)
	}
}

// dropOldAgents forgets a session's finished subagents but the newest keepDoneAgents, by when they ended: every
// snapshot carries them, and a long session may run hundreds. The one that just ended (last) always stays, so a late
// event of it finds it rather than counting it again.
func dropOldAgents(s *Session, last *Agent) {
	var done []*Agent
	for _, a := range s.Agents {
		if a.State == StateDone && a.Ended != nil && a != last {
			done = append(done, a)
		}
	}
	if extra := len(done) - (keepDoneAgents - 1); extra > 0 {
		sort.Slice(done, func(i, j int) bool { return done[i].Ended.Before(*done[j].Ended) })
		for _, a := range done[:extra] {
			delete(s.Agents, a.ID)
		}
	}
}

// markStateLocked records when the state changes.
func (h *Hub) markStateLocked(s *Session, before string) {
	if s.State != before {
		s.StateSince = h.now()
	}
}

// refreshWaitingLocked marks a session with pending asks as waiting, and puts it back to thinking when none are left.
func (h *Hub) refreshWaitingLocked(s *Session) {
	if s.State == StateEnded {
		return
	}
	for _, a := range h.asks {
		if a.Session == s.ID && a.Status == AskPending {
			s.State = StateWaiting
			return
		}
	}
	if s.State == StateWaiting {
		s.State = StateThinking
	}
}

// offDutyLocked is the one way a session goes off duty, whatever the cause: it said goodbye, it was lost, its process
// went, its task ended, a new session took its place. The state and why (when known), the chat's notice, the activity
// line, and its pending asks withdrawn; a session already off duty is left as it is. The caller broadcasts and checks
// the session's task.
func (h *Hub) offDutyLocked(s *Session, reason string) {
	if s.State == StateEnded {
		return
	}
	before := s.State
	s.State, s.Tool = StateEnded, ""
	if reason != "" {
		s.EndReason = reason
	}
	h.markStateLocked(s, before)
	h.chatNoticeLocked(s.ID, "ended")
	h.noteLocked(s, "end", reason)
	h.cancelSessionAsksLocked(s.ID)
}

// cancelSessionAsksLocked withdraws what a session left pending.
func (h *Hub) cancelSessionAsksLocked(id string) {
	for _, a := range h.asks {
		if a.Session == id && a.Status == AskPending {
			h.cancelAskLocked(a)
		}
	}
}

// cancelAskLocked withdraws a pending ask nobody will answer: the waiting mod gets "cancelled" (and the terminal asks),
// the chat's card says so and the pages are told, as when the mod withdraws it itself.
func (h *Hub) cancelAskLocked(a *Ask) {
	a.Status = AskCancelled
	close(a.done)
	h.chatSettledLocked(a)
	h.noteLocked(h.sessions[a.Session], "settled", strings.TrimSpace(AskCancelled+" "+a.Tool))
	h.broadcastLocked(map[string]any{"type": "settled", "id": a.ID, "status": AskCancelled})
}

// exitedLocked puts a session whose process is gone off duty (a process that dies has no time to say goodbye). One that
// didn't end well (a crash, a kill) says how in its chat, with the last lines of its screen.
func (h *Hub) exitedLocked(s *Session, exitErr error, screen string) {
	if s.State == StateEnded {
		return
	}
	if !cleanExit(exitErr) {
		detail := exitErr.Error()
		if lines := tellingLines(screen, 4, 400); lines != "" {
			detail += "\n" + lines
		}
		h.chatNoticeDetailLocked(s.ID, "exited", detail)
		h.noteLocked(s, "error", exitErr.Error())
	}
	h.offDutyLocked(s, "exited")
}

// tellingLines is the last n lines of screen that tell what the session was doing: not the input box's borders (lines
// mostly of box-drawing characters, the employee's name in one or not), not Claude Code's input line (❯) or status
// line (⏵⏵), and only lines with a letter or a digit; whole lines from the end within max runes, so a notice shows
// "✻ Churned for 2m" rather than half a border.
func tellingLines(screen string, n, max int) string {
	lines := strings.Split(screen, "\n")
	// the input box is the screen's bottom: from its top border on, nothing is the session's output: the typed or
	// suggested prompt (❯, which a console may not show), the bottom border, the status line. The bottom border is
	// the last line mostly of box-drawing characters; the top one the border within the few lines above it
	if bottom := lastBorder(lines, len(lines)-1); bottom >= 0 {
		cut := bottom
		if top := lastBorder(lines, bottom-1); top >= 0 && bottom-top <= 6 {
			cut = top
		}
		lines = lines[:cut]
	}
	var keep []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "❯") || strings.HasPrefix(l, "⏵⏵") || !tells(l) {
			continue
		}
		keep = append(keep, l)
	}
	if len(keep) > n {
		keep = keep[len(keep)-n:]
	}
	size := 0
	for i := len(keep) - 1; i >= 0; i-- {
		size += utf8.RuneCountInString(keep[i]) + 1
		if size > max && i < len(keep)-1 {
			keep = keep[i+1:]
			break
		}
	}
	if len(keep) == 1 && utf8.RuneCountInString(keep[0]) > max {
		return cutRunes(keep[0], max)
	}
	return strings.Join(keep, "\n")
}

// lastBorder is the index of the last border line at or before i, -1 when there is none.
func lastBorder(lines []string, i int) int {
	for ; i >= 0; i-- {
		if border(strings.TrimSpace(lines[i])) {
			return i
		}
	}
	return -1
}

// tells says whether a line says something: a letter or a digit in it, and it isn't a border.
func tells(l string) bool {
	return !border(l) && strings.IndexFunc(l, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

// border says whether a line is mostly box-drawing characters (U+2500 to U+257F): the input box's edges, the
// employee's name in one or not.
func border(l string) bool {
	boxes, all := 0, 0
	for _, r := range l {
		if r == ' ' {
			continue
		}
		all++
		if r >= 0x2500 && r <= 0x257f {
			boxes++
		}
	}
	return all > 0 && boxes*2 >= all
}

// cleanExit is true for a process that exited by itself with status 0 (a Term reports that as "exit 0").
func cleanExit(err error) bool { return err == nil || err.Error() == "exit 0" }

// Reap marks sessions silent for too long as off duty (closing the terminal leaves the mod no time to say goodbye).
func (h *Hub) Reap() {
	h.mu.Lock()
	defer h.mu.Unlock()
	changed := false
	var tasks []string
	for _, s := range h.sessions {
		if s.State != StateEnded && h.now().Sub(s.Updated) >= StaleAfter {
			h.offDutyLocked(s, "lost")
			changed = true
			if s.Task != "" {
				tasks = append(tasks, s.Task)
			}
		}
	}
	if changed {
		h.broadcastLocked(nil)
	}
	for _, t := range tasks {
		h.checkTaskLocked(t)
	}
}

// dropSessionLocked forgets a session: its record, its chat and its inbox. Nothing it asked stays pending.
func (h *Hub) dropSessionLocked(id string) {
	h.cancelSessionAsksLocked(id)
	delete(h.sessions, id)
	delete(h.chats, id)
	delete(h.inboxes, id)
}

// Prune drops long-ended sessions and long-settled asks, so a bridge that runs for days doesn't pile them up.
func (h *Hub) Prune() {
	h.mu.Lock()
	defer h.mu.Unlock()
	cut := h.now().Add(-endedTTL)
	changed := false
	for id, s := range h.sessions {
		if s.State == StateEnded && s.Updated.Before(cut) {
			h.dropSessionLocked(id)
			changed = true
		}
	}
	for id, a := range h.asks {
		if a.Status != AskPending && a.Created.Before(cut) {
			delete(h.asks, id)
		}
	}
	for id, l := range h.launches {
		// a launch whose process still runs stays: it holds the desk (reserveDesk), and is marked ended with it
		if l.Status != LaunchStarting && l.Time.Before(cut) && (h.terms == nil || !h.terms.Live(id)) {
			delete(h.launches, id)
		}
	}
	if changed {
		h.broadcastLocked(nil)
	}
}

// ---------- messages between sessions ----------

var (
	crossSession = regexp.MustCompile(`(?s)^\s*<cross-session-message\b([^>]*)>\s*(.*?)\s*(?:</cross-session-message>\s*)?$`)
	fromName     = regexp.MustCompile(`\bfrom-name="([^"]*)"`)
)

// parseIncoming splits a received text into the sender's name and the message itself.
func parseIncoming(text string) (from, body string) {
	if m := crossSession.FindStringSubmatch(text); m != nil {
		if f := fromName.FindStringSubmatch(m[1]); f != nil {
			from = f[1]
		}
		return from, m[2]
	}
	return "", strings.TrimSpace(text)
}

// sessionByNameLocked finds an online session by name: the session name in Claude first, then the
// account name (a desk session's only: bob is not bob-T7); the most recently active one wins.
func (h *Hub) sessionByNameLocked(name string) *Session {
	var best *Session
	for _, s := range h.sessions {
		if s.State == StateEnded {
			continue
		}
		if s.Title != name && (s.Task != "" || h.accounts[s.Account] == nil || h.accounts[s.Account].Name != name) {
			continue
		}
		if best == nil || s.Updated.After(best.Updated) {
			best = s
		}
	}
	return best
}

// nameLocked is the name a session goes by: a desk session's is the account name (or the session name once the
// account is gone), a task session's its session name (bob-T7).
func (h *Hub) nameLocked(s *Session) string {
	if s.Task != "" && s.Title != "" {
		return s.Title
	}
	if a := h.accounts[s.Account]; a != nil {
		if s.Task != "" {
			return a.Name + "-" + s.Task
		}
		return a.Name
	}
	return s.Title
}

func sameText(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	n := min(len(a), len(b), 200)
	return n > 0 && a[:n] == b[:n]
}

// findPairLocked finds the other half: for a receive the recorded send, for a send the recorded receive.
// Either may come first (the sender's hook reports only after next returns), so they pair by sending
// session + text + a time window.
func (h *Hub) findPairLocked(from, text string, wantReceived bool) *Message {
	cut := h.now().Add(-pairWindow)
	for i := len(h.messages) - 1; i >= 0; i-- {
		m := h.messages[i]
		if m.Time.Before(cut) {
			break
		}
		if m.From == from && m.Received == wantReceived && sameText(m.Text, text) {
			return m
		}
	}
	return nil
}

// messageTask is the task a message between two sessions belongs to: the sender's, else the receiver's (either may be
// unknown).
func messageTask(from, to *Session) string {
	if from != nil && from.Task != "" {
		return from.Task
	}
	if to != nil {
		return to.Task
	}
	return ""
}

// pushMessageLocked adds a message to the log. The log keeps the latest keepMessages messages of each task the office
// shows (running, ending, failed), so a busy office doesn't push a team chat out, and as many of the others (between
// desks, of tasks gone); the oldest go first.
func (h *Hub) pushMessageLocked(m *Message) {
	h.messages = append(h.messages, m)
	h.noteLocked(h.sessions[m.From], "message", m.FromName+" → "+m.ToName+": "+m.Text) // once per message, whichever half came first
	if m.Task != "" {
		h.activity[len(h.activity)-1].Task = m.Task
	}
	if len(h.messages) <= keepMessages {
		return
	}
	shown := map[string]bool{}
	for _, t := range h.tasks {
		if t.Status != TaskEnded {
			shown[t.ID] = true
		}
	}
	count := map[string]int{}
	keep := make([]bool, len(h.messages))
	n := 0
	for i := len(h.messages) - 1; i >= 0; i-- {
		group := h.messages[i].Task
		if !shown[group] {
			group = ""
		}
		if count[group] < keepMessages {
			count[group]++
			keep[i] = true
			n++
		}
	}
	if n == len(h.messages) {
		return
	}
	kept := make([]*Message, 0, n)
	for i, msg := range h.messages {
		if keep[i] {
			kept = append(kept, msg)
		}
	}
	h.messages = kept
}

func originKind(origin any) string {
	switch o := origin.(type) {
	case string:
		return o
	case map[string]any:
		k, _ := o["kind"].(string)
		return k
	}
	return ""
}

func (h *Hub) receiveLocked(to *Session, ev Event) *Message {
	name, body := parseIncoming(ev.Text)
	var from *Session
	if name != "" {
		from = h.sessionByNameLocked(name)
	}
	fromID := ""
	if from != nil {
		fromID = from.ID
		if m := h.findPairLocked(fromID, body, false); m != nil {
			m.To, m.ToName, m.Received = to.ID, h.nameLocked(to), true
			m.Task = messageTask(from, to)
			return m
		}
	}
	m := &Message{
		ID: newID(), From: fromID, FromName: name, To: to.ID, ToName: h.nameLocked(to),
		Text: body, Origin: originKind(ev.Origin), AgentID: ev.AgentID, Task: messageTask(from, to),
		Received: true, Delivered: true, Time: h.now(),
	}
	if from != nil {
		m.FromName = h.nameLocked(from)
	}
	h.pushMessageLocked(m)
	return m
}

func (h *Hub) sendLocked(from *Session, ev Event) *Message {
	if m := h.findPairLocked(from.ID, ev.Text, true); m != nil {
		m.Delivered = ev.Delivered
		m.Task = messageTask(from, h.sessions[m.To])
		return m
	}
	to := h.sessionByNameLocked(ev.To)
	m := &Message{
		ID: newID(), From: from.ID, FromName: h.nameLocked(from), ToName: ev.To,
		Text: strings.TrimSpace(ev.Text), Origin: originKind(ev.Origin), AgentID: ev.AgentID, Task: messageTask(from, to),
		Delivered: ev.Delivered, Time: h.now(),
	}
	if to != nil {
		m.To, m.ToName = to.ID, h.nameLocked(to)
	}
	h.pushMessageLocked(m)
	return m
}

// ---------- pending asks ----------

// trimInput is v with every string longer than inputPreview cut to that length: its head and its tail, with how many
// runes were cut between them (… [+N] …); and whether anything was cut. Maps and lists are copied as far as needed;
// v itself is left alone.
func trimInput(v any) (any, bool) {
	switch x := v.(type) {
	case string:
		if utf8.RuneCountInString(x) <= inputPreview {
			return x, false
		}
		runes := []rune(x)
		tail := len(runes) - (inputPreview - inputHead)
		return string(runes[:inputHead]) + " … [+" + strconv.Itoa(len(runes)-inputPreview) + "] … " + string(runes[tail:]), true
	case map[string]any:
		var out map[string]any
		for k, val := range x {
			if t, cut := trimInput(val); cut {
				if out == nil {
					out = make(map[string]any, len(x))
					for k2, v2 := range x {
						out[k2] = v2
					}
				}
				out[k] = t
			}
		}
		if out == nil {
			return x, false
		}
		return out, true
	case []any:
		var out []any
		for i, val := range x {
			if t, cut := trimInput(val); cut {
				if out == nil {
					out = append([]any(nil), x...)
				}
				out[i] = t
			}
		}
		if out == nil {
			return x, false
		}
		return out, true
	}
	return v, false
}

type AskRequest struct {
	Session   string     `json:"session"`
	Kind      string     `json:"kind"`
	Tool      string     `json:"tool,omitempty"`
	Input     any        `json:"input,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	Questions any        `json:"questions,omitempty"`
	AgentID   string     `json:"agentId,omitempty"`
	Remember  *Rule      `json:"remember,omitempty"`
	Server    string     `json:"server,omitempty"`
	Message   string     `json:"message,omitempty"`
	Mode      string     `json:"mode,omitempty"`
	URL       string     `json:"url,omitempty"`
	Fields    []AskField `json:"fields,omitempty"`
}

func (h *Hub) CreateAsk(req AskRequest, acc *Account) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// the mod doesn't send the task with an ask: the session reported it before, or, when the hub no longer has the
	// session (pruned), the task's members or launches know it
	task := h.sessionTaskLocked(req.Session, acc.ID)
	if s := h.sessions[req.Session]; s != nil {
		task = s.Task
	}
	s, _, err := h.sessionLocked(req.Session, acc, task, "", "")
	if err != nil {
		return "", err
	}
	s.Updated = h.now()
	a := &Ask{
		ID: newID(), Session: req.Session, Account: acc.ID, Kind: req.Kind,
		Tool: req.Tool, Input: req.Input, Reason: req.Reason, Questions: req.Questions,
		AgentID: req.AgentID, Status: AskPending, Created: h.now(), done: make(chan struct{}),
	}
	if req.Kind == "permission" && req.Remember != nil && req.Remember.Tool != "" {
		r := Rule{Tool: cutRunes(req.Remember.Tool, 200), Prefix: cutRunes(req.Remember.Prefix, 200)}
		a.Remember = &r
	}
	if req.Kind == "elicitation" {
		a.Server, a.Message, a.Mode, a.URL = cutRunes(req.Server, 200), cutRunes(req.Message, 4000), req.Mode, cutRunes(req.URL, 2000)
		if a.Mode != "url" {
			a.Mode, a.URL = "form", ""
		}
		for i, f := range req.Fields {
			if i == 50 {
				break
			}
			f.Name, f.Title, f.Description = cutRunes(f.Name, 200), cutRunes(f.Title, 200), cutRunes(f.Description, 1000)
			if len(f.Enum) > 50 {
				f.Enum = f.Enum[:50]
			}
			a.Fields = append(a.Fields, f)
		}
	}
	a.preview, a.InputTruncated = trimInput(req.Input)
	h.asks[a.ID] = a
	h.chatAskLocked(a)
	if req.Kind == "question" {
		h.noteLocked(s, "ask", strings.TrimSpace("question "+askSummary(req.Questions)))
	} else if req.Kind == "elicitation" {
		h.noteLocked(s, "ask", strings.TrimSpace("elicitation "+a.Server+" "+cutRunes(a.Message, 120)))
	} else {
		h.noteLocked(s, "ask", strings.TrimSpace(req.Tool+" "+toolSummary(req.Tool, req.Input)))
	}
	before := s.State
	h.refreshWaitingLocked(s)
	h.markStateLocked(s, before)
	h.broadcastLocked(map[string]any{"type": "ask", "id": a.ID, "session": a.Session})
	return a.ID, nil
}

// AskInput is a pending ask's input as the mod sent it, long strings whole (the pages get it trimmed, trimInput);
// ErrUnknownAsk once the ask is settled.
func (h *Hub) AskInput(id string) (any, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	a := h.asks[id]
	if a == nil || a.Status != AskPending {
		return nil, ErrUnknownAsk
	}
	return a.Input, nil // never changed after the ask is made, so it can be encoded once the lock is let go
}

// Wait blocks until there is an answer, time runs out or the request goes away, and returns the state then.
// An empty account skips the check (tests and internal use only). A waiting mod means the session is alive,
// so it also counts as a heartbeat.
func (h *Hub) Wait(id, account string, timeout time.Duration, cancel <-chan struct{}) (string, map[string]any, error) {
	h.mu.Lock()
	a := h.asks[id]
	if a == nil {
		h.mu.Unlock()
		return "", nil, ErrUnknownAsk
	}
	if account != "" && a.Account != account {
		h.mu.Unlock()
		return "", nil, ErrWrongAccount
	}
	if s := h.sessions[a.Session]; s != nil && s.State != StateEnded {
		s.Updated = h.now()
	}
	done := a.done
	h.mu.Unlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	case <-cancel:
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return a.Status, a.Answer, nil
}

// Settle answers an ask (done) or withdraws it (cancelled). An empty account means the page; no check.
func (h *Hub) Settle(id, account, status string, answer map[string]any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	a := h.asks[id]
	if a == nil {
		return ErrUnknownAsk
	}
	if account != "" && a.Account != account {
		return ErrWrongAccount
	}
	if a.Status != AskPending {
		return ErrSettled
	}
	a.Status, a.Answer = status, answer
	close(a.done)
	h.chatSettledLocked(a)
	s := h.sessions[a.Session]
	if s != nil {
		before := s.State
		h.refreshWaitingLocked(s)
		h.markStateLocked(s, before)
	}
	outcome := status
	if d, _ := answer["decision"].(string); d != "" {
		outcome = d
	}
	h.noteLocked(s, "settled", strings.TrimSpace(outcome+" "+a.Tool))
	h.broadcastLocked(map[string]any{"type": "settled", "id": id, "status": status})
	return nil
}

// RosterEntry is one session in the office as another session's model hears of it (the mod's office context).
type RosterEntry struct {
	Name   string `json:"name"`             // the session's name, the one SendMessage finds it by
	Folder string `json:"folder,omitempty"` // the last element of its working folder
	Task   string `json:"task,omitempty"`
	State  string `json:"state"`
}

// Roster is the sessions online besides session (of account), and session's own name.
func (h *Hub) Roster(session, account string) map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	you := ""
	if s := h.sessions[session]; s != nil && s.Account == account {
		you = sessionName(h, s)
	}
	others := []RosterEntry{}
	for _, s := range h.sessions {
		if s.ID == session || s.State == StateEnded {
			continue
		}
		others = append(others, RosterEntry{Name: sessionName(h, s), Folder: filepath.Base(s.Cwd), Task: s.Task, State: s.State})
	}
	slices.SortFunc(others, func(a, b RosterEntry) int { return strings.Compare(a.Name, b.Name) })
	return map[string]any{"you": you, "others": others}
}

// sessionName is the name a session goes by in Claude Code (--name, /rename), else the one the office gives it.
func sessionName(h *Hub, s *Session) string {
	if s.Title != "" {
		return s.Title
	}
	return h.nameLocked(s)
}

// SettleByMod settles an ask the mod made that the page can't answer (an elicitation, answered in the session's
// terminal) with what became of it; any other kind is the page's to answer (ErrNotModSettled).
func (h *Hub) SettleByMod(id, account string, answer map[string]any) error {
	if k := h.askKind(id); k != "" && k != "elicitation" {
		return ErrNotModSettled
	}
	return h.Settle(id, account, AskDone, answer)
}

// AnswerByPage settles an ask with the page's answer: a permission's decision or a question's answers. An elicitation
// is answered in the session's terminal, not here (ErrNotPageAnswer).
func (h *Hub) AnswerByPage(id string, answer map[string]any) error {
	if h.askKind(id) == "elicitation" {
		return ErrNotPageAnswer
	}
	return h.Settle(id, "", AskDone, answer)
}

// askKind is a pending ask's kind ("" for none): the kind never changes, so a check before Settle holds.
func (h *Hub) askKind(id string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if a := h.asks[id]; a != nil {
		return a.Kind
	}
	return ""
}

// ---------- starts from the page ----------

func (h *Hub) addLaunch(l *Launch) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.launches[l.ID] = l
	// its session may have reported in already, while the start was being recorded
	for _, s := range h.sessions {
		if s.Launch == l.ID && s.Account == l.Account && s.State != StateEnded {
			h.launchOnlineLocked(l, s)
		}
	}
	h.noteLocked(nil, "launch", strings.TrimSpace(l.Name+" "+l.Dir))
	h.broadcastLocked(map[string]any{"type": "launch", "id": l.ID})
}

// launchOnlineLocked is the one place a launch comes online, a desk's or a task member's: its session reported in, and
// the session takes what the launch knows (its terminal, the model and effort picked).
func (h *Hub) launchOnlineLocked(l *Launch, s *Session) {
	s.Attach = l.Attach
	l.Status, l.Session, l.Error, l.ErrorCode, l.ErrorParams = LaunchOnline, s.ID, "", "", nil
	// until the session reports its model and effort, use the ones picked at launch
	if s.Model == "" {
		s.Model = l.Model
	}
	if s.Effort == "" {
		s.Effort = l.Effort
	}
}

func (h *Hub) launchStatus(id string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if l := h.launches[id]; l != nil {
		return l.Status
	}
	return ""
}

// failLaunch fails a launch that is still starting (its process exited, or time ran out before it reported in).
func (h *Hub) failLaunch(id string, why *Error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if l := h.launches[id]; l != nil {
		h.failLaunchLocked(l, why)
	}
}

// failLaunchLocked is the one place a launch fails, a desk's or a task member's; one no longer starting stays as it is.
func (h *Hub) failLaunchLocked(l *Launch, why *Error) {
	id := l.ID
	if l.Status == LaunchStarting {
		l.Status, l.Error, l.ErrorCode, l.ErrorParams = LaunchFailed, why.Text, why.Code, why.Params
		h.noteLocked(nil, "launchFailed", strings.TrimSpace(l.Name+" "+why.Text))
		h.broadcastLocked(map[string]any{"type": "launch", "id": id})
		// a session that never reported in won't: its process goes (stuck at a prompt, it would hold the desk or keep
		// the task running for nothing, out of sight); the error keeps the last lines of its screen
		// (not while someone uses its terminal: a login or a first-run prompt there; the launch comes online when the
		// session reports in)
		if h.terms != nil {
			if term := h.terms.Get(id); term != nil && term.Clients() == 0 {
				term.Kill()
			}
		}
		if l.Task != "" {
			h.checkTaskLocked(l.Task) // a task whose last member never came ends
		}
	}
}

// launchExited: a desk session's process is gone (Watch follows it once it is online). Its session goes off duty at
// once, saying why when it didn't end well; one that said goodbye first is off duty already. The launch is over.
func (h *Hub) launchExited(id string, exitErr error, screen string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopping {
		return // the bridge ends its sessions on its way out
	}
	changed := false
	for _, s := range h.sessions {
		if s.Launch == id && s.Task == "" && s.State != StateEnded {
			h.exitedLocked(s, exitErr, screen)
			changed = true
		}
	}
	if l := h.launches[id]; l != nil && l.Status == LaunchOnline {
		l.Status = LaunchEnded
		changed = true
	}
	if changed {
		h.broadcastLocked(map[string]any{"type": "launch", "id": id})
	}
}

// reserveDesk holds an account's desk for one start from the page, so two starts can't both get through: none while
// the account is online, has a start on the way or a process from the page still running (deskBusyLocked). release
// lets go of it; by then the launch is in the hub and holds the desk itself.
func (h *Hub) reserveDesk(account string) (release func(), err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.deskBusyLocked(account); err != nil {
		return nil, err
	}
	if h.reserved == nil {
		h.reserved = map[string]bool{}
	}
	h.reserved[account] = true
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.reserved, account)
	}, nil
}

// deskBusyLocked says whether the account's desk is taken: its session is online, a start from the page is being made
// or on its way, or the process of one still runs (even with its session off duty: it may come back, or still be at a
// prompt in its terminal).
func (h *Hub) deskBusyLocked(account string) *Error {
	if h.onlineLocked(account) != nil || h.reserved[account] {
		return ErrAccountBusy
	}
	live := func(id string) bool { return id != "" && h.terms != nil && h.terms.Live(id) }
	for _, l := range h.launches {
		if l.Account == account && l.Task == "" && l.Status == LaunchStarting {
			return ErrAccountBusy
		}
	}
	// a process whose session is off duty (it stopped answering) or never came: the person decides, in its terminal
	still := func(id, attach string) *Error {
		return newError("launch_still_running", "the process of this desk's last start still runs; open its terminal ("+attach+") or end it", "command", attach)
	}
	for _, l := range h.launches {
		if l.Account == account && l.Task == "" && live(l.ID) {
			return still(l.ID, l.Attach)
		}
	}
	for _, s := range h.sessions {
		if s.Account == account && s.Task == "" && live(s.Launch) {
			return still(s.Launch, s.Attach)
		}
	}
	return nil
}

// recentDirs are recently used folders, offered as shortcuts in the launch dialog; task sessions' worktrees aren't.
func (h *Hub) recentDirs(n int) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	type seen struct {
		dir string
		at  time.Time
	}
	var all []seen
	for _, l := range h.launches {
		if l.Task == "" {
			all = append(all, seen{l.Dir, l.Time})
		}
	}
	for _, s := range h.sessions {
		if s.Cwd != "" && s.Task == "" {
			all = append(all, seen{s.Cwd, s.Updated})
		}
	}
	for _, a := range h.accounts {
		if a.Last != nil && a.Last.Dir != "" {
			all = append(all, seen{a.Last.Dir, a.Created})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].at.After(all[j].at) })
	dirs := []string{}
	for _, x := range all {
		if !slices.Contains(dirs, x.dir) {
			dirs = append(dirs, x.dir)
		}
		if len(dirs) == n {
			break
		}
	}
	return dirs
}
