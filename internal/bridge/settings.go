package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Settings are the bridge's own settings, which the pages' settings change (settings.json in the data folder). An
// empty field stands for its default.
type Settings struct {
	// BotHome is where an employee started from the page without a working folder gets a folder of its own
	// (botFolder); empty means Hub.BotHomeDefault
	BotHome string `json:"botHome,omitempty"`
}

// SettingsView is what GET /api/settings answers: the settings in effect, and the defaults to go back to.
type SettingsView struct {
	BotHome        string `json:"botHome"`        // the one set, else the default
	BotHomeDefault string `json:"botHomeDefault"` // where the employees' folders go when none is set
}

var (
	errBotHomeUnset = newError("bot_home_unset", "no folder is set for the employees' own folders (and there is no home folder to make one in); set one in the settings, or give a working folder")
	// errSettingsWrite is settings.json that couldn't be written; the bridge's log has the details
	errSettingsWrite = newError("data_write_failed", "can't write settings.json in the data folder; the bridge's log says why", "file", "settings.json")
)

// DefaultBotHome is where the employees' own folders go unless the settings name another place: symphony-bots in the
// home folder ("" without one).
func DefaultBotHome() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "symphony-bots")
}

// loadSettings reads settings.json; a missing file means the defaults. One that doesn't parse (an empty one too) is set
// aside as a damaged log is (loadLog), and the bridge starts on the defaults; one that can't be read stops it.
func loadSettings(h *Hub, path string) (Settings, error) {
	var s Settings
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		aside := fmt.Sprintf("%s.corrupt-%d", path, h.now().Unix())
		if os.Rename(path, aside) != nil {
			return Settings{}, &DamagedFile{Path: path, Err: err}
		}
		h.warn("log.data_set_aside", "file", path, "reason", err.Error(), "aside", aside)
		return Settings{}, nil
	}
	return s, nil
}

// checkBotHome checks a folder for the employees' own folders: empty (the default), or an absolute path, cleaned, that
// isn't a file and isn't in the data folder (the tokens and the page key are there). It needn't exist yet: the first
// folder made in it makes it.
func checkBotHome(dir, dataDir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", nil
	}
	if !filepath.IsAbs(dir) {
		return dir, newError("launch_dir_not_absolute", "the folder must be an absolute path")
	}
	dir = filepath.Clean(dir)
	if fi, err := os.Stat(dir); err == nil && !fi.IsDir() {
		return dir, newError("bot_home_not_dir", dir+" is a file, not a folder", "dir", dir)
	}
	if dataDir != "" && within(dir, filepath.Clean(dataDir)) {
		return dir, newError("bot_home_in_data", dir+" is in the bridge's data folder, which holds the tokens; pick another folder", "dir", dir)
	}
	return dir, nil
}

// Settings returns the settings in effect.
func (h *Hub) Settings() SettingsView {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.settingsViewLocked()
}

func (h *Hub) settingsViewLocked() SettingsView {
	return SettingsView{BotHome: h.botHomeLocked(), BotHomeDefault: h.BotHomeDefault}
}

// botHomeLocked is the folder the employees' own folders go in: the one set, else the default.
func (h *Hub) botHomeLocked() string {
	if h.settings.BotHome != "" {
		return h.settings.BotHome
	}
	return h.BotHomeDefault
}

// SetSettings checks new settings and keeps them (settings.json, written at once): BotHome an absolute folder, or empty
// for the default. It answers the settings in effect.
func (h *Hub) SetSettings(s Settings) (SettingsView, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	home, err := checkBotHome(s.BotHome, h.dataDir)
	if err != nil {
		return SettingsView{}, err
	}
	s.BotHome = home
	if h.dataDir != "" {
		path := filepath.Join(h.dataDir, "settings.json")
		data, err := json.MarshalIndent(s, "", "  ")
		if err == nil {
			err = writeSynced(path, data)
		}
		if err != nil {
			h.warnWrite(path, err)
			return SettingsView{}, errSettingsWrite
		}
	}
	h.settings = s
	return h.settingsViewLocked(), nil
}

