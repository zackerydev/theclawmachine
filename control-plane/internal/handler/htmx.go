package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
)

const maxRequestBodySize int64 = 1 << 20 // 1 MB

// isHTMX reports whether the request was initiated by HTMX.
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// htmxRedirectOrStatus sends an HX-Redirect header for HTMX requests, or a plain status code otherwise.
func htmxRedirectOrStatus(w http.ResponseWriter, r *http.Request, url string, code int) {
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", url)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// For regular browser form posts, empty 201/204 responses render a blank page.
	// Redirect to the provided URL so non-HTMX fallback keeps the UI usable.
	if url != "" {
		if code >= 300 && code < 400 {
			http.Redirect(w, r, url, code)
			return
		}
		// Keep non-HTMX API semantics for non-POST methods (e.g. PUT /config).
		if r.Method == http.MethodPost {
			switch code {
			case http.StatusOK, http.StatusCreated, http.StatusNoContent:
				http.Redirect(w, r, url, http.StatusSeeOther)
				return
			}
		}
	}

	w.WriteHeader(code)
}

// parseRequest decodes a request body into dst (pointer to struct).
// Supports both JSON (application/json) and form data (default).
// For form data, struct fields are matched by their json tag or lowercase field name.
func parseRequest(r *http.Request, dst any) error {
	ct := r.Header.Get("Content-Type")
	isJSON := strings.HasPrefix(ct, "application/json")

	// If no Content-Type, peek at the body to detect JSON
	if !isJSON && ct == "" && r.Body != nil {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize))
		if err != nil {
			return err
		}
		trimmed := bytes.TrimSpace(body)
		if len(trimmed) > 0 && trimmed[0] == '{' {
			isJSON = true
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
	}

	if isJSON {
		return json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxRequestBodySize)).Decode(dst)
	}
	// Form data
	if err := r.ParseForm(); err != nil {
		return err
	}
	v := reflect.ValueOf(dst).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name := field.Tag.Get("json")
		if name == "" || name == "-" {
			name = strings.ToLower(field.Name[:1]) + field.Name[1:]
		}
		if val := r.FormValue(name); val != "" {
			if field.Type.Kind() == reflect.String {
				v.Field(i).SetString(val)
			} else if field.Type.Kind() == reflect.Bool {
				v.Field(i).SetBool(val == "true" || val == "on" || val == "1")
			}
		}
	}
	return nil
}

// htmxError writes an HTMX-friendly error alert or falls back to http.Error.
func htmxError(w http.ResponseWriter, r *http.Request, msg string, code int) {
	if isHTMX(r) {
		w.WriteHeader(code)
		if _, err := fmt.Fprintf(w, `<div class="alert alert-error mb-4">%s</div>`, html.EscapeString(msg)); err != nil {
			slog.Warn("htmx: failed to write error fragment", "error", err)
		}
		return
	}
	http.Error(w, msg, code)
}

// renderOrError renders a template and writes a generic 500 response on failure.
func renderOrError(w http.ResponseWriter, r *http.Request, tmpl TemplateRenderer, name string, data any, htmx bool) bool {
	if err := tmpl.Render(w, name, data, htmx); err != nil {
		slog.Error("template render failed", "template", name, "error", err)
		htmxError(w, r, "Failed to render page", http.StatusInternalServerError)
		return false
	}
	return true
}
