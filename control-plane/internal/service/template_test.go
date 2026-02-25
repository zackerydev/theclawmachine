package service

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func setupTestTemplates(t *testing.T) (cleanup func()) {
	t.Helper()

	// Create temp dir structure mimicking the expected layout
	tmpDir := t.TempDir()

	layoutDir := filepath.Join(tmpDir, "templates", "layouts")
	pageDir := filepath.Join(tmpDir, "templates", "pages")
	if err := os.MkdirAll(layoutDir, 0o755); err != nil {
		t.Fatalf("mkdir layout dir: %v", err)
	}
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		t.Fatalf("mkdir page dir: %v", err)
	}

	// Layout template that defines layout and content blocks
	layoutHTML := `{{define "layout"}}<!DOCTYPE html><html><body>{{template "content" .}}</body></html>{{end}}`
	if err := os.WriteFile(filepath.Join(layoutDir, "index.html"), []byte(layoutHTML), 0o644); err != nil {
		t.Fatalf("write layout template: %v", err)
	}

	// Page template
	pageHTML := `{{define "test-page"}}{{template "layout" .}}{{end}}{{define "content"}}<h1>{{.Title}}</h1>{{end}}`
	if err := os.WriteFile(filepath.Join(pageDir, "test-page.html"), []byte(pageHTML), 0o644); err != nil {
		t.Fatalf("write test-page template: %v", err)
	}

	// Another page for listing
	botsHTML := `{{define "bots"}}{{template "layout" .}}{{end}}{{define "content"}}<ul>{{range .Releases}}<li>{{.Name}}</li>{{end}}</ul>{{end}}`
	if err := os.WriteFile(filepath.Join(pageDir, "bots.html"), []byte(botsHTML), 0o644); err != nil {
		t.Fatalf("write bots template: %v", err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	return func() {
		if err := os.Chdir(origDir); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}
}

func TestNewTemplateService(t *testing.T) {
	cleanup := setupTestTemplates(t)
	defer cleanup()

	svc, err := NewTemplateService(false)
	if err != nil {
		t.Fatalf("NewTemplateService returned error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil TemplateService")
	}
	if svc.dev {
		t.Fatal("expected dev=false")
	}
	if len(svc.pages) == 0 {
		t.Fatal("expected pages to be loaded")
	}
}

func TestTemplateService_Render_FullPage(t *testing.T) {
	cleanup := setupTestTemplates(t)
	defer cleanup()

	svc, err := NewTemplateService(false)
	if err != nil {
		t.Fatalf("NewTemplateService returned error: %v", err)
	}
	var buf bytes.Buffer

	data := struct{ Title string }{Title: "Hello"}
	err = svc.Render(&buf, "test-page", data, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	if !bytes.Contains([]byte(output), []byte("<h1>Hello</h1>")) {
		t.Fatalf("expected rendered title, got: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte("<!DOCTYPE html>")) {
		t.Fatalf("expected full layout, got: %s", output)
	}
}

func TestTemplateService_Render_HTMXPartial(t *testing.T) {
	cleanup := setupTestTemplates(t)
	defer cleanup()

	svc, err := NewTemplateService(false)
	if err != nil {
		t.Fatalf("NewTemplateService returned error: %v", err)
	}
	var buf bytes.Buffer

	data := struct{ Title string }{Title: "Partial"}
	err = svc.Render(&buf, "test-page", data, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// HTMX should render only the "content" block
	if !bytes.Contains([]byte(output), []byte("<h1>Partial</h1>")) {
		t.Fatalf("expected content block, got: %s", output)
	}
}

func TestTemplateService_Render_NotFound(t *testing.T) {
	cleanup := setupTestTemplates(t)
	defer cleanup()

	svc, err := NewTemplateService(false)
	if err != nil {
		t.Fatalf("NewTemplateService returned error: %v", err)
	}
	var buf bytes.Buffer

	err = svc.Render(&buf, "nonexistent", nil, false)
	if err == nil {
		t.Fatal("expected missing template error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got: %v", err)
	}
}

func TestTemplateService_DevMode_ReloadsTemplates(t *testing.T) {
	cleanup := setupTestTemplates(t)
	defer cleanup()

	svc, err := NewTemplateService(true)
	if err != nil {
		t.Fatalf("NewTemplateService returned error: %v", err)
	}
	if !svc.dev {
		t.Fatal("expected dev=true")
	}

	var buf bytes.Buffer
	data := struct{ Title string }{Title: "Dev"}
	err = svc.Render(&buf, "test-page", data, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("<h1>Dev</h1>")) {
		t.Fatalf("expected rendered content, got: %s", buf.String())
	}
}

func TestTemplateService_DevMode_ConcurrentRender(t *testing.T) {
	cleanup := setupTestTemplates(t)
	defer cleanup()

	svc, err := NewTemplateService(true)
	if err != nil {
		t.Fatalf("NewTemplateService returned error: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for range 20 {
		wg.Go(func() {
			var buf bytes.Buffer
			if err := svc.Render(&buf, "test-page", struct{ Title string }{Title: "Concurrent"}, false); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("unexpected render error: %v", err)
	}
}
