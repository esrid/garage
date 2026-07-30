// Package assets embeds the files served to browsers under /static/.
package assets

import (
	"embed"
	"io/fs"
)

// The whole css directory is embedded rather than individual files: app.css
// @imports tokens.css as a sibling URL, so a single-file embed would 404 on
// tokens.css and every page would render unstyled with no error anywhere.
//
//go:embed src/css src/js
var files embed.FS

// Static is the browser-facing tree. Paths are relative to assets/src, so
// /static/css/app.css resolves to assets/src/css/app.css.
func Static() fs.FS {
	sub, err := fs.Sub(files, "src")
	if err != nil {
		// The embedded paths are fixed at compile time: this cannot fail at
		// runtime, and failing loudly at startup beats serving a blank page.
		panic("assets: " + err.Error())
	}
	return sub
}
