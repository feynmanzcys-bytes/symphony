package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
)

// ModName is the mod's plugin name in Claude Code; GitHubMarket is the repository that holds the
// plugin marketplace (.claude-plugin/marketplace.json).
const (
	ModName      = "symphony-bridge"
	GitHubMarket = "feynmanzcys-bytes/symphony"
)

// The drop-downs of the launch dialog. An empty value leaves the flag out, so Claude Code's own
// settings apply. The labels are English fallbacks; the page shows its own translations.
var (
	launchModels          = []Option{{"", "Default"}, {"fable", "Fable"}, {"opus", "Opus"}, {"sonnet", "Sonnet"}, {"haiku", "Haiku"}}
	launchEfforts         = []Option{{"", "Default"}, {"low", "low"}, {"medium", "medium"}, {"high", "high"}, {"xhigh", "xhigh"}, {"max", "max"}}
	launchPermissionModes = []Option{{"", "Default"}, {"manual", "manual: always ask"}, {"acceptEdits", "acceptEdits: file edits without asking"}, {"auto", "auto: decides for itself"}, {"plan", "plan: plan only"}, {"dontAsk", "dontAsk: never ask, deny what isn't allowed"}, {"bypassPermissions", "bypassPermissions: allow everything"}}
	launchResumes         = []Option{{"", "New session"}, {"continue", "Continue the last session in this folder"}}
)

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func validOption(opts []Option, v string) bool {
	return slices.ContainsFunc(opts, func(o Option) bool { return o.Value == v })
}

// LaunchRequest is what the launch dialog submits. Account is the id of an existing account to put
// back to work; without it a new account named Name is created. Lang is the page's language, which
// the session's mod uses for its messages.
type LaunchRequest struct {
	Account        string `json:"account,omitempty"`
	Name           string `json:"name,omitempty"`
	Dir            string `json:"dir"`
	Model          string `json:"model"`
	Effort         string `json:"effort"`
	PermissionMode string `json:"permissionMode"`
	Resume         string `json:"resume"`
	Lang           string `json:"lang,omitempty"`
}

// Launch is one start of a session from the page.
const (
	LaunchStarting = "starting"
	LaunchOnline   = "online"
	LaunchFailed   = "failed"
	LaunchEnded    = "ended" // it was online, and its process has exited
)

