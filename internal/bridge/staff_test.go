package bridge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// staffBridge is a bridge with a data folder, for the employees page: the server, a client with the page key, the hub
// and its folder.
func staffBridge(t *testing.T) (*Server, client, *Hub, string) {
	t.Helper()
	dir := t.TempDir()
	hub := NewHub()
	var w warnings
	w.catch(hub)
	if err := hub.Open(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = hub.Flush() }) // the error log is written in the background (before the folder goes)
	srv := NewServer(hub, testWeb)
	srv.Key = testKey
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return srv, client{t: t, base: ts.URL}, hub, dir
}

func strPtr(s string) *string { return &s }

// TestImportStaff: an import makes the employees it names that aren't there (the lowest free seats, what they start
// with as the settings say, or as the entry says over the settings) and changes the ones that are (what they start
// with, the entry over their last choices; one never started from the page over the settings and the folder it last
// worked in); a field left out stays as it is. It answers both lists without tokens, and accounts.json keeps them.
func TestImportStaff(t *testing.T) {
	srv, page, hub, data := staffBridge(t)
	if _, err := hub.SetSettings(Settings{Model: "opus", Effort: "high"}); err != nil {
		t.Fatal(err)
	}
	alice, _ := hub.CreateAccount("alice") // seat 0, never started from the page; worked in /work/app by hand
	mod := client{t: t, base: page.base, header: map[string]string{"authorization": "Bearer " + alice.Token}, modOnly: true}
	mod.ok("POST", "/api/event", map[string]any{"session": "s-a", "kind": "session.start", "cwd": "/work/app"})
	mod.ok("POST", "/api/event", map[string]any{"session": "s-a", "kind": "session.end"})
	bob, _ := hub.CreateAccount("bob") // seat 1, started from the page before
	folder := t.TempDir()
	hub.setLast(bob.ID, LaunchRequest{Dir: folder, Model: "sonnet", Effort: "low", PermissionMode: "acceptEdits", Resume: "continue"})
	gone, _ := hub.CreateAccount("gone") // seat 2, deleted: its seat is free again
	if err := hub.DeleteAccount(gone.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.CreateAdvisor("rev"); err != nil {
		t.Fatal(err)
	}

	own := t.TempDir()
	got := page.ok("POST", "/api/accounts/import", map[string]any{"employees": []map[string]any{
		{"name": "alice", "model": "haiku"},
		{"name": "bob", "effort": "max", "context": true},
		{"name": "carol"},
		{"name": " dave ", "dir": own + "/", "permissionMode": "plan", "context": true},
		{"name": "erin", "dir": ""},
	}})
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "token") || strings.Contains(string(raw), alice.Token) {
		t.Fatalf("the answer should carry no token: %s", raw)
	}
	var out StaffImport
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	names := func(list []AccountView) (s []string) {
		for _, a := range list {
			s = append(s, a.Name+"@"+strconv.Itoa(a.Seat))
		}
		return
	}
	if c, u := strings.Join(names(out.Created), ","), strings.Join(names(out.Updated), ","); c != "carol@2,dave@3,erin@4" || u != "alice@0,bob@1" {
		t.Fatalf("made %s (want carol@2,dave@3,erin@4: the lowest free seats), changed %s", c, u)
	}
	last := func(name string) *LaunchRequest {
		t.Helper()
		for _, a := range hub.Snapshot().Accounts {
			if a.Name == name {
				return a.Last
			}
		}
		t.Fatalf("no employee %s", name)
		return nil
	}
	want := map[string]*LaunchRequest{
		// never started from the page: the settings' choices and the folder it worked in, the model the import's
		"alice": {Dir: "/work/app", Model: "haiku", Effort: "high"},
		// its last choices, the effort and the context the import's (the session to continue stays)
		"bob": {Dir: folder, Model: "sonnet", Effort: "max", PermissionMode: "acceptEdits", Resume: "continue", Context: true},
		// nothing said: as the settings say, until its first start
		"carol": nil,
		// the settings' choices with the import's over them; the folder cleaned
		"dave": {Dir: own, Model: "opus", Effort: "high", PermissionMode: "plan", Context: true},
		// its own folder, said: the settings' choices
		"erin": {Model: "opus", Effort: "high"},
	}
	for name, w := range want {
		if l := last(name); (l == nil) != (w == nil) || (l != nil && *l != *w) {
			t.Errorf("%s starts with %+v, want %+v", name, l, w)
		}
	}
	// accounts.json keeps them, each with a token of its own (an advisor without a task goes as a bridge starts)
	reread := NewHub()
	if err := reread.Open(data); err != nil {
		t.Fatal(err)
	}
	tokens := map[string]bool{}
	for _, a := range reread.accounts {
		if a.Name == "dave" && (a.Last == nil || a.Last.PermissionMode != "plan") {
			t.Errorf("accounts.json lost dave's choices: %+v", a.Last)
		}
		if len(a.Token) != 48 || tokens[a.Token] {
			t.Errorf("%s has no token of its own: %q", a.Name, a.Token)
		}
		tokens[a.Token] = true
	}
	if len(tokens) != 5 {
		t.Fatalf("want the 5 employees in accounts.json, got %d", len(tokens))
	}
	_ = reread.Flush()

	// an entry that says nothing, or nothing new, of what an employee starts with leaves it as it is, in neither list
	again := page.ok("POST", "/api/accounts/import", map[string]any{"employees": []map[string]any{{"name": "carol"}, {"name": "bob"}, {"name": "dave", "permissionMode": "plan", "dir": own}}})
	if last("carol") != nil || *last("bob") != *want["bob"] || *last("dave") != *want["dave"] {
		t.Errorf("named alone, or as they are, carol, bob and dave should stay: %+v %+v %+v", last("carol"), last("bob"), last("dave"))
	}
	if c, u := again["created"].([]any), again["updated"].([]any); len(c) != 0 || len(u) != 0 {
		t.Errorf("nothing made and nothing changed: %v", again)
	}
	// one going by the settings that gets their own values stops going by them: a change
	opus := page.ok("POST", "/api/accounts/import", map[string]any{"employees": []map[string]any{{"name": "carol", "model": "opus"}}})
	if u := opus["updated"].([]any); len(u) != 1 || last("carol") == nil || *last("carol") != (LaunchRequest{Model: "opus", Effort: "high"}) {
		t.Errorf("carol on the settings' model is carol with choices of her own: %v %+v", opus, last("carol"))
	}

	// the snapshot the pages get shows them as the answer did
	if v := view(t, srv, accountIDOf(srv, "dave")); v.Last == nil || v.Last.Dir != own || v.Home != "" {
		t.Errorf("dave in the snapshot: %+v", v)
	}
	if st := (client{t: t, base: page.base, modOnly: true}).status("POST", "/api/accounts/import", map[string]any{"employees": []map[string]any{{"name": "x"}}}); st != http.StatusUnauthorized {
		t.Errorf("without the page key: %d", st)
	}
}

