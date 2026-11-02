//go:build windows

package bridge

import (
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

var procAssocQueryStringW = syscall.NewLazyDLL("shlwapi.dll").NewProc("AssocQueryStringW")

// httpHandler is the command line Windows runs to open an http address: the default browser's, with %1 for the
// address. "" when Windows names none.
func httpHandler() string {
	const assocfIsProtocol = 0x00001000
	const assocstrCommand = 1
	if procAssocQueryStringW.Find() != nil {
		return ""
	}
	assoc, _ := syscall.UTF16PtrFromString("http")
	verb, _ := syscall.UTF16PtrFromString("open")
	buf := make([]uint16, 4096)
	n := uint32(len(buf))
	r, _, _ := procAssocQueryStringW.Call(assocfIsProtocol, assocstrCommand, uintptr(unsafe.Pointer(assoc)),
		uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r != 0 { // S_OK only
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// BrowserForFile starts the default browser on a local file's address (FileURL) through the http protocol's own
// command: the program that opens .html files may be no browser at all (an editor), or none Windows trusts, and then
// Windows asks which app to use and the office never opens. nil when Windows names no command for http (the caller
// falls back to BrowserCommand).
func BrowserForFile(fileURL string) *exec.Cmd {
	exe, line, ok := HandlerCommand(httpHandler(), fileURL, os.Getenv)
	if !ok {
		return nil
	}
	cmd := exec.Command(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line} // the command line as Windows gave it, not quoted again
	return cmd
}
