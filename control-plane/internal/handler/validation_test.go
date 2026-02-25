package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zackerydev/clawmachine/control-plane/internal/service"
)

// stubHelm is a minimal HelmServicer for testing.
type stubHelm struct{}

func (s *stubHelm) List(namespace string) ([]service.ReleaseInfo, error) { return nil, nil }
func (s *stubHelm) Install(ctx context.Context, opts service.InstallOptions) (*service.ReleaseInfo, error) {
	return &service.ReleaseInfo{Name: opts.ReleaseName}, nil
}
func (s *stubHelm) Status(name, namespace string) (*service.ReleaseInfo, error) {
	return &service.ReleaseInfo{Name: name}, nil
}
func (s *stubHelm) Upgrade(ctx context.Context, name, namespace string, botType service.BotType, values map[string]any) (*service.ReleaseInfo, error) {
	return &service.ReleaseInfo{Name: name}, nil
}
func (s *stubHelm) Uninstall(name, namespace string) error { return nil }
func (s *stubHelm) GetValues(name, namespace string) (map[string]any, error) { return nil, nil }
func (s *stubHelm) GetValuesAll(name, namespace string) (map[string]any, error) {
	return nil, nil
}

// stubTmpl is a minimal TemplateRenderer for testing.
type stubTmpl struct{}

func (s *stubTmpl) Render(w io.Writer, name string, data any, isHTMX bool) error { return nil }

// stubConnect is a minimal ConnectServicer for testing.
type stubConnect struct{}

func (s *stubConnect) GetStatus(ctx context.Context) (*service.ConnectStatus, error) {
	return &service.ConnectStatus{}, nil
}
func (s *stubConnect) Install(ctx context.Context, creds, token string) error { return nil }
func (s *stubConnect) Uninstall(ctx context.Context) error                    { return nil }

// stubSecrets is a minimal SecretsServicer for testing.
type stubSecrets struct{}

func (s *stubSecrets) GetSecretStoreStatus(ctx context.Context, namespace string) (*service.SecretStoreStatus, error) {
	return &service.SecretStoreStatus{}, nil
}
func (s *stubSecrets) CreateSecretStore(ctx context.Context, namespace string, opts service.CreateSecretStoreOptions) error {
	return nil
}
func (s *stubSecrets) DeleteSecretStore(ctx context.Context, namespace string) error { return nil }
func (s *stubSecrets) ListExternalSecrets(ctx context.Context, namespace string) ([]service.ExternalSecretInfo, error) {
	return nil, nil
}
func (s *stubSecrets) CreateExternalSecret(ctx context.Context, namespace string, opts service.CreateExternalSecretOptions) error {
	return nil
}
func (s *stubSecrets) DeleteExternalSecret(ctx context.Context, namespace, name string) error {
	return nil
}

func TestInstallValidation(t *testing.T) {
	h := NewHelmHandler(&stubHelm{}, &stubTmpl{}, nil, nil, nil, nil, false)

	tests := []struct {
		name string
		body map[string]any
		want int
	}{
		{"missing fields", map[string]any{}, http.StatusBadRequest},
		{"invalid name", map[string]any{"releaseName": "INVALID!", "botType": "picoclaw"}, http.StatusBadRequest},
		{"valid", map[string]any{"releaseName": "my-bot", "botType": "picoclaw"}, http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/bots", bytes.NewReader(body))
			req.Header.Set("HX-Request", "true")
			rec := httptest.NewRecorder()
			h.Install(rec, req)
			if rec.Code != tt.want {
				t.Errorf("got status %d, want %d; body: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestCreateExternalSecretValidation(t *testing.T) {
	h := NewSecretsHandler(&stubSecrets{}, &stubConnect{}, &stubTmpl{})

	tests := []struct {
		name string
		body map[string]any
		want int
	}{
		{"missing fields", map[string]any{}, http.StatusBadRequest},
		{"invalid name", map[string]any{
			"name": "INVALID!", "item": "my-item",
		}, http.StatusBadRequest},
		{"valid", map[string]any{
			"name": "my-secret", "item": "my-item", "field": "password",
		}, http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/secrets", bytes.NewReader(body))
			req.Header.Set("HX-Request", "true")
			rec := httptest.NewRecorder()
			h.CreateExternalSecret(rec, req)
			if rec.Code != tt.want {
				t.Errorf("got status %d, want %d; body: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestHtmxErrorEscapesHTML(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	htmxError(rec, req, `<script>alert("xss")</script>`, http.StatusBadRequest)

	body := rec.Body.String()
	if bytes.Contains([]byte(body), []byte("<script>")) {
		t.Errorf("htmxError did not escape HTML: %s", body)
	}
}

func TestCreateSecretStoreValidation(t *testing.T) {
	h := NewSecretsHandler(&stubSecrets{}, &stubConnect{}, &stubTmpl{})

	body, _ := json.Marshal(map[string]string{"connectHost": "", "connectToken": "", "vaultName": ""})
	req := httptest.NewRequest("POST", "/settings/provider", bytes.NewReader(body))
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	h.CreateSecretStore(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
