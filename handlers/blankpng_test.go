package handlers

import (
	"encoding/base64"
	"testing"
)

func Test_BlankPNG(t *testing.T) {
	blankPNG := BlankPNG()
	b64 := base64.StdEncoding.EncodeToString(blankPNG)
	expected := "iVBORw0KGgoAAAANSUhEUgAAAQAAAAEAAQMAAABmvDolAAAAA1BMVEUAAACnej3aAAAAAXRSTlMAQObYZgAAAB9JREFUaN7twQENAAAAwiD7pzbHN2AAAAAAAAAAAHEHIQAAAadXKdcAAAAASUVORK5CYII="
	if b64 != expected {
		t.Error("BlankPNG returned unexpected value (base64):", b64)
	}
}
