//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// termScript is the fake claude of TestTerminalView: colored text, wide characters lined up under narrow ones, a cursor
// move, then what it reads back (the terminal's size for "size", eighty lines for "many"). The terminal's own echo
// shows what is typed.
const termScript = `#!/bin/sh
printf '\033[?2004l'
printf '\033[1;31mred alert\033[0m plain \033[38;2;0;128;255mtruecolor\033[0m \033[7minverse\033[0m\n'
printf '0123456789|ascii\n'
printf '中文宽字符|wide\n'
printf 'abc\033[2Dx\n'
while IFS= read -r line; do
  case "$line" in
    size) stty size ;;
    many) i=1; while [ $i -le 80 ]; do echo "line $i"; i=$((i+1)); done ;;
    *) printf 'got: %s\n' "$line" ;;
  esac
done
`

// TestTerminalView walks through the in-page terminal (ui/terminal.js) without spending tokens: claude is a script
// (termScript), and the test reports its session in as the mod would. First the screen model is fed sequences in the
// page and checked cell by cell. Then the employee window's "Terminal here", beside "Open terminal", shows the session's
// terminal in the window: the text with its colors, the wide characters two cells each, the terminal sized to the
// window, keys typed into it reaching the script (Enter, Backspace, Ctrl+U, a paste), Esc going to the session and not
// closing the window; the button and Ctrl+] bring the chat back. Then Coworkers' computer shows the screen small, and a
// click on it shows it in the conversation. Screenshots into e2e/artifacts/terminal-*.png. It fails on a page error, a
// content security policy violation included, or a key a catalog lacks.
func TestTerminalView(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake claude is a shell script")
	}
	requireBrowser(t)
	repo, _ := filepath.Abs("..")
	b := startBridge(t, filepath.Join(repo, "mods", "symphony-bridge"))
	fake := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(fake, []byte(termScript), 0o755); err != nil {
		t.Fatal(err)
	}
	b.Launcher.Claude = fake

	// ann is started from the page (the page's call, made here) and reports in from her terminal, as the mod would
	acc, err := b.Hub.CreateAccount("ann")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	body, _ := json.Marshal(bridge.LaunchRequest{Account: acc.ID, Dir: dir})
	req := pageReq("POST", b.URL, "/api/launch", strings.NewReader(string(body)))
	req.Header.Set("content-type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var launch bridge.Launch
	err = json.NewDecoder(res.Body).Decode(&launch)
	res.Body.Close()
	if err != nil || res.StatusCode != 200 || launch.ID == "" {
		t.Fatalf("starting ann: %s %v %+v", res.Status, err, launch)
	}
	b.post(t, acc.Token, "/api/event", map[string]any{"session": "s-ann", "kind": "session.start", "cwd": dir, "launch": launch.ID, "model": "claude-opus-5-5"})
	waitUntil(t, 10*time.Second, "ann is online in her terminal", func() bool {
		s := b.online("ann")
		return s != nil && s.Launch == launch.ID && b.Launcher.Terms.Live(launch.ID)
	})
	term := b.Launcher.Terms.Get(launch.ID)
	waitUntil(t, 10*time.Second, "the script has drawn its screen", func() bool { return strings.Contains(term.Screen(10), "axc") },
		func() string { return term.Screen(10) })

	browser := newBrowser(t)
	office, _ := clockTab(t, browser, b.URL+"/?lang=en", 0) // collects the page's errors in window.__errors
	p := &screens{t: t, ctx: office, lang: "terminal-office"}

	// ---------- the screen model, in the page ----------
	if fails := get(t, office, 20*time.Second, chromedp.Evaluate[[]string](termModelChecks, chromedp.EvalAwaitPromise)); len(fails) > 0 {
		t.Fatalf("the screen model:\n%s", strings.Join(fails, "\n"))
	}

	// ---------- the employee window: the terminal in it ----------
	waitUntil(t, 20*time.Second, "ann sits at her desk", func() bool {
		return p.is(fmt.Sprintf(`!!document.querySelector('.emp-label[data-account=%q]')`, acc.ID))
	})
	clickLabel(t, office, acc.ID)
	waitUntil(t, 10*time.Second, "her window opens with Terminal here beside Open terminal", func() bool {
		return p.is(`!document.getElementById('chat').hidden && document.querySelector('#chat .btns .terminal')?.nextElementSibling?.classList.contains('termview')`)
	}, func() string { return p.js(`document.querySelector('#chat .head').innerHTML`) })
	if got := p.js(`document.querySelector('#chat .termview').textContent`); got != "Terminal here" {
		t.Fatalf("the button should read Terminal here: %q", got)
	}
	p.run(`document.querySelector('#chat .termview').click()`)
	screenText := `document.querySelector('#chat .term-screen').innerText`
	waitUntil(t, 10*time.Second, "the window shows the terminal with the script's output", func() bool {
		return p.is(`document.querySelector('#chat .panel').classList.contains('term-on') && !document.querySelector('#chat .termpane').hidden && getComputedStyle(document.querySelector('#chat .flow')).display === 'none'`) &&
			p.is(screenText+`.includes('red alert') && `+screenText+`.includes('axc') && `+screenText+`.includes('中文宽字符|wide')`)
	}, func() string { return p.js(screenText) })
	if got := p.js(`document.querySelector('#chat .termview').textContent`); got != "Back to chat" {
		t.Fatalf("the button should read Back to chat now: %q", got)
	}
	if p.js(`document.activeElement?.className`) != "term-keys" {
		t.Fatalf("the terminal should take the keys: the focus is on %s", p.js(`document.activeElement?.outerHTML.slice(0, 80)`))
	}
	// the colors: bold palette red, a true color, inverse (the default colors swapped)
	colors := p.js(`(() => {
		const scr = document.querySelector('#chat .term-screen'), span = text => [...scr.querySelectorAll('span')].find(s => s.textContent === text)
		const probe = document.createElement('span'); scr.closest('.term').appendChild(probe)
		const css = v => { probe.style.color = v; return getComputedStyle(probe).color }
		const red = span('red alert'), tc = span('truecolor'), inv = span('inverse')
		const out = { red: getComputedStyle(red).color === css('var(--term-c1)') && getComputedStyle(red).fontWeight === '700',
			truecolor: getComputedStyle(tc).color === 'rgb(0, 128, 255)',
			inverse: getComputedStyle(inv).color === css('var(--term-bg)') && getComputedStyle(inv).backgroundColor === css('var(--term-fg)') }
		probe.remove()
		return JSON.stringify(out)
	})()`)
	if colors != `{"red":true,"truecolor":true,"inverse":true}` {
		t.Fatalf("the terminal's colors: %s", colors)
	}
	// the wide characters take two cells each: the bar after five of them stands under the bar after ten digits
	align := p.js(`(() => {
		const rows = [...document.querySelectorAll('#chat .term-screen .term-row')]
		const bar = text => {
			const row = rows.find(r => r.textContent.startsWith(text)), walk = document.createTreeWalker(row, NodeFilter.SHOW_TEXT)
			for (let n; (n = walk.nextNode());) {
				const i = n.data.indexOf('|')
				if (i >= 0) { const r = document.createRange(); r.setStart(n, i); r.setEnd(n, i + 1); return r.getBoundingClientRect().left }
			}
			return NaN
		}
		return JSON.stringify([bar('0123456789|'), bar('中文')])
	})()`)
	var bars [2]float64
	if err := json.Unmarshal([]byte(align), &bars); err != nil || bars[0] < 1 || bars[1]-bars[0] > 1 || bars[0]-bars[1] > 1 {
		t.Fatalf("the bar after the wide characters should stand under the one after the digits: %s", align)
	}
	screenshot(t, office, "terminal-view.png")

	// the terminal fits the window: the script reads the size the page gave it
	size := p.js(`document.querySelector('#chat .term').dataset.size`)
	var cols, rows int
	if _, err := fmt.Sscanf(size, "%dx%d", &cols, &rows); err != nil || cols < 60 || rows < 10 || (cols == 120 && rows == 40) {
		t.Fatalf("the terminal should take the window's size, not the default 120x40: %q", size)
	}
	keys := func(k string, opts ...chromedp.KeyOption) {
		do(t, office, 10*time.Second, chromedp.KeyEvent(k, opts...))
	}
	shows := func(what, text string) {
		t.Helper()
		waitUntil(t, 10*time.Second, what, func() bool { return p.is(fmt.Sprintf(screenText+`.includes(%q)`, text)) }, func() string { return p.js(screenText) })
	}
	keys("size\r")
	shows("the script got the window's size", fmt.Sprintf("%d %d", rows, cols))
	// typing: the script reads the line; Backspace erases; Esc goes to the session and the window stays; Ctrl+U kills the line
	keys("hello\r")
	shows("the typed line reached the script", "got: hello")
	keys("abX")
	keys(kb.Backspace)
	keys("c\r")
	shows("Backspace erased the X", "got: abc")
	keys(kb.Escape)
	if !p.is(`!document.getElementById('chat').hidden && document.querySelector('#chat .panel').classList.contains('term-on')`) {
		t.Fatal("Esc should go to the session, not close the window or leave the terminal")
	}
	keys("junk")
	keys("u", chromedp.KeyModifiers(chromedp.ModifierCtrl))
	keys("after\r")
	shows("Ctrl+U killed the line with the Esc and the junk", "got: after")
	if p.is(screenText + `.includes('got: junk')`) {
		t.Fatalf("Ctrl+U should have killed the line:\n%s", p.js(screenText))
	}
	// a paste goes as typed, its line break an Enter (the script turned bracketed paste off)
	p.run(`(() => { const dt = new DataTransfer(); dt.setData('text/plain', 'pasted one\nand two'); document.querySelector('#chat .term-keys').dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true })) })()`)
	shows("the paste's first line reached the script", "got: pasted one")
	keys("\r")
	shows("the paste's second line too", "got: and two")
	// what scrolls off the screen stays above it; scrolled up there, a key brings the screen back
	keys("many\r")
	shows("the script's eighty lines", "line 80")
	scrolled := `(() => { const s = document.querySelector('#chat .term-scroll'), back = [...document.querySelectorAll('#chat .term-back .term-row')]
		return JSON.stringify({ kept: back.some(r => r.textContent === 'line 1') && back.some(r => r.textContent.startsWith('red alert')), atEnd: s.scrollTop + s.clientHeight >= s.scrollHeight - 2 }) })()`
	if got := p.js(scrolled); got != `{"kept":true,"atEnd":true}` {
		t.Fatalf("the lines that scrolled off should be above the screen, the view at its end: %s", got)
	}
	p.run(`document.querySelector('#chat .term-scroll').scrollTop = 0`)
	keys("up\r")
	shows("typed while scrolled up", "got: up")
	if got := p.js(scrolled); got != `{"kept":true,"atEnd":true}` {
		t.Fatalf("a key should bring the screen back: %s", got)
	}
	// an ask's card rises over the terminal as over the chat, and goes once answered
	b.post(t, acc.Token, "/api/ask", map[string]any{"session": "s-ann", "kind": "permission", "tool": "Bash", "input": map[string]any{"command": "ls"}})
	waitUntil(t, 10*time.Second, "the ask's card rises over the terminal", func() bool {
		return p.is(`(c => !!c && document.querySelector('#chat .panel').classList.contains('term-on') && document.querySelector('#chat .dock').classList.contains('asking') &&
			(r => c.contains(document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2)))(c.getBoundingClientRect()))(document.querySelector('#chat #asks .card'))`)
	})
	screenshot(t, office, "terminal-ask.png")
	p.run(`document.querySelector('#chat #asks .card .allow').click()`)
	waitUntil(t, 10*time.Second, "the card goes, the terminal stays", func() bool {
		return len(b.Hub.Snapshot().Asks) == 0 && p.is(`document.querySelector('#chat .panel').classList.contains('term-on') && getComputedStyle(document.querySelector('#chat .dock')).display === 'none'`)
	})
	p.run(`document.querySelector('#chat .term-keys').focus()`)
	// back to the chat with the button; again into the terminal, and out with Ctrl+]
	p.run(`document.querySelector('#chat .termview').click()`)
	waitUntil(t, 5*time.Second, "Back to chat shows the chat", func() bool {
		return p.is(`!document.querySelector('#chat .panel').classList.contains('term-on') && document.querySelector('#chat .termpane').hidden && getComputedStyle(document.querySelector('#chat .flow')).display !== 'none' && document.querySelector('#chat .termview').textContent === 'Terminal here'`)
	})
	p.run(`document.querySelector('#chat .termview').click()`)
	waitUntil(t, 5*time.Second, "the terminal again", func() bool { return p.is(`document.querySelector('#chat .panel').classList.contains('term-on')`) })
	shows("its screen again", "got: up")
	keys("]", chromedp.KeyModifiers(chromedp.ModifierCtrl))
	waitUntil(t, 5*time.Second, "Ctrl+] shows the chat", func() bool {
		return p.is(`!document.querySelector('#chat .panel').classList.contains('term-on') && !document.getElementById('chat').hidden`)
	})
	p.check()
	p.done()

	// ---------- Coworkers: the computer's screen ----------
	cw, _ := clockTab(t, browser, b.URL+"/t/messenger/?lang=en", 0)
	do(t, cw, 10*time.Second, chromedp.EmulateViewport(1400, 860))
	q := &screens{t: t, ctx: cw, lang: "terminal-messenger"}
	waitUntil(t, 20*time.Second, "Coworkers lists ann", func() bool { return q.is(`document.querySelectorAll('#people .row').length === 1`) })
	q.run(`document.querySelector('#people .row').click()`)
	peek := `document.querySelector('#profile .computer .term')`
	waitUntil(t, 10*time.Second, "her computer shows the terminal, small", func() bool {
		return q.is(peek + ` && ` + peek + `.querySelector('.term-screen').innerText.includes('line 80') && ` + peek + `.querySelector('.term-screen').innerText.includes('got: up')`)
	}, func() string { return q.js(`document.getElementById('profile').innerHTML.slice(0, 2000)`) })
	if !q.is(peek + `.classList.contains('peek') && !` + peek + `.querySelector('textarea') && /scale\(/.test(` + peek + `.querySelector('.term-screen').style.transform) && ` + peek + `.getBoundingClientRect().width <= document.getElementById('profile').getBoundingClientRect().width`) {
		t.Fatalf("the computer's screen should be watched only, scaled into the profile: %s", q.js(peek+`.outerHTML.slice(0, 600)`))
	}
	if !q.is(`!document.querySelector('#chat .panel').classList.contains('term-on')`) {
		t.Fatal("the conversation should show the chat until the screen is clicked")
	}
	screenshot(t, cw, "terminal-preview.png")
	// the screen holds a stream only while it shows: a window too narrow for the profile lets it go
	waitUntil(t, 10*time.Second, "one window watches the terminal: the computer's screen", func() bool { return term.Clients() == 1 })
	do(t, cw, 10*time.Second, chromedp.EmulateViewport(1000, 860))
	waitUntil(t, 10*time.Second, "a narrow window hides the profile, and the screen lets its stream go", func() bool { return term.Clients() == 0 }, func() string {
		return fmt.Sprintf("clients %d; %s", term.Clients(), q.js(`JSON.stringify({ profile: getComputedStyle(document.getElementById('profile')).display, rects: `+peek+`?.getClientRects().length, w: innerWidth, errors: window.__errors })`))
	})
	do(t, cw, 10*time.Second, chromedp.EmulateViewport(1400, 860))
	waitUntil(t, 10*time.Second, "shown again, it takes the stream again", func() bool {
		return term.Clients() == 1 && q.is(peek+`.querySelector('.term-screen').innerText.includes('got: up')`)
	})
	q.run(peek + `.click()`)
	// (the history it came from holds the start: above the screen, the first lines)
	waitUntil(t, 10*time.Second, "a click on it shows the terminal in the conversation", func() bool {
		return q.is(`document.querySelector('#chat .panel').classList.contains('term-on') && document.querySelector('#chat .term-screen').innerText.includes('got: up') && document.querySelector('#chat .term-back').innerText.includes('red alert')`)
	}, func() string { return q.js(`document.querySelector('#chat .term').innerText`) })
	// the terminal now has the conversation's size, and the computer's screen, watching, follows it
	waitUntil(t, 10*time.Second, "the computer's screen follows the size the conversation gave the terminal", func() bool {
		return q.is(fmt.Sprintf(`(a => a !== %q && a === %s.dataset.size)(document.querySelector('#chat .term').dataset.size)`, size, peek)) &&
			q.is(`(p => Math.abs(p.querySelector('.term-scroll').getBoundingClientRect().height - p.querySelector('.term-screen').getBoundingClientRect().height) < 2)(`+peek+`)`)
	}, func() string {
		return q.js(`JSON.stringify({ chat: document.querySelector('#chat .term').dataset.size, peek: ` + peek + `.dataset.size, scroll: ` + peek + `.querySelector('.term-scroll').getBoundingClientRect().height, screen: ` + peek + `.querySelector('.term-screen').getBoundingClientRect().height, transform: ` + peek + `.querySelector('.term-screen').style.transform })`)
	})
	time.Sleep(300 * time.Millisecond) // the asks' card rises in 0.2 s
	screenshot(t, cw, "terminal-conversation.png")
	q.check()
	q.done()
}