// TestImportStaffAllOrNothing: an import with one entry wrong does nothing and says which entry and why (its place,
// its name, the error's own code and params): a name that can't be one, a name twice, an advisor's name, a relative
// folder or one that isn't there, a choice the drop-downs don't offer; nothing, or more than 100. A failed write of
// accounts.json takes back what it made and changed.
func TestImportStaffAllOrNothing(t *testing.T) {
	_, page, hub, data := staffBridge(t)
	bob, _ := hub.CreateAccount("bob")
	hub.setLast(bob.ID, LaunchRequest{Model: "sonnet"})
	if _, err := hub.CreateAdvisor("rev"); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		var list []string
		for _, a := range hub.Snapshot().Accounts {
			l, _ := json.Marshal(a.Last)
			list = append(list, a.Name+string(l))
		}
		return strings.Join(list, ";")
	}
	before := snapshot()
	ok := map[string]any{"name": "carol", "model": "opus"}
	missing := filepath.Join(t.TempDir(), "nope")
	for _, c := range []struct {
		bad          map[string]any
		code, params string
	}{
		{map[string]any{"name": "  "}, "account_bad_name", "entry=2 name="},
		{map[string]any{"name": strings.Repeat("x", 41)}, "account_bad_name", "entry=2 name=" + strings.Repeat("x", 41)},
		{map[string]any{"name": "carol"}, "import_duplicate", "entry=2 name=carol"},
		{map[string]any{"name": "rev", "model": "opus"}, "account_name_taken", "entry=2 name=rev"},
		{map[string]any{"name": "dave", "dir": "work/app"}, "launch_dir_not_absolute", "entry=2 name=dave"},
		{map[string]any{"name": "dave", "dir": missing}, "launch_dir_missing", "dir=" + missing + " entry=2 name=dave"},
		{map[string]any{"name": "bob", "model": "gpt"}, "launch_bad_option", "entry=2 name=bob"},
		{map[string]any{"name": "bob", "effort": "huge"}, "launch_bad_option", "entry=2 name=bob"},
		{map[string]any{"name": "bob", "permissionMode": "yolo"}, "launch_bad_option", "entry=2 name=bob"},
	} {
		status, body := page.do("POST", "/api/accounts/import", map[string]any{"employees": []map[string]any{ok, c.bad}})
		params, _ := body["params"].(map[string]any)
		var kv []string
		for _, k := range []string{"dir", "entry", "name"} {
			if v, ok := params[k]; ok {
				kv = append(kv, k+"="+v.(string))
			}
		}
		if status != http.StatusBadRequest || body["code"] != c.code || strings.Join(kv, " ") != c.params {
			t.Errorf("%v: want 400 %s %s, got %d %v", c.bad, c.code, c.params, status, body)
		}
		if text, _ := body["error"].(string); !strings.HasPrefix(text, "employee 2 (") {
			t.Errorf("%v: the English text should name the entry: %q", c.bad, text)
		}
	}
	if st, code := page.failure("POST", "/api/accounts/import", map[string]any{"employees": []any{}}); st != http.StatusBadRequest || code != "import_empty" {
		t.Errorf("nothing to import: %d %q", st, code)
	}
	many := make([]map[string]any, maxImport+1)
	for i := range many {
		many[i] = map[string]any{"name": "e" + strconv.Itoa(i)}
	}
	if st, code := page.failure("POST", "/api/accounts/import", map[string]any{"employees": many}); st != http.StatusBadRequest || code != "import_too_many" {
		t.Errorf("more than %d: %d %q", maxImport, st, code)
	}
	if st, _ := page.failure("POST", "/api/accounts/import", map[string]any{"employees": []map[string]any{{"name": "x", "context": "yes"}}}); st != http.StatusBadRequest {
		t.Errorf("context must be true or false: %d", st)
	}
	if now := snapshot(); now != before {
		t.Fatalf("a refused import should change nothing:\n%s\n%s", before, now)
	}

	// accounts.json can't be written: nothing made, nothing changed
	blocked := filepath.Join(data, "accounts.json.tmp")
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	status, body := page.do("POST", "/api/accounts/import", map[string]any{"employees": []map[string]any{ok, {"name": "bob", "model": "haiku"}}})
	if status != http.StatusInternalServerError || body["code"] != "data_write_failed" {
		t.Fatalf("want 500 data_write_failed, got %d %v", status, body)
	}
	if now := snapshot(); now != before {
		t.Fatalf("an import that couldn't be saved should be taken back:\n%s\n%s", before, now)
	}
	if hub.nameTaken("carol") {
		t.Error("carol couldn't be saved: she should be gone")
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	page.ok("POST", "/api/accounts/import", map[string]any{"employees": []map[string]any{ok}})
	if !hub.nameTaken("carol") {
		t.Error("once accounts.json can be written, the import goes through")
	}
}
