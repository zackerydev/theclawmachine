package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTMXRedirectOrStatus_HTMXRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/secrets", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	htmxRedirectOrStatus(rec, req, "/secrets", http.StatusCreated)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("HX-Redirect"); got != "/secrets" {
		t.Fatalf("HX-Redirect = %q, want %q", got, "/secrets")
	}
	if got := rec.Header().Get("Location"); got != "" {
		t.Fatalf("Location = %q, want empty", got)
	}
}

func TestHTMXRedirectOrStatus_NonHTMXRedirectFallback(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/secrets", nil)
	rec := httptest.NewRecorder()

	htmxRedirectOrStatus(rec, req, "/secrets", http.StatusCreated)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/secrets" {
		t.Fatalf("Location = %q, want %q", got, "/secrets")
	}
}

func TestHTMXRedirectOrStatus_NonHTMXNoRedirect(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/secrets", nil)
	rec := httptest.NewRecorder()

	htmxRedirectOrStatus(rec, req, "", http.StatusCreated)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("Location"); got != "" {
		t.Fatalf("Location = %q, want empty", got)
	}
}

func TestHTMXRedirectOrStatus_NonHTMXPutKeepsStatus(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/bots/my-bot/config", nil)
	rec := httptest.NewRecorder()

	htmxRedirectOrStatus(rec, req, "/bots/my-bot/page", http.StatusOK)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Location"); got != "" {
		t.Fatalf("Location = %q, want empty", got)
	}
}