// termModelChecks feeds the screen model (Screen in ui/terminal.js) output and checks what it holds: printing and
// wrapping, scrolling and what scrolled off, the cursor moves, erasing, inserting and deleting characters and lines in
// a scroll region, saving the cursor, the alternate screen, wide and combining characters, the pens (16, 256, true
// colors, the colon form), the modes, sequences cut between two pieces, the size the bridge reports.
const termModelChecks = `(async () => {
	const { Screen } = await import('/ui/terminal.js')
	const fails = []
	const eq = (name, got, want) => { if (JSON.stringify(got) !== JSON.stringify(want)) fails.push(name + ': got ' + JSON.stringify(got) + ', want ' + JSON.stringify(want)) }
	const run = (cols, rows, ...pieces) => { const s = new Screen(cols, rows); for (const p of pieces) s.write(p); return s }
	const rows = s => s.lines.map(l => l.text)
	let s
	s = run(5, 3, 'abcdefg'); eq('wrap', rows(s), ['abcde', 'fg', ''])
	s = run(5, 2, 'abcde\rX'); eq('a carriage return at the pending wrap stays on the line', rows(s), ['Xbcde', ''])
	s = run(4, 2, 'a\r\nb\r\nc'); eq('scrolling', rows(s), ['b', 'c']); eq('what scrolled off', s.back.map(l => l.text), ['a'])
	s = run(6, 3, '\x1b[2;3Hx'); eq('a cursor move', rows(s), ['', '  x', ''])
	s = run(8, 1, 'hello\x1b[3D\x1b[K'); eq('erase to the end of the line', rows(s), ['he'])
	s = run(8, 2, 'abc\r\ndef\x1b[2D\x1b[1J'); eq('erase to the cursor', rows(s), ['', '  f'])
	s = run(8, 1, 'abcd\x1b[1;2H\x1b[2@XY'); eq('insert characters', rows(s), ['aXYbcd'])
	s = run(8, 1, 'abcdef\x1b[1;2H\x1b[2P'); eq('delete characters', rows(s), ['adef'])
	s = run(8, 1, 'abcdef\x1b[1;2H\x1b[3X'); eq('erase characters', rows(s), ['a   ef'])
	s = run(3, 4, 'a\r\nb\r\nc\r\nd\x1b[2;3r\x1b[2;1H\x1b[L'); eq('insert a line in the scroll region', rows(s), ['a', '', 'b', 'd'])
	s.write('\x1b[M'); eq('delete a line in it', rows(s), ['a', 'b', '', 'd'])
	s = run(3, 3, 'x\x1b[2;3r\x1b[2;1H1\r\n2\r\n3\r\n4'); eq('a line feed at the region bottom scrolls the region only', rows(s), ['x', '3', '4']); eq('a region below the top keeps nothing', s.back.length, 0)
	s = run(3, 2, 'a\r\nb\x1b[H\x1bM'); eq('reverse index at the top', rows(s), ['', 'a'])
	s = run(6, 1, 'ab\x1b7cd\x1b8X'); eq('save and restore the cursor', rows(s), ['abXd'])
	s = run(8, 2, 'main\x1b[?1049h\x1b[Halt'); eq('the alternate screen', [s.altOn, rows(s)], [true, ['alt', '']])
	s.write('\x1b[?1049lX'); eq('back on the main screen, the cursor where it was', [s.altOn, rows(s)], [false, ['mainX', '']])
	s = run(6, 2, '中文x'); eq('wide characters', [s.lines[0].chars, s.x], [['中', '', '文', '', 'x', ' '], 5])
	s = run(3, 2, 'ab中'); eq('a wide character past the last column wraps', rows(s), ['ab', '中'])
	s = run(4, 1, '中文\x1b[1;2HX'); eq('drawing over half a wide character', rows(s), [' X文'])
	s = run(4, 1, 'éz'); eq('a combining mark joins its cell', [s.lines[0].chars.slice(0, 2), s.x], [['é', 'z'], 2])
	s = run(9, 1, 'a\tb'); eq('tab', rows(s), ['a       b'])
	s = run(4, 1, 'ab\bX'); eq('backspace', rows(s), ['aX'])
	s = run(6, 1, 'a\x1b[3b'); eq('repeat', rows(s), ['aaaa'])
	s = run(6, 1, '\x1b(0qx\x1b(Bq'); eq('line drawing', rows(s), ['─│q'])
	s = run(4, 2, 'a\x1b\nb'); eq('a control character after a lone ESC still acts', rows(s), ['a', ' b'])
	s = run(8, 1, '\x1b]0;a title\x07ok \x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\'); eq('strings are read and ignored', rows(s), ['ok link'])
	s = run(8, 1); s.write('\x1b['); s.write('3'); s.write('1mR\x1b]0;ti'); s.write('tle\x07ok'); eq('sequences cut between two pieces', [rows(s), s.lines[0].pens[0].fg], [['Rok'], 1])
	const pen = (seq) => { const p = run(4, 1, seq + 'X').lines[0].pens[0]; return [p.fg, p.bg, p.fl] }
	eq('bold red', pen('\x1b[1;31m'), [1, -1, 1])
	eq('bright', pen('\x1b[91;104m'), [9, 12, 0])
	eq('256 colors', pen('\x1b[38;5;196;48;5;21m'), [196, 21, 0])
	eq('true color', pen('\x1b[48;2;1;2;3m'), [-1, 0x1000000 + 0x010203, 0])
	eq('true color, the colon form', pen('\x1b[38:2::10:20:30m'), [0x1000000 + 0x0a141e, -1, 0])
	eq('256 colors, the colon form', pen('\x1b[38:5:100m'), [100, -1, 0])
	eq('attributes', pen('\x1b[2;3;4;7;9m'), [-1, -1, 2 | 4 | 8 | 16 | 64])
	eq('attributes off', pen('\x1b[1;3;4m\x1b[22;23;24m'), [-1, -1, 0])
	eq('a reset', pen('\x1b[1;31;42m\x1b[m'), [-1, -1, 0])
	s = run(4, 2, '\x1b[44m\x1b[2J'); eq('an erase takes the background', s.lines[1].pens[3].bg, 4)
	s = new Screen(10, 3); eq('a screen joined midway: Claude Code hides the cursor, takes pastes in brackets', [s.cursorOn, s.paste], [false, true])
	s.write('\x1b[?25h\x1b[?2004l'); eq('the modes as the output sets them', [s.cursorOn, s.paste], [true, false])
	s.write('\x1b[?2004h\x1b[?1h'); eq('bracketed paste and the cursor keys', [s.paste, s.appCursor], [true, true])
	s.write('\x1bc'); eq('a full reset', [s.cursorOn, s.paste, s.appCursor], [true, false, false])
	s = run(10, 3, 'a\r\nb\r\nc\x1b[8;2;5t'); eq('the size the bridge reports', [s.cols, s.rows, rows(s), s.back.map(l => l.text)], [5, 2, ['b', 'c'], ['a']])
	s.write('\x1b[8;4;6t'); eq('a bigger one: what was kept above comes back down', [s.cols, s.rows, rows(s), s.y, s.back.length], [6, 4, ['a', 'b', 'c', ''], 2, 0])
	s.write('\x1b[4;1Hx\r\ny\r\nz'); eq('scrolled off again', [rows(s), s.back.map(l => l.text)], [['c', 'x', 'y', 'z'], ['a', 'b']])
	const epoch = s.backEpoch
	s.write('\x1b[3J'); eq('erasing what scrolled off', [s.back.length, s.backEpoch - epoch, rows(s)], [0, 1, ['c', 'x', 'y', 'z']])
	return fails
})()`
