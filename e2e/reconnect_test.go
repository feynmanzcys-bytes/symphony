//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// TestReconnect checks with made-up activity (no tokens) how a page lives with its link to the bridge:
//   - the client keeps the message log as the bridge does: each task shown its newest 300, the rest together 300;
//   - a bridge that can't be reached shows the connection banner, and a call without an answer says so (code
//     unreachable); when the bridge is back the banner goes, and the open employee window fetches its log again, with
//     what happened meanwhile;
//   - a page hidden for 10 seconds lets its stream go, and catches up when it shows again;
//   - a row of the roster redrawn while it is pressed still opens its window on the release.
func TestReconnect(t *testing.T) {
	requireBrowser(t)
	b := startBridge(t, "")
	alice, err := b.Hub.CreateAccount("alice")
	if err != nil {
		t.Fatal(err)
	}
	event := func(body map[string]any) {
		t.Helper()
		body["session"] = "s-alice"
		b.post(t, alice.Token, "/api/event", body)
	}
	tool := func(id, name string) {
		t.Helper()
		event(map[string]any{"kind": "tool.start", "tool": name, "toolUseId": id, "summary": name + " " + id})
		event(map[string]any{"kind": "tool.end", "tool": name, "toolUseId": id})
	}
	event(map[string]any{"kind": "session.start", "cwd": "/work/alice", "model": "claude-sonnet-5-5"})
	tool("u1", "Read")

	browser := newBrowser(t)
	pg, _ := clockTab(t, browser, b.URL+"/t/messenger/?lang=en", 0) // (it records the page's errors)
	js := func(expr string) string {
		t.Helper()
		return get(t, pg, 10*time.Second, chromedp.Evaluate[string](expr, chromedp.EvalAwaitPromise))
	}
	is := func(expr string) bool { t.Helper(); return js(`String(!!(`+expr+`))`) == "true" }
	row := fmt.Sprintf(`#people .row[data-account=%q]`, alice.ID)
	waitUntil(t, 20*time.Second, "the roster lists alice", func() bool { return is(`document.querySelector('` + row + `')`) })

	// ---------- the message log, kept as the bridge keeps it ----------
	trim := js(`(async () => {
		const { connect } = await import('/symphony.js')
		const sym = connect({ stream: false })
		const msg = (i, task) => ({ id: 'm' + String(i).padStart(4, '0'), task, text: 'x', fromName: 'a', toName: 'b', time: new Date().toISOString() })
		const counts = () => { const m = sym.state.messages, by = k => m.filter(x => (x.task || '') === k).length
			return { n: m.length, t1: by('T1'), t2: by('T2'), rest: by(''), first: m[0]?.id, sorted: m.every((x, i) => !i || m[i - 1].id < x.id) } }
		const list = []
		for (let i = 0; i < 200; i++) list.push(msg(i, 'T1'))
		for (let i = 200; i < 400; i++) list.push(msg(i, 'T2'))
		for (let i = 400; i < 450; i++) list.push(msg(i, ''))
		const snap = tasks => ({ type: 'snapshot', protocol: 2, accounts: [], sessions: [], asks: [], launches: [], ledger: [], tasks: tasks.map(id => ({ id, members: [] })) })
		sym.take({ ...snap(['T1', 'T2']), messages: list })
		for (let i = 450; i < 460; i++) sym.take({ type: 'message', message: msg(i, '') })
		const a = counts()
		for (let i = 460; i < 760; i++) sym.take({ type: 'message', message: msg(i, '') })
		const b = counts()
		sym.take({ ...snap(['T2']), messages: null }) // T1 ended: its messages are among the rest now
		sym.take({ type: 'message', message: msg(760, '') })
		return JSON.stringify([a, b, counts()])
	})()`)
	var got []struct {
		N, T1, T2, Rest int
		First           string
		Sorted          bool
	}
	if err := json.Unmarshal([]byte(trim), &got); err != nil {
		t.Fatalf("%v: %s", err, trim)
	}
	if a := got[0]; a.N != 460 || a.T1 != 200 || a.First != "m0000" || !a.Sorted {
		t.Fatalf("two tasks of 200 messages and 60 others should all stay: %+v", a)
	}
	if b := got[1]; b.T1 != 200 || b.T2 != 200 || b.Rest != 300 || b.First != "m0000" || !b.Sorted {
		t.Fatalf("the rest should keep its newest 300, each task shown its own: %+v", b)
	}
	if c := got[2]; c.T1 != 0 || c.T2 != 200 || c.Rest != 300 || !c.Sorted {
		t.Fatalf("an ended task's messages should go with the rest: %+v", c)
	}

	// ---------- the bridge unreachable, then back ----------
	do(t, pg, 10*time.Second, chromedp.Click(chromedp.CSS(row)))
	waitUntil(t, 10*time.Second, "alice's window shows her first tool call", func() bool {
		return is(`!document.getElementById('chat').hidden && document.querySelectorAll('#chat .flow .it.tool').length === 1`)
	})
	if is(`!document.getElementById('conn').hidden`) {
		t.Fatal("the connection banner shows while the bridge is there")
	}
	b.cutPages(true)
	waitUntil(t, 20*time.Second, "the connection banner shows", func() bool { return is(`!document.getElementById('conn').hidden`) })
	if r := js(`import('/symphony.js').then(m => m.request('GET', '/api/state')).then(r => JSON.stringify([r.ok, r.status, r.data && r.data.code]))`); r != `[false,0,"unreachable"]` {
		t.Fatalf("a call the bridge doesn't answer should say it's unreachable: %s", r)
	}
	tool("u2", "Grep") // missed by the page
	b.cutPages(false)
	waitUntil(t, 30*time.Second, "the banner goes, and the window fetches the log again with the call it missed", func() bool {
		return is(`document.getElementById('conn').hidden && document.querySelectorAll('#chat .flow .it.tool').length === 2`)
	})
	js(`document.querySelector('#chat .close').click(); ''`) // (Coworkers shows the window's close button only in one column)

	// ---------- hidden for a while: no stream, and caught up when shown ----------
	js(`Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'hidden' }); document.dispatchEvent(new Event('visibilitychange')); ''`)
	time.Sleep(11 * time.Second)
	event(map[string]any{"kind": "tool.start", "tool": "Glob", "toolUseId": "u3", "summary": "*.go"})
	time.Sleep(2 * time.Second)
	if is(`document.querySelector('` + row + `').textContent.includes('Glob')`) {
		t.Fatal("a page hidden for more than 10 seconds should have let its stream go")
	}
	js(`delete document.visibilityState; document.dispatchEvent(new Event('visibilitychange')); ''`)
	waitUntil(t, 15*time.Second, "shown again, the page catches up", func() bool {
		return is(`document.querySelector('` + row + `').textContent.includes('Glob')`)
	})

	// ---------- a row redrawn under a press still takes the click ----------
	var box struct{ X, Y float64 }
	if err := json.Unmarshal([]byte(js(`(r => JSON.stringify({ X: r.left + r.width / 2, Y: r.top + r.height / 2 }))(document.querySelector('`+row+`').getBoundingClientRect())`)), &box); err != nil {
		t.Fatal(err)
	}
	do(t, pg, 10*time.Second, chromedp.MouseEvent(chromedp.MouseMoved, box.X, box.Y), chromedp.MouseEvent(chromedp.MousePressed, box.X, box.Y, chromedp.ButtonLeft, chromedp.ClickCount(1)))
	for i, name := range []string{"Edit", "Write"} { // the row's state changes twice while it is held
		event(map[string]any{"kind": "tool.start", "tool": name, "toolUseId": fmt.Sprintf("p%d", i), "summary": name})
		time.Sleep(80 * time.Millisecond) // (the press stays well under the page's 500 ms hold)
	}
	do(t, pg, 10*time.Second, chromedp.MouseEvent(chromedp.MouseReleased, box.X, box.Y, chromedp.ButtonLeft, chromedp.ClickCount(1)))
	waitUntil(t, 10*time.Second, "the press opened alice's window, and the roster caught up after it", func() bool {
		return is(`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === 'alice' && document.querySelector('` + row + `').textContent.includes('Write')`)
	})
	if missing := get(t, pg, 5*time.Second, chromedp.Evaluate[[]string](`window.__i18nMissing || []`)); len(missing) > 0 {
		t.Fatalf("keys the catalog lacks: %v", missing)
	}
	// (a call that got no answer is no error of the page's)
	if errs := get(t, pg, 5*time.Second, chromedp.Evaluate[[]string](`window.__errors`)); len(errs) > 0 {
		t.Fatalf("the page threw: %v", errs)
	}
}

