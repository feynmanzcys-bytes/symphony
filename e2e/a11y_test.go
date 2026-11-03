//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// a11yTeam is the made-up office the accessibility tests look at (no tokens: claude is a script that sits there, and
// the sessions are reported in as the mod would): alice works (her chat has every kind of item), bob idles, carol waits
// for a permission and an answer, dave has never worked, erin went off duty; alice and bob are in a task, with messages
// between them and one from you. It gives the bridge, the task and the accounts' tokens by name.
func a11yTeam(t *testing.T) (*testBridge, *bridge.Task, map[string]string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	requireBrowser(t)
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))
	t.Cleanup(func() {
		for _, k := range b.Hub.Tasks() {
			_ = b.Hub.EndTask(k.ID)
		}
		b.Hub.Stop()
	})
	b.Launcher.Claude = sleeper(t) // claude sits there; the test reports its sessions in
	work := t.TempDir()
	dir := func(name string) string { return filepath.Join(work, name) }
	for _, d := range []string{"app", "web", "docs"} {
		if err := os.MkdirAll(dir(d), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir(d), "notes.txt"), []byte(d+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir("app"), "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	tokens := map[string]string{}
	event := func(name, session string, body map[string]any) {
		t.Helper()
		body["session"] = session
		b.post(t, tokens[name], "/api/event", body)
	}
	team := []struct{ name, dir, model, effort string }{{"alice", "app", "opus", "max"}, {"bob", "web", "sonnet", "high"}, {"carol", "docs", "haiku", "low"}, {"dave", "", "", ""}, {"erin", "docs", "sonnet", "medium"}}
	for _, m := range team {
		acc, err := b.Hub.CreateAccount(m.name)
		if err != nil {
			t.Fatal(err)
		}
		tokens[m.name] = acc.Token
		if m.dir == "" {
			continue
		}
		event(m.name, "s-"+m.name, map[string]any{"kind": "session.start", "cwd": dir(m.dir), "model": "claude-" + m.model + "-5-5", "effort": m.effort,
			"engine": map[string]any{"version": "2.1.316"}, "mod": "1.2.0", "surface": "terminal", "repo": map[string]any{"root": dir(m.dir), "name": m.dir}})
		event(m.name, "s-"+m.name, map[string]any{"kind": "session.title", "title": m.name + "-desk"})
	}
	// alice: a prompt, thinking and an answer, tool calls that went well, failed and were denied, a compaction, an API
	// error, the usage figures, a subagent; then she works
	event("alice", "s-alice", map[string]any{"kind": "chat.user", "text": "Run the test suite and fix what fails", "origin": "composer"})
	event("alice", "s-alice", map[string]any{"kind": "turn.start", "text": "Run the test suite and fix what fails"})
	event("alice", "s-alice", map[string]any{"kind": "chat.delta", "turnId": "a1", "step": 0, "blocks": []map[string]any{
		{"i": 0, "kind": "thinking", "text": "The suite is in `make test`; let me run it first."},
		{"i": 1, "kind": "text", "text": "I'll run the tests first:\n\n```\nmake test\n```\n\nThen I'll look at **each failure** in turn."}}})
	event("alice", "s-alice", map[string]any{"kind": "chat.stop", "turnId": "a1", "step": 0, "tokens": map[string]any{"input": 12000, "output": 800, "cacheRead": 90000, "cacheWrite": 0, "model": "claude-opus-5-5"}})
	for i, tool := range []struct {
		name, summary string
		end           map[string]any
	}{{"Read", "src/router.js", map[string]any{}}, {"Bash", "make lint", map[string]any{"isError": true}}, {"Bash", "rm -rf /tmp/cache", map[string]any{"denied": true}}} {
		id := fmt.Sprintf("tu%d", i)
		event("alice", "s-alice", map[string]any{"kind": "tool.start", "tool": tool.name, "summary": tool.summary, "toolUseId": id})
		end := map[string]any{"kind": "tool.end", "tool": tool.name, "summary": tool.summary, "toolUseId": id, "durationMs": 1500}
		for k, v := range tool.end {
			end[k] = v
		}
		event("alice", "s-alice", end)
	}
	event("alice", "s-alice", map[string]any{"kind": "session.compact"})
	event("alice", "s-alice", map[string]any{"kind": "turn.error", "text": "overloaded"})
	event("alice", "s-alice", map[string]any{"kind": "session.usage", "usage": map[string]any{"context": map[string]any{"tokens": 68000, "window": 200000, "percent": 34},
		"rateLimits": []map[string]any{{"kind": "five_hour", "percentUsed": 12}, {"kind": "seven_day", "percentUsed": 81}}, "cost": map[string]any{"usd": 1.25}}})
	event("alice", "s-alice", map[string]any{"kind": "agent.start", "agentId": "ag1", "agentType": "Explore", "description": "find the failing tests", "model": "claude-haiku-5-5"})
	event("alice", "s-alice", map[string]any{"kind": "tool.start", "tool": "Bash", "summary": "make test", "toolUseId": "tu9"})
	event("bob", "s-bob", map[string]any{"kind": "session.usage", "usage": map[string]any{"context": map[string]any{"tokens": 190000, "window": 200000, "percent": 95}, "cost": map[string]any{"usd": 0.4}}})
	event("bob", "s-bob", map[string]any{"kind": "chat.stop", "turnId": "b1", "step": 0, "tokens": map[string]any{"input": 3000, "output": 400, "cacheRead": 0, "cacheWrite": 0, "model": "claude-sonnet-5-5"}})
	event("erin", "s-erin", map[string]any{"kind": "session.end", "reason": "exit"})
	b.post(t, tokens["carol"], "/api/ask", map[string]any{"session": "s-carol", "kind": "permission", "tool": "Bash", "reason": "Runs a build", "input": map[string]any{"command": "rm -rf build && npm run build"}})
	b.post(t, tokens["carol"], "/api/ask", map[string]any{"session": "s-carol", "kind": "question", "questions": []map[string]any{{"question": "Should the README cover the Windows build too?", "header": "Scope",
		"options": []map[string]any{{"label": "Yes", "description": "briefly"}, {"label": "No", "description": "later"}}}}})
	waitUntil(t, 10*time.Second, "the asks are pending", func() bool { return len(b.Hub.Snapshot().Asks) == 2 })

	// the task: alice leads, bob helps; they talk, and you say something too
	body, _ := json.Marshal(map[string]any{"title": "Write the CHANGELOG", "leader": true, "permissionMode": "", "lang": "en", "room": 0,
		"members": []map[string]any{{"account": b.account("alice").ID, "model": "opus", "effort": "max"}, {"account": b.account("bob").ID, "model": "sonnet", "effort": "high"}}})
	res, err := httpPost(b, "/api/tasks", string(body))
	if err != nil {
		t.Fatal(err)
	}
	var task bridge.Task
	if err := json.Unmarshal(res, &task); err != nil || task.ID == "" {
		t.Fatalf("creating a task failed: %s", res)
	}
	sids := map[string]string{}
	for _, m := range task.Members {
		name := strings.TrimSuffix(m.Name, "-"+task.ID)
		sids[name] = "t-" + m.Name
		event(name, sids[name], map[string]any{"kind": "session.start", "cwd": m.Worktree, "launch": m.Launch, "task": task.ID, "model": "claude-opus-5-5"})
		event(name, sids[name], map[string]any{"kind": "session.title", "task": task.ID, "title": m.Name})
	}
	alice, bob := "alice-"+task.ID, "bob-"+task.ID
	wrapped := func(from, text string) string {
		return fmt.Sprintf("<cross-session-message from=\"uds:/tmp/x.sock\" from-name=%q from-mode=\"prompting\">\n%s\n</cross-session-message>", from, text)
	}
	event("bob", sids["bob"], map[string]any{"kind": "message.receive", "task": task.ID, "text": wrapped(alice, "Please write the release steps."), "origin": map[string]any{"kind": "peer"}})
	event("alice", sids["alice"], map[string]any{"kind": "message.send", "task": task.ID, "to": bob, "text": "Please write the release steps.", "origin": "model", "delivered": true})
	event("alice", sids["alice"], map[string]any{"kind": "message.receive", "task": task.ID, "text": wrapped(bob, "On it."), "origin": map[string]any{"kind": "peer"}})
	event("bob", sids["bob"], map[string]any{"kind": "message.send", "task": task.ID, "to": alice, "text": "On it.", "origin": "model", "delivered": true})
	event("bob", sids["bob"], map[string]any{"kind": "tool.start", "task": task.ID, "tool": "Edit", "summary": "CHANGELOG.md", "toolUseId": "tb1"})
	if res, err := httpPost(b, "/api/tasks/"+task.ID+"/say", `{"text":"@`+bob+` cover Windows too"}`); err != nil {
		t.Fatalf("saying something in the task chat: %v %s", err, res)
	}
	waitUntil(t, 10*time.Second, "the task's members are in", func() bool {
		for _, k := range b.Hub.Tasks() {
			if k.ID == task.ID && k.Members[0].Session != "" && k.Members[1].Session != "" {
				return true
			}
		}
		return false
	})
	return b, &task, tokens
}

// httpPost is a page call with the bridge's key, its answer's body.
func httpPost(b *testBridge, path, body string) ([]byte, error) {
	req := pageReq("POST", b.URL, path, strings.NewReader(body))
	req.Header.Set("content-type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	out, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		return out, fmt.Errorf("POST %s: %s", path, res.Status)
	}
	return out, nil
}

// contrastJS measures, under root, the colors the way WCAG 2.1 AA asks, from the computed styles: what lies behind an
// element is the backgrounds of it and its ancestors composed with their alpha and opacity (over the office's canvas for
// what floats on it). pass says what it measures:
//   - "colors": every piece of text against what it sits on (4.5:1, 3:1 for large text, by the font's size and
//     weight), the marks in the state colors against theirs (3:1), and the edge of every text field and select (3:1:
//     its border, a ring of its box-shadow or its own fill against what lies around it, the best of them), with the
//     animations stopped;
//   - "hover": the same over each element a :hover rule styles, one at a time, hovered (copies of the page's :hover
//     rules that a class turns on, on the element and its ancestors, as the pointer does);
//   - "frames": the same over the elements of each animation that runs longer than 5 seconds, stopped at each of its
//     keyframes and between them;
//   - "focus": every control's focus indicator (3:1 against what lies next to it: an outline, a border, a ring or a
//     background that focus changes; the browser's own ring passes), each focused in turn as by the keyboard.
//
// Disabled controls (and their labels), placeholders and what a search dims are exempt. It gives
// { checked, fails: [{ what, kind, text, fg, bg, ratio, need }], info }.
const contrastJS = `((root, pass) => {
  const still = document.createElement('style')
  still.textContent = pass === 'frames' ? '*, *::before, *::after { transition: none !important }' : '*, *::before, *::after { animation: none !important; transition: none !important }'
  document.head.appendChild(still)
  const later = [] // what a pass put on the page, taken away at the end
  try {
    const styles = new Map()
    const cs = el => { let s = styles.get(el); if (!s) { s = getComputedStyle(el); styles.set(el, s) } return s }
    const probe = document.createElement('canvas').getContext('2d', { willReadFrequently: true })
    const parse = c => {
      let m = /^rgba?\(([^)]*)\)$/.exec(c)
      if (m) { const p = m[1].split(/[\s,\/]+/).filter(Boolean).map(parseFloat); return [p[0], p[1], p[2], p.length > 3 ? p[3] : 1] }
      m = /^color\(srgb ([^)]*)\)$/.exec(c)
      if (m) { const p = m[1].split(/[\s\/]+/).filter(Boolean).map(parseFloat); return [p[0] * 255, p[1] * 255, p[2] * 255, p.length > 3 ? p[3] : 1] }
      if (!c || c === 'none' || c.startsWith('url(')) return null
      probe.clearRect(0, 0, 1, 1); probe.fillStyle = '#000'; probe.fillStyle = c; probe.fillRect(0, 0, 1, 1)
      const d = probe.getImageData(0, 0, 1, 1).data
      return [d[0], d[1], d[2], d[3] / 255]
    }
    const over = (top, bottom) => {
      const a = top[3] + bottom[3] * (1 - top[3])
      if (!a) return [0, 0, 0, 0]
      return [0, 1, 2].map(i => (top[i] * top[3] + bottom[i] * bottom[3] * (1 - top[3])) / a).concat(a)
    }
    const lin = c => { c /= 255; return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4 }
    const lum = c => 0.2126 * lin(c[0]) + 0.7152 * lin(c[1]) + 0.0722 * lin(c[2])
    const ratio = (a, b) => { const x = lum(a), y = lum(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05) }
    const hex = c => '#' + c.slice(0, 3).map(v => Math.round(v).toString(16).padStart(2, '0')).join('')
    // what lies behind the page's own backgrounds: the office's canvas under what floats on it (name tags, nameplates),
    // a few points of it; else the browser's white
    const office = document.getElementById('office'), octx = office && office.getContext('2d')
    function base(el) {
      const layer = octx && el.closest('#labels')
      if (!layer) return { from: null, under: [[255, 255, 255, 1]] }
      const r = el.getBoundingClientRect(), k = devicePixelRatio || 1
      const at = (x, y) => { const d = octx.getImageData(Math.max(0, Math.min(office.width - 1, Math.round(x * k))), Math.max(0, Math.min(office.height - 1, Math.round(y * k))), 1, 1).data; return [d[0], d[1], d[2], 1] }
      return { from: layer, under: [at(r.left + 1, r.top + 1), at(r.right - 2, r.top + 1), at(r.left + 1, r.bottom - 2), at(r.right - 2, r.bottom - 2), at((r.left + r.right) / 2, (r.top + r.bottom) / 2)] }
    }
    // the backgrounds from the top down to el (its own included), each with its opacity. An ancestor's background is
    // behind el only where its box is: one el lies outside of (a label hanging beside its stripe) is left out, unless
    // it scrolls (what it scrolls comes into its box); one without a box (display: contents) paints nothing at all
    function chain(el, from) {
      const list = []
      for (let n = el; n; n = n.parentElement) { list.unshift(n); if (n === from) break }
      // the middle of what shows of el: its box cut by the ancestors that clip it (an ellipsis cuts a long line short)
      let r = el.getBoundingClientRect(), [l, t, rr, b] = [r.left, r.top, r.right, r.bottom]
      for (const n of list) if (n !== el && (cs(n).overflowX !== 'visible' || cs(n).overflowY !== 'visible')) {
        const c = n.getBoundingClientRect()
        if (c.right > c.left && c.bottom > c.top) { l = Math.max(l, c.left); t = Math.max(t, c.top); rr = Math.min(rr, c.right); b = Math.min(b, c.bottom) }
      }
      const x = (l + rr) / 2, y = (t + b) / 2
      return list.filter(n => cs(n).display !== 'contents').map(n => {
        const s = cs(n), b = n.getBoundingClientRect()
        const under = n === el || n === document.documentElement || n === document.body || // (the page's own paint the whole window)
          s.overflowX !== 'visible' || s.overflowY !== 'visible' || (x >= b.left && x <= b.right && y >= b.top && y <= b.bottom)
        return { bg: under && parse(s.backgroundColor) || [0, 0, 0, 0], op: parseFloat(s.opacity) }
      })
    }
    // the same chain with el's own background left out: what lies around el
    const around = list => list.slice(0, -1).concat([{ bg: [0, 0, 0, 0], op: list[list.length - 1].op }])
    // what one pixel comes out as: the backgrounds painted down the chain (an opacity below 1 makes a group of its own,
    // put over what is below it at that opacity), then tops over the innermost
    function paint(list, tops, under) {
      const stack = [{ c: under, op: 1 }]
      for (const n of list) {
        if (n.op < 1) stack.push({ c: [0, 0, 0, 0], op: n.op })
        stack[stack.length - 1].c = over(n.bg, stack[stack.length - 1].c)
      }
      for (const top of tops) stack[stack.length - 1].c = over(top, stack[stack.length - 1].c)
      while (stack.length > 1) { const l = stack.pop(), below = stack[stack.length - 1]; below.c = over([l.c[0], l.c[1], l.c[2], l.c[3] * l.op], below.c) }
      return stack[0].c
    }
    // the lowest contrast of one painting against another, over every point behind them
    function versus(a, ta, c, tc, b) {
      let low = Infinity, at = null
      for (const under of b.under) {
        const fg = paint(a, ta, under), bg = paint(c, tc, under), r = ratio(fg, bg)
        if (r < low) { low = r; at = [fg, bg] }
      }
      return { r: low, fg: at[0], bg: at[1] }
    }
    // the lowest contrast of color over el's backgrounds against the backgrounds alone
    const worst = (list, tops, b) => versus(list, tops, list, [], b)
    const best = ms => ms.filter(Boolean).reduce((x, m) => !x || m.r > x.r ? m : x, null)
    const part = n => n.tagName.toLowerCase() + (n.id ? '#' + n.id : '') + [...n.classList].filter(c => c !== '__hover').map(c => '.' + c).join('')
    const what = el => { const host = el.parentElement && el.parentElement.closest('[id]'); return (host ? part(host) + ' … ' : '') + part(el) }
    // out of sight for screen readers only (a box of a pixel, clipped away) counts as not shown
    const clipped = el => { for (let n = el; n; n = n.parentElement) { const b = n.getBoundingClientRect(), s = cs(n); if (b.width <= 1 && b.height <= 1 && (s.overflowX !== 'visible' || s.clipPath !== 'none')) return true } return false }
    const shown = el => el.getClientRects().length > 0 && cs(el).visibility === 'visible' && el.checkVisibility({ contentVisibilityAuto: false, visibilityProperty: true }) && !clipped(el)
    const EXEMPT = ':disabled, :disabled *, label:has(:disabled), label:has(:disabled) *, [aria-disabled="true"], [aria-disabled="true"] *, option, .dim, .dim *'
    const FIELDS = 'input:not([type=checkbox]):not([type=radio]):not([type=hidden]):not([type=range]):not([type=color]), textarea, select'
    const fails = new Map(), info = {}
    let checked = 0
    let tag = '' // what state the page is in: '' or ' hovered', ' at <animation> <n>%'
    function note(el, kind, m, need, text) {
      checked++
      if (m.r >= need - 0.005) return
      const key = what(el) + '|' + kind + '|' + hex(m.fg) + '|' + hex(m.bg)
      if (!fails.has(key)) fails.set(key, { what: what(el), kind: kind + tag, text: String(text || '').trim().slice(0, 40), fg: hex(m.fg), bg: hex(m.bg), ratio: Math.round(m.r * 100) / 100, need })
    }
    function text(el) {
      if (!shown(el) || el.closest(EXEMPT)) return
      const s = cs(el), svg = el instanceof SVGElement
      const fg = parse(svg ? s.fill : s.color)
      if (!fg || !fg[3]) return
      const size = parseFloat(s.fontSize), bold = (parseInt(s.fontWeight, 10) || 400) >= 700
      const need = size >= 24 || (size >= 18.66 && bold) ? 3 : 4.5
      const b = base(el)
      const list = chain(el, b.from)
      if (list.some(n => n.op === 0)) return
      const words = el.value || [...el.childNodes].filter(n => n.nodeType === 3).map(n => n.textContent).join('')
      // a filter changes the colors in ways this doesn't follow: text under one can't be measured, so it fails
      for (let n = el; n; n = n.parentElement) if (cs(n).filter !== 'none') return note(el, 'text under a filter', { r: 0, fg: fg, bg: [255, 255, 255, 1] }, need, words)
      note(el, 'text', worst(list, [fg], b), need, words)
    }
    // the box-shadows of a computed value that draw a line (no blur): [{ c, inset }]
    function rings(v) {
      if (!v || v === 'none') return []
      const out = []
      for (const one of v.split(/,(?![^(]*\))/)) {
        const c = /(rgba?\([^)]*\)|color\([^)]*\))/.exec(one), rest = one.replace(c ? c[0] : '', '').trim().split(/\s+/)
        const inset = rest.includes('inset'), len = rest.filter(x => x !== 'inset').map(parseFloat)
        const color = c && parse(c[0])
        if (!color || !color[3] || (len[2] || 0) > 0) continue // blurred: a shadow, not a line
        if ((len[3] || 0) <= 0 && !len[0] && !len[1]) continue
        out.push({ c: color, inset })
      }
      return out
    }
    const sides = s => ['Top', 'Right', 'Bottom', 'Left'].filter(x => parseFloat(s['border' + x + 'Width']) >= 1 && !['none', 'hidden'].includes(s['border' + x + 'Style'])).map(x => parse(s['border' + x + 'Color'])).filter(c => c && c[3])
    // a text field's or select's edge: the best of its border, a line its box-shadow draws and its own fill, against
    // what lies around it
    function edge(el) {
      if (!shown(el) || el.closest(EXEMPT)) return
      const s = cs(el), b = base(el), list = chain(el, b.from)
      if (list.some(n => n.op === 0)) return
      const out = around(list)
      const m = best([versus(list, [], out, [], b), ...sides(s).map(c => versus(list, [c], out, [], b)),
        ...rings(s.boxShadow).map(r => r.inset ? versus(list, [r.c], out, [], b) : versus(out, [r.c], out, [], b))])
      note(el, 'edge', m, 3, el.getAttribute('aria-label') || el.placeholder || el.name)
    }
    // the marks in the state colors: [selector, the property with the color, what it sits on: its parent's background
    // ('parent') or its own (a band of its border: 'self')]
    const MARKS = [['.dot', 'backgroundColor', 'parent'], ['#hud .chip:not(.hot)', 'borderLeftColor', 'self'], ['#hud .chip.hot', 'backgroundColor', 'parent'],
      ['.emp-label', 'borderLeftColor', 'self'], ['.room-plate i', 'backgroundColor', 'parent'], ['#taskwin .top .status', 'borderLeftColor', 'self'],
      ['.chips .chip i', 'backgroundColor', 'parent'], ['.board .col h2 i', 'backgroundColor', 'parent'], ['.tile .st i', 'backgroundColor', 'parent']]
    function mark(el, prop, against) {
      if (!shown(el) || el.closest(EXEMPT)) return
      const c = parse(cs(el)[prop])
      if (!c || !c[3]) return
      const b = base(el), list = chain(el, b.from)
      if (list.some(n => n.op === 0)) return
      note(el, 'mark', worst(against === 'self' ? list : around(list), [c], b), 3, el.textContent)
    }
    // everything measured under (and at) one element
    function measure(top) {
      const seen = new Set(), walk = document.createTreeWalker(top, NodeFilter.SHOW_TEXT)
      while (walk.nextNode()) {
        const n = walk.currentNode, el = n.parentElement
        if (!el || seen.has(el) || !n.textContent.trim() || ['SCRIPT', 'STYLE', 'TITLE'].includes(el.tagName)) continue
        seen.add(el)
        text(el)
      }
      const all = sel => [top, ...top.querySelectorAll(sel)].filter(el => el.matches(sel))
      for (const f of all(FIELDS)) { if (f.tagName !== 'SELECT') text(f); edge(f) }
      for (const [sel, prop, against] of MARKS) for (const el of all(sel)) mark(el, prop, against)
    }
    const scope = document.querySelector(root)
    if (!scope) return JSON.stringify({ checked: 0, fails: [{ what: 'no ' + root }] })

    if (pass === 'colors') measure(scope)

    if (pass === 'hover') {
      // a top-level split of a selector list, and the element a selector hovers: the selector up to its last compound
      // with :hover, :hover taken out
      const split = (sel, at) => {
        const out = []; let depth = 0, q = null, start = 0
        for (let i = 0; i <= sel.length; i++) {
          const c = sel[i]
          if (q) { if (c === q) q = null; continue }
          if (c === '"' || c === "'") q = c
          else if (c === '(' || c === '[') depth++
          else if (c === ')' || c === ']') depth--
          else if (i === sel.length || (depth === 0 && at.test(c))) { out.push([start, i]); start = i + 1 }
        }
        return out
      }
      const hovered = sel => {
        let end = -1
        for (const [a, z] of split(sel, /[\s>+~]/)) if (sel.slice(a, z).includes(':hover')) end = z
        const s = end < 0 ? '' : sel.slice(0, end).replace(/:hover/g, '').trim()
        return !s || /[\s>+~]$/.test(s) ? null : s
      }
      // each copy goes in right after its rule, so it takes the rule's place in the cascade (a later rule as specific
      // still wins over it, as over the hover itself)
      const added = [], targets = new Set()
      const walkRules = box => {
        for (let i = 0; i < box.cssRules.length; i++) {
          const r = box.cssRules[i]
          if (r instanceof CSSStyleRule) {
            if (!r.selectorText.includes(':hover')) continue
            try { box.insertRule(r.selectorText.replace(/:hover/g, '.__hover') + ' { ' + r.style.cssText + ' }', i + 1); added.push([box, box.cssRules[++i]]) } catch (e) {}
            for (const [a, z] of split(r.selectorText, /,/)) {
              const s = hovered(r.selectorText.slice(a, z).trim())
              if (s) try { for (const el of scope.querySelectorAll(s)) targets.add(el) } catch (e) {}
            }
          } else if (r instanceof CSSMediaRule || r instanceof CSSSupportsRule) walkRules(r)
        }
      }
      for (const sheet of document.styleSheets) { let ok = false; try { ok = !!sheet.cssRules } catch (e) {} if (ok) walkRules(sheet) }
      later.push(() => { for (const [box, rule] of added.reverse()) { const k = [...box.cssRules].indexOf(rule); if (k >= 0) box.deleteRule(k) } })
      info.rules = added.length; info.hovered = 0
      tag = ' hovered'
      for (const el of [...targets].slice(0, 300)) {
        if (!shown(el)) continue
        const up = []; for (let n = el; n; n = n.parentElement) { n.classList.add('__hover'); up.push(n) }
        info.hovered++
        try { measure(el) } finally { for (const n of up) n.classList.remove('__hover') }
      }
    }

    if (pass === 'frames') {
      const anims = document.getAnimations().filter(a => { const tg = a.effect && a.effect.target; return tg && (scope.contains(tg) || tg.contains(scope)) })
      const was = anims.map(a => ({ a, t: a.currentTime, run: a.playState === 'running' }))
      later.push(() => { for (const w of was) { w.a.currentTime = w.t; if (w.run) w.a.play() } })
      for (const a of anims) a.pause()
      info.animations = anims.length; info.frames = 0
      for (const a of anims) {
        const tm = a.effect.getComputedTiming(), dur = tm.duration, delay = tm.delay || 0, tg = a.effect.target
        // one that is over within 5 seconds (a card rising into place, a find flashing) passes, as WCAG lets blinking
        // that short be (2.2.2); what keeps going must keep its contrast in every frame
        if (!(dur > 0) || !isFinite(dur) || tm.endTime <= 5000) continue
        const offs = [...new Set(a.effect.getKeyframes().map(k => k.computedOffset))].sort((x, y) => x - y), at = new Set()
        offs.forEach((o, i) => { at.add(o); if (i + 1 < offs.length) at.add((o + offs[i + 1]) / 2) })
        for (const o of at) {
          a.currentTime = delay + Math.min(o, 0.999) * dur
          tag = ' at ' + (a.animationName || 'an animation') + ' ' + Math.round(o * 100) + '%'
          info.frames++
          measure(scope.contains(tg) ? tg : scope)
        }
        a.currentTime = delay
      }
    }

    if (pass === 'focus') {
      // each control focused in turn after a text field, as by the keyboard, so the browser shows its focus (a field
      // always does, and a focus moved on from one that showed shows too)
      const lead = document.createElement('input')
      lead.style.cssText = 'position: fixed; left: 0; top: 0; width: 1px; height: 1px; opacity: 0; pointer-events: none'
      lead.tabIndex = -1; lead.setAttribute('aria-hidden', 'true')
      const was = document.activeElement
      scope.appendChild(lead)
      later.push(() => { if (was && was !== document.body && was.isConnected) was.focus({ preventScroll: true }); else document.activeElement?.blur(); lead.remove() })
      const look = el => { const s = cs(el); return { os: s.outlineStyle, ow: parseFloat(s.outlineWidth) || 0, oc: s.outlineColor, oo: parseFloat(s.outlineOffset) || 0,
        bc: ['Top', 'Right', 'Bottom', 'Left'].map(x => s['border' + x + 'Color']).join(), bs: s.boxShadow, bg: s.backgroundColor } }
      const els = [...scope.querySelectorAll('a[href], button, input:not([type=hidden]), select, textarea, summary, [tabindex]:not([tabindex="-1"])')]
        .filter(el => el !== lead && !el.closest(EXEMPT) && !el.closest('[hidden], [inert]') && el.checkVisibility({ visibilityProperty: true }))
      info.focused = 0; info.notVisible = []
      for (const el of els.slice(0, 400)) {
        lead.focus({ preventScroll: true })
        const b = base(el), was = look(el), plain = chain(el, b.from)
        el.focus({ preventScroll: true })
        if (document.activeElement !== el || !shown(el)) continue
        if (!el.matches(':focus-visible')) { info.notVisible.push(what(el)); continue }
        info.focused++
        const now = look(el)
        if (now.os === 'auto') { checked++; continue } // the browser's own ring, drawn in two colors
        const list = chain(el, b.from), out = around(list), s = cs(el)
        const ms = []
        if (now.os !== 'none' && now.ow >= 1) { const c = parse(now.oc); if (c && c[3]) ms.push(now.oo >= 0 ? versus(out, [c], out, [], b) : versus(list, [c], list, [], b)) }
        if (now.bc !== was.bc) ms.push(...sides(s).map(c => versus(list, [c], out, [], b)))
        if (now.bs !== was.bs) ms.push(...rings(s.boxShadow).map(r => r.inset ? versus(list, [r.c], list, [], b) : versus(out, [r.c], out, [], b)))
        if (now.bg !== was.bg) ms.push(versus(list, [], plain, [], b), versus(list, [], out, [], b))
        const m = best(ms) || { r: 1, fg: paint(list, [], b.under[0]), bg: paint(list, [], b.under[0]) }
        note(el, 'focus', m, 3, el.getAttribute('aria-label') || el.textContent)
      }
      if (!info.notVisible.length) delete info.notVisible
    }
    return JSON.stringify({ checked, fails: [...fails.values()], info })
  } finally { for (const f of later.reverse()) f(); still.remove() }
})`

// contrastView is one state of a page to measure: what to do to get there, what says it's there, where to measure,
// and what to do after.
type contrastView struct{ name, setup, ready, root, after string }

// contrastPasses are what audit measures in every view (contrastJS).
var contrastPasses = []string{"colors", "hover", "frames", "focus"}

// audit measures a view in every pass and gives its failures, each a line, adding what the passes went through to
// seen (contrastJS's info: the elements hovered, the animations and their frames, the controls focused).
func audit(t *testing.T, p *screens, label string, v contrastView, seen map[string]float64) (int, []string) {
	t.Helper()
	if v.setup != "" {
		p.run(v.setup)
	}
	if v.ready != "" {
		waitUntil(t, 15*time.Second, label+": "+v.name, func() bool { return p.is(v.ready) })
	}
	time.Sleep(150 * time.Millisecond)
	total := 0
	var out []string
	for _, pass := range contrastPasses {
		var res struct {
			Checked int
			Fails   []struct {
				What, Kind, Text, Fg, Bg string
				Ratio, Need              float64
			}
			Info map[string]any
		}
		if err := json.Unmarshal([]byte(get(t, p.ctx, 30*time.Second, chromedp.Evaluate[string](contrastJS+fmt.Sprintf("(%q, %q)", v.root, pass)))), &res); err != nil {
			t.Fatalf("%s %s %s: %v", label, v.name, pass, err)
		}
		if nv, ok := res.Info["notVisible"]; ok {
			t.Logf("%s %s: focused without showing it (:focus-visible): %v", label, v.name, nv)
		}
		for k, n := range res.Info {
			if f, ok := n.(float64); ok {
				seen[k] += f
			}
		}
		total += res.Checked
		for _, f := range res.Fails {
			out = append(out, fmt.Sprintf("%s %s: %s %s on %s = %.2f < %g %q", label, v.name, f.Kind, f.Fg, f.Bg, f.Ratio, f.Need, f.What+" "+f.Text))
		}
	}
	if v.after != "" {
		p.run(v.after)
	}
	return total, out
}

// TestContrast measures the colors of every theme in each of its schemes the way WCAG 2.1 AA asks (no tokens): with
// the made-up office of a11yTeam, it opens the pixel office, HQ and Coworkers light and dark, Mission Control dark and
// light, and the approval page, and in each the views that show text: the page, the employee window (a chat with every
// kind of item, the asks' cards, an employee off duty), the task window and a member's window, the launch and task
// dialogs, the search list and the logs. Every piece of text must reach 4.5:1 against what it sits on (3:1 when large)
// and every mark in a state color 3:1 (contrastJS). It fails naming each that falls short, with its colors.
func TestContrast(t *testing.T) {
	b, _, _ := a11yTeam(t)
	browser := newBrowser(t)
	id := func(name string) string { return b.account(name).ID }
	var fails []string
	total := 0
	seen := map[string]float64{}
	run := func(p *screens, label string, views []contrastView) {
		for _, v := range views {
			n, f := audit(t, p, label, v, seen)
			total += n
			fails = append(fails, f...)
		}
	}
	closeAll := `for (const s of ['#chat .close', '#taskwin .close', '#launch .cancel', '#task .cancel']) { const e = document.querySelector(s); if (e && e.closest('.modal, #chat, #taskwin') && !e.closest('[hidden]')) e.click() }`
	dialogs := []contrastView{
		{"launch dialog", `document.getElementById('new-emp').click()`, `!document.getElementById('launch').hidden && document.querySelectorAll('#launch select option').length > 1`, `#launch`, ``},
		{"launch dialog, bypass", `(s => { s.value = 'bypassPermissions'; s.dispatchEvent(new Event('change', { bubbles:true })) })(document.querySelector('#launch [name=permissionMode]'))`, `!document.querySelector('#launch .warn').hidden`, `#launch`, `document.querySelector('#launch .cancel').click()`},
		{"task dialog", `document.getElementById('new-task').click()`, `!document.getElementById('task').hidden && document.querySelectorAll('#task .mem').length >= 4`, `#task`, ``},
		{"task dialog, advisor", `document.querySelector('#task .add-adv').click(); [...document.querySelectorAll('#task .mem input')].find(i => !i.disabled)?.click()`, `!!document.querySelector('#task .adv')`, `#task`, `document.querySelector('#task .cancel').click()`},
	}

	// ---------- the pixel office ----------
	office, _ := clockTab(t, browser, b.URL+"/t/office/?lang=en", 0)
	do(t, office, 10*time.Second, chromedp.EmulateViewport(1400, 860))
	p := &screens{t: t, ctx: office, lang: "a11y-office"}
	waitUntil(t, 20*time.Second, "the office shows the team", func() bool {
		return p.is(`document.querySelectorAll('.emp-label').length >= 5 && document.querySelectorAll('.room-plate').length > 0 && !!document.querySelector('.room-plate[data-task]')`)
	})
	time.Sleep(time.Second) // the figures find their places
	label := func(name string) string {
		return fmt.Sprintf(`document.querySelector('.emp-label:not(.member)[data-account=%q]').click()`, id(name))
	}
	run(p, "office", append([]contrastView{
		{"page", ``, ``, `body`, ``},
		{"roster", `document.querySelector('#roster button').focus()`, `getComputedStyle(document.getElementById('roster')).position === 'fixed'`, `#roster`, `document.activeElement.blur()`},
		{"logs", `document.getElementById('open-log').click(); document.getElementById('open-msgs').click()`, `!document.getElementById('log').hidden && !document.getElementById('msgs-panel').hidden`, `body`,
			`document.querySelector('#log .close').click(); document.querySelector('#msgs-panel .close').click()`},
		{"search", `(f => { f.focus(); f.value = 'a'; f.dispatchEvent(new Event('input')) })(document.getElementById('find'))`, `!document.getElementById('find-list').hidden`, `#hud`,
			`(f => { f.value = ''; f.dispatchEvent(new Event('input')); f.blur() })(document.getElementById('find'))`},
		{"chat", label("alice"), `!document.getElementById('chat').hidden && document.querySelectorAll('#chat .flow .it').length >= 8`, `#chat`, closeAll},
		{"asks", label("carol"), `document.querySelectorAll('#chat #asks .card').length === 2`, `#chat`, closeAll},
		{"off duty", label("erin"), `!document.getElementById('chat').hidden && !document.querySelector('#chat .offline').hidden`, `#chat`, closeAll},
		{"task window", `document.querySelector('.room-plate[data-task]').click()`, `!document.getElementById('taskwin').hidden && document.querySelectorAll('#taskwin .tmsg').length >= 3`, `#taskwin`, ``},
		{"member", `document.querySelector('#taskwin .mrow').click()`, `!document.getElementById('chat').hidden`, `#taskwin`, closeAll},
	}, dialogs...))

	// ---------- HQ, Coworkers, Mission Control: light and dark ----------
	scheme := func(p *screens, dark bool) bool {
		if !p.is(`!!document.getElementById('dark')`) {
			return false
		}
		if p.is(`document.documentElement.hasAttribute('data-dark')`) != dark {
			p.run(`document.getElementById('dark').click()`)
		}
		return p.is(`document.documentElement.hasAttribute('data-dark')`) == dark
	}
	themes := []struct {
		name, ready string
		views       []contrastView
	}{
		{"hq", `document.querySelectorAll('#kpis .kpi').length === 7`, append([]contrastView{
			{"overview", `document.querySelector('#tabs [data-tab=overview]').click()`, `!!document.querySelector('#ov-recent .row')`, `body`, ``},
			{"people", `document.querySelector('#tabs [data-tab=people]').click()`, `document.querySelectorAll('#people tbody tr').length === 5`, `body`, ``},
			{"approvals", `document.querySelector('#tabs [data-tab=approvals]').click()`, `document.querySelectorAll('#queue .card').length === 2`, `body`, ``},
			{"projects", `document.querySelector('#tabs [data-tab=projects]').click()`, `document.querySelectorAll('.board .task').length === 1`, `body`, ``},
			{"activity", `document.querySelector('#tabs [data-tab=activity]').click()`, `document.querySelectorAll('#act-list .row').length > 3`, `body`, ``},
			{"search", `(f => { f.focus(); f.value = 'a'; f.dispatchEvent(new Event('input')) })(document.getElementById('find'))`, `!document.getElementById('find-list').hidden`, `.top`,
				`(f => { f.value = ''; f.dispatchEvent(new Event('input')); f.blur() })(document.getElementById('find'))`},
			{"chat", `document.querySelector('#tabs [data-tab=people]').click(); document.querySelector('#people tbody tr[data-account="` + id("alice") + `"] [data-act=open]').click()`, `!document.getElementById('chat').hidden && document.querySelectorAll('#chat .flow .it').length >= 8`, `#chat`, closeAll},
			{"asks", `document.querySelector('#people tbody tr[data-account="` + id("carol") + `"] [data-act=open]').click()`, `document.querySelectorAll('#chat #asks .card').length === 2`, `#chat`, closeAll},
			{"task window", `document.querySelector('#tabs [data-tab=projects]').click(); document.querySelector('.board .task').click()`, `!document.getElementById('taskwin').hidden && document.querySelectorAll('#taskwin .tmsg').length >= 3`, `#taskwin`, ``},
			{"member", `document.querySelector('#taskwin .mrow').click()`, `!document.getElementById('chat').hidden`, `#chat`, closeAll},
		}, dialogs...)},
		{"messenger", `document.querySelectorAll('#people .row').length === 5`, append([]contrastView{
			{"sidebar", ``, ``, `body`, ``},
			{"chat", `[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('alice')).click()`, `!document.getElementById('chat').hidden && document.querySelectorAll('#chat .flow .it').length >= 8 && !document.getElementById('profile').hidden`, `body`, ``},
			{"asks", `[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('carol')).click()`, `document.querySelectorAll('#chat .flow .it.ask.carded .card').length === 2`, `body`, ``},
			{"off duty", `[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('erin')).click()`, `!document.getElementById('chat').hidden && !document.querySelector('#chat .offline').hidden`, `body`, ``},
			{"group", `document.querySelector('#groups .row').click()`, `!document.getElementById('taskwin').hidden && document.querySelectorAll('#taskwin .tmsg').length >= 3`, `body`, ``},
		}, dialogs...)},
		{"mission", `document.querySelectorAll('#wall .tile').length >= 7`, append([]contrastView{
			{"wall", ``, `document.querySelectorAll('#ticker .item').length > 3`, `body`, ``},
			{"chat", `[...document.querySelectorAll('#wall .tile')].find(x => x.querySelector('.nm').textContent === 'alice').click()`, `!document.getElementById('chat').hidden && document.querySelectorAll('#chat .flow .it').length >= 8`, `#chat`, closeAll},
			{"asks", `document.getElementById('stat-alarms').click()`, `document.querySelectorAll('#chat #asks .card').length === 2`, `#chat`, closeAll},
			{"off duty", `[...document.querySelectorAll('#wall .tile')].find(x => x.querySelector('.nm').textContent === 'erin').click()`, `!document.getElementById('chat').hidden && !document.querySelector('#chat .offline').hidden`, `#chat`, closeAll},
			{"member", `document.querySelector('#wall .tile.member').click()`, `!document.getElementById('taskwin').hidden && !document.getElementById('chat').hidden && document.querySelectorAll('#taskwin .tmsg').length >= 3`, `body`, closeAll},
		}, dialogs...)},
	}
	for _, th := range themes {
		ctx, closeTab := clockTab(t, browser, b.URL+"/t/"+th.name+"/?lang=en", 0)
		do(t, ctx, 10*time.Second, chromedp.EmulateViewport(1400, 860))
		q := &screens{t: t, ctx: ctx, lang: "a11y-" + th.name}
		waitUntil(t, 20*time.Second, th.name+": the page shows the team", func() bool { return q.is(th.ready) })
		for _, dark := range []bool{false, true} {
			name := map[bool]string{false: "light", true: "dark"}[dark]
			if !scheme(q, dark) {
				if dark {
					continue
				}
				name = "only"
			}
			run(q, th.name+" "+name, th.views)
		}
		scheme(q, false) // (the switch is remembered for the next tab)
		q.done()
		closeTab()
	}

	// ---------- the approval page ----------
	approve, closeApprove := clockTab(t, browser, b.URL+"/approve?lang=en", 0)
	do(t, approve, 10*time.Second, chromedp.EmulateViewport(1400, 860))
	a := &screens{t: t, ctx: approve, lang: "a11y-approve"}
	run(a, "approve", []contrastView{{"page", ``, `document.querySelectorAll('#list .card').length === 2`, `body`, ``}})
	closeApprove()

	sort.Strings(fails)
	t.Logf("%d measured, %d short; %.0f elements hovered (%.0f :hover rules), %.0f frames of %.0f animations, %.0f controls focused",
		total, len(fails), seen["hovered"], seen["rules"], seen["frames"], seen["animations"], seen["focused"])
	if seen["hovered"] == 0 || seen["frames"] == 0 || seen["focused"] == 0 {
		t.Errorf("a pass measured nothing: %v", seen)
	}
	if len(fails) > 0 {
		t.Errorf("colors short of WCAG 2.1 AA:\n%s", strings.Join(fails, "\n"))
	}
}

// fitsJS says what doesn't fit the window: the page scrolling sideways, a window or dialog over the page reaching out
// of it, anything in one wider than its box (a scroller of its own aside), or a control of a bar or panel floating over
// the page cut off by the window's edge; each with what sticks out.
const fitsJS = `(() => {
  const out = [], W = innerWidth, H = innerHeight
  const name = c => c.tagName.toLowerCase() + (c.id ? '#' + c.id : '') + [...c.classList].map(x => '.' + x).join('')
  // what reaches past a right edge: the outermost elements that do, inside a box that doesn't scroll them itself
  const past = (root, right) => {
    const hits = []
    const walk = el => {
      for (const n of el.childNodes) if (n.nodeType === 3 && n.textContent.trim()) {
        const range = document.createRange(); range.selectNodeContents(n)
        const r = range.getBoundingClientRect()
        if (r.right > right + 1) { hits.push('text "' + n.textContent.trim().slice(0, 30) + '" in ' + name(el) + ' ' + Math.round(r.width) + 'px'); if (hits.length >= 4) return }
      }
      for (const c of el.children) {
        if (!c.getClientRects().length || getComputedStyle(c).position === 'fixed') continue
        const r = c.getBoundingClientRect()
        if (r.right > right + 1) { hits.push(name(c) + ' ' + Math.round(r.width) + 'px'); if (hits.length >= 4) return }
        else if (!['auto', 'scroll', 'hidden', 'clip'].includes(getComputedStyle(c).overflowX)) walk(c)
        if (hits.length >= 4) return
      }
    }
    walk(root)
    return hits.length ? ' (' + hits.join(', ') + ')' : ''
  }
  for (const el of [document.documentElement, document.body]) if (el.scrollWidth > W + 1) out.push(el.tagName.toLowerCase() + ' scrolls sideways: ' + el.scrollWidth + ' > ' + W + past(document.body, W))
  for (const p of document.querySelectorAll('.modal:not([hidden]) .panel, .modal:not([hidden]) > form')) {
    if (!p.getClientRects().length) continue
    const r = p.getBoundingClientRect(), id = p.closest('[id]').id
    if (r.left < -1 || r.top < -1 || r.right > W + 1 || r.bottom > H + 1) out.push(id + ' reaches out of the window: ' + JSON.stringify([Math.round(r.left), Math.round(r.top), Math.round(r.right), Math.round(r.bottom)]) + ' in ' + W + 'x' + H)
    if (p.scrollWidth > p.clientWidth + 1) out.push(id + ' scrolls sideways: ' + p.scrollWidth + ' > ' + p.clientWidth + past(p, r.left + p.clientWidth))
  }
  // what floats over the page in sight (a bar, a panel; not the drawing's name tags) keeps its controls in the window
  for (const f of document.querySelectorAll('body *')) {
    if (getComputedStyle(f).position !== 'fixed' || f.closest('[aria-hidden="true"], .modal')) continue
    const r = f.getBoundingClientRect()
    if (r.width <= 1 || r.height <= 1 || r.bottom <= 0 || r.top >= H || r.right <= 0 || r.left >= W) continue
    const cut = [...f.querySelectorAll('button, select, input, textarea, a[href], [tabindex]')]
      .filter(c => c.getClientRects().length && c.checkVisibility({ visibilityProperty: true }) && (q => q.right > W + 1 || q.left < -1)(c.getBoundingClientRect()))
    if (cut.length) out.push(name(f) + ' cuts off ' + cut.slice(0, 3).map(name).join(', '))
  }
  return out.join('\n')
})()`

// TestNarrow opens every theme in a window 800 pixels wide (and the approval page in one 400 wide, a phone's) with the
// made-up office of a11yTeam (no tokens): the page doesn't scroll sideways, and the windows and dialogs fit in it,
// scrolling inside (a member's window stacks over its task window, as below 1250 pixels). Screenshots into
// e2e/artifacts/screens/narrow-*.
func TestNarrow(t *testing.T) {
	b, _, _ := a11yTeam(t)
	browser := newBrowser(t)
	id := func(name string) string { return b.account(name).ID }
	check := func(p *screens, where string) {
		t.Helper()
		time.Sleep(200 * time.Millisecond)
		if bad := p.js(fitsJS); bad != "" {
			t.Errorf("%s %s:\n%s", p.lang, where, bad)
		}
	}
	closeAll := `for (const s of ['#chat .close', '#taskwin .close', '#launch .cancel', '#task .cancel']) { const e = document.querySelector(s); if (e && !e.closest('[hidden]')) e.click() }`
	type step struct{ name, setup, ready string }
	pages := []struct {
		theme string
		w, h  int64
		ready string
		steps []step
	}{
		{"office", 800, 600, `document.querySelectorAll('.emp-label').length >= 5`, []step{
			{"page", ``, ``},
			{"chat", fmt.Sprintf(`document.querySelector('.emp-label:not(.member)[data-account=%q]').click()`, id("alice")), `!document.getElementById('chat').hidden`},
			{"asks", closeAll + fmt.Sprintf(`; document.querySelector('.emp-label:not(.member)[data-account=%q]').click()`, id("carol")), `document.querySelectorAll('#chat #asks .card').length === 2`},
			{"task window", closeAll + `; document.querySelector('.room-plate[data-task]').click()`, `!document.getElementById('taskwin').hidden`},
			{"member", `document.querySelector('#taskwin .mrow').click()`, `!document.getElementById('chat').hidden && document.getElementById('chat').classList.contains('stacked')`},
			{"launch", closeAll + `; document.getElementById('new-emp').click()`, `!document.getElementById('launch').hidden`},
			{"task dialog", closeAll + `; document.getElementById('new-task').click()`, `!document.getElementById('task').hidden`},
			{"logs", closeAll + `; document.getElementById('open-log').click(); document.getElementById('open-msgs').click()`, `!document.getElementById('msgs-panel').hidden`},
		}},
		{"hq", 800, 600, `document.querySelectorAll('#kpis .kpi').length === 7`, []step{
			{"overview", ``, ``},
			{"people", `document.querySelector('#tabs [data-tab=people]').click()`, `document.querySelectorAll('#people tbody tr').length === 5`},
			{"approvals", `document.querySelector('#tabs [data-tab=approvals]').click()`, `document.querySelectorAll('#queue .card').length === 2`},
			{"projects", `document.querySelector('#tabs [data-tab=projects]').click()`, `document.querySelectorAll('.board .task').length === 1`},
			{"activity", `document.querySelector('#tabs [data-tab=activity]').click()`, `document.querySelectorAll('#act-list .row').length > 3`},
			{"chat", `document.querySelector('#tabs [data-tab=people]').click(); document.querySelector('#people tbody tr [data-act=open]').click()`, `!document.getElementById('chat').hidden`},
			{"task window", closeAll + `; document.querySelector('#tabs [data-tab=projects]').click(); document.querySelector('.board .task').click()`, `!document.getElementById('taskwin').hidden`},
			{"member", `document.querySelector('#taskwin .mrow').click()`, `!document.getElementById('chat').hidden`},
			{"task dialog", closeAll + `; document.getElementById('new-task').click()`, `!document.getElementById('task').hidden`},
		}},
		{"messenger", 800, 600, `document.querySelectorAll('#people .row').length === 5`, []step{
			{"sidebar", ``, ``},
			{"chat", `[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('alice')).click()`, `!document.getElementById('chat').hidden`},
			{"asks", `[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('carol')).click()`, `document.querySelectorAll('#chat .flow .it.ask.carded .card').length === 2`},
			{"group", `document.querySelector('#groups .row').click()`, `!document.getElementById('taskwin').hidden`},
			{"launch", `document.getElementById('new-emp').click()`, `!document.getElementById('launch').hidden`},
		}},
		{"mission", 800, 600, `document.querySelectorAll('#wall .tile').length >= 7`, []step{
			{"wall", ``, ``},
			{"chat", `document.getElementById('stat-alarms').click()`, `document.querySelectorAll('#chat #asks .card').length === 2`},
			{"member", closeAll + `; document.querySelector('#wall .tile.member').click()`, `!document.getElementById('chat').hidden && !document.getElementById('taskwin').hidden`},
			{"task dialog", closeAll + `; document.getElementById('new-task').click()`, `!document.getElementById('task').hidden`},
		}},
	}
	for _, pg := range pages {
		// the theme's narrow window, then 320 pixels: a phone, or a window of 1280 zoomed to 400% (WCAG 2.1's reflow)
		for _, size := range [][2]int64{{pg.w, pg.h}, {320, 640}} {
			ctx, closeTab := clockTab(t, browser, b.URL+"/t/"+pg.theme+"/?lang=en", 0)
			do(t, ctx, 10*time.Second, chromedp.EmulateViewport(size[0], size[1]))
			p := &screens{t: t, ctx: ctx, lang: fmt.Sprintf("narrow-%s-%d", pg.theme, size[0])}
			waitUntil(t, 20*time.Second, pg.theme+": the page shows the team", func() bool { return p.is(pg.ready) })
			for _, s := range pg.steps {
				if s.setup != "" {
					p.run(s.setup)
				}
				if s.ready != "" {
					waitUntil(t, 10*time.Second, fmt.Sprintf("%s %d: %s", pg.theme, size[0], s.name), func() bool { return p.is(s.ready) })
				}
				check(p, s.name)
				p.shot(strings.ReplaceAll(s.name, " ", "-"))
			}
			if pg.theme == "messenger" && size[0] == 320 { // one column: what is picked fills it, its close button leads back
				p.run(`[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('alice')).click()`)
				waitUntil(t, 10*time.Second, "messenger 320: alice's conversation fills the window, the lists away", func() bool {
					return p.is(`!document.getElementById('chat').hidden && getComputedStyle(document.querySelector('.side')).display === 'none' && document.getElementById('chat').getBoundingClientRect().width > 300`)
				})
				if !p.is(`(c => !!c && c.getClientRects().length > 0)(document.querySelector('#chat .head .close'))`) {
					t.Error("messenger 320: the conversation should show its close button, the way back to the lists")
				}
				p.run(`document.querySelector('#chat .head .close').click()`)
				waitUntil(t, 10*time.Second, "messenger 320: the close button leads back to the lists", func() bool {
					return p.is(`document.getElementById('chat').hidden && getComputedStyle(document.querySelector('.side')).display !== 'none'`)
				})
			}
			p.done()
			closeTab()
		}
	}
	// the approval page on phones
	for _, w := range []int64{400, 320} {
		approve, closeApprove := clockTab(t, browser, b.URL+"/approve?lang=en", 0)
		do(t, approve, 10*time.Second, chromedp.EmulateViewport(w, 760))
		a := &screens{t: t, ctx: approve, lang: fmt.Sprintf("narrow-approve-%d", w)}
		waitUntil(t, 20*time.Second, "the approval page lists the two asks", func() bool { return a.is(`document.querySelectorAll('#list .card').length === 2`) })
		check(a, "page")
		if bad := a.js(`[...document.querySelectorAll('#list .card, #list .card *')].filter(e => e.getBoundingClientRect().right > innerWidth + 1).map(e => e.tagName + '.' + e.className).slice(0, 5).join(', ')`); bad != "" {
			t.Errorf("the approval page's cards reach out of a %d-pixel window: %s", w, bad)
		}
		a.shot("page")
		a.done()
		closeApprove()
	}
}

// axNode is what Chrome's accessibility tree, the one screen readers read, says of an element.
type axNode struct {
	Role, Name string
	Ignored    bool
}

func axString(v *accessibility.Value) string {
	var s string
	if v != nil {
		_ = json.Unmarshal(v.Value, &s)
	}
	return s
}

// ax asks the accessibility tree about the element expr evaluates to.
func ax(t *testing.T, ctx context.Context, expr string) axNode {
	t.Helper()
	return get(t, ctx, 10*time.Second, chromedp.Action[axNode](func(ctx context.Context, tg *chromedp.Target) (axNode, error) {
		ev, err := cdp.Call(ctx, tg, cdpruntime.Evaluate, cdpruntime.EvaluateParams{Expression: expr})
		if err != nil {
			return axNode{}, err
		}
		if ev.ExceptionDetails != nil || ev.Result == nil || ev.Result.ObjectID == "" {
			return axNode{}, fmt.Errorf("%s is no element", expr)
		}
		res, err := cdp.Call(ctx, tg, accessibility.GetPartialAXTree, accessibility.GetPartialAXTreeParams{ObjectID: ev.Result.ObjectID, FetchRelatives: ptrTo(false)})
		if err != nil || len(res.Nodes) == 0 {
			return axNode{Ignored: true}, err
		}
		n := res.Nodes[0]
		return axNode{Role: axString(n.Role), Name: axString(n.Name), Ignored: n.Ignored}, nil
	}))
}

// unnamed lists the controls under the element expr evaluates to that the accessibility tree gives no name: what a
// screen reader would call just "button".
func unnamed(t *testing.T, ctx context.Context, expr string) []string {
	t.Helper()
	controls := map[string]bool{"button": true, "textbox": true, "searchbox": true, "combobox": true, "checkbox": true, "radio": true, "link": true,
		"listbox": true, "slider": true, "spinbutton": true, "switch": true, "tab": true, "menuitem": true, "dialog": true, "region": true}
	return get(t, ctx, 20*time.Second, chromedp.Action[[]string](func(ctx context.Context, tg *chromedp.Target) ([]string, error) {
		ev, err := cdp.Call(ctx, tg, cdpruntime.Evaluate, cdpruntime.EvaluateParams{Expression: expr})
		if err != nil || ev.Result == nil || ev.Result.ObjectID == "" {
			return nil, fmt.Errorf("%s is no element: %v", expr, err)
		}
		res, err := cdp.Call(ctx, tg, accessibility.QueryAXTree, accessibility.QueryAXTreeParams{ObjectID: ev.Result.ObjectID})
		if err != nil {
			return nil, err
		}
		var out []string
		for _, n := range res.Nodes {
			role := axString(n.Role)
			if n.Ignored || !controls[role] || strings.TrimSpace(axString(n.Name)) != "" {
				continue
			}
			what := role
			if d, err := cdp.Call(ctx, tg, dom.DescribeNode, dom.DescribeNodeParams{BackendNodeID: n.BackendDOMNodeID}); err == nil && d.Node != nil {
				what += " " + strings.ToLower(d.Node.NodeName) + fmt.Sprint(d.Node.Attributes)
			}
			out = append(out, what)
		}
		return out, nil
	}))
}

// TestA11y walks the pages with the keyboard and asks the accessibility tree what a screen reader hears, with the
// made-up office of a11yTeam (no tokens). The pixel office: "/" and the search box (a combobox) open an employee's
// window, a modal dialog named by the employee, where Tab and Shift+Tab stay and from which Esc goes back to the search
// box; Esc leaves a window whose asks wait alone; the roster (out of sight until it has the focus) reaches every
// employee and room, the task window's members open their windows docked beside it (a region), and closing each puts
// the focus back where it was; the launch dialog's fields are named by their labels; a new ask is announced; the
// connection banner is a polite status. HQ, Coworkers and Mission Control: their switches say whether they're on, their
// rows and tiles open with Enter, Coworkers lays its windows out as regions. Every control on every view has a name.
func TestA11y(t *testing.T) {
	b, task, tokens := a11yTeam(t)
	browser := newBrowser(t)
	id := func(name string) string { return b.account(name).ID }
	key := func(ctx context.Context, keys string, shift bool) {
		t.Helper()
		var opts []chromedp.KeyOption
		if shift {
			opts = append(opts, chromedp.KeyModifiers(chromedp.ModifierShift))
		}
		do(t, ctx, 10*time.Second, chromedp.KeyEvent(keys, opts...))
		time.Sleep(120 * time.Millisecond) // (the windows' observers and the theme's redraws)
	}
	named := func(p *screens, where, expr string) {
		t.Helper()
		if bad := unnamed(t, p.ctx, expr); len(bad) > 0 {
			t.Errorf("%s %s: controls without a name: %v", p.lang, where, bad)
		}
	}
	focusIn := func(p *screens, sel string) bool {
		return p.is(fmt.Sprintf(`!!document.activeElement && !!document.activeElement.closest(%q)`, sel))
	}

	// ---------- the pixel office ----------
	office, _ := clockTab(t, browser, b.URL+"/t/office/?lang=en", 0)
	do(t, office, 10*time.Second, chromedp.EmulateViewport(1400, 860))
	p := &screens{t: t, ctx: office, lang: "a11y-office"}
	waitUntil(t, 20*time.Second, "the office shows the team", func() bool {
		return p.is(`document.querySelectorAll('.emp-label').length >= 5 && !!document.querySelector('.room-plate[data-task]') && document.querySelectorAll('#roster button').length >= 6`)
	})
	named(p, "page", `document.body`)
	if !p.is(`(c => c.getAttribute('role') === 'status' && c.getAttribute('aria-live') === 'polite')(document.getElementById('conn'))`) {
		t.Error("the connection banner should be a polite status")
	}
	if n := ax(t, office, `document.getElementById('hud-tab')`); n.Name == "" || n.Role != "button" {
		t.Errorf("the HUD's tab should be a named button: %+v", n)
	}

	// "/", the search box, Enter: bob's window, a modal dialog named bob
	key(office, "/", false)
	if !p.is(`document.activeElement === document.getElementById('find')`) {
		t.Fatal(`"/" should focus the search box`)
	}
	if n := ax(t, office, `document.getElementById('find')`); n.Role != "combobox" || n.Name == "" {
		t.Errorf("the search box should be a named combobox: %+v", n)
	}
	p.run(`document.getElementById('find').value = ''`) // (CDP types the "/" as a char event of its own, which a real keyboard wouldn't)
	key(office, "bob", false)
	waitUntil(t, 5*time.Second, "the search lists bob, the box points at him", func() bool {
		return p.is(`!document.getElementById('find-list').hidden && document.getElementById('find').getAttribute('aria-expanded') === 'true' && document.getElementById(document.getElementById('find').getAttribute('aria-activedescendant'))?.getAttribute('aria-selected') === 'true'`)
	}, func() string {
		return p.js(`JSON.stringify({ value: document.getElementById('find').value, active: document.activeElement.id, hidden: document.getElementById('find-list').hidden, list: document.getElementById('find-list').innerHTML.slice(0, 300), expanded: document.getElementById('find').getAttribute('aria-expanded'), ad: document.getElementById('find').getAttribute('aria-activedescendant') })`)
	})
	key(office, kb.Enter, false)
	waitUntil(t, 5*time.Second, "Enter opens bob's window with the focus in it", func() bool {
		return p.is(`!document.getElementById('chat').hidden`) && focusIn(p, "#chat")
	})
	if n := ax(t, office, `document.getElementById('chat')`); n.Role != "dialog" || n.Name != "bob" || !p.is(`document.getElementById('chat').getAttribute('aria-modal') === 'true'`) {
		t.Errorf("bob's window should be a modal dialog named bob: %+v", n)
	}
	named(p, "chat", `document.getElementById('chat')`)
	for i := 0; i < 14; i++ { // round and round, never out
		key(office, kb.Tab, i >= 7)
		if !focusIn(p, "#chat") {
			t.Fatalf("Tab %d left the window: the focus is on %s", i, p.js(`document.activeElement.outerHTML.slice(0, 80)`))
		}
	}
	key(office, kb.Escape, false)
	if !p.is(`document.getElementById('chat').hidden && document.activeElement === document.getElementById('find')`) {
		t.Errorf("Esc should close bob's window and put the focus back on the search box: %s", p.js(`document.activeElement.outerHTML.slice(0, 80)`))
	}

	// a window whose asks wait stays on Esc
	p.run(`document.querySelector('#stats .chip.hot').focus()`)
	key(office, kb.Enter, false)
	waitUntil(t, 5*time.Second, "the pending chip opens carol's window, the focus on her first card", func() bool {
		return p.is(`document.querySelectorAll('#chat #asks .card').length === 2`) && focusIn(p, "#chat #asks")
	})
	if n := ax(t, office, `document.querySelector('#chat #asks .card')`); n.Role != "group" || !strings.Contains(n.Name, "Permission") {
		t.Errorf("an ask's card should be a group named by its top line: %+v", n)
	}
	named(p, "asks", `document.getElementById('chat')`)
	key(office, kb.Escape, false)
	if p.is(`document.getElementById('chat').hidden`) {
		t.Error("Esc shouldn't close a window whose asks wait for an answer")
	}
	p.run(`document.querySelector('#chat .close').click()`)

	// the roster: Tab from the HUD's tab, out of sight until then
	if p.is(`getComputedStyle(document.getElementById('roster')).position === 'fixed'`) {
		t.Error("the roster should be out of sight until it has the focus")
	}
	p.run(`document.getElementById('hud-tab').focus()`)
	key(office, kb.Tab, false)
	first := fmt.Sprintf(`#roster button[data-account=%q]`, id("alice"))
	if !p.is(fmt.Sprintf(`document.activeElement === document.querySelector(%q) && getComputedStyle(document.getElementById('roster')).position === 'fixed'`, first)) {
		t.Fatalf("Tab after the HUD should reach the roster's first employee, and show the roster: %s", p.js(`document.activeElement.outerHTML.slice(0, 80)`))
	}
	if n := ax(t, office, `document.querySelector('`+first+`')`); !strings.HasPrefix(n.Name, "alice") || !strings.Contains(n.Name, "Working") {
		t.Errorf("alice's button should say who and what she does: %+v", n)
	}
	p.shot("roster")
	key(office, kb.Enter, false)
	waitUntil(t, 5*time.Second, "Enter on alice opens her window", func() bool {
		return p.is(`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === 'alice'`) && focusIn(p, "#chat")
	})
	key(office, kb.Escape, false)
	if !p.is(fmt.Sprintf(`document.getElementById('chat').hidden && document.activeElement === document.querySelector(%q)`, first)) {
		t.Errorf("closing alice's window should put the focus back on her button: %s", p.js(`document.activeElement.outerHTML.slice(0, 80)`))
	}
	// the room with the task: its window (a dialog), a member's window docked beside it (a region), back step by step
	room := `#roster button[data-room]:not([aria-disabled])`
	p.run(fmt.Sprintf(`[...document.querySelectorAll(%q)].find(b => b.textContent.includes(%q)).focus()`, room, task.ID))
	key(office, kb.Enter, false)
	waitUntil(t, 5*time.Second, "Enter on the room opens its task window, the focus in it", func() bool {
		return p.is(`!document.getElementById('taskwin').hidden`) && focusIn(p, "#taskwin")
	})
	if n := ax(t, office, `document.getElementById('taskwin')`); n.Role != "dialog" || !strings.HasPrefix(n.Name, task.ID) {
		t.Errorf("the task window should be a dialog named by its task: %+v", n)
	}
	named(p, "task window", `document.getElementById('taskwin')`)
	// the team chat's mention picker: "@" lists the members, the box points at the one picked, the arrows move it, Esc
	// closes the picker alone
	box := `document.querySelector('#taskwin .tsay textarea')`
	p.run(box + `.focus()`)
	key(office, "@", false)
	waitUntil(t, 5*time.Second, "@ opens the mention picker, the box pointing at its first member", func() bool {
		return p.is(`(box => { const list = document.getElementById(box.getAttribute('aria-controls')), at = document.getElementById(box.getAttribute('aria-activedescendant'));` +
			` return !!list && !list.hidden && box.getAttribute('aria-autocomplete') === 'list' && box.getAttribute('aria-haspopup') === 'listbox' &&` +
			` !!at && list.contains(at) && at.getAttribute('role') === 'option' && at.getAttribute('aria-selected') === 'true' })(` + box + `)`)
	}, func() string {
		return p.js(`(box => JSON.stringify({ attrs: [...box.attributes].map(a => a.name + '=' + a.value), list: document.querySelector('#taskwin .tpick').outerHTML.slice(0, 400) }))(` + box + `)`)
	})
	if n := ax(t, office, `document.querySelector('#taskwin .tpick')`); n.Role != "listbox" || n.Name == "" {
		t.Errorf("the mention picker should be a named listbox: %+v", n)
	}
	if n := ax(t, office, box); n.Role != "textbox" || n.Name == "" {
		t.Errorf("the team chat's box should stay a named textbox: %+v", n)
	}
	picked := p.js(box + `.getAttribute('aria-activedescendant')`)
	key(office, kb.ArrowDown, false)
	if !p.is(fmt.Sprintf(`(box => box.getAttribute('aria-activedescendant') !== %q && document.getElementById(box.getAttribute('aria-activedescendant'))?.getAttribute('aria-selected') === 'true' && document.querySelectorAll('#taskwin .tpick [aria-selected=true]').length === 1)(`+box+`)`, picked)) {
		t.Errorf("ArrowDown should move the picker's choice, and the box's aria-activedescendant with it: %s", p.js(`document.querySelector('#taskwin .tpick').outerHTML.slice(0, 400)`))
	}
	key(office, kb.Escape, false)
	if !p.is(`document.querySelector('#taskwin .tpick').hidden && !` + box + `.hasAttribute('aria-activedescendant') && !document.getElementById('taskwin').hidden`) {
		t.Error("Esc should close the mention picker alone, the box pointing at nothing")
	}
	p.run(`(b => { b.value = ''; b.dispatchEvent(new Event('input')) })(` + box + `)`)
	for i := 0; i < 20 && !p.is(`document.activeElement.classList.contains('mrow')`); i++ {
		key(office, kb.Tab, false)
	}
	if !p.is(`document.activeElement.classList.contains('mrow')`) {
		t.Fatal("Tab should reach the task's members")
	}
	member := p.js(`document.activeElement.querySelector('b').textContent`)
	key(office, kb.Enter, false)
	waitUntil(t, 5*time.Second, "Enter on a member opens its window docked beside the task window", func() bool {
		return p.is(fmt.Sprintf(`!document.getElementById('chat').hidden && document.getElementById('chat').classList.contains('docked') && document.querySelector('#chat h2').textContent === %q`, member)) && focusIn(p, "#chat")
	})
	if n := ax(t, office, `document.getElementById('chat')`); n.Role != "region" || n.Name != member {
		t.Errorf("a docked member's window should be a region named by the member: %+v", n)
	}
	key(office, kb.Escape, false)
	if !p.is(fmt.Sprintf(`document.getElementById('chat').hidden && !document.getElementById('taskwin').hidden && document.activeElement.classList.contains('mrow') && document.activeElement.querySelector('b').textContent === %q`, member)) {
		t.Errorf("Esc should close the member's window and put the focus back on its row: %s", p.js(`document.activeElement.outerHTML.slice(0, 80)`))
	}
	key(office, kb.Escape, false)
	if !p.is(fmt.Sprintf(`document.getElementById('taskwin').hidden && document.activeElement?.dataset.room !== undefined && document.activeElement.textContent.includes(%q)`, task.ID)) {
		t.Errorf("Esc should close the task window and put the focus back on the room's button: %s", p.js(`document.activeElement.outerHTML.slice(0, 80)`))
	}

	// the launch dialog: its fields named by their labels, back to its button
	p.run(`document.getElementById('new-emp').focus()`)
	key(office, kb.Enter, false)
	waitUntil(t, 10*time.Second, "the launch dialog opens on its name field", func() bool {
		return p.is(`!document.getElementById('launch').hidden && document.activeElement === document.getElementById('launch-name')`)
	})
	if n := ax(t, office, `document.getElementById('launch-name')`); !strings.HasPrefix(n.Name, "Name") {
		t.Errorf("the name field should be named by its label: %+v", n)
	}
	named(p, "launch", `document.getElementById('launch')`)
	key(office, kb.Tab, true)
	if !focusIn(p, "#launch") {
		t.Error("Shift+Tab from the first field should stay in the dialog")
	}
	key(office, kb.Escape, false)
	if !p.is(`document.getElementById('launch').hidden && document.activeElement === document.getElementById('new-emp')`) {
		t.Errorf("Esc should close the launch dialog and put the focus back on its button: %s", p.js(`document.activeElement.outerHTML.slice(0, 80)`))
	}
	p.run(`document.getElementById('new-task').click()`)
	waitUntil(t, 10*time.Second, "the task dialog opens", func() bool { return p.is(`!document.getElementById('task').hidden`) })
	p.run(`document.querySelector('#task .add-adv').click()`)
	named(p, "task dialog", `document.getElementById('task')`)
	p.run(`document.querySelector('#task .cancel').click()`)

	// a new ask is said out loud
	b.post(t, tokens["bob"], "/api/ask", map[string]any{"session": "s-bob", "kind": "permission", "tool": "Bash", "input": map[string]any{"command": "make release"}})
	waitUntil(t, 10*time.Second, "the third ask is announced", func() bool {
		return p.is(`(a => !!a && a.getAttribute('aria-live') === 'polite' && a.textContent.includes('3'))(document.getElementById('announce'))`)
	})
	p.done()

	// ---------- HQ, Coworkers, Mission Control ----------
	for _, th := range []struct{ name, ready, row, opened string }{
		{"hq", `document.querySelectorAll('#kpis .kpi').length === 7`, `document.querySelector('#kpis .kpi[data-tab=people]')`, `!document.querySelector('section[data-tab=people]').hidden`},
		{"messenger", `document.querySelectorAll('#people .row').length === 5`, `[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('alice'))`, `!document.getElementById('chat').hidden`},
		{"mission", `document.querySelectorAll('#wall .tile').length >= 7`, `[...document.querySelectorAll('#wall .tile')].find(x => x.querySelector('.nm').textContent === 'alice')`, `!document.getElementById('chat').hidden`},
	} {
		ctx, closeTab := clockTab(t, browser, b.URL+"/t/"+th.name+"/?lang=en", 0)
		do(t, ctx, 10*time.Second, chromedp.EmulateViewport(1400, 860))
		q := &screens{t: t, ctx: ctx, lang: "a11y-" + th.name}
		waitUntil(t, 20*time.Second, th.name+": the page shows the team", func() bool { return q.is(th.ready) })
		named(q, "page", `document.body`)
		// the dark switch says whether it's on
		was := q.is(`document.getElementById('dark').getAttribute('aria-pressed') === 'true'`)
		q.run(`document.getElementById('dark').click()`)
		if q.is(`document.getElementById('dark').getAttribute('aria-pressed') === 'true'`) == was || ax(t, ctx, `document.getElementById('dark')`).Name == "" {
			t.Errorf("%s: the dark switch should be named and say whether it's on", th.name)
		}
		q.run(`document.getElementById('dark').click()`)
		// a row or a tile opens with Enter
		q.run(th.row + `.focus()`)
		key(ctx, kb.Enter, false)
		waitUntil(t, 5*time.Second, th.name+": Enter on "+th.row+" opens it", func() bool { return q.is(th.opened) })
		if th.name != "hq" {
			want := map[string]string{"messenger": "region", "mission": "dialog"}[th.name]
			if n := ax(t, ctx, `document.getElementById('chat')`); n.Role != want {
				t.Errorf("%s: the employee window should be a %s: %+v", th.name, want, n)
			}
			named(q, "chat", `document.getElementById('chat')`)
		}
		if th.name == "mission" { // Esc closes it, the focus back on the tile
			key(ctx, kb.Escape, false)
			if !q.is(`document.getElementById('chat').hidden && document.activeElement.classList.contains('tile')`) {
				t.Errorf("mission: Esc should close the window and put the focus back on its tile: %s", q.js(`document.activeElement.outerHTML.slice(0, 80)`))
			}
		}
		q.done()
		closeTab()
	}

	// ---------- the approval page ----------
	approve, closeApprove := clockTab(t, browser, b.URL+"/approve?lang=en", 0)
	a := &screens{t: t, ctx: approve, lang: "a11y-approve"}
	waitUntil(t, 20*time.Second, "the approval page lists the asks", func() bool { return a.is(`document.querySelectorAll('#list .card').length === 3`) })
	named(a, "page", `document.body`)
	if n := ax(t, approve, `document.querySelector('#list .card')`); n.Role != "group" || n.Name == "" {
		t.Errorf("an ask's card should be a named group: %+v", n)
	}
	a.done()
	closeApprove()
}
