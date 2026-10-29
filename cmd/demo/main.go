// 往 bridge 灌假数据，看办公室效果：go run ./cmd/demo [-base http://127.0.0.1:7788] [-seconds 60]
// 会建几个 demo 账号（已有同名的就跳过），用它们的 token 上报事件。
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"time"
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

func main() {
	flag.StringVar(&base, "base", "http://127.0.0.1:7788", "bridge 地址")
	seconds := flag.Float64("seconds", 0, "跑多久，0 表示一直跑")
	flag.Parse()

	// 名字、模型、强度：块头和肌肉各不相同
	roster := [][3]string{
		{"demo-小王", "haiku", "low"}, {"demo-老李", "sonnet", "medium"}, {"demo-阿珍", "sonnet", "high"},
		{"demo-大刘", "opus", "high"}, {"demo-小美", "opus", "max"}, {"demo-老周", "fable", "max"},
	}
	var team []member
	for _, r := range roster {
		res := post("", "/api/accounts", map[string]string{"name": r[0]})
		token, _ := res["token"].(string)
		if token == "" {
			log.Printf("%s 建不了（可能已经有了）：%v", r[0], res["error"])
			continue
		}
		m := member{id: fmt.Sprintf("demo-%06x", rand.IntN(1<<24)), name: r[0], token: token}
		team = append(team, m)
		event(m, "session.start", map[string]any{"cwd": "/work/" + r[0], "model": r[1], "effort": r[2]})
		event(m, "session.title", map[string]any{"title": r[0]})
	}
	if len(team) < 2 {
		log.Fatal("demo 账号不够，先删掉旧的 demo 账号再跑")
	}
	// 留一个待审批、一个待回答
	post(team[1].token, "/api/ask", map[string]any{"session": team[1].id, "kind": "permission", "tool": "Bash", "input": map[string]any{"command": "rm -rf build"}})
	post(team[0].token, "/api/ask", map[string]any{"session": team[0].id, "kind": "question", "questions": []map[string]any{{
		"question": "首页用哪种布局？", "header": "布局", "multiSelect": false,
		"options": []map[string]string{{"label": "单栏", "description": "一列"}, {"label": "双栏", "description": "左右两栏"}},
	}}})

	tools := []string{"Bash", "Read", "Edit", "Grep", "Write", "WebFetch"}
	lines := []string{"接口改好了，你那边联调一下", "今晚八点开会", "测试用例过了 12/13", "帮我看下这个报错", "设计稿更新了"}
	end := time.Now().Add(time.Duration(*seconds * float64(time.Second)))
	last := time.Now()
	for *seconds == 0 || time.Now().Before(end) {
		if time.Since(last) > 10*time.Second { // 心跳，免得被当成失联
			for _, m := range team {
				event(m, "heartbeat", nil)
			}
			last = time.Now()
		}
		m := team[2+rand.IntN(max(1, len(team)-2))%len(team)]
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
