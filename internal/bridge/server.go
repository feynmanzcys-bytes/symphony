package bridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
)

type Server struct {
	Hub *Hub
	// WaitTimeout is how long one long poll hangs; the mod simply comes back.
	WaitTimeout time.Duration
	// Launcher, when set, lets the page start sessions
	Launcher *Launcher
	// AllowedHosts are the host names a request's Host may carry, against DNS rebinding; only this machine by default
	AllowedHosts []string
	// Window opens a terminal window on this machine (the employee window's "open terminal"); nil means none can be opened
	Window *Window
	// AttachCommand is the command such a window runs to attach to a launch's terminal (symphony attach ...)
	AttachCommand func(launchID string) []string
	// Key is the page key every request but the mod's must show (key.go); NewServer makes one, the bridge sets the one
	// in its data folder, so a page's cookie outlives a restart
	Key string
	// Shutdown stops the bridge, its sessions with it (POST /api/shutdown, which symphony -stop sends); the program
	// sets it, and without it the endpoint answers 501
	Shutdown func()
	// Version is the program's (symphony -version), for the error log's report; LogTail the last lines of its log
	Version string
	LogTail *LogTail

	web    fs.FS
	mux    *http.ServeMux
	themes []Theme // the page's themes, from web (themes.go)
}

var (
	errTokenInvalid   = newError("token_invalid", "no valid account token")
	errLaunchDisabled = newError("launch_disabled", "this bridge can't start sessions from the page (claude not found, or -mod isn't a mod folder; see the bridge's startup log)")
)

// NewServer builds a bridge; web must hold the pages: themes/<name>/{theme.json,index.html,...} (the office page's
// themes), approve.html and approve.js, the shared modules symphony.js, asks.js, i18n.js and ui/*, and locales/*.json.
func NewServer(hub *Hub, web fs.FS) *Server {
	s := &Server{Hub: hub, WaitTimeout: 20 * time.Second, AllowedHosts: []string{"localhost", "127.0.0.1", "::1"}, Key: newKey(),
		web: web, mux: http.NewServeMux(), themes: loadThemes(web)}

	s.mux.HandleFunc("GET /{$}", s.themeIndex)
	s.mux.HandleFunc("GET /t/{theme}/{$}", s.themeRoot)
	s.mux.HandleFunc("GET /t/{theme}/{file...}", s.themeFile)
	s.mux.HandleFunc("GET /api/themes", s.themesAPI)
	s.mux.HandleFunc("GET /approve", s.file("approve.html"))
	s.mux.HandleFunc("GET /approve.js", s.file("approve.js"))
	s.mux.HandleFunc("GET /asks.js", s.file("asks.js"))
	s.mux.HandleFunc("GET /i18n.js", s.file("i18n.js"))
	s.mux.HandleFunc("GET /symphony.js", s.file("symphony.js"))
	s.mux.HandleFunc("GET /ui/{file}", func(w http.ResponseWriter, r *http.Request) { s.serveFile(w, r, "ui/"+r.PathValue("file")) })
	s.mux.HandleFunc("GET /locales/{file}", s.locale)
	s.mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, hub.Snapshot()) })
	s.mux.HandleFunc("GET /api/stream", s.stream)
	s.mux.HandleFunc("GET /api/activity", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": hub.Activity()})
	})
	s.mux.HandleFunc("POST /api/shutdown", s.shutdown)
	s.mux.HandleFunc("GET /api/errors", s.faultsAPI)
	s.mux.HandleFunc("POST /api/errors", s.reportFault)
	s.mux.HandleFunc("DELETE /api/errors", s.clearFaults)
	s.mux.HandleFunc("GET /api/diagnostics", s.diagnostics)
	s.mux.HandleFunc("GET /api/ledger", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{"rows": hub.Ledger()}) })

	// called by the mod, with the account's token (modEndpoint in key.go lists these)
	s.mux.HandleFunc("POST /api/hello", s.hello)
	s.mux.HandleFunc("POST /api/event", s.event)
	s.mux.HandleFunc("POST /api/ask", s.ask)
	s.mux.HandleFunc("GET /api/wait/{id}", s.wait)
	s.mux.HandleFunc("POST /api/cancel/{id}", s.cancel)
	s.mux.HandleFunc("POST /api/settle/{id}", s.settle)
	s.mux.HandleFunc("GET /api/roster", s.roster)
	s.mux.HandleFunc("GET /api/inbox", s.inbox)

	// called by the page: answers, accounts, starting sessions
	s.mux.HandleFunc("POST /api/answer/{id}", s.answer)
	s.mux.HandleFunc("GET /api/asks/{id}/input", s.askInput)
	s.mux.HandleFunc("POST /api/accounts", s.createAccount)
	s.mux.HandleFunc("POST /api/accounts/import", s.importStaff)
	s.mux.HandleFunc("DELETE /api/accounts/{id}", s.deleteAccount)
	s.mux.HandleFunc("GET /api/launch/options", s.launchOptions)
	s.mux.HandleFunc("GET /api/settings", s.settings)
	s.mux.HandleFunc("POST /api/settings", s.settings)
	s.mux.HandleFunc("GET /api/dirs", s.dirs)
	s.mux.HandleFunc("POST /api/launch", s.launch)
	// the employee window: the chat, and the session's terminal
	s.mux.HandleFunc("GET /api/chat/{session}", s.chat)
	s.mux.HandleFunc("POST /api/say/{session}", s.say)
	s.mux.HandleFunc("POST /api/abort/{session}", s.abort)
	s.mux.HandleFunc("POST /api/sessions/{session}/model", s.setModel)
	s.mux.HandleFunc("POST /api/files/{session}", s.uploadFile)
	s.mux.HandleFunc("GET /api/files/{session}/{id}", s.serveUpload)
	s.mux.HandleFunc("GET /api/terminal/{id}/stream", s.termStream)
	s.mux.HandleFunc("POST /api/terminal/{id}/input", s.termInput)
	s.mux.HandleFunc("POST /api/terminal/{id}/resize", s.termResize)
	s.mux.HandleFunc("POST /api/terminal/{id}/open", s.termOpen)
	// tasks in the small meeting rooms
	s.mux.HandleFunc("GET /api/tasks", s.tasks)
	s.mux.HandleFunc("POST /api/tasks", s.createTask)
	s.mux.HandleFunc("POST /api/tasks/{id}/end", s.endTask)
	s.mux.HandleFunc("DELETE /api/tasks/{id}", s.forgetTask)
	s.mux.HandleFunc("POST /api/tasks/{id}/say", s.sayTask)
	s.mux.HandleFunc("GET /api/tasks/{id}/chat", s.taskChat)
	return s
}

