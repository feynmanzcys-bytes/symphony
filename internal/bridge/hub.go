// Package bridge is the hub of symphony: it takes the session events and pending asks the mod
// reports and pushes them to the approval page and the office page. Accounts live in a file, the rest
// in memory, all under one lock.
package bridge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
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
	// inputPreview: a string longer than this in a pending ask's input reaches the page cut to this length (a Write of a
	// whole file would otherwise ride along with every snapshot until the ask is settled)
	inputPreview = 4096
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
	ErrSessionOffline = newError("session_offline", "the session is not online")
)

type Agent struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Tool  string `json:"tool,omitempty"`
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
	Agents  map[string]*Agent `json:"agents"`
	Tools   int               `json:"tools"`
	Model   string            `json:"model,omitempty"`  // the model in use; sets the character's size
	Effort  string            `json:"effort,omitempty"` // the thinking effort; sets the character's muscles
	Launch  string            `json:"launch,omitempty"` // only for sessions started from the page
	Attach  string            `json:"attach,omitempty"` // where to find its terminal
	Started time.Time         `json:"started"`
	Updated time.Time         `json:"updated"`
	// StateSince is when the current state began (the page uses it to tell how long a session idles; long idlers fall asleep)
	StateSince time.Time `json:"stateSince"`
}

