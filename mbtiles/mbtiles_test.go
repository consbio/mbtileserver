package mbtiles_test

import (
	"testing"

	"github.com/consbio/mbtileserver/mbtiles"
)

func Test_ListDBs(t *testing.T) {
	var expected = []string{
		"testdata/geography-class-jpg.mbtiles",
		"testdata/geography-class-png.mbtiles",
		"testdata/world_cities.mbtiles",
		"testdata/openstreetmap/open-streets-dc.mbtiles",
	}

	filenames, err := mbtiles.ListDBs("./testdata")
	if err != nil {
		t.Error("Could not list mbtiles files in testdata directory")
	}

	found := 0

	for _, expectedFilename := range expected {
		for _, filename := range filenames {
			if filename == expectedFilename {
				found += 1
			}
		}
	}
	if found != len(expected) {
		t.Error("Did not list all expected mbtiles files in testdata directory")
	}

	// invalid directory should raise an error
	_, err = mbtiles.ListDBs("./invalid")
	if err == nil {
		t.Error("Did not fail to list mbtiles in invalid directory")
	}

	// valid directory with no mbtiles should be empty
	filenames, err = mbtiles.ListDBs("../handlers")
	if err != nil {
		t.Error("Failed when listing valid directory")
	}

	if len(filenames) != 0 {
		t.Error("Directory with no mbtiles files did not return 0 mbtiles files")
	}
}

func Test_stringToFloats(t *testing.T) {
	var conditions = []struct {
		in  string
		out []float64
	}{
		{"0", []float64{0}},
		{"0,1.5", []float64{0, 1.5}},
		{"0, 1.5, 123.456 ", []float64{0, 1.5, 123.456}},
	}
	for _, condition := range conditions {
		result, err := mbtiles.StringToFloats(condition.in)
		if err != nil {
			t.Errorf("Unexpected error in stringToFloats: %q", err)
		}
		if len(result) != len(condition.out) {
			t.Errorf("Failed stringToFloats: %q => %v, expected %v", condition.in, result, condition.out)
		}
		for i, expected := range condition.out {
			if expected != condition.out[i] {
				t.Errorf("Failed stringToFloats: %q => %v, expected %v", condition.in, result, condition.out)
			}
		}
	}

	var invalids = []string{
		"", "a", "1,a", "1, ",
	}
	for _, invalid := range invalids {
		_, err := mbtiles.StringToFloats(invalid)
		if err == nil {
			t.Errorf("stringToFloats did not fail as expected: %q", invalid)
		}
	}

}

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

	// invalid tile image format should raise error
	_, err = mbtiles.NewDB("./testdata/invalid-tile-format.mbtiles")
	if err == nil {
		t.Error("Invalid tileset did not raise validation error")
	}
}
