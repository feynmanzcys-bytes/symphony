//go:build e2e

package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/heapprofiler"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/performance"
	"github.com/chromedp/cdproto/profiler"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// TestPerfTasks (no tokens) measures the page and the bridge with collaborative tasks: claude is a script that sits
// there and the test reports every session in and streams made-up activity the way the mod would.
//   - office: 30 employees and a task in every small room (up to 8), desk and member sessions streaming chat, tools and
//     messages; frame timing at pixel size steps 1 and 3, the cost of a frame and where it goes, the push sizes with and
//     without tasks, the page's heap, the name tags' overlaps. Fails on a page error or a frame frozen for 2 s.
//   - create: making and ending a 5-member task on a tiny repository, on one with 5,000 files and on a plain folder with
//     5,000 files, and the latency of other API calls meanwhile.
//   - cycles: 200 tasks made and ended in a row (SYMPHONY_PERF_CYCLES), the bridge's memory and state before and after.
//
// The numbers are logged (search the output for "PERF"); only gross failures fail the test.
func TestPerfTasks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake claude is a shell script")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	t.Run("office", perfTasksOffice)
	t.Run("create", perfTasksCreate)
	t.Run("cycles", perfTasksCycles)
}

// ---------- helpers ----------

// perfHTTP keeps connections open: thousands of posts a second would otherwise run out of ports.
var perfHTTP = &http.Client{Transport: &http.Transport{MaxIdleConns: 400, MaxIdleConnsPerHost: 400}, Timeout: 30 * time.Second}

// send posts like the mod does (with a token) or the page (without, with the page key), from any goroutine; it
// returns the status and how long the call took.
func send(base, token, path string, body any) (int, time.Duration, []byte, error) {
	data, _ := json.Marshal(body)
	var req *http.Request
	if token != "" {
		req, _ = http.NewRequest("POST", base+path, bytes.NewReader(data))
		req.Header.Set("authorization", "Bearer "+token)
	} else {
		req = pageReq("POST", base, path, bytes.NewReader(data))
	}
	req.Header.Set("content-type", "application/json")
	start := time.Now()
	res, err := perfHTTP.Do(req)
	if err != nil {
		return 0, time.Since(start), nil, err
	}
	out, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res.StatusCode, time.Since(start), out, nil
}

func mustSend(t *testing.T, base, token, path string, body any) []byte {
	t.Helper()
	code, _, out, err := send(base, token, path, body)
	if err != nil || code != 200 {
		t.Fatalf("POST %s: %d %v %s", path, code, err, out)
	}
	return out
}

func stateBytes(t *testing.T, base string) int {
	t.Helper()
	res, err := perfHTTP.Do(pageReq("GET", base, "/api/state", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return len(data)
}

// dist summarizes durations or sizes: p50, p95, max.
type dist []float64

func (d dist) pct(p float64) float64 {
	if len(d) == 0 {
		return 0
	}
	s := append(dist(nil), d...)
	sort.Float64s(s)
	return s[int(math.Min(float64(len(s)-1), math.Floor(p*float64(len(s)))))]
}
func (d dist) max() float64 { return d.pct(1) }
func (d dist) over(v float64) int {
	n := 0
	for _, x := range d {
		if x > v {
			n++
		}
	}
	return n
}
func (d dist) String() string {
	return fmt.Sprintf("n=%d p50=%.1f p95=%.1f max=%.1f", len(d), d.pct(.5), d.pct(.95), d.max())
}

// prober fires GET /api/state and POST /api/event (a bystander's tool call) every few milliseconds until stopped,
// for what the bridge does to other callers meanwhile.
type prober struct {
	stop          chan struct{}
	wg            sync.WaitGroup
	mu            sync.Mutex
	state, events dist
}

func probe(base, token, session string) *prober {
	p := &prober{stop: make(chan struct{})}
	p.wg.Add(2)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-p.stop:
				return
			default:
			}
			start := time.Now()
			if res, err := perfHTTP.Do(pageReq("GET", base, "/api/state", nil)); err == nil {
				_, _ = io.Copy(io.Discard, res.Body)
				res.Body.Close()
			}
			p.mu.Lock()
			p.state = append(p.state, float64(time.Since(start).Microseconds())/1000)
			p.mu.Unlock()
			time.Sleep(10 * time.Millisecond)
		}
	}()
	go func() {
		defer p.wg.Done()
		for i := 0; ; i++ {
			select {
			case <-p.stop:
				return
			default:
			}
			kind := "tool.start"
			if i%2 == 1 {
				kind = "tool.end"
			}
			_, d, _, _ := send(base, token, "/api/event", map[string]any{"session": session, "kind": kind, "tool": "Read", "toolUseId": fmt.Sprintf("p%d", i/2)})
			p.mu.Lock()
			p.events = append(p.events, float64(d.Microseconds())/1000)
			p.mu.Unlock()
			time.Sleep(10 * time.Millisecond)
		}
	}()
	return p
}

func (p *prober) done() (state, events dist) {
	close(p.stop)
	p.wg.Wait()
	return p.state, p.events
}

func perfGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

