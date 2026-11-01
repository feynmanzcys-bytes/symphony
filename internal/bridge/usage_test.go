package bridge

import (
	"bufio"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSessionDetails: what the mod reports besides the state and the chat lands on the session: the engine, the mod's
// version, the repository, the surfaces, the tools, the background work, the agent team's tasks, a subagent's name.
func TestSessionDetails(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "a")
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Cwd: "/w", Surface: "terminal", Mod: "0.8.0",
		Engine: &EngineInfo{Version: "2.1.316", Base: "2.1.316"}, Repo: &RepoInfo{Root: "/w", Remote: "git@x:y/z.git", Name: "z"}})
	s := session(t, srv, "s1")
	if s.Engine == nil || s.Engine.Version != "2.1.316" || s.Mod != "0.8.0" || s.Surface != "terminal" || s.Repo == nil || s.Repo.Name != "z" {
		t.Fatalf("the start's details are missing: %+v", s)
	}
	snap := srv.Hub.Snapshot()
	if snap.Protocol != Protocol || snap.Started.IsZero() || snap.Ledger == nil {
		t.Fatalf("the snapshot should carry the protocol, the start time and the ledger: %+v", snap)
	}

	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.surfaces", Surfaces: []string{"terminal", "mobile"}})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.tools", ToolCount: 42, MCP: []string{"github", "slack"}})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.background", BgTasks: 2, BgCrons: 1})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.cwd", Cwd: "/w/sub"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "todo.create", Todo: "1", Subject: "write tests", Teammate: "bob"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "todo.create", Todo: "2", Subject: "ship"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "todo.done", Todo: "1"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.model", Model: "claude-opus-5-5", Effort: "max"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "agent.start", AgentID: "ag1", AgentType: "Explore", Description: "find the tests", Model: "claude-haiku-5-5", Background: true})
	s = session(t, srv, "s1")
	if s.Model != "claude-opus-5-5" || s.Effort != "max" {
		t.Fatalf("a subagent's model must not become the session's: %s %s", s.Model, s.Effort)
	}
	if len(s.Surfaces) != 2 || s.ToolCount != 42 || strings.Join(s.MCP, ",") != "github,slack" || s.BgTasks != 2 || s.BgCrons != 1 || s.Cwd != "/w/sub" {
		t.Fatalf("details: %+v", s)
	}
	if len(s.Todos) != 1 || s.Todos[0].ID != "2" || s.Todos[0].Subject != "ship" {
		t.Fatalf("todos: %+v", s.Todos)
	}
	if a := s.Agents["ag1"]; a == nil || a.Type != "Explore" || a.Description != "find the tests" || a.Model != "claude-haiku-5-5" || !a.Background || a.Started == nil {
		t.Fatalf("the subagent's details are missing: %+v", a)
	}
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "agent.end", AgentID: "ag1", AgentType: "Explore"})
	if a := session(t, srv, "s1").Agents["ag1"]; a.State != StateDone {
		t.Fatalf("agent.end should mark the subagent done: %+v", a)
	}
	// an older mod sends none of it: nothing breaks, nothing is invented
	c2, _ := as(t, srv, base, "b")
	c2.ok("POST", "/api/event", Event{Session: "s2", Kind: "session.start", Cwd: "/v"})
	if s := session(t, srv, "s2"); s.Engine != nil || s.Usage != nil || s.Todos != nil || s.Mod != "" {
		t.Fatalf("an older mod's session should carry none of the details: %+v", s)
	}
}

