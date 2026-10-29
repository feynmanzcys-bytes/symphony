// symphony bridge：go run ./cmd/bridge [-addr 127.0.0.1:7788] [-data 数据目录] [-web web]
package main

import (
	"flag"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
	"github.com/feynmanzcys-bytes/symphony/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7788", "监听地址")
	dataDir := flag.String("data", defaultDataDir(), "数据目录：账号（accounts.json，含 token）和消息记录（messages.json）")
	webDir := flag.String("web", "", "开发用：直接读这个目录下的页面，改了刷新就生效；不给就用编译进来的")
	modDir := flag.String("mod", "mods/symphony-bridge", "mod 目录，页面启动会话时用 --plugin-dir 带上")
	claude := flag.String("claude", "claude", "claude 可执行文件")
	terminal := flag.String("terminal", "auto", "页面启动会话用什么终端：auto（Linux 有 tmux 用 tmux，其余弹窗口）| tmux | window")
	flag.Parse()

	var pages fs.FS = web.FS
	if *webDir != "" {
		pages = os.DirFS(*webDir)
	}

	hub := bridge.NewHub()
	if err := hub.Open(*dataDir); err != nil {
		log.Fatal(err)
	}
	srv := bridge.NewServer(hub, pages)
	host, port, err := net.SplitHostPort(*addr)
	if err != nil {
		log.Fatal(err)
	}
	if host != "" && host != "0.0.0.0" && host != "::" {
		srv.AllowedHosts = append(srv.AllowedHosts, host)
	} else {
		host = "127.0.0.1"
	}
	if l := launcher(*modDir, *claude, *terminal, "http://"+net.JoinHostPort(host, port)); l != nil {
		srv.SetLauncher(l)
	}
	go func() {
		for range time.Tick(time.Minute) {
			hub.Prune()
		}
	}()
	go func() {
		for range time.Tick(5 * time.Second) {
			hub.Reap() // 太久没心跳的会话标成下班
		}
	}()

	log.Printf("symphony bridge: http://%s  （办公室 /，审批台 /approve；数据目录 %s）", *addr, *dataDir)
	log.Fatal(http.ListenAndServe(*addr, srv))
}

func defaultDataDir() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "symphony")
	}
	return ".symphony"
}

// launcher 准备页面启动会话要用的东西；缺 claude、mod 目录或能用的终端就不开这个功能。
func launcher(modDir, claude, terminal, bridgeURL string) *bridge.Launcher {
	abs, err := filepath.Abs(modDir)
	if err == nil {
		_, err = os.Stat(filepath.Join(abs, ".claude-plugin", "plugin.json"))
	}
	if err != nil {
		log.Printf("页面启动会话关闭：找不到 mod（-mod %s）", modDir)
		return nil
	}
	claudePath, err := exec.LookPath(claude)
	if err != nil {
		log.Printf("页面启动会话关闭：找不到 %s", claude)
		return nil
	}
	term, err := bridge.PickTerminal(runtime.GOOS, terminal, exec.LookPath)
	if err != nil {
		log.Printf("页面启动会话关闭：%v", err)
		return nil
	}
	log.Printf("页面启动会话：用 %s 开终端", term.Name())
	return bridge.NewLauncher(claudePath, abs, bridgeURL, term)
}
