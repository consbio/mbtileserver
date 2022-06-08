package handlers

import (
	"io/fs"
	"testing"
)

func Test_StaticAssets(t *testing.T) {
	root := "templates/static/dist"
	assetsFS, _ := fs.Sub(staticAssets, root)

	expected := []string{
		"index.js",
		"index.css",
	}

	// verify that expected files are present in the embedded filesystem
	for _, filename := range expected {
		if _, err := fs.ReadFile(assetsFS, filename); err != nil {
			t.Error("Could not find expected file in embedded filesystem:", root+filename)
			continue
		}
	}

	// verify that only these files are present
	entries, err := fs.ReadDir(assetsFS, ".")
	if err != nil {
		t.Error("Could not list files in embedded filesystem:", root)
	}
	if len(entries) != len(expected) {
		t.Errorf("Got unexpected number of entries in embedded filesystem (%s): %v, expected: %v", root, len(entries), len(expected))
	}
}
