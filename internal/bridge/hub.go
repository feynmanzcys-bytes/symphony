// Package bridge 是 symphony 的中枢：收 mod 上报的会话事件和待决事项，
// 推给审批台和办公室页面。账号存文件，其余状态在内存里，一把锁管到底。
package bridge

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// 会话状态
const (
	StateIdle     = "idle"
	StateThinking = "thinking"
	StateTool     = "tool"
	StateWaiting  = "waiting"
	StateEnded    = "ended"
	StateDone     = "done" // 子代理跑完
)

// Ask 状态
const (
	AskPending   = "pending"
	AskDone      = "done"
	AskCancelled = "cancelled"
)

const (
	keepMessages = 300
	// 结束的会话保留多久再撤掉（账号和座位一直都在）
	endedTTL = 10 * time.Minute
	// 发出和收到的消息在这个时间内按内容配对
	pairWindow = time.Minute
	// StaleAfter：这么久没收到会话的任何消息（mod 每 15 秒报一次心跳）就当它失联下班了
	StaleAfter = 45 * time.Second
)

var (
	ErrUnknownAsk   = errors.New("unknown ask")
	ErrSettled      = errors.New("ask already settled")
	ErrWrongAccount = errors.New("belongs to another account")
)

type Agent struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Tool  string `json:"tool,omitempty"`
}

type Session struct {
	ID      string            `json:"id"`
	Account string            `json:"account"`
	Login   string            `json:"login,omitempty"` // mod 自报的 Claude 登录账号，只做展示
	Title   string            `json:"title"`           // Claude 里的会话名（--name / /rename）
	Cwd     string            `json:"cwd"`
	State   string            `json:"state"`
	Tool    string            `json:"tool,omitempty"`
	Prompt  string            `json:"prompt,omitempty"`
	Agents  map[string]*Agent `json:"agents"`
	Tools   int               `json:"tools"`
	Model   string            `json:"model,omitempty"`  // 当前用的模型，决定小人的块头
	Effort  string            `json:"effort,omitempty"` // 思考强度，决定小人的肌肉
	Launch  string            `json:"launch,omitempty"` // 从页面启动的才有
	Attach  string            `json:"attach,omitempty"` // 去哪儿找它的终端
	Started time.Time         `json:"started"`
	Updated time.Time         `json:"updated"`
	// StateSince 是进入当前状态的时间（页面靠它算闲了多久，闲久了小人就睡觉）
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

	done chan struct{}
}

