package bridge

import (
	"errors"
	"runtime"
	"slices"
	"testing"
)

func TestBrowserCommand(t *testing.T) {
	const url = "http://127.0.0.1:7788/"
	env := func(vars map[string]string) func(string) string { return func(k string) string { return vars[k] } }
	look := func(found ...string) LookPath {
		return func(file string) (string, error) {
			if slices.Contains(found, file) {
				return "/usr/bin/" + file, nil
			}
			return "", errors.New("not found")
		}
	}
	for _, c := range []struct {
		name string
		goos string
		env  map[string]string
		look LookPath
		want []string
	}{
		{"windows", "windows", nil, look(), []string{"rundll32", "url.dll,FileProtocolHandler", url}},
		{"macOS", "darwin", nil, look(), []string{"open", url}},
		{"Linux with X11", "linux", map[string]string{"DISPLAY": ":0"}, look("xdg-open"), []string{"xdg-open", url}},
		{"Linux with Wayland", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, look("xdg-open"), []string{"xdg-open", url}},
		{"Linux without a graphical session", "linux", nil, look("xdg-open"), nil},
		{"Linux without xdg-open", "linux", map[string]string{"DISPLAY": ":0"}, look(), nil},
		{"FreeBSD", "freebsd", map[string]string{"DISPLAY": ":0"}, look("xdg-open"), []string{"xdg-open", url}},
	} {
		argv, ok := BrowserCommand(c.goos, url, env(c.env), c.look)
		if ok != (c.want != nil) || !slices.Equal(argv, c.want) {
			t.Errorf("%s: got %q (ok %v), want %q", c.name, argv, ok, c.want)
		}
	}
}

// TestFileURL: the office opens through a file (open.html), whose address every opener takes as one word.
func TestFileURL(t *testing.T) {
	for _, c := range []struct{ goos, path, want string }{
		{"linux", "/home/ann/.config/symphony/open.html", "file:///home/ann/.config/symphony/open.html"},
		{"darwin", "/Users/ann/Library/Application Support/symphony/open.html", "file:///Users/ann/Library/Application%20Support/symphony/open.html"},
		{"windows", `C:\Users\Ann Lee\AppData\Roaming\symphony\open.html`, "file:///C:/Users/Ann%20Lee/AppData/Roaming/symphony/open.html"},
		{"windows", `\\server\home\ann\AppData\Roaming\symphony\open.html`, "file://server/home/ann/AppData/Roaming/symphony/open.html"},
		{"linux", "/tmp/a#b?c%d/open.html", "file:///tmp/a%23b%3Fc%25d/open.html"},
	} {
		if got := FileURL(c.goos, c.path); got != c.want {
			t.Errorf("FileURL(%s, %q) = %q, want %q", c.goos, c.path, got, c.want)
		}
	}
	// what each system's opener gets: the file's address, never the key
	for _, goos := range []string{"windows", "darwin", "linux"} {
		u := FileURL(goos, "/data/open.html")
		argv, ok := BrowserCommand(goos, u, func(string) string { return ":0" }, func(string) (string, error) { return "/usr/bin/xdg-open", nil })
		if !ok || argv[len(argv)-1] != u {
			t.Errorf("%s: %q", goos, argv)
		}
	}
}

// A snap or flatpak browser can't read the data folder (a hidden folder of the home folder): the file that opens the
// office goes to that app's own folder instead; any other program reads it from the data folder.
func TestConfinedDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("snaps and flatpaks are Linux's: ConfinedDir is asked only there")
	}
	files := map[string]bool{
		"/var/lib/snapd/desktop/applications/firefox_firefox.desktop":                         true,
		"/home/u/.local/share/flatpak/exports/share/applications/org.mozilla.firefox.desktop": true,
		"/var/lib/flatpak/exports/share/applications/com.google.Chrome.desktop":               true,
	}
	exists := func(p string) bool { return files[p] }
	for handler, want := range map[string]string{
		"firefox_firefox.desktop":     "/home/u/snap/firefox/common",
		"org.mozilla.firefox.desktop": "/home/u/.var/app/org.mozilla.firefox",
		"com.google.Chrome.desktop":   "/home/u/.var/app/com.google.Chrome",
		"firefox.desktop":             "", // a packaged browser reads the data folder
		"":                            "",
		"../firefox_firefox.desktop":  "",
		"firefox_firefox":             "",
	} {
		if got := ConfinedDir(handler, "/home/u", exists); got != want {
			t.Errorf("%q: %q, want %q", handler, got, want)
		}
	}
	if got := ConfinedDir("firefox_firefox.desktop", "", exists); got != "" {
		t.Errorf("no home: %q", got)
	}
}
