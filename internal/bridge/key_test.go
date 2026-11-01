package bridge

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
)

// TestLoadKey: the first start makes the key (64 hex digits, only the user may read it, in a folder only the user may
// open), later starts read the same one, a damaged one is replaced, and ReadKey never makes one.
func TestLoadKey(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if _, err := ReadKey(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ReadKey without a key file: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("ReadKey must not make the data folder")
	}
	key, err := LoadKey(dir)
	if err != nil || !hex64.MatchString(key) {
		t.Fatalf("LoadKey: %q %v", key, err)
	}
	path := filepath.Join(dir, "key")
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("the key file should be 0600: %v %v", fi.Mode(), err)
		}
		if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
			t.Fatalf("the data folder LoadKey makes should be 0700: %v %v", fi.Mode(), err)
		}
	}
	if again, err := LoadKey(dir); err != nil || again != key {
		t.Fatalf("a second start should read the same key: %q %v", again, err)
	}
	if read, err := ReadKey(dir); err != nil || read != key {
		t.Fatalf("ReadKey: %q %v", read, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("only the key should be left in the folder: %v", entries)
	}

	if runtime.GOOS != "windows" { // a key others may read is made private again
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		if again, err := LoadKey(dir); err != nil || again != key {
			t.Fatalf("LoadKey: %q %v", again, err)
		}
		if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
			t.Fatalf("LoadKey should make the key file 0600 again, it is %v", fi.Mode())
		}
	}

	if err := os.WriteFile(path, []byte("not a key\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadKey(dir); !errors.Is(err, errBadKey) {
		t.Fatalf("ReadKey of a damaged key: %v", err)
	}
	fresh, err := LoadKey(dir)
	if err != nil || !hex64.MatchString(fresh) || fresh == key {
		t.Fatalf("a damaged key should be replaced: %q %v", fresh, err)
	}
	if fi, _ := os.Stat(path); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("the new key file should be 0600, it is %v", fi.Mode())
	}
}

// TestLoadKeyAtOnce: starts at the same moment on a fresh data folder all end up with the same key.
func TestLoadKeyAtOnce(t *testing.T) {
	for range 20 {
		dir := filepath.Join(t.TempDir(), "data")
		keys := make(chan string, 8)
		for range cap(keys) {
			go func() {
				key, err := LoadKey(dir)
				if err != nil {
					t.Error(err)
				}
				keys <- key
			}()
		}
		first := <-keys
		for range cap(keys) - 1 {
			if k := <-keys; k != first {
				t.Fatalf("two keys for one folder: %q %q", first, k)
			}
		}
		if read, _ := ReadKey(dir); read != first {
			t.Fatalf("the file holds %q, the starts got %q", read, first)
		}
	}
}

// TestWriteOpenFile: open.html sends the browser on to the office with the key, and only the user may read it, even
// when an older one could be read by others.
func TestWriteOpenFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "open.html"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	office := "http://127.0.0.1:7788/?key=" + testKey + "&a=<b>"
	path, err := WriteOpenFile(dir, office)
	if err != nil || path != filepath.Join(dir, "open.html") {
		t.Fatalf("WriteOpenFile: %q %v", path, err)
	}
	data, _ := os.ReadFile(path)
	if want := `<meta http-equiv="refresh" content="0;url=http://127.0.0.1:7788/?key=` + testKey + `&amp;a=&lt;b&gt;">`; !strings.Contains(string(data), want) {
		t.Fatalf("open.html should send the browser on to the office:\n%s", data)
	}
	if strings.Contains(string(data), "<script") {
		t.Fatalf("open.html needs no script:\n%s", data)
	}
	if fi, _ := os.Stat(path); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("open.html should be 0600, it is %v", fi.Mode())
	}
}

