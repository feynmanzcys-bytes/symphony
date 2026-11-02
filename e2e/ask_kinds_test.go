//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// askScript is the fake claude of TestAskKinds: it says whether the bridge asked for the office's context, then echoes
// what is typed, as a session at its prompt.
const askScript = `#!/bin/sh
printf 'context=%s\n' "$SYMPHONY_CONTEXT"
while IFS= read -r line; do printf 'got: %s\n' "$line"; done
`

// TestAskKinds walks through what v1.2.0 adds to the asks, without tokens (claude is a script, and the test reports in
// as the mod would). The launch dialog's "Tell it who else is in the office": the preview and the session's environment
// say SYMPHONY_CONTEXT=1, and the next start has it ticked. A permission's "Allow for this session" button, named after
// the rule the mod proposed (a command's prefix, every edit), held back with Allow while the input is cut; it answers
// with remember, the chat says so, and the mod's report of the rule is a notice. An MCP server's form as a card that
// lists the fields and can't be answered on the page: its button shows the session's terminal, where the card stays out
// of the way while a permission's card still rises over it, and the mod's settle takes it away; a link only for
// http(s). In Coworkers the cards sit in the flow under their lines and the input stays usable (a message goes while
// one waits); with the terminal shown they rise over it and go back to their lines after, what was typed in them kept.
// Screenshots into e2e/artifacts/screens/asks-*. It fails on a page error or a key a catalog lacks.
func TestAskKinds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake claude is a shell script")
	}
	requireBrowser(t)
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))
	t.Cleanup(b.Hub.Stop)
	fake := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(fake, []byte(askScript), 0o755); err != nil {
		t.Fatal(err)
	}
	b.Launcher.Claude = fake
	dir := t.TempDir()

	browser := newBrowser(t)
	office, _ := clockTab(t, browser, b.URL+"/?lang=en", 0)
	do(t, office, 10*time.Second, chromedp.EmulateViewport(1400, 900))
	p := &screens{t: t, ctx: office, lang: "asks-office"}

	// ---------- the launch dialog: the office's context ----------
	waitUntil(t, 20*time.Second, "the office offers a new employee", func() bool {
		return p.is(`!!document.getElementById('new-emp') && !document.getElementById('new-emp').hidden`)
	})
	p.run(`document.getElementById('new-emp').click()`)
	waitUntil(t, 10*time.Second, "the launch dialog opens, the context unticked", func() bool {
		return p.is(`!document.getElementById('launch').hidden && document.querySelector('#launch form').elements.context.checked === false`)
	})
	if p.js(`document.querySelector('#launch .context').title`) == "" || p.js(`document.querySelector('#launch .context').textContent`) != "Tell it who else is in the office" {
		t.Fatalf("the box should say what it does: %q / %q", p.js(`document.querySelector('#launch .context').textContent`), p.js(`document.querySelector('#launch .context').title`))
	}
	p.run(fmt.Sprintf(`(f => { f.elements.name.value = 'ann'; f.elements.dir.value = %q; f.elements.dir.dispatchEvent(new Event('input', { bubbles: true })); f.elements.context.click() })(document.querySelector('#launch form'))`, dir))
	waitUntil(t, 5*time.Second, "the preview names the context", func() bool {
		return p.is(`document.querySelector('#launch .preview').textContent.includes('SYMPHONY_CONTEXT=1 claude --name ann')`)
	}, func() string { return p.js(`document.querySelector('#launch .preview').textContent`) })
	p.shot("launch-context")
	p.check()
	p.run(`document.querySelector('#launch form').requestSubmit()`)
	var launchID string
	waitUntil(t, 20*time.Second, "ann's start is under way", func() bool {
		for _, l := range b.Hub.Snapshot().Launches {
			if l.Name == "ann" {
				launchID = l.ID
			}
		}
		return launchID != "" && b.Launcher.Terms.Live(launchID)
	})
	term := b.Launcher.Terms.Get(launchID)
	waitUntil(t, 10*time.Second, "her session got SYMPHONY_CONTEXT=1", func() bool { return strings.Contains(term.Screen(5), "context=1") },
		func() string { return term.Screen(5) })
	view := b.account("ann")
	if view == nil || view.Last == nil || !view.Last.Context {
		t.Fatalf("the bridge should keep the choice for her next start: %+v", view)
	}
	acc := b.Hub.Account(view.ID)
	b.post(t, acc.Token, "/api/event", map[string]any{"session": "s-ann", "kind": "session.start", "cwd": dir, "launch": launchID, "model": "claude-sonnet-5-5"})
	waitUntil(t, 20*time.Second, "she is online and her window opens", func() bool {
		return b.online("ann") != nil && p.is(`!document.getElementById('chat').hidden && document.querySelector('#chat h2').textContent === 'ann'`)
	})

	// ---------- Allow for this session ----------
	askOf := func(match func(kind, tool string) bool) string {
		for _, a := range b.Hub.Snapshot().Asks {
			if match(a.Kind, a.Tool) {
				return a.ID
			}
		}
		return ""
	}
	b.post(t, acc.Token, "/api/ask", map[string]any{"session": "s-ann", "kind": "permission", "tool": "Bash",
		"input": map[string]any{"command": "npm test -- --watch=false"}, "remember": map[string]any{"tool": "Bash", "prefix": "npm test"}})
	card := `#chat #asks .card[data-kind=permission]`
	waitUntil(t, 10*time.Second, "the card offers to allow npm test for the session", func() bool {
		return p.is(fmt.Sprintf(`(b => !!b && b.textContent === %q && b.title.includes('until the session ends'))(document.querySelector('%s .allow-session'))`, "Allow “npm test” this session", card))
	}, func() string { return p.js(`document.querySelector('#chat #asks').innerHTML.slice(0, 1500)`) })
	p.shot("allow-session")
	p.check()
	p.run(fmt.Sprintf(`document.querySelector('%s .allow-session').click()`, card))
	waitUntil(t, 10*time.Second, "the chat says it was allowed for the session", func() bool {
		return len(b.Hub.Snapshot().Asks) == 0 && p.is(`[...document.querySelectorAll('#chat .flow .it.ask')].some(n => n.classList.contains('allowSession') && n.textContent.includes('allowed for the session'))`)
	})
	// the mod keeps it and says so (once, whatever it reports again)
	for range 2 {
		b.post(t, acc.Token, "/api/event", map[string]any{"session": "s-ann", "kind": "session.rule", "rule": map[string]any{"tool": "Bash", "prefix": "npm test"}})
	}
	waitUntil(t, 10*time.Second, "the rule's notice shows once", func() bool {
		return p.is(`[...document.querySelectorAll('#chat .flow .it.notice')].filter(n => n.textContent === 'Allowed for the rest of the session: Bash npm test').length === 1`)
	}, func() string {
		return p.js(`[...document.querySelectorAll('#chat .flow .it.notice')].map(n => n.textContent).join(' | ')`)
	})
	// every edit, as one rule; with a cut input, the session's button waits for Show all as Allow does
	b.post(t, acc.Token, "/api/ask", map[string]any{"session": "s-ann", "kind": "permission", "tool": "Write",
		"input": map[string]any{"file_path": dir + "/big.txt", "content": strings.Repeat("filler ", 1200)}, "remember": map[string]any{"tool": "Edit"}})
	waitUntil(t, 10*time.Second, "the cut input's card holds both allows back", func() bool {
		return p.is(fmt.Sprintf(`(c => !!c && c.querySelector('.allow-session').textContent === 'Allow edits this session' && c.querySelector('.allow').disabled && c.querySelector('.allow-session').disabled)(document.querySelector('%s'))`, card))
	})
	p.run(fmt.Sprintf(`document.querySelector('%s .show-all').click()`, card))
	waitUntil(t, 10*time.Second, "Show all lets both go", func() bool {
		return p.is(fmt.Sprintf(`(c => !c.querySelector('.allow').disabled && !c.querySelector('.allow-session').disabled)(document.querySelector('%s'))`, card))
	})
	p.run(fmt.Sprintf(`document.querySelector('%s .deny').click()`, card))
	waitUntil(t, 10*time.Second, "denied", func() bool { return len(b.Hub.Snapshot().Asks) == 0 })

	// ---------- an MCP server's form ----------
	b.post(t, acc.Token, "/api/ask", map[string]any{"session": "s-ann", "kind": "elicitation", "server": "github", "message": "Which repository should I open?",
		"fields": []map[string]any{
			{"name": "repo", "title": "Repository", "type": "string", "required": true, "description": "owner/name"},
			{"name": "private", "type": "boolean"},
			{"name": "visibility", "type": "string", "enum": []string{"public", "internal"}},
		}})
	form := `#chat #asks .card[data-kind=elicitation]`
	waitUntil(t, 10*time.Second, "the form's card lists the fields, with the terminal's button", func() bool {
		return p.is(fmt.Sprintf(`(c => !!c && c.querySelector('.tool').textContent === 'github asks' && c.querySelector('.msg').textContent === 'Which repository should I open?' &&
			c.querySelectorAll('.fields li').length === 3 && c.querySelectorAll('.fields .req').length === 1 && c.querySelector('.fields').textContent.includes('public / internal') &&
			!c.querySelector('.allow, .deny, .submit') && !c.querySelector('.to-term').parentElement.hidden && c.querySelector('.to-term').textContent === 'Answer in the terminal')(document.querySelector('%s'))`, form))
	}, func() string { return p.js(`document.querySelector('#chat #asks').innerHTML.slice(0, 1500)`) })
	if !p.is(fmt.Sprintf(`document.querySelector('%s .who').textContent.includes('MCP form') && document.querySelector('#chat .who .state').textContent === 'Needs an MCP form filled in'`, form)) {
		t.Fatalf("the card should say it's an MCP form, the window what she waits for: %q / %q", p.js(fmt.Sprintf(`document.querySelector('%s .who').textContent`, form)), p.js(`document.querySelector('#chat .who .state').textContent`))
	}
	if !p.is(`[...document.querySelectorAll('#chat .flow .it.ask.pending')].some(n => n.textContent.startsWith('MCP form') && n.textContent.includes('github'))`) {
		t.Fatalf("the chat should have the form's line: %s", p.js(`[...document.querySelectorAll('#chat .flow .it.ask')].map(n => n.textContent).join(' | ')`))
	}
	p.shot("mcp-form")
	p.check()
	formID := askOf(func(kind, _ string) bool { return kind == "elicitation" })
	p.run(fmt.Sprintf(`document.querySelector('%s .to-term').click()`, form))
	waitUntil(t, 10*time.Second, "the button shows the terminal, the form's card out of its way", func() bool {
		return p.is(`document.querySelector('#chat .panel').classList.contains('term-on') && getComputedStyle(document.querySelector('#chat .dock')).display === 'none' && document.querySelector('#chat .term-screen').innerText.includes('context=1')`)
	})
	// a permission still rises over the terminal, without the form
	b.post(t, acc.Token, "/api/ask", map[string]any{"session": "s-ann", "kind": "permission", "tool": "Bash", "input": map[string]any{"command": "ls -la"}})
	waitUntil(t, 10*time.Second, "the permission rises over the terminal, alone", func() bool {
		return p.is(fmt.Sprintf(`(c => !!c && document.querySelector('#chat .dock').classList.contains('asking') && getComputedStyle(document.querySelector('%s')).display === 'none' &&
			(r => c.contains(document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2)))(c.getBoundingClientRect()))(document.querySelector('%s'))`, form, card))
	})
	p.run(fmt.Sprintf(`document.querySelector('%s .allow').click()`, card))
	waitUntil(t, 10*time.Second, "the permission is answered, the dock gone from the terminal again", func() bool {
		return askOf(func(kind, _ string) bool { return kind == "permission" }) == "" && p.is(`getComputedStyle(document.querySelector('#chat .dock')).display === 'none'`)
	})
	// the terminal's answer: the mod settles the card
	b.post(t, acc.Token, "/api/settle/"+formID, map[string]any{"action": "accept"})
	p.run(`document.querySelector('#chat .termview').click()`)
	waitUntil(t, 10*time.Second, "back in the chat, the form's line says it was submitted", func() bool {
		return p.is(`!document.querySelector('#chat .panel').classList.contains('term-on') && !document.querySelector('#chat .dock').classList.contains('asking') &&
			[...document.querySelectorAll('#chat .flow .it.ask')].some(n => n.textContent.startsWith('MCP form') && n.textContent.includes('submitted'))`)
	})
	// a form that sends the person to a page: a link for http(s) only
	b.post(t, acc.Token, "/api/ask", map[string]any{"session": "s-ann", "kind": "elicitation", "server": "auth", "message": "Sign in", "mode": "url", "url": "https://example.com/sign-in"})
	b.post(t, acc.Token, "/api/ask", map[string]any{"session": "s-ann", "kind": "elicitation", "server": "evil", "message": "Click", "mode": "url", "url": "javascript:alert(1)"})
	waitUntil(t, 10*time.Second, "two link forms", func() bool {
		return p.is(`document.querySelectorAll('#chat #asks .card[data-kind=elicitation]').length === 2`)
	})
	if !p.is(`(l => l.length === 1 && l[0].href === 'https://example.com/sign-in' && l[0].target === '_blank' && l[0].rel.includes('noopener'))(document.querySelectorAll('#chat #asks a'))`) {
		t.Fatalf("only the https address should be a link: %s", p.js(`document.querySelector('#chat #asks').innerHTML`))
	}
	for _, a := range b.Hub.Snapshot().Asks {
		b.post(t, acc.Token, "/api/settle/"+a.ID, map[string]any{"action": "cancel"})
	}
	waitUntil(t, 10*time.Second, "the link forms are settled", func() bool {
		return p.is(`!document.querySelector('#chat .dock').classList.contains('asking') && [...document.querySelectorAll('#chat .flow .it.ask')].filter(n => n.textContent.includes('dismissed')).length === 2`)
	})
	p.done()

	// ---------- Coworkers: the cards in the flow ----------
	cw, _ := clockTab(t, browser, b.URL+"/t/messenger/?lang=en", 0)
	do(t, cw, 10*time.Second, chromedp.EmulateViewport(1400, 900))
	q := &screens{t: t, ctx: cw, lang: "asks-coworkers"}
	waitUntil(t, 20*time.Second, "Coworkers lists ann", func() bool { return q.is(`document.querySelectorAll('#people .row').length === 1`) })
	q.run(`document.querySelector('#people .row').click()`)
	waitUntil(t, 10*time.Second, "her conversation shows", func() bool {
		return q.is(`!document.getElementById('chat').hidden && document.querySelectorAll('#chat .flow .it').length > 5`)
	})
	b.post(t, acc.Token, "/api/ask", map[string]any{"session": "s-ann", "kind": "permission", "tool": "Bash",
		"input": map[string]any{"command": "go vet ./..."}, "remember": map[string]any{"tool": "Bash", "prefix": "go vet"}})
	askID := askOf(func(kind, _ string) bool { return kind == "permission" })
	inFlow := fmt.Sprintf(`#chat .flow .it.ask.carded[data-ask="%s"] > .card[data-id="%s"]`, askID, askID)
	waitUntil(t, 10*time.Second, "the card sits in the flow under its line", func() bool {
		return q.is(fmt.Sprintf(`!!document.querySelector('%s') && document.querySelector('%s .allow-session').textContent === 'Allow “go vet” this session'`, inFlow, inFlow))
	}, func() string { return q.js(`document.querySelector('#chat .flow').innerHTML.slice(-1500)`) })
	if !q.is(`!document.querySelector('#chat .dock').classList.contains('asking') && getComputedStyle(document.querySelector('#chat .say')).display !== 'none' && !document.querySelector('#chat .say textarea').disabled`) {
		t.Fatal("the input should stay usable while the card waits in the flow")
	}
	// a message that doesn't answer it goes meanwhile
	q.run(`(a => { a.value = 'while you wait: which branch?'; a.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true })) })(document.querySelector('#chat .say textarea'))`)
	waitUntil(t, 10*time.Second, "the message is queued for her, the card still waiting", func() bool {
		return q.is(`[...document.querySelectorAll('#chat .flow .it.user')].some(n => n.textContent.includes('while you wait: which branch?'))`) && b.pending(askID)
	})
	q.run(fmt.Sprintf(`document.querySelector('%s .reason').value = 'kept across moves'`, inFlow))
	q.shot("inline")
	q.check()
	// the terminal: the card rises over it, and goes back to its line with the chat, what was typed in it kept
	q.run(`document.querySelector('#chat .termview').click()`)
	waitUntil(t, 10*time.Second, "the card rises over the terminal", func() bool {
		return q.is(fmt.Sprintf(`(c => !!c && document.querySelector('#chat .panel').classList.contains('term-on') && document.querySelector('#chat .dock').classList.contains('asking') &&
			(r => c.contains(document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2)))(c.getBoundingClientRect()))(document.querySelector('#chat #asks .card[data-id="%s"]'))`, askID))
	})
	q.shot("inline-terminal")
	q.run(`document.querySelector('#chat .termview').click()`)
	waitUntil(t, 10*time.Second, "back in its line, the reason kept", func() bool {
		return q.is(fmt.Sprintf(`!document.querySelector('#chat .dock').classList.contains('asking') && document.querySelector('%s .reason')?.value === 'kept across moves'`, inFlow))
	})
	q.run(fmt.Sprintf(`document.querySelector('%s .allow').click()`, inFlow))
	waitUntil(t, 10*time.Second, "answered: the card leaves its line", func() bool {
		return !b.pending(askID) && q.is(fmt.Sprintf(`!document.querySelector('#chat .flow .card') && !document.querySelector('#chat .flow .it.ask[data-ask="%s"]').classList.contains('carded')`, askID))
	})
	q.done()

	// ---------- her next start has the box ticked ----------
	b.post(t, acc.Token, "/api/event", map[string]any{"session": "s-ann", "kind": "session.end", "reason": "prompt_input_exit"})
	waitUntil(t, 10*time.Second, "off duty, her window offers Start work", func() bool {
		return p.is(`!!document.querySelector('#chat .btns .go-work')`)
	}, func() string { return p.js(`document.querySelector('#chat .btns').innerHTML`) })
	p.run(`document.querySelector('#chat .btns .go-work').click()`)
	waitUntil(t, 10*time.Second, "the dialog opens with the context ticked", func() bool {
		return p.is(`!document.getElementById('launch').hidden && document.querySelector('#launch form').elements.context.checked && document.querySelector('#launch .preview').textContent.includes('SYMPHONY_CONTEXT=1')`)
	})
	p.run(`document.querySelector('#launch .cancel').click()`)
	p.done()
}