// SetLauncher turns on starting sessions from the page; where to find a session's terminal follows
// the same terminal backend.
func (s *Server) SetLauncher(l *Launcher) {
	s.Launcher = l
	s.Hub.mu.Lock()
	s.Hub.AttachHint = l.AttachFor
	s.Hub.terms = l.Terms
	s.Hub.mu.Unlock()
}

// ServeHTTP first refuses two kinds of cross-site requests: a Host that isn't this machine (DNS
// rebinding), and writes another site sends through the browser (an Origin that isn't this site).
// The mod and curl send no Origin. Only this machine may connect at all, and every request but the
// mod's (which carry the account's token) must show the page key (key.go).
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	securityHeaders(w.Header())
	host := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		host = h
	}
	if !slices.Contains(s.AllowedHosts, strings.Trim(host, "[]")) {
		http.Error(w, "host not allowed", http.StatusForbidden)
		return
	}
	if !loopback(r.RemoteAddr) {
		http.Error(w, "only this machine may connect", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if origin := r.Header.Get("Origin"); origin != "" {
			if u, err := url.Parse(origin); err != nil || u.Host != r.Host {
				http.Error(w, "cross-origin request refused", http.StatusForbidden)
				return
			}
		}
	}
	if !modEndpoint(r) {
		// A browser says where a request comes from. The API answers the bridge's own pages and addresses typed in, not a
		// link, redirect or fetch from another site or another port of this machine, which would bring the cookie along
		// (SameSite=Lax lets it ride on a top-level GET, and cookies ignore the port): even a GET makes the bridge do
		// something (/api/dirs reads the folder it names). Programs (curl, attach, the demo, symphony -stop) send no
		// Sec-Fetch-Site; the sign-in (/?key=, opened from a file) and the pages themselves stay reachable.
		// /api/dirs reads any folder it is given, so not even a link opened from outside the browser (a mail, a chat, a
		// terminal: Sec-Fetch-Site none) reaches it; the pages fetch it, they never navigate to it.
		site, mode := r.Header.Get("Sec-Fetch-Site"), r.Header.Get("Sec-Fetch-Mode")
		if strings.HasPrefix(r.URL.Path, "/api/") && (site == "cross-site" || site == "same-site" || (r.URL.Path == "/api/dirs" && mode == "navigate")) {
			http.Error(w, "cross-site request refused", http.StatusForbidden)
			return
		}
		if s.signIn(w, r) {
			return
		}
		if !s.keyOK(r) {
			s.refuse(w, r)
			return
		}
		if watched(r) { // a failure the page is answered with goes into the error log too (faults.go)
			fw := &faultWriter{ResponseWriter: w, r: r}
			s.mux.ServeHTTP(fw, r)
			fw.keep(s.Hub)
			return
		}
	}
	s.mux.ServeHTTP(w, r)
}

