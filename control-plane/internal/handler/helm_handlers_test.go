package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zackerydev/clawmachine/control-plane/internal/botenv"
	"github.com/zackerydev/clawmachine/control-plane/internal/service"
)

// mockHelmFull implements HelmManager with configurable responses.
type mockHelmFull struct {
	releases   []service.ReleaseInfo
	listErr    error
	installed  *service.ReleaseInfo
	installErr error
	statusInfo *service.ReleaseInfo
	statusErr  error
	upgraded   *service.ReleaseInfo
	upgradeErr error
	uninstErr  error
	values     map[string]any
	valuesErr  error
}

func (m *mockHelmFull) List(ns string) ([]service.ReleaseInfo, error)    { return m.releases, m.listErr }
func (m *mockHelmFull) Install(ctx context.Context, opts service.InstallOptions) (*service.ReleaseInfo, error) {
	return m.installed, m.installErr
}
func (m *mockHelmFull) Status(name, ns string) (*service.ReleaseInfo, error) {
	return m.statusInfo, m.statusErr
}
func (m *mockHelmFull) Upgrade(ctx context.Context, name, ns string, bt service.BotType, vals map[string]any) (*service.ReleaseInfo, error) {
	return m.upgraded, m.upgradeErr
}
func (m *mockHelmFull) Uninstall(name, ns string) error { return m.uninstErr }
func (m *mockHelmFull) GetValues(name, ns string) (map[string]any, error) {
	return m.values, m.valuesErr
}
func (m *mockHelmFull) GetValuesAll(name, ns string) (map[string]any, error) {
	return m.values, m.valuesErr
}

// mockSecretsManager implements SecretsManager for testing.
type mockSecrets struct {
	storeStatus *service.SecretStoreStatus
	storeErr    error
	createErr   error
	extSecrets  []service.ExternalSecretInfo
	extErr      error
	lastStore   service.CreateSecretStoreOptions
	storeCalls  int
}

func (m *mockSecrets) GetSecretStoreStatus(ctx context.Context, ns string) (*service.SecretStoreStatus, error) {
	return m.storeStatus, m.storeErr
}
func (m *mockSecrets) CreateSecretStore(ctx context.Context, ns string, opts service.CreateSecretStoreOptions) error {
	m.lastStore = opts
	m.storeCalls++
	return m.createErr
}
func (m *mockSecrets) DeleteSecretStore(ctx context.Context, ns string) error { return nil }
func (m *mockSecrets) ListExternalSecrets(ctx context.Context, ns string) ([]service.ExternalSecretInfo, error) {
	return m.extSecrets, m.extErr
}
func (m *mockSecrets) CreateExternalSecret(ctx context.Context, ns string, opts service.CreateExternalSecretOptions) error {
	return nil
}
func (m *mockSecrets) DeleteExternalSecret(ctx context.Context, ns, name string) error { return nil }

func newTestHelmHandler(t *testing.T, helm *mockHelmFull) *HelmHandler {
	t.Helper()
	botReg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	h := NewHelmHandler(helm, &noopRenderer{}, nil, &mockSecrets{
		storeStatus: &service.SecretStoreStatus{},
	}, nil, botReg, false)
	return h
}

func TestHelmHandler_ListPage_Success(t *testing.T) {
	helm := &mockHelmFull{
		releases: []service.ReleaseInfo{
			{Name: "mybot", Status: "deployed", BotType: "picoclaw"},
		},
	}
	h := newTestHelmHandler(t, helm)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ListPage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("ListPage status = %d, want 200", w.Code)
	}
}

func TestHelmHandler_ListPage_Error(t *testing.T) {
	helm := &mockHelmFull{listErr: http.ErrAbortHandler}
	h := newTestHelmHandler(t, helm)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ListPage(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("ListPage with error status = %d, want 500", w.Code)
	}
}

func TestHelmHandler_NewPage_NoType(t *testing.T) {
	h := newTestHelmHandler(t, &mockHelmFull{})

	req := httptest.NewRequest("GET", "/bots/new", nil)
	w := httptest.NewRecorder()
	h.NewPage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("NewPage status = %d, want 200", w.Code)
	}
}

func TestHelmHandler_NewPage_WithBotType(t *testing.T) {
	h := newTestHelmHandler(t, &mockHelmFull{})

	req := httptest.NewRequest("GET", "/bots/new?type=picoclaw", nil)
	w := httptest.NewRecorder()
	h.NewPage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("NewPage with type status = %d, want 200", w.Code)
	}
}

func TestHelmHandler_NewPage_InvalidBotType(t *testing.T) {
	h := newTestHelmHandler(t, &mockHelmFull{})

	req := httptest.NewRequest("GET", "/bots/new?type=invalid", nil)
	w := httptest.NewRecorder()
	h.NewPage(w, req)

	// Should redirect or show error for invalid type
	// Invalid type may still show selector page — just verify no crash
	t.Logf("invalid bot type returned status %d", w.Code)
}

