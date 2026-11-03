//go:build e2e

package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// TestChatSteps walks through the steps between answers in the employee window with made-up activity (no tokens): a
// prompt, then a fold that waits for its first step and counts from the prompt; the thinking and the tool calls folding
// into it, folded away; the line counting up ("Thinking… 2m 38s", the page's clock moved on) with a screen reader's
// name that has no time in it; the answer's text ending the fold ("Thought for …"); a pending ask staying out of the
// next fold until it's settled, then joining it; the turn's end finishing the last fold; a click unfolding it; the
// settings unfolding every fold, new ones too, after a reload as well. In Coworkers, where the asks' cards sit in the
// flow, a pending ask's card stays outside the folds, and motion paused stops the count.
func TestChatSteps(t *testing.T) {
	requireBrowser(t)
	b := startBridge(t, "")
	alice, err := b.Hub.CreateAccount("alice")
	if err != nil {
		t.Fatal(err)
	}
	ev := func(body map[string]any) {
		t.Helper()
		body["session"] = "s-a"
		b.post(t, alice.Token, "/api/event", body)
	}
	ev(map[string]any{"kind": "session.start", "cwd": "/work/app", "model": "claude-opus-5-5"})
	browser := newBrowser(t)
	ctx, _ := clockTab(t, browser, b.URL+"/t/office/?lang=en", 0)
	do(t, ctx, 10*time.Second, chromedp.EmulateViewport(1400, 900))
	p := &screens{t: t, ctx: ctx, lang: "steps"}
	label := fmt.Sprintf(`document.querySelector('.emp-label[data-account=%q]')`, alice.ID)
	open := func() {
		t.Helper()
		waitUntil(t, 20*time.Second, "the office shows alice", func() bool { return p.is(`!!` + label) })
		p.run(label + `.click()`)
		waitUntil(t, 10*time.Second, "alice's window opens", func() bool { return p.is(`!document.getElementById('chat').hidden`) })
	}
	open()
	flow := func() string { return p.js(`document.querySelector('#chat .flow').innerText`) }

	// ---------- a prompt: a fold waits for its first step, counting from the prompt ----------
	ev(map[string]any{"kind": "chat.user", "text": "Fix the build", "origin": "composer"})
	ev(map[string]any{"kind": "turn.start", "text": "Fix the build"})
	waitUntil(t, 10*time.Second, "a fold waits for its first step and counts", func() bool {
		return p.is(`(f => !!f && f.classList.contains('running') && /^Thinking… \d+s$/.test(f.querySelector('.lbl').textContent) && !f.querySelector('details, summary'))(document.querySelector('#chat .steps.ghost'))`)
	}, flow) // (nothing to unfold yet: a line)

	// ---------- the steps fold into it, folded away ----------
	ev(map[string]any{"kind": "chat.delta", "turnId": "a1", "step": 0, "blocks": []map[string]any{{"i": 0, "kind": "thinking", "text": "The log first."}}})
	ev(map[string]any{"kind": "chat.stop", "turnId": "a1", "step": 0})
	ev(map[string]any{"kind": "tool.start", "tool": "Read", "summary": "build.log", "toolUseId": "tu1"})
	ev(map[string]any{"kind": "tool.end", "tool": "Read", "summary": "build.log", "toolUseId": "tu1", "durationMs": 1200})
	waitUntil(t, 10*time.Second, "the thinking and the tool call fold into one line", func() bool {
		return p.is(`document.querySelectorAll('#chat .steps').length === 1 && !document.querySelector('#chat .steps.ghost') && document.querySelectorAll('#chat .steps .list > .it').length === 2 && !document.querySelector('#chat .steps > details').open`)
	}, flow)
	if p.is(`document.querySelector('#chat .steps .it.tool').checkVisibility()`) {
		t.Fatal("folded, the steps shouldn't show")
	}
	// the page's clock moves on 2 min 38 s: the line counts up; a screen reader reads it without the time
	p.run(`window.__warp = 158000`)
	waitUntil(t, 5*time.Second, "the line counts up", func() bool {
		return p.is(`/^Thinking… 2m 3[89]s$/.test(document.querySelector('#chat .steps > details > summary .lbl').textContent)`)
	}, func() string { return p.js(`document.querySelector('#chat .steps > details > summary').textContent`) })
	if name := p.js(`document.querySelector('#chat .steps > details > summary').getAttribute('aria-label')`); name != "Thinking…" {
		t.Errorf("while it counts, the line's name should leave the time out: %q", name)
	}
	p.shot("running")
	p.run(`window.__warp = 0`)

	// ---------- the answer's text ends it ----------
	ev(map[string]any{"kind": "chat.delta", "turnId": "a1", "step": 1, "blocks": []map[string]any{{"i": 0, "kind": "text", "text": "The build needs Node 22."}}})
	ev(map[string]any{"kind": "chat.stop", "turnId": "a1", "step": 1})
	waitUntil(t, 10*time.Second, "the answer ends the fold, which says how long it took", func() bool {
		return p.is(`(f => !f.classList.contains('running') && /^Thought for \d+s$/.test(f.querySelector(':scope > details > summary .lbl').textContent) && !f.querySelector(':scope > details > summary').hasAttribute('aria-label'))(document.querySelector('#chat .steps'))`) &&
			p.is(`!!document.querySelector('#chat .flow > .it.text')`)
	}, flow)

	// ---------- a pending ask stays out of the next fold, which doesn't count on while it waits; then it joins it ----------
	ev(map[string]any{"kind": "tool.start", "tool": "Bash", "summary": "npm ci", "toolUseId": "tu2"})
	b.post(t, alice.Token, "/api/ask", map[string]any{"session": "s-a", "kind": "permission", "tool": "Bash", "input": map[string]any{"command": "npm ci"}})
	waitUntil(t, 10*time.Second, "the pending ask is a line of its own, after the second fold, which waits with it", func() bool {
		return p.is(`document.querySelectorAll('#chat .steps').length === 2 && !!document.querySelector('#chat .flow > .it.ask.pending') && document.querySelectorAll('#chat #asks .card').length === 1`) &&
			p.is(`(f => !f.classList.contains('running') && /^Thought for /.test(f.querySelector('.lbl').textContent))(document.querySelectorAll('#chat .steps')[1])`)
	}, flow)
	asks := b.Hub.Snapshot().Asks
	if len(asks) != 1 {
		t.Fatalf("one ask should wait: %+v", asks)
	}
	p.fetch("POST", "/api/answer/"+asks[0].ID, `{"decision":"allow"}`)
	ev(map[string]any{"kind": "tool.end", "tool": "Bash", "summary": "npm ci", "toolUseId": "tu2", "durationMs": 4000, "isError": true})
	waitUntil(t, 10*time.Second, "settled, the ask joins the steps; the call failed, and the folded line says so", func() bool {
		return p.is(`!document.querySelector('#chat .flow > .it.ask') && document.querySelectorAll('#chat .steps').length === 2 && document.querySelectorAll('#chat .steps')[1].querySelectorAll('.list > .it').length === 2`) &&
			p.is(`(b => !!b && !b.hidden && b.textContent === 'failed')(document.querySelectorAll('#chat .steps')[1].querySelector('summary .bad'))`)
	}, flow)

	// ---------- the turn's end finishes the last fold; a click unfolds it ----------
	ev(map[string]any{"kind": "turn.complete", "reason": "end_turn"})
	waitUntil(t, 10*time.Second, "the last fold is done", func() bool {
		return p.is(`!document.querySelector('#chat .steps.running') && /^Thought for /.test(document.querySelectorAll('#chat .steps')[1].querySelector(':scope > details > summary .lbl').textContent)`)
	}, flow)
	p.run(`document.querySelectorAll('#chat .steps > details > summary')[1].click()`)
	waitUntil(t, 5*time.Second, "unfolded, its steps show", func() bool {
		return p.is(`document.querySelectorAll('#chat .steps > details')[1].open && document.querySelectorAll('#chat .steps')[1].querySelector('.it.tool').checkVisibility()`)
	})
	p.shot("unfolded")

	// ---------- the settings unfold them all, new ones too, after a reload as well ----------
	p.run(`document.getElementById('open-settings').click()`)
	waitUntil(t, 5*time.Second, "the settings open", func() bool { return p.is(`!document.getElementById('settings').hidden`) })
	p.run(`document.getElementById('set-steps').click()`)
	waitUntil(t, 5*time.Second, "every fold unfolds", func() bool {
		return p.is(`[...document.querySelectorAll('#chat .steps > details')].every(d => d.open)`)
	})
	p.run(`document.querySelector('#settings .close').click()`)
	do(t, ctx, 20*time.Second, chromedp.Reload())
	open()
	waitUntil(t, 10*time.Second, "after a reload the folds start unfolded", func() bool {
		return p.is(`document.querySelectorAll('#chat .steps').length === 2 && [...document.querySelectorAll('#chat .steps > details')].every(d => d.open)`)
	}, flow)
	p.run(`document.getElementById('open-settings').click(); document.getElementById('set-steps').click(); document.querySelector('#settings .close').click()`)
	waitUntil(t, 5*time.Second, "unticked, they fold again", func() bool {
		return p.is(`[...document.querySelectorAll('#chat .steps > details')].every(d => !d.open)`)
	})
	p.done()

	// ---------- Coworkers: a pending ask's card stays out of the folds; paused, the count goes by the minute ----------
	mctx, _ := clockTab(t, browser, b.URL+"/t/messenger/?lang=en", 0)
	do(t, mctx, 10*time.Second, chromedp.EmulateViewport(1400, 900))
	m := &screens{t: t, ctx: mctx, lang: "steps-coworkers"}
	waitUntil(t, 20*time.Second, "Coworkers lists alice", func() bool { return m.is(`document.querySelectorAll('#people .row').length === 1`) })
	m.run(`document.querySelector('#people .row').click()`)
	waitUntil(t, 10*time.Second, "her conversation shows the folds", func() bool {
		return m.is(`!document.getElementById('chat').hidden && document.querySelectorAll('#chat .steps').length === 2`)
	})
	ev(map[string]any{"kind": "chat.user", "text": "Now the tests", "origin": "composer"})
	ev(map[string]any{"kind": "turn.start", "text": "Now the tests"})
	ev(map[string]any{"kind": "tool.start", "tool": "Bash", "summary": "npm test", "toolUseId": "tu3"})
	b.post(t, alice.Token, "/api/ask", map[string]any{"session": "s-a", "kind": "question", "questions": []map[string]any{{"question": "Run the slow tests too?", "header": "Tests",
		"options": []map[string]any{{"label": "Yes", "description": "all"}, {"label": "No", "description": "fast only"}}}}})
	waitUntil(t, 10*time.Second, "the card sits in its line, outside the folds", func() bool {
		return m.is(`!!document.querySelector('#chat .flow > .it.ask.pending.carded .card') && !document.querySelector('#chat .steps .card')`)
	}, func() string { return m.js(`document.querySelector('#chat .flow').innerText`) })
	m.shot("card")
	asks = b.Hub.Snapshot().Asks
	if len(asks) != 1 {
		t.Fatalf("one ask should wait: %+v", asks)
	}
	m.fetch("POST", "/api/answer/"+asks[0].ID, `{"answers":{"Run the slow tests too?":"No"}}`)
	m.run(`localStorage.setItem('symphony.still', '1')`)
	do(t, mctx, 20*time.Second, chromedp.Reload())
	waitUntil(t, 20*time.Second, "Coworkers lists alice again", func() bool { return m.is(`document.querySelectorAll('#people .row').length === 1`) })
	m.run(`document.querySelector('#people .row').click()`)
	lbl := `document.querySelector('#chat .steps.running .lbl').textContent`
	waitUntil(t, 10*time.Second, "the running fold shows, saying no time in its first minute", func() bool {
		return m.is(`!!document.querySelector('#chat .steps.running') && ` + lbl + ` === 'Thinking…'`)
	}, func() string { return m.js(`document.querySelector('#chat .flow').innerText`) })
	m.run(`window.__warp = 150000`)
	waitUntil(t, 5*time.Second, "with motion paused, it counts whole minutes", func() bool {
		return m.is(`/^Thinking… [23]m$/.test(` + lbl + `)`)
	}, func() string { return m.js(lbl) })
	m.run(`window.__warp = 0; localStorage.removeItem('symphony.still')`)
	m.done()
}
