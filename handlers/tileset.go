package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/consbio/mbtileserver/mbtiles"
)

// Tileset provides a tileset constructed from an mbtiles file
type Tileset struct {
	db  *mbtiles.DB
	id  string
	svc *ServiceSet
}

// NewTileset constructs a new Tileset from an mbtiles filename.
// Any errors encountered opening the tileset are returned.
func NewTileset(filename string, id string) (*Tileset, error) {
	db, err := mbtiles.NewDB(filename)
	if err != nil {
		return nil, fmt.Errorf("Invalid mbtiles file %q: %v", filename, err)
	}

	return &Tileset{
		db: db,
		id: id,
	}, nil
}

// Reload reloads the mbtiles file from disk using the same filename as
// used when this was first constructed
func (ts *Tileset) Reload() error {
	if ts.db == nil {
		return nil
	}

	filename := ts.db.Filename()

	err := ts.db.Close()
	if err != nil {
		return err
	}

	db, err := mbtiles.NewDB(filename)
	if err != nil {
		return fmt.Errorf("Invalid mbtiles file %q: %v", filename, err)
	}
	ts.db = db

	return nil
}

// Delete closes and deletes the mbtiles file for this tileset
func (ts *Tileset) Delete() error {
	if ts.db != nil {

		err := ts.db.Close()
		if err != nil {
			return err
		}
	}
	ts.db = nil

	return nil
}

func (ts *Tileset) tileFormatString() string {
	return ts.db.TileFormatString()
}

func (ts *Tileset) tileJSONHandler(enablePreview bool) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {

		query := ""
		if r.URL.RawQuery != "" {
			query = "?" + r.URL.RawQuery
		}

		tilesetURL := fmt.Sprintf("%s://%s%s", scheme(r), r.Host, r.URL.Path)

		tileJSON, err := ts.TileJSON(tilesetURL, query)
		if enablePreview {
			tileJSON["map"] = fmt.Sprintf("%s/map", tilesetURL)
		}

		bytes, err := json.Marshal(tileJSON)
		if err != nil {
			return http.StatusInternalServerError, fmt.Errorf("could not render TileJSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(bytes)
		return http.StatusOK, err
	}
}

func (ts *Tileset) TileJSON(svcURL string, query string) (map[string]interface{}, error) {
	db := ts.db

	imgFormat := db.TileFormatString()
	out := map[string]interface{}{
		"tilejson": "2.1.0",
		"id":       ts.id,
		"scheme":   "xyz",
		"format":   imgFormat,
		"tiles":    []string{fmt.Sprintf("%s/tiles/{z}/{x}/{y}.%s%s", svcURL, imgFormat, query)},
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

	if db.HasUTFGrid() {
		out["grids"] = []string{fmt.Sprintf("%s/tiles/{z}/{x}/{y}.json%s", svcURL, query)}
	}
	return out, nil
}

func (ts *Tileset) tileHandler() handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		db := ts.db
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

func (ts *Tileset) previewHandler() handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {

		query := ""
		if r.URL.RawQuery != "" {
			query = "?" + r.URL.RawQuery
		}

		tilesetURL := fmt.Sprintf("%s://%s%s", scheme(r), r.Host, strings.TrimSuffix(r.URL.Path, "/map"))

		tileJSON, err := ts.TileJSON(tilesetURL, query)
		bytes, err := json.Marshal(tileJSON)
		if err != nil {
			return http.StatusInternalServerError, fmt.Errorf("could not render preview: %v", err)
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

		switch ts.db.TileFormat() {
		default:
			return executeTemplate(w, "map", p)
		case mbtiles.PBF:
			return executeTemplate(w, "map_gl", p)
		}
	}
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
