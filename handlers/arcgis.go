package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
)

type arcGISLOD struct {
	Level      int     `json:"level"`
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

// root of ArcGIS server is at /arcgis/rest/
// all map services are served under /arcgis/rest/services

const ArcGISRoot = "/arcgis/rest/"
const ArcGISServicesRoot = ArcGISRoot + "services/"
const ArcGISInfoRoot = ArcGISRoot + "info"

var webMercatorSR = arcGISSpatialReference{Wkid: 3857}
var geographicSR = arcGISSpatialReference{Wkid: 4326}

func arcgisInfoJSON() ([]byte, error) {
	out := map[string]interface{}{
		"currentVersion": 10.71,
		"fullVersion":    "10.7.1",
		"soapUrl":        nil,
		"secureSoapUrl":  nil,
	}

	bytes, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func (svc *ServiceSet) arcgisInfoHandler(w http.ResponseWriter, r *http.Request) {
	infoJSON, err := arcgisInfoJSON()

	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		svc.logError("Could not render ArcGIS Server Info JSON for %v: %v", r.URL.Path, err)
		return
	}

	err = wrapJSONP(w, r, infoJSON)

	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		svc.logError("Could not render ArcGIS Server Info JSON to JSONP for %v: %v", r.URL.Path, err)
	}
}

// arcGISServiceJSON returns ArcGIS standard JSON describing the ArcGIS
// tile service.
func (ts *Tileset) arcgisServiceJSON() ([]byte, error) {
	db := ts.db
	imgFormat := db.GetTileFormat().String()
	metadata, err := db.ReadMetadata()
	if err != nil {
		return nil, err
	}
	name, _ := metadata["name"].(string)
	description, _ := metadata["description"].(string)
	attribution, _ := metadata["attribution"].(string)
	tags, _ := metadata["tags"].(string)
	credits, _ := metadata["credits"].(string)

	// TODO: make sure that min and max zoom always populated
	minZoom, _ := metadata["minzoom"].(int)
	maxZoom, _ := metadata["maxzoom"].(int)
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

	bounds, ok := metadata["bounds"].([]float64)
	if !ok {
		bounds = []float64{-180, -85, 180, 85} // default to world bounds
	}
	extent := geoBoundsToWMExtent(bounds)

	tileInfo := map[string]interface{}{
		"rows": 256,
		"cols": 256,
		"dpi":  dpi,
		"origin": map[string]float64{
			"x": -earthCircumference,
			"y": earthCircumference,
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
		"id":                        ts.id,
		"name":                      name,
		"mapName":                   name,
		"capabilities":              "Map,TilesOnly",
		"description":               description,
		"serviceDescription":        description,
		"copyrightText":             attribution,
		"singleFusedMapCache":       true,
		"supportedImageFormatTypes": strings.ToUpper(imgFormat),
		"units":                     "esriMeters",
		// TODO: enable for vector tiles
		// "layers": []arcGISLayerStub{
		// 	{
		// 		ID:                0,
		// 		Name:              name,
		// 		ParentLayerID:     -1,
		// 		DefaultVisibility: true,
		// 		SubLayerIDs:       nil,
		// 		MinScale:          minScale,
		// 		MaxScale:          maxScale,
		// 	},
		// },
		"layers":                []string{},
		"tables":                []string{},
		"spatialReference":      webMercatorSR,
		"minScale":              minScale,
		"maxScale":              maxScale,
		"tileInfo":              tileInfo,
		"documentInfo":          documentInfo,
		"initialExtent":         extent,
		"fullExtent":            extent,
		"exportTilesAllowed":    false,
		"maxExportTilesCount":   0,
		"resampling":            false,
		"supportsDynamicLayers": false,
	}

	bytes, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

// arcgisServiceHandler is an http.HandlerFunc that returns standard ArcGIS
// JSON for a given ArcGIS tile service
func (ts *Tileset) arcgisServiceHandler(w http.ResponseWriter, r *http.Request) {
	svcJSON, err := ts.arcgisServiceJSON()

	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		ts.svc.logError("Could not render ArcGIS Service JSON for %v: %v", r.URL.Path, err)
		return
	}

	err = wrapJSONP(w, r, svcJSON)

	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		ts.svc.logError("Could not render ArcGIS Service JSON to JSONP for %v: %v", r.URL.Path, err)
	}
}