type Launch struct {
	ID      string `json:"id"`
	Attach  string `json:"attach"` // where to find the session's terminal: a tmux command, AttachWindow or AttachConsole
	Name    string `json:"name"`
	Dir     string `json:"dir"`
	Account string `json:"account"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
	Command string `json:"command"` // the full command, for people to read
	Status  string `json:"status"`
	// Error is the English reason a launch failed; the page shows ErrorCode and ErrorParams in its own language.
	Error       string            `json:"error,omitempty"`
	ErrorCode   string            `json:"errorCode,omitempty"`
	ErrorParams map[string]string `json:"errorParams,omitempty"`
	Session     string            `json:"session,omitempty"`
	Time        time.Time         `json:"time"`
	Task        string            `json:"task,omitempty"` // the task a member's session is started for
}

// Launcher starts Claude Code in an interactive terminal (how depends on the platform, see Terminal)
// with the bridge's environment variables. The mod normally comes from Claude Code's own install
// (claude plugin install).
type Launcher struct {
	Claude string // the claude executable
	// ModDir is a mod folder for development: when set, sessions get it with --plugin-dir, which
	// overrides an installed copy, and the install is not checked.
	ModDir string
	// Market is a local plugin marketplace (the repository root). Install hints prefer it, so the mod
	// and the bridge are the same code; without it they point at GitHub.
	Market    string
	BridgeURL string // the address sessions use to reach the bridge
	Term      Terminal
	// Terms are the sessions' terminals when the bridge hosts them (Term is a ptyTerminal); nil with another backend
	Terms *Terms
	// OnlineTimeout is how long to wait for a session to come online
	OnlineTimeout time.Duration
	// ListPlugins runs `claude plugin list --json`; tests swap in a fake
	ListPlugins func(ctx context.Context) ([]byte, error)
	// AttachHint, when set, is the command for reaching a session's terminal by hand, by launch id (the program sets
	// it: its own name, with the address and the data folder when they aren't the defaults); else the backend's
	AttachHint func(launchID string) string

	modMu sync.Mutex
	mod   ModStatus
	modAt time.Time
	now   func() time.Time // tests move the clock
}

// AttachFor is the command for reaching a launch's terminal by hand.
func (l *Launcher) AttachFor(launchID string) string {
	if l.AttachHint != nil {
		return l.AttachHint(launchID)
	}
	return l.Term.Attach(launchID)
}

func NewLauncher(claude, modDir, bridgeURL string, term Terminal) *Launcher {
	l := &Launcher{Claude: claude, ModDir: modDir, BridgeURL: bridgeURL, Term: term, OnlineTimeout: 90 * time.Second}
	if p, ok := term.(*ptyTerminal); ok {
		l.Terms = p.Terms
	}
	return l
}

// ModStatus says whether started sessions will have the mod. When they won't, the page shows the
// message for Code (Error is its English text) and Fix, a command that sorts it out.
type ModStatus struct {
	OK      bool              `json:"ok"`
	Dev     bool              `json:"dev,omitempty"`     // development mode: the -mod folder
	ID      string            `json:"id,omitempty"`      // the installed plugin, e.g. symphony-bridge@symphony
	Version string            `json:"version,omitempty"` // the installed version
	Error   string            `json:"error,omitempty"`
	Code    string            `json:"code,omitempty"`
	Params  map[string]string `json:"params,omitempty"`
	Fix     string            `json:"fix,omitempty"`
}

func modProblem(err *Error, fix string) ModStatus {
	return ModStatus{Error: err.Text, Code: err.Code, Params: err.Params, Fix: fix}
}

// How long an answer of Mod is kept: a good one for a minute; a bad one briefly, so after installing, reloading
// the page is enough and the bridge needs no restart, but opening the page doesn't run claude every time.
const (
	modGoodTTL = time.Minute
	modBadTTL  = 10 * time.Second
)

// Mod checks whether the mod is installed (claude plugin list --json, which takes half a second or more); the
// answer is kept for a while (modGoodTTL, modBadTTL).
func (l *Launcher) Mod(ctx context.Context) ModStatus {
	if l.ModDir != "" {
		return ModStatus{OK: true, Dev: true}
	}
	now := time.Now
	if l.now != nil {
		now = l.now
	}
	l.modMu.Lock()
	defer l.modMu.Unlock()
	ttl := modBadTTL
	if l.mod.OK {
		ttl = modGoodTTL
	}
	if !l.modAt.IsZero() && now().Sub(l.modAt) < ttl {
		return l.mod
	}
	// not under the caller's ctx: a page that goes away meanwhile would leave "signal: killed" as everyone's answer
	// (checkMod has a time limit of its own)
	l.mod, l.modAt = l.checkMod(context.WithoutCancel(ctx)), now()
	return l.mod
}

func (l *Launcher) checkMod(ctx context.Context) ModStatus {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	list := l.ListPlugins
	if list == nil {
		list = func(ctx context.Context) ([]byte, error) {
			cmd := exec.CommandContext(ctx, l.Claude, "plugin", "list", "--json")
			hideConsole(cmd)
			cmd.WaitDelay = 5 * time.Second // what it started may hold its output open
			return cmd.Output()
		}
	}
	var plugins []struct {
		ID      string `json:"id"`
		Version string `json:"version"` // the version recorded at install time
		// FolderVersion is set when the plugin is read straight from a folder marketplace: the version
		// sessions actually load, which moves on with the folder (git pull) while Version stays put
		FolderVersion string `json:"folderVersion"`
		Scope         string `json:"scope"`
		Enabled       bool   `json:"enabled"`
	}
	out, err := list(ctx)
	if ee := (*exec.ExitError)(nil); errors.As(err, &ee) && len(ee.Stderr) > 0 {
		err = fmt.Errorf("%w: %s", err, lastLines(string(ee.Stderr), 2))
	}
	if err == nil {
		err = json.Unmarshal(out, &plugins)
	}
	if err != nil {
		return modProblem(newError("mod_check_failed",
			"can't check whether the mod is installed (claude plugin list --json): "+err.Error(), "detail", err.Error()), "")
	}
	st := modProblem(newError("mod_not_installed", "the "+ModName+" mod is not installed"), l.installCommand())
	for _, p := range plugins {
		if !strings.HasPrefix(p.ID, ModName+"@") {
			continue
		}
		switch {
		case p.Scope == "project" || p.Scope == "local":
			// it only works in one project; sessions started in other folders won't have it
			st = modProblem(newError("mod_project_scope",
				"the mod is only installed for one project ("+p.Scope+"); install it at user scope", "scope", p.Scope), l.installCommand())
		case !p.Enabled:
			st = modProblem(newError("mod_disabled", "the mod is installed but disabled"), "claude plugin enable "+p.ID)
		default:
			version := p.Version
			if p.FolderVersion != "" {
				version = p.FolderVersion
			}
			return ModStatus{OK: true, ID: p.ID, Version: version}
		}
	}
	return st
}

// ModMarket says whether dir is a plugin marketplace that offers the mod (.claude-plugin/marketplace.json): this
// repository, or a copy of it.
func ModMarket(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "marketplace.json"))
	if err != nil {
		return false
	}
	var market struct {
		Plugins []struct {
			Name string `json:"name"`
		} `json:"plugins"`
	}
	if json.Unmarshal(data, &market) != nil {
		return false
	}
	for _, p := range market.Plugins {
		if p.Name == ModName {
			return true
		}
	}
	return false
}

func (l *Launcher) installCommand() string {
	market := GitHubMarket
	if l.Market != "" {
		market = quoteArg(l.Market)
	}
	return "claude plugin install " + ModName + " --marketplace " + market
}

// quoteArg quotes one argument of a command people will copy: double quotes when it holds spaces
// and the like (every common shell understands them).
func quoteArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"'$`&|;<>()*?!#~") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

// argv builds claude's arguments (no shell involved).
func (l *Launcher) argv(req LaunchRequest) []string {
	args := []string{l.Claude}
	if l.ModDir != "" {
		args = append(args, "--plugin-dir", l.ModDir)
	}
	args = append(args, "--name", req.Name)
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

// normalizeLaunch checks a launch request: dir must be an existing absolute directory, and a new
// account needs a name. An unsupported language is dropped.
func normalizeLaunch(req LaunchRequest) (LaunchRequest, error) {
	req.Account = strings.TrimSpace(req.Account)
	req.Name = strings.TrimSpace(req.Name)
	req.Dir = strings.TrimSpace(req.Dir)
	req.Lang = i18n.Match(req.Lang)
	if req.Account == "" && !validName(req.Name) {
		return req, ErrBadName
	}
	var err error
	if req.Dir, err = normalizeDir(req.Dir); err != nil {
		return req, err
	}
	if !validOption(launchModels, req.Model) || !validOption(launchEfforts, req.Effort) ||
		!validOption(launchPermissionModes, req.PermissionMode) || !validOption(launchResumes, req.Resume) {
		return req, newError("launch_bad_option", "unknown drop-down value")
	}
	return req, nil
}

// normalizeDir checks a folder to start a session in: an existing absolute directory, cleaned.
func normalizeDir(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if !filepath.IsAbs(dir) {
		return dir, newError("launch_dir_not_absolute", "the folder must be an absolute path")
	}
	dir = filepath.Clean(dir)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return dir, newError("launch_dir_missing", "folder does not exist: "+dir, "dir", dir)
	}
	return dir, nil
}

// Start opens a terminal that runs claude; env holds the extra environment variables.
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
	launch.Attach = l.AttachFor(launch.ID)
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

// DirEntry is one entry of the folder browser; the bridge builds path, so the page never deals with / or \.
type DirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ListDirs lists the folders under path (hidden ones left out) for the launch dialog's folder browser.
func ListDirs(path string) (dirs []DirEntry, parent string, err error) {
	dirs = []DirEntry{} // an empty list, not null, when there are no subfolders
	if !filepath.IsAbs(path) {
		return nil, "", newError("dirs_not_absolute", "need an absolute path")
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

// Roots are the top levels the browser can reach: the drive letters on Windows, / elsewhere.
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
