package autostart

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fake records the commands a System runs; fail makes the one starting with these words fail.
type fake struct {
	ran, started [][]string
	fail         []string
}

func (f *fake) system(goos string, t *testing.T) System {
	dir := t.TempDir()
	return System{
		GOOS: goos, Name: "symphony", Home: filepath.Join(dir, "home"), Config: filepath.Join(dir, "config"), UID: 501,
		Run: func(argv ...string) error {
			f.ran = append(f.ran, argv)
			if f.fail != nil && len(argv) >= len(f.fail) && slices.Equal(argv[:len(f.fail)], f.fail) {
				return errors.New("failed")
			}
			return nil
		},
		Start: func(argv ...string) error { f.started = append(f.started, argv); return nil },
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLinux(t *testing.T) {
	f := &fake{}
	s := f.system("linux", t)
	where, err := s.Enable(Entry{
		Exe:  "/opt/sym phony/symphony",
		Args: []string{"-addr=127.0.0.1:7788", `-data=/home/u/a "b" $c`},
		Path: "/usr/bin:/home/u/100%/bin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(s.Config, "systemd", "user", "symphony.service"); where != want {
		t.Errorf("unit at %s, want %s", where, want)
	}
	unit := read(t, where)
	for _, line := range []string{
		`ExecStart="/opt/sym phony/symphony" "-addr=127.0.0.1:7788" "-data=/home/u/a \"b\" $$c" "-open=false"`,
		`Environment="PATH=/usr/bin:/home/u/100%%/bin"`,
		"KillMode=process",
		"Restart=on-failure",
		"WantedBy=default.target",
	} {
		if !strings.Contains(unit, "\n"+line+"\n") {
			t.Errorf("the unit lacks %q:\n%s", line, unit)
		}
	}
	want := [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "symphony.service"},
		{"systemctl", "--user", "restart", "symphony.service"},
	}
	if !slices.EqualFunc(f.ran, want, slices.Equal) {
		t.Errorf("Enable ran %q, want %q", f.ran, want)
	}

	f.ran = nil
	if where, err := s.Disable(); err != nil || where == "" {
		t.Fatalf("Disable: %q, %v", where, err)
	}
	want = [][]string{{"systemctl", "--user", "disable", "--now", "symphony.service"}, {"systemctl", "--user", "daemon-reload"}}
	if !slices.EqualFunc(f.ran, want, slices.Equal) {
		t.Errorf("Disable ran %q, want %q", f.ran, want)
	}
	if _, err := os.Stat(where); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the unit is still there: %v", err)
	}

	f.ran = nil
	if where, err := s.Disable(); err != nil || where != "" || f.ran != nil {
		t.Errorf("Disable without an entry should do nothing: %q, %v, ran %q", where, err, f.ran)
	}
}

func TestLinuxFailure(t *testing.T) {
	f := &fake{fail: []string{"systemctl", "--user", "enable"}}
	s := f.system("linux", t)
	if _, err := s.Enable(Entry{Exe: "/usr/local/bin/symphony", Path: "/usr/bin"}); err == nil {
		t.Fatal("Enable should fail when systemctl does")
	}
	if _, err := os.Stat(s.unitPath()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a failed Enable should leave no unit behind: %v", err)
	}
}

func TestMacOS(t *testing.T) {
	f := &fake{}
	s := f.system("darwin", t)
	where, err := s.Enable(Entry{Exe: "/Applications/sym & phony/symphony", Args: []string{"-addr=127.0.0.1:7788"}, Path: "/opt/homebrew/bin:/usr/bin"})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(s.Home, "Library", "LaunchAgents", "io.github.feynmanzcys-bytes.symphony.plist"); where != want {
		t.Errorf("plist at %s, want %s", where, want)
	}
	plist := read(t, where)
	for _, part := range []string{
		"<string>io.github.feynmanzcys-bytes.symphony</string>",
		"<string>/Applications/sym &amp; phony/symphony</string>\n\t\t<string>-addr=127.0.0.1:7788</string>\n\t\t<string>-open=false</string>",
		"<key>PATH</key>\n\t\t<string>/opt/homebrew/bin:/usr/bin</string>",
		"<key>RunAtLoad</key>\n\t<true/>",
		"<string>" + filepath.Join(s.Home, "Library", "Logs", "symphony.log") + "</string>",
	} {
		if !strings.Contains(plist, part) {
			t.Errorf("the plist lacks %q:\n%s", part, plist)
		}
	}
	for d := xml.NewDecoder(strings.NewReader(plist)); ; {
		if _, err := d.Token(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("the plist isn't well-formed XML: %v", err)
		}
	}
	want := [][]string{
		{"launchctl", "bootout", "gui/501/io.github.feynmanzcys-bytes.symphony"},
		{"launchctl", "bootstrap", "gui/501", where},
	}
	if !slices.EqualFunc(f.ran, want, slices.Equal) {
		t.Errorf("Enable ran %q, want %q", f.ran, want)
	}

	f.ran = nil
	if got, err := s.Disable(); err != nil || got != where {
		t.Fatalf("Disable: %q, %v", got, err)
	}
	if want := [][]string{{"launchctl", "bootout", "gui/501/io.github.feynmanzcys-bytes.symphony"}}; !slices.EqualFunc(f.ran, want, slices.Equal) {
		t.Errorf("Disable ran %q, want %q", f.ran, want)
	}
	if _, err := os.Stat(where); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the plist is still there: %v", err)
	}
}