// Message 是会话之间的一条内部消息（SendMessage / agent team 邮箱）。
type Message struct {
	ID        string    `json:"id"`
	From      string    `json:"from,omitempty"` // 发件会话 id，认不出来就空着
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

// Event 是 mod 上报的一条事件，字段按 kind 取用。
type Event struct {
	Session     string  `json:"session"`
	Kind        string  `json:"kind"`
	Login       string  `json:"login,omitempty"` // mod 自报的 Claude 登录账号
	Model       string  `json:"model,omitempty"`
	Effort      string  `json:"effort,omitempty"`
	Launch      string  `json:"launch,omitempty"` // 从页面启动时 bridge 给的编号
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
}

type Snapshot struct {
	Type     string        `json:"type"`
	Accounts []AccountView `json:"accounts"`
	Sessions []*Session    `json:"sessions"`
	Asks     []*Ask        `json:"asks"`
	Messages []*Message    `json:"messages"`
	Launches []*Launch     `json:"launches"`
}

// Hub 持有全部状态。
type Hub struct {
	mu       sync.Mutex
	accounts map[string]*Account
	dataDir  string // 账号和消息记录存在这里；空表示不落盘（测试用）
	sessions map[string]*Session
	asks     map[string]*Ask
	messages []*Message
	launches map[string]*Launch
	subs     map[chan any]struct{}
	now      func() time.Time
	// AttachHint 按启动编号说出去哪儿找会话的终端，由当前的终端后端提供
	AttachHint func(launchID string) string
}

func NewHub() *Hub {
	return &Hub{
		accounts: map[string]*Account{},
		sessions: map[string]*Session{},
		asks:     map[string]*Ask{},
		launches: map[string]*Launch{},
		subs:     map[chan any]struct{}{},
		now:      time.Now,
	}
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------- 订阅 ----------

// Subscribe 返回一个通道，先收到一份快照，之后每次变化都会收到事件和新快照。
func (h *Hub) Subscribe() (<-chan any, func()) {
	ch := make(chan any, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	ch <- h.snapshotLocked()
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

// broadcastLocked 推一条事件（可选）再推最新快照；订阅方跟不上就丢，它下次拿到快照就追上了。
func (h *Hub) broadcastLocked(extra any) {
	snap := h.snapshotLocked()
	for ch := range h.subs {
		if extra != nil {
			select {
			case ch <- extra:
			default:
			}
		}
		select {
		case ch <- snap:
		default:
		}
	}
}

func (h *Hub) Snapshot() Snapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.snapshotLocked()
}

func (h *Hub) snapshotLocked() Snapshot {
	snap := Snapshot{Type: "snapshot", Accounts: []AccountView{}, Sessions: []*Session{}, Asks: []*Ask{}, Messages: []*Message{}, Launches: []*Launch{}}
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
			snap.Asks = append(snap.Asks, &cp)
		}
	}
	sort.Slice(snap.Asks, func(i, j int) bool { return snap.Asks[i].Created.Before(snap.Asks[j].Created) })
	for _, m := range h.messages {
		cp := *m
		snap.Messages = append(snap.Messages, &cp)
	}
	for _, l := range h.launches {
		cp := *l
		snap.Launches = append(snap.Launches, &cp)
	}
	sort.Slice(snap.Launches, func(i, j int) bool { return snap.Launches[i].Time.Before(snap.Launches[j].Time) })
	return snap
}

// ---------- 会话 ----------

// sessionLocked 取会话，没有就建。一个账号同一时间只给一个会话用：
// 账号上已有会话在线、而且没失联，新会话就被拒（ErrAccountBusy）。
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
		other.State, other.Tool = StateEnded, "" // 那个早就失联了，让位
	}
	// 一个账号只留现在这一个会话的记录
	for sid, old := range h.sessions {
		if old.Account == acc.ID && sid != id {
			delete(h.sessions, sid)
		}
	}
	now := h.now()
	if s == nil {
		s = &Session{ID: id, Account: acc.ID, Agents: map[string]*Agent{}, Started: now}
		h.sessions[id] = s
	}
	s.State, s.Updated, s.StateSince = StateIdle, now, now // 失联后又回来的，接着用
	return s, true, nil
}

// Apply 处理一条 mod 事件。acc 是按 token 认出来的账号。
func (h *Hub) Apply(ev Event, acc *Account) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if ev.Kind == "session.end" {
		// 不认识的会话说要下班（bridge 重启前就开着的），没必要为它新建一个
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
	// mod 每条事件都带着目录和启动编号，bridge 重启后靠它们补回来
	if ev.Cwd != "" {
		s.Cwd = ev.Cwd
	}
	if ev.Launch != "" && s.Launch == "" {
		s.Launch = ev.Launch
		if h.AttachHint != nil {
			s.Attach = h.AttachHint(ev.Launch) // bridge 重启后启动记录没了，靠终端后端按编号推出来
		}
		if l := h.launches[ev.Launch]; l != nil && l.Account == acc.ID {
			s.Attach = l.Attach
			l.Status, l.Session, l.Error = LaunchOnline, s.ID, ""
			// 会话自己报模型和强度之前，先用启动时选的
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
		return nil // 心跳只续命，不用通知页面
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
	case "session.title":
		// 新会话的 SessionStart 不带名字，别把已知的名字清掉
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
		}
	case "tool.start":
		setState(StateTool, ev.Tool)
		s.Tools++
	case "tool.end":
		setState(StateThinking, "")
	case "message.receive":
		if m := h.receiveLocked(s, ev); m != nil {
			extra = map[string]any{"type": "message", "message": m}
			h.saveMessagesLocked()
		}
	case "message.send":
		if m := h.sendLocked(s, ev); m != nil {
			extra = map[string]any{"type": "message", "message": m}
			h.saveMessagesLocked()
		}
	}
	h.refreshWaitingLocked(s)
	h.markStateLocked(s, before)
	h.broadcastLocked(extra)
	return nil
}

