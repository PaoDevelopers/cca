package main

import (
	"fmt"
	"html/template"
	"path/filepath"
)

func LoadTemplates(dir string) (*template.Template, error) {
	pattern := filepath.Join(dir, "*.html")
	tmpl, err := template.ParseGlob(pattern)
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return tmpl, nil
}
