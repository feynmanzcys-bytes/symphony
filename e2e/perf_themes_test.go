//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/performance"
	"github.com/chromedp/chromedp"
)

// TestPerfThemes (no tokens) measures the DOM theme, Coworkers (/t/messenger/), under the made-up load of TestPerf and
// TestPerfTasks: claude is a script that sits there, and the test reports every session in and streams for it the way
// the mod would. In a tab of its own:
//   - one busy session with its employee window open, opened from the theme's own list: TestPerf's 12 turns of 40
//     pieces of thinking and text (1.2 KiB each) and a tool call each, posted as fast as they go;
//   - then the team: 31 employees with a desk session each, 15 of them streaming, 8 tasks with 36 members streaming and
//     messaging their teammates (TestPerfTasks's streamer), the usage figures every 2 s, two asks waiting all along and
//     one that comes and goes every 3 s. The theme's heaviest views are measured one after the other: a task's group
//     chat at its 300 messages twice, and a coworker's conversation with her profile and computer;
//   - the stream cut and back (a resync) while it all streams.
//
// Each window records the gaps between frames (a probe of the test's own asks for every frame), the page's long tasks,
// what each pushed message costs its handler, the theme's draws (every frame callback it asks for), the main thread's
// share by kind and per push, and the DOM's size and the event listeners once a second; a CPU profile shows where the
// heaviest view's time goes. At checkpoints, with the load paused, the page is weighed after garbage collections: the
// heap, the DOM nodes, the elements in the document, the listeners. It fails on a page error or a CSP violation, a
// frame that waits over a second while updates flow (two in the busy window's burst), frames down to a trickle (under
// 20 a second), a theme's draws taking more than a frame and a half (25 ms at p95), and a page that keeps growing: the
// DOM growing all through a window, or the page grown from one window of its heaviest view to the next, or across the
// resync. Everything else is logged: search the output for "PERF". Each view is measured for 10 s, or
// SYMPHONY_PERF_SECS.
func TestPerfThemes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake claude is a shell script")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	requireBrowser(t)
	l := newThemeLoad(t)
	browser := newBrowser(t) // (after the bridge: it signs in to the bridges already started)
	t.Run("messenger", func(t *testing.T) { perfMessenger(t, l, browser) })
}

// What fails a window while updates flow.
const (
	themeFreeze = 1000.0 // ms: a frame that waited longer means a frozen page
	// ms: the same in the busy window, whose burst (2.4 MB in a quarter of a second, far beyond what the mod sends) the
	// shared employee window takes in with long tasks of a few hundred ms, 600 to 700 in the pixel office too (TestPerf);
	// more on a busy machine
	themeBurstFreeze = 2000.0
	themeMinFPS      = 20.0 // frames a second at least, over a window (a page with time to spare draws 60)
	themeDrawP95     = 25.0 // ms: a theme's draw (one a frame at most) may take a frame and a half, at p95, in a steady window
)

// themeSecs is how long a view is measured: 10 s, or SYMPHONY_PERF_SECS.
func themeSecs() time.Duration {
	if v, err := strconv.Atoi(os.Getenv("SYMPHONY_PERF_SECS")); err == nil && v > 0 {
		return time.Duration(v) * time.Second
	}
	return 10 * time.Second
}

const themeLoadText = "15 desk sessions and 36 task members streaming, the figures every 2 s, an alarm every 3 s"

// ---------- the made-up office ----------

// themeEmp is an employee of the made-up team: its account, its token and its desk session.
type themeEmp struct{ id, name, token, session string }

// themeLoad is the office every theme is measured in: the bridge, the team with its desk sessions, the tasks with their
// members.
type themeLoad struct {
	b        *testBridge
	emps     []themeEmp          // e00 to e29
	desks    []streamSession     // the ones streaming at their desks: e00 to e14
	teams    []streamSession     // the tasks' members, each with its teammates (who get its messages)
	members  []member            // the same members, with their tasks
	busy     map[string]themeEmp // each theme's busy employee (TestPerf's busy session), idle until its turn
	alarmers []themeEmp          // the employees an alarm comes and goes for: e25 to e27
}

