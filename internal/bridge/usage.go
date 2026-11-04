package bridge

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Besides the chat and the state, the mod reports what the engine knows of a session, and the hub keeps it for the
// pages to show as they like: the engine's version and the repository at the start, the usage figures (the context
// window's fill, the rate limits, the cost; pushed as a "usage" message without a snapshot, the way the chat is), each
// response's tokens (summed by model in the session's Usage), subagents by name, the agent team's tasks (todos), the
// surfaces attached, the MCP servers. The ledger adds up the cost and tokens by account and day (ledger.json), and the
// activity log keeps the last keepActivity things that happened, so a page that opens late sees the same timeline.

const (
	// Protocol is the version of what the bridge gives the pages; the snapshot carries it. Additions keep the
	// number; a removed or renamed field raises it.
	Protocol = 3
	// keepActivity is how many activity lines the hub keeps (/api/activity)
	keepActivity = 500
	// keepLedgerDays is how many days of the ledger stay in ledger.json
	keepLedgerDays = 90
)

// EngineInfo is the Claude Code the session runs on.
type EngineInfo struct {
	Version string `json:"version"`
	Base    string `json:"base,omitempty"` // the release it is built from (2.1.316)
}

// RepoInfo is the git repository the session works in.
type RepoInfo struct {
	Root     string `json:"root"`
	Remote   string `json:"remote,omitempty"`
	Name     string `json:"name,omitempty"`
	Internal bool   `json:"internal,omitempty"`
}

