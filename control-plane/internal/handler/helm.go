package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/zackerydev/clawmachine/control-plane/internal/botenv"
	"github.com/zackerydev/clawmachine/control-plane/internal/middleware"
	"github.com/zackerydev/clawmachine/control-plane/internal/onboarding"
	"github.com/zackerydev/clawmachine/control-plane/internal/service"
	versionutil "github.com/zackerydev/clawmachine/control-plane/internal/version"
)

type HelmHandler struct {
	helm       HelmManager
	tmpl       TemplateRenderer
	botSecrets *service.BotSecretsService
	secrets    SecretsManager
	k8s        KubernetesManager
	bots       *botenv.Registry
	onboarding *onboarding.Engine
	devMode    bool
	runtimeVer string
}

func NewHelmHandler(helm HelmManager, tmpl TemplateRenderer, botSecrets *service.BotSecretsService, secrets SecretsManager, k8s KubernetesManager, bots *botenv.Registry, devMode bool) *HelmHandler {
	return NewHelmHandlerWithVersion(helm, tmpl, botSecrets, secrets, k8s, bots, devMode, "")
}

func NewHelmHandlerWithVersion(helm HelmManager, tmpl TemplateRenderer, botSecrets *service.BotSecretsService, secrets SecretsManager, k8s KubernetesManager, bots *botenv.Registry, devMode bool, runtimeVersion string) *HelmHandler {
	return &HelmHandler{
		helm:       helm,
		tmpl:       tmpl,
		botSecrets: botSecrets,
		secrets:    secrets,
		k8s:        k8s,
		bots:       bots,
		onboarding: onboarding.NewEngine(bots),
		devMode:    devMode,
		runtimeVer: strings.TrimSpace(runtimeVersion),
	}
}

// defaultNamespace returns the namespace for bot operations.
// Reads from POD_NAMESPACE (set via downward API), falls back to "claw-machine".
func defaultNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}
	if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		return string(data)
	}
	return "claw-machine"
}

