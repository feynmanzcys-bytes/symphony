// Package web bundles the office page's themes, the approval page, the shared modules and their translations into
// the bridge binary, so it doesn't depend on the folder it runs from.
package web

import "embed"

//go:embed approve.html approve.js asks.js i18n.js symphony.js locales/*.json ui themes
var FS embed.FS
