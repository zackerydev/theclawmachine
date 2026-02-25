package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zackerydev/clawmachine/control-plane/internal/service"
)

// mockConnect implements ConnectServicer for testing.
type mockConnect struct {
	status       *service.ConnectStatus
	statusErr    error
	installErr   error
	uninstallErr error
}

func (m *mockConnect) GetStatus(ctx context.Context) (*service.ConnectStatus, error) {
	return m.status, m.statusErr
}
func (m *mockConnect) Install(ctx context.Context, creds string, token string) error {
	return m.installErr
}
func (m *mockConnect) Uninstall(ctx context.Context) error { return m.uninstallErr }

func newTestSecretsHandler(secrets *mockSecrets, connect *mockConnect) *SecretsHandler {
	return NewSecretsHandler(secrets, connect, &noopRenderer{})
}

func TestSecretsHandler_AvailableSecrets_Empty(t *testing.T) {
	h := newTestSecretsHandler(
		&mockSecrets{extSecrets: nil},
		&mockConnect{},
	)
	req := httptest.NewRequest("GET", "/secrets/available", nil)
	w := httptest.NewRecorder()
	h.AvailableSecrets(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "[]") {
		t.Errorf("body = %q, want empty array", w.Body.String())
	}
}

func TestSecretsHandler_AvailableSecrets_FiltersSynced(t *testing.T) {
	h := newTestSecretsHandler(
		&mockSecrets{extSecrets: []service.ExternalSecretInfo{
			{Name: "synced-one", Status: "Synced", TargetSecret: "s1", DataKeys: []string{"value"}},
			{Name: "pending", Status: "Pending"},
			{Name: "synced-two", Status: "Synced", TargetSecret: "s2", DataKeys: []string{"key"}},
		}},
		&mockConnect{},
	)
	req := httptest.NewRequest("GET", "/secrets/available", nil)
	w := httptest.NewRecorder()
	h.AvailableSecrets(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "synced-one") || !strings.Contains(body, "synced-two") {
		t.Error("should include synced secrets")
	}
	if strings.Contains(body, "pending") {
		t.Error("should not include pending secrets")
	}
}