func noRedirects() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// TestPageKeySignIn: ?key= with the right key sets the cookie and sends the browser to the same address without it,
// keeping the rest of the query; the cookie then lets the page in. A wrong key signs nobody in.
func TestPageKeySignIn(t *testing.T) {
	_, base := newBridge(t)
	u, _ := url.Parse(base)
	_, port, _ := net.SplitHostPort(u.Host)
	browser := noRedirects()
	get := func(path string, cookies ...*http.Cookie) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", base+path, nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		res, err := browser.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}

	res := get("/?theme=zen&key=" + testKey + "&lang=ja")
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/?lang=ja&theme=zen" {
		t.Fatalf("?key= should redirect to the address without it: %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "symphony_key_"+port {
			cookie = c
		}
	}
	if cookie == nil || cookie.Value != testKey || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.MaxAge != 400*24*3600 {
		t.Fatalf("the cookie: %+v (all: %v)", cookie, res.Header.Values("Set-Cookie"))
	}
	if res := get("/?lang=ja&theme=zen", cookie); res.StatusCode != 200 {
		t.Fatalf("the cookie should let the page in: %d", res.StatusCode)
	}
	if res := get("/api/state", cookie); res.StatusCode != 200 {
		t.Fatalf("the cookie should let the page's API calls in: %d", res.StatusCode)
	}
	// the cookie of a bridge on another port is not this one's
	if res := get("/api/state", &http.Cookie{Name: "symphony_key_1", Value: testKey}); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("another port's cookie: %d", res.StatusCode)
	}
	if res := get("/api/state", &http.Cookie{Name: "symphony_key_" + port, Value: strings.Repeat("0", 64)}); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a cookie with the wrong key: %d", res.StatusCode)
	}

	if res := get("/t/zen/?key=" + testKey); res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/t/zen/" {
		t.Fatalf("a theme's page: %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	// a wrong key is no key: no cookie, no redirect
	res = get("/?key=" + strings.Repeat("f", 64))
	if res.StatusCode != http.StatusForbidden || len(res.Cookies()) != 0 {
		t.Fatalf("a wrong key: %d %v", res.StatusCode, res.Cookies())
	}
	// only a GET (or HEAD) signs in
	req, _ := http.NewRequest("POST", base+"/api/accounts?key="+testKey, strings.NewReader(`{"name":"x"}`))
	if res, err := browser.Do(req); err != nil || res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a POST with ?key=: %v %v", res, err)
	}
	req, _ = http.NewRequest("HEAD", base+"/approve?key="+testKey, nil)
	if res, err := browser.Do(req); err != nil || res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/approve" {
		t.Fatalf("a HEAD with ?key=: %v %v", res, err)
	}
	// the redirect stays on this host, whatever the path looks like
	rec := httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/?key="+testKey, nil)
	req.URL.Path, req.RemoteAddr, req.Host = "//elsewhere.example/x", "127.0.0.1:5000", "127.0.0.1:7788"
	keyedServer(t).ServeHTTP(rec, req)
	if loc := rec.Header().Get("Location"); rec.Code != http.StatusSeeOther || loc != "/elsewhere.example/x" {
		t.Fatalf("a path starting with // should redirect on this host: %d %q", rec.Code, loc)
	}
	// the header does as well as the cookie
	if res, err := pageGet(base + "/api/state"); err != nil || res.StatusCode != 200 {
		t.Fatalf("X-Symphony-Key: %v %v", res, err)
	}
}

// keyedServer is a server with the tests' key, for requests made by hand.
func keyedServer(t *testing.T) *Server {
	t.Helper()
	srv := NewServer(NewHub(), testWeb)
	srv.Key = testKey
	return srv
}

