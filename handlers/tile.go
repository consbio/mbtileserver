package handlers

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	earthRadius              = 6378137.0
	earthCircumference       = math.Pi * earthRadius
	initialResolution        = 2 * earthCircumference / 256
	dpi                uint8 = 96
)

type tileCoord struct {
	z, x, y int64
}

// tileCoordFromString parses and returns tileCoord coordinates and an optional
// extension from the three parameters. The parameter z is interpreted as the
// web mercator zoom level, it is supposed to be an unsigned integer that will
// fit into 8 bit. The parameters x and y are interpreted as longitude and
// latitude tile indices for that zoom level, both are supposed be integers in
// the integer interval [0,2^z). Additionally, y may also have an optional
// filename extension (e.g. "42.png") which is removed before parsing the
// number, and returned, too. In case an error occurred during parsing or if the
// values are not in the expected interval, the returned error is non-nil.
func tileCoordFromString(z, x, y string) (tc tileCoord, ext string, err error) {
	if tc.z, err = strconv.ParseInt(z, 10, 64); err != nil {
		err = fmt.Errorf("cannot parse zoom level: %v", err)
		return
	}
	const (
		errMsgParse = "cannot parse %s coordinate axis: %v"
		errMsgOOB   = "%s coordinate (%d) is out of bounds for zoom level %d"
	)
	if tc.x, err = strconv.ParseInt(x, 10, 64); err != nil {
		err = fmt.Errorf(errMsgParse, "first", err)
		return
	}
	if tc.x >= (1 << tc.z) {
		err = fmt.Errorf(errMsgOOB, "x", tc.x, tc.z)
		return
	}
	s := y
	if l := strings.LastIndex(s, "."); l >= 0 {
		s, ext = s[:l], s[l:]
	}
	if tc.y, err = strconv.ParseInt(s, 10, 64); err != nil {
		err = fmt.Errorf(errMsgParse, "y", err)
		return
	}
	if tc.y >= (1 << tc.z) {
		err = fmt.Errorf(errMsgOOB, "y", tc.y, tc.z)
		return
	}
	return
}

func calcScaleResolution(zoomLevel int, dpi uint8) (float64, float64) {
	var denom = 1 << zoomLevel
	resolution := initialResolution / float64(denom)
	scale := float64(dpi) * 39.37 * resolution // 39.37 in/m
	return scale, resolution
}
