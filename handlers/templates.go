package handlers

//go:generate go run -tags=dev assets_generate.go

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"net/http"

	"github.com/shurcooL/httpfs/html/vfstemplate"
)

var templates *template.Template

func init() {
	// load templates
	templates = template.New("_base_")
	vfstemplate.ParseFiles(Assets, templates, "map.html", "map_gl.html")
}

// executeTemplates first tries to find the template with the given name for
// the ServiceSet. If that fails because it is not available, an HTTP status
// Internal Server Error is returned.
func executeTemplate(w http.ResponseWriter, name string, data interface{}) (int, error) {
	t := templates.Lookup(name)
	if t == nil {
		return http.StatusInternalServerError, fmt.Errorf("template not found %q", name)
	}
	buf := &bytes.Buffer{}
	err := t.Execute(buf, data)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	_, err = io.Copy(w, buf)
	return http.StatusOK, err
}
