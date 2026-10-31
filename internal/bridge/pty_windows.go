//go:build windows

package bridge

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// ConPTY (Windows 10 1809 and later): a pseudo console the child writes to and reads from through two pipes.
var (
	kernel32                              = syscall.NewLazyDLL("kernel32.dll")
	procCreatePseudoConsole               = kernel32.NewProc("CreatePseudoConsole")
	procResizePseudoConsole               = kernel32.NewProc("ResizePseudoConsole")
	procClosePseudoConsole                = kernel32.NewProc("ClosePseudoConsole")
	procInitializeProcThreadAttributeList = kernel32.NewProc("InitializeProcThreadAttributeList")
	procUpdateProcThreadAttribute         = kernel32.NewProc("UpdateProcThreadAttribute")
	procDeleteProcThreadAttributeList     = kernel32.NewProc("DeleteProcThreadAttributeList")
)

const (
	procThreadAttributePseudoConsole = 0x20016
	extendedStartupInfoPresent       = 0x00080000
)

// coordArg packs a COORD (passed by value) into one argument: X in the low 16 bits, Y in the high ones.
func coordArg(cols, rows int) uintptr {
	return uintptr(uint32(uint16(cols)) | uint32(uint16(rows))<<16)
}

type startupInfoEx struct {
	syscall.StartupInfo
	AttributeList *byte
}

type windowsProc struct {
	console syscall.Handle
	in, out *os.File // our ends: in is the console's input (we write), out its output (we read)
	process syscall.Handle
	waited  chan struct{}
	err     error
}

func utf16Block(env []string) []uint16 {
	var block []uint16
	for _, kv := range env {
		block = append(block, syscall.StringToUTF16(kv)...)
	}
	return append(block, 0)
}

func startPTY(dir string, env []string, argv []string, cols, rows int) (ptyProc, error) {
	var inR, inW, outR, outW syscall.Handle
	if err := syscall.CreatePipe(&inR, &inW, nil, 0); err != nil {
		return nil, err
	}
	if err := syscall.CreatePipe(&outR, &outW, nil, 0); err != nil {
		syscall.CloseHandle(inR)
		syscall.CloseHandle(inW)
		return nil, err
	}
	var console syscall.Handle
	if hr, _, _ := procCreatePseudoConsole.Call(coordArg(cols, rows), uintptr(inR), uintptr(outW), 0, uintptr(unsafe.Pointer(&console))); hr != 0 {
		for _, h := range []syscall.Handle{inR, inW, outR, outW} {
			syscall.CloseHandle(h)
		}
		return nil, fmt.Errorf("CreatePseudoConsole: HRESULT 0x%x", hr)
	}
	// the console keeps its own copies of its ends
	syscall.CloseHandle(inR)
	syscall.CloseHandle(outW)
	p := &windowsProc{console: console, in: os.NewFile(uintptr(inW), "conpty-in"), out: os.NewFile(uintptr(outR), "conpty-out"), waited: make(chan struct{})}

	var size uintptr
	procInitializeProcThreadAttributeList.Call(0, 1, 0, uintptr(unsafe.Pointer(&size)))
	attrs := make([]byte, size)
	if ok, _, err := procInitializeProcThreadAttributeList.Call(uintptr(unsafe.Pointer(&attrs[0])), 1, 0, uintptr(unsafe.Pointer(&size))); ok == 0 {
		p.closeAll()
		return nil, fmt.Errorf("InitializeProcThreadAttributeList: %v", err)
	}
	defer procDeleteProcThreadAttributeList.Call(uintptr(unsafe.Pointer(&attrs[0])))
	if ok, _, err := procUpdateProcThreadAttribute.Call(uintptr(unsafe.Pointer(&attrs[0])), 0, procThreadAttributePseudoConsole,
		uintptr(console), unsafe.Sizeof(console), 0, 0); ok == 0 {
		p.closeAll()
		return nil, fmt.Errorf("UpdateProcThreadAttribute: %v", err)
	}

	si := &startupInfoEx{AttributeList: &attrs[0]}
	si.Cb = uint32(unsafe.Sizeof(*si))
	var pi syscall.ProcessInformation
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = syscall.EscapeArg(a)
	}
	block := utf16Block(env)
	cmdline, _ := syscall.UTF16PtrFromString(strings.Join(quoted, " "))
	cwd, _ := syscall.UTF16PtrFromString(dir)
	err := syscall.CreateProcess(nil, cmdline, nil, nil, false, extendedStartupInfoPresent|syscall.CREATE_UNICODE_ENVIRONMENT,
		&block[0], cwd, &si.StartupInfo, &pi)
	if err != nil {
		p.closeAll()
		return nil, err
	}
	syscall.CloseHandle(pi.Thread)
	p.process = pi.Process
	return p, nil
}

func (p *windowsProc) closeAll() {
	procClosePseudoConsole.Call(uintptr(p.console))
	p.in.Close()
	p.out.Close()
}

func (p *windowsProc) Read(b []byte) (int, error)  { return p.out.Read(b) }
func (p *windowsProc) Write(b []byte) (int, error) { return p.in.Write(b) }
func (p *windowsProc) Resize(cols, rows int) error {
	if hr, _, _ := procResizePseudoConsole.Call(uintptr(p.console), coordArg(cols, rows)); hr != 0 {
		return fmt.Errorf("ResizePseudoConsole: HRESULT 0x%x", hr)
	}
	return nil
}

// Wait waits for the process; then it closes the console, which ends the output (and the reader).
func (p *windowsProc) Wait() error {
	if _, err := syscall.WaitForSingleObject(p.process, syscall.INFINITE); err != nil {
		p.err = err
	} else {
		var code uint32
		if err := syscall.GetExitCodeProcess(p.process, &code); err != nil {
			p.err = err
		} else if code != 0 {
			p.err = fmt.Errorf("exit status %d", code)
		}
	}
	syscall.CloseHandle(p.process)
	procClosePseudoConsole.Call(uintptr(p.console))
	p.in.Close()
	return p.err
}

func (p *windowsProc) Close() error { return p.out.Close() }
func (p *windowsProc) Kill() error {
	if p.process == 0 {
		return errors.New("not started")
	}
	return syscall.TerminateProcess(p.process, 1)
}
