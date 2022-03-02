package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// ServiceSetConfig provides configuration options for a ServiceSet
type ServiceSetConfig struct {
	EnableServiceList  bool
	EnableTileJSON     bool
	EnablePreview      bool
	EnableArcGIS       bool
	EnableReloadSignal bool
	AuthToken          string

	RootURL     *url.URL
	ErrorWriter io.Writer
	GenerateID  func(string, string) (string, error)
	BaseDir     string
}

// ServiceSet is a group of tilesets plus configuration options.
// It provides access to all tilesets from a root URL.
type ServiceSet struct {
	tilesets map[string]*Tileset

	enableServiceList  bool
	enableTileJSON     bool
	enablePreview      bool
	enableArcGIS       bool
	enableReloadSignal bool
	authToken          string

	domain      string
	rootURL     *url.URL
	errorWriter io.Writer
	generateID  func(string, string) (string, error)
	baseDir     string
}

// New returns a new ServiceSet.
// If no ServiceSetConfig is provided, the service is initialized with default
// values of ServiceSetConfig.
func New(cfg *ServiceSetConfig) (*ServiceSet, error) {
	if cfg == nil {
		cfg = &ServiceSetConfig{}
	}

	s := &ServiceSet{
		tilesets:           make(map[string]*Tileset),
		enableServiceList:  cfg.EnableServiceList,
		enableTileJSON:     cfg.EnableTileJSON,
		enablePreview:      cfg.EnablePreview,
		enableArcGIS:       cfg.EnableArcGIS,
		enableReloadSignal: cfg.EnableReloadSignal,
		authToken:          cfg.AuthToken,

		rootURL:     cfg.RootURL,
		errorWriter: cfg.ErrorWriter,
		generateID:  cfg.GenerateID,
		baseDir:     cfg.BaseDir,
	}

	return s, nil
}

// AddTileset adds a single tileset identified by idGenerator using the filename.
// If a service already exists with that ID, an error is returned.
func (s *ServiceSet) AddTileset(filename, id string) error {
	if _, ok := s.tilesets[id]; ok {
		return fmt.Errorf("Tileset already exists for ID: %q", id)
	}

	path := s.rootURL.Path + "/" + id
	ts, err := newTileset(s, filename, id, path)
	if err != nil {
		return err
	}

	s.tilesets[id] = ts

	return nil
}

// UpdateTileset reloads the Tileset identified by id, if it already exists.
// Otherwise, this returns an error.
// Any errors encountered updating the Tileset are returned.
func (s *ServiceSet) UpdateTileset(id string) error {
	ts, ok := s.tilesets[id]
	if !ok {
		return fmt.Errorf("Tileset does not exist with ID: %q", id)
	}

	err := ts.reload()
	if err != nil {
		return err
	}

	return nil
}

// RemoveTileset removes the Tileset and closes the associated mbtiles file
// identified by id, if it already exists.
// If it does not exist, this returns without error.
// Any errors encountered removing the Tileset are returned.
func (s *ServiceSet) RemoveTileset(id string) error {
	ts, ok := s.tilesets[id]
	if !ok {
		return nil
	}

	err := ts.delete()
	if err != nil {
		return err
	}

	// remove from tilesets and router
	delete(s.tilesets, id)

	return nil
}

// LockTileset sets a write mutex on the tileset to block reads while this
// tileset is being updated.
// This is ignored if the tileset does not exist.
func (s *ServiceSet) LockTileset(id string) {
	ts, ok := s.tilesets[id]
	if !ok || ts == nil {
		return
	}

	ts.locked = true
}

// UnlockTileset removes the write mutex on the tileset.
// This is ignored if the tileset does not exist.
func (s *ServiceSet) UnlockTileset(id string) {
	ts, ok := s.tilesets[id]
	if !ok || ts == nil {
		return
	}

	ts.locked = false
}

// HasTileset returns true if the tileset identified by id exists within this
// ServiceSet.
func (s *ServiceSet) HasTileset(id string) bool {
	if _, ok := s.tilesets[id]; ok {
		return true
	}
	return false
}

// Size returns the number of tilesets in this ServiceSet
func (s *ServiceSet) Size() int {
	return len(s.tilesets)
}

// ServiceInfo provides basic information about the service.
type ServiceInfo struct {
	ImageType string `json:"imageType"`
	URL       string `json:"url"`
	Name      string `json:"name"`
}

// logError writes to the configured ServiceSet.errorWriter if available
// or the standard logger otherwise.
func (s *ServiceSet) logError(format string, args ...interface{}) {
	if s.errorWriter != nil {
		s.errorWriter.Write([]byte(fmt.Sprintf(format, args...)))
	} else {
		log.Printf(format, args...)
	}
}

