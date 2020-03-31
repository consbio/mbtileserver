package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/asdawn/mbtileserver/mbtiles"
)

type arcGISLOD struct {
	Level      uint8   `json:"level"`
	Resolution float64 `json:"resolution"`
	Scale      float64 `json:"scale"`
}

type arcGISSpatialReference struct {
	Wkid uint16 `json:"wkid"`
}

type arcGISExtent struct {
	Xmin             float64                `json:"xmin"`
	Ymin             float64                `json:"ymin"`
	Xmax             float64                `json:"xmax"`
	Ymax             float64                `json:"ymax"`
	SpatialReference arcGISSpatialReference `json:"spatialReference"`
}

type arcGISLayerStub struct {
	ID                uint8   `json:"id"`
	Name              string  `json:"name"`
	ParentLayerID     int16   `json:"parentLayerId"`
	DefaultVisibility bool    `json:"defaultVisibility"`
	SubLayerIDs       []uint8 `json:"subLayerIds"`
	MinScale          float64 `json:"minScale"`
	MaxScale          float64 `json:"maxScale"`
}

type arcGISLayer struct {
	ID                uint8             `json:"id"`
	Name              string            `json:"name"`
	Type              string            `json:"type"`
	Description       string            `json:"description"`
	GeometryType      string            `json:"geometryType"`
	CopyrightText     string            `json:"copyrightText"`
	ParentLayer       interface{}       `json:"parentLayer"`
	SubLayers         []arcGISLayerStub `json:"subLayers"`
	MinScale          float64           `json:"minScale"`
	MaxScale          float64           `json:"maxScale"`
	DefaultVisibility bool              `json:"defaultVisibility"`
	Extent            arcGISExtent      `json:"extent"`
	HasAttachments    bool              `json:"hasAttachments"`
	HTMLPopupType     string            `json:"htmlPopupType"`
	DrawingInfo       interface{}       `json:"drawingInfo"`
	DisplayField      interface{}       `json:"displayField"`
	Fields            []interface{}     `json:"fields"`
	TypeIDField       interface{}       `json:"typeIdField"`
	Types             interface{}       `json:"types"`
	Relationships     []interface{}     `json:"relationships"`
	Capabilities      string            `json:"capabilities"`
	CurrentVersion    float32           `json:"currentVersion"`
}

const (
	earthRadius              = 6378137.0
	earthCircumference       = math.Pi * earthRadius
	initialResolution        = 2 * earthCircumference / 256
	dpi                uint8 = 96
)

var webMercatorSR = arcGISSpatialReference{Wkid: 3857}
var geographicSR = arcGISSpatialReference{Wkid: 4326}

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

