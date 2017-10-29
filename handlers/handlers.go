package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/consbio/mbtileserver/mbtiles"
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

// ServiceInfo consists of two strings that contain the image type and a URL.
type ServiceInfo struct {
	ImageType string `json:"imageType"`
	URL       string `json:"url"`
}

// ServiceSet is the base type for the HTTP handlers which combines multiple
// mbtiles.DB tilesets.
type ServiceSet struct {
	tilesets map[string]*mbtiles.DB
	Domain   string
	Path     string
}

// New returns a new ServiceSet. Use AddDBFromPath to add a mbtiles file.
func New() *ServiceSet {
	return &ServiceSet{
		tilesets: make(map[string]*mbtiles.DB),
	}
}

// AddDB interprets filename a path to an mbtiles file which is opened and
// served under id. In case the file cannot be opened, nil is returned.
func (s *ServiceSet) AddDB(filename string) error {
	subpath, err := filepath.Rel(".", filename)
	if err != nil {
		return fmt.Errorf("unable to extract ID for %q: %v", filename, err)
	}
	return s.addDBWithSubPath(filename, subpath)
}

func (s *ServiceSet) addDBWithSubPath(filename, subpath string) error {
	e := filepath.Ext(filename)
	p := filepath.ToSlash(subpath)
	id := strings.ToLower(p[:len(p)-len(e)])
	var err error
	ts, err := mbtiles.NewDB(filename)
	if err != nil {
		return fmt.Errorf("could not open mbtiles file %q: %v", filename, err)
	}
	s.tilesets[id] = ts
	return nil
}

// NewFromPath returns a ServiceSet that combines all .mbtiles files under path.
func NewFromPath(path string) (*ServiceSet, error) {
	var filenames []string
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if ext := filepath.Ext(path); ext == ".mbtiles" {
			filenames = append(filenames, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("unable to scan tilesets: %v", err)
	}

	if len(filenames) == 0 {
		return nil, fmt.Errorf("no tilesets found in %q", path)
	}

	s := New()

	for _, filename := range filenames {
		subpath, err := filepath.Rel(path, filename)
		if err != nil {
			return nil, fmt.Errorf("unable to extract ID for %q: %v", filename, err)
		}
		err = s.addDBWithSubPath(filename, subpath)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Len returns the number of tilesets in this ServiceSet
func (s *ServiceSet) Len() int {
	return len(s.tilesets)
}

// RootURL returns the root URL of the service. If s.Domain is non-empty, it
// will be used as the hostname. If s.Path is non-empty, it will be used as a
// prefix.
func (s *ServiceSet) RootURL(r *http.Request) string {
	return RootURL(r, s.Domain, s.Path)
}

func (s *ServiceSet) listServices(w http.ResponseWriter, r *http.Request) (int, error) {
	rootURL := fmt.Sprintf("%s%s", s.RootURL(r), r.URL)
	services := []ServiceInfo{}
	for id, tileset := range s.tilesets {
		services = append(services, ServiceInfo{
			ImageType: tileset.TileFormatString(),
			URL:       fmt.Sprintf("%s/%s", rootURL, id),
		})
	}
	bytes, err := json.Marshal(services)
	if err != nil {
		return 500, fmt.Errorf("cannot marshal services JSON: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(bytes)
	if err != nil {
		return 0, err
	}
	return 0, nil
}

func (s *ServiceSet) serviceInfo(db *mbtiles.DB) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		// TODO implement this
		return http.StatusNotImplemented, nil
	}
}

func (s *ServiceSet) serviceHTML(db *mbtiles.DB) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		// TODO implement this
		return http.StatusNotImplemented, nil
	}
}

func (s *ServiceSet) tiles(db *mbtiles.DB) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		// TODO implement this
		return http.StatusNotImplemented, nil
	}
}

// Handler returns a http.Handler that serves all endpoints of the ServiceSet.
// The function ef is called with any occuring error if it is non-nil, so it
// can be used for e.g. logging with logging facitilies of the caller.
func (s *ServiceSet) Handler(ef func(error)) http.Handler {
	m := http.NewServeMux()
	m.Handle("/services", wrapGetWithErrors(ef, s.listServices))
	for id, db := range s.tilesets {
		p := "/services/" + id
		m.Handle(p, wrapGetWithErrors(ef, s.serviceInfo(db)))
		m.Handle(p+"/map", wrapGetWithErrors(ef, s.serviceHTML(db)))
		m.Handle(p+"/tiles/", wrapGetWithErrors(ef, s.tiles(db)))
		// TODO arcgis handlers
		// p = "//arcgis/rest/services/" + id + "/MapServer"
		// m.Handle(p, wrapGetWithErrors(s.getArcGISService))
		// m.Handle(p + "/layers", wrapGetWithErrors(s.getArcGISLayers))
		// m.Handle(p + "/legend", wrapGetWithErrors(s.getArcGISLegend))
		// m.Handle(p + "/tile/", wrapGetWithErrors(s.getArcGISTile))
	}
	return m
}
