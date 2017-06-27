package main

import (
	"bytes"
	"fmt"

	"github.com/golang/groupcache"
	"github.com/labstack/echo"
	"github.com/labstack/echo/engine"
	// "github.com/labstack/echo/engine/fasthttp"

	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/evalphobia/logrus_sentry"
	"github.com/labstack/echo/engine/standard"
	"github.com/labstack/echo/middleware"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
)

//var ContentTypes = map[string]string{
//"png": "image/png",
//"jpg": "image/jpeg",
//"pbf": "application/x-protobuf", // Content-Encoding header must be gzip
//}

type ServiceInfo struct {
	ImageType string `json:"imageType"`
	URL       string `json:"url"`
}

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

type TemplateParams struct {
	URL string
	ID  string
}

var (
	blankPNG    []byte
	cache       *groupcache.Group
	tilesets    map[string]Mbtiles
	startuptime = time.Now()
)

var RootCmd = &cobra.Command{
	Use:   "mbtileserver",
	Short: "Serve tiles from mbtiles files",
	Run: func(cmd *cobra.Command, args []string) {
		serve()
	},
}

var (
	port        int
	tilePath    string
	cacheSize   int64
	certificate string
	privateKey  string
	sentry_DSN  string
	verbose     bool
)

func init() {
	flags := RootCmd.Flags()
	flags.IntVarP(&port, "port", "p", 8000, "Server port.")
	flags.StringVarP(&tilePath, "dir", "d", "./tilesets", "Directory containing mbtiles files.")
	flags.StringVarP(&certificate, "cert", "c", "", "X.509 TLS certificate filename.  If present, will be used to enable SSL on the server.")
	flags.StringVarP(&privateKey, "key", "k", "", "TLS private key")
	flags.Int64Var(&cacheSize, "cachesize", 250, "Size of cache in MB.")
	flags.StringVar(&sentry_DSN, "dsn", "", "Sentry DSN")
	flags.BoolVarP(&verbose, "verbose", "v", false, "Verbose logging")
}

func main() {
	if err := RootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}

func serve() {
	if verbose {
		log.SetLevel(log.DebugLevel)
	}

	if len(sentry_DSN) > 0 {
		hook, err := logrus_sentry.NewSentryHook(sentry_DSN, []log.Level{
			log.PanicLevel,
			log.FatalLevel,
			log.ErrorLevel,
			log.WarnLevel,
		})
		if err != nil {
			log.Fatalln(err)
		}
		hook.Timeout = 30 * time.Second // allow up to 30 seconds for Sentry to respond
		log.AddHook(hook)
		log.Debugln("Added logging hook for Sentry")
	}

	certExists := len(certificate) > 0
	keyExists := len(privateKey) > 0

	if (certExists || keyExists) && !(certExists && keyExists) {
		log.Fatalln("Both certificate and private key are required to use SSL")
	}

	blankPNG, _ = ioutil.ReadFile("blank.png") // Cache the blank PNG in memory (it is tiny)

	// Must manage these in main, based on how we are deferring closing of connections to DB
	// TODO: clean that up
	tilesets = make(map[string]Mbtiles)
	filenames, _ := filepath.Glob(path.Join(tilePath, "*.mbtiles"))

	if len(filenames) == 0 {
		log.Fatal("No tilesets found in tileset directory")
	}

	log.Infof("Found %v mbtiles files in %s\n", len(filenames), tilePath)

	for _, filename := range filenames {
		_, id := filepath.Split(filename)
		id = strings.Split(id, ".")[0]
		tileset, err := NewMbtiles(filename)
		if err != nil {
			log.Errorf("could not open mbtiles file: %s\n%v", filename, err)
			continue
		}
		tilesets[id] = *tileset
	}

	log.Debugf("Cache size: %v MB\n", cacheSize)

	cache = groupcache.NewGroup("TileCache", cacheSize*1048576, groupcache.GetterFunc(cacheGetter))

	e := echo.New()
	e.Pre(middleware.RemoveTrailingSlash())
	if verbose {
		e.Use(middleware.Logger())
	}
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	t := &Template{
		templates: template.Must(template.ParseGlob("templates/*.html")),
	}
	e.SetRenderer(t)

	gzip := middleware.Gzip()

	// Setup routing
	e.File("/favicon.ico", "favicon.ico")
	e.File("/favicon.png", "favicon.png")

	// TODO: can use more caching here
	e.Group("/static/", gzip, middleware.Static("templates/static/dist/"))

	e.GET("/services", ListServices, NotModifiedMiddleware, gzip)

	services := e.Group("/services/") // has to be separate from endpoint for ListServices
	services.GET(":id", GetServiceInfo, NotModifiedMiddleware, gzip)
	services.GET(":id/map", GetServiceHTML, NotModifiedMiddleware, gzip)
	services.Get(":id/tiles/:z/:x/:y/grid.json", GetGrid, NotModifiedMiddleware)
	services.Get(":id/tiles/:z/:x/:filename", GetTile, NotModifiedMiddleware)

	arcgis := e.Group("/arcgis/rest/")
	// arcgis.GET("services", GetArcGISServices, NotModifiedMiddleware, gzip)
	arcgis.GET("services/:id/MapServer", GetArcGISService, NotModifiedMiddleware, gzip)
	arcgis.GET("services/:id/MapServer/layers", GetArcGISServiceLayers, NotModifiedMiddleware, gzip)
	arcgis.GET("services/:id/MapServer/legend", GetArcGISServiceLegend, NotModifiedMiddleware, gzip)
	arcgis.Get("services/:id/MapServer/tile/:z/:y/:x", GetArcGISTile, NotModifiedMiddleware)

	e.Get("/admin/cache", CacheInfo, gzip)

	config := engine.Config{
		Address: fmt.Sprintf(":%v", port),
	}

	// Start the server
	if certExists {
		if _, err := os.Stat(certificate); os.IsNotExist(err) {
			log.Fatalf("Could not find certificate file: %s\n", certificate)
		}
		if _, err := os.Stat(privateKey); os.IsNotExist(err) {
			log.Fatalf("Could not find private key file: %s\n", privateKey)
		}

		config.TLSCertFile = certificate
		config.TLSKeyFile = privateKey
		fmt.Printf("Starting HTTPS server on port %v\n", port)
		fmt.Println("Use Ctrl-C to exit the server")
		e.Run(standard.WithConfig(config))

	} else {
		// TODO: use fasthttp engine, but beware issues with path (differs from standard)

		fmt.Println("\n--------------------------------------")
		fmt.Printf("Starting HTTP server on port %v\n", port)
		fmt.Println("Use Ctrl-C to exit the server")
		fmt.Println("--------------------------------------\n")
		e.Run(standard.WithConfig(config))
	}

}