// markStateLocked 在状态变了时记下变的时间。
func (h *Hub) markStateLocked(s *Session, before string) {
	if s.State != before {
		s.StateSince = h.now()
	}
}

// refreshWaitingLocked：有待决事项的会话标成 waiting，没了就退回 thinking。
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

// Reap 把太久没动静的会话标成下班（直接关掉终端时 mod 来不及说再见）。
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

// Prune 撤掉结束很久的会话和早已了结的事项，长时间跑的 bridge 不至于越积越多。
func (h *Hub) Prune() {
	h.mu.Lock()
	defer h.mu.Unlock()
	cut := h.now().Add(-endedTTL)
	changed := false
	for id, s := range h.sessions {
		if s.State == StateEnded && s.Updated.Before(cut) {
			delete(h.sessions, id)
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

// ---------- 会话间消息 ----------

var (
	crossSession = regexp.MustCompile(`(?s)^\s*<cross-session-message\b([^>]*)>\s*(.*?)\s*(?:</cross-session-message>\s*)?$`)
	fromName     = regexp.MustCompile(`\bfrom-name="([^"]*)"`)
)

// parseIncoming 从收到的正文里拆出发件人名字和消息本体。
func parseIncoming(text string) (from, body string) {
	if m := crossSession.FindStringSubmatch(text); m != nil {
		if f := fromName.FindStringSubmatch(m[1]); f != nil {
			from = f[1]
		}
		return from, m[2]
	}
	return "", strings.TrimSpace(text)
}

// sessionByNameLocked 按名字找在线的会话：先认 Claude 里的会话名，再认账号名，取最近活跃的。
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

// nameLocked 是会话对外的名字：账号名，账号没了就用会话名。
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

// findPairLocked 找对面那一半：收到时找已经记下的发出，发出时找已经记下的收到。
// 两边先后不定（发件方的 hook 要等 next 返回才上报），所以按发件会话 + 内容 + 时间窗配。
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

// ---------- 待决事项 ----------

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
	h.asks[a.ID] = a
	before := s.State
	h.refreshWaitingLocked(s)
	h.markStateLocked(s, before)
	h.broadcastLocked(map[string]any{"type": "ask", "id": a.ID, "session": a.Session})
	return a.ID, nil
}

// Wait 挂起直到有结果、超时或请求断开，返回当时的状态。
// account 为空表示不校验（只给测试和内部用）。mod 在等，说明会话还活着，顺手续命。
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

// Settle 给出结果（done）或撤销（cancelled）。account 为空表示网页端，不校验。
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
	if s := h.sessions[a.Session]; s != nil {
		before := s.State
		h.refreshWaitingLocked(s)
		h.markStateLocked(s, before)
	}
	h.broadcastLocked(map[string]any{"type": "settled", "id": id, "status": status})
	return nil
}

// ---------- 页面启动 ----------

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

func (h *Hub) failLaunch(id, why string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if l := h.launches[id]; l != nil && l.Status == LaunchStarting {
		l.Status, l.Error = LaunchFailed, why
		h.broadcastLocked(map[string]any{"type": "launch", "id": id})
	}
}

// recentDirs 是最近用过的工作目录，给启动框当快捷选项。
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
