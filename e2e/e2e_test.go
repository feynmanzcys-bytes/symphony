//go:build e2e

// End-to-end acceptance: starts real Claude Code sessions in tmux with the mod, connected to a bridge in this
// process, and uses headless chromium to click through the office page's popups and check the rendering. It spends
// real tokens, so it has to be turned on explicitly:
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
	for _, bin := range []string{"claude", "tmux", "chromium"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not found", bin)
		}
	}
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

// loginEmail reads this machine's Claude login email, to check what the mod detects.
func loginEmail() string {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".claude.json")
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		path = filepath.Join(dir, ".claude.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg struct {
		OAuthAccount struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"oauthAccount"`
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg.OAuthAccount.EmailAddress
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

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
	if email := loginEmail(); email != "" {
		waitUntil(t, 10*time.Second, "the mod detects the Claude login email", func() bool { return b.session(sidA).Login == email })
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

	// ---------- 2. the office page ----------
	browser := newBrowser(t)
	office := tab(t, browser, b.URL+"/")
	type officeDOM struct {
		Emps   []string `json:"emps"`
		Adds   int      `json:"adds"`
		Canvas bool     `json:"canvas"`
	}
	var page officeDOM
	waitUntil(t, 30*time.Second, `the office shows two employees and one empty seat with a "+"`, func() bool {
		page = get(t, office, 10*time.Second, chromedp.Evaluate[officeDOM](`({
			emps: [...document.querySelectorAll('.emp-label')].map(d => d.textContent),
			adds: [...document.querySelectorAll('.seat-add')].filter(b => !b.hidden).length,
			canvas: (() => {
				const c = document.querySelector('canvas#office')
				return !!c && c.width > 0 && c.getContext('2d').getImageData(c.width / 2 | 0, c.height / 2 | 0, 1, 1).data[3] > 0
			})(),
		})`))
		return len(page.Emps) == 2 && page.Adds == 1 && page.Canvas
	}, func() string { return fmt.Sprintf("%+v", page) })
	joined := strings.Join(page.Emps, "|")
	if !strings.Contains(joined, nameA) || !strings.Contains(joined, nameB) {
		t.Fatalf("the office lacks the two names: %s", joined)
	}
	screenshot(t, office, "office-start.png")

	// ---------- 3. the sessions work at the same time ----------
	paneA.say(`First use the AskUserQuestion tool to ask me one question: "Which color do you like?", with exactly two options, "Red" and "Blue". Once you have my answer, use Bash to run echo <the color I picked> > answer.txt (only that one command), then tell me the result.`)
	paneB.say(`Use Bash to run touch denied.txt, only that one command. If it is denied, don't try anything else.`)
	paneC.say(`Use Bash to run touch fallback.txt, only that one command.`)
	paneE.say(`Use Bash to run touch outsider.txt, only that one command.`)

	// ---------- 4. handled in the office's popup ----------
	handled := map[string]bool{}
	var answeredQuestion, deniedB, allowedA bool
	waitUntil(t, 4*time.Minute, "the popup handles A's question, B's denial and A's permission", func() bool {
		for _, a := range b.Hub.Snapshot().Asks {
			if handled[a.ID] {
				continue
			}
			card := fmt.Sprintf(`#asks-modal #asks .card[data-id="%s"]`, a.ID)
			do(t, office, 20*time.Second, chromedp.WaitVisible(chromedp.CSS(card)))
			switch {
			case a.Session == sidA && a.Kind == "question":
				screenshot(t, office, "office-popup.png")
				// the model writes the option labels; pick "Blue" whatever its case
				picked := get(t, office, 10*time.Second, chromedp.Evaluate[bool](fmt.Sprintf(
					`(b => b ? (b.click(), true) : false)([...document.querySelectorAll('%s input')].find(i => i.value.trim().toLowerCase() === 'blue'))`, card)))
				if !picked {
					t.Fatalf("no Blue option in the question:\n%s", get(t, office, 5*time.Second, chromedp.Evaluate[string](fmt.Sprintf(`document.querySelector('%s').innerText`, card))))
				}
				do(t, office, 10*time.Second, chromedp.Click(chromedp.CSS(card+` .submit`)))
				answeredQuestion = true
			case a.Session == sidB && a.Kind == "permission" && a.Tool == "Bash":
				do(t, office, 10*time.Second,
					chromedp.SetValue(chromedp.CSS(card+` .reason`), "denied in the page: the e2e test of the deny path"),
					chromedp.Click(chromedp.CSS(card+` .deny`)))
				deniedB = true
			case a.Session == sidA && a.Kind == "permission" && a.Tool == "Bash":
				do(t, office, 10*time.Second, chromedp.Click(chromedp.CSS(card+` .allow`)))
				allowedA = true
			default:
				t.Fatalf("unexpected pending ask: %+v", a)
			}
			handled[a.ID] = true
		}
		return answeredQuestion && deniedB && allowedA
	}, dumpAll)

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

	// ---------- 6. messages between sessions: animation + log ----------
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
	waitUntil(t, 20*time.Second, "the office's message log shows the message", func() bool {
		do(t, office, 5*time.Second, chromedp.Click(chromedp.CSS(`#open-msgs`)))
		record = get(t, office, 5*time.Second, chromedp.Text(chromedp.CSS(`#msgs-panel`)))
		return strings.Contains(strings.ToLower(record), "meeting at 8 tonight") && strings.Contains(record, nameA)
	}, func() string { return record })
	screenshot(t, office, "office-message.png")
	do(t, office, 5*time.Second, chromedp.Click(chromedp.CSS(`#msgs-panel .close`)))

	// ---------- 7. hire a new employee with the empty seat's "+" ----------
	dirD := workDir(t, "d")
	do(t, office, 20*time.Second,
		chromedp.Click(chromedp.CSS(`.seat-add`)),
		chromedp.WaitVisible(chromedp.CSS(`#launch form`)),
		chromedp.SetValue(chromedp.CSS(`#launch input[name=name]`), nameD),
		chromedp.SetValue(chromedp.CSS(`#launch input[name=dir]`), dirD),
		chromedp.SetValue(chromedp.CSS(`#launch select[name=model]`), model),
		chromedp.SetValue(chromedp.CSS(`#launch select[name=permissionMode]`), "manual"),
	)
	screenshot(t, office, "office-launch.png")
	do(t, office, 10*time.Second, chromedp.Click(chromedp.CSS(`#launch .go`)))
	var tmuxD string
	waitUntil(t, 2*time.Minute, "the new employee comes online, matched with its tmux window", func() bool {
		d := b.online(nameD)
		if d == nil || d.Launch == "" || !strings.HasPrefix(d.Attach, "tmux attach -t ") {
			return false
		}
		if tmuxD == "" {
			tmuxD = "sym-" + d.Launch
			t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", tmuxD).Run() })
		}
		return d.Cwd == dirD
	}, func() string { data, _ := json.Marshal(b.Hub.Snapshot().Launches); return string(data) })
	if acc := b.account(nameD); acc == nil || acc.Seat != 2 {
		t.Fatalf("the new employee should take seat 2 (counting from 0): %+v", acc)
	}
	// the page passes its language (headless chromium asks for English), so the mod speaks it too
	for _, l := range b.Hub.Snapshot().Launches {
		if l.Name == nameD && !strings.Contains(l.Command, "SYMPHONY_LANG=en ") {
			t.Fatalf("the launch should carry the page's language: %s", l.Command)
		}
	}
	waitUntil(t, 10*time.Second, "the launch dialog closes by itself", func() bool {
		return get(t, office, 5*time.Second, chromedp.Evaluate[bool](`document.getElementById('launch').hidden`))
	})
	screenshot(t, office, "office-launched.png")

	// ---------- 8. B goes off duty, then back to work from its card: same seat ----------
	seatB := b.account(nameB).Seat
	// B just got A's message and may be working on it; wait until it's idle before sending it home
	waitUntil(t, 2*time.Minute, "B is done with the message and idle", func() bool {
		s := b.online(nameB)
		return s != nil && s.State == bridge.StateIdle
	}, paneB.dump)
	paneB.say("/exit")
	waitUntil(t, 30*time.Second, "B is off duty after /exit", func() bool { return b.online(nameB) == nil }, paneB.dump)
	clickLabel(t, office, accB.ID) // B may still be walking out of the door
	do(t, office, 20*time.Second,
		chromedp.WaitVisible(chromedp.CSS(`#card .go-work`)),
		chromedp.Click(chromedp.CSS(`#card .go-work`)),
		chromedp.WaitVisible(chromedp.CSS(`#launch form`)),
	)
	if name := get(t, office, 5*time.Second, chromedp.Evaluate[string](`document.querySelector('#launch input[name=name]').value`)); name != nameB {
		t.Fatalf("putting an employee back to work should fill in the name: %q", name)
	}
	do(t, office, 10*time.Second,
		chromedp.SetValue(chromedp.CSS(`#launch input[name=dir]`), dirB), // B was started by hand, so there is no last folder to fill in
		chromedp.SetValue(chromedp.CSS(`#launch select[name=model]`), model),
		chromedp.SetValue(chromedp.CSS(`#launch select[name=permissionMode]`), "manual"),
		chromedp.Click(chromedp.CSS(`#launch .go`)))
	var tmuxB string
	waitUntil(t, 2*time.Minute, "B is back at work in the same seat", func() bool {
		s := b.online(nameB)
		if s == nil || s.Launch == "" {
			return false
		}
		if tmuxB == "" {
			tmuxB = "sym-" + s.Launch
			t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", tmuxB).Run() })
		}
		return s.ID != sidB && b.account(nameB).Seat == seatB && s.Cwd == dirB
	}, func() string { data, _ := json.Marshal(b.Hub.Snapshot().Launches); return string(data) })
	screenshot(t, office, "office-back.png")

	// ---------- 9. off duty: a normal /exit, and the bridge marks it ----------
	paneA.say("/exit")
	waitUntil(t, 30*time.Second, "A is off duty after /exit", func() bool { return b.online(nameA) == nil }, paneA.dump)
}
