package bridge

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

var testWeb = fstest.MapFS{
	"themes/messenger/index.html": {Data: []byte("<html>messenger</html>")},
	"themes/messenger/theme.json": {Data: []byte(`{"name":"messenger","title":{"en":"Coworkers"},"since":1}`)},
	"themes/plain/index.html":     {Data: []byte("<html>plain</html>")},
	"themes/plain/theme.json":     {Data: []byte(`{"name":"plain","title":{"en":"Plain"},"since":1}`)},
	"themes/zen/index.html":       {Data: []byte("<html>zen</html>")},
	"themes/zen/theme.json":       {Data: []byte(`{"name":"zen","title":{"en":"Zen","zh-CN":"禅"},"since":1}`)},
	"themes/zen/zen.css":          {Data: []byte("body{}")},
	"themes/bad/index.html":       {Data: []byte("<html>bad</html>")}, // no manifest: not a theme
	"themes/wrong/index.html":     {Data: []byte("<html>wrong</html>")},
	"themes/wrong/theme.json":     {Data: []byte(`{"name":"other"}`)}, // the manifest's name must be the folder's
	"approve.html":                {Data: []byte("<html>approve</html>")},
	"approve.js":                  {Data: []byte("export const approve = 1")},
	"asks.js":                     {Data: []byte("export const x = 1")},
	"i18n.js":                     {Data: []byte("export const t = key => key")},
	"symphony.js":                 {Data: []byte("export const connect = () => {}")},
	"ui/chat.js":                  {Data: []byte("export const chat = 1")},
	"ui/ui.css":                   {Data: []byte(".it{}")},
	"locales/en.json":             {Data: []byte(`{"error.unknown": "something went wrong"}`)},
	"locales/xx.json":             {Data: []byte(`{}`)}, // not a supported language: the bridge must not serve it
}

// testKey is the page key of the bridges these tests start.
const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// client sends what the page sends, with the page key, or with modOnly what the mod sends: no page key, its token in
// header.
type client struct {
	t       *testing.T
	base    string
	header  map[string]string
	modOnly bool
}

func newBridge(t *testing.T) (*Server, string) {
	t.Helper()
	srv := NewServer(NewHub(), testWeb)
	srv.Key = testKey
	srv.WaitTimeout = 300 * time.Millisecond
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return srv, ts.URL
}

// as creates an account and returns a client that carries its token, as its session's mod.
func as(t *testing.T, srv *Server, base, name string) (client, *Account) {
	t.Helper()
	acc, err := srv.Hub.CreateAccount(name)
	if err != nil {
		t.Fatal(err)
	}
	return client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + acc.Token}, modOnly: true}, acc
}

// pageGet is a GET with the page key, as a signed-in page sends it.
func pageGet(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(keyHeader, testKey)
	return http.DefaultClient.Do(req)
}

func (c client) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("content-type", "application/json")
	if !c.modOnly {
		req.Header.Set(keyHeader, testKey)
	}
	for k, v := range c.header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (c client) ok(method, path string, body any) map[string]any {
	c.t.Helper()
	status, out := c.do(method, path, body)
	if status != 200 {
		c.t.Fatalf("%s %s: %d %v", method, path, status, out)
	}
	return out
}

func (c client) status(method, path string, body any) int {
	c.t.Helper()
	status, _ := c.do(method, path, body)
	return status
}

// failure sends a request that should fail and returns the status and the error code from the JSON body.
func (c client) failure(method, path string, body any) (int, string) {
	c.t.Helper()
	status, out := c.do(method, path, body)
	code, _ := out["code"].(string)
	return status, code
}

