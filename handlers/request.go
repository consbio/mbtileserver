package handlers

import (
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"
)

// scheme returns the underlying URL scheme of the original request.
func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if scheme := r.Header.Get("X-Forwarded-Proto"); scheme != "" {
		return scheme
	}
	if scheme := r.Header.Get("X-Forwarded-Protocol"); scheme != "" {
		return scheme
	}
	if ssl := r.Header.Get("X-Forwarded-Ssl"); ssl == "on" {
		return "https"
	}
	if scheme := r.Header.Get("X-Url-Scheme"); scheme != "" {
		return scheme
	}
	return "http"
}

type handlerFunc func(http.ResponseWriter, *http.Request) (int, error)

// wrapJSONP writes b (JSON marshalled to bytes) as a JSONP response to
// w if the callback query parameter is present, and writes b as a JSON
// response otherwise. Any error that occurs during writing is returned.
func wrapJSONP(w http.ResponseWriter, r *http.Request, b []byte) (err error) {
	callback := r.URL.Query().Get("callback")

	if callback != "" {
		w.Header().Set("Content-Type", "application/javascript")
		_, err = w.Write([]byte(fmt.Sprintf("%s(%s);", callback, b)))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(b)
	return
}

func wrapGetWithErrors(ef func(error), hf handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			status := http.StatusMethodNotAllowed
			http.Error(w, http.StatusText(status), status)
			return
		}
		status, err := hf(w, r) // run the handlerFunc and obtain the return codes
		if err != nil && ef != nil {
			ef(err) // handle the error with the supplied function
		}
		// in case it's an error, write the status code for the requester
		if status >= 400 {
			http.Error(w, http.StatusText(status), status)
		}
	})
}

func staticHandler(prefix string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		filePath := strings.TrimPrefix(r.URL.Path, prefix)
		f, err := Assets.Open(path.Join("/static/dist", filePath))
		if err != nil {
			// not an error, file was not found
			return http.StatusNotFound, nil
		}
		defer f.Close()
		mtime := time.Now()
		st, err := f.Stat()
		if err == nil {
			mtime = st.ModTime()
		}
		http.ServeContent(w, r, path.Base(r.URL.Path), mtime, f)
		return http.StatusOK, nil
	}
}
