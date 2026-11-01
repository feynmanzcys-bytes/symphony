package bridge

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
)

// The page key: every page request shows it to the bridge, as a cookie (symphony_key_<port>) or in the X-Symphony-Key
// header, so only someone who can read the data folder, or a browser signed in with it, gets in; a program that only
// reaches the address does not. A browser sends the cookie to every port of 127.0.0.1 (cookies don't go by the port),
// which SECURITY.md tells. The mod's endpoints keep their own credential, the account's token.

const (
	keyFile   = "key"       // in the data folder: the page key, 64 hex digits
	openFile  = "open.html" // in the data folder: what the browser opens to get to the office signed in
	keyHeader = "X-Symphony-Key"
	keyParam  = "key"
	keyMaxAge = 400 * 24 * 3600 // seconds: as long as browsers keep a cookie
)

// contentSecurityPolicy lets the pages load only the bridge's own files: no inline scripts, nothing from elsewhere, and
// no other site may frame them.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; " +
	"connect-src 'self'; font-src 'self'; media-src 'self' data: blob:; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

var (
	errPageKey = newError("page_key", "this page isn't signed in to the bridge: open the office from the symphony program, "+
		"or with the address and key the bridge printed in its log")
	errBadKey = errors.New("not a page key")
	hex64     = regexp.MustCompile(`^[0-9a-f]{64}$`) // a page key, or the SHA-256 of a token
	helloOnce = regexp.MustCompile(`^[0-9a-fA-F]{16,128}$`)
)

// DefaultDataDir is SYMPHONY_DATA, else symphony in the user config folder. The environment variable reaches a bridge
// started by the mod too, which gets no -data.
func DefaultDataDir() string {
	if dir := os.Getenv("SYMPHONY_DATA"); dir != "" {
		return dir
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "symphony")
	}
	return ".symphony"
}

func newKey() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ReadKey reads the page key from the data folder, without making one: for the programs that talk to a running bridge
// (symphony attach, the demo).
func ReadKey(dataDir string) (string, error) {
	path := filepath.Join(dataDir, keyFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(string(data))
	if !hex64.MatchString(key) {
		return "", fmt.Errorf("%s: %w", path, errBadKey)
	}
	return key, nil
}

// LoadKey reads the page key from the data folder, or makes one (32 random bytes, in hex) when there is none or it is
// damaged. The key stays from one start to the next, so a page's cookie stays good; only this user may read it.
func LoadKey(dataDir string) (string, error) {
	path := filepath.Join(dataDir, keyFile)
	key, err := ReadKey(dataDir)
	switch {
	case err == nil:
		// others may read it: take that back, and when this user can't (someone else's file), don't use it
		if fi, err := os.Stat(path); err == nil && runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0 {
			if err := os.Chmod(path, 0o600); err != nil {
				return "", err
			}
		}
		return key, nil
	case errors.Is(err, errBadKey):
		key = newKey()
		return key, writePrivate(path, []byte(key+"\n"))
	case !errors.Is(err, fs.ErrNotExist):
		return "", err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", err
	}
	// written aside, then linked into place: a symphony starting at the same moment (two sessions' mods running
	// symphony -background, say) sees the whole key or none, and when the other one got there first, its key wins
	key = newKey()
	tmp, err := writeTemp(path, []byte(key+"\n"))
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp)
	switch err := os.Link(tmp, path); {
	case err == nil:
		return key, nil
	case errors.Is(err, fs.ErrExist):
		return ReadKey(dataDir)
	default: // a file system without hard links
		return key, os.Rename(tmp, path)
	}
}

// writeTemp writes data to a new file next to path that only this user may read, synced to the disk (renamed into
// place, it is never found empty after a crash), and returns its name.
func writeTemp(path string, data []byte) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return "", err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// writePrivate replaces path with data, readable by this user only whatever the old file's mode was.
func writePrivate(path string, data []byte) error {
	tmp, err := writeTemp(path, data)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// WriteOpenFile writes open.html into the data folder: a page that sends the browser on to office (the office's address
// with ?key=), and returns its path. Opening this file instead of the address keeps the key off the command line,
// where other users of this machine could read it (ps).
func WriteOpenFile(dataDir, office string) (string, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", err
	}
	u := html.EscapeString(office)
	page := `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="refresh" content="0;url=` + u +
		`"><title>symphony</title></head><body><a href="` + u + `">symphony</a></body></html>` + "\n"
	path := filepath.Join(dataDir, openFile)
	return path, writePrivate(path, []byte(page))
}

