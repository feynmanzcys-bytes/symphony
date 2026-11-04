//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
)

// all text in a page, hidden parts included, without the language picker (it names every language in its own script)
const pageText = `(() => { const c = document.body.cloneNode(true); c.querySelectorAll('select.lang, script, style').forEach(e => e.remove()); return c.textContent })()`

var (
	otherScripts = regexp.MustCompile(`[\x{3040}-\x{30ff}\x{3400}-\x{9fff}\x{ac00}-\x{d7af}\x{0400}-\x{04ff}]+`)
	latinLangs   = map[string]bool{"en": true, "es": true, "fr": true, "de": true, "pt-BR": true}
)

// TestI18n opens Coworkers (with the employees page) and the approval page in every language with made-up activity
// (no sessions, no tokens). Every key a page asks for must exist in its language, and pages in a Latin-script language
// must show no CJK, kana, Hangul or Cyrillic: that catches text the code shows without going through t().
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
		checkCoworkers(t, b, browser, lang)
		checkApprove(t, b, browser, lang)
	}

	// how the page picks a language from what the browser asks for
	cw := tab(t, browser, b.URL+"/")
	got := get(t, cw, 10*time.Second, chromedp.Evaluate[string](`import('/i18n.js').then(m =>
		[['zh-HK'], ['zh'], ['zh-Hant'], ['pt'], ['it', 'de-AT'], ['xx'], [], ['ja-JP', 'en']].map(w => m.pickLang(w)).join(' '))`,
		chromedp.EvalAwaitPromise))
	if want := "zh-TW zh-CN zh-TW pt-BR de en en ja"; got != want {
		t.Fatalf("pickLang: got %q, want %q", got, want)
	}
	// an unsupported ?lang= falls through to the browser's language (English: newBrowser pins it)
	odd := tab(t, browser, b.URL+"/t/messenger/?lang=xx")
	if lang := get(t, odd, 10*time.Second, chromedp.Evaluate[string](`document.documentElement.lang`)); lang != "en" {
		t.Fatalf("?lang=xx should fall back to English, got %q", lang)
	}
	waitUntil(t, 10*time.Second, "the English page shows its title", func() bool { // once the catalogs are loaded
		return strings.TrimSpace(get(t, odd, 10*time.Second, chromedp.Evaluate[string](`document.querySelector('h1[data-i18n="msgr.title"]').textContent`))) == "Symphony Coworkers"
	})
}

// checkCoworkers walks Coworkers in one language: the waiting coworker's conversation with her cards in it, an online
// and an offline coworker's conversation, the launch dialog (folded and unfolded), the settings and the employees page
// (a row being edited, two selected, the import saying what it would do, entry by entry). Its tab is closed afterwards.
func checkCoworkers(t *testing.T, b *testBridge, browser context.Context, lang string) {
	t.Helper()
	cw, closeTab := openTab(t, browser, b.URL+"/t/messenger/?lang="+lang)
	defer closeTab()
	var texts []string
	read := func() { texts = append(texts, get(t, cw, 5*time.Second, chromedp.Evaluate[string](pageText))) }
	is := func(expr string) bool { return get(t, cw, 5*time.Second, chromedp.Evaluate[bool](expr)) }
	run := func(stmt string) {
		get(t, cw, 10*time.Second, chromedp.Evaluate[bool](fmt.Sprintf(`(() => { %s; return true })()`, stmt)))
	}
	waitUntil(t, 20*time.Second, lang+": Coworkers lists the three", func() bool { return is(`document.querySelectorAll('#people .row').length === 3`) })
	if got := get(t, cw, 5*time.Second, chromedp.Evaluate[string](`document.documentElement.lang`)); got != lang {
		t.Fatalf("%s: Coworkers says lang=%q", lang, got)
	}
	run(fmt.Sprintf(`document.querySelector('#people .row[data-account=%q]').click()`, b.account("alice").ID))
	waitUntil(t, 10*time.Second, lang+": alice's conversation shows her asks' cards", func() bool {
		return is(`!document.getElementById('chat').hidden && document.querySelectorAll('#chat .flow .it.ask.carded .card').length >= 2 && !!document.querySelector('#chat .allow-session')`)
	})
	read()
	screenshot(t, cw, "i18n-"+lang+"-asks.png")
	for _, name := range []string{"bob", "carol"} { // an online coworker's conversation (with the chat) and an offline one's
		run(fmt.Sprintf(`document.querySelector('#people .row[data-account=%q]').click()`, b.account(name).ID))
		waitUntil(t, 10*time.Second, lang+": "+name+"'s conversation opens", func() bool {
			return is(fmt.Sprintf(`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === %q`, name))
		})
		if name == "bob" {
			waitUntil(t, 10*time.Second, lang+": bob's chat shows alice's message in purple", func() bool {
				return is(`!!document.querySelector('#chat .flow .it.message')`)
			})
		}
		read()
	}
	// the launch dialog: folded, the line that says what the start will use; unfolded, the fields
	run(`document.getElementById('new-emp').click()`)
	waitUntil(t, 10*time.Second, lang+": the launch dialog opens", func() bool {
		return is(`!document.getElementById('launch').hidden && !!document.querySelector('#launch form')`)
	})
	read()
	run(`document.querySelector('#launch .adv').open = true`)
	waitUntil(t, 10*time.Second, lang+": the fields unfold", func() bool { return is(`document.querySelector('#launch .preview').getClientRects().length > 0`) })
	read()
	screenshot(t, cw, "i18n-"+lang+"-launch.png")
	run(`document.querySelector('#launch .cancel').click()`)
	// the settings and the employees page
	run(`document.querySelector('.open-settings').click()`)
	waitUntil(t, 10*time.Second, lang+": the settings open", func() bool { return is(`!document.getElementById('settings').hidden`) })
	read()
	run(`document.querySelector('#settings .people .all').click()`)
	waitUntil(t, 10*time.Second, lang+": the employees page lists the three", func() bool {
		return is(`!document.getElementById('staff').hidden && document.querySelectorAll('#staff .emp').length === 3 && document.querySelectorAll('#staff .batch select[name=model] option').length > 2`)
	})
	run(`document.querySelector('#staff .emp .edit').click(); [...document.querySelectorAll('#staff .emp .pick')].slice(1).forEach(b => b.click())`)
	waitUntil(t, 10*time.Second, lang+": a row unfolds, two selected", func() bool {
		return is(`!!document.getElementById('staff-editor') && !document.querySelector('#staff .batch .delete').disabled`)
	})
	read()
	run(`document.querySelector('#staff .tools .import').click(); (x => { x.value = '[{"name":"zed","context":true},{"name":"bob","model":"haiku"},{"name":"carol","model":"gpt","color":1},{"x":1}]'; x.dispatchEvent(new Event('input')) })(document.getElementById('staff-import-text'))`)
	waitUntil(t, 10*time.Second, lang+": the import says what each entry would do", func() bool {
		return is(`document.querySelectorAll('#staff .importer .preview li').length === 4`)
	})
	read()
	screenshot(t, cw, "i18n-"+lang+"-staff.png")
	checkTexts(t, lang, "Coworkers", texts)
	if miss := get(t, cw, 5*time.Second, chromedp.Evaluate[[]string](`window.__i18nMissing`)); len(miss) > 0 {
		t.Errorf("%s: Coworkers asked for keys its catalogs lack: %v", lang, miss)
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
