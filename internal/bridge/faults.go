package bridge

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
)

// The error log: what went wrong while the office ran, kept so that whoever uses it can hand it to whoever helps them
// (the page's settings: copy or download a report). The bridge keeps its own warnings (a file it couldn't write, a
// push it couldn't encode, a task's git work that failed), the failures it answered a page with (every 5xx, and every
// failed start of a session or a task but for a form left incomplete), the sessions' API errors and bad exits, the
// launches that never came online, and what the pages report of their own (a script's error, a promise nobody caught,
// something the content security policy blocked: POST /api/errors). Each is kept once and counted when it comes again;
// the newest keepFaults stay, in errors.json in the data folder. Nothing secret goes in: runs of 48 or more hex digits
// (the page key, the accounts' tokens), a URL's user and password and the bearer of a token are taken out, and the
// home folder reads ~. No prompt and no chat goes in either: only what the error itself says.

const (
	keepFaults          = 200
	faultTextMax        = 8 << 10
	faultDetailMax      = 8 << 10
	pageFaultsPerMinute = 30 // what all the pages together may report in a minute; more is dropped (a loop that throws)
)

// Fault is one thing that went wrong, as the error log keeps it.
type Fault struct {
	ID     string    `json:"id"`
	Time   time.Time `json:"time"` // when it first happened
	Last   time.Time `json:"last"` // when it last happened: Count times in all
	Count  int       `json:"count"`
	Source string    `json:"source"`           // bridge | api | task | launch | session | page
	Code   string    `json:"code,omitempty"`   // the error's code, the log line's key, or the page's kind (js, rejection, csp)
	Text   string    `json:"text"`             // what went wrong: the bridge's own words in English, a page's as its browser said it
	Detail string    `json:"detail,omitempty"` // where: the request and its answer, the script's place and stack, the page
	Name   string    `json:"name,omitempty"`   // the employee (or the task's member) it happened to
	Task   string    `json:"task,omitempty"`
}

// FaultSummary is what every snapshot says of the error log: how much it keeps and when the newest one happened (a
// page marks its settings button while that is newer than what its user has seen).
type FaultSummary struct {
	Count int        `json:"count"`
	Last  *time.Time `json:"last,omitempty"`
}

type faultLog struct {
	mu   sync.Mutex
	list []*Fault // oldest first, by when each last happened
	path string   // errors.json; empty: nothing goes to disk (tests)
	save saver
	page []time.Time // when the pages reported, over the last minute
	home string      // the home folder, which the log writes as ~
	// what is still to be told to the pages, in the order it was kept, and whether a goroutine tells them now
	unsent  []faultPush
	sending bool
}

type faultPush struct {
	fault Fault
	sum   FaultSummary
}

