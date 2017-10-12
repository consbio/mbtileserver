package main

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/golang/groupcache"
	"github.com/labstack/echo"
)

type ArcGISLOD struct {
	Level      int     `json:"level"`
	Resolution float64 `json:"resolution"`
	Scale      float64 `json:"scale"`
}

type ArcGISSpatialReference struct {
	Wkid uint16 `json:"wkid"`
}

type ArcGISExtent struct {
	Xmin             float64                `json:"xmin"`
	Ymin             float64                `json:"ymin"`
	Xmax             float64                `json:"xmax"`
	Ymax             float64                `json:"ymax"`
	SpatialReference ArcGISSpatialReference `json:"spatialReference"`
}

type ArcGISLayerStub struct {
	Id                uint8   `json:"id"`
	Name              string  `json:"name"`
	ParentLayerId     int16   `json:"parentLayerId"`
	DefaultVisibility bool    `json:"defaultVisibility"`
	SubLayerIds       []uint8 `json:"subLayerIds"`
	MinScale          float64 `json:"minScale"`
	MaxScale          float64 `json:"maxScale"`
}

type ArcGISLayer struct {
	Id                uint8             `json:"id"`
	Name              string            `json:"name"`
	Type              string            `json:"type"`
	Description       string            `json:"description"`
	GeometryType      string            `json:"geometryType"`
	CopyrightText     string            `json:"copyrightText"`
	ParentLayer       interface{}       `json:"parentLayer"`
	SubLayers         []ArcGISLayerStub `json:"subLayers"`
	MinScale          float64           `json:"minScale"`
	MaxScale          float64           `json:"maxScale"`
	DefaultVisibility bool              `json:"defaultVisibility"`
	Extent            ArcGISExtent      `json:"extent"`
	HasAttachments    bool              `json:"hasAttachments"`
	HtmlPopupType     string            `json:"htmlPopupType"`
	DrawingInfo       interface{}       `json:"drawingInfo"`
	DisplayField      interface{}       `json:"displayField"`
	Fields            []interface{}     `json:"fields"`
	TypeIdField       interface{}       `json:"typeIdField"`
	Types             interface{}       `json:"types"`
	Relationships     []interface{}     `json:"relationships"`
	Capabilities      string            `json:"capabilities"`
	CurrentVersion    float32           `json:"currentVersion"`
}

var WebMercatorSR = ArcGISSpatialReference{Wkid: 3857}
var GeographicSR = ArcGISSpatialReference{Wkid: 4326}

func jsonOrJsonP(c echo.Context, out map[string]interface{}) error {
	callback := c.QueryParam("callback")
	if callback != "" {
		return c.JSONP(http.StatusOK, callback, out)
	}
	return c.JSON(http.StatusOK, out)
}

