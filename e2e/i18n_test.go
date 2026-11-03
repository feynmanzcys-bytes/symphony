//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
)

// all text in a page, hidden parts included, without the language picker (it names every language in its own script)
const pageText = `(() => { const c = document.body.cloneNode(true); c.querySelectorAll('select.lang, script, style').forEach(e => e.remove()); return c.textContent })()`

var (
	otherScripts = regexp.MustCompile(`[\x{3040}-\x{30ff}\x{3400}-\x{9fff}\x{ac00}-\x{d7af}\x{0400}-\x{04ff}]+`)
	latinLangs   = map[string]bool{"en": true, "es": true, "fr": true, "de": true, "pt-BR": true}
)

// TestI18n opens the office and approval pages in every language with made-up activity (no sessions, no
// tokens). Every key a page asks for must exist in its language, and pages in a Latin-script language must
// show no CJK, kana, Hangul or Cyrillic: that catches text the code shows without going through t().
func TestI18n(t *testing.T) {
	requireBrowser(t)
	repo, _ := filepath.Abs("..")
	// development mode, so the launch dialog opens without an installed mod
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))

	// made-up activity in ASCII: alice works and waits for a permission (one she could allow for the session), an
	// answer and an MCP server's form, bob idles with a message from alice, carol is off duty
	post := func(token, path string, body any) { t.Helper(); b.post(t, token, path, body) }
	alice, _ := b.Hub.CreateAccount("alice")
	bob, _ := b.Hub.CreateAccount("bob")
	if _, err := b.Hub.CreateAccount("carol"); err != nil {
		t.Fatal(err)
	}
	post(alice.Token, "/api/event", map[string]any{"session": "s-alice", "kind": "session.start", "cwd": "/work/alice", "model": "opus", "effort": "high"})
	post(alice.Token, "/api/event", map[string]any{"session": "s-alice", "kind": "session.title", "title": "alice"})
	post(alice.Token, "/api/event", map[string]any{"session": "s-alice", "kind": "tool.start", "tool": "Bash"})
	post(bob.Token, "/api/event", map[string]any{"session": "s-bob", "kind": "session.start", "cwd": "/work/bob", "model": "sonnet"})
	post(bob.Token, "/api/event", map[string]any{"session": "s-bob", "kind": "session.title", "title": "bob"})
	post(bob.Token, "/api/event", map[string]any{"session": "s-bob", "kind": "message.receive", "origin": map[string]any{"kind": "peer"},
		"text": "<cross-session-message from=\"uds:/tmp/x.sock\" from-name=\"alice\" from-mode=\"prompting\">\nhello bob\n</cross-session-message>"})
	post(alice.Token, "/api/event", map[string]any{"session": "s-alice", "kind": "message.send", "to": "bob", "text": "hello bob", "delivered": true})
	post(alice.Token, "/api/ask", map[string]any{"session": "s-alice", "kind": "permission", "tool": "Bash", "input": map[string]any{"command": "npm run build"},
		"remember": map[string]any{"tool": "Bash", "prefix": "npm run"}}) // with "Allow for this session"
	post(alice.Token, "/api/ask", map[string]any{"session": "s-alice", "kind": "question", "questions": []map[string]any{{
		"question": "Which layout?", "header": "Layout", "multiSelect": false,
		"options": []map[string]string{{"label": "One column", "description": "simple"}, {"label": "Two columns", "description": "dense"}},
	}}})
	post(alice.Token, "/api/ask", map[string]any{"session": "s-alice", "kind": "elicitation", "server": "github", "message": "Which repository?",
		"fields": []map[string]any{{"name": "repo", "title": "Repository", "type": "string", "required": true}}}) // an MCP server's form

	browser := newBrowser(t)
	langs := i18n.Langs
	if only := os.Getenv("SYMPHONY_E2E_LANGS"); only != "" { // e.g. en,zh-CN while a translation is in progress
		langs = strings.Split(only, ",")
	}
	for _, lang := range langs {
		checkOffice(t, b, browser, lang)
		checkApprove(t, b, browser, lang)
	}

	// how the page picks a language from what the browser asks for
	office := tab(t, browser, b.URL+"/")
	got := get(t, office, 10*time.Second, chromedp.Evaluate[string](`import('/i18n.js').then(m =>
		[['zh-HK'], ['zh'], ['zh-Hant'], ['pt'], ['it', 'de-AT'], ['xx'], [], ['ja-JP', 'en']].map(w => m.pickLang(w)).join(' '))`,
		chromedp.EvalAwaitPromise))
	if want := "zh-TW zh-CN zh-TW pt-BR de en en ja"; got != want {
		t.Fatalf("pickLang: got %q, want %q", got, want)
	}
	// an unsupported ?lang= falls through to the browser's language (English: newBrowser pins it)
	odd := tab(t, browser, b.URL+"/?lang=xx")
	if lang := get(t, odd, 10*time.Second, chromedp.Evaluate[string](`document.documentElement.lang`)); lang != "en" {
		t.Fatalf("?lang=xx should fall back to English, got %q", lang)
	}
	waitUntil(t, 10*time.Second, "the English page labels the message log button (in the settings)", func() bool { // once the catalogs are loaded
		return strings.TrimSpace(get(t, odd, 10*time.Second, chromedp.Evaluate[string](`document.getElementById('open-msgs').textContent`))) == "Message log"
	})
}