// TestPageKeyRequired: without the key the API answers 401 page_key, a page a person opens answers 403 with a page
// that says how to get in (in the bridge's language, without a script), and the pages' other files answer 401.
func TestPageKeyRequired(t *testing.T) {
	_, base := newBridge(t)
	prev := i18n.Lang()
	t.Cleanup(func() { i18n.SetLang(prev) })
	i18n.SetLang("ja")
	do := func(method, path string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, base+path, nil)
		res, err := noRedirects().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return res, string(body)
	}
	for _, path := range []string{"/", "/?theme=zen", "/t/zen/", "/t/office/", "/t/nope/", "/approve"} {
		res, body := do("GET", path)
		if res.StatusCode != http.StatusForbidden || !strings.HasPrefix(res.Header.Get("content-type"), "text/html") {
			t.Fatalf("GET %s without the key: %d %s", path, res.StatusCode, res.Header.Get("content-type"))
		}
		if !strings.Contains(body, `<html lang="ja">`) || !strings.Contains(body, i18n.T("page_key.title")) ||
			!strings.Contains(body, i18n.T("page_key.run")) || strings.Contains(body, "<script") || strings.Contains(body, "<html>office</html>") {
			t.Fatalf("GET %s: the page should say how to get in, in the bridge's language, and nothing else:\n%s", path, body)
		}
	}
	if res, _ := do("HEAD", "/"); res.StatusCode != http.StatusForbidden {
		t.Fatalf("HEAD / without the key: %d", res.StatusCode)
	}
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/state"}, {"GET", "/api/stream"}, {"GET", "/api/themes"}, {"GET", "/api/chat/s1"}, {"GET", "/api/terminal/x/stream"},
		{"POST", "/api/accounts"}, {"POST", "/api/launch"}, {"POST", "/api/answer/x"}, {"DELETE", "/api/accounts/x"}, {"GET", "/api/nope"},
		{"GET", "/api/wait/a/b"}, {"POST", "/api/wait/x"}, {"GET", "/api/cancel/x"},
	} {
		res, body := do(c.method, c.path)
		if res.StatusCode != http.StatusUnauthorized || !strings.Contains(body, `"code":"page_key"`) {
			t.Fatalf("%s %s without the key: %d %s", c.method, c.path, res.StatusCode, body)
		}
	}
	for _, path := range []string{"/symphony.js", "/approve.js", "/ui/ui.css", "/t/zen/zen.css", "/locales/en.json", "/favicon.ico", "/t/zen/a/"} {
		if res, body := do("GET", path); res.StatusCode != http.StatusUnauthorized || strings.Contains(body, "export") || strings.Contains(body, "<html") {
			t.Fatalf("GET %s without the key: %d %q", path, res.StatusCode, body)
		}
	}
}

// TestModEndpointsWithoutPageKey: the mod shows its token, never the page key, and its endpoints take that; the
// token is no page key anywhere else.
func TestModEndpointsWithoutPageKey(t *testing.T) {
	srv, base := newBridge(t)
	mod, acc := as(t, srv, base, "a") // as sends no page key
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start"})
	id := mod.ok("POST", "/api/ask", AskRequest{Session: "s1", Kind: "permission", Tool: "Bash"})["id"].(string)
	mod.ok("POST", "/api/cancel/"+id, nil)
	if got := mod.ok("GET", "/api/wait/"+id, nil)["status"]; got != AskCancelled {
		t.Fatalf("wait: %v", got)
	}
	mod.ok("GET", "/api/inbox?session=s1", nil)
	sum := sha256.Sum256([]byte(acc.Token))
	mod.ok("POST", "/api/hello", map[string]string{"id": hex.EncodeToString(sum[:]), "nonce": "00112233445566778899aabbccddeeff"})
	for _, path := range []string{"/api/state", "/api/chat/s1"} {
		if st, code := mod.failure("GET", path, nil); st != http.StatusUnauthorized || code != "page_key" {
			t.Fatalf("the mod's token on GET %s: %d %q", path, st, code)
		}
	}
	if st, code := mod.failure("POST", "/api/say/s1", map[string]string{"text": "hi"}); st != http.StatusUnauthorized || code != "page_key" {
		t.Fatalf("the mod's token on a page write: %d %q", st, code)
	}
}

