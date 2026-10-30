package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
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

	web fs.FS
	mux *http.ServeMux
}

var (
	errTokenInvalid   = newError("token_invalid", "no valid account token")
	errLaunchDisabled = newError("launch_disabled", "this bridge can't start sessions from the page (no claude or no usable terminal; see the bridge's startup log)")
)

// NewServer builds a bridge; web must hold office.html, approve.html, asks.js, i18n.js and locales/*.json.
func NewServer(hub *Hub, web fs.FS) *Server {
	s := &Server{Hub: hub, WaitTimeout: 20 * time.Second, AllowedHosts: []string{"localhost", "127.0.0.1", "::1"},
		web: web, mux: http.NewServeMux()}

	s.mux.HandleFunc("GET /{$}", s.file("office.html"))
	s.mux.HandleFunc("GET /approve", s.file("approve.html"))
	s.mux.HandleFunc("GET /asks.js", s.file("asks.js"))
	s.mux.HandleFunc("GET /i18n.js", s.file("i18n.js"))
	s.mux.HandleFunc("GET /locales/{file}", s.locale)
	s.mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, hub.Snapshot()) })
	s.mux.HandleFunc("GET /api/stream", s.stream)

	// called by the mod, with the account's token
	s.mux.HandleFunc("POST /api/event", s.event)
	s.mux.HandleFunc("POST /api/ask", s.ask)
	s.mux.HandleFunc("GET /api/wait/{id}", s.wait)
	s.mux.HandleFunc("POST /api/cancel/{id}", s.cancel)

	// called by the page: answers, accounts, starting sessions
	s.mux.HandleFunc("POST /api/answer/{id}", s.answer)
	s.mux.HandleFunc("POST /api/accounts", s.createAccount)
	s.mux.HandleFunc("DELETE /api/accounts/{id}", s.deleteAccount)
	s.mux.HandleFunc("GET /api/launch/options", s.launchOptions)
	s.mux.HandleFunc("GET /api/dirs", s.dirs)
	s.mux.HandleFunc("POST /api/launch", s.launch)
	return s
}

// SetLauncher turns on starting sessions from the page; where to find a session's terminal follows
// the same terminal backend.
func (s *Server) SetLauncher(l *Launcher) {
	s.Launcher = l
	s.Hub.mu.Lock()
	s.Hub.AttachHint = l.Term.Attach
	s.Hub.mu.Unlock()
}

// ServeHTTP first refuses two kinds of cross-site requests: a Host that isn't this machine (DNS
// rebinding), and writes another site sends through the browser (an Origin that isn't this site).
// The mod and curl send no Origin.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		host = h
	}
	if !slices.Contains(s.AllowedHosts, strings.Trim(host, "[]")) {
		http.Error(w, "host not allowed", http.StatusForbidden)
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
	s.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, ErrUnknownAsk), errors.Is(err, ErrUnknownAccount):
		status = http.StatusNotFound
	case errors.Is(err, ErrSettled), errors.Is(err, ErrAccountBusy), errors.Is(err, ErrNameTaken):
		status = http.StatusConflict
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
	if err := s.Hub.Apply(ev, acc); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "account": acc.Name})
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
	if req.Session == "" || (req.Kind != "permission" && req.Kind != "question") {
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

func (s *Server) answer(w http.ResponseWriter, r *http.Request) {
	var answer map[string]any
	if !decode(w, r, &answer) {
		return
	}
	if err := s.Hub.Settle(r.PathValue("id"), "", AskDone, answer); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// createAccount creates an account and returns its token once (for starting a session by hand:
// SYMPHONY_TOKEN=<token>).
func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &req) {
		return
	}
	acc, err := s.Hub.CreateAccount(req.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, acc)
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
	flusher.Flush()

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
		}
		flusher.Flush()
	}
}

func (s *Server) file(name string) http.HandlerFunc {
	ctype := "text/html; charset=utf-8"
	switch {
	case strings.HasSuffix(name, ".js"):
		ctype = "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".json"):
		ctype = "application/json; charset=utf-8"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(s.web, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("content-type", ctype)
		w.Header().Set("cache-control", "no-cache")
		_, _ = w.Write(data)
	}
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
		"recentDirs":      s.Hub.recentDirs(8),
	})
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

// launch starts a session for an account: with account it puts that (offline) account back to work,
// without it a new account named name is created and takes the lowest free seat.
func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	if s.Launcher == nil {
		writeJSON(w, http.StatusNotImplemented, errorBody(errLaunchDisabled))
		return
	}
	// without the mod the session could never reach the office, so don't start it
	if mod := s.Launcher.Mod(r.Context()); !mod.OK {
		body := map[string]any{"error": mod.Error, "code": mod.Code, "params": mod.Params, "fix": mod.Fix}
		if mod.Fix != "" {
			body["error"] = mod.Error + "; run first: " + mod.Fix
		}
		writeJSON(w, http.StatusPreconditionFailed, body)
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
		if acc = s.Hub.Account(req.Account); acc == nil {
			writeErr(w, ErrUnknownAccount)
			return
		}
		if s.Hub.Online(acc.ID) {
			writeErr(w, ErrAccountBusy)
			return
		}
	} else {
		if acc, err = s.Hub.CreateAccount(req.Name); err != nil {
			writeErr(w, err)
			return
		}
		created = true
	}
	req.Account, req.Name = acc.ID, acc.Name

	id := newID()
	env := map[string]string{"SYMPHONY_BRIDGE": s.Launcher.BridgeURL, "SYMPHONY_LAUNCH": id, "SYMPHONY_TOKEN": acc.Token}
	if req.Lang != "" {
		env["SYMPHONY_LANG"] = req.Lang // the mod speaks the page's language
	}
	l := &Launch{ID: id, Name: acc.Name, Dir: req.Dir, Account: acc.ID,
		Model: req.Model, Effort: req.Effort, Status: LaunchStarting, Time: time.Now()}
	if err := s.Launcher.Start(l, req, env); err != nil {
		if created {
			_ = s.Hub.DeleteAccount(acc.ID)
		}
		writeJSON(w, http.StatusInternalServerError, errorBody(err))
		return
	}
	s.Hub.setLast(acc.ID, req)
	resp := *l // the hub updates l in the background once it has it; the page gets a copy
	s.Hub.addLaunch(l)
	go s.Launcher.watch(s.Hub, l)
	writeJSON(w, 200, resp)
}
