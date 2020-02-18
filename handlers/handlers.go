package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
)

type ServiceSetConfig struct {
	EnableServiceList bool
	EnableTileJSON    bool
	EnablePreview     bool

	Domain    string
	Path      string
	SecretKey string
}

// ServiceSet is the base type for the HTTP handlers which combines multiple
// mbtiles.DB tilesets.
type ServiceSet struct {
	tilesets map[string]*Tileset

	enableServiceList bool
	enableTileJSON    bool
	enablePreview     bool

	domain    string
	url       *url.URL
	secretKey string
}

// New returns a new ServiceSet.
// If no ServiceSetConfig is provided, the service is initialized with default
// values of ServiceSetConfig.
func New(cfg *ServiceSetConfig) (*ServiceSet, error) {
	if cfg == nil {
		cfg = &ServiceSetConfig{}
	}

	parsedPath, err := url.Parse(cfg.Path)
	if err != nil {
		return nil, err
	}

	s := &ServiceSet{
		tilesets:          make(map[string]*Tileset),
		enableServiceList: cfg.EnableServiceList,
		enableTileJSON:    cfg.EnableTileJSON,
		enablePreview:     cfg.EnablePreview,
		domain:            cfg.Domain,
		url:               parsedPath,
		secretKey:         cfg.SecretKey,
	}

	return s, nil
}

// AddTileset adds a single tileset identified by idGenerator using the filename.
// If a service already exists with that ID, it is reloaded.
// Returns any errors encountered adding the tileset.
func (s *ServiceSet) AddTileset(filename string, idGenerator IDGenerator) error {
	id, err := idGenerator(filename)
	if err != nil {
		return err
	}
	if ts, ok := s.tilesets[id]; ok {
		// if exists, then reload it
		err := ts.Reload()
		if err != nil {
			return err
		}
	} else {
		ts, err := NewTileset(filename, id)
		if err != nil {
			return err
		}

		s.tilesets[id] = ts
	}

	return nil
}

// AddTilesets adds multiple tilsets at a time.  Each is identified using
// idGenerator against its filename.
// If errors are encountered, they are combined across all failing tilesets
// and returned in a single error.
func (s *ServiceSet) AddTilesets(filenames []string, idGenerator IDGenerator) error {
	var errors = NewErrors()
	for _, filename := range filenames {
		err := s.AddTileset(filename, idGenerator)
		if err != nil {
			errors.AddError(err)
		}
	}

	if !errors.Empty() {
		return errors.Error()
	}

	return nil
}

// func (s *ServiceSet) UpdateTileset(filename string) error {

// 	return nil
// }

// func (s *ServiceSet) UpdateTilesets(filenames []string) error {

// 	return nil
// }

// func (s *ServiceSet) RemoveTileset(filename string) error {

// 	return nil
// }

// func (s *ServiceSet) RemoveTilesets(filename []string) error {

// 	return nil
// }

// Size returns the number of tilesets in this ServiceSet
func (s *ServiceSet) Size() int {
	return len(s.tilesets)
}

// rootURL returns the root URL of the service.
func (s *ServiceSet) rootURL(r *http.Request) string {
	return fmt.Sprintf("%s://%s", scheme(r), r.Host)
}

// ServiceInfo consists of two strings that contain the image type and a URL.
type ServiceInfo struct {
	ImageType string `json:"imageType"`
	URL       string `json:"url"`
}

// listServices returns a listing of all published services in this ServiceSet
func (s *ServiceSet) listServices(w http.ResponseWriter, r *http.Request) (int, error) {
	rootURL := fmt.Sprintf("%s%s", s.rootURL(r), r.URL)
	services := []ServiceInfo{}

	// sort ids alpabetically
	var ids []string
	for id := range s.tilesets {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// for id, tileset := range s.tilesets {
	for _, id := range ids {
		ts := s.tilesets[id]
		services = append(services, ServiceInfo{
			ImageType: ts.tileFormatString(),
			URL:       fmt.Sprintf("%s/%s", rootURL, id),
		})
	}
	bytes, err := json.Marshal(services)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("cannot marshal services JSON: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(bytes)
	return http.StatusOK, err
}

// Handler returns a http.Handler that serves the endpoints of the ServiceSet.
// The function ef is called with any occurring error if it is non-nil, so it
// can be used for e.g. logging with logging facilities of the caller.
func (s *ServiceSet) Handler(ef func(error)) (http.Handler, error) {
	m := http.NewServeMux()

	root := s.url.Path
	if s.enableServiceList {
		m.Handle(root, wrapGetWithErrors(ef, hmacAuth(s.listServices, s.secretKey, "")))
	}

	errors := NewErrors()
	for _, ts := range s.tilesets {
		prefix := root + "/" + ts.id

		m.Handle(prefix+"/tiles/", wrapGetWithErrors(ef, hmacAuth(ts.tileHandler(), s.secretKey, ts.id)))

		if s.enablePreview {
			m.Handle(prefix+"/map", wrapGetWithErrors(ef, hmacAuth(ts.previewHandler(), s.secretKey, ts.id)))
		}

		if s.enableTileJSON {
			m.Handle(prefix, wrapGetWithErrors(ef, hmacAuth(ts.tileJSONHandler(s.enablePreview), s.secretKey, ts.id)))
		}
	}

	if !errors.Empty() {
		return nil, errors.Error()
	}

	return m, nil
}
