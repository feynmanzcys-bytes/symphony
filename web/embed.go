// Package web 打包审批台和办公室页面，bridge 二进制自带，不依赖运行目录。
package web

import "embed"

//go:embed office.html approve.html asks.js
var FS embed.FS