func (s *ServiceSet) loadEndpointHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	path := r.URL.Query().Get("path")
	if s.authToken == "" || path == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	} else if token == s.authToken {
		_, err := w.Write([]byte("OK"))
		if err != nil {
			s.logError("Error rendering response during reload: %v", err)
			return
		}
		id, err := s.generateID(path, s.baseDir)
		if err != nil {
			s.logError("Could not create ID for tileset %q\n%v", path, err)
			return
		}
		err = s.AddTileset(path, id)
		if err != nil {
			s.logError("Could not add tileset for %q with ID %q\n%v", path, id, err)
		} else {
			log.Printf("Updated tileset %q with ID %q\n", path, id)
		}
		return
	}
	http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
}

func (s *ServiceSet) unloadEndpointHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	path := r.URL.Query().Get("path")
	if s.authToken == "" || path == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	} else if token == s.authToken {
		_, err := w.Write([]byte("OK"))
		if err != nil {
			s.logError("Error rendering response during reload: %v", err)
			return
		}
		id, err := s.generateID(path, s.baseDir)
		if err != nil {
			s.logError("Could not create ID for tileset %q\n%v", path, err)
			return
		}
		err = s.RemoveTileset(id)
		if err != nil {
			s.logError("Could not remove tileset for %q with ID %q\n%v", path, id, err)
		} else {
			log.Printf("Removed tileset %q with ID %q\n", path, id)
		}
		return
	}
	http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
}

// serviceListHandler is an http.HandlerFunc that provides a listing of all
// published services in this ServiceSet
func (s *ServiceSet) serviceListHandler(w http.ResponseWriter, r *http.Request) {
	rootURL := fmt.Sprintf("%s://%s%s", scheme(r), getRequestHost(r), r.URL)
	services := []ServiceInfo{}

	// sort ids alpabetically
	var ids []string
	for id := range s.tilesets {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		ts := s.tilesets[id]
		services = append(services, ServiceInfo{
			ImageType: ts.tileFormatString(),
			URL:       fmt.Sprintf("%s/%s", rootURL, id),
			Name:      ts.name,
		})
	}
	bytes, err := json.Marshal(services)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		s.logError("Error marshalling service list JSON for %v: %v", r.URL.Path, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(bytes)

	if err != nil {
		s.logError("Error writing service list content: %v", err)
	}
}

// tilesetHandler is an http.HandlerFunc that handles a given tileset
// and associated subpaths
func (s *ServiceSet) tilesetHandler(w http.ResponseWriter, r *http.Request) {
	id := s.IDFromURLPath((r.URL.Path))

	if id == "" {
		http.Error(w, "404 page not found", http.StatusNotFound)
		return
	}

	s.tilesets[id].router.ServeHTTP(w, r)
}

// IDFromURLPath extracts a tileset ID from a URL Path.
// If no valid ID is found, a blank string is returned.
func (s *ServiceSet) IDFromURLPath(id string) string {
	root := s.rootURL.Path + "/"

	if strings.HasPrefix(id, root) {
		id = strings.TrimPrefix(id, root)

		// test exact match first
		if _, ok := s.tilesets[id]; ok {
			return id
		}

		// Split on /tiles/ and /map/ and trim /map
		i := strings.LastIndex(id, "/tiles/")
		if i != -1 {
			id = id[:i]
		} else if s.enablePreview {
			id = strings.TrimSuffix(id, "/map")

			i = strings.LastIndex(id, "/map/")
			if i != -1 {
				id = id[:i]
			}
		}
	} else if s.enableArcGIS && strings.HasPrefix(id, ArcGISServicesRoot) {
		id = strings.TrimPrefix(id, ArcGISServicesRoot)
		// MapServer should be a reserved word, so should be OK to split on it
		id = strings.Split(id, "/MapServer")[0]
	} else {
		// not on a subpath of service roots, so no id
		return ""
	}

	// make sure tileset exists
	if _, ok := s.tilesets[id]; ok {
		return id
	}

	return ""
}

// Handler returns a http.Handler that serves the endpoints of the ServiceSet.
// The function ef is called with any occurring error if it is non-nil, so it
// can be used for e.g. logging with logging facilities of the caller.
func (s *ServiceSet) Handler() http.Handler {
	m := http.NewServeMux()

	root := s.rootURL.Path + "/"

	// Route requests at the tileset or subpath to the corresponding tileset
	m.HandleFunc(root, s.tilesetHandler)

	if s.enableServiceList {
		m.HandleFunc(s.rootURL.Path, s.serviceListHandler)
	} else {
		m.Handle(s.rootURL.Path, http.NotFoundHandler())
	}

	if s.enableArcGIS {
		m.HandleFunc(ArcGISInfoRoot, s.arcgisInfoHandler)
		m.HandleFunc(ArcGISServicesRoot, s.tilesetHandler)
	} else {
		m.Handle(ArcGISRoot, http.NotFoundHandler())
	}

	m.HandleFunc("/load", s.loadEndpointHandler)
	m.HandleFunc("/unload", s.unloadEndpointHandler)
	return m
}

// getRequestHost returns the value of the X-Forwarded-Host header if set, otherwise
// the request Host value
func getRequestHost(r *http.Request) string {
	host := r.Header.Get("X-Forwarded-Host")
	if host != "" {
		return host
	}
	return r.Host
}