// TestHello: the bridge proves it knows the token whose hash the mod sends, without the token ever being sent; an
// unknown hash or a body that isn't one gets 401 token_invalid.
func TestHello(t *testing.T) {
	srv, base := newBridge(t)
	_, acc := as(t, srv, base, "a")
	as(t, srv, base, "b")
	mod := client{t: t, base: base, modOnly: true} // no token: the hello comes first
	sum := sha256.Sum256([]byte(acc.Token))
	id := hex.EncodeToString(sum[:])
	for _, nonce := range []string{"0123456789abcdef", strings.Repeat("aB", 64)} {
		res := mod.ok("POST", "/api/hello", map[string]string{"id": id, "nonce": nonce})
		mac := hmac.New(sha256.New, []byte(acc.Token))
		mac.Write([]byte("symphony-hello:" + nonce))
		if res["proof"] != hex.EncodeToString(mac.Sum(nil)) {
			t.Fatalf("nonce %s: proof %v", nonce, res["proof"])
		}
	}
	other := sha256.Sum256([]byte("not a token"))
	for _, body := range []any{
		map[string]string{"id": hex.EncodeToString(other[:]), "nonce": "0123456789abcdef"}, // no such account
		map[string]string{"id": strings.ToUpper(id), "nonce": "0123456789abcdef"},          // the hash in lowercase only
		map[string]string{"id": acc.Token, "nonce": "0123456789abcdef"},                    // the token itself is no hash
		map[string]string{"id": id, "nonce": "0123456789abcde"},                            // a nonce too short
		map[string]string{"id": id, "nonce": strings.Repeat("a", 129)},                     // too long
		map[string]string{"id": id, "nonce": "0123456789abcdeg"},                           // not hex
		map[string]string{"id": id},
		"not an object",
	} {
		if st, code := mod.failure("POST", "/api/hello", body); st != http.StatusUnauthorized || code != "token_invalid" {
			t.Fatalf("%v: want 401 token_invalid, got %d %q", body, st, code)
		}
	}
	req, _ := http.NewRequest("POST", base+"/api/hello", strings.NewReader("{"))
	if res, err := http.DefaultClient.Do(req); err != nil || res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a broken body: %v %v", res, err)
	}
	// an account's hash, compared with every account's
	if got := srv.Hub.accountByTokenHash(id); got == nil || got.ID != acc.ID {
		t.Fatalf("accountByTokenHash: %+v", got)
	}
	if got := srv.Hub.accountByTokenHash(""); got != nil {
		t.Fatalf("an empty hash: %+v", got)
	}
}

// TestProveKey: the bridge proves it has the page key without anyone showing it the key, with a label of its own (a
// token's proof is no page key's); a program that took the address, answering anything, never sees the key; nothing
// listening is an error.
func TestProveKey(t *testing.T) {
	srv, base := newBridge(t)
	if ok, err := ProveKey(base, srv.Key); !ok || err != nil {
		t.Fatalf("the bridge with the key: %v %v", ok, err)
	}
	if ok, err := ProveKey(base, strings.Repeat("0", 64)); ok || err != nil {
		t.Fatalf("another key: %v %v", ok, err)
	}
	// the page key's proof is keyed with it over its own label
	mod := client{t: t, base: base, modOnly: true}
	res := mod.ok("POST", "/api/hello", map[string]string{"id": keyID(srv.Key), "nonce": "0123456789abcdef"})
	mac := hmac.New(sha256.New, []byte(srv.Key))
	mac.Write([]byte("symphony-page:0123456789abcdef"))
	if res["proof"] != hex.EncodeToString(mac.Sum(nil)) {
		t.Fatalf("the page key's proof: %v", res["proof"])
	}
	// a squatter that answers every request like a bridge would
	var seen []string
	var mu sync.Mutex
	squatter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen = append(seen, r.URL.String(), string(body))
		for k, v := range r.Header {
			seen = append(seen, k+": "+strings.Join(v, ","))
		}
		mu.Unlock()
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, `{"type":"snapshot","proof":"`+strings.Repeat("ab", 32)+`"}`)
	}))
	defer squatter.Close()
	if ok, err := ProveKey(squatter.URL, srv.Key); ok || err != nil {
		t.Fatalf("a squatter: %v %v", ok, err)
	}
	mu.Lock()
	if all := strings.Join(seen, "\n"); strings.Contains(all, srv.Key) || len(seen) == 0 {
		t.Fatalf("the squatter saw the key, or nothing: %s", all)
	}
	mu.Unlock()
	// nothing there
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	free := "http://" + ln.Addr().String()
	ln.Close()
	if ok, err := ProveKey(free, srv.Key); ok || err == nil {
		t.Fatalf("nothing listening: %v %v", ok, err)
	}
}

