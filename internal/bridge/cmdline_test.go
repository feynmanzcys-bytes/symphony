package bridge

import (
	"errors"
	"testing"
)

// TestCheckBatchArgs: a batch file runs through cmd.exe, whose own characters in an argument are refused with a coded
// error; a program that isn't one takes anything.
func TestCheckBatchArgs(t *testing.T) {
	for _, argv := range [][]string{
		{`C:\\Users\\me\\AppData\\Roaming\\npm\\claude.cmd`, "--name", "R&D"},
		{`claude.CMD`, "--name", "50%x%"},
		{`run.bat`, "--plugin-dir", `C:\\a "b"`},
		{`x.cmd`, "a|b"}, {`x.cmd`, "a^b"}, {`x.cmd`, "a<b"}, {`x.cmd`, "a>b"}, {`x.cmd`, "a!b"},
	} {
		var e *Error
		if err := checkBatchArgs(argv); !errors.As(err, &e) || e.Code != "cmd_unsafe_arg" || e.Params["arg"] != argv[len(argv)-1] {
			t.Errorf("%q: want cmd_unsafe_arg naming the argument, got %v", argv, err)
		}
	}
	for _, argv := range [][]string{
		{`claude.cmd`, "--name", "Ann-Marie O'Neil", "--model", "haiku", "--permission-mode", "default", `C:\\Users\\me\\my project`},
		{`/usr/bin/claude`, "--name", "R&D"},
		{`claude.exe`, "--name", "50%x%"},
		{`claude`, "--name", "a|b"},
		nil,
	} {
		if err := checkBatchArgs(argv); err != nil {
			t.Errorf("%q: should pass, got %v", argv, err)
		}
	}
}
