package bridge

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestSettings: the default folder for the employees' own folders is the default until one is set; one that is set is
// checked (absolute, not a file, not in the data folder), kept in settings.json and read again by the next bridge; an
// empty one goes back to the default. A settings.json that doesn't parse is set aside, and the bridge starts on the
// defaults.
func TestSettings(t *testing.T) {
	data, def := t.TempDir(), filepath.Join(t.TempDir(), "bots")
	// a bridge on the data folder, flushed before the folders go: what it warns of (the damaged file below) goes to
	// its error log, errors.json in there, which is written in the background
	open := func() (*Hub, error) {
		h := NewHub()
		h.BotHomeDefault = def
		t.Cleanup(func() { _ = h.Flush() })
		return h, h.Open(data)
	}
	h, err := open()
	if err != nil {
		t.Fatal(err)
	}
	if v := h.Settings(); v.BotHome != def || v.BotHomeDefault != def {
		t.Fatalf("the default first: %+v", v)
	}
	file := filepath.Join(t.TempDir(), "f")
	writeFile(t, file, "x")
	for name, tc := range map[string]struct{ dir, code string }{
		"relative":             {"bots", "launch_dir_not_absolute"},
		"a file":               {file, "bot_home_not_dir"},
		"the data folder":      {data, "bot_home_in_data"},
		"in the data folder":   {filepath.Join(data, "bots"), "bot_home_in_data"},
		"in it, written oddly": {data + string(filepath.Separator) + "x" + string(filepath.Separator) + "..", "bot_home_in_data"},
	} {
		if _, err := h.SetSettings(Settings{BotHome: tc.dir}); err == nil || err.(*Error).Code != tc.code {
			t.Errorf("%s: want %s, got %v", name, tc.code, err)
		}
	}
	if exists(filepath.Join(data, "settings.json")) {
		t.Fatal("nothing refused is written")
	}
	mine := filepath.Join(t.TempDir(), "not", "there", "yet")
	if v, err := h.SetSettings(Settings{BotHome: "  " + mine + string(filepath.Separator) + "  "}); err != nil || v.BotHome != mine || v.BotHomeDefault != def {
		t.Fatalf("an absolute folder, made later, cleaned: %+v %v", v, err)
	}
	if exists(mine) {
		t.Fatal("setting it makes nothing yet")
	}
	again, err := open()
	if err != nil {
		t.Fatal(err)
	}
	if v := again.Settings(); v.BotHome != mine {
		t.Fatalf("the next bridge reads it: %+v", v)
	}
	if v, err := again.SetSettings(Settings{}); err != nil || v.BotHome != def {
		t.Fatalf("empty is the default again: %+v %v", v, err)
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(data, "settings.json"))); got != "{}" {
		t.Fatalf("the default isn't written down: %q", got)
	}

	// what a start begins with: Claude Code's own first; values of the launch dialog's drop-downs only, kept and read
	// again by the next bridge, the folder untouched
	if v := again.Settings(); v.LaunchDefaults != (LaunchDefaults{}) || len(v.Models) != len(launchModels) || len(v.PermissionModes) != len(launchPermissionModes) {
		t.Fatalf("Claude Code's own, and the choices: %+v", v)
	}
	for name, bad := range map[string]Settings{"model": {Model: "gpt"}, "effort": {Effort: "huge"}, "permission mode": {PermissionMode: "yolo"}} {
		if _, err := again.SetSettings(bad); err == nil || err.(*Error).Code != "launch_bad_option" {
			t.Errorf("an unknown %s: %v", name, err)
		}
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(data, "settings.json"))); got != "{}" {
		t.Fatalf("nothing refused is written: %q", got)
	}
	want := Settings{BotHome: mine, Model: "opus", Effort: "high", PermissionMode: "manual", Context: true}
	if v, err := again.SetSettings(want); err != nil || v.LaunchDefaults != want.launchDefaults() || v.BotHome != mine {
		t.Fatalf("set: %+v %v", v, err)
	}
	fourth, err := open()
	if err != nil {
		t.Fatal(err)
	}
	if v := fourth.Settings(); v.LaunchDefaults != want.launchDefaults() || v.BotHome != mine {
		t.Fatalf("the next bridge reads them: %+v", v)
	}
	// a hand edit's choice the drop-downs don't offer is Claude Code's own; the rest stays
	writeFile(t, filepath.Join(data, "settings.json"), `{"model": "gpt", "effort": "max", "permissionMode": "yolo", "context": true}`)
	fifth, err := open()
	if err != nil {
		t.Fatal(err)
	}
	if v := fifth.Settings(); v.LaunchDefaults != (LaunchDefaults{Effort: "max", Context: true}) || v.BotHome != def {
		t.Fatalf("unknown choices dropped: %+v", v)
	}

	// damaged: set aside, and the defaults
	writeFile(t, filepath.Join(data, "settings.json"), `{"botHome": `)
	third, err := open()
	if err != nil {
		t.Fatalf("a damaged settings.json doesn't stop the bridge: %v", err)
	}
	aside, _ := filepath.Glob(filepath.Join(data, "settings.json.corrupt-*"))
	if v := third.Settings(); v.BotHome != def || len(aside) != 1 || exists(filepath.Join(data, "settings.json")) {
		t.Fatalf("set aside (%v), the default in effect: %+v", aside, v)
	}
}

