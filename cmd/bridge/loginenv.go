package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// loginEnvNames are the variables -autostart on carries from the shell it runs in to the bridge started at login, which
// gets nothing but PATH from systemd or launchd: where Claude Code keeps its settings and login, the proxy, the extra
// certificate authorities. Without them a session the page starts finds no mod or sits at a login screen. Nothing else
// is carried: no ANTHROPIC_* (keys), no NODE_OPTIONS (code that runs in every node process).
var loginEnvNames = []string{
	"CLAUDE_CONFIG_DIR",
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY",
	"http_proxy", "https_proxy", "no_proxy", "all_proxy",
	"NODE_EXTRA_CA_CERTS", "SSL_CERT_FILE", "SSL_CERT_DIR",
}

// loginEnvFile is where -autostart on keeps them: in the data folder, readable by this user only (a proxy's address may
// carry a password; the unit file or the plist, which anyone on the machine may read, names only this file).
const loginEnvFile = "autostart-env.json"

type loginEnv struct {
	Env map[string]string `json:"env"`
}

// captureLoginEnv is what of loginEnvNames is set (not empty) in getenv's environment.
func captureLoginEnv(getenv func(string) string) map[string]string {
	env := map[string]string{}
	for _, name := range loginEnvNames {
		if v := getenv(name); v != "" {
			env[name] = v
		}
	}
	return env
}

// saveLoginEnv writes env to path, readable by this user only, replacing what was there; with nothing to keep it
// removes the file. It answers the names it kept, sorted.
func saveLoginEnv(path string, env map[string]string) ([]string, error) {
	if len(env) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		return nil, nil
	}
	data, err := json.MarshalIndent(loginEnv{Env: env}, "", "  ")
	if err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return nil, err
	}
	if err := os.Chmod(tmp, 0o600); err != nil { // a .tmp left by someone else's umask
		os.Remove(tmp)
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return nil, err
	}
	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	slices.Sort(names)
	return names, nil
}

// applyLoginEnv sets, from the file -autostart on wrote, each of loginEnvNames that the bridge's own environment
// doesn't have (what systemd, launchd or the Run key gave it wins). A name not in loginEnvNames is never set, whatever
// the file says. It answers the names it set, sorted.
func applyLoginEnv(path string, getenv func(string) string, setenv func(string, string) error) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file loginEnv
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	var set []string
	for _, name := range loginEnvNames {
		v, ok := file.Env[name]
		if !ok || v == "" || getenv(name) != "" || strings.ContainsRune(v, 0) {
			continue
		}
		if err := setenv(name, v); err != nil {
			return set, err
		}
		set = append(set, name)
	}
	slices.Sort(set)
	return set, nil
}

// loginEnvPath is the file's place in a data folder, absolute.
func loginEnvPath(dataDir string) string {
	if abs, err := filepath.Abs(dataDir); err == nil {
		dataDir = abs
	}
	return filepath.Join(dataDir, loginEnvFile)
}
