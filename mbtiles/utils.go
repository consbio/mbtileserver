package mbtiles

import (
	"fmt"
	"strconv"
	"strings"
)

// stringToFloats converts a commma-delimited string of floats to a slice of
// float64 and returns it and the first error that was encountered.
func stringToFloats(str string) ([]float64, error) {
	split := strings.Split(str, ",")
	var out []float64
	for _, v := range split {
		value, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return out, fmt.Errorf("could not parse %q to floats: %v", str, err)
		}
		out = append(out, value)
	}
	return out, nil
}
