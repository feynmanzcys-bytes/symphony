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

func consoleSize() (cols, rows int) {
	var info consoleScreenBufferInfo
	h, _ := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if ok, _, _ := procGetConsoleScreenBufferInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&info))); ok == 0 {
		return 80, 24
	}
	return int(info.Window[2]-info.Window[0]) + 1, int(info.Window[3]-info.Window[1]) + 1
}

// watchSize polls the window's size (Windows sends no signal) and calls fn when it changes.
func watchSize(fn func(cols, rows int)) {
	cols, rows := consoleSize()
	for {
		time.Sleep(300 * time.Millisecond)
		if c, r := consoleSize(); c != cols || r != rows {
			cols, rows = c, r
			fn(c, r)
		}
	}
}

// rawMode turns the console's own line editing and echo off, keys come as VT sequences, output is VT processed
// and UTF-8 both ways; it returns what restores the console.
func rawMode() (func(), error) {
	in, _ := syscall.GetStdHandle(syscall.STD_INPUT_HANDLE)
	out, _ := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
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
	_ = os.Stdout
	return func() {
		_ = set(in, inMode)
		_ = set(out, outMode)
	}, nil
}
