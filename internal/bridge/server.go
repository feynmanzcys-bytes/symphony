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
)

type Server struct {
	Hub *Hub
	// WaitTimeout 是长轮询单次挂起时间，mod 那边会循环再来。
	WaitTimeout time.Duration
	// Launcher 不为空时页面上可以启动会话
	Launcher *Launcher
	// AllowedHosts 是请求 Host 允许的主机名，挡 DNS rebinding；默认只认本机
	AllowedHosts []string

	web fs.FS
	mux *http.ServeMux
}

// NewServer 建一个 bridge；web 里要有 office.html、approve.html 和 asks.js。
func NewServer(hub *Hub, web fs.FS) *Server {
	s := &Server{Hub: hub, WaitTimeout: 20 * time.Second, AllowedHosts: []string{"localhost", "127.0.0.1", "::1"},
		web: web, mux: http.NewServeMux()}

	s.mux.HandleFunc("GET /{$}", s.file("office.html"))
	s.mux.HandleFunc("GET /approve", s.file("approve.html"))
	s.mux.HandleFunc("GET /asks.js", s.file("asks.js"))
	s.mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, hub.Snapshot()) })
	s.mux.HandleFunc("GET /api/stream", s.stream)

	// 下面这些是 mod 调的，要凭账号 token
	s.mux.HandleFunc("POST /api/event", s.event)
	s.mux.HandleFunc("POST /api/ask", s.ask)
	s.mux.HandleFunc("GET /api/wait/{id}", s.wait)
	s.mux.HandleFunc("POST /api/cancel/{id}", s.cancel)

	// 网页端：回答、账号、启动会话
	s.mux.HandleFunc("POST /api/answer/{id}", s.answer)
	s.mux.HandleFunc("POST /api/accounts", s.createAccount)
	s.mux.HandleFunc("DELETE /api/accounts/{id}", s.deleteAccount)
	s.mux.HandleFunc("GET /api/launch/options", s.launchOptions)
	s.mux.HandleFunc("GET /api/dirs", s.dirs)
	s.mux.HandleFunc("POST /api/launch", s.launch)
	return s
}

// SetLauncher 打开页面启动；会话的“去哪儿找终端”也跟着这个终端后端走。
func (s *Server) SetLauncher(l *Launcher) {
	s.Launcher = l
	s.Hub.mu.Lock()
	s.Hub.AttachHint = l.Term.Attach
	s.Hub.mu.Unlock()
}

// ServeHTTP 先挡掉两类跨站请求：Host 不是本机的（DNS rebinding），
// 以及别的网站借浏览器发来的写请求（带了 Origin 且不是本站）。mod 和 curl 不带 Origin。
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
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// authed 按 Authorization: Bearer <token> 认出账号；认不出就 401，mod 那边会退回终端。
func (s *Server) authed(w http.ResponseWriter, r *http.Request) (*Account, bool) {
	token := strings.TrimPrefix(r.Header.Get("authorization"), "Bearer ")
	acc := s.Hub.AccountByToken(token)
	if acc == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "没有有效的军团账号 token"})
		return nil, false
	}
	return acc, true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺 session"})
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺 session 或 kind 不对"})
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

// createAccount 建账号并把 token 返回一次（命令行手动起会话时用：SYMPHONY_TOKEN=<token>）。
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
	if strings.HasSuffix(name, ".js") {
		ctype = "text/javascript; charset=utf-8"
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

// ---------- 页面启动会话 ----------

func (s *Server) launchOptions(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	trustHint, terminal := false, ""
	reason := "这个 bridge 没开页面启动（缺 claude 或能用的终端，看 bridge 启动时的日志）"
	var mod *ModStatus
	if s.Launcher != nil {
		trustHint, terminal = s.Launcher.Term.TrustHint(), s.Launcher.Term.Name()
		m := s.Launcher.Mod(r.Context())
		mod, reason = &m, m.Error
	}
	writeJSON(w, 200, map[string]any{
		"enabled":         mod != nil && mod.OK,
		"reason":          reason, // 开不了的原因
		"mod":             mod,    // mod 装好没有；没装好时 fix 是装的命令
		"terminal":        terminal,
		"trustHint":       trustHint, // 新目录第一次启动要人在弹出的窗口里点“信任”
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"path": filepath.Clean(path), "parent": parent, "dirs": dirs, "roots": Roots(runtime.GOOS)})
}

// launch 用一个账号启动会话：给了 account 就是让这个（不在线的）账号上班，
// 没给就按 name 新建一个账号，坐最小的空座位。
func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	if s.Launcher == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "这个 bridge 没开页面启动"})
		return
	}
	// 没装 mod 的话起出来的会话进不了办公室，干脆不起
	if mod := s.Launcher.Mod(r.Context()); !mod.OK {
		msg := mod.Error
		if mod.Fix != "" {
			msg += "，先运行：" + mod.Fix
		}
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": msg})
		return
	}
	var req LaunchRequest
	if !decode(w, r, &req) {
		return
	}
	req, err := normalizeLaunch(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
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
	l := &Launch{ID: id, Name: acc.Name, Dir: req.Dir, Account: acc.ID,
		Model: req.Model, Effort: req.Effort, Status: LaunchStarting, Time: time.Now()}
	if err := s.Launcher.Start(l, req, env); err != nil {
		if created {
			_ = s.Hub.DeleteAccount(acc.ID)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.Hub.setLast(acc.ID, req)
	resp := *l // l 交给 hub 之后会被后台改状态，回给页面的用一份拷贝
	s.Hub.addLaunch(l)
	go s.Launcher.watch(s.Hub, l)
	writeJSON(w, 200, resp)
}
