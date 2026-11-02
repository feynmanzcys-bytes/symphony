//go:build !windows

package bridge

import "os/exec"

// BrowserForFile is Windows's way to the default browser (browser_windows.go); elsewhere BrowserCommand is the way.
func BrowserForFile(fileURL string) *exec.Cmd { return nil }
