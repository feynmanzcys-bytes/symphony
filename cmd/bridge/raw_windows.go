//go:build windows

package main

import (
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleMode             = kernel32.NewProc("SetConsoleMode")
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procSetConsoleCP               = kernel32.NewProc("SetConsoleCP")
	procSetConsoleOutputCP         = kernel32.NewProc("SetConsoleOutputCP")
)

const (
	enableProcessedInput            = 0x0001
	enableLineInput                 = 0x0002
	enableEchoInput                 = 0x0004
	enableVirtualTerminalInput      = 0x0200
	enableProcessedOutput           = 0x0001
	enableVirtualTerminalProcessing = 0x0004
	disableNewlineAutoReturn        = 0x0008
	utf8CodePage                    = 65001
)

type consoleScreenBufferInfo struct {
	Size              [2]int16
	CursorPosition    [2]int16
	Attributes        uint16
	Window            [4]int16 // left, top, right, bottom
	MaximumWindowSize [2]int16
}

// terminalFiles are where attach reads the keys and writes the screen: its standard input and output, or the
// console's own (CONIN$, CONOUT$) when they aren't the console: in the window the bridge opens for it, they are NUL.
func terminalFiles() (in, out *os.File) {
	in, out = os.Stdin, os.Stdout
	var mode uint32
	if syscall.GetConsoleMode(syscall.Handle(in.Fd()), &mode) != nil {
		if f, err := os.OpenFile("CONIN$", os.O_RDWR, 0); err == nil {
			in = f
		}
	}
	if syscall.GetConsoleMode(syscall.Handle(out.Fd()), &mode) != nil {
		if f, err := os.OpenFile("CONOUT$", os.O_RDWR, 0); err == nil {
			out = f
		}
	}
	return in, out
}

func consoleSize(out *os.File) (cols, rows int) {
	var info consoleScreenBufferInfo
	if ok, _, _ := procGetConsoleScreenBufferInfo.Call(out.Fd(), uintptr(unsafe.Pointer(&info))); ok == 0 {
		return 80, 24
	}
	return int(info.Window[2]-info.Window[0]) + 1, int(info.Window[3]-info.Window[1]) + 1
}

// watchSize polls the window's size (Windows sends no signal) and calls fn when it changes.
func watchSize(out *os.File, fn func(cols, rows int)) {
	cols, rows := consoleSize(out)
	for {
		time.Sleep(300 * time.Millisecond)
		if c, r := consoleSize(out); c != cols || r != rows {
			cols, rows = c, r
			fn(c, r)
		}
	}
}

// rawMode turns the console's own line editing and echo off, keys come as VT sequences, output is VT processed
// and UTF-8 both ways; it returns what restores the console.
func rawMode(inFile, outFile *os.File) (func(), error) {
	in, out := syscall.Handle(inFile.Fd()), syscall.Handle(outFile.Fd())
	var inMode, outMode uint32
	if err := syscall.GetConsoleMode(in, &inMode); err != nil {
		return nil, err
	}
	if err := syscall.GetConsoleMode(out, &outMode); err != nil {
		return nil, err
	}
	set := func(h syscall.Handle, mode uint32) error {
		if ok, _, err := procSetConsoleMode.Call(uintptr(h), uintptr(mode)); ok == 0 {
			return err
		}
		return nil
	}
	if err := set(in, inMode&^(enableEchoInput|enableLineInput|enableProcessedInput)|enableVirtualTerminalInput); err != nil {
		return nil, err
	}
	if err := set(out, outMode|enableProcessedOutput|enableVirtualTerminalProcessing|disableNewlineAutoReturn); err != nil {
		_ = set(in, inMode)
		return nil, err
	}
	procSetConsoleCP.Call(utf8CodePage)
	procSetConsoleOutputCP.Call(utf8CodePage)
	return func() {
		_ = set(in, inMode)
		_ = set(out, outMode)
	}, nil
}
