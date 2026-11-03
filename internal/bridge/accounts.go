package bridge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Account is one identity in the office: a name, a token and a fixed seat, used by one desk session at a time (and
// one session per task it is in). Claude logins are not managed here; all sessions may share one login. An advisor
// is an account made for one task: it has no seat (-1) and goes when the task ends.
type Account struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Token   string    `json:"token"`
	Seat    int       `json:"seat"`
	Advisor bool      `json:"advisor,omitempty"`
	Created time.Time `json:"created"`
	// Last holds the options of the last start from the page, filled in again next time
	Last *LaunchRequest `json:"last,omitempty"`
	// Dir is the folder the last desk session worked in, as the mod reported it (a session started in a terminal
	// too), so an employee off duty still has a folder for a task
	Dir string `json:"dir,omitempty"`
}

// AccountView is an account as the snapshot shows it: without the token.
type AccountView struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Seat    int            `json:"seat"`
	Advisor bool           `json:"advisor,omitempty"`
	Session string         `json:"session,omitempty"` // the online desk session's id; empty when off duty
	Last    *LaunchRequest `json:"last,omitempty"`
	Dir     string         `json:"dir,omitempty"`
}

var (
	ErrUnknownAccount = newError("account_unknown", "no such account")
	ErrNameTaken      = newError("account_name_taken", "that name is already taken")
	ErrAccountBusy    = newError("account_busy", "this account already has a session online, or one starting")
	ErrSessionGone    = newError("session_gone", "this session's process has ended")
	ErrBadName        = newError("account_bad_name", "a name needs 1 to 40 characters and no control characters")
	// errAccountsWrite is accounts.json that couldn't be written; the bridge's log has the details (saveLocked)
	errAccountsWrite = newError("data_write_failed", "can't write accounts.json in the data folder; the bridge's log says why", "file", "accounts.json")
)

func newToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func validName(name string) bool {
	return name != "" && len([]rune(name)) <= 40 && strings.IndexFunc(name, unicode.IsControl) < 0
}

// DamagedFile is a data file that is there but doesn't parse, one the bridge can't start without (accounts.json with
// the tokens, tasks.json with the task counter): restored from a backup or moved aside, it starts again.
type DamagedFile struct {
	Path string
	Err  error
}

func (e *DamagedFile) Error() string { return e.Path + ": " + e.Err.Error() }
func (e *DamagedFile) Unwrap() error { return e.Err }

// withoutNil drops the null entries of a list read from a data file (a hand edit, say), which would crash the bridge.
func withoutNil[T any](list []*T) []*T {
	return slices.DeleteFunc(list, func(v *T) bool { return v == nil })
}

// loadAccounts reads the accounts file; a missing file means no accounts.
func loadAccounts(path string) ([]*Account, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []*Account
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, &DamagedFile{Path: path, Err: err}
	}
	return withoutNil(list), nil
}

// loadLog reads a log the bridge can start without (messages.json, ledger.json); a missing file is an empty log. One
// that doesn't parse (a crash left it half written, say) is set aside as <name>.corrupt-<unix seconds>, the log says
// so, and the bridge starts with an empty one. One that is there but can't be read is an error: taken for empty, it
// would be written over.
func loadLog[T any](h *Hub, path string) ([]*T, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []*T
	if err := json.Unmarshal(data, &list); err != nil {
		aside := fmt.Sprintf("%s.corrupt-%d", path, h.now().Unix())
		if os.Rename(path, aside) != nil {
			return nil, &DamagedFile{Path: path, Err: err}
		}
		h.warn("log.data_set_aside", "file", path, "reason", err.Error(), "aside", aside)
		return nil, nil
	}
	return withoutNil(list), nil
}

// writeSynced replaces path with data so that a crash can't leave it half written: data goes to path.tmp (only this
// user may read it), which is synced to the disk and then renamed over path. A failed write takes path.tmp away again.
func writeSynced(path string, data []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// saveAccounts writes atomically (writeSynced); the file holds tokens, so only the owner may read it.
func saveAccounts(path string, list []*Account) error {
	if path == "" {
		return nil
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Seat < list[j].Seat })
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeSynced(path, data)
}

// ---------- accounts on the Hub ----------