// shutdown answers POST /api/shutdown (symphony -stop, showing the page key like every page call) with 202, then stops
// the bridge as SIGTERM does: its sessions end.
func (s *Server) shutdown(w http.ResponseWriter, r *http.Request) {
	if s.Shutdown == nil {
		http.Error(w, "this bridge can't be stopped this way", http.StatusNotImplemented)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go s.Shutdown()
}

// writeJSON answers with v as JSON; when v doesn't encode, the answer is a 500 that says so rather than a 200 cut short.
func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		status = http.StatusInternalServerError
		data, _ = json.Marshal(map[string]string{"error": "can't encode the answer: " + err.Error()})
	}
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}

// errStatus is the HTTP status of errors made on the spot (with params), which errors.Is can't tell apart.
var errStatus = map[string]int{
	"task_member_busy": http.StatusConflict, "task_ended": http.StatusConflict,
	"task_running": http.StatusConflict, "task_unknown": http.StatusNotFound, "task_no_git": http.StatusPreconditionFailed,
	"ask_not_mod_settled": http.StatusForbidden, "ask_not_page_answered": http.StatusForbidden, "task_git_failed": http.StatusInternalServerError, "pty_start_failed": http.StatusInternalServerError, "cmd_unsafe_arg": http.StatusBadRequest, "task_repo_in_use": http.StatusConflict,
	"data_write_failed": http.StatusInternalServerError, "launch_still_running": http.StatusConflict,
	"bot_home_failed": http.StatusInternalServerError, "file_too_big": http.StatusRequestEntityTooLarge, "file_unknown": http.StatusNotFound,
	"files_nowhere": http.StatusInternalServerError,
}

func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if e := (*Error)(nil); errors.As(err, &e) && errStatus[e.Code] != 0 {
		status = errStatus[e.Code]
	}
	switch {
	case errors.Is(err, ErrUnknownAsk), errors.Is(err, ErrUnknownAccount):
		status = http.StatusNotFound
	case errors.Is(err, ErrSettled), errors.Is(err, ErrAccountBusy), errors.Is(err, ErrNameTaken), errors.Is(err, ErrSessionOffline), errors.Is(err, ErrTermEnded):
		status = http.StatusConflict
	case errors.Is(err, ErrTermUnknown):
		status = http.StatusNotFound
	case errors.Is(err, ErrSessionGone):
		status = http.StatusGone
	case errors.Is(err, ErrWrongAccount):
		status = http.StatusForbidden
	}
	writeJSON(w, status, errorBody(err))
}

// authed finds the account from Authorization: Bearer <token>; otherwise it answers 401 and the mod
// falls back to the terminal.
func (s *Server) authed(w http.ResponseWriter, r *http.Request) (*Account, bool) {
	token := strings.TrimPrefix(r.Header.Get("authorization"), "Bearer ")
	acc := s.Hub.AccountByToken(token)
	if acc == nil {
		writeJSON(w, http.StatusUnauthorized, errorBody(errTokenInvalid))
		return nil, false
	}
	return acc, true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody(err))
		return false
	}
	return true
}