type Ask struct {
	ID        string         `json:"id"`
	Session   string         `json:"session"`
	Account   string         `json:"account"`
	Kind      string         `json:"kind"` // permission | question
	Tool      string         `json:"tool,omitempty"`
	Input     any            `json:"input,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	Questions any            `json:"questions,omitempty"`
	AgentID   string         `json:"agentId,omitempty"`
	Status    string         `json:"status"`
	Answer    map[string]any `json:"answer,omitempty"`
	Created   time.Time      `json:"created"`
	// InputTruncated says that the Input the page gets is a preview: long strings in it are cut (inputPreview)
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
	Text      string    `json:"text"`
	Origin    string    `json:"origin,omitempty"`
	AgentID   string    `json:"agentId,omitempty"`
	Delivered bool      `json:"delivered"`
	Received  bool      `json:"received"`
	Time      time.Time `json:"time"`
}

// Event is one event the mod reports; which fields matter depends on kind.
type Event struct {
	Session     string  `json:"session"`
	Kind        string  `json:"kind"`
	Login       string  `json:"login,omitempty"` // the Claude login the mod reports
	Model       string  `json:"model,omitempty"`
	Effort      string  `json:"effort,omitempty"`
	Launch      string  `json:"launch,omitempty"` // the id the bridge gave when the page started the session
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
}

// Snapshot is the whole state as the pages see it. Messages is the message log in a full snapshot (the first one a
// subscriber gets, and /api/state) and null in the ones pushed on every change: the page keeps its own log and gets
// each new or updated message as a "message" event, so a tool call doesn't carry 300 messages along.
type Snapshot struct {
	Type     string        `json:"type"`
	Accounts []AccountView `json:"accounts"`
	Sessions []*Session    `json:"sessions"`
	Asks     []*Ask        `json:"asks"`
	Messages []*Message    `json:"messages"`
	Launches []*Launch     `json:"launches"`
	// MessageTail is the latest message's id, in every snapshot: a page that doesn't have it fell behind and
	// fetches the log again (/api/state)
	MessageTail string `json:"messageTail,omitempty"`
}

// Push is one message for the subscribers: an event or a snapshot, encoded once for all of them.
type Push struct {
	Value any
	once  sync.Once
	data  []byte
}

// JSON is the message encoded, the same bytes for every subscriber.
func (p *Push) JSON() []byte {
	p.once.Do(func() { p.data, _ = json.Marshal(p.Value) })
	return p.data
}

// Hub holds all the state.
type Hub struct {
	mu       sync.Mutex
	accounts map[string]*Account
	dataDir  string // accounts and the message log are stored here; empty means nothing goes to disk (tests)
	sessions map[string]*Session
	asks     map[string]*Ask
	messages []*Message
	launches map[string]*Launch
	chats    map[string]*chatLog // by session id
	inboxes  map[string]*inbox   // by session id
	subs     map[chan *Push]struct{}
	now      func() time.Time
	msgSave  saver // writes the message log in the background
	// AttachHint tells, by launch id, where to find a session's terminal; the current terminal backend provides it
	AttachHint func(launchID string) string
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
	}
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------- subscriptions ----------

// Subscribe returns a channel that first gets a full snapshot, then an event and a new snapshot (without the
// message log) on every change.
func (h *Hub) Subscribe() (<-chan *Push, func()) {
	ch := make(chan *Push, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	ch <- &Push{Value: h.snapshotLocked(true)}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

// broadcastLocked pushes an event (optional), then the latest snapshot; a subscriber that falls behind misses
// messages and catches up with the next snapshot. Each is encoded once, however many subscribers there are.
func (h *Hub) broadcastLocked(extra any) {
	if len(h.subs) == 0 {
		return
	}
	var ev *Push
	if extra != nil {
		ev = &Push{Value: extra}
	}
	snap := &Push{Value: h.snapshotLocked(false)}
	for ch := range h.subs {
		if ev != nil {
			select {
			case ch <- ev:
			default:
			}
		}
		select {
		case ch <- snap:
		default:
		}
	}
}

// Snapshot is the whole state, message log included.
func (h *Hub) Snapshot() Snapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.snapshotLocked(true)
}

// snapshotLocked builds a snapshot; a full one carries the message log, the others leave it null.
func (h *Hub) snapshotLocked(full bool) Snapshot {
	snap := Snapshot{Type: "snapshot", Accounts: []AccountView{}, Sessions: []*Session{}, Asks: []*Ask{}, Launches: []*Launch{}}
	for _, a := range h.accounts {
		v := AccountView{ID: a.ID, Name: a.Name, Seat: a.Seat, Last: a.Last}
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

// sessionLocked returns the session, creating it if needed. An account serves one session at a time:
// while it has a session online that isn't lost, a new one is refused (ErrAccountBusy).
func (h *Hub) sessionLocked(id string, acc *Account) (s *Session, fresh bool, err error) {
	s = h.sessions[id]
	if s != nil && s.Account != acc.ID {
		return nil, false, ErrWrongAccount
	}
	if s != nil && s.State != StateEnded {
		return s, false, nil
	}
	if other := h.onlineLocked(acc.ID); other != nil && other.ID != id {
		if h.now().Sub(other.Updated) < StaleAfter {
			return nil, false, ErrAccountBusy
		}
		other.State, other.Tool = StateEnded, "" // that one was lost long ago; make way
	}
	// an account keeps the record of its current session only
	for sid, old := range h.sessions {
		if old.Account == acc.ID && sid != id {
			h.dropSessionLocked(sid)
		}
	}
	now := h.now()
	if s == nil {
		s = &Session{ID: id, Account: acc.ID, Agents: map[string]*Agent{}, Started: now}
		h.sessions[id] = s
	}
	s.State, s.Updated, s.StateSince = StateIdle, now, now // lost and back again; carry on
	return s, true, nil
}

// Apply handles one mod event. acc is the account its token belongs to.
func (h *Hub) Apply(ev Event, acc *Account) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if ev.Kind == "session.end" {
		// an unknown session going off duty (it started before the bridge restarted); no need to create it
		if s := h.sessions[ev.Session]; s == nil || s.Account != acc.ID || s.State == StateEnded {
			return nil
		}
	}
	s, fresh, err := h.sessionLocked(ev.Session, acc)
	if err != nil {
		return err
	}
	if ev.Login != "" {
		s.Login = ev.Login
	}
	if ev.Model != "" {
		s.Model = ev.Model
	}
	if ev.Effort != "" {
		s.Effort = ev.Effort
	}
	// every mod event carries the folder and the launch id, so a restarted bridge can restore them
	if ev.Cwd != "" {
		s.Cwd = ev.Cwd
	}
	if ev.Launch != "" && s.Launch == "" {
		s.Launch = ev.Launch
		if h.AttachHint != nil {
			s.Attach = h.AttachHint(ev.Launch) // the launch record is gone after a restart; the terminal backend derives it from the id
		}
		if l := h.launches[ev.Launch]; l != nil && l.Account == acc.ID {
			s.Attach = l.Attach
			l.Status, l.Session, l.Error = LaunchOnline, s.ID, ""
			// until the session reports its model and effort, use the ones picked at launch
			if s.Model == "" {
				s.Model = l.Model
			}
			if s.Effort == "" {
				s.Effort = l.Effort
			}
		}
	}
	s.Updated = h.now()
	if ev.Kind == "heartbeat" && !fresh {
		return nil // a heartbeat only keeps the session alive; nothing to tell the page
	}
	// the chat's own events change nothing in the office: they go to the pages as chat events, without a snapshot
	chatOnly := true
	switch ev.Kind {
	case "chat.delta":
		h.chatStreamLocked(s, ev.TurnID, ev.Step, ev.Blocks)
	case "chat.stop":
		h.chatStopLocked(s, ev.TurnID, ev.Step)
	case "chat.user":
		h.chatUserLocked(s, ev)
	case "chat.sent":
		h.chatSentLocked(s, ev.Text)
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
		if target == nil {
			target = &Agent{ID: ev.AgentID, State: StateIdle}
			s.Agents[ev.AgentID] = target
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
	case "session.end":
		s.State, s.Tool = StateEnded, ""
		h.chatNoticeLocked(s.ID, "ended")
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
			if ev.IsAborted {
				h.chatNoticeLocked(s.ID, "aborted")
			}
		}
	case "tool.start":
		setState(StateTool, ev.Tool)
		s.Tools++
		h.chatToolLocked(s, ev, false)
	case "tool.end":
		setState(StateThinking, "")
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
	return nil
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

// Reap marks sessions silent for too long as off duty (closing the terminal leaves the mod no time to say goodbye).
func (h *Hub) Reap() {
	h.mu.Lock()
	defer h.mu.Unlock()
	changed := false
	for _, s := range h.sessions {
		if s.State != StateEnded && h.now().Sub(s.Updated) >= StaleAfter {
			s.State, s.Tool, s.StateSince = StateEnded, "", h.now()
			changed = true
		}
	}
	if changed {
		h.broadcastLocked(nil)
	}
}

// dropSessionLocked forgets a session: its record, its chat and its inbox.
func (h *Hub) dropSessionLocked(id string) {
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
		if l.Status != LaunchStarting && l.Time.Before(cut) {
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
// account name; the most recently active one wins.
func (h *Hub) sessionByNameLocked(name string) *Session {
	var best *Session
	for _, s := range h.sessions {
		if s.State == StateEnded {
			continue
		}
		if s.Title != name && (h.accounts[s.Account] == nil || h.accounts[s.Account].Name != name) {
			continue
		}
		if best == nil || s.Updated.After(best.Updated) {
			best = s
		}
	}
	return best
}

// nameLocked is the name a session goes by: the account name, or the session name once the account is gone.
func (h *Hub) nameLocked(s *Session) string {
	if a := h.accounts[s.Account]; a != nil {
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

func (h *Hub) pushMessageLocked(m *Message) {
	h.messages = append(h.messages, m)
	if len(h.messages) > keepMessages {
		h.messages = h.messages[len(h.messages)-keepMessages:]
	}
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
			return m
		}
	}
	m := &Message{
		ID: newID(), From: fromID, FromName: name, To: to.ID, ToName: h.nameLocked(to),
		Text: body, Origin: originKind(ev.Origin), AgentID: ev.AgentID,
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
		return m
	}
	m := &Message{
		ID: newID(), From: from.ID, FromName: h.nameLocked(from), ToName: ev.To,
		Text: strings.TrimSpace(ev.Text), Origin: originKind(ev.Origin), AgentID: ev.AgentID,
		Delivered: ev.Delivered, Time: h.now(),
	}
	if to := h.sessionByNameLocked(ev.To); to != nil {
		m.To, m.ToName = to.ID, h.nameLocked(to)
	}
	h.pushMessageLocked(m)
	return m
}

// ---------- pending asks ----------

// trimInput is v with every string longer than inputPreview cut to that length (a note at the end says so), and
// whether anything was cut. Maps and lists are copied as far as needed; v itself is left alone.
func trimInput(v any) (any, bool) {
	switch x := v.(type) {
	case string:
		if utf8.RuneCountInString(x) <= inputPreview {
			return x, false
		}
		runes := []rune(x)
		return string(runes[:inputPreview]) + "… [+" + strconv.Itoa(len(runes)-inputPreview) + "]", true
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
	Session   string `json:"session"`
	Kind      string `json:"kind"`
	Tool      string `json:"tool,omitempty"`
	Input     any    `json:"input,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Questions any    `json:"questions,omitempty"`
	AgentID   string `json:"agentId,omitempty"`
}

