package bridge

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
)

// Themes: the office page comes in themes, one folder each under web/themes/<name>/ with a theme.json (the name, a
// title and a description by language, the version, the protocol version it needs) and an index.html; its own files
// are served under /t/<name>/, the shared ones (symphony.js, i18n.js, asks.js, ui/, locales/) at the root. GET /
// serves the theme the browser picked (?theme=<name>, else the symphony.theme cookie, else the default), and
// GET /t/<name>/ picks one and serves it; GET /api/themes lists them.

const (
	themeCookie  = "symphony.theme"
	defaultTheme = "office"
)

var themeNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Theme is a theme's manifest, theme.json.
type Theme struct {
	Name        string            `json:"name"`
	Title       map[string]string `json:"title"`                 // by language; en is the fallback
	Description map[string]string `json:"description,omitempty"` // by language
	Version     string            `json:"version,omitempty"`
	Since       int               `json:"since,omitempty"` // the protocol version it was written for
	Icon        string            `json:"icon,omitempty"`
}

// loadThemes reads every themes/<name>/theme.json: the default theme first, then the others by name. A folder whose
// manifest is missing or broken, or whose name doesn't match its folder, is left out.
func loadThemes(web fs.FS) []Theme {
	var out []Theme
	entries, _ := fs.ReadDir(web, "themes")
	for _, e := range entries {
		if !e.IsDir() || !themeNameRe.MatchString(e.Name()) {
			continue
		}
		data, err := fs.ReadFile(web, "themes/"+e.Name()+"/theme.json")
		if err != nil {
			continue
		}
		var th Theme
		if json.Unmarshal(data, &th) != nil || th.Name != e.Name() {
			continue
		}
		if _, err := fs.Stat(web, "themes/"+e.Name()+"/index.html"); err != nil {
			continue
		}
		out = append(out, th)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].Name == defaultTheme) != (out[j].Name == defaultTheme) {
			return out[i].Name == defaultTheme
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Themes lists the themes the bridge has.
func (s *Server) Themes() []Theme { return s.themes }

func (s *Server) hasTheme(name string) bool {
	for _, th := range s.themes {
		if th.Name == name {
			return true
		}
	}
	return false
}

func setThemeCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: themeCookie, Value: name, Path: "/", MaxAge: 365 * 24 * 3600, SameSite: http.SameSiteLaxMode})
}

// themeIndex serves GET /: the theme named in ?theme= (remembered in the cookie), else the cookie's, else the
// default; a name the bridge doesn't have falls back to the default.
func (s *Server) themeIndex(w http.ResponseWriter, r *http.Request) {
	name := defaultTheme
	if q := r.URL.Query().Get("theme"); q != "" && s.hasTheme(q) {
		name = q
		setThemeCookie(w, name)
	} else if c, err := r.Cookie(themeCookie); err == nil && s.hasTheme(c.Value) {
		name = c.Value
	}
	s.serveFile(w, r, "themes/"+name+"/index.html")
}

// themeRoot serves GET /t/<name>/: that theme, remembered from now on. A theme this bridge doesn't have (a link kept
// from a version that had it: HQ and Mission Control went after v1.2.2) leads to / and the theme it serves.
func (s *Server) themeRoot(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("theme")
	if !s.hasTheme(name) {
		to := "/"
		if r.URL.RawQuery != "" {
			to += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, to, http.StatusSeeOther)
		return
	}
	setThemeCookie(w, name)
	s.serveFile(w, r, "themes/"+name+"/index.html")
}

// themeFile serves a theme's own file, GET /t/<name>/<file>; the path may not leave the theme's folder.
func (s *Server) themeFile(w http.ResponseWriter, r *http.Request) {
	name, file := r.PathValue("theme"), r.PathValue("file")
	if !s.hasTheme(name) || !fs.ValidPath(file) || strings.HasSuffix(file, "/") {
		http.NotFound(w, r)
		return
	}
	s.serveFile(w, r, path.Join("themes", name, file))
}

// themesAPI answers GET /api/themes: the manifests, and which one the browser has picked.
func (s *Server) themesAPI(w http.ResponseWriter, r *http.Request) {
	current := defaultTheme
	if c, err := r.Cookie(themeCookie); err == nil && s.hasTheme(c.Value) {
		current = c.Value
	}
	writeJSON(w, 200, map[string]any{"themes": s.themes, "current": current, "protocol": Protocol})
}