// checkOffice walks the office in one language: the pending asks in the waiting employee's window, the message
// log, an online and an offline employee's window, and the launch dialog. Its tab is closed afterwards.
func checkOffice(t *testing.T, b *testBridge, browser context.Context, lang string) {
	t.Helper()
	office, closeTab := openTab(t, browser, b.URL+"/?lang="+lang)
	defer closeTab()
	var texts []string
	read := func() { texts = append(texts, get(t, office, 5*time.Second, chromedp.Evaluate[string](pageText))) }
	waitUntil(t, 20*time.Second, lang+": the office shows three employees and the red pending chip", func() bool {
		return get(t, office, 5*time.Second, chromedp.Evaluate[bool](
			`document.querySelectorAll('.emp-label').length === 3 && !!document.querySelector('#stats .chip.hot[data-group=waiting]')`))
	})
	// the red chip opens the waiting employee's window, with her three asks' cards over the input
	do(t, office, 10*time.Second, chromedp.Click(chromedp.CSS(`#stats .chip.hot[data-group=waiting]`)))
	waitUntil(t, 10*time.Second, lang+": alice's window shows her three asks", func() bool {
		return get(t, office, 5*time.Second, chromedp.Evaluate[bool](
			`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === 'alice' && document.querySelectorAll('#chat .dock.asking #asks .card').length === 3 && !!document.querySelector('#chat #asks .allow-session')`))
	})
	if got := get(t, office, 5*time.Second, chromedp.Evaluate[string](`document.documentElement.lang`)); got != lang {
		t.Fatalf("%s: the page says lang=%q", lang, got)
	}
	read()
	screenshot(t, office, "i18n-"+lang+"-asks.png")
	// the window closes; the message log and the activity log open from the settings, which close
	do(t, office, 10*time.Second, chromedp.Click(chromedp.CSS(`#chat .close`)), chromedp.WaitNotVisible(chromedp.CSS(`#chat`)),
		chromedp.Click(chromedp.CSS(`#open-settings`)), chromedp.Click(chromedp.CSS(`#open-msgs`)), chromedp.WaitVisible(chromedp.CSS(`#msgs-panel`)),
		chromedp.WaitNotVisible(chromedp.CSS(`#settings`)),
		chromedp.Click(chromedp.CSS(`#open-settings`)), chromedp.Click(chromedp.CSS(`#open-log`)), chromedp.WaitVisible(chromedp.CSS(`#log`)),
		chromedp.WaitNotVisible(chromedp.CSS(`#settings`)))
	read()
	do(t, office, 10*time.Second, chromedp.Click(chromedp.CSS(`#msgs-panel .close`)), chromedp.Click(chromedp.CSS(`#log .close`)))
	for _, name := range []string{"bob", "carol"} { // an online employee's window (with the chat) and an offline one's
		clickLabel(t, office, b.account(name).ID)
		do(t, office, 10*time.Second, chromedp.WaitVisible(chromedp.CSS(`#chat`)))
		if name == "bob" {
			waitUntil(t, 10*time.Second, lang+": bob's chat shows alice's message in purple", func() bool {
				return get(t, office, 5*time.Second, chromedp.Evaluate[bool](`!!document.querySelector('#chat .flow .it.message')`))
			})
		}
		read()
		do(t, office, 10*time.Second, chromedp.Click(chromedp.CSS(`#chat .close`)))
	}
	// the search box: "bo" finds bob alone, and Enter opens his window
	do(t, office, 10*time.Second, chromedp.SendKeys(chromedp.CSS(`#find`), "bo"))
	waitUntil(t, 10*time.Second, lang+": the search lists bob alone", func() bool {
		return get(t, office, 5*time.Second, chromedp.Evaluate[bool](
			`(h => h.length === 1 && h[0].textContent.includes('bob'))(document.querySelectorAll('#find-list:not([hidden]) .hit'))`))
	})
	read()
	do(t, office, 10*time.Second, chromedp.SendKeys(chromedp.CSS(`#find`), kb.Enter), chromedp.WaitVisible(chromedp.CSS(`#chat`)))
	if got := get(t, office, 5*time.Second, chromedp.Evaluate[string](`document.querySelector('#chat h2').textContent`)); got != "bob" {
		t.Fatalf("%s: Enter in the search box should open bob's window, got %q", lang, got)
	}
	do(t, office, 10*time.Second, chromedp.Click(chromedp.CSS(`#chat .close`)),
		chromedp.Click(chromedp.CSS(`#new-emp`)), chromedp.WaitVisible(chromedp.CSS(`#launch form`)))
	read()
	screenshot(t, office, "i18n-"+lang+"-launch.png")
	checkTexts(t, lang, "office", texts)
	if miss := get(t, office, 5*time.Second, chromedp.Evaluate[[]string](`window.__i18nMissing`)); len(miss) > 0 {
		t.Errorf("%s: the office asked for keys its catalog lacks: %v", lang, miss)
	}
}

