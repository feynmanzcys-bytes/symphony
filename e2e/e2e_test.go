//go:build e2e

// 端到端验收：在 tmux 里起三个真的 Claude Code 会话，带 mod 连进程内 bridge，
// 用无头 chromium 在审批台上点按钮，在办公室页面上检查渲染。会花真实 token，要显式打开：
//
//	SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/
//
// 可选：SYMPHONY_E2E_MODEL（默认 sonnet）。截图留在 e2e/artifacts/。
package e2e

import (
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
		t.Skip("会起真实 Claude 会话、花 token；设置 SYMPHONY_E2E=1 才跑")
	}
	for _, bin := range []string{"claude", "tmux", "chromium"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("缺 %s", bin)
		}
	}
}

// workDir 用固定目录（只需信任一次），每次跑之前清空；放在仓库外面，免得读到本仓库的 CLAUDE.md。
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

// loginEmail 读本机 Claude 登录邮箱，用来核对 mod 自动识别的结果。
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

func TestLegion(t *testing.T) {
	requireTools(t)
	model := os.Getenv("SYMPHONY_E2E_MODEL")
	if model == "" {
		model = "sonnet"
	}
	repo, _ := filepath.Abs("..")
	mod := filepath.Join(repo, "mods", "symphony-bridge")
	stamp := time.Now().Format("150405")
	// 名字带上时间，免得和本机其它同名会话撞上（SendMessage 按名字找人）
	nameA := "xiaowang-" + stamp // 用 --name 起名
	nameB := "后端老李" + stamp      // 不带 --name 启动，进去 /rename
	nameD := "产品小美" + stamp      // 在页面上点“+”新招的

	b := startBridge(t, mod)
	t.Logf("bridge: %s", b.URL)
	accA, err := b.Hub.CreateAccount(nameA)
	if err != nil {
		t.Fatal(err)
	}
	accB, _ := b.Hub.CreateAccount(nameB)

	dirA, dirB, dirC := workDir(t, "a"), workDir(t, "b"), workDir(t, "c")
	claudeArgs := func(extra ...string) []string {
		return append([]string{"--model", model, "--permission-mode", "manual", "--plugin-dir", mod}, extra...)
	}
	env := func(token string) map[string]string {
		return map[string]string{"SYMPHONY_BRIDGE": b.URL, "SYMPHONY_TOKEN": token}
	}
	paneA := startClaude(t, "sym-e2e-"+stamp+"-a", dirA, env(accA.Token), claudeArgs("--name", nameA)...)
	paneB := startClaude(t, "sym-e2e-"+stamp+"-b", dirB, env(accB.Token), claudeArgs()...)
	dumpAB := func() string { return paneA.dump() + "\n" + paneB.dump() }

	// ---------- 1. 账号、会话名、模型 ----------
	paneB.say("/rename " + nameB)
	var sidA, sidB string
	waitUntil(t, 60*time.Second, "两个会话凭 token 坐上各自账号的座位，B 改名生效", func() bool {
		a, bb := b.online(nameA), b.online(nameB)
		if a == nil || bb == nil || bb.Title != nameB {
			return false
		}
		sidA, sidB = a.ID, bb.ID
		return true
	}, dumpAB)
	if s := b.session(sidA); !strings.Contains(s.Model, model) {
		t.Fatalf("mod 应报上模型 %q，得到 %q", model, s.Model)
	}
	if email := loginEmail(); email != "" {
		waitUntil(t, 10*time.Second, "mod 识别出 Claude 登录邮箱", func() bool { return b.session(sidA).Login == email })
	}
	// C 拿 A 的 token 来：A 在线，C 应被拒，退回终端
	paneC := startClaude(t, "sym-e2e-"+stamp+"-c", dirC, env(accA.Token), claudeArgs("--name", "intruder-"+stamp)...)
	dumpAll := func() string { return dumpAB() + "\n" + paneC.dump() }
	time.Sleep(2 * time.Second)
	if n := len(b.Hub.Snapshot().Sessions); n != 2 {
		t.Fatalf("同一账号的第二个会话不该进来，现在 %d 个会话", n)
	}

	// ---------- 2. 办公室页面 ----------
	browser := newBrowser(t)
	office := tab(t, browser, b.URL+"/")
	type officeDOM struct {
		Emps   []string `json:"emps"`
		Adds   int      `json:"adds"`
		Canvas bool     `json:"canvas"`
	}
	var page officeDOM
	waitUntil(t, 30*time.Second, "办公室画出两个员工和一个带“+”的空座位", func() bool {
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
		t.Fatalf("办公室里没有两个人的名字: %s", joined)
	}
	screenshot(t, office, "office-start.png")

	// ---------- 3. 三个会话同时干活 ----------
	paneA.say(`请先用 AskUserQuestion 工具问我一个问题："你喜欢哪个颜色？"，选项只有"红"和"蓝"两个。拿到回答后，用 Bash 执行 echo <我选的颜色> > answer.txt ，只执行这一条，然后告诉我结果。`)
	paneB.say(`用 Bash 执行 touch denied.txt ，只执行这一条，被拒绝也不要换别的办法。`)
	paneC.say(`用 Bash 执行 touch fallback.txt ，只执行这一条。`)

	// ---------- 4. 在办公室的弹窗里处理 ----------
	handled := map[string]bool{}
	var answeredQuestion, deniedB, allowedA bool
	waitUntil(t, 4*time.Minute, "弹窗里处理完 A 的提问、B 的拒绝、A 的放行", func() bool {
		for _, a := range b.Hub.Snapshot().Asks {
			if handled[a.ID] {
				continue
			}
			card := fmt.Sprintf(`#asks-modal #asks .card[data-id="%s"]`, a.ID)
			do(t, office, 20*time.Second, chromedp.WaitVisible(chromedp.CSS(card)))
			switch {
			case a.Session == sidA && a.Kind == "question":
				screenshot(t, office, "office-popup.png")
				do(t, office, 10*time.Second,
					chromedp.Click(chromedp.CSS(card+` input[value="蓝"]`)),
					chromedp.Click(chromedp.CSS(card+` .submit`)))
				answeredQuestion = true
			case a.Session == sidB && a.Kind == "permission" && a.Tool == "Bash":
				do(t, office, 10*time.Second,
					chromedp.SetValue(chromedp.CSS(card+` .reason`), "网页端拒绝：e2e 测试拒绝路径"),
					chromedp.Click(chromedp.CSS(card+` .deny`)))
				deniedB = true
			case a.Session == sidA && a.Kind == "permission" && a.Tool == "Bash":
				do(t, office, 10*time.Second, chromedp.Click(chromedp.CSS(card+` .allow`)))
				allowedA = true
			default:
				t.Fatalf("没料到的待决事项: %+v", a)
			}
			handled[a.ID] = true
		}
		return answeredQuestion && deniedB && allowedA
	}, dumpAll)

	// ---------- 5. 结果 ----------
	waitUntil(t, 2*time.Minute, "A 按弹窗里的回答写出 answer.txt = 蓝", func() bool {
		data, err := os.ReadFile(filepath.Join(dirA, "answer.txt"))
		return err == nil && strings.TrimSpace(string(data)) == "蓝"
	}, paneA.dump)
	// A 启动时没给 --effort，强度只能靠 mod 在 turn.step 里报上来
	if s := b.session(sidA); s.Effort == "" {
		t.Fatalf("mod 没从 turn.step 报上思考强度: %+v", s)
	} else {
		t.Logf("A 的模型和强度：%s · %s", s.Model, s.Effort)
	}
	waitUntil(t, time.Minute, "bridge 收到 B 的 Bash 被拒", func() bool {
		return b.hasEvent(func(ev bridge.Event) bool {
			return ev.Session == sidB && ev.Kind == "tool.end" && ev.Tool == "Bash" && ev.Denied
		})
	}, paneB.dump)
	if exists(filepath.Join(dirB, "denied.txt")) {
		t.Fatal("B 的命令被拒了，denied.txt 不应存在")
	}
	waitUntil(t, 2*time.Minute, "C（账号被占用）退回终端原生审批框", func() bool {
		return strings.Contains(paneC.capture(), "Do you want to proceed?")
	}, paneC.dump)
	paneC.key("Escape")
	paneC.kill()
	if exists(filepath.Join(dirC, "fallback.txt")) {
		t.Fatal("C 的命令没人批，fallback.txt 不应存在")
	}
	for _, p := range []*pane{paneA, paneB} {
		if strings.Contains(p.capture(), "Do you want to proceed?") {
			t.Fatalf("%s 不应弹终端审批框:\n%s", p.name, p.capture())
		}
	}

	// ---------- 6. 会话间消息：动画 + 记录 ----------
	waitUntil(t, 2*time.Minute, "A 这一轮做完", func() bool { return b.session(sidA).State == bridge.StateIdle }, paneA.dump)
	paneA.say(fmt.Sprintf(`用 SendMessage 给名叫 %s 的会话发一句：今晚八点开会。发完就结束，不要做别的。`, nameB))
	waitUntil(t, 3*time.Minute, "bridge 记下 A 发给 B 的消息，两头都对上", func() bool {
		for _, m := range b.Hub.Snapshot().Messages {
			if m.From == sidA && m.To == sidB && m.FromName == nameA && m.Received && strings.Contains(m.Text, "今晚八点开会") {
				return true
			}
		}
		return false
	}, dumpAB, func() string { data, _ := json.Marshal(b.Hub.Snapshot().Messages); return string(data) })
	var record string
	waitUntil(t, 20*time.Second, "办公室的消息记录里有这条消息", func() bool {
		do(t, office, 5*time.Second, chromedp.Click(chromedp.CSS(`#open-msgs`)))
		record = get(t, office, 5*time.Second, chromedp.Text(chromedp.CSS(`#msgs-panel`)))
		return strings.Contains(record, "今晚八点开会") && strings.Contains(record, nameA)
	}, func() string { return record })
	screenshot(t, office, "office-message.png")
	do(t, office, 5*time.Second, chromedp.Click(chromedp.CSS(`#msgs-panel .close`)))

	// ---------- 7. 点空座位的“+”招一个新员工 ----------
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
	waitUntil(t, 2*time.Minute, "新员工上线，和 tmux 对上", func() bool {
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
		t.Fatalf("新员工应坐 2 号座位（从 0 数）: %+v", acc)
	}
	waitUntil(t, 10*time.Second, "启动框自己关掉", func() bool {
		return get(t, office, 5*time.Second, chromedp.Evaluate[bool](`document.getElementById('launch').hidden`))
	})
	screenshot(t, office, "office-launched.png")

	// ---------- 8. B 下班，再从卡片上让他上班：还是原来的座位 ----------
	seatB := b.account(nameB).Seat
	// B 刚收到 A 的消息，可能正在处理，等它闲下来再让它下班
	waitUntil(t, 2*time.Minute, "B 处理完消息闲下来", func() bool {
		s := b.online(nameB)
		return s != nil && s.State == bridge.StateIdle
	}, paneB.dump)
	paneB.say("/exit")
	waitUntil(t, 30*time.Second, "B 退出后标成下班", func() bool { return b.online(nameB) == nil }, paneB.dump)
	do(t, office, 20*time.Second,
		chromedp.Click(chromedp.CSS(fmt.Sprintf(`.emp-label[data-account="%s"]`, accB.ID))),
		chromedp.WaitVisible(chromedp.CSS(`#card .go-work`)),
		chromedp.Click(chromedp.CSS(`#card .go-work`)),
		chromedp.WaitVisible(chromedp.CSS(`#launch form`)),
	)
	if name := get(t, office, 5*time.Second, chromedp.Evaluate[string](`document.querySelector('#launch input[name=name]').value`)); name != nameB {
		t.Fatalf("让老员工上班时名字应带出来: %q", name)
	}
	do(t, office, 10*time.Second,
		chromedp.SetValue(chromedp.CSS(`#launch input[name=dir]`), dirB), // B 当初是手动起的，没有上次的目录可带
		chromedp.SetValue(chromedp.CSS(`#launch select[name=model]`), model),
		chromedp.SetValue(chromedp.CSS(`#launch select[name=permissionMode]`), "manual"),
		chromedp.Click(chromedp.CSS(`#launch .go`)))
	var tmuxB string
	waitUntil(t, 2*time.Minute, "B 重新上班，还是原来的座位", func() bool {
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

	// ---------- 9. 下班：正常退出（/exit），bridge 标成下班 ----------
	paneA.say("/exit")
	waitUntil(t, 30*time.Second, "A 退出后标成下班", func() bool { return b.online(nameA) == nil }, paneA.dump)
}
