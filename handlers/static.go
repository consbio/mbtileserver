package handlers

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed templates/static/dist
var staticAssets embed.FS

// staticHandler returns a handler that retrieves static files from the virtual
// assets filesystem based on a path.  The URL prefix of the resource where
// these are accessed is first trimmed before requesting from the filesystem.
func staticHandler(prefix string) http.Handler {
	assetsFS, _ := fs.Sub(staticAssets, "templates/static/dist")
	return http.StripPrefix(prefix, http.FileServer(http.FS(assetsFS)))
}
