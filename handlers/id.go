package handlers

import (
	"crypto/sha1"
	"encoding/base64"
	"path/filepath"
)

type IDGenerator func(filename, baseDir string) (string, error)

// SHA1ID generates a URL safe base64 encoded SHA1 hash of the filename.
func SHA1ID(filename string) string {
	// generate IDs from hash of full file path
	filenameHash := sha1.Sum([]byte(filename))
	return base64.RawURLEncoding.EncodeToString(filenameHash[:])
}

// RelativePathID returns a relative path from the basedir to the filename.
func RelativePathID(filename, baseDir string) (string, error) {
	subpath, err := filepath.Rel(baseDir, filename)
	if err != nil {
		return "", err
	}
	subpath = filepath.ToSlash(subpath)
	return subpath[:len(subpath)-len(filepath.Ext(filename))], nil
}
