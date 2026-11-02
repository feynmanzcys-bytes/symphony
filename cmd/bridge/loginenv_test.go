package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// TestLoginEnv: -autostart on keeps only the variables a login-started bridge needs (settings, proxy, certificates),
// in a file only this user may read, and the bridge sets from it only those of them its own environment lacks; a name
// the file holds beyond the list is never set; with nothing to keep, the file goes.
func TestLoginEnv(t *testing.T) {
	shell := map[string]string{
		"HTTPS_PROXY":       "http://me:secret@proxy.example:3128",
		"no_proxy":          "localhost,127.0.0.1",
		"CLAUDE_CONFIG_DIR": "/home/me/.claude-work",
		"ANTHROPIC_API_KEY": "sk-not-for-the-file",
		"NODE_OPTIONS":      "--require /tmp/evil.js",
		"SSL_CERT_FILE":     "",
	}
	got := captureLoginEnv(func(k string) string { return shell[k] })
	if len(got) != 3 || got["HTTPS_PROXY"] == "" || got["no_proxy"] == "" || got["CLAUDE_CONFIG_DIR"] == "" {
		t.Fatalf("captured %v, want the proxy, no_proxy and CLAUDE_CONFIG_DIR only", got)
	}
	path := filepath.Join(t.TempDir(), loginEnvFile)
	names, err := saveLoginEnv(path, got)
	if err != nil || !slices.Equal(names, []string{"CLAUDE_CONFIG_DIR", "HTTPS_PROXY", "no_proxy"}) {
		t.Fatalf("saved %v %v", names, err)
	}
	if fi, err := os.Stat(path); err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600) {
		t.Fatalf("the file should be the owner's only: %v %v", fi.Mode(), err)
	}
	// the file is edited by hand to carry something else: never set
	if err := os.WriteFile(path, []byte(`{"env":{"HTTPS_PROXY":"http://proxy.example:3128","CLAUDE_CONFIG_DIR":"/c","NODE_OPTIONS":"--require /tmp/x.js","PATH":"/evil"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"CLAUDE_CONFIG_DIR": "/from/systemd"}
	set, err := applyLoginEnv(path, func(k string) string { return env[k] }, func(k, v string) error { env[k] = v; return nil })
	if err != nil || !slices.Equal(set, []string{"HTTPS_PROXY"}) {
		t.Fatalf("set %v %v, want only HTTPS_PROXY (CLAUDE_CONFIG_DIR is the environment's own)", set, err)
	}
	if env["CLAUDE_CONFIG_DIR"] != "/from/systemd" || env["NODE_OPTIONS"] != "" || env["PATH"] != "" {
		t.Fatalf("what the environment has wins, and names beyond the list are never set: %v", env)
	}
	if _, err := applyLoginEnv(filepath.Join(t.TempDir(), "missing.json"), os.Getenv, func(string, string) error { return nil }); err == nil {
		t.Fatal("a missing file should say so")
	}
	if names, err := saveLoginEnv(path, nil); err != nil || names != nil {
		t.Fatalf("nothing to keep: %v %v", names, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("with nothing to keep the file should go: %v", err)
	}
	if _, err := saveLoginEnv(path, nil); err != nil {
		t.Fatalf("removing a file that isn't there is fine: %v", err)
	}
}
