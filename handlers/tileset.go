package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	mbtiles "github.com/brendan-ward/mbtiles-go"
)

// Tileset provides a tileset constructed from an mbtiles file
type Tileset struct {
	svc        *ServiceSet
	db         *mbtiles.MBtiles
	id         string
	name       string
	tileformat mbtiles.TileFormat
	published  bool
	locked     bool
	router     *http.ServeMux
}

// newTileset constructs a new Tileset from an mbtiles filename.
// Tileset is registered at the passed in path.
// Any errors encountered opening the tileset are returned.
func newTileset(svc *ServiceSet, filename, id, path string) (*Tileset, error) {
	db, err := mbtiles.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("Invalid mbtiles file %q: %v", filename, err)
	}

	metadata, err := db.ReadMetadata()
	if err != nil {
		return nil, fmt.Errorf("Invalid mbtiles file %q: %v", filename, err)
	}

	name, ok := metadata["name"].(string)
	if !ok {
		name = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}

	ts := &Tileset{
		svc:        svc,
		db:         db,
		id:         id,
		name:       name,
		tileformat: db.GetTileFormat(),
		published:  true,
	}

	// setup routes for tileset
	m := http.NewServeMux()
	m.HandleFunc(path+"/tiles/", ts.tileHandler)

	if svc.enableTileJSON {
		m.HandleFunc(path, ts.tileJSONHandler)
	}

	if svc.enablePreview {
		m.HandleFunc(path+"/map", ts.previewHandler)

		staticPrefix := path + "/map/static/"
		m.Handle(staticPrefix, staticHandler(staticPrefix))
	}

	if svc.enableArcGIS {
		arcgisRoot := ArcGISServicesRoot + id + "/MapServer"
		m.HandleFunc(arcgisRoot, ts.arcgisServiceHandler)
		m.HandleFunc(arcgisRoot+"/layers", ts.arcgisLayersHandler)
		m.HandleFunc(arcgisRoot+"/legend", ts.arcgisLegendHandler)
		m.HandleFunc(arcgisRoot+"/tile/", ts.arcgisTileHandler)
	}

	ts.router = m

	return ts, nil
}

// Reload reloads the mbtiles file from disk using the same filename as
// used when this was first constructed
func (ts *Tileset) reload() error {
	if ts.db == nil {
		return nil
	}

	filename := ts.db.GetFilename()
	ts.db.Close()

	db, err := mbtiles.Open(filename)
	if err != nil {
		return fmt.Errorf("Invalid mbtiles file %q: %v", filename, err)
	}
	ts.db = db

	return nil
}

// Delete closes and deletes the mbtiles file connection for this tileset
func (ts *Tileset) delete() error {
	if ts.db != nil {
		ts.db.Close()
	}
	ts.db = nil
	ts.published = false

	return nil
}

// tileFormatString returns the tile format string of the underlying mbtiles file
func (ts *Tileset) tileFormatString() string {
	return ts.tileformat.String()
}

