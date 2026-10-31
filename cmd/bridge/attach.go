package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
)

// symphony attach [-addr host:port] <launch id>: shows a session's terminal in this one. The bridge hosts the
// session on a pseudo-terminal; this window gets its output and sends the keystrokes, like tmux attach. Ctrl-]
// (or closing the window) detaches; the session goes on.
const detachKey = 0x1d // Ctrl-]

func attach(args []string) {
	fs := flag.NewFlagSet("attach", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:7788", i18n.T("flag.addr"))
	fs.Usage = func() { fmt.Fprintln(os.Stderr, i18n.T("attach.usage")) }
	_ = fs.Parse(args)
	id := fs.Arg(0)
	if id == "" {
		fs.Usage()
		os.Exit(2)
	}
	base := "http://" + *addr + "/api/terminal/" + id
	cols, rows := consoleSize()
	res, err := http.Get(base + "/stream?cols=" + strconv.Itoa(cols) + "&rows=" + strconv.Itoa(rows))
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("attach.failed", "reason", err.Error()))
		os.Exit(1)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		var body struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		_ = json.NewDecoder(res.Body).Decode(&body)
		reason := body.Error
		if i18n.Has("error." + body.Code) {
			reason = i18n.T("error." + body.Code)
		}
		fmt.Fprintln(os.Stderr, i18n.T("attach.failed", "reason", reason))
		os.Exit(1)
	}
	name := res.Header.Get("x-symphony-name")
	restore, err := rawMode()
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("attach.failed", "reason", err.Error()))
		os.Exit(1)
	}
	defer restore()
	// the window's title, and one line of help before the session's screen
	fmt.Fprintf(os.Stdout, "\x1b]0;%s\x07\r\n\x1b[2m%s\x1b[0m\r\n", name, i18n.T("attach.banner", "name", name))

	client := &http.Client{Timeout: 10 * time.Second}
	post := func(path string, body []byte, ctype string) {
		req, _ := http.NewRequest("POST", base+path, bytes.NewReader(body))
		req.Header.Set("content-type", ctype)
		if r, err := client.Do(req); err == nil {
			r.Body.Close()
		}
	}
	done := make(chan struct{})
	go func() { // keystrokes
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				if i := bytes.IndexByte(buf[:n], detachKey); i >= 0 {
					if i > 0 {
						post("/input", buf[:i], "application/octet-stream")
					}
					close(done)
					return
				}
				post("/input", buf[:n], "application/octet-stream")
			}
			if err != nil {
				close(done)
				return
			}
		}
	}()
	go watchSize(func(cols, rows int) { // the window changes size
		body, _ := json.Marshal(map[string]int{"cols": cols, "rows": rows})
		post("/resize", body, "application/json")
	})
	ended := make(chan struct{})
	go func() {
		_, _ = io.Copy(os.Stdout, res.Body)
		close(ended)
	}()
	select {
	case <-done:
		fmt.Fprintf(os.Stdout, "\r\n\x1b[2m%s\x1b[0m\r\n", i18n.T("attach.detached", "name", name))
	case <-ended:
		fmt.Fprintf(os.Stdout, "\r\n\x1b[2m%s\x1b[0m\r\n", i18n.T("attach.ended", "name", name))
	}
}