func newThemeLoad(t *testing.T) *themeLoad {
	t.Helper()
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))
	b.Launcher.Claude = fakeClaude(t, 0)
	// the tasks end, and git is tidied up, before the temporary folders go
	t.Cleanup(func() {
		for _, k := range b.Hub.Tasks() {
			_ = b.Hub.EndTask(k.ID)
		}
		b.Hub.Stop()
	})
	app := folder(t, filepath.Join(t.TempDir(), "app"), 3, true)
	l := &themeLoad{b: b, busy: map[string]themeEmp{}}
	start := time.Now()
	models, efforts := []string{"haiku", "sonnet", "opus", "fable"}, []string{"low", "medium", "high", "xhigh", "max"}
	hire := func(name, session string, i int) themeEmp {
		acc, err := b.Hub.CreateAccount(name)
		if err != nil {
			t.Fatal(err)
		}
		e := themeEmp{acc.ID, acc.Name, acc.Token, session}
		mustSend(t, b.URL, e.token, "/api/event", map[string]any{"session": session, "kind": "session.start", "cwd": app,
			"model": "claude-" + models[i%4] + "-5-5", "effort": efforts[i%5], "engine": map[string]any{"version": "2.1.316"}, "mod": "1.2.0",
			"surface": "terminal", "repo": map[string]any{"root": app, "name": "app"}})
		mustSend(t, b.URL, e.token, "/api/event", map[string]any{"session": session, "kind": "session.title", "title": name + "-desk"})
		return e
	}
	for i := 0; i < 30; i++ {
		l.emps = append(l.emps, hire(fmt.Sprintf("e%02d", i), fmt.Sprintf("d%02d", i), i))
	}
	for i, theme := range []string{"messenger"} {
		l.busy[theme] = hire("busy-"+theme, "busy-"+theme, i)
	}
	for _, e := range l.emps[:15] {
		l.desks = append(l.desks, streamSession{token: e.token, session: e.session, name: e.name})
	}
	l.alarmers = l.emps[25:28]
	// two asks that wait all along: a permission with a long command, a question
	e28, e29 := l.emps[28], l.emps[29]
	mustSend(t, b.URL, e28.token, "/api/ask", map[string]any{"session": e28.session, "kind": "permission", "tool": "Bash",
		"input": map[string]any{"command": "rm -rf build && " + strings.Repeat("npm run build -- --verbose ", 20)}})
	mustSend(t, b.URL, e29.token, "/api/ask", map[string]any{"session": e29.session, "kind": "question", "questions": []map[string]any{{
		"question": "Ship it today or wait for the review?", "header": "Release", "multiSelect": false,
		"options": []map[string]any{{"label": "Today", "description": "as it is"}, {"label": "Wait", "description": "for the review"}}}}})
	// 8 tasks of 5 and 4 members: e00 to e09, then advisors
	next := 0
	for k := 0; k < 8; k++ {
		var accs, advs []string
		for j := 0; j < 5-k%2; j++ {
			if next < 10 {
				accs = append(accs, l.emps[next].id)
				next++
			} else {
				advs = append(advs, fmt.Sprintf("adv%d%d", k, j))
			}
		}
		_, ms, _ := makeTask(t, b, k, accs, advs, app)
		for _, m := range ms {
			reportIn(t, b, m)
		}
		l.members = append(l.members, ms...)
	}
	l.teams = teamsOf(l.members)
	// every task's chat near the pages' cap (each task's newest 300 messages)
	l.prefill(t, 280)
	// the whole load for a few seconds: the activity log full (its 500 lines), the chats and the figures under way
	ld := l.start()
	time.Sleep(5 * time.Second)
	ld.halt()
	if n := ld.fails(); n > 0 {
		t.Fatalf("%d posts failed while warming up", n)
	}
	t.Logf("PERF the office: %d employees (%d streaming at their desks, %d idle, 3 busy ones waiting for their theme's turn), 8 tasks with %d members; made in %v",
		len(l.emps)+len(l.busy), len(l.desks), len(l.emps)-len(l.desks), len(l.members), time.Since(start).Round(time.Millisecond))
	return l
}

// teamsOf is the members as sessions to stream for, each with the rest of its team.
func teamsOf(ms []member) (list []streamSession) {
	byTask := map[string][]streamSession{}
	var order []string
	for _, m := range ms {
		if byTask[m.task] == nil {
			order = append(order, m.task)
		}
		byTask[m.task] = append(byTask[m.task], streamSession{token: m.token, session: m.session, name: m.name})
	}
	for _, id := range order {
		team := byTask[id]
		for i, ss := range team {
			for j, o := range team {
				if i != j {
					ss.mates = append(ss.mates, o)
				}
			}
			list = append(list, ss)
		}
	}
	return list
}

// prefill gives every task n messages between its members, sent the way they send them (the tasks at once).
func (l *themeLoad) prefill(t *testing.T, n int) {
	t.Helper()
	byTask := map[string][]member{}
	for _, m := range l.members {
		byTask[m.task] = append(byTask[m.task], m)
	}
	var wg sync.WaitGroup
	var failed atomic.Int64
	for _, team := range byTask {
		wg.Add(1)
		go func(team []member) {
			defer wg.Done()
			for i := 0; i < n; i++ {
				from, to := team[i%len(team)], team[(i+1)%len(team)]
				code, _, _, err := send(l.b.URL, from.token, "/api/event", map[string]any{"session": from.session, "kind": "message.send", "to": to.name,
					"text": fmt.Sprintf("earlier, step %d from %s: done with my part of the tests, over to you", i, from.name), "delivered": true})
				if err != nil || code != 200 {
					failed.Add(1)
				}
			}
		}(team)
	}
	wg.Wait()
	if n := failed.Load(); n > 0 {
		t.Fatalf("%d messages failed to post", n)
	}
}