func cacheGetter(ctx groupcache.Context, key string, dest groupcache.Sink) error {
	pathParams := strings.Split(key, "/")
	id := pathParams[0]
	tileType := pathParams[1]
	z64, _ := strconv.ParseUint(pathParams[2], 0, 8)
	z := uint8(z64)
	x, _ := strconv.ParseUint(pathParams[3], 0, 64)
	y, _ := strconv.ParseUint(pathParams[4], 0, 64)
	//flip y to match the spec
	y = (1 << z) - 1 - y

	// TODO: if y is a very large number, e.g., 18446744073709551615, then it is an overflow and not a valid y value

	var data []byte
	tileset := tilesets[id]

	if tileType == "tile" {
		err := tileset.ReadTile(z, x, y, &data)
		if err != nil {
			log.Errorf("Error encountered reading tile for z=%v, x=%v, y=%v, \n%v", z, x, y, err)
			return err
		}
	} else if tileType == "grid" && tileset.utfgridConfig != nil {
		err := tileset.ReadGrid(z, x, y, &data)
		if err != nil {
			log.Errorf("Error encountered reading grid for z=%v, x=%v, y=%v, \n%v", z, x, y, err)
			return err
		}
	}

	dest.SetBytes(data)
	return nil
}

// Verifies that service exists and return 404 otherwise
func getServiceOr404(c echo.Context) (string, error) {
	id := c.Param("id")
	if _, exists := tilesets[id]; !exists {
		log.Warnf("Service not found: %s\n", id)
		return "", echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("Service not found: %s", id))
	}
	return id, nil
}

func getRootURL(c echo.Context) string {
	// TODO: this won't be correct if we received this via proxy
	return fmt.Sprintf("%s://%s", c.Request().Scheme(), c.Request().Host())
}

func ListServices(c echo.Context) error {
	// TODO: need to paginate the responses
	rootURL := fmt.Sprintf("%s%s", getRootURL(c), c.Request().URL())
	services := make([]ServiceInfo, len(tilesets))
	i := 0
	for id, tileset := range tilesets {
		services[i] = ServiceInfo{
			ImageType: TileFormatStr[tileset.tileformat],
			URL:       fmt.Sprintf("%s/%s", rootURL, id),
		}
		i++
	}
	return c.JSON(http.StatusOK, services)
}