func TestSecretsHandler_AvailableSecrets_Error(t *testing.T) {
	h := newTestSecretsHandler(
		&mockSecrets{extErr: context.DeadlineExceeded},
		&mockConnect{},
	)
	req := httptest.NewRequest("GET", "/secrets/available", nil)
	w := httptest.NewRecorder()
	h.AvailableSecrets(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestSecretsHandler_CreateExternalSecret_Success(t *testing.T) {
	h := newTestSecretsHandler(
		&mockSecrets{storeStatus: &service.SecretStoreStatus{Configured: true}},
		&mockConnect{},
	)
	body := `{"name":"my-secret","item":"vault-item","field":"password"}`
	req := httptest.NewRequest("POST", "/secrets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateExternalSecret(w, req)

	// Should redirect to /secrets (303) or return 201
	if w.Code != http.StatusCreated && w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 201 or 303", w.Code)
	}
}

func TestSecretsHandler_CreateExternalSecret_MissingFields(t *testing.T) {
	h := newTestSecretsHandler(&mockSecrets{}, &mockConnect{})

	tests := []struct {
		name string
		body string
	}{
		{"empty name", `{"name":"","item":"foo"}`},
		{"empty item", `{"name":"foo","item":""}`},
		{"both empty", `{"name":"","item":""}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/secrets", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.CreateExternalSecret(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
	}
}

func TestSecretsHandler_CreateExternalSecret_InvalidName(t *testing.T) {
	h := newTestSecretsHandler(&mockSecrets{}, &mockConnect{})
	body := `{"name":"INVALID_NAME","item":"foo"}`
	req := httptest.NewRequest("POST", "/secrets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateExternalSecret(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestSecretsHandler_DeleteExternalSecret_Success(t *testing.T) {
	h := newTestSecretsHandler(&mockSecrets{}, &mockConnect{})
	req := httptest.NewRequest("DELETE", "/secrets/my-secret", nil)
	req.SetPathValue("name", "my-secret")
	w := httptest.NewRecorder()
	h.DeleteExternalSecret(w, req)

	if w.Code != http.StatusNoContent && w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 204 or 303", w.Code)
	}
}

func TestSecretsHandler_DeleteExternalSecret_HTMXSuccess(t *testing.T) {
	h := newTestSecretsHandler(&mockSecrets{}, &mockConnect{})
	req := httptest.NewRequest("DELETE", "/secrets/my-secret", nil)
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("name", "my-secret")
	w := httptest.NewRecorder()
	h.DeleteExternalSecret(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestSecretsHandler_DeleteExternalSecret_InvalidName(t *testing.T) {
	h := newTestSecretsHandler(&mockSecrets{}, &mockConnect{})
	req := httptest.NewRequest("DELETE", "/secrets/INVALID!", nil)
	req.SetPathValue("name", "INVALID!")
	w := httptest.NewRecorder()
	h.DeleteExternalSecret(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestSecretsHandler_SettingsStatusPartial_NotConfigured(t *testing.T) {
	h := newTestSecretsHandler(
		&mockSecrets{storeStatus: &service.SecretStoreStatus{Configured: false}},
		&mockConnect{},
	)
	req := httptest.NewRequest("GET", "/settings/status", nil)
	w := httptest.NewRecorder()
	h.SettingsStatusPartial(w, req)

	if !strings.Contains(w.Body.String(), "Not Configured") {
		t.Errorf("body = %q, want 'Not Configured'", w.Body.String())
	}
}

func TestSecretsHandler_SettingsStatusPartial_Ready(t *testing.T) {
	h := newTestSecretsHandler(
		&mockSecrets{storeStatus: &service.SecretStoreStatus{Configured: true, Ready: true}},
		&mockConnect{},
	)
	req := httptest.NewRequest("GET", "/settings/status", nil)
	w := httptest.NewRecorder()
	h.SettingsStatusPartial(w, req)

	if !strings.Contains(w.Body.String(), "Ready") {
		t.Errorf("body = %q, want 'Ready'", w.Body.String())
	}
}

func TestSecretsHandler_SettingsStatusPartial_Error(t *testing.T) {
	h := newTestSecretsHandler(
		&mockSecrets{storeErr: context.DeadlineExceeded},
		&mockConnect{},
	)
	req := httptest.NewRequest("GET", "/settings/status", nil)
	w := httptest.NewRecorder()
	h.SettingsStatusPartial(w, req)

	if !strings.Contains(w.Body.String(), "Error") {
		t.Errorf("body = %q, want 'Error'", w.Body.String())
	}
}

func TestSecretsHandler_CreateSecretStore_MissingFields(t *testing.T) {
	h := newTestSecretsHandler(&mockSecrets{}, &mockConnect{})
	body := `{"connectHost":"","connectToken":"","vaultName":""}`
	req := httptest.NewRequest("POST", "/settings/provider", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateSecretStore(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestSecretsHandler_DeleteSecretStore_Success(t *testing.T) {
	h := newTestSecretsHandler(&mockSecrets{}, &mockConnect{})
	req := httptest.NewRequest("DELETE", "/settings/provider", nil)
	w := httptest.NewRecorder()
	h.DeleteSecretStore(w, req)

	if w.Code != http.StatusNoContent && w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 204 or 303", w.Code)
	}
}

func TestSecretsHandler_InstallConnect_MissingFields(t *testing.T) {
	h := newTestSecretsHandler(&mockSecrets{}, &mockConnect{})
	body := `{"credentialsJson":"","token":"","vaultName":""}`
	req := httptest.NewRequest("POST", "/settings/connect/install", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.InstallConnect(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestSecretsHandler_InstallConnect_Success(t *testing.T) {
	secrets := &mockSecrets{storeStatus: &service.SecretStoreStatus{}}
	connect := &mockConnect{}
	h := newTestSecretsHandler(secrets, connect)
	body := `{"credentialsJson":"{\"verifier\":\"test\"}","token":"tok_123","vaultName":"my-vault"}`
	req := httptest.NewRequest("POST", "/settings/connect/install", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	h.InstallConnect(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
	if got := w.Header().Get("HX-Redirect"); got != "/settings/providers" {
		t.Errorf("HX-Redirect = %q, want /settings/providers", got)
	}
	if secrets.storeCalls != 1 {
		t.Fatalf("CreateSecretStore calls = %d, want 1", secrets.storeCalls)
	}
	if secrets.lastStore.VaultName != "my-vault" {
		t.Fatalf("CreateSecretStore vault = %q, want my-vault", secrets.lastStore.VaultName)
	}
}

func TestSecretsHandler_UninstallConnect_Success(t *testing.T) {
	h := newTestSecretsHandler(&mockSecrets{}, &mockConnect{})
	req := httptest.NewRequest("DELETE", "/settings/connect", nil)
	w := httptest.NewRecorder()
	h.UninstallConnect(w, req)

	if w.Code != http.StatusNoContent && w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 204 or 303", w.Code)
	}
}

func TestSecretsHandler_SecretsStatusPartial_AllSynced(t *testing.T) {
	h := newTestSecretsHandler(
		&mockSecrets{extSecrets: []service.ExternalSecretInfo{
			{Name: "s1", Status: "Synced"},
			{Name: "s2", Status: "Synced"},
		}},
		&mockConnect{},
	)
	req := httptest.NewRequest("GET", "/secrets/status", nil)
	w := httptest.NewRecorder()
	h.SecretsStatusPartial(w, req)

	// When all settled, should return 286 to stop HTMX polling
	if w.Code != 286 {
		t.Errorf("status = %d, want 286 (stop polling)", w.Code)
	}
}

func TestSecretsHandler_SecretsStatusPartial_Pending(t *testing.T) {
	h := newTestSecretsHandler(
		&mockSecrets{extSecrets: []service.ExternalSecretInfo{
			{Name: "s1", Status: "Synced"},
			{Name: "s2", Status: "Pending"},
		}},
		&mockConnect{},
	)
	req := httptest.NewRequest("GET", "/secrets/status", nil)
	w := httptest.NewRecorder()
	h.SecretsStatusPartial(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (keep polling)", w.Code)
	}
	if !strings.Contains(w.Body.String(), "hx-get") {
		t.Error("should include hx-get for continued polling")
	}
}

func TestSecretsHandler_SecretsStatusPartial_EscapesUntrustedValues(t *testing.T) {
	h := newTestSecretsHandler(
		&mockSecrets{extSecrets: []service.ExternalSecretInfo{
			{
				Name:   `bad"><script>alert(1)</script>`,
				Status: `<img src=x onerror=alert(1)>`,
			},
		}},
		&mockConnect{},
	)

	req := httptest.NewRequest("GET", "/secrets/status", nil)
	w := httptest.NewRecorder()
	h.SecretsStatusPartial(w, req)

	body := w.Body.String()
	if strings.Contains(body, "<script>") || strings.Contains(body, "<img") {
		t.Fatalf("response should escape untrusted values, got: %s", body)
	}
	if !strings.Contains(body, "&lt;img src=x onerror=alert(1)&gt;") {
		t.Fatalf("expected escaped status in body, got: %s", body)
	}
}