// Open sets the data folder and reads the accounts (accounts.json), the message log (messages.json), the ledger
// (ledger.json), the error log (errors.json) and the tasks (tasks.json; tasks the previous bridge left running are ended now). The bridge has to
// hold the folder's lock first (LockData): another bridge's running tasks would look left over.
func (h *Hub) Open(dir string) error {
	for _, s := range []*saver{&h.msgSave, &h.taskSave, &h.ledgerSave} {
		s.warn = h.warnWrite
	}
	list, err := loadAccounts(filepath.Join(dir, "accounts.json"))
	if err != nil {
		return err
	}
	msgs, err := loadLog[Message](h, filepath.Join(dir, "messages.json"))
	if err != nil {
		return err
	}
	ledger, err := loadLog[LedgerRow](h, filepath.Join(dir, "ledger.json"))
	if err != nil {
		return err
	}
	if err := h.openFaults(dir); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dataDir = dir
	for _, a := range list {
		h.accounts[a.ID] = a
	}
	h.messages = msgs
	h.ledger = ledger
	return h.openTasksLocked(dir)
}

// saveLocked writes accounts.json. A failure goes to the log as well (Warn), so a caller that answers nobody may let
// the error go.
func (h *Hub) saveLocked() error {
	if h.dataDir == "" {
		return nil
	}
	list := make([]*Account, 0, len(h.accounts))
	for _, a := range h.accounts {
		list = append(list, a)
	}
	path := filepath.Join(h.dataDir, "accounts.json")
	if err := saveAccounts(path, list); err != nil {
		h.warnWrite(path, err)
		return err
	}
	return nil
}

// warnWrite tells the log that a data file couldn't be written (the disk is full, the folder read-only ...): the
// office goes on, but what changed is in memory only until a write gets through.
func (h *Hub) warnWrite(path string, err error) {
	h.warn("log.save_failed", "file", path, "reason", err.Error())
}

// saver writes a file (the message log, the tasks) in the background: the hub's lock is held while it is encoded,
// never while the disk is written; writes that pile up collapse into the latest. A write that fails is in the log, at
// most once a minute, and gets one more try when the bridge stops (wait).
type saver struct {
	mu      sync.Mutex
	path    string
	pending func() ([]byte, error) // makes the latest contents to write (in the writer, outside the hub's lock); nil when written
	idle    chan struct{}          // closed when the writer is done; nil while none runs
	failed  []byte                 // the contents the last write didn't get on disk; nil once one did
	err     error                  // why the last write failed; nil once one succeeded
	warned  time.Time              // when the log last heard of a failed write
	warn    func(path string, err error)
}

// saveMessagesLocked tells the background writer the message log changed, so it survives a bridge restart. The writer
// copies the log under the hub's lock when it gets to it (a later report may still mark a message received, so it
// copies the values) and encodes the copy outside it: a burst of messages costs one copy per write, and encoding a long
// log doesn't hold up every report and page.
func (h *Hub) saveMessagesLocked() {
	if h.dataDir == "" {
		return
	}
	h.msgSave.putFunc(filepath.Join(h.dataDir, "messages.json"), func() ([]byte, error) {
		h.mu.Lock()
		msgs := make([]Message, len(h.messages))
		for i, m := range h.messages {
			msgs[i] = *m
		}
		h.mu.Unlock()
		return json.Marshal(msgs)
	})
}

// put hands the latest contents to the writer.
func (s *saver) put(path string, data []byte) {
	s.putFunc(path, func() ([]byte, error) { return data, nil })
}

