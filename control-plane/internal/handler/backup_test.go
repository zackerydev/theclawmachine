package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewBackupHandler(t *testing.T) {
	h := NewBackupHandler(nil, nil, nil, nil, nil)
	if h == nil {
		t.Fatal("NewBackupHandler returned nil")
	}
}

func TestBackupHandler_BackupConfigPage_NilServices(t *testing.T) {
	h := NewBackupHandler(nil, nil, nil, nil, nil)

	req := httptest.NewRequest("GET", "/bots/mybot/backup", nil)
	req.SetPathValue("name", "mybot")
	w := httptest.NewRecorder()

	h.BackupConfigPage(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestBotEnvHandler(t *testing.T) {
	h := NewBotEnvHandler()
	if h == nil {
		t.Fatal("NewBotEnvHandler returned nil")
	}

	req := httptest.NewRequest("GET", "/api/botenv", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("BotEnvHandler status = %d, want 200", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}
