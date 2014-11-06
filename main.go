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
	Port     int    `long:"port" description:"Server port" default:"8080"`
	Tilesets string `long:"tilesets" description:"path to tilesets" default:"./tilesets"`
}

var (
	pool        *groupcache.HTTPPool
	cache       *groupcache.Group
	connections map[string]*sql.DB
	pngQueries  map[string]*sql.Stmt
)

func main() {
	_, err := flags.ParseArgs(&options, os.Args)
	if err != nil {
		log.Fatal(err)
	}

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

	pool = groupcache.NewHTTPPool("http://127.0.0.1:8000")
	cache = groupcache.NewGroup("TileCache", 64<<20, groupcache.GetterFunc(
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
				if err == sql.ErrNoRows {
					// log.Println("No tile found")
					// tile_data = make([]byte, 1, 1)
					// tile_data[0] = 0
				} else {
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
			// log.Println("Empty data")
			data, _ = ioutil.ReadFile("blank.png")
		}

		// log.Println("Bytes returned:", len(data))
		//TODO: make based on data
		c.Data(200, "image/png", data)
	})

	// router.GET("/:service/:z/:x/:y", func(c *gin.Context) {
	// 	service := c.Params.ByName("service")
	// 	stmt, ok := pngQueries[service]
	// 	if !ok {
	// 		log.Printf("Service not found: %s", service)
	// 		c.Abort(404)
	// 		return
	// 	}

	// 	//TODO: cache response data in memory

	// 	var tile_data []byte
	// 	y_params := strings.Split(c.Params.ByName("y"), ".")
	// 	if len(y_params) != 2 {
	// 		c.Abort(400)
	// 		return
	// 	}

	// 	z, _ := strconv.ParseUint(c.Params.ByName("z"), 0, 64)
	// 	y, _ := strconv.ParseUint(y_params[0], 0, 64)
	// 	extension := y_params[1]
	// 	log.Println(extension)
	// 	//flip y to match TMS spec
	// 	y = (1 << z) - 1 - y

	// 	err := stmt.QueryRow(z, c.Params.ByName("x"), y).Scan(&tile_data)
	// 	if err != nil {
	// 		if err == sql.ErrNoRows {
	// 			tile_data, _ = ioutil.ReadFile("blank.png")
	// 		} else {
	// 			log.Fatal(err)
	// 		}
	// 	}
	// 	//TODO set headers for cache control
	// 	c.Writer.Header().Add("Cache-Control", "public, max-age= 300")
	// 	c.Data(200, "image/png", tile_data)
	// })

	router.Run(fmt.Sprintf(":%v", options.Port))
}
