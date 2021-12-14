package handlers

import "testing"

func Test_SHA1ID(t *testing.T) {
	tests := []struct {
		path string
		id   string
	}{
		{path: "geography-class-jpg.mbtiles", id: "TL6FA75kn46-zpifOfLsLesTLrY"},
		{path: "geography-class-png.mbtiles", id: "8YAiWw7-AjNp9pPppb-ksNM1RMg"},
		{path: "geography-class-webp.mbtiles", id: "-8JT6wT5OSPbXvbONZGCVWOQb1g"},
		{path: "world_cities.mbtiles", id: "L7qsx7KIKNf96L3KKXovAsWV4uE"},
	}

	for _, tc := range tests {
		filename := "./testdata/" + tc.path
		id := SHA1ID(filename)

		if id != tc.id {
			t.Error("SHA1ID:", id, "not expected value:", tc.id, "for path:", filename)
			continue
		}
	}
}

func Test_RelativePathID(t *testing.T) {
	tests := []struct {
		path string
		id   string
	}{
		{path: "geography-class-jpg.mbtiles", id: "geography-class-jpg"},
		{path: "geography-class-png.mbtiles", id: "geography-class-png"},
		{path: "geography-class-webp.mbtiles", id: "geography-class-webp"},
		{path: "world_cities.mbtiles", id: "world_cities"},
	}

	for _, tc := range tests {
		filename := "./testdata/" + tc.path
		id, err := RelativePathID(filename, "./testdata")
		if err != nil {
			t.Error("Could not create RelativePathID for path:", filename)
			continue
		}

		if id != tc.id {
			t.Error("RelativePathID:", id, "not expected value:", tc.id, "for path:", filename)
			continue
		}
	}
}
