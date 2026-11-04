package bridge

import (
	"net/http"
	"testing"
)

// An employee's face (Account.Look): picked when it is made (POST /api/accounts, or the launch that hires it, which
// never keeps it in Last), set or changed by an import (the same one again changes nothing), refused when it isn't
// a shape or a hue the page draws.
func TestLook(t *testing.T) {
	_, page, hub, _ := staffBridge(t)
	hex := map[string]any{"shape": "hex", "hue": 212}
	if st, code := page.failure("POST", "/api/accounts", map[string]any{"name": "zed", "look": map[string]any{"shape": "star", "hue": 10}}); st != http.StatusBadRequest || code != "account_bad_look" {
		t.Fatalf("a shape the page doesn't draw: %d %s", st, code)
	}
	if st, code := page.failure("POST", "/api/accounts", map[string]any{"name": "zed", "look": map[string]any{"shape": "hex", "hue": 400}}); st != http.StatusBadRequest || code != "account_bad_look" {
		t.Fatalf("a hue off the wheel: %d %s", st, code)
	}
	made := page.ok("POST", "/api/accounts", map[string]any{"name": "zed", "look": hex})
	zed := hub.Account(made["id"].(string))
	if zed.Look == nil || zed.Look.Shape != "hex" || zed.Look.Hue != 212 {
		t.Fatalf("the face should be kept on the account: %+v", zed.Look)
	}
	plain, _ := hub.CreateAccount("ann") // no face: the page picks one from the name
	if plain.Look != nil {
		t.Fatalf("an account made without a face should have none: %+v", plain.Look)
	}

	// the import: a new employee with a face, a change of face, the same face again (nothing), a bad one (all or nothing)
	got := page.ok("POST", "/api/accounts/import", map[string]any{"employees": []map[string]any{
		{"name": "ann", "look": map[string]any{"shape": "drop", "hue": 46}},
		{"name": "zed", "look": hex},
		{"name": "new", "look": map[string]any{"shape": "pill", "hue": 285}},
	}})
	if n := len(got["created"].([]any)); n != 1 {
		t.Fatalf("one made: %v", got)
	}
	if upd := got["updated"].([]any); len(upd) != 1 || upd[0].(map[string]any)["name"] != "ann" {
		t.Fatalf("ann changed, zed (the same face again) not: %v", got["updated"])
	}
	if l := hub.Account(plain.ID).Look; l == nil || l.Shape != "drop" || l.Hue != 46 {
		t.Fatalf("ann's face should be set: %+v", l)
	}
	if st, code := page.failure("POST", "/api/accounts/import", map[string]any{"employees": []map[string]any{{"name": "ann", "look": map[string]any{"shape": "hex", "hue": -1}}}}); st != http.StatusBadRequest || code != "account_bad_look" {
		t.Fatalf("a bad face in an import: %d %s", st, code)
	}
	// a change of choices alone leaves the face; the face alone counts as a change
	page.ok("POST", "/api/accounts/import", map[string]any{"employees": []map[string]any{{"name": "ann", "model": "haiku"}}})
	if l := hub.Account(plain.ID).Look; l == nil || l.Shape != "drop" {
		t.Fatalf("the face should stay through a change of choices: %+v", l)
	}
}

// The launch that hires an employee gives it the face picked, and keeps it on the account only: not in Last, which
// the next start fills in from.
func TestLaunchLook(t *testing.T) {
	srv, base, _ := newLaunchBridge(t)
	web := client{t: t, base: base}
	if st, code := web.failure("POST", "/api/launch", LaunchRequest{Name: "zoe", Dir: t.TempDir(), Look: &Look{Shape: "cube", Hue: 1}}); st != http.StatusBadRequest || code != "account_bad_look" {
		t.Fatalf("a bad face: %d %s", st, code)
	}
	web.ok("POST", "/api/launch", LaunchRequest{Name: "zoe", Dir: t.TempDir(), Look: &Look{Shape: "pill", Hue: 325}})
	var zoe *Account
	for _, a := range srv.Hub.Snapshot().Accounts {
		if a.Name == "zoe" {
			zoe = srv.Hub.Account(a.ID)
		}
	}
	if zoe == nil || zoe.Look == nil || zoe.Look.Shape != "pill" || zoe.Look.Hue != 325 {
		t.Fatalf("the face should be the account's: %+v", zoe)
	}
	if zoe.Last == nil || zoe.Last.Look != nil {
		t.Fatalf("Last should hold the start's choices without the face: %+v", zoe.Last)
	}
}
