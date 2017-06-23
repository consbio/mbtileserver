package main

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"database/sql"
	"encoding/json"
	"errors"
	"io"

	"github.com/jmoiron/sqlx"
	"github.com/murlokswarm/log"
)

type CompressionType uint8

const (
	UNKNOWN CompressionType = iota
	GZIP                    // encoding = gzip
	ZLIB                    // encoding = deflate
)

// Inspects first two bytes and determines if compression is zlib or gzip
func detectCompressionType(data *[]byte) (CompressionType, error) {
	// first two bytes are 78 9c if zlib, and 1f 8b if zlib
	if bytes.HasPrefix(*data, []byte("\x1f\x8b")) {
		return GZIP, nil
	} else if bytes.HasPrefix(*data, []byte("\x78\x9c")) {
		return ZLIB, nil
	}

	return UNKNOWN, errors.New("Could not detect compresion type")
}

type UTFGridConfig struct {
	gridQuery       *sql.Stmt
	gridDataQuery   *sql.Stmt
	compressionType CompressionType
}

func getUTFGridConfig(db *sqlx.DB) (*UTFGridConfig, error) {
	// Queries created here will be closed using defer in the caller

	// first check to see if requisite tables exist
	var count int
	err := db.Get(&count, "SELECT count(*) FROM sqlite_master WHERE type='view' AND name = 'grids'")
	if err != nil {
		log.Error(err)
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}

	// prepare query to fetch grid data
	gridQuery, err := db.Prepare("select grid from grids where zoom_level = ? and tile_column = ? and tile_row = ?")
	if err != nil {
		log.Error("could not create prepared grid query for file")
		return nil, err
	}

	// query a sample grid to detect type
	var data []byte
	err = db.Get(&data, "select grid from grids where grid is not null LIMIT 1")
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // view exists but has no data
		}
		log.Error("Could not read sample grid to determine type")
		return nil, err
	}

	compressionType, err := detectCompressionType(&data)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	config := UTFGridConfig{
		gridQuery:       gridQuery,
		compressionType: compressionType,
	}

	count = 0 // prevent use of prior value
	err = db.Get(&count, "SELECT count(*) FROM sqlite_master WHERE type='view' AND name = 'grid_data'")
	if err != nil {
		return nil, err
	}
	if count == 1 {
		// prepare query to fetch grid key data
		gridDataQuery, err := db.Prepare("select key_name, key_json FROM grid_data where zoom_level = ? and tile_column = ? and tile_row = ?")
		if err != nil {
			log.Error("could not create prepared grid data query for file")
			return nil, err
		}

		config.gridDataQuery = gridDataQuery
	}

	return &config, nil
}

//type Tile struct {
//Z uint8
//X uint64
//Y uint64
//}

// Reads a grid at z, x, y into provided *[]byte.
// This merges in grid key data, if any exist
// The data is returned in the original compression encoding (zlib or gzip)
func (config *UTFGridConfig) ReadGrid(z uint8, x uint64, y uint64, data *[]byte) error {
	err := config.gridQuery.QueryRow(z, x, y).Scan(data)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Error(err)
			return err
		}
		*data = nil // not a problem, just return empty bytes
		return nil
	}

	if config.gridDataQuery != nil {
		keydata := make(map[string]interface{})
		var (
			key   string
			value []byte
		)

		rows, err := config.gridDataQuery.Query(z, x, y)
		if err != nil {
			log.Error("Error fetching grid data")
			return err
		}

		for rows.Next() {
			err := rows.Scan(&key, &value)
			if err != nil {
				log.Error("Error scanning grid key data")
				return err
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

		if config.compressionType == ZLIB {
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
