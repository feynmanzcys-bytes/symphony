package bridge

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// The chat: what a session's employee window shows. The mod reports what the model streams (thinking and text, in
// pieces), the tools it calls, the prompts that enter; the hub adds the messages between sessions and the asks, and
// keeps the last keepChat items of each session. The page gets the log once (/api/chat/<session>) and then each
// change as a "chat" event: a whole item, or a piece of text to append to one.

const (
	keepChat = 400
	// chatTextMax is the most text one item keeps (a long answer or thought is cut from the front)
	chatTextMax = 64 << 10
	// chatPromptMax is how much of a prompt the mod sends
	chatPromptMax = 4000
)

// chat item kinds
const (
	ChatUser     = "user"     // a prompt: typed in the terminal, sent from the page, or a command
	ChatThinking = "thinking" // a thinking block, streamed
	ChatText     = "text"     // a text block of the answer, streamed
	ChatTool     = "tool"     // a tool call: its name, main argument and how it ended
	ChatMessage  = "message"  // a message from or to another session
	ChatAsk      = "ask"      // a permission request or question, and how it was settled
	ChatNotice   = "notice"   // the session's own events: interrupted, off duty, compacted, an error (Text is the code, Summary the detail)
	ChatCommand  = "command"  // a slash command that ran: /name and its arguments
)

type ChatItem struct {
	ID      string    `json:"id"`
	Seq     int       `json:"seq"`
	Kind    string    `json:"kind"`
	Text    string    `json:"text,omitempty"`
	Done    bool      `json:"done"`              // a streamed block ended; a tool call ended; an ask was settled
	Tool    string    `json:"tool,omitempty"`    // tool and ask: the tool's name
	Summary string    `json:"summary,omitempty"` // tool: its main argument; user: why it was dropped
	Status  string    `json:"status,omitempty"`  // tool: ok|error|denied; user: queued|sent|dropped; ask: pending|allow|deny|answered|cancelled
	AgentID string    `json:"agentId,omitempty"`
	Origin  string    `json:"origin,omitempty"` // user: composer (typed), page, task (a task's kickoff), or the engine's origin kind
	From    string    `json:"from,omitempty"`   // message: the sender's name
	To      string    `json:"to,omitempty"`     // message: the recipient's name
	Ask     string    `json:"ask,omitempty"`    // ask: its id; kind: permission|question in Status until settled
	AskKind string    `json:"askKind,omitempty"`
	Secs    int       `json:"secs,omitempty"` // thinking: how long it took, once done
	Ms      int       `json:"ms,omitempty"`   // tool: how long it took; ask: how long it waited to be settled
	Time    time.Time `json:"time"`
	key     string    // streamed blocks: turn/step/block; tools: the tool_use id; asks: the ask id
}

type chatLog struct {
	items []*ChatItem
	seq   int
	byKey map[string]*ChatItem
}

