package handlers

//go:generate go run -tags=dev assets_generate.go

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
	tilesets  map[string]*mbtiles.DB
	templates *template.Template
	Domain    string
	Path      string
}

// New returns a new ServiceSet. Use AddDBOnPath to add a mbtiles file.
func New() *ServiceSet {
	s := &ServiceSet{
		tilesets:  make(map[string]*mbtiles.DB),
		templates: template.New("_base_"),
	}
	return s

}

// AddDBOnPath interprets filename as mbtiles file which is opened and which will be
// served under "/services/<urlPath>" by Handler(). The parameter urlPath may not be
// nil, otherwise an error is returned. In case the DB cannot be opened the returned
// error is non-nil.
func (s *ServiceSet) AddDBOnPath(filename string, urlPath string) error {
	var err error
	if urlPath == "" {
		return fmt.Errorf("path parameter may not be empty")
	}
	ts, err := mbtiles.NewDB(filename)
	if err != nil {
		return fmt.Errorf("could not open mbtiles file %q: %v", filename, err)
	}
	s.tilesets[urlPath] = ts
	return nil
}

// NewFromBaseDir returns a ServiceSet that combines all .mbtiles files under
// the directory at baseDir. The DBs will all be served under their relative paths
// to baseDir.
func NewFromBaseDir(baseDir string) (*ServiceSet, error) {
	var filenames []string
	err := filepath.Walk(baseDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if ext := filepath.Ext(p); ext == ".mbtiles" {
			filenames = append(filenames, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("unable to scan tilesets: %v", err)
	}

	if len(filenames) == 0 {
		return nil, fmt.Errorf("no tilesets found in %q", baseDir)
	}

	s := New()

	for _, filename := range filenames {
		subpath, err := filepath.Rel(baseDir, filename)
		if err != nil {
			return nil, fmt.Errorf("unable to extract URL path for %q: %v", filename, err)
		}
		e := filepath.Ext(filename)
		p := filepath.ToSlash(subpath)
		id := strings.ToLower(p[:len(p)-len(e)])
		err = s.AddDBOnPath(filename, id)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Len returns the number of tilesets in this ServiceSet
func (s *ServiceSet) Size() int {
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
		return http.StatusInternalServerError, fmt.Errorf("cannot marshal services JSON: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(bytes)
	return http.StatusOK, err
}

func (s *ServiceSet) serviceInfo(id string, db *mbtiles.DB) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		svcURL := fmt.Sprintf("%s%s", s.RootURL(r), r.URL.Path)
		imgFormat := db.TileFormatString()
		out := map[string]interface{}{
			"tilejson": "2.1.0",
			"id":       id,
			"scheme":   "xyz",
			"format":   imgFormat,
			"tiles":    []string{fmt.Sprintf("%s/tiles/{z}/{x}/{y}.%s", svcURL, imgFormat)},
			"map":      fmt.Sprintf("%s/map", svcURL),
		}
		metadata, err := db.ReadMetadata()
		if err != nil {
			return http.StatusInternalServerError, err
		}
		for k, v := range metadata {
			switch k {
			// strip out values above
			case "tilejson", "id", "scheme", "format", "tiles", "map":
				continue

			// strip out values that are not supported or are overridden below
			case "grids", "interactivity", "modTime":
				continue

			// strip out values that come from TileMill but aren't useful here
			case "metatile", "scale", "autoscale", "_updated", "Layer", "Stylesheet":
				continue

			default:
				out[k] = v
			}
		}

		if db.HasUTFGrid() {
			out["grids"] = []string{fmt.Sprintf("%s/tiles/{z}/{x}/{y}.json", svcURL)}
		}
		bytes, err := json.Marshal(out)
		if err != nil {
			return http.StatusInternalServerError, fmt.Errorf("cannot marshal service info JSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(bytes)
		return http.StatusOK, err
	}
}

// executeTemplates first tries to find the template with the given name for
// the ServiceSet. If that fails, it tries to instantiate it from the assets.
// If a valid template is obtained it is used to render a response, otherwise
// the HTTP status Internal Server Error is returned.
func (s *ServiceSet) executeTemplate(w http.ResponseWriter, name string, data interface{}) (int, error) {
	t := s.templates.Lookup(name)
	var err error
	if t == nil {
		t, err = tmplFromAssets(s.templates, name)
		if err != nil {
			err = fmt.Errorf("could not parse template asset %q: %v", name, err)
			return http.StatusInternalServerError, err
		}
	}
	buf := &bytes.Buffer{}
	err = t.Execute(buf, data)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	_, err = io.Copy(w, buf)
	return http.StatusOK, err
}

func (s *ServiceSet) serviceHTML(id string, db *mbtiles.DB) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		p := struct {
			URL string
			ID  string
		}{
			fmt.Sprintf("%s%s", s.RootURL(r), strings.TrimSuffix(r.URL.Path, "/map")),
			id,
		}

		switch db.TileFormat() {
		default:
			return s.executeTemplate(w, "map", p)
		case mbtiles.PBF:
			return s.executeTemplate(w, "map_gl", p)

		}
	}
}

type tileCoord struct {
	z    uint8
	x, y uint64
}

// tileCoordFromString parses and returns tileCoord coordinates and an optional
// extension from the three parameters. The parameter z is interpreted as the
// web mercator zoom level, it is supposed to be an unsigned integer that will
// fit into 8 bit. The parameters x and y are interpreted as longitude and
// latitude tile indices for that zoom level, both are supposed be integers in
// the integer interval [0,2^z). Additionally, y may also have an optional
// filename extension (e.g. "42.png") which is removed before parsing the
// number, and returned, too. In case an error occured during parsing or if the
// values are not in the expected interval, the returned error is non-nil.
func tileCoordFromString(z, x, y string) (tc tileCoord, ext string, err error) {
	var z64 uint64
	if z64, err = strconv.ParseUint(z, 10, 8); err != nil {
		err = fmt.Errorf("cannot parse zoom level: %v", err)
		return
	}
	tc.z = uint8(z64)
	const (
		errMsgParse = "cannot parse %s coordinate axis: %v"
		errMsgOOB   = "%s coordinate (%d) is out of bounds for zoom level %d"
	)
	if tc.x, err = strconv.ParseUint(x, 10, 64); err != nil {
		err = fmt.Errorf(errMsgParse, "first", err)
		return
	}
	if tc.x >= (1 << z64) {
		err = fmt.Errorf(errMsgOOB, "x", tc.x, tc.z)
		return
	}
	s := y
	if l := strings.LastIndex(s, "."); l >= 0 {
		s, ext = s[:l], s[l:]
	}
	if tc.y, err = strconv.ParseUint(s, 10, 64); err != nil {
		err = fmt.Errorf(errMsgParse, "y", err)
		return
	}
	if tc.y >= (1 << z64) {
		err = fmt.Errorf(errMsgOOB, "y", tc.y, tc.z)
		return
	}
	return
}

// tileNotFoundHandler writes the default response for a non-existing tile of type f to w
func tileNotFoundHandler(w http.ResponseWriter, f mbtiles.TileFormat) (int, error) {
	var err error
	switch f {
	case mbtiles.PNG, mbtiles.JPG, mbtiles.WEBP:
		// Return blank PNG for all image types
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(BlankPNG())
	case mbtiles.PBF:
		// Return 204
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message": "Tile does not exist"}`)
	}
	return http.StatusOK, err // http.StatusOK doesn't matter, code was written by w.WriteHeader already
}

func (s *ServiceSet) tiles(db *mbtiles.DB) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		// split path components to extract tile coordinates x, y and z
		pcs := strings.Split(r.URL.Path[1:], "/")
		// we are expecting at least "services", <id> , "tiles", <z>, <x>, <y plus .ext>
		l := len(pcs)
		if l < 6 || pcs[5] == "" {
			return http.StatusBadRequest, fmt.Errorf("requested path is too short")
		}
		z, x, y := pcs[l-3], pcs[l-2], pcs[l-1]
		tc, ext, err := tileCoordFromString(z, x, y)
		if err != nil {
			return http.StatusBadRequest, err
		}
		var data []byte
		// flip y to match the spec
		tc.y = (1 << uint64(tc.z)) - 1 - tc.y
		isGrid := ext == ".json"
		switch {
		case !isGrid:
			err = db.ReadTile(tc.z, tc.x, tc.y, &data)
		case isGrid && db.HasUTFGrid():
			err = db.ReadGrid(tc.z, tc.x, tc.y, &data)
		default:
			err = fmt.Errorf("no grid supplied by tile database")
		}
		if err != nil {
			// augment error info
			t := "tile"
			if isGrid {
				t = "grid"
			}
			err = fmt.Errorf("cannot fetch %s from DB for z=%d, x=%d, y=%d: %v", t, tc.z, tc.x, tc.y, err)
			return http.StatusInternalServerError, err
		}
		if data == nil || len(data) <= 1 {
			return tileNotFoundHandler(w, db.TileFormat())
		}

		if isGrid {
			w.Header().Set("Content-Type", "application/json")
			if db.UTFGridCompression() == mbtiles.ZLIB {
				w.Header().Set("Content-Encoding", "deflate")
			} else {
				w.Header().Set("Content-Encoding", "gzip")
			}
		} else {
			w.Header().Set("Content-Type", db.ContentType())
			if db.TileFormat() == mbtiles.PBF {
				w.Header().Set("Content-Encoding", "gzip")
			}
		}
		_, err = w.Write(data)
		return http.StatusOK, err
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
		m.Handle(p, wrapGetWithErrors(ef, s.serviceInfo(id, db)))
		m.Handle(p+"/map", wrapGetWithErrors(ef, s.serviceHTML(id, db)))
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