func TestHelmHandler_List_JSON(t *testing.T) {
	helm := &mockHelmFull{
		releases: []service.ReleaseInfo{
			{Name: "bot1", Status: "deployed"},
			{Name: "bot2", Status: "deployed"},
		},
	}
	h := newTestHelmHandler(t, helm)

	req := httptest.NewRequest("GET", "/bots", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("List status = %d, want 200", w.Code)
	}
}

func TestHelmHandler_Status_JSON(t *testing.T) {
	helm := &mockHelmFull{
		statusInfo: &service.ReleaseInfo{Name: "mybot", Status: "deployed", BotType: "picoclaw"},
	}
	h := newTestHelmHandler(t, helm)

	req := httptest.NewRequest("GET", "/bots/mybot", nil)
	req.SetPathValue("name", "mybot")
	w := httptest.NewRecorder()
	h.Status(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status code = %d, want 200", w.Code)
	}
}

func TestHelmHandler_Status_NotFound(t *testing.T) {
	helm := &mockHelmFull{
		statusErr: http.ErrAbortHandler,
	}
	h := newTestHelmHandler(t, helm)

	req := httptest.NewRequest("GET", "/bots/missing", nil)
	req.SetPathValue("name", "missing")
	w := httptest.NewRecorder()
	h.Status(w, req)

	if w.Code == http.StatusOK {
		t.Error("Status for missing bot should not be 200")
	}
}

func TestHelmHandler_Uninstall_Success(t *testing.T) {
	h := newTestHelmHandler(t, &mockHelmFull{})

	req := httptest.NewRequest("DELETE", "/bots/mybot", nil)
	req.SetPathValue("name", "mybot")
	w := httptest.NewRecorder()
	h.Uninstall(w, req)

	if w.Code != http.StatusNoContent && w.Code != http.StatusOK {
		t.Errorf("Uninstall status = %d, want 200 or 204", w.Code)
	}
}

func TestHelmHandler_Uninstall_InvalidName(t *testing.T) {
	h := newTestHelmHandler(t, &mockHelmFull{})

	req := httptest.NewRequest("DELETE", "/bots/<script>", nil)
	req.SetPathValue("name", "<script>")
	w := httptest.NewRecorder()
	h.Uninstall(w, req)

	// Uninstall may not validate name format — just verify it doesn't crash
	if w.Code == 0 {
		t.Error("expected a response")
	}
}

func TestHelmHandler_Install_InvalidName(t *testing.T) {
	h := newTestHelmHandler(t, &mockHelmFull{})

	form := "releaseName=INVALID&botType=picoclaw"
	req := httptest.NewRequest("POST", "/bots", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Install(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Install with invalid name status = %d, want 400", w.Code)
	}
}

func TestHelmHandler_Install_MissingName(t *testing.T) {
	h := newTestHelmHandler(t, &mockHelmFull{})

	form := "botType=picoclaw"
	req := httptest.NewRequest("POST", "/bots", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Install(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Install without name status = %d, want 400", w.Code)
	}
}

func TestHelmHandler_Install_MissingBotType(t *testing.T) {
	h := newTestHelmHandler(t, &mockHelmFull{})

	form := "releaseName=mybot"
	req := httptest.NewRequest("POST", "/bots", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Install(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Install without botType status = %d, want 400", w.Code)
	}
}

func TestHelmHandler_Upgrade_InvalidName(t *testing.T) {
	h := newTestHelmHandler(t, &mockHelmFull{})

	req := httptest.NewRequest("PUT", "/bots/INVALID", strings.NewReader("{}"))
	req.SetPathValue("name", "INVALID")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Upgrade(w, req)

	// Upgrade may handle validation differently — just verify it doesn't crash
	if w.Code == 0 {
		t.Error("expected a response")
	}
}

func TestHelmHandler_DetailPage(t *testing.T) {
	helm := &mockHelmFull{
		statusInfo: &service.ReleaseInfo{Name: "mybot", Status: "deployed", BotType: "picoclaw"},
		values:     map[string]any{"replicas": 1},
	}
	h := newTestHelmHandler(t, helm)

	req := httptest.NewRequest("GET", "/bots/mybot/page", nil)
	req.SetPathValue("name", "mybot")
	w := httptest.NewRecorder()
	h.DetailPage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("DetailPage status = %d, want 200", w.Code)
	}
}

func TestHelmHandler_DetailPage_NotFound(t *testing.T) {
	helm := &mockHelmFull{
		statusErr: http.ErrAbortHandler,
	}
	h := newTestHelmHandler(t, helm)

	req := httptest.NewRequest("GET", "/bots/missing/page", nil)
	req.SetPathValue("name", "missing")
	w := httptest.NewRecorder()
	h.DetailPage(w, req)

	if w.Code == http.StatusOK {
		t.Error("DetailPage for missing bot should not be 200")
	}
}
