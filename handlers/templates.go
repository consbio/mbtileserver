package handlers

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
)

//go:embed templates/*.html
var templateAssets embed.FS

var templates *template.Template

func init() {
	// load templates
	templatesFS, err := fs.Sub(templateAssets, "templates")
	if err != nil {
		fmt.Errorf("Error getting embedded path for templates: %w", err)
		panic(err)
	}

	t, err := template.ParseFS(templatesFS, "map.html")
	if err != nil {
		fmt.Errorf("Could not resolve template: %w", err)
		panic(err)
	}
	templates = t
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
