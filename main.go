package main

import (
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/golang/groupcache"
	"github.com/jessevdk/go-flags"
	_ "github.com/mattn/go-sqlite3"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

var options struct {
	Port      int    `long:"port" description:"Server port" default:"8080"`
	Tilesets  string `long:"tilesets" description:"path to tilesets" default:"./tilesets"`
	CachePort int    `long:"cacheport" description:"GroupCache port" default:"8000"`
	CacheSize int64  `long:"cachesize" description:"Size of Cache (MB)" default:"10"`
}

var (
	pool        *groupcache.HTTPPool
	cache       *groupcache.Group
	connections map[string]*sql.DB
	pngQueries  map[string]*sql.Stmt
	blankPNG    []byte
)

func main() {
	_, err := flags.ParseArgs(&options, os.Args)
	if err != nil {
		log.Fatal(err)
	}
	blankPNG, _ = ioutil.ReadFile("blank.png")

	connections = make(map[string]*sql.DB)
	pngQueries = make(map[string]*sql.Stmt)
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
		pngQueries[service] = stmt
	}

	pool = groupcache.NewHTTPPool(fmt.Sprintf("http://127.0.0.1:%v", options.CachePort))
	cache = groupcache.NewGroup("TileCache", options.CacheSize*1048576, groupcache.GetterFunc(
		func(ctx groupcache.Context, key string, dest groupcache.Sink) error {
			// log.Println("Requested", key)

			pathParams := strings.Split(key, "/")
			service := pathParams[1]
			yParams := strings.Split(pathParams[4], ".")
			z, _ := strconv.ParseUint(pathParams[2], 0, 64)
			x, _ := strconv.ParseUint(pathParams[3], 0, 64)
			y, _ := strconv.ParseUint(yParams[0], 0, 64)

			//flip y to match TMS spec
			y = (1 << z) - 1 - y

			var stmt *sql.Stmt
			if yParams[1] == "png" {
				stmt = pngQueries[service]
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

	router.GET("/*key", func(c *gin.Context) {
		var data []byte
		pathParams := strings.Split(c.Params.ByName("key"), "/")
		// fmt.Println("path segments", pathParams, len(pathParams))

		// fmt.Println(cache.CacheStats(1))

		if len(pathParams) != 5 {
			c.Abort(400)
			return
		}
		//TODO: validate x, y, z, and extension

		key := c.Params.ByName("key")
		err := cache.Get(nil, key, groupcache.AllocatingByteSliceSink(&data))
		if err != nil {
			log.Println("Error fetching key", key)
			c.Abort(500)
			return
		}
		if len(data) <= 1 {
			data = blankPNG
		}

		//TODO: make based on data
		c.Data(200, "image/png", data)
	})

	router.Run(fmt.Sprintf(":%v", options.Port))
}