// ---------- the load ----------

// themeStreams is the team's load: TestPerfTasks's streamer (chat, tools, turns, messages between the members) and,
// beside it, the usage figures and the alarm. It pauses while the page is weighed (nothing new for it to digest then)
// and resumes after; pause, resume, halt and the counts are for the test's goroutine.
type themeStreams struct {
	b                     *testBridge
	all                   []streamSession
	st                    *streamer   // the streamer running, nil while paused (test goroutine only)
	ran                   []*streamer // every one that ran, for the counts (test goroutine only)
	paused                atomic.Bool
	stop                  chan struct{}
	once                  sync.Once
	wg                    sync.WaitGroup
	usage, alarms, failed atomic.Int64
}

func (l *themeLoad) start() *themeStreams {
	s := &themeStreams{b: l.b, all: append(append([]streamSession{}, l.desks...), l.teams...), stop: make(chan struct{})}
	post := func(token, path string, body any) []byte {
		code, _, out, err := send(l.b.URL, token, path, body)
		if err != nil || code != 200 {
			s.failed.Add(1)
			return nil
		}
		return out
	}
	// the figures, as the mod reports them (every 2 s at most): a response's tokens, then the context window, the rate
	// limits and the cost so far
	for k, ss := range s.all {
		s.wg.Add(1)
		go func(k int, ss streamSession) {
			defer s.wg.Done()
			select {
			case <-s.stop:
				return
			case <-time.After(time.Duration(k*37%2000) * time.Millisecond): // not all at once
			}
			tick := time.NewTicker(2 * time.Second)
			defer tick.Stop()
			for i := 0; ; i++ {
				if !s.paused.Load() {
					post(ss.token, "/api/event", map[string]any{"session": ss.session, "kind": "chat.stop", "turnId": fmt.Sprintf("u%d", i), "step": 0,
						"tokens": map[string]any{"input": 1500 + 10*k, "output": 200 + 40*(i%7), "cacheRead": 20000, "cacheWrite": 0}})
					pct := float64(5 + (7*k+3*i)%90)
					post(ss.token, "/api/event", map[string]any{"session": ss.session, "kind": "session.usage", "usage": map[string]any{
						"context":    map[string]any{"tokens": int(pct * 2000), "window": 200000, "percent": pct},
						"rateLimits": []map[string]any{{"kind": "five_hour", "percentUsed": float64((k + i) % 100)}, {"kind": "seven_day", "percentUsed": float64(30 + k%60)}},
						"cost":       map[string]any{"usd": 0.04 * float64((i+1)*(1+k%3))}}})
					s.usage.Add(1)
				}
				select {
				case <-s.stop:
					return
				case <-tick.C:
				}
			}
		}(k, ss)
	}
	// an alarm that comes and goes: an ask every 3 s for one of the alarmers, withdrawn 1.5 s later (also when the load
	// pauses or stops meanwhile)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for i := 0; ; i++ {
			var ask struct{ ID string }
			e := l.alarmers[i%len(l.alarmers)]
			if !s.paused.Load() {
				if out := post(e.token, "/api/ask", map[string]any{"session": e.session, "kind": "permission", "tool": "Bash",
					"input": map[string]any{"command": fmt.Sprintf("make deploy ENV=staging-%d", i)}}); out != nil {
					_ = json.Unmarshal(out, &ask)
					s.alarms.Add(1)
				}
			}
			stopped := false
			select {
			case <-s.stop:
				stopped = true
			case <-time.After(1500 * time.Millisecond):
			}
			if ask.ID != "" {
				post(e.token, "/api/cancel/"+ask.ID, nil)
			}
			if stopped {
				return
			}
			select {
			case <-s.stop:
				return
			case <-time.After(1500 * time.Millisecond):
			}
		}
	}()
	s.resume()
	return s
}

// pause stops the load's posts; resume starts them again.
func (s *themeStreams) pause() {
	s.paused.Store(true)
	if s.st != nil {
		s.st.halt()
		s.st = nil
	}
}

func (s *themeStreams) resume() {
	s.st = stream(s.b.URL, s.all)
	s.ran = append(s.ran, s.st)
	s.paused.Store(false)
}

func (s *themeStreams) halt() {
	s.once.Do(func() {
		s.pause()
		close(s.stop)
		s.wg.Wait()
	})
}

// fails is how many of the load's posts failed so far.
func (s *themeStreams) fails() int64 {
	n := s.failed.Load()
	for _, st := range s.ran {
		n += st.fails.Load()
	}
	return n
}