func (h *HelmHandler) List(w http.ResponseWriter, r *http.Request) {
	releases, err := h.helm.List(defaultNamespace())
	if err != nil {
		slog.Error("list releases failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Debug("listed releases", "count", len(releases))

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(releases); err != nil {
		slog.Warn("list: failed to encode releases", "error", err)
	}
}

func (h *HelmHandler) Install(w http.ResponseWriter, r *http.Request) {
	var opts service.InstallOptions
	var err error

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/x-www-form-urlencoded") || strings.HasPrefix(ct, "multipart/form-data") {
		opts, err = parseInstallForm(r, h.bots)
	} else {
		err = json.NewDecoder(r.Body).Decode(&opts)
	}
	if err != nil {
		slog.Warn("install: invalid request body", "error", err)
		htmxError(w, r, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	slog.Info("install: starting",
		"release", opts.ReleaseName,
		"botType", opts.BotType,
		"configFields", len(opts.ConfigFields),
		"valueKeys", len(opts.Values),
	)

	if !middleware.ValidName(opts.ReleaseName) {
		slog.Warn("install: invalid release name", "release", opts.ReleaseName)
		htmxError(w, r, "Invalid release name", http.StatusBadRequest)
		return
	}

	if opts.BotType == "" {
		htmxError(w, r, "Bot type is required", http.StatusBadRequest)
		return
	}
	opts.Namespace = defaultNamespace()

	if opts.Values == nil {
		opts.Values = make(map[string]any)
	}
	if opts.OnboardingVersion == "" && len(opts.ConfigFields) > 0 {
		opts.OnboardingVersion = onboarding.ProfileVersion
	}

	// Ensure sub-maps exist so chart templates don't nil-pointer
	for _, key := range []string{"workspace", "backup", "persistence", "networkPolicy"} {
		if _, ok := opts.Values[key]; !ok {
			opts.Values[key] = map[string]any{"enabled": false}
		}
	}

	// Auto-detect Cilium: if CiliumNetworkPolicy CRD exists, enable DNS-aware policies
	if np, ok := opts.Values["networkPolicy"].(map[string]any); ok {
		if _, explicit := np["useCilium"]; !explicit {
			if h.k8s != nil && h.k8s.HasCRD("ciliumnetworkpolicies.cilium.io") {
				np["useCilium"] = true
				slog.Info("install: auto-detected Cilium CNI", "release", opts.ReleaseName)
			}
		}
	}

	// Process backup credential secret refs selected from the install form.
	// Each 1Password ExternalSecret name is translated to its synced target
	// Kubernetes Secret name and wired directly into backup env var refs.
	if backupCredRefs, ok := opts.Values["backupCredentialSecretRefs"].(map[string]any); ok {
		accessSecret, _ := backupCredRefs["accessKeyId"].(string)
		secretSecret, _ := backupCredRefs["secretAccessKey"].(string)

		if accessSecret != "" && secretSecret != "" {
			accessTarget := accessSecret
			secretTarget := secretSecret

			if h.secrets != nil {
				externalSecrets, err := h.secrets.ListExternalSecrets(r.Context(), defaultNamespace())
				if err != nil {
					slog.Warn("install: failed to list external secrets for backup credentials", "error", err)
				} else {
					for _, s := range externalSecrets {
						if s.Name == accessSecret && s.TargetSecret != "" {
							accessTarget = s.TargetSecret
						}
						if s.Name == secretSecret && s.TargetSecret != "" {
							secretTarget = s.TargetSecret
						}
					}
				}
			}

			if backup, ok := opts.Values["backup"].(map[string]any); ok {
				backup["credentials"] = map[string]any{
					"accessKeyIdSecretRef": map[string]any{
						"name": accessTarget,
						"key":  "value",
					},
					"secretAccessKeySecretRef": map[string]any{
						"name": secretTarget,
						"key":  "value",
					},
				}
				opts.Values["backup"] = backup
			}
		}
		delete(opts.Values, "backupCredentialSecretRefs")
	}

	// Legacy fallback: raw backup credentials create a K8s Secret.
	if backupCreds, ok := opts.Values["backupCredentials"].(map[string]any); ok {
		accessKey, _ := backupCreds["accessKeyId"].(string)
		secretKey, _ := backupCreds["secretAccessKey"].(string)
		if accessKey != "" && secretKey != "" && h.botSecrets != nil {
			secretName := opts.ReleaseName + "-backup-credentials"
			secretData := map[string]string{
				"AWS_ACCESS_KEY_ID":     accessKey,
				"AWS_SECRET_ACCESS_KEY": secretKey,
			}
			if _, err := h.botSecrets.CreateOrUpdate(r.Context(), opts.ReleaseName+"-backup", defaultNamespace(), secretData); err != nil {
				slog.Error("install: failed to create backup credentials", "release", opts.ReleaseName, "error", err)
				htmxError(w, r, "Failed to create backup credentials: "+err.Error(), http.StatusInternalServerError)
				return
			}
			// Set the credentials secret name in backup config
			if backup, ok := opts.Values["backup"].(map[string]any); ok {
				backup["credentialsSecret"] = secretName
				opts.Values["backup"] = backup
			}
		}
		delete(opts.Values, "backupCredentials")
	}

	// Process envSecrets mappings: each maps an env var to an existing K8s secret + key.
	// These are passed to the chart as envSecrets[] for per-env-var secretKeyRef mounting.
	if envSecrets, ok := opts.Values["envSecrets"].([]any); ok && len(envSecrets) > 0 {
		opts.Values["envSecrets"] = normalizeEnvSecrets(envSecrets)
	}

	// Generate config file from structured fields (step 2 of install wizard)
	if len(opts.ConfigFields) > 0 {
		compiled, err := h.compileConfigFields(r.Context(), opts.BotType, opts.ConfigFields)
		if err != nil {
			var modelErr *openClawModelValidationError
			if errors.As(err, &modelErr) {
				htmxError(w, r, modelErr.Error(), http.StatusBadRequest)
				return
			}
			slog.Error("install: config build failed", "release", opts.ReleaseName, "error", err)
			htmxError(w, r, "Failed to build config: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if compiled != nil {
			if opts.BotType != service.BotTypeOpenClaw && compiled.ConfigFile != nil && compiled.ConfigFile.Enabled {
				slog.Info("install: generated config file",
					"release", opts.ReleaseName,
					"format", compiled.ConfigFile.Format,
					"bytes", len(compiled.ConfigFile.Content))
				if _, ok := opts.Values["configFile"]; !ok {
					opts.Values["configFile"] = make(map[string]any)
				}
				cfgFile := opts.Values["configFile"].(map[string]any)
				cfgFile["enabled"] = true
				cfgFile["content"] = compiled.ConfigFile.Content
			} else if opts.BotType == service.BotTypeOpenClaw && compiled.ConfigFile != nil && compiled.ConfigFile.Enabled {
				// OpenClaw is env/values-driven and reconciles runtime config in the
				// startup script. Ignore generated config seeds to avoid split-brain.
				slog.Debug("install: ignoring generated config file for openclaw",
					"release", opts.ReleaseName,
					"bytes", len(compiled.ConfigFile.Content))
			}

			if compiled.ValuesPatch != nil {
				onboarding.DeepMergeValues(opts.Values, compiled.ValuesPatch)
			}

			if len(compiled.EnvSecrets) > 0 {
				existing, _ := opts.Values["envSecrets"].([]any)
				opts.Values["envSecrets"] = mergeEnvSecrets(existing, compiled.EnvSecrets)
			}
		}

		if opts.BotType == service.BotTypeOpenClaw {
			delete(opts.Values, "configFile")
		}
	}

	ensureBuiltInPostgresPassword(&opts)
	ensureRuntimeImageTag(opts.Values, h.runtimeVer, false)

	// Fire install synchronously for immediate error handling, but do not wait
	// on workload readiness (Helm service uses hook-only wait strategy).
	slog.Info("install: calling helm install", "release", opts.ReleaseName, "chart", opts.BotType, "namespace", defaultNamespace())
	if _, err := h.helm.Install(context.Background(), opts); err != nil {
		slog.Error("install: helm install failed", "release", opts.ReleaseName, "error", err)
		htmxError(w, r, "Install failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("install: success", "release", opts.ReleaseName, "botType", opts.BotType)

	htmxRedirectOrStatus(w, r, "/bots/"+opts.ReleaseName+"/page", http.StatusCreated)
}

func (h *HelmHandler) Status(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	info, err := h.helm.Status(name, defaultNamespace())
	if err != nil {
		slog.Error("status failed", "release", name, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(info); err != nil {
		slog.Warn("status: failed to encode release info", "release", name, "error", err)
	}
}

func (h *HelmHandler) Upgrade(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	slog.Info("upgrade: starting", "release", name)
	if !middleware.ValidName(name) {
		http.Error(w, "invalid release name", http.StatusBadRequest)
		return
	}

	namespace := defaultNamespace()

	var body struct {
		BotType service.BotType `json:"botType"`
		Values  map[string]any  `json:"values"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		slog.Warn("upgrade: invalid body", "release", name, "error", err)
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	currentValues, err := h.helm.GetValues(name, namespace)
	if err != nil {
		slog.Error("upgrade: failed to load current values", "release", name, "error", err)
		http.Error(w, "failed to load current values: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if currentValues == nil {
		currentValues = make(map[string]any)
	}
	if body.Values != nil {
		onboarding.DeepMergeValues(currentValues, body.Values)
	}

	botType := body.BotType
	if botType == "" {
		if env, ok := currentValues["env"].(map[string]any); ok {
			if bt, ok := env["BOT_TYPE"].(string); ok && bt != "" {
				botType = service.BotType(bt)
			}
		}
	}
	if botType == "" {
		status, statusErr := h.helm.Status(name, namespace)
		if statusErr != nil {
			slog.Error("upgrade: failed to determine bot type", "release", name, "error", statusErr)
			http.Error(w, "failed to determine bot type: "+statusErr.Error(), http.StatusInternalServerError)
			return
		}
		if status != nil && status.BotType != "" {
			botType = service.BotType(status.BotType)
		}
	}
	if botType == "" {
		http.Error(w, "botType is required", http.StatusBadRequest)
		return
	}
	ensureRuntimeImageTag(currentValues, h.runtimeVer, !hasExplicitImageTag(body.Values))

	info, err := h.helm.Upgrade(r.Context(), name, namespace, botType, currentValues)
	if err != nil {
		slog.Error("upgrade: failed", "release", name, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if info == nil {
		slog.Error("upgrade: nil response", "release", name)
		http.Error(w, "upgrade returned empty response", http.StatusInternalServerError)
		return
	}
	slog.Info("upgrade: success", "release", name, "status", info.Status)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(info); err != nil {
		slog.Warn("upgrade: failed to encode response", "release", name, "error", err)
	}
}

func (h *HelmHandler) Uninstall(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ns := defaultNamespace()
	slog.Info("uninstall: starting", "release", name)

	// Run uninstall synchronously so immediate errors are surfaced to the user.
	// Helm service is configured with a non-blocking wait strategy, so this
	// returns as soon as uninstall is accepted by Helm/Kubernetes.
	if err := h.helm.Uninstall(name, ns); err != nil {
		slog.Error("uninstall: failed", "release", name, "error", err)
		htmxError(w, r, "Uninstall failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("uninstall: helm release removed", "release", name)
	if h.botSecrets != nil {
		if err := h.botSecrets.Delete(context.Background(), name, ns); err != nil {
			slog.Warn("failed to delete bot secret on uninstall", "release", name, "error", err)
		}
	}

	if isHTMX(r) {
		w.Header().Set("HX-Redirect", "/")
	}
	w.WriteHeader(http.StatusNoContent)
}

// Page handlers

func (h *HelmHandler) ListPage(w http.ResponseWriter, r *http.Request) {
	releases, err := h.helm.List(defaultNamespace())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		Releases []service.ReleaseInfo
	}{
		Releases: releases,
	}

	if !renderOrError(w, r, h.tmpl, "bots", data, isHTMX(r)) {
		return
	}
}

func (h *HelmHandler) allowedBotTypes() []string {
	types := []string{"openclaw", "ironclaw", "picoclaw"}
	if h.devMode {
		types = append(types, "busybox")
	}
	return types
}

func (h *HelmHandler) NewPage(w http.ResponseWriter, r *http.Request) {
	botType := r.URL.Query().Get("type")

	// If a type is specified, render step 1 (infrastructure).
	if botType != "" {
		h.renderInstallInfraPage(w, r, botType, nil)
		return
	}

	// Otherwise, render the bot type picker (step 1)
	botTypes := h.allowedBotTypes()
	configMap := make(map[string]*botenv.BotConfig)
	if h.bots != nil {
		for _, c := range h.bots.All() {
			configMap[c.Name] = c
		}
	}
	// Build configs list in order, creating fallback entries for types without YAML
	var configs []*botenv.BotConfig
	for _, t := range botTypes {
		if c, ok := configMap[t]; ok {
			configs = append(configs, c)
		} else {
			configs = append(configs, &botenv.BotConfig{
				Name:        t,
				DisplayName: t,
			})
		}
	}

	data := struct {
		BotTypes   []string
		BotConfigs []*botenv.BotConfig
		DevMode    bool
	}{
		BotTypes:   botTypes,
		BotConfigs: configs,
		DevMode:    h.devMode,
	}

	if !renderOrError(w, r, h.tmpl, "bots-new", data, isHTMX(r)) {
		return
	}
}

// NewInfraPage renders step 1 with posted values (used by Back buttons).
func (h *HelmHandler) NewInfraPage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		htmxError(w, r, "Invalid form: "+err.Error(), http.StatusBadRequest)
		return
	}

	botType := formValue(r, "botType")
	h.renderInstallInfraPage(w, r, botType, allFormValues(r))
}

// NewConfigPage renders step 2 after validating step 1.
func (h *HelmHandler) NewConfigPage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		htmxError(w, r, "Invalid form: "+err.Error(), http.StatusBadRequest)
		return
	}

	botType := formValue(r, "botType")
	releaseName := strings.TrimSpace(formValue(r, "releaseName"))
	if !middleware.ValidName(releaseName) {
		htmxError(w, r, "Invalid release name", http.StatusBadRequest)
		return
	}

	h.renderInstallConfigPage(w, r, botType, allFormValues(r))
}

// NewSoftwarePage renders step 3 after validating required prior inputs.
func (h *HelmHandler) NewSoftwarePage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		htmxError(w, r, "Invalid form: "+err.Error(), http.StatusBadRequest)
		return
	}

	botType := formValue(r, "botType")
	releaseName := strings.TrimSpace(formValue(r, "releaseName"))
	if !middleware.ValidName(releaseName) {
		htmxError(w, r, "Invalid release name", http.StatusBadRequest)
		return
	}

	h.renderInstallSoftwarePage(w, r, botType, allFormValues(r))
}

// AvailableSecret is a synced ExternalSecret for template rendering.
type AvailableSecret struct {
	Name         string
	TargetSecret string
}

type installQuestionSection struct {
	Key         string
	Label       string
	Description string
	Questions   []onboarding.Question
}

func (h *HelmHandler) renderInstallInfraPage(w http.ResponseWriter, r *http.Request, botType string, values map[string]string) {
	// Validate bot type
	allowed := slices.Contains(h.allowedBotTypes(), botType)
	if !allowed {
		http.NotFound(w, r)
		return
	}

	var botConfig *botenv.BotConfig
	if h.bots != nil {
		botConfig = h.bots.Get(botType)
	}
	if botConfig == nil {
		botConfig = &botenv.BotConfig{
			Name:        botType,
			DisplayName: botType,
		}
	}

	if values == nil {
		values = make(map[string]string)
	}
	values["botType"] = botType
	if values["onboardingVersion"] == "" {
		values["onboardingVersion"] = onboarding.ProfileVersion
	}
	if values["persistence"] == "" {
		values["persistence"] = "on"
	}
	if values["persistenceSize"] == "" {
		values["persistenceSize"] = "5Gi"
	}

	data := struct {
		BotConfig *botenv.BotConfig
		Secrets   []AvailableSecret
		Values    map[string]string
	}{
		BotConfig: botConfig,
		Secrets:   h.availableSecrets(r.Context()),
		Values:    values,
	}

	if !renderOrError(w, r, h.tmpl, "bot-form-infra", data, isHTMX(r)) {
		return
	}
}

func (h *HelmHandler) renderInstallConfigPage(w http.ResponseWriter, r *http.Request, botType string, values map[string]string) {
	allowed := slices.Contains(h.allowedBotTypes(), botType)
	if !allowed {
		http.NotFound(w, r)
		return
	}

	var botConfig *botenv.BotConfig
	if h.bots != nil {
		botConfig = h.bots.Get(botType)
	}
	if botConfig == nil {
		botConfig = &botenv.BotConfig{
			Name:        botType,
			DisplayName: botType,
		}
	}

	if values == nil {
		values = make(map[string]string)
	}
	values["botType"] = botType
	if values["onboardingVersion"] == "" {
		values["onboardingVersion"] = onboarding.ProfileVersion
	}

	profile, err := h.onboarding.Profile(service.BotType(botType))
	if err != nil {
		htmxError(w, r, "Failed to load onboarding profile: "+err.Error(), http.StatusBadRequest)
		return
	}

	questionByID := make(map[string]onboarding.Question, len(profile.Questions))
	for _, q := range profile.Questions {
		questionByID[q.ID] = q
	}

	sections := make([]installQuestionSection, 0, len(profile.Sections))
	for _, sec := range profile.Sections {
		section := installQuestionSection{
			Key:         sec.Key,
			Label:       sec.Label,
			Description: sec.Description,
			Questions:   make([]onboarding.Question, 0, len(sec.QuestionIDs)),
		}
		for _, qID := range sec.QuestionIDs {
			if q, ok := questionByID[qID]; ok {
				section.Questions = append(section.Questions, q)
			}
		}
		sections = append(sections, section)
	}

	data := struct {
		BotConfig *botenv.BotConfig
		Secrets   []AvailableSecret
		Values    map[string]string
		Sections  []installQuestionSection
	}{
		BotConfig: botConfig,
		Secrets:   h.availableSecrets(r.Context()),
		Values:    values,
		Sections:  sections,
	}

	if !renderOrError(w, r, h.tmpl, "bot-form-config", data, isHTMX(r)) {
		return
	}
}

func (h *HelmHandler) renderInstallSoftwarePage(w http.ResponseWriter, r *http.Request, botType string, values map[string]string) {
	allowed := slices.Contains(h.allowedBotTypes(), botType)
	if !allowed {
		http.NotFound(w, r)
		return
	}

	var botConfig *botenv.BotConfig
	if h.bots != nil {
		botConfig = h.bots.Get(botType)
	}
	if botConfig == nil {
		botConfig = &botenv.BotConfig{
			Name:        botType,
			DisplayName: botType,
		}
	}

	if values == nil {
		values = make(map[string]string)
	}
	values["botType"] = botType
	if values["onboardingVersion"] == "" {
		values["onboardingVersion"] = onboarding.ProfileVersion
	}

	data := struct {
		BotConfig *botenv.BotConfig
		Values    map[string]string
	}{
		BotConfig: botConfig,
		Values:    values,
	}

	if !renderOrError(w, r, h.tmpl, "bot-form-software", data, isHTMX(r)) {
		return
	}
}

func (h *HelmHandler) availableSecrets(ctx context.Context) []AvailableSecret {
	return availableSecretsForTemplates(ctx, h.secrets)
}

// BotDetailData is the template data for the bot detail page.
type BotDetailData struct {
	*service.ReleaseInfo
	Values           map[string]any
	RawValues        map[string]any
	Persistence      bool
	PodStatus        string
	AllowedDomains   []string
	ConfigContent    string
	ConfigTab        bool
	ConfigEditable   bool
	ConfigWarning    string
	BotConfig        *botenv.BotConfig
	AvailableSecrets []AvailableSecret
}

type botCLIResultData struct {
	Command string
	Stdout  string
	Stderr  string
	Error   string
	Success bool
}

func (h *HelmHandler) DetailPage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	info, err := h.helm.Status(name, defaultNamespace())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	values, _ := h.helm.GetValuesAll(name, defaultNamespace())
	if values == nil {
		values = make(map[string]any)
	}

	// Check if persistence is enabled
	persistence := false
	if p, ok := values["persistence"].(map[string]any); ok {
		if enabled, ok := p["enabled"].(bool); ok {
			persistence = enabled
		}
	}

	podStatus := "unknown"
	if h.k8s != nil {
		healthy, healthErr := h.k8s.GetReleasePodHealthy(r.Context(), defaultNamespace(), name)
		if healthErr != nil {
			slog.Warn("detail: failed to determine pod health", "release", name, "error", healthErr)
		} else if healthy {
			podStatus = "healthy"
		} else {
			podStatus = "unhealthy"
		}
	}

	// Extract allowed domains from networkPolicy
	var allowedDomains []string
	if np, ok := values["networkPolicy"].(map[string]any); ok {
		if domains, ok := np["allowedDomains"].([]any); ok {
			for _, d := range domains {
				if s, ok := d.(string); ok {
					allowedDomains = append(allowedDomains, s)
				}
			}
		}
	}

	// Extract config file content if present
	var configContent string
	if cf, ok := values["configFile"].(map[string]any); ok {
		if content, ok := cf["content"].(string); ok {
			configContent = content
		}
	}

	// Determine bot type from values first, then status fallback.
	detectedBotType := ""
	if botType, ok := values["BOT_TYPE"].(string); ok && botType != "" {
		detectedBotType = botType
	}
	if detectedBotType == "" {
		if env, ok := values["env"].(map[string]any); ok {
			if botType, ok := env["BOT_TYPE"].(string); ok && botType != "" {
				detectedBotType = botType
			}
		}
	}
	if detectedBotType == "" && info != nil && info.BotType != "" {
		detectedBotType = info.BotType
	}

	// Look up bot config from registry
	var botCfg *botenv.BotConfig
	if detectedBotType != "" {
		botCfg = h.bots.Get(detectedBotType)
	}

	isOpenClaw := strings.EqualFold(detectedBotType, string(service.BotTypeOpenClaw))
	if isOpenClaw {
		liveConfig := h.loadOpenClawConfigFromPod(r.Context(), name)
		if strings.TrimSpace(liveConfig) != "" {
			configContent = liveConfig
		} else {
			configContent = "{}"
		}
	}
	_, secretNamesByValue := h.loadSecretValueIndex(r.Context())

	// For JSON config files, render only a redacted/tokenized version in the UI.
	configTab := false
	configEditable := false
	configWarning := ""
	if isOpenClaw || (botCfg != nil && botCfg.HasConfigFile() && botCfg.ConfigFormat == "json") {
		if strings.TrimSpace(configContent) == "" {
			configContent = "{}"
		}
		configTab = true
		configEditable = true
		redacted, redactErr := redactConfigContentForUI(botCfg, configContent, secretNamesByValue)
		if redactErr != nil {
			slog.Warn("detail: failed to redact config content", "release", name, "error", redactErr)
			configWarning = "Configuration JSON could not be safely rendered. Secret fields are hidden and JSON editing is disabled until the config is valid."
			configContent = "{}"
			configEditable = false
		} else {
			configContent = redacted
		}
	}

	data := BotDetailData{
		ReleaseInfo:      info,
		Values:           flattenValues("", values),
		RawValues:        values,
		Persistence:      persistence,
		PodStatus:        podStatus,
		AllowedDomains:   allowedDomains,
		ConfigContent:    configContent,
		ConfigTab:        configTab,
		ConfigEditable:   configEditable,
		ConfigWarning:    configWarning,
		BotConfig:        botCfg,
		AvailableSecrets: h.availableSecrets(r.Context()),
	}

	if !renderOrError(w, r, h.tmpl, "bot-detail", data, isHTMX(r)) {
		return
	}
}

func (h *HelmHandler) loadOpenClawConfigFromPod(ctx context.Context, releaseName string) string {
	if h.k8s == nil {
		return ""
	}

	const readConfigScript = `if [ -n "${OPENCLAW_HOME:-}" ] && [ -f "${OPENCLAW_HOME}/openclaw.json" ]; then cat "${OPENCLAW_HOME}/openclaw.json";
elif [ -f "/root/.openclaw/openclaw.json" ]; then cat "/root/.openclaw/openclaw.json";
else echo "{}"; fi`

	stdout, stderr, err := h.k8s.ExecInReleasePod(ctx, defaultNamespace(), releaseName, "openclaw", []string{"sh", "-lc", readConfigScript})
	if err != nil {
		slog.Warn("detail: failed reading openclaw config from pod", "release", releaseName, "error", err, "stderr", stderr)
		return ""
	}
	return strings.TrimSpace(stdout)
}

func (h *HelmHandler) writeOpenClawConfigToPod(ctx context.Context, releaseName, content string) error {
	if h.k8s == nil {
		return fmt.Errorf("kubernetes service unavailable")
	}

	b64 := base64.StdEncoding.EncodeToString([]byte(content))
	writeScript := fmt.Sprintf(`set -e
CONFIG_DIR="${OPENCLAW_HOME:-${HOME:-/root}/.openclaw}"
CONFIG_FILE="$CONFIG_DIR/openclaw.json"
mkdir -p "$CONFIG_DIR"
printf %%s %q | base64 -d > "$CONFIG_FILE"
`, b64)

	_, stderr, err := h.k8s.ExecInReleasePod(ctx, defaultNamespace(), releaseName, "openclaw", []string{"sh", "-lc", writeScript})
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr))
		}
		return err
	}
	return nil
}

// Logs returns recent pod logs for the bot as an HTML fragment.
func (h *HelmHandler) Logs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	logs, err := h.k8s.GetPodLogs(r.Context(), defaultNamespace(), name, 100)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		if _, writeErr := fmt.Fprintf(w, `<pre class="text-error">Error fetching logs: %s</pre>`, err.Error()); writeErr != nil {
			slog.Warn("logs: failed to write error response", "release", name, "error", writeErr)
		}
		return
	}

	if logs == "" {
		logs = "No logs available."
	}

	w.Header().Set("Content-Type", "text/html")
	if _, err := fmt.Fprintf(w, `<pre class="whitespace-pre-wrap break-all">%s</pre>`, template.HTMLEscapeString(logs)); err != nil {
		slog.Warn("logs: failed to write response", "release", name, "error", err)
	}
}

// ExecCLI executes an OpenClaw CLI command in the bot pod and returns output.
func (h *HelmHandler) ExecCLI(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !middleware.ValidName(name) {
		htmxError(w, r, "Invalid bot name", http.StatusBadRequest)
		return
	}
	if h.k8s == nil {
		htmxError(w, r, "Kubernetes service unavailable", http.StatusInternalServerError)
		return
	}

	info, err := h.helm.Status(name, defaultNamespace())
	if err != nil {
		htmxError(w, r, "Failed to load bot status: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if info == nil || !strings.EqualFold(info.BotType, string(service.BotTypeOpenClaw)) {
		htmxError(w, r, "CLI execution is currently supported only for openclaw bots", http.StatusBadRequest)
		return
	}

	var body struct {
		Args string `json:"args"`
	}
	if err := parseRequest(r, &body); err != nil {
		htmxError(w, r, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	rawArgs := strings.TrimSpace(body.Args)
	if rawArgs == "" {
		htmxError(w, r, "Command arguments are required", http.StatusBadRequest)
		return
	}

	args := strings.Fields(rawArgs)
	if len(args) > 0 && strings.EqualFold(args[0], "openclaw") {
		args = args[1:]
	}
	command := append([]string{"openclaw"}, args...)

	subcommand := ""
	if len(command) > 1 {
		subcommand = command[1]
	}
	slog.Info("cli-exec: running", "release", name, "subcommand", subcommand, "argCount", len(command)-1)

	stdout, stderr, execErr := h.k8s.ExecInReleasePod(r.Context(), defaultNamespace(), name, "openclaw", command)
	result := botCLIResultData{
		Command: strings.Join(command, " "),
		Stdout:  stdout,
		Stderr:  stderr,
		Success: execErr == nil,
	}
	if execErr != nil {
		result.Error = execErr.Error()
		slog.Warn("cli-exec: failed", "release", name, "error", execErr)
	} else {
		slog.Info("cli-exec: success", "release", name)
	}

	if isHTMX(r) {
		w.Header().Set("Content-Type", "text/html")
		if !renderOrError(w, r, h.tmpl, "bot-cli-result", result, true) {
			return
		}
		return
	}

	status := http.StatusOK
	if execErr != nil {
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      result.Success,
		"command": result.Command,
		"stdout":  result.Stdout,
		"stderr":  result.Stderr,
		"error":   result.Error,
	})
}

// Restart deletes the bot's pods so the deployment recreates them.
func (h *HelmHandler) Restart(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	slog.Info("restart: deleting pods", "release", name)

	if err := h.k8s.RestartBot(r.Context(), defaultNamespace(), name); err != nil {
		slog.Error("restart: failed", "release", name, "error", err)
		htmxError(w, r, "Restart failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("restart: success", "release", name)

	htmxRedirectOrStatus(w, r, "/bots/"+name+"/page", http.StatusNoContent)
}

// UpdateConfig handles PUT /bots/{name}/config — updates the bot's config file and restarts.
func (h *HelmHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	slog.Info("config-update: starting", "release", name)

	var body struct {
		ConfigFields  map[string]string `json:"configFields"`
		ConfigContent *string           `json:"configContent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		slog.Warn("config-update: invalid body", "release", name, "error", err)
		htmxError(w, r, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.ConfigContent == nil && len(body.ConfigFields) == 0 {
		htmxError(w, r, "configFields or configContent is required", http.StatusBadRequest)
		return
	}
	slog.Debug("config-update: request parsed", "release", name, "fields", len(body.ConfigFields), "hasConfigContent", body.ConfigContent != nil)

	// Get current values
	values, err := h.helm.GetValues(name, defaultNamespace())
	if err != nil {
		htmxError(w, r, "Failed to get current values: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Determine bot type from values
	var botType string
	if env, ok := values["env"].(map[string]any); ok {
		if bt, ok := env["BOT_TYPE"].(string); ok {
			botType = bt
		}
	}
	// Fallback: check chart name from release info
	if botType == "" {
		info, err := h.helm.Status(name, defaultNamespace())
		if err == nil && info != nil {
			botType = info.BotType
		}
	}

	var botCfg *botenv.BotConfig
	if h.bots != nil {
		botCfg = h.bots.Get(botType)
	}
	isOpenClaw := service.BotType(botType) == service.BotTypeOpenClaw
	supportsConfigFields := isOpenClaw || (botCfg != nil && botCfg.HasConfigFile())

	// Direct JSON config editing path.
	if body.ConfigContent != nil {
		if !isOpenClaw && (botCfg == nil || !botCfg.HasConfigFile()) {
			htmxError(w, r, "Bot type does not support config files", http.StatusBadRequest)
			return
		}
		if !isOpenClaw && botCfg.ConfigFormat != "json" {
			htmxError(w, r, "Direct config editing is only supported for JSON config bots", http.StatusBadRequest)
			return
		}

		currentContent := ""
		if isOpenClaw {
			currentContent = h.loadOpenClawConfigFromPod(r.Context(), name)
			if strings.TrimSpace(currentContent) == "" {
				currentContent = "{}"
			}
		} else {
			if cf, ok := values["configFile"].(map[string]any); ok {
				if content, ok := cf["content"].(string); ok {
					currentContent = content
				}
			}
		}

		secretValuesByName, _ := h.loadSecretValueIndex(r.Context())
		content, mergeErr := mergeEditedConfigContent(botCfg, currentContent, *body.ConfigContent, secretValuesByName)
		if mergeErr != nil {
			status := http.StatusBadRequest
			if strings.HasPrefix(mergeErr.Error(), "current config is not valid JSON") {
				status = http.StatusInternalServerError
			}
			htmxError(w, r, mergeErr.Error(), status)
			return
		}

		if isOpenClaw {
			if err := h.writeOpenClawConfigToPod(r.Context(), name, content); err != nil {
				slog.Error("config-update: openclaw pod write failed", "release", name, "error", err)
				htmxError(w, r, "Failed to update OpenClaw config: "+err.Error(), http.StatusInternalServerError)
				return
			}
			slog.Info("config-update: openclaw pod config updated", "release", name, "mode", "pod-write")
			htmxRedirectOrStatus(w, r, "/bots/"+name+"/page", http.StatusOK)
			return
		}

		if _, ok := values["configFile"]; !ok {
			values["configFile"] = make(map[string]any)
		}
		cfgFile := values["configFile"].(map[string]any)
		cfgFile["enabled"] = true
		cfgFile["content"] = content

		slog.Info("config-update: upgrading helm release", "release", name, "botType", botType, "mode", "json-content")
		if err := h.upgradeWithRetry(r.Context(), name, botType, values); err != nil {
			slog.Error("config-update: upgrade failed", "release", name, "error", err)
			htmxError(w, r, "Upgrade failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		slog.Info("config-update: success", "release", name)

		htmxRedirectOrStatus(w, r, "/bots/"+name+"/page", http.StatusOK)
		return
	}

	if !supportsConfigFields {
		htmxError(w, r, "Bot type does not support structured config updates", http.StatusBadRequest)
		return
	}

	compiled, cfgErr := h.compileConfigFields(r.Context(), service.BotType(botType), body.ConfigFields)
	if cfgErr != nil {
		var modelErr *openClawModelValidationError
		if errors.As(cfgErr, &modelErr) {
			htmxError(w, r, modelErr.Error(), http.StatusBadRequest)
			return
		}
		htmxError(w, r, "Failed to build config: "+cfgErr.Error(), http.StatusInternalServerError)
		return
	}
	if compiled != nil {
		if compiled.ValuesPatch != nil {
			onboarding.DeepMergeValues(values, compiled.ValuesPatch)
		}
		if !isOpenClaw && compiled.ConfigFile != nil {
			if _, ok := values["configFile"]; !ok {
				values["configFile"] = make(map[string]any)
			}
			cfgFile := values["configFile"].(map[string]any)
			cfgFile["enabled"] = true
			cfgFile["content"] = compiled.ConfigFile.Content
		}
		if len(compiled.EnvSecrets) > 0 {
			existing, _ := values["envSecrets"].([]any)
			values["envSecrets"] = mergeEnvSecrets(existing, compiled.EnvSecrets)
		}
	}
	if isOpenClaw {
		delete(values, "configFile")
	}

	// Helm upgrade with new values (retry on lock contention)
	slog.Info("config-update: upgrading helm release", "release", name, "botType", botType)
	if err := h.upgradeWithRetry(r.Context(), name, botType, values); err != nil {
		slog.Error("config-update: upgrade failed", "release", name, "error", err)
		htmxError(w, r, "Upgrade failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("config-update: success", "release", name)

	htmxRedirectOrStatus(w, r, "/bots/"+name+"/page", http.StatusOK)
}

func (h *HelmHandler) upgradeWithRetry(ctx context.Context, name, botType string, values map[string]any) error {
	ensureRuntimeImageTag(values, h.runtimeVer, true)

	var upgradeErr error
	for attempt := range 3 {
		_, upgradeErr = h.helm.Upgrade(ctx, name, defaultNamespace(), service.BotType(botType), values)
		if upgradeErr == nil {
			return nil
		}
		if strings.Contains(upgradeErr.Error(), "another operation") && attempt < 2 {
			slog.Warn("config-update: helm lock contention, retrying", "release", name, "attempt", attempt+1)
			time.Sleep(5 * time.Second)
			continue
		}
		break
	}
	return upgradeErr
}

func normalizeReleaseImageTag(v string) string {
	tag, err := versionutil.NormalizeRuntimeImageTag(v)
	if err != nil {
		return ""
	}
	return tag
}

func hasExplicitImageTag(values map[string]any) bool {
	if values == nil {
		return false
	}
	image, ok := values["image"].(map[string]any)
	if !ok || image == nil {
		return false
	}
	tag, ok := image["tag"]
	if !ok || tag == nil {
		return false
	}
	return strings.TrimSpace(fmt.Sprintf("%v", tag)) != ""
}

func ensureRuntimeImageTag(values map[string]any, runtimeVersion string, force bool) {
	if values == nil {
		return
	}

	tag, err := versionutil.NormalizeRuntimeImageTag(runtimeVersion)
	if err != nil {
		slog.Warn("runtime image tag sync skipped due to invalid runtime version", "runtimeVersion", runtimeVersion, "error", err)
		return
	}
	if tag == "" {
		return
	}

	stampImageTag(values, tag, force)
	if backup, ok := values["backup"].(map[string]any); ok {
		stampImageTag(backup, tag, force)
	}
}

func stampImageTag(values map[string]any, tag string, force bool) {
	image, _ := values["image"].(map[string]any)
	if image == nil {
		image = make(map[string]any)
		values["image"] = image
	}

	currentTag := ""
	if rawTag, ok := image["tag"]; ok && rawTag != nil {
		currentTag = strings.TrimSpace(fmt.Sprintf("%v", rawTag))
	}
	if !force && currentTag != "" {
		return
	}
	image["tag"] = tag
}

// resolveSecretsForConfig resolves 1Password ExternalSecret references to their
// actual plaintext values from the backing K8s secrets. This allows bots that
// embed configuration directly in a JSON/TOML file (e.g. picoclaw) to receive
// real key values rather than env-var references.
func (h *HelmHandler) resolveSecretsForConfig(ctx context.Context, secretRefs map[string]string) map[string]string {
	allSecrets, err := h.secrets.ListExternalSecrets(ctx, defaultNamespace())
	if err != nil {
		slog.Warn("install: failed to list external secrets for config resolution", "error", err)
		return nil
	}

	esMap := make(map[string]service.ExternalSecretInfo, len(allSecrets))
	for _, s := range allSecrets {
		esMap[s.Name] = s
	}

	resolved := make(map[string]string)
	for fieldKey, esName := range secretRefs {
		esInfo, ok := esMap[esName]
		if !ok {
			slog.Warn("install: ExternalSecret not found for config resolution", "name", esName, "field", fieldKey)
			continue
		}
		if esInfo.TargetSecret == "" {
			slog.Warn("install: ExternalSecret has no target secret", "name", esName)
			continue
		}

		secretData, err := h.k8s.ReadSecretData(ctx, defaultNamespace(), esInfo.TargetSecret)
		if err != nil {
			slog.Warn("install: failed to read K8s secret for config", "secret", esInfo.TargetSecret, "error", err)
			continue
		}

		// Use the first declared data key, defaulting to "value"
		dataKey := "value"
		if len(esInfo.DataKeys) > 0 {
			dataKey = esInfo.DataKeys[0]
		}

		val, ok := secretData[dataKey]
		if !ok || len(val) == 0 {
			slog.Warn("install: K8s secret missing expected key", "secret", esInfo.TargetSecret, "key", dataKey)
			continue
		}

		resolved[fieldKey] = string(val)
		slog.Info("install: resolved 1Password secret to config value", "field", fieldKey, "secret", esName)
	}
	return resolved
}

// loadSecretValueIndex loads synced ExternalSecret values and returns
// name->value and value->name lookup tables used by the JSON config editor.
func (h *HelmHandler) loadSecretValueIndex(ctx context.Context) (map[string]string, map[string]string) {
	if h.secrets == nil || h.k8s == nil {
		return nil, nil
	}

	allSecrets, err := h.secrets.ListExternalSecrets(ctx, defaultNamespace())
	if err != nil {
		slog.Warn("config-editor: failed to list external secrets", "error", err)
		return nil, nil
	}

	byName := make(map[string]string)
	byValue := make(map[string]string)
	for _, s := range allSecrets {
		if s.Status != "Synced" || s.TargetSecret == "" {
			continue
		}

		secretData, err := h.k8s.ReadSecretData(ctx, defaultNamespace(), s.TargetSecret)
		if err != nil {
			slog.Warn("config-editor: failed to read target secret", "secret", s.TargetSecret, "error", err)
			continue
		}

		dataKey := "value"
		if len(s.DataKeys) > 0 {
			dataKey = s.DataKeys[0]
		}

		raw, ok := secretData[dataKey]
		if !ok || len(raw) == 0 {
			continue
		}
		value := string(raw)
		byName[s.Name] = value
		if _, exists := byValue[value]; !exists {
			byValue[value] = s.Name
		}
	}

	if len(byName) == 0 {
		return nil, nil
	}
	return byName, byValue
}

func (h *HelmHandler) compileConfigFields(ctx context.Context, botType service.BotType, configFields map[string]string) (*onboarding.CompileResult, error) {
	if len(configFields) == 0 {
		return nil, nil
	}
	if h.onboarding == nil {
		return nil, fmt.Errorf("onboarding engine not initialized")
	}

	cleanFields, secretRefs := onboarding.SplitAnswers(configFields)

	if botType == service.BotTypeOpenClaw {
		authChoice := cleanFields["authChoice"]
		if authChoice == "" {
			authChoice = "apiKey"
		}
		if rawModel := cleanFields["defaultModel"]; rawModel != "" {
			validatedModel, err := validateOpenClawDefaultModel(authChoice, rawModel)
			if err != nil {
				return nil, err
			}
			cleanFields["defaultModel"] = validatedModel
		}
	}

	var botCfg *botenv.BotConfig
	if h.bots != nil {
		botCfg = h.bots.Get(string(botType))
	}

	// For bots that use config files, resolve referenced secrets so the generated
	// config receives plaintext values where required.
	if botCfg != nil && botCfg.HasConfigFile() && len(secretRefs) > 0 && h.secrets != nil && h.k8s != nil {
		resolved := h.resolveSecretsForConfig(ctx, secretRefs)
		for k, v := range resolved {
			cleanFields[k] = v
			delete(secretRefs, k)
		}
	}

	return h.onboarding.Compile(botType, onboarding.MergeAnswers(cleanFields, secretRefs))
}

func normalizeEnvSecrets(existing []any) []any {
	merged := mergeEnvSecrets(existing, nil)
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func mergeEnvSecrets(existing []any, additions []onboarding.EnvSecretBinding) []any {
	byEnvVar := make(map[string]map[string]any)
	order := make([]string, 0, len(existing)+len(additions))

	add := func(entry map[string]any) {
		envVar, _ := entry["envVar"].(string)
		if envVar == "" {
			return
		}
		if _, exists := byEnvVar[envVar]; !exists {
			order = append(order, envVar)
		}
		byEnvVar[envVar] = entry
	}

	for _, raw := range existing {
		if entry, ok := normalizeEnvSecretEntry(raw); ok {
			add(entry)
		}
	}

	for _, binding := range additions {
		entry := map[string]any{
			"envVar": binding.EnvVar,
			"secretRef": map[string]any{
				"name": binding.SecretRef.Name,
				"key":  binding.SecretRef.Key,
			},
		}
		add(entry)
	}

	out := make([]any, 0, len(order))
	for _, envVar := range order {
		out = append(out, byEnvVar[envVar])
	}
	return out
}

func normalizeEnvSecretEntry(raw any) (map[string]any, bool) {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}

	envVar, _ := m["envVar"].(string)
	if envVar == "" {
		return nil, false
	}

	secretName := ""
	secretKey := "value"

	if secretRef, ok := m["secretRef"].(map[string]any); ok {
		secretName, _ = secretRef["name"].(string)
		if key, _ := secretRef["key"].(string); key != "" {
			secretKey = key
		}
	}

	if secretName == "" {
		secretName, _ = m["secretName"].(string)
		if key, _ := m["secretKey"].(string); key != "" {
			secretKey = key
		}
	}

	if secretName == "" {
		return nil, false
	}

	return map[string]any{
		"envVar": envVar,
		"secretRef": map[string]any{
			"name": secretName,
			"key":  secretKey,
		},
	}, true
}

// parseInstallForm reads flat HTML form fields and reconstructs InstallOptions.
func parseInstallForm(r *http.Request, bots *botenv.Registry) (service.InstallOptions, error) {
	if err := r.ParseForm(); err != nil {
		return service.InstallOptions{}, err
	}

	opts := service.InstallOptions{
		ReleaseName:       formValue(r, "releaseName"),
		BotType:           service.BotType(formValue(r, "botType")),
		OnboardingVersion: formValue(r, "onboardingVersion"),
		Values:            make(map[string]any),
	}

	// Persistence
	opts.Values["persistence"] = map[string]any{
		"enabled": formBool(r, "persistence"),
		"size":    formDefault(r, "persistenceSize", "5Gi"),
	}

	// Network policy
	ingress := formBool(r, "ingress")
	egress := formBool(r, "egress")
	np := map[string]any{
		"ingress": ingress,
		"egress":  egress,
	}
	if !egress {
		domains := splitLines(formValue(r, "allowedDomains"))
		if len(domains) > 0 {
			np["useCilium"] = true
			np["allowedDomains"] = domains
		}
	}
	opts.Values["networkPolicy"] = np

	// Workspace import
	if formBool(r, "workspaceEnabled") {
		wp := formDefault(r, "workspaceProvider", "s3")
		opts.Values["workspace"] = map[string]any{
			"enabled":  true,
			"provider": wp,
			"s3": map[string]any{
				"bucket":     formValue(r, "wsS3Bucket"),
				"region":     formValue(r, "wsS3Region"),
				"prefix":     formValue(r, "wsS3Prefix"),
				"secretName": formValue(r, "wsS3SecretName"),
			},
			"github": map[string]any{
				"repository": formValue(r, "wsGithubRepo"),
				"branch":     formValue(r, "wsGithubBranch"),
				"path":       formValue(r, "wsGithubPath"),
				"secretName": formValue(r, "wsGithubSecretName"),
			},
		}
	}

	// Backup
	if formBool(r, "backupEnabled") {
		bp := formDefault(r, "backupProvider", "s3")
		accessKeyRef := normalizeExternalSecretSelection(formValue(r, "backupAccessKeySecret"))
		secretKeyRef := normalizeExternalSecretSelection(formValue(r, "backupSecretKeySecret"))
		opts.Values["backup"] = map[string]any{
			"enabled":          true,
			"schedule":         formDefault(r, "backupSchedule", "0 */6 * * *"),
			"restoreOnStartup": formBool(r, "backupRestoreOnStartup"),
			"provider":         bp,
			"s3": map[string]any{
				"endpoint": formValue(r, "bkS3Endpoint"),
				"bucket":   formValue(r, "bkS3Bucket"),
				"region":   formDefault(r, "bkS3Region", "us-east-1"),
				"prefix":   formValue(r, "bkS3Prefix"),
			},
			"github": map[string]any{
				"repository": formValue(r, "bkGithubRepo"),
				"branch":     formValue(r, "bkGithubBranch"),
				"path":       formValue(r, "bkGithubPath"),
			},
		}

		if accessKeyRef != "" && secretKeyRef != "" {
			opts.Values["backupCredentialSecretRefs"] = map[string]any{
				"accessKeyId":     accessKeyRef,
				"secretAccessKey": secretKeyRef,
			}
		}

		ak := formValue(r, "bkCredentialsAccessKey")
		sk := formValue(r, "bkCredentialsSecretKey")
		if accessKeyRef == "" && secretKeyRef == "" && ak != "" && sk != "" {
			opts.Values["backupCredentials"] = map[string]any{
				"accessKeyId":     ak,
				"secretAccessKey": sk,
			}
		}
	}

	extraToolVersions := strings.TrimSpace(formValue(r, "extraToolVersions"))
	if extraToolVersions != "" {
		opts.Values["extraSoftware"] = map[string]any{
			"toolVersions": extraToolVersions,
		}
	}

	// Config fields (cfg:key form names)
	configFields := make(map[string]string)
	for key, vals := range r.Form {
		after, ok := strings.CutPrefix(key, "cfg:")
		if !ok || len(vals) == 0 {
			continue
		}
		v := vals[len(vals)-1]
		// Convert HTML checkbox "on" to "true" for checkbox-type fields
		if v == "on" && bots != nil {
			botCfg := bots.Get(string(opts.BotType))
			if botCfg != nil {
				for _, f := range botCfg.AllFields() {
					if f.Key == after && f.Type == "checkbox" {
						v = "true"
						break
					}
				}
			}
		}
		configFields[after] = v
	}
	if len(configFields) > 0 {
		opts.ConfigFields = configFields
	}

	return opts, nil
}

// formDefault returns the form value for key, or fallback if empty.
func allFormValues(r *http.Request) map[string]string {
	out := make(map[string]string, len(r.Form))
	for k, vals := range r.Form {
		if len(vals) == 0 {
			continue
		}
		out[k] = vals[len(vals)-1]
	}
	return out
}

func formValue(r *http.Request, key string) string {
	vals := r.Form[key]
	if len(vals) == 0 {
		return ""
	}
	return vals[len(vals)-1]
}

func formBool(r *http.Request, key string) bool {
	switch strings.ToLower(strings.TrimSpace(formValue(r, key))) {
	case "on", "true", "1":
		return true
	default:
		return false
	}
}

func formDefault(r *http.Request, key, fallback string) string {
	if v := formValue(r, key); v != "" {
		return v
	}
	return fallback
}

func normalizeExternalSecretSelection(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if ref, ok := strings.CutPrefix(v, "1p:"); ok {
		return ref
	}
	return v
}

// splitLines splits a string on newlines, trims whitespace, and drops empties.
func splitLines(s string) []string {
	var out []string
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// randomHex returns a hex-encoded random string of n bytes (2n characters).
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		seed := fmt.Sprintf("%d", time.Now().UnixNano())
		sum := sha256.Sum256([]byte(seed))
		return hex.EncodeToString(sum[:n])
	}
	return hex.EncodeToString(b)
}

// ensureBuiltInPostgresPassword guarantees built-in Postgres charts have a non-empty
// password when enabled. IronClaw defaults to postgresql.enabled=true even if the
// incoming install values omit the postgresql map entirely.
func ensureBuiltInPostgresPassword(opts *service.InstallOptions) {
	if opts == nil || opts.Values == nil {
		return
	}
	if opts.BotType != service.BotTypeIronClaw {
		return
	}

	pg, ok := opts.Values["postgresql"].(map[string]any)
	if !ok || pg == nil {
		pg = make(map[string]any)
		opts.Values["postgresql"] = pg
	}

	enabled, hasEnabled := pg["enabled"].(bool)
	if !hasEnabled {
		enabled = true
		pg["enabled"] = true
	}
	if !enabled {
		return
	}

	if pw, _ := pg["password"].(string); strings.TrimSpace(pw) == "" {
		pg["password"] = randomHex(16)
	}
}

// flattenValues converts nested maps to dotted key paths for display.
// e.g., {"networkPolicy": {"egress": true}} → {"networkPolicy.egress": true}
func flattenValues(prefix string, m map[string]any) map[string]any {
	out := make(map[string]any)
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch val := v.(type) {
		case map[string]any:
			maps.Copy(out, flattenValues(key, val))
		default:
			out[key] = v
			_ = val
		}
	}
	return out
}