// TestRemoteWithoutUserinfo: an older mod reports the repository's remote as git has it, a token in it maybe; neither
// the session nor the event pushed to the pages keeps the user name and password.
func TestRemoteWithoutUserinfo(t *testing.T) {
	for remote, want := range map[string]string{
		"https://x-access-token:ghp_secret@github.com/o/r.git": "https://github.com/o/r.git",
		"https://ghp_secret@github.com/o/r":                    "https://github.com/o/r",
		"https://user:p%40ss%2Fw@host.example/o/r.git":         "https://host.example/o/r.git",
		"https://user:p@ss@host.example/o/r.git":               "https://host.example/o/r.git",
		"https://user:p#ss@host.example:8443/o/r.git":          "https://host.example:8443/o/r.git",
		"ssh://git@github.com:22/o/r.git":                      "ssh://github.com:22/o/r.git",
		"git@github.com:o/r.git":                               "github.com:o/r.git",
		"user:secret@host:o/r.git":                             "host:o/r.git",
		"https://github.com/o/r.git":                           "https://github.com/o/r.git",
		"https://github.com/o/r@v2.git":                        "https://github.com/o/r@v2.git",
		"github.com:o/r@v2.git":                                "github.com:o/r@v2.git",
		"file:///home/me@corp/r":                               "file:///home/me@corp/r",
		"/home/me@corp/r":                                      "/home/me@corp/r",
		`C:\Users\me@corp\r`:                                   `C:\Users\me@corp\r`,
		"":                                                     "",
	} {
		if got := withoutUserinfo(remote); got != want {
			t.Errorf("withoutUserinfo(%q) = %q, want %q", remote, got, want)
		}
	}

	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "a")
	ch, stop := srv.Hub.Subscribe()
	defer stop()
	<-ch
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Repo: &RepoInfo{Root: "/w", Remote: "https://bob:ghp_secret@github.com/o/r.git", Name: "r"}})
	if s := session(t, srv, "s1"); s.Repo == nil || s.Repo.Remote != "https://github.com/o/r.git" || s.Repo.Name != "r" || s.Repo.Root != "/w" {
		t.Fatalf("the session should keep the remote without its credentials: %+v", s.Repo)
	}
	for range 2 { // the event, then the snapshot
		if data := string((<-ch).JSON()); strings.Contains(data, "ghp_secret") || strings.Contains(data, "bob") || !strings.Contains(data, "https://github.com/o/r.git") {
			t.Fatalf("a push should carry the remote without its credentials: %s", data)
		}
	}
}