// String is what the load posted.
func (s *themeStreams) String() string {
	var deltas, tools, msgs int64
	var post dist
	for _, st := range s.ran {
		deltas, tools, msgs = deltas+st.deltas.Load(), tools+st.tools.Load(), msgs+st.msgs.Load()
		post = append(post, st.post...)
	}
	return fmt.Sprintf("%d chat deltas, %d tool events, %d messages (x2 events), %d usage reports (x2 events), %d alarms; event POST latency ms %v",
		deltas, tools, msgs, s.usage.Load(), s.alarms.Load(), post)
}

// ---------- the page ----------

// themeScript goes in before a theme's own: it collects the page's errors and CSP violations, times every frame
// callback the page asks for (a theme draws once a frame at most, and only when something changed) and the handling of
// every message from the event stream, by type, counts the stream's openings and the tones a page plays (an oscillator
// each), and runs a frame probe of its own: how long since the frame before, every frame, while measuring.
const themeScript = `
window.__errors = []
addEventListener('error', e => window.__errors.push(String(e.message) + ' @' + e.filename + ':' + e.lineno))
addEventListener('unhandledrejection', e => window.__errors.push('rejection: ' + String(e.reason && e.reason.stack || e.reason)))
addEventListener('securitypolicyviolation', e => window.__errors.push('csp: ' + e.violatedDirective + ' ' + e.blockedURI + ' @' + e.sourceFile + ':' + e.lineNumber))
window.__tp = { on:false, gaps:[], draws:[], msgs:{}, long:[], opens:0, tones:0 }
{
  const P = window.__tp, raf = window.requestAnimationFrame.bind(window)
  try {
    const A = window.BaseAudioContext || window.AudioContext, osc = A.prototype.createOscillator
    A.prototype.createOscillator = function (...a) { P.tones++; return osc.apply(this, a) }
  } catch {}
  window.requestAnimationFrame = cb => raf(ts => {
    const t0 = performance.now()
    try { cb(ts) } finally { if (P.on) P.draws.push(performance.now() - t0) }
  })
  let last = 0
  const probe = ts => { if (P.on) { if (last) P.gaps.push(ts - last); last = ts } else last = 0; raf(probe) }
  raf(probe)
  const ES = window.EventSource
  window.EventSource = class extends ES {
    constructor(...a) { super(...a); this.addEventListener('open', () => P.opens++) }
    get onmessage() { return super.onmessage }
    set onmessage(f) {
      super.onmessage = m => {
        const t0 = performance.now()
        try { f(m) } finally {
          if (P.on) {
            const d = m.data, k = d.startsWith('{"type":"snapshot"') ? (d.includes('"messages":[') ? 'full snapshot' : 'snapshot')
              : (d.slice(-40).match(/"type":"(\w+)"\}$/) || [, d.includes('"type":"usage","usage":') ? 'usage' : '?'])[1]
            const s = P.msgs[k] || (P.msgs[k] = { n:0, bytes:0, ms:0, max:0 })
            const ms = performance.now() - t0
            s.n++; s.bytes += d.length; s.ms += ms; s.max = Math.max(s.max, ms)
          }
        }
      }
    }
  }
  try { new PerformanceObserver(l => l.getEntries().forEach(e => P.on && P.long.push(Math.round(e.duration)))).observe({ entryTypes:['longtask'] }) } catch {}
}`

// themePage is a theme's tab with themeScript in.
type themePage struct {
	*screens
	errs  int // the page's errors already reported
	close context.CancelFunc
}

func openTheme(t *testing.T, browser context.Context, url, name string) *themePage {
	t.Helper()
	ctx, cancel := chromedp.NewContext(browser)
	t.Cleanup(cancel)
	if err := chromedp.Do(ctx); err != nil {
		t.Fatalf("opening a tab failed: %v", err)
	}
	do(t, ctx, 30*time.Second, chromedp.Func(func(ctx context.Context, tg *chromedp.Target) error {
		if _, err := cdp.Call(ctx, tg, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: themeScript}); err != nil {
			return err
		}
		_, err := cdp.Call(ctx, tg, performance.Enable, performance.EnableParams{})
		return err
	}), chromedp.EmulateViewport(1400, 860), chromedp.Navigate(url))
	return &themePage{screens: &screens{t: t, ctx: ctx, lang: "perf-" + name}, close: cancel}
}

func (p *themePage) num(expr string) float64 {
	p.t.Helper()
	return get(p.t, p.ctx, 10*time.Second, chromedp.Evaluate[float64](expr))
}

// newErrors is what the page threw, or the CSP blocked, since the last call.
func (p *themePage) newErrors() []string {
	p.t.Helper()
	errs := get(p.t, p.ctx, 5*time.Second, chromedp.Evaluate[[]string](fmt.Sprintf(`window.__errors.slice(%d)`, p.errs)))
	p.errs += len(errs)
	return errs
}