// folder makes a folder with files files in it (100 to a subfolder), a git repository with them committed if repo.
func folder(t *testing.T, dir string, files int, repo bool) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := strings.Repeat("some source code in a file of a middling project\n", 4)
	for i := 0; i < files; i++ {
		sub := filepath.Join(dir, fmt.Sprintf("pkg%03d", i/100))
		if i%100 == 0 {
			if err := os.MkdirAll(sub, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(sub, fmt.Sprintf("f%04d.txt", i)), []byte(fmt.Sprintf("%d\n%s", i, line)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("perf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if repo {
		perfGit(t, dir, "init", "-q", "-b", "main")
		perfGit(t, dir, "add", "-A")
		perfGit(t, dir, "commit", "-qm", "init")
	}
	return dir
}

func fakeClaude(t *testing.T, output int) string {
	t.Helper()
	script := "#!/bin/sh\n"
	if output > 0 { // a TUI writes a lot to its terminal; the bridge keeps the last 256 KiB of it
		script += fmt.Sprintf("head -c %d /dev/zero | tr '\\0' x\n", output)
	}
	script += "exec sleep 100000\n"
	fake := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return fake
}

type member struct {
	task, name, account, token, session, launch, worktree string
}

// makeTask posts a task and returns its members, with their tokens (advisors' accounts are made by the bridge).
func makeTask(t *testing.T, b *testBridge, room int, accounts []string, advisors []string, dir string) (string, []member, time.Duration) {
	t.Helper()
	var ms []map[string]any
	for _, a := range accounts {
		ms = append(ms, map[string]any{"account": a, "model": "opus", "effort": "max"})
	}
	for _, n := range advisors {
		ms = append(ms, map[string]any{"advisor": map[string]any{"name": n, "dir": dir}, "model": "sonnet", "effort": "high"})
	}
	start := time.Now()
	out := mustSend(t, b.URL, "", "/api/tasks", map[string]any{"room": room, "title": "Make the tests faster and write it up",
		"leader": true, "permissionMode": "bypassPermissions", "lang": "en", "members": ms})
	took := time.Since(start)
	var task bridge.Task
	if err := json.Unmarshal(out, &task); err != nil {
		t.Fatal(err)
	}
	var got []member
	for _, m := range task.Members {
		acc := b.Hub.Account(m.Account)
		if acc == nil {
			t.Fatalf("no account for member %s", m.Name)
		}
		got = append(got, member{task: task.ID, name: m.Name, account: m.Account, token: acc.Token, session: "t-" + m.Name, launch: m.Launch, worktree: m.Worktree})
	}
	return task.ID, got, took
}

// reportIn reports a member's session in, as the mod does once claude is up.
func reportIn(t *testing.T, b *testBridge, m member) {
	t.Helper()
	mustSend(t, b.URL, m.token, "/api/event", map[string]any{"session": m.session, "kind": "session.start", "cwd": m.worktree, "launch": m.launch, "task": m.task, "model": "opus", "effort": "max"})
	mustSend(t, b.URL, m.token, "/api/event", map[string]any{"session": m.session, "kind": "session.title", "task": m.task, "title": m.name})
}

func taskShown(b *testBridge, id string) bool {
	for _, k := range b.Hub.Tasks() {
		if k.ID == id {
			return true
		}
	}
	return false
}

// endTask ends a task and waits until it leaves the snapshot (the cleanup is done).
func endTask(t *testing.T, b *testBridge, id string) time.Duration {
	t.Helper()
	start := time.Now()
	mustSend(t, b.URL, "", "/api/tasks/"+id+"/end", map[string]any{})
	for taskShown(b, id) {
		if time.Since(start) > 2*time.Minute {
			data, _ := json.Marshal(b.Hub.Tasks())
			t.Fatalf("task %s still shown after 2 minutes: %s", id, data)
		}
		time.Sleep(5 * time.Millisecond)
	}
	return time.Since(start)
}

// ---------- the made-up activity ----------

type streamer struct {
	base string
	stop chan struct{}
	wg   sync.WaitGroup
	// what was posted, by kind; failed posts
	deltas, tools, msgs, fails atomic.Int64
	lat                        sync.Mutex
	post                       dist // event post latency, ms
}

type streamSession struct {
	token, session string
	mates          []streamSession // for members: the rest of the team, who get messages
	name           string
}

// stream makes each session look busy: a chat delta every 150 ms (as the mod batches them), a tool call every 1.5 s,
// a new turn every 9 s; a member also sends a teammate a message every 3 s or so.
func stream(base string, sessions []streamSession) *streamer {
	s := &streamer{base: base, stop: make(chan struct{})}
	text := strings.Repeat("streaming words of a model that thinks out loud ", 4) // ~200 bytes
	for k, ss := range sessions {
		s.wg.Add(1)
		go func(k int, ss streamSession) {
			defer s.wg.Done()
			time.Sleep(time.Duration(k*7%150) * time.Millisecond) // not all at once
			tick := time.NewTicker(150 * time.Millisecond)
			defer tick.Stop()
			ev := func(body map[string]any) {
				body["session"] = ss.session
				code, d, _, err := send(base, ss.token, "/api/event", body)
				if err != nil || code != 200 {
					s.fails.Add(1)
				}
				s.lat.Lock()
				s.post = append(s.post, float64(d.Microseconds())/1000)
				s.lat.Unlock()
			}
			for i := 0; ; i++ {
				select {
				case <-s.stop:
					return
				case <-tick.C:
				}
				turn := fmt.Sprintf("t%d", i/60)
				if i%60 == 0 {
					if i > 0 {
						ev(map[string]any{"kind": "chat.stop", "turnId": fmt.Sprintf("t%d", i/60-1), "step": 0})
					}
					ev(map[string]any{"kind": "chat.user", "text": "next step " + turn, "origin": "composer"})
					ev(map[string]any{"kind": "turn.start", "text": "next step " + turn})
				}
				kind := "text"
				if i%60 < 20 {
					kind = "thinking"
				}
				ev(map[string]any{"kind": "chat.delta", "turnId": turn, "step": 0, "blocks": []map[string]any{{"i": 0, "kind": kind, "text": text}}})
				s.deltas.Add(1)
				switch (i + k) % 10 {
				case 0:
					ev(map[string]any{"kind": "tool.start", "tool": "Bash", "toolUseId": fmt.Sprintf("%s-%d", ss.session, i), "summary": "go test ./internal/..."})
					s.tools.Add(1)
				case 1:
					ev(map[string]any{"kind": "tool.end", "tool": "Bash", "toolUseId": fmt.Sprintf("%s-%d", ss.session, i-1)})
					s.tools.Add(1)
				}
				if len(ss.mates) > 0 && (i+k)%20 == 5 {
					to := ss.mates[(i/20)%len(ss.mates)]
					body := fmt.Sprintf("step %d from %s: done with my part, over to you", i, ss.name)
					ev(map[string]any{"kind": "message.send", "to": to.name, "text": body, "delivered": true})
					code, _, _, err := send(base, to.token, "/api/event", map[string]any{"session": to.session, "kind": "message.receive",
						"text": fmt.Sprintf(`<cross-session-message from-name="%s">%s</cross-session-message>`, ss.name, body)})
					if err != nil || code != 200 {
						s.fails.Add(1)
					}
					s.msgs.Add(1)
				}
			}
		}(k, ss)
	}
	return s
}

func (s *streamer) halt() {
	close(s.stop)
	s.wg.Wait()
}

// sseCount reads /api/stream for d and sums the bytes and count of what comes, by type.
type sseStat struct{ n, bytes, max int }

func sseCount(t *testing.T, base string, d time.Duration) map[string]*sseStat {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	res, err := http.DefaultClient.Do(pageReq("GET", base, "/api/stream", nil).WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	stats := map[string]*sseStat{}
	first := true
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}
		data := line[6:]
		var head struct{ Type string }
		_ = json.Unmarshal(data, &head)
		kind := head.Type
		if first { // the full snapshot every subscriber gets first
			kind, first = "first snapshot", false
		}
		st := stats[kind]
		if st == nil {
			st = &sseStat{}
			stats[kind] = st
		}
		st.n++
		st.bytes += len(data)
		st.max = max(st.max, len(data))
	}
	return stats
}

func sseReport(stats map[string]*sseStat, d time.Duration) string {
	var keys []string
	total := 0
	for k, v := range stats {
		keys = append(keys, k)
		total += v.bytes
	}
	sort.Strings(keys)
	var b strings.Builder
	fmt.Fprintf(&b, "total %.1f KiB/s;", float64(total)/1024/d.Seconds())
	for _, k := range keys {
		v := stats[k]
		fmt.Fprintf(&b, " %s: %d (%.1f/s) avg %d B max %d B, %.1f KiB/s;", k, v.n, float64(v.n)/d.Seconds(), v.bytes/max(v.n, 1), v.max, float64(v.bytes)/1024/d.Seconds())
	}
	return b.String()
}

// ---------- the page ----------

// perfScript goes in before the page's own: it times each animation frame the page draws (one that draws onto the
// visible canvas) and each message from the event stream, by type, and collects the page's errors.
const perfScript = `
window.__warp = 0
window.__errors = []
addEventListener('error', e => window.__errors.push(String(e.message)))
addEventListener('unhandledrejection', e => window.__errors.push(String(e.reason)))
window.__perf = { on:false, draws:[], cost:[], msgs:{}, long:[] }
{
  const P = window.__perf, raf = window.requestAnimationFrame.bind(window)
  let drew = false
  const di = CanvasRenderingContext2D.prototype.drawImage
  CanvasRenderingContext2D.prototype.drawImage = function (...a) { if (this.canvas.isConnected) drew = true; return di.apply(this, a) }
  window.requestAnimationFrame = cb => raf(ts => {
    drew = false
    const t0 = performance.now()
    try { cb(ts) } finally { if (P.on && drew) { P.draws.push(ts); P.cost.push(performance.now() - t0) } }
  })
  const ES = window.EventSource
  window.EventSource = class extends ES {
    get onmessage() { return super.onmessage }
    set onmessage(f) {
      super.onmessage = m => {
        const t0 = performance.now()
        try { f(m) } finally {
          if (P.on) {
            const d = m.data, k = d.startsWith('{"type":"snapshot"') ? 'snapshot' : (d.slice(-40).match(/"type":"(\w+)"\}$/) || [, '?'])[1]
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

func perfTab(t *testing.T, browser context.Context, url string) context.Context {
	t.Helper()
	ctx, cancel := chromedp.NewContext(browser)
	t.Cleanup(cancel)
	if err := chromedp.Do(ctx); err != nil {
		t.Fatalf("opening a tab failed: %v", err)
	}
	do(t, ctx, 30*time.Second, chromedp.Func(func(ctx context.Context, tg *chromedp.Target) error {
		if _, err := cdp.Call(ctx, tg, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: perfScript}); err != nil {
			return err
		}
		_, err := cdp.Call(ctx, tg, performance.Enable, performance.EnableParams{})
		return err
	}), chromedp.Navigate(url))
	return ctx
}

func cdpRun[R any](t *testing.T, ctx context.Context, f func(ctx context.Context, tg *chromedp.Target) (R, error)) R {
	t.Helper()
	return get(t, ctx, 60*time.Second, chromedp.Action[R](f))
}

// pageMem is the page's heap after a garbage collection, and the DOM's size.
func pageMem(t *testing.T, ctx context.Context) (heapMiB float64, metrics map[string]float64) {
	t.Helper()
	cdpRun(t, ctx, func(ctx context.Context, tg *chromedp.Target) (cdp.Empty, error) {
		return cdp.Call(ctx, tg, heapprofiler.CollectGarbage, cdp.Empty{})
	})
	use := cdpRun(t, ctx, func(ctx context.Context, tg *chromedp.Target) (cdpruntime.GetHeapUsageResult, error) {
		return cdp.Call(ctx, tg, cdpruntime.GetHeapUsage, cdp.Empty{})
	})
	return use.UsedSize / (1 << 20), perfMetrics(t, ctx)
}

func perfMetrics(t *testing.T, ctx context.Context) map[string]float64 {
	t.Helper()
	res := cdpRun(t, ctx, func(ctx context.Context, tg *chromedp.Target) (performance.GetMetricsResult, error) {
		return cdp.Call(ctx, tg, performance.GetMetrics, cdp.Empty{})
	})
	m := map[string]float64{}
	for _, x := range res.Metrics {
		m[x.Name] = x.Value
	}
	return m
}

// cpuProfile samples the page's main thread for d and sums the time by function: self and total (with callees).
func cpuProfile(t *testing.T, ctx context.Context, d time.Duration) (self, total map[string]float64, wall float64) {
	t.Helper()
	cdpRun(t, ctx, func(ctx context.Context, tg *chromedp.Target) (cdp.Empty, error) {
		if _, err := cdp.Call(ctx, tg, profiler.Enable, cdp.Empty{}); err != nil {
			return cdp.Empty{}, err
		}
		if _, err := cdp.Call(ctx, tg, profiler.SetSamplingInterval, profiler.SetSamplingIntervalParams{Interval: 200}); err != nil {
			return cdp.Empty{}, err
		}
		return cdp.Call(ctx, tg, profiler.Start, cdp.Empty{})
	})
	time.Sleep(d)
	res := cdpRun(t, ctx, func(ctx context.Context, tg *chromedp.Target) (profiler.StopResult, error) {
		return cdp.Call(ctx, tg, profiler.Stop, cdp.Empty{})
	})
	p := res.Profile
	// the time between samples is put on the sample's node
	byNode := map[int64]float64{}
	for i, id := range p.Samples {
		if i+1 < len(p.TimeDeltas) {
			byNode[id] += float64(p.TimeDeltas[i+1]) / 1000
		}
	}
	nodes := map[int64]*profiler.ProfileNode{}
	parent := map[int64]int64{}
	for _, n := range p.Nodes {
		nodes[n.ID] = n
		for _, c := range n.Children {
			parent[c] = n.ID
		}
	}
	name := func(n *profiler.ProfileNode) string {
		f := n.CallFrame.FunctionName
		if f == "" {
			f = "(anonymous)"
		}
		if strings.HasPrefix(f, "(") {
			return f
		}
		return fmt.Sprintf("%s:%d", f, n.CallFrame.LineNumber+1)
	}
	self, total = map[string]float64{}, map[string]float64{}
	for id, ms := range byNode {
		self[name(nodes[id])] += ms
		seen := map[string]bool{}
		for n, ok := id, true; ok; n, ok = parent[n] {
			k := name(nodes[n])
			if !seen[k] {
				total[k] += ms
				seen[k] = true
			}
		}
	}
	return self, total, float64(p.EndTime-p.StartTime) / 1000
}

func top(m map[string]float64, n int, skip func(string) bool) string {
	type kv struct {
		k string
		v float64
	}
	var l []kv
	for k, v := range m {
		if skip == nil || !skip(k) {
			l = append(l, kv{k, v})
		}
	}
	sort.Slice(l, func(i, j int) bool { return l[i].v > l[j].v })
	var b strings.Builder
	for i, x := range l {
		if i == n {
			break
		}
		fmt.Fprintf(&b, "\n      %8.0f ms  %s", x.v, x.k)
	}
	return b.String()
}

type frameStats struct {
	gaps, cost dist
	long       []int
	msgs       map[string]struct{ N, Bytes, Max, Ms float64 }
}

// measureFrames records the page's drawn frames for d.
func measureFrames(t *testing.T, ctx context.Context, d time.Duration) frameStats {
	t.Helper()
	do(t, ctx, 5*time.Second, chromedp.Evaluate[chromedp.Void](`Object.assign(window.__perf, { on:true, draws:[], cost:[], msgs:{}, long:[] })`))
	time.Sleep(d)
	raw := get(t, ctx, 10*time.Second, chromedp.Evaluate[string](`(P => { P.on = false; return JSON.stringify({ draws:P.draws, cost:P.cost, msgs:P.msgs, long:P.long }) })(window.__perf)`))
	var r struct {
		Draws, Cost []float64
		Long        []int
		Msgs        map[string]struct{ N, Bytes, Max, Ms float64 }
	}
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatal(err)
	}
	fs := frameStats{cost: r.Cost, long: r.Long, msgs: r.Msgs}
	for i := 1; i < len(r.Draws); i++ {
		fs.gaps = append(fs.gaps, r.Draws[i]-r.Draws[i-1])
	}
	return fs
}

func (fs frameStats) String() string {
	sort.Sort(sort.Reverse(sort.IntSlice(fs.long)))
	var b strings.Builder
	fmt.Fprintf(&b, "frame gaps ms %v, >250 ms: %d; frame cost ms %v; long tasks %d (worst %v)", fs.gaps, fs.gaps.over(250), fs.cost, len(fs.long), fs.long[:min(5, len(fs.long))])
	var keys []string
	for k := range fs.msgs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		m := fs.msgs[k]
		fmt.Fprintf(&b, "\n    page handled %-8s %5.0f msgs, avg %6.0f B, handler avg %.2f ms max %.1f ms, total %.0f ms", k, m.N, m.Bytes/math.Max(m.N, 1), m.Ms/math.Max(m.N, 1), m.Max, m.Ms)
	}
	return b.String()
}

// tagOverlaps is how the name tags overlap right now: pairs of visible tags whose boxes cover each other by more than a
// quarter of the smaller one, and the tags partly off the window.
const tagOverlaps = `JSON.stringify((() => {
  const box = l => ({ l, r:l.getBoundingClientRect(), n:l.querySelector('.nm')?.textContent ?? l.textContent.slice(0, 12) })
  const tags = [...document.querySelectorAll('.emp-label')].filter(l => !l.hidden && l.offsetParent).map(box)
  const bubbles = [...document.querySelectorAll('.bubble')].map(box)
  const share = (a, b) => {
    const w = Math.min(a.right, b.right) - Math.max(a.left, b.left), h = Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top)
    return w <= 0 || h <= 0 ? 0 : w * h / Math.min(a.width * a.height, b.width * b.height)
  }
  let worst = 0, off = 0, truncated = 0
  const pairs = [], bubblePairs = [], covered = new Set()
  for (const a of tags) {
    if (a.r.left < 0 || a.r.top < 0 || a.r.right > innerWidth || a.r.bottom > innerHeight) off++
    const nm = a.l.querySelector('.nm'); if (nm && nm.scrollWidth > nm.clientWidth) truncated++
  }
  for (let i = 0; i < tags.length; i++) for (let j = i + 1; j < tags.length; j++) {
    const s = share(tags[i].r, tags[j].r)
    worst = Math.max(worst, s)
    if (s > 0.25) pairs.push(tags[i].n + '/' + tags[j].n + ' ' + Math.round(s * 100) + '%')
  }
  for (let i = 0; i < bubbles.length; i++) {
    for (let j = i + 1; j < bubbles.length; j++) if (share(bubbles[i].r, bubbles[j].r) > 0.25) bubblePairs.push(i + '/' + j)
    for (const t of tags) if (share(bubbles[i].r, t.r) > 0.5) covered.add(t.n)
  }
  return { tags:tags.length, members:tags.filter(t => t.l.classList.contains('member')).length, away:tags.filter(t => t.l.classList.contains('away')).length,
    pairs:pairs.length, worst:Math.round(worst * 100), which:pairs, off, truncated, bubbles:bubbles.length, bubblePairs:bubblePairs.length, tagsUnderBubbles:covered.size }
})())`

// tagJitter samples the tags' positions every 100 ms for d and counts jumps: a tag moving more than 12 px between two
// samples while its figure stands still (a storm of tags pushing each other around would show here).
const tagJitter = `new Promise(done => {
  const last = new Map(); let jumps = 0, samples = 0, moves = 0
  const iv = setInterval(() => {
    samples++
    for (const l of document.querySelectorAll('.emp-label')) {
      if (l.hidden) continue
      const r = l.getBoundingClientRect(), p = last.get(l)
      if (p) {
        const d = Math.hypot(r.left - p.x, r.top - p.y)
        if (d > 12) { if (l.dataset.moving === '1' || p.moving) moves++; else jumps++ }
      }
      last.set(l, { x:r.left, y:r.top, moving:l.dataset.moving === '1' })
    }
  }, 100)
  setTimeout(() => { clearInterval(iv); done(JSON.stringify({ samples, jumps, moves })) }, %d)
})`

// ---------- office: the page under load with a task in every small room ----------

func perfTasksOffice(t *testing.T) {
	requireBrowser(t)
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))
	b.Launcher.Claude = fakeClaude(t, 0)
	app := folder(t, filepath.Join(t.TempDir(), "app"), 3, true)
	secs := 20
	if v, err := strconv.Atoi(os.Getenv("SYMPHONY_PERF_SECS")); err == nil && v > 0 {
		secs = v
	}
	window := time.Duration(secs) * time.Second

	// 30 employees with a desk session each in the tiny repository: e00 to e14 busy, e15 to e29 idle (they wander)
	type emp struct{ id, name, token, session string }
	var emps []emp
	for i := 0; i < 30; i++ {
		acc, err := b.Hub.CreateAccount(fmt.Sprintf("e%02d", i))
		if err != nil {
			t.Fatal(err)
		}
		e := emp{acc.ID, acc.Name, acc.Token, fmt.Sprintf("d%02d", i)}
		emps = append(emps, e)
		models := []string{"haiku", "sonnet", "opus", "fable"}
		mustSend(t, b.URL, e.token, "/api/event", map[string]any{"session": e.session, "kind": "session.start", "cwd": app, "model": models[i%4], "effort": "high"})
	}
	var desks []streamSession
	for _, e := range emps[:15] {
		desks = append(desks, streamSession{token: e.token, session: e.session, name: e.name})
	}

	browser := newBrowser(t)
	office := perfTab(t, browser, b.URL+"/?lang=en")
	jsb := func(e string) bool { return get(t, office, 10*time.Second, chromedp.Evaluate[bool](e)) }
	js := func(e string) string { return get(t, office, 10*time.Second, chromedp.Evaluate[string](e)) }
	do(t, office, 5*time.Second, chromedp.Evaluate[chromedp.Void](`localStorage.setItem('symphony.pixels', '0')`))
	// the page shows the members sitting in their rooms at once when it loads after they came (no walking in)
	reload := func(members int) {
		do(t, office, 30*time.Second, chromedp.Reload())
		waitUntil(t, 30*time.Second, "the office shows everyone, the members in their rooms", func() bool {
			return jsb(fmt.Sprintf(`document.querySelectorAll('.emp-label:not(.member)').length === 30 && (tags => tags.length === %d && tags.every(l => !l.hidden && l.dataset.room === 'small' && l.dataset.moving === '0'))([...document.querySelectorAll('.emp-label.member')])`, members))
		}, func() string {
			return js(`[...document.querySelectorAll('.emp-label.member')].map(l => l.querySelector('.nm').textContent + ' ' + JSON.stringify(l.dataset)).join('\n')`)
		})
	}
	reload(0)
	var seats []int
	if err := json.Unmarshal([]byte(js(`JSON.stringify([...document.querySelectorAll('.room-plate')].map(p => +p.dataset.seats))`)), &seats); err != nil {
		t.Fatal(err)
	}
	rooms := min(8, len(seats))
	t.Logf("PERF layout: %d small rooms with seats %v (window 1400x860); 8 tasks go in the first %d", len(seats), seats, rooms)
	if rooms < 2 {
		t.Fatalf("too few small rooms: %v", seats)
	}
	check := func(what string, fs frameStats) {
		t.Helper()
		if errs := get(t, office, 5*time.Second, chromedp.Evaluate[[]string](`window.__errors`)); len(errs) > 0 {
			t.Errorf("page errors during %s: %v", what, errs)
		}
		if fs.gaps.max() > 2000 {
			t.Errorf("a frame froze for %.0f ms during %s", fs.gaps.max(), what)
		}
		if len(fs.gaps) < int(window.Seconds())*5/2 {
			t.Errorf("only %d frames drawn during %s", len(fs.gaps), what)
		}
	}
	cpu := func(a, b map[string]float64, d time.Duration) string {
		return fmt.Sprintf("main thread busy %.0f%% (script %.0f%%, layout %.0f%%, style %.0f%%); layouts %.0f/s",
			100*(b["TaskDuration"]-a["TaskDuration"])/d.Seconds(), 100*(b["ScriptDuration"]-a["ScriptDuration"])/d.Seconds(),
			100*(b["LayoutDuration"]-a["LayoutDuration"])/d.Seconds(), 100*(b["RecalcStyleDuration"]-a["RecalcStyleDuration"])/d.Seconds(),
			(b["LayoutCount"]-a["LayoutCount"])/d.Seconds())
	}
	// frames measures the page's frames for d while /api/stream is read alongside for the first 10 s
	frames := func(what string, d time.Duration, sse bool) {
		t.Helper()
		var stats map[string]*sseStat
		done := make(chan struct{})
		go func() {
			defer close(done)
			if sse {
				stats = sseCount(t, b.URL, 10*time.Second)
			}
		}()
		m0 := perfMetrics(t, office)
		fs := measureFrames(t, office, d)
		m1 := perfMetrics(t, office)
		<-done
		check(what, fs)
		t.Logf("PERF %s, detail %s, %v: %s\n    %s", what, js(`document.querySelector('canvas').dataset.detail`), d, fs, cpu(m0, m1, d))
		if sse {
			t.Logf("PERF %s: /api/state %d B; /api/stream over 10 s: %s", what, stateBytes(t, b.URL), sseReport(stats, 10*time.Second))
		}
	}
	skip := func(k string) bool { return k == "(root)" || k == "(program)" || k == "(idle)" }
	profile := func(what string) {
		self, total, wall := cpuProfile(t, office, 5*time.Second)
		t.Logf("PERF CPU profile %s (%.0f ms sampled), total time (with callees):%s\n    self time:%s", what, wall, top(total, 14, skip), top(self, 10, skip))
	}
	tags := func(what string) {
		jitter := get(t, office, 15*time.Second, chromedp.Evaluate[string](fmt.Sprintf(tagJitter, 5000), chromedp.EvalAwaitPromise))
		t.Logf("PERF name tags %s: %s; jitter over 5 s: %s", what, js(tagOverlaps), jitter)
	}

	// ---------- 0 tasks: the baseline ----------
	st := stream(b.URL, desks)
	time.Sleep(time.Second)
	frames("0 tasks, 15 desk sessions streaming, step 1", 10*time.Second, true)
	st.halt()

	// ---------- a task in each of 8 rooms: e00 to e09, then advisors ----------
	var members []member
	var created dist
	next := 0
	task := func(k int) {
		var accs, advs []string
		for j := 0; j < seats[k]; j++ {
			if next < 10 {
				accs = append(accs, emps[next].id)
				next++
			} else {
				advs = append(advs, fmt.Sprintf("adv%d%d", k, j))
			}
		}
		_, ms, took := makeTask(t, b, k, accs, advs, app)
		created = append(created, float64(took.Milliseconds()))
		for _, m := range ms {
			reportIn(t, b, m)
		}
		members = append(members, ms...)
	}
	for k := 0; k < rooms; k++ {
		task(k)
	}
	t.Logf("PERF made %d tasks with %d members on a tiny repo: POST /api/tasks ms %v", rooms, len(members), created)
	reload(len(members))
	// the members stream too, and message their teammates
	teams := func(ms []member) (list []streamSession) {
		byTask := map[string][]streamSession{}
		for _, m := range ms {
			byTask[m.task] = append(byTask[m.task], streamSession{token: m.token, session: m.session, name: m.name})
		}
		for _, team := range byTask {
			for i, ss := range team {
				for j, o := range team {
					if i != j {
						ss.mates = append(ss.mates, o)
					}
				}
				list = append(list, ss)
			}
		}
		return
	}
	all := append(append([]streamSession{}, desks...), teams(members)...)
	heap0, met0 := pageMem(t, office)
	st = stream(b.URL, all)
	time.Sleep(time.Second)
	what := fmt.Sprintf("%d tasks, %d sessions streaming (15 desk + %d members)", rooms, len(all), len(members))
	frames(what+", step 1", window, true)
	snap := b.Hub.Snapshot()
	part := func(v any) int { data, _ := json.Marshal(v); return len(data) }
	t.Logf("PERF   a snapshot's parts: tasks %d B (%d), sessions %d B (%d), launches %d B (%d), accounts %d B (%d), asks+messages+rest %d B",
		part(snap.Tasks), len(snap.Tasks), part(snap.Sessions), len(snap.Sessions), part(snap.Launches), len(snap.Launches), part(snap.Accounts), len(snap.Accounts),
		part(snap)-part(snap.Tasks)-part(snap.Sessions)-part(snap.Launches)-part(snap.Accounts))
	heap1, met1 := pageMem(t, office)
	enc := time.Now()
	for i := 0; i < 50; i++ {
		_, _ = json.Marshal(b.Hub.Snapshot())
	}
	t.Logf("PERF   building and encoding a snapshot takes %.2f ms (the bridge does it once per push, for all pages)", float64(time.Since(enc).Microseconds())/1000/50)
	tags("at step 1")
	screenshot(t, office, "perf-tasks-step1.png")
	do(t, office, 5*time.Second, chromedp.Evaluate[chromedp.Void](`document.querySelector('.pixels button[data-level="2"]').click()`))
	time.Sleep(time.Second)
	frames(what+", step 3", window, false)
	screenshot(t, office, "perf-tasks-step3.png")
	// a page that stalls for a second (a long task, a busy machine) while all this streams: what the bridge pushed
	// and what the page got, side by side
	do(t, office, 5*time.Second, chromedp.Evaluate[chromedp.Void](`Object.assign(window.__perf, { on:true, msgs:{} }); setTimeout(() => { const t = performance.now(); while (performance.now() - t < 1000); }, 1500)`))
	got := sseCount(t, b.URL, 6*time.Second)
	time.Sleep(500 * time.Millisecond)
	pageGot := js(`(P => { P.on = false; return JSON.stringify(Object.fromEntries(Object.entries(P.msgs).map(([k, v]) => [k, v.n]))) })(window.__perf)`)
	bridgeGot := map[string]int{}
	for k, v := range got {
		bridgeGot[k] = v.n
	}
	t.Logf("PERF page stalled for 1 s in a 6 s window: a steady reader of /api/stream got %v, the page got %s", bridgeGot, pageGot)
	profile("step 3, " + what)
	do(t, office, 5*time.Second, chromedp.Evaluate[chromedp.Void](`document.querySelector('.pixels button[data-level="0"]').click()`))
	time.Sleep(time.Second)
	profile("step 1, " + what)
	heap2, met2 := pageMem(t, office)
	t.Logf("PERF page heap after GC: %.1f MiB before streaming, %.1f MiB after %v, %.1f MiB after ~%v; DOM nodes %.0f -> %.0f -> %.0f, listeners %.0f -> %.0f -> %.0f",
		heap0, heap1, window, heap2, 2*window+20*time.Second, met0["Nodes"], met1["Nodes"], met2["Nodes"], met0["JSEventListeners"], met1["JSEventListeners"], met2["JSEventListeners"])

	// ---------- every room full (as many member figures as there are seats), for the planner and the tags ----------
	before := len(members)
	for k := rooms; k < len(seats); k++ {
		task(k)
	}
	if len(members) > before {
		reload(len(members))
		st2 := stream(b.URL, teams(members[before:]))
		time.Sleep(time.Second)
		frames(fmt.Sprintf("%d tasks, %d members (every room full), %d sessions streaming, step 1", len(seats), len(members), len(all)+len(members)-before), 10*time.Second, false)
		profile(fmt.Sprintf("step 1, every room full (%d members)", len(members)))
		tags(fmt.Sprintf("with every room full (%d members)", len(members)))
		screenshot(t, office, "perf-tasks-full.png")
		st2.halt()
		st.fails.Add(st2.fails.Load())
	}
	st.halt()
	t.Logf("PERF posted %d chat deltas, %d tool events, %d messages (x2 events) by the 8-task streamer, %d posts failed; event POST latency ms %v",
		st.deltas.Load(), st.tools.Load(), st.msgs.Load(), st.fails.Load(), st.post)
	if st.fails.Load() > 0 {
		t.Errorf("%d posts failed", st.fails.Load())
	}

	// ending them all at once
	start := time.Now()
	ids := map[string]bool{}
	for _, m := range members {
		ids[m.task] = true
	}
	for id := range ids {
		mustSend(t, b.URL, "", "/api/tasks/"+id+"/end", map[string]any{})
	}
	waitUntil(t, 120*time.Second, "every task ended", func() bool { return len(b.Hub.Tasks()) == 0 })
	t.Logf("PERF ended %d tasks at once: all gone from the snapshot after %v", len(ids), time.Since(start).Round(time.Millisecond))
	if errs := get(t, office, 5*time.Second, chromedp.Evaluate[[]string](`window.__errors`)); len(errs) > 0 {
		t.Errorf("page errors: %v", errs)
	}
}

// ---------- create: making and ending a task on small and big folders ----------

func perfTasksCreate(t *testing.T) {
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))
	b.Launcher.Claude = fakeClaude(t, 0)
	work := t.TempDir()
	start := time.Now()
	cases := []struct{ name, dir string }{
		{"tiny repo", folder(t, filepath.Join(work, "tiny"), 3, true)},
		{"repo, 5000 files", folder(t, filepath.Join(work, "big"), 5000, true)},
		{"plain folder, 5000 files", folder(t, filepath.Join(work, "plain"), 5000, false)},
	}
	other := folder(t, filepath.Join(work, "other"), 3, true)
	t.Logf("PERF made the folders in %v", time.Since(start).Round(time.Millisecond))
	// a bystander whose tool calls and the office's state are timed meanwhile, and the employees
	by, _ := b.Hub.CreateAccount("bystander")
	mustSend(t, b.URL, by.Token, "/api/event", map[string]any{"session": "by", "kind": "session.start", "cwd": other})
	// many sessions in the office, so /api/state is not trivially small
	for i := 0; i < 30; i++ {
		acc, _ := b.Hub.CreateAccount(fmt.Sprintf("x%02d", i))
		mustSend(t, b.URL, acc.Token, "/api/event", map[string]any{"session": fmt.Sprintf("x%02d", i), "kind": "session.start", "cwd": other})
	}
	employ := func(prefix, dir string, n int) []string {
		var ids []string
		for i := 0; i < n; i++ {
			acc, err := b.Hub.CreateAccount(fmt.Sprintf("%s%d", prefix, i))
			if err != nil {
				t.Fatal(err)
			}
			mustSend(t, b.URL, acc.Token, "/api/event", map[string]any{"session": fmt.Sprintf("%s%d", prefix, i), "kind": "session.start", "cwd": dir})
			ids = append(ids, acc.ID)
		}
		return ids
	}
	p := probe(b.URL, by.Token, "by")
	time.Sleep(time.Second)
	bs, be := p.done()
	t.Logf("PERF baseline with nothing going on: /api/state ms %v; event POST ms %v", bs, be)
	spare := employ("o", other, 2)
	for i, c := range cases {
		accs := employ(fmt.Sprintf("c%d-", i), c.dir, 5)
		p := probe(b.URL, by.Token, "by")
		id, ms, took := makeTask(t, b, i, accs, nil, c.dir)
		ps, pe := p.done()
		t.Logf("PERF %-26s POST /api/tasks (5 members) %6d ms; meanwhile /api/state ms %v, event POST ms %v", c.name+":", took.Milliseconds(), ps, pe)
		for _, m := range ms {
			reportIn(t, b, m)
		}
		// some work in each worktree, so the end has something to commit (and, in the plain folder, to merge)
		for j, m := range ms {
			if err := os.WriteFile(filepath.Join(m.worktree, fmt.Sprintf("work-%d.txt", j)), []byte("done\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		p = probe(b.URL, by.Token, "by")
		var otherTook time.Duration
		var otherID string
		var otherMs []member
		done := make(chan time.Duration, 1)
		go func() { done <- endTask(t, b, id) }()
		if c.name == "plain folder, 5000 files" { // a task on another, tiny repository, made while this one is cleaned up
			time.Sleep(20 * time.Millisecond)
			otherID, otherMs, otherTook = makeTask(t, b, 7, spare, nil, other)
		}
		endTook := <-done
		ps, pe = p.done()
		t.Logf("PERF %-26s end until it leaves the snapshot %6d ms; meanwhile /api/state ms %v, event POST ms %v", c.name+":", endTook.Milliseconds(), ps, pe)
		if otherID != "" {
			t.Logf("PERF   a 2-member task on another tiny repo, POSTed 20 ms into that cleanup, took %d ms to make", otherTook.Milliseconds())
			for _, m := range otherMs {
				reportIn(t, b, m)
			}
			endTask(t, b, otherID)
		}
	}
	if _, err := os.Stat(filepath.Join(cases[2].dir, ".git")); err != nil {
		t.Error("the plain folder should keep the repository made for the task")
	}
	for j := 0; j < 5; j++ {
		if _, err := os.Stat(filepath.Join(cases[2].dir, fmt.Sprintf("work-%d.txt", j))); err != nil {
			t.Errorf("the members' work should be merged into the plain folder: %v", err)
		}
	}
}

// ---------- cycles: many tasks made and ended, looking for what stays behind ----------

func rssKiB() int {
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()
	if err != nil {
		return -1
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n
}

func perfTasksCycles(t *testing.T) {
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))
	// claude writes 128 KiB to its terminal before it sits there, as a real one fills its screen many times over
	b.Launcher.Claude = fakeClaude(t, 128<<10)
	app := folder(t, filepath.Join(t.TempDir(), "app"), 3, true)
	cycles := 200
	if v, err := strconv.Atoi(os.Getenv("SYMPHONY_PERF_CYCLES")); err == nil && v > 0 {
		cycles = v
	}
	var accs []string
	for i := 0; i < 2; i++ {
		acc, _ := b.Hub.CreateAccount(fmt.Sprintf("w%d", i))
		mustSend(t, b.URL, acc.Token, "/api/event", map[string]any{"session": fmt.Sprintf("w%d", i), "kind": "session.start", "cwd": app})
		accs = append(accs, acc.ID)
	}
	var launches []string
	look := func(when string) {
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		snap := b.Hub.Snapshot()
		terms := 0
		for _, id := range launches {
			if b.Launcher.Terms.Get(id) != nil {
				terms++
			}
		}
		tasksJSON, _ := os.Stat(filepath.Join(b.Data, "tasks.json"))
		size := int64(0)
		if tasksJSON != nil {
			size = tasksJSON.Size()
		}
		t.Logf("PERF %-18s RSS %6d KiB, Go heap %6.1f MiB, goroutines %4d; snapshot %7d B: %d accounts, %d sessions, %d launches, %d tasks; ended terminals still held %d of %d; tasks.json %d B",
			when+":", rssKiB(), float64(ms.HeapAlloc)/(1<<20), runtime.NumGoroutine(), stateBytes(t, b.URL), len(snap.Accounts), len(snap.Sessions), len(snap.Launches), len(snap.Tasks), terms, len(launches), size)
	}
	look("before")
	var made, ended dist
	start := time.Now()
	for i := 0; i < cycles; i++ {
		// one advisor each time too: its account is made and deleted with the task
		id, ms, took := makeTask(t, b, i%8, accs, []string{"adv"}, app)
		made = append(made, float64(took.Microseconds())/1000)
		for _, m := range ms {
			reportIn(t, b, m)
			mustSend(t, b.URL, m.token, "/api/event", map[string]any{"session": m.session, "kind": "chat.delta", "turnId": "t", "step": 0,
				"blocks": []map[string]any{{"i": 0, "kind": "text", "text": strings.Repeat("x", 2000)}}})
			launches = append(launches, m.launch)
			// its terminal has taken in what claude wrote (its history: Screen is cut short)
			deadline := time.Now().Add(5 * time.Second)
			written := func(term *bridge.Term) int { history, _, detach := term.Attach(0, 0); detach(); return len(history) }
			for term := b.Launcher.Terms.Get(m.launch); term != nil && written(term) < 128<<10 && time.Now().Before(deadline); {
				time.Sleep(5 * time.Millisecond)
			}
		}
		ended = append(ended, float64(endTask(t, b, id).Microseconds())/1000)
		if i+1 == cycles/2 {
			look(fmt.Sprintf("after %d", i+1))
		}
	}
	elapsed := time.Since(start)
	look(fmt.Sprintf("after %d", cycles))
	first, last := made[:min(20, len(made))], made[max(0, len(made)-20):]
	t.Logf("PERF %d cycles in %v: make ms %v (first 20 %v, last 20 %v); end ms %v", cycles, elapsed.Round(time.Millisecond), made, first, last, ended)
	// tasks.json is encoded whole, under the hub's lock, several times for every task made and ended
	if data, err := os.ReadFile(filepath.Join(b.Data, "tasks.json")); err == nil {
		var f struct {
			Next  int            `json:"next"`
			Tasks []*bridge.Task `json:"tasks"`
		}
		if err := json.Unmarshal(data, &f); err == nil {
			enc := time.Now()
			for i := 0; i < 20; i++ {
				_, _ = json.MarshalIndent(f, "", "  ")
			}
			t.Logf("PERF   tasks.json holds %d tasks, %d B; encoding it takes %.2f ms", len(f.Tasks), len(data), float64(time.Since(enc).Microseconds())/1000/20)
		}
	}
	if n := len(b.Hub.Snapshot().Accounts); n != 2 {
		t.Errorf("the advisors should be gone: %d accounts left", n)
	}
}