// TestSettingsAPI: GET /api/settings answers the settings in effect and the default; POST changes the fields it names
// (an empty botHome is the default) and answers 400 with the code for a folder it refuses; the launch dialog's options
// carry the folder in effect.
func TestSettingsAPI(t *testing.T) {
	srv, base, _ := newLaunchBridge(t)
	def := filepath.Join(t.TempDir(), "bots")
	srv.Hub.BotHomeDefault = def
	c := client{t: t, base: base}
	if got := c.ok("GET", "/api/settings", nil); got["botHome"] != def || got["botHomeDefault"] != def {
		t.Fatalf("the default: %v", got)
	}
	if st, code := c.failure("POST", "/api/settings", map[string]any{"botHome": "relative/bots"}); st != http.StatusBadRequest || code != "launch_dir_not_absolute" {
		t.Fatalf("a relative folder: %d %q", st, code)
	}
	mine := t.TempDir()
	if got := c.ok("POST", "/api/settings", map[string]any{"botHome": mine}); got["botHome"] != mine || got["botHomeDefault"] != def {
		t.Fatalf("set: %v", got)
	}
	if got := c.ok("POST", "/api/settings", map[string]any{}); got["botHome"] != mine {
		t.Fatalf("a field not named stays: %v", got)
	}
	if got := c.ok("GET", "/api/launch/options", nil); got["botHome"] != mine {
		t.Fatalf("the launch dialog's options carry it: %v", got["botHome"])
	}
	if got := c.ok("POST", "/api/settings", map[string]any{"botHome": ""}); got["botHome"] != def {
		t.Fatalf("empty is the default: %v", got)
	}
	if st := (client{t: t, base: base, modOnly: true}).status("GET", "/api/settings", nil); st != http.StatusUnauthorized {
		t.Fatalf("without the page key: %d", st)
	}

	// what a start begins with: Claude Code's own first, with the drop-downs' choices; each field changed alone
	got := c.ok("GET", "/api/settings", nil)
	if got["model"] != "" || got["effort"] != "" || got["permissionMode"] != "" || got["context"] != false || len(got["models"].([]any)) != len(launchModels) ||
		len(got["efforts"].([]any)) != len(launchEfforts) || len(got["permissionModes"].([]any)) != len(launchPermissionModes) {
		t.Fatalf("Claude Code's own, and the choices: %v", got)
	}
	if got := c.ok("POST", "/api/settings", map[string]any{"model": "opus"}); got["model"] != "opus" || got["effort"] != "" || got["botHome"] != def {
		t.Fatalf("the model alone: %v", got)
	}
	if got := c.ok("POST", "/api/settings", map[string]any{"effort": "max", "permissionMode": "bypassPermissions", "context": true}); got["model"] != "opus" ||
		got["effort"] != "max" || got["permissionMode"] != "bypassPermissions" || got["context"] != true {
		t.Fatalf("the rest, the model kept: %v", got)
	}
	for _, bad := range []map[string]any{{"model": "gpt"}, {"effort": "huge"}, {"permissionMode": "yolo"}} {
		if st, code := c.failure("POST", "/api/settings", bad); st != http.StatusBadRequest || code != "launch_bad_option" {
			t.Errorf("%v: %d %q", bad, st, code)
		}
	}
	if st, _ := c.failure("POST", "/api/settings", map[string]any{"context": "yes"}); st != http.StatusBadRequest {
		t.Errorf("context must be true or false: %d", st)
	}
	opts := c.ok("GET", "/api/launch/options", nil)
	if d, _ := json.Marshal(opts["defaults"]); string(d) != `{"context":true,"effort":"max","model":"opus","permissionMode":"bypassPermissions"}` {
		t.Fatalf("the launch dialog's options carry them, the refused ones not taken: %s", d)
	}
	if got := c.ok("POST", "/api/settings", map[string]any{"model": "", "effort": "", "permissionMode": "", "context": false}); got["model"] != "" || got["context"] != false {
		t.Fatalf("back to Claude Code's own: %v", got)
	}

	// changes at once (a page's drop-downs save as they change) each keep the others'
	for round := 0; round < 20; round++ {
		if _, err := srv.Hub.SetSettings(Settings{}); err != nil {
			t.Fatal(err)
		}
		opus, high, plan, yes := "opus", "high", "plan", true
		var wg sync.WaitGroup
		errs := make(chan error, 4)
		for _, ch := range []SettingsChange{{Model: &opus}, {Effort: &high}, {PermissionMode: &plan}, {Context: &yes}} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := srv.Hub.ChangeSettings(ch); err != nil {
					errs <- err
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
		if v := srv.Hub.Settings(); v.LaunchDefaults != (LaunchDefaults{Model: "opus", Effort: "high", PermissionMode: "plan", Context: true}) {
			t.Fatalf("round %d: a change lost: %+v", round, v.LaunchDefaults)
		}
	}
}

// TestLaunchWithoutFolder: an employee started from the page without a folder works in one of its own: a new folder in
// the default folder (made with it), named after it, a git repository with a first, empty commit and the exclude file;
// bound to the account (Home, the snapshot's home, the last start's folder), so it goes back there; made again when it's
// gone. What is in the default folder already is never taken over: a name that's taken gets -2. A start that fails takes
// a new employee's account and its new folder away; without a default folder, or with one in the data folder (a hand
// edit), nothing starts.
func TestLaunchWithoutFolder(t *testing.T) {
	needGit(t)
	srv, base, starts := newLaunchBridge(t)
	starts.dead = true // every process exits at once: the desk is free again for the next start
	home := filepath.Join(t.TempDir(), "bots")
	srv.Hub.BotHomeDefault = home
	c := client{t: t, base: base}
	// relaunch puts an employee back to work without a folder, once its last start is over (its process gone)
	relaunch := func(acc string) {
		t.Helper()
		var st int
		var body map[string]any
		waitFor(t, "the desk to be free", func() bool {
			st, body = c.do("POST", "/api/launch", LaunchRequest{Account: acc})
			return st != http.StatusConflict
		})
		if st != http.StatusOK {
			t.Fatalf("back to work: %d %v", st, body)
		}
	}

	c.ok("POST", "/api/launch", LaunchRequest{Name: "Ann Lee"})
	dir := filepath.Join(home, "Ann-Lee")
	if st := starts.last(); st == nil || st.dir != dir {
		t.Fatalf("it should start in %s: %+v", dir, st)
	}
	if got := mustGit(t, dir, "log", "--format=%s|%an", "--all"); got != "symphony: Ann Lee's folder|symphony" {
		t.Fatalf("a repository with one empty commit: %q", got)
	}
	if files := mustGit(t, dir, "ls-files"); files != "" {
		t.Fatalf("nothing committed: %q", files)
	}
	if ex := readFile(t, filepath.Join(dir, ".git", "info", "exclude")); !strings.Contains(ex, ".symphony/") || !strings.Contains(ex, ".env") {
		t.Fatalf("the exclude file lists .symphony/ and the secrets: %q", ex)
	}
	accID := srv.Hub.Snapshot().Accounts[0].ID
	acc := srv.Hub.Account(accID)
	if acc.Home != dir || acc.Last == nil || acc.Last.Dir != dir || view(t, srv, accID).Home != dir {
		t.Fatalf("bound to the account: home %q, last %+v", acc.Home, acc.Last)
	}

	// back to work without a folder: the same one, nothing made
	relaunch(accID)
	if st := starts.last(); st.dir != dir {
		t.Fatalf("back in its own folder: %+v", st)
	}
	if entries, _ := os.ReadDir(home); len(entries) != 1 {
		t.Fatalf("no second folder: %v", entries)
	}
	// gone: made again, at the same place
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	relaunch(accID)
	if st := starts.last(); st.dir != dir || mustGit(t, dir, "log", "--format=%s") != "symphony: Ann Lee's folder" {
		t.Fatalf("made again in the same place: %+v", st)
	}

	// taken names: by another employee's folder, or by anything already there, which stays as it was
	c.ok("POST", "/api/launch", LaunchRequest{Name: "Ann/Lee"})
	if st := starts.last(); st.dir != dir+"-2" {
		t.Fatalf("the same folder name gets -2: %+v", st)
	}
	writeFile(t, filepath.Join(home, "zoe", "notes.txt"), "mine")
	c.ok("POST", "/api/launch", LaunchRequest{Name: "zoe"})
	if st := starts.last(); st.dir != filepath.Join(home, "zoe-2") || exists(filepath.Join(home, "zoe", ".git")) {
		t.Fatalf("a folder already there is left alone: %+v", st)
	}
	// a folder given is used as it is, and nothing is made
	given := t.TempDir()
	c.ok("POST", "/api/launch", LaunchRequest{Name: "given", Dir: given})
	if st := starts.last(); st.dir != given || exists(filepath.Join(given, ".git")) || exists(filepath.Join(home, "given")) {
		t.Fatalf("a folder given is the one: %+v", st)
	}
	if a := srv.Hub.Account(accountIDOf(srv, "given")); a == nil || a.Home != "" {
		t.Fatalf("a folder given isn't bound as the employee's own: %q", a.Home)
	}

	// a start that fails: no account, no folder
	starts.failStart = true
	if st, code := c.failure("POST", "/api/launch", LaunchRequest{Name: "eve"}); st != http.StatusInternalServerError || code != "pty_start_failed" {
		t.Fatalf("the start fails: %d %q", st, code)
	}
	if exists(filepath.Join(home, "eve")) || accountIDOf(srv, "eve") != "" {
		t.Fatal("a failed start takes the new employee and its folder away")
	}
	starts.failStart = false

	// no default folder, or one in the data folder: refused, and the new employee goes
	srv.Hub.BotHomeDefault = ""
	if st, code := c.failure("POST", "/api/launch", LaunchRequest{Name: "nohome"}); st != http.StatusBadRequest || code != "bot_home_unset" {
		t.Fatalf("no default folder: %d %q", st, code)
	}
	data := t.TempDir()
	srv.Hub.mu.Lock()
	srv.Hub.dataDir, srv.Hub.settings.BotHome = data, filepath.Join(data, "bots")
	srv.Hub.mu.Unlock()
	if st, code := c.failure("POST", "/api/launch", LaunchRequest{Name: "indata"}); st != http.StatusBadRequest || code != "bot_home_in_data" {
		t.Fatalf("a default folder in the data folder: %d %q", st, code)
	}
	if accountIDOf(srv, "nohome") != "" || accountIDOf(srv, "indata") != "" || exists(filepath.Join(data, "bots")) {
		t.Fatal("refused: no account, no folder")
	}
}

// TestLaunchWithoutFolderNoGit: without git the employee's own folder is still made, a plain folder, and the log says
// why it isn't a repository.
func TestLaunchWithoutFolderNoGit(t *testing.T) {
	srv, base, starts := newLaunchBridge(t)
	home := filepath.Join(t.TempDir(), "bots")
	srv.Hub.BotHomeDefault = home
	t.Setenv("PATH", t.TempDir())
	client{t: t, base: base}.ok("POST", "/api/launch", LaunchRequest{Name: "nogit"})
	dir := filepath.Join(home, "nogit")
	if st := starts.last(); st.dir != dir || !isDir(dir) || exists(filepath.Join(dir, ".git")) {
		t.Fatalf("a plain folder: %+v", st)
	}
	found := false
	for _, f := range srv.Hub.Faults() {
		found = found || (f.Code == "bot_home_no_git" && strings.Contains(f.Text, dir))
	}
	if !found {
		t.Fatalf("the log says git is missing: %+v", srv.Hub.Faults())
	}
}

// TestEmployeeFolderFallsBackToHome: a task finds an employee's folder in the one made for it when nothing newer says
// where it works.
func TestEmployeeFolderFallsBackToHome(t *testing.T) {
	h := NewHub()
	acc, _ := h.CreateAccount("solo")
	if err := h.bindHome(acc.ID, "/bots/solo"); err != nil {
		t.Fatal(err)
	}
	if _, dir := h.employee(acc.ID); dir != "/bots/solo" {
		t.Fatalf("the employee's own folder: %q", dir)
	}
}

// TestBotFolderNames: the bridge's names for the employees' own folders, which the launch dialog's preview copies
// (folderName in web/ui/launch.js: the e2e TestSettings checks it names these the same)
func TestBotFolderNames(t *testing.T) {
	for name, want := range map[string]string{
		"Ann Lee": "Ann-Lee", "Ann/Lee": "Ann-Lee", "前端 小王": "前端-小王", "a..b": "a.b", "-x-": "x", "...": "employee",
		"x.lock": "x-lock", "a-!b": "a--b", "é_1": "é_1",
	} {
		if got := slugOr(name, "employee"); got != want {
			t.Errorf("%q: got %q, want %q", name, got, want)
		}
	}
}

// accountIDOf is the id of the account with this name, "" when there is none (accountNamed fails then).
func accountIDOf(srv *Server, name string) string {
	for _, a := range srv.Hub.Snapshot().Accounts {
		if a.Name == name {
			return a.ID
		}
	}
	return ""
}
