package bridge

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// One bridge per data folder: two bridges on one folder (at two addresses) would each take the other's running tasks
// for ones a stopped bridge left behind and end them, and write over each other's files. The bridge locks <data>/lock
// before it reads anything there, and notes its address in <data>/bridge.addr, so a second one can tell where the
// first runs.

const (
	lockFile = "lock"        // in the data folder: locked while a bridge uses the folder
	addrFile = "bridge.addr" // in the data folder: host:port of the bridge holding the lock
)

// ErrDataLocked is a data folder another bridge (another process) holds.
var ErrDataLocked = errors.New("another symphony bridge uses this data folder")

// heldLocks keeps the lock files this process holds open while it runs: a file the garbage collector closed would let
// another bridge in.
var heldLocks sync.Map // *os.File → struct{}

// LockData locks the data folder for this process, which listens at addr (host:port), and notes the address for the
// next bridge that tries (BridgeAddr). ErrDataLocked means another process has it; unlock lets it go (tests: a
// bridge holds it until it exits).
func LockData(dir, addr string) (unlock func(), err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := lockPath(filepath.Join(dir, lockFile))
	if err != nil {
		return nil, err
	}
	heldLocks.Store(f, struct{}{})
	unlock = func() {
		if _, held := heldLocks.LoadAndDelete(f); held {
			f.Close()
		}
	}
	if err := writePrivate(filepath.Join(dir, addrFile), []byte(addr+"\n")); err != nil {
		unlock()
		return nil, err
	}
	return unlock, nil
}

// BridgeAddr is the address (host:port) the bridge that locked the data folder noted, or "" when there is none. It may
// be an old one: ask what answers there before believing it (ProveKey).
func BridgeAddr(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, addrFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
