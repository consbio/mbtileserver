package handlers

import (
	"math"
	"strings"
	"testing"
)

func Test_TileCoordFromString(t *testing.T) {
	tests := []struct {
		path string
		ext  string
		z    int64
		x    int64
		y    int64
	}{
		{path: "0/0/0", ext: "", z: 0, x: 0, y: 0},
		{path: "10/1/2", ext: "", z: 10, x: 1, y: 2},
		{path: "0/0/0.png", ext: ".png", z: 0, x: 0, y: 0},
		{path: "0/0/0.jpg", ext: ".jpg", z: 0, x: 0, y: 0},
		{path: "0/0/0.webp", ext: ".webp", z: 0, x: 0, y: 0},
		{path: "0/0/0.pbf", ext: ".pbf", z: 0, x: 0, y: 0},
	}

	for _, tc := range tests {
		// split the tile path for easier testing
		pcs := strings.Split(tc.path, "/")
		l := len(pcs)
		zIn, xIn, yIn := pcs[l-3], pcs[l-2], pcs[l-1]

		coord, ext, err := tileCoordFromString(zIn, xIn, yIn)
		if err != nil {
			t.Error("Could not extract tile coordinate from tile path:", tc.path, err)
			continue
		}

		if ext != tc.ext {
			t.Error("tileCoordFromString returned unexpected extension:", ext, "expected:", tc.ext)
			continue
		}
		if coord.z != tc.z {
			t.Error("tileCoordFromString returned unexpected z:", coord.z, "expected:", tc.z)
			continue
		}
		if coord.x != tc.x {
			t.Error("tileCoordFromString returned unexpected x:", coord.x, "expected:", tc.x)
			continue
		}
		if coord.y != tc.y {
			t.Error("tileCoordFromString returned unexpected y:", coord.y, "expected:", tc.y)
			continue
		}
	}
}

func Test_TileCoordFromString_Invalid(t *testing.T) {
	tests := []struct {
		path string
		err  string
	}{
		{path: "0/1/2", err: "out of bounds for zoom level"},
		{path: "0/0/a", err: "cannot parse y coordinate"},
		{path: "0/0/a.png", err: "cannot parse y coordinate"},
		{path: "0/0/0.foo.bar", err: "cannot parse y coordinate"},
		{path: "a/0/0", err: "cannot parse zoom level"},
		{path: "0/a/0", err: "cannot parse x coordinate"},
	}

	for _, tc := range tests {
		// split the tile path for easier testing
		pcs := strings.Split(tc.path, "/")
		l := len(pcs)
		zIn, xIn, yIn := pcs[l-3], pcs[l-2], pcs[l-1]

		_, _, err := tileCoordFromString(zIn, xIn, yIn)
		if err == nil {
			t.Error("tileCoordFromString did not raise expected error for invalid tile path:", tc.path)
			continue
		}
		if !strings.Contains(err.Error(), tc.err) {
			t.Error("tileCoordFromString returned unexpected error message:", err, "expected:", tc.err)
			continue
		}
	}
}

func Test_CalcScaleResolution(t *testing.T) {
	zoom0resolution := (2 * earthCircumference / 256) / (1 << 0)
	zoom2resolution := (2 * earthCircumference / 256) / (1 << 2)

	tests := []struct {
		zoom       int
		dpi        uint8
		scale      float64
		resolution float64
	}{
		{zoom: 0, dpi: 0, scale: 0, resolution: zoom0resolution},
		{zoom: 0, dpi: 100, scale: 100 * 39.37 * zoom0resolution, resolution: zoom0resolution},
		{zoom: 2, dpi: 0, scale: 0, resolution: zoom2resolution},
		{zoom: 2, dpi: 100, scale: 100 * 39.37 * zoom2resolution, resolution: zoom2resolution},
	}

	tolerance := 1e-9

	for _, tc := range tests {
		scale, resolution := calcScaleResolution(tc.zoom, tc.dpi)
		if math.Abs(resolution-tc.resolution) > tolerance {
			t.Error("calcScaleResolution returned unexpected resolution:", resolution, "expected:", tc.resolution)
			continue
		}
		if math.Abs(scale-tc.scale) > tolerance {
			t.Error("calcScaleResolution returned unexpected scale:", scale, "expected:", tc.scale)
			continue
		}
	}
}
