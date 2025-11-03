package main

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"
)

func (app *App) admLoadTemplates() error {
	pages, err := filepath.Glob("admin_templates/pages/*.tmpl")
	if err != nil {
		return fmt.Errorf("load admin admin_templates: %w", err)
	}

	app.admTmpl = make(map[string]*template.Template)

	for _, page := range pages {
		name := filepath.Base(page)
		base := "admin_templates/base.tmpl"
		t, err := template.New("").ParseFiles(base, page)
		if err != nil {
			return fmt.Errorf("parse admin_templates %#v and %#v: %w", base, page, err)
		}
		_, err = t.ParseGlob("admin_templates/partials/*.tmpl")
		if err != nil {
			return fmt.Errorf("parse partials while loading %#v: %w", page, err)
		}
		app.admTmpl[name] = t
		// fmt.Printf("base=%s, page=%s, name=%s\n", base, page, name)
	}

	return nil
}

func (app *App) admRenderTemplate(w http.ResponseWriter, r *http.Request, name string, data any, extra ...slog.Attr) error {
	t, ok := app.admTmpl[name+".tmpl"]
	if !ok {
		err := fmt.Errorf("unknown template %s", name)
		app.logError(r, logMsgTemplatesRenderMissing, slog.String("template", name))
		return err
	}

	if err := t.ExecuteTemplate(w, "base", struct {
		ActiveTab string
		Data      any
	}{
		ActiveTab: name,
		Data:      data,
	}); err != nil {
		app.logError(r, logMsgTemplatesRenderError, append(extra, slog.String("template", name), slog.Any("error", err))...)
		return err
	}

	app.logInfo(r, logMsgTemplatesRenderSuccess, append(extra, slog.String("template", name))...)
	return nil
}