// ---------- checking requests ----------

func securityHeaders(h http.Header) {
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	h.Set("X-Frame-Options", "DENY")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
}

// loopback says whether a request's remote address is this machine.
func loopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.Unmap().IsLoopback()
}

// modEndpoint says whether the request goes to one of the mod's endpoints, which take the account's token instead of
// the page key.
func modEndpoint(r *http.Request) bool {
	p := r.URL.Path
	oneID := func(prefix string) bool {
		id, ok := strings.CutPrefix(p, prefix)
		return ok && id != "" && !strings.Contains(id, "/") && id != "." && id != ".."
	}
	switch r.Method {
	case http.MethodPost:
		return p == "/api/event" || p == "/api/ask" || p == "/api/hello" || oneID("/api/cancel/")
	case http.MethodGet, http.MethodHead:
		return p == "/api/inbox" || oneID("/api/wait/")
	}
	return false
}

// keyCookie is the cookie's name for a request: by the port, so two bridges on this machine keep theirs apart (a cookie
// is the host's, whatever the port).
func keyCookie(r *http.Request) string {
	_, port, err := net.SplitHostPort(r.Host)
	if err != nil || port == "" {
		port = "80"
	}
	return "symphony_key_" + port
}

func (s *Server) isKey(k string) bool {
	return s.Key != "" && subtle.ConstantTimeCompare([]byte(k), []byte(s.Key)) == 1
}

// keyOK says whether the request shows the page key, in the cookie or the header.
func (s *Server) keyOK(r *http.Request) bool {
	if c, err := r.Cookie(keyCookie(r)); err == nil && s.isKey(c.Value) {
		return true
	}
	return s.isKey(r.Header.Get(keyHeader))
}