func (s *Server) event(w http.ResponseWriter, r *http.Request) {
	acc, ok := s.authed(w, r)
	if !ok {
		return
	}
	var ev Event
	if !decode(w, r, &ev) {
		return
	}
	if ev.Session == "" {
		writeJSON(w, http.StatusBadRequest, errorBody(newError("event_no_session", "missing session")))
		return
	}
	fresh, err := s.Hub.Report(ev, acc)
	if err != nil {
		writeErr(w, err)
		return
	}
	res := map[string]any{"ok": true, "account": acc.Name}
	if fresh {
		res["fresh"] = true // the bridge didn't know the session: the mod reports its tools again
	}
	writeJSON(w, 200, res)
}

func (s *Server) ask(w http.ResponseWriter, r *http.Request) {
	acc, ok := s.authed(w, r)
	if !ok {
		return
	}
	var req AskRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Session == "" || (req.Kind != "permission" && req.Kind != "question" && req.Kind != "elicitation") {
		writeJSON(w, http.StatusBadRequest, errorBody(newError("ask_bad_request", "missing session or unknown kind")))
		return
	}
	id, err := s.Hub.CreateAsk(req, acc)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"id": id})
}

func (s *Server) wait(w http.ResponseWriter, r *http.Request) {
	acc, ok := s.authed(w, r)
	if !ok {
		return
	}
	status, answer, err := s.Hub.Wait(r.PathValue("id"), acc.ID, s.WaitTimeout, r.Context().Done())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"status": status, "answer": answer})
}

func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	acc, ok := s.authed(w, r)
	if !ok {
		return
	}
	if err := s.Hub.Settle(r.PathValue("id"), acc.ID, AskCancelled, nil); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// settle is the mod saying what became of an ask the page can't answer, an MCP server's form the session's terminal
// answered: {action, content}.
func (s *Server) settle(w http.ResponseWriter, r *http.Request) {
	acc, ok := s.authed(w, r)
	if !ok {
		return
	}
	var answer map[string]any
	if !decode(w, r, &answer) {
		return
	}
	if err := s.Hub.SettleByMod(r.PathValue("id"), acc.ID, answer); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// roster is who else is in the office, for a session that tells its model (the mod's office context): the other
// sessions online, by the name SendMessage reaches them by.
func (s *Server) roster(w http.ResponseWriter, r *http.Request) {
	acc, ok := s.authed(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, s.Hub.Roster(r.URL.Query().Get("session"), acc.ID))
}

func (s *Server) answer(w http.ResponseWriter, r *http.Request) {
	var answer map[string]any
	if !decode(w, r, &answer) {
		return
	}
	if err := s.Hub.AnswerByPage(r.PathValue("id"), answer); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// askInput answers a pending ask's whole input: the page's preview may have cut long strings (inputTruncated).
func (s *Server) askInput(w http.ResponseWriter, r *http.Request) {
	input, err := s.Hub.AskInput(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"input": input})
}

// createAccount creates an account and returns its token once (for starting a session by hand:
// SYMPHONY_TOKEN=<token>).
func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Look *Look  `json:"look"`
	}
	if !decode(w, r, &req) {
		return
	}
	acc, err := s.Hub.CreateAccountLook(req.Name, req.Look)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, acc)
}

// importStaff makes and changes employees in one go (POST /api/accounts/import, the settings' employees page):
// {employees: [{name, dir?, model?, effort?, permissionMode?, context?}]}, all of it or none (Hub.ImportStaff).
func (s *Server) importStaff(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Employees []StaffEntry `json:"employees"`
	}
	if !decode(w, r, &req) {
		return
	}
	out, err := s.Hub.ImportStaff(req.Employees)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	if err := s.Hub.DeleteAccount(r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch, unsubscribe := s.Hub.Subscribe()
	defer unsubscribe()

	w.Header().Set("content-type", "text/event-stream")
	w.Header().Set("cache-control", "no-cache")
	w.WriteHeader(200)
	// once the stream ends (the hub lets a page go that fell behind), the EventSource comes back a second later
	if _, err := fmt.Fprint(w, "retry: 1000\n\n"); err != nil {
		return
	}
	flusher.Flush()

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	// A burst's last messages can wait in the browser (Chrome's, at least) until more bytes come down the stream after
	// the page has worked through the rest, which in a quiet office is the keepalive, up to 15 s later: the page saw a
	// long burst's tail only then. So after a message the stream writes a comment soon (streamNudge), then every second
	// while the page may still be working (streamNudgeFor).
	nudge := time.NewTimer(time.Hour)
	nudge.Stop()
	defer nudge.Stop()
	var last time.Time // the last message's
	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return // let go: the page comes back for a full snapshot
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", msg.JSON()); err != nil {
				return
			}
			last = time.Now()
			nudge.Reset(streamNudge)
		case <-nudge.C:
			if _, err := fmt.Fprint(w, ":\n\n"); err != nil {
				return
			}
			if time.Since(last) < streamNudgeFor {
				nudge.Reset(time.Second)
			}
		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
		}
		flusher.Flush()
	}
}

