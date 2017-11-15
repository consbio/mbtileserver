// +build dev

package main

import (
	"net/http"

	"github.com/consbio/mbtileserver/handlers"
)

// This is an temporary vehicle until we can stop serving the assets from the
// main package (that is, until https://github.com/consbio/mbtileserver/pull/47
// is merged). Afterwards, it is safe to remove this whole file.
func init() {
	handlers.Assets = http.Dir("handlers/templates")
}