var (
	hexRun   = regexp.MustCompile(`[0-9A-Fa-f]{48,}`) // the page key (64), a token (48); a git commit (40) stays
	userinfo = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/\s@]+@`)
	bearer   = regexp.MustCompile(`(?i)\b(bearer\s+)\S+`)
)

// redact takes out of s what must not leave this machine with a report: keys and tokens, a URL's user and password,
// the bearer of a token; the home folder becomes ~.
func (l *faultLog) redact(s string) string {
	s = hexRun.ReplaceAllString(s, "‹redacted›")
	s = userinfo.ReplaceAllString(s, "${1}‹redacted›@")
	s = bearer.ReplaceAllString(s, "${1}‹redacted›")
	if len(l.home) < 2 {
		return s
	}
	s = tildeHome(s, l.home)
	if runtime.GOOS == "windows" {
		s = tildeHome(s, strings.ReplaceAll(l.home, `\`, `/`))
	}
	return s
}

// tildeHome writes the home folder in s as ~: the folder itself, not a longer name that starts like it (/home/ann in
// /home/anna).
func tildeHome(s, home string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, home)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		end := i + len(home)
		if r, _ := utf8.DecodeRuneInString(s[end:]); end < len(s) && (r == '_' || r == '-' || r == '.' || isWordRune(r)) {
			b.WriteString(s[:end])
		} else {
			b.WriteString(s[:i] + "~")
		}
		s = s[end:]
	}
}

func isWordRune(r rune) bool {
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 127
}

// cutText keeps the first n bytes of s (at a rune boundary), saying how much was left out.
func cutText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "\n… [" + itoa(len(s)-cut) + " more bytes]"
}

func (l *faultLog) summaryLocked() FaultSummary {
	sum := FaultSummary{Count: len(l.list)}
	if n := len(l.list); n > 0 {
		last := l.list[n-1].Last
		sum.Last = &last
	}
	return sum
}

func (l *faultLog) saveLocked() {
	if l.path == "" {
		return
	}
	l.save.putFunc(l.path, func() ([]byte, error) {
		l.mu.Lock()
		list := make([]Fault, len(l.list))
		for i, f := range l.list {
			list[i] = *f
		}
		l.mu.Unlock()
		return json.Marshal(list)
	})
}

// fault keeps one thing that went wrong (redacted, cut to size), counting it when the same one comes again, and tells
// the pages. It takes only the error log's own lock: it is called with the hub's lock held and without it.
func (h *Hub) fault(f Fault) {
	l := &h.faults
	f.Text = cutText(l.redact(strings.TrimSpace(f.Text)), faultTextMax)
	f.Detail = cutText(l.redact(strings.TrimSpace(f.Detail)), faultDetailMax)
	f.Name, f.Task, f.Code, f.Source = cutRunes(f.Name, 80), cutRunes(f.Task, 20), cutRunes(f.Code, 60), cutRunes(f.Source, 20)
	if f.Text == "" {
		return
	}
	now := h.now()
	l.mu.Lock()
	var kept *Fault
	for i := len(l.list) - 1; i >= 0; i-- {
		if o := l.list[i]; o.Source == f.Source && o.Code == f.Code && o.Text == f.Text && o.Name == f.Name && o.Task == f.Task {
			kept = o
			l.list = append(l.list[:i], l.list[i+1:]...)
			break
		}
	}
	if kept != nil {
		kept.Count++
		kept.Last = now
		if f.Detail != "" {
			kept.Detail = f.Detail
		}
	} else {
		f.ID, f.Time, f.Last, f.Count = newID(), now, now, 1
		kept = &f
	}
	l.list = append(l.list, kept)
	if len(l.list) > keepFaults {
		l.list = l.list[len(l.list)-keepFaults:]
	}
	l.unsent = append(l.unsent, faultPush{*kept, l.summaryLocked()})
	l.saveLocked()
	start := !l.sending
	l.sending = true
	l.mu.Unlock()
	if start {
		go h.sendFaults()
	}
}

// sendFaults tells the pages of the error log's new entries, one goroutine at a time, so they hear them in the order
// they were kept (an event with no snapshot each: the next snapshot carries the summary too). It can't be done where
// fault is called, which may hold the hub's lock.
func (h *Hub) sendFaults() {
	l := &h.faults
	for {
		l.mu.Lock()
		todo := l.unsent
		l.unsent = nil
		if len(todo) == 0 {
			l.sending = false
			l.mu.Unlock()
			return
		}
		l.mu.Unlock()
		h.mu.Lock()
		for _, f := range todo {
			if len(h.subs) == 0 {
				break
			}
			if p := h.pushLocked(map[string]any{"type": "fault", "fault": f.fault, "faults": f.sum}); p != nil {
				h.deliverLocked(p)
			}
		}
		h.mu.Unlock()
	}
}

// warn writes a line to the bridge's log in the bridge's language (Warn) and keeps it in the error log in English.
func (h *Hub) warn(key string, args ...string) {
	if h.Warn != nil {
		h.Warn(i18n.T(key, args...))
	}
	params := map[string]string{}
	for i := 0; i+1 < len(args); i += 2 {
		params[args[i]] = args[i+1]
	}
	h.fault(Fault{Source: "bridge", Code: strings.TrimPrefix(key, "log."), Text: i18n.TLang("en", key, params)})
}

func homeDir() string { home, _ := os.UserHomeDir(); return home }

// engineOf says what a session runs on, for the error log: Claude Code's version, the mod's, the model.
func engineOf(s *Session) string {
	var parts []string
	if s.Engine != nil && s.Engine.Version != "" {
		parts = append(parts, "Claude Code "+s.Engine.Version)
	}
	if s.Mod != "" {
		parts = append(parts, "mod "+s.Mod)
	}
	if s.Model != "" {
		parts = append(parts, s.Model)
	}
	return strings.Join(parts, " · ")
}

// openFaults reads errors.json (a damaged one is set aside, as the message log is) and keeps writing it.
func (h *Hub) openFaults(dir string) error {
	path := filepath.Join(dir, "errors.json")
	list, err := loadLog[Fault](h, path)
	if err != nil {
		return err
	}
	l := &h.faults
	l.mu.Lock()
	defer l.mu.Unlock()
	// what this bridge kept since it started (its warnings while it read the other files) goes after what was kept
	kept := append(list, l.list...)
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Last.Before(kept[j].Last) })
	if len(kept) > keepFaults {
		kept = kept[len(kept)-keepFaults:]
	}
	l.list, l.path, l.save.warn = kept, path, h.warnWrite
	if len(l.list) > 0 && len(list) != len(l.list) {
		l.saveLocked()
	}
	return nil
}

// Faults is the error log, oldest first.
func (h *Hub) Faults() []Fault {
	l := &h.faults
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Fault, len(l.list))
	for i, f := range l.list {
		out[i] = *f
	}
	return out
}

// ClearFaults empties the error log.
func (h *Hub) ClearFaults() {
	l := &h.faults
	l.mu.Lock()
	l.list = nil
	sum := l.summaryLocked()
	l.saveLocked()
	l.mu.Unlock()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.broadcastLocked(map[string]any{"type": "fault", "faults": sum})
}

func (h *Hub) faultSummary() FaultSummary {
	l := &h.faults
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.summaryLocked()
}

// pageFaultKinds are what a page may report of its own: a script's error, a promise nobody caught, a resource the
// content security policy blocked, a call the bridge didn't answer as it should, something the page itself noticed.
var pageFaultKinds = map[string]bool{"js": true, "rejection": true, "csp": true, "api": true, "ui": true}

// PageFault keeps what a page reported of its own; false when the pages reported more than pageFaultsPerMinute in the
// last minute (the rest is dropped: a script that throws in a loop).
func (h *Hub) PageFault(f Fault) bool {
	l := &h.faults
	now := h.now()
	l.mu.Lock()
	keep := l.page[:0]
	for _, t := range l.page {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	l.page = keep
	if len(l.page) >= pageFaultsPerMinute {
		l.mu.Unlock()
		return false
	}
	l.page = append(l.page, now)
	l.mu.Unlock()
	f.Source = "page"
	h.fault(f)
	return true
}

// ---------- the API ----------

// GET /api/errors: the error log, oldest first.
func (s *Server) faultsAPI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"errors": s.Hub.Faults()})
}

// POST /api/errors: a page reports something that went wrong in it: {kind, text, detail, theme, page}.
func (s *Server) reportFault(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind   string `json:"kind"`
		Text   string `json:"text"`
		Detail string `json:"detail"`
		Theme  string `json:"theme"`
		Page   string `json:"page"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody(err))
		return
	}
	if !pageFaultKinds[req.Kind] || strings.TrimSpace(req.Text) == "" {
		writeJSON(w, http.StatusBadRequest, errorBody(newError("fault_bad_request", "a report needs a known kind and a text")))
		return
	}
	detail := strings.TrimSpace(req.Detail)
	if where := strings.TrimSpace(strings.Join([]string{cutRunes(req.Theme, 40), cutRunes(req.Page, 200)}, " ")); where != "" {
		detail = strings.TrimSpace(detail + "\npage: " + where)
	}
	if !s.Hub.PageFault(Fault{Code: req.Kind, Text: req.Text, Detail: detail}) {
		writeJSON(w, http.StatusTooManyRequests, errorBody(newError("fault_too_many", "the pages reported too much in the last minute")))
		return
	}
	writeJSON(w, 200, map[string]any{})
}

