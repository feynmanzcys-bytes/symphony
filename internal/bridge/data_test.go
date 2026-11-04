package bridge

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestOpenDamagedFiles: what the bridge does with each data file that is garbage, empty, holds null entries or can't
// be read (here a folder in its place). The message log and the ledger are set aside as <name>.corrupt-<unix
// seconds>, with a line in the log naming the full path, and the bridge starts without them; accounts.json and
// tasks.json (the tokens, the task counter) stop it with the full path (DamagedFile); an empty tasks.json, which holds
// nothing, is set aside like the logs (the task counter comes back from the members' branches). Null
// entries are dropped. A file that can't be read stops the bridge and stays as it is: taken for empty, it would be
// written over.
func TestOpenDamagedFiles(t *testing.T) {
	type check func(t *testing.T, h *Hub, dir string, err error, warned []string)
	stops := func(t *testing.T, h *Hub, dir string, err error, warned []string) {
		var d *DamagedFile
		if !errors.As(err, &d) || !filepath.IsAbs(d.Path) || !strings.Contains(err.Error(), d.Path) {
			t.Fatalf("want a DamagedFile with the full path, got %v", err)
		}
		if _, err := os.Stat(d.Path); err != nil {
			t.Fatalf("the damaged file should stay where it is: %v", err)
		}
	}
	setAside := func(name string) check {
		return func(t *testing.T, h *Hub, dir string, err error, warned []string) {
			if err != nil {
				t.Fatalf("the bridge should start without %s: %v", name, err)
			}
			if exists(filepath.Join(dir, name)) {
				t.Fatalf("%s should have been moved aside", name)
			}
			aside, _ := filepath.Glob(filepath.Join(dir, name+".corrupt-*"))
			if len(aside) != 1 {
				t.Fatalf("want one %s.corrupt-<time>, got %q", name, aside)
			}
			if len(warned) != 1 || !strings.Contains(warned[0], filepath.Join(dir, name)) || !strings.Contains(warned[0], aside[0]) {
				t.Fatalf("the log should name the file and where it went: %q", warned)
			}
			if len(h.messages) != 0 || len(h.ledger) != 0 {
				t.Fatal("the log should start empty")
			}
		}
	}
	unreadable := func(t *testing.T, h *Hub, dir string, err error, warned []string) {
		var d *DamagedFile
		if err == nil || errors.As(err, &d) || !strings.Contains(err.Error(), dir) {
			t.Fatalf("want an error naming the file, not a damaged one, got %v", err)
		}
		if aside, _ := filepath.Glob(filepath.Join(dir, "*.corrupt-*")); len(aside) != 0 {
			t.Fatalf("a file that can't be read is never set aside: %q", aside)
		}
	}
	fine := func(want func(h *Hub) bool) check {
		return func(t *testing.T, h *Hub, dir string, err error, warned []string) {
			if err != nil {
				t.Fatal(err)
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			if !want(h) {
				t.Fatalf("null entries should be dropped: accounts %v, tasks %v, messages %v, ledger %v", h.accounts, h.tasks, h.messages, h.ledger)
			}
		}
	}
	cases := []struct {
		file, content string // content "dir": a folder in the file's place
		check         check
	}{
		{"accounts.json", "{garbage", stops},
		{"accounts.json", "", stops},
		{"accounts.json", `[null, {"id":"a1","name":"alice","token":"t1","seat":0}]`, fine(func(h *Hub) bool { return len(h.accounts) == 1 && h.accounts["a1"] != nil })},
		{"accounts.json", "dir", unreadable},
		{"tasks.json", "{garbage", stops},
		{"tasks.json", "", setAside("tasks.json")}, // the counter comes back from the branches
		{"tasks.json", `{"next":3,"tasks":[null,{"id":"T2","status":"failed","members":[null,{"account":"a1","name":"alice-T2"}],"repos":[null]}]}`,
			fine(func(h *Hub) bool {
				return h.taskNext == 3 && len(h.tasks) == 1 && len(h.tasks[0].Members) == 1 && len(h.tasks[0].Repos) == 0
			})},
		{"tasks.json", "dir", unreadable},
		{"messages.json", "{garbage", setAside("messages.json")},
		{"messages.json", "", setAside("messages.json")},
		{"messages.json", `[null, {"id":"m1","fromName":"a","toName":"b","text":"hi"}, null]`, fine(func(h *Hub) bool { return len(h.messages) == 1 })},
		{"messages.json", "dir", unreadable},
		{"ledger.json", "[{garbage", setAside("ledger.json")},
		{"ledger.json", "", setAside("ledger.json")},
		{"ledger.json", `[null, {"day":"2026-11-02","account":"a1","usd":1.5}]`, fine(func(h *Hub) bool { return len(h.ledger) == 1 })},
		{"ledger.json", "dir", unreadable},
	}
	for _, c := range cases {
		name := c.file + "/" + map[string]string{"{garbage": "garbage", "[{garbage": "garbage", "": "empty", "dir": "unreadable"}[c.content]
		if strings.Contains(c.content, "null") {
			name = c.file + "/null"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, c.file)
			if c.content == "dir" {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, []byte(c.content), 0o600); err != nil {
				t.Fatal(err)
			}
			h := NewHub()
			var w warnings
			h.Warn = w.add
			err := h.Open(dir)
			h.Flush()
			c.check(t, h, dir, err, w.all())
		})
	}
}

