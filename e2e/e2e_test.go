//go:build e2e

// End-to-end acceptance: starts real Claude Code sessions in tmux with the mod, connected to a bridge in this
// process, and uses headless chromium to click through the Coworkers page and check the rendering. It spends real
// tokens, so it has to be turned on explicitly:
//
//	SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/
//
// Optional: SYMPHONY_E2E_MODEL (sonnet by default). Screenshots go to e2e/artifacts/.
// By default the sessions get this repository's mod (--plugin-dir, testing the code at hand); with
// SYMPHONY_E2E_MOD=installed they use the mod installed in Claude Code and get no --plugin-dir.
// TestInstall and TestI18n start no sessions and spend no tokens.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

func requireTools(t *testing.T) {
	if os.Getenv("SYMPHONY_E2E") != "1" {
		t.Skip("starts real Claude sessions and spends tokens; set SYMPHONY_E2E=1 to run")
	}
	for _, bin := range []string{"claude", "tmux"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not found", bin)
		}
	}
	requireBrowser(t)
}

// workDir uses fixed folders (trusted once) and empties them before each run; they live outside the repository,
// so the sessions don't read this repository's CLAUDE.md.
func workDir(t *testing.T, name string) string {
	t.Helper()
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cache, "symphony-e2e", name)
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