func (s *ServiceSet) arcgisService(id string, db *mbtiles.DB) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		imgFormat := db.TileFormatString()
		metadata, err := db.ReadMetadata()
		if err != nil {
			return http.StatusInternalServerError, fmt.Errorf("Could not read metadata for tileset %v", id)
		}
		name, _ := metadata["name"].(string)
		description, _ := metadata["description"].(string)
		attribution, _ := metadata["attribution"].(string)
		tags, _ := metadata["tags"].(string)
		credits, _ := metadata["credits"].(string)

		// TODO: make sure that min and max zoom always populated
		minZoom, _ := metadata["minzoom"].(uint8)
		maxZoom, _ := metadata["maxzoom"].(uint8)
		// TODO: extract dpi from the image instead
		var lods []arcGISLOD
		for i := minZoom; i <= maxZoom; i++ {
			scale, resolution := calcScaleResolution(i, dpi)
			lods = append(lods, arcGISLOD{
				Level:      i,
				Resolution: resolution,
				Scale:      scale,
			})
		}

		minScale := lods[0].Scale
		maxScale := lods[len(lods)-1].Scale

		bounds, ok := metadata["bounds"].([]float32)
		if !ok {
			bounds = []float32{-180, -85, 180, 85} // default to world bounds
		}
		extent := geoBoundsToWMExtent(bounds)

		tileInfo := map[string]interface{}{
			"rows": 256,
			"cols": 256,
			"dpi":  dpi,
			"origin": map[string]float32{
				"x": -20037508.342787,
				"y": 20037508.342787,
			},
			"spatialReference": webMercatorSR,
			"lods":             lods,
		}

		documentInfo := map[string]string{
			"Title":    name,
			"Author":   attribution,
			"Comments": "",
			"Subject":  "",
			"Category": "",
			"Keywords": tags,
			"Credits":  credits,
		}

		out := map[string]interface{}{
			"currentVersion":            "10.4",
			"id":                        id,
			"name":                      name,
			"mapName":                   name,
			"capabilities":              "Map,TilesOnly",
			"description":               description,
			"serviceDescription":        description,
			"copyrightText":             attribution,
			"singleFusedMapCache":       true,
			"supportedImageFormatTypes": strings.ToUpper(imgFormat),
			"units":                     "esriMeters",
			"layers": []arcGISLayerStub{
				{
					ID:                0,
					Name:              name,
					ParentLayerID:     -1,
					DefaultVisibility: true,
					SubLayerIDs:       nil,
					MinScale:          minScale,
					MaxScale:          maxScale,
				},
			},
			"tables":              []string{},
			"spatialReference":    webMercatorSR,
			"minScale":            minScale,
			"maxScale":            maxScale,
			"tileInfo":            tileInfo,
			"documentInfo":        documentInfo,
			"initialExtent":       extent,
			"fullExtent":          extent,
			"exportTilesAllowed":  false,
			"maxExportTilesCount": 0,
			"resampling":          false,
		}

		bytes, err := json.Marshal(out)
		if err != nil {
			return http.StatusInternalServerError, fmt.Errorf("cannot marshal ArcGIS service info JSON: %v", err)
		}

		return http.StatusOK, wrapJSONP(w, r, bytes)
	}
}

func (s *ServiceSet) arcgisLayers(id string, db *mbtiles.DB) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		metadata, err := db.ReadMetadata()
		if err != nil {
			return http.StatusInternalServerError, fmt.Errorf("Could not read metadata for tileset %v", id)
		}

		name, _ := metadata["name"].(string)
		description, _ := metadata["description"].(string)
		attribution, _ := metadata["attribution"].(string)

		bounds, ok := metadata["bounds"].([]float32)
		if !ok {
			bounds = []float32{-180, -85, 180, 85} // default to world bounds
		}
		extent := geoBoundsToWMExtent(bounds)

		minZoom, _ := metadata["minzoom"].(uint8)
		maxZoom, _ := metadata["maxzoom"].(uint8)
		minScale, _ := calcScaleResolution(minZoom, dpi)
		maxScale, _ := calcScaleResolution(maxZoom, dpi)

		// for now, just create a placeholder root layer
		emptyArray := []interface{}{}
		emptyLayerArray := []arcGISLayerStub{}

		var layers [1]arcGISLayer
		layers[0] = arcGISLayer{
			ID:                0,
			DefaultVisibility: true,
			ParentLayer:       nil,
			Name:              name,
			Description:       description,
			Extent:            extent,
			MinScale:          minScale,
			MaxScale:          maxScale,
			CopyrightText:     attribution,
			HTMLPopupType:     "esriServerHTMLPopupTypeAsHTMLText",
			Fields:            emptyArray,
			Relationships:     emptyArray,
			SubLayers:         emptyLayerArray,
			CurrentVersion:    10.4,
			Capabilities:      "Map",
		}

		out := map[string]interface{}{
			"layers": layers,
		}

		bytes, err := json.Marshal(out)
		if err != nil {
			return http.StatusInternalServerError, fmt.Errorf("cannot marshal ArcGIS layer info JSON: %v", err)
		}

		return http.StatusOK, wrapJSONP(w, r, bytes)
	}
}

func (s *ServiceSet) arcgisLegend(id string, db *mbtiles.DB) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {

		metadata, err := db.ReadMetadata()
		if err != nil {
			return http.StatusInternalServerError, fmt.Errorf("Could not read metadata for tileset %v", id)
		}

		name, _ := metadata["name"].(string)

		// TODO: pull the legend from ArcGIS specific metadata tables
		var elements [0]interface{}
		var layers [1]map[string]interface{}

		layers[0] = map[string]interface{}{
			"layerId":   0,
			"layerName": name,
			"layerType": "",
			"minScale":  0,
			"maxScale":  0,
			"legend":    elements,
		}

		out := map[string]interface{}{
			"layers": layers,
		}

		bytes, err := json.Marshal(out)
		if err != nil {
			return http.StatusInternalServerError, fmt.Errorf("cannot marshal ArcGIS legend info JSON: %v", err)
		}
		return http.StatusOK, wrapJSONP(w, r, bytes)
	}
}