// DELETE /api/errors: the error log emptied.
func (s *Server) clearFaults(w http.ResponseWriter, r *http.Request) {
	s.Hub.ClearFaults()
	writeJSON(w, 200, map[string]any{})
}

// GET /api/diagnostics: what a report says besides the error log: this bridge (its version, the system it runs on, how
// long it has run, its data folder), whether it can start sessions, the Claude Code and mod versions its sessions
// reported (counted, without names), the themes, and the last lines of its log, with the error log itself.
func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) {
	h := s.Hub
	type count map[string]int
	sessions := map[string]any{}
	engines, mods, surfaces := count{}, count{}, count{}
	online, total := 0, 0
	tasks := count{}
	h.mu.Lock()
	for _, ses := range h.sessions {
		total++
		if ses.State == StateEnded {
			continue
		}
		online++
		if ses.Engine != nil && ses.Engine.Version != "" {
			engines[ses.Engine.Version]++
		}
		if ses.Mod != "" {
			mods[ses.Mod]++
		}
		if ses.Surface != "" {
			surfaces[ses.Surface]++
		}
	}
	for _, t := range h.tasks {
		if t.Status != TaskEnded {
			tasks[t.Status]++
		}
	}
	accounts, started, dataDir := len(h.accounts), h.started, h.dataDir
	h.mu.Unlock()
	sessions["online"], sessions["known"], sessions["engines"], sessions["mods"], sessions["surfaces"] = online, total, engines, mods, surfaces
	launch := map[string]any{"enabled": false}
	if s.Launcher != nil {
		m := s.Launcher.Mod(r.Context())
		launch["enabled"], launch["mod"], launch["terminal"] = m.OK, m, s.Launcher.Term.Name()
	}
	themes := []string{}
	for _, th := range s.themes {
		themes = append(themes, th.Name)
	}
	var lines []string
	if s.LogTail != nil {
		lines = s.LogTail.Lines()
	}
	l := &h.faults
	if abs, err := filepath.Abs(dataDir); err == nil && dataDir != "" {
		dataDir = abs
	}
	writeJSON(w, 200, map[string]any{
		"version":  s.Version,
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
		"go":       runtime.Version(),
		"protocol": Protocol,
		"started":  started,
		"now":      h.now(),
		"lang":     i18n.Lang(),
		"dataDir":  l.redact(dataDir),
		"accounts": accounts,
		"sessions": sessions,
		"tasks":    tasks,
		"launch":   launch,
		"themes":   themes,
		"log":      lines,
		"errors":   h.Faults(),
	})
}

