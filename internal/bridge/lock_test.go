package bridge

import (
	"errors"
	"path/filepath"
	"testing"
)

// TestLockData: a data folder is one bridge's at a time. A second lock fails while the first is held (a second open
// of the file counts as another bridge, as it would be another process) and leaves the first one's address in place;
// once let go, the folder can be locked again.
func TestLockData(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if BridgeAddr(dir) != "" {
		t.Fatal("no address before a bridge locked the folder")
	}
	unlock, err := LockData(dir, "127.0.0.1:7788")
	if err != nil {
		t.Fatal(err)
	}
	if got := BridgeAddr(dir); got != "127.0.0.1:7788" {
		t.Fatalf("bridge.addr = %q", got)
	}
	if _, err := LockData(dir, "127.0.0.1:7789"); !errors.Is(err, ErrDataLocked) {
		t.Fatalf("a second lock: want ErrDataLocked, got %v", err)
	}
	if got := BridgeAddr(dir); got != "127.0.0.1:7788" {
		t.Fatalf("the refused bridge changed bridge.addr to %q", got)
	}
	unlock()
	unlock() // twice is harmless
	again, err := LockData(dir, "[::1]:7789")
	if err != nil {
		t.Fatalf("let go, the folder should lock again: %v", err)
	}
	defer again()
	if got := BridgeAddr(dir); got != "[::1]:7789" {
		t.Fatalf("bridge.addr = %q", got)
	}
}
