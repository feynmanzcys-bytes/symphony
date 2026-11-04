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
		{"i": 1, "kind": "text", "text": "I'll run the tests first:\n\n```sh\n$ make test -j4 $CI 2>&1 | tee out.log  # the whole suite\n```\n\nThen I'll look at **each failure** in turn, starting from the router:\n\n" +
			"```go\n// the router's handler\nfunc route(p string) (int, error) {\n\tif len(p) > 64 {\n\t\treturn 0, nil\n\t}\n\treturn strconv.Atoi(p)\n}\n```\n\n```diff\n@@ -1,2 +1,2 @@\n-old line\n+new line\n```"}}})
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
	event("alice", "s-alice", map[string]any{"kind": "tool.start", "tool": "Grep", "summary": "FAIL", "toolUseId": "ts1", "agentId": "ag1"})
	event("alice", "s-alice", map[string]any{"kind": "tool.end", "tool": "Grep", "summary": "FAIL", "toolUseId": "ts1", "agentId": "ag1", "durationMs": 300})
	event("alice", "s-alice", map[string]any{"kind": "tool.start", "tool": "Bash", "summary": "make test", "toolUseId": "tu9"})
	event("bob", "s-bob", map[string]any{"kind": "session.usage", "usage": map[string]any{"context": map[string]any{"tokens": 190000, "window": 200000, "percent": 95}, "cost": map[string]any{"usd": 0.4}}})
	event("bob", "s-bob", map[string]any{"kind": "chat.stop", "turnId": "b1", "step": 0, "tokens": map[string]any{"input": 3000, "output": 400, "cacheRead": 0, "cacheWrite": 0, "model": "claude-sonnet-5-5"}})
	event("erin", "s-erin", map[string]any{"kind": "session.end", "reason": "exit"})
	b.post(t, tokens["carol"], "/api/ask", map[string]any{"session": "s-carol", "kind": "permission", "tool": "Bash", "reason": "Runs a build", "input": map[string]any{"command": "rm -rf build && npm run build"}})
	b.post(t, tokens["carol"], "/api/ask", map[string]any{"session": "s-carol", "kind": "question", "questions": []map[string]any{{"question": "Should the README cover the Windows build too?", "header": "Scope",
		"options": []map[string]any{{"label": "Yes", "description": "briefly"}, {"label": "No", "description": "later"}}}}})
	waitUntil(t, 10*time.Second, "the asks are pending", func() bool { return len(b.Hub.Snapshot().Asks) == 2 })

	// the task: alice leads, bob helps; they talk, and you say something too
	body, _ := json.Marshal(map[string]any{"title": "Write the CHANGELOG", "leader": true, "permissionMode": "", "lang": "en",
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
// element is the backgrounds of it and its ancestors composed with their alpha and opacity, over the browser's white.
// pass says what it measures:
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
    // what lies behind the page's own backgrounds: the browser's white (from: where the chain of backgrounds starts;
    // null for the root)
    function base(el) { return { from: null, under: [[255, 255, 255, 1]] } }
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
    const MARKS = [['.dot', 'backgroundColor', 'parent'], ['#taskwin .top .status .lamp', 'backgroundColor', 'parent']]
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

// TestContrast measures the colors of Coworkers, light and dark, and of the approval page the way WCAG 2.1 AA asks
// (no tokens): with the made-up office of a11yTeam, it opens the views that show text: the page, the employee window
// (a chat with every kind of item, the asks' cards, an employee off duty), the task window and a member's window, the
// launch and task dialogs, the settings and the employees page. Every piece of text must reach 4.5:1 against what it
// sits on (3:1 when large) and every mark in a state color 3:1 (contrastJS). It fails naming each that falls short,
// with its colors.
func TestContrast(t *testing.T) {
	b, _, _ := a11yTeam(t)
	browser := newBrowser(t)
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
	dialogs := []contrastView{
		{"launch dialog", `document.getElementById('new-emp').click()`, `!document.getElementById('launch').hidden && document.querySelectorAll('#launch select option').length > 1`, `#launch`, ``},
		// folded, the bypassPermissions warning under its line; unfolded, the fields, the warning under the select
		{"launch dialog, bypass", `(s => { s.value = 'bypassPermissions'; s.dispatchEvent(new Event('change', { bubbles:true })) })(document.querySelector('#launch [name=permissionMode]'))`, `!document.querySelector('#launch .warn.out').hidden && document.querySelector('#launch .warn.out').getClientRects().length > 0`, `#launch`, ``},
		{"launch dialog, unfolded", `document.querySelector('#launch .adv').open = true`, `document.querySelector('#launch .adv .warn').getClientRects().length > 0 && !document.querySelector('#launch .use').hidden`, `#launch`, `document.querySelector('#launch .cancel').click()`},
		{"task dialog", `document.getElementById('new-task').click()`, `!document.getElementById('task').hidden && document.querySelectorAll('#task .mem').length >= 4`, `#task`, ``},
		{"task dialog, advisor", `document.querySelector('#task .add-adv').click(); [...document.querySelectorAll('#task .mem input')].find(i => !i.disabled)?.click()`, `!!document.querySelector('#task .adv')`, `#task`, `document.querySelector('#task .cancel').click()`},
		// the settings, an entry of the error log unfolded (a page's error the test reports), the report taken (its
		// download a no-op)
		{"settings", `import('/symphony.js').then(m => m.reportError('ui', 'a made-up error, for the colors', 'detail line')); document.querySelector('.open-settings').click()`,
			`!document.getElementById('settings').hidden && !!document.querySelector('#settings .entry') && !!document.querySelector('#settings .about dd')`, `#settings`, ``},
		// what a new employee starts with: the red line under bypassPermissions and what saving said (set back after)
		{"settings, bypass", `(s => { s.value = 'bypassPermissions'; s.dispatchEvent(new Event('change', { bubbles:true })) })(document.getElementById('set-mode'))`,
			`document.getElementById('set-mode-warn').getClientRects().length > 0 && document.querySelector('#settings .defaults .said').textContent !== ''`, `#settings`,
			`(s => { s.value = ''; s.dispatchEvent(new Event('change', { bubbles:true })) })(document.getElementById('set-mode'))`},
		{"settings, entry", `HTMLAnchorElement.prototype.click = () => {}; document.querySelector('#settings .recorded').open = true; document.querySelector('#settings .entry').open = true; document.querySelector('#settings .errlog .report').click()`,
			`!!document.querySelector('#settings .entry[open] pre') && document.querySelector('#settings .errlog .saved').textContent !== ''`, `#settings`, `document.querySelector('#settings .close').click()`},
		{"error", `import('/ui/errors.js').then(m => m.showError('first line\nsecond line: what went wrong'))`, `(o => !!o && !o.hidden)(document.getElementById('oops'))`, `#oops`,
			`document.querySelector('#oops .actions button').click()`},
		// the employees page over the settings: the list, a row being edited (the warning under bypassPermissions), two
		// selected with the drop-downs over them (the warning there too), the import with an entry of each kind
		{"employees", `document.querySelector('.open-settings').click(); document.querySelector('#settings .people .all').click()`,
			`!document.getElementById('staff').hidden && document.querySelectorAll('#staff .emp').length === 5 && document.querySelectorAll('#staff .batch select[name=model] option').length > 2`, `#staff`, ``},
		{"employees, edited", `document.querySelector('#staff .emp .edit').click(); (s => { s.value = 'bypassPermissions'; s.dispatchEvent(new Event('change', { bubbles:true })) })(document.getElementById('staff-mode'))`,
			`!!document.getElementById('staff-editor') && !document.getElementById('staff-warn').hidden`, `#staff`, `document.querySelector('#staff-editor .cancel').click()`},
		{"employees, selected", `[...document.querySelectorAll('#staff .emp .pick')].slice(0, 2).forEach(b => b.click()); (s => { s.value = 'bypassPermissions'; s.dispatchEvent(new Event('change', { bubbles:true })) })(document.querySelector('#staff .batch select[name=permissionMode]'))`,
			`!document.getElementById('staff-batch-warn').hidden && !document.querySelector('#staff .batch .apply').disabled`, `#staff`,
			`(s => { s.value = '*'; s.dispatchEvent(new Event('change', { bubbles:true })) })(document.querySelector('#staff .batch select[name=permissionMode]')); document.getElementById('staff-all').click(); document.getElementById('staff-all').click()`},
		{"employees, import", `document.querySelector('#staff .tools .import').click(); (x => { x.value = '[{"name":"zed"},{"name":"alice","model":"haiku"},{"name":"bob"},{"name":"carol","model":"gpt"}]'; x.dispatchEvent(new Event('input')) })(document.getElementById('staff-import-text'))`,
			`document.querySelectorAll('#staff .importer .preview li').length === 4 && !!document.querySelector('#staff .importer .preview li.bad')`, `#staff`,
			`document.querySelector('#staff .importer .cancel').click(); document.querySelector('#staff .close').click(); document.querySelector('#settings .close').click()`},
	}
	// the steps between answers, unfolded as the settings can have them (so the tool rows in them are measured)
	steps := func(open string) contrastView {
		return contrastView{"steps", `localStorage.setItem('symphony.steps', 'open'); ` + open, `!document.getElementById('chat').hidden && document.querySelectorAll('#chat .steps > details[open]').length >= 2 && document.querySelector('#chat .steps .it.tool').checkVisibility()`,
			`#chat`, `localStorage.removeItem('symphony.steps')`}
	}

	// ---------- Coworkers: light and dark (picked in its settings) ----------
	scheme := func(p *screens, dark bool) bool {
		if !p.is(`!!document.getElementById('set-scheme-dark')`) {
			return false
		}
		p.run(fmt.Sprintf(`document.getElementById(%q).click()`, map[bool]string{false: "set-scheme-light", true: "set-scheme-dark"}[dark]))
		return p.is(`document.documentElement.hasAttribute('data-dark')`) == dark
	}
	themes := []struct {
		name, ready string
		views       []contrastView
	}{
		{"messenger", `document.querySelectorAll('#people .row').length === 5`, append([]contrastView{
			{"sidebar", ``, ``, `body`, ``},
			{"chat", `[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('alice')).click()`, `!document.getElementById('chat').hidden && document.querySelectorAll('#chat .flow .it').length >= 8 && !document.getElementById('profile').hidden`, `body`, ``},
			withTeardown(steps(`[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('bob')).click(); [...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('alice')).click()`), ``),
			{"asks", `[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('carol')).click()`, `document.querySelectorAll('#chat .flow .it.ask.carded .card').length === 2`, `body`, ``},
			{"off duty", `[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('erin')).click()`, `!document.getElementById('chat').hidden && !document.querySelector('#chat .offline').hidden`, `body`, ``},
			{"group", `document.querySelector('#groups .row').click()`, `!document.getElementById('taskwin').hidden && document.querySelectorAll('#taskwin .tmsg').length >= 3`, `body`, ``},
			{"member", `document.querySelector('#taskwin .mrow').click()`, `!document.getElementById('chat').hidden && !!document.querySelector('#profile .link[data-group]')`, `body`, ``},
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
		q.run(`document.getElementById('set-scheme-system').click()`) // (the pick is remembered for the next tab)
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

// TestNarrow opens Coworkers in a window 800 pixels wide and in one 320 wide (and the approval page in one 400 wide, a
// phone's, and one 320 wide) with the made-up office of a11yTeam (no tokens): the page doesn't scroll sideways, and
// the windows and dialogs fit in it, scrolling inside. Screenshots into e2e/artifacts/screens/narrow-*.
func TestNarrow(t *testing.T) {
	b, _, _ := a11yTeam(t)
	browser := newBrowser(t)
	check := func(p *screens, where string) {
		t.Helper()
		time.Sleep(200 * time.Millisecond)
		if bad := p.js(fitsJS); bad != "" {
			t.Errorf("%s %s:\n%s", p.lang, where, bad)
		}
	}
	// the employees page over the settings: its list, a row being edited, the import with what it would do
	staffOpen, staffReady := `document.querySelector('#settings .people .all').click()`, `!document.getElementById('staff').hidden && document.querySelectorAll('#staff .emp').length === 5`
	staffImport := `document.querySelector('#staff-editor .cancel').click(); document.querySelector('#staff .tools .import').click(); (x => { x.value = 'zed\nalice'; x.dispatchEvent(new Event('input')) })(document.getElementById('staff-import-text'))`
	type step struct{ name, setup, ready string }
	pages := []struct {
		theme string
		w, h  int64
		ready string
		steps []step
	}{
		{"messenger", 800, 600, `document.querySelectorAll('#people .row').length === 5`, []step{
			{"sidebar", ``, ``},
			{"chat", `[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('alice')).click()`, `!document.getElementById('chat').hidden`},
			{"asks", `[...document.querySelectorAll('#people .row')].find(r => r.textContent.includes('carol')).click()`, `document.querySelectorAll('#chat .flow .it.ask.carded .card').length === 2`},
			{"group", `document.querySelector('#groups .row').click()`, `!document.getElementById('taskwin').hidden`},
			{"member", `document.querySelector('#taskwin .mrow').click()`, `!document.getElementById('chat').hidden && !!document.querySelector('#profile .link[data-group]')`},
			{"launch", `document.getElementById('new-emp').click()`, `!document.getElementById('launch').hidden`},
			{"launch advanced", `document.querySelector('#launch .adv').open = true`, `document.querySelector('#launch .preview').getClientRects().length > 0`},
			{"task dialog", `document.querySelector('#launch .cancel').click(); document.getElementById('new-task').click()`, `!document.getElementById('task').hidden`},
			{"settings", `document.querySelector('#task .cancel').click(); document.querySelector('.open-settings').click()`, `!document.getElementById('settings').hidden && !!document.querySelector('#settings .about dd')`},
			{"employees", staffOpen, staffReady},
			{"employees edited", `document.querySelector('#staff .emp .edit').click()`, `!!document.getElementById('staff-editor')`},
			{"employees import", staffImport, `document.querySelectorAll('#staff .importer .preview li').length === 2`},
		}},
	}
	for _, pg := range pages {
		// a narrow window, then 320 pixels: a phone, or a window of 1280 zoomed to 400% (WCAG 2.1's reflow)
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
				// a member's folder and branch keep room to be read (a line of their own when the row is narrow), and
				// the launch dialog's buttons stay in sight however its fields scroll
				if bad := p.js(`[...document.querySelectorAll('#taskwin:not([hidden]) .where')].filter(w => w.clientWidth < 80).map(w => w.textContent + ': ' + w.clientWidth + 'px').join(', ')`); bad != "" {
					t.Errorf("%s %d %s: a member's folder and branch squeezed: %s", pg.theme, size[0], s.name, bad)
				}
				if !p.is(`(a => !a || !a.getClientRects().length || (r => (q => r.bottom <= q.bottom + 1 && r.top >= q.top - 1)(a.closest('.panel').getBoundingClientRect()))(a.getBoundingClientRect()))(document.querySelector('#launch:not([hidden]) .actions'))`) {
					t.Errorf("%s %d %s: the launch dialog's buttons are out of sight", pg.theme, size[0], s.name)
				}
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

// TestA11y walks Coworkers and the approval page with the keyboard and asks the accessibility tree what a screen
// reader hears, with the made-up office of a11yTeam (no tokens). The settings' gear is named and opens a dialog named
// Settings, where Tab stays and the colors are a named group, and Esc goes back to the gear; a coworker's row opens
// with Enter into a region named by the coworker, the conversation a region named after her, a subagent's tool call
// read apart from its tool, and the window keeps the focus when the button it was on goes; an ask's card is a group
// named by its top line; the group's window is a region named by its task, its team chat's mention picker a named
// listbox the box points into, and a member's row opens a region named by the member; the launch dialog opens on its
// name field, its fields named by their labels, Advanced settings a disclosure that says what the start will use, and
// Esc puts the focus back on its button; a new ask is announced; the connection banner is a polite status. Every
// control on every view has a name.
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

	// ---------- Coworkers ----------
	ctx, closeTab := clockTab(t, browser, b.URL+"/t/messenger/?lang=en", 0)
	do(t, ctx, 10*time.Second, chromedp.EmulateViewport(1400, 860))
	q := &screens{t: t, ctx: ctx, lang: "a11y-messenger"}
	waitUntil(t, 20*time.Second, "the page shows the team and the group", func() bool {
		return q.is(`document.querySelectorAll('#people .row').length === 5 && document.querySelectorAll('#groups .row').length === 1`)
	})
	named(q, "page", `document.body`)
	if !q.is(`(c => c.getAttribute('role') === 'status' && c.getAttribute('aria-live') === 'polite')(document.getElementById('conn'))`) {
		t.Error("the connection banner should be a polite status")
	}
	// the gear: named, it opens the settings, a dialog named by its heading, where Tab stays, the colors a named group
	// of named choices, and Esc goes back to the gear
	if n := ax(t, ctx, `document.querySelector('.open-settings')`); !strings.HasPrefix(n.Name, "Settings") { // (and says when the error log has news)
		t.Errorf("the settings' gear should be named Settings: %+v", n)
	}
	q.run(`document.querySelector('.open-settings').focus()`)
	key(ctx, kb.Enter, false)
	waitUntil(t, 5*time.Second, "the settings open, the focus in them", func() bool { return focusIn(q, "#settings") })
	if n := ax(t, ctx, `document.getElementById('settings')`); n.Role != "dialog" || n.Name != "Settings" {
		t.Errorf("the settings should be a dialog named Settings: %+v", n)
	}
	if n := ax(t, ctx, `document.querySelector('#settings .scheme')`); n.Role != "radiogroup" || n.Name != "Colors" {
		t.Errorf("light or dark should be a group named Colors: %+v", n)
	}
	for i := 0; i < 40; i++ {
		key(ctx, kb.Tab, false)
		if !focusIn(q, "#settings") {
			t.Errorf("Tab left the settings at its %d. press", i+1)
			break
		}
	}
	named(q, "settings", `document.getElementById('settings')`)
	key(ctx, kb.Escape, false)
	waitUntil(t, 5*time.Second, "Esc closes the settings, back on the gear", func() bool {
		return q.is(`document.getElementById('settings').hidden && document.activeElement === document.querySelector('.open-settings')`)
	})
	if n := ax(t, ctx, `document.getElementById('groups-count').parentElement`); n.Name != "Group chats 1" {
		t.Errorf("the group chats' heading should read its count apart: %q", n.Name)
	}

	// a row opens with Enter: alice's conversation, a region named by her; the conversation a region named after her
	// (the keyboard reaches it as it scrolls); a subagent's tool call read apart from its tool
	row := func(name string) string {
		return fmt.Sprintf(`document.querySelector('#people .row[data-account=%q]')`, id(name))
	}
	q.run(row("alice") + `.focus()`)
	key(ctx, kb.Enter, false)
	waitUntil(t, 5*time.Second, "Enter on alice's row opens her conversation", func() bool {
		return q.is(`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === 'alice'`)
	})
	if n := ax(t, ctx, `document.getElementById('chat')`); n.Role != "region" || n.Name != "alice" {
		t.Errorf("the employee window should be a region named by the employee: %+v", n)
	}
	named(q, "chat", `document.getElementById('chat')`)
	if n := ax(t, ctx, `document.querySelector('#chat .flow')`); n.Role != "region" || n.Name != "Conversation with alice" {
		t.Errorf("the conversation should be a region named after the employee (the keyboard reaches it): %+v", n)
	}
	if !q.is(`[...document.querySelectorAll('#chat .flow .it.tool')].some(r => r.textContent.includes('subagent Grep'))`) {
		t.Errorf("a subagent's tool call should read its badge apart from the tool: %s", q.js(`[...document.querySelectorAll('#chat .flow .it.tool')].map(r => r.textContent).join(' | ')`))
	}
	// the window keeps the focus when the button it was on goes (a screen reader lost its place when the Stop button went)
	say := func(name, session string, body map[string]any) {
		t.Helper()
		body["session"] = session
		b.post(t, tokens[name], "/api/event", body)
	}
	waitUntil(t, 5*time.Second, "alice's window shows her Stop button", func() bool { return q.is(`!!document.querySelector('#chat .head .btns .stop')`) })
	q.run(`document.querySelector('#chat .head .btns .stop').focus()`)
	say("alice", "s-alice", map[string]any{"kind": "tool.end", "tool": "Bash", "summary": "make test", "toolUseId": "tu9", "durationMs": 1200})
	say("alice", "s-alice", map[string]any{"kind": "turn.complete", "turnId": "a2"})
	waitUntil(t, 5*time.Second, "alice's turn ends, her Stop button goes", func() bool { return q.is(`!document.querySelector('#chat .head .btns .stop')`) })
	if !q.is(`document.activeElement !== document.body && document.getElementById('chat').contains(document.activeElement)`) {
		t.Errorf("the focus fell out of alice's window when her Stop button went: %s", q.js(`document.activeElement.outerHTML.slice(0, 80)`))
	}

	// an ask's card: a group named by its top line
	q.run(row("carol") + `.click()`)
	waitUntil(t, 5*time.Second, "carol's conversation shows her two cards", func() bool {
		return q.is(`document.querySelectorAll('#chat .flow .it.ask.carded .card').length === 2`)
	})
	if n := ax(t, ctx, `document.querySelector('#chat .flow .card')`); n.Role != "group" || !strings.Contains(n.Name, "Permission") {
		t.Errorf("an ask's card should be a group named by its top line: %+v", n)
	}
	named(q, "asks", `document.getElementById('chat')`)

	// the group: Enter on its row opens its window, a region named by the task; the team chat's mention picker: "@"
	// lists the members, the box points at the one picked, the arrows move it, Esc closes the picker alone; a member's
	// row opens its conversation, a region named by the member
	q.run(fmt.Sprintf(`document.querySelector('#groups .row[data-group=%q]').focus()`, task.ID))
	key(ctx, kb.Enter, false)
	waitUntil(t, 5*time.Second, "Enter on the group opens its window", func() bool { return q.is(`!document.getElementById('taskwin').hidden`) })
	if n := ax(t, ctx, `document.getElementById('taskwin')`); n.Role != "region" || !strings.HasPrefix(n.Name, task.ID) {
		t.Errorf("the group's window should be a region named by its task: %+v", n)
	}
	named(q, "task window", `document.getElementById('taskwin')`)
	box := `document.querySelector('#taskwin .tsay textarea')`
	q.run(box + `.focus()`)
	key(ctx, "@", false)
	waitUntil(t, 5*time.Second, "@ opens the mention picker, the box pointing at its first member", func() bool {
		return q.is(`(box => { const list = document.getElementById(box.getAttribute('aria-controls')), at = document.getElementById(box.getAttribute('aria-activedescendant'));` +
			` return !!list && !list.hidden && box.getAttribute('aria-autocomplete') === 'list' && box.getAttribute('aria-haspopup') === 'listbox' &&` +
			` !!at && list.contains(at) && at.getAttribute('role') === 'option' && at.getAttribute('aria-selected') === 'true' })(` + box + `)`)
	}, func() string {
		return q.js(`(box => JSON.stringify({ attrs: [...box.attributes].map(a => a.name + '=' + a.value), list: document.querySelector('#taskwin .tpick').outerHTML.slice(0, 400) }))(` + box + `)`)
	})
	if n := ax(t, ctx, `document.querySelector('#taskwin .tpick')`); n.Role != "listbox" || n.Name == "" {
		t.Errorf("the mention picker should be a named listbox: %+v", n)
	}
	if n := ax(t, ctx, box); n.Role != "textbox" || n.Name == "" {
		t.Errorf("the team chat's box should stay a named textbox: %+v", n)
	}
	picked := q.js(box + `.getAttribute('aria-activedescendant')`)
	key(ctx, kb.ArrowDown, false)
	if !q.is(fmt.Sprintf(`(box => box.getAttribute('aria-activedescendant') !== %q && document.getElementById(box.getAttribute('aria-activedescendant'))?.getAttribute('aria-selected') === 'true' && document.querySelectorAll('#taskwin .tpick [aria-selected=true]').length === 1)(`+box+`)`, picked)) {
		t.Errorf("ArrowDown should move the picker's choice, and the box's aria-activedescendant with it: %s", q.js(`document.querySelector('#taskwin .tpick').outerHTML.slice(0, 400)`))
	}
	key(ctx, kb.Escape, false)
	if !q.is(`document.querySelector('#taskwin .tpick').hidden && !` + box + `.hasAttribute('aria-activedescendant') && !document.getElementById('taskwin').hidden`) {
		t.Error("Esc should close the mention picker alone, the box pointing at nothing")
	}
	q.run(`(b => { b.value = ''; b.dispatchEvent(new Event('input')) })(` + box + `)`)
	q.run(`document.querySelector('#taskwin .mrow').focus()`)
	member := q.js(`document.activeElement.querySelector('b').textContent`)
	key(ctx, kb.Enter, false)
	waitUntil(t, 5*time.Second, "Enter on a member opens its conversation", func() bool {
		return q.is(fmt.Sprintf(`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === %q`, member))
	})
	if n := ax(t, ctx, `document.getElementById('chat')`); n.Role != "region" || n.Name != member {
		t.Errorf("a member's window should be a region named by the member: %+v", n)
	}

	// the launch dialog: its fields named by their labels, back to its button
	q.run(`document.getElementById('new-emp').focus()`)
	key(ctx, kb.Enter, false)
	waitUntil(t, 10*time.Second, "the launch dialog opens on its name field", func() bool {
		return q.is(`!document.getElementById('launch').hidden && document.activeElement === document.getElementById('launch-name')`)
	})
	if n := ax(t, ctx, `document.getElementById('launch-name')`); !strings.HasPrefix(n.Name, "Name") {
		t.Errorf("the name field should be named by its label: %+v", n)
	}
	named(q, "launch", `document.getElementById('launch')`)
	// the face: two radio groups, the shapes and the colors, each a named radio (Tab enters a group at the one picked
	// and leaves it with the next Tab)
	key(ctx, kb.Tab, false)
	if n := ax(t, ctx, `document.activeElement`); !q.is(`document.activeElement.matches('#launch .look-pick .shapes input:checked')`) || n.Role != "radio" || n.Name != "Round" {
		t.Errorf("Tab from the name should reach the shape picked, a radio named Round: %+v, %s", n, q.js(`document.activeElement.outerHTML.slice(0, 80)`))
	}
	if g := ax(t, ctx, `document.querySelector('#launch .look-pick .shapes')`); g.Role != "radiogroup" || g.Name != "Shape" {
		t.Errorf("the shapes should be a radio group named Shape: %+v", g)
	}
	key(ctx, kb.Tab, false)
	if n := ax(t, ctx, `document.activeElement`); !q.is(`document.activeElement.matches('#launch .look-pick .hues input:checked')`) || n.Role != "radio" || n.Name != "Red" {
		t.Errorf("the next Tab should reach the color picked, a radio named Red: %+v, %s", n, q.js(`document.activeElement.outerHTML.slice(0, 80)`))
	}
	// Advanced settings: the next stop, a disclosure named by what it is and what the start will use; Enter unfolds it
	// and Tab goes on into its fields
	key(ctx, kb.Tab, false)
	if n := ax(t, ctx, `document.querySelector('#launch .adv > summary')`); !q.is(`document.activeElement === document.querySelector('#launch .adv > summary')`) ||
		!strings.HasPrefix(n.Name, "Advanced settings own folder") {
		t.Errorf("Tab from the face should reach Advanced settings, named with what the start will use: %+v, %s", n, q.js(`document.activeElement.outerHTML.slice(0, 80)`))
	}
	key(ctx, kb.Enter, false)
	key(ctx, kb.Tab, false)
	if n := ax(t, ctx, `document.activeElement`); !q.is(`document.querySelector('#launch .adv').open && document.activeElement === document.getElementById('launch-dir')`) || !strings.HasPrefix(n.Name, "Working folder") {
		t.Errorf("Enter should unfold it, and Tab go on to the folder, named by its label: %+v, %s", n, q.js(`document.activeElement.outerHTML.slice(0, 80)`))
	}
	q.run(`document.getElementById('launch-name').focus()`)
	key(ctx, kb.Tab, true)
	if !focusIn(q, "#launch") {
		t.Error("Shift+Tab from the first field should stay in the dialog")
	}
	key(ctx, kb.Escape, false)
	if !q.is(`document.getElementById('launch').hidden && document.activeElement === document.getElementById('new-emp')`) {
		t.Errorf("Esc should close the launch dialog and put the focus back on its button: %s", q.js(`document.activeElement.outerHTML.slice(0, 80)`))
	}
	q.run(`document.getElementById('new-task').click()`)
	waitUntil(t, 10*time.Second, "the task dialog opens", func() bool { return q.is(`!document.getElementById('task').hidden`) })
	q.run(`document.querySelector('#task .add-adv').click()`)
	named(q, "task dialog", `document.getElementById('task')`)
	q.run(`document.querySelector('#task .cancel').click()`)

	// a new ask is said out loud
	b.post(t, tokens["bob"], "/api/ask", map[string]any{"session": "s-bob", "kind": "permission", "tool": "Bash", "input": map[string]any{"command": "make release"}})
	waitUntil(t, 10*time.Second, "the third ask is announced", func() bool {
		return q.is(`(a => !!a && a.getAttribute('aria-live') === 'polite' && a.textContent.includes('3'))(document.getElementById('announce'))`)
	})
	q.done()
	closeTab()

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

// withTeardown gives a view another way out (the views of a theme close their windows their own way).
func withTeardown(v contrastView, out string) contrastView {
	if out != "" {
		v.after = strings.TrimSpace(v.after + "; " + out)
	}
	return v
}