// ---------- failures the bridge answers a page with ----------

// formErrors are failed starts the error log leaves out: a form left incomplete, a desk already in use.
var formErrors = map[string]bool{
	"task_empty_title": true, "task_too_few": true, "task_too_many": true, "task_member_dup": true, "launch_bad_option": true,
	"account_bad_name": true, "account_name_taken": true, "launch_dir_not_absolute": true, "account_busy": true,
	"task_member_busy": true, "launch_still_running": true,
}

// faultWriter watches the answer to a page's call: one that fails as the error log keeps (faultWorthy) is copied
// (its first 16 KiB) to be kept once it is written.
type faultWriter struct {
	http.ResponseWriter
	r      *http.Request
	status int
	body   bytes.Buffer
}

func (fw *faultWriter) WriteHeader(status int) {
	if fw.status == 0 {
		fw.status = status
	}
	fw.ResponseWriter.WriteHeader(status)
}

func (fw *faultWriter) Write(b []byte) (int, error) {
	if fw.status == 0 {
		fw.status = http.StatusOK
	}
	if fw.status >= 400 && fw.body.Len() < 16<<10 {
		fw.body.Write(b[:min(len(b), 16<<10-fw.body.Len())])
	}
	return fw.ResponseWriter.Write(b)
}

func (fw *faultWriter) Flush() {
	if f, ok := fw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (fw *faultWriter) Unwrap() http.ResponseWriter { return fw.ResponseWriter }

// starts are the page's calls that start something, whose every failure but a form's the error log keeps
func startCall(r *http.Request) bool {
	return r.Method == http.MethodPost && (r.URL.Path == "/api/tasks" || r.URL.Path == "/api/launch")
}

// keep puts a failed answer in the error log: every 5xx, and a failed start of a task or a session but a form's.
func (fw *faultWriter) keep(h *Hub) {
	if fw.status < 400 || fw.status < 500 && !startCall(fw.r) {
		return
	}
	var body struct {
		Error  string            `json:"error"`
		Code   string            `json:"code"`
		Params map[string]string `json:"params"`
	}
	raw := fw.body.Bytes()
	if json.Unmarshal(raw, &body) != nil || body.Error == "" {
		body.Error = strings.TrimSpace(string(raw))
	}
	if formErrors[body.Code] {
		return
	}
	source := "api"
	switch {
	case strings.HasPrefix(fw.r.URL.Path, "/api/tasks"):
		source = "task"
	case strings.HasPrefix(fw.r.URL.Path, "/api/launch"):
		source = "launch"
	}
	detail := fw.r.Method + " " + fw.r.URL.Path + " → " + itoa(fw.status)
	if len(body.Params) > 0 {
		keys := make([]string, 0, len(body.Params))
		for k := range body.Params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			detail += "\n" + k + ": " + body.Params[k]
		}
	}
	if body.Error == "" {
		body.Error = http.StatusText(fw.status)
	}
	h.fault(Fault{Source: source, Code: body.Code, Text: body.Error, Detail: detail})
}

