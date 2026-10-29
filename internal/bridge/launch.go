package bridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
)

// 启动框里的下拉选项。空字符串表示不传这个参数，跟随 Claude Code 自己的设置。
var (
	launchModels          = []Option{{"", "跟随设置"}, {"fable", "Fable"}, {"opus", "Opus"}, {"sonnet", "Sonnet"}, {"haiku", "Haiku"}}
	launchEfforts         = []Option{{"", "跟随设置"}, {"low", "low"}, {"medium", "medium"}, {"high", "high"}, {"xhigh", "xhigh"}, {"max", "max"}}
	launchPermissionModes = []Option{{"", "跟随设置"}, {"manual", "manual：每次都问"}, {"acceptEdits", "acceptEdits：改文件不问"}, {"auto", "auto：自动判断"}, {"plan", "plan：只做计划"}, {"dontAsk", "dontAsk：不问，不许的直接拒"}, {"bypassPermissions", "bypassPermissions：全部放行"}}
	launchResumes         = []Option{{"", "新会话"}, {"continue", "继续这个目录上次的会话"}}
)

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func validOption(opts []Option, v string) bool {
	return slices.ContainsFunc(opts, func(o Option) bool { return o.Value == v })
}

// LaunchRequest 是启动框提交的内容。Account 是已有账号的 id（让它上班）；
// 不给就按 Name 新建账号。
type LaunchRequest struct {
	Account        string `json:"account,omitempty"`
	Name           string `json:"name,omitempty"`
	Dir            string `json:"dir"`
	Model          string `json:"model"`
	Effort         string `json:"effort"`
	PermissionMode string `json:"permissionMode"`
	Resume         string `json:"resume"`
}

// Launch 是一次从页面发起的启动。
const (
	LaunchStarting = "starting"
	LaunchOnline   = "online"
	LaunchFailed   = "failed"
)

type Launch struct {
	ID      string    `json:"id"`
	Attach  string    `json:"attach"` // 去哪儿找这个会话的终端（tmux attach 命令或“弹出的窗口”）
	Name    string    `json:"name"`
	Dir     string    `json:"dir"`
	Account string    `json:"account"`
	Model   string    `json:"model,omitempty"`
	Effort  string    `json:"effort,omitempty"`
	Command string    `json:"command"` // 给人看的完整命令
	Status  string    `json:"status"`
	Error   string    `json:"error,omitempty"`
	Session string    `json:"session,omitempty"`
	Time    time.Time `json:"time"`
}

// Launcher 在一个交互终端里拉起 Claude Code（终端怎么开看平台，见 Terminal），
// 并带上 mod 和 bridge 的环境变量。
type Launcher struct {
	Claude    string // claude 可执行文件
	ModDir    string // mods/symphony-bridge 的绝对路径
	BridgeURL string // 会话回连 bridge 的地址
	Term      Terminal
	// OnlineTimeout 是等会话上线的时间
	OnlineTimeout time.Duration
}

func NewLauncher(claude, modDir, bridgeURL string, term Terminal) *Launcher {
	return &Launcher{Claude: claude, ModDir: modDir, BridgeURL: bridgeURL, Term: term, OnlineTimeout: 90 * time.Second}
}

var errBadLaunch = errors.New("启动参数不对")

// argv 拼出 claude 的参数（不经过 shell）。
func (l *Launcher) argv(req LaunchRequest) []string {
	args := []string{l.Claude, "--plugin-dir", l.ModDir, "--name", req.Name}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Effort != "" {
		args = append(args, "--effort", req.Effort)
	}
	if req.PermissionMode != "" {
		args = append(args, "--permission-mode", req.PermissionMode)
	}
	if req.Resume == "continue" {
		args = append(args, "--continue")
	}
	return args
}

func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("-_./=:@", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// normalizeLaunch 校验启动请求；dir 必须是已存在的绝对路径目录，新账号要有名字。
func normalizeLaunch(req LaunchRequest) (LaunchRequest, error) {
	req.Account = strings.TrimSpace(req.Account)
	req.Name = strings.TrimSpace(req.Name)
	req.Dir = strings.TrimSpace(req.Dir)
	if req.Account == "" && !validName(req.Name) {
		return req, fmt.Errorf("%w：%v", errBadLaunch, ErrBadName)
	}
	if !filepath.IsAbs(req.Dir) {
		return req, fmt.Errorf("%w：目录要写绝对路径", errBadLaunch)
	}
	req.Dir = filepath.Clean(req.Dir)
	if fi, err := os.Stat(req.Dir); err != nil || !fi.IsDir() {
		return req, fmt.Errorf("%w：目录不存在：%s", errBadLaunch, req.Dir)
	}
	if !validOption(launchModels, req.Model) || !validOption(launchEfforts, req.Effort) ||
		!validOption(launchPermissionModes, req.PermissionMode) || !validOption(launchResumes, req.Resume) {
		return req, fmt.Errorf("%w：下拉选项不认识", errBadLaunch)
	}
	return req, nil
}

// Start 开终端跑 claude。env 是要额外带上的环境变量。
func (l *Launcher) Start(launch *Launch, req LaunchRequest, env map[string]string) error {
	argv := l.argv(req)
	shown := []string{"cd", shellQuote(req.Dir), "&&"}
	for _, k := range sortedKeys(env) {
		v := env[k]
		if k == "SYMPHONY_TOKEN" {
			v = "***"
		}
		shown = append(shown, k+"="+shellQuote(v))
	}
	for _, a := range argv {
		shown = append(shown, shellQuote(a))
	}
	launch.Command = strings.Join(shown, " ")
	launch.Attach = l.Term.Attach(launch.ID)
	return l.Term.Start(launch, req.Dir, env, argv)
}

func (l *Launcher) watch(hub *Hub, launch *Launch) { l.Term.Watch(hub, launch, l.OnlineTimeout) }

func lastLines(s string, n int) string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimRight(l, " "))
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// DirEntry 是目录浏览里的一项；path 由 bridge 拼好，页面不用管 / 还是 \。
type DirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ListDirs 列出 path 下的子目录（不含隐藏目录），给启动框的目录浏览用。
func ListDirs(path string) (dirs []DirEntry, parent string, err error) {
	dirs = []DirEntry{} // 没有子目录时也要给页面一个空列表，不是 null
	if !filepath.IsAbs(path) {
		return nil, "", fmt.Errorf("要绝对路径")
	}
	path = filepath.Clean(path)
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, "", err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(path, e.Name())
		if e.IsDir() || (e.Type()&os.ModeSymlink != 0 && isDir(full)) {
			dirs = append(dirs, DirEntry{Name: e.Name(), Path: full})
		}
		if len(dirs) >= 500 {
			break
		}
	}
	slices.SortFunc(dirs, func(a, b DirEntry) int { return strings.Compare(a.Name, b.Name) })
	if p := filepath.Dir(path); p != path {
		parent = p
	}
	return dirs, parent, nil
}

// Roots 是可以浏览的最上层：Windows 上是各个盘符，别的系统只有 /。
func Roots(goos string) []string {
	if goos != "windows" {
		return []string{"/"}
	}
	var roots []string
	for c := 'A'; c <= 'Z'; c++ {
		if d := string(c) + `:\`; isDir(d) {
			roots = append(roots, d)
		}
	}
	return roots
}

func isDir(p string) bool { fi, err := os.Stat(p); return err == nil && fi.IsDir() }
