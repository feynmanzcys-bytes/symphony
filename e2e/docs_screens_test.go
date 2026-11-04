//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"math"
	"math/rand/v2"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// TestDocsScreens remakes the README's screenshots in docs/ from made-up activity (no tokens: claude is a script that
// sits there, and the sessions are reported in as the mod would): a team of nine in every state, two of them waiting
// for you, two idle ones wandering, a running task of three in a small meeting room, the others off duty or at work.
// It writes docs/office.png (the pixel office), docs/social-preview.png (the same at 1280x640, for GitHub's social
// preview), docs/approve.png (an employee window with a permission card and a question card over its input),
// and docs/coworkers.png (the Coworkers theme with a conversation open), all at 1400x860 CSS pixels unless named
// otherwise. It writes into the
// repository, so it runs only with SYMPHONY_E2E=1 and SYMPHONY_E2E_DOCS=1. It fails on a page error, a key a catalog
// lacks, a raw key on screen, or the test's temporary folder showing anywhere: the folders on screen read like a
// user's (the task's must exist for git, so they are temporary, but they are named like projects and kept off screen).
func TestDocsScreens(t *testing.T) {
	if os.Getenv("SYMPHONY_E2E") != "1" || os.Getenv("SYMPHONY_E2E_DOCS") != "1" {
		t.Skip("writes docs/*.png; set SYMPHONY_E2E=1 and SYMPHONY_E2E_DOCS=1 to remake them")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	requireBrowser(t)
	repo, _ := filepath.Abs("..")
	docs := filepath.Join(repo, "docs")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))
	started := time.Now()
	t.Cleanup(func() {
		for _, k := range b.Hub.Tasks() {
			_ = b.Hub.EndTask(k.ID)
		}
		b.Hub.Stop()
	})
	b.Launcher.Claude = sleeper(t) // claude sits there; the test reports its sessions in
	var plugin struct{ Version string }
	if raw, err := os.ReadFile(filepath.Join(repo, "mods", "symphony-bridge", ".claude-plugin", "plugin.json")); err != nil || json.Unmarshal(raw, &plugin) != nil {
		t.Fatalf("reading the mod's version: %v", err)
	}
	const engine = "2.1.317" // the Claude Code version the mod is tested with (CLAUDE.md, ci.yml)

	// the task's folder: a real repository (the members get worktrees in it), named like a project
	work := t.TempDir()
	payments := filepath.Join(work, "payments")
	for name, body := range map[string]string{
		"README.md":              "# payments\n\nWebhooks and refunds for the shop.\n",
		"package.json":           "{ \"name\": \"payments\", \"scripts\": { \"test\": \"vitest run\" } }\n",
		"src/webhooks/handle.ts": "export async function handle(event: unknown) {}\n",
		"src/webhooks/store.ts":  "export const seen = new Set<string>()\n",
	} {
		path := filepath.Join(payments, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "Initial commit"}} {
		if out, err := exec.Command("git", append([]string{"-C", payments, "-c", "user.name=dev", "-c", "user.email=dev@example.com"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// ---------- the team ----------
	// seats in this order: the ones at their desks, the wanderers and the task's members alternate along the rows
	const home = "/home/dev/projects/"
	type desk struct{ name, dir, model, effort string }
	team := []desk{
		{"alice", home + "shop-api", "claude-opus-5-5", "max"}, {"erin", home + "mobile-app", "claude-fable-5-1", "high"},
		{"bob", home + "shop-web", "claude-sonnet-5-5", "high"}, {"grace", payments, "claude-opus-5-5", "high"},
		{"carol", home + "shop-api", "claude-opus-5-5", "xhigh"}, {"frank", home + "infra", "claude-haiku-5-5", "low"},
		{"dave", home + "docs-site", "claude-sonnet-5-5", "medium"}, {"heidi", payments, "claude-sonnet-5-5", "xhigh"},
		{"ivan", home + "data-pipeline", "claude-opus-5-5", "high"},
	}
	tokens, models := map[string]string{}, map[string]string{}
	ids := map[string]string{} // account name -> id
	event := func(name, sid string, body map[string]any) {
		t.Helper()
		body["session"] = sid
		b.post(t, tokens[name], "/api/event", body)
	}
	for _, m := range team {
		acc, err := b.Hub.CreateAccount(m.name)
		if err != nil {
			t.Fatal(err)
		}
		tokens[m.name], ids[m.name], models["s-"+m.name] = acc.Token, acc.ID, m.model
		event(m.name, "s-"+m.name, map[string]any{"kind": "session.start", "cwd": m.dir, "model": m.model, "effort": m.effort,
			"engine": map[string]any{"version": engine}, "mod": plugin.Version, "surface": "terminal",
			"repo": map[string]any{"root": m.dir, "name": filepath.Base(m.dir)}})
		event(m.name, "s-"+m.name, map[string]any{"kind": "session.title", "title": m.name})
		event(m.name, "s-"+m.name, map[string]any{"kind": "session.tools", "toolCount": 31, "mcp": []string{"github"}})
	}
	limits := []map[string]any{{"kind": "five_hour", "percentUsed": 23}, {"kind": "seven_day", "percentUsed": 38}} // one login for all
	usage := func(name, sid string, context, cost float64) {
		t.Helper()
		event(name, sid, map[string]any{"kind": "session.usage", "usage": map[string]any{
			"context": map[string]any{"tokens": int(context * 10000), "window": 1000000, "percent": context}, "rateLimits": limits, "cost": map[string]any{"usd": cost}}})
	}
	tokensOf := func(name, sid, turn string, step, in, out int) {
		t.Helper()
		event(name, sid, map[string]any{"kind": "chat.stop", "turnId": turn, "step": step, "stopReason": "tool_use",
			"tokens": map[string]any{"input": in, "output": out, "cacheRead": in * 6, "cacheWrite": in / 4, "model": models[sid]}})
	}
	tool := func(name, sid, id, tool, summary string, ms int) {
		t.Helper()
		event(name, sid, map[string]any{"kind": "tool.start", "tool": tool, "summary": summary, "toolUseId": id})
		if ms > 0 {
			event(name, sid, map[string]any{"kind": "tool.end", "tool": tool, "summary": summary, "toolUseId": id, "durationMs": ms})
		}
	}
	say := func(name, sid, turn string, step int, blocks ...string) { // pairs of kind and text
		t.Helper()
		var bl []map[string]any
		for i := 0; i+1 < len(blocks); i += 2 {
			bl = append(bl, map[string]any{"i": i / 2, "kind": blocks[i], "text": blocks[i+1]})
		}
		event(name, sid, map[string]any{"kind": "chat.delta", "turnId": turn, "step": step, "blocks": bl})
	}
	prompt := func(name, sid, text string) {
		t.Helper()
		event(name, sid, map[string]any{"kind": "chat.user", "text": text, "origin": "composer"})
		event(name, sid, map[string]any{"kind": "turn.start", "text": text})
	}
	var stops []func() // the steps' ends, a few seconds after their text, so the thinking took a while

	// alice: the checkout tests, a subagent looking for callers, the fix being tested
	prompt("alice", "s-alice", "The checkout tests fail since the tax change. Find out why and fix it.")
	say("alice", "s-alice", "a1", 0, "thinking", "The tax change touched src/checkout/tax.ts. I'll run the checkout tests to see which fail, then read the tax code.",
		"text", "I'll run the checkout tests first to see what fails.")
	tool("alice", "s-alice", "a-t1", "Bash", "npm test -- checkout", 8400)
	say("alice", "s-alice", "a1", 1, "text", "3 of 41 tests fail, all in `tax.test.ts`: the totals are a cent off when an order mixes taxed and tax-free items. That points at rounding.")
	tool("alice", "s-alice", "a-t2", "Read", "src/checkout/tax.ts", 40)
	event("alice", "s-alice", map[string]any{"kind": "agent.start", "agentId": "a-ag1", "agentType": "Explore", "description": "Find every caller of applyTax", "model": "claude-haiku-5-5"})
	tool("alice", "s-alice", "a-t3", "Grep", "roundCents", 120)
	say("alice", "s-alice", "a1", 2, "thinking", "applyTax now rounds each line before adding them up; the old code added first and rounded once. The tests expect one rounding at the end.",
		"text", "Found it: `applyTax` rounds every line before adding them up, so mixed orders drift by a cent. I'll round **once**, at the end, as before:\n\n```ts\nconst total = lines.reduce((sum, l) => sum + l.net * l.rate, 0)\nreturn roundCents(total)\n```")
	tool("alice", "s-alice", "a-t4", "Edit", "src/checkout/tax.ts", 60)
	tool("alice", "s-alice", "a-t5", "Bash", "npm test -- checkout", 0)
	stops = append(stops, func() {
		for step, n := range []int{18000, 9000, 21000} {
			tokensOf("alice", "s-alice", "a1", step, n, n/12)
		}
	})
	usage("alice", "s-alice", 41, 2.84)

	// bob: the cart page
	prompt("bob", "s-bob", "Show the tax as its own line in the cart summary.")
	say("bob", "s-bob", "b1", 0, "text", "I'll add a tax row to `CartSummary` and take the amount from the cart's totals.")
	tool("bob", "s-bob", "b-t1", "Read", "src/components/CartSummary.tsx", 30)
	tool("bob", "s-bob", "b-t2", "Edit", "src/components/CartSummary.tsx", 0)
	stops = append(stops, func() { tokensOf("bob", "s-bob", "b1", 0, 26000, 2400) })
	usage("bob", "s-bob", 28, 0.96)

	// carol: a migration; she asks which status the old orders get, and her subagent asks to run it
	prompt("carol", "s-carol", "Add a status column to orders, with a migration, and fill it in for the existing rows.")
	say("carol", "s-carol", "c1", 0, "thinking", "There are 41 migrations in db/migrations, plain SQL. The new one adds the column; the backfill depends on what the old orders should be.",
		"text", "I'll write the migration like the existing ones in `db/migrations`.")
	tool("carol", "s-carol", "c-t1", "Glob", "db/migrations/*.sql", 20)
	tool("carol", "s-carol", "c-t2", "Read", "db/migrations/0041_add_coupons.sql", 30)
	tool("carol", "s-carol", "c-t3", "Write", "db/migrations/0042_order_status.sql", 50)
	event("carol", "s-carol", map[string]any{"kind": "agent.start", "agentId": "c-ag1", "agentType": "general-purpose", "description": "Apply the migration to the local database", "model": "claude-sonnet-5-5"})
	say("carol", "s-carol", "c1", 1, "text", "The migration is written. One thing the code can't tell me: which status should the orders that already exist get?")
	stops = append(stops, func() { tokensOf("carol", "s-carol", "c1", 0, 31000, 3100) })
	usage("carol", "s-carol", 33, 1.47)
	b.post(t, tokens["carol"], "/api/ask", map[string]any{"session": "s-carol", "kind": "question", "questions": []map[string]any{{
		"question": "Which status should the existing orders get?", "header": "Backfill", "multiSelect": false,
		"options": []map[string]string{
			{"label": "completed", "description": "Every order placed before today has shipped"},
			{"label": "From the shipments", "description": "completed when a shipment exists, pending otherwise"}}}}})
	b.post(t, tokens["carol"], "/api/ask", map[string]any{"session": "s-carol", "kind": "permission", "tool": "Bash", "agentId": "c-ag1",
		"input":    map[string]any{"command": "psql -d shop_dev -f db/migrations/0042_order_status.sql", "description": "Apply the migration to the local database"},
		"remember": map[string]any{"tool": "Bash", "prefix": "psql"}}) // what the mod proposes: "Allow for this session"

	// dave: the docs, asking to push
	prompt("dave", "s-dave", "Document the new orders endpoints in the API reference.")
	say("dave", "s-dave", "d1", 0, "text", "The reference for `GET /orders/{id}` and `PATCH /orders/{id}` is written and the site builds. I'll push the branch.")
	tool("dave", "s-dave", "d-t1", "Write", "content/api/orders.md", 40)
	tool("dave", "s-dave", "d-t2", "Bash", "npm run build", 14200)
	stops = append(stops, func() { tokensOf("dave", "s-dave", "d1", 0, 12000, 1800) })
	usage("dave", "s-dave", 17, 0.21)
	b.post(t, tokens["dave"], "/api/ask", map[string]any{"session": "s-dave", "kind": "permission", "tool": "Bash",
		"input":    map[string]any{"command": "git push origin docs/orders-api", "description": "Push the docs branch"},
		"remember": map[string]any{"tool": "Bash", "prefix": "git push"}})

	// erin and frank are done and idle: they will wander about
	for _, x := range []struct{ name, text, answer string }{
		{"erin", "Upgrade React Native to 0.80 and fix the build.", "The app builds again on iOS and Android; three deprecated imports needed new paths."},
		{"frank", "Renew the staging TLS certificates.", "Renewed both certificates; they now expire on 30 January."},
	} {
		sid := "s-" + x.name
		prompt(x.name, sid, x.text)
		say(x.name, sid, x.name+"1", 0, "text", x.answer)
		event(x.name, sid, map[string]any{"kind": "turn.complete", "reason": "answer"})
	}
	stops = append(stops, func() {
		tokensOf("erin", "s-erin", "erin1", 0, 64000, 5200)
		tokensOf("frank", "s-frank", "frank1", 0, 9000, 700)
	})
	usage("erin", "s-erin", 52, 3.40)
	usage("frank", "s-frank", 9, 0.07)
	usage("grace", "s-grace", 12, 0.38)
	usage("heidi", "s-heidi", 6, 0.12)
	// ivan worked this morning and is off duty now
	tokensOf("ivan", "s-ivan", "i1", 0, 40000, 3000)
	usage("ivan", "s-ivan", 22, 0.62)
	event("ivan", "s-ivan", map[string]any{"kind": "session.end", "reason": "prompt_input_exit"})

	// ---------- background: heartbeats, and the working sessions keep working ----------
	client := &http.Client{Timeout: 5 * time.Second}
	send := func(token, sid string, body map[string]any) { // from the background: no t.Fatal there
		body["session"] = sid
		data, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", b.URL+"/api/event", bytes.NewReader(data))
		req.Header.Set("content-type", "application/json")
		req.Header.Set("authorization", "Bearer "+token)
		if res, err := client.Do(req); err == nil {
			res.Body.Close()
		}
	}
	type worker struct {
		sid, token, model, turn string
		tools                   [][2]string // tool and summary, in turn; none: it stays on the tool it runs
		open                    [2]string   // the tool running and its summary
		cost, context           float64
		step, k                 int
	}
	type note struct{ from, to, fromToken, toToken, text string }
	var (
		mu      sync.Mutex
		alive   = map[string]string{} // session -> token, for the heartbeats
		workers []*worker
		chatter []note // messages between the task's members, now and then
		stop    = make(chan struct{})
		wg      sync.WaitGroup
	)
	for _, m := range team {
		if m.name != "ivan" {
			alive["s-"+m.name] = tokens[m.name]
		}
	}
	addWorker := func(w *worker) { mu.Lock(); workers = append(workers, w); alive[w.sid] = w.token; mu.Unlock() }
	addWorker(&worker{sid: "s-alice", token: tokens["alice"], model: models["s-alice"], turn: "a1", step: 3, cost: 2.84, context: 41})
	addWorker(&worker{sid: "s-bob", token: tokens["bob"], model: models["s-bob"], turn: "b1", step: 1, cost: 0.96, context: 28,
		open: [2]string{"Edit", "src/components/CartSummary.tsx"}, tools: [][2]string{{"Read", "src/hooks/useCart.ts"}, {"Edit", "src/hooks/useCart.ts"},
			{"Bash", "npm run typecheck"}, {"Edit", "src/components/CartSummary.test.tsx"}, {"Bash", "npm test -- CartSummary"}, {"Read", "src/i18n/en.json"}}})
	wg.Add(2)
	go func() { // heartbeats, as the mod sends every 15 seconds
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			case <-time.After(10 * time.Second):
			}
			mu.Lock()
			beat := make(map[string]string, len(alive))
			for sid, token := range alive {
				beat[sid] = token
			}
			mu.Unlock()
			for sid, token := range beat {
				send(token, sid, map[string]any{"kind": "heartbeat"})
			}
		}
	}()
	go func() { // a working session at a time: its tool ends, the response's tokens, the figures, the next tool
		defer wg.Done()
		rng := rand.New(rand.NewPCG(7, 11))
		pace := []time.Duration{700 * time.Millisecond, 500 * time.Millisecond, 900 * time.Millisecond, 600 * time.Millisecond, 1100 * time.Millisecond} // busier minutes and quieter ones
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			case <-time.After(pace[int(time.Since(started)/time.Minute)%len(pace)]):
			}
			mu.Lock()
			if len(workers) == 0 {
				mu.Unlock()
				continue
			}
			w := workers[i%len(workers)]
			mu.Unlock()
			if len(w.tools) > 0 && w.open[1] != "" {
				send(w.token, w.sid, map[string]any{"kind": "tool.end", "tool": w.open[0], "summary": w.open[1], "toolUseId": fmt.Sprintf("%s-%d", w.sid, w.k), "durationMs": 200 + rng.IntN(6000)})
			}
			in, out := 800+rng.IntN(5000), 150+rng.IntN(1400) // most of the input is read from the cache
			send(w.token, w.sid, map[string]any{"kind": "chat.stop", "turnId": w.turn, "step": w.step, "stopReason": "tool_use",
				"tokens": map[string]any{"input": in, "output": out, "cacheRead": in * 5, "cacheWrite": in / 5, "model": w.model}})
			w.step++
			w.cost += 0.005 + float64(rng.IntN(30))/1000
			w.context += 0.1 + float64(rng.IntN(4))/10
			send(w.token, w.sid, map[string]any{"kind": "session.usage", "usage": map[string]any{
				"context": map[string]any{"tokens": int(w.context * 10000), "window": 1000000, "percent": w.context}, "rateLimits": limits, "cost": map[string]any{"usd": w.cost}}})
			mu.Lock()
			m, ok := note{}, len(chatter) > 0 && i%25 == 24
			if ok {
				m = chatter[(i/25)%len(chatter)]
			}
			mu.Unlock()
			if ok {
				wrapped := fmt.Sprintf("<cross-session-message from=\"uds:/tmp/x.sock\" from-name=%q from-mode=\"prompting\">\n%s\n</cross-session-message>", strings.TrimPrefix(m.from, "t-"), m.text)
				send(m.toToken, m.to, map[string]any{"kind": "message.receive", "text": wrapped, "origin": map[string]any{"kind": "peer"}})
				send(m.fromToken, m.from, map[string]any{"kind": "message.send", "to": strings.TrimPrefix(m.to, "t-"), "text": m.text, "origin": "model", "delivered": true})
			}
			if len(w.tools) > 0 {
				w.open = w.tools[w.k%len(w.tools)]
				w.k++
				send(w.token, w.sid, map[string]any{"kind": "tool.start", "tool": w.open[0], "summary": w.open[1], "toolUseId": fmt.Sprintf("%s-%d", w.sid, w.k)})
			}
		}
	}()
	t.Cleanup(func() { close(stop); wg.Wait() }) // before the bridge closes

	time.Sleep(4 * time.Second) // the thinking takes a few seconds
	for _, f := range stops {
		f()
	}

	// ---------- the task: grace leads, heidi and an advisor in the payments repository ----------
	browser := newBrowser(t)
	light := chromedp.Func(func(ctx context.Context, tg *chromedp.Target) error { // the light themes, whatever the machine prefers
		_, err := cdp.Call(ctx, tg, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: "light"}}})
		return err
	})
	open := func(path string) (*screens, context.CancelFunc) {
		t.Helper()
		ctx, cancel := clockTab(t, browser, b.URL+path, 0)
		do(t, ctx, 10*time.Second, light, chromedp.EmulateViewport(1400, 860))
		return &screens{t: t, ctx: ctx, lang: "en"}, cancel
	}
	office, _ := open("/t/office/?lang=en") // (not /: that serves the theme last picked, a cookie)
	waitUntil(t, 20*time.Second, "the office shows the team and the rooms' nameplates", func() bool {
		return office.is(`document.querySelectorAll('.emp-label').length >= 8 && document.querySelectorAll('.room-plate.click').length >= 4`)
	}, func() string {
		return office.js(`document.querySelectorAll('.emp-label').length + ' tags, ' + document.querySelectorAll('.room-plate.click').length + ' clickable plates; errors: ' + JSON.stringify(window.__errors)`)
	})
	var plates []struct {
		Name, Seats string
		X, Y        float64
	}
	if err := json.Unmarshal([]byte(office.js(`[...document.querySelectorAll('.room-plate')].map(p => ({ Name:p.textContent, Seats:p.dataset.seats, X:p.getBoundingClientRect().left, Y:p.getBoundingClientRect().top }))`)), &plates); err != nil {
		t.Fatal(err)
	}
	t.Logf("rooms: %+v", plates)
	room := -1 // the room for 3 nearest the middle of the window's height, clear of the HUD and roomy for the tags
	for i, p := range plates {
		if p.Seats == "3" && (room < 0 || math.Abs(p.Y-430) < math.Abs(plates[room].Y-430)) {
			room = i
		}
	}
	if room < 0 {
		t.Fatalf("no small meeting room for 3: %+v", plates)
	}
	body, _ := json.Marshal(map[string]any{"room": room, "title": "Add retries and idempotency keys to the payment webhooks", "leader": true,
		"permissionMode": "acceptEdits", "lang": "en", "members": []map[string]any{
			{"account": ids["grace"], "model": "opus", "effort": "high"}, {"account": ids["heidi"], "model": "sonnet", "effort": "xhigh"},
			{"advisor": map[string]string{"name": "judy", "dir": payments}, "model": "opus", "effort": "max"}}})
	var task bridge.Task
	if res := office.fetch("POST", "/api/tasks", string(body)); json.Unmarshal([]byte(res), &task) != nil || task.ID == "" {
		t.Fatalf("creating the task failed: %s", res)
	}
	var list []struct{ Name, Token string }
	if raw, err := os.ReadFile(filepath.Join(b.Data, "accounts.json")); err != nil || json.Unmarshal(raw, &list) != nil {
		t.Fatalf("reading the accounts: %v", err)
	}
	for _, a := range list {
		tokens[a.Name] = a.Token
	}
	member := map[string]string{} // account name -> member session
	for _, m := range task.Members {
		account := strings.TrimSuffix(m.Name, "-"+task.ID)
		sid := "t-" + m.Name
		member[account], models[sid] = sid, map[string]string{"opus": "claude-opus-5-5", "sonnet": "claude-sonnet-5-5"}[m.Model]
		event(account, sid, map[string]any{"kind": "session.start", "cwd": m.Worktree, "launch": m.Launch, "task": task.ID, "model": models[sid], "effort": m.Effort,
			"engine": map[string]any{"version": engine}, "mod": plugin.Version, "surface": "terminal"})
		event(account, sid, map[string]any{"kind": "session.title", "task": task.ID, "title": m.Name})
		// the kickoff waits in the inbox; the mod takes it and sends it, as a prompt
		req, _ := http.NewRequest("GET", b.URL+"/api/inbox?session="+sid, nil)
		req.Header.Set("Authorization", "Bearer "+tokens[account])
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var inbox struct{ Items []struct{ Text string } }
		err = json.NewDecoder(res.Body).Decode(&inbox)
		res.Body.Close()
		if err != nil || len(inbox.Items) != 1 {
			t.Fatalf("%s should have its kickoff waiting: %+v (%v)", m.Name, inbox.Items, err)
		}
		event(account, sid, map[string]any{"kind": "chat.sent", "task": task.ID, "text": inbox.Items[0].Text})
		event(account, sid, map[string]any{"kind": "turn.start", "task": task.ID, "text": string([]rune(inbox.Items[0].Text)[:200])}) // as the mod cuts it
	}
	message := func(from, to, text string) {
		t.Helper()
		fromSid, toSid := member[from], member[to]
		wrapped := fmt.Sprintf("<cross-session-message from=\"uds:/tmp/x.sock\" from-name=%q from-mode=\"prompting\">\n%s\n</cross-session-message>", strings.TrimPrefix(fromSid, "t-"), text)
		event(to, toSid, map[string]any{"kind": "message.receive", "task": task.ID, "text": wrapped, "origin": map[string]any{"kind": "peer"}})
		event(from, fromSid, map[string]any{"kind": "message.send", "task": task.ID, "to": strings.TrimPrefix(toSid, "t-"), "text": text, "origin": "model", "delivered": true})
	}
	message("grace", "heidi", "Please take the idempotency keys in src/webhooks/store.ts; I'll write the retry queue. Commit on your branch and tell me when the tests pass.")
	message("grace", "judy", "Could you review the retry backoff once it's on my branch?")
	message("heidi", "grace", "On it. Do we keep the keys for 24 hours like the payment provider does?")
	addWorker(&worker{sid: member["grace"], token: tokens["grace"], model: models[member["grace"]], turn: "g1", context: 8, cost: 0.2,
		tools: [][2]string{{"Write", "src/webhooks/retry.ts"}, {"Read", "src/webhooks/handle.ts"}, {"Edit", "src/webhooks/handle.ts"}, {"Bash", "npm test -- webhooks"}, {"SendMessage", "judy-" + task.ID}}})
	addWorker(&worker{sid: member["heidi"], token: tokens["heidi"], model: models[member["heidi"]], turn: "h1", context: 6, cost: 0.1,
		tools: [][2]string{{"Read", "src/webhooks/store.ts"}, {"Edit", "src/webhooks/store.ts"}, {"Write", "src/webhooks/store.test.ts"}, {"Bash", "npm test -- store"}}})
	addWorker(&worker{sid: member["judy"], token: tokens["judy"], model: models[member["judy"]], turn: "j1", context: 4, cost: 0.05,
		tools: [][2]string{{"Grep", "retry"}, {"Read", "src/webhooks/retry.ts"}, {"Bash", "git log --oneline main..symphony/" + task.ID + "/grace"}}})

	// ---------- what a shot checks: no raw keys, no errors, no temporary folder on screen ----------
	shot := func(s *screens, name string, w, h int) {
		t.Helper()
		s.check()
		if s.is(fmt.Sprintf(`document.body.innerText.includes(%q)`, filepath.Dir(work))) { // the test's temporary folders: the task's, the data folder
			t.Fatalf("%s: the test's temporary folder shows on the page", name)
		}
		time.Sleep(400 * time.Millisecond)
		buf := get(t, s.ctx, 20*time.Second, chromedp.CaptureScreenshot())
		cfg, err := png.DecodeConfig(bytes.NewReader(buf))
		if err != nil || cfg.Width != w || cfg.Height != h {
			t.Fatalf("%s: %dx%d (%v), want %dx%d", name, cfg.Width, cfg.Height, err, w, h)
		}
		path := filepath.Join(docs, name)
		if err := os.WriteFile(path, buf, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d KB", path, len(buf)/1024)
	}

	// ---------- coworkers.png: alice's conversation in the Coworkers theme ----------
	coworkers, closeCoworkers := open("/t/messenger/?lang=en")
	waitUntil(t, 20*time.Second, "Coworkers lists the nine and the task's group", func() bool {
		return coworkers.is(`document.querySelectorAll('#people .row').length === 9 && document.querySelectorAll('#groups .row').length === 1 && !document.documentElement.hasAttribute('data-dark')`)
	})
	coworkers.run(`[...document.querySelectorAll('#people .row')].find(r => r.dataset.account === ` + fmt.Sprintf("%q", ids["alice"]) + `).click()`)
	waitUntil(t, 10*time.Second, "alice's conversation and profile", func() bool {
		return coworkers.is(`!document.getElementById('chat').hidden && document.querySelector('#chat .flow').textContent.includes('roundCents(total)') && /Context\s*\d+%/.test(document.getElementById('profile').innerText) && document.getElementById('profile').textContent.includes('Explore')`)
	}, func() string { return coworkers.js(`document.getElementById('profile').innerText`) })
	coworkers.run(`(f => f.scrollTop = f.scrollHeight)(document.querySelector('#chat .flow'))`)
	shot(coworkers, "coworkers.png", 1400, 860)
	coworkers.done()
	closeCoworkers()

	// ---------- office.png: the pixel office, the idle ones wandering (the page's clock moved on) ----------
	idleFor := func() time.Duration {
		for _, s := range b.Hub.Snapshot().Sessions {
			if s.ID == "s-erin" {
				return time.Since(s.StateSince)
			}
		}
		t.Fatal("erin has no session")
		return 0
	}
	office.run(fmt.Sprintf(`window.__warp = %d`, (70*time.Second - idleFor()).Milliseconds()))
	hudDown := true
	settled := func() bool {
		return office.is(fmt.Sprintf(`(() => {
			const tags = [...document.querySelectorAll('.emp-label')]
			const seated = tags.filter(l => l.classList.contains('member') && l.dataset.task === %q && l.dataset.moving === '0' && (l.dataset.spot || '').startsWith('room')).length
			const idle = [%q, %q].map(id => tags.find(l => !l.classList.contains('member') && l.dataset.account === id))
			const still = idle.every(l => l && (l.dataset.moving === '0' || l.dataset.act === 'chase'))
			const away = idle.filter(l => l && l.dataset.room && l.dataset.room !== 'desk').length
			return seated === 3 && still && away === 2 && !!document.querySelector('#stats .chip.hot') && !document.getElementById('hud').classList.contains('up') === %v
		})()`, task.ID, ids["erin"], ids["frank"], hudDown))
	}
	wander := func() {
		t.Helper()
		for try := 0; ; try++ {
			if eventually(30*time.Second, settled) {
				time.Sleep(time.Second) // the tags settle
				return
			}
			if try == 3 {
				t.Fatalf("the idle ones never left their desks:\n%s", office.js(`[...document.querySelectorAll('.emp-label')].map(l => l.querySelector('.nm').textContent + ' ' + JSON.stringify(l.dataset)).join('\n')`))
			}
			office.run(`window.__warp += 30000`) // a new half minute: each picks again where to go
		}
	}
	wander()
	shot(office, "office.png", 1400, 860)

	// ---------- social-preview.png: the office at GitHub's 1280x640, the HUD slid up out of the way ----------
	do(t, office.ctx, 10*time.Second, chromedp.EmulateViewport(1280, 640))
	office.run(`document.getElementById('hud-tab').click()`)
	time.Sleep(3 * time.Second) // the figures find their new places
	hudDown = false
	wander()
	shot(office, "social-preview.png", 1280, 640)
	office.run(`document.getElementById('hud-tab').click()`)
	hudDown = true
	do(t, office.ctx, 10*time.Second, chromedp.EmulateViewport(1400, 860))
	time.Sleep(2 * time.Second)

	// ---------- approve.png: carol's window, her question and her subagent's permission over the input ----------
	clickLabel(t, office.ctx, ids["carol"])
	waitUntil(t, 10*time.Second, "carol's window shows both cards", func() bool {
		return office.is(`!document.getElementById('chat').hidden && document.querySelectorAll('#chat #asks .card').length === 2 && document.querySelector('#chat .dock').classList.contains('asking')`)
	})
	time.Sleep(time.Second) // the dock's rise
	office.run(`document.activeElement?.blur()`)
	shot(office, "approve.png", 1400, 860)
	office.done()
}