// finish fails the test on the page's errors since the last check and on keys a catalog lacks.
func (p *themePage) finish(what string) {
	p.t.Helper()
	if errs := p.newErrors(); len(errs) > 0 {
		p.t.Errorf("%s: the page threw or broke the CSP: %v", what, errs)
	}
	if miss := get(p.t, p.ctx, 5*time.Second, chromedp.Evaluate[[]string](`window.__i18nMissing || []`)); len(miss) > 0 {
		p.t.Errorf("%s: keys the catalogs lack: %v", what, miss)
	}
}

// ---------- a window ----------

// themeWin is one measuring window: what themeScript records, the page's metrics at its start and its end, and, once
// a second, the DOM's size and the event listeners.
type themeWin struct {
	p                       *themePage
	start                   time.Time
	m0, m1                  map[string]float64
	elems, nodes, listeners []float64
}

func (p *themePage) begin() *themeWin {
	p.t.Helper()
	do(p.t, p.ctx, 5*time.Second, chromedp.Evaluate[chromedp.Void](`Object.assign(window.__tp, { on:true, gaps:[], draws:[], msgs:{}, long:[] })`))
	w := &themeWin{p: p, start: time.Now()}
	w.sample()
	w.m0 = w.m1
	return w
}

func (w *themeWin) sample() {
	w.p.t.Helper()
	w.m1 = perfMetrics(w.p.t, w.p.ctx)
	w.nodes = append(w.nodes, w.m1["Nodes"])
	w.listeners = append(w.listeners, w.m1["JSEventListeners"])
	w.elems = append(w.elems, w.p.num(`document.getElementsByTagName('*').length`))
}

// hold goes on measuring for d, with a sample a second.
func (w *themeWin) hold(d time.Duration) {
	w.p.t.Helper()
	for end := time.Now().Add(d); time.Now().Before(end); {
		time.Sleep(min(time.Second, time.Until(end)))
		w.sample()
	}
}

// themeStats is what a window saw.
type themeStats struct {
	dur         time.Duration
	gaps, draws dist // ms between frames; ms per frame callback of the page's (its draws)
	long        []int
	msgs        map[string]struct{ N, Bytes, Max, Ms float64 }
	// samples, a second apart: the elements in the document, the DOM nodes and the event listeners alive (collected or not)
	elems, nodes, listeners []float64
	m0, m1                  map[string]float64
}

func (w *themeWin) end() themeStats {
	w.p.t.Helper()
	w.sample()
	raw := get(w.p.t, w.p.ctx, 10*time.Second, chromedp.Evaluate[string](`(P => { P.on = false; return JSON.stringify({ gaps:P.gaps, draws:P.draws, msgs:P.msgs, long:P.long }) })(window.__tp)`))
	var r struct {
		Gaps, Draws []float64
		Long        []int
		Msgs        map[string]struct{ N, Bytes, Max, Ms float64 }
	}
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		w.p.t.Fatal(err)
	}
	return themeStats{dur: time.Since(w.start), gaps: r.Gaps, draws: r.Draws, long: r.Long, msgs: r.Msgs,
		elems: w.elems, nodes: w.nodes, listeners: w.listeners, m0: w.m0, m1: w.m1}
}

func (s themeStats) pushes() (n, handlerMs float64) {
	for _, m := range s.msgs {
		n += m.N
		handlerMs += m.Ms
	}
	return n, handlerMs
}

func (s themeStats) fps() float64 { return float64(len(s.gaps)) / s.dur.Seconds() }

// perSec is how much of a metric the window added a second: a share of the time for the durations, a rate for counts.
func (s themeStats) perSec(metric string) float64 {
	return (s.m1[metric] - s.m0[metric]) / s.dur.Seconds()
}

// msPerPush is the main thread's time over the window for each message pushed: what an update costs the page, its
// handling, its draws, the style and layout they bring, everything.
func (s themeStats) msPerPush() float64 {
	n, _ := s.pushes()
	return 1000 * (s.m1["TaskDuration"] - s.m0["TaskDuration"]) / math.Max(n, 1)
}

