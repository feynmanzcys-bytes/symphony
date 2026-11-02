package bridge

import (
	"strings"
)

// cmdMeta are the characters cmd.exe reads as its own in a command line (variables, escapes, pipes, redirections,
// delayed expansion, quoting), which syscall.EscapeArg doesn't escape for it.
const cmdMeta = "%^&|<>!\""

// isBatch says whether program is a Windows batch file (claude.cmd, as npm installs it), which CreateProcess runs
// through cmd.exe /c.
func isBatch(program string) bool {
	p := strings.ToLower(program)
	return strings.HasSuffix(p, ".cmd") || strings.HasSuffix(p, ".bat")
}

// checkBatchArgs refuses, with a coded error, an argument that cmd.exe would read as its own when the program is a
// batch file: an employee named R&D would end claude.cmd's command line at --name R and start another.
func checkBatchArgs(argv []string) error {
	if len(argv) == 0 || !isBatch(argv[0]) {
		return nil
	}
	for _, a := range argv[1:] {
		if strings.ContainsAny(a, cmdMeta) {
			return newError("cmd_unsafe_arg", "can't pass "+a+" through cmd.exe (the program is a batch file): it holds one of "+cmdMeta, "arg", a, "chars", cmdMeta)
		}
	}
	return nil
}
