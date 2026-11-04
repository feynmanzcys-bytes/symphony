//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// TestStaff walks through the employees page with made-up employees (no tokens; claude is a script that sits there):
// alice works in a session started by hand, bob was started from the page before, carol worked by hand and is off duty,
// an advisor is in no list. In Coworkers, Settings → "All coworkers (3)" opens the page over the settings, a dialog
// named "All coworkers", every control named: the employees by name, each with its state, its folder and what it
// starts with (one never started from the page "as the settings say", its folder the one it worked in); the find box
// narrows them. bob's Edit unfolds his row: a relative folder is refused, his own folder, the model, the effort,
// bypassPermissions (its warning) and the office context saved, the focus back on his Edit. alice and carol selected
// get an effort together, on the settings' other choices and the folders they worked in. Import: one name per line (a
// comment left out, a line that looks like columns noted), what it would do entry by entry (new, as it is), two new
// employees; JSON whose entries are wrong (a choice no drop-down has, an advisor's name, a name twice, no name) can't be
// imported; a folder that isn't there is refused by the bridge, saying which entry, and nothing is made; a new one with
// its folder and choices, a field it doesn't know noted, and a change (from → to); a file chosen fills the list.
// Export gives every employee as the import reads it (no advisor, no token), and read back it changes nothing. Delete
// deletes the selected ones off duty and says why one on duty stays. Esc closes the row being edited, then the page,
// back on the settings' button. The launch dialog starts an employee as the page says (its own folder, not where its
// session went). The pixel office's settings open the same page.
func TestStaff(t *testing.T) {
	requireBrowser(t)
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge")) // (development mode: the page can start sessions)
	b.Launcher.Claude = sleeper(t)
	if _, err := b.Hub.SetSettings(bridge.Settings{Model: "opus", Effort: "high"}); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	bobDir, frankDir := filepath.Join(work, "web"), filepath.Join(work, "frank")
	for _, d := range []string{bobDir, frankDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tokens := map[string]string{}
	for _, name := range []string{"alice", "bob", "carol"} {
		acc, err := b.Hub.CreateAccount(name)
		if err != nil {
			t.Fatal(err)
		}
		tokens[name] = acc.Token
	}
	event := func(name, session string, body map[string]any) {
		t.Helper()
		body["session"] = session
		b.post(t, tokens[name], "/api/event", body)
	}
	event("alice", "s-alice", map[string]any{"kind": "session.start", "cwd": "/work/app", "model": "claude-opus-5-5"})
	event("alice", "s-alice", map[string]any{"kind": "tool.start", "tool": "Bash"})
	event("carol", "s-carol", map[string]any{"kind": "session.start", "cwd": "/work/docs"})
	event("carol", "s-carol", map[string]any{"kind": "session.end"})
	sonnet, low := "sonnet", "low"
	if _, err := b.Hub.ImportStaff([]bridge.StaffEntry{{Name: "bob", Dir: &bobDir, Model: &sonnet, Effort: &low}}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Hub.CreateAdvisor("rev"); err != nil {
		t.Fatal(err)
	}
	id := func(name string) string {
		t.Helper()
		if a := b.account(name); a != nil {
			return a.ID
		}
		t.Fatalf("no employee %s", name)
		return ""
	}
	last := func(name string) bridge.LaunchRequest {
		t.Helper()
		if a := b.account(name); a != nil && a.Last != nil {
			return *a.Last
		}
		return bridge.LaunchRequest{Name: "(none)"}
	}

	browser := newBrowser(t)
	key := func(p *screens, k string) {
		t.Helper()
		do(t, p.ctx, 10*time.Second, chromedp.KeyEvent(k))
		time.Sleep(120 * time.Millisecond) // (the windows' observers)
	}
	ctx, _ := clockTab(t, browser, b.URL+"/t/messenger/?lang=en", 0)
	do(t, ctx, 10*time.Second, chromedp.EmulateViewport(1400, 900))
	p := &screens{t: t, ctx: ctx, lang: "staff"}
	waitUntil(t, 20*time.Second, "Coworkers lists the three", func() bool { return p.is(`document.querySelectorAll('#people .row').length === 3`) })
	row := func(name string) string {
		return fmt.Sprintf(`document.querySelector('#staff .emp[data-account=%q]')`, id(name))
	}
	setText := func(sel, value string) {
		t.Helper()
		v, _ := json.Marshal(value)
		p.run(fmt.Sprintf(`(x => { x.value = %s; x.dispatchEvent(new Event('input', { bubbles: true })) })(document.querySelector(%q))`, v, sel))
	}
	pick := func(sel, value string) {
		t.Helper()
		p.run(fmt.Sprintf(`(s => { s.value = %q; s.dispatchEvent(new Event('change', { bubbles: true })) })(document.querySelector(%q))`, value, sel))
	}

	// ---------- the page, from the settings ----------
	p.run(`document.querySelector('.open-settings').click()`)
	waitUntil(t, 10*time.Second, "the settings show the button to every employee", func() bool {
		return p.is(`!document.getElementById('settings').hidden && document.querySelector('#settings .people .all').textContent === 'All coworkers (3)'`)
	})
	p.run(`document.querySelector('#settings .people .all').focus()`)
	key(p, kb.Enter)
	waitUntil(t, 10*time.Second, "the page opens over the settings, the focus in it, the three by name", func() bool {
		return p.is(`!document.getElementById('staff').hidden && !document.getElementById('settings').hidden && document.getElementById('staff').contains(document.activeElement) &&
			[...document.querySelectorAll('#staff .emp .nm')].map(n => n.textContent).join() === 'alice,bob,carol' && document.querySelectorAll('#staff .batch select[name=model] option').length > 2`)
	}, func() string { return p.js(`document.getElementById('staff').innerText`) })
	if n := ax(t, ctx, `document.getElementById('staff')`); n.Role != "dialog" || n.Name != "All coworkers" {
		t.Errorf("the page should be a dialog named All coworkers: %+v", n)
	}
	for name, want := range map[string][]string{
		"alice": {"Working · Bash", "as the settings say", "/work/app", "Opus · high"},
		"bob":   {"Off duty", bobDir, "Sonnet · low"},
		"carol": {"Off duty", "as the settings say", "/work/docs", "Opus · high"},
	} {
		text := p.js(row(name) + `.innerText`)
		for _, w := range want {
			if !strings.Contains(text, w) {
				t.Errorf("%s's row should say %q: %q", name, w, text)
			}
		}
	}
	if p.is(`document.querySelector('#staff .list').textContent.includes('rev')`) || !p.is(`document.querySelector('#staff .head .count').textContent === 'Coworkers: 3'`) {
		t.Error("an advisor belongs to its task: not on the page, nor counted")
	}
	if bad := unnamed(t, ctx, `document.getElementById('staff')`); len(bad) > 0 {
		t.Errorf("controls without a name on the page: %v", bad)
	}
	p.shot("list")
	setText("#staff-find", "car")
	waitUntil(t, 5*time.Second, "the find box narrows the list to carol", func() bool {
		return p.is(`[...document.querySelectorAll('#staff .emp .nm')].map(n => n.textContent).join() === 'carol'`)
	})
	setText("#staff-find", "zzz")
	waitUntil(t, 5*time.Second, "nobody matches, and it says so", func() bool {
		return p.is(`!document.querySelector('#staff .emp') && document.querySelector('#staff .list .empty').textContent === 'No employee matches.'`)
	})
	setText("#staff-find", "")

	// ---------- one row edited ----------
	p.run(row("bob") + `.querySelector('.edit').click()`)
	waitUntil(t, 5*time.Second, "bob's row unfolds with what he starts with, the focus on the folder", func() bool {
		return p.is(row("bob") + `.nextElementSibling?.id === 'staff-editor' && document.getElementById('staff-edit-h').textContent === 'What bob starts with' && ` +
			fmt.Sprintf(`document.getElementById('staff-dir').value === %q && document.getElementById('staff-model').value === 'sonnet' && document.getElementById('staff-effort').value === 'low' && `, bobDir) +
			`document.getElementById('staff-mode').value === '' && document.activeElement === document.getElementById('staff-dir') && ` + row("bob") + `.querySelector('.edit').getAttribute('aria-expanded') === 'true'`)
	})
	setText("#staff-dir", "relative/dir")
	p.run(`document.querySelector('#staff-editor .save').click()`)
	waitUntil(t, 5*time.Second, "a relative folder is refused", func() bool {
		return p.is(`document.querySelector('#staff-editor .said').textContent === 'Not saved: the folder must be an absolute path'`)
	}, func() string { return p.js(`document.querySelector('#staff-editor .said').textContent`) })
	if l := last("bob"); l.Dir != bobDir {
		t.Fatalf("nothing should have been saved: %+v", l)
	}
	setText("#staff-dir", "")
	if !p.is(`document.querySelector('#staff-editor .said').textContent === ''`) {
		t.Error("what saving said goes once the fields change")
	}
	pick("#staff-model", "haiku")
	pick("#staff-effort", "max")
	pick("#staff-mode", "bypassPermissions")
	if !p.is(`!document.getElementById('staff-warn').hidden && document.getElementById('staff-mode').getAttribute('aria-describedby') === 'staff-warn'`) {
		t.Error("bypassPermissions should show its warning, the select's description")
	}
	p.run(`document.querySelector('#staff-editor [name=context]').click()`)
	p.shot("edit")
	p.run(`document.querySelector('#staff-editor .save').click()`)
	waitUntil(t, 10*time.Second, "bob's choices saved, his row folded, the focus back on his Edit", func() bool {
		l := last("bob")
		return l.Dir == "" && l.Model == "haiku" && l.Effort == "max" && l.PermissionMode == "bypassPermissions" && l.Context &&
			p.is(`!document.getElementById('staff-editor') && document.activeElement === `+row("bob")+`.querySelector('.edit') && document.querySelector('#staff .said.main').textContent === 'Saved: bob starts so next time.'`)
	}, func() string {
		return fmt.Sprintf("%+v %s", last("bob"), p.js(`document.querySelector('#staff .said.main').textContent`))
	})
	if text := p.js(row("bob") + `.querySelector('.cfg').textContent`); !strings.Contains(text, "own folder") || !strings.Contains(text, filepath.Join("bots", "bob")) || !strings.Contains(text, "Haiku · max · bypassPermissions · knows who else is in the office") {
		t.Errorf("bob's row should say his own folder (made at his first start) and his choices: %q", text)
	}

	// ---------- the selected ones, together ----------
	if !p.is(`document.querySelector('#staff-batch-n').textContent === 'Select employees to change them together.' && document.querySelector('#staff .batch .apply').disabled && document.querySelector('#staff .batch select').disabled`) {
		t.Error("with nobody selected the drop-downs and buttons wait")
	}
	p.run(row("alice") + `.querySelector('.pick').click(); ` + row("carol") + `.querySelector('.pick').click()`)
	waitUntil(t, 5*time.Second, "two selected, the select-all box half", func() bool {
		return p.is(`document.querySelector('#staff-batch-n').textContent === 'Selected: 2' && document.getElementById('staff-all').indeterminate && document.querySelector('#staff .batch .apply').disabled && !document.querySelector('#staff .batch select').disabled`)
	})
	pick("#staff .batch select[name=effort]", "medium")
	p.run(`document.querySelector('#staff .batch .apply').click()`)
	waitUntil(t, 10*time.Second, "alice and carol get the effort, on the settings' model and the folders they worked in", func() bool {
		a, c := last("alice"), last("carol")
		return a == bridge.LaunchRequest{Dir: "/work/app", Model: "opus", Effort: "medium"} && c == bridge.LaunchRequest{Dir: "/work/docs", Model: "opus", Effort: "medium"} &&
			p.is(`document.querySelector('#staff .said.main').textContent === 'Changed: alice, carol.' && document.querySelector('#staff .batch select[name=effort]').value === '*'`)
	}, func() string {
		return fmt.Sprintf("%+v %+v %s", last("alice"), last("carol"), p.js(`document.querySelector('#staff .said.main').textContent`))
	})
	if p.is(row("alice") + `.textContent.includes('as the settings say')`) {
		t.Error("alice has choices of her own now")
	}
	p.run(`document.getElementById('staff-all').click()`)
	if !p.is(`document.querySelectorAll('#staff .emp .pick:checked').length === 3 && document.getElementById('staff-all').checked`) {
		t.Error("select all should select the three")
	}
	p.run(`document.getElementById('staff-all').click()`)

	// ---------- importing ----------
	importing := func(text string) {
		t.Helper()
		if p.is(`document.getElementById('staff-import').hidden`) {
			p.run(`document.querySelector('#staff .tools .import').click()`)
		}
		setText("#staff-import-text", text)
	}
	summary := func(want string) {
		t.Helper()
		waitUntil(t, 5*time.Second, "the import's summary: "+want, func() bool { return p.is(fmt.Sprintf(`document.getElementById('staff-sum').textContent === %q`, want)) },
			func() string {
				return p.js(`document.getElementById('staff-sum').textContent + ' | ' + document.querySelector('#staff .importer .preview').innerText`)
			})
	}
	previewOf := func() string {
		return p.js(`[...document.querySelectorAll('#staff .importer .preview li')].map(l => l.className + ': ' + l.innerText.replace(/\s+/g, ' ')).join('\n')`)
	}
	importing("dave\nerin\n# a comment\nbob\nalice, the lead")
	summary("New: 3 · changed: 0 · with a problem: 0")
	if !p.is(`document.querySelector('#staff .tools .import').getAttribute('aria-expanded') === 'true'`) || !strings.Contains(previewOf(), "the whole line is the name") {
		t.Errorf("a line that looks like columns should be noted:\n%s", previewOf())
	}
	importing("dave\nerin\n# a comment\nbob")
	summary("New: 2 · changed: 0 · with a problem: 0")
	if got := previewOf(); got != "new: New dave as the settings say\nnew: New erin as the settings say\nsame: As it is bob nothing changes" {
		t.Errorf("the preview, entry by entry:\n%s", got)
	}
	if !p.is(`document.querySelector('#staff .importer .go').textContent === 'Import (2)' && !document.querySelector('#staff .importer .go').disabled`) {
		t.Errorf("Import should go for two: %s", p.js(`document.querySelector('#staff .importer .go').textContent`))
	}
	p.shot("import")
	p.run(`document.querySelector('#staff .importer .go').click()`)
	waitUntil(t, 10*time.Second, "dave and erin are employees, going by the settings", func() bool {
		return b.account("dave") != nil && b.account("dave").Last == nil && b.account("erin") != nil &&
			p.is(`document.getElementById('staff-import').hidden && document.querySelector('#staff .said.main').textContent === 'Imported: 2 new, 0 changed.' && document.querySelectorAll('#staff .emp').length === 5`)
	}, func() string {
		return fmt.Sprintf("dave=%v erin=%v %s", b.account("dave"), b.account("erin"), p.js(`[document.getElementById('staff-import').hidden, document.querySelector('#staff .said.main').textContent, document.querySelector('#staff .importer .said').textContent, document.querySelectorAll('#staff .emp').length].join(' | ')`))
	})
	importing(`[{"name": "frank", "model": "gpt"}, {"name": "rev"}, {"name": "frank"}, {"nope": 1}]`)
	summary("New: 0 · changed: 0 · with a problem: 4")
	if got := previewOf(); !strings.Contains(got, `frank Model can't be “gpt”`) || !strings.Contains(got, "rev that name is already taken") ||
		!strings.Contains(got, "frank the list names this employee twice") || !strings.Contains(got, "Entry 4 not a name, nor an object with a name") || !p.is(`document.querySelector('#staff .importer .go').disabled`) {
		t.Errorf("each wrong entry should say why, and nothing go:\n%s", got)
	}
	importing(`{"employees": [{"name": "bob"}, {"name": "frank", "dir": "/no/such/folder"}]}`)
	summary("New: 1 · changed: 0 · with a problem: 0")
	p.run(`document.querySelector('#staff .importer .go').click()`)
	waitUntil(t, 10*time.Second, "the bridge refuses a folder that isn't there, saying which entry of the list", func() bool {
		return p.is(`document.querySelector('#staff .importer .said').textContent === 'Nothing was imported: employee 2, frank: folder does not exist: /no/such/folder'`)
	}, func() string { return p.js(`document.querySelector('#staff .importer .said').textContent`) })
	if b.account("frank") != nil {
		t.Fatal("a refused import should make nobody")
	}
	importing(fmt.Sprintf(`{"employees": [{"name": "frank", "dir": %q, "model": "Opus", "permissionMode": "plan", "context": "yes", "color": "red"}, {"name": "bob", "model": "sonnet"}, {"name": "dave", "model": "opus"}]}`, frankDir))
	summary("New: 1 · changed: 1 · with a problem: 0")
	if got, want := previewOf(), fmt.Sprintf("new: New frank Folder: %s · Model: Opus · Permission mode: plan · Office context: on · “color” isn't read and is left out\nchange: Changes bob Model: Haiku → Sonnet\nsame: As it is dave nothing changes", frankDir); got != want {
		t.Errorf("the preview:\n%s\nwant:\n%s", got, want)
	}
	p.run(`document.querySelector('#staff .importer .go').click()`)
	waitUntil(t, 10*time.Second, "frank made with his folder and choices (the settings' effort), bob's model changed", func() bool {
		return last("frank") == bridge.LaunchRequest{Dir: frankDir, Model: "opus", Effort: "high", PermissionMode: "plan", Context: true} &&
			last("bob") == bridge.LaunchRequest{Model: "sonnet", Effort: "max", PermissionMode: "bypassPermissions", Context: true} &&
			b.account("dave").Last == nil && // (named as the settings say: he goes on going by them)
			p.is(`document.querySelector('#staff .said.main').textContent === 'Imported: 1 new, 1 changed.'`)
	}, func() string { return fmt.Sprintf("%+v %+v", last("frank"), last("bob")) })
	names := filepath.Join(work, "names.txt")
	if err := os.WriteFile(names, []byte("gina\nhank\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p.run(`document.querySelector('#staff .tools .import').click()`)
	do(t, ctx, 10*time.Second, chromedp.SetUploadFiles(`#staff .importer input[type=file]`, []string{names}))
	waitUntil(t, 10*time.Second, "a file chosen fills the list", func() bool {
		return p.is(`document.getElementById('staff-import-text').value === 'gina\nhank\n' && document.getElementById('staff-sum').textContent === 'New: 2 · changed: 0 · with a problem: 0'`)
	})
	p.run(`document.querySelector('#staff .importer .go').click()`)
	waitUntil(t, 10*time.Second, "gina and hank are employees", func() bool { return b.account("gina") != nil && b.account("hank") != nil })

	// ---------- exporting ----------
	p.run(`HTMLAnchorElement.prototype.click = function () { window.__saved = { name: this.download, href: this.href } }; URL.createObjectURL = (make => blob => { window.__blob = blob; return make(blob) })(URL.createObjectURL.bind(URL))`)
	p.run(`document.querySelector('#staff .tools .export').click()`)
	waitUntil(t, 10*time.Second, "the list is saved as a file, and the page says where", func() bool {
		return p.is(`!!window.__saved && /^symphony-employees-\d{8}-\d{6}\.json$/.test(window.__saved.name) && document.querySelector('#staff .said.main').textContent.includes(window.__saved.name)`)
	})
	exported := get(t, ctx, 10*time.Second, chromedp.Evaluate[string](`window.__blob.text()`, chromedp.EvalAwaitPromise))
	var file struct {
		Employees []map[string]any `json:"employees"`
	}
	if err := json.Unmarshal([]byte(exported), &file); err != nil {
		t.Fatalf("the export isn't JSON: %v\n%s", err, exported)
	}
	var got []string
	for _, e := range file.Employees {
		got = append(got, fmt.Sprintf("%v %d", e["name"], len(e)))
	}
	if strings.Join(got, ",") != "alice 6,bob 6,carol 6,dave 1,erin 1,frank 6,gina 1,hank 1" || strings.Contains(exported, "rev") || strings.Contains(exported, "token") || strings.Contains(exported, tokens["bob"]) {
		t.Errorf("the export should hold every employee by name, its choices when it has them, no advisor, no token: %v\n%s", got, exported)
	}
	importing(exported)
	summary("New: 0 · changed: 0 · with a problem: 0")
	if !p.is(`document.querySelector('#staff .importer .go').disabled && document.querySelector('#staff .importer .go').textContent === 'Import' && [...document.querySelectorAll('#staff .importer .preview li')].every(l => l.className === 'same')`) {
		t.Errorf("the export read back should change nothing:\n%s", previewOf())
	}
	p.run(`document.querySelector('#staff .importer .cancel').click()`)

	// ---------- deleting the selected ----------
	p.run(`window.confirm = () => true; ` + row("alice") + `.querySelector('.pick').click(); ` + row("gina") + `.querySelector('.pick').click(); ` + row("hank") + `.querySelector('.pick').click()`)
	p.run(`document.querySelector('#staff .batch .delete').click()`)
	waitUntil(t, 10*time.Second, "gina and hank deleted, alice on duty stays and the page says why", func() bool {
		return b.account("gina") == nil && b.account("hank") == nil && b.account("alice") != nil &&
			p.is(`document.querySelector('#staff .said.main').textContent === "Deleted: gina, hank.\nalice wasn't deleted: this account already has a session online, or one starting" && document.querySelectorAll('#staff .emp').length === 6 && document.querySelector('#staff-batch-n').textContent === 'Selected: 1'`)
	}, func() string { return p.js(`document.querySelector('#staff .said.main').textContent`) })
	p.run(row("alice") + `.querySelector('.pick').click()`)

	// ---------- Esc: the row being edited, then the page ----------
	p.run(row("carol") + `.querySelector('.edit').focus()`)
	key(p, kb.Enter)
	waitUntil(t, 5*time.Second, "carol's row unfolds", func() bool { return p.is(row("carol") + `.nextElementSibling?.id === 'staff-editor'`) })
	key(p, kb.Escape)
	waitUntil(t, 5*time.Second, "Esc folds the row, the focus on its Edit", func() bool {
		return p.is(`!document.getElementById('staff-editor') && !document.getElementById('staff').hidden && document.activeElement === ` + row("carol") + `.querySelector('.edit')`)
	})
	key(p, kb.Escape)
	waitUntil(t, 5*time.Second, "Esc closes the page, back on the settings' button", func() bool {
		return p.is(`document.getElementById('staff').hidden && !document.getElementById('settings').hidden && document.activeElement === document.querySelector('#settings .people .all') && document.querySelector('#settings .people .all').textContent === 'All coworkers (6)'`)
	})
	key(p, kb.Escape)
	waitUntil(t, 5*time.Second, "Esc closes the settings", func() bool { return p.is(`document.getElementById('settings').hidden`) })

	// ---------- the launch dialog starts them as the page says ----------
	empty := ""
	if _, err := b.Hub.ImportStaff([]bridge.StaffEntry{{Name: "carol", Dir: &empty}}); err != nil { // her own folder, not where she worked
		t.Fatal(err)
	}
	for _, c := range []struct{ name, dir, model, effort, mode string }{{"carol", "", "opus", "medium", ""}, {"bob", "", "sonnet", "max", "bypassPermissions"}, {"frank", frankDir, "opus", "high", "plan"}} {
		p.run(fmt.Sprintf(`document.querySelector('#people .row[data-account=%q]').click()`, id(c.name)))
		waitUntil(t, 10*time.Second, c.name+"'s conversation offers to start work", func() bool { return p.is(`!!document.querySelector('#chat .go-work')`) })
		p.run(`document.querySelector('#chat .go-work').click()`)
		waitUntil(t, 10*time.Second, "the launch dialog fills in "+c.name+"'s choices", func() bool {
			return p.is(fmt.Sprintf(`!document.getElementById('launch').hidden && document.querySelector('#launch [name=dir]').value === %q && document.querySelector('#launch [name=model]').value === %q && document.querySelector('#launch [name=effort]').value === %q && document.querySelector('#launch [name=permissionMode]').value === %q`, c.dir, c.model, c.effort, c.mode))
		}, func() string {
			return p.js(`['dir', 'model', 'effort', 'permissionMode'].map(n => n + '=' + document.querySelector('#launch [name=' + n + ']').value).join(' ')`)
		})
		p.run(`document.querySelector('#launch .cancel').click()`)
	}
	p.done()

	// ---------- the pixel office's settings open the same page ----------
	office, _ := clockTab(t, browser, b.URL+"/t/office/?lang=en", 0)
	do(t, office, 10*time.Second, chromedp.EmulateViewport(1400, 900))
	o := &screens{t: t, ctx: office, lang: "staff-office"}
	waitUntil(t, 20*time.Second, "the office shows the six", func() bool { return o.is(`document.querySelectorAll('.emp-label').length === 6`) })
	o.run(`document.getElementById('open-settings').click(); document.querySelector('#settings .people .all').click()`)
	waitUntil(t, 10*time.Second, "the employees page lists the six", func() bool {
		return o.is(`!document.getElementById('staff').hidden && [...document.querySelectorAll('#staff .emp .nm')].map(n => n.textContent).join() === 'alice,bob,carol,dave,erin,frank'`)
	})
	o.shot("list")
	o.done()
}