// After a message the stream writes a comment streamNudge later when nothing else came, then every second until
// streamNudgeFor has passed since the message (stream).
const (
	streamNudge    = 250 * time.Millisecond
	streamNudgeFor = 10 * time.Second
)

func (s *Server) file(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.serveFile(w, r, name) }
}

// contentTypes are the types of the pages' files, by extension; anything else is served as what the OS says.
var contentTypes = map[string]string{
	".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".mjs": "text/javascript; charset=utf-8",
	".json": "application/json; charset=utf-8", ".css": "text/css; charset=utf-8", ".svg": "image/svg+xml",
	".png": "image/png", ".ico": "image/x-icon", ".woff2": "font/woff2", ".txt": "text/plain; charset=utf-8",
}

// serveFile serves one of the pages' files from web (a name as fs.FS has it, / separated).
func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	data, err := fs.ReadFile(s.web, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// no-cache means "ask again": with the ETag an unchanged page is a 304, not a download
	sum := sha256.Sum256(data)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("etag", etag)
	w.Header().Set("cache-control", "no-cache")
	if strings.Contains(r.Header.Get("if-none-match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	ctype := contentTypes[strings.ToLower(path.Ext(name))]
	if ctype == "" {
		ctype = mime.TypeByExtension(path.Ext(name))
	}
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("content-type", ctype)
	_, _ = w.Write(data)
}

// locale serves the page's translations, locales/<lang>.json, for the supported languages only.
func (s *Server) locale(w http.ResponseWriter, r *http.Request) {
	lang, ok := strings.CutSuffix(r.PathValue("file"), ".json")
	if !ok || !slices.Contains(i18n.Langs, lang) {
		http.NotFound(w, r)
		return
	}
	s.file("locales/"+lang+".json")(w, r)
}

// ---------- starting sessions from the page ----------

func (s *Server) launchOptions(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	settings := s.Hub.Settings()
	trustHint, terminal := false, ""
	var mod *ModStatus
	if s.Launcher != nil {
		trustHint, terminal = s.Launcher.Term.TrustHint(), s.Launcher.Term.Name()
		m := s.Launcher.Mod(r.Context())
		mod = &m
	}
	writeJSON(w, 200, map[string]any{
		"enabled":         mod != nil && mod.OK,
		"mod":             mod, // whether sessions will have the mod; when not, its code says why and fix how to install it
		"terminal":        terminal,
		"trustHint":       trustHint, // the first start in a new folder needs a click on "trust" in the window that opens
		"models":          launchModels,
		"efforts":         launchEfforts,
		"permissionModes": launchPermissionModes,
		"resumes":         launchResumes,
		"home":            home,
		"botHome":         settings.BotHome,        // where an employee started without a folder gets its own
		"defaults":        settings.LaunchDefaults, // what a start begins with (the settings: model, effort, permissionMode, context)
		"recentDirs":      s.Hub.recentDirs(8),
	})
}

// settings answers the bridge's settings in effect (GET /api/settings), and changes them (POST, with the fields to
// change: botHome "" goes back to the default; model, effort and permissionMode "" are Claude Code's own).
func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, 200, s.Hub.Settings())
		return
	}
	var req SettingsChange
	if !decode(w, r, &req) {
		return
	}
	view, err := s.Hub.ChangeSettings(req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, view)
}

