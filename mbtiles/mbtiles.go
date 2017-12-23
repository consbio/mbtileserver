package mbtiles

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // import sqlite3 driver
)

// TileFormat is an enum that defines the tile format of a tile
// in the mbtiles file.  Supported image formats:
//   * PNG
//   * JPG
//   * WEBP
//   * PBF  (vector tile protocol buffers)
// Tiles may be compressed, in which case the type is one of:
//   * GZIP
//   * ZLIB
// Compressed tiles may be PBF or UTFGrids
type TileFormat uint8

// Constants representing TileFormat types
const (
	UNKNOWN TileFormat = iota // UNKNOWN TileFormat cannot be determined from first few bytes of tile
	GZIP                      // encoding = gzip
	ZLIB                      // encoding = deflate
	PNG
	JPG
	PBF
	WEBP
)

// String returns a string representing the TileFormat
func (t TileFormat) String() string {
	switch t {
	case PNG:
		return "png"
	case JPG:
		return "jpg"
	case PBF:
		return "pbf"
	case WEBP:
		return "webp"
	default:
		return ""
	}
}

// ContentType returns the MIME content type of the tile
func (t TileFormat) ContentType() string {
	switch t {
	case PNG:
		return "image/png"
	case JPG:
		return "image/jpeg"
	case PBF:
		return "application/x-protobuf" // Content-Encoding header must be gzip
	case WEBP:
		return "image/webp"
	default:
		return ""
	}
}

// DB represents an mbtiles file connection.
type DB struct {
	filename           string     // name of tile mbtiles file
	db                 *sql.DB    // database connection for mbtiles file
	tileformat         TileFormat // tile format: PNG, JPG, PBF, WEBP
	timestamp          time.Time  // timestamp of file, for cache control headers
	hasUTFGrid         bool       // true if mbtiles file contains additional tables with UTFGrid data
	utfgridCompression TileFormat // compression (GZIP or ZLIB) of UTFGrids
	hasUTFGridData     bool       // true if UTFGrids have corresponding key / value data that need to be joined and returned with the UTFGrid
}

// NewDB creates a new DB instance.
// Connection is closed by runtime on application termination or by calling
// its Close() method.
func NewDB(filename string) (*DB, error) {
	_, id := filepath.Split(filename)
	id = strings.Split(id, ".")[0]

	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}

	//Saves last modified mbtiles time for setting Last-Modified header
	fileStat, err := os.Stat(filename)
	if err != nil {
		return nil, fmt.Errorf("could not read file stats for mbtiles file: %s\n", filename)
	}

	//query a sample tile to determine format
	var data []byte
	err = db.QueryRow("select tile_data from tiles limit 1").Scan(&data)
	if err != nil {
		return nil, err
	}
	tileformat, err := detectTileFormat(&data)
	if err != nil {
		return nil, err
	}
	if tileformat == GZIP {
		tileformat = PBF // GZIP masks PBF, which is only expected type for tiles in GZIP format
	}
	out := DB{
		db:         db,
		tileformat: tileformat,
		timestamp:  fileStat.ModTime().Round(time.Second), // round to nearest second
	}

	// UTFGrids
	// first check to see if requisite tables exist
	var count int
	err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='view' AND name = 'grids'").Scan(&count)
	if err != nil {
		return nil, err
	}
	if count == 1 {
		// query a sample grid to detect type
		var gridData []byte
		err = db.QueryRow("select grid from grids where grid is not null LIMIT 1").Scan(&gridData)
		if err != nil {
			if err != sql.ErrNoRows {
				return nil, fmt.Errorf("could not read sample grid to determine type: %v", err)
			}
		} else {
			out.hasUTFGrid = true
			out.utfgridCompression, err = detectTileFormat(&gridData)
			if err != nil {
				return nil, fmt.Errorf("could not determine UTF Grid compression type: %v", err)
			}

			// Check to see if grid_data view exists
			count = 0 // prevent use of prior value
			err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='view' AND name = 'grid_data'").Scan(&count)
			if err != nil {
				return nil, err
			}
			if count == 1 {
				out.hasUTFGridData = true
			}
		}
	}

	return &out, nil

}

// ReadTile reads a tile with tile identifiers z, x, y into provided *[]byte.
// data will be nil if the tile does not exist in the database
func (tileset *DB) ReadTile(z uint8, x uint64, y uint64, data *[]byte) error {
	err := tileset.db.QueryRow("select tile_data from tiles where zoom_level = ? and tile_column = ? and tile_row = ?", z, x, y).Scan(data)
	if err != nil {
		if err == sql.ErrNoRows {
			*data = nil // If this tile does not exist in the database, return empty bytes
			return nil
		}
		return err
	}
	return nil
}