func (s themeStats) String() string {
	n, handler := s.pushes()
	secs := s.dur.Seconds()
	draw := 0.0
	for _, d := range s.draws {
		draw += d
	}
	long := append([]int(nil), s.long...)
	sort.Sort(sort.Reverse(sort.IntSlice(long)))
	var b strings.Builder
	fmt.Fprintf(&b, "    %.0f pushes (%.0f/s); frames %.0f/s, gaps ms %v, >100 ms: %d, >250 ms: %d; long tasks %d (worst %v)\n",
		n, n/secs, s.fps(), s.gaps, s.gaps.over(100), s.gaps.over(250), len(long), long[:min(5, len(long))])
	fmt.Fprintf(&b, "    main thread busy %.0f%% (script %.0f%%, layout %.0f%%, style %.0f%%), %.2f ms a push; push handlers %.0f ms/s; the page's frame callbacks %.0f/s, ms %v, %.0f ms/s; layouts %.0f/s, style recalcs %.0f/s\n",
		100*s.perSec("TaskDuration"), 100*s.perSec("ScriptDuration"), 100*s.perSec("LayoutDuration"), 100*s.perSec("RecalcStyleDuration"), s.msPerPush(),
		handler/secs, float64(len(s.draws))/secs, s.draws, draw/secs, s.perSec("LayoutCount"), s.perSec("RecalcStyleCount"))
	fmt.Fprintf(&b, "    DOM elements %s; DOM nodes (alive, collected or not) %s; event listeners (alive) %s", spread(s.elems), spread(s.nodes), spread(s.listeners))
	var keys []string
	for k := range s.msgs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		m := s.msgs[k]
		fmt.Fprintf(&b, "\n    page handled %-13s %5.0f msgs, avg %6.0f B, handler avg %.2f ms max %.1f ms, total %.0f ms", k, m.N, m.Bytes/math.Max(m.N, 1), m.Ms/math.Max(m.N, 1), m.Max, m.Ms)
	}
	return b.String()
}

// spread is how a series of samples went: the first and the last, the lowest and the highest.
func spread(xs []float64) string {
	if len(xs) == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f -> %.0f (%.0f..%.0f)", xs[0], xs[len(xs)-1], slices.Min(xs), slices.Max(xs))
}

// check fails the test on what no theme may do while updates flow: a page error or a CSP violation, a frame that
// waited longer than freeze, frames down to a trickle; and in a steady window (the load's caps reached long before,
// nothing on the page that should grow) draws that take more than a frame and a half, and the DOM growing all through
// the window.
func (p *themePage) check(what string, s themeStats, steady bool, freeze float64) {
	t := p.t
	t.Helper()
	if errs := p.newErrors(); len(errs) > 0 {
		t.Errorf("%s: the page threw or broke the CSP: %v", what, errs)
	}
	if worst := s.gaps.max(); worst > freeze {
		t.Errorf("%s: a frame waited %.0f ms (frozen)", what, worst)
	}
	if fps := s.fps(); fps < themeMinFPS {
		t.Errorf("%s: only %.1f frames a second", what, fps)
	}
	if !steady {
		return
	}
	if d := s.draws.pct(.95); d > themeDrawP95 {
		t.Errorf("%s: the theme's draws take %.1f ms (p95), more than a frame and a half", what, d)
	}
	if third := len(s.elems) / 3; third >= 2 {
		if first, last := slices.Max(s.elems[:third]), slices.Min(s.elems[len(s.elems)-third:]); last > first*1.1+200 {
			t.Errorf("%s: the DOM grew all through the window: %s elements", what, spread(s.elems))
		}
	}
}

// ---------- checkpoints ----------

// themeMem is the page's size after garbage collections: the JS heap (MiB), the DOM nodes (in the document or not),
// the elements in the document, the event listeners.
type themeMem struct{ heap, nodes, elems, listeners float64 }

// mem weighs the page. With updates coming in, a page that parses 100 snapshots a second and rebuilds a list on every
// message has fresh garbage at any moment, so the load pauses first (weigh) and the page gets a second to settle.
func (p *themePage) mem() themeMem {
	p.t.Helper()
	time.Sleep(time.Second)
	// twice: what a collection finds dead is swept a little later, and the node count goes down only then (a new
	// collection finishes the sweeping first)
	pageMem(p.t, p.ctx)
	heap, m := pageMem(p.t, p.ctx)
	return themeMem{heap, m["Nodes"], p.num(`document.getElementsByTagName('*').length`), m["JSEventListeners"]}
}

// weigh is mem with the load paused meanwhile.
func (p *themePage) weigh(ld *themeStreams) themeMem {
	p.t.Helper()
	ld.pause()
	defer ld.resume()
	return p.mem()
}

func (m themeMem) String() string {
	return fmt.Sprintf("heap %.1f MiB, %.0f DOM nodes (%.0f elements in the document), %.0f listeners", m.heap, m.nodes, m.elems, m.listeners)
}

// grew fails the test when the page grew from a to b, checkpoints of the same view under the same load, by more than
// what comes and goes (weighed with the load paused, the same view stays within a few nodes and a tenth of a MiB):
// then it would go on growing. The heap may grow by heapMiB and a twentieth of a's.
func (p *themePage) grew(what string, a, b themeMem, heapMiB float64) {
	t := p.t
	t.Helper()
	if b.nodes > a.nodes*1.05+300 {
		t.Errorf("%s: DOM nodes grew from %.0f to %.0f", what, a.nodes, b.nodes)
	}
	if b.elems > a.elems*1.05+100 {
		t.Errorf("%s: the document's elements grew from %.0f to %.0f", what, a.elems, b.elems)
	}
	if b.listeners > a.listeners*1.05+20 {
		t.Errorf("%s: event listeners grew from %.0f to %.0f", what, a.listeners, b.listeners)
	}
	if b.heap > a.heap*1.05+heapMiB {
		t.Errorf("%s: the heap grew from %.1f to %.1f MiB", what, a.heap, b.heap)
	}
}

