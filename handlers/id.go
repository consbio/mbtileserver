package handlers

import (
	"crypto/sha1"
	"encoding/base64"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3" // import sqlite3 driver
)

// IDGenerator is an interface for functions that generate IDs given
// mbtiles filename and other optional arguments that vary by generator
// type IDGenerator func(string, ...interface{}) (string, error)
type IDGenerator func(string) (string, error)

// SHA1IDGenerator generates an ID from the mbtiles file.
// This is a URL safe base64 encoded SHA1 hash of the filename.
func SHA1IDGenerator(filename string) (string, error) {
	// generate IDs from hash of full file path
	filenameHash := sha1.Sum([]byte(filename))
	return base64.RawURLEncoding.EncodeToString(filenameHash[:]), nil
}

// CreateRelativePathIDGenerator creates an IDGenerator from the relative path of the
// mbtiles file within a baseDirectory
func CreateRelativePathIDGenerator(baseDir string) IDGenerator {
	return func(filename string) (string, error) {

		subpath, err := filepath.Rel(baseDir, filename)
		if err != nil {
			return "", err
		}
		subpath = filepath.ToSlash(subpath)
		return subpath[:len(subpath)-len(filepath.Ext(filename))], nil
	}
}

// // SHA1IDGenerator generates an ID from the mbtiles file.
// // This is a URL safe base64 encoded SHA1 hash of the filename.
// func SHA1IDGenerator(filename string, args ...interface{}) (string, error) {
// 	// generate IDs from hash of full file path
// 	filenameHash := sha1.Sum([]byte(filename))
// 	return base64.RawURLEncoding.EncodeToString(filenameHash[:]), nil
// }

// // RelativePathIDGenerator generates an ID from the relative path of the
// // mbtiles file within a baseDirectory
// // Usage: RelativePathIDGenerator(mbtilesFilename, baseDir)
// func RelativePathIDGenerator(filename string, args ...interface{}) (string, error) {
// 	var baseDir string
// 	if len(args) > 0 {
// 		baseDir = args[0].(string)
// 	}
// 	subpath, err := filepath.Rel(baseDir, filename)
// 	if err != nil {
// 		return "", err
// 	}
// 	subpath = filepath.ToSlash(subpath)
// 	return subpath[:len(subpath)-len(filepath.Ext(filename))], nil

// }
