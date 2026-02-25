package service

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
)

type TemplateService struct {
	mu       sync.RWMutex
	pages    map[string]*template.Template
	partials *template.Template
	dev      bool
}

var funcMap = template.FuncMap{
	"botConfigsJSON": func(v any) template.JS {
		b, _ := json.Marshal(v)
		return template.JS(b)
	},
	"formatValue": func(v any) string {
		switch val := v.(type) {
		case string:
			return val
		case bool:
			if val {
				return "true"
			}
			return "false"
		case nil:
			return "—"
		default:
			b, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return fmt.Sprintf("%v", v)
			}
			return string(b)
		}
	},
}

func loadTemplates() (map[string]*template.Template, error) {
	pages := make(map[string]*template.Template)

	layoutFiles, err := filepath.Glob("templates/layouts/*.html")
	if err != nil {
		return nil, fmt.Errorf("glob layouts: %w", err)
	}
	partialFiles, err := filepath.Glob("templates/partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("glob partials: %w", err)
	}
	pageFiles, err := filepath.Glob("templates/pages/*.html")
	if err != nil {
		return nil, fmt.Errorf("glob pages: %w", err)
	}
	if len(pageFiles) == 0 {
		return nil, fmt.Errorf("no page templates found under templates/pages")
	}

	baseFiles := append(layoutFiles, partialFiles...)

	for _, page := range pageFiles {
		files := append(baseFiles, page)
		tmpl, err := template.New(filepath.Base(page)).Funcs(funcMap).ParseFiles(files...)
		if err != nil {
			return nil, fmt.Errorf("parse page template %q: %w", page, err)
		}

		name := filepath.Base(page)
		name = name[:len(name)-len(filepath.Ext(name))]
		pages[name] = tmpl
	}

	return pages, nil
}

func loadPartials() (*template.Template, error) {
	files, err := filepath.Glob("templates/partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("glob partials: %w", err)
	}
	if len(files) == 0 {
		return template.New("empty").Parse("")
	}
	tmpl, err := template.New("partials").Funcs(funcMap).ParseFiles(files...)
	if err != nil {
		return nil, fmt.Errorf("parse partial templates: %w", err)
	}
	return tmpl, nil
}

func NewTemplateService(dev bool) (*TemplateService, error) {
	pages, err := loadTemplates()
	if err != nil {
		return nil, err
	}
	partials, err := loadPartials()
	if err != nil {
		return nil, err
	}

	return &TemplateService{
		pages:    pages,
		partials: partials,
		dev:      dev,
	}, nil
}

func (t *TemplateService) reload() error {
	pages, err := loadTemplates()
	if err != nil {
		return err
	}
	partials, err := loadPartials()
	if err != nil {
		return err
	}

	t.mu.Lock()
	t.pages = pages
	t.partials = partials
	t.mu.Unlock()
	return nil
}

func (t *TemplateService) Render(w io.Writer, name string, data any, isHTMX bool) error {
	if t.dev {
		if err := t.reload(); err != nil {
			slog.Error("error reloading templates", "error", err)
			return err
		}
	}

	t.mu.RLock()
	tmpl, ok := t.pages[name]
	partials := t.partials
	t.mu.RUnlock()

	// Try page templates first
	if ok {
		renderName := name
		if isHTMX {
			renderName = "content"
		}
		err := tmpl.ExecuteTemplate(w, renderName, data)
		if err != nil {
			slog.Error("error rendering template", "template", name, "error", err)
			return err
		}
		return nil
	}

	// Fall back to partials (HTMX fragments)
	if partials != nil && partials.Lookup(name) != nil {
		err := partials.ExecuteTemplate(w, name, data)
		if err != nil {
			slog.Error("error rendering partial", "template", name, "error", err)
			return err
		}
		return nil
	}

	err := fmt.Errorf("template %q not found", name)
	slog.Error("template not found", "template", name)
	return err
}
