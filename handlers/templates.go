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
	fis, err := d.Readdir(0)
	if err != nil {
		return t, err
	}
	for _, fi := range fis {
		n := fi.Name()
		e := strings.ToLower(path.Ext(n))
		if e != ".html" || fi.IsDir() {
			continue
		}
		n = n[:len(n)-len(e)]
		if _, err := tmplFromAssets(t, n); err != nil {
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
	n, err := t.New(name).Parse(string(buf))
	if err != nil {
		return nil, err
	}
	return n, nil
}
