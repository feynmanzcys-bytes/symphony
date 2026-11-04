//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
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

// TestDesignScreens is a screenshot tour of Coworkers for a design review: TestDocsScreens' made-up world (a team of
// nine in every state, two waiting for you, a running task of three), then every key screen of the theme at 1400x860
// CSS pixels, twice as big, into the folder SYMPHONY_DESIGN_OUT names (it runs only with it set). No tokens.
func TestDesignScreens(t *testing.T) {
	out := os.Getenv("SYMPHONY_DESIGN_OUT")
	if out == "" {
		t.Skip("set SYMPHONY_DESIGN_OUT to the folder for the screenshots")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	requireBrowser(t)
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))
	started := time.Now()
	t.Cleanup(func() {
		for _, k := range b.Hub.Tasks() {
			_ = b.Hub.EndTask(k.ID)
		}
		b.Hub.Stop()
	})
	b.Launcher.Claude = sleeper(t)                   // claude sits there; the test reports its sessions in
	b.Hub.BotHomeDefault = "/home/dev/symphony-bots" // named like a user's (no start happens here)
	var plugin struct{ Version string }
	if raw, err := os.ReadFile(filepath.Join(repo, "mods", "symphony-bridge", ".claude-plugin", "plugin.json")); err != nil || json.Unmarshal(raw, &plugin) != nil {
		t.Fatalf("reading the mod's version: %v", err)
	}
	const engine = "2.1.317"

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
	cw, closeCw := open("/t/messenger/?lang=en") // (not /: that serves the theme last picked, a cookie)
	waitUntil(t, 20*time.Second, "Coworkers lists the nine", func() bool {
		return cw.is(`document.querySelectorAll('#people .row').length === 9`)
	}, func() string {
		return cw.js(`document.querySelectorAll('#people .row').length + ' rows; errors: ' + JSON.stringify(window.__errors)`)
	})
	body, _ := json.Marshal(map[string]any{"title": "Add retries and idempotency keys to the payment webhooks", "leader": true,
		"permissionMode": "acceptEdits", "lang": "en", "members": []map[string]any{
			{"account": ids["grace"], "model": "opus", "effort": "high"}, {"account": ids["heidi"], "model": "sonnet", "effort": "xhigh"},
			{"advisor": map[string]string{"name": "judy", "dir": payments}, "model": "opus", "effort": "max"}}})
	var task bridge.Task
	if res := cw.fetch("POST", "/api/tasks", string(body)); json.Unmarshal([]byte(res), &task) != nil || task.ID == "" {
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

	// grace and heidi's desk sessions move on to a folder named like the others' (the task's is a temporary one, for git,
	// and would show on the employees page); what each one starts with from the page, ivan going by the settings
	for _, name := range []string{"grace", "heidi"} {
		event(name, "s-"+name, map[string]any{"kind": "session.cwd", "cwd": home + "payments"})
	}
	opt, yes := func(v string) *string { return &v }, true
	if _, err := b.Hub.ImportStaff([]bridge.StaffEntry{
		{Name: "alice", Model: opt("opus"), Effort: opt("max"), Context: &yes}, {Name: "bob", Model: opt("sonnet"), Effort: opt("high"), PermissionMode: opt("acceptEdits")},
		{Name: "carol", Model: opt("opus"), Effort: opt("xhigh")}, {Name: "dave", Model: opt("sonnet"), Effort: opt("medium"), PermissionMode: opt("acceptEdits")},
		{Name: "erin", Model: opt("fable"), Effort: opt("high"), Context: &yes}, {Name: "frank", Model: opt("haiku"), Effort: opt("low")},
		{Name: "grace", Model: opt("opus"), Effort: opt("high"), Context: &yes}, {Name: "heidi", Model: opt("sonnet"), Effort: opt("xhigh")},
	}); err != nil {
		t.Fatal(err)
	}

	// ---------- what a shot does: the window twice as big into the output folder ----------
	shot := func(s *screens, name string) {
		t.Helper()
		s.check()
		if s.is(fmt.Sprintf(`document.body.innerText.includes(%q)`, filepath.Dir(work))) {
			t.Logf("%s: the test's temporary folder shows on the page", name)
		}
		time.Sleep(500 * time.Millisecond)
		buf := get(t, s.ctx, 20*time.Second, chromedp.CaptureScreenshot())
		cfg, err := png.DecodeConfig(bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		path := filepath.Join(out, name+".png")
		if err := os.WriteFile(path, buf, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %dx%d, %d KB", path, cfg.Width, cfg.Height, len(buf)/1024)
	}
	size := func(s *screens, w, h int64) {
		t.Helper()
		do(t, s.ctx, 10*time.Second, chromedp.EmulateViewport(w, h, chromedp.EmulateScale(2)))
		time.Sleep(600 * time.Millisecond)
	}
	pick := func(s *screens, name string) {
		t.Helper()
		s.run(`[...document.querySelectorAll('#people .row')].find(r => r.dataset.account === ` + fmt.Sprintf("%q", ids[name]) + `).click()`)
		waitUntil(t, 10*time.Second, name+"'s conversation", func() bool {
			return s.is(`!document.getElementById('chat').hidden && document.querySelector('#chat .flow').children.length > 0`)
		})
		time.Sleep(700 * time.Millisecond)
		s.run(`(f => f.scrollTop = f.scrollHeight)(document.querySelector('#chat .flow')); document.activeElement?.blur()`)
	}
	closeDialog := func(s *screens, id string) {
		s.run(fmt.Sprintf(`(document.querySelector('#%s .cancel') || document.querySelector('#%s .close'))?.click()`, id, id))
		time.Sleep(300 * time.Millisecond)
	}

	size(cw, 1400, 860)
	waitUntil(t, 20*time.Second, "Coworkers lists the nine and the task's group", func() bool {
		return cw.is(`document.querySelectorAll('#people .row').length === 9 && document.querySelectorAll('#groups .row').length === 1 && !document.documentElement.hasAttribute('data-dark')`)
	})
	time.Sleep(time.Second)
	shot(cw, "01-empty")

	// conversations in every state
	pick(cw, "alice")
	waitUntil(t, 10*time.Second, "alice's answer and profile", func() bool {
		return cw.is(`document.querySelector('#chat .flow').textContent.includes('roundCents(total)') && document.getElementById('profile').textContent.includes('Explore')`)
	})
	cw.run(`(f => f.scrollTop = f.scrollHeight)(document.querySelector('#chat .flow'))`)
	shot(cw, "02-conv-working")
	cw.run(`for (const d of document.querySelectorAll('#chat .flow details.steps')) d.open = true; (f => f.scrollTop = f.scrollHeight)(document.querySelector('#chat .flow'))`)
	time.Sleep(500 * time.Millisecond)
	shot(cw, "03-conv-steps-open")
	cw.run(`for (const d of document.querySelectorAll('#chat .flow details.steps')) d.open = false`)

	pick(cw, "carol")
	waitUntil(t, 10*time.Second, "carol's two cards", func() bool {
		return cw.is(`document.querySelectorAll('#chat .flow .it.ask.carded .card').length === 2`)
	})
	time.Sleep(500 * time.Millisecond)
	cw.run(`(f => f.scrollTop = f.scrollHeight)(document.querySelector('#chat .flow')); document.activeElement?.blur()`)
	shot(cw, "04-conv-asks")

	pick(cw, "dave")
	waitUntil(t, 10*time.Second, "dave's permission card", func() bool {
		return cw.is(`document.querySelectorAll('#chat .flow .it.ask.carded .card').length === 1`)
	})
	time.Sleep(500 * time.Millisecond)
	cw.run(`(f => f.scrollTop = f.scrollHeight)(document.querySelector('#chat .flow')); document.activeElement?.blur()`)
	shot(cw, "05-conv-permission")

	pick(cw, "ivan")
	shot(cw, "06-conv-offduty")
	pick(cw, "erin")
	shot(cw, "07-conv-idle")

	// the group chat, its mention picker, a member's conversation from it
	cw.run(`document.querySelector('#groups .row').click()`)
	waitUntil(t, 10*time.Second, "the group chat", func() bool {
		return cw.is(`!document.getElementById('taskwin').hidden && document.querySelectorAll('#taskwin .mrow').length === 3 && document.querySelectorAll('#taskwin .bubble, #taskwin .msg, #taskwin .tchat > *').length > 0`)
	}, func() string { return cw.js(`document.getElementById('taskwin').innerText.slice(0, 500)`) })
	time.Sleep(time.Second)
	cw.run(`document.activeElement?.blur()`)
	shot(cw, "08-group")
	cw.run(`(b => { b.focus(); b.value = '@'; b.setSelectionRange(1, 1); b.dispatchEvent(new Event('input')) })(document.querySelector('#taskwin .tsay textarea'))`)
	waitUntil(t, 5*time.Second, "the mention picker", func() bool {
		return cw.is(`document.querySelectorAll('#taskwin .tpick:not([hidden]) .hit').length === 3`)
	})
	shot(cw, "09-group-mention")
	cw.run(`(b => { b.value = ''; b.dispatchEvent(new Event('input')); b.blur() })(document.querySelector('#taskwin .tsay textarea'))`)
	cw.run(`document.querySelector('#taskwin .mrow').click()`)
	waitUntil(t, 10*time.Second, "the member's conversation", func() bool {
		return cw.is(`!document.getElementById('chat').hidden && document.getElementById('taskwin').hidden && !!document.querySelector('#profile .link[data-group]')`)
	})
	time.Sleep(700 * time.Millisecond)
	cw.run(`(f => f.scrollTop = f.scrollHeight)(document.querySelector('#chat .flow')); document.activeElement?.blur()`)
	shot(cw, "10-member")
	cw.run(`document.querySelector('#profile .link[data-group]').click()`)
	waitUntil(t, 10*time.Second, "back to the group", func() bool { return cw.is(`!document.getElementById('taskwin').hidden`) })

	// the dialogs: a new coworker (folded, unfolded with the warning), a new group
	cw.run(`document.getElementById('new-emp').click()`)
	waitUntil(t, 10*time.Second, "the launch dialog", func() bool { return cw.is(`!document.getElementById('launch').hidden`) })
	cw.run(`(f => { f.value = 'kim'; f.dispatchEvent(new Event('input', { bubbles: true })) })(document.querySelector('#launch [name=name]')); document.activeElement?.blur()`)
	time.Sleep(500 * time.Millisecond)
	shot(cw, "11-launch-folded")
	cw.run(`document.querySelector('#launch .adv').open = true; (s => { s.value = 'bypassPermissions'; s.dispatchEvent(new Event('change', { bubbles: true })) })(document.querySelector('#launch [name=permissionMode]')); document.activeElement?.blur()`)
	time.Sleep(500 * time.Millisecond)
	shot(cw, "12-launch-unfolded")
	closeDialog(cw, "launch")

	cw.run(`document.getElementById('new-task').click()`)
	waitUntil(t, 10*time.Second, "the task dialog", func() bool { return cw.is(`!document.getElementById('task').hidden`) })
	cw.run(`for (const n of ['alice', 'bob']) [...document.querySelectorAll('#task .mem')].find(r => r.textContent.includes(n))?.querySelector('input').click();
		document.querySelector('#task .add-adv').click();
		(r => { r.querySelector('[name=name]').value = 'reviewer'; r.querySelector('[name=dir]').value = '/home/dev/projects/shop-api' })(document.querySelector('#task .adv'));
		document.querySelector('#task [name=desc]').value = 'Add retries and idempotency keys to the payment webhooks, with tests.'; document.activeElement?.blur()`)
	time.Sleep(500 * time.Millisecond)
	shot(cw, "13-task-dialog")
	closeDialog(cw, "task")

	// the settings, the report section, the employees page, a row edited, an import previewed
	cw.run(`document.querySelector('.open-settings').click()`)
	waitUntil(t, 10*time.Second, "the settings", func() bool { return cw.is(`!document.getElementById('settings').hidden`) })
	cw.run(`document.activeElement?.blur()`)
	time.Sleep(500 * time.Millisecond)
	shot(cw, "14-settings")
	cw.run(`[...document.querySelectorAll('#settings button')].find(b => /Show what/.test(b.textContent))?.click();
		(el => el.scrollTop = el.scrollHeight)(document.querySelector('#settings .body') || document.querySelector('#settings .modal-body') || document.getElementById('settings')); document.activeElement?.blur()`)
	time.Sleep(700 * time.Millisecond)
	shot(cw, "15-settings-bottom")
	cw.run(`document.querySelector('#settings .people .all').click()`)
	waitUntil(t, 10*time.Second, "the employees page", func() bool {
		return cw.is(`!document.getElementById('staff').hidden && document.querySelectorAll('#staff .emp').length === 9`)
	})
	cw.run(`document.activeElement?.blur()`)
	time.Sleep(500 * time.Millisecond)
	shot(cw, "16-staff")
	cw.run(`[...document.querySelectorAll('#staff .emp')].find(r => r.querySelector('.nm').textContent === 'bob').querySelector('.edit').click()`)
	waitUntil(t, 5*time.Second, "bob's row unfolded", func() bool { return cw.is(`!!document.getElementById('staff-editor')`) })
	cw.run(`document.activeElement?.blur()`)
	time.Sleep(500 * time.Millisecond)
	shot(cw, "17-staff-edit")
	cw.run(`document.querySelector('#staff-editor .cancel')?.click(); document.querySelector('#staff .tools .import').click();
		(ta => { ta.value = 'kim\nlee\nbob\n# the data team\nmaya'; ta.dispatchEvent(new Event('input', { bubbles: true })) })(document.getElementById('staff-import-text')); document.activeElement?.blur()`)
	waitUntil(t, 5*time.Second, "the import preview", func() bool { return cw.is(`document.querySelectorAll('#staff .importer .preview li').length >= 3`) })
	time.Sleep(500 * time.Millisecond)
	shot(cw, "18-staff-import")
	closeDialog(cw, "staff")
	closeDialog(cw, "settings")

	// dark: a conversation and the group
	cw.run(`document.getElementById('set-scheme-dark').click()`)
	pick(cw, "alice")
	shot(cw, "19-dark-conv")
	pick(cw, "carol")
	shot(cw, "20-dark-asks")
	cw.run(`document.querySelector('#groups .row').click()`)
	waitUntil(t, 10*time.Second, "the group chat, dark", func() bool { return cw.is(`!document.getElementById('taskwin').hidden`) })
	time.Sleep(700 * time.Millisecond)
	cw.run(`document.activeElement?.blur()`)
	shot(cw, "21-dark-group")
	cw.run(`document.getElementById('set-scheme-light').click()`)

	// narrower windows: the profile gone, one column, a phone
	size(cw, 1000, 800)
	pick(cw, "alice")
	shot(cw, "22-w1000-conv")
	size(cw, 600, 800)
	cw.run(`document.querySelector('#chat .close')?.click()`)
	time.Sleep(500 * time.Millisecond)
	shot(cw, "23-w600-lists")
	pick(cw, "carol")
	shot(cw, "24-w600-asks")
	size(cw, 390, 844)
	pick(cw, "alice")
	shot(cw, "25-phone-conv")
	size(cw, 1400, 860)

	// the bridge gone: the connection banner
	pick(cw, "alice")
	b.cutPages(true)
	waitUntil(t, 15*time.Second, "the connection banner", func() bool {
		return cw.is(`(c => !!c && !c.hidden && c.textContent.trim().length > 0)(document.getElementById('conn'))`)
	})
	time.Sleep(500 * time.Millisecond)
	shot(cw, "26-banner")
	b.cutPages(false)
	waitUntil(t, 15*time.Second, "the banner gone", func() bool {
		return cw.is(`(c => !c || c.hidden || c.textContent.trim().length === 0)(document.getElementById('conn'))`)
	})
	cw.done()
	closeCw()

	// zh-CN
	zh, closeZh := open("/t/messenger/?lang=zh-CN")
	size(zh, 1400, 860)
	waitUntil(t, 20*time.Second, "zh-CN: the sidebar", func() bool {
		return zh.is(`document.querySelectorAll('#people .row').length === 9 && document.documentElement.lang === 'zh-CN'`)
	})
	pick(zh, "carol")
	waitUntil(t, 10*time.Second, "zh-CN: carol's two cards", func() bool {
		return zh.is(`document.querySelectorAll('#chat .flow .it.ask.carded .card').length === 2`)
	})
	time.Sleep(500 * time.Millisecond)
	zh.run(`(f => f.scrollTop = f.scrollHeight)(document.querySelector('#chat .flow')); document.activeElement?.blur()`)
	shot(zh, "27-zh-asks")
	zh.done()
	closeZh()
}
