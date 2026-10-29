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
	"office.html":  {Data: []byte("<html>office</html>")},
	"approve.html": {Data: []byte("<html>approve</html>")},
	"asks.js":      {Data: []byte("export const x = 1")},
}

type client struct {
	t      *testing.T
	base   string
	header map[string]string
}

func newBridge(t *testing.T) (*Server, string) {
	t.Helper()
	srv := NewServer(NewHub(), testWeb)
	srv.WaitTimeout = 300 * time.Millisecond
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return srv, ts.URL
}

// as 建一个账号，返回带着它 token 的客户端。
func as(t *testing.T, srv *Server, base, name string) (client, *Account) {
	t.Helper()
	acc, err := srv.Hub.CreateAccount(name)
	if err != nil {
		t.Fatal(err)
	}
	return client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + acc.Token}}, acc
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

func session(t *testing.T, srv *Server, id string) *Session {
	t.Helper()
	for _, s := range srv.Hub.Snapshot().Sessions {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("没有会话 %s", id)
	return nil
}

func view(t *testing.T, srv *Server, id string) AccountView {
	t.Helper()
	for _, a := range srv.Hub.Snapshot().Accounts {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("没有账号 %s", id)
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
		t.Fatalf("状态不对: %+v", s)
	}
	if view(t, srv, acc.ID).Session != "s1" {
		t.Fatal("账号应显示在线会话")
	}

	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "tool.start", Tool: "Read", AgentID: "ag1"})
	if a := session(t, srv, "s1").Agents["ag1"]; a == nil || a.Tool != "Read" || a.State != StateTool {
		t.Fatalf("子代理不对: %+v", a)
	}
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "turn.complete", AgentID: "ag1"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "turn.complete"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.model", Model: "claude-sonnet-5-5", Effort: "high"})
	s = session(t, srv, "s1")
	if s.State != StateIdle || s.Agents["ag1"].State != StateDone || s.Tools != 2 || s.Model != "claude-sonnet-5-5" || s.Effort != "high" {
		t.Fatalf("回合结束后状态不对: %+v", s)
	}
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.end"})
	if session(t, srv, "s1").State != StateEnded || view(t, srv, acc.ID).Session != "" {
		t.Fatal("会话应已下班，账号不在线")
	}
}

func TestStateSince(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "a")
	now := time.Now()
	srv.Hub.now = func() time.Time { return now }
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start"})
	idleAt := session(t, srv, "s1").StateSince
	// 心跳和不改状态的事件不动 StateSince（页面靠它算闲了多久）
	now = now.Add(time.Minute)
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "heartbeat"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.title", Title: "x"})
	if got := session(t, srv, "s1").StateSince; !got.Equal(idleAt) {
		t.Fatalf("状态没变，StateSince 不该动: %v -> %v", idleAt, got)
	}
	// 推给页面的快照里也得是新时间（不能先推后改）
	ch, stop := srv.Hub.Subscribe()
	defer stop()
	<-ch
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "turn.start"})
	if got := session(t, srv, "s1").StateSince; !got.Equal(now) {
		t.Fatalf("状态变了，StateSince 应更新: %v", got)
	}
	for msg := range ch {
		if snap, ok := msg.(Snapshot); ok {
			if !snap.Sessions[0].StateSince.Equal(now) {
				t.Fatalf("推出去的快照里 StateSince 是旧的: %v", snap.Sessions[0].StateSince)
			}
			break
		}
	}
}

func TestTokenRequired(t *testing.T) {
	srv, base := newBridge(t)
	anon := client{t: t, base: base}
	bad := client{t: t, base: base, header: map[string]string{"authorization": "Bearer nope"}}
	for _, c := range []client{anon, bad} {
		if st := c.status("POST", "/api/event", Event{Session: "s1", Kind: "session.start"}); st != http.StatusUnauthorized {
			t.Fatalf("没 token / 错 token 应 401，得到 %d", st)
		}
	}
	if n := len(srv.Hub.Snapshot().Sessions); n != 0 {
		t.Fatalf("不该建会话: %d", n)
	}
}

