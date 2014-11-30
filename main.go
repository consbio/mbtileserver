package main

import (
	"crypto/md5"
	"database/sql"
	"encoding/json"
	// "flag"
	"fmt"
	"github.com/golang/groupcache"
	"github.com/jessevdk/go-flags"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var options struct {
	Hostname  string `long:"hostname" description:"externally accessible hostname" default:"localhost"`
	Bind      string `long:"bind" description:"Server port" default:":8000"`
	Tilesets  string `long:"tilesets" description:"path to tilesets" default:"./tilesets"`
	CachePort uint16 `long:"cacheport" description:"GroupCache port" default:"8001"`
	CacheSize int64  `long:"cachesize" description:"Size of Cache (MB)" default:"10"`
	MaxAge    uint   `long:"max_age" description:"Response max-age duration (seconds)" default:"3600"`
}

type DBClient struct {
	connection  *sqlx.DB
	imageStmt   *sql.Stmt
	infoStmt    *sqlx.Stmt
	contentType string
}

type KeyValuePair struct {
	Name  string
	Value string
}

var (
	pool       *groupcache.HTTPPool
	cache      *groupcache.Group
	dbClients  map[string]DBClient
	blankPNG   []byte
	cacheSince = time.Now().Format(http.TimeFormat)
)

func main() {
	_, err := flags.ParseArgs(&options, os.Args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Flags:", options)
	blankPNG, _ = ioutil.ReadFile("blank.png")

	dbClients = make(map[string]DBClient)
	tilesets, _ := filepath.Glob(path.Join(options.Tilesets, "*.mbtiles"))

	for _, filename := range tilesets {
		_, service := filepath.Split(filename)
		service = strings.Split(service, ".")[0]

		db, err := sqlx.Open("sqlite3", filename)
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()

		infoStmt, err := db.Preparex("select * from metadata where value is not ''")
		if err != nil {
			log.Fatal(err)
		}
		defer infoStmt.Close()

		stmt, err := db.Prepare("select tile_data from tiles where zoom_level = ? and tile_column = ? and tile_row = ?")
		if err != nil {
			log.Fatal(err)
		}
		defer stmt.Close()

		//query a sample tile to determine if png or jpg, since metadata from tilemill doesn't give this to us
		var tileData []byte
		err = db.QueryRow("select tile_data from images limit 1").Scan(&tileData)
		if err != nil {
			log.Fatal(err)
		}

		dbClients[service] = DBClient{
			connection:  db,
			infoStmt:    infoStmt,
			imageStmt:   stmt,
			contentType: http.DetectContentType(tileData),
		}
	}

	pool = groupcache.NewHTTPPool(fmt.Sprintf("http://127.0.0.1:%v", options.CachePort))
	cache = groupcache.NewGroup("TileCache", options.CacheSize*1048576, groupcache.GetterFunc(
		func(ctx groupcache.Context, key string, dest groupcache.Sink) error {
			pathParams := strings.Split(key, "/")
			service := pathParams[0]
			yParams := strings.Split(pathParams[3], ".")
			z, _ := strconv.ParseUint(pathParams[1], 0, 64)
			x, _ := strconv.ParseUint(pathParams[2], 0, 64)
			y, _ := strconv.ParseUint(yParams[0], 0, 64)
			//flip y to match TMS spec
			y = (1 << z) - 1 - y

			var tileData []byte
			err := dbClients[service].imageStmt.QueryRow(uint8(z), uint16(x), uint16(y)).Scan(&tileData)
			if err != nil {
				if err != sql.ErrNoRows {
					log.Fatal(err)
				}
			}
			dest.SetBytes(tileData)
			return nil
		}))

	//TODO: add gzip
	goji.Get("/services", ListServices)
	goji.Get("/services/:service", GetService)
	goji.Get("/services/:service/tiles/:z/:x/:filename", GetTile)
	//TODO:  goji.Get("/:service/grids/:z/:x/:filename", GetGrid) //return UTF8 grid

	goji.Serve()
}

type ServiceInfo struct {
	URI       string `json:"uri"`
	ImageType string `json:"imageType"`
}

func ListServices(c web.C, w http.ResponseWriter, r *http.Request) {
	services := make([]ServiceInfo, len(dbClients))
	i := 0
	for service, _ := range dbClients {
		services[i] = ServiceInfo{
			URI:       fmt.Sprintf("/services/%s", service),
			ImageType: strings.Split(dbClients[service].contentType, "/")[1],
		}
		i++
	}
	w.Header().Add("Content-Type", "application/json")
	json, _ := json.Marshal(services)
	w.Write(json)
}

func GetService(c web.C, w http.ResponseWriter, r *http.Request) {
	//https://github.com/mapbox/tilejson-spec/tree/master/2.1.0
	//FIXME: https://a.tiles.mapbox.com/v4/bcward.salcc.json?access_token=pk.eyJ1IjoiYmN3YXJkIiwiYSI6InJ5NzUxQzAifQ.CVyzbyOpnStfYUQ_6r8AgQ
	service := c.URLParams["service"]
	if _, exists := dbClients[service]; !exists {
		http.Error(w, fmt.Sprintf("Service not found: %s", service), http.StatusNotFound)
		return
	}
	results := make(map[string]string)
	rows, err := dbClients[service].infoStmt.Queryx()
	if err != nil {
		http.Error(w, fmt.Sprintf("Metadata query failed for: %s", service), http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var record KeyValuePair
		rows.StructScan(&record)
		results[record.Name] = record.Value
	}
	rootURL := fmt.Sprintf("http://%s", options.Hostname)
	if options.Bind != ":80" {
		rootURL = fmt.Sprintf("%s%s", rootURL, options.Bind)
	}
	out := map[string]interface{}{
		"tilejson": "2.1.0",
		"id":       service,
		"scheme":   "tms",
		"format":   strings.Split(dbClients[service].contentType, "/")[1],
		"tiles":    []string{fmt.Sprintf("%s/services/%s/tiles/{z}/{x}/{y}", rootURL, service)},
	}

	for k := range results {
		out[k] = results[k]
	}
	var value string
	var ok bool
	multiFloatFields := []string{"bounds", "center"}
	intFields := []string{"maxzoom", "minzoom"}
	for _, field := range multiFloatFields {
		if value, ok = results[field]; ok {
			out[field] = stringToFloats(value)
		}
	}
	for _, field := range intFields {
		if value, ok = results[field]; ok {
			out[field], _ = strconv.Atoi(results[field])
		}
	}

	w.Header().Add("Content-Type", "application/json")
	json, _ := json.Marshal(out)
	w.Write(json)
}

func GetTile(c web.C, w http.ResponseWriter, r *http.Request) {
	var (
		data        []byte
		contentType string
	)
	//TODO: validate x, y, z

	service := c.URLParams["service"]
	if _, exists := dbClients[service]; !exists {
		http.Error(w, fmt.Sprintf("Service not found: %s", service), http.StatusNotFound)
		return
	}

	key := strings.Join([]string{c.URLParams["service"], c.URLParams["z"], c.URLParams["x"], c.URLParams["filename"]}, "/")
	fmt.Println("URL key: ", key)

	err := cache.Get(nil, key, groupcache.AllocatingByteSliceSink(&data))
	if err != nil {
		log.Println("Error fetching key", key)
		http.Error(w, fmt.Sprintf("Cache get failed for key: %s", key), http.StatusInternalServerError)
		return
	}
	etag := fmt.Sprintf("%x", md5.Sum(data))

	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	if len(data) <= 1 {
		data = blankPNG
		contentType = "image/png"
	} else {
		contentType = dbClients[service].contentType
	}

	w.Header().Add("Cache-Control", fmt.Sprintf("max-age=%v", options.MaxAge))
	w.Header().Add("Last-Modified", cacheSince)
	w.Header().Add("Content-Type", contentType)
	w.Header().Add("ETag", etag)
	w.Write(data)
}

func stringToFloats(str string) []float32 {
	split := strings.Split(str, ",")
	var out = make([]float32, len(split))
	for i, v := range split {
		value, _ := strconv.ParseFloat(v, 32)
		out[i] = float32(value)
	}
	return out
}