// TestUsageAndLedger: the usage figures update the session and reach the pages as a "usage" push without a snapshot;
// the tokens of every response add up by model; the cost and the tokens go to the account's row of today's ledger,
// which is on disk and comes back after a restart.
func TestUsageAndLedger(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub()
	if err := hub.Open(dir); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(hub, testWeb)
	srv.Key = testKey
	server := httptest.NewServer(srv)
	t.Cleanup(server.Close)
	ts := server.URL
	c, acc := as(t, srv, ts, "a")
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Cwd: "/w", Model: "claude-opus-5-5"})

	// subscribe, then watch what the usage report pushes
	resp, err := pageGet(ts + "/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	r := bufio.NewReader(resp.Body)
	for { // the reconnect delay, then the first snapshot
		l, err := r.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(l, "data: ") {
			break
		}
	}
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.usage", Usage: &Usage{
		Context: &ContextUsage{Tokens: 40000, Window: 200000, Percent: 20}, RateLimits: []RateLimit{{Kind: "five_hour", PercentUsed: 12}}, Cost: &Cost{USD: 0.5}}})
	lines := make(chan string, 4)
	go func() {
		for {
			l, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if strings.TrimSpace(l) != "" {
				lines <- l
			}
		}
	}()
	select {
	case l := <-lines:
		if !strings.HasPrefix(l, `data: {`) || !strings.Contains(l, `"type":"usage"`) || !strings.Contains(l, `"session":"s1"`) || !strings.Contains(l, `"percent":20`) {
			t.Fatalf("expected a usage push, got %s", l)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no usage push")
	}
	select {
	case l := <-lines:
		t.Fatalf("a usage report must not push a snapshot: %s", l)
	case <-time.After(200 * time.Millisecond):
	}
	s := session(t, srv, "s1")
	if s.Usage == nil || s.Usage.Context.Percent != 20 || len(s.Usage.RateLimits) != 1 || s.Usage.Cost.USD != 0.5 || s.Usage.Updated.IsZero() {
		t.Fatalf("usage: %+v", s.Usage)
	}

	// two responses: their tokens add up by model (the first names no model: the session's)
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "chat.stop", TurnID: "t1", Step: 0, StopReason: "tool_use", Tokens: &Tokens{Input: 100, Output: 20, CacheRead: 1000}})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "chat.stop", TurnID: "t1", Step: 1, StopReason: "end_turn", Tokens: &Tokens{Input: 50, Output: 30, Model: "claude-opus-5-5"}})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "chat.stop", TurnID: "t1", Step: 2, Tokens: &Tokens{Input: 5, Output: 5, Model: "claude-haiku-5-5"}})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "turn.complete", TurnID: "t1", Reason: "answer"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "tool.start", Tool: "Bash", ToolUseID: "tu1"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "tool.end", Tool: "Bash", ToolUseID: "tu1", DurationMs: 1500})
	s = session(t, srv, "s1")
	opus := s.Usage.Totals["claude-opus-5-5"]
	if opus == nil || opus.Input != 150 || opus.Output != 50 || opus.CacheRead != 1000 || s.Usage.Totals["claude-haiku-5-5"].Input != 5 {
		t.Fatalf("totals: %+v", s.Usage.Totals)
	}
	if s.Turns != 1 || s.ToolMs != 1500 {
		t.Fatalf("turns and tool time: %+v", s)
	}
	// the cost is the session's total so far: only the difference goes to the ledger
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.usage", Usage: &Usage{Cost: &Cost{USD: 0.75}}})
	rows := srv.Hub.Snapshot().Ledger
	if len(rows) != 1 || rows[0].Account != acc.ID || rows[0].USD != 0.75 || rows[0].Tokens.Input != 155 || rows[0].Turns != 1 || rows[0].Tools != 1 {
		t.Fatalf("today's ledger: %+v", rows)
	}
	if rows[0].Day != time.Now().Format("2006-01-02") {
		t.Fatalf("the row's day: %s", rows[0].Day)
	}
	// the usage push carries the totals too
	if items := (client{t: t, base: ts}).ok("GET", "/api/ledger", nil); len(items["rows"].([]any)) != 1 {
		t.Fatalf("/api/ledger: %v", items)
	}
	// on disk, and back after a restart
	hub.Flush()
	if _, err := os.Stat(filepath.Join(dir, "ledger.json")); err != nil {
		t.Fatal(err)
	}
	hub2 := NewHub()
	if err := hub2.Open(dir); err != nil {
		t.Fatal(err)
	}
	if rows := hub2.Ledger(); len(rows) != 1 || rows[0].USD != 0.75 {
		t.Fatalf("the ledger should survive a restart: %+v", rows)
	}
	// a page that falls behind the usage pushes still gets the figures from the snapshot
	for _, cp := range srv.Hub.Snapshot().Sessions {
		if cp.ID == "s1" && (cp.Usage == nil || cp.Usage.Totals["claude-opus-5-5"].Input != 150) {
			t.Fatalf("the snapshot's session lacks its usage: %+v", cp.Usage)
		}
	}
}

