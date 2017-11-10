package main

import (
	"bytes"
	"fmt"
	"path"

	"golang.org/x/crypto/acme/autocert"

	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang/groupcache"
	"github.com/labstack/echo"

	"github.com/evalphobia/logrus_sentry"
	"github.com/labstack/echo/middleware"
	_ "github.com/mattn/go-sqlite3"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/consbio/mbtileserver/handlers"
	"github.com/consbio/mbtileserver/mbtiles"
)

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
	cache       *groupcache.Group
	tilesets    map[string]mbtiles.DB
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
	pathPrefix  string
	domain      string
	sentry_DSN  string
	verbose     bool
	autotls     bool
	redirect    bool
)

func init() {
	flags := RootCmd.Flags()
	flags.IntVarP(&port, "port", "p", 8000, "Server port.")
	flags.StringVarP(&tilePath, "dir", "d", "./tilesets", "Directory containing mbtiles files.")
	flags.StringVarP(&certificate, "cert", "c", "", "X.509 TLS certificate filename.  If present, will be used to enable SSL on the server.")
	flags.StringVarP(&privateKey, "key", "k", "", "TLS private key")
	flags.Int64Var(&cacheSize, "cachesize", 250, "Size of cache in MB.")
	flags.StringVar(&pathPrefix, "path", "", "URL root path of this server (if behind a proxy)")
	flags.StringVar(&domain, "domain", "", "Domain name of this server")
	flags.StringVar(&sentry_DSN, "dsn", "", "Sentry DSN")
	flags.BoolVarP(&verbose, "verbose", "v", false, "Verbose logging")
	flags.BoolVarP(&autotls, "tls", "t", false, "Auto TLS via Let's Encrypt")
	flags.BoolVarP(&redirect, "redirect", "r", false, "Redirect HTTP to HTTPS")
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
	domainExists := len(domain) > 0

	if certExists != keyExists {
		log.Fatalln("Both certificate and private key are required to use SSL")
	}

	if len(pathPrefix) > 0 && !domainExists {
		log.Fatalln("Domain is required if path is provided")
	}

	if autotls && !domainExists {
		log.Fatalln("Domain is required to use auto TLS")
	}

	if (certExists || autotls) && port != 443 {
		log.Warnln("Port 443 should be used for TLS")
	}

	if redirect && !(certExists || autotls) {
		log.Fatalln("Certificate or tls options are required to use redirect")
	}

	var filenames []string
	err := filepath.Walk(tilePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(strings.ToLower(path), ".mbtiles") {
			filenames = append(filenames, path)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Unable to scan tileset directory for mbtiles files\n%v", err)
	}

	if len(filenames) == 0 {
		log.Fatal("No tilesets found in tileset directory")
	}

	log.Infof("Found %v mbtiles files in %s", len(filenames), tilePath)

	tilesets = make(map[string]mbtiles.DB)
	for _, filename := range filenames {
		subpath, err := filepath.Rel(tilePath, filename)
		if err != nil {
			log.Errorf("Unable to extract ID for file: %s\n%v", filename, err)
			continue
		}
		e := filepath.Ext(filename)
		p := filepath.ToSlash(subpath)
		id := strings.ToLower(p[:len(p)-len(e)])

		tileset, err := mbtiles.NewDB(filename)
		if err != nil {
			log.Errorf("could not open mbtiles file: %s\n%v", filename, err)
			continue
		}
		log.Infof("providing tiles from %q as %q", filename, id)
		tilesets[id] = *tileset
	}

	log.Debugf("Cache size: %v MB\n", cacheSize)

	cache = groupcache.NewGroup("TileCache", cacheSize*1048576, groupcache.GetterFunc(cacheGetter))

	e := echo.New()
	e.HideBanner = true
	e.Pre(middleware.RemoveTrailingSlash())
	if verbose {
		e.Use(middleware.Logger())
	}
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	t := &Template{
		templates: template.Must(handlers.TemplatesFromAssets()),
	}
	e.Renderer = t

	gzip := middleware.Gzip()

	// Setup routing
	e.File("/favicon.ico", "favicon.ico")
	e.File("/favicon.png", "favicon.png")

	// TODO: can use more caching here
	staticPrefix := "/static"
	if pathPrefix != "" {
		staticPrefix = "/" + pathPrefix + staticPrefix
	}
	staticHandler := http.StripPrefix(staticPrefix, handlers.Static())
	e.GET(staticPrefix+"*", echo.WrapHandler(staticHandler), gzip)

	e.GET("/services", ListServices, NotModifiedMiddleware, gzip)

	services := e.Group("/services/")
	arcgis := e.Group("/arcgis/rest/services/")

	var g, ag *echo.Group
	for id := range tilesets {
		dir, _ := path.Split(id)
		if len(dir) == 0 {
			g = services
			ag = arcgis
			// TODO: e.GET("/arcgis/rest/services", GetArcGISServices, NotModifiedMiddleware, gzip)
		} else {
			g = services.Group(dir)
			ag = arcgis.Group(dir)
			// TODO: services listing for tiles in this dir
			// TODO: services listing for ArcGIS in this dir
		}

		g.GET(":id", GetServiceInfo, NotModifiedMiddleware, gzip)
		g.GET(":id/map", GetServiceHTML, NotModifiedMiddleware, gzip)
		g.GET(":id/tiles/:z/:x/:filename", GetTile, NotModifiedMiddleware)

		ag.GET(":id/MapServer", GetArcGISService, NotModifiedMiddleware, gzip)
		ag.GET(":id/MapServer/layers", GetArcGISServiceLayers, NotModifiedMiddleware, gzip)
		ag.GET(":id/MapServer/legend", GetArcGISServiceLegend, NotModifiedMiddleware, gzip)
		ag.GET(":id/MapServer/tile/:z/:y/:x", GetArcGISTile, NotModifiedMiddleware)
	}

	e.GET("/admin/cache", CacheInfo, gzip)

	// Start the server
	fmt.Println("\n--------------------------------------")
	fmt.Println("Use Ctrl-C to exit the server")
	fmt.Println("--------------------------------------")

	// If starting TLS on 443, start server on port 80 too
	if redirect {
		e.Pre(middleware.HTTPSRedirect())
		if port == 443 {
			go func(c *echo.Echo) {
				fmt.Println("HTTP server with redirect started on port 80\n")
				log.Fatal(e.Start(":80"))
			}(e)
		}
	}

	switch {
	case certExists:
		{
			log.Debug("Starting HTTPS using provided certificate")
			if _, err := os.Stat(certificate); os.IsNotExist(err) {
				log.Fatalf("Could not find certificate file: %s\n", certificate)
			}
			if _, err := os.Stat(privateKey); os.IsNotExist(err) {
				log.Fatalf("Could not find private key file: %s\n", privateKey)
			}

			fmt.Printf("HTTPS server started on port %v\n", port)
			log.Fatal(e.StartTLS(fmt.Sprintf(":%v", port), certificate, privateKey))
		}
	case autotls:
		{
			log.Debug("Starting HTTPS using Let's Encrypt")
			e.AutoTLSManager.Cache = autocert.DirCache(".certs")
			e.AutoTLSManager.HostPolicy = autocert.HostWhitelist(domain)
			fmt.Printf("HTTPS server started on port %v\n", port)
			log.Fatal(e.StartAutoTLS(fmt.Sprintf(":%v", port)))
		}
	default:
		{
			fmt.Printf("HTTP server started on port %v\n", port)
			log.Fatal(e.Start(fmt.Sprintf(":%v", port)))
		}
	}

}

func cacheGetter(ctx groupcache.Context, key string, dest groupcache.Sink) error {
	pathParams := strings.Split(key, "|")
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
	} else if tileType == "grid" && tileset.HasUTFGrid() {
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
	requestPath := strings.ToLower(c.Request().URL.Path)
	idPos := strings.Index(c.Path(), ":id")
	if idPos != -1 {
		// remove trailing part of path after :id
		requestPath = fmt.Sprintf("%s/%s", requestPath[:idPos-1], strings.ToLower(c.Param("id")))
	}
	id := strings.Split(requestPath, "/services/")[1]
	if _, exists := tilesets[id]; !exists {
		log.Warnf("Service not found: %s\n", id)
		return "", echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("Service not found: %s", id))
	}
	return id, nil
}

