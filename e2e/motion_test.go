//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

// runningJS lists the infinite animations running on the page: what moves or blinks by itself for good.
const runningJS = `document.getAnimations().filter(a => a.playState === 'running' && a.effect && a.effect.getComputedTiming().iterations === Infinity).map(a => a.animationName || 'an animation').join(',')`

// lessMotion makes the tab's page ask for less motion (prefers-reduced-motion: reduce), or for no less.
func lessMotion(t *testing.T, ctx context.Context, on bool) {
	t.Helper()
	value := map[bool]string{true: "reduce", false: "no-preference"}[on]
	do(t, ctx, 10*time.Second, chromedp.Func(func(ctx context.Context, tg *chromedp.Target) error {
		_, err := cdp.Call(ctx, tg, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-reduced-motion", Value: value}}})
		return err
	}))
}

// TestMotion checks that what moves or blinks by itself can be stopped (WCAG 2.1 2.2.2), with the made-up office of
// a11yTeam and bob thinking (no tokens). In every theme something moves to begin with (the thinking dots in bob's
// window, the pending chip, the faces, the alarms); with the system asking for less motion (prefers-reduced-motion,
// emulated) nothing does, html has data-still and the page's switch says it's on and can't be turned off; asking for
// no less brings it all back. The switch stops it all too, is remembered after a reload and brings it back. The pixel
// office's drawing stands still as well: two frames a second and a half apart differ, and are the same with the
// switch on.
func TestMotion(t *testing.T) {
	b, _, tokens := a11yTeam(t)
	say := func(name, session string, body map[string]any) {
		t.Helper()
		body["session"] = session
		b.post(t, tokens[name], "/api/event", body)
	}
	say("bob", "s-bob", map[string]any{"kind": "turn.start", "text": "Look at the build"})
	say("bob", "s-bob", map[string]any{"kind": "chat.delta", "turnId": "b9", "step": 0, "blocks": []map[string]any{{"i": 0, "kind": "thinking", "text": "The build first."}}})
	browser := newBrowser(t)
	bob := b.account("bob").ID
	for _, th := range []struct{ name, ready, pre, row string }{ // the page ready, what to do first, bob's row that opens his window
		{"office", `document.querySelectorAll('.emp-label').length >= 5`, ``, `document.querySelector('#roster button[data-account="` + bob + `"]')`},
		{"messenger", `document.querySelectorAll('#people .row').length === 5`, ``, `[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('bob'))`},
	} {
		ctx, closeTab := clockTab(t, browser, b.URL+"/t/"+th.name+"/?lang=en", 0)
		do(t, ctx, 10*time.Second, chromedp.EmulateViewport(1400, 860))
		p := &screens{t: t, ctx: ctx, lang: "motion-" + th.name}
		moving := func() string { return p.js(runningJS) }
		still := func() bool { return p.is(`document.documentElement.hasAttribute('data-still')`) }
		show := func() {
			t.Helper()
			waitUntil(t, 20*time.Second, th.name+": the page shows the team", func() bool { return p.is(th.ready) })
			if th.pre != "" {
				p.run(th.pre)
			}
			waitUntil(t, 10*time.Second, th.name+": bob's row is there", func() bool { return p.is(`!!` + th.row) })
			p.run(th.row + `.click()`)
			waitUntil(t, 10*time.Second, th.name+": bob's window shows him thinking", func() bool { return p.is(`!!document.querySelector('#chat .it.thinking:not(.done) .dots')`) })
		}
		show()
		if moving() == "" || still() {
			t.Fatalf("%s: something should move by itself to begin with (or the checks below prove nothing): %q, still %v", th.name, moving(), still())
		}
		// the system asks for less motion, then for no less
		lessMotion(t, ctx, true)
		if !p.is(`matchMedia('(prefers-reduced-motion: reduce)').matches`) {
			t.Fatalf("%s: the page doesn't see the system asking for less motion", th.name)
		}
		waitUntil(t, 5*time.Second, th.name+": asked for less motion, nothing moves by itself", func() bool { return still() && moving() == "" }, func() string { return moving() })
		if !p.is(`(s => !!s && s.checked && s.disabled)(document.getElementById('set-still'))`) {
			t.Errorf("%s: with the system asking for less motion, the settings' switch should be on and held so", th.name)
		}
		lessMotion(t, ctx, false)
		waitUntil(t, 5*time.Second, th.name+": asked for no less, it moves again", func() bool { return !still() && moving() != "" })
		// the switch, in the settings: on, remembered, off
		p.run(`document.querySelector('.open-settings').click()`)
		if n := ax(t, ctx, `document.getElementById('set-still')`); n.Name != "Pause motion" || !p.is(`(s => !s.checked && !s.disabled)(document.getElementById('set-still'))`) {
			t.Errorf("%s: the switch should be named and off: %+v", th.name, n)
		}
		p.run(`document.getElementById('set-still').click(); document.querySelector('#settings .close').click()`)
		waitUntil(t, 5*time.Second, th.name+": the switch stops what moves", func() bool {
			return still() && moving() == "" && p.is(`document.getElementById('set-still').checked`)
		}, func() string { return moving() })
		do(t, ctx, 15*time.Second, chromedp.Reload())
		show()
		if !still() || moving() != "" {
			t.Errorf("%s: the switch should be remembered after a reload: still %v, moving %q", th.name, still(), moving())
		}
		if !p.is(`document.getElementById('set-still').checked`) {
			t.Errorf("%s: after a reload the settings' switch should still be on", th.name)
		}
		p.run(`document.getElementById('set-still').click()`)
		waitUntil(t, 5*time.Second, th.name+": the switch turned off, it moves again", func() bool { return !still() && moving() != "" })
		p.done()
		closeTab()
	}

	// the pixel office's drawing
	ctx, closeTab := clockTab(t, browser, b.URL+"/t/office/?lang=en", 0)
	defer closeTab()
	do(t, ctx, 10*time.Second, chromedp.EmulateViewport(1400, 860))
	p := &screens{t: t, ctx: ctx, lang: "motion-drawing"}
	waitUntil(t, 20*time.Second, "the office shows the team", func() bool { return p.is(`document.querySelectorAll('.emp-label').length >= 5`) })
	frames := func() (string, string) {
		p.run(`window.__frame = document.getElementById('office').toDataURL()`)
		time.Sleep(1500 * time.Millisecond)
		return p.js(`window.__frame`), p.js(`document.getElementById('office').toDataURL()`)
	}
	if a, z := frames(); a == z {
		t.Fatal("the office's drawing should move to begin with (or the check below proves nothing)")
	}
	p.run(`document.getElementById('set-still').click()`)
	time.Sleep(500 * time.Millisecond)
	if a, z := frames(); a != z {
		t.Error("with the switch on, the office's drawing should stand still")
	}
	p.run(`document.getElementById('set-still').click()`)
	p.done()
}