// putFunc hands the writer what makes the latest contents: it runs in the writer's goroutine, and only the latest one
// handed over runs (a newer one replaces one that didn't run yet). The writer holds s.mu only to take it, never while
// it runs, so contents may take the hub's lock.
func (s *saver) putFunc(path string, contents func() ([]byte, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path, s.pending = path, contents
	if s.idle == nil {
		s.idle = make(chan struct{})
		go s.write()
	}
}

func (s *saver) write() {
	for {
		s.mu.Lock()
		next, path := s.pending, s.path
		s.pending = nil
		if next == nil {
			close(s.idle)
			s.idle = nil
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		data, err := next()
		if err != nil {
			s.done(path, nil, err)
			continue
		}
		s.done(path, data, writeSynced(path, data))
	}
}

// done notes how writing data to path went (data is nil when it didn't encode): a failure is kept for wait, and the
// log hears of it, at most once a minute.
func (s *saver) done(path string, data []byte, err error) {
	s.mu.Lock()
	s.failed, s.err = nil, nil
	say := false
	if err != nil {
		s.failed, s.err = data, fmt.Errorf("%s: %w", filepath.Base(path), err)
		if now := time.Now(); now.Sub(s.warned) >= time.Minute {
			s.warned, say = now, true
		}
	}
	warn := s.warn
	s.mu.Unlock()
	if say && warn != nil {
		warn(path, err)
	}
}

// wait waits until what was handed over is on disk, and says why not when it isn't: a write that failed gets one more
// try first (the disk may have room again, or another program let go of the file).
func (s *saver) wait() error {
	s.mu.Lock()
	idle := s.idle
	s.mu.Unlock()
	if idle != nil {
		<-idle
	}
	s.mu.Lock()
	if s.idle == nil && s.failed != nil {
		failed := s.failed
		s.pending, s.idle = func() ([]byte, error) { return failed, nil }, make(chan struct{})
		go s.write()
	}
	idle = s.idle
	s.mu.Unlock()
	if idle != nil {
		<-idle
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Flush waits until the tasks being ended are done, and the message log, the tasks and the ledger are on disk; the
// error says which of them couldn't be written.
func (h *Hub) Flush() error {
	h.taskJobs.wait()
	return errors.Join(h.msgSave.wait(), h.taskSave.wait(), h.ledgerSave.wait(), h.faults.save.wait())
}

// CreateAccount creates an account in the lowest free seat. Names are unique (sessions message each other by name).
func (h *Hub) CreateAccount(name string) (*Account, error) { return h.createAccount(name, false) }

// CreateAdvisor creates an account for one task: no seat, deleted when the task ends.
func (h *Hub) CreateAdvisor(name string) (*Account, error) { return h.createAccount(name, true) }

func (h *Hub) createAccount(name string, advisor bool) (*Account, error) {
	name = strings.TrimSpace(name)
	if !validName(name) {
		return nil, ErrBadName
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	taken := map[int]bool{}
	for _, a := range h.accounts {
		if a.Name == name {
			return nil, ErrNameTaken
		}
		taken[a.Seat] = true
	}
	seat := 0
	for taken[seat] {
		seat++
	}
	if advisor {
		seat = -1
	}
	a := &Account{ID: newID(), Name: name, Token: newToken(), Seat: seat, Advisor: advisor, Created: h.now()}
	h.accounts[a.ID] = a
	if err := h.saveLocked(); err != nil {
		delete(h.accounts, a.ID)
		return nil, errAccountsWrite
	}
	h.broadcastLocked(nil)
	cp := *a
	return &cp, nil
}

// DeleteAccount deletes an account that is offline and in no task the office shows (running, ending or failed), and
// frees its seat.
func (h *Hub) DeleteAccount(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.deleteAccountLocked(id); err != nil {
		return err
	}
	h.broadcastLocked(nil)
	return nil
}

func (h *Hub) deleteAccountLocked(id string) error {
	a := h.accounts[id]
	if a == nil {
		return ErrUnknownAccount
	}
	if h.onlineLocked(id) != nil || h.shownTaskLocked(id) != nil {
		return ErrAccountBusy
	}
	delete(h.accounts, id)
	for sid, s := range h.sessions {
		if s.Account == id {
			h.dropSessionLocked(sid)
		}
	}
	// cancel what its lost sessions left pending, so the popup shows no orphaned cards
	for _, ask := range h.asks {
		if ask.Account == id && ask.Status == AskPending {
			h.cancelAskLocked(ask)
		}
	}
	if err := h.saveLocked(); err != nil {
		h.accounts[id] = a
		return errAccountsWrite
	}
	return nil
}

// AccountByToken finds an account by its token, or returns nil.
func (h *Hub) AccountByToken(token string) *Account {
	if token == "" {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, a := range h.accounts {
		if a.Token == token {
			cp := *a
			return &cp
		}
	}
	return nil
}

// Account returns a copy of the account with this id.
func (h *Hub) Account(id string) *Account {
	h.mu.Lock()
	defer h.mu.Unlock()
	if a := h.accounts[id]; a != nil {
		cp := *a
		return &cp
	}
	return nil
}

func (h *Hub) setLast(id string, req LaunchRequest) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if a := h.accounts[id]; a != nil {
		req.Account, req.Name = "", ""
		a.Last = &req
		_ = h.saveLocked() // a failure is in the log
	}
}

// onlineLocked returns the account's online desk session (neither off duty nor lost).
func (h *Hub) onlineLocked(account string) *Session { return h.onlineTaskLocked(account, "") }

// onlineTaskLocked returns the account's online session of a task ("" for the desk).
func (h *Hub) onlineTaskLocked(account, task string) *Session {
	for _, s := range h.sessions {
		if s.Account == account && s.Task == task && s.State != StateEnded {
			return s
		}
	}
	return nil
}

// Online reports whether the account has a desk session online now.
func (h *Hub) Online(account string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.onlineLocked(account) != nil
}