func session(t *testing.T, srv *Server, id string) *Session {
	t.Helper()
	for _, s := range srv.Hub.Snapshot().Sessions {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no session %s", id)
	return nil
}

func view(t *testing.T, srv *Server, id string) AccountView {
	t.Helper()
	for _, a := range srv.Hub.Snapshot().Accounts {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("no account %s", id)
	return AccountView{}
}

func TestEventsDriveSessionState(t *testing.T) {
	srv, base := newBridge(t)
	c, acc := as(t, srv, base, "前端小王")

	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Cwd: "/w/a", Login: "a@x.com", Model: "claude-opus-5-5"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "turn.start", Text: "hi"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "tool.start", Tool: "Bash"})
	s := session(t, srv, "s1")
	if s.State != StateTool || s.Tool != "Bash" || s.Cwd != "/w/a" || s.Login != "a@x.com" || s.Account != acc.ID || s.Model != "claude-opus-5-5" {
		t.Fatalf("wrong state: %+v", s)
	}
	if view(t, srv, acc.ID).Session != "s1" {
		t.Fatal("the account should show its online session")
	}

	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "tool.start", Tool: "Read", AgentID: "ag1"})
	if a := session(t, srv, "s1").Agents["ag1"]; a == nil || a.Tool != "Read" || a.State != StateTool {
		t.Fatalf("wrong subagent: %+v", a)
	}
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "turn.complete", AgentID: "ag1"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "turn.complete"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.model", Model: "claude-sonnet-5-5", Effort: "high"})
	s = session(t, srv, "s1")
	if s.State != StateIdle || s.Agents["ag1"].State != StateDone || s.Tools != 2 || s.Model != "claude-sonnet-5-5" || s.Effort != "high" {
		t.Fatalf("wrong state after the turn: %+v", s)
	}
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.end"})
	if session(t, srv, "s1").State != StateEnded || view(t, srv, acc.ID).Session != "" {
		t.Fatal("the session should be off duty and the account offline")
	}
}

func TestStateSince(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "a")
	now := time.Now()
	srv.Hub.now = func() time.Time { return now }
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start"})
	idleAt := session(t, srv, "s1").StateSince
	// heartbeats and events that keep the state leave StateSince alone (the page uses it to tell how long a session idles)
	now = now.Add(time.Minute)
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "heartbeat"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.title", Title: "x"})
	if got := session(t, srv, "s1").StateSince; !got.Equal(idleAt) {
		t.Fatalf("the state didn't change, so StateSince shouldn't move: %v -> %v", idleAt, got)
	}
	// the snapshot pushed to the page carries the new time too (no pushing before updating)
	ch, stop := srv.Hub.Subscribe()
	defer stop()
	<-ch
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "turn.start"})
	if got := session(t, srv, "s1").StateSince; !got.Equal(now) {
		t.Fatalf("the state changed, so StateSince should move: %v", got)
	}
	for msg := range ch {
		if snap, ok := msg.Value.(Snapshot); ok {
			if !snap.Sessions[0].StateSince.Equal(now) {
				t.Fatalf("the pushed snapshot has a stale StateSince: %v", snap.Sessions[0].StateSince)
			}
			break
		}
	}
}

func TestTokenRequired(t *testing.T) {
	srv, base := newBridge(t)
	anon := client{t: t, base: base, modOnly: true}
	bad := client{t: t, base: base, header: map[string]string{"authorization": "Bearer nope"}, modOnly: true}
	for _, c := range []client{anon, bad} {
		if st, code := c.failure("POST", "/api/event", Event{Session: "s1", Kind: "session.start"}); st != http.StatusUnauthorized || code != "token_invalid" {
			t.Fatalf("a missing or wrong token should get 401 token_invalid, got %d %q", st, code)
		}
	}
	if n := len(srv.Hub.Snapshot().Sessions); n != 0 {
		t.Fatalf("no session should be created, found %d", n)
	}
}