// TestChatDetails: the chat gets the new items and details: a slash command, a tool's duration and the reason it
// failed (either side first), a refusal, an API error, a compaction, a model switch, a notification (but not the idle
// one), and why the session ended.
func TestChatDetails(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "a")
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Cwd: "/w", Model: "claude-sonnet-5-5"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "command.run", Command: "compact", Args: "keep the API"})
	// a command typed as a prompt is already a bubble: no row for it
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "chat.user", Text: "/rename bob", Origin: "composer", Command: "rename"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "command.run", Command: "rename", Args: "bob"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "tool.start", Tool: "Bash", ToolUseID: "tu1", Summary: "rm -rf build"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "tool.fail", Tool: "Bash", ToolUseID: "tu1", Text: "denied by a rule", Denied: true})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "tool.end", Tool: "Bash", ToolUseID: "tu1", IsError: true, DurationMs: 12})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "tool.start", Tool: "Read", ToolUseID: "tu2", Summary: "/x"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "tool.end", Tool: "Read", ToolUseID: "tu2", IsError: true, DurationMs: 3})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "tool.fail", Tool: "Read", ToolUseID: "tu2", Text: "no such file"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "turn.complete", TurnID: "t1", Reason: "refusal", Refusal: &Refusal{Category: "harm", Explanation: "no"}})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "turn.error", Text: "rate_limit", Details: "try later"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "turn.complete", TurnID: "t2", Reason: "error"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.compact", Trigger: "auto"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.model", Model: "claude-opus-5-5", From: "claude-sonnet-5-5", Source: "command"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.notify", NotifyType: "idle_prompt", Text: "waiting for your input"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.notify", NotifyType: "auth_success", Title: "Signed in", Text: "as bob"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.end", Reason: "clear"})

	items := srv.Hub.Chat("s1")
	var got []string
	for _, it := range items {
		// a tool row's outcome is its status; a command or a notice is just done
		status := it.Status
		if it.Kind != ChatTool && it.Done {
			status = "done"
		}
		got = append(got, it.Kind+":"+it.Text+"|"+status+"|"+it.Summary)
	}
	want := []string{
		"command:/compact|done|keep the API",
		"user:/rename bob|done|",
		"tool:denied by a rule|denied|rm -rf build",
		"tool:no such file|error|/x",
		"notice:refused|done|harm no",
		"notice:error|done|rate_limit try later",
		"notice:compacted|done|",
		"notice:modelSwitched|done|claude-sonnet-5-5 → claude-opus-5-5",
		"notice:notify|done|Signed in as bob",
		"notice:ended|done|",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("chat:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if items[2].Ms != 12 || items[3].Ms != 3 {
		t.Fatalf("tool durations: %d %d", items[2].Ms, items[3].Ms)
	}
	s := session(t, srv, "s1")
	if s.Compactions != 1 || s.LastError != "rate_limit try later" || s.EndReason != "clear" || s.Notify == nil || s.Notify.Type != "auth_success" || s.Model != "claude-opus-5-5" {
		t.Fatalf("session: %+v", s)
	}
	if s.Turns != 2 {
		t.Fatalf("turns: %d", s.Turns)
	}
}

// TestActivity: the hub keeps a log of what happened, for a page that opens late.
func TestActivity(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "ann")
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Cwd: "/w"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "tool.start", Tool: "Bash", ToolUseID: "tu1", Summary: "go test ./..."})
	res := c.ok("POST", "/api/ask", AskRequest{Session: "s1", Kind: "permission", Tool: "Bash", Input: map[string]any{"command": "rm x"}})
	id := res["id"].(string)
	page := client{t: t, base: base}
	page.ok("POST", "/api/answer/"+id, map[string]any{"decision": "deny", "reason": "no"})
	// a message between sessions is one line, whichever half the bridge hears first
	c2, _ := as(t, srv, base, "bob")
	c2.ok("POST", "/api/event", Event{Session: "s2", Kind: "session.start", Cwd: "/v"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "message.send", To: "bob", Text: "hi", Delivered: true})
	c2.ok("POST", "/api/event", Event{Session: "s2", Kind: "message.receive", Text: `<cross-session-message from-name="ann">hi</cross-session-message>`})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "turn.error", Text: "overloaded"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.end", Reason: "logout"})
	srv.Hub.addLaunch(&Launch{ID: "L1", Name: "bob", Dir: "/b", Status: LaunchStarting})
	srv.Hub.failLaunch("L1", newError("launch_process_exited", "it exited"))

	var kinds, texts []string
	for _, a := range srv.Hub.Activity() {
		kinds = append(kinds, a.Kind)
		texts = append(texts, a.Name+":"+a.Text)
	}
	if strings.Join(kinds, " ") != "start tool ask settled start message error end launch launchFailed" {
		t.Fatalf("activity kinds: %v\n%v", kinds, texts)
	}
	if texts[1] != "ann:Bash go test ./..." || texts[2] != "ann:Bash rm x" || texts[3] != "ann:deny Bash" || texts[5] != "ann:ann → bob: hi" || texts[7] != "ann:logout" || texts[8] != ":bob /b" {
		t.Fatalf("activity texts: %v", texts)
	}
	if out := page.ok("GET", "/api/activity", nil); len(out["items"].([]any)) != 10 {
		t.Fatalf("/api/activity: %v", out)
	}
	// the log is bounded
	for i := 0; i < keepActivity+10; i++ {
		srv.Hub.mu.Lock()
		srv.Hub.noteLocked(nil, "launch", "x")
		srv.Hub.mu.Unlock()
	}
	if n := len(srv.Hub.Activity()); n != keepActivity {
		t.Fatalf("the log should keep %d lines, has %d", keepActivity, n)
	}
}