//TODO: separate out tileJSON render into a separate function
//then it can be directly injected into template HTML instead of URL, and bypass one request
func GetServiceInfo(c echo.Context) error {
	id, err := getServiceOr404(c)
	if err != nil {
		return err
	}

	svcURL := fmt.Sprintf("%s%s", getRootURL(c), c.Request().URL())

	tileset := tilesets[id]
	imgFormat := TileFormatStr[tileset.tileformat]

	out := map[string]interface{}{
		"tilejson": "2.1.0",
		"id":       id,
		"scheme":   "xyz",
		"format":   imgFormat,
		"tiles":    []string{fmt.Sprintf("%s/tiles/{z}/{x}/{y}.%s", svcURL, imgFormat)},
		"map":      fmt.Sprintf("%s/map", svcURL),
	}

	metadata, err := tileset.ReadMetadata()
	if err != nil {
		log.Errorf("Could not read metadata for tileset %v", id)
		return err
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

	if tileset.utfgridConfig != nil {
		out["grids"] = []string{fmt.Sprintf("%s/tiles/{z}/{x}/{y}/grid.json", svcURL)}
	}

	return c.JSON(http.StatusOK, out)
}

func GetServiceHTML(c echo.Context) error {
	id, err := getServiceOr404(c)
	if err != nil {
		return err
	}

	p := TemplateParams{
		URL: fmt.Sprintf("%s%s", getRootURL(c), strings.TrimSuffix(c.Request().URL().Path(), "/map")),
		ID:  id,
	}

	if tilesets[id].tileformat == PBF {
		return c.Render(http.StatusOK, "map_gl", p)
	}

	return c.Render(http.StatusOK, "map", p)
}

func GetTile(c echo.Context) error {
	var (
		data        []byte
		contentType string
	)
	//TODO: validate x, y, z

	id, err := getServiceOr404(c)
	if err != nil {
		return err
	}

	yParams := strings.Split(c.Param("filename"), ".")
	key := strings.Join([]string{id, "tile", c.Param("z"), c.Param("x"), yParams[0]}, "/")

	err = cache.Get(nil, key, groupcache.AllocatingByteSliceSink(&data))
	if err != nil {
		log.Errorf("Error fetching key from cache: %s", key)
		// TODO: log
		return echo.NewHTTPError(http.StatusInternalServerError, "Error retrieving tile")
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

func GetGrid(c echo.Context) error {
	var data []byte

	id, err := getServiceOr404(c)
	if err != nil {
		return err
	}

	key := strings.Join([]string{id, "grid", c.Param("z"), c.Param("x"), c.Param("y")}, "/")

	err = cache.Get(nil, key, groupcache.AllocatingByteSliceSink(&data))
	if err != nil {
		log.Errorf("Error fetching key from cache: %s", key)
		// TODO: log
		return echo.NewHTTPError(http.StatusInternalServerError, "Error retriving tile")
	}

	if len(data) <= 1 {
		// TODO: confirm proper response type
		return c.JSON(http.StatusNotFound, struct {
			Message string `json:"message"`
		}{"UTF Grid does not exist"})
	}

	res := c.Response()
	res.Header().Add("Content-Type", "application/json")

	if tilesets[id].utfgridConfig.compressionType == ZLIB {
		res.Header().Add("Content-Encoding", "deflate")
	} else {
		res.Header().Add("Content-Encoding", "gzip")
	}

	res.WriteHeader(http.StatusOK)
	_, err = io.Copy(res, bytes.NewReader(data))
	return err
}

func CacheInfo(c echo.Context) error {
	hotStats := cache.CacheStats(groupcache.HotCache)
	mainStats := cache.CacheStats(groupcache.MainCache)

	out := map[string]interface{}{
		"hot":  hotStats,
		"main": mainStats,
	}
	return c.JSON(http.StatusOK, out)
}

func NotModifiedMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		var lastModified time.Time
		id := c.Param("id")
		if _, exists := tilesets[id]; exists {
			lastModified = tilesets[id].timestamp
		} else {
			lastModified = startuptime // startup time of server
		}

		if t, err := time.Parse(http.TimeFormat, c.Request().Header().Get(echo.HeaderIfModifiedSince)); err == nil && lastModified.Before(t.Add(1*time.Second)) {
			c.Response().Header().Del(echo.HeaderContentType)
			c.Response().Header().Del(echo.HeaderContentLength)
			return c.NoContent(http.StatusNotModified)
		}

		c.Response().Header().Set(echo.HeaderLastModified, lastModified.UTC().Format(http.TimeFormat))
		return next(c)
	}
}
