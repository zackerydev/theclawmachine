package handler

import (
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/zackerydev/clawmachine/control-plane/internal/service"
)

// validK8sName matches valid Kubernetes resource names (RFC 1123 subdomain).
var validK8sName = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?$`)

type SecretsHandler struct {
	secrets SecretsManager
	connect ConnectServicer
	tmpl    TemplateRenderer
}

func NewSecretsHandler(secrets SecretsManager, connect ConnectServicer, tmpl TemplateRenderer) *SecretsHandler {
	return &SecretsHandler{secrets: secrets, connect: connect, tmpl: tmpl}
}

// Page handlers

func (h *SecretsHandler) SettingsPage(w http.ResponseWriter, r *http.Request) {
	storeStatus, err := h.secrets.GetSecretStoreStatus(r.Context(), defaultNamespace())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	secretCount := 0
	if storeStatus.Configured {
		secrets, err := h.secrets.ListExternalSecrets(r.Context(), defaultNamespace())
		if err == nil {
			secretCount = len(secrets)
		}
	}

	providerCount := 0
	if storeStatus.Configured {
		providerCount = 1
	}

	data := struct {
		ProviderConfigured bool
		ProviderCount      int
		SecretCount        int
	}{storeStatus.Configured, providerCount, secretCount}

	if !renderOrError(w, r, h.tmpl, "settings", data, isHTMX(r)) {
		return
	}
}

func (h *SecretsHandler) ProvidersPage(w http.ResponseWriter, r *http.Request) {
	storeStatus, err := h.secrets.GetSecretStoreStatus(r.Context(), defaultNamespace())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	connectStatus, err := h.connect.GetStatus(r.Context())
	if err != nil {
		connectStatus = &service.ConnectStatus{Installed: false}
	}

	data := struct {
		*service.SecretStoreStatus
		Connect *service.ConnectStatus
	}{storeStatus, connectStatus}

	if !renderOrError(w, r, h.tmpl, "providers", data, isHTMX(r)) {
		return
	}
}

func (h *SecretsHandler) InstallConnect(w http.ResponseWriter, r *http.Request) {
	var opts struct {
		CredentialsJSON string `json:"credentialsJson"`
		Token           string `json:"token"`
		VaultName       string `json:"vaultName"`
	}
	if err := parseRequest(r, &opts); err != nil {
		htmxError(w, r, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if opts.CredentialsJSON == "" || opts.Token == "" || opts.VaultName == "" {
		htmxError(w, r, "Credentials JSON, token, and vault name are all required", http.StatusBadRequest)
		return
	}

	slog.Info("connect: installing 1Password Connect Server", "vault", opts.VaultName)
	if err := h.connect.Install(r.Context(), opts.CredentialsJSON, opts.Token); err != nil {
		slog.Error("connect: install failed", "error", err)
		htmxError(w, r, "Failed to install Connect Server: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("connect: installed successfully")

	// Auto-configure the ESO SecretStore pointing at the in-cluster service.
	connectHost := "http://onepassword-connect.1password.svc:8080"
	storeOpts := service.CreateSecretStoreOptions{
		ConnectHost:  connectHost,
		ConnectToken: opts.Token,
		VaultName:    opts.VaultName,
	}
	if err := h.secrets.CreateSecretStore(r.Context(), defaultNamespace(), storeOpts); err != nil {
		slog.Error("connect: SecretStore config failed", "error", err)
		htmxError(w, r, "Connect Server installed but failed to configure SecretStore: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("connect: SecretStore configured", "vault", opts.VaultName)

	htmxRedirectOrStatus(w, r, "/settings/providers", http.StatusCreated)
}

func (h *SecretsHandler) UninstallConnect(w http.ResponseWriter, r *http.Request) {
	slog.Info("connect: uninstalling")
	if err := h.connect.Uninstall(r.Context()); err != nil {
		slog.Error("connect: uninstall failed", "error", err)
		htmxError(w, r, "Failed to uninstall Connect Server: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("connect: uninstalled")

	htmxRedirectOrStatus(w, r, "/settings/providers", http.StatusNoContent)
}

func (h *SecretsHandler) SecretsListPage(w http.ResponseWriter, r *http.Request) {
	storeStatus, err := h.secrets.GetSecretStoreStatus(r.Context(), defaultNamespace())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var secrets []service.ExternalSecretInfo
	if storeStatus.Configured {
		secrets, err = h.secrets.ListExternalSecrets(r.Context(), defaultNamespace())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	anyPending := false
	for _, s := range secrets {
		if s.Status != "Synced" && s.Status != "Error" {
			anyPending = true
			break
		}
	}

	data := struct {
		StoreConfigured bool
		Secrets         []service.ExternalSecretInfo
		StoreName       string
		ProviderName    string
		AnyPending      bool
	}{storeStatus.Configured, secrets, storeStatus.Name, storeStatus.Provider, anyPending}

	if !renderOrError(w, r, h.tmpl, "secrets", data, isHTMX(r)) {
		return
	}
}

func (h *SecretsHandler) SecretsNewPage(w http.ResponseWriter, r *http.Request) {
	storeStatus, err := h.secrets.GetSecretStoreStatus(r.Context(), defaultNamespace())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !renderOrError(w, r, h.tmpl, "secrets-new", struct{ StoreName string }{storeStatus.Name}, isHTMX(r)) {
		return
	}
}

// API handlers

func (h *SecretsHandler) CreateSecretStore(w http.ResponseWriter, r *http.Request) {
	var opts service.CreateSecretStoreOptions
	if err := parseRequest(r, &opts); err != nil {
		htmxError(w, r, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if opts.ConnectHost == "" || opts.ConnectToken == "" || opts.VaultName == "" {
		htmxError(w, r, "All fields are required", http.StatusBadRequest)
		return
	}

	slog.Info("secret-store: creating", "host", opts.ConnectHost, "vault", opts.VaultName)
	if err := h.secrets.CreateSecretStore(r.Context(), defaultNamespace(), opts); err != nil {
		slog.Error("secret-store: create failed", "error", err)
		htmxError(w, r, "Failed to configure provider: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("secret-store: created")

	htmxRedirectOrStatus(w, r, "/settings/providers", http.StatusCreated)
}

func (h *SecretsHandler) DeleteSecretStore(w http.ResponseWriter, r *http.Request) {
	slog.Info("secret-store: deleting")
	if err := h.secrets.DeleteSecretStore(r.Context(), defaultNamespace()); err != nil {
		slog.Error("secret-store: delete failed", "error", err)
		htmxError(w, r, "Failed to remove provider: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("secret-store: deleted")

	htmxRedirectOrStatus(w, r, "/settings/providers", http.StatusNoContent)
}

func (h *SecretsHandler) CreateExternalSecret(w http.ResponseWriter, r *http.Request) {
	// Simplified form: name + 1Password item + field → derive everything else
	var body struct {
		Name            string `json:"name"`
		Item            string `json:"item"`
		Field           string `json:"field"`
		RefreshInterval string `json:"refreshInterval"`
	}
	if err := parseRequest(r, &body); err != nil {
		htmxError(w, r, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if body.Name == "" || body.Item == "" {
		htmxError(w, r, "Name and 1Password item are required", http.StatusBadRequest)
		return
	}
	if !validK8sName.MatchString(body.Name) {
		htmxError(w, r, "Invalid name: must be a valid Kubernetes resource name (lowercase, hyphens ok)", http.StatusBadRequest)
		return
	}

	// Default field to "credential"
	field := body.Field
	if field == "" {
		field = "credential"
	}

	// Derive the full ExternalSecret options from the simplified input
	refreshInterval := "1h"
	if body.RefreshInterval != "" {
		refreshInterval = body.RefreshInterval
	}

	opts := service.CreateExternalSecretOptions{
		Name:            body.Name,
		SecretStore:     "onepassword-store",
		TargetSecret:    body.Name,
		RefreshInterval: refreshInterval,
		Data: []service.ExternalSecretData{
			{
				SecretKey:      "value",
				RemoteKey:      body.Item,
				RemoteProperty: field,
			},
		},
	}

	slog.Info("external-secret: creating", "name", body.Name, "item", body.Item, "field", field)
	if err := h.secrets.CreateExternalSecret(r.Context(), defaultNamespace(), opts); err != nil {
		slog.Error("external-secret: create failed", "name", body.Name, "error", err)
		htmxError(w, r, "Failed to create secret: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("external-secret: created", "name", body.Name)

	htmxRedirectOrStatus(w, r, "/secrets", http.StatusCreated)
}

func (h *SecretsHandler) DeleteExternalSecret(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validK8sName.MatchString(name) {
		htmxError(w, r, "Invalid resource name", http.StatusBadRequest)
		return
	}
	slog.Info("external-secret: deleting", "name", name)
	if err := h.secrets.DeleteExternalSecret(r.Context(), defaultNamespace(), name); err != nil {
		slog.Error("external-secret: delete failed", "name", name, "error", err)
		htmxError(w, r, "Failed to delete secret: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("external-secret: deleted", "name", name)

	// For HTMX row actions, return 200 with an empty body so the client can
	// swap/remove the row directly.
	if isHTMX(r) {
		w.WriteHeader(http.StatusOK)
		return
	}

	htmxRedirectOrStatus(w, r, "/secrets", http.StatusNoContent)
}

// Polling partials — return just status badge HTML for HTMX polling

func (h *SecretsHandler) SettingsStatusPartial(w http.ResponseWriter, r *http.Request) {
	status, err := h.secrets.GetSecretStoreStatus(r.Context(), defaultNamespace())
	if err != nil {
		if _, writeErr := fmt.Fprintf(w, `<span class="badge badge-error">Error</span>`); writeErr != nil {
			slog.Warn("settings status: failed to write error badge", "error", writeErr)
		}
		return
	}
	if !status.Configured {
		if _, writeErr := fmt.Fprintf(w, `<span class="badge badge-ghost">Not Configured</span>`); writeErr != nil {
			slog.Warn("settings status: failed to write not configured badge", "error", writeErr)
		}
		return
	}
	if status.Ready {
		if _, writeErr := fmt.Fprintf(w, `<span id="provider-status" class="badge badge-success">Ready</span>`); writeErr != nil {
			slog.Warn("settings status: failed to write ready badge", "error", writeErr)
		}
	} else {
		if _, writeErr := fmt.Fprintf(w, `<span id="provider-status" class="badge badge-warning" hx-get="/settings/status" hx-trigger="every 3s" hx-target="#provider-status" hx-swap="outerHTML">Not Ready</span>`); writeErr != nil {
			slog.Warn("settings status: failed to write not-ready badge", "error", writeErr)
		}
	}
}

func (h *SecretsHandler) SecretsStatusPartial(w http.ResponseWriter, r *http.Request) {
	secrets, err := h.secrets.ListExternalSecrets(r.Context(), defaultNamespace())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	allSettled := true
	for _, s := range secrets {
		if s.Status != "Synced" && s.Status != "Error" {
			allSettled = false
			break
		}
	}

	// Build OOB swap elements for each secret's status cell.
	var oobHTML string
	for _, s := range secrets {
		var badgeHTML string
		safeStatus := html.EscapeString(s.Status)
		switch s.Status {
		case "Synced":
			badgeHTML = `<span class="badge badge-success badge-sm">Synced</span>`
		case "Error":
			badgeHTML = `<span class="badge badge-error badge-sm">Error</span>`
		default:
			badgeHTML = fmt.Sprintf(`<span class="badge badge-warning badge-sm">%s</span>`, safeStatus)
		}
		oobHTML += fmt.Sprintf(`<div id="secret-status-%s" hx-swap-oob="true">%s</div>`, html.EscapeString(s.Name), badgeHTML)
	}

	if allSettled {
		// HTTP 286 signals HTMX to stop polling.
		w.WriteHeader(286)
		if _, writeErr := fmt.Fprintf(w, `<div id="secrets-status"></div>%s`, oobHTML); writeErr != nil {
			slog.Warn("secrets status: failed to write settled response", "error", writeErr)
		}
	} else {
		if _, writeErr := fmt.Fprintf(w, `<div id="secrets-status" hx-get="/secrets/status" hx-trigger="every 3s" hx-swap="outerHTML"></div>%s`, oobHTML); writeErr != nil {
			slog.Warn("secrets status: failed to write polling response", "error", writeErr)
		}
	}
}

// AvailableSecrets returns synced secrets with their data keys as JSON (for bot form dropdowns)
func (h *SecretsHandler) AvailableSecrets(w http.ResponseWriter, r *http.Request) {
	secrets, err := h.secrets.ListExternalSecrets(r.Context(), defaultNamespace())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		if encodeErr := json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}); encodeErr != nil {
			slog.Warn("available secrets: failed to encode error payload", "error", encodeErr)
		}
		return
	}

	type availableSecret struct {
		Name         string   `json:"name"`
		TargetSecret string   `json:"targetSecret"`
		DataKeys     []string `json:"dataKeys"`
		Status       string   `json:"status"`
	}

	var result []availableSecret
	for _, s := range secrets {
		if s.Status == "Synced" {
			result = append(result, availableSecret{
				Name:         s.Name,
				TargetSecret: s.TargetSecret,
				DataKeys:     s.DataKeys,
				Status:       s.Status,
			})
		}
	}

	if result == nil {
		result = []availableSecret{}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Warn("available secrets: failed to encode result", "error", err)
	}
}
