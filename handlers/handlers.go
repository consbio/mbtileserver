package handlers

import (
	"fmt"
	"net/http"
)

// RootURL returns the root URL of the HTTP request. Optionally, a domain and a
// base path may be provided which will be used to construct the root URL if
// they are not empty. Otherwise the hostname will be determined from the
// request and the path will be empty.
func RootURL(r *http.Request, domain, path string) string {
	host := r.Host
	if len(domain) > 0 {
		host = domain
	}

	root := fmt.Sprintf("%s://%s", Scheme(r), host)
	if len(path) > 0 {
		root = fmt.Sprintf("%s/%s", root, path)
	}

	return root
}

// Scheme returns the underlying URL scheme of the original request.
func Scheme(r *http.Request) string {
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
