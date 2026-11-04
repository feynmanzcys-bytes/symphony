//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// TestMarkdown checks the pages' Markdown (web/ui/markdown.js) in the browser (no tokens): every block and mark it knows,
// what it does with a text still streaming, and that nothing it's given gets out of hand (raw HTML shows as text, a link
// leads only to http, https or mailto, a quote in an address can't open an attribute). Then, in alice's conversation in
// Coworkers, her answer is drawn with its headings, list and table and her prompt as typed, and a message from another
// session with it too.
func TestMarkdown(t *testing.T) {
	requireBrowser(t)
	b := startBridge(t, "")
	alice, err := b.Hub.CreateAccount("alice")
	if err != nil {
		t.Fatal(err)
	}
	browser := newBrowser(t)
	ctx, _ := clockTab(t, browser, b.URL+"/t/messenger/?lang=en", 0)
	do(t, ctx, 10*time.Second, chromedp.EmulateViewport(1400, 900))
	p := &screens{t: t, ctx: ctx, lang: "markdown"}
	waitUntil(t, 20*time.Second, "Coworkers lists alice", func() bool { return p.is(`document.querySelectorAll('#people .row').length === 1`) })

	link := func(href, label, cls string) string {
		return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer"%s>%s</a>`, href, cls, label)
	}
	sp := func(cls, text string) string { return fmt.Sprintf(`<span class="hl-%s">%s</span>`, cls, text) }
	cases := []struct{ in, want string }{
		{"# Title", `<h3>Title</h3>`},
		{"## Sub\ntext", `<h4>Sub</h4><p>text</p>`},
		{"### Title ###", `<h5>Title</h5>`},
		{"#hashtag", `<p>#hashtag</p>`},
		{"a\nb\n\nc", `<p>a<br>b</p><p>c</p>`},
		{"- a\n- b\n  - c\n- d", `<ul><li>a</li><li>b<ul><li>c</li></ul></li><li>d</li></ul>`},
		{"1. one\n2. two", `<ol><li>one</li><li>two</li></ol>`},
		{"3. three\n4. four", `<ol start="3"><li>three</li><li>four</li></ol>`},
		{"1. step\n   - detail\n2. next", `<ol><li>step<ul><li>detail</li></ul></li><li>next</li></ol>`},
		{"- one\n\n- two", `<ul><li><p>one</p></li><li><p>two</p></li></ul>`},
		{"- [ ] todo\n- [x] done", `<ul><li class="task"><input type="checkbox" disabled> todo</li><li class="task done"><input type="checkbox" disabled checked> done</li></ul>`},
		{"| a | b |\n|:-|-:|\n| 1 | 2 |", `<div class="table"><table><thead><tr><th style="text-align:left">a</th><th style="text-align:right">b</th></tr></thead>` +
			`<tbody><tr><td style="text-align:left">1</td><td style="text-align:right">2</td></tr></tbody></table></div>`},
		{"| x \\| y | z |\n|---|---|\n| `a|b` | **c** |", `<div class="table"><table><thead><tr><th>x | y</th><th>z</th></tr></thead><tbody><tr><td><code>a|b</code></td><td><strong>c</strong></td></tr></tbody></table></div>`},
		// fenced code, colored by its language (ui/highlight.js): a span per token, the code escaped, operators plain
		{"```js\nconst a = '<b>'\n```", `<pre data-lang="js"><code>` + sp("k", "const") + ` a = ` + sp("s", "'&lt;b&gt;'") + `</code></pre>`},
		{"```go\nfunc main() {\n\tfmt.Println(\"hi\", 42) // say\n}\n```", `<pre data-lang="go"><code>` + sp("k", "func") + ` ` + sp("f", "main") + "() {\n    fmt." + sp("f", "Println") +
			`(` + sp("s", "&quot;hi&quot;") + `, ` + sp("n", "42") + `) ` + sp("c", "// say") + "\n}</code></pre>"},
		{"```sh\n$ ls -la /tmp # list\n```", `<pre data-lang="sh"><code>` + sp("p", "$ ") + sp("f", "ls") + ` ` + sp("a", "-la") + ` /tmp ` + sp("c", "# list") + `</code></pre>`},
		{"```json\n{\"a\": 1, \"b\": [true, null]}\n```", `<pre data-lang="json"><code>{` + sp("a", "&quot;a&quot;") + `: ` + sp("n", "1") + `, ` + sp("a", "&quot;b&quot;") + `: [` + sp("n", "true") + `, ` + sp("n", "null") + `]}</code></pre>`},
		{"```diff\n-old\n+new\n```", `<pre data-lang="diff"><code>` + sp("d", "-old") + "\n" + sp("i", "+new") + `</code></pre>`},
		{"```brainfuck\n+++ <\n```", `<pre data-lang="brainfuck"><code>+++ &lt;</code></pre>`}, // a language it doesn't know: no spans
		{"```\nstill streaming", `<pre><code>still streaming</code></pre>`},
		{"```py\nx = 'still", `<pre data-lang="py"><code>x = ` + sp("s", "'still") + `</code></pre>`}, // a string cut by the stream runs to the end
		// an entity is its character once (the model writes &amp; meaning &); what it decodes to is escaped like anything
		{"a &amp; b &lt;b&gt; &#65;&#x42; &bogus; `&amp;`", `<p>a &amp; b &lt;b&gt; AB &amp;bogus; <code>&amp;amp;</code></p>`},
		{"&lt;script&gt;alert(1)&lt;/script&gt;", `<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>`},
		{"a `<code>` **bold** *it* ~~gone~~ snake_case_name _em_", `<p>a <code>&lt;code&gt;</code> <strong>bold</strong> <em>it</em> <del>gone</del> snake_case_name <em>em</em></p>`},
		{"2 * 3 * 4", `<p>2 * 3 * 4</p>`},
		{"**`x`**", `<p><strong><code>x</code></strong></p>`},
		{"\\*not italic\\*", `<p>*not italic*</p>`},
		{"[site](https://example.com/a?b=1&c=2) and [bad](javascript:alert(1)) and [rel](src/x.js)",
			`<p>` + link("https://example.com/a?b=1&amp;c=2", "site", "") + ` and bad and rel</p>`},
		{"see https://example.com/path.", `<p>see ` + link("https://example.com/path", "https://example.com/path", "") + `.</p>`},
		{"<https://example.com> or mailto: [me](mailto:a@b.c)", `<p>` + link("https://example.com", "https://example.com", "") + ` or mailto: ` + link("mailto:a@b.c", "me", "") + `</p>`},
		{"![chart](https://example.com/c.png)", `<p>` + link("https://example.com/c.png", "chart", ` class="img"`) + `</p>`},
		{"<script>alert(1)</script> <img src=x onerror=alert(1)>", `<p>&lt;script&gt;alert(1)&lt;/script&gt; &lt;img src=x onerror=alert(1)&gt;</p>`},
		{`[x](https://a.com/"onmouseover="alert(1))`, `<p>` + link("https://a.com/&quot;onmouseover=&quot;alert(1)", "x", "") + `</p>`},
		{"> quoted\n> more", `<blockquote><p>quoted<br>more</p></blockquote>`},
		{"a\n\n---\n\nb", `<p>a</p><hr><p>b</p>`},
		{"text\n- item", `<p>text</p><ul><li>item</li></ul>`},
	}
	var ins []string
	for _, c := range cases {
		ins = append(ins, c.in)
	}
	raw, _ := json.Marshal(ins)
	var got []string
	out := get(t, ctx, 10*time.Second, chromedp.Evaluate[string](`import('/ui/markdown.js').then(m => JSON.stringify(`+string(raw)+`.map(m.markdown)))`, chromedp.EvalAwaitPromise))
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for i, c := range cases {
		if got[i] != c.want {
			t.Errorf("markdown(%q)\n got %s\nwant %s", c.in, got[i], c.want)
		}
	}
	// what a page does with what it renders: no script ran, no element came from the text but the ones it makes
	if p.is(`(() => { const d = document.createElement('div'); let ran = false; window.__mdRan = () => { ran = true }
		return import('/ui/markdown.js').then(m => { d.innerHTML = m.markdown('<img src=x onerror="__mdRan()"> [a](javascript:__mdRan()) <svg onload=__mdRan()>'); return ran || !!d.querySelector('img, svg, [onerror], [onload]') || [...d.querySelectorAll('a')].length > 0 }) })()`) {
		t.Error("Markdown let markup or a script through")
	}

	// ---------- in the pages ----------
	ev := func(body map[string]any) {
		t.Helper()
		body["session"] = "s-a"
		b.post(t, alice.Token, "/api/event", body)
	}
	ev(map[string]any{"kind": "session.start", "cwd": "/work/app", "model": "claude-opus-5-5"})
	ev(map[string]any{"kind": "chat.user", "text": "Plan it, **briefly**", "origin": "composer"})
	answer := "## Plan\n\n1. Read the **log**\n2. Fix `build.sh`\n   - then the tests\n\n| step | time |\n|---|--:|\n| build | 2m |\n\nSee https://example.com/docs."
	ev(map[string]any{"kind": "chat.delta", "turnId": "a1", "step": 0, "blocks": []map[string]any{{"i": 0, "kind": "text", "text": answer}}})
	ev(map[string]any{"kind": "chat.stop", "turnId": "a1", "step": 0})
	clickRow(t, ctx, alice.ID)
	waitUntil(t, 10*time.Second, "her answer is drawn as Markdown", func() bool {
		return p.is(`(b => !!b && !!b.querySelector('h4') && b.querySelectorAll('ol > li').length === 2 && !!b.querySelector('ol li ul li') && b.querySelectorAll('table td').length === 2 && b.querySelector('table td + td').style.textAlign === 'right' && b.querySelector('a').target === '_blank')(document.querySelector('#chat .it.text .body.md'))`)
	}, func() string { return p.js(`document.querySelector('#chat .flow').innerHTML`) })
	if !p.is(`(b => !!b && !b.classList.contains('md') && b.innerHTML === 'Plan it, <b>briefly</b>')(document.querySelector('#chat .it.user .body'))`) {
		t.Errorf("a prompt should stay as typed: %s", p.js(`document.querySelector('#chat .it.user .body').outerHTML`))
	}
	p.shot("answer")
	// a message between sessions, drawn as Markdown in the sender's conversation (bob's, where it shows as sent)
	bob, err := b.Hub.CreateAccount("bob")
	if err != nil {
		t.Fatal(err)
	}
	b.post(t, bob.Token, "/api/event", map[string]any{"session": "s-b", "kind": "session.start", "cwd": "/work/web"})
	b.post(t, bob.Token, "/api/event", map[string]any{"session": "s-b", "kind": "message.send", "to": "alice", "text": "Done:\n- **build** fixed\n- tests green", "origin": "model", "delivered": true})
	waitUntil(t, 10*time.Second, "Coworkers lists bob", func() bool {
		return p.is(fmt.Sprintf(`!!document.querySelector('#people .row[data-account=%q]')`, bob.ID))
	})
	clickRow(t, ctx, bob.ID)
	waitUntil(t, 10*time.Second, "the message is drawn as Markdown in bob's conversation", func() bool {
		return p.is(`(m => !!m && m.querySelectorAll('ul li').length === 2 && !!m.querySelector('strong'))(document.querySelector('#chat .flow .it.message.out .body.md'))`)
	}, func() string { return p.js(`document.querySelector('#chat .flow').innerHTML`) })
	if strings.Contains(p.js(`document.querySelector('#chat .flow .it.message').innerText`), "**") {
		t.Error("the message should show no Markdown marks")
	}
	p.done()
}