func (s *ServiceSet) arcgisTiles(db *mbtiles.DB) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		// split path components to extract tile coordinates x, y and z
		pcs := strings.Split(r.URL.Path[1:], "/")
		// strip off /arcgis/rest/services/ and then
		// we should have at least <id> , "MapServer", "tiles", <z>, <y>, <x>
		l := len(pcs)
		if l < 6 || pcs[5] == "" {
			return http.StatusBadRequest, fmt.Errorf("requested path is too short")
		}
		z, y, x := pcs[l-3], pcs[l-2], pcs[l-1]
		tc, _, err := tileCoordFromString(z, x, y)
		if err != nil {
			return http.StatusBadRequest, err
		}

		var data []byte
		err = db.ReadTile(tc.z, tc.x, tc.y, &data)

		if err != nil {
			// augment error info
			t := "tile"
			err = fmt.Errorf("cannot fetch %s from DB for z=%d, x=%d, y=%d: %v", t, tc.z, tc.x, tc.y, err)
			return http.StatusInternalServerError, err
		}

		if data == nil || len(data) <= 1 {
			// Return blank PNG for all image types
			w.Header().Set("Content-Type", "image/png")
			_, err = w.Write(BlankPNG())
		} else {
			w.Header().Set("Content-Type", db.ContentType())
			_, err = w.Write(data)
		}

		return http.StatusOK, err
	}
}

func geoBoundsToWMExtent(bounds []float32) arcGISExtent {
	xmin, ymin := geoToMercator(float64(bounds[0]), float64(bounds[1]))
	xmax, ymax := geoToMercator(float64(bounds[2]), float64(bounds[3]))
	return arcGISExtent{
		Xmin:             xmin,
		Ymin:             ymin,
		Xmax:             xmax,
		Ymax:             ymax,
		SpatialReference: webMercatorSR,
	}
}

func calcScaleResolution(zoomLevel uint8, dpi uint8) (float64, float64) {
	var denom = 1 << zoomLevel
	resolution := initialResolution / float64(denom)
	scale := float64(dpi) * 39.37 * resolution // 39.37 in/m
	return scale, resolution
}

// Cast interface to a string if not nil, otherwise empty string
func toString(s interface{}) string {
	if s != nil {
		return s.(string)
	}
	return ""
}

// Convert a latitude and longitude to mercator coordinates, bounded to world domain.
func geoToMercator(longitude, latitude float64) (float64, float64) {
	// bound to world coordinates
	if latitude > 85 {
		latitude = 85
	} else if latitude < -85 {
		latitude = -85
	}

	x := longitude * earthCircumference / 180
	y := math.Log(math.Tan((90+latitude)*math.Pi/360)) / (math.Pi / 180) * (earthCircumference / 180)

	return x, y
}

// ArcGISHandler returns a http.Handler that serves the ArcGIS endpoints of the ServiceSet.
// The function ef is called with any occurring error if it is non-nil, so it
// can be used for e.g. logging with logging facilities of the caller.
func (s *ServiceSet) ArcGISHandler(ef func(error)) http.Handler {
	m := http.NewServeMux()
	rootPath := "/arcgis/rest/services/"

	for id, db := range s.tilesets {
		p := rootPath + id + "/MapServer"
		m.Handle(p, wrapGetWithErrors(ef, hmacAuth(s.arcgisService(id, db), s.secretKey, id)))
		m.Handle(p+"/layers", wrapGetWithErrors(ef, hmacAuth(s.arcgisLayers(id, db), s.secretKey, id)))
		m.Handle(p+"/legend", wrapGetWithErrors(ef, hmacAuth(s.arcgisLegend(id, db), s.secretKey, id)))

		m.Handle(p+"/tile/", wrapGetWithErrors(ef, hmacAuth(s.arcgisTiles(db), s.secretKey, id)))
	}
	return m
}
