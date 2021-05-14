package mbtiles_test

import (
	"testing"

	"github.com/consbio/mbtileserver/mbtiles"
)

func Test_ListDBs(t *testing.T) {
	var expected = []string{
		"testdata/geography-class-jpg.mbtiles",
		"testdata/geography-class-png.mbtiles",
		"testdata/geography-class-webp.mbtiles",
		"testdata/world_cities.mbtiles",
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
	filename := "./testdata/geography-class-png.mbtiles"
	db, err := mbtiles.NewDB(filename)
	if err != nil {
		t.Errorf("Valid tileset could not be opened: %q", err)
	}

	// closing mbtiles file should not raise error
	err = db.Close()
	if err != nil {
		t.Error("Closing mbtiles file raised error")
	}

	// nonexistent tileset should raise error
	_, err = mbtiles.NewDB("does_not_exist.mbtiles")
	if err == nil {
		t.Error("Nonexistent tileset did not raise validation error")
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

func Test_Metadata(t *testing.T) {
	filename := "./testdata/geography-class-png.mbtiles"
	db, err := mbtiles.NewDB(filename)

	expectedMetadata := map[string]interface{}{
		"name":        "Geography Class",
		"description": "One of the example maps that comes with TileMill - a bright & colorful world map that blends retro and high-tech with its folded paper texture and interactive flag tooltips. ",
		"minzoom":     0,
		"maxzoom":     1,
	}
	metadata, err := db.ReadMetadata()
	if err != nil {
		t.Error("Error raised when reading metadata")
	}
	for key, expectedValue := range expectedMetadata {
		value, ok := metadata[key]
		if !ok {
			t.Errorf("Metadata missing expected key: %q", key)
		}
		if value != expectedValue {
			t.Errorf("Metadata value '%v' does not match expected value '%v'", value, expectedValue)
		}
	}
	var expectedBounds = []float64{-180, -85.0511, 180, 85.0511}
	bounds, ok := metadata["bounds"]
	if !ok {
		t.Error("Metadata missing expected key: bounds")
	}
	boundsValues := bounds.([]float64)
	if len(boundsValues) != 4 {
		t.Error("Metadata bounds not expected length")
	}
	for i, expectedValue := range expectedBounds {
		if boundsValues[i] != expectedValue {
			t.Errorf("Metadata bounds does not have expected values.  Found: %v expected: %v", boundsValues[i], expectedValue)
		}
	}
}

func Test_ReadTile(t *testing.T) {
	filename := "./testdata/geography-class-png.mbtiles"
	db, err := mbtiles.NewDB(filename)

	// valid tile should return data
	var data []byte
	err = db.ReadTile(0, 0, 0, &data)
	if err != nil {
		t.Error("Error raised when reading valid tile")
	}
	if data == nil {
		t.Error("Did not read tile data")
	}

	// missing tile should return nil
	err = db.ReadTile(10, 0, 0, &data)
	if err != nil {
		t.Error("Error raised when reading missing tile")
	}
	if data != nil {
		t.Error("Tile data should have been empty for missing tile")
	}
}

func Test_ReadGrid(t *testing.T) {
	filename := "./testdata/geography-class-png.mbtiles"
	db, err := mbtiles.NewDB(filename)

	// valid UTF grid should return data
	var data []byte
	err = db.ReadGrid(0, 0, 0, &data)
	if err != nil {
		t.Error("Error raised when reading valid UTF grid")
	}
	if data == nil {
		t.Error("Did not read UTF grid data")
	}

	// missing UTF grid should return nil
	err = db.ReadGrid(10, 0, 0, &data)
	if err != nil {
		t.Error("Error raised when reading missing UTF grid")
	}
	if data != nil {
		t.Error("Tile data should have been empty for missing UTF grid")
	}
}

func Test_Property_Methods(t *testing.T) {
	filename := "./testdata/geography-class-png.mbtiles"
	db, err := mbtiles.NewDB(filename)
	if err != nil {
		t.Errorf("Valid tileset could not be opened: %q", err)
	}

	if db.Filename() != filename {
		t.Errorf("Unexpected filename: %q => %q", filename, db.Filename())
	}

	if db.TileFormat() != mbtiles.PNG {
		t.Errorf("TileFormat %v is not expected value", db.TileFormat())
	}

	if db.TileFormatString() != "png" {
		t.Errorf("TileFormatString %q is not expected value 'png'", db.TileFormatString())
	}

	if db.ContentType() != "image/png" {
		t.Errorf("ContentType %q is not expected value 'image/png'", db.ContentType())
	}

	if !db.HasUTFGrid() {
		t.Error("Tileset with UTF grids claims to not have UTF grids")
	}

	if db.UTFGridCompression() != mbtiles.ZLIB {
		t.Errorf("UTF grid compression %v is not expected value", db.UTFGridCompression())
	}

	filename = "./testdata/world_cities.mbtiles"
	db, err = mbtiles.NewDB(filename)
	if err != nil {
		t.Errorf("Valid tileset could not be opened: %q", err)
	}

	if db.TileFormat() != mbtiles.PBF {
		t.Errorf("TileFormat %v is not expected value", db.TileFormat())
	}

	if db.TileFormatString() != "pbf" {
		t.Errorf("TileFormatString %q is not expected value 'pbf'", db.TileFormatString())
	}

	if db.ContentType() != "application/x-protobuf" {
		t.Errorf("ContentType %q is not expected value 'application/x-protobuf'", db.ContentType())
	}

	if db.HasUTFGrid() {
		t.Error("Tileset with no UTF grids claims to have UTF grids")
	}

}

// filename := "./testdata/geography-class-png.mbtiles"
// db, err := mbtiles.NewDB(filename)