// TestOffice is the acceptance with real sessions (its name from the days of the pixel office; CI and the docs name
// it): two sessions started by hand take their seats, a third with a taken token and one without a token stay out;
// in Coworkers their asks (a question, a permission denied with a reason, one allowed) are answered in the
// conversations, A's conversation shows the chat as the mod streamed it, a message between the sessions shows in both
// conversations, a new coworker is hired with "+ Coworker" and talked to through its conversation, and one off duty
// goes back to work from its conversation, in the same seat.
func TestOffice(t *testing.T) {
	requireTools(t)
	model := os.Getenv("SYMPHONY_E2E_MODEL")
	if model == "" {
		model = "sonnet"
	}
	repo, _ := filepath.Abs("..")
	mod := filepath.Join(repo, "mods", "symphony-bridge")
	installed := os.Getenv("SYMPHONY_E2E_MOD") == "installed"
	if installed {
		mod = ""
	}
	stamp := time.Now().Format("150405")
	// the names carry the time, so they don't collide with other sessions on this machine (SendMessage goes by name)
	nameA := "xiaowang-" + stamp // named with --name
	nameB := "后端老李" + stamp      // a non-ASCII name: started without --name, renamed with /rename
	nameD := "ana-" + stamp      // hired with the "+" in the page

	b := startBridge(t, mod)
	t.Logf("bridge: %s", b.URL)
	if installed {
		m := b.Launcher.Mod(context.Background())
		if !m.OK {
			t.Skipf("SYMPHONY_E2E_MOD=installed needs the mod installed first: %s %s", m.Error, m.Fix)
		}
		t.Logf("using the installed mod: %s %s", m.ID, m.Version)
	}
	accA, err := b.Hub.CreateAccount(nameA)
	if err != nil {
		t.Fatal(err)
	}
	accB, _ := b.Hub.CreateAccount(nameB)

	dirA, dirB, dirC, dirE := workDir(t, "a"), workDir(t, "b"), workDir(t, "c"), workDir(t, "e")
	claudeArgs := func(extra ...string) []string {
		args := []string{"--model", model, "--permission-mode", "manual"}
		if mod != "" {
			args = append(args, "--plugin-dir", mod)
		}
		return append(args, extra...)
	}
	env := func(token string) map[string]string {
		// SYMPHONY_LANG pins the mod's messages to English, whatever this machine's locale is
		return map[string]string{"SYMPHONY_BRIDGE": b.URL, "SYMPHONY_TOKEN": token, "SYMPHONY_LANG": "en"}
	}
	paneA := startClaude(t, "sym-e2e-"+stamp+"-a", dirA, env(accA.Token), claudeArgs("--name", nameA)...)
	paneB := startClaude(t, "sym-e2e-"+stamp+"-b", dirB, env(accB.Token), claudeArgs()...)
	dumpAB := func() string { return paneA.dump() + "\n" + paneB.dump() }

	// ---------- 1. accounts, session names, models ----------
	paneB.say("/rename " + nameB)
	var sidA, sidB string
	waitUntil(t, 60*time.Second, "both sessions take their accounts' seats by token, and B's rename shows", func() bool {
		a, bb := b.online(nameA), b.online(nameB)
		if a == nil || bb == nil || bb.Title != nameB {
			return false
		}
		sidA, sidB = a.ID, bb.ID
		return true
	}, dumpAB)
	if s := b.session(sidA); !strings.Contains(s.Model, model) {
		t.Fatalf("the mod should report model %q, got %q", model, s.Model)
	}
	// the mod reports a login only when $.session.identity() gives it one, which it does for a mod an organization's
	// managed settings install; it reads no config file for it
	if s := b.session(sidA); s.Login != "" {
		t.Fatalf("the mod should report no login when managed settings didn't install it, got %q", s.Login)
	}
	// C comes with A's token: A is online, so C is refused and falls back to the terminal
	paneC := startClaude(t, "sym-e2e-"+stamp+"-c", dirC, env(accA.Token), claudeArgs("--name", "intruder-"+stamp)...)
	dumpAll := func() string { return dumpAB() + "\n" + paneC.dump() }
	time.Sleep(2 * time.Second)
	if n := len(b.Hub.Snapshot().Sessions); n != 2 {
		t.Fatalf("a second session of the same account must not get in; there are %d sessions", n)
	}
	// E has no SYMPHONY_TOKEN: not an office session, so the mod must do nothing there
	paneE := startClaude(t, "sym-e2e-"+stamp+"-e", dirE, map[string]string{"SYMPHONY_BRIDGE": b.URL}, claudeArgs("--name", "outsider-"+stamp)...)
	dumpAll = func() string { return dumpAB() + "\n" + paneC.dump() + "\n" + paneE.dump() }

	// ---------- 2. the Coworkers page ----------
	browser := newBrowser(t)
	page := tab(t, browser, b.URL+"/t/messenger/")
	type sideDOM struct {
		Names []string `json:"names"`
		Hire  bool     `json:"hire"`
	}
	var side sideDOM
	waitUntil(t, 30*time.Second, `the sidebar lists the two coworkers and offers "+ Coworker"`, func() bool {
		side = get(t, page, 10*time.Second, chromedp.Evaluate[sideDOM](`({
			names: [...document.querySelectorAll('#people .row .nm')].map(d => d.textContent.trim()),
			hire: (b => !!b && !b.hidden)(document.getElementById('new-emp')),
		})`))
		return len(side.Names) == 2 && side.Hire
	}, func() string { return fmt.Sprintf("%+v", side) })
	joined := strings.Join(side.Names, "|")
	if !strings.Contains(joined, nameA) || !strings.Contains(joined, nameB) {
		t.Fatalf("the sidebar lacks the two names: %s", joined)
	}
	screenshot(t, page, "coworkers-start.png")

	// ---------- 3. the sessions work at the same time ----------
	paneA.say(`First use the AskUserQuestion tool to ask me one question: "Which color do you like?", with exactly two options, "Red" and "Blue". Once you have my answer, use Bash to run echo <the color I picked> > answer.txt (only that one command), then tell me the result.`)
	paneB.say(`Use Bash to run touch denied.txt, only that one command. If it is denied, don't try anything else.`)
	paneC.say(`Use Bash to run touch fallback.txt, only that one command.`)
	paneE.say(`Use Bash to run touch outsider.txt, only that one command.`)

	// ---------- 4. handled in the coworkers' conversations ----------
	handled := map[string]bool{}
	var answeredQuestion, deniedB, allowedA bool
	// press clicks a card's button like a person, and counts it once the answer has reached the bridge and the card
	// has left the window. A click that went nowhere is tried again, a few times.
	press := func(id, button string) {
		t.Helper()
		gone := func() bool {
			return !b.pending(id) && get(t, page, 5*time.Second, chromedp.Evaluate[bool](fmt.Sprintf(`!document.querySelector('#chat .card[data-id="%s"]')`, id)))
		}
		for try := 1; ; try++ {
			do(t, page, 10*time.Second, chromedp.Click(chromedp.CSS(button)))
			if eventually(5*time.Second, gone) {
				return
			}
			if try == 3 || !b.pending(id) {
				t.Fatalf("%s was clicked %d times, but ask %s is still in the window (pending in the bridge: %v)", button, try, id, b.pending(id))
			}
		}
	}
	waitUntil(t, 4*time.Minute, "the conversations handle A's question, B's denial and A's permission", func() bool {
		for _, a := range b.Hub.Snapshot().Asks {
			if handled[a.ID] {
				continue
			}
			// a click on the waiting coworker's row opens its conversation, where the ask's card sits under its line
			clickRow(t, page, a.Account)
			card := fmt.Sprintf(`#chat:not([hidden]) .card[data-id="%s"]`, a.ID)
			do(t, page, 20*time.Second, chromedp.WaitVisible(chromedp.CSS(card)))
			switch {
			case a.Session == sidA && a.Kind == "question":
				screenshot(t, page, "coworkers-chat-ask.png")
				// the model writes the option labels; pick "Blue" whatever its case
				picked := get(t, page, 10*time.Second, chromedp.Evaluate[bool](fmt.Sprintf(
					`(b => b ? (b.click(), true) : false)([...document.querySelectorAll('%s input')].find(i => i.value.trim().toLowerCase() === 'blue'))`, card)))
				if !picked {
					t.Fatalf("no Blue option in the question:\n%s", get(t, page, 5*time.Second, chromedp.Evaluate[string](fmt.Sprintf(`document.querySelector('%s').innerText`, card))))
				}
				press(a.ID, card+` .submit`)
				answeredQuestion = true
			case a.Session == sidB && a.Kind == "permission" && a.Tool == "Bash":
				do(t, page, 10*time.Second, chromedp.SetValue(chromedp.CSS(card+` .reason`), "denied in the page: the e2e test of the deny path"))
				press(a.ID, card+` .deny`)
				deniedB = true
			case a.Session == sidA && a.Kind == "permission" && a.Tool == "Bash":
				press(a.ID, card+` .allow`)
				allowedA = true
			default:
				t.Fatalf("unexpected pending ask: %+v", a)
			}
			handled[a.ID] = true
		}
		return answeredQuestion && deniedB && allowedA
	}, dumpAll)
	// A's conversation shows the session as a chat: the prompt that was typed in the terminal, its thinking and answer
	// as the mod streamed them, the Bash call and the settled asks
	clickRow(t, page, accA.ID)
	waitUntil(t, 2*time.Minute, "A's conversation shows the prompt, the streamed answer, the tool call and the asks", func() bool {
		kinds := get(t, page, 5*time.Second, chromedp.Evaluate[string](`[...document.querySelectorAll('#chat:not([hidden]) .flow .it')].map(e => e.className).join(' ')`))
		return strings.Contains(kinds, "it user") && strings.Contains(kinds, "it text") && strings.Contains(kinds, "it tool ok done") && strings.Contains(kinds, "it ask answered")
	}, func() string {
		return get(t, page, 5*time.Second, chromedp.Evaluate[string](`document.querySelector('#chat .flow')?.innerText ?? ''`))
	})
	screenshot(t, page, "coworkers-chat.png")

	// ---------- 5. results ----------
	waitUntil(t, 2*time.Minute, "A writes the popup's answer to answer.txt: blue", func() bool {
		data, err := os.ReadFile(filepath.Join(dirA, "answer.txt"))
		return err == nil && strings.ToLower(strings.TrimSpace(string(data))) == "blue"
	}, paneA.dump)
	// A started without --effort, so only the mod's turn.step report can tell the effort
	if s := b.session(sidA); s.Effort == "" {
		t.Fatalf("the mod didn't report the thinking effort from turn.step: %+v", s)
	} else {
		t.Logf("A's model and effort: %s · %s", s.Model, s.Effort)
	}
	// what else the mod reports: the engine's version, the mod's, the response tokens summed by model, the context
	// window's fill, the turn count, the tool's duration in the chat
	waitUntil(t, time.Minute, "the mod reports the engine, the usage figures and the turn", func() bool {
		s := b.session(sidA)
		return s.Engine != nil && s.Engine.Version != "" && s.Usage != nil && len(s.Usage.Totals) > 0 && s.Usage.Context != nil && s.Usage.Context.Window > 0 && s.Turns >= 1
	}, func() string { data, _ := json.Marshal(b.session(sidA)); return string(data) })
	if s := b.session(sidA); !installed && s.Mod == "" {
		t.Fatalf("the mod should report its version from the repository's manifest: %+v", s)
	} else {
		t.Logf("A's engine %s, mod %q, totals %v, context %v, cost %v", s.Engine.Version, s.Mod, s.Usage.Totals, s.Usage.Context, s.Usage.Cost)
	}
	if !b.hasEvent(func(ev bridge.Event) bool {
		return ev.Session == sidA && ev.Kind == "tool.end" && ev.Tool == "Bash" && ev.DurationMs > 0
	}) {
		t.Fatal("A's Bash call should carry its duration")
	}
	waitUntil(t, time.Minute, "the bridge hears that B's Bash was denied", func() bool {
		return b.hasEvent(func(ev bridge.Event) bool {
			return ev.Session == sidB && ev.Kind == "tool.end" && ev.Tool == "Bash" && ev.Denied
		})
	}, paneB.dump)
	if exists(filepath.Join(dirB, "denied.txt")) {
		t.Fatal("B's command was denied, so denied.txt must not exist")
	}
	waitUntil(t, 2*time.Minute, "C (busy account) falls back to the terminal's own permission prompt", func() bool {
		return strings.Contains(paneC.capture(), "Do you want to proceed?")
	}, paneC.dump)
	paneC.key("Escape")
	paneC.kill()
	if exists(filepath.Join(dirC, "fallback.txt")) {
		t.Fatal("nobody approved C's command, so fallback.txt must not exist")
	}
	waitUntil(t, 2*time.Minute, "E (no token) uses the terminal's own permission prompt", func() bool {
		return strings.Contains(paneE.capture(), "Do you want to proceed?")
	}, paneE.dump)
	screenE := paneE.capture()
	paneE.key("Escape")
	paneE.kill()
	if n := b.anon.Load(); n != 0 {
		t.Fatalf("a session without a token must not contact the bridge; it got %d requests", n)
	}
	if strings.Contains(screenE, "symphony:") {
		t.Fatalf("a session without a token must show no symphony messages:\n%s", screenE)
	}
	for _, s := range b.Hub.Snapshot().Sessions {
		if strings.HasPrefix(s.Title, "outsider-") {
			t.Fatalf("a session without a token must not enter the office: %+v", s)
		}
	}
	for _, p := range []*pane{paneA, paneB} {
		if strings.Contains(p.capture(), "Do you want to proceed?") {
			t.Fatalf("%s must not show the terminal's permission prompt:\n%s", p.name, p.capture())
		}
	}

	// ---------- 6. messages between sessions: in both conversations ----------
	waitUntil(t, 2*time.Minute, "A finishes this turn", func() bool { return b.session(sidA).State == bridge.StateIdle }, paneA.dump)
	paneA.say(fmt.Sprintf(`Use SendMessage to send the session named %s this sentence: Meeting at 8 tonight. Stop once it is sent and do nothing else.`, nameB))
	waitUntil(t, 3*time.Minute, "the bridge records A's message to B with both ends paired", func() bool {
		for _, m := range b.Hub.Snapshot().Messages {
			if m.From == sidA && m.To == sidB && m.FromName == nameA && m.Received && strings.Contains(strings.ToLower(m.Text), "meeting at 8 tonight") {
				return true
			}
		}
		return false
	}, dumpAB, func() string { data, _ := json.Marshal(b.Hub.Snapshot().Messages); return string(data) })
	var record string
	clickRow(t, page, accB.ID)
	waitUntil(t, 20*time.Second, "B's conversation shows the message, from A", func() bool {
		record = get(t, page, 5*time.Second, chromedp.Evaluate[string](`[...document.querySelectorAll('#chat:not([hidden]) .flow .it.message')].map(e => e.innerText).join('\n')`))
		return strings.Contains(strings.ToLower(record), "meeting at 8 tonight") && strings.Contains(record, nameA)
	}, func() string { return record })
	screenshot(t, page, "coworkers-message.png")
	clickRow(t, page, accA.ID)
	waitUntil(t, 20*time.Second, "A's conversation shows it as sent", func() bool {
		return get(t, page, 5*time.Second, chromedp.Evaluate[bool](`[...document.querySelectorAll('#chat:not([hidden]) .flow .it.message.out')].some(e => e.innerText.toLowerCase().includes('meeting at 8 tonight'))`))
	}, func() string {
		return get(t, page, 5*time.Second, chromedp.Evaluate[string](`document.querySelector('#chat .flow')?.innerText ?? ''`))
	})

	// ---------- 7. hire a new coworker with "+ Coworker" ----------
	dirD := workDir(t, "d")
	do(t, page, 20*time.Second,
		chromedp.Click(chromedp.CSS(`#new-emp`)),
		chromedp.WaitVisible(chromedp.CSS(`#launch form`)),
		chromedp.SetValue(chromedp.CSS(`#launch input[name=name]`), nameD),
		chromedp.Click(chromedp.CSS(`#launch .adv > summary`)), // the folder and the options are in Advanced settings
		chromedp.WaitVisible(chromedp.CSS(`#launch input[name=dir]`)),
		chromedp.SetValue(chromedp.CSS(`#launch input[name=dir]`), dirD),
		chromedp.SetValue(chromedp.CSS(`#launch select[name=model]`), model),
		chromedp.SetValue(chromedp.CSS(`#launch select[name=permissionMode]`), "manual"),
	)
	screenshot(t, page, "coworkers-launch.png")
	do(t, page, 10*time.Second, chromedp.Click(chromedp.CSS(`#launch .go`)))
	var launchD string
	waitUntil(t, 2*time.Minute, "the new employee comes online, matched with its terminal inside the bridge", func() bool {
		d := b.online(nameD)
		if d == nil || d.Launch == "" || d.Attach != "symphony attach "+d.Launch || !b.Launcher.Terms.Live(d.Launch) {
			return false
		}
		launchD = d.Launch
		return d.Cwd == dirD
	}, func() string { data, _ := json.Marshal(b.Hub.Snapshot().Launches); return string(data) })
	if acc := b.account(nameD); acc == nil || acc.Seat != 2 {
		t.Fatalf("the new coworker should take seat 2 (counting from 0): %+v", acc)
	}
	// the page passes its language (headless chromium asks for English), so the mod speaks it too
	for _, l := range b.Hub.Snapshot().Launches {
		if l.Name == nameD && !strings.Contains(l.Command, "SYMPHONY_LANG=en ") {
			t.Fatalf("the launch should carry the page's language: %s", l.Command)
		}
	}
	waitUntil(t, 10*time.Second, "the launch dialog closes by itself and the new coworker's conversation opens", func() bool {
		return get(t, page, 5*time.Second, chromedp.Evaluate[bool](fmt.Sprintf(`document.getElementById('launch').hidden && !document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === %q`, nameD)))
	})
	screenshot(t, page, "coworkers-launched.png")
	// ---------- 7b. the page talks to D through its conversation: the prompt reaches the session, the answer streams back ----------
	do(t, page, 10*time.Second,
		chromedp.SetValue(chromedp.CSS(`#chat .say textarea`), "Reply with exactly the word pineapple and nothing else."),
		chromedp.Click(chromedp.CSS(`#chat .say button`)))
	waitUntil(t, 3*time.Minute, "D's conversation shows the page's prompt as sent, and the streamed answer", func() bool {
		return get(t, page, 5*time.Second, chromedp.Evaluate[bool](
			`!!document.querySelector('#chat .flow .it.user.sent') && [...document.querySelectorAll('#chat .flow .it.text .body')].some(e => e.textContent.toLowerCase().includes('pineapple'))`))
	}, func() string {
		return get(t, page, 5*time.Second, chromedp.Evaluate[string](`document.querySelector('#chat .flow')?.innerText ?? ''`))
	})
	// the bridge's log of D's chat has the page's prompt, sent by the mod, and the answer that streamed (the page
	// shows the answer as it streams; the log marks it done a moment later, at chat.stop)
	var sent, answered bool
	waitUntil(t, 30*time.Second, "the chat log holds the page's prompt as sent and the finished answer", func() bool {
		sent, answered = false, false
		for _, it := range b.Hub.Chat(b.online(nameD).ID) {
			sent = sent || it.Kind == bridge.ChatUser && it.Origin == "page" && it.Status == "sent" && strings.Contains(it.Text, "pineapple")
			answered = answered || it.Kind == bridge.ChatText && it.Done && strings.Contains(strings.ToLower(it.Text), "pineapple")
		}
		return sent && answered
	}, func() string { return fmt.Sprintf("sent=%v answered=%v", sent, answered) })
	screenshot(t, page, "coworkers-chat-say.png")
	// the terminal inside the bridge shows the same conversation, and a window could attach to it
	termD := b.Launcher.Terms.Get(launchD)
	waitUntil(t, 30*time.Second, "D's terminal shows the answer", func() bool { return strings.Contains(strings.ToLower(termD.Screen(40)), "pineapple") },
		func() string { return termD.Screen(40) })
	history, _, detach := termD.Attach(120, 40)
	detach()
	if len(history) == 0 {
		t.Fatal("attaching to D's terminal should give its history")
	}

	// ---------- 7c. /clear typed in D's terminal: the conversation goes on under a new session id, in the same launch ----------
	// (the mod reports the old session's end and the new session's start, in whichever order the engine says them)
	sidD := b.online(nameD).ID
	if err := termD.Write([]byte("/clear")); err != nil {
		t.Fatalf("typing into D's terminal: %v", err)
	}
	time.Sleep(time.Second) // the typeahead picks the command up
	_ = termD.Write([]byte("\r"))
	waitUntil(t, 60*time.Second, "D carries on under a new session id, online in the same launch", func() bool {
		s := b.online(nameD)
		if s == nil || s.ID == sidD || s.Launch != launchD {
			return false
		}
		for _, l := range b.Hub.Snapshot().Launches {
			if l.ID == launchD {
				return l.Status == "online"
			}
		}
		return false
	}, func() string {
		data, _ := json.Marshal(b.Hub.Snapshot().Sessions)
		return string(data) + "\n" + termD.Screen(40)
	})
	if sess := b.Hub.Snapshot().Sessions; len(sess) > 0 { // the old session is off duty with reason clear, not lost
		for _, s := range sess {
			if s.ID == sidD && (s.State != bridge.StateEnded || s.EndReason != "clear") {
				t.Fatalf("D's old session should have ended with reason clear: state %s, reason %q", s.State, s.EndReason)
			}
		}
	}

	// ---------- 8. B goes off duty, then back to work from its conversation: same seat ----------
	seatB := b.account(nameB).Seat
	// B just got A's message and may be working on it; wait until it's idle before sending it home
	waitUntil(t, 2*time.Minute, "B is done with the message and idle", func() bool {
		s := b.online(nameB)
		return s != nil && s.State == bridge.StateIdle
	}, paneB.dump)
	paneB.say("/exit")
	waitUntil(t, 30*time.Second, "B is off duty after /exit", func() bool { return b.online(nameB) == nil }, paneB.dump)
	clickRow(t, page, accB.ID)
	do(t, page, 20*time.Second,
		chromedp.WaitVisible(chromedp.CSS(`#chat .go-work`)),
		chromedp.Click(chromedp.CSS(`#chat .go-work`)),
		chromedp.WaitVisible(chromedp.CSS(`#launch form`)),
	)
	if name := get(t, page, 5*time.Second, chromedp.Evaluate[string](`document.querySelector('#launch input[name=name]').value`)); name != nameB {
		t.Fatalf("putting an employee back to work should fill in the name: %q", name)
	}
	do(t, page, 10*time.Second,
		chromedp.Click(chromedp.CSS(`#launch .adv > summary`)),
		chromedp.WaitVisible(chromedp.CSS(`#launch input[name=dir]`)),
		chromedp.SetValue(chromedp.CSS(`#launch input[name=dir]`), dirB), // B was started by hand, so there is no last folder to fill in
		chromedp.SetValue(chromedp.CSS(`#launch select[name=model]`), model),
		chromedp.SetValue(chromedp.CSS(`#launch select[name=permissionMode]`), "manual"),
		chromedp.Click(chromedp.CSS(`#launch .go`)))
	waitUntil(t, 2*time.Minute, "B is back at work in the same seat", func() bool {
		s := b.online(nameB)
		if s == nil || s.Launch == "" {
			return false
		}
		return s.ID != sidB && b.account(nameB).Seat == seatB && s.Cwd == dirB
	}, func() string { data, _ := json.Marshal(b.Hub.Snapshot().Launches); return string(data) })
	screenshot(t, page, "coworkers-back.png")

	// ---------- 9. off duty: a normal /exit, and the bridge marks it ----------
	paneA.say("/exit")
	waitUntil(t, 30*time.Second, "A is off duty after /exit", func() bool { return b.online(nameA) == nil }, paneA.dump)
}
