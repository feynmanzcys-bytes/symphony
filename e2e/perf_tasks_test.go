//go:build e2e

package e2e

import (
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
	"github.com/chromedp/cdproto/performance"
	"github.com/chromedp/cdproto/profiler"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// TestPerfTasks (no tokens) measures the bridge with collaborative tasks: claude is a script that sits there and the
// test reports every session in the way the mod would (the page under the load of many tasks is TestPerfThemes).
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
func makeTask(t *testing.T, b *testBridge, accounts []string, advisors []string, dir string) (string, []member, time.Duration) {
	t.Helper()
	var ms []map[string]any
	for _, a := range accounts {
		ms = append(ms, map[string]any{"account": a, "model": "opus", "effort": "max"})
	}
	for _, n := range advisors {
		ms = append(ms, map[string]any{"advisor": map[string]any{"name": n, "dir": dir}, "model": "sonnet", "effort": "high"})
	}
	start := time.Now()
	out := mustSend(t, b.URL, "", "/api/tasks", map[string]any{"title": "Make the tests faster and write it up",
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

// ---------- the page (what TestPerfThemes measures with) ----------

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
		id, ms, took := makeTask(t, b, accs, nil, c.dir)
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
			otherID, otherMs, otherTook = makeTask(t, b, spare, nil, other)
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
		id, ms, took := makeTask(t, b, accs, []string{"adv"}, app)
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
