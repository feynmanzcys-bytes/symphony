//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// TestSettings walks through the settings and the error log with made-up activity (no tokens). The pixel office: its
// HUD's Settings button opens a dialog named "Settings" with the focus in it; the pixel size, motion and the steps'
// fold take effect and are remembered; Esc closes it, and only it. Reporting a problem: nothing recorded at first; a
// script's error on the page and a start that failed (a folder that isn't there) go into the error log by themselves,
// marking the HUD's button while the dialog is shut; the settings count them, and what's recorded unfolds into them;
// "I want to report a problem" gives the report as a file (the browser's download) holding the errors, the bridge, the
// page and the bridge's last log lines but not the page key or a token, and says where it went; nothing asks to copy
// anything; Clear empties the log. Errors shown: the launch dialog's failure is a box saying the error log has it
// (its selection survives the office's redraws), and a drag that selects in a dialog and lets go over the backdrop
// leaves the dialog open (a click on the backdrop still closes it); a failure shown over the page (showError, instead
// of alert) goes into the error log too. Coworkers: its gear opens the same dialog, where light, dark or as the system
// picks its colors, remembered.
func TestSettings(t *testing.T) {
	requireBrowser(t)
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))
	b.Launcher.Claude = sleeper(t)
	srv := b.srv.Load()
	srv.Version = "v9.9.9-test"
	srv.LogTail = bridge.NewLogTail(50)
	fmt.Fprintf(srv.LogTail, "symphony v9.9.9-test listening: %s/?key=%s\nanother line\n", b.URL, b.Key)
	alice, err := b.Hub.CreateAccount("alice")
	if err != nil {
		t.Fatal(err)
	}
	b.post(t, alice.Token, "/api/event", map[string]any{"session": "s-a", "kind": "session.start", "cwd": "/work/app", "model": "claude-sonnet-5-5",
		"engine": map[string]any{"version": "2.1.316"}, "mod": "1.2.2"})
	browser := newBrowser(t)
	key := func(ctx context.Context, keys string) {
		t.Helper()
		do(t, ctx, 10*time.Second, chromedp.KeyEvent(keys))
		time.Sleep(120 * time.Millisecond) // (the windows' observers)
	}
	ctx, _ := clockTab(t, browser, b.URL+"/t/office/?lang=en", 0)
	do(t, ctx, 10*time.Second, chromedp.EmulateViewport(1400, 900))
	p := &screens{t: t, ctx: ctx, lang: "settings"}
	waitUntil(t, 20*time.Second, "the office shows alice and the Settings button", func() bool {
		return p.is(`document.querySelectorAll('.emp-label').length === 1 && !!document.getElementById('open-settings')`)
	})
	if p.is(`!!document.querySelector('#hud .prefs, #hud select, #hud .pixel-steps, #still')`) {
		t.Fatal("the HUD should keep its rarely used controls in the settings now")
	}
	news := `!document.querySelector('#open-settings .news').hidden`
	if p.is(news) {
		t.Fatal("no error yet: the Settings button shouldn't be marked")
	}

	// ---------- the dialog and what it sets ----------
	p.run(`document.getElementById('open-settings').focus()`)
	key(ctx, kb.Enter)
	waitUntil(t, 5*time.Second, "the settings open, the focus in them", func() bool {
		return p.is(`!document.getElementById('settings').hidden && document.getElementById('settings').contains(document.activeElement)`)
	})
	if n := ax(t, ctx, `document.getElementById('settings')`); n.Role != "dialog" || n.Name != "Settings" {
		t.Errorf("the settings should be a dialog named Settings: %+v", n)
	}
	waitUntil(t, 10*time.Second, "the theme picker and the about section fill in", func() bool {
		return p.is(`!!document.querySelector('#set-theme option[value=messenger]') && document.querySelector('#settings .about dl').textContent.includes('v9.9.9-test')`)
	})
	if !p.is(`document.querySelector('#settings .errlog .state').textContent === 'No errors recorded so far.' && document.querySelector('#settings .recorded').hidden && !!document.querySelector('#settings .errlog .report:not(:disabled)')`) {
		t.Errorf("an empty error log should say so: %q", p.js(`document.querySelector('#settings .errlog .state').textContent`))
	}
	p.shot("dialog")
	p.run(`document.querySelector('.pixel-steps button[data-level="2"]').click()`)
	waitUntil(t, 5*time.Second, "the pixel size changes and is kept", func() bool {
		return p.is(`localStorage.getItem('symphony.pixels') === '2' && document.querySelector('.pixel-steps button[data-level="2"]').getAttribute('aria-pressed') === 'true'`)
	})
	p.run(`document.querySelector('.pixel-steps button[data-level="0"]').click(); document.getElementById('set-still').click()`)
	waitUntil(t, 5*time.Second, "motion pauses", func() bool {
		return p.is(`document.documentElement.hasAttribute('data-still') && localStorage.getItem('symphony.still') === '1'`)
	})
	p.run(`document.getElementById('set-still').click()`)
	waitUntil(t, 5*time.Second, "motion goes on", func() bool { return p.is(`!document.documentElement.hasAttribute('data-still')`) })
	p.run(`document.getElementById('set-steps').click()`)
	if !p.is(`localStorage.getItem('symphony.steps') === 'open'`) {
		t.Error("the steps' fold should be remembered")
	}
	p.run(`document.getElementById('set-steps').click()`)
	key(ctx, kb.Escape)
	waitUntil(t, 5*time.Second, "Esc closes the settings, the focus back on the button", func() bool {
		return p.is(`document.getElementById('settings').hidden && document.activeElement === document.getElementById('open-settings')`)
	})

	// ---------- the error log fills as things go wrong ----------
	p.run(`setTimeout(() => { throw new Error('boom from the test') })`)
	if r := p.fetch("POST", "/api/launch", `{"account":"`+alice.ID+`","dir":"/no/such/folder","model":"","effort":"","permissionMode":"","resume":""}`); !strings.Contains(r, "launch_dir_missing") && !strings.Contains(r, "account_busy") {
		t.Logf("the launch answered: %s", r)
	}
	waitUntil(t, 10*time.Second, "the bridge keeps the page's error and the failed start", func() bool {
		srcs := map[string]bool{}
		for _, f := range b.Hub.Faults() {
			srcs[f.Source] = true
		}
		return srcs["page"] && srcs["launch"]
	}, func() string { return fmt.Sprint(b.Hub.Faults()) })
	waitUntil(t, 10*time.Second, "the Settings button is marked", func() bool {
		return p.is(news) && p.is(`document.getElementById('open-settings').getAttribute('aria-label') === 'Settings: something new in the error log'`)
	})
	// (the page's own errors are the test's, not the page's: they're cleared for done())
	p.run(`window.__errors = []`)
	p.run(`document.getElementById('open-settings').click()`)
	waitUntil(t, 10*time.Second, "the settings count them, and what's recorded unfolds into them, newest first", func() bool {
		p.run(`document.querySelector('#settings .recorded').open = true`)
		return p.is(`document.querySelectorAll('#settings .entry').length === 2 && document.querySelector('#settings .errlog .state').textContent === 'Errors recorded: 2' && !document.querySelector('#settings .recorded').hidden`) &&
			p.is(`[...document.querySelectorAll('#settings .entry .src')].map(s => s.textContent).sort().join() === 'Page,Session start'`)
	}, func() string { return p.js(`document.querySelector('#settings .errlog').innerText`) })
	waitUntil(t, 5*time.Second, "the error log in sight, the button's mark goes", func() bool { return !p.is(news) }, func() string {
		var lasts []string
		for _, f := range b.Hub.Faults() {
			lasts = append(lasts, f.Last.Format(time.RFC3339Nano)+" "+f.Source)
		}
		return p.js(`JSON.stringify({ seen: localStorage.getItem('symphony.faultsSeen'), scroll: document.querySelector('#settings .body').scrollTop, top: document.getElementById('set-errlog').getBoundingClientRect().top, body: document.querySelector('#settings .body').getBoundingClientRect().top })`) + " bridge: " + strings.Join(lasts, ", ")
	})
	p.run(`[...document.querySelectorAll('#settings .entry')].find(e => e.querySelector('.src').textContent === 'Page').querySelector('summary').click()`)
	waitUntil(t, 5*time.Second, "an entry unfolds into its whole text", func() bool {
		return p.is(`(e => e.open && e.querySelector('pre').textContent.includes('boom from the test') && e.querySelector('pre').textContent.includes('page: office'))([...document.querySelectorAll('#settings .entry')].find(e => e.open))`)
	})
	// "I want to report a problem": the report file (the browser's download is the test's: its name, its contents)
	p.run(`HTMLAnchorElement.prototype.click = function () { window.__saved = { name: this.download, href: this.href } }; URL.createObjectURL = (make => blob => { window.__blob = blob; return make(blob) })(URL.createObjectURL.bind(URL))`)
	p.run(`document.querySelector('#settings .errlog .report').click()`)
	waitUntil(t, 10*time.Second, "the report is saved as a file, and the settings say where", func() bool {
		return p.is(`!!window.__saved && /^symphony-report-\d{8}-\d{6}\.txt$/.test(window.__saved.name) && document.querySelector('#settings .errlog .saved').textContent.includes(window.__saved.name)`)
	}, func() string { return p.js(`JSON.stringify(window.__saved)`) })
	report := get(t, ctx, 10*time.Second, chromedp.Evaluate[string](`window.__blob.text()`, chromedp.EvalAwaitPromise))
	for _, want := range []string{"symphony — error report", "bridge: v9.9.9-test", "page: office · en", "errors: 2, newest first", "boom from the test", "/no/such/folder", "another line", "browser: "} {
		if !strings.Contains(report, want) {
			t.Errorf("the report lacks %q:\n%s", want, report)
		}
	}
	for name, secret := range map[string]string{"the page key": b.Key, "alice's token": alice.Token} {
		if strings.Contains(report, secret) {
			t.Errorf("the report carries %s:\n%s", name, report)
		}
	}
	if p.is(`!!document.querySelector('#settings button.copy, #settings [class*=copy]')`) {
		t.Error("nothing in the settings should ask to copy anything")
	}
	p.shot("errlog")
	p.run(`window.confirm = () => true; document.querySelector('#settings .errlog .clear').click()`)
	waitUntil(t, 10*time.Second, "cleared, the log is empty", func() bool {
		return len(b.Hub.Faults()) == 0 && p.is(`!document.querySelector('#settings .entry') && document.querySelector('#settings .recorded').hidden && document.querySelector('#settings .errlog .state').textContent === 'No errors recorded so far.'`)
	})
	p.run(`document.querySelector('#settings .close').click()`)

	// ---------- errors one can copy ----------
	// the launch dialog: a failed start in a box (the bridge answers the folder is missing), its text still selected
	// through the office's redraws; a drag that ends over the backdrop leaves the dialog open
	p.run(`document.querySelector('.emp-label').click()`)
	waitUntil(t, 10*time.Second, "alice's window opens", func() bool { return p.is(`!document.getElementById('chat').hidden`) })
	b.post(t, alice.Token, "/api/event", map[string]any{"session": "s-a", "kind": "session.end", "reason": "exit"})
	waitUntil(t, 10*time.Second, "she goes off duty, with Start work", func() bool {
		return p.is(`[...document.querySelectorAll('#chat .btns button')].some(b => b.textContent === 'Start work')`)
	})
	p.run(`[...document.querySelectorAll('#chat .btns button')].find(b => b.textContent === 'Start work').click()`)
	waitUntil(t, 10*time.Second, "the launch dialog opens", func() bool { return p.is(`!document.getElementById('launch').hidden`) })
	p.run(`(f => { f.value = '/no/such/folder'; f.dispatchEvent(new Event('input')) })(document.querySelector('#launch [name=dir]')); document.querySelector('#launch form').requestSubmit()`)
	waitUntil(t, 20*time.Second, "the failure shows in a box that says the error log has it, with nothing to copy", func() bool {
		return p.is(`(m => !!m && m.textContent.includes('/no/such/folder'))(document.querySelector('#launch .status .errbox .msg')) && document.querySelector('#launch .status .errbox .hint').textContent === 'Recorded in the error log. To report it: Settings → Report a problem.' && !document.querySelector('#launch .status .errbox button')`)
	}, func() string { return p.js(`document.querySelector('#launch .status').textContent`) })
	p.run(`getSelection().selectAllChildren(document.querySelector('#launch .status .errbox .msg'))`)
	selected := p.js(`getSelection().toString()`)
	for i := 0; i < 3; i++ { // the office redraws on every change
		b.post(t, alice.Token, "/api/event", map[string]any{"session": "s-a", "kind": "session.title", "title": fmt.Sprint("redraw ", i)})
		time.Sleep(150 * time.Millisecond)
	}
	if now := p.js(`getSelection().toString()`); now != selected || !strings.Contains(now, "/no/such/folder") {
		t.Errorf("the selection should survive the redraws: %q, then %q", selected, now)
	}
	var box struct{ X, Y, W, H float64 }
	if err := json.Unmarshal([]byte(p.js(`(r => JSON.stringify({ X:r.left, Y:r.top, W:r.width, H:r.height }))(document.querySelector('#launch .status .errbox .msg').getBoundingClientRect())`)), &box); err != nil {
		t.Fatal(err)
	}
	do(t, ctx, 10*time.Second,
		chromedp.MouseEvent(chromedp.MouseMoved, box.X+4, box.Y+4),
		chromedp.MouseEvent(chromedp.MousePressed, box.X+4, box.Y+4, chromedp.ButtonLeft, chromedp.ClickCount(1)),
		chromedp.MouseEvent(chromedp.MouseMoved, 30, 450, chromedp.ButtonLeft),
		chromedp.MouseEvent(chromedp.MouseReleased, 30, 450, chromedp.ButtonLeft, chromedp.ClickCount(1)))
	time.Sleep(300 * time.Millisecond)
	if p.is(`document.getElementById('launch').hidden`) {
		t.Error("a drag that selects in the dialog and lets go over the backdrop shouldn't close it")
	}
	do(t, ctx, 10*time.Second, chromedp.MouseClickXY(30, 450))
	waitUntil(t, 5*time.Second, "a click on the backdrop closes it", func() bool { return p.is(`document.getElementById('launch').hidden`) })
	// a failure over the page, in place of alert(): what it says, that the error log has it (the page reported it), OK
	p.run(`import('/ui/errors.js').then(m => m.showError('first line\n' + 'a long error, line after line\n'.repeat(60)))`)
	waitUntil(t, 5*time.Second, "the error shows over the page", func() bool {
		return p.is(`(o => !!o && !o.hidden && !!o.querySelector('.errbox .hint') && o.querySelectorAll('button').length === 1)(document.getElementById('oops'))`)
	})
	waitUntil(t, 10*time.Second, "the error it showed is in the error log", func() bool {
		for _, f := range b.Hub.Faults() {
			if f.Source == "page" && f.Code == "ui" && strings.HasPrefix(f.Text, "first line") {
				return true
			}
		}
		return false
	})
	if n := ax(t, ctx, `document.getElementById('oops')`); n.Role != "dialog" || n.Name != "Something went wrong" {
		t.Errorf("the error should be a dialog named for it: %+v", n)
	}
	p.shot("oops")
	key(ctx, kb.Escape)
	waitUntil(t, 5*time.Second, "Esc closes it", func() bool { return p.is(`document.getElementById('oops').hidden`) })
	p.done()

	// ---------- Coworkers: the gear, light or dark ----------
	mctx, _ := clockTab(t, browser, b.URL+"/t/messenger/?lang=en", 0)
	do(t, mctx, 10*time.Second, chromedp.EmulateViewport(1400, 900))
	m := &screens{t: t, ctx: mctx, lang: "settings-coworkers"}
	waitUntil(t, 20*time.Second, "Coworkers shows its gear", func() bool {
		return m.is(`!!document.querySelector('.side-actions .open-settings') && !document.getElementById('dark')`)
	})
	// (named Settings, and saying the error log has news: the failed start above came after the log was cleared)
	if n := ax(t, mctx, `document.querySelector('.side-actions .open-settings')`); n.Name != "Settings: something new in the error log" || !m.is(`!document.querySelector('.side-actions .open-settings .news').hidden`) {
		t.Errorf("the gear should be named Settings and say the error log has news: %+v", n)
	}
	m.run(`document.querySelector('.side-actions .open-settings').click()`)
	waitUntil(t, 5*time.Second, "the settings open with the colors", func() bool {
		return m.is(`!document.getElementById('settings').hidden && !document.querySelector('#settings .scheme').hidden && document.getElementById('set-scheme-system').checked`)
	})
	m.run(`document.getElementById('set-scheme-dark').click()`)
	waitUntil(t, 5*time.Second, "dark", func() bool {
		return m.is(`document.documentElement.hasAttribute('data-dark') && localStorage.getItem('symphony.messenger.dark') === '1'`)
	})
	m.shot("dark")
	m.run(`document.getElementById('set-scheme-light').click()`)
	waitUntil(t, 5*time.Second, "light", func() bool {
		return m.is(`!document.documentElement.hasAttribute('data-dark') && localStorage.getItem('symphony.messenger.dark') === '0'`)
	})
	m.run(`document.getElementById('set-scheme-system').click()`)
	waitUntil(t, 5*time.Second, "as the system (light here)", func() bool {
		return m.is(`!document.documentElement.hasAttribute('data-dark') && localStorage.getItem('symphony.messenger.dark') === null`)
	})
	m.done()
}