// ReadGrid reads a UTFGrid with identifiers z, x, y into provided *[]byte. data
// will be nil if the grid does not exist in the database, and an error will be
// raised. This merges in grid key data, if any exist The data is returned in
// the original compression encoding (zlib or gzip)
func (tileset *DB) ReadGrid(z uint8, x uint64, y uint64, data *[]byte) error {
	if !tileset.hasUTFGrid {
		return errors.New("Tileset does not contain UTFgrids")
	}

	err := tileset.db.QueryRow("select grid from grids where zoom_level = ? and tile_column = ? and tile_row = ?", z, x, y).Scan(data)
	if err != nil {
		if err == sql.ErrNoRows {
			*data = nil // If this tile does not exist in the database, return empty bytes
			return nil
		}
		return err
	}

	if tileset.hasUTFGridData {
		keydata := make(map[string]interface{})
		var (
			key   string
			value []byte
		)

		rows, err := tileset.db.Query("select key_name, key_json FROM grid_data where zoom_level = ? and tile_column = ? and tile_row = ?", z, x, y)
		if err != nil {
			return fmt.Errorf("cannot fetch grid data: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			err := rows.Scan(&key, &value)
			if err != nil {
				return fmt.Errorf("could not fetch grid data: %v", err)
			}
			valuejson := make(map[string]interface{})
			json.Unmarshal(value, &valuejson)
			keydata[key] = valuejson
		}

		if len(keydata) == 0 {
			return nil // there is no key data for this tile, return
		}

		var (
			zreader io.ReadCloser  // instance of zlib or gzip reader
			zwriter io.WriteCloser // instance of zlip or gzip writer
			buf     bytes.Buffer
		)
		reader := bytes.NewReader(*data)

		if tileset.utfgridCompression == ZLIB {
			zreader, err = zlib.NewReader(reader)
			if err != nil {
				return err
			}
			zwriter = zlib.NewWriter(&buf)
		} else {
			zreader, err = gzip.NewReader(reader)
			if err != nil {
				return err
			}
			zwriter = gzip.NewWriter(&buf)
		}

		var utfjson map[string]interface{}
		jsonDecoder := json.NewDecoder(zreader)
		jsonDecoder.Decode(&utfjson)
		zreader.Close()

		// splice the key data into the UTF json
		utfjson["data"] = keydata
		if err != nil {
			return err
		}

		// now re-encode to original zip encoding
		jsonEncoder := json.NewEncoder(zwriter)
		err = jsonEncoder.Encode(utfjson)
		if err != nil {
			return err
		}
		zwriter.Close()
		*data = buf.Bytes()
	}
	return nil
}

// ReadMetadata reads the metadata table into a map, casting their values into
// the appropriate type
func (tileset *DB) ReadMetadata() (map[string]interface{}, error) {
	var (
		key   string
		value string
	)
	metadata := make(map[string]interface{})

	rows, err := tileset.db.Query("select * from metadata where value is not ''")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		rows.Scan(&key, &value)

		switch key {
		case "maxzoom", "minzoom":
			metadata[key], err = strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("cannot read metadata item %s: %v", key, err)
			}
		case "bounds", "center":
			metadata[key], err = stringToFloats(value)
			if err != nil {
				return nil, fmt.Errorf("cannot read metadata item %s: %v", key, err)
			}
		case "json":
			err = json.Unmarshal([]byte(value), &metadata)
			if err != nil {
				return nil, fmt.Errorf("unable to parse JSON metadata item: %v", err)
			}
		default:
			metadata[key] = value
		}
	}

	// Supplement missing values by inferring from available data
	_, hasMinZoom := metadata["minzoom"]
	_, hasMaxZoom := metadata["maxzoom"]
	if !(hasMinZoom && hasMaxZoom) {
		var minZoom, maxZoom int
		err := tileset.db.QueryRow("select min(zoom_level), max(zoom_level) from tiles").Scan(&minZoom, &maxZoom)
		if err != nil {
			return metadata, nil
		}
		metadata["minzoom"] = minZoom
		metadata["maxzoom"] = maxZoom
	}
	return metadata, nil
}

// TileFormat returns the TileFormat of the DB.
func (tileset *DB) TileFormat() TileFormat {
	return tileset.tileformat
}

// TileFormatString returns the string representation of the TileFormat of the DB.
func (tileset *DB) TileFormatString() string {
	return tileset.tileformat.String()
}

// ContentType returns the content-type string of the TileFormat of the DB.
func (tileset *DB) ContentType() string {
	return tileset.tileformat.ContentType()
}

// HasUTFGrid returns whether the DB has a UTF grid.
func (tileset *DB) HasUTFGrid() bool {
	return tileset.hasUTFGrid
}

// HasUTFGridData returns whether the DB has UTF grid data.
func (tileset *DB) HasUTFGridData() bool {
	return tileset.hasUTFGridData
}

// UTFGridCompression returns the compression type of the UTFGrid in the DB:
// ZLIB or GZIP
func (tileset *DB) UTFGridCompression() TileFormat {
	return tileset.utfgridCompression
}

// TimeStamp returns the time stamp of the DB.
func (tileset *DB) TimeStamp() time.Time {
	return tileset.timestamp
}

// Close closes the database connection
func (tileset *DB) Close() error {
	return tileset.db.Close()
}

// detectFileFormat inspects the first few bytes of byte array to determine tile
// format PBF tile format does not have a distinct signature, it will be
// returned as GZIP, and it is up to caller to determine that it is a PBF format
func detectTileFormat(data *[]byte) (TileFormat, error) {
	patterns := map[TileFormat][]byte{
		GZIP: []byte("\x1f\x8b"), // this masks PBF format too
		ZLIB: []byte("\x78\x9c"),
		PNG:  []byte("\x89\x50\x4E\x47\x0D\x0A\x1A\x0A"),
		JPG:  []byte("\xFF\xD8\xFF"),
		WEBP: []byte("\x52\x49\x46\x46\xc0\x00\x00\x00\x57\x45\x42\x50\x56\x50"),
	}

	for format, pattern := range patterns {
		if bytes.HasPrefix(*data, pattern) {
			return format, nil
		}
	}

	return UNKNOWN, errors.New("Could not detect tile format")
}

// stringToFloats converts a commma-delimited string of floats to a slice of
// float64 and returns it and the first error that was encountered.
// Example: "1.5,2.1" => [1.5, 2.1]
func stringToFloats(str string) ([]float64, error) {
	split := strings.Split(str, ",")
	var out []float64
	for _, v := range split {
		value, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return out, fmt.Errorf("could not parse %q to floats: %v", str, err)
		}
		out = append(out, value)
	}
	return out, nil
}
