package routes

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zackerydev/clawmachine/control-plane/internal/handler"
	"github.com/zackerydev/clawmachine/control-plane/internal/service" // used by noop mocks
)

// Minimal no-op implementations for route registration testing

type noopRenderer struct{}

func (n *noopRenderer) Render(w io.Writer, name string, data any, isHTMX bool) error {
	return nil
}

type noopHelmService struct{}

func (n *noopHelmService) List(namespace string) ([]service.ReleaseInfo, error) {
	return nil, nil
}
func (n *noopHelmService) Install(ctx context.Context, opts service.InstallOptions) (*service.ReleaseInfo, error) {
	return &service.ReleaseInfo{}, nil
}
func (n *noopHelmService) Status(name, namespace string) (*service.ReleaseInfo, error) {
	return &service.ReleaseInfo{}, nil
}
func (n *noopHelmService) Upgrade(ctx context.Context, name, namespace string, botType service.BotType, values map[string]any) (*service.ReleaseInfo, error) {
	return &service.ReleaseInfo{}, nil
}
func (n *noopHelmService) GetValues(name, namespace string) (map[string]any, error) {
	return nil, nil
}
func (n *noopHelmService) GetValuesAll(name, namespace string) (map[string]any, error) {
	return nil, nil
}

func (n *noopHelmService) Uninstall(name, namespace string) error {
	return nil
}

type noopConnectService struct{}

func (n *noopConnectService) GetStatus(ctx context.Context) (*service.ConnectStatus, error) {
	return &service.ConnectStatus{}, nil
}
func (n *noopConnectService) Install(ctx context.Context, credentialsJSON string, token string) error {
	return nil
}
func (n *noopConnectService) Uninstall(ctx context.Context) error {
	return nil
}

type noopSecretsService struct{}

func (n *noopSecretsService) GetSecretStoreStatus(ctx context.Context, namespace string) (*service.SecretStoreStatus, error) {
	return &service.SecretStoreStatus{}, nil
}
func (n *noopSecretsService) CreateSecretStore(ctx context.Context, namespace string, opts service.CreateSecretStoreOptions) error {
	return nil
}
func (n *noopSecretsService) DeleteSecretStore(ctx context.Context, namespace string) error {
	return nil
}
func (n *noopSecretsService) ListExternalSecrets(ctx context.Context, namespace string) ([]service.ExternalSecretInfo, error) {
	return nil, nil
}
func (n *noopSecretsService) CreateExternalSecret(ctx context.Context, namespace string, opts service.CreateExternalSecretOptions) error {
	return nil
}
func (n *noopSecretsService) DeleteExternalSecret(ctx context.Context, namespace, name string) error {
	return nil
}

func TestSetup_HealthEndpoint(t *testing.T) {
	mux := http.NewServeMux()

	Setup(mux, &Handlers{
		Helm:    handler.NewHelmHandler(&noopHelmService{}, &noopRenderer{}, nil, nil, nil, nil, false),
		Secrets: handler.NewSecretsHandler(&noopSecretsService{}, &noopConnectService{}, &noopRenderer{}),
		Network: handler.NewNetworkHandler(nil, &noopRenderer{}),
		Backup:  handler.NewBackupHandler(nil, nil, nil, nil, nil),
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/health", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "OK" {
		t.Fatalf("expected 'OK', got %q", w.Body.String())
	}
}

func TestSetup_AllRoutesRegistered(t *testing.T) {
	mux := http.NewServeMux()

	Setup(mux, &Handlers{
		Helm:    handler.NewHelmHandler(&noopHelmService{}, &noopRenderer{}, nil, nil, nil, nil, false),
		Secrets: handler.NewSecretsHandler(&noopSecretsService{}, &noopConnectService{}, &noopRenderer{}),
		Network: handler.NewNetworkHandler(nil, &noopRenderer{}),
		Backup:  handler.NewBackupHandler(nil, nil, nil, nil, nil),
	})

	routes := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/health", ""},
		{"GET", "/", ""},
		{"GET", "/bots/new", ""},
		{"POST", "/bots/new/infra", "botType=picoclaw&releaseName=my-bot"},
		{"POST", "/bots/new/config", "botType=picoclaw&releaseName=my-bot"},
		{"POST", "/bots/new/software", "botType=picoclaw&releaseName=my-bot"},
		{"GET", "/bots", ""},
		{"GET", "/bots/my-bot/cli", ""},
		{"GET", "/settings", ""},
		{"GET", "/secrets", ""},
		{"GET", "/secrets/new", ""},
	}

	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body))
			if rt.body != "" {
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			mux.ServeHTTP(w, r)

			if w.Code == http.StatusNotFound {
				t.Fatalf("route %s %s returned 404", rt.method, rt.path)
			}
		})
	}
}