func TestOneSessionPerAccount(t *testing.T) {
	srv, base := newBridge(t)
	c, acc := as(t, srv, base, "后端老李")
	now := time.Now()
	srv.Hub.now = func() time.Time { return now }

	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start"})
	if st := c.status("POST", "/api/event", Event{Session: "s2", Kind: "session.start"}); st != http.StatusConflict {
		t.Fatalf("同一账号第二个会话应 409，得到 %d", st)
	}
	if st := c.status("POST", "/api/ask", AskRequest{Session: "s2", Kind: "permission"}); st != http.StatusConflict {
		t.Fatalf("被拒的会话也不能挂审批，得到 %d", st)
	}
	// 心跳续命：没超时就还是占着
	now = now.Add(StaleAfter - time.Second)
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "heartbeat"})
	now = now.Add(StaleAfter - time.Second)
	if st := c.status("POST", "/api/event", Event{Session: "s2", Kind: "session.start"}); st != http.StatusConflict {
		t.Fatalf("有心跳就不该让位，得到 %d", st)
	}
	// 失联超时后，新会话可以接手，老的算下班
	now = now.Add(2 * time.Second)
	c.ok("POST", "/api/event", Event{Session: "s2", Kind: "session.start"})
	if view(t, srv, acc.ID).Session != "s2" {
		t.Fatal("账号应换成 s2")
	}
	if n := len(srv.Hub.Snapshot().Sessions); n != 1 {
		t.Fatalf("一个账号只留一个会话记录，现在 %d 个", n)
	}
	// 别的账号不能动这个会话
	other, _ := as(t, srv, base, "测试阿珍")
	if st := other.status("POST", "/api/event", Event{Session: "s2", Kind: "turn.start"}); st != http.StatusForbidden {
		t.Fatalf("跨账号写应 403，得到 %d", st)
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
		t.Fatal("还没超时不该下班")
	}
	now = now.Add(2 * time.Second)
	srv.Hub.Reap()
	if session(t, srv, "s1").State != StateEnded || view(t, srv, acc.ID).Session != "" {
		t.Fatal("超时没动静应标成下班")
	}
	// 失联的会话又回来了（比如电脑睡醒），接着用
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "heartbeat"})
	if session(t, srv, "s1").State == StateEnded {
		t.Fatal("回来的会话应重新上线")
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
			t.Fatalf("%s 座位应为 %d，得到 %d", name, len(ids)-1, a.Seat)
		}
	}
	if _, err := hub.CreateAccount("乙"); err != ErrNameTaken {
		t.Fatalf("重名应拒绝: %v", err)
	}
	if _, err := hub.CreateAccount(" "); err != ErrBadName {
		t.Fatalf("空名字应拒绝: %v", err)
	}
	if err := hub.DeleteAccount(ids[1]); err != nil {
		t.Fatal(err)
	}
	ding, _ := hub.CreateAccount("丁")
	if ding.Seat != 1 {
		t.Fatalf("新账号应坐空出来的 1 号座位，得到 %d", ding.Seat)
	}

	// 换一个 Hub 从同一目录读，账号、座位、token 都在
	again := NewHub()
	if err := again.Open(dir); err != nil {
		t.Fatal(err)
	}
	if a := again.AccountByToken(ding.Token); a == nil || a.Name != "丁" || a.Seat != 1 {
		t.Fatalf("重启后账号不对: %+v", a)
	}
	if n := len(again.Snapshot().Accounts); n != 3 {
		t.Fatalf("应有 3 个账号，得到 %d", n)
	}
	// 快照（会推给页面）里不能有 token
	data, _ := json.Marshal(again.Snapshot())
	if strings.Contains(string(data), ding.Token) {
		t.Fatal("快照里露出了 token")
	}
}

func TestAccountAPI(t *testing.T) {
	srv, base := newBridge(t)
	c := client{t: t, base: base}
	res := c.ok("POST", "/api/accounts", map[string]string{"name": "架构老张"})
	if res["token"] == "" || res["seat"] != float64(0) {
		t.Fatalf("建账号应返回 token 和座位: %v", res)
	}
	if st := c.status("POST", "/api/accounts", map[string]string{"name": "架构老张"}); st != http.StatusConflict {
		t.Fatalf("重名应 409，得到 %d", st)
	}
	id := res["id"].(string)
	mod := client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + res["token"].(string)}}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start"})
	if st := c.status("DELETE", "/api/accounts/"+id, nil); st != http.StatusConflict {
		t.Fatalf("在线的账号不能删，得到 %d", st)
	}
	askID := mod.ok("POST", "/api/ask", AskRequest{Session: "s1", Kind: "question"})["id"].(string)
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.end"})
	c.ok("DELETE", "/api/accounts/"+id, nil)
	if n := len(srv.Hub.Snapshot().Asks); n != 0 {
		t.Fatalf("删账号时它的待处理事项也该撤掉，还剩 %d 个", n)
	}
	if st, _, _ := srv.Hub.Wait(askID, "", time.Millisecond, nil); st != AskCancelled {
		t.Fatalf("应为 cancelled: %s", st)
	}
	if len(srv.Hub.Snapshot().Accounts) != 0 || len(srv.Hub.Snapshot().Sessions) != 0 {
		t.Fatal("删账号后账号和会话都该没了")
	}
	if st := mod.status("POST", "/api/event", Event{Session: "s1", Kind: "turn.start"}); st != http.StatusUnauthorized {
		t.Fatalf("删掉的账号 token 应失效，得到 %d", st)
	}
}