// TestPageKeyChanged: a bridge that started again with a new page key (its key file deleted) refuses an open page's old
// one: the page says so at once (not "can't reach the bridge") instead of reconnecting forever in silence, and once the
// browser is signed in with the new key (the office opened again from the symphony program, in another tab), it comes
// back by itself.
func TestPageKeyChanged(t *testing.T) {
	requireBrowser(t)
	b := startBridge(t, "")
	alice, err := b.Hub.CreateAccount("alice")
	if err != nil {
		t.Fatal(err)
	}
	b.post(t, alice.Token, "/api/event", map[string]any{"session": "s-alice", "kind": "session.start", "cwd": "/work/alice"})
	browser := newBrowser(t)
	tab, _ := clockTab(t, browser, b.URL+"/t/messenger/?lang=en", 0)
	js := func(expr string) string {
		t.Helper()
		return get(t, tab, 10*time.Second, chromedp.Evaluate[string](expr, chromedp.EvalAwaitPromise))
	}
	is := func(expr string) bool { t.Helper(); return js(`String(!!(`+expr+`))`) == "true" }
	waitUntil(t, 20*time.Second, "the roster lists alice", func() bool { return is(`document.querySelector('#people .row[data-account]')`) })
	banner := `(e => e && !e.hidden && e.textContent)(document.getElementById('conn')) || ''`

	key := b.rekey(t)
	want := "The bridge's key changed (it made a new one), so this page is signed out: open the office again by running symphony"
	waitUntil(t, 15*time.Second, "the page says its key is no longer taken", func() bool { return js(banner) == want },
		func() string { return js(banner) })

	// signed in again in another tab, as the symphony program does (open.html, then ?key=): the page comes back
	other, cancel := chromedp.NewContext(browser)
	defer cancel()
	do(t, other, 30*time.Second, chromedp.Navigate(b.URL+"/api/themes?key="+key))
	waitUntil(t, 30*time.Second, "the banner goes once the browser has the new key", func() bool { return js(banner) == "" },
		func() string { return js(banner) })
	if errs := js(`JSON.stringify(window.__errors || [])`); errs != "[]" {
		t.Fatalf("page errors: %s", errs)
	}
}
