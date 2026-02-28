package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zackerydev/clawmachine/control-plane/internal/botenv"
	"github.com/zackerydev/clawmachine/control-plane/internal/service"
)

// ── Mocks ──

type mockHelm struct {
	releases []service.ReleaseInfo
	listErr  error

	installed   *service.ReleaseInfo
	installErr  error
	installOpts service.InstallOptions
	installDone chan struct{} // closed when Install completes

	statusInfo *service.ReleaseInfo
	statusErr  error
	statusName string

	uninstallErr  error
	uninstallName string
	uninstallDone chan struct{} // closed when Uninstall completes

	upgraded       *service.ReleaseInfo
	upgradeErr     error
	upgradeName    string
	upgradeBotType service.BotType
	upgradeValues  map[string]any

	values    map[string]any
	valuesErr error
}

func (m *mockHelm) List(namespace string) ([]service.ReleaseInfo, error) {
	return m.releases, m.listErr
}

func (m *mockHelm) Install(ctx context.Context, opts service.InstallOptions) (*service.ReleaseInfo, error) {
	m.installOpts = opts
	if m.installDone != nil {
		defer close(m.installDone)
	}
	return m.installed, m.installErr
}

func (m *mockHelm) Upgrade(ctx context.Context, name, namespace string, botType service.BotType, values map[string]any) (*service.ReleaseInfo, error) {
	m.upgradeName = name
	m.upgradeBotType = botType
	m.upgradeValues = values
	return m.upgraded, m.upgradeErr
}

func (m *mockHelm) Uninstall(name, namespace string) error {
	m.uninstallName = name
	if m.uninstallDone != nil {
		defer close(m.uninstallDone)
	}
	return m.uninstallErr
}

func (m *mockHelm) Status(name, namespace string) (*service.ReleaseInfo, error) {
	m.statusName = name
	return m.statusInfo, m.statusErr
}

func (m *mockHelm) GetValues(name, namespace string) (map[string]any, error) {
	return m.values, m.valuesErr
}

func (m *mockHelm) GetValuesAll(name, namespace string) (map[string]any, error) {
	return m.values, m.valuesErr
}

type mockTemplate struct {
	rendered string
	err      error
	calls    []templateCall
	renderFn func(w io.Writer, name string, data any, isHTMX bool) error
}

type templateCall struct {
	name   string
	data   any
	isHTMX bool
}

func (m *mockTemplate) Render(w io.Writer, name string, data any, isHTMX bool) error {
	m.calls = append(m.calls, templateCall{name: name, data: data, isHTMX: isHTMX})
	if m.renderFn != nil {
		return m.renderFn(w, name, data, isHTMX)
	}
	if m.err != nil {
		return m.err
	}
	if m.rendered != "" {
		if _, err := w.Write([]byte(m.rendered)); err != nil {
			return err
		}
	}
	return nil
}

type mockKubernetes struct {
	secretData map[string]map[string][]byte
	hasCRD     bool
	podHealthy bool
	podErr     error
	execStdout string
	execStderr string
	execErr    error

	execNamespace string
	execRelease   string
	execContainer string
	execCommand   []string
}

func (m *mockKubernetes) HasCRD(name string) bool { return m.hasCRD }
func (m *mockKubernetes) GetPodLogs(ctx context.Context, namespace, releaseName string, tailLines int64) (string, error) {
	return "", nil
}
func (m *mockKubernetes) GetReleasePodHealthy(ctx context.Context, namespace, releaseName string) (bool, error) {
	return m.podHealthy, m.podErr
}
func (m *mockKubernetes) RestartBot(ctx context.Context, namespace, releaseName string) error {
	return nil
}
func (m *mockKubernetes) ExecInReleasePod(ctx context.Context, namespace, releaseName, container string, command []string) (string, string, error) {
	m.execNamespace = namespace
	m.execRelease = releaseName
	m.execContainer = container
	m.execCommand = append([]string(nil), command...)
	return m.execStdout, m.execStderr, m.execErr
}
func (m *mockKubernetes) ReadSecretData(ctx context.Context, namespace, name string) (map[string][]byte, error) {
	if data, ok := m.secretData[name]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("secret %q not found", name)
}

// ── Tests ──

