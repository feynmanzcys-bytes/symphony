//go:build e2e

package e2e

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// TestPerf (no tokens) drives the page hard with made-up streaming: one busy session with its window open, then
// many sessions streaming at once with no window. It fails only on a page error or a frozen frame; the rest is
// logged for a look.
func TestPerf(t *testing.T) {
	requireBrowser(t)
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))
	browser := newBrowser(t)
	office, _ := clockTab(t, browser, b.URL+"/?lang=en", 0)
	js := func(e string) string { return get(t, office, 10*time.Second, chromedp.Evaluate[string](e)) }
	jsb := func(e string) bool { return get(t, office, 10*time.Second, chromedp.Evaluate[bool](e)) }
	count := func(sel string) int {
		return int(get(t, office, 5*time.Second, chromedp.Evaluate[float64](fmt.Sprintf(`document.querySelectorAll(%q).length`, sel))))
	}
	watchFrames := func(ms int) {
		do(t, office, 5*time.Second, chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`window.__long = []; try { new PerformanceObserver(l => l.getEntries().forEach(e => window.__long.push(Math.round(e.duration)))).observe({ entryTypes: ['longtask'] }) } catch {}
			window.__gaps = []; (() => { let p = performance.now(); const s = p; const f = ts => { window.__gaps.push(Math.round(ts - p)); p = ts; if (ts - s < %d) requestAnimationFrame(f) }; requestAnimationFrame(f) })()`, ms)))
	}
	frames := func() (longTasks, worstGaps string) {
		return js(`JSON.stringify(window.__long.sort((a,b)=>b-a).slice(0,6))`), js(`JSON.stringify(window.__gaps.sort((a, b) => b - a).slice(0, 8))`)
	}
	piece := strings.Repeat("lorem ipsum dolor sit amet consectetur ", 30) // ~1.2 KiB

	// ---------- one busy session, window open: 12 turns, each 40 deltas of thinking+text, a tool ----------
	acc, _ := b.Hub.CreateAccount("load")
	b.post(t, acc.Token, "/api/event", map[string]any{"session": "s0", "kind": "session.start", "cwd": "/w", "model": "opus"})
	waitUntil(t, 10*time.Second, "the office shows it", func() bool { return count(`.emp-label`) == 1 })
	clickLabel(t, office, acc.ID)
	waitUntil(t, 10*time.Second, "its window is open", func() bool { return !jsb(`document.getElementById('chat').hidden`) })
	watchFrames(14000)
	start := time.Now()
	events := 0
	for turn := 0; turn < 12; turn++ {
		tid := fmt.Sprintf("t%d", turn)
		b.post(t, acc.Token, "/api/event", map[string]any{"session": "s0", "kind": "chat.user", "text": "prompt " + tid, "origin": "composer"})
		for i := 0; i < 40; i++ {
			b.post(t, acc.Token, "/api/event", map[string]any{"session": "s0", "kind": "chat.delta", "turnId": tid, "step": 0,
				"blocks": []map[string]any{{"i": 0, "kind": "thinking", "text": piece}, {"i": 1, "kind": "text", "text": piece}}})
			events++
		}
		b.post(t, acc.Token, "/api/event", map[string]any{"session": "s0", "kind": "chat.stop", "turnId": tid, "step": 0})
		b.post(t, acc.Token, "/api/event", map[string]any{"session": "s0", "kind": "tool.start", "tool": "Bash", "toolUseId": "tu" + tid, "summary": "go test ./..."})
		b.post(t, acc.Token, "/api/event", map[string]any{"session": "s0", "kind": "tool.end", "tool": "Bash", "toolUseId": "tu" + tid})
		events += 4
	}
	posted := time.Since(start)
	// a burst's tail used to wait in the browser for the stream's next bytes, the keepalive up to 15 s later; the bridge
	// writes a comment a moment after a burst now (streamNudge)
	waitUntil(t, 8*time.Second, "all items shown, the burst's tail too", func() bool { return count(`#chat .flow .it`) == 48 })
	shown := time.Since(start)
	time.Sleep(2 * time.Second)
	lt, wg := frames()
	t.Logf("ONE BUSY SESSION (window open): %d events in %v, 48 items shown after %v; page long tasks ms %s; worst frame gaps ms %s", events, posted, shown, lt, wg)
	chatBytes := get(t, office, 10*time.Second, chromedp.Evaluate[string](`fetch('/api/chat/s0').then(r => r.text()).then(t => String(t.length))`, chromedp.EvalAwaitPromise))
	t.Logf("  /api/chat/s0 is %s bytes for 48 items (each text block capped at 64 KiB)", chatBytes)
	if errs := get(t, office, 5*time.Second, chromedp.Evaluate[[]string](`window.__errors`)); len(errs) > 0 {
		t.Errorf("page threw during the busy session: %v", errs)
	}
	do(t, office, 5*time.Second, chromedp.Click(chromedp.CSS(`#chat .close`)))

	// ---------- 30 sessions streaming at once, no window open ----------
	toks := []string{}
	for i := 0; i < 30; i++ {
		a, err := b.Hub.CreateAccount(fmt.Sprintf("m%02d", i))
		if err != nil {
			t.Fatal(err)
		}
		toks = append(toks, a.Token)
		b.post(t, a.Token, "/api/event", map[string]any{"session": fmt.Sprintf("m%d", i), "kind": "session.start", "cwd": "/w"})
	}
	waitUntil(t, 20*time.Second, "31 employees", func() bool { return count(`.emp-label`) == 31 })
	watchFrames(10000)
	start = time.Now()
	for round := 0; round < 25; round++ {
		for i, tok := range toks {
			b.post(t, tok, "/api/event", map[string]any{"session": fmt.Sprintf("m%d", i), "kind": "chat.delta", "turnId": "t", "step": round,
				"blocks": []map[string]any{{"i": 0, "kind": "text", "text": piece}}})
			if round%6 == 0 {
				b.post(t, tok, "/api/event", map[string]any{"session": fmt.Sprintf("m%d", i), "kind": "tool.start", "tool": "Read", "toolUseId": fmt.Sprintf("r%d-%d", i, round)})
				b.post(t, tok, "/api/event", map[string]any{"session": fmt.Sprintf("m%d", i), "kind": "tool.end", "tool": "Read", "toolUseId": fmt.Sprintf("r%d-%d", i, round)})
			}
		}
	}
	t.Logf("30 SESSIONS AT ONCE (no window): ~%d events in %v", 30*25, time.Since(start))
	time.Sleep(3 * time.Second)
	lt, wg = frames()
	t.Logf("  page long tasks ms %s; worst frame gaps ms %s (30 fps = ~33 ms is one frame)", lt, wg)
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	t.Logf("  bridge test-process heap %d MiB, goroutines %d", ms.HeapAlloc>>20, runtime.NumGoroutine())
	// the office still redraws and the HUD counts are right
	if !jsb(`document.querySelectorAll('.emp-label').length === 31`) {
		t.Error("the office lost employees under load")
	}
	if errs := get(t, office, 5*time.Second, chromedp.Evaluate[[]string](`window.__errors`)); len(errs) > 0 {
		t.Errorf("page threw under many-session load: %v", errs)
	}
	// worst single frame should not be a multi-second freeze
	worst := get(t, office, 5*time.Second, chromedp.Evaluate[float64](`Math.max(0, ...window.__gaps)`))
	if worst > 2000 {
		t.Errorf("a frame froze for %d ms under load", int(worst))
	}
	screenshot(t, office, "perf-many.png")
}
