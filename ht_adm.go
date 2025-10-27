package main

import (
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
)

func (app *App) admLoadTemplates() error {
	pages, err := filepath.Glob("admin_templates/pages/*.tmpl")
	if err != nil {
		return fmt.Errorf("Cannot load admin admin_templates: %w", err)
	}

	app.admTmpl = make(map[string]*template.Template)

	for _, page := range pages {
		name := filepath.Base(page)
		base := "admin_templates/base.tmpl"
		t, err := template.New("").ParseFiles(base, page)
		if err != nil {
			return fmt.Errorf("Cannot parse admin_templates %#v and %#v: %w", base, page, err)
		}
		_, err = t.ParseGlob("admin_templates/partials/*.tmpl")
		if err != nil {
			return fmt.Errorf("Cannot parse partials while loading %#v: %w", page, err)
		}
		app.admTmpl[name] = t
		// fmt.Printf("base=%s, page=%s, name=%s\n", base, page, name)
	}

	return nil
}

func (app *App) admRenderTemplate(w http.ResponseWriter, name string, data any) {
	t, ok := app.admTmpl[name+".tmpl"]
	if !ok {
		panic("unknown template " + name)
	}

	t.ExecuteTemplate(w, "base", data)
}
