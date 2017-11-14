package handlers

import (
	"html/template"
	"io/ioutil"
	"path"
	"strings"
)

// TemplatesFromAssets returns the HTML templates from the assets filesystem.
// It has a signature that allows it to be used with templates.Must.
func TemplatesFromAssets() (*template.Template, error) {
	t := template.New("_base_")
	d, err := Assets.Open("/")
	if err != nil {
		return t, err
	}
	files, err := d.Readdir(0)
	if err != nil {
		return t, err
	}
	for _, file := range files {
		name := file.Name()
		ext := strings.ToLower(path.Ext(name))
		if ext != ".html" || file.IsDir() {
			continue
		}
		name = name[:len(name)-len(ext)]
		if _, err := tmplFromAssets(t, name); err != nil {
			return t, err
		}
	}
	return t, nil
}

// tmplFromAssets returns the HTML templates from the assets filesystem.
func tmplFromAssets(t *template.Template, name string) (*template.Template, error) {
	f, err := Assets.Open("/" + name + ".html")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	t, err = t.New(name).Parse(string(buf))
	if err != nil {
		return nil, err
	}
	return t, nil
}
