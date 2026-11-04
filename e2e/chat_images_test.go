//go:build e2e

package e2e

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// TestChatImages sends an image into a conversation (no tokens): picked with the clip (the hidden file input), it
// shows small under the box until sent; sent with a text, the bridge keeps it in the session's folder, the bubble shows the
// text with the image, the session's inbox gets the text with the file's path for its Read tool, and the mod's echo of
// that text settles the bubble; one removed from the strip doesn't go.
func TestChatImages(t *testing.T) {
	b := startBridge(t, "")
	alice, err := b.Hub.CreateAccount("alice")
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir() // the session's folder: the images go in its .symphony/inbox
	b.post(t, alice.Token, "/api/event", map[string]any{"session": "s-alice", "kind": "session.start", "cwd": work})

	// a small PNG on disk, for the file input
	dir := t.TempDir()
	shot := filepath.Join(dir, "shot.png")
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	img.Set(2, 2, color.RGBA{R: 200, A: 255})
	f, err := os.Create(shot)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	browser := newBrowser(t)
	ctx, _ := clockTab(t, browser, b.URL+"/t/messenger/?lang=en", 0)
	do(t, ctx, 10*time.Second, chromedp.EmulateViewport(1400, 900))
	p := &screens{t: t, ctx: ctx, lang: "chat-images"}
	waitUntil(t, 20*time.Second, "Coworkers lists alice", func() bool { return p.is(`document.querySelectorAll('#people .row').length === 1`) })
	clickRow(t, ctx, alice.ID)
	waitUntil(t, 10*time.Second, "her conversation offers the clip", func() bool {
		return p.is(`!document.getElementById('chat').hidden && !!document.querySelector('#chat .say .attach') && document.querySelector('#chat .say .attach').getAttribute('aria-label').startsWith('Add an image')`)
	})
	// two picked, one removed: the strip shows what will go
	do(t, ctx, 10*time.Second, chromedp.SetUploadFiles(`#chat .say input[type=file]`, []string{shot, shot}))
	waitUntil(t, 10*time.Second, "the strip shows the two images", func() bool {
		return p.is(`document.querySelectorAll('#chat .pending .thumb img').length === 2 && !document.querySelector('#chat .pending').hidden`)
	})
	p.run(`document.querySelector('#chat .pending .thumb .rm').click()`)
	if !p.is(`document.querySelectorAll('#chat .pending .thumb').length === 1`) {
		t.Fatal("removing one from the strip should leave the other")
	}
	p.shot("pending")
	// sent with a text: the bubble, the file, the inbox
	p.run(`(b => { b.value = 'What is in this screenshot?'; b.form.requestSubmit() })(document.querySelector('#chat .say textarea'))`)
	waitUntil(t, 10*time.Second, "the queued bubble shows the text and the image", func() bool {
		return p.is(`(u => !!u && u.querySelector('.body').textContent === 'What is in this screenshot?' && u.querySelectorAll('.imgs img').length === 1 && u.querySelector('.imgs img').getAttribute('src').startsWith('/api/files/') && u.querySelector('.imgs img').naturalWidth === 8)(document.querySelector('#chat .flow .it.user'))` +
			` && document.querySelector('#chat .pending').hidden && document.querySelector('#chat .say textarea').value === ''`)
	}, func() string { return p.js(`document.querySelector('#chat .flow').innerHTML.slice(0, 600)`) })
	p.shot("sent")
	items, err := b.Hub.Inbox("s-alice", alice.ID, 0, time.Second, nil)
	if err != nil || len(items) != 1 || items[0].Kind != "say" {
		t.Fatalf("the inbox should hold the say: %v %v", items, err)
	}
	sent := items[0].Text
	if !strings.HasPrefix(sent, "What is in this screenshot?") || !strings.Contains(sent, "Attached image 1 (read it with the Read tool): ") {
		t.Fatalf("the session should get the text and the file's path: %q", sent)
	}
	path := strings.TrimSpace(sent[strings.LastIndex(sent, ": ")+2:])
	if st, err := os.Stat(path); err != nil || st.Size() == 0 || !strings.HasPrefix(path, filepath.Join(work, ".symphony", "inbox")) {
		t.Fatalf("the file should be in the session's folder: %s %v", path, err)
	}
	// the mod echoes the sent text (with the path): the bubble is sent
	b.post(t, alice.Token, "/api/event", map[string]any{"session": "s-alice", "kind": "chat.sent", "text": sent})
	waitUntil(t, 10*time.Second, "the bubble is sent", func() bool { return p.is(`!!document.querySelector('#chat .flow .it.user.sent .imgs img')`) })
	p.check()
}