// TestShutdown: POST /api/shutdown needs the page key like every page call, answers 202 and then stops the bridge (the
// program's Shutdown); without one set it is 501, and a GET is no way to stop it.
func TestShutdown(t *testing.T) {
	srv, base := newBridge(t)
	stopped := make(chan struct{}, 1)
	web := client{t: t, base: base}
	if st := web.status("POST", "/api/shutdown", nil); st != http.StatusNotImplemented {
		t.Fatalf("without Shutdown: %d", st)
	}
	srv.Shutdown = func() { stopped <- struct{}{} }
	mod := client{t: t, base: base, modOnly: true}
	if st, code := mod.failure("POST", "/api/shutdown", nil); st != http.StatusUnauthorized || code != "page_key" {
		t.Fatalf("without the key: %d %q", st, code)
	}
	if st := web.status("GET", "/api/shutdown", nil); st != http.StatusMethodNotAllowed {
		t.Fatalf("a GET: %d", st)
	}
	select {
	case <-stopped:
		t.Fatal("nothing but an authorized POST may stop the bridge")
	default:
	}
	if st := web.status("POST", "/api/shutdown", nil); st != http.StatusAccepted {
		t.Fatalf("with the key: %d", st)
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("the bridge wasn't stopped")
	}
}

// TestCrossSiteAPI: the API refuses what a browser says comes from another site or another port of this machine (a
// link, a redirect, a fetch: the cookie would ride along), even a GET, as /api/dirs reads the folder it names; the
// pages, addresses typed in, programs (no Sec-Fetch-Site), the sign-in and the mod's endpoints are as before.
func TestCrossSiteAPI(t *testing.T) {
	srv := NewServer(NewHub(), testWeb)
	srv.Key = testKey
	tmp := t.TempDir()
	for _, c := range []struct {
		site, method, path string
		cookie             bool
		want               int
		mode               string
	}{
		{"none", "GET", "/api/dirs?path=" + tmp, true, 403, "navigate"}, // a link opened from another app
		{"none", "GET", "/api/state", true, 200, "navigate"},            // an address typed in, to look
		{"cross-site", "GET", "/api/dirs?path=" + tmp, true, 403, ""},
		{"same-site", "GET", "/api/dirs?path=" + tmp, true, 403, ""},
		{"cross-site", "GET", "/api/launch/options", true, 403, ""},
		{"cross-site", "GET", "/api/state", true, 403, ""},
		{"cross-site", "HEAD", "/api/state", true, 403, ""},
		{"same-origin", "GET", "/api/dirs?path=" + tmp, true, 200, ""},
		{"none", "GET", "/api/dirs?path=" + tmp, true, 200, ""},
		{"", "GET", "/api/dirs?path=" + tmp, true, 200, ""},
		{"cross-site", "GET", "/", true, 200, ""},
		{"cross-site", "GET", "/?key=" + testKey, false, 303, ""},
		{"cross-site", "GET", "/api/inbox", false, 401, ""}, // a mod endpoint: its token check, as before
	} {
		req := httptest.NewRequest(c.method, "http://127.0.0.1:7788"+c.path, nil)
		req.RemoteAddr = "127.0.0.1:5555"
		if c.cookie {
			req.AddCookie(&http.Cookie{Name: "symphony_key_7788", Value: testKey})
		}
		if c.site != "" {
			req.Header.Set("Sec-Fetch-Site", c.site)
		}
		if c.mode != "" {
			req.Header.Set("Sec-Fetch-Mode", c.mode)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("Sec-Fetch-Site %q mode %q, %s %s: %d, want %d", c.site, c.mode, c.method, c.path, rec.Code, c.want)
		}
	}
}

