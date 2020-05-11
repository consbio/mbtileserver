package handlers

import (
	"net/http"
	"path"
	"strings"
	"time"
)

// staticHandler returns a handler that retrieves static files from the virtual
// assets filesystem based on a path.  The URL prefix of the resource where
// these are accessed is first trimmed before requesting from the filesystem.
func staticHandler(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filePath := strings.TrimPrefix(r.URL.Path, prefix)
		f, err := Assets.Open(path.Join("/static/dist", filePath))
		if err != nil {
			// not an error, file was not found
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		mtime := time.Now()
		st, err := f.Stat()
		if err == nil {
			mtime = st.ModTime()
		}
		http.ServeContent(w, r, path.Base(r.URL.Path), mtime, f)
	})
}
