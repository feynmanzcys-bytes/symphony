package bridge

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestPushesAreEncodedWhenMade: a push carries the state as it was when it was made, whenever a subscriber reads it:
// a chat item pushed whole and then streamed on comes out with the text it had then, so the delta after it isn't
// doubled on the page.
func TestPushesAreEncodedWhenMade(t *testing.T) {
	hub := NewHub()
	acc, err := hub.CreateAccount("a")
	if err != nil {
		t.Fatal(err)
	}
	if err := hub.Apply(Event{Session: "s1", Kind: "session.start"}, acc); err != nil {
		t.Fatal(err)
	}
	ch, stop := hub.Subscribe()
	defer stop()
	<-ch // the first snapshot
	for _, text := range []string{"Hello", ", world"} {
		if err := hub.Apply(Event{Session: "s1", Kind: "chat.delta", TurnID: "t1", Blocks: []ChatDelta{{Kind: ChatText, Text: text}}}, acc); err != nil {
			t.Fatal(err)
		}
	}
	var item struct {
		Item  *ChatItem      `json:"item"`
		Delta map[string]any `json:"delta"`
	}
	if err := json.Unmarshal((<-ch).JSON(), &item); err != nil || item.Item == nil || item.Item.Text != "Hello" {
		t.Fatalf("the item should be pushed as it was then: %+v (%v)", item.Item, err)
	}
	if err := json.Unmarshal((<-ch).JSON(), &item); err != nil || item.Delta["text"] != ", world" {
		t.Fatalf("then the delta: %+v (%v)", item.Delta, err)
	}
}

// TestPushesRaceNothing: subscribers reading every push's JSON, and /api/state, while the hub streams the chat, adds up
// tokens of new models, keeps the todos and the usage, and pairs the two halves of messages: nothing they read may be
// changing under them (go test -race; a map read while written is a fatal error even without it).
func TestPushesRaceNothing(t *testing.T) {
	hub := NewHub()
	srv := NewServer(hub, testWeb)
	a, err := hub.CreateAccount("a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := hub.CreateAccount("b")
	if err != nil {
		t.Fatal(err)
	}
	apply := func(acc *Account, ev Event) {
		if err := hub.Apply(ev, acc); err != nil {
			t.Error(err)
		}
	}
	apply(a, Event{Session: "sa", Kind: "session.start"})
	apply(b, Event{Session: "sb", Kind: "session.start"})

	done := make(chan struct{})
	var readers sync.WaitGroup
	for range 3 {
		ch, unsubscribe := hub.Subscribe()
		readers.Add(1)
		go func() {
			defer readers.Done()
			defer unsubscribe()
			for {
				select {
				case p := <-ch:
					if !json.Valid(p.JSON()) {
						t.Error("a push isn't JSON")
					}
				case <-done:
					return
				}
			}
		}()
	}
	readers.Add(1)
	go func() { // the page's /api/state is encoded once the lock is let go
		defer readers.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			req := httptest.NewRequest("GET", "/api/state", nil)
			req.Host = "127.0.0.1"
			req.RemoteAddr = "127.0.0.1:1234"
			req.Header.Set("X-Symphony-Key", srv.Key)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			if rec.Code != 200 || !json.Valid(rec.Body.Bytes()) {
				t.Errorf("/api/state: %d", rec.Code)
			}
		}
	}()

	const rounds = 150
	var writers sync.WaitGroup
	writers.Add(2)
	go func() { // a's session streams, counts the tokens of a new model each step, keeps its todos and usage
		defer writers.Done()
		for i := range rounds {
			step, id := i, itoa(i)
			apply(a, Event{Session: "sa", Kind: "chat.delta", TurnID: "t", Step: step, Blocks: []ChatDelta{{I: 0, Kind: ChatThinking, Text: "hm"}, {I: 1, Kind: ChatText, Text: "a"}}})
			apply(a, Event{Session: "sa", Kind: "chat.delta", TurnID: "t", Step: step, Blocks: []ChatDelta{{I: 0, Kind: ChatThinking, Text: "m"}, {I: 1, Kind: ChatText, Text: "b"}}})
			apply(a, Event{Session: "sa", Kind: "chat.stop", TurnID: "t", Step: step, Tokens: &Tokens{Input: 1, Output: 2, Model: "model-" + id}})
			apply(a, Event{Session: "sa", Kind: "todo.create", Todo: id, Subject: "new"})
			apply(a, Event{Session: "sa", Kind: "todo.create", Todo: id, Subject: "renamed", Teammate: "b"})
			apply(a, Event{Session: "sa", Kind: "todo.done", Todo: itoa(i - 1)})
			apply(a, Event{Session: "sa", Kind: "session.usage", Usage: &Usage{Cost: &Cost{USD: float64(i)},
				Context: &ContextUsage{Tokens: int64(i), Window: 200000}, RateLimits: []RateLimit{{Kind: "five_hour", PercentUsed: float64(i % 100)}}}})
		}
	}()
	go func() { // messages from a to b: the receive completes the message the send pushed
		defer writers.Done()
		for i := range rounds {
			text := "hello " + itoa(i)
			apply(a, Event{Session: "sa", Kind: "message.send", To: "b", Text: text, Delivered: true})
			apply(b, Event{Session: "sb", Kind: "message.receive", Origin: "peer",
				Text: `<cross-session-message from="sa" from-name="a">` + text + `</cross-session-message>`})
		}
	}()
	writers.Wait()
	close(done)
	readers.Wait()

	snap := hub.Snapshot()
	var sa *Session
	for _, s := range snap.Sessions {
		if s.ID == "sa" {
			sa = s
		}
	}
	if sa == nil || sa.Usage == nil || len(sa.Usage.Totals) != rounds || len(sa.Todos) != 1 || sa.Todos[0].Subject != "renamed" {
		t.Fatalf("the session should have every model's tokens and the last todo: %+v", sa)
	}
	if n := len(snap.Messages); n != rounds || !snap.Messages[0].Received || !strings.HasPrefix(snap.Messages[0].Text, "hello") {
		t.Fatalf("every message should be paired: %d %+v", n, snap.Messages[0])
	}
}
