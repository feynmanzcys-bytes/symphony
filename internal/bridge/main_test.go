package bridge

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain keeps git away from the config of the machine the tests run on (~/.gitconfig, the system's): the tests'
// repositories behave the same everywhere, and a test that wants a config of its own points GIT_CONFIG_GLOBAL at it.
func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "symphony-bridge-test")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	global := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(global, nil, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	os.Setenv("GIT_CONFIG_GLOBAL", global)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return m.Run()
}
