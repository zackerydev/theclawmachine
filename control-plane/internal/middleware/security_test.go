package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := SecurityHeaders(inner)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	tests := []struct {
		header string
		want   string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
	}

	for _, tt := range tests {
		got := rec.Header().Get(tt.header)
		if got != tt.want {
			t.Errorf("header %s = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestLimitBody(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, MaxRequestBodySize+1)
		_, err := r.Body.Read(buf)
		if err == nil {
			t.Error("expected error reading oversized body")
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := LimitBody(inner)
	body := strings.NewReader(strings.Repeat("x", MaxRequestBodySize+100))
	req := httptest.NewRequest("POST", "/", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

func TestValidName(t *testing.T) {
	valid := []string{"mybot", "my-bot", "a", "my-bot-123", "a1"}
	invalid := []string{"", "-start", "end-", "UPPER", "has space", "has.dot", strings.Repeat("a", 64), "<script>"}

	for _, name := range valid {
		if !ValidName(name) {
			t.Errorf("ValidName(%q) = false, want true", name)
		}
	}
	for _, name := range invalid {
		if ValidName(name) {
			t.Errorf("ValidName(%q) = true, want false", name)
		}
	}
}

func TestEscapeHTML(t *testing.T) {
	input := `<script>alert("xss")</script>`
	got := EscapeHTML(input)
	if strings.Contains(got, "<script>") {
		t.Errorf("EscapeHTML failed to escape: %s", got)
	}
}

func TestGenerateCSRFToken(t *testing.T) {
	token, err := GenerateCSRFToken()
	if err != nil {
		t.Fatalf("GenerateCSRFToken error: %v", err)
	}
	if len(token) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("token length = %d, want 64", len(token))
	}
	// Tokens should be unique
	token2, _ := GenerateCSRFToken()
	if token == token2 {
		t.Error("two tokens should not be identical")
	}
}

func TestNoListingFileServer(t *testing.T) {
	// Create a temp dir with a file but no index.html
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/test.txt", []byte("hello"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	handler := http.StripPrefix("/static/", NoListingFileServer(dir))

	// File should be served
	req := httptest.NewRequest("GET", "/static/test.txt", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /static/test.txt = %d, want 200", rec.Code)
	}

	// Directory listing should be denied (no index.html)
	req = httptest.NewRequest("GET", "/static/", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Error("directory listing should be denied without index.html")
	}
}

func TestNoListingFileServer_WithIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/index.html", []byte("<html>ok</html>"), 0644); err != nil {
		t.Fatalf("write index file: %v", err)
	}

	handler := http.StripPrefix("/static/", NoListingFileServer(dir))

	// Directory with index.html should work
	req := httptest.NewRequest("GET", "/static/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /static/ with index.html = %d, want 200", rec.Code)
	}
}

func TestLimitBody_GET_NoLimit(t *testing.T) {
	// GET requests should not have body limits applied
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := LimitBody(inner)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET with LimitBody = %d, want 200", rec.Code)
	}
}