// signIn takes ?key= with the right key on a GET: it sets the cookie and sends the browser to the same address without
// the key, so it doesn't stay in the address bar. It says whether it answered.
func (s *Server) signIn(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	q := r.URL.Query()
	if !q.Has(keyParam) || !s.isKey(q.Get(keyParam)) {
		return false
	}
	http.SetCookie(w, &http.Cookie{Name: keyCookie(r), Value: s.Key, Path: "/", MaxAge: keyMaxAge, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	q.Del(keyParam)
	// one leading slash, so the address can't turn into another host's (//elsewhere)
	to := "/" + strings.TrimLeft(r.URL.EscapedPath(), `/\`)
	if enc := q.Encode(); enc != "" {
		to += "?" + enc
	}
	w.Header().Set("cache-control", "no-store")
	http.Redirect(w, r, to, http.StatusSeeOther)
	return true
}

// pagePath says whether a path is one of the pages a person opens: the office (/ and /t/<theme>/) and /approve.
func pagePath(p string) bool {
	if p == "/" || p == "/approve" {
		return true
	}
	theme, ok := strings.CutPrefix(p, "/t/")
	return ok && strings.HasSuffix(theme, "/") && strings.Count(theme, "/") == 1 && len(theme) > 1
}

// refuse answers a request without the page key: the API's callers get the error's code, a person opening a page gets a
// page saying how to get in, and the pages' files just 401.
func (s *Server) refuse(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/api/"):
		writeJSON(w, http.StatusUnauthorized, errorBody(errPageKey))
	case (r.Method == http.MethodGet || r.Method == http.MethodHead) && pagePath(r.URL.Path):
		w.Header().Set("content-type", "text/html; charset=utf-8")
		w.Header().Set("cache-control", "no-store")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, keyPage())
	default:
		http.Error(w, "this bridge needs its page key", http.StatusUnauthorized)
	}
}

// keyPage is the page a browser without the key gets instead of the office, in the bridge's language.
func keyPage() string {
	e := html.EscapeString
	title := e(i18n.T("page_key.title"))
	return `<!doctype html>
<html lang="` + e(i18n.Lang()) + `">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + title + `</title>
<style>
  body { margin:0; background:#f7f6f3; color:#2b2a28; font:15px/1.6 system-ui, sans-serif; }
  main { max-width:560px; margin:12vh auto; padding:0 20px; }
  h1 { font-size:18px; font-weight:600; }
</style>
</head>
<body>
<main>
<h1>` + title + `</h1>
<p>` + e(i18n.T("page_key.run")) + `</p>
<p>` + e(i18n.T("page_key.log")) + `</p>
</main>
</body>
</html>
`
}

// ---------- the mod's check that it talks to the real bridge ----------

// accountByTokenHash finds the account whose token's SHA-256 (in lowercase hex) is id. It looks at every account and
// compares in constant time, so how long it takes tells nothing about the tokens.
func (h *Hub) accountByTokenHash(id string) *Account {
	h.mu.Lock()
	defer h.mu.Unlock()
	var found *Account
	for _, a := range h.accounts {
		sum := sha256.Sum256([]byte(a.Token))
		if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(id)) == 1 {
			cp := *a
			found = &cp
		}
	}
	return found
}

// hello answers POST /api/hello {id, nonce}, which the mod sends before its token: id is the SHA-256 of the token, and
// the answer {proof} is the HMAC-SHA256 of "symphony-hello:" + nonce keyed with the token, which only a bridge that
// knows the token can give. The symphony program asks the same with the page key (id its SHA-256, the proof keyed with
// it over "symphony-page:" + nonce) before it shows the key. No such account or key, or a body that isn't one, is a 401.
func (s *Server) hello(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID    string `json:"id"`
		Nonce string `json:"nonce"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil ||
		!hex64.MatchString(req.ID) || !helloOnce.MatchString(req.Nonce) {
		writeJSON(w, http.StatusUnauthorized, errorBody(errTokenInvalid))
		return
	}
	if acc := s.Hub.accountByTokenHash(req.ID); acc != nil {
		writeJSON(w, http.StatusOK, map[string]string{"proof": helloProof(acc.Token, helloToken, req.Nonce)})
		return
	}
	// the page key's turn: the symphony program asks before it shows the key (ProveKey)
	if s.Key != "" && subtle.ConstantTimeCompare([]byte(keyID(s.Key)), []byte(req.ID)) == 1 {
		writeJSON(w, http.StatusOK, map[string]string{"proof": helloProof(s.Key, helloPageKey, req.Nonce)})
		return
	}
	writeJSON(w, http.StatusUnauthorized, errorBody(errTokenInvalid))
}

// What a hello's proof covers besides the nonce: one label for a token, another for the page key, so neither proof
// stands for the other.
const (
	helloToken   = "symphony-hello:"
	helloPageKey = "symphony-page:"
)

// keyID is what /api/hello takes as id for a secret (a token or the page key): its SHA-256, in lowercase hex.
func keyID(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// helloProof is the HMAC-SHA256 of label + nonce keyed with secret, in hex.
func helloProof(secret, label, nonce string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(label + nonce))
	return hex.EncodeToString(mac.Sum(nil))
}

// ProveKey asks what answers at base (http://host:port) to prove that it is the bridge with this page key, without
// showing it the key: a program that holds the key (symphony itself, attach, the demo) calls it before any request that
// carries the key, so a program that took the bridge's address never gets it. err is set when nothing answers (or not
// in time); ok says whether the answer was that bridge's. An older bridge, without page keys, isn't.
func ProveKey(base, key string) (ok bool, err error) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	nonce := hex.EncodeToString(b)
	body, _ := json.Marshal(map[string]string{"id": keyID(key), "nonce": nonce})
	req, err := http.NewRequest(http.MethodPost, base+"/api/hello", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("content-type", "application/json")
	c := http.Client{Timeout: 2 * time.Second}
	res, err := c.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	var ans struct {
		Proof string `json:"proof"`
	}
	if res.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&ans) != nil {
		return false, nil
	}
	return hmac.Equal([]byte(ans.Proof), []byte(helloProof(key, helloPageKey, nonce))), nil
}
