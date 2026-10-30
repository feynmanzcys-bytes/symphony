// Fills a bridge with made-up activity to show off the office: go run ./cmd/demo [-base http://127.0.0.1:7788] [-seconds 60]
// It creates a few demo accounts (skipping names that exist already) and reports events with their tokens.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"time"

	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
)

var base string

type member struct{ id, name, token string }

func post(token, path string, body any) map[string]any {
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", base+path, bytes.NewReader(data))
	req.Header.Set("content-type", "application/json")
	if token != "" {
		req.Header.Set("authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func event(m member, kind string, extra map[string]any) {
	body := map[string]any{"session": m.id, "kind": kind}
	for k, v := range extra {
		body[k] = v
	}
	post(m.token, "/api/event", body)
}

// errorText is the bridge's error in this language when the catalog knows its code, its English text otherwise.
func errorText(res map[string]any) string {
	if code, _ := res["code"].(string); code != "" && i18n.Has("error."+code) {
		params := map[string]string{}
		if p, ok := res["params"].(map[string]any); ok {
			for k, v := range p {
				params[k] = fmt.Sprint(v)
			}
		}
		return i18n.TMap("error."+code, params)
	}
	return fmt.Sprint(res["error"])
}

func main() {
	i18n.SetLang(i18n.Detect(os.Getenv))
	flag.StringVar(&base, "base", "http://127.0.0.1:7788", i18n.T("demo.flag.base"))
	seconds := flag.Float64("seconds", 0, i18n.T("demo.flag.seconds"))
	flag.Parse()

	// name, model, effort, role: every character has its own size and muscles, and the names come from around the
	// world. Two wait for you (a question, a permission), some keep busy, and the idle ones have nothing to do: they
	// wander around the office, and nap after a while.
	roster := [][4]string{
		{"demo-Alice", "haiku", "low", "ask"}, {"demo-小王", "sonnet", "medium", "ask"}, {"demo-Yuki", "sonnet", "high", "work"},
		{"demo-Carlos", "opus", "high", "work"}, {"demo-Amélie", "opus", "max", "work"}, {"demo-Олег", "fable", "max", "work"},
		{"demo-Priya", "sonnet", "high", "idle"}, {"demo-Kwame", "opus", "medium", "idle"}, {"demo-Lena", "haiku", "low", "idle"},
		{"demo-Mateo", "fable", "xhigh", "idle"},
	}
	var team, askers, workers []member
	for _, r := range roster {
		res := post("", "/api/accounts", map[string]string{"name": r[0]})
		token, _ := res["token"].(string)
		if token == "" {
			log.Print(i18n.T("demo.create_failed", "name", r[0], "error", errorText(res)))
			continue
		}
		m := member{id: fmt.Sprintf("demo-%06x", rand.IntN(1<<24)), name: r[0], token: token}
		team = append(team, m)
		switch r[3] {
		case "ask":
			askers = append(askers, m)
		case "work":
			workers = append(workers, m)
		}
		event(m, "session.start", map[string]any{"cwd": "/work/" + r[0], "model": r[1], "effort": r[2]})
		event(m, "session.title", map[string]any{"title": r[0]})
	}
	if len(team) < 2 || len(workers) == 0 {
		log.Fatal(i18n.T("demo.not_enough"))
	}
	// leave one question and one permission pending
	if len(askers) > 0 {
		post(askers[0].token, "/api/ask", map[string]any{"session": askers[0].id, "kind": "question", "questions": []map[string]any{{
			"question": "Which layout should the home page use?", "header": "Layout", "multiSelect": false,
			"options": []map[string]string{{"label": "Single column", "description": "One column"}, {"label": "Two columns", "description": "Left and right"}},
		}}})
	}
	if len(askers) > 1 {
		post(askers[1].token, "/api/ask", map[string]any{"session": askers[1].id, "kind": "permission", "tool": "Bash", "input": map[string]any{"command": "rm -rf build"}})
	}

	tools := []string{"Bash", "Read", "Edit", "Grep", "Write", "WebFetch"}
	lines := []string{"The API is ready, please try it on your side", "Meeting at 8 tonight", "12 of 13 test cases pass", "Could you look at this error?", "The design mockups are updated"}
	end := time.Now().Add(time.Duration(*seconds * float64(time.Second)))
	last := time.Now()
	for *seconds == 0 || time.Now().Before(end) {
		if time.Since(last) > 10*time.Second { // heartbeats, so nobody counts as lost
			for _, m := range team {
				event(m, "heartbeat", nil)
			}
			last = time.Now()
		}
		m := workers[rand.IntN(len(workers))]
		agent := ""
		if rand.Float64() < 0.25 {
			agent = fmt.Sprintf("agent-%d", rand.IntN(3))
		}
		event(m, "turn.start", map[string]any{"text": "demo", "agentId": agent})
		tool := tools[rand.IntN(len(tools))]
		event(m, "tool.start", map[string]any{"tool": tool, "agentId": agent})
		time.Sleep(time.Duration(300+rand.IntN(900)) * time.Millisecond)
		event(m, "tool.end", map[string]any{"tool": tool, "agentId": agent})
		if rand.Float64() < 0.35 {
			event(m, "turn.complete", map[string]any{"durationMs": 1200, "agentId": agent})
		}
		if rand.Float64() < 0.15 {
			to := team[rand.IntN(len(team))]
			if to.id != m.id {
				text := lines[rand.IntN(len(lines))]
				wrapped := fmt.Sprintf("<cross-session-message from=\"uds:/tmp/x.sock\" from-name=%q from-mode=\"prompting\">\n%s\n</cross-session-message>", m.name, text)
				event(to, "message.receive", map[string]any{"text": wrapped, "origin": map[string]any{"kind": "peer"}})
				event(m, "message.send", map[string]any{"to": to.name, "text": text, "origin": "model", "delivered": true})
			}
		}
	}
}
