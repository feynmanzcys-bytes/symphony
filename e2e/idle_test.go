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

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// TestIdle walks through the office's idle life with made-up sessions (no tokens): idle employees stay at their
// desks at first, then wander off, each to a spot of its own; a click calls one back to its desk, where it stays;
// the search box finds employees; after a long while they nap; work sends a napper back to its desk, and off duty
// sends one out of the door. Instead of waiting minutes, the test moves the page's clock forward.
func TestIdle(t *testing.T) {
	requireBrowser(t)
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge")) // development mode: no installed mod needed
	tokens := map[string]string{}
	for i, name := range []string{"alice", "bob", "carol", "dave", "erin", "frank", "gina"} {
		acc, err := b.Hub.CreateAccount(name)
		if err != nil {
			t.Fatal(err)
		}
		tokens[name] = acc.Token
		if name == "gina" {
			break // off duty
		}
		session := "s-" + name
		b.post(t, acc.Token, "/api/event", map[string]any{"session": session, "kind": "session.start", "cwd": "/work/proj-" + name, "model": []string{"haiku", "sonnet", "opus", "fable"}[i%4]})
		b.post(t, acc.Token, "/api/event", map[string]any{"session": session, "kind": "session.title", "title": name})
	}
	b.post(t, tokens["alice"], "/api/event", map[string]any{"session": "s-alice", "kind": "tool.start", "tool": "Bash"})
	idle := []string{"bob", "carol", "dave", "erin", "frank"}

	browser := newBrowser(t)
	office, _ := clockTab(t, browser, b.URL+"/?lang=en", 0)
	type tag struct{ Act, Room, Spot, Moving, State, Classes string }
	tags := func() map[string]tag {
		var raw map[string]tag
		data := get(t, office, 5*time.Second, chromedp.Evaluate[string](`JSON.stringify(Object.fromEntries([...document.querySelectorAll('.emp-label')].map(l =>
			[l.querySelector('.nm').textContent, { Act:l.dataset.act, Room:l.dataset.room, Spot:l.dataset.spot, Moving:l.dataset.moving, State:l.dataset.state, Classes:l.className }])))`))
		if err := json.Unmarshal([]byte(data), &raw); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	show := func() string { data, _ := json.MarshalIndent(tags(), "", " "); return string(data) }
	// how long the idle sessions have really been idle, so the page's clock can be set to a moment in their day
	idleFor := func() time.Duration {
		for _, s := range b.Hub.Snapshot().Sessions {
			if s.Title == "bob" {
				return time.Since(s.StateSince)
			}
		}
		t.Fatal("bob has no session")
		return 0
	}
	var lastWarp int64
	warp := func(to time.Duration) {
		ms := (to - idleFor()).Milliseconds()
		get(t, office, 5*time.Second, chromedp.Evaluate[bool](fmt.Sprintf(`(window.__warp = %d, true)`, ms)))
		lastWarp = ms
	}
	// nobody on the way somewhere (a game of tag keeps its players running)
	settled := func(names ...string) bool {
		all := tags()
		for _, n := range names {
			if all[n].Moving != "0" && all[n].Act != "chase" {
				return false
			}
		}
		return true
	}
	distinct := func(names []string) {
		t.Helper()
		all, seen := tags(), map[string]string{}
		for _, n := range names {
			s := all[n].Spot
			if s == "desk" || s == "" {
				continue
			}
			if other, ok := seen[s]; ok {
				t.Fatalf("%s and %s share the spot %s:\n%s", other, n, s, show())
			}
			seen[s] = n
		}
	}

	// ---------- 1. just idle: everyone stays at the desk ----------
	waitUntil(t, 20*time.Second, "the five idle employees sit at their desks, alice works", func() bool {
		all := tags()
		for _, n := range idle {
			if all[n].Act != "desk" || all[n].Room != "desk" || all[n].Moving != "0" {
				return false
			}
		}
		return all["alice"].Act == "work" && all["alice"].Room == "desk" && all["gina"].Room == ""
	}, show)

	// the walkways: from every spot, the way to every desk is little longer than going straight across and then
	// down (desks in a middle row once had to be reached round by the back wall or the front)
	type route struct {
		From string  `json:"from"`
		Seat int     `json:"seat"`
		Walk float64 `json:"walk"`
		Dx   float64 `json:"dx"`
		Dy   float64 `json:"dy"`
	}
	routes := get(t, office, 5*time.Second, chromedp.Evaluate[[]route](`window.__routes()`))
	if len(routes) < 100 {
		t.Fatalf("too few routes to check: %d", len(routes))
	}
	for _, r := range routes {
		if over := r.Walk - r.Dx - r.Dy; over > 150 {
			t.Errorf("the way from %s to desk %d is %.0f px, %.0f more than across and down", r.From, r.Seat+1, r.Walk, over)
		}
	}

	// ---------- 2. a minute later they wander off, each to a spot of its own ----------
	warp(time.Minute)
	waitUntil(t, 40*time.Second, "most idle employees leave their desks and get where they're going", func() bool {
		all, away := tags(), 0
		for _, n := range idle {
			if all[n].Room != "desk" && all[n].Room != "" {
				away++
			}
		}
		return away >= 2 && settled(idle...)
	}, show)
	distinct(idle)
	if all := tags(); all["alice"].Room != "desk" || all["alice"].Act != "work" {
		t.Fatalf("alice is working and must stay at her desk:\n%s", show())
	}
	screenshot(t, office, "idle-wander.png")

	// the idle chip in the HUD lights up the idle ones and dims alice, who works; a second click lets go
	do(t, office, 10*time.Second, chromedp.Click(chromedp.CSS(`#stats .chip[data-group=idle]`)))
	waitUntil(t, 10*time.Second, "the idle chip lights up the idle employees", func() bool {
		all := tags()
		for _, n := range idle {
			if !strings.Contains(all[n].Classes, "match") {
				return false
			}
		}
		return strings.Contains(all["alice"].Classes, "dim") && strings.Contains(all["gina"].Classes, "dim")
	}, show)
	do(t, office, 10*time.Second, chromedp.Click(chromedp.CSS(`#stats .chip[data-group=idle]`)))
	waitUntil(t, 10*time.Second, "a second click on the chip lights everyone again", func() bool {
		return !strings.Contains(tags()["alice"].Classes, "dim")
	}, show)

	// ---------- 3. a click calls bob back: his card opens, he walks to his desk and stays there ----------
	clickLabel(t, office, b.account("bob").ID)
	do(t, office, 10*time.Second, chromedp.WaitVisible(chromedp.CSS(`#card`)))
	if got := get(t, office, 5*time.Second, chromedp.Evaluate[string](`document.querySelector('#card h2').textContent`)); got != "bob" {
		t.Fatalf("clicking bob should open his card, got %q", got)
	}
	waitUntil(t, 30*time.Second, "bob is back at his desk, ready for work", func() bool {
		b := tags()["bob"]
		return b.Act == "ready" && b.Room == "desk" && b.Moving == "0"
	}, show)
	if line := get(t, office, 5*time.Second, chromedp.Evaluate[string](`document.querySelector('#card dd.state-line').textContent`)); !strings.Contains(line, "ready for work") {
		t.Fatalf("bob's card should say he's ready for work: %q", line)
	}
	do(t, office, 5*time.Second, chromedp.Click(chromedp.CSS(`#card .close`)))

	// ---------- 4. the search box: by folder, matches light up, Enter picks ----------
	do(t, office, 10*time.Second, chromedp.SendKeys(chromedp.CSS(`#find`), "proj-car"))
	waitUntil(t, 10*time.Second, "searching a folder lists carol alone and dims the others", func() bool {
		all := tags()
		hits := get(t, office, 5*time.Second, chromedp.Evaluate[[]string](`[...document.querySelectorAll('#find-list:not([hidden]) .hit b')].map(b => b.textContent)`))
		return len(hits) == 1 && hits[0] == "carol" && strings.Contains(all["carol"].Classes, "match") && strings.Contains(all["dave"].Classes, "dim")
	}, show)
	screenshot(t, office, "idle-search.png")
	do(t, office, 10*time.Second, chromedp.SendKeys(chromedp.CSS(`#find`), kb.Enter), chromedp.WaitVisible(chromedp.CSS(`#card`)))
	if got := get(t, office, 5*time.Second, chromedp.Evaluate[string](`document.querySelector('#card h2').textContent`)); got != "carol" {
		t.Fatalf("Enter in the search box should open carol's card, got %q", got)
	}
	waitUntil(t, 30*time.Second, "picking carol calls her back too, and the office lights up again", func() bool {
		all := tags()
		return all["carol"].Act == "ready" && all["carol"].Room == "desk" && !strings.Contains(all["dave"].Classes, "dim") &&
			get(t, office, 5*time.Second, chromedp.Evaluate[bool](`document.getElementById('find').value === '' && document.getElementById('find-list').hidden`))
	}, show)
	do(t, office, 5*time.Second, chromedp.Click(chromedp.CSS(`#card .close`)))

	// ---------- 5. a long while later the others nap, bob and carol stay at their desks ----------
	warp(510 * time.Second) // in the second nap of everyone's rhythm, whatever its offset
	napping := []string{"dave", "erin", "frank"}
	waitUntil(t, 40*time.Second, "dave, erin and frank nap, and get where they nap", func() bool {
		all := tags()
		for _, n := range napping {
			if all[n].Act != "nap" {
				return false
			}
		}
		return settled(idle...)
	}, show)
	distinct(napping)
	for _, n := range []string{"bob", "carol"} {
		if tag := tags()[n]; tag.Room != "desk" || tag.Act != "ready" {
			t.Fatalf("%s was called back and must stay at the desk until the next turn:\n%s", n, show())
		}
	}
	screenshot(t, office, "idle-nap.png")
	// a page opened now puts everyone straight where they are, nobody walks there from the desk
	fresh, closeFresh := clockTab(t, browser, b.URL+"/?lang=en", lastWarp)
	var first string
	waitUntil(t, 20*time.Second, "a new page fills in the name tags", func() bool {
		first = get(t, fresh, 5*time.Second, chromedp.Evaluate[string](`[...document.querySelectorAll('.emp-label')].map(l => l.dataset.state === 'ended' ? '0' : l.dataset.act ? l.dataset.moving : '?').join('')`))
		return len(first) == 7 && !strings.Contains(first, "?")
	})
	if first != "0000000" {
		t.Fatalf("on a new page everyone should start where the plan puts them, not walk there: moving flags %q", first)
	}
	closeFresh()

	// ---------- 6. work wakes dave: back to his desk ----------
	b.post(t, tokens["dave"], "/api/event", map[string]any{"session": "s-dave", "kind": "tool.start", "tool": "Read"})
	waitUntil(t, 30*time.Second, "dave hurries back to his desk to work", func() bool {
		d := tags()["dave"]
		return d.Act == "work" && d.Room == "desk" && d.Moving == "0" && d.State == "tool"
	}, show)

	// ---------- 7. off duty: erin walks out of the door ----------
	b.post(t, tokens["erin"], "/api/event", map[string]any{"session": "s-erin", "kind": "session.end"})
	waitUntil(t, 40*time.Second, "erin walks out, and her tag goes back to her desk", func() bool {
		e := tags()["erin"]
		return e.State == "ended" && e.Room == "" && strings.Contains(e.Classes, "off")
	}, show)

	if miss := get(t, office, 5*time.Second, chromedp.Evaluate[[]string](`window.__i18nMissing`)); len(miss) > 0 {
		t.Errorf("the office asked for keys its catalog lacks: %v", miss)
	}
	if errs := get(t, office, 5*time.Second, chromedp.Evaluate[[]string](`window.__errors`)); len(errs) > 0 {
		t.Errorf("the page threw: %v", errs)
	}
}

// clockTab opens url in a new tab whose clock runs window.__warp milliseconds ahead (warp at first), and which
// records uncaught errors in window.__errors; cancel closes it.
func clockTab(t *testing.T, browser context.Context, url string, warp int64) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := chromedp.NewContext(browser)
	t.Cleanup(cancel)
	if err := chromedp.Do(ctx); err != nil { // creates the tab
		t.Fatalf("opening a tab failed: %v", err)
	}
	script := fmt.Sprintf(`window.__warp = %d`, warp) + `
		window.__errors = []
		addEventListener('error', e => window.__errors.push(String(e.message)))
		addEventListener('unhandledrejection', e => window.__errors.push(String(e.reason)))
		Date.now = (now => () => now() + window.__warp)(Date.now.bind(Date))`
	do(t, ctx, 30*time.Second, chromedp.Func(func(ctx context.Context, tg *chromedp.Target) error {
		_, err := cdp.Call(ctx, tg, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: script})
		return err
	}), chromedp.Navigate(url))
	return ctx, cancel
}
