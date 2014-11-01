package main

import (
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"io/ioutil"
	"log"
	"path/filepath"
	"strconv"
	"strings"
)

var connections map[string]*sql.DB
var pngQueries map[string]*sql.Stmt

func main() {

	connections = make(map[string]*sql.DB)
	pngQueries = make(map[string]*sql.Stmt)
	tilesets, _ := filepath.Glob("tilesets/*.mbtiles")
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

	router := gin.Default()
	router.GET("/:service/:z/:x/:y", func(c *gin.Context) {
		service := c.Params.ByName("service")
		stmt, ok := pngQueries[service]
		if !ok {
			log.Printf("Service not found: %s", service)
			c.Abort(404)
			return
		}

		//TODO: cache response data in memory

		var tile_data []byte

		z, _ := strconv.ParseUint(c.Params.ByName("z"), 0, 64)
		y, _ := strconv.ParseUint(c.Params.ByName("y"), 0, 64)
		//flip y to match TMS spec
		y = (1 << z) - 1 - y

		err := stmt.QueryRow(z, c.Params.ByName("x"), y).Scan(&tile_data)
		if err != nil {
			if err == sql.ErrNoRows {
				tile_data, _ = ioutil.ReadFile("blank.png")
			} else {
				log.Fatal(err)
			}
		}
		//TODO set headers for cache control
		c.Writer.Header().Add("Cache-Control", "max-age: 300")
		c.Data(200, "image/png", tile_data)
	})

	router.Run(":8080")
}