// ChatDelta is one piece of the model's response the mod reports: block i of a step, of kind thinking or text.
type ChatDelta struct {
	I    int    `json:"i"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}

func (h *Hub) chatLocked(session string) *chatLog {
	c := h.chats[session]
	if c == nil {
		c = &chatLog{byKey: map[string]*ChatItem{}}
		h.chats[session] = c
	}
	return c
}

// chatAppendLocked adds an item and pushes it to the pages.
func (h *Hub) chatAppendLocked(session string, it *ChatItem) *ChatItem {
	c := h.chatLocked(session)
	c.seq++
	it.Seq = c.seq
	if it.ID == "" {
		it.ID = newID()
	}
	if it.Time.IsZero() {
		it.Time = h.now()
	}
	c.items = append(c.items, it)
	if it.key != "" {
		c.byKey[it.key] = it
	}
	if len(c.items) > keepChat {
		for _, old := range c.items[:len(c.items)-keepChat] {
			if old.key != "" && c.byKey[old.key] == old {
				delete(c.byKey, old.key)
			}
		}
		c.items = c.items[len(c.items)-keepChat:]
	}
	h.pushChatLocked(session, it, "")
	return it
}

// pushChatLocked sends a chat event to the pages without a snapshot: a whole item, or (with delta) text to append.
func (h *Hub) pushChatLocked(session string, it *ChatItem, delta string) {
	if len(h.subs) == 0 {
		return
	}
	ev := map[string]any{"type": "chat", "session": session}
	if delta != "" {
		ev["delta"] = map[string]any{"id": it.ID, "seq": it.Seq, "text": delta}
	} else {
		ev["item"] = it
	}
	if p := h.pushLocked(ev); p != nil {
		h.deliverLocked(p)
	}
}

// chatStreamLocked appends pieces of the response to their blocks, making the blocks as they first appear.
func (h *Hub) chatStreamLocked(s *Session, turn string, step int, blocks []ChatDelta) {
	c := h.chatLocked(s.ID)
	for _, d := range blocks {
		// a thinking block may carry no words (a model whose thinking is summarized); it still shows that it thinks
		if d.Kind != ChatThinking && d.Kind != ChatText || d.Text == "" && d.Kind != ChatThinking {
			continue
		}
		key := turn + "/" + itoa(step) + "/" + itoa(d.I)
		if it := c.byKey[key]; it != nil {
			if d.Text != "" {
				it.Text = cutFront(it.Text+d.Text, chatTextMax)
				h.pushChatLocked(s.ID, it, d.Text)
			}
			continue
		}
		h.chatAppendLocked(s.ID, &ChatItem{Kind: d.Kind, Text: cutFront(d.Text, chatTextMax), key: key})
	}
}

// chatStopLocked marks the step's blocks done.
func (h *Hub) chatStopLocked(s *Session, turn string, step int) {
	c := h.chatLocked(s.ID)
	prefix := turn + "/" + itoa(step) + "/"
	var said *ChatItem // the step's last text block: the conversation's newest line
	for key, it := range c.byKey {
		if strings.HasPrefix(key, prefix) && !it.Done {
			it.Done = true
			if it.Kind == ChatThinking {
				it.Secs = int(h.now().Sub(it.Time).Round(time.Second) / time.Second)
			}
			if it.Kind == ChatText && it.AgentID == "" && (said == nil || it.Seq > said.Seq) {
				said = it
			}
			h.pushChatLocked(s.ID, it, "")
		}
	}
	if said != nil {
		h.lastLocked(s, said.Text)
	}
}

// chatToolLocked records a tool call starting or ending.
func (h *Hub) chatToolLocked(s *Session, ev Event, end bool) {
	c := h.chatLocked(s.ID)
	key := "tool/" + ev.ToolUseID
	if ev.ToolUseID == "" {
		key = ""
	}
	if end {
		it := c.byKey[key]
		if it == nil {
			return
		}
		it.Done = true
		it.Ms = int(ev.DurationMs)
		switch {
		case ev.Denied || it.Status == "denied": // a tool.fail may have named the denial first
			it.Status = "denied"
		case ev.IsError:
			it.Status = "error"
		default:
			it.Status = "ok"
		}
		h.pushChatLocked(s.ID, it, "")
		return
	}
	h.chatAppendLocked(s.ID, &ChatItem{Kind: ChatTool, Tool: ev.Tool, Summary: cutRunes(ev.Summary, 300), AgentID: ev.AgentID, key: key})
}

// chatToolFailLocked puts the reason a tool call failed, or a permission was denied, on its row (either side may
// come first).
func (h *Hub) chatToolFailLocked(s *Session, ev Event) {
	if ev.ToolUseID == "" {
		return
	}
	c := h.chatLocked(s.ID)
	it := c.byKey["tool/"+ev.ToolUseID]
	if it == nil {
		return
	}
	it.Text = cutRunes(strings.TrimSpace(ev.Text), 500)
	if ev.Denied {
		it.Status = "denied"
	} else if it.Done && it.Status == "ok" {
		it.Status = "error"
	}
	h.pushChatLocked(s.ID, it, "")
}

// chatCommandLocked records a slash command that ran, unless the prompt bubble just before it is that command typed
// (prompt.submit reports it first): then the bubble is enough.
func (h *Hub) chatCommandLocked(s *Session, ev Event) {
	if ev.Command == "" {
		return
	}
	c := h.chatLocked(s.ID)
	if n := len(c.items); n > 0 {
		if last := c.items[n-1]; last.Kind == ChatUser && strings.HasPrefix(last.Text, "/"+ev.Command) {
			return
		}
	}
	h.chatAppendLocked(s.ID, &ChatItem{Kind: ChatCommand, Text: "/" + ev.Command, Summary: cutRunes(ev.Args, 200), Done: true})
}

// chatUserLocked records a prompt that entered the session.
func (h *Hub) chatUserLocked(s *Session, ev Event) {
	origin := originKind(ev.Origin)
	switch origin {
	case "peer", "peer-send-message", "coordinator":
		return // messages from other sessions have their own items
	}
	text := strings.TrimSpace(ev.Text)
	if text == "" {
		return
	}
	h.chatAppendLocked(s.ID, &ChatItem{Kind: ChatUser, Text: cutRunes(text, chatPromptMax), Origin: origin, Done: true})
	switch origin {
	case "task-notification", "scheduled-trigger": // the engine's own prompts: not what was said
	default:
		h.lastLocked(s, text)
	}
}

// lastMax is how much of the conversation's newest line a session keeps (Session.Last), in runes.
const lastMax = 80

// lastLocked notes the conversation's newest line on the session (a prompt's, an answer's or a message's first line,
// cut to lastMax runes) and tells the pages without a snapshot, like the usage figures: a list shows it under the name.
func (h *Hub) lastLocked(s *Session, text string) {
	line := strings.TrimSpace(text)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	line = cutRunes(line, lastMax)
	if line == "" || line == s.Last {
		return
	}
	s.Last = line
	if len(h.subs) == 0 {
		return
	}
	if p := h.pushLocked(map[string]any{"type": "last", "session": s.ID, "text": line}); p != nil {
		h.deliverLocked(p)
	}
}

// chatSentLocked: the mod submitted a prompt the page sent; the queued item becomes sent.
func (h *Hub) chatSentLocked(s *Session, text string) { h.chatSubmittedLocked(s, text, "sent", "") }

// chatDroppedLocked: a prompt the page sent didn't enter the session (a hook refused it, or the engine); the queued
// item becomes dropped, with the reason.
func (h *Hub) chatDroppedLocked(s *Session, text, reason string) {
	h.chatSubmittedLocked(s, text, "dropped", cutRunes(strings.TrimSpace(reason), 500))
}

// chatSubmittedLocked settles the queued item of a prompt the page sent: sent, or dropped and why (summary). One the
// log no longer has comes back as a new item.
func (h *Hub) chatSubmittedLocked(s *Session, text, status, summary string) {
	c := h.chatLocked(s.ID)
	text = strings.TrimSpace(text)
	for _, it := range c.items {
		if it.Kind == ChatUser && it.Status == "queued" && sameText(it.Text, text) {
			it.Status, it.Summary, it.Done = status, summary, true
			h.pushChatLocked(s.ID, it, "")
			return
		}
	}
	h.chatAppendLocked(s.ID, &ChatItem{Kind: ChatUser, Text: cutRunes(text, chatPromptMax), Origin: "page", Status: status, Summary: summary, Done: true})
}

func (h *Hub) chatMessageLocked(session string, m *Message) {
	c := h.chatLocked(session)
	if it := c.byKey["msg/"+m.ID]; it != nil { // the other half of a pair: names may have been filled in
		it.From, it.To, it.Text = m.FromName, m.ToName, m.Text
		h.pushChatLocked(session, it, "")
		return
	}
	h.chatAppendLocked(session, &ChatItem{Kind: ChatMessage, Text: m.Text, From: m.FromName, To: m.ToName, AgentID: m.AgentID, Done: true, key: "msg/" + m.ID})
}

func (h *Hub) chatAskLocked(a *Ask) {
	it := &ChatItem{Kind: ChatAsk, Ask: a.ID, AskKind: a.Kind, Tool: a.Tool, AgentID: a.AgentID, Status: "pending", key: "ask/" + a.ID}
	if a.Kind == "question" {
		it.Summary = askSummary(a.Questions)
	} else if a.Kind == "elicitation" {
		it.Tool = a.Server
		it.Summary = cutRunes(a.Message, 200)
	} else {
		it.Summary = toolSummary(a.Tool, a.Input)
	}
	h.chatAppendLocked(a.Session, it)
}

// chatSettledLocked marks an ask's item with its outcome.
func (h *Hub) chatSettledLocked(a *Ask) {
	c := h.chats[a.Session]
	if c == nil {
		return
	}
	it := c.byKey["ask/"+a.ID]
	if it == nil {
		return
	}
	it.Done = true
	it.Ms = int(h.now().Sub(it.Time) / time.Millisecond) // (the page leaves it out of how long the model worked)
	switch {
	case a.Status == AskCancelled:
		it.Status = "cancelled"
	case a.Kind == "question":
		it.Status = "answered"
		var parts []string
		if answers, ok := a.Answer["answers"].(map[string]any); ok {
			for _, v := range answers {
				if s, ok := v.(string); ok && s != "" {
					parts = append(parts, s)
				}
			}
		}
		it.Text = cutRunes(strings.Join(parts, " · "), 500)
	case a.Kind == "elicitation":
		it.Status, _ = a.Answer["action"].(string) // accept | decline | cancel, as the terminal answered it
	default:
		it.Status, _ = a.Answer["decision"].(string)
		it.Text, _ = a.Answer["reason"].(string)
		if a.Answer["remember"] == true && a.Remember != nil && it.Status == "allow" {
			it.Status = "allowSession"
		}
	}
	h.pushChatLocked(a.Session, it, "")
}

func (h *Hub) chatNoticeLocked(session, text string) {
	h.chatAppendLocked(session, &ChatItem{Kind: ChatNotice, Text: text, Done: true})
}

// chatNoticeDetailLocked is a notice with a detail the page shows after the translated code (an error's text, the
// models of a switch).
func (h *Hub) chatNoticeDetailLocked(session, code, detail string) {
	h.chatAppendLocked(session, &ChatItem{Kind: ChatNotice, Text: code, Summary: cutRunes(detail, 300), Done: true})
}

// Chat is a session's chat log.
func (h *Hub) Chat(session string) []*ChatItem {
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.chats[session]
	if c == nil {
		return []*ChatItem{}
	}
	out := make([]*ChatItem, 0, len(c.items))
	for _, it := range c.items {
		cp := *it
		out = append(out, &cp)
	}
	return out
}

// askSummary is the first question's text.
func askSummary(questions any) string {
	list, _ := questions.([]any)
	if len(list) == 0 {
		return ""
	}
	q, _ := list[0].(map[string]any)
	text, _ := q["question"].(string)
	if len(list) > 1 {
		text += " (+" + itoa(len(list)-1) + ")"
	}
	return cutRunes(text, 300)
}

// toolSummary is a tool call's main argument: the command, the file, the pattern, the prompt.
func toolSummary(tool string, input any) string {
	m, _ := input.(map[string]any)
	if m == nil {
		return ""
	}
	for _, k := range []string{"command", "file_path", "notebook_path", "pattern", "url", "query", "description", "prompt", "skill", "path"} {
		if v, ok := m[k].(string); ok && v != "" {
			return cutRunes(strings.TrimSpace(v), 300)
		}
	}
	return ""
}

func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// cutFront keeps the last n bytes of s (at a rune boundary).
func cutFront(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[len(s)-n:]
	for i := 0; i < len(s) && i < 4; i++ {
		if utf8.RuneStart(s[i]) {
			return s[i:]
		}
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// ---------- the inbox: what the page sends to a session ----------

// InboxItem is one thing for the mod to do: say submits text as the person's prompt, abort stops the running turn.
// Its id counts up per session; the mod names the last one it did in its next poll (ack), and only then is it dropped:
// a poll cut off between the bridge handing items out and the mod reading them loses nothing, the next poll gets
// them again.
type InboxItem struct {
	ID      int64  `json:"id,omitempty"`
	Kind    string `json:"kind"`              // say | abort | command
	Text    string `json:"text,omitempty"`    // say: the prompt
	Command string `json:"command,omitempty"` // command: the slash command's name (model, effort), run as if typed
	Args    string `json:"args,omitempty"`    // command: what follows the name
}

// SetModel changes a running session's model and thinking effort from the page: the slash commands /model and /effort
// go to the session's inbox as command items, which the mod runs as if typed (the engine's own command.run report
// then shows them in the chat, and the model switch comes back as session.model). Each only when asked for, and one
// of the launch dialog's choices ("" for the model is Claude Code's own: /model default; the effort can't be unset);
// nothing asked for is model_empty.
func (h *Hub) SetModel(session string, model, effort *string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.sessions[session]
	if s == nil || s.State == StateEnded {
		return ErrSessionOffline
	}
	var items []InboxItem
	if model != nil {
		if !validOption(launchModels, *model) {
			return newError("launch_bad_option", "no such model: "+*model)
		}
		arg := *model
		if arg == "" {
			arg = "default"
		}
		items = append(items, InboxItem{Kind: "command", Command: "model", Args: arg})
	}
	if effort != nil {
		if *effort == "" || !validOption(launchEfforts, *effort) {
			return newError("launch_bad_option", "no such effort: "+*effort)
		}
		items = append(items, InboxItem{Kind: "command", Command: "effort", Args: *effort})
	}
	if len(items) == 0 {
		return newError("model_empty", "nothing to change")
	}
	for _, it := range items {
		h.postInboxLocked(session, it)
	}
	return nil
}

type inbox struct {
	items  []InboxItem // not yet acknowledged, oldest first; handed out again until they are
	next   int64       // the last item's id (the next is one more)
	handed time.Time   // when the unacknowledged items were last handed out
	wake   chan struct{}
}

// inboxRehand is how long the bridge waits before it hands unacknowledged items out again: a second poll out at the
// same time (two mods in one session: an installed one beside a --plugin-dir one) would submit the prompt too; a poll
// that was cut off gets them at its next try, this much later.
const inboxRehand = 3 * time.Second

func (h *Hub) inboxLocked(session string) *inbox {
	b := h.inboxes[session]
	if b == nil {
		// the ids start at the time in microseconds: a bridge that restarted hands out ids larger than any the mod
		// acknowledged in its earlier life, so a stale ack drops nothing (the mod names the last id it got, and keeps
		// it across the restart)
		b = &inbox{wake: make(chan struct{}), next: time.Now().UnixMicro()}
		h.inboxes[session] = b
	}
	return b
}

func (h *Hub) postInboxLocked(session string, it InboxItem) {
	b := h.inboxLocked(session)
	b.next++
	it.ID = b.next
	b.items = append(b.items, it)
	b.handed = time.Time{} // new: the next poll gets it at once
	close(b.wake)
	b.wake = make(chan struct{})
}

// Say queues text for a session as the person's next prompt; the chat shows it as queued until the mod has it.
func (h *Hub) Say(session, text string) (*ChatItem, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.sessions[session]
	if s == nil || s.State == StateEnded {
		return nil, ErrSessionOffline
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, newError("say_empty", "nothing to send")
	}
	cp := *h.sayLocked(session, text, "page")
	return &cp, nil
}

// sayLocked queues text for a session as the person's next prompt: a queued item in the chat (origin page, or task for
// a task's kickoff) and a say in the inbox.
func (h *Hub) sayLocked(session, text, origin string) *ChatItem {
	it := h.chatAppendLocked(session, &ChatItem{Kind: ChatUser, Text: cutRunes(text, chatPromptMax), Origin: origin, Status: "queued"})
	h.postInboxLocked(session, InboxItem{Kind: "say", Text: text})
	if s := h.sessions[session]; s != nil && origin == "page" {
		h.lastLocked(s, text)
	}
	return it
}

// Abort asks the session to stop its running turn.
func (h *Hub) Abort(session string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.sessions[session]
	if s == nil || s.State == StateEnded {
		return ErrSessionOffline
	}
	h.postInboxLocked(session, InboxItem{Kind: "abort"})
	return nil
}

// Inbox hands the mod what the page queued for its session, waiting up to timeout for something to come. ack is the
// id of the last item the mod did (its last poll's): that one and the ones before it are dropped, the rest handed out
// again. ack < 0 is a mod that acknowledges nothing (before 1.2.0): what is handed out is dropped at once, as before. A
// waiting mod means the session is alive, so it counts as a heartbeat.
func (h *Hub) Inbox(session, account string, ack int64, timeout time.Duration, cancel <-chan struct{}) ([]InboxItem, error) {
	h.mu.Lock()
	s := h.sessions[session]
	if s == nil || s.Account != account {
		h.mu.Unlock()
		return nil, ErrWrongAccount
	}
	if s.State != StateEnded {
		s.Updated = h.now()
	}
	b := h.inboxLocked(session)
	take := func() []InboxItem {
		if ack < 0 {
			items := b.items
			b.items = nil
			return items
		}
		b.items = slices.DeleteFunc(b.items, func(it InboxItem) bool { return it.ID <= ack })
		if len(b.items) == 0 || h.now().Sub(b.handed) < inboxRehand {
			return nil // nothing, or handed out a moment ago: another poll of the same session has them
		}
		b.handed = h.now()
		return slices.Clone(b.items)
	}
	if items := take(); len(items) > 0 {
		h.mu.Unlock()
		return items, nil
	}
	wake := b.wake
	h.mu.Unlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-wake:
	case <-timer.C:
	case <-cancel:
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	items := take()
	if items == nil {
		items = []InboxItem{}
	}
	return items, nil
}
