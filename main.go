package main

import (
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/golang/groupcache"
	"github.com/jessevdk/go-flags"
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
	Port      uint16 `long:"port" description:"Server port" default:"8080"`
	Tilesets  string `long:"tilesets" description:"path to tilesets" default:"./tilesets"`
	CachePort uint16 `long:"cacheport" description:"GroupCache port" default:"8000"`
	CacheSize int64  `long:"cachesize" description:"Size of Cache (MB)" default:"10"`
	MaxAge    uint   `long:"max_age" description:"Response max-age duration (seconds)" default:"3600"`
}

var (
	pool  *groupcache.HTTPPool
	cache *groupcache.Group
	//TODO: consolidate these into a single map of structs!
	connections  map[string]*sql.DB
	imageQueries map[string]*sql.Stmt
	contentTypes map[string]string
	blankPNG     []byte
	cacheSince   = time.Now().Format(http.TimeFormat)
)

func main() {
	_, err := flags.ParseArgs(&options, os.Args)
	if err != nil {
		log.Fatal(err)
	}
	blankPNG, _ = ioutil.ReadFile("blank.png")

	connections = make(map[string]*sql.DB)
	imageQueries = make(map[string]*sql.Stmt)
	contentTypes = make(map[string]string)
	tilesets, _ := filepath.Glob(path.Join(options.Tilesets, "*.mbtiles"))

	for _, filename := range tilesets {
		_, service := filepath.Split(filename)
		service = strings.Split(service, ".")[0]

		fmt.Println("Service: ", service)

		db, err := sql.Open("sqlite3", filename)
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		connections[service] = db

		stmt, err := db.Prepare("select tile_data from tiles where zoom_level = ? and tile_column = ? and tile_row = ?")
		if err != nil {
			log.Fatal(err)
		}
		defer stmt.Close()
		imageQueries[service] = stmt

		//query a sample tile to determine if png or jpg, since metadata from tilemill doesn't give this to us
		var tileData []byte
		err = db.QueryRow("select tile_data from images limit 1").Scan(&tileData)
		if err != nil {
			log.Fatal(err)
		}
		contentTypes[service] = http.DetectContentType(tileData)
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

			var stmt *sql.Stmt
			stmt = imageQueries[service]

			var tileData []byte
			err := stmt.QueryRow(uint8(z), uint16(x), uint16(y)).Scan(&tileData)
			if err != nil {
				if err != sql.ErrNoRows {
					log.Fatal(err)
				}
			}
			dest.SetBytes(tileData)
			return nil
		}))

	//TODO: add gzip
	//TODO: add 301s for all non-slash terminated routes
	goji.Get("/services/", ListServices)
	goji.Get("/services", http.RedirectHandler("/services/", http.StatusMovedPermanently))
	goji.Get("/services/:service", GetService)
	goji.Get("/services/:service/tiles/:z/:x/:filename", GetTile)
	//TODO:  goji.Get("/:service/grids/:z/:x/:filename", GetGrid) //return UTF8 grid

	flag.Set("bind", fmt.Sprintf(":%v", options.Port))
	goji.Serve()
}

type ServiceInfo struct {
	URI       string `json:"uri"`
	ImageType string `json:"imageType"`
}

func ListServices(c web.C, w http.ResponseWriter, r *http.Request) {
	services := make([]ServiceInfo, len(imageQueries))
	i := 0
	for service, _ := range imageQueries {
		services[i] = ServiceInfo{
			URI:       fmt.Sprintf("/%s", service),
			ImageType: strings.Split(contentTypes[service], "/")[1],
		}
		i++
	}
	json, _ := json.Marshal(services)
	w.Write(json)
}

func GetService(c web.C, w http.ResponseWriter, r *http.Request) {
	fmt.Println(c.URLParams)
	service := c.URLParams["service"]
	if _, exists := imageQueries[service]; !exists {
		http.Error(w, fmt.Sprintf("Service not found: %s", service), http.StatusNotFound)
		return
	}
	w.Write([]byte("TODO: Service info"))
}

func GetTile(c web.C, w http.ResponseWriter, r *http.Request) {
	var (
		data        []byte
		contentType string
	)
	//TODO: validate x, y, z

	service := c.URLParams["service"]
	if _, exists := imageQueries[service]; !exists {
		http.Error(w, fmt.Sprintf("Service not found: %s", service), http.StatusNotFound)
		return
	}

	// key := strings.TrimPrefix(r.URL.String(), "/services/")
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
		contentType = contentTypes[service]
	}

	w.Header().Add("Cache-Control", fmt.Sprintf("max-age=%v", options.MaxAge))
	w.Header().Add("Last-Modified", cacheSince)
	w.Header().Add("Content-Type", contentType)
	w.Header().Add("ETag", etag)
	w.Write(data)
}