// TestOnlyThisMachine: a request from another machine is refused before anything else.
func TestOnlyThisMachine(t *testing.T) {
	srv := keyedServer(t)
	for remote, want := range map[string]int{
		"127.0.0.1:5000": 200, "[::1]:5000": 200, "127.0.0.2:5000": 200, "[::ffff:127.0.0.1]:5000": 200,
		"192.168.1.5:5000": http.StatusForbidden, "[fe80::1%en0]:5000": http.StatusForbidden, "10.0.0.1:5000": http.StatusForbidden,
		"[2001:db8::1]:5000": http.StatusForbidden, "": http.StatusForbidden, "bogus": http.StatusForbidden,
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/state", nil)
		req.Host, req.RemoteAddr = "127.0.0.1:7788", remote
		req.Header.Set(keyHeader, testKey)
		srv.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("from %q: want %d, got %d", remote, want, rec.Code)
		}
	}
}

// TestSecurityHeaders: every answer carries the content security policy and the other headers, whatever it is.
func TestSecurityHeaders(t *testing.T) {
	_, base := newBridge(t)
	check := func(what string, res *http.Response) {
		t.Helper()
		res.Body.Close()
		for k, v := range map[string]string{
			"Content-Security-Policy": contentSecurityPolicy, "X-Frame-Options": "DENY",
			"X-Content-Type-Options": "nosniff", "Referrer-Policy": "no-referrer",
		} {
			if got := res.Header.Get(k); got != v {
				t.Errorf("%s: %s is %q, want %q", what, k, got, v)
			}
		}
	}
	for _, path := range []string{"/", "/api/state", "/symphony.js", "/nope", "/t/nope/x"} {
		res, err := pageGet(base + path)
		if err != nil {
			t.Fatal(err)
		}
		check("GET "+path, res)
	}
	for _, path := range []string{"/", "/api/state", "/ui/ui.css"} { // no key
		res, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		check("GET "+path+" without the key", res)
	}
	req, _ := http.NewRequest("GET", base+"/", nil)
	req.Host = "evil.example"
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	check("a refused Host", res)
	res, err = noRedirects().Get(base + "/?key=" + testKey)
	if err != nil {
		t.Fatal(err)
	}
	check("the sign-in redirect", res)
	res, err = pageGet(base + "/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	check("the event stream", res)
	if !strings.Contains(contentSecurityPolicy, "script-src 'self';") || !strings.Contains(contentSecurityPolicy, "frame-ancestors 'none'") {
		t.Fatalf("the policy: %s", contentSecurityPolicy)
	}
}

func TestModEndpoint(t *testing.T) {
	for _, c := range []struct {
		method, path string
		want         bool
	}{
		{"POST", "/api/event", true}, {"POST", "/api/ask", true}, {"POST", "/api/hello", true}, {"POST", "/api/cancel/a1", true},
		{"GET", "/api/wait/a1", true}, {"HEAD", "/api/wait/a1", true}, {"GET", "/api/inbox", true},
		{"GET", "/api/event", false}, {"POST", "/api/inbox", false}, {"GET", "/api/cancel/a1", false}, {"POST", "/api/wait/a1", false},
		{"GET", "/api/wait/", false}, {"GET", "/api/wait/a/b", false}, {"GET", "/api/wait/..", false}, {"POST", "/api/cancel/", false},
		{"GET", "/api/state", false}, {"POST", "/api/answer/a1", false}, {"POST", "/api/event/x", false}, {"GET", "/", false},
	} {
		r := &http.Request{Method: c.method, URL: &url.URL{Path: c.path}}
		if got := modEndpoint(r); got != c.want {
			t.Errorf("%s %s: %v, want %v", c.method, c.path, got, c.want)
		}
	}
	for p, want := range map[string]bool{
		"/": true, "/approve": true, "/t/zen/": true, "/t/x/": true,
		"/t/": false, "/t//": false, "/t/zen": false, "/t/zen/a": false, "/t/zen/a/": false, "/approve/": false, "/api/state": false, "/symphony.js": false,
	} {
		if got := pagePath(p); got != want {
			t.Errorf("pagePath(%q) = %v, want %v", p, got, want)
		}
	}
}