func TestOneSessionPerAccount(t *testing.T) {
	srv, base := newBridge(t)
	c, acc := as(t, srv, base, "后端老李")
	now := time.Now()
	srv.Hub.now = func() time.Time { return now }

	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start"})
	if st, code := c.failure("POST", "/api/event", Event{Session: "s2", Kind: "session.start"}); st != http.StatusConflict || code != "account_busy" {
		t.Fatalf("a second session on the same account should get 409 account_busy, got %d %q", st, code)
	}
	if st, code := c.failure("POST", "/api/ask", AskRequest{Session: "s2", Kind: "permission"}); st != http.StatusConflict || code != "account_busy" {
		t.Fatalf("the refused session can't post an ask either: want 409 account_busy, got %d %q", st, code)
	}
	// heartbeats keep the account taken until the session goes stale
	now = now.Add(StaleAfter - time.Second)
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "heartbeat"})
	now = now.Add(StaleAfter - time.Second)
	if st, code := c.failure("POST", "/api/event", Event{Session: "s2", Kind: "session.start"}); st != http.StatusConflict || code != "account_busy" {
		t.Fatalf("a session that sends heartbeats keeps the account: want 409 account_busy, got %d %q", st, code)
	}
	// once it is stale, a new session takes over and the old one is off duty
	now = now.Add(2 * time.Second)
	c.ok("POST", "/api/event", Event{Session: "s2", Kind: "session.start"})
	if view(t, srv, acc.ID).Session != "s2" {
		t.Fatal("the account should now show s2")
	}
	if n := len(srv.Hub.Snapshot().Sessions); n != 1 {
		t.Fatalf("an account keeps one session record, found %d", n)
	}
	// another account can't touch this session
	other, _ := as(t, srv, base, "测试阿珍")
	if st, code := other.failure("POST", "/api/event", Event{Session: "s2", Kind: "turn.start"}); st != http.StatusForbidden || code != "ask_wrong_account" {
		t.Fatalf("a write from another account should get 403 ask_wrong_account, got %d %q", st, code)
	}
}

func TestReapStaleSessions(t *testing.T) {
	srv, base := newBridge(t)
	c, acc := as(t, srv, base, "运维大刘")
	now := time.Now()
	srv.Hub.now = func() time.Time { return now }
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start"})

	now = now.Add(StaleAfter - time.Second)
	srv.Hub.Reap()
	if session(t, srv, "s1").State == StateEnded {
		t.Fatal("not stale yet, so the session shouldn't be off duty")
	}
	now = now.Add(2 * time.Second)
	srv.Hub.Reap()
	if session(t, srv, "s1").State != StateEnded || view(t, srv, acc.ID).Session != "" {
		t.Fatal("a session silent past the timeout should be marked off duty")
	}
	// a lost session that comes back (say the computer wakes up) carries on
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "heartbeat"})
	if session(t, srv, "s1").State == StateEnded {
		t.Fatal("the returning session should be online again")
	}
}

func TestAccountsPersistAndSeats(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub()
	if err := hub.Open(dir); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, name := range []string{"甲", "乙", "丙"} {
		a, err := hub.CreateAccount(name)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, a.ID)
		if a.Seat != len(ids)-1 {
			t.Fatalf("%s should sit in seat %d, got %d", name, len(ids)-1, a.Seat)
		}
	}
	if _, err := hub.CreateAccount("乙"); err != ErrNameTaken {
		t.Fatalf("a duplicate name should be refused: %v", err)
	}
	if _, err := hub.CreateAccount(" "); err != ErrBadName {
		t.Fatalf("an empty name should be refused: %v", err)
	}
	if err := hub.DeleteAccount(ids[1]); err != nil {
		t.Fatal(err)
	}
	ding, _ := hub.CreateAccount("丁")
	if ding.Seat != 1 {
		t.Fatalf("the new account should take the freed seat 1, got %d", ding.Seat)
	}

	// a new Hub reading the same folder finds the accounts, seats and tokens
	again := NewHub()
	if err := again.Open(dir); err != nil {
		t.Fatal(err)
	}
	if a := again.AccountByToken(ding.Token); a == nil || a.Name != "丁" || a.Seat != 1 {
		t.Fatalf("wrong account after the restart: %+v", a)
	}
	if n := len(again.Snapshot().Accounts); n != 3 {
		t.Fatalf("want 3 accounts, got %d", n)
	}
	// the snapshot (pushed to the page) must not hold tokens
	data, _ := json.Marshal(again.Snapshot())
	if strings.Contains(string(data), ding.Token) {
		t.Fatal("the snapshot leaks a token")
	}
}

