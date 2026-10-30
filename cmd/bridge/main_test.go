package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestVersion builds the bridge the way .github/release.sh does and checks that -version prints the
// version set with -ldflags, so a renamed variable can't quietly turn releases into "dev".
func TestVersion(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("needs the go command")
	}
	exe := filepath.Join(t.TempDir(), "symphony")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	build := exec.Command(gobin, "build", "-trimpath", "-ldflags", "-s -w -X main.version=v9.8.7", "-o", exe, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command(exe, "-version").Output()
	if err != nil {
		t.Fatalf("-version: %v", err)
	}
	if want := "symphony v9.8.7 (" + runtime.GOOS + "/" + runtime.GOARCH + ")\n"; string(out) != want {
		t.Errorf("-version printed %q, want %q", out, want)
	}
}