func (s *Server) dirs(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path, _ = os.UserHomeDir()
	}
	dirs, parent, err := ListDirs(path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody(err))
		return
	}
	writeJSON(w, 200, map[string]any{"path": filepath.Clean(path), "parent": parent, "dirs": dirs, "roots": Roots(runtime.GOOS)})
}

// launchReady says whether sessions can be started from the page; when not, it answers why: no launcher (501), or no
// mod, without which a session could never reach the office (412, with the command that installs it).
func (s *Server) launchReady(w http.ResponseWriter, r *http.Request) bool {
	if s.Launcher == nil {
		writeJSON(w, http.StatusNotImplemented, errorBody(errLaunchDisabled))
		return false
	}
	if mod := s.Launcher.Mod(r.Context()); !mod.OK {
		body := map[string]any{"error": mod.Error, "code": mod.Code, "params": mod.Params, "fix": mod.Fix}
		if mod.Fix != "" {
			body["error"] = mod.Error + "; run first: " + mod.Fix
		}
		writeJSON(w, http.StatusPreconditionFailed, body)
		return false
	}
	return true
}

// launch starts a session for an account: with account it puts that (offline) account back to work,
// without it a new account named name is created and takes the lowest free seat.
func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	if !s.launchReady(w, r) {
		return
	}
	var req LaunchRequest
	if !decode(w, r, &req) {
		return
	}
	req, err := normalizeLaunch(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody(err))
		return
	}

	var acc *Account
	created := false
	if req.Account != "" {
		// an advisor has no desk to work at
		if acc = s.Hub.Account(req.Account); acc == nil || acc.Advisor {
			writeErr(w, ErrUnknownAccount)
			return
		}
	} else {
		if acc, err = s.Hub.CreateAccountLook(req.Name, req.Look); err != nil {
			writeErr(w, err)
			return
		}
		created = true
	}
	req.Look = nil // (the face is the account's, never part of what the next start fills in)
	// one start at a time: the desk is held until the launch is in the hub (a failed start lets go of it)
	release, err := s.Hub.reserveDesk(acc.ID)
	if err != nil {
		if created {
			_ = s.Hub.DeleteAccount(acc.ID)
		}
		writeErr(w, err)
		return
	}
	defer release()
	req.Account, req.Name = acc.ID, acc.Name
	// no folder given: the employee's own, in the bot home (made now the first time: a new git repository)
	made := ""
	if req.Dir == "" {
		dir, fresh, err := s.Hub.botFolder(acc.ID)
		if err != nil {
			if created {
				_ = s.Hub.DeleteAccount(acc.ID)
			}
			writeErr(w, err)
			return
		}
		req.Dir = dir
		if fresh {
			made = dir
		}
	}

	id := newID()
	env := map[string]string{"SYMPHONY_BRIDGE": s.Launcher.BridgeURL, "SYMPHONY_LAUNCH": id, "SYMPHONY_TOKEN": acc.Token}
	if req.Lang != "" {
		env["SYMPHONY_LANG"] = req.Lang // the mod speaks the page's language
	}
	if req.Context {
		env["SYMPHONY_CONTEXT"] = "1" // the session tells its model who else is in the office
	}
	l := &Launch{ID: id, Name: acc.Name, Dir: req.Dir, Account: acc.ID,
		Model: req.Model, Effort: req.Effort, Status: LaunchStarting, Time: time.Now()}
	if err := s.Launcher.Start(l, req, env); err != nil {
		if created {
			_ = s.Hub.DeleteAccount(acc.ID)
			if made != "" {
				_ = os.RemoveAll(made) // made for the account just dropped, and nothing ran in it
			}
		}
		if e := (*Error)(nil); errors.As(err, &e) {
			writeErr(w, err) // a coded error keeps its status: 400 cmd_unsafe_arg, 500 pty_start_failed
		} else {
			writeJSON(w, http.StatusInternalServerError, errorBody(err))
		}
		return
	}
	s.Hub.setLast(acc.ID, req)
	resp := *l // the hub updates l in the background once it has it; the page gets a copy
	s.Hub.addLaunch(l)
	go s.Launcher.watch(s.Hub, l)
	writeJSON(w, 200, resp)
}