// profile samples the page's main thread for 5 s and logs where the time went.
func (p *themePage) profile(what string) {
	p.t.Helper()
	self, total, wall := cpuProfile(p.t, p.ctx, 5*time.Second)
	skip := func(k string) bool { return k == "(root)" || k == "(program)" || k == "(idle)" }
	p.t.Logf("PERF CPU profile, %s (%.0f ms sampled), total time (with callees):%s\n    self time:%s", what, wall, top(total, 14, skip), top(self, 10, skip))
}

// ---------- what every theme goes through ----------

// busyWindow is TestPerf's busy session in a theme: the window of the theme's busy employee, opened by open through the
// theme's own list, then 12 turns of 40 pieces of thinking and text (1.2 KiB each) and a tool call each, posted as fast
// as they go; it waits until the window shows the 48 items.
//
// A burst this big (about 2.4 MB in a quarter of a second) used to leave its tail in the browser until more came down
// the stream, in a quiet office the keepalive up to 15 s later; the bridge writes a comment a moment after a burst
// (streamNudge), so the page shows all of it within seconds, and fails here when it doesn't.
func (p *themePage) busyWindow(l *themeLoad, theme, open string) {
	t := p.t
	t.Helper()
	e := l.busy[theme]
	p.run(open)
	waitUntil(t, 10*time.Second, theme+": the busy employee's window shows its empty chat", func() bool {
		return p.is(fmt.Sprintf(`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === %q && !!document.querySelector('#chat .flow .empty')`, e.name))
	})
	piece := strings.Repeat("lorem ipsum dolor sit amet consectetur ", 30) // ~1.2 KiB
	ev := func(body map[string]any) {
		body["session"] = e.session
		mustSend(t, l.b.URL, e.token, "/api/event", body)
	}
	w := p.begin()
	start := time.Now()
	events := 0
	for turn := 0; turn < 12; turn++ {
		tid := fmt.Sprintf("t%d", turn)
		ev(map[string]any{"kind": "chat.user", "text": "prompt " + tid, "origin": "composer"})
		for i := 0; i < 40; i++ {
			ev(map[string]any{"kind": "chat.delta", "turnId": tid, "step": 0,
				"blocks": []map[string]any{{"i": 0, "kind": "thinking", "text": piece}, {"i": 1, "kind": "text", "text": piece}}})
		}
		ev(map[string]any{"kind": "chat.stop", "turnId": tid, "step": 0})
		ev(map[string]any{"kind": "tool.start", "tool": "Bash", "toolUseId": "tu" + tid, "summary": "go test ./..."})
		ev(map[string]any{"kind": "tool.end", "tool": "Bash", "toolUseId": "tu" + tid})
		events += 44
	}
	posted := time.Since(start)
	items := func() float64 { return p.num(`document.querySelectorAll('#chat .flow .it').length`) }
	stalled := ""
	waitUntil(t, 8*time.Second, theme+": all 48 items shown, the burst's tail too", func() bool { return items() == 48 },
		func() string { return p.js(`document.querySelectorAll('#chat .flow .it').length`) })
	shown := time.Since(start)
	w.hold(2 * time.Second)
	s := w.end()
	t.Logf("PERF %s busy window (%s: %d events posted in %v, as fast as they go): 48 items shown after %v%s\n%s",
		theme, e.name, events, posted.Round(time.Millisecond), shown.Round(time.Millisecond), stalled, s)
	p.check(theme+" busy window", s, false, themeBurstFreeze)
}

// view shows a view (show: what makes the theme show it), waits until it shows what it should (ready), lets it settle
// and measures it for the window's length.
func (p *themePage) view(theme, name, show, ready string) themeStats {
	t := p.t
	t.Helper()
	if show != "" {
		p.run(show)
	}
	waitUntil(t, 20*time.Second, theme+": "+name+" shows", func() bool { return p.is(ready) })
	time.Sleep(time.Second)
	w := p.begin()
	w.hold(themeSecs())
	s := w.end()
	t.Logf("PERF %s %s (%s):\n%s", theme, name, themeLoadText, s)
	p.check(theme+" "+name, s, true, themeFreeze)
	if !p.is(ready) {
		t.Errorf("%s: %s no longer shows what it should", theme, name)
	}
	return s
}