func TestAccountAPI(t *testing.T) {
	srv, base := newBridge(t)
	c := client{t: t, base: base}
	res := c.ok("POST", "/api/accounts", map[string]string{"name": "架构老张"})
	if res["token"] == "" || res["seat"] != float64(0) {
		t.Fatalf("creating an account should return its token and seat: %v", res)
	}
	// an error body has the English text and the code the page translates
	if st, body := c.do("POST", "/api/accounts", map[string]string{"name": "架构老张"}); st != http.StatusConflict ||
		body["code"] != "account_name_taken" || body["error"] != ErrNameTaken.Text {
		t.Fatalf("a duplicate name should get 409 account_name_taken, got %d %v", st, body)
	}
	id := res["id"].(string)
	mod := client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + res["token"].(string)}, modOnly: true}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start"})
	if st, code := c.failure("DELETE", "/api/accounts/"+id, nil); st != http.StatusConflict || code != "account_busy" {
		t.Fatalf("an online account can't be deleted: want 409 account_busy, got %d %q", st, code)
	}
	askID := mod.ok("POST", "/api/ask", AskRequest{Session: "s1", Kind: "question"})["id"].(string)
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.end"})
	c.ok("DELETE", "/api/accounts/"+id, nil)
	if n := len(srv.Hub.Snapshot().Asks); n != 0 {
		t.Fatalf("deleting the account should cancel its pending asks, %d left", n)
	}
	if st, _, _ := srv.Hub.Wait(askID, "", time.Millisecond, nil); st != AskCancelled {
		t.Fatalf("want cancelled, got %s", st)
	}
	if len(srv.Hub.Snapshot().Accounts) != 0 || len(srv.Hub.Snapshot().Sessions) != 0 {
		t.Fatal("the account and its session should be gone after the delete")
	}
	if st, code := mod.failure("POST", "/api/event", Event{Session: "s1", Kind: "turn.start"}); st != http.StatusUnauthorized || code != "token_invalid" {
		t.Fatalf("a deleted account's token should stop working: want 401 token_invalid, got %d %q", st, code)
	}
}

func TestSessionTitle(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "前端小王")
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.title", Title: "前端小王"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.title"}) // a new session's SessionStart carries no name
	if got := session(t, srv, "s1").Title; got != "前端小王" {
		t.Fatalf("the name was cleared: %q", got)
	}
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.title", Args: " 小王2号 "})
	if got := session(t, srv, "s1").Title; got != "小王2号" {
		t.Fatalf("/rename didn't take effect: %q", got)
	}
}

func TestAskWaitAnswer(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "a")
	web := client{t: t, base: base}

	id := c.ok("POST", "/api/ask", AskRequest{Session: "s2", Kind: "permission", Tool: "Bash", Input: map[string]any{"command": "ls"}})["id"].(string)
	if got := c.ok("GET", "/api/wait/"+id, nil)["status"]; got != AskPending {
		t.Fatalf("want pending, got %v", got)
	}
	if len(srv.Hub.Snapshot().Asks) != 1 || session(t, srv, "s2").State != StateWaiting {
		t.Fatal("want one pending ask and the session waiting")
	}

	srv.WaitTimeout = 10 * time.Second
	got := make(chan map[string]any)
	go func() { got <- c.ok("GET", "/api/wait/"+id, nil) }()
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	web.ok("POST", "/api/answer/"+id, map[string]any{"decision": "allow", "reason": "ok"})
	res := <-got
	if time.Since(start) > 2*time.Second {
		t.Fatal("the answer didn't wake the waiter in time")
	}
	answer := res["answer"].(map[string]any)
	if res["status"] != AskDone || answer["decision"] != "allow" || answer["reason"] != "ok" {
		t.Fatalf("wrong result: %v", res)
	}
	if len(srv.Hub.Snapshot().Asks) != 0 || session(t, srv, "s2").State == StateWaiting {
		t.Fatal("the answer should clear the pending ask")
	}
	if st, code := web.failure("POST", "/api/answer/"+id, map[string]any{}); st != http.StatusConflict || code != "ask_settled" {
		t.Fatalf("a second answer should get 409 ask_settled, got %d %q", st, code)
	}
	other, _ := as(t, srv, base, "b")
	id2 := c.ok("POST", "/api/ask", AskRequest{Session: "s2", Kind: "question"})["id"].(string)
	if st, code := other.failure("GET", "/api/wait/"+id2, nil); st != http.StatusForbidden || code != "ask_wrong_account" {
		t.Fatalf("another account waiting on this ask should get 403 ask_wrong_account, got %d %q", st, code)
	}
}

