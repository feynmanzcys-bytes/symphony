package bridge

import (
	"net/url"
	"path/filepath"
	"strings"
)

// BrowserCommand is the command that opens url in the default browser on goos. ok is false when there is no way
// to: on Linux and the like without a graphical session (no DISPLAY or WAYLAND_DISPLAY) or without xdg-open.
// url may be a local file's address (FileURL): every one of these commands hands it to the program for its type.
func BrowserCommand(goos, url string, getenv func(string) string, look LookPath) (argv []string, ok bool) {
	switch goos {
	case "windows":
		// hands the address to the default browser without a console window or cmd's quoting rules
		return []string{"rundll32", "url.dll,FileProtocolHandler", url}, true
	case "darwin":
		return []string{"open", url}, true
	}
	if getenv("DISPLAY") == "" && getenv("WAYLAND_DISPLAY") == "" {
		return nil, false
	}
	if _, err := look("xdg-open"); err != nil {
		return nil, false
	}
	return []string{"xdg-open", url}, true
}

// HandlerCommand fills in a program's command line from Windows's registry (template, as AssocQueryString gives the
// http protocol's: "C:\...\msedge.exe" --single-argument %1) with addr: %1 and %L become addr, %* and the other
// numbered ones nothing, %NAME% the variable's value (getenv); without a %1 or %L, addr goes at the end. It answers
// the program's path and the whole command line. addr is one word (FileURL escapes spaces), so it needs no quotes.
func HandlerCommand(template, addr string, getenv func(string) string) (exe, line string, ok bool) {
	template = strings.TrimSpace(template)
	if template == "" {
		return "", "", false
	}
	var b strings.Builder
	placed := false
	for i := 0; i < len(template); i++ {
		c := template[i]
		if c != '%' || i+1 >= len(template) {
			b.WriteByte(c)
			continue
		}
		// %NAME%, a variable (two letters or more: %LOCALAPPDATA% is no %L)
		if end := strings.IndexByte(template[i+1:], '%'); end >= 2 && varName(template[i+1:i+1+end]) {
			b.WriteString(getenv(template[i+1 : i+1+end]))
			i += end + 1
			continue
		}
		switch next := template[i+1]; {
		case next == '1' || next == 'L' || next == 'l':
			b.WriteString(addr)
			placed = true
			i++
		case next == '*' || (next >= '0' && next <= '9'):
			i++
		default: // a lone %
			b.WriteByte(c)
		}
	}
	line = strings.TrimSpace(b.String())
	if !placed {
		line += " " + addr
	}
	if rest, quoted := strings.CutPrefix(line, `"`); quoted {
		exe, _, _ = strings.Cut(rest, `"`)
	} else {
		exe, _, _ = strings.Cut(line, " ")
	}
	if exe == "" {
		return "", "", false
	}
	return exe, line, true
}

// varName says whether s is a Windows variable's name as a command line from the registry spells one: letters,
// digits, _ and the parentheses of ProgramFiles(x86)
func varName(s string) bool {
	for _, r := range s {
		if !(r == '_' || r == '(' || r == ')' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return s != "" && !(s[0] >= '0' && s[0] <= '9')
}

// FileURL is the file:// address of a local file on goos, for BrowserCommand: forward slashes, a Windows drive after a
// third slash (file:///C:/...) and a network share's server as the host (file://server/share/...), with spaces and the
// like escaped, so the address is one word that no command line splits or quotes.
func FileURL(goos, path string) string {
	if goos == "windows" {
		path = strings.ReplaceAll(path, `\`, "/")
		if share, ok := strings.CutPrefix(path, "//"); ok {
			host, rest, _ := strings.Cut(share, "/")
			return (&url.URL{Scheme: "file", Host: host, Path: "/" + rest}).String()
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

// ConfinedDir is where a confined browser can read the file that opens the office, or "" when the program that opens
// .html files (handler, the desktop file xdg-mime names, like firefox_firefox.desktop) isn't confined. A snap or a
// flatpak can't read hidden folders of the home folder, so not the data folder (~/.config/symphony): a snap reads its
// own ~/snap/<snap>/common, a flatpak its own ~/.var/app/<app id>. exists says whether a file exists.
func ConfinedDir(handler, home string, exists func(string) bool) string {
	if handler == "" || home == "" || strings.ContainsAny(handler, `/\`) || !strings.HasSuffix(handler, ".desktop") {
		return ""
	}
	if exists(filepath.Join("/var/lib/snapd/desktop/applications", handler)) {
		if snap, _, ok := strings.Cut(handler, "_"); ok && snap != "" {
			return filepath.Join(home, "snap", snap, "common")
		}
	}
	for _, apps := range []string{filepath.Join(home, ".local/share/flatpak/exports/share/applications"), "/var/lib/flatpak/exports/share/applications"} {
		if exists(filepath.Join(apps, handler)) {
			return filepath.Join(home, ".var", "app", strings.TrimSuffix(handler, ".desktop"))
		}
	}
	return ""
}