func TestSessionTitle(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "前端小王")
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.title", Title: "前端小王"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.title"}) // 新会话的 SessionStart 不带名字
	if got := session(t, srv, "s1").Title; got != "前端小王" {
		t.Fatalf("名字被清掉了: %q", got)
	}
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.title", Args: " 小王2号 "})
	if got := session(t, srv, "s1").Title; got != "小王2号" {
		t.Fatalf("/rename 没生效: %q", got)
	}
}

func TestAskWaitAnswer(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "a")
	web := client{t: t, base: base}

	id := c.ok("POST", "/api/ask", AskRequest{Session: "s2", Kind: "permission", Tool: "Bash", Input: map[string]any{"command": "ls"}})["id"].(string)
	if got := c.ok("GET", "/api/wait/"+id, nil)["status"]; got != AskPending {
		t.Fatalf("应为 pending: %v", got)
	}
	if len(srv.Hub.Snapshot().Asks) != 1 || session(t, srv, "s2").State != StateWaiting {
		t.Fatal("应有一个待决事项且会话在等人")
	}

	srv.WaitTimeout = 10 * time.Second
	got := make(chan map[string]any)
	go func() { got <- c.ok("GET", "/api/wait/"+id, nil) }()
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	web.ok("POST", "/api/answer/"+id, map[string]any{"decision": "allow", "reason": "ok"})
	res := <-got
	if time.Since(start) > 2*time.Second {
		t.Fatal("回答后没有及时唤醒")
	}
	answer := res["answer"].(map[string]any)
	if res["status"] != AskDone || answer["decision"] != "allow" || answer["reason"] != "ok" {
		t.Fatalf("结果不对: %v", res)
	}
	if len(srv.Hub.Snapshot().Asks) != 0 || session(t, srv, "s2").State == StateWaiting {
		t.Fatal("回答后应清掉待决事项")
	}
	if st := web.status("POST", "/api/answer/"+id, map[string]any{}); st != http.StatusConflict {
		t.Fatalf("重复回答应 409，得到 %d", st)
	}
	other, _ := as(t, srv, base, "b")
	id2 := c.ok("POST", "/api/ask", AskRequest{Session: "s2", Kind: "question"})["id"].(string)
	if st := other.status("GET", "/api/wait/"+id2, nil); st != http.StatusForbidden {
		t.Fatalf("别的账号等这个事项应 403，得到 %d", st)
	}
}

func TestCancel(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "a")
	id := c.ok("POST", "/api/ask", AskRequest{Session: "s3", Kind: "question", Questions: []any{}})["id"].(string)
	c.ok("POST", "/api/cancel/"+id, nil)
	if got := c.ok("GET", "/api/wait/"+id, nil)["status"]; got != AskCancelled {
		t.Fatalf("应为 cancelled: %v", got)
	}
	if st := c.status("POST", "/api/answer/"+id, map[string]any{}); st != http.StatusConflict {
		t.Fatalf("撤销后回答应 409，得到 %d", st)
	}
	if st := c.status("GET", "/api/wait/nope", nil); st != http.StatusNotFound {
		t.Fatalf("不存在的事项应 404，得到 %d", st)
	}
}

func TestParseIncoming(t *testing.T) {
	from, body := parseIncoming("<cross-session-message from=\"uds:/run/user/1000/cc-socks/9126.sock\" from-name=\"xiaowang\" from-mode=\"prompting\">\n今晚八点开会。\n</cross-session-message>")
	if from != "xiaowang" || body != "今晚八点开会。" {
		t.Fatalf("解析不对: %q %q", from, body)
	}
	from, body = parseIncoming("  随便一段话 ")
	if from != "" || body != "随便一段话" {
		t.Fatalf("普通文本解析不对: %q %q", from, body)
	}
}

