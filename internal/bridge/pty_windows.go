//go:build windows

package bridge

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
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
	procCreateJobObject                   = kernel32.NewProc("CreateJobObjectW")
	procAssignProcessToJobObject          = kernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject                = kernel32.NewProc("TerminateJobObject")
	procResumeThread                      = kernel32.NewProc("ResumeThread")
)

const (
	procThreadAttributePseudoConsole = 0x20016
	extendedStartupInfoPresent       = 0x00080000
	createSuspended                  = 0x00000004
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
	jobMu   sync.Mutex
	job     syscall.Handle // the job object the process and whatever it starts are in, so Kill gets them all; 0 when none
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
	// started suspended, so it is in its job before it can start anything
	err := syscall.CreateProcess(nil, cmdline, nil, nil, false, extendedStartupInfoPresent|syscall.CREATE_UNICODE_ENVIRONMENT|createSuspended,
		&block[0], cwd, &si.StartupInfo, &pi)
	if err != nil {
		p.closeAll()
		return nil, err
	}
	p.process = pi.Process
	if job, _, _ := procCreateJobObject.Call(0, 0); job != 0 {
		if ok, _, _ := procAssignProcessToJobObject.Call(job, uintptr(pi.Process)); ok != 0 {
			p.job = syscall.Handle(job)
		} else {
			syscall.CloseHandle(syscall.Handle(job)) // no job then; Kill ends the process alone
		}
	}
	if n, _, err := procResumeThread.Call(uintptr(pi.Thread)); n == 0xFFFFFFFF {
		syscall.CloseHandle(pi.Thread)
		p.Kill()
		syscall.CloseHandle(p.process)
		p.closeJob()
		p.closeAll()
		return nil, fmt.Errorf("ResumeThread: %v", err)
	}
	syscall.CloseHandle(pi.Thread)
	return p, nil
}

func (p *windowsProc) closeJob() {
	p.jobMu.Lock()
	defer p.jobMu.Unlock()
	if p.job != 0 {
		syscall.CloseHandle(p.job)
		p.job = 0
	}
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

func (p *windowsProc) Close() error {
	p.closeJob()
	return p.out.Close()
}

// Kill ends the process and whatever it started (its job), or the process alone when it has no job.
func (p *windowsProc) Kill() error {
	if p.process == 0 {
		return errors.New("not started")
	}
	p.jobMu.Lock()
	defer p.jobMu.Unlock()
	if p.job != 0 {
		if ok, _, _ := procTerminateJobObject.Call(uintptr(p.job), 1); ok != 0 {
			return nil
		}
	}
	return syscall.TerminateProcess(p.process, 1)
}
