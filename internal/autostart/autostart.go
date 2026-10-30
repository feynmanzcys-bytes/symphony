// Package autostart starts the symphony bridge when the user logs in: a systemd user service on Linux, a launchd
// agent on macOS, a value under the registry's Run key on Windows. Enable writes the entry and starts the bridge right
// away; Disable stops it (Windows: from the next login on) and removes the entry.
package autostart

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Entry is what to start at login.
type Entry struct {
	Exe  string   // the symphony program, an absolute path
	Args []string // its flags; -open=false is added, there's nobody to show a browser to at login
	Path string   // PATH for the bridge on Linux and macOS, so it finds claude and tmux (Windows starts it with the user's own)
}

// System is where the entry goes. Run and Start are swapped for fakes in tests.
type System struct {
	GOOS   string
	Name   string // the entry's name: symphony.service, the launchd label's last part, the Run value
	Home   string // the home folder
	Config string // the user config folder: ~/.config on Linux
	UID    int    // the user id, for launchctl's gui/<uid> domain
	// Run runs systemctl, launchctl or reg and returns their error, their output included.
	Run func(argv ...string) error
	// Start starts the bridge now on Windows, in a console window of its own.
	Start func(argv ...string) error
}

// RunKey is the registry key whose values Windows starts at login.
const RunKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

// Default is the System of this computer.
func Default(name string) System {
	s := System{GOOS: runtime.GOOS, Name: name, UID: os.Getuid(), Run: run, Start: startConsole}
	s.Home, _ = os.UserHomeDir()
	s.Config, _ = os.UserConfigDir()
	return s
}

func run(argv ...string) error {
	out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", strings.Join(argv, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func startConsole(argv ...string) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	newConsole(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// Enable makes the bridge start at login and starts it now. It returns where the entry is.
func (s System) Enable(e Entry) (string, error) {
	argv := append(append([]string{e.Exe}, e.Args...), "-open=false")
	switch s.GOOS {
	case "windows":
		data := make([]string, len(argv))
		for i, a := range argv {
			data[i] = winArg(a)
		}
		if err := s.Run("reg", "add", RunKey, "/v", s.Name, "/t", "REG_SZ", "/d", strings.Join(data, " "), "/f"); err != nil {
			return "", err
		}
		return RunKey + `\` + s.Name, s.Start(argv...)
	case "darwin":
		path := s.plistPath()
		if err := write(path, s.plist(argv, e.Path)); err != nil {
			return "", err
		}
		_ = s.Run("launchctl", "bootout", s.domain()+"/"+s.label()) // a copy from before, if any
		if err := s.Run("launchctl", "bootstrap", s.domain(), path); err != nil {
			os.Remove(path)
			return "", err
		}
		return path, nil
	default:
		path := s.unitPath()
		if err := write(path, s.unit(argv, e.Path)); err != nil {
			return "", err
		}
		for _, cmd := range [][]string{{"daemon-reload"}, {"enable", s.Name + ".service"}, {"restart", s.Name + ".service"}} {
			if err := s.Run(append([]string{"systemctl", "--user"}, cmd...)...); err != nil {
				os.Remove(path)
				_ = s.Run("systemctl", "--user", "daemon-reload")
				return "", err
			}
		}
		return path, nil
	}
}

// Disable stops the bridge started at login (on Windows it keeps running until its window is closed) and removes
// the entry. It returns where the entry was, or "" when there was none.
func (s System) Disable() (string, error) {
	switch s.GOOS {
	case "windows":
		if s.Run("reg", "query", RunKey, "/v", s.Name) != nil {
			return "", nil
		}
		return RunKey + `\` + s.Name, s.Run("reg", "delete", RunKey, "/v", s.Name, "/f")
	case "darwin":
		path := s.plistPath()
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		_ = s.Run("launchctl", "bootout", s.domain()+"/"+s.label())
		return path, os.Remove(path)
	default:
		path := s.unitPath()
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		if err := s.Run("systemctl", "--user", "disable", "--now", s.Name+".service"); err != nil {
			return "", err
		}
		if err := os.Remove(path); err != nil {
			return "", err
		}
		return path, s.Run("systemctl", "--user", "daemon-reload")
	}
}

func write(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// ---------- Linux: a systemd user service ----------

func (s System) unitPath() string {
	return filepath.Join(s.Config, "systemd", "user", s.Name+".service")
}

func (s System) unit(argv []string, path string) string {
	line := make([]string, len(argv))
	for i, a := range argv {
		line[i] = systemdQuote(strings.ReplaceAll(a, "$", "$$"))
	}
	return `# Starts the symphony bridge when you log in. Made by "symphony -autostart on"; "symphony -autostart off" removes it.
[Unit]
Description=symphony bridge, the office for Claude Code sessions

[Service]
ExecStart=` + strings.Join(line, " ") + `
Environment=` + systemdQuote("PATH="+path) + `
# stopping the bridge leaves the sessions it started in tmux running
KillMode=process
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`
}

// systemdQuote quotes a word for a unit file: backslashes and double quotes escaped, % doubled (systemd's specifiers).
func systemdQuote(s string) string {
	s = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%").Replace(s)
	return `"` + s + `"`
}

// ---------- macOS: a launchd agent ----------

func (s System) label() string  { return "io.github.feynmanzcys-bytes." + s.Name }
func (s System) domain() string { return fmt.Sprintf("gui/%d", s.UID) }
func (s System) plistPath() string {
	return filepath.Join(s.Home, "Library", "LaunchAgents", s.label()+".plist")
}
func (s System) logPath() string { return filepath.Join(s.Home, "Library", "Logs", s.Name+".log") }

func (s System) plist(argv []string, path string) string {
	str := func(v string) string {
		var b strings.Builder
		xml.EscapeText(&b, []byte(v))
		return "<string>" + b.String() + "</string>"
	}
	var args strings.Builder
	for _, a := range argv {
		args.WriteString("\n\t\t" + str(a))
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<!-- Starts the symphony bridge when you log in. Made by "symphony -autostart on"; "symphony -autostart off" removes it. -->
<plist version="1.0">
<dict>
	<key>Label</key>
	` + str(s.label()) + `
	<key>ProgramArguments</key>
	<array>` + args.String() + `
	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		` + str(path) + `
	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>StandardOutPath</key>
	` + str(s.logPath()) + `
	<key>StandardErrorPath</key>
	` + str(s.logPath()) + `
</dict>
</plist>
`
}

// ---------- Windows: a value under the Run key ----------

// winArg quotes an argument for a Windows command line so that the program splits it back the same way
// (backslashes count only before a double quote).
func winArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, c := range s {
		switch c {
		case '\\':
			slashes++
		case '"':
			b.WriteString(strings.Repeat(`\`, slashes+1))
			slashes = 0
		default:
			slashes = 0
		}
		b.WriteRune(c)
	}
	b.WriteString(strings.Repeat(`\`, slashes))
	b.WriteByte('"')
	return b.String()
}