// watched says whether a page's call has its answer watched: the API's, but its streams (they stay open; a failed one
// says nothing the error log needs) and the error log's own calls.
func watched(r *http.Request) bool {
	p := r.URL.Path
	return strings.HasPrefix(p, "/api/") && p != "/api/stream" && p != "/api/errors" && p != "/api/diagnostics" &&
		!(strings.HasPrefix(p, "/api/terminal/") && strings.HasSuffix(p, "/stream"))
}

// ---------- the bridge's log, its last lines ----------

// LogTail keeps the last lines written to it (the program writes its log through it, beside the terminal or the log
// file), redacted, for a report.
type LogTail struct {
	mu      sync.Mutex
	lines   []string
	partial []byte
	max     int
	red     faultLog // (only its redaction: the home folder)
}

// NewLogTail keeps the last n lines.
func NewLogTail(n int) *LogTail {
	return &LogTail{max: n, red: faultLog{home: homeDir()}}
}

func (t *LogTail) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.partial = append(t.partial, b...)
	for {
		i := bytes.IndexByte(t.partial, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(t.partial[:i]), "\r")
		t.partial = t.partial[i+1:]
		t.lines = append(t.lines, cutText(t.red.redact(line), 2048))
		if len(t.lines) > t.max {
			t.lines = t.lines[len(t.lines)-t.max:]
		}
	}
	if len(t.partial) > 64<<10 { // a line that never ends
		t.partial = t.partial[:0]
	}
	return len(b), nil
}

// Lines are the last lines kept, oldest first.
func (t *LogTail) Lines() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string{}, t.lines...)
}
