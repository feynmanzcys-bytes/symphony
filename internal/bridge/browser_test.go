package bridge

import (
	"errors"
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