func TestMacOSFailure(t *testing.T) {
	f := &fake{fail: []string{"launchctl", "bootstrap"}}
	s := f.system("darwin", t)
	if _, err := s.Enable(Entry{Exe: "/usr/local/bin/symphony", Path: "/usr/bin"}); err == nil {
		t.Fatal("Enable should fail when launchctl does")
	}
	if _, err := os.Stat(s.plistPath()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a failed Enable should leave no plist behind: %v", err)
	}
}

func TestWindows(t *testing.T) {
	f := &fake{}
	s := f.system("windows", t)
	exe := `C:\Program Files\symphony\symphony.exe`
	where, err := s.Enable(Entry{Exe: exe, Args: []string{`-data=C:\Users\u\AppData\Roaming\symphony`}})
	if err != nil {
		t.Fatal(err)
	}
	if where != RunKey+`\symphony` {
		t.Errorf("entry at %s", where)
	}
	data := `"C:\Program Files\symphony\symphony.exe" -data=C:\Users\u\AppData\Roaming\symphony -open=false`
	want := [][]string{{"reg", "add", RunKey, "/v", "symphony", "/t", "REG_SZ", "/d", data, "/f"}}
	if !slices.EqualFunc(f.ran, want, slices.Equal) {
		t.Errorf("Enable ran %q, want %q", f.ran, want)
	}
	if started := [][]string{{exe, `-data=C:\Users\u\AppData\Roaming\symphony`, "-open=false"}}; !slices.EqualFunc(f.started, started, slices.Equal) {
		t.Errorf("Enable started %q, want %q", f.started, started)
	}

	f.ran = nil
	if got, err := s.Disable(); err != nil || got != where {
		t.Fatalf("Disable: %q, %v", got, err)
	}
	want = [][]string{{"reg", "query", RunKey, "/v", "symphony"}, {"reg", "delete", RunKey, "/v", "symphony", "/f"}}
	if !slices.EqualFunc(f.ran, want, slices.Equal) {
		t.Errorf("Disable ran %q, want %q", f.ran, want)
	}

	f.ran, f.fail = nil, []string{"reg", "query"}
	if got, err := s.Disable(); err != nil || got != "" || len(f.ran) != 1 {
		t.Errorf("Disable without an entry should only look: %q, %v, ran %q", got, err, f.ran)
	}
}

func TestWinArg(t *testing.T) {
	for in, want := range map[string]string{
		`symphony.exe`:                  `symphony.exe`,
		``:                              `""`,
		`C:\Program Files\symphony.exe`: `"C:\Program Files\symphony.exe"`,
		`C:\my dir\`:                    `"C:\my dir\\"`,
		`say "hi"`:                      `"say \"hi\""`,
		`a\"b`:                          `"a\\\"b"`,
	} {
		if got := winArg(in); got != want {
			t.Errorf("winArg(%q) = %s, want %s", in, got, want)
		}
	}
}
