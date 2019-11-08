package mbtiles_test

import (
	"testing"

	"github.com/consbio/mbtileserver/mbtiles"
)

func Test_TileFormat_String(t *testing.T) {
	var conditions = []struct {
		in  mbtiles.TileFormat
		out string
	}{
		{mbtiles.UNKNOWN, ""},
		{mbtiles.PNG, "png"},
		{mbtiles.JPG, "jpg"},
		{mbtiles.PNG, "png"},
		{mbtiles.PBF, "pbf"},
		{mbtiles.WEBP, "webp"},
	}

	for _, condition := range conditions {
		if condition.in.String() != condition.out {
			t.Errorf("%q.String() => %q, expected %q", condition.in, condition.in.String(), condition.out)
		}
	}
}

func Test_TileFormat_ContentType(t *testing.T) {
	var conditions = []struct {
		in  mbtiles.TileFormat
		out string
	}{
		{mbtiles.UNKNOWN, ""},
		{mbtiles.PNG, "image/png"},
		{mbtiles.JPG, "image/jpeg"},
		{mbtiles.PNG, "image/png"},
		{mbtiles.PBF, "application/x-protobuf"},
		{mbtiles.WEBP, "image/webp"},
	}

	for _, condition := range conditions {
		if condition.in.ContentType() != condition.out {
			t.Errorf("%q.ContentType() => %q, expected %q", condition.in, condition.in.ContentType(), condition.out)
		}
	}
}

func Test_NewDB(t *testing.T) {
	// valid tileset should not raise error
	_, err := mbtiles.NewDB("./testdata/geography-class-png.mbtiles")
	if err != nil {
		t.Errorf("Valid tileset could not be opened: %q", err)
	}

	// invalid tileset should raise error
	_, err = mbtiles.NewDB("./testdata/invalid.mbtiles")
	if err == nil {
		t.Error("Invalid tileset did not raise validation error")
	}
}