// TileJSON returns the TileJSON (as a map of strings to interface{} values)
// for the tileset.  This can be rendered into templates or returned via a
// handler.
func (ts *Tileset) TileJSON(svcURL string, query string) (map[string]interface{}, error) {
	if ts == nil || !ts.published {
		return nil, fmt.Errorf("Tileset does not exist")
	}

	db := ts.db

	imgFormat := db.GetTileFormat().String()
	out := map[string]interface{}{
		"tilejson": "2.1.0",
		"scheme":   "xyz",
		"format":   imgFormat,
		"tiles":    []string{fmt.Sprintf("%s/tiles/{z}/{x}/{y}.%s%s", svcURL, imgFormat, query)},
		"name":     ts.name,
	}

	metadata, err := db.ReadMetadata()
	if err != nil {
		return nil, err
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
	return out, nil
}

// tilesJSONHandler is an http.HandlerFunc for the TileJSON endpoint of the tileset
func (ts *Tileset) tileJSONHandler(w http.ResponseWriter, r *http.Request) {
	if ts == nil || !ts.published {
		http.NotFound(w, r)
		return
	}

	// wait up to 30 seconds to see if tileset is ready and return it if possible
	if ts.isLockedWithTimeout(30 * time.Second) {
		tilesetLockedHandler(w, r)
		return
	}

	query := ""
	if r.URL.RawQuery != "" {
		query = "?" + r.URL.RawQuery
	}

	tilesetURL := fmt.Sprintf("%s://%s%s", scheme(r), getRequestHost(r), r.URL.Path)

	tileJSON, err := ts.TileJSON(tilesetURL, query)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		ts.svc.logError("could not create tileJSON content for %v: %v", r.URL.Path, err)
		return
	}

	if ts.svc.enablePreview {
		tileJSON["map"] = fmt.Sprintf("%s/map", tilesetURL)
	}

	bytes, err := json.Marshal(tileJSON)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		ts.svc.logError("could not render TileJSON for %v: %v", r.URL.Path, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(bytes)

	if err != nil {
		ts.svc.logError("could not write tileJSON content for %v: %v", r.URL.Path, err)
	}
}

// tileHandler is an http.HandlerFunc for the tile endpoint of the tileset.
// If a tile is not found, the handler returns a blank image if the tileset
// has images, and an empty response if the tileset has vector tiles.
func (ts *Tileset) tileHandler(w http.ResponseWriter, r *http.Request) {
	if ts == nil || !ts.published {
		// In order to not break any requests from when this tileset was published
		// return the appropriate not found handler for the original tile format.
		tileNotFoundHandler(w, r, ts.tileformat)
		return
	}

	// wait up to 30 seconds to see if tileset is ready and return it if possible
	if ts.isLockedWithTimeout(30 * time.Second) {
		tilesetLockedHandler(w, r)
		return
	}

	db := ts.db
	// split path components to extract tile coordinates x, y and z
	pcs := strings.Split(r.URL.Path[1:], "/")
	// we are expecting at least "services", <id> , "tiles", <z>, <x>, <y plus .ext>
	l := len(pcs)
	if l < 6 || pcs[5] == "" {
		http.Error(w, "requested path is too short", http.StatusBadRequest)
		return
	}
	z, x, y := pcs[l-3], pcs[l-2], pcs[l-1]
	tc, _, err := tileCoordFromString(z, x, y)
	if err != nil {
		http.Error(w, "invalid tile coordinates", http.StatusBadRequest)
		return
	}
	var data []byte
	// flip y to match the spec
	tc.y = (1 << uint64(tc.z)) - 1 - tc.y
	err = db.ReadTile(tc.z, tc.x, tc.y, &data)

	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		ts.svc.logError("cannot fetch tile from DB for z=%d, x=%d, y=%d at path %v: %v", tc.z, tc.x, tc.y, r.URL.Path, err)
		return
	}
	if data == nil || len(data) <= 1 {
		tileNotFoundHandler(w, r, ts.tileformat)
		return
	}

	w.Header().Set("Content-Type", db.GetTileFormat().MimeType())
	if db.GetTileFormat() == mbtiles.PBF {
		w.Header().Set("Content-Encoding", "gzip")
	}

	_, err = w.Write(data)

	if err != nil && !errors.Is(err, syscall.EPIPE) {
		ts.svc.logError("Could not write tile data for %v: %v", r.URL.Path, err)

	}
}

// previewHandler is an http.HandlerFunc that renders the map preview template
// appropriate for the type of tileset.  Image tilesets use Leaflet, whereas
// vector tilesets use Mapbox GL.
func (ts *Tileset) previewHandler(w http.ResponseWriter, r *http.Request) {
	if ts == nil || !ts.published {
		http.NotFound(w, r)
		return
	}

	// wait up to 30 seconds to see if tileset is ready and return it if possible
	if ts.isLockedWithTimeout(30 * time.Second) {
		tilesetLockedHandler(w, r)
		return
	}

	query := ""
	if r.URL.RawQuery != "" {
		query = "?" + r.URL.RawQuery
	}

	tilesetURL := fmt.Sprintf("%s://%s%s", scheme(r), getRequestHost(r), strings.TrimSuffix(r.URL.Path, "/map"))

	tileJSON, err := ts.TileJSON(tilesetURL, query)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		ts.svc.logError("could not create tileJSON content for %v: %v", r.URL.Path, err)
		return
	}

	bytes, err := json.Marshal(tileJSON)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		ts.svc.logError("could not render tileJSON for preview for %v: %v", r.URL.Path, err)
		return
	}

	p := struct {
		URL      string
		ID       string
		TileJSON template.JS
	}{
		tilesetURL,
		ts.id,
		template.JS(string(bytes)),
	}

	switch ts.db.GetTileFormat() {
	default:
		executeTemplate(w, "map", p)
	case mbtiles.PBF:
		executeTemplate(w, "map_gl", p)
	}
}

// tileNotFoundHandler is an http.HandlerFunc that writes the default response
// for a non-existing tile of type f to w
func tileNotFoundHandler(w http.ResponseWriter, r *http.Request, f mbtiles.TileFormat) {
	switch f {
	case mbtiles.PNG, mbtiles.JPG, mbtiles.WEBP:
		// Return blank PNG for all image types
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		w.Write(BlankPNG())
	case mbtiles.PBF:
		// Return 204
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message": "Tile does not exist"}`)
	}
}

// tilesetLockedHandler returns a 503 Service Unavailable response when
// requests are made to a tileset that is beging updated
func tilesetLockedHandler(w http.ResponseWriter, r *http.Request) {
	// send back service unavailable response with header to retry in 10 seconds
	w.Header().Set("Retry-After", "10")
	http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
}

func (ts *Tileset) isLockedWithTimeout(timeout time.Duration) bool {
	if ts == nil || !ts.locked {
		return false
	}

	timeoutReached := time.After(timeout)
	// poll locked status every 500 ms
	ticker := time.Tick(500 * time.Millisecond)
	for {
		select {
		case <-timeoutReached:
			return ts.locked
		case <-ticker:
			if !ts.locked {
				return false
			}
			// otherwise, still locked
		}
	}
}