func GetArcGISService(c echo.Context) error {
	id, err := getServiceOr404(c)
	if err != nil {
		return err
	}

	tileset := tilesets[id]
	imgFormat := TileFormatStr[tileset.tileformat]
	metadata, err := tileset.ReadMetadata()
	if err != nil {
		log.Errorf("Could not read metadata for tileset %v", id)
		return err
	}
	name := toString(metadata["name"])
	description := toString(metadata["description"])
	attribution := toString(metadata["attribution"])

	// TODO: make sure that min and max zoom always populated
	minZoom := metadata["minzoom"].(int)
	maxZoom := metadata["maxzoom"].(int)
	dpi := 96 // TODO: extract dpi from the image instead
	var lods []ArcGISLOD
	for i := minZoom; i <= maxZoom; i++ {
		scale, resolution := calcScaleResolution(i, dpi)
		lods = append(lods, ArcGISLOD{
			Level:      i,
			Resolution: resolution,
			Scale:      scale,
		})
	}

	minScale := lods[0].Scale
	maxScale := lods[len(lods)-1].Scale

	bounds := metadata["bounds"].([]float32) // TODO: make sure this is always present
	extent := geoBoundsToWMExtent(bounds)

	tileInfo := map[string]interface{}{
		"rows": 256,
		"cols": 256,
		"dpi":  dpi,
		"origin": map[string]float32{
			"x": -20037508.342787,
			"y": 20037508.342787,
		},
		"spatialReference": WebMercatorSR,
		"lods":             lods,
	}

	documentInfo := map[string]string{
		"Title":    name,
		"Author":   attribution,
		"Comments": "",
		"Subject":  "",
		"Category": "",
		"Keywords": toString(metadata["tags"]),
		"Credits":  toString(metadata["credits"]),
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
		"layers": []ArcGISLayerStub{
			ArcGISLayerStub{
				Id:                0,
				Name:              name,
				ParentLayerId:     -1,
				DefaultVisibility: true,
				SubLayerIds:       nil,
				MinScale:          minScale,
				MaxScale:          maxScale,
			},
		},
		"tables":              []string{},
		"spatialReference":    WebMercatorSR,
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

	return jsonOrJsonP(c, out)
}

func GetArcGISServiceLayers(c echo.Context) error {
	id, err := getServiceOr404(c)
	if err != nil {
		return err
	}

	tileset := tilesets[id]
	metadata, err := tileset.ReadMetadata()
	if err != nil {
		log.Errorf("Could not read metadata for tileset %v", id)
		return err
	}

	bounds := metadata["bounds"].([]float32) // TODO: make sure this is always present
	extent := geoBoundsToWMExtent(bounds)

	minZoom := metadata["minzoom"].(int)
	maxZoom := metadata["maxzoom"].(int)
	minScale, _ := calcScaleResolution(minZoom, 96)
	maxScale, _ := calcScaleResolution(maxZoom, 96)

	// for now, just create a placeholder root layer
	emptyArray := []interface{}{}
	emptyLayerArray := []ArcGISLayerStub{}

	var layers [1]ArcGISLayer
	layers[0] = ArcGISLayer{
		Id:                0,
		DefaultVisibility: true,
		ParentLayer:       nil,
		Name:              toString(metadata["name"]),
		Description:       toString(metadata["description"]),
		Extent:            extent,
		MinScale:          minScale,
		MaxScale:          maxScale,
		CopyrightText:     toString(metadata["attribution"]),
		HtmlPopupType:     "esriServerHTMLPopupTypeAsHTMLText",
		Fields:            emptyArray,
		Relationships:     emptyArray,
		SubLayers:         emptyLayerArray,
		CurrentVersion:    10.4,
		Capabilities:      "Map",
	}

	out := map[string]interface{}{
		"layers": layers,
	}

	return jsonOrJsonP(c, out)
}

func GetArcGISServiceLegend(c echo.Context) error {
	id, err := getServiceOr404(c)
	if err != nil {
		return err
	}

	tileset := tilesets[id]
	metadata, err := tileset.ReadMetadata()
	if err != nil {
		log.Errorf("Could not read metadata for tileset %v", id)
		return err
	}

	// TODO: pull the legend from ArcGIS specific metadata tables
	var elements [0]interface{}
	var layers [1]map[string]interface{}

	layers[0] = map[string]interface{}{
		"layerId":   0,
		"layerName": toString(metadata["name"]),
		"layerType": "",
		"minScale":  0,
		"maxScale":  0,
		"legend":    elements,
	}

	out := map[string]interface{}{
		"layers": layers,
	}

	return jsonOrJsonP(c, out)
}

func GetArcGISTile(c echo.Context) error {
	var (
		data        []byte
		contentType string
	)
	//TODO: validate x, y, z

	id, err := getServiceOr404(c)
	if err != nil {
		return err
	}

	key := strings.Join([]string{id, "tile", c.Param("z"), c.Param("x"), c.Param("y")}, "|")

	err = cache.Get(nil, key, groupcache.AllocatingByteSliceSink(&data))
	if err != nil {
		log.Println("Error fetching key", key)
		// TODO: log
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Cache get failed for key: %s", key))
	}

	tileset := tilesets[id]

	if len(data) <= 1 {
		if tileset.tileformat == PBF {
			// If pbf, return 404 w/ json, consistent w/ mapbox
			return c.JSON(http.StatusNotFound, struct {
				Message string `json:"message"`
			}{"Tile does not exist"})
		}

		data = blankPNG
		contentType = "image/png"
	} else {
		contentType = TileContentType[tileset.tileformat]
	}

	res := c.Response()
	res.Header().Add("Content-Type", contentType)

	if tileset.tileformat == PBF {
		res.Header().Add("Content-Encoding", "gzip")
	}

	res.WriteHeader(http.StatusOK)
	_, err = io.Copy(res, bytes.NewReader(data))
	return err
}

func geoBoundsToWMExtent(bounds []float32) ArcGISExtent {
	xmin, ymin := geoToMercator(float64(bounds[0]), float64(bounds[1]))
	xmax, ymax := geoToMercator(float64(bounds[2]), float64(bounds[3]))
	return ArcGISExtent{
		Xmin:             xmin,
		Ymin:             ymin,
		Xmax:             xmax,
		Ymax:             ymax,
		SpatialReference: WebMercatorSR,
	}
}

func calcScaleResolution(zoomLevel int, dpi int) (float64, float64) {
	resolution := 156543.033928 / math.Pow(2, float64(zoomLevel))
	scale := float64(dpi) * 39.37 * resolution // 39.37 in/m
	return scale, resolution
}