// arcGISLayersJSON returns JSON for the layers in a given ArcGIS tile service
func (ts *Tileset) arcgisLayersJSON() ([]byte, error) {
	// TODO: enable for vector tiles

	// metadata, err := ts.db.ReadMetadata()
	// if err != nil {
	// 	return nil, err
	// }

	// name, _ := metadata["name"].(string)
	// description, _ := metadata["description"].(string)
	// attribution, _ := metadata["attribution"].(string)

	// bounds, ok := metadata["bounds"].([]float32)
	// if !ok {
	// 	bounds = []float32{-180, -85, 180, 85} // default to world bounds
	// }
	// extent := geoBoundsToWMExtent(bounds)

	// minZoom, _ := metadata["minzoom"].(int)
	// maxZoom, _ := metadata["maxzoom"].(int)
	// minScale, _ := calcScaleResolution(minZoom, dpi)
	// maxScale, _ := calcScaleResolution(maxZoom, dpi)

	// // for now, just create a placeholder root layer
	// emptyArray := []interface{}{}
	// emptyLayerArray := []arcGISLayerStub{}

	// var layers [1]arcGISLayer
	// layers[0] = arcGISLayer{
	// 	ID:                0,
	// 	DefaultVisibility: true,
	// 	ParentLayer:       nil,
	// 	Name:              name,
	// 	Description:       description,
	// 	Extent:            extent,
	// 	MinScale:          minScale,
	// 	MaxScale:          maxScale,
	// 	CopyrightText:     attribution,
	// 	HTMLPopupType:     "esriServerHTMLPopupTypeAsHTMLText",
	// 	Fields:            emptyArray,
	// 	Relationships:     emptyArray,
	// 	SubLayers:         emptyLayerArray,
	// 	CurrentVersion:    10.4,
	// 	Capabilities:      "Map",
	// }

	out := map[string]interface{}{
		// "layers": layers,
		"layers": []string{},
		"tables": []string{},
	}

	bytes, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

// arcgisLayersHandler is an http.HandlerFunc that returns standard ArcGIS
// Layers JSON for a given ArcGIS tile service
func (ts *Tileset) arcgisLayersHandler(w http.ResponseWriter, r *http.Request) {
	layersJSON, err := ts.arcgisLayersJSON()
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		ts.svc.logError("Could not render ArcGIS layer JSON for %v: %v", r.URL.Path, err)
		return
	}

	err = wrapJSONP(w, r, layersJSON)

	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		ts.svc.logError("Could not render ArcGIS layers JSON to JSONP for %v: %v", r.URL.Path, err)
	}
}

// arcgisLegendJSON returns minimal ArcGIS legend JSON for a given ArcGIS
// tile service.  Legend elements are not yet supported.
func (ts *Tileset) arcgisLegendJSON() ([]byte, error) {
	metadata, err := ts.db.ReadMetadata()
	if err != nil {
		return nil, err
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
		return nil, err
	}
	return bytes, nil
}

// arcgisLegendHandler is an http.HandlerFunc that returns minimal ArcGIS
// legend JSON for a given ArcGIS tile service
func (ts *Tileset) arcgisLegendHandler(w http.ResponseWriter, r *http.Request) {
	legendJSON, err := ts.arcgisLegendJSON()

	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		ts.svc.logError("Could not render ArcGIS legend JSON for %v: %v", r.URL.Path, err)
		return
	}

	err = wrapJSONP(w, r, legendJSON)

	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		ts.svc.logError("Could not render ArcGIS legend JSON to JSONP for %v: %v", r.URL.Path, err)
	}
}

// arcgisTileHandler returns an image tile or blank image for a given
// tile request within a given ArcGIS tile service
func (ts *Tileset) arcgisTileHandler(w http.ResponseWriter, r *http.Request) {
	db := ts.db

	// split path components to extract tile coordinates x, y and z
	pcs := strings.Split(r.URL.Path[1:], "/")
	// strip off /arcgis/rest/services/ and then
	// we should have at least <id> , "MapServer", "tiles", <z>, <y>, <x>
	l := len(pcs)
	if l < 6 || pcs[5] == "" {
		http.Error(w, "requested path is too short", http.StatusBadRequest)
		return
	}
	z, y, x := pcs[l-3], pcs[l-2], pcs[l-1]
	tc, _, err := tileCoordFromString(z, x, y)
	if err != nil {
		http.Error(w, "invalid tile coordinates", http.StatusBadRequest)
		return
	}

	// flip y to match the spec
	tc.y = (1 << uint64(tc.z)) - 1 - tc.y

	var data []byte
	err = db.ReadTile(tc.z, tc.x, tc.y, &data)

	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		ts.svc.logError("cannot fetch tile from DB for z=%d, x=%d, y=%d for %v: %v", tc.z, tc.x, tc.y, r.URL.Path, err)
		return
	}

	if data == nil || len(data) <= 1 {
		// Return blank PNG for all image types
		w.Header().Set("Content-Type", "image/png")
		_, err = w.Write(BlankPNG())

		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			ts.svc.logError("could not return blank image for %v: %v", r.URL.Path, err)
		}
	} else {
		w.Header().Set("Content-Type", db.GetTileFormat().MimeType())
		_, err = w.Write(data)

		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			ts.svc.logError("could not write tile data to response for %v: %v", r.URL.Path, err)
		}
	}
}

func geoBoundsToWMExtent(bounds []float64) arcGISExtent {
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
