package main

import (
	"crypto/md5"
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/golang/groupcache"
	"github.com/jessevdk/go-flags"
	_ "github.com/mattn/go-sqlite3"
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
	pool         *groupcache.HTTPPool
	cache        *groupcache.Group
	connections  map[string]*sql.DB
	imageQueries map[string]*sql.Stmt
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
	tilesets, _ := filepath.Glob(path.Join(options.Tilesets, "*.mbtiles"))
	fmt.Println(tilesets)

	for i, filename := range tilesets {
		_, service := filepath.Split(filename)
		service = strings.Split(service, ".")[0]

		fmt.Println(i, filename, service)

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
	}

	pool = groupcache.NewHTTPPool(fmt.Sprintf("http://127.0.0.1:%v", options.CachePort))
	cache = groupcache.NewGroup("TileCache", options.CacheSize*1048576, groupcache.GetterFunc(
		func(ctx groupcache.Context, key string, dest groupcache.Sink) error {
			pathParams := strings.Split(key, "/")
			service := pathParams[1]
			yParams := strings.Split(pathParams[4], ".")
			z, _ := strconv.ParseUint(pathParams[2], 0, 64)
			x, _ := strconv.ParseUint(pathParams[3], 0, 64)
			y, _ := strconv.ParseUint(yParams[0], 0, 64)

			//flip y to match TMS spec
			y = (1 << z) - 1 - y

			var stmt *sql.Stmt
			if yParams[1] == "png" || yParams[1] == "jpg" {
				stmt = imageQueries[service]
			}

			var tile_data []byte
			err := stmt.QueryRow(uint8(z), uint16(x), uint16(y)).Scan(&tile_data)
			if err != nil {
				if err != sql.ErrNoRows {
					log.Fatal(err)
				}
			}
			dest.SetBytes(tile_data)
			return nil
		}))

	router := gin.Default()

	router.GET("/:service/:z/:x/:filename", func(c *gin.Context) {
		var (
			data        []byte
			blank       []byte
			contentType string
		)
		fmt.Println("URL", c.Request.URL)

		filename := c.Params.ByName("filename")

		//TODO: validate x, y, z
		extension := path.Ext(filename)
		switch extension {
		default:
			{
				log.Println("Invalid extension", extension)
				c.String(400, fmt.Sprintf("Invalid extension: %s", extension))
				return
			}
		case ".png":
			{
				blank = blankPNG
				contentType = "image/png"
			}
		case ".jpg":
			{
				blank = blankPNG // TODO: replace w/ JPG?
				contentType = "image/jpeg"
			}
		}

		key := c.Request.URL.String()

		err := cache.Get(nil, key, groupcache.AllocatingByteSliceSink(&data))
		if err != nil {
			log.Println("Error fetching key", key)
			c.String(500, fmt.Sprintf("Cache get failed for key: %s", key))
			return
		}
		etag := fmt.Sprintf("%x", md5.Sum(data))

		if c.Request.Header.Get("If-None-Match") == etag {
			c.Abort(304)
			return
		}

		if len(data) <= 1 {
			data = blank
		}

		c.Writer.Header().Add("Cache-Control", fmt.Sprintf("max-age=%v", options.MaxAge))
		c.Writer.Header().Add("Last-Modified", cacheSince)
		c.Writer.Header().Add("ETag", etag)
		c.Data(200, contentType, data)

		//TODO: gzip response
	})

	router.Run(fmt.Sprintf(":%v", options.Port))
}