// botFolderFailed is a folder for an employee that couldn't be made, or made a git repository.
func botFolderFailed(dir string, err error) *Error {
	return newError("bot_home_failed", "can't make the folder "+dir+": "+err.Error(), "dir", dir, "detail", err.Error())
}

// botFolder is the folder an employee started without a working folder works in. That's the one made for it before
// (Account.Home), made again when it's gone; else a new one in the bot home, named after it (<bot home>/<name>, then
// -2, -3 ... past whatever is there already, so nothing there is ever taken over), made now and bound to the account.
// A folder it makes becomes a git repository with a first, empty commit, which a task can branch off (without git it
// stays a plain folder, and the log says so). made says whether the folder was made now; it holds nothing but its
// .git then.
func (h *Hub) botFolder(account string) (dir string, made bool, err error) {
	h.mu.Lock()
	a := h.accounts[account]
	if a == nil {
		h.mu.Unlock()
		return "", false, ErrUnknownAccount
	}
	name, bound, home, dataDir := a.Name, a.Home, h.botHomeLocked(), h.dataDir
	h.mu.Unlock()

	if bound != "" {
		if isDir(bound) {
			return bound, false, nil
		}
		if _, err := os.Lstat(bound); err == nil {
			return "", false, botFolderFailed(bound, errors.New("something that isn't a folder is in its place"))
		}
		if err := os.MkdirAll(filepath.Dir(bound), 0o755); err != nil {
			return "", false, botFolderFailed(bound, err)
		}
		if err := os.Mkdir(bound, 0o755); err != nil {
			return "", false, botFolderFailed(bound, err)
		}
		dir = bound
	} else {
		if home == "" {
			return "", false, errBotHomeUnset
		}
		// (settings.json may have been edited by hand)
		if _, err := checkBotHome(home, dataDir); err != nil {
			return "", false, err
		}
		if err := os.MkdirAll(home, 0o755); err != nil {
			return "", false, botFolderFailed(home, err)
		}
		if dir, err = newFolder(home, slugOr(name, "employee")); err != nil {
			return "", false, botFolderFailed(home, err)
		}
	}
	if _, lerr := exec.LookPath("git"); lerr != nil {
		h.warn("log.bot_home_no_git", "dir", dir)
	} else if err := gitInit(dir, "symphony: "+name+"'s folder"); err != nil {
		_ = os.RemoveAll(dir) // made just now: nothing but what git made is in it
		return "", false, botFolderFailed(dir, err)
	}
	if bound == "" {
		if err := h.bindHome(account, dir); err != nil {
			_ = os.RemoveAll(dir)
			return "", false, err
		}
	}
	return dir, true, nil
}

// newFolder makes a new folder in parent named name, or name-2, name-3 ... when that is taken (a file or a folder,
// in any case where the file system ignores case), and returns its path.
func newFolder(parent, name string) (string, error) {
	for n := 1; n <= 1000; n++ {
		p := filepath.Join(parent, name)
		if n > 1 {
			p += "-" + strconv.Itoa(n)
		}
		if _, err := os.Lstat(p); err == nil {
			continue
		}
		if err := os.Mkdir(p, 0o755); err != nil {
			if errors.Is(err, fs.ErrExist) {
				continue // made meanwhile
			}
			return "", err
		}
		return p, nil
	}
	return "", errors.New("every name from " + name + " to " + name + "-1000 is taken")
}

// bindHome records the folder made for an account (Account.Home).
func (h *Hub) bindHome(account, dir string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	a := h.accounts[account]
	if a == nil {
		return ErrUnknownAccount
	}
	a.Home = dir
	if err := h.saveLocked(); err != nil {
		a.Home = ""
		return errAccountsWrite
	}
	h.broadcastLocked(nil)
	return nil
}
