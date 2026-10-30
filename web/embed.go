// Package web bundles the approval and office pages and their translations into the bridge binary,
// so it doesn't depend on the folder it runs from.
package web

import "embed"

//go:embed office.html approve.html asks.js i18n.js locales/*.json
var FS embed.FS
