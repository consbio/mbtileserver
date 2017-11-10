package handlers

import (
	"net/http"
	"path"
	"time"
)

// Static returns an http.Handler that will serve the contents of
// the subdirectory "/static/dist" of the Assets.
func Static() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := Assets.Open(path.Join("/static/dist", r.URL.Path))
		if err != nil {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
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
