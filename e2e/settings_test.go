//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
// picks its colors, remembered. The employees' default folder: the bridge's, shown in the settings; a relative one
// refused; another saved (settings.json) and the default back with its button, the focus kept in the dialog; the page
// names an employee's own folder as the bridge does; a new employee started without a folder gets one there, which the
// launch dialog's placeholder and preview name, a new git repository bound to it. What a new employee starts with: the
// model, the effort, the permission mode and the office context, Claude Code's own at first, each saved as it changes
// (settings.json), a red line under bypassPermissions; the launch dialog folds them, with the folder, into Advanced
// settings, a line saying what the start will use (and the bypassPermissions warning under it); unfolded, filled in as
// the settings say, one changed for this start (the settings untouched) and all set back by "Use the settings"; the
// start gets what the dialog says. An employee going back to work starts as it last did, and can take the settings';
// the task dialog's permission mode, a new advisor's and an employee's never started from the page as the settings say.
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

	// ---------- the employees' default folder ----------
	// (in a tab of its own, last: opening the settings counts the error log as seen, which Coworkers' gear checks above)
	sctx, _ := clockTab(t, browser, b.URL+"/t/office/?lang=en", 0)
	do(t, sctx, 10*time.Second, chromedp.EmulateViewport(1400, 900))
	s := &screens{t: t, ctx: sctx, lang: "settings-staff"}
	waitUntil(t, 20*time.Second, "the office shows its Settings button", func() bool { return s.is(`!!document.getElementById('open-settings')`) })
	def := b.Hub.Settings().BotHomeDefault
	s.run(`document.getElementById('open-settings').click()`)
	waitUntil(t, 10*time.Second, "the settings show the bridge's default folder, nothing to go back to", func() bool {
		return s.is(fmt.Sprintf(`document.getElementById('set-bot-home').value === %q && document.querySelector('#settings .staff .reset').hidden`, def))
	}, func() string { return s.js(`document.querySelector('#settings .staff').innerText`) })
	if n := ax(t, sctx, `document.getElementById('set-bot-home')`); n.Name != "Default folder" {
		t.Errorf("the field should be named by its label: %+v", n)
	}
	// a relative folder: refused, said in the status line
	s.run(`(f => { f.value = 'relative/bots'; f.dispatchEvent(new Event('input')) })(document.getElementById('set-bot-home')); document.querySelector('#settings .staff form').requestSubmit()`)
	waitUntil(t, 10*time.Second, "a relative folder is refused", func() bool {
		return s.is(`(x => x.classList.contains('err') && x.textContent === 'Not saved: the folder must be an absolute path')(document.querySelector('#settings .staff .said'))`)
	}, func() string { return s.js(`document.querySelector('#settings .staff .said').textContent`) })
	// another folder: saved with Enter, in settings.json, with "Use the default" to go back
	mine := filepath.Join(t.TempDir(), "my bots")
	s.run(fmt.Sprintf(`(f => { f.value = %q; f.dispatchEvent(new Event('input')); f.focus() })(document.getElementById('set-bot-home'))`, mine))
	key(sctx, kb.Enter)
	waitUntil(t, 10*time.Second, "saved", func() bool {
		return b.Hub.Settings().BotHome == mine && s.is(`document.querySelector('#settings .staff .said').textContent === 'Saved.' && !document.querySelector('#settings .staff .said').classList.contains('err') && !document.querySelector('#settings .staff .reset').hidden`)
	}, func() string { return s.js(`document.querySelector('#settings .staff').innerText`) })
	if data, err := os.ReadFile(filepath.Join(b.Data, "settings.json")); err != nil || !strings.Contains(string(data), "my bots") {
		t.Errorf("settings.json should keep it: %s %v", data, err)
	}
	s.shot("bot-home")
	s.run(`document.querySelector('#settings .staff .reset').focus()`)
	key(sctx, kb.Enter)
	waitUntil(t, 10*time.Second, "back to the default, the focus on the field", func() bool {
		return b.Hub.Settings().BotHome == def && s.is(fmt.Sprintf(`document.getElementById('set-bot-home').value === %q && document.querySelector('#settings .staff .reset').hidden && document.activeElement === document.getElementById('set-bot-home')`, def))
	})
	// what was typed and not saved goes when the settings open again
	s.run(`document.getElementById('set-bot-home').value = '/typed/not/saved'; document.querySelector('#settings .close').click()`)
	s.run(`document.getElementById('open-settings').click()`)
	waitUntil(t, 10*time.Second, "the field shows the bridge's folder again", func() bool {
		return s.is(fmt.Sprintf(`document.getElementById('set-bot-home').value === %q && document.querySelector('#settings .staff .said').textContent === ''`, def))
	})
	s.run(`document.querySelector('#settings .close').click()`)

	// the page names the employees' own folders as the bridge does (internal/bridge's TestBotFolderNames)
	names := map[string]string{"Ann Lee": "Ann-Lee", "Ann/Lee": "Ann-Lee", "前端 小王": "前端-小王", "a..b": "a.b", "-x-": "x", "...": "employee",
		"x.lock": "x-lock", "a-!b": "a--b", "é_1": "é_1"}
	list, _ := json.Marshal(slices.Collect(maps.Keys(names)))
	var named map[string]string
	if err := json.Unmarshal([]byte(get(t, sctx, 10*time.Second, chromedp.Evaluate[string](fmt.Sprintf(`import('/ui/launch.js').then(m => JSON.stringify(Object.fromEntries(%s.map(n => [n, m.folderName(n)]))))`, list), chromedp.EvalAwaitPromise))), &named); err != nil {
		t.Fatal(err)
	}
	for name, want := range names {
		if named[name] != want {
			t.Errorf("folderName(%q) = %q, the bridge names it %q", name, named[name], want)
		}
	}

	// a new employee without a folder: the dialog says where it goes, and it works there, a new git repository
	if _, err := b.Hub.SetSettings(bridge.Settings{BotHome: mine}); err != nil {
		t.Fatal(err)
	}
	s.run(`document.getElementById('new-emp').click()`)
	waitUntil(t, 10*time.Second, "the launch dialog opens, its folder empty", func() bool {
		return s.is(`!document.getElementById('launch').hidden && document.querySelector('#launch [name=dir]').value === '' && !document.querySelector('#launch [name=dir]').required`)
	})
	s.run(`(f => { f.value = 'Kai Ro'; f.dispatchEvent(new Event('input', { bubbles: true })) })(document.querySelector('#launch [name=name]'))`)
	own := filepath.Join(mine, "Kai-Ro")
	waitUntil(t, 5*time.Second, "the placeholder and the preview name the folder it gets", func() bool {
		return s.is(fmt.Sprintf(`document.querySelector('#launch [name=dir]').placeholder === %q && document.querySelector('#launch .preview').textContent.startsWith(%q)`,
			"Leave empty for a new folder: "+own, "cd '"+own+"'\n"))
	}, func() string {
		return s.js(`document.querySelector('#launch [name=dir]').placeholder + ' | ' + document.querySelector('#launch .preview').textContent`)
	})
	s.shot("launch-own-folder")
	s.run(`document.querySelector('#launch form').requestSubmit()`)
	var started *bridge.Launch
	waitUntil(t, 20*time.Second, "it starts", func() bool {
		for _, l := range b.Hub.Snapshot().Launches {
			if l.Name == "Kai Ro" {
				started = l
			}
		}
		return started != nil
	}, func() string { return s.js(`document.querySelector('#launch .status').textContent`) })
	if started.Dir != own {
		t.Fatalf("it should start in %s: %s", own, started.Dir)
	}
	if out, err := exec.Command("git", "-C", own, "log", "--format=%s").CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "symphony: Kai Ro's folder" {
		t.Errorf("its folder should be a new git repository: %s %v", out, err)
	}
	// online (as its mod would report it): the dialog closes, and "Start work" later fills in its folder
	kai := b.Hub.Account(started.Account)
	if kai.Home != own {
		t.Errorf("the folder should be bound to it: %q", kai.Home)
	}
	b.post(t, kai.Token, "/api/event", map[string]any{"session": "s-k", "kind": "session.start", "cwd": own, "launch": started.ID})
	waitUntil(t, 10*time.Second, "online, the dialog closes", func() bool { return s.is(`document.getElementById('launch').hidden`) })

	// ---------- what a new employee starts with ----------
	// the settings: the model, the effort, the permission mode and the office context, Claude Code's own at first, each
	// saved as it changes (settings.json), the bypassPermissions warning under its select
	s.run(`document.getElementById('open-settings').click()`)
	waitUntil(t, 10*time.Second, "the settings show Claude Code's own for each", func() bool {
		return s.is(`(f => f('set-model').options.length > 1 && f('set-model').value === '' && f('set-effort').value === '' && f('set-mode').value === '' && !f('set-context').checked && f('set-mode-warn').hidden)(id => document.getElementById(id))`)
	})
	for id, want := range map[string]string{"set-model": "Model (--model)", "set-effort": "Thinking effort (--effort)", "set-mode": "Permission mode (--permission-mode)", "set-context": "Tell it who else is in the office"} {
		if n := ax(t, sctx, `document.getElementById('`+id+`')`); n.Name != want {
			t.Errorf("%s should be named by its label: %+v", id, n)
		}
	}
	pick := func(sel, value string) {
		t.Helper()
		s.run(fmt.Sprintf(`(s => { s.value = %q; s.dispatchEvent(new Event('change', { bubbles: true })) })(document.querySelector(%q))`, value, sel))
	}
	pick("#set-model", "opus")
	waitUntil(t, 10*time.Second, "the model saved as it changes", func() bool {
		return b.Hub.Settings().Model == "opus" && s.is(`document.querySelector('#settings .defaults .said').textContent === 'Saved.'`)
	}, func() string { return s.js(`document.querySelector('#settings .defaults .said').textContent`) })
	pick("#set-effort", "high")
	pick("#set-mode", "bypassPermissions")
	s.run(`document.getElementById('set-context').click()`)
	want := bridge.LaunchDefaults{Model: "opus", Effort: "high", PermissionMode: "bypassPermissions", Context: true}
	waitUntil(t, 10*time.Second, "each saved, the warning under the permission mode", func() bool {
		return b.Hub.Settings().LaunchDefaults == want && s.is(`!document.getElementById('set-mode-warn').hidden && document.getElementById('set-mode-warn').getClientRects().length > 0 && document.getElementById('set-mode').getAttribute('aria-describedby') === 'set-mode-warn'`)
	}, func() string { return fmt.Sprintf("%+v", b.Hub.Settings().LaunchDefaults) })
	if data, err := os.ReadFile(filepath.Join(b.Data, "settings.json")); err != nil || !strings.Contains(string(data), `"permissionMode": "bypassPermissions"`) || !strings.Contains(string(data), "my bots") {
		t.Errorf("settings.json should keep them, the folder too: %s %v", data, err)
	}
	s.shot("launch-defaults")
	s.run(`document.querySelector('#settings .close').click()`)

	// a new employee: Advanced settings folded, its line saying what the start will use (the settings'), the warning
	// under it; unfolded, the fields as the settings say, one changed for this start and all set back by a button
	s.run(`document.getElementById('new-emp').click()`)
	waitUntil(t, 10*time.Second, "the launch dialog opens folded, filled in as the settings say", func() bool {
		return s.is(`(f => !document.getElementById('launch').hidden && !f.querySelector('.adv').open && f.elements.model.value === 'opus' && f.elements.effort.value === 'high' && f.elements.permissionMode.value === 'bypassPermissions' && f.elements.context.checked && f.elements.dir.value === '' && f.querySelector('.use').hidden)(document.querySelector('#launch form'))`)
	}, func() string { return s.js(`document.querySelector('#launch .digest').textContent`) })
	if got := s.js(`document.querySelector('#launch .digest').textContent`); got != "own folder · Opus · high · bypassPermissions · knows who else is in the office" {
		t.Errorf("the folded line should say what the start will use: %q", got)
	}
	if !s.is(`(w => { if (w.hidden) return false; const r = w.getBoundingClientRect(); return r.height > 0 && w.contains(document.elementFromPoint(r.left + 4, r.top + r.height / 2)) })(document.querySelector('#launch .warn.out'))`) {
		t.Error("the bypassPermissions warning should show under the folded line")
	}
	s.shot("launch-folded")
	s.run(`document.querySelector('#launch .adv > summary').click()`)
	waitUntil(t, 5*time.Second, "unfolded: where the fields come from, the warning under the select, not under the line", func() bool {
		return s.is(`document.querySelector('#launch .adv').open && document.querySelector('#launch .adv-note').textContent === 'Filled in from the settings (Settings → Employees). Changing them here doesn\'t change the settings.' && document.querySelector('#launch .adv .warn').getClientRects().length > 0 && !document.querySelector('#launch .warn.out').getClientRects().length`)
	}, func() string { return s.js(`document.querySelector('#launch .adv-note').textContent`) })
	pick("#launch [name=model]", "sonnet")
	waitUntil(t, 5*time.Second, "one changed for this start: the button to set them back shows", func() bool {
		return s.is(`!document.querySelector('#launch .use').hidden && document.querySelector('#launch .preview').textContent.includes('--model sonnet --effort high --permission-mode bypassPermissions')`)
	}, func() string { return s.js(`document.querySelector('#launch .preview').textContent`) })
	s.shot("launch-unfolded")
	s.run(`document.querySelector('#launch .use').focus(); document.querySelector('#launch .use').click()`)
	waitUntil(t, 5*time.Second, "set back to the settings', the button gone, the focus on the folder", func() bool {
		return s.is(`(f => f.elements.model.value === 'opus' && f.querySelector('.use').hidden && document.activeElement === f.elements.dir)(document.querySelector('#launch form'))`)
	})
	if b.Hub.Settings().Model != "opus" {
		t.Errorf("the dialog changes no setting: %+v", b.Hub.Settings())
	}
	// started with the effort changed for this start: the bridge gets what the dialog says
	s.run(`(f => { f.value = 'Lee'; f.dispatchEvent(new Event('input', { bubbles: true })) })(document.querySelector('#launch [name=name]'))`)
	pick("#launch [name=effort]", "max")
	s.run(`document.querySelector('#launch form').requestSubmit()`)
	var lee *bridge.Launch
	waitUntil(t, 20*time.Second, "Lee starts", func() bool {
		for _, l := range b.Hub.Snapshot().Launches {
			if l.Name == "Lee" {
				lee = l
			}
		}
		return lee != nil
	}, func() string { return s.js(`document.querySelector('#launch .status').textContent`) })
	if lee.Model != "opus" || lee.Effort != "max" || !strings.Contains(lee.Command, "SYMPHONY_CONTEXT=1") || !strings.Contains(lee.Command, "--permission-mode bypassPermissions") {
		t.Errorf("it should start as the dialog says: %+v", lee)
	}
	leeAcc := b.Hub.Account(lee.Account)
	b.post(t, leeAcc.Token, "/api/event", map[string]any{"session": "s-l", "kind": "session.start", "cwd": lee.Dir, "launch": lee.ID})
	waitUntil(t, 10*time.Second, "Lee online, the dialog closes", func() bool { return s.is(`document.getElementById('launch').hidden`) })

	// the settings change: an employee going back to work starts as it last did, with a button to the settings'
	if _, err := b.Hub.SetSettings(bridge.Settings{BotHome: mine, Model: "haiku", Effort: "low", PermissionMode: "plan"}); err != nil {
		t.Fatal(err)
	}
	b.post(t, leeAcc.Token, "/api/event", map[string]any{"session": "s-l", "kind": "session.end", "reason": "exit"})
	s.run(fmt.Sprintf(`document.querySelector('.emp-label:not(.member)[data-account=%q]').click()`, leeAcc.ID))
	waitUntil(t, 10*time.Second, "Lee's window offers Start work", func() bool { return s.is(`!!document.querySelector('#chat .btns .go-work')`) })
	s.run(`document.querySelector('#chat .btns .go-work').click()`)
	waitUntil(t, 10*time.Second, "the dialog opens folded, as Lee last worked", func() bool {
		return s.is(`(f => !document.getElementById('launch').hidden && !f.querySelector('.adv').open && f.elements.model.value === 'opus' && f.elements.effort.value === 'max' && f.elements.permissionMode.value === 'bypassPermissions' && f.elements.context.checked && document.activeElement === f.querySelector('.go'))(document.querySelector('#launch form'))`)
	}, func() string { return s.js(`document.querySelector('#launch .digest').textContent`) })
	s.run(`document.querySelector('#launch .adv > summary').click()`)
	waitUntil(t, 5*time.Second, "it says so, and offers the settings'", func() bool {
		return s.is(`document.querySelector('#launch .adv-note').textContent === 'Filled in as it last worked. Changing them here doesn\'t change the settings.' && !document.querySelector('#launch .use').hidden`)
	})
	s.run(`document.querySelector('#launch .use').click()`)
	waitUntil(t, 5*time.Second, "the settings' taken: its own folder, Haiku, low, plan", func() bool {
		return s.is(`(f => f.elements.dir.value === '' && f.elements.model.value === 'haiku' && f.elements.effort.value === 'low' && f.elements.permissionMode.value === 'plan' && !f.elements.context.checked && f.querySelector('.warn.out').hidden)(document.querySelector('#launch form'))`)
	})
	s.run(`document.querySelector('#launch .adv > summary').click()`)
	if got := s.js(`document.querySelector('#launch .digest').textContent`); got != "own folder · Haiku · low · plan" {
		t.Errorf("the folded line: %q", got)
	}
	s.run(`document.querySelector('#launch .cancel').click()`)

	// the task dialog: its permission mode and a new advisor's model and effort as the settings say, and those of an
	// employee never started from the page (alice); one that was keeps its last choices (Lee's, Kai Ro's Claude Code's own)
	s.run(`document.querySelector('#chat .close').click(); document.getElementById('new-task').click()`)
	waitUntil(t, 10*time.Second, "the task dialog opens", func() bool {
		return s.is(`!document.getElementById('task').hidden && document.querySelectorAll('#task .mem').length === 3`)
	})
	s.run(`document.querySelector('#task .add-adv').click()`)
	got := s.js(`(() => JSON.stringify(Object.fromEntries([...document.querySelectorAll('#task .mem, #task .adv')].map(r => [r.querySelector('.nm')?.textContent ?? 'advisor', r.querySelector('[name=model]').value + '/' + r.querySelector('[name=effort]').value]).concat([['mode', document.querySelector('#task [name=permissionMode]').value]]))))()`)
	var choices map[string]string
	if err := json.Unmarshal([]byte(got), &choices); err != nil || !maps.Equal(choices, map[string]string{"alice": "haiku/low", "Kai Ro": "/", "Lee": "opus/max", "advisor": "haiku/low", "mode": "plan"}) {
		t.Errorf("the task dialog's choices: %s %v", got, err)
	}
	s.run(`document.querySelector('#task .cancel').click()`)
	s.done()
}