// TestSaveFailures: a data file that can't be written (here a folder stands where its temporary file goes) is in the
// log, once a minute at most however often it fails, and Flush says which files didn't make it, after one more try;
// once the way is free, that try writes what was waiting, without a new change.
func TestSaveFailures(t *testing.T) {
	dir := t.TempDir()
	h := NewHub()
	var w warnings
	h.Warn = w.add
	if err := h.Open(dir); err != nil {
		t.Fatal(err)
	}
	files := []string{"messages.json", "tasks.json", "ledger.json"}
	for _, f := range files {
		if err := os.Mkdir(filepath.Join(dir, f+".tmp"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	change := func(n int) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.messages = append(h.messages, &Message{ID: "m" + string(rune('0'+n)), Text: "hi"})
		h.saveMessagesLocked()
		h.taskNext = n
		h.saveTasksLocked()
		h.ledger = append(h.ledger, &LedgerRow{Day: "2026-11-02", Account: "a1", USD: float64(n)})
		h.saveLedgerLocked()
	}
	change(1)
	h.Flush()
	change(2)
	err := h.Flush()
	if err == nil {
		t.Fatal("Flush should say what couldn't be written")
	}
	for _, f := range files {
		if !strings.Contains(err.Error(), f) {
			t.Errorf("Flush's error should name %s: %v", f, err)
		}
		if exists(filepath.Join(dir, f)) {
			t.Errorf("%s was written through a folder?", f)
		}
	}
	warned := w.all()
	if len(warned) != len(files) {
		t.Fatalf("want one line per file, however many writes failed: %q", warned)
	}
	for _, f := range files {
		if !slices.ContainsFunc(warned, func(l string) bool { return strings.Contains(l, filepath.Join(dir, f)) }) {
			t.Errorf("no line names %s: %q", filepath.Join(dir, f), warned)
		}
	}

	for _, f := range files {
		if err := os.Remove(filepath.Join(dir, f+".tmp")); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.Flush(); err != nil {
		t.Fatalf("with the way free, the last try should write: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "messages.json")); !strings.Contains(got, `"m2"`) {
		t.Errorf("messages.json should hold the latest log: %s", got)
	}
	if got := readFile(t, filepath.Join(dir, "tasks.json")); !strings.Contains(got, `"next": 2`) {
		t.Errorf("tasks.json should hold the latest counter: %s", got)
	}
	again := NewHub()
	if err := again.Open(dir); err != nil || len(again.messages) != 2 || len(again.ledger) != 2 || again.taskNext != 2 {
		t.Fatalf("a restart should find everything: %v, %d messages, %d rows, next %d", err, len(again.messages), len(again.ledger), again.taskNext)
	}
	if len(w.all()) != len(files) {
		t.Errorf("a write that succeeds says nothing: %q", w.all())
	}
}

// TestAccountsWriteFailure: accounts.json that can't be written. Making or deleting an account answers 500 with the
// code data_write_failed (the page translates it; the absolute path stays in the log) and changes nothing; what the
// bridge saves for itself (an employee's folder, its last start) goes to the log instead of vanishing.
func TestAccountsWriteFailure(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub()
	var w warnings
	hub.Warn = w.add
	if err := hub.Open(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = hub.Flush() }) // the error log is written in the background (before the folder goes)
	srv := NewServer(hub, testWeb)
	srv.Key = testKey
	ts := httptest.NewServer(srv)
	defer ts.Close()
	page := client{t: t, base: ts.URL}
	bob, err := hub.CreateAccount("bob")
	if err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(dir, "accounts.json.tmp")
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}

	status, body := page.do("POST", "/api/accounts", map[string]string{"name": "alice"})
	params, _ := body["params"].(map[string]any)
	if status != 500 || body["code"] != "data_write_failed" || params["file"] != "accounts.json" {
		t.Fatalf("want 500 data_write_failed for accounts.json, got %d %v", status, body)
	}
	if text, _ := body["error"].(string); strings.Contains(text, dir) {
		t.Errorf("the answer shouldn't carry the data folder's path: %q", text)
	}
	if hub.nameTaken("alice") {
		t.Error("an account that couldn't be saved should be gone again")
	}
	if status, body := page.do("DELETE", "/api/accounts/"+bob.ID, nil); status != 500 || body["code"] != "data_write_failed" {
		t.Fatalf("deleting: want 500 data_write_failed, got %d %v", status, body)
	}
	if hub.Account(bob.ID) == nil {
		t.Error("an account whose deletion couldn't be saved should stay")
	}
	if n := len(w.all()); n != 2 {
		t.Fatalf("each failed write should be in the log: %q", w.all())
	}

	hub.setLast(bob.ID, LaunchRequest{Dir: dir, Model: "sonnet"})
	warned := w.all()
	if len(warned) != 3 || !strings.Contains(warned[2], filepath.Join(dir, "accounts.json")) {
		t.Fatalf("a failed save of the last start should be in the log: %q", warned)
	}
}

// TestTaskNumberPastBranches: task numbers never collide with the task branches the members' repositories already have
// (another data folder's tasks, or a data folder that was reset). With symphony/T1/bob in bob's repository and
// symphony/T3/rev in the advisor's, a fresh data folder's first task is T4, made on the first try, and the counter
// is kept.
func TestTaskNumberPastBranches(t *testing.T) {
	data := t.TempDir()
	e := newTaskEnv(t, data)
	mine := initRepo(t, e.folder(t, "mine"), map[string]string{"README.md": "hi\n"})
	theirs := initRepo(t, e.folder(t, "theirs"), map[string]string{"README.md": "hi\n"})
	mustGit(t, mine, "branch", "symphony/T1/bob")
	mustGit(t, theirs, "branch", "symphony/T3/rev")
	mustGit(t, theirs, "branch", "symphony/notes") // not a task's
	bob, bobAcc := as(t, e.srv, e.base, "bob")
	bob.ok("POST", "/api/event", Event{Session: "bob-desk", Kind: "session.start", Cwd: mine})

	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Git: true, Title: "t", Members: []TaskMemberRequest{
		{Account: bobAcc.ID}, {Advisor: &AdvisorRequest{Name: "rev", Dir: theirs}}}}))
	if task.ID != "T4" || task.Members[0].Branch != "symphony/T4/bob" {
		t.Fatalf("want task T4 past the repositories' branches, got %s (%s)", task.ID, task.Members[0].Branch)
	}
	e.srv.Hub.Flush()
	var file taskFile
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(data, "tasks.json"))), &file); err != nil || file.Next != 5 {
		t.Fatalf("tasks.json should count on from there: %+v (%v)", file, err)
	}
}