// heaviest measures a theme's heaviest view under the team's load twice, weighs the page after each window and fails
// when it grew from one to the next; it profiles the view, then cuts the page off the bridge and lets it back (a
// resync) while the load goes on, and checks the page after that too. lost is the load's posts the cut caught.
func (p *themePage) heaviest(l *themeLoad, ld *themeStreams, theme, name, show, ready string) (a, b, r themeMem, lost int64) {
	t := p.t
	t.Helper()
	p.view(theme, name+" #1", show, ready)
	a = p.weigh(ld)
	p.view(theme, name+" #2", "", ready)
	b = p.weigh(ld)
	p.grew(theme+" "+name+", from one window to the next", a, b, 1)
	time.Sleep(time.Second)
	p.profile(theme + " " + name)
	// the resync: the page cut off the bridge (its stream drops, its calls go unanswered) until the connection banner
	// shows, then let back: its stream opens again with a full snapshot, and what the theme fetched itself is fetched
	// again
	opens := p.num(`window.__tp.opens`)
	failed := ld.fails()
	l.b.cutPages(true)
	waitUntil(t, 20*time.Second, theme+": the connection banner shows", func() bool { return p.is(`!document.getElementById('conn').hidden`) })
	time.Sleep(time.Second)
	l.b.cutPages(false)
	w := p.begin()
	start := time.Now()
	waitUntil(t, 30*time.Second, theme+": the stream is back", func() bool {
		return p.is(fmt.Sprintf(`document.getElementById('conn').hidden && window.__tp.opens > %v`, opens))
	})
	back := time.Since(start)
	lost = ld.fails() - failed // the load's posts the cut caught on their way (it closes every connection)
	w.hold(themeSecs())
	s := w.end()
	t.Logf("PERF %s %s after the stream was cut and came back (back %v after; %d of the load's posts lost to the cut):\n%s", theme, name, back.Round(time.Millisecond), lost, s)
	p.check(theme+" "+name+" after the resync", s, false, themeFreeze)
	if !p.is(ready) {
		t.Errorf("%s: after the resync %s doesn't show what it should", theme, name)
	}
	screenshot(t, p.ctx, "perf-themes-"+theme+".png")
	r = p.weigh(ld)
	p.grew(theme+" "+name+", the resync", b, r, 2)
	return a, b, r, lost
}

// done stops the load and fails the test on the load's own failures (but the ones the cut caught), on the page's
// errors, on keys a catalog lacks; it logs what the load posted and the page's sizes at the checkpoints.
func (p *themePage) done(ld *themeStreams, theme string, lost int64, base, a, b, r themeMem) {
	t := p.t
	t.Helper()
	ld.halt()
	if n := ld.fails() - lost; n > 0 {
		t.Errorf("%s: %d of the load's posts failed", theme, n)
	}
	t.Logf("PERF %s posted by the load: %s", theme, ld)
	t.Logf("PERF %s page after GC, the load paused: %s before the load; %s after the heaviest view's 1st window; %s after its 2nd; %s after the resync", theme, base, a, b, r)
	p.finish(theme)
}

// ---------- Coworkers ----------

func perfMessenger(t *testing.T, l *themeLoad, browser context.Context) {
	p := openTheme(t, browser, l.b.URL+"/t/messenger/?lang=en", "messenger")
	defer p.close()
	waitUntil(t, 20*time.Second, "Coworkers lists the team and the groups", func() bool {
		return p.is(fmt.Sprintf(`document.querySelectorAll('#people .row').length === %d && document.querySelectorAll('#groups .row').length === 8`, len(l.emps)+len(l.busy)))
	})
	base := p.mem()
	// the busy session's conversation in the middle, opened from its row in the sidebar
	p.busyWindow(l, "messenger", fmt.Sprintf(`document.querySelector('#people .row[data-account=%q]').click()`, l.busy["messenger"].id))

	ld := l.start()
	defer ld.halt()
	// a task's group chat in the middle: its 300 messages (the cap), the members streaming and messaging; the members
	// on the right
	task := l.members[0].task
	a, b, r, lost := p.heaviest(l, ld, "messenger", "group chat "+task, fmt.Sprintf(`document.querySelector('#groups .row[data-group=%q]').click()`, task),
		`!document.getElementById('taskwin').hidden && document.getElementById('chat').hidden && document.querySelectorAll('#taskwin .tlist .tmsg').length === 300 && document.querySelectorAll('#taskwin .mrow').length === 5 && document.querySelectorAll('#profile .list .row').length === 5`)
	// a coworker's conversation: one streaming at her desk, with her profile and her computer (the activity log) beside it
	e := l.emps[10]
	p.view("messenger", "conversation", fmt.Sprintf(`document.querySelector('#people .row[data-account=%q]').click()`, e.id),
		fmt.Sprintf(`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === %q && document.querySelectorAll('#chat .flow .it').length > 5 && !document.getElementById('profile').hidden && !!document.querySelector('#profile .computer .recent')`, e.name))
	p.done(ld, "messenger", lost, base, a, b, r)
}
