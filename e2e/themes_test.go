//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// TestThemesLoad opens every theme the bridge lists (/api/themes) at /t/<name>/ and checks the theme contract's
// basics: the page loads without an error, <html data-theme> names the theme, the shared pieces (the employee
// window, the launch and task dialogs, the task window) are on the page, no catalog key is missing, and the bridge
// remembers the pick. No tokens.
func TestThemesLoad(t *testing.T) {
	requireBrowser(t)
	b := startBridge(t, "")
	browser := newBrowser(t)
	var listed struct {
		Themes  []struct{ Name string } `json:"themes"`
		Current string                  `json:"current"`
	}
	resp, err := http.DefaultClient.Do(pageReq("GET", b.URL, "/api/themes", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil || len(listed.Themes) == 0 {
		t.Fatalf("/api/themes: %v %+v", err, listed)
	}
	if listed.Themes[0].Name != "office" || listed.Current != "office" {
		t.Fatalf("the default theme should be the pixel office, listed first: %+v", listed)
	}
	// what the bridge's content security policy blocks counts as an error too
	script := `window.__errors = []
addEventListener('error', e => window.__errors.push(String(e.message) + ' @' + e.filename + ':' + e.lineno))
addEventListener('unhandledrejection', e => window.__errors.push('rejection: ' + String(e.reason && e.reason.stack || e.reason)))
addEventListener('securitypolicyviolation', e => window.__errors.push('csp: ' + e.violatedDirective + ' ' + e.blockedURI + ' @' + e.sourceFile + ':' + e.lineNumber))`
	for _, th := range listed.Themes {
		ctx, cancel := chromedp.NewContext(browser)
		if err := chromedp.Do(ctx); err != nil {
			t.Fatal(err)
		}
		do(t, ctx, 30*time.Second, chromedp.Func(func(ctx context.Context, tg *chromedp.Target) error {
			_, err := cdp.Call(ctx, tg, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: script})
			return err
		}), chromedp.Navigate(b.URL+"/t/"+th.Name+"/"))
		waitUntil(t, 20*time.Second, th.Name+": the shared pieces are on the page", func() bool {
			return get(t, ctx, 5*time.Second, chromedp.Evaluate[bool](`!!document.getElementById('chat') && !!document.getElementById('launch') && !!document.getElementById('taskwin') && !!document.getElementById('task')`))
		})
		if errs := get(t, ctx, 5*time.Second, chromedp.Evaluate[[]string](`window.__errors`)); len(errs) > 0 {
			t.Fatalf("%s: page errors: %v", th.Name, errs)
		}
		if name := get(t, ctx, 5*time.Second, chromedp.Evaluate[string](`document.documentElement.dataset.theme || ''`)); name != th.Name {
			t.Fatalf("%s: <html data-theme> is %q", th.Name, name)
		}
		if missing := get(t, ctx, 5*time.Second, chromedp.Evaluate[[]string](`window.__i18nMissing || []`)); len(missing) > 0 {
			t.Fatalf("%s: keys the catalog lacks: %v", th.Name, missing)
		}
		// the pick is remembered: / now serves this theme
		do(t, ctx, 30*time.Second, chromedp.Navigate(b.URL+"/"))
		waitUntil(t, 20*time.Second, th.Name+": / serves the theme picked", func() bool {
			return get(t, ctx, 5*time.Second, chromedp.Evaluate[string](`document.documentElement.dataset.theme || ''`)) == th.Name
		})
		cancel()
	}
}