func TestHelmHandler_List(t *testing.T) {
	tests := []struct {
		name       string
		releases   []service.ReleaseInfo
		listErr    error
		wantStatus int
	}{
		{
			name: "returns releases as JSON",
			releases: []service.ReleaseInfo{
				{Name: "my-bot", Status: "deployed", BotType: "picoclaw"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "returns empty array for no releases",
			releases:   []service.ReleaseInfo{},
			wantStatus: http.StatusOK,
		},
		{
			name:       "returns 500 on error",
			listErr:    fmt.Errorf("cluster unreachable"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHelmHandler(&mockHelm{releases: tt.releases, listErr: tt.listErr}, &mockTemplate{}, nil, nil, nil, nil, false)
			req := httptest.NewRequest(http.MethodGet, "/bots", nil)
			rec := httptest.NewRecorder()

			h.List(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				var got []service.ReleaseInfo
				if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if len(got) != len(tt.releases) {
					t.Errorf("got %d releases, want %d", len(got), len(tt.releases))
				}
			}
		})
	}
}

func TestHelmHandler_Install(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		htmx         bool
		wantStatus   int
		wantRedirect string
	}{
		{
			name:       "non-HTMX install redirects to bot list after install",
			body:       `{"releaseName":"test-bot","botType":"picoclaw"}`,
			wantStatus: http.StatusSeeOther,
		},
		{
			name:         "HTMX install redirects to bot list",
			body:         `{"releaseName":"test-bot","botType":"picoclaw"}`,
			htmx:         true,
			wantStatus:   http.StatusNoContent,
			wantRedirect: "/bots/test-bot/page",
		},
		{
			name:       "invalid JSON returns 400",
			body:       `{invalid`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "HTMX invalid JSON returns 400 with alert",
			body:       `{invalid`,
			htmx:       true,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan struct{})
			installed := &service.ReleaseInfo{Name: "test-bot", Status: "deployed"}
			h := NewHelmHandler(
				&mockHelm{installed: installed, installDone: done},
				&mockTemplate{},
				nil,
				nil,
				nil,
				nil,
				false,
			)

			req := httptest.NewRequest(http.MethodPost, "/bots", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.htmx {
				req.Header.Set("HX-Request", "true")
			}
			rec := httptest.NewRecorder()

			h.Install(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantRedirect != "" {
				if loc := rec.Header().Get("HX-Redirect"); loc != tt.wantRedirect {
					t.Errorf("got HX-Redirect %q, want %q", loc, tt.wantRedirect)
				}
			}

			// Install executes synchronously; this should already be closed on success.
			if tt.wantStatus == http.StatusSeeOther || tt.wantStatus == http.StatusNoContent {
				<-done
			}
		})
	}
}

func TestNormalizeReleaseImageTag(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "stable", in: "0.1.0", want: "0.1.0"},
		{name: "stable with v", in: "v0.1.0", want: "0.1.0"},
		{name: "dev", in: "dev", want: ""},
		{name: "prerelease", in: "0.1.0-rc.1", want: "0.1.0-rc.1"},
		{name: "invalid", in: "dirty-build", want: ""},
		{name: "metadata", in: "0.1.0+sha.123", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeReleaseImageTag(tt.in)
			if got != tt.want {
				t.Fatalf("normalizeReleaseImageTag(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHelmHandler_Install_RuntimeTagDefaulting(t *testing.T) {
	t.Run("sets runtime tag when missing", func(t *testing.T) {
		done := make(chan struct{})
		mock := &mockHelm{
			installed:   &service.ReleaseInfo{Name: "test-bot", Status: "deployed"},
			installDone: done,
		}
		h := NewHelmHandlerWithVersion(mock, &mockTemplate{}, nil, nil, nil, nil, false, "0.1.0")

		req := httptest.NewRequest(http.MethodPost, "/bots", bytes.NewBufferString(`{"releaseName":"test-bot","botType":"picoclaw"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Install(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
		}
		<-done

		image, ok := mock.installOpts.Values["image"].(map[string]any)
		if !ok {
			t.Fatalf("image missing from install values: %#v", mock.installOpts.Values)
		}
		if tag := image["tag"]; tag != "0.1.0" {
			t.Fatalf("image.tag = %v, want 0.1.0", tag)
		}
	})

	t.Run("keeps explicit tag", func(t *testing.T) {
		done := make(chan struct{})
		mock := &mockHelm{
			installed:   &service.ReleaseInfo{Name: "test-bot", Status: "deployed"},
			installDone: done,
		}
		h := NewHelmHandlerWithVersion(mock, &mockTemplate{}, nil, nil, nil, nil, false, "0.1.0")

		body := `{"releaseName":"test-bot","botType":"picoclaw","values":{"image":{"tag":"9.9.9"}}}`
		req := httptest.NewRequest(http.MethodPost, "/bots", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Install(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
		}
		<-done

		image, ok := mock.installOpts.Values["image"].(map[string]any)
		if !ok {
			t.Fatalf("image missing from install values: %#v", mock.installOpts.Values)
		}
		if tag := image["tag"]; tag != "9.9.9" {
			t.Fatalf("image.tag = %v, want 9.9.9", tag)
		}
	})

	t.Run("accepts prerelease runtime tag", func(t *testing.T) {
		done := make(chan struct{})
		mock := &mockHelm{
			installed:   &service.ReleaseInfo{Name: "test-bot", Status: "deployed"},
			installDone: done,
		}
		h := NewHelmHandlerWithVersion(mock, &mockTemplate{}, nil, nil, nil, nil, false, "v0.1.7-rc.1")

		req := httptest.NewRequest(http.MethodPost, "/bots", bytes.NewBufferString(`{"releaseName":"test-bot","botType":"picoclaw"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Install(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
		}
		<-done

		image, ok := mock.installOpts.Values["image"].(map[string]any)
		if !ok {
			t.Fatalf("image missing from install values: %#v", mock.installOpts.Values)
		}
		if tag := image["tag"]; tag != "0.1.7-rc.1" {
			t.Fatalf("image.tag = %v, want 0.1.7-rc.1", tag)
		}
	})
}

func TestHelmHandler_Status(t *testing.T) {
	tests := []struct {
		name       string
		botName    string
		info       *service.ReleaseInfo
		err        error
		wantStatus int
	}{
		{
			name:       "returns status",
			botName:    "my-bot",
			info:       &service.ReleaseInfo{Name: "my-bot", Status: "deployed", BotType: "picoclaw"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "returns 500 on error",
			botName:    "missing-bot",
			err:        fmt.Errorf("release not found"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHelmHandler(&mockHelm{statusInfo: tt.info, statusErr: tt.err}, &mockTemplate{}, nil, nil, nil, nil, false)

			req := httptest.NewRequest(http.MethodGet, "/bots/"+tt.botName, nil)
			req.SetPathValue("name", tt.botName)
			rec := httptest.NewRecorder()

			h.Status(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestHelmHandler_Upgrade_MergesExistingValues(t *testing.T) {
	mock := &mockHelm{
		values: map[string]any{
			"persistence": map[string]any{
				"enabled":      true,
				"size":         "1Gi",
				"storageClass": "local-path",
			},
			"networkPolicy": map[string]any{
				"ingress":        false,
				"egress":         false,
				"useCilium":      true,
				"allowedDomains": []any{"old.example.com"},
			},
		},
		upgraded: &service.ReleaseInfo{Name: "dorothy", Status: "deployed", BotType: "openclaw"},
	}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, nil, nil, nil, false)

	body := `{
		"botType":"openclaw",
		"values":{
			"persistence":{"enabled":true},
			"networkPolicy":{"ingress":true,"egress":true,"useCilium":false,"allowedDomains":[]}
		}
	}`
	req := httptest.NewRequest(http.MethodPut, "/bots/dorothy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "dorothy")
	rec := httptest.NewRecorder()

	h.Upgrade(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}
	if mock.upgradeBotType != service.BotTypeOpenClaw {
		t.Fatalf("upgradeBotType = %q, want %q", mock.upgradeBotType, service.BotTypeOpenClaw)
	}

	persistence, ok := mock.upgradeValues["persistence"].(map[string]any)
	if !ok {
		t.Fatalf("persistence missing from upgrade values: %#v", mock.upgradeValues["persistence"])
	}
	if persistence["size"] != "1Gi" {
		t.Fatalf("persistence.size = %v, want 1Gi", persistence["size"])
	}
	if persistence["storageClass"] != "local-path" {
		t.Fatalf("persistence.storageClass = %v, want local-path", persistence["storageClass"])
	}

	networkPolicy, ok := mock.upgradeValues["networkPolicy"].(map[string]any)
	if !ok {
		t.Fatalf("networkPolicy missing from upgrade values: %#v", mock.upgradeValues["networkPolicy"])
	}
	if networkPolicy["ingress"] != true || networkPolicy["egress"] != true {
		t.Fatalf("networkPolicy ingress/egress = %#v, want true/true", networkPolicy)
	}
	allowed, ok := networkPolicy["allowedDomains"].([]any)
	if !ok {
		t.Fatalf("networkPolicy.allowedDomains has wrong type: %T", networkPolicy["allowedDomains"])
	}
	if len(allowed) != 0 {
		t.Fatalf("networkPolicy.allowedDomains length = %d, want 0", len(allowed))
	}
}

func TestHelmHandler_Upgrade_BotTypeFallbackFromStatus(t *testing.T) {
	mock := &mockHelm{
		values:     map[string]any{"networkPolicy": map[string]any{"ingress": false, "egress": true}},
		statusInfo: &service.ReleaseInfo{Name: "dorothy", Status: "deployed", BotType: "openclaw"},
		upgraded:   &service.ReleaseInfo{Name: "dorothy", Status: "deployed", BotType: "openclaw"},
	}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, nil, nil, nil, false)

	req := httptest.NewRequest(http.MethodPut, "/bots/dorothy", strings.NewReader(`{"values":{"networkPolicy":{"ingress":true}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "dorothy")
	rec := httptest.NewRecorder()

	h.Upgrade(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}
	if mock.upgradeBotType != service.BotTypeOpenClaw {
		t.Fatalf("upgradeBotType = %q, want %q", mock.upgradeBotType, service.BotTypeOpenClaw)
	}
}

func TestHelmHandler_Upgrade_RuntimeTagDefaulting(t *testing.T) {
	t.Run("forces runtime tag when body does not set image tag", func(t *testing.T) {
		mock := &mockHelm{
			values: map[string]any{
				"image": map[string]any{
					"tag": "2026.2.21",
				},
				"env": map[string]any{
					"BOT_TYPE": "openclaw",
				},
			},
			upgraded: &service.ReleaseInfo{Name: "dorothy", Status: "deployed", BotType: "openclaw"},
		}
		h := NewHelmHandlerWithVersion(mock, &mockTemplate{}, nil, nil, nil, nil, false, "0.1.0")

		req := httptest.NewRequest(http.MethodPut, "/bots/dorothy", strings.NewReader(`{"values":{"networkPolicy":{"ingress":true}}}`))
		req.Header.Set("Content-Type", "application/json")
		req.SetPathValue("name", "dorothy")
		rec := httptest.NewRecorder()
		h.Upgrade(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
		}
		image, ok := mock.upgradeValues["image"].(map[string]any)
		if !ok {
			t.Fatalf("image missing from upgrade values: %#v", mock.upgradeValues)
		}
		if tag := image["tag"]; tag != "0.1.0" {
			t.Fatalf("image.tag = %v, want 0.1.0", tag)
		}
	})

	t.Run("keeps explicit body image tag", func(t *testing.T) {
		mock := &mockHelm{
			values: map[string]any{
				"image": map[string]any{
					"tag": "2026.2.21",
				},
				"env": map[string]any{
					"BOT_TYPE": "openclaw",
				},
			},
			upgraded: &service.ReleaseInfo{Name: "dorothy", Status: "deployed", BotType: "openclaw"},
		}
		h := NewHelmHandlerWithVersion(mock, &mockTemplate{}, nil, nil, nil, nil, false, "0.1.0")

		req := httptest.NewRequest(http.MethodPut, "/bots/dorothy", strings.NewReader(`{"values":{"image":{"tag":"7.7.7"}}}`))
		req.Header.Set("Content-Type", "application/json")
		req.SetPathValue("name", "dorothy")
		rec := httptest.NewRecorder()
		h.Upgrade(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
		}
		image, ok := mock.upgradeValues["image"].(map[string]any)
		if !ok {
			t.Fatalf("image missing from upgrade values: %#v", mock.upgradeValues)
		}
		if tag := image["tag"]; tag != "7.7.7" {
			t.Fatalf("image.tag = %v, want 7.7.7", tag)
		}
	})
}

func TestHelmHandler_Uninstall(t *testing.T) {
	tests := []struct {
		name         string
		botName      string
		htmx         bool
		uninstallErr error
		wantStatus   int
		wantRedirect string
	}{
		{
			name:       "uninstall returns 204",
			botName:    "my-bot",
			wantStatus: http.StatusNoContent,
		},
		{
			name:         "HTMX uninstall redirects to bot list",
			botName:      "my-bot",
			htmx:         true,
			wantStatus:   http.StatusNoContent,
			wantRedirect: "/",
		},
		{
			name:         "uninstall error returns 500 and no redirect",
			botName:      "my-bot",
			htmx:         true,
			uninstallErr: errors.New("boom"),
			wantStatus:   http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockHelm{uninstallErr: tt.uninstallErr}
			h := NewHelmHandler(mock, &mockTemplate{}, nil, nil, nil, nil, false)

			req := httptest.NewRequest(http.MethodDelete, "/bots/"+tt.botName, nil)
			req.SetPathValue("name", tt.botName)
			if tt.htmx {
				req.Header.Set("HX-Request", "true")
			}
			rec := httptest.NewRecorder()

			h.Uninstall(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}

			if tt.wantRedirect != "" {
				if loc := rec.Header().Get("HX-Redirect"); loc != tt.wantRedirect {
					t.Errorf("HX-Redirect = %q, want %q", loc, tt.wantRedirect)
				}
			}

			if mock.uninstallName != tt.botName {
				t.Errorf("uninstalled %q, want %q", mock.uninstallName, tt.botName)
			}
		})
	}
}

func TestHelmHandler_ListPage(t *testing.T) {
	tests := []struct {
		name     string
		releases []service.ReleaseInfo
		htmx     bool
		wantTmpl string
	}{
		{
			name:     "renders bots template with releases",
			releases: []service.ReleaseInfo{{Name: "bot1", Status: "deployed"}},
			wantTmpl: "bots",
		},
		{
			name:     "HTMX request passes isHTMX=true",
			htmx:     true,
			wantTmpl: "bots",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := &mockTemplate{}
			h := NewHelmHandler(&mockHelm{releases: tt.releases}, tmpl, nil, nil, nil, nil, false)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.htmx {
				req.Header.Set("HX-Request", "true")
			}
			rec := httptest.NewRecorder()

			h.ListPage(rec, req)

			if len(tmpl.calls) != 1 {
				t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
			}
			if tmpl.calls[0].name != tt.wantTmpl {
				t.Errorf("rendered template %q, want %q", tmpl.calls[0].name, tt.wantTmpl)
			}
			if tmpl.calls[0].isHTMX != tt.htmx {
				t.Errorf("isHTMX=%v, want %v", tmpl.calls[0].isHTMX, tt.htmx)
			}
		})
	}
}

func TestHelmHandler_NewPage(t *testing.T) {
	tests := []struct {
		name     string
		devMode  bool
		wantBots int
	}{
		{
			name:     "production mode shows 3 bot types",
			devMode:  false,
			wantBots: 3,
		},
		{
			name:     "dev mode shows 4 bot types (includes busybox)",
			devMode:  true,
			wantBots: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := &mockTemplate{}
			h := NewHelmHandler(&mockHelm{}, tmpl, nil, nil, nil, nil, tt.devMode)

			req := httptest.NewRequest(http.MethodGet, "/bots/new", nil)
			rec := httptest.NewRecorder()

			h.NewPage(rec, req)

			if len(tmpl.calls) != 1 {
				t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
			}

			data, ok := tmpl.calls[0].data.(struct {
				BotTypes   []string
				BotConfigs []*botenv.BotConfig
				DevMode    bool
			})
			if !ok {
				t.Fatalf("unexpected data type: %T", tmpl.calls[0].data)
			}
			if len(data.BotTypes) != tt.wantBots {
				t.Errorf("got %d bot types, want %d", len(data.BotTypes), tt.wantBots)
			}
			if data.DevMode != tt.devMode {
				t.Errorf("DevMode=%v, want %v", data.DevMode, tt.devMode)
			}
		})
	}
}

func TestHelmHandler_NewPage_WithType_SetsPersistenceDefaults(t *testing.T) {
	tmpl := &mockTemplate{}
	botReg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	h := NewHelmHandler(&mockHelm{}, tmpl, nil, nil, nil, botReg, false)

	req := httptest.NewRequest(http.MethodGet, "/bots/new?type=openclaw", nil)
	rec := httptest.NewRecorder()
	h.NewPage(rec, req)

	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	if tmpl.calls[0].name != "bot-form-infra" {
		t.Fatalf("template = %q, want bot-form-infra", tmpl.calls[0].name)
	}

	data, ok := tmpl.calls[0].data.(struct {
		BotConfig *botenv.BotConfig
		Secrets   []AvailableSecret
		Values    map[string]string
	})
	if !ok {
		t.Fatalf("unexpected data type: %T", tmpl.calls[0].data)
	}
	if got := data.Values["persistence"]; got != "on" {
		t.Fatalf("values[persistence] = %q, want on", got)
	}
	if got := data.Values["persistenceSize"]; got != "5Gi" {
		t.Fatalf("values[persistenceSize] = %q, want 5Gi", got)
	}
}

func TestHelmHandler_NewSoftwarePage_RendersStep3(t *testing.T) {
	tmpl := &mockTemplate{}
	botReg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	h := NewHelmHandler(&mockHelm{}, tmpl, nil, nil, nil, botReg, false)

	form := "releaseName=my-bot&botType=openclaw&extraToolVersions=node+22%0Ajq+1.8.1"
	req := httptest.NewRequest(http.MethodPost, "/bots/new/software", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.NewSoftwarePage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	if tmpl.calls[0].name != "bot-form-software" {
		t.Fatalf("template = %q, want bot-form-software", tmpl.calls[0].name)
	}

	data, ok := tmpl.calls[0].data.(struct {
		BotConfig *botenv.BotConfig
		Values    map[string]string
	})
	if !ok {
		t.Fatalf("unexpected data type: %T", tmpl.calls[0].data)
	}
	if got := data.Values["extraToolVersions"]; got != "node 22\njq 1.8.1" {
		t.Fatalf("values[extraToolVersions] = %q, want multiline tool versions", got)
	}
}

func TestHelmHandler_NewSoftwarePage_InvalidReleaseName(t *testing.T) {
	tmpl := &mockTemplate{}
	h := NewHelmHandler(&mockHelm{}, tmpl, nil, nil, nil, nil, false)

	form := "releaseName=Bad_Name&botType=openclaw"
	req := httptest.NewRequest(http.MethodPost, "/bots/new/software", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	h.NewSoftwarePage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if len(tmpl.calls) != 0 {
		t.Fatalf("expected no template calls, got %d", len(tmpl.calls))
	}
}

func TestHelmHandler_Install_FormPost(t *testing.T) {
	done := make(chan struct{})
	installed := &service.ReleaseInfo{Name: "form-bot", Status: "deployed"}
	mock := &mockHelm{installed: installed, installDone: done}
	reg := &botenv.Registry{}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, nil, nil, reg, false)

	form := "releaseName=form-bot&botType=picoclaw&persistence=on&persistenceSize=2Gi&ingress=on&extraToolVersions=node+22%0Ajq+1.8.1"
	req := httptest.NewRequest(http.MethodPost, "/bots", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	h.Install(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("got status %d, want %d; body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if loc := rec.Header().Get("HX-Redirect"); loc != "/bots/form-bot/page" {
		t.Errorf("HX-Redirect = %q, want %q", loc, "/bots/form-bot/page")
	}

	// Wait for the background goroutine before checking opts.
	<-done
	// Verify parsed opts
	if mock.installOpts.ReleaseName != "form-bot" {
		t.Errorf("releaseName = %q, want %q", mock.installOpts.ReleaseName, "form-bot")
	}
	if string(mock.installOpts.BotType) != "picoclaw" {
		t.Errorf("botType = %q, want %q", mock.installOpts.BotType, "picoclaw")
	}
	if mock.installOpts.Namespace != "claw-machine" {
		t.Errorf("namespace = %q, want %q", mock.installOpts.Namespace, "claw-machine")
	}
	p, _ := mock.installOpts.Values["persistence"].(map[string]any)
	if p == nil || p["enabled"] != true {
		t.Errorf("persistence.enabled = %v, want true", p)
	}
	if p["size"] != "2Gi" {
		t.Errorf("persistence.size = %v, want 2Gi", p["size"])
	}
	np, _ := mock.installOpts.Values["networkPolicy"].(map[string]any)
	if np == nil || np["ingress"] != true {
		t.Errorf("networkPolicy.ingress = %v, want true", np)
	}
	extraSoftware, _ := mock.installOpts.Values["extraSoftware"].(map[string]any)
	if extraSoftware == nil {
		t.Fatalf("extraSoftware missing from install values: %#v", mock.installOpts.Values)
	}
	if got := extraSoftware["toolVersions"]; got != "node 22\njq 1.8.1" {
		t.Errorf("extraSoftware.toolVersions = %v, want multiline tool versions", got)
	}
}

func TestHelmHandler_ListPage_TemplateError(t *testing.T) {
	tmpl := &mockTemplate{err: errors.New("render failed")}
	h := NewHelmHandler(&mockHelm{}, tmpl, nil, nil, nil, nil, false)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.ListPage(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "Failed to render page") {
		t.Fatalf("body = %q, want render error message", rec.Body.String())
	}
}

func TestParseInstallForm(t *testing.T) {
	tests := []struct {
		name  string
		form  string
		check func(t *testing.T, opts service.InstallOptions)
	}{
		{
			name: "basic fields",
			form: "releaseName=my-bot&botType=picoclaw",
			check: func(t *testing.T, opts service.InstallOptions) {
				if opts.ReleaseName != "my-bot" {
					t.Errorf("releaseName = %q", opts.ReleaseName)
				}
				if string(opts.BotType) != "picoclaw" {
					t.Errorf("botType = %q", opts.BotType)
				}
			},
		},
		{
			name: "persistence enabled",
			form: "releaseName=x&botType=picoclaw&persistence=on&persistenceSize=5Gi",
			check: func(t *testing.T, opts service.InstallOptions) {
				p := opts.Values["persistence"].(map[string]any)
				if p["enabled"] != true {
					t.Error("persistence not enabled")
				}
				if p["size"] != "5Gi" {
					t.Errorf("size = %v", p["size"])
				}
			},
		},
		{
			name: "persistence disabled uses defaults",
			form: "releaseName=x&botType=picoclaw",
			check: func(t *testing.T, opts service.InstallOptions) {
				p := opts.Values["persistence"].(map[string]any)
				if p["enabled"] != false {
					t.Error("persistence should be disabled")
				}
				if p["size"] != "5Gi" {
					t.Errorf("default size = %v, want 5Gi", p["size"])
				}
			},
		},
		{
			name: "network policy with allowed domains",
			form: "releaseName=x&botType=picoclaw&ingress=on&allowedDomains=api.openai.com%0A*.anthropic.com",
			check: func(t *testing.T, opts service.InstallOptions) {
				np := opts.Values["networkPolicy"].(map[string]any)
				if np["ingress"] != true {
					t.Error("ingress not set")
				}
				if np["egress"] != false {
					t.Error("egress should be false")
				}
				domains, ok := np["allowedDomains"].([]string)
				if !ok || len(domains) != 2 {
					t.Fatalf("allowedDomains = %v", np["allowedDomains"])
				}
				if domains[0] != "api.openai.com" || domains[1] != "*.anthropic.com" {
					t.Errorf("domains = %v", domains)
				}
				if np["useCilium"] != true {
					t.Error("useCilium should be true when domains set")
				}
			},
		},
		{
			name: "egress on skips allowed domains",
			form: "releaseName=x&botType=picoclaw&egress=on&allowedDomains=ignored.com",
			check: func(t *testing.T, opts service.InstallOptions) {
				np := opts.Values["networkPolicy"].(map[string]any)
				if np["egress"] != true {
					t.Error("egress not set")
				}
				if _, ok := np["allowedDomains"]; ok {
					t.Error("allowedDomains should not be set when egress is on")
				}
			},
		},
		{
			name: "workspace s3",
			form: "releaseName=x&botType=picoclaw&workspaceEnabled=on&workspaceProvider=s3&wsS3Bucket=mybucket&wsS3Region=us-west-2",
			check: func(t *testing.T, opts service.InstallOptions) {
				ws := opts.Values["workspace"].(map[string]any)
				if ws["enabled"] != true {
					t.Error("workspace not enabled")
				}
				s3 := ws["s3"].(map[string]any)
				if s3["bucket"] != "mybucket" {
					t.Errorf("bucket = %v", s3["bucket"])
				}
			},
		},
		{
			name: "backup with credentials",
			form: "releaseName=x&botType=picoclaw&backupEnabled=on&backupProvider=s3&bkS3Bucket=bk&bkCredentialsAccessKey=AKIA&bkCredentialsSecretKey=secret",
			check: func(t *testing.T, opts service.InstallOptions) {
				bk := opts.Values["backup"].(map[string]any)
				if bk["enabled"] != true {
					t.Error("backup not enabled")
				}
				creds := opts.Values["backupCredentials"].(map[string]any)
				if creds["accessKeyId"] != "AKIA" {
					t.Errorf("accessKeyId = %v", creds["accessKeyId"])
				}
			},
		},
		{
			name: "workspace backup from secret refs with restore on startup",
			form: "releaseName=x&botType=openclaw&backupEnabled=on&backupSchedule=15+*+*+*+*&backupRestoreOnStartup=on&bkS3Endpoint=https%3A%2F%2Fs3.example.com&bkS3Bucket=workspace-bk&bkS3Region=us-west-2&bkS3Prefix=bots%2Fx&backupAccessKeySecret=1p%3Aaws-access&backupSecretKeySecret=1p%3Aaws-secret",
			check: func(t *testing.T, opts service.InstallOptions) {
				bk := opts.Values["backup"].(map[string]any)
				if bk["enabled"] != true {
					t.Error("backup not enabled")
				}
				if bk["restoreOnStartup"] != true {
					t.Error("restoreOnStartup not set")
				}
				if bk["provider"] != "s3" {
					t.Errorf("provider = %v, want s3", bk["provider"])
				}
				s3 := bk["s3"].(map[string]any)
				if s3["endpoint"] != "https://s3.example.com" {
					t.Errorf("endpoint = %v", s3["endpoint"])
				}
				if s3["bucket"] != "workspace-bk" {
					t.Errorf("bucket = %v", s3["bucket"])
				}
				if s3["region"] != "us-west-2" {
					t.Errorf("region = %v", s3["region"])
				}
				refs := opts.Values["backupCredentialSecretRefs"].(map[string]any)
				if refs["accessKeyId"] != "aws-access" {
					t.Errorf("accessKeyId ref = %v", refs["accessKeyId"])
				}
				if refs["secretAccessKey"] != "aws-secret" {
					t.Errorf("secretAccessKey ref = %v", refs["secretAccessKey"])
				}
				if _, ok := opts.Values["backupCredentials"]; ok {
					t.Error("legacy backupCredentials should not be set when secret refs are provided")
				}
			},
		},
		{
			name: "workspace backup defaults restore off",
			form: "releaseName=x&botType=openclaw&backupEnabled=on&bkS3Bucket=workspace-bk",
			check: func(t *testing.T, opts service.InstallOptions) {
				bk := opts.Values["backup"].(map[string]any)
				if bk["restoreOnStartup"] != false {
					t.Errorf("restoreOnStartup = %v, want false", bk["restoreOnStartup"])
				}
				s3 := bk["s3"].(map[string]any)
				if s3["region"] != "us-east-1" {
					t.Errorf("default region = %v, want us-east-1", s3["region"])
				}
			},
		},
		{
			name: "config fields with text values",
			form: "releaseName=x&botType=picoclaw&cfg%3AdefaultModel=claude-sonnet-4-20250514",
			check: func(t *testing.T, opts service.InstallOptions) {
				if opts.ConfigFields["defaultModel"] != "claude-sonnet-4-20250514" {
					t.Errorf("configFields = %v", opts.ConfigFields)
				}
			},
		},
		{
			name: "extra software tool versions",
			form: "releaseName=x&botType=picoclaw&extraToolVersions=node+22%0Ajq+1.8.1",
			check: func(t *testing.T, opts service.InstallOptions) {
				extraSoftware, ok := opts.Values["extraSoftware"].(map[string]any)
				if !ok {
					t.Fatalf("extraSoftware missing from values: %#v", opts.Values)
				}
				if got := extraSoftware["toolVersions"]; got != "node 22\njq 1.8.1" {
					t.Errorf("toolVersions = %v, want multiline tool versions", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/bots", strings.NewReader(tt.form))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			opts, err := parseInstallForm(req, nil)
			if err != nil {
				t.Fatalf("parseInstallForm error: %v", err)
			}
			tt.check(t, opts)
		})
	}
}

func TestEnsureBuiltInPostgresPassword(t *testing.T) {
	t.Run("ironclaw defaults postgresql enabled and generates password", func(t *testing.T) {
		opts := &service.InstallOptions{
			BotType: service.BotTypeIronClaw,
			Values:  map[string]any{},
		}
		ensureBuiltInPostgresPassword(opts)

		pg, ok := opts.Values["postgresql"].(map[string]any)
		if !ok {
			t.Fatalf("postgresql map missing: %#v", opts.Values["postgresql"])
		}
		if pg["enabled"] != true {
			t.Fatalf("postgresql.enabled = %v, want true", pg["enabled"])
		}
		pw, _ := pg["password"].(string)
		if strings.TrimSpace(pw) == "" {
			t.Fatal("postgresql.password should be generated and non-empty")
		}
	})

	t.Run("ironclaw disabled postgresql keeps password unset", func(t *testing.T) {
		opts := &service.InstallOptions{
			BotType: service.BotTypeIronClaw,
			Values: map[string]any{
				"postgresql": map[string]any{
					"enabled": false,
				},
			},
		}
		ensureBuiltInPostgresPassword(opts)

		pg := opts.Values["postgresql"].(map[string]any)
		if _, exists := pg["password"]; exists {
			t.Fatalf("postgresql.password should not be set when postgres is disabled: %v", pg["password"])
		}
	})

	t.Run("non-ironclaw untouched", func(t *testing.T) {
		opts := &service.InstallOptions{
			BotType: service.BotTypeOpenClaw,
			Values:  map[string]any{},
		}
		ensureBuiltInPostgresPassword(opts)
		if _, ok := opts.Values["postgresql"]; ok {
			t.Fatalf("postgresql should not be created for non-ironclaw bots: %#v", opts.Values["postgresql"])
		}
	})
}

func TestHelmHandler_Install_BackupCredentialSecretRefs_ResolveTargetSecrets(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	done := make(chan struct{})
	mock := &mockHelm{
		installed:   &service.ReleaseInfo{Name: "my-bot", Status: "deployed"},
		installDone: done,
	}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, &mockSecrets{
		extSecrets: []service.ExternalSecretInfo{
			{Name: "aws-access", TargetSecret: "aws-access-target", Status: "Synced"},
			{Name: "aws-secret", TargetSecret: "aws-secret-target", Status: "Synced"},
		},
	}, nil, reg, false)

	form := "releaseName=my-bot&botType=openclaw&backupEnabled=on&backupSchedule=0+*+*+*+*&bkS3Bucket=my-backups&backupAccessKeySecret=1p%3Aaws-access&backupSecretKeySecret=1p%3Aaws-secret"
	req := httptest.NewRequest(http.MethodPost, "/bots", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.Install(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}

	<-done

	backup := mock.installOpts.Values["backup"].(map[string]any)
	creds := backup["credentials"].(map[string]any)
	accessRef := creds["accessKeyIdSecretRef"].(map[string]any)
	secretRef := creds["secretAccessKeySecretRef"].(map[string]any)

	if accessRef["name"] != "aws-access-target" {
		t.Errorf("accessKey secret name = %v, want aws-access-target", accessRef["name"])
	}
	if accessRef["key"] != "value" {
		t.Errorf("accessKey secret key = %v, want value", accessRef["key"])
	}
	if secretRef["name"] != "aws-secret-target" {
		t.Errorf("secretAccessKey secret name = %v, want aws-secret-target", secretRef["name"])
	}
	if secretRef["key"] != "value" {
		t.Errorf("secretAccessKey secret key = %v, want value", secretRef["key"])
	}
	if _, ok := mock.installOpts.Values["backupCredentialSecretRefs"]; ok {
		t.Fatal("temporary backupCredentialSecretRefs should be removed before Helm install")
	}
}

func TestHelmHandler_Install_BackupCredentialSecretRefs_FallbackToExternalSecretName(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	done := make(chan struct{})
	mock := &mockHelm{
		installed:   &service.ReleaseInfo{Name: "my-bot", Status: "deployed"},
		installDone: done,
	}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, nil, nil, reg, false)

	form := "releaseName=my-bot&botType=openclaw&backupEnabled=on&bkS3Bucket=my-backups&backupAccessKeySecret=1p%3Aaws-access&backupSecretKeySecret=1p%3Aaws-secret"
	req := httptest.NewRequest(http.MethodPost, "/bots", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.Install(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}

	<-done

	backup := mock.installOpts.Values["backup"].(map[string]any)
	creds := backup["credentials"].(map[string]any)
	accessRef := creds["accessKeyIdSecretRef"].(map[string]any)
	secretRef := creds["secretAccessKeySecretRef"].(map[string]any)

	if accessRef["name"] != "aws-access" {
		t.Errorf("accessKey secret name = %v, want aws-access", accessRef["name"])
	}
	if secretRef["name"] != "aws-secret" {
		t.Errorf("secretAccessKey secret name = %v, want aws-secret", secretRef["name"])
	}
}

// TestInstall_DirectAPIKey_GeneratesModelList is a regression test for the bug
// where model_list was missing when the model name included a provider prefix
// (e.g. "anthropic/claude-sonnet-4.6"). API keys are typed directly in the
// providers form (not via 1Password) so the real key must appear in model_list.
func TestInstall_DirectAPIKey_GeneratesModelList(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	done := make(chan struct{})
	mock := &mockHelm{installed: &service.ReleaseInfo{Name: "my-bot", Status: "deployed"}, installDone: done}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, nil, nil, reg, false)

	form := "releaseName=my-bot&botType=picoclaw" +
		"&cfg%3AagentModel=anthropic%2Fclaude-sonnet-4.6" +
		"&cfg%3AanthropicApiKey=sk-ant-test123"
	req := httptest.NewRequest(http.MethodPost, "/bots", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.Install(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}

	// Wait for background goroutine before inspecting opts.
	<-done

	cfgFileAny, ok := mock.installOpts.Values["configFile"]
	if !ok {
		t.Fatal("configFile not present in install values")
	}
	cfgFile := cfgFileAny.(map[string]any)
	content, _ := cfgFile["content"].(string)
	if content == "" {
		t.Fatal("configFile.content is empty")
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(content), &cfg); err != nil {
		t.Fatalf("invalid config JSON: %v\n%s", err, content)
	}

	rawList, ok := cfg["model_list"]
	if !ok {
		t.Fatal("model_list missing from config when direct API key is provided")
	}
	list := rawList.([]any)
	if len(list) == 0 {
		t.Fatal("model_list is empty")
	}
	entry := list[0].(map[string]any)
	if entry["api_key"] != "sk-ant-test123" {
		t.Errorf("api_key = %v, want actual key sk-ant-test123 (not an env var reference)", entry["api_key"])
	}
	if entry["model_name"] != "anthropic/claude-sonnet-4.6" {
		t.Errorf("model_name = %v, want anthropic/claude-sonnet-4.6", entry["model_name"])
	}
	if entry["model"] != "anthropic/claude-sonnet-4.6" {
		t.Errorf("model = %v, want anthropic/claude-sonnet-4.6", entry["model"])
	}
}

// TestInstall_1PasswordSecret_ResolvedToConfigJSON verifies that when a field
// is submitted as a 1Password secret reference (1p:name), the handler resolves
// the actual K8s secret value and embeds it as plaintext in the config JSON.
// This is required for picoclaw, which reads tokens/keys directly from its JSON
// config file rather than from environment variables.
func TestInstall_1PasswordSecret_ResolvedToConfigJSON(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	done := make(chan struct{})
	mock := &mockHelm{installed: &service.ReleaseInfo{Name: "my-bot", Status: "deployed"}, installDone: done}

	mockSec := &mockSecrets{
		extSecrets: []service.ExternalSecretInfo{
			{
				Name:         "my-anthropic-key",
				TargetSecret: "my-anthropic-key-target",
				Status:       "Synced",
				DataKeys:     []string{"value"},
			},
		},
	}
	mockK8s := &mockKubernetes{
		secretData: map[string]map[string][]byte{
			"my-anthropic-key-target": {"value": []byte("sk-ant-actual-key-123")},
		},
	}

	h := NewHelmHandler(mock, &mockTemplate{}, nil, mockSec, mockK8s, reg, false)

	form := "releaseName=my-bot&botType=picoclaw" +
		"&cfg%3AagentModel=anthropic%2Fclaude-sonnet-4.6" +
		"&cfg%3AanthropicApiKey=1p%3Amy-anthropic-key"
	req := httptest.NewRequest(http.MethodPost, "/bots", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.Install(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}

	<-done

	cfgFileAny, ok := mock.installOpts.Values["configFile"]
	if !ok {
		t.Fatal("configFile not present in install values")
	}
	cfgFile := cfgFileAny.(map[string]any)
	content, _ := cfgFile["content"].(string)
	if content == "" {
		t.Fatal("configFile.content is empty")
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(content), &cfg); err != nil {
		t.Fatalf("invalid config JSON: %v\n%s", err, content)
	}

	rawList, ok := cfg["model_list"]
	if !ok {
		t.Fatal("model_list missing from config — 1Password secret was not resolved")
	}
	list := rawList.([]any)
	if len(list) == 0 {
		t.Fatal("model_list is empty")
	}
	entry := list[0].(map[string]any)
	if entry["api_key"] != "sk-ant-actual-key-123" {
		t.Errorf("api_key = %v, want resolved plaintext sk-ant-actual-key-123", entry["api_key"])
	}
}

func TestInstall_OpenClaw_ConfigFieldsUseValuesAndEnvSecretsOnly(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	done := make(chan struct{})
	mock := &mockHelm{installed: &service.ReleaseInfo{Name: "dorothy", Status: "deployed"}, installDone: done}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, nil, nil, reg, false)

	body, _ := json.Marshal(map[string]any{
		"releaseName": "dorothy",
		"botType":     "openclaw",
		"configFields": map[string]string{
			"authChoice":      "apiKey",
			"anthropicApiKey": "1p:anthropic-main",
			"discordEnabled":  "true",
			"discordBotToken": "1p:discord-main",
			"defaultModel":    "anthropic/claude-sonnet-4-5-20250929",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/bots", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Install(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}
	<-done

	if _, ok := mock.installOpts.Values["configFile"]; ok {
		t.Fatal("openclaw install must not generate configFile values")
	}

	auth, ok := mock.installOpts.Values["auth"].(map[string]any)
	if !ok {
		t.Fatalf("auth values missing or wrong type: %#v", mock.installOpts.Values["auth"])
	}
	if auth["choice"] != "apiKey" {
		t.Fatalf("auth.choice = %v, want apiKey", auth["choice"])
	}

	agent, ok := mock.installOpts.Values["agent"].(map[string]any)
	if !ok {
		t.Fatalf("agent values missing or wrong type: %#v", mock.installOpts.Values["agent"])
	}
	if agent["defaultModel"] != "anthropic/claude-sonnet-4-5-20250929" {
		t.Fatalf("agent.defaultModel = %v", agent["defaultModel"])
	}

	channels, ok := mock.installOpts.Values["channels"].(map[string]any)
	if !ok {
		t.Fatalf("channels values missing or wrong type: %#v", mock.installOpts.Values["channels"])
	}
	discord, ok := channels["discord"].(map[string]any)
	if !ok {
		t.Fatalf("channels.discord missing or wrong type: %#v", channels["discord"])
	}
	if discord["enabled"] != true {
		t.Fatalf("channels.discord.enabled = %v, want true", discord["enabled"])
	}

	envSecrets, ok := mock.installOpts.Values["envSecrets"].([]any)
	if !ok || len(envSecrets) == 0 {
		t.Fatalf("envSecrets missing from openclaw install values: %#v", mock.installOpts.Values["envSecrets"])
	}

	byEnv := map[string]string{}
	for _, raw := range envSecrets {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		envVar, _ := m["envVar"].(string)
		secretRef, _ := m["secretRef"].(map[string]any)
		secretName, _ := secretRef["name"].(string)
		byEnv[envVar] = secretName
	}

	if byEnv["ANTHROPIC_API_KEY"] != "anthropic-main" {
		t.Fatalf("ANTHROPIC_API_KEY secret = %q, want anthropic-main", byEnv["ANTHROPIC_API_KEY"])
	}
	if byEnv["DISCORD_BOT_TOKEN"] != "discord-main" {
		t.Fatalf("DISCORD_BOT_TOKEN secret = %q, want discord-main", byEnv["DISCORD_BOT_TOKEN"])
	}
}

func TestHelmHandler_DetailPage_RedactsJSONConfigSecrets(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	const rawConfig = `{
  "channels": {
    "discord": {
      "enabled": true,
      "token": "discord-secret-value"
    }
  },
  "model_list": [
    {
      "model_name": "anthropic/claude-sonnet-4.6",
      "model": "anthropic/claude-sonnet-4.6",
      "api_key": "model-provider-secret"
    }
  ]
}`

	tmpl := &mockTemplate{}
	mock := &mockHelm{
		statusInfo: &service.ReleaseInfo{Name: "my-bot", BotType: "picoclaw", Status: "deployed"},
		values: map[string]any{
			"configFile": map[string]any{
				"content": rawConfig,
			},
		},
	}

	h := NewHelmHandler(mock, tmpl, nil, nil, nil, reg, false)
	req := httptest.NewRequest(http.MethodGet, "/bots/my-bot/page", nil)
	req.SetPathValue("name", "my-bot")
	rec := httptest.NewRecorder()

	h.DetailPage(rec, req)

	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}

	data, ok := tmpl.calls[0].data.(BotDetailData)
	if !ok {
		t.Fatalf("unexpected data type: %T", tmpl.calls[0].data)
	}

	if strings.Contains(data.ConfigContent, "discord-secret-value") {
		t.Fatal("detail page leaked plaintext discord token")
	}
	if strings.Contains(data.ConfigContent, "model-provider-secret") {
		t.Fatal("detail page leaked plaintext model_list api_key")
	}
	if !strings.Contains(data.ConfigContent, secretTokenCurrent) {
		t.Fatalf("secret preserve token %q not found in rendered config", secretTokenCurrent)
	}
	if !data.ConfigEditable {
		t.Fatal("expected JSON config to be editable")
	}
}

func TestHelmHandler_UpdateConfig_DirectJSONPreservesSecrets(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	const currentContent = `{
  "agents": {
    "defaults": {
      "max_tokens": 8192
    }
  },
  "channels": {
    "discord": {
      "enabled": true,
      "token": "discord-secret-value"
    }
  },
  "model_list": [
    {
      "model_name": "anthropic/claude-sonnet-4.6",
      "model": "anthropic/claude-sonnet-4.6",
      "api_key": "model-provider-secret"
    }
  ]
}`
	const editedContent = `{
  "agents": {
    "defaults": {
      "max_tokens": 4096
    }
  },
  "channels": {
    "discord": {
      "enabled": true,
      "token": "__CLAWMACHINE_REDACTED__"
    }
  },
  "model_list": [
    {
      "model_name": "anthropic/claude-3-7-sonnet",
      "model": "anthropic/claude-3-7-sonnet",
      "api_key": "__CLAWMACHINE_REDACTED__"
    }
  ]
}`

	mock := &mockHelm{
		values: map[string]any{
			"env": map[string]any{
				"BOT_TYPE": "picoclaw",
			},
			"configFile": map[string]any{
				"content": currentContent,
			},
		},
		upgraded: &service.ReleaseInfo{Name: "my-bot", BotType: "picoclaw", Status: "deployed"},
	}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, nil, nil, reg, false)

	body, _ := json.Marshal(map[string]any{"configContent": editedContent})
	req := httptest.NewRequest(http.MethodPut, "/bots/my-bot/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "my-bot")
	rec := httptest.NewRecorder()

	h.UpdateConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}

	cfgFileAny, ok := mock.upgradeValues["configFile"]
	if !ok {
		t.Fatal("configFile missing from upgrade values")
	}
	cfgFile := cfgFileAny.(map[string]any)
	updated, _ := cfgFile["content"].(string)

	var cfg map[string]any
	if err := json.Unmarshal([]byte(updated), &cfg); err != nil {
		t.Fatalf("updated content is invalid JSON: %v\n%s", err, updated)
	}

	discord := cfg["channels"].(map[string]any)["discord"].(map[string]any)
	if discord["token"] != "discord-secret-value" {
		t.Fatalf("discord token was not preserved, got %v", discord["token"])
	}

	modelList := cfg["model_list"].([]any)
	entry := modelList[0].(map[string]any)
	if entry["api_key"] != "model-provider-secret" {
		t.Fatalf("model_list api_key was not preserved, got %v", entry["api_key"])
	}

	maxTokens := cfg["agents"].(map[string]any)["defaults"].(map[string]any)["max_tokens"]
	if maxTokens != float64(4096) {
		t.Fatalf("max_tokens = %v, want 4096", maxTokens)
	}
}

func TestHelmHandler_DetailPage_RendersMappedSecretTokens(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	const rawConfig = `{
  "channels": {
    "discord": {
      "enabled": true,
      "token": "discord-secret-value"
    }
  }
}`

	tmpl := &mockTemplate{}
	mock := &mockHelm{
		statusInfo: &service.ReleaseInfo{Name: "my-bot", BotType: "picoclaw", Status: "deployed"},
		values: map[string]any{
			"configFile": map[string]any{
				"content": rawConfig,
			},
		},
	}
	secrets := &mockSecrets{
		extSecrets: []service.ExternalSecretInfo{
			{Name: "discord-main", Status: "Synced", TargetSecret: "discord-target", DataKeys: []string{"value"}},
		},
	}
	k8s := &mockKubernetes{
		secretData: map[string]map[string][]byte{
			"discord-target": {"value": []byte("discord-secret-value")},
		},
	}

	h := NewHelmHandler(mock, tmpl, nil, secrets, k8s, reg, false)
	req := httptest.NewRequest(http.MethodGet, "/bots/my-bot/page", nil)
	req.SetPathValue("name", "my-bot")
	rec := httptest.NewRecorder()

	h.DetailPage(rec, req)

	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	data, ok := tmpl.calls[0].data.(BotDetailData)
	if !ok {
		t.Fatalf("unexpected data type: %T", tmpl.calls[0].data)
	}

	if strings.Contains(data.ConfigContent, "discord-secret-value") {
		t.Fatal("detail page leaked plaintext discord token")
	}
	if !strings.Contains(data.ConfigContent, "{{secret.discord-main}}") {
		t.Fatalf("mapped secret token not found in config: %s", data.ConfigContent)
	}
}

func TestHelmHandler_DetailPage_OpenClawShowsConfigTab(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	tmpl := &mockTemplate{}
	mock := &mockHelm{
		statusInfo: &service.ReleaseInfo{Name: "dorothy", BotType: "openclaw", Status: "deployed"},
		values: map[string]any{
			"configFile": map[string]any{
				"enabled": true,
				"content": "{}",
			},
		},
	}
	h := NewHelmHandler(mock, tmpl, nil, nil, nil, reg, false)
	req := httptest.NewRequest(http.MethodGet, "/bots/dorothy/page", nil)
	req.SetPathValue("name", "dorothy")
	rec := httptest.NewRecorder()

	h.DetailPage(rec, req)

	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	data, ok := tmpl.calls[0].data.(BotDetailData)
	if !ok {
		t.Fatalf("unexpected data type: %T", tmpl.calls[0].data)
	}
	if !data.ConfigTab {
		t.Fatal("expected config tab to be enabled for openclaw")
	}
	if !data.ConfigEditable {
		t.Fatal("expected openclaw config to be editable")
	}
}

func TestHelmHandler_DetailPage_IncludesPodHealthStatus(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	tmpl := &mockTemplate{}
	mock := &mockHelm{
		statusInfo: &service.ReleaseInfo{Name: "dorothy", BotType: "openclaw", Status: "deployed"},
		values: map[string]any{
			"env": map[string]any{
				"BOT_TYPE": "openclaw",
			},
		},
	}
	k8s := &mockKubernetes{
		podHealthy: true,
	}

	h := NewHelmHandler(mock, tmpl, nil, nil, k8s, reg, false)
	req := httptest.NewRequest(http.MethodGet, "/bots/dorothy/page", nil)
	req.SetPathValue("name", "dorothy")
	rec := httptest.NewRecorder()

	h.DetailPage(rec, req)

	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	data, ok := tmpl.calls[0].data.(BotDetailData)
	if !ok {
		t.Fatalf("unexpected data type: %T", tmpl.calls[0].data)
	}
	if data.PodStatus != "healthy" {
		t.Fatalf("PodStatus = %q, want healthy", data.PodStatus)
	}
}

func TestHelmHandler_DetailPage_OpenClawFromEnvBotTypeShowsConfigTab(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	tmpl := &mockTemplate{}
	mock := &mockHelm{
		// Simulate status lacking bot type while values include env.BOT_TYPE.
		statusInfo: &service.ReleaseInfo{Name: "dorothy", BotType: "", Status: "deployed"},
		values: map[string]any{
			"env": map[string]any{
				"BOT_TYPE": "openclaw",
			},
		},
	}
	h := NewHelmHandler(mock, tmpl, nil, nil, nil, reg, false)
	req := httptest.NewRequest(http.MethodGet, "/bots/dorothy/page", nil)
	req.SetPathValue("name", "dorothy")
	rec := httptest.NewRecorder()

	h.DetailPage(rec, req)

	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	data, ok := tmpl.calls[0].data.(BotDetailData)
	if !ok {
		t.Fatalf("unexpected data type: %T", tmpl.calls[0].data)
	}
	if !data.ConfigTab {
		t.Fatal("expected config tab to be enabled when env.BOT_TYPE=openclaw")
	}
	if !data.ConfigEditable {
		t.Fatal("expected config to be editable when env.BOT_TYPE=openclaw")
	}
}

func TestHelmHandler_DetailPage_OpenClawLoadsConfigFromPodWhenValuesMissing(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	tmpl := &mockTemplate{}
	mock := &mockHelm{
		statusInfo: &service.ReleaseInfo{Name: "dorothy", BotType: "openclaw", Status: "deployed"},
		values: map[string]any{
			"env": map[string]any{
				"BOT_TYPE": "openclaw",
			},
		},
	}
	k8s := &mockKubernetes{
		execStdout: `{"gateway":{"port":18789}}`,
	}
	h := NewHelmHandler(mock, tmpl, nil, nil, k8s, reg, false)
	req := httptest.NewRequest(http.MethodGet, "/bots/dorothy/page", nil)
	req.SetPathValue("name", "dorothy")
	rec := httptest.NewRecorder()

	h.DetailPage(rec, req)

	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	data, ok := tmpl.calls[0].data.(BotDetailData)
	if !ok {
		t.Fatalf("unexpected data type: %T", tmpl.calls[0].data)
	}
	if !strings.Contains(data.ConfigContent, `"gateway"`) {
		t.Fatalf("expected live pod config content, got: %s", data.ConfigContent)
	}
	if k8s.execContainer != "openclaw" {
		t.Fatalf("exec container = %q, want openclaw", k8s.execContainer)
	}
}

func TestHelmHandler_DetailPage_OpenClawPrefersPodConfigOverValues(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	tmpl := &mockTemplate{}
	mock := &mockHelm{
		statusInfo: &service.ReleaseInfo{Name: "dorothy", BotType: "openclaw", Status: "deployed"},
		values: map[string]any{
			"env": map[string]any{
				"BOT_TYPE": "openclaw",
			},
			"configFile": map[string]any{
				"enabled": true,
				"content": `{"from":"values"}`,
			},
		},
	}
	k8s := &mockKubernetes{
		execStdout: `{"from":"pod"}`,
	}
	h := NewHelmHandler(mock, tmpl, nil, nil, k8s, reg, false)
	req := httptest.NewRequest(http.MethodGet, "/bots/dorothy/page", nil)
	req.SetPathValue("name", "dorothy")
	rec := httptest.NewRecorder()

	h.DetailPage(rec, req)

	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	data, ok := tmpl.calls[0].data.(BotDetailData)
	if !ok {
		t.Fatalf("unexpected data type: %T", tmpl.calls[0].data)
	}
	if !strings.Contains(data.ConfigContent, `"pod"`) {
		t.Fatalf("expected pod config to be rendered, got: %s", data.ConfigContent)
	}
	if strings.Contains(data.ConfigContent, `"values"`) {
		t.Fatalf("expected values config to be ignored, got: %s", data.ConfigContent)
	}
}

func TestHelmHandler_UpdateConfig_DirectJSONRejectsSecretEdits(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	const currentContent = `{
  "channels": {
    "discord": {
      "enabled": true,
      "token": "discord-secret-value"
    }
  }
}`
	const editedContent = `{
  "channels": {
    "discord": {
      "enabled": true,
      "token": "new-secret-value"
    }
  }
}`

	mock := &mockHelm{
		values: map[string]any{
			"env": map[string]any{
				"BOT_TYPE": "picoclaw",
			},
			"configFile": map[string]any{
				"content": currentContent,
			},
		},
	}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, nil, nil, reg, false)

	body, _ := json.Marshal(map[string]any{"configContent": editedContent})
	req := httptest.NewRequest(http.MethodPut, "/bots/my-bot/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "my-bot")
	rec := httptest.NewRecorder()

	h.UpdateConfig(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "cannot be edited") {
		t.Fatalf("unexpected error body: %s", rec.Body.String())
	}
	if mock.upgradeValues != nil {
		t.Fatal("upgrade should not be called when secret edits are rejected")
	}
}

func TestHelmHandler_UpdateConfig_DirectJSONResolvesSecretTokens(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	const currentContent = `{
  "channels": {
    "discord": {
      "enabled": true,
      "token": "discord-secret-value"
    }
  }
}`
	const editedContent = `{
  "channels": {
    "discord": {
      "enabled": true,
      "token": "{{secret.discord-rotated}}"
    }
  }
}`

	mock := &mockHelm{
		values: map[string]any{
			"env": map[string]any{
				"BOT_TYPE": "picoclaw",
			},
			"configFile": map[string]any{
				"content": currentContent,
			},
		},
		upgraded: &service.ReleaseInfo{Name: "my-bot", BotType: "picoclaw", Status: "deployed"},
	}
	secrets := &mockSecrets{
		extSecrets: []service.ExternalSecretInfo{
			{Name: "discord-main", Status: "Synced", TargetSecret: "discord-main-target", DataKeys: []string{"value"}},
			{Name: "discord-rotated", Status: "Synced", TargetSecret: "discord-rotated-target", DataKeys: []string{"value"}},
		},
	}
	k8s := &mockKubernetes{
		secretData: map[string]map[string][]byte{
			"discord-main-target":    {"value": []byte("discord-secret-value")},
			"discord-rotated-target": {"value": []byte("new-rotated-secret")},
		},
	}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, secrets, k8s, reg, false)

	body, _ := json.Marshal(map[string]any{"configContent": editedContent})
	req := httptest.NewRequest(http.MethodPut, "/bots/my-bot/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "my-bot")
	rec := httptest.NewRecorder()

	h.UpdateConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}

	cfgFile := mock.upgradeValues["configFile"].(map[string]any)
	updated, _ := cfgFile["content"].(string)
	if !strings.Contains(updated, "new-rotated-secret") {
		t.Fatalf("resolved secret value missing from updated config: %s", updated)
	}
	if strings.Contains(updated, "{{secret.discord-rotated}}") {
		t.Fatalf("token should be resolved before save: %s", updated)
	}
}

func TestHelmHandler_UpdateConfig_DirectJSONRejectsUnknownSecretTokens(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	const currentContent = `{
  "channels": {
    "discord": {
      "enabled": true,
      "token": "discord-secret-value"
    }
  }
}`
	const editedContent = `{
  "channels": {
    "discord": {
      "enabled": true,
      "token": "{{secret.missing-secret}}"
    }
  }
}`

	mock := &mockHelm{
		values: map[string]any{
			"env": map[string]any{
				"BOT_TYPE": "picoclaw",
			},
			"configFile": map[string]any{
				"content": currentContent,
			},
		},
	}
	secrets := &mockSecrets{
		extSecrets: []service.ExternalSecretInfo{
			{Name: "discord-main", Status: "Synced", TargetSecret: "discord-main-target", DataKeys: []string{"value"}},
		},
	}
	k8s := &mockKubernetes{
		secretData: map[string]map[string][]byte{
			"discord-main-target": {"value": []byte("discord-secret-value")},
		},
	}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, secrets, k8s, reg, false)

	body, _ := json.Marshal(map[string]any{"configContent": editedContent})
	req := httptest.NewRequest(http.MethodPut, "/bots/my-bot/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "my-bot")
	rec := httptest.NewRecorder()

	h.UpdateConfig(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unknown secret") {
		t.Fatalf("unexpected error body: %s", rec.Body.String())
	}
	if mock.upgradeValues != nil {
		t.Fatal("upgrade should not be called when secret token is unknown")
	}
}

func TestHelmHandler_UpdateConfig_ConfigFieldsStillSupported(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	mock := &mockHelm{
		values: map[string]any{
			"env": map[string]any{
				"BOT_TYPE": "picoclaw",
			},
		},
		upgraded: &service.ReleaseInfo{Name: "my-bot", BotType: "picoclaw", Status: "deployed"},
	}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, nil, nil, reg, false)

	body, _ := json.Marshal(map[string]any{
		"configFields": map[string]string{
			"agentMaxTokens": "2048",
			"discordEnabled": "true",
			"discordToken":   "discord-token-123",
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/bots/my-bot/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "my-bot")
	rec := httptest.NewRecorder()

	h.UpdateConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}
	if mock.upgradeValues == nil {
		t.Fatal("expected upgrade values to be populated")
	}
	cfgFileAny, ok := mock.upgradeValues["configFile"]
	if !ok {
		t.Fatal("configFile missing from upgrade values")
	}
	cfgFile := cfgFileAny.(map[string]any)
	content, _ := cfgFile["content"].(string)
	if strings.TrimSpace(content) == "" {
		t.Fatal("configFile.content is empty")
	}
}

func TestHelmHandler_UpdateConfig_OpenClawAllowsDirectJSONEdits(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	mock := &mockHelm{
		values: map[string]any{
			"env": map[string]any{
				"BOT_TYPE": "openclaw",
			},
			"configFile": map[string]any{
				"enabled": true,
				"content": "{}",
			},
		},
	}
	mockK8s := &mockKubernetes{
		execStdout: `{"gateway":{"port":18789}}`,
	}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, nil, mockK8s, reg, false)

	body, _ := json.Marshal(map[string]any{"configContent": `{"gateway":{"port":18789}}`})
	req := httptest.NewRequest(http.MethodPut, "/bots/dorothy/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "dorothy")
	rec := httptest.NewRecorder()

	h.UpdateConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}
	if mock.upgradeValues != nil {
		t.Fatal("direct OpenClaw JSON edits should not run Helm upgrade")
	}
	if mockK8s.execContainer != "openclaw" {
		t.Fatalf("exec container = %q, want openclaw", mockK8s.execContainer)
	}
	if len(mockK8s.execCommand) < 3 {
		t.Fatalf("expected pod write command, got %#v", mockK8s.execCommand)
	}
	if !strings.Contains(mockK8s.execCommand[2], "openclaw.json") {
		t.Fatalf("pod write script missing openclaw.json path: %s", mockK8s.execCommand[2])
	}
}

func TestHelmHandler_UpdateConfig_OpenClawConfigFieldsUseValuesAndEnvSecretsOnly(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	mock := &mockHelm{
		values: map[string]any{
			"env": map[string]any{
				"BOT_TYPE": "openclaw",
			},
			// Ensure legacy config file values are not re-used after upgrade.
			"configFile": map[string]any{
				"enabled": true,
				"content": "{}",
			},
		},
		upgraded: &service.ReleaseInfo{Name: "dorothy", BotType: "openclaw", Status: "deployed"},
	}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, nil, nil, reg, false)

	body, _ := json.Marshal(map[string]any{
		"configFields": map[string]string{
			"authChoice":      "openai-api-key",
			"openaiApiKey":    "1p:openai-main",
			"discordEnabled":  "true",
			"discordBotToken": "1p:discord-main",
			"defaultModel":    "openai/gpt-5",
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/bots/dorothy/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "dorothy")
	rec := httptest.NewRecorder()

	h.UpdateConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}
	if mock.upgradeValues == nil {
		t.Fatal("expected upgrade values to be set")
	}
	if _, ok := mock.upgradeValues["configFile"]; ok {
		t.Fatal("openclaw config update must not write configFile values")
	}

	auth := mock.upgradeValues["auth"].(map[string]any)
	if auth["choice"] != "openai-api-key" {
		t.Fatalf("auth.choice = %v, want openai-api-key", auth["choice"])
	}
	agent := mock.upgradeValues["agent"].(map[string]any)
	if agent["defaultModel"] != "openai/gpt-5" {
		t.Fatalf("agent.defaultModel = %v, want openai/gpt-5", agent["defaultModel"])
	}

	channels := mock.upgradeValues["channels"].(map[string]any)
	discord := channels["discord"].(map[string]any)
	if discord["enabled"] != true {
		t.Fatalf("channels.discord.enabled = %v, want true", discord["enabled"])
	}

	envSecrets := mock.upgradeValues["envSecrets"].([]any)
	byEnv := map[string]string{}
	for _, raw := range envSecrets {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		envVar, _ := m["envVar"].(string)
		secretRef, _ := m["secretRef"].(map[string]any)
		secretName, _ := secretRef["name"].(string)
		byEnv[envVar] = secretName
	}
	if byEnv["OPENAI_API_KEY"] != "openai-main" {
		t.Fatalf("OPENAI_API_KEY secret = %q, want openai-main", byEnv["OPENAI_API_KEY"])
	}
	if byEnv["DISCORD_BOT_TOKEN"] != "discord-main" {
		t.Fatalf("DISCORD_BOT_TOKEN secret = %q, want discord-main", byEnv["DISCORD_BOT_TOKEN"])
	}
}

func TestHelmHandler_ExecCLI_OpenClawSuccess(t *testing.T) {
	tmpl := &mockTemplate{}
	mock := &mockHelm{
		statusInfo: &service.ReleaseInfo{Name: "dorothy", BotType: "openclaw", Status: "deployed"},
	}
	mockK8s := &mockKubernetes{
		execStdout: "approved",
	}

	h := NewHelmHandler(mock, tmpl, nil, nil, mockK8s, nil, false)

	req := httptest.NewRequest(http.MethodPost, "/bots/dorothy/cli", strings.NewReader("args=pairing+approve+discord+123456"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("name", "dorothy")
	rec := httptest.NewRecorder()

	h.ExecCLI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}

	if mockK8s.execRelease != "dorothy" {
		t.Fatalf("exec release = %q, want dorothy", mockK8s.execRelease)
	}
	if mockK8s.execContainer != "openclaw" {
		t.Fatalf("exec container = %q, want openclaw", mockK8s.execContainer)
	}

	wantCommand := []string{"openclaw", "pairing", "approve", "discord", "123456"}
	if len(mockK8s.execCommand) != len(wantCommand) {
		t.Fatalf("exec command length = %d, want %d (%v)", len(mockK8s.execCommand), len(wantCommand), mockK8s.execCommand)
	}
	for i := range wantCommand {
		if mockK8s.execCommand[i] != wantCommand[i] {
			t.Fatalf("exec command[%d] = %q, want %q", i, mockK8s.execCommand[i], wantCommand[i])
		}
	}

	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template render call, got %d", len(tmpl.calls))
	}
	if tmpl.calls[0].name != "bot-cli-result" {
		t.Fatalf("template = %q, want bot-cli-result", tmpl.calls[0].name)
	}
	data, ok := tmpl.calls[0].data.(botCLIResultData)
	if !ok {
		t.Fatalf("unexpected template data type: %T", tmpl.calls[0].data)
	}
	if !data.Success {
		t.Fatalf("expected success=true, got false with error %q", data.Error)
	}
	if data.Stdout != "approved" {
		t.Fatalf("stdout = %q, want %q", data.Stdout, "approved")
	}
}

func TestHelmHandler_ExecCLI_StripsOpenClawPrefix(t *testing.T) {
	mock := &mockHelm{
		statusInfo: &service.ReleaseInfo{Name: "dorothy", BotType: "openclaw", Status: "deployed"},
	}
	mockK8s := &mockKubernetes{}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, nil, mockK8s, nil, false)

	body := `{"args":"openclaw pairing approve discord 777777"}`
	req := httptest.NewRequest(http.MethodPost, "/bots/dorothy/cli", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "dorothy")
	rec := httptest.NewRecorder()

	h.ExecCLI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}

	wantCommand := []string{"openclaw", "pairing", "approve", "discord", "777777"}
	if len(mockK8s.execCommand) != len(wantCommand) {
		t.Fatalf("exec command length = %d, want %d", len(mockK8s.execCommand), len(wantCommand))
	}
	for i := range wantCommand {
		if mockK8s.execCommand[i] != wantCommand[i] {
			t.Fatalf("exec command[%d] = %q, want %q", i, mockK8s.execCommand[i], wantCommand[i])
		}
	}

	var got map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}
	if ok, _ := got["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %+v", got)
	}
}

func TestHelmHandler_ExecCLI_RejectsNonOpenClaw(t *testing.T) {
	mock := &mockHelm{
		statusInfo: &service.ReleaseInfo{Name: "mybot", BotType: "picoclaw", Status: "deployed"},
	}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, nil, &mockKubernetes{}, nil, false)

	req := httptest.NewRequest(http.MethodPost, "/bots/mybot/cli", strings.NewReader("args=pairing+approve+discord+123"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("name", "mybot")
	rec := httptest.NewRecorder()

	h.ExecCLI(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "openclaw") {
		t.Fatalf("expected openclaw-only message, got: %s", rec.Body.String())
	}
}

func TestHelmHandler_ExecCLI_RejectsEmptyArgs(t *testing.T) {
	mock := &mockHelm{
		statusInfo: &service.ReleaseInfo{Name: "dorothy", BotType: "openclaw", Status: "deployed"},
	}
	h := NewHelmHandler(mock, &mockTemplate{}, nil, nil, &mockKubernetes{}, nil, false)

	req := httptest.NewRequest(http.MethodPost, "/bots/dorothy/cli", strings.NewReader("args=+++"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("name", "dorothy")
	rec := httptest.NewRecorder()

	h.ExecCLI(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "required") {
		t.Fatalf("expected required message, got: %s", rec.Body.String())
	}
}

func TestHelmHandler_ExecCLI_ExecErrorRendersResult(t *testing.T) {
	tmpl := &mockTemplate{}
	mock := &mockHelm{
		statusInfo: &service.ReleaseInfo{Name: "dorothy", BotType: "openclaw", Status: "deployed"},
	}
	mockK8s := &mockKubernetes{
		execStderr: "invalid code",
		execErr:    errors.New("exit status 1"),
	}
	h := NewHelmHandler(mock, tmpl, nil, nil, mockK8s, nil, false)

	req := httptest.NewRequest(http.MethodPost, "/bots/dorothy/cli", strings.NewReader("args=pairing+approve+discord+bad"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("name", "dorothy")
	rec := httptest.NewRecorder()

	h.ExecCLI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
	}
	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template render call, got %d", len(tmpl.calls))
	}
	data, ok := tmpl.calls[0].data.(botCLIResultData)
	if !ok {
		t.Fatalf("unexpected template data type: %T", tmpl.calls[0].data)
	}
	if data.Success {
		t.Fatal("expected success=false")
	}
	if data.Error == "" {
		t.Fatal("expected error to be populated")
	}
	if data.Stderr != "invalid code" {
		t.Fatalf("stderr = %q, want %q", data.Stderr, "invalid code")
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"a\nb\nc", []string{"a", "b", "c"}},
		{"  a  \n  \n  b  ", []string{"a", "b"}},
	}
	for _, tt := range tests {
		got := splitLines(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitLines(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitLines(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestIsHTMX(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{"no header", "", false},
		{"true header", "true", true},
		{"false header", "false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("HX-Request", tt.header)
			}
			if got := isHTMX(req); got != tt.want {
				t.Errorf("isHTMX() = %v, want %v", got, tt.want)
			}
		})
	}
}
