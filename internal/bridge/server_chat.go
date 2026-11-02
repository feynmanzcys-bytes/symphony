package bridge

import (
	"io"
	"net/http"
	"strconv"
)

// ---------- the employee window: chat ----------

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"items": s.Hub.Chat(r.PathValue("session"))})
}

func (s *Server) say(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &req) {
		return
	}
	it, err := s.Hub.Say(r.PathValue("session"), req.Text)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, it)
}

func (s *Server) abort(w http.ResponseWriter, r *http.Request) {
	if err := s.Hub.Abort(r.PathValue("session")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// inbox is the mod's long poll for what the page sent its session.
func (s *Server) inbox(w http.ResponseWriter, r *http.Request) {
	acc, ok := s.authed(w, r)
	if !ok {
		return
	}
	// ack: the id of the last item the mod did; a mod before 1.1.2 sends none, and gets each item once
	ack := int64(-1)
	if v := r.URL.Query().Get("ack"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			ack = n
		}
	}
	items, err := s.Hub.Inbox(r.URL.Query().Get("session"), acc.ID, ack, s.WaitTimeout, r.Context().Done())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

// ---------- the employee window: the session's terminal ----------

func (s *Server) term(w http.ResponseWriter, r *http.Request) *Term {
	if s.Launcher == nil || s.Launcher.Terms == nil {
		writeErr(w, ErrTermUnknown)
		return nil
	}
	t := s.Launcher.Terms.Get(r.PathValue("id"))
	if t == nil {
		writeErr(w, ErrTermUnknown)
		return nil
	}
	return t
}

func size(r *http.Request) (cols, rows int) {
	cols, _ = strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ = strconv.Atoi(r.URL.Query().Get("rows"))
	return cols, rows
}

// termStream is the terminal's output: the recent history first, then everything new as it comes, until the
// process ends or the client goes. cols and rows size the terminal to the window. sizes=1 (a window that draws the
// screen itself, the page's terminal view) puts the terminal's size in the output, first and whenever it changes
// (sizeNote).
func (s *Server) termStream(w http.ResponseWriter, r *http.Request) {
	t := s.term(w, r)
	if t == nil {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	attach := t.Attach
	if r.URL.Query().Get("sizes") == "1" {
		attach = t.AttachSizes
	}
	history, c, detach := attach(size(r))
	defer detach()
	w.Header().Set("content-type", "application/octet-stream")
	w.Header().Set("cache-control", "no-cache")
	w.Header().Set("x-symphony-name", t.Name)
	w.WriteHeader(200)
	if _, err := w.Write(history); err != nil {
		return
	}
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-c.gone:
			// whatever is still queued, then the end
			for {
				select {
				case chunk := <-c.out:
					if _, err := w.Write(chunk); err != nil {
						return
					}
				default:
					flusher.Flush()
					return
				}
			}
		case chunk := <-c.out:
			if _, err := w.Write(chunk); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// termInput sends the request body to the process as keystrokes.
func (s *Server) termInput(w http.ResponseWriter, r *http.Request) {
	t := s.term(w, r)
	if t == nil {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody(err))
		return
	}
	if err := t.Write(data); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) termResize(w http.ResponseWriter, r *http.Request) {
	t := s.term(w, r)
	if t == nil {
		return
	}
	var req struct {
		Cols int `json:"cols"`
		Rows int `json:"rows"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := t.Resize(req.Cols, req.Rows); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// termOpen opens a terminal window on this machine attached to the session (symphony attach).
func (s *Server) termOpen(w http.ResponseWriter, r *http.Request) {
	t := s.term(w, r)
	if t == nil {
		return
	}
	if t.Ended() {
		writeErr(w, ErrTermEnded)
		return
	}
	if s.Window == nil || s.AttachCommand == nil {
		writeErr(w, newError("window_unavailable", "this bridge can't open terminal windows; attach by hand: "+s.Launcher.AttachFor(t.ID), "command", s.Launcher.AttachFor(t.ID)))
		return
	}
	if err := s.Window.Open(t.Name, s.AttachCommand(t.ID)); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "command": s.Launcher.AttachFor(t.ID)})
}
