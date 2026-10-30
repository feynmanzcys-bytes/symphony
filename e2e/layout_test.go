//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// TestLayout checks the office's layout with made-up sessions (no tokens). In windows of a few sizes the building
// fills the whole window with the open office in the middle, and the room left over goes to small meeting rooms for 3 to
// 5, on the office's left (in a tall window some go below the lounge too), whose nameplates show; the HUD sits at the
// top in the middle, its tab slides it up out of the way (showing the pending asks while it's up) and back down, the
// page remembers which, and the side panels start below it. Each finer step of the pixel size draws the same building in
// finer pixels and more detail, and the page remembers the step.
func TestLayout(t *testing.T) {
	requireBrowser(t)
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))
	alice, _ := b.Hub.CreateAccount("alice")
	bob, _ := b.Hub.CreateAccount("bob")
	b.post(t, alice.Token, "/api/event", map[string]any{"session": "s-alice", "kind": "session.start", "cwd": "/work/alice", "model": "opus"})
	b.post(t, bob.Token, "/api/event", map[string]any{"session": "s-bob", "kind": "session.start", "cwd": "/work/bob", "model": "sonnet"})
	b.post(t, alice.Token, "/api/ask", map[string]any{"session": "s-alice", "kind": "permission", "tool": "Bash", "input": map[string]any{"command": "ls"}})

	browser := newBrowser(t)
	office, _ := clockTab(t, browser, b.URL+"/?lang=en", 0)
	later := func() { // the pending popup opens when the page does
		waitUntil(t, 20*time.Second, "the office opens the pending popup", func() bool {
			return get(t, office, 5*time.Second, chromedp.Evaluate[bool](`document.querySelectorAll('.emp-label').length === 2 && !document.getElementById('asks-modal').hidden`))
		})
		do(t, office, 10*time.Second, chromedp.Click(chromedp.CSS(`#asks-later`)), chromedp.WaitNotVisible(chromedp.CSS(`#asks-modal`)))
	}
	later()

	type view struct {
		W, H    float64  // the window
		Painted bool     // the canvas covers the window, and the building every edge of it
		Plates  []string // the small meeting rooms' nameplates
		Inside  bool     // the nameplates are all inside the window, none on another
		Hover   bool     // the nameplates take the mouse, so their tooltips show
		Left    int      // nameplates left of the open office
		Right   int      // nameplates right of it
		Seats   []int    // each small meeting room's seats
		Mid     float64  // the middle of the open office, across
		Hud     struct{ Left, Top, Width, Bottom float64 }
		Tab     struct{ Top, Bottom float64 }
		Badge   string // the number on the tab, when it shows
		Up      bool   // the HUD is up
	}
	look := func() view {
		var v view
		data := get(t, office, 5*time.Second, chromedp.Evaluate[string](`JSON.stringify((() => {
			const cv = document.getElementById('office'), dpr = devicePixelRatio || 1, g = cv.getContext('2d')
			const sized = cv.width === Math.round(innerWidth * dpr) && cv.height === Math.round(innerHeight * dpr)
			const at = (x, y) => [...g.getImageData(Math.min(cv.width - 1, x | 0), Math.min(cv.height - 1, y | 0), 1, 1).data]
			const edges = [[0, 0], [cv.width / 2, 0], [cv.width, 0], [0, cv.height / 2], [cv.width, cv.height / 2], [0, cv.height], [cv.width / 2, cv.height], [cv.width, cv.height]]
			const bare = edges.some(([x, y]) => { const [r, gg, b] = at(x, y); return r === 47 && gg === 46 && b === 43 }) // the page's own background
			const plates = [...document.querySelectorAll('.room-plate')], boxes = plates.map(p => p.getBoundingClientRect())
			const inside = boxes.every(r => r.width > 0 && r.left >= 0 && r.top >= 0 && r.right <= innerWidth && r.bottom <= innerHeight) &&
				boxes.every((a, i) => boxes.every((b, j) => i === j || a.right <= b.left || b.right <= a.left || a.bottom <= b.top || b.bottom <= a.top))
			const hover = plates.every(p => getComputedStyle(p).pointerEvents !== 'none' && p.title !== '')
			const [ol, or] = (cv.dataset.office || '0,0').split(',').map(Number)
			const hud = document.getElementById('hud'), h = hud.getBoundingClientRect(), tab = document.getElementById('hud-tab').getBoundingClientRect()
			const badge = document.querySelector('#hud-tab .badge')
			return { W:innerWidth, H:innerHeight, Painted:sized && !bare, Plates:plates.map(p => p.textContent), Inside:inside, Hover:hover,
				Left:boxes.filter(r => r.right <= ol).length, Right:boxes.filter(r => r.left >= or).length, Seats:plates.map(p => Number(p.dataset.seats)), Mid:(ol + or) / 2,
				Hud:{ Left:h.left, Top:h.top, Width:h.width, Bottom:h.bottom }, Tab:{ Top:tab.top, Bottom:tab.bottom },
				Badge:getComputedStyle(badge).display === 'none' ? '' : badge.textContent, Up:hud.classList.contains('up') }
		})())`))
		if err := json.Unmarshal([]byte(data), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	show := func() string { data, _ := json.MarshalIndent(look(), "", " "); return string(data) }
	centered := func(v view) bool { return math.Abs(v.Hud.Left+v.Hud.Width/2-v.W/2) <= 1 }

	// ---------- 1. the building fills windows of every shape, and wider ones get more small meeting rooms ----------
	rooms := map[string]int{}
	for _, size := range []struct {
		w, h  int64
		scale float64
	}{{1400, 860, 1}, {1000, 1300, 1}, {2400, 1000, 1}, {820, 620, 1}, {1280, 800, 2}} {
		name := fmt.Sprintf("%dx%d@%gx", size.w, size.h, size.scale)
		do(t, office, 10*time.Second, chromedp.EmulateViewport(size.w, size.h, chromedp.EmulateScale(size.scale)))
		waitUntil(t, 10*time.Second, name+": the building fills the window, the nameplates show and the HUD is at the top in the middle", func() bool {
			v := look()
			return v.W == float64(size.w) && v.Painted && len(v.Plates) >= 2 && v.Inside && v.Hover && centered(v) && v.Hud.Top == 0 && !v.Up
		}, show)
		v := look()
		rooms[name] = len(v.Plates)
		if math.Abs(v.Mid-v.W/2) > 3 {
			t.Errorf("%s: the open office should be in the middle of the window, its middle is at %.1f of %.0f", name, v.Mid, v.W)
		}
		if v.Left < v.Right || (size.w > size.h && v.Right > 0) {
			t.Errorf("%s: the small meeting rooms belong on the office's left (only a tall window has some right of it): %d left, %d right", name, v.Left, v.Right)
		}
		kinds := map[int]bool{}
		for _, n := range v.Seats {
			if n < 3 || n > 5 {
				t.Errorf("%s: a small meeting room has %d seats, not 3 to 5: %v", name, n, v.Seats)
			}
			kinds[n] = true
		}
		if len(v.Seats) >= 4 && len(kinds) < 2 {
			t.Errorf("%s: the small meeting rooms should differ in seats: %v", name, v.Seats)
		}
		screenshot(t, office, "layout-"+name+".png")
	}
	if rooms["2400x1000@1x"] <= rooms["1400x860@1x"] {
		t.Errorf("a wider window should have more small meeting rooms: %v", rooms)
	}
	do(t, office, 10*time.Second, chromedp.EmulateViewport(1400, 860))

	// ---------- 2. the tab slides the HUD up, and then shows the pending asks ----------
	do(t, office, 10*time.Second, chromedp.Click(chromedp.CSS(`#hud-tab`)))
	waitUntil(t, 10*time.Second, "the HUD slides up out of the window, its tab stays, with the pending ask on it", func() bool {
		v := look()
		return v.Up && v.Hud.Bottom <= 0.5 && v.Tab.Top <= 0 && v.Tab.Bottom >= 12 && v.Badge == "1"
	}, show)
	screenshot(t, office, "layout-hud-up.png")

	// ---------- 3. the page remembers it; "/" brings the HUD down to the search box ----------
	do(t, office, 30*time.Second, chromedp.Reload())
	later()
	if v := look(); !v.Up || v.Hud.Bottom > 0.5 {
		t.Fatalf("after a reload the HUD should still be up:\n%s", show())
	}
	do(t, office, 10*time.Second, chromedp.KeyEvent("/"))
	waitUntil(t, 10*time.Second, `"/" brings the HUD down and focuses the search box`, func() bool {
		v := look()
		return !v.Up && v.Hud.Top == 0 && v.Badge == "" &&
			get(t, office, 5*time.Second, chromedp.Evaluate[bool](`document.activeElement === document.getElementById('find')`))
	}, show)
	do(t, office, 10*time.Second, chromedp.SendKeys(chromedp.CSS(`#find`), kb.Escape))

	// ---------- 4. a card opens below the HUD; the tab brings the HUD back down after it was put up ----------
	clickLabel(t, office, b.account("bob").ID)
	do(t, office, 10*time.Second, chromedp.WaitVisible(chromedp.CSS(`#card`)))
	waitUntil(t, 10*time.Second, "the card starts below the HUD", func() bool {
		return get(t, office, 5*time.Second, chromedp.Evaluate[bool](
			`document.getElementById('card').getBoundingClientRect().top >= document.getElementById('hud').getBoundingClientRect().bottom`))
	}, show)
	do(t, office, 10*time.Second, chromedp.Click(chromedp.CSS(`#card .close`)), chromedp.Click(chromedp.CSS(`#hud-tab`)))
	waitUntil(t, 10*time.Second, "the HUD is up again", func() bool { return look().Hud.Bottom <= 0.5 }, show)
	do(t, office, 10*time.Second, chromedp.Click(chromedp.CSS(`#hud-tab`)))
	waitUntil(t, 10*time.Second, "the tab brings the HUD back down", func() bool { v := look(); return !v.Up && v.Hud.Top == 0 }, show)

	// ---------- 5. the pixel size: the same building, drawn finer at each step; the page remembers the step ----------
	type drawing struct {
		Detail int    `json:"detail"` // the drawing's pixels to a pixel of the office
		Office string `json:"office"` // where the open office is
		Rooms  int    `json:"rooms"`  // how many small meeting rooms there are
		On     int    `json:"on"`     // the step that's pressed
	}
	drawn := func() drawing {
		var d drawing
		data := get(t, office, 5*time.Second, chromedp.Evaluate[string](`JSON.stringify({ detail:Number(document.getElementById('office').dataset.detail),
			office:document.getElementById('office').dataset.office, rooms:document.querySelectorAll('.room-plate').length,
			on:[...document.querySelectorAll('#hud .steps button')].findIndex(b => b.getAttribute('aria-pressed') === 'true') })`))
		if err := json.Unmarshal([]byte(data), &d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	step := func(level int) {
		do(t, office, 10*time.Second, chromedp.Click(chromedp.CSS(fmt.Sprintf(`#hud .steps button[data-level="%d"]`, level))))
	}
	base := drawn()
	if base.Detail != 1 || base.On != 0 {
		t.Fatalf("the office should start at the standard pixel size: %+v", base)
	}
	last := base.Detail
	for level := 1; level < 4; level++ {
		step(level)
		waitUntil(t, 10*time.Second, fmt.Sprintf("pixel size step %d: the same building, drawn at least as fine, the window still filled", level+1), func() bool {
			d := drawn()
			return d.On == level && d.Detail >= 2 && d.Detail >= last && d.Office == base.Office && d.Rooms == base.Rooms && look().Painted
		}, func() string { return fmt.Sprintf("%+v (at the standard size %+v)", drawn(), base) })
		last = drawn().Detail
		screenshot(t, office, fmt.Sprintf("layout-pixels-%d.png", level+1))
	}
	// on a screen with twice the pixels, the finer steps draw three to a pixel of the office (the scale there is 3)
	do(t, office, 10*time.Second, chromedp.EmulateViewport(1280, 800, chromedp.EmulateScale(2)))
	waitUntil(t, 10*time.Second, "at 2x the finest step draws as many pixels as the screen has", func() bool {
		return drawn().Detail == 3 && look().Painted
	}, func() string { return fmt.Sprintf("%+v", drawn()) })
	screenshot(t, office, "layout-pixels-4@2x.png")
	do(t, office, 10*time.Second, chromedp.EmulateViewport(1400, 860))
	do(t, office, 30*time.Second, chromedp.Reload())
	later()
	if d := drawn(); d.On != 3 || d.Detail != last {
		t.Fatalf("after a reload the pixel size should stay at the finest step: %+v", d)
	}
	step(0)
	waitUntil(t, 10*time.Second, "back to the standard pixel size", func() bool { d := drawn(); return d.On == 0 && d.Detail == 1 })

	if miss := get(t, office, 5*time.Second, chromedp.Evaluate[[]string](`window.__i18nMissing`)); len(miss) > 0 {
		t.Errorf("the office asked for keys its catalog lacks: %v", miss)
	}
	if errs := get(t, office, 5*time.Second, chromedp.Evaluate[[]string](`window.__errors`)); len(errs) > 0 {
		t.Errorf("the page threw: %v", errs)
	}
}
