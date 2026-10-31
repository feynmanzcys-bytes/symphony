package bridge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Account is one identity in the office: a name, a token and a fixed seat, used by one session at a time.
// Claude logins are not managed here; all sessions may share one login.
type Account struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Token   string    `json:"token"`
	Seat    int       `json:"seat"`
	Created time.Time `json:"created"`
	// Last holds the options of the last start from the page, filled in again next time
	Last *LaunchRequest `json:"last,omitempty"`
}

// AccountView is an account as the snapshot shows it: without the token.
type AccountView struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Seat    int            `json:"seat"`
	Session string         `json:"session,omitempty"` // the online session's id; empty when off duty
	Last    *LaunchRequest `json:"last,omitempty"`
}

var (
	ErrUnknownAccount = newError("account_unknown", "no such account")
	ErrNameTaken      = newError("account_name_taken", "that name is already taken")
	ErrAccountBusy    = newError("account_busy", "this account already has a session online")
	ErrBadName        = newError("account_bad_name", "a name needs 1 to 40 characters and no control characters")
)

func newToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func validName(name string) bool {
	return name != "" && len([]rune(name)) <= 40 && strings.IndexFunc(name, unicode.IsControl) < 0
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
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return list, nil
}

// saveAccounts writes atomically (a temp file, then a rename); the file holds tokens, so only the owner may read it.
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
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ---------- accounts on the Hub ----------

// Open sets the data folder and reads the accounts (accounts.json) and the message log (messages.json).
func (h *Hub) Open(dir string) error {
	list, err := loadAccounts(filepath.Join(dir, "accounts.json"))
	if err != nil {
		return err
	}
	var msgs []*Message
	if data, err := os.ReadFile(filepath.Join(dir, "messages.json")); err == nil {
		if err := json.Unmarshal(data, &msgs); err != nil {
			return fmt.Errorf("messages.json: %w", err)
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dataDir = dir
	for _, a := range list {
		h.accounts[a.ID] = a
	}
	h.messages = msgs
	return nil
}

func (h *Hub) saveLocked() error {
	if h.dataDir == "" {
		return nil
	}
	list := make([]*Account, 0, len(h.accounts))
	for _, a := range h.accounts {
		list = append(list, a)
	}
	return saveAccounts(filepath.Join(h.dataDir, "accounts.json"), list)
}

// saver writes the message log in the background: the hub's lock is held while the log is encoded, never while
// the disk is written; writes that pile up collapse into the latest.
type saver struct {
	mu      sync.Mutex
	pending []byte        // the latest log to write, nil when written
	idle    chan struct{} // closed when the writer is done; nil while none runs
}

// saveMessagesLocked hands the message log to the background writer, so it survives a bridge restart.
func (h *Hub) saveMessagesLocked() {
	if h.dataDir == "" {
		return
	}
	data, err := json.Marshal(h.messages)
	if err != nil {
		return
	}
	path := filepath.Join(h.dataDir, "messages.json")
	s := &h.msgSave
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = data
	if s.idle == nil {
		s.idle = make(chan struct{})
		go s.write(path)
	}
}

func (s *saver) write(path string) {
	for {
		s.mu.Lock()
		data := s.pending
		s.pending = nil
		if data == nil {
			close(s.idle)
			s.idle = nil
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		if os.WriteFile(path+".tmp", data, 0o600) == nil {
			_ = os.Rename(path+".tmp", path)
		}
	}
}

// Flush waits until the message log is on disk.
func (h *Hub) Flush() {
	h.msgSave.mu.Lock()
	idle := h.msgSave.idle
	h.msgSave.mu.Unlock()
	if idle != nil {
		<-idle
	}
}

// CreateAccount creates an account in the lowest free seat. Names are unique (sessions message each other by name).
func (h *Hub) CreateAccount(name string) (*Account, error) {
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
	a := &Account{ID: newID(), Name: name, Token: newToken(), Seat: seat, Created: h.now()}
	h.accounts[a.ID] = a
	if err := h.saveLocked(); err != nil {
		delete(h.accounts, a.ID)
		return nil, err
	}
	h.broadcastLocked(nil)
	cp := *a
	return &cp, nil
}

// DeleteAccount deletes an account that is offline and frees its seat.
func (h *Hub) DeleteAccount(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	a := h.accounts[id]
	if a == nil {
		return ErrUnknownAccount
	}
	if h.onlineLocked(id) != nil {
		return ErrAccountBusy
	}
	delete(h.accounts, id)
	for sid, s := range h.sessions {
		if s.Account == id {
			delete(h.sessions, sid)
		}
	}
	// cancel what its lost sessions left pending, so the popup shows no orphaned cards
	for _, ask := range h.asks {
		if ask.Account == id && ask.Status == AskPending {
			ask.Status = AskCancelled
			close(ask.done)
		}
	}
	if err := h.saveLocked(); err != nil {
		h.accounts[id] = a
		return err
	}
	h.broadcastLocked(nil)
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
		_ = h.saveLocked()
	}
}

// onlineLocked returns the account's online session (neither off duty nor lost).
func (h *Hub) onlineLocked(account string) *Session {
	for _, s := range h.sessions {
		if s.Account == account && s.State != StateEnded {
			return s
		}
	}
	return nil
}

// Online reports whether the account has a session online now.
func (h *Hub) Online(account string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.onlineLocked(account) != nil
}
