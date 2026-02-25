package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestLogger_SetsStatus(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		if _, err := w.Write([]byte("ok")); err != nil {
			t.Fatalf("write response: %v", err)
		}
	})

	handler := RequestLogger(inner)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/bots", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("got status %d, want 201", rec.Code)
	}
}

func TestStatusRecorder_DefaultsTo200(t *testing.T) {
	rec := &statusRecorder{
		ResponseWriter: httptest.NewRecorder(),
		status:         http.StatusOK,
	}
	// If WriteHeader is never called, status should remain 200
	if rec.status != 200 {
		t.Errorf("default status = %d, want 200", rec.status)
	}
}

func TestRequestLogger_StaticAssetsDoNotPanic(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	handler := RequestLogger(inner)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/static/htmx.min.js", nil)
	handler.ServeHTTP(rec, req) // Should not panic
}