func (h *Hub) CreateAsk(req AskRequest, acc *Account) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, _, err := h.sessionLocked(req.Session, acc)
	if err != nil {
		return "", err
	}
	s.Updated = h.now()
	a := &Ask{
		ID: newID(), Session: req.Session, Account: acc.ID, Kind: req.Kind,
		Tool: req.Tool, Input: req.Input, Reason: req.Reason, Questions: req.Questions,
		AgentID: req.AgentID, Status: AskPending, Created: h.now(), done: make(chan struct{}),
	}
	a.preview, a.InputTruncated = trimInput(req.Input)
	h.asks[a.ID] = a
	h.chatAskLocked(a)
	before := s.State
	h.refreshWaitingLocked(s)
	h.markStateLocked(s, before)
	h.broadcastLocked(map[string]any{"type": "ask", "id": a.ID, "session": a.Session})
	return a.ID, nil
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
	if s := h.sessions[a.Session]; s != nil {
		before := s.State
		h.refreshWaitingLocked(s)
		h.markStateLocked(s, before)
	}
	h.broadcastLocked(map[string]any{"type": "settled", "id": id, "status": status})
	return nil
}

// ---------- starts from the page ----------

func (h *Hub) addLaunch(l *Launch) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.launches[l.ID] = l
	h.broadcastLocked(map[string]any{"type": "launch", "id": l.ID})
}

func (h *Hub) launchStatus(id string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if l := h.launches[id]; l != nil {
		return l.Status
	}
	return ""
}

func (h *Hub) failLaunch(id string, why *Error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if l := h.launches[id]; l != nil && l.Status == LaunchStarting {
		l.Status, l.Error, l.ErrorCode, l.ErrorParams = LaunchFailed, why.Text, why.Code, why.Params
		h.broadcastLocked(map[string]any{"type": "launch", "id": id})
	}
}

// recentDirs are recently used folders, offered as shortcuts in the launch dialog.
func (h *Hub) recentDirs(n int) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	type seen struct {
		dir string
		at  time.Time
	}
	var all []seen
	for _, l := range h.launches {
		all = append(all, seen{l.Dir, l.Time})
	}
	for _, s := range h.sessions {
		if s.Cwd != "" {
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