// withoutUserinfo is a remote's address without the user name and password it may hold: whatever comes before an @
// in the part before the first slash goes, so https://<user>:<token>@host/path becomes https://host/path and
// <user>@host:path (scp-like) host:path. A local path's / or \ comes first and leaves it alone.
func withoutUserinfo(remote string) string {
	prefix, rest := "", remote
	if i := strings.Index(remote, "://"); i >= 0 {
		prefix, rest = remote[:i+3], remote[i+3:]
	}
	host := rest
	if i := strings.IndexAny(rest, `/\`); i >= 0 {
		host = rest[:i]
	}
	if at := strings.LastIndex(host, "@"); at >= 0 {
		return prefix + rest[at+1:]
	}
	return remote
}

// Tokens is what a response, a turn or a session cost in tokens.
type Tokens struct {
	Input      int64  `json:"input"`
	Output     int64  `json:"output"`
	CacheRead  int64  `json:"cacheRead"`
	CacheWrite int64  `json:"cacheWrite"`
	Model      string `json:"model,omitempty"`
}

func (t *Tokens) add(o *Tokens) {
	t.Input += o.Input
	t.Output += o.Output
	t.CacheRead += o.CacheRead
	t.CacheWrite += o.CacheWrite
}

// ContextUsage is the live context window: its size and how full it is.
type ContextUsage struct {
	Tokens  int64   `json:"tokens,omitempty"`
	Window  int64   `json:"window"`
	Percent float64 `json:"percent,omitempty"`
}

// RateLimit is one of the account's rate-limit windows (five_hour, seven_day, ...), as the last response reported it.
type RateLimit struct {
	Kind        string  `json:"kind"`
	PercentUsed float64 `json:"percentUsed"`
	ResetsAt    string  `json:"resetsAt,omitempty"`
}

type Cost struct {
	USD float64 `json:"usd"`
}

// Usage is a session's figures: the context window, the rate limits and the cost as the mod reports them
// (session.measure), and the tokens of its responses summed by model (the stop chunk of every step).
type Usage struct {
	Context    *ContextUsage      `json:"context,omitempty"`
	RateLimits []RateLimit        `json:"rateLimits,omitempty"`
	Cost       *Cost              `json:"cost,omitempty"`
	Totals     map[string]*Tokens `json:"totals,omitempty"`
	Updated    time.Time          `json:"updated"`
}

// copy is a copy of u the hub won't change: the totals are added up in place (the other figures are replaced whole).
func (u *Usage) copy() *Usage {
	if u == nil {
		return nil
	}
	cp := *u
	if u.Totals != nil {
		cp.Totals = make(map[string]*Tokens, len(u.Totals))
		for model, t := range u.Totals {
			tc := *t
			cp.Totals[model] = &tc
		}
	}
	return &cp
}

// Refusal is what the API said about a turn it refused.
type Refusal struct {
	Category    string `json:"category,omitempty"`
	Explanation string `json:"explanation,omitempty"`
}

// Todo is one open item of the session's agent-team task list (TaskCreate); "todo" because "task" is the office's
// collaborative task.
type Todo struct {
	ID       string    `json:"id"`
	Subject  string    `json:"subject"`
	Teammate string    `json:"teammate,omitempty"`
	Created  time.Time `json:"created"`
}

// Notify is the last notification the engine raised for the session (idle, an error, a permission prompt in the
// terminal).
type Notify struct {
	Type  string    `json:"type"`
	Title string    `json:"title,omitempty"`
	Text  string    `json:"text"`
	Time  time.Time `json:"time"`
}

// LedgerRow is what one account's sessions cost on one day.
type LedgerRow struct {
	Day     string  `json:"day"` // YYYY-MM-DD, local time
	Account string  `json:"account"`
	USD     float64 `json:"usd"`
	Tokens  Tokens  `json:"tokens"`
	Turns   int     `json:"turns"`
	Tools   int     `json:"tools"`
}

// Activity is one line of the activity log: a session came or went, a tool ran, an ask came or was settled, a message
// went, a launch started or failed.
type Activity struct {
	Time    time.Time `json:"time"`
	Kind    string    `json:"kind"` // start | end | tool | ask | settled | message | error | launch
	Session string    `json:"session,omitempty"`
	Account string    `json:"account,omitempty"`
	Name    string    `json:"name,omitempty"` // the session's name in the office
	Task    string    `json:"task,omitempty"`
	Text    string    `json:"text,omitempty"`
}

// ---------- usage ----------

const keepTodos = 50

// usageLocked takes the figures the mod reports: the session's usage, the ledger's cost (the report is the session's
// total so far; the difference since the last one goes to today), and a push to the pages.
func (h *Hub) usageLocked(s *Session, u *Usage) {
	if s.Usage == nil {
		s.Usage = &Usage{}
	}
	if u.Context != nil {
		s.Usage.Context = u.Context
	}
	if u.RateLimits != nil {
		s.Usage.RateLimits = u.RateLimits
	}
	if u.Cost != nil {
		if d := u.Cost.USD - s.costSeen; d > 0 {
			h.ledgerRowLocked(s.Account).USD += d
			h.saveLedgerLocked()
		}
		s.costSeen = u.Cost.USD
		s.Usage.Cost = u.Cost
	}
	s.Usage.Updated = h.now()
	h.pushUsageLocked(s)
}

// countTokensLocked adds a response's tokens to the session's totals (by model) and to today's ledger.
func (h *Hub) countTokensLocked(s *Session, t *Tokens) {
	if t == nil {
		return
	}
	if s.Usage == nil {
		s.Usage = &Usage{}
	}
	if s.Usage.Totals == nil {
		s.Usage.Totals = map[string]*Tokens{}
	}
	model := t.Model
	if model == "" {
		model = s.Model
	}
	sum := s.Usage.Totals[model]
	if sum == nil {
		sum = &Tokens{Model: model}
		s.Usage.Totals[model] = sum
	}
	sum.add(t)
	s.Usage.Updated = h.now()
	h.ledgerRowLocked(s.Account).Tokens.add(t)
	h.saveLedgerLocked()
}

// pushUsageLocked sends a session's usage to the pages, without a snapshot (like the chat).
func (h *Hub) pushUsageLocked(s *Session) {
	if len(h.subs) == 0 || s.Usage == nil {
		return
	}
	if p := h.pushLocked(map[string]any{"type": "usage", "session": s.ID, "usage": s.Usage}); p != nil {
		h.deliverLocked(p)
	}
}

// todoLocked adds or removes an item of the session's agent-team task list.
func (h *Hub) todoLocked(s *Session, ev Event, done bool) {
	for i, t := range s.Todos {
		if t.ID == ev.Todo {
			if done {
				s.Todos = append(s.Todos[:i], s.Todos[i+1:]...)
			} else {
				t.Subject, t.Teammate = ev.Subject, ev.Teammate
			}
			return
		}
	}
	if done {
		return
	}
	s.Todos = append(s.Todos, &Todo{ID: ev.Todo, Subject: ev.Subject, Teammate: ev.Teammate, Created: h.now()})
	if len(s.Todos) > keepTodos {
		s.Todos = s.Todos[len(s.Todos)-keepTodos:]
	}
}

// ---------- the ledger ----------

func (h *Hub) dayLocked() string {
	return h.now().Format("2006-01-02")
}

// ledgerRowLocked is today's row of an account, made when missing.
func (h *Hub) ledgerRowLocked(account string) *LedgerRow {
	day := h.dayLocked()
	for _, r := range h.ledger {
		if r.Day == day && r.Account == account {
			return r
		}
	}
	r := &LedgerRow{Day: day, Account: account}
	h.ledger = append(h.ledger, r)
	// rows older than keepLedgerDays go
	cut := h.now().AddDate(0, 0, -keepLedgerDays).Format("2006-01-02")
	kept := h.ledger[:0]
	for _, x := range h.ledger {
		if x.Day >= cut {
			kept = append(kept, x)
		}
	}
	h.ledger = kept
	return r
}

// saveLedgerLocked tells the background writer the ledger changed: the writer copies its rows under the hub's lock
// (each report adds to one in place) and encodes them outside it, once per write however many reports came.
func (h *Hub) saveLedgerLocked() {
	if h.dataDir == "" {
		return
	}
	h.ledgerSave.putFunc(filepath.Join(h.dataDir, "ledger.json"), func() ([]byte, error) {
		h.mu.Lock()
		rows := make([]LedgerRow, len(h.ledger))
		for i, r := range h.ledger {
			rows[i] = *r
		}
		h.mu.Unlock()
		return json.Marshal(rows)
	})
}

// ledgerTodayLocked is today's rows, for the snapshot.
func (h *Hub) ledgerTodayLocked() []*LedgerRow {
	day := h.dayLocked()
	out := []*LedgerRow{}
	for _, r := range h.ledger {
		if r.Day == day {
			cp := *r
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Account < out[j].Account })
	return out
}

// Ledger is every row kept (keepLedgerDays), oldest first.
func (h *Hub) Ledger() []*LedgerRow {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*LedgerRow, 0, len(h.ledger))
	for _, r := range h.ledger {
		cp := *r
		out = append(out, &cp)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out
}

// ---------- the activity log ----------

// noteLocked adds a line to the activity log.
func (h *Hub) noteLocked(s *Session, kind, text string) {
	a := &Activity{Time: h.now(), Kind: kind, Text: cutRunes(text, 300)}
	if s != nil {
		a.Session, a.Account, a.Name, a.Task = s.ID, s.Account, h.nameLocked(s), s.Task
	}
	h.activity = append(h.activity, a)
	if len(h.activity) > keepActivity {
		h.activity = h.activity[len(h.activity)-keepActivity:]
	}
}

// Activity is the log, oldest first.
func (h *Hub) Activity() []*Activity {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*Activity, 0, len(h.activity))
	for _, a := range h.activity {
		cp := *a
		out = append(out, &cp)
	}
	return out
}
