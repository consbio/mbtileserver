package handlers

import (
	"net/http"
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