func TestCancel(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "a")
	id := c.ok("POST", "/api/ask", AskRequest{Session: "s3", Kind: "question", Questions: []any{}})["id"].(string)
	c.ok("POST", "/api/cancel/"+id, nil)
	if got := c.ok("GET", "/api/wait/"+id, nil)["status"]; got != AskCancelled {
		t.Fatalf("want cancelled, got %v", got)
	}
	if st, code := (client{t: t, base: base}).failure("POST", "/api/answer/"+id, map[string]any{}); st != http.StatusConflict || code != "ask_settled" {
		t.Fatalf("an answer after the cancel should get 409 ask_settled, got %d %q", st, code)
	}
	if st, code := c.failure("GET", "/api/wait/nope", nil); st != http.StatusNotFound || code != "ask_unknown" {
		t.Fatalf("an unknown ask should get 404 ask_unknown, got %d %q", st, code)
	}
}

func TestParseIncoming(t *testing.T) {
	from, body := parseIncoming("<cross-session-message from=\"uds:/run/user/1000/cc-socks/9126.sock\" from-name=\"xiaowang\" from-mode=\"prompting\">\n今晚八点开会。\n</cross-session-message>")
	if from != "xiaowang" || body != "今晚八点开会。" {
		t.Fatalf("wrong parse: %q %q", from, body)
	}
	from, body = parseIncoming("  随便一段话 ")
	if from != "" || body != "随便一段话" {
		t.Fatalf("wrong parse of plain text: %q %q", from, body)
	}
}

func TestMessagesPairEitherOrderAndPersist(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub()
	if err := hub.Open(dir); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(hub, testWeb)
	srv.Key = testKey
	ts := httptest.NewServer(srv)
	defer ts.Close()
	a, _ := as(t, srv, ts.URL, "xiaowang")
	b, _ := as(t, srv, ts.URL, "laoli")
	a.ok("POST", "/api/event", Event{Session: "A", Kind: "session.start"})
	b.ok("POST", "/api/event", Event{Session: "B", Kind: "session.start"})
	wrap := func(text string) string {
		return "<cross-session-message from=\"uds:/x.sock\" from-name=\"xiaowang\" from-mode=\"prompting\">\n" + text + "\n</cross-session-message>"
	}

	// the order seen in practice: the receiver reports first, then the sender
	b.ok("POST", "/api/event", Event{Session: "B", Kind: "message.receive", Text: wrap("今晚八点开会。"), Origin: map[string]any{"kind": "peer"}})
	a.ok("POST", "/api/event", Event{Session: "A", Kind: "message.send", To: "laoli", Text: "今晚八点开会。", Origin: "model", Delivered: true})
	// the other order pairs up too
	a.ok("POST", "/api/event", Event{Session: "A", Kind: "message.send", To: "laoli", Text: "收到请回复", Delivered: true})
	b.ok("POST", "/api/event", Event{Session: "B", Kind: "message.receive", Text: wrap("收到请回复")})

	msgs := hub.Snapshot().Messages
	if len(msgs) != 2 {
		t.Fatalf("want 2 paired messages, got %d: %+v", len(msgs), msgs)
	}
	for _, m := range msgs {
		if m.From != "A" || m.To != "B" || m.FromName != "xiaowang" || m.ToName != "laoli" || !m.Delivered || !m.Received {
			t.Fatalf("wrong message: %+v", m)
		}
	}
	// the log is on disk and survives a restart
	hub.Flush()
	again := NewHub()
	if err := again.Open(dir); err != nil {
		t.Fatal(err)
	}
	if n := len(again.Snapshot().Messages); n != 2 {
		t.Fatalf("want 2 messages after the restart, got %d", n)
	}
}

func TestStreamStartsWithSnapshot(t *testing.T) {
	srv, base := newBridge(t)
	resp, err := pageGet(base + "/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("content-type"); ct != "text/event-stream" {
		t.Fatalf("content-type: %q", ct)
	}
	r := bufio.NewReader(resp.Body)
	// the EventSource is told to come back a second after the stream ends (the hub lets a page go that fell behind)
	if retry, err := r.ReadString('\n'); err != nil || retry != "retry: 1000\n" {
		t.Fatalf("the stream should start with the reconnect delay: %q %v", retry, err)
	}
	if blank, _ := r.ReadString('\n'); blank != "\n" {
		t.Fatalf("then a blank line: %q", blank)
	}
	line, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, `data: {"type":"snapshot"`) {
		t.Fatalf("the first message should be a snapshot: %q %v", line, err)
	}
	c, _ := as(t, srv, base, "a")
	go c.ok("POST", "/api/event", Event{Session: "s9", Kind: "session.start"})
	got := make(chan string)
	go func() {
		for {
			l, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if strings.Contains(l, `"s9"`) {
				got <- l
				return
			}
		}
	}()
	select {
	case <-got:
	case <-time.After(3 * time.Second):
		t.Fatal("the event never arrived")
	}
}

