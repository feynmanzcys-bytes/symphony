package bridge

import (
	"net/http"
	"strings"
	"testing"
)

// TestAskRemember: a permission ask may carry the rule an allow could be kept as for the rest of the session (the mod
// proposes it); the page's allow with remember marks the chat item so; the mod's session.rule report lists the rule on
// the session and says so in its chat, once.
func TestAskRemember(t *testing.T) {
	srv, base := newBridge(t)
	mod, _ := as(t, srv, base, "a")
	page := client{t: t, base: base}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Cwd: "/w"})
	res := mod.ok("POST", "/api/ask", AskRequest{Session: "s1", Kind: "permission", Tool: "Bash", Input: map[string]any{"command": "npm test -- --watch"},
		Remember: &Rule{Tool: "Bash", Prefix: "npm test"}})
	id := res["id"].(string)
	snap := srv.Hub.Snapshot()
	if len(snap.Asks) != 1 || snap.Asks[0].Remember == nil || *snap.Asks[0].Remember != (Rule{Tool: "Bash", Prefix: "npm test"}) {
		t.Fatalf("the ask should carry the rule it may be kept as: %+v", snap.Asks)
	}
	page.ok("POST", "/api/answer/"+id, map[string]any{"decision": "allow", "remember": true})
	var item *ChatItem
	for _, it := range srv.Hub.Chat("s1") {
		if it.Kind == ChatAsk {
			item = it
		}
	}
	if item == nil || item.Status != "allowSession" {
		t.Fatalf("the chat should say it was allowed for the session: %+v", item)
	}
	// the mod keeps it and says so; a second report of the same rule changes nothing
	for range 2 {
		mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.rule", Rule: &Rule{Tool: "Bash", Prefix: "npm test"}})
	}
	s := session(t, srv, "s1")
	if len(s.Rules) != 1 || s.Rules[0].Prefix != "npm test" {
		t.Fatalf("the session should list the rule once: %+v", s.Rules)
	}
	notices := 0
	for _, it := range srv.Hub.Chat("s1") {
		if it.Kind == ChatNotice && it.Text == "remembered" && it.Summary == "Bash npm test" {
			notices++
		}
	}
	if notices != 1 {
		t.Fatalf("the chat should say once what was allowed for the session: %d", notices)
	}
	// a question carries no rule, whatever is sent
	q := mod.ok("POST", "/api/ask", AskRequest{Session: "s1", Kind: "question", Questions: []any{}, Remember: &Rule{Tool: "Bash"}})
	for _, a := range srv.Hub.Snapshot().Asks {
		if a.ID == q["id"] && a.Remember != nil {
			t.Fatalf("only a permission ask carries a rule: %+v", a)
		}
	}
}

// TestElicitationAsk: an MCP server's form is a third kind of ask, which the page shows and the session's terminal
// answers: the session waits while it's pending, the mod settles it with what the terminal answered, and the mod can't
// settle any other kind (only the page answers a permission).
func TestElicitationAsk(t *testing.T) {
	srv, base := newBridge(t)
	mod, _ := as(t, srv, base, "a")
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Cwd: "/w"})
	res := mod.ok("POST", "/api/ask", AskRequest{Session: "s1", Kind: "elicitation", Server: "github", Message: "Which repository?",
		Fields: []AskField{{Name: "repo", Title: "Repository", Type: "string", Required: true}, {Name: "private", Type: "boolean"}}})
	id := res["id"].(string)
	if s := session(t, srv, "s1"); s.State != StateWaiting {
		t.Fatalf("a session with an MCP form pending waits for a person: %s", s.State)
	}
	a := srv.Hub.Snapshot().Asks[0]
	if a.Kind != "elicitation" || a.Server != "github" || a.Mode != "form" || len(a.Fields) != 2 || !a.Fields[0].Required {
		t.Fatalf("the ask should carry the server, the question and the form: %+v", a)
	}
	// the page can't answer it: the terminal does
	page := client{t: t, base: base}
	if st, code := page.failure("POST", "/api/answer/"+id, map[string]any{"decision": "allow"}); st != http.StatusForbidden || code != "ask_not_page_answered" {
		t.Fatalf("the page answering an MCP form: want 403 ask_not_page_answered, got %d %s", st, code)
	}
	// a permission ask: only the page answers it
	p := mod.ok("POST", "/api/ask", AskRequest{Session: "s1", Kind: "permission", Tool: "Bash", Input: map[string]any{"command": "ls"}})
	if st, code := mod.failure("POST", "/api/settle/"+p["id"].(string), map[string]any{"action": "accept"}); st != http.StatusForbidden || code != "ask_not_mod_settled" {
		t.Fatalf("the mod settling a permission: want 403 ask_not_mod_settled, got %d %s", st, code)
	}
	mod.ok("POST", "/api/cancel/"+p["id"].(string), nil)
	mod.ok("POST", "/api/settle/"+id, map[string]any{"action": "accept"})
	if len(srv.Hub.Snapshot().Asks) != 0 {
		t.Fatal("the settled form should leave the asks")
	}
	if s := session(t, srv, "s1"); s.State == StateWaiting {
		t.Fatal("the session no longer waits once the form is answered")
	}
	var item *ChatItem
	for _, it := range srv.Hub.Chat("s1") {
		if it.Kind == ChatAsk && it.Ask == id {
			item = it
		}
	}
	if item == nil || item.Status != "accept" || item.Tool != "github" || !strings.Contains(item.Summary, "Which repository?") {
		t.Fatalf("the chat should say the form was answered (accept), by which server: %+v", item)
	}
	// /api/settle and /api/roster are the mod's: its token, no page key
	other := client{t: t, base: base, modOnly: true}
	if st, _ := other.do("POST", "/api/settle/"+id, map[string]any{}); st != http.StatusUnauthorized {
		t.Fatalf("settle without a token: want 401, got %d", st)
	}
}

// TestRoster: a session hears of the others online by the names SendMessage finds them by (its own besides), with
// their folders and tasks; sessions off duty aren't in it.
func TestRoster(t *testing.T) {
	srv, base := newBridge(t)
	alice, _ := as(t, srv, base, "alice")
	bob, _ := as(t, srv, base, "bob")
	carol, _ := as(t, srv, base, "carol")
	alice.ok("POST", "/api/event", Event{Session: "a1", Kind: "session.start", Cwd: "/src/web"})
	bob.ok("POST", "/api/event", Event{Session: "b1", Kind: "session.start", Cwd: "/src/api"})
	bob.ok("POST", "/api/event", Event{Session: "b1", Kind: "session.title", Title: "backend-bob"})
	carol.ok("POST", "/api/event", Event{Session: "c1", Kind: "session.start", Cwd: "/src/docs"})
	carol.ok("POST", "/api/event", Event{Session: "c1", Kind: "session.end"})
	res := alice.ok("GET", "/api/roster?session=a1", nil)
	if res["you"] != "alice" {
		t.Fatalf("its own name: %v", res["you"])
	}
	others := res["others"].([]any)
	if len(others) != 1 {
		t.Fatalf("one other online (carol is off duty): %v", others)
	}
	o := others[0].(map[string]any)
	if o["name"] != "backend-bob" || o["folder"] != "api" {
		t.Fatalf("bob by his session's name, in his folder: %v", o)
	}
	if st := (client{t: t, base: base, modOnly: true}).status("GET", "/api/roster?session=a1", nil); st != http.StatusUnauthorized {
		t.Fatalf("the roster without a token: want 401, got %d", st)
	}
}