// checkApprove opens the approval page in one language; its tab is closed afterwards.
func checkApprove(t *testing.T, b *testBridge, browser context.Context, lang string) {
	t.Helper()
	approve, closeTab := openTab(t, browser, b.URL+"/approve?lang="+lang)
	defer closeTab()
	waitUntil(t, 20*time.Second, lang+": the approval page lists the three asks", func() bool {
		return get(t, approve, 5*time.Second, chromedp.Evaluate[bool](`document.querySelectorAll('#list .card').length === 3`))
	})
	checkTexts(t, lang, "approval page", []string{get(t, approve, 5*time.Second, chromedp.Evaluate[string](pageText))})
	if miss := get(t, approve, 5*time.Second, chromedp.Evaluate[[]string](`window.__i18nMissing`)); len(miss) > 0 {
		t.Errorf("%s: the approval page asked for keys its catalog lacks: %v", lang, miss)
	}
}

// checkTexts fails when a page in a Latin-script language shows text in another script.
func checkTexts(t *testing.T, lang, where string, texts []string) {
	t.Helper()
	if !latinLangs[lang] {
		return
	}
	for _, text := range texts {
		if bad := otherScripts.FindAllString(text, 5); len(bad) > 0 {
			t.Errorf("%s %s shows text in another script: %q", lang, where, bad)
		}
	}
}