// getRootURL is a convenience function to determine the root URL from the
// echo.Context.
func getRootURL(c echo.Context) string {
	return handlers.RootURL(c.Request(), domain, pathPrefix)
}

func ListServices(c echo.Context) error {
	// TODO: need to paginate the responses
	rootURL := fmt.Sprintf("%s%s", getRootURL(c), c.Request().URL)
	services := make([]ServiceInfo, len(tilesets))
	i := 0
	for id, tileset := range tilesets {
		services[i] = ServiceInfo{
			ImageType: tileset.TileFormatString(),
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

	svcURL := fmt.Sprintf("%s%s", getRootURL(c), c.Request().URL)

	tileset := tilesets[id]
	imgFormat := tileset.TileFormatString()

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

	if tileset.HasUTFGrid() {
		out["grids"] = []string{fmt.Sprintf("%s/tiles/{z}/{x}/{y}.json", svcURL)}
	}

	return c.JSON(http.StatusOK, out)
}

func GetServiceHTML(c echo.Context) error {
	id, err := getServiceOr404(c)
	if err != nil {
		return err
	}

	p := TemplateParams{
		URL: fmt.Sprintf("%s%s", getRootURL(c), strings.TrimSuffix(c.Request().URL.Path, "/map")),
		ID:  id,
	}

	if tilesets[id].TileFormat() == mbtiles.PBF {
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

	filename := c.Param("filename")
	y := strings.Split(filename, ".")[0]
	tileType := "tile"
	if strings.HasSuffix(filename, ".json") {
		tileType = "grid"
	}
	key := strings.Join([]string{id, tileType, c.Param("z"), c.Param("x"), y}, "|")

	err = cache.Get(nil, key, groupcache.AllocatingByteSliceSink(&data))
	if err != nil {
		log.Errorf("Error fetching key from cache: %s", key)
		return echo.NewHTTPError(http.StatusInternalServerError, "Error retrieving tile")
	}
	tileset := tilesets[id]
	res := c.Response()

	if data == nil || len(data) <= 1 {
		switch tileset.TileFormat() {
		case mbtiles.PNG, mbtiles.JPG, mbtiles.WEBP:
			// Return blank PNG for all image types
			res.Header().Add("Content-Type", "image/png")
			res.WriteHeader(http.StatusOK)
			_, err = res.Write(handlers.BlankPNG())
			return err

		case mbtiles.PBF:
			// Return 204
			res.WriteHeader(http.StatusNoContent) // this must be after setting other headers
			return nil

		default:
			// If  utfgrid, return 404 w/ json, consistent w/ mapbox
			return c.JSON(http.StatusNotFound, struct {
				Message string `json:"message"`
			}{"Tile does not exist"})
		}
	}

	if tileType == "grid" {
		contentType = "application/json"

		if tileset.UTFGridCompression() == mbtiles.ZLIB {
			res.Header().Add("Content-Encoding", "deflate")
		} else {
			res.Header().Add("Content-Encoding", "gzip")
		}
	} else {
		contentType = tileset.ContentType()

		if tileset.TileFormat() == mbtiles.PBF {
			res.Header().Add("Content-Encoding", "gzip")
		}
	}
	res.Header().Add("Content-Type", contentType)

	res.WriteHeader(http.StatusOK) // this must be after setting other headers
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
			lastModified = tilesets[id].TimeStamp()
		} else {
			lastModified = startuptime // startup time of server
		}

		if t, err := time.Parse(http.TimeFormat, c.Request().Header.Get(echo.HeaderIfModifiedSince)); err == nil && lastModified.Before(t.Add(1*time.Second)) {
			c.Response().Header().Del(echo.HeaderContentType)
			c.Response().Header().Del(echo.HeaderContentLength)
			return c.NoContent(http.StatusNotModified)
		}

		c.Response().Header().Set(echo.HeaderLastModified, lastModified.UTC().Format(http.TimeFormat))
		return next(c)
	}
}