func TestPages(t *testing.T) {
	_, base := newBridge(t)
	for path, want := range map[string]string{
		"/": "text/html", "/approve": "text/html", "/approve.js": "text/javascript", "/asks.js": "text/javascript", "/symphony.js": "text/javascript",
		"/i18n.js": "text/javascript", "/locales/en.json": "application/json", "/ui/chat.js": "text/javascript", "/ui/ui.css": "text/css",
	} {
		resp, err := pageGet(base + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("content-type"), want) {
			t.Fatalf("%s: %d %s", path, resp.StatusCode, resp.Header.Get("content-type"))
		}
	}
}

// TestThemes: / serves the theme picked with ?theme= or remembered in the cookie, else the default; /t/<name>/ picks
// one; a theme's files come from its folder only; /api/themes lists the themes with a manifest.
func TestThemes(t *testing.T) {
	srv, base := newBridge(t)
	get := func(path string, cookie string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest("GET", base+path, nil)
		req.Header.Set(keyHeader, testKey)
		if cookie != "" {
			req.Header.Set("Cookie", "symphony.theme="+cookie)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp, string(body)
	}
	cookieOf := func(resp *http.Response) string {
		for _, c := range resp.Cookies() {
			if c.Name == "symphony.theme" {
				return c.Value
			}
		}
		return ""
	}
	if names := func() (n []string) {
		for _, th := range srv.Themes() {
			n = append(n, th.Name)
		}
		return
	}(); strings.Join(names, ",") != "messenger,plain,zen" {
		t.Fatalf("themes: %v (the default first, then by name, a folder without a good manifest left out)", names)
	}
	if resp, body := get("/", ""); body != "<html>messenger</html>" || cookieOf(resp) != "" {
		t.Fatalf("/ without a cookie should be the default theme and set none: %q %q", body, cookieOf(resp))
	}
	if resp, body := get("/?theme=zen", ""); body != "<html>zen</html>" || cookieOf(resp) != "zen" {
		t.Fatalf("/?theme=zen should serve zen and remember it: %q %q", body, cookieOf(resp))
	}
	if _, body := get("/", "zen"); body != "<html>zen</html>" {
		t.Fatalf("/ with the cookie should serve the remembered theme: %q", body)
	}
	if _, body := get("/", "plain"); body != "<html>plain</html>" {
		t.Fatalf("/ with another theme remembered should serve it: %q", body)
	}
	if _, body := get("/", "nope"); body != "<html>messenger</html>" {
		t.Fatalf("an unknown theme in the cookie means the default: %q", body)
	}
	if _, body := get("/?theme=nope", "zen"); body != "<html>zen</html>" {
		t.Fatalf("an unknown ?theme= is ignored: %q", body)
	}
	if resp, body := get("/t/zen/", ""); body != "<html>zen</html>" || cookieOf(resp) != "zen" || !strings.HasPrefix(resp.Header.Get("content-type"), "text/html") {
		t.Fatalf("/t/zen/ should serve zen and remember it: %q %q", body, cookieOf(resp))
	}
	if resp, body := get("/t/zen/zen.css", ""); body != "body{}" || !strings.HasPrefix(resp.Header.Get("content-type"), "text/css") {
		t.Fatalf("/t/zen/zen.css: %q %s", body, resp.Header.Get("content-type"))
	}
	for _, path := range []string{"/t/zen/nope.css", "/t/zen/..%2Fplain%2Findex.html", "/t/zen/..%2F..%2Fasks.js", "/t/nope/x.css"} {
		if resp, _ := get(path, ""); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: want 404, got %d", path, resp.StatusCode)
		}
	}
	// a theme the bridge doesn't have (gone since, a link kept): / and the theme it serves, the query kept
	stay := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for path, to := range map[string]string{"/t/nope/": "/", "/t/bad/": "/", "/t/wrong/": "/", "/t/hq/?lang=ja": "/?lang=ja"} {
		req, _ := http.NewRequest("GET", base+path, nil)
		req.Header.Set(keyHeader, testKey)
		resp, err := stay.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != to {
			t.Errorf("%s: want 303 to %s, got %d %q", path, to, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	resp, body := get("/api/themes", "zen")
	var list struct {
		Themes  []Theme `json:"themes"`
		Current string  `json:"current"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil || resp.StatusCode != 200 {
		t.Fatalf("/api/themes: %d %q %v", resp.StatusCode, body, err)
	}
	if len(list.Themes) != 3 || list.Themes[2].Title["zh-CN"] != "禅" || list.Current != "zen" {
		t.Fatalf("/api/themes: %+v", list)
	}
}

// TestLocales checks that /locales/ serves the catalogs of the supported languages and nothing else.
func TestLocales(t *testing.T) {
	_, base := newBridge(t)
	get := func(path string) (int, string) {
		t.Helper()
		resp, err := pageGet(base + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	if st, body := get("/locales/en.json"); st != 200 || body != string(testWeb["locales/en.json"].Data) {
		t.Fatalf("/locales/en.json should serve the English catalog: %d %q", st, body)
	}
	for _, path := range []string{
		"/locales/xx.json",                 // the file exists, but xx is not a supported language
		"/locales/..%2Foffice.html",        // the encoded slash reaches the handler as ../office.html
		"/locales/..%2F..%2Faccounts.json", // and here as ../../accounts.json
	} {
		if st, _ := get(path); st != http.StatusNotFound {
			t.Errorf("%s: want 404, got %d", path, st)
		}
	}
}

func TestPruneEndedSessions(t *testing.T) {
	hub := NewHub()
	now := time.Now()
	hub.now = func() time.Time { return now }
	a, _ := hub.CreateAccount("a")
	b, _ := hub.CreateAccount("b")
	_ = hub.Apply(Event{Session: "old", Kind: "session.start"}, a)
	_ = hub.Apply(Event{Session: "old", Kind: "session.end"}, a)
	_ = hub.Apply(Event{Session: "live", Kind: "session.start"}, b)
	now = now.Add(endedTTL + time.Second)
	_ = hub.Apply(Event{Session: "live", Kind: "heartbeat"}, b)
	hub.Prune()
	snap := hub.Snapshot()
	if len(snap.Sessions) != 1 || snap.Sessions[0].ID != "live" || len(snap.Accounts) != 2 {
		t.Fatalf("want only the live session, and both accounts: %+v %+v", snap.Sessions, snap.Accounts)
	}
}

func TestUnknownSessionEndIgnored(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "a")
	c.ok("POST", "/api/event", Event{Session: "ghost", Kind: "session.end"})
	if n := len(srv.Hub.Snapshot().Sessions); n != 0 {
		t.Fatalf("an unknown session going off duty shouldn't create a session, found %d", n)
	}
}

func TestRecoverAfterRestart(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "a")
	// after a bridge restart the first event isn't session.start; the folder and the launch id still come back
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "heartbeat", Cwd: "/w/a", Launch: "abc"})
	if s := session(t, srv, "s1"); s.Cwd != "/w/a" || s.Launch != "abc" || s.State != StateIdle {
		t.Fatalf("not restored: %+v", s)
	}
	// with page launches on, the launch id tells where to find the session's terminal
	srv.SetLauncher(&Launcher{Term: NewPTYTerminal()})
	c2, _ := as(t, srv, base, "b")
	c2.ok("POST", "/api/event", Event{Session: "s2", Kind: "heartbeat", Launch: "def"})
	if s := session(t, srv, "s2"); s.Attach != "symphony attach def" {
		t.Fatalf("no attach command derived from the launch id: %+v", s)
	}
	if dirs := srv.Hub.recentDirs(8); len(dirs) != 1 {
		t.Fatalf("recent folders: %v", dirs)
	}
	if dirs := NewHub().recentDirs(8); dirs == nil {
		t.Fatal("with no recent folders the list should be empty, not nil")
	}
}
