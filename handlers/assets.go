// +build dev

package handlers

import "net/http"

// Assets contains project assets.
var Assets http.FileSystem = http.Dir("templates")
