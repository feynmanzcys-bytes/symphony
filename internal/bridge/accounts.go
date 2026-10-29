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
	"time"
	"unicode"
)

// Account 是军团里的一个身份：一个名字、一把 token、一个固定座位，一次只给一个会话用。
// Claude 的登录不在这里管，所有会话可以共用同一个登录。
type Account struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Token   string    `json:"token"`
	Seat    int       `json:"seat"`
	Created time.Time `json:"created"`
	// Last 是上次从页面启动时用的参数，下次“上班”直接带出来
	Last *LaunchRequest `json:"last,omitempty"`
}

// AccountView 是账号在快照里的样子：不带 token。
type AccountView struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Seat    int            `json:"seat"`
	Session string         `json:"session,omitempty"` // 在线会话的 id，下班了就空着
	Last    *LaunchRequest `json:"last,omitempty"`
}

var (
	ErrUnknownAccount = errors.New("没有这个账号")
	ErrNameTaken      = errors.New("这个名字已经有人用了")
	ErrAccountBusy    = errors.New("这个账号已经有会话在线")
	ErrBadName        = errors.New("名字要有，不超过 40 个字，不能有控制字符")
)

func newToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func validName(name string) bool {
	return name != "" && len([]rune(name)) <= 40 && strings.IndexFunc(name, unicode.IsControl) < 0
}

// loadAccounts 读账号文件；文件不存在就当没有账号。
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

// saveAccounts 原子写入（先写临时文件再改名），文件里有 token，只给自己读。
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

// ---------- Hub 上的账号操作 ----------

// Open 指定数据目录，把账号（accounts.json）和消息记录（messages.json）读进来。
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

// saveMessagesLocked 把消息记录落盘，bridge 重启后还能翻。
func (h *Hub) saveMessagesLocked() {
	if h.dataDir == "" {
		return
	}
	data, err := json.Marshal(h.messages)
	if err != nil {
		return
	}
	path := filepath.Join(h.dataDir, "messages.json")
	if os.WriteFile(path+".tmp", data, 0o600) == nil {
		_ = os.Rename(path+".tmp", path)
	}
}

// CreateAccount 建一个新账号，坐最小的空座位。名字不能重复（会话间发消息按名字找人）。
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

// DeleteAccount 删掉一个不在线的账号，座位空出来。
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
	// 失联会话留下的待处理事项一并撤掉，免得弹窗里挂着没主的卡片
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

// AccountByToken 按 token 找账号，找不到返回 nil。
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

// Account 按 id 取账号（拷贝）。
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

// onlineLocked 返回账号当前在线的会话（没下班、也没失联）。
func (h *Hub) onlineLocked(account string) *Session {
	for _, s := range h.sessions {
		if s.Account == account && s.State != StateEnded {
			return s
		}
	}
	return nil
}

// Online 告诉调用方这个账号现在有没有会话在线。
func (h *Hub) Online(account string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.onlineLocked(account) != nil
}