func TestMessagesPairEitherOrderAndPersist(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub()
	if err := hub.Open(dir); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(hub, testWeb)
	ts := httptest.NewServer(srv)
	defer ts.Close()
	a, _ := as(t, srv, ts.URL, "xiaowang")
	b, _ := as(t, srv, ts.URL, "laoli")
	a.ok("POST", "/api/event", Event{Session: "A", Kind: "session.start"})
	b.ok("POST", "/api/event", Event{Session: "B", Kind: "session.start"})
	wrap := func(text string) string {
		return "<cross-session-message from=\"uds:/x.sock\" from-name=\"xiaowang\" from-mode=\"prompting\">\n" + text + "\n</cross-session-message>"
	}

	// 实测顺序：收件方先报，发件方后报
	b.ok("POST", "/api/event", Event{Session: "B", Kind: "message.receive", Text: wrap("今晚八点开会。"), Origin: map[string]any{"kind": "peer"}})
	a.ok("POST", "/api/event", Event{Session: "A", Kind: "message.send", To: "laoli", Text: "今晚八点开会。", Origin: "model", Delivered: true})
	// 反过来的顺序也要配上
	a.ok("POST", "/api/event", Event{Session: "A", Kind: "message.send", To: "laoli", Text: "收到请回复", Delivered: true})
	b.ok("POST", "/api/event", Event{Session: "B", Kind: "message.receive", Text: wrap("收到请回复")})

	msgs := hub.Snapshot().Messages
	if len(msgs) != 2 {
		t.Fatalf("应配成 2 条消息，得到 %d: %+v", len(msgs), msgs)
	}
	for _, m := range msgs {
		if m.From != "A" || m.To != "B" || m.FromName != "xiaowang" || m.ToName != "laoli" || !m.Delivered || !m.Received {
			t.Fatalf("消息不对: %+v", m)
		}
	}
	// 记录落盘，重启还在
	again := NewHub()
	if err := again.Open(dir); err != nil {
		t.Fatal(err)
	}
	if n := len(again.Snapshot().Messages); n != 2 {
		t.Fatalf("重启后应还有 2 条消息，得到 %d", n)
	}
}

func TestStreamStartsWithSnapshot(t *testing.T) {
	srv, base := newBridge(t)
	resp, err := http.Get(base + "/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("content-type"); ct != "text/event-stream" {
		t.Fatalf("content-type: %q", ct)
	}
	r := bufio.NewReader(resp.Body)
	line, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, `data: {"type":"snapshot"`) {
		t.Fatalf("第一条应是快照: %q %v", line, err)
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
		t.Fatal("事件没推过来")
	}
}

func TestPages(t *testing.T) {
	_, base := newBridge(t)
	for path, want := range map[string]string{"/": "text/html", "/approve": "text/html", "/asks.js": "text/javascript"} {
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("content-type"), want) {
			t.Fatalf("%s: %d %s", path, resp.StatusCode, resp.Header.Get("content-type"))
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
		t.Fatalf("应只剩 live 会话，账号都在: %+v %+v", snap.Sessions, snap.Accounts)
	}
}

func TestUnknownSessionEndIgnored(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "a")
	c.ok("POST", "/api/event", Event{Session: "ghost", Kind: "session.end"})
	if n := len(srv.Hub.Snapshot().Sessions); n != 0 {
		t.Fatalf("不认识的会话下班不该建会话，现在有 %d 个", n)
	}
}

func TestRecoverAfterRestart(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "a")
	// bridge 重启后第一条不是 session.start，也能补回目录和 tmux
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "heartbeat", Cwd: "/w/a", Launch: "abc"})
	if s := session(t, srv, "s1"); s.Cwd != "/w/a" || s.Launch != "abc" || s.State != StateIdle {
		t.Fatalf("没补回来: %+v", s)
	}
	// 开了页面启动时，按启动编号推回“去哪儿找终端”
	srv.SetLauncher(&Launcher{Term: &tmuxTerminal{}})
	c2, _ := as(t, srv, base, "b")
	c2.ok("POST", "/api/event", Event{Session: "s2", Kind: "heartbeat", Launch: "def"})
	if s := session(t, srv, "s2"); s.Attach != "tmux attach -t sym-def" {
		t.Fatalf("没推回 tmux 命令: %+v", s)
	}
	if dirs := srv.Hub.recentDirs(8); len(dirs) != 1 {
		t.Fatalf("最近目录: %v", dirs)
	}
	if dirs := NewHub().recentDirs(8); dirs == nil {
		t.Fatal("没有最近目录时也要是空列表，不是 nil")
	}
}
