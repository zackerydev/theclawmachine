package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/zackerydev/clawmachine/control-plane/internal/service"
)

type BackupHandler struct {
	backup  *service.BackupService
	helm    *service.HelmService
	tmpl    *service.TemplateService
	k8s     *service.KubernetesService
	secrets SecretsManager
}

func NewBackupHandler(backup *service.BackupService, helm *service.HelmService, tmpl *service.TemplateService, k8s *service.KubernetesService, secrets SecretsManager) *BackupHandler {
	return &BackupHandler{backup: backup, helm: helm, tmpl: tmpl, k8s: k8s, secrets: secrets}
}

func (h *BackupHandler) BackupConfigPage(w http.ResponseWriter, r *http.Request) {
	if h.backup == nil || h.helm == nil || h.tmpl == nil {
		http.Error(w, "backup service unavailable", http.StatusServiceUnavailable)
		return
	}

	name := r.PathValue("name")

	config, err := h.backup.GetBackupConfig(name, defaultNamespace())
	if err != nil {
		slog.Error("failed to get backup config", "name", name, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	info, err := h.helm.Status(name, defaultNamespace())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	values := map[string]string{
		"backupEnabled":          boolFormValue(config.Enabled),
		"backupSchedule":         config.Schedule,
		"backupProvider":         config.Provider,
		"backupRestoreOnStartup": boolFormValue(config.RestoreOnStartup),
		"bkS3Endpoint":           config.S3.Endpoint,
		"bkS3Bucket":             config.S3.Bucket,
		"bkS3Region":             config.S3.Region,
		"bkS3Prefix":             config.S3.Prefix,
		"bkGithubRepo":           config.GitHub.Repository,
		"bkGithubBranch":         config.GitHub.Branch,
		"bkGithubPath":           config.GitHub.Path,
		"backupAccessKeySecret":  config.Credentials.AccessKeyIDSecretRef.Name,
		"backupSecretKeySecret":  config.Credentials.SecretAccessKeySecretRef.Name,
	}

	if values["backupSchedule"] == "" {
		values["backupSchedule"] = "0 */6 * * *"
	}
	if values["backupProvider"] == "" {
		values["backupProvider"] = "s3"
	}
	if values["bkS3Region"] == "" {
		values["bkS3Region"] = "us-east-1"
	}
	if values["bkGithubBranch"] == "" {
		values["bkGithubBranch"] = "backups"
	}

	secrets := availableSecretsForTemplates(r.Context(), h.secrets)
	for _, s := range secrets {
		if values["backupAccessKeySecret"] != "" && values["backupAccessKeySecret"] == s.TargetSecret {
			values["backupAccessKeySecret"] = s.Name
		}
		if values["backupSecretKeySecret"] != "" && values["backupSecretKeySecret"] == s.TargetSecret {
			values["backupSecretKeySecret"] = s.Name
		}
	}

	var lastBackupLabel string
	var lastBackupKnown bool
	var lastBackupLookupError bool
	if config.Enabled && h.k8s != nil {
		lastBackup, found, err := h.k8s.GetBackupLastSuccess(r.Context(), defaultNamespace(), name)
		if err != nil {
			slog.Warn("backup: failed to get last successful backup", "release", name, "error", err)
			lastBackupLookupError = true
		} else if found {
			lastBackupKnown = true
			lastBackupLabel = lastBackup.UTC().Format(time.RFC3339)
		}
	}

	data := struct {
		Name                  string
		BotType               string
		Config                *service.BackupConfig
		Secrets               []AvailableSecret
		Values                map[string]string
		LastBackupKnown       bool
		LastBackupLabel       string
		LastBackupLookupError bool
	}{
		Name:                  name,
		BotType:               info.BotType,
		Config:                config,
		Secrets:               secrets,
		Values:                values,
		LastBackupKnown:       lastBackupKnown,
		LastBackupLabel:       lastBackupLabel,
		LastBackupLookupError: lastBackupLookupError,
	}

	if !renderOrError(w, r, h.tmpl, "backup-config", data, isHTMX(r)) {
		return
	}
}

func (h *BackupHandler) SaveBackupConfig(w http.ResponseWriter, r *http.Request) {
	if h.backup == nil || h.helm == nil {
		http.Error(w, "backup service unavailable", http.StatusServiceUnavailable)
		return
	}

	name := r.PathValue("name")

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	info, err := h.helm.Status(name, defaultNamespace())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	enabled := formBool(r, "backupEnabled")
	if !enabled {
		// Backward compatibility with earlier form field name.
		enabled = formBool(r, "enabled")
	}

	provider := formDefault(r, "backupProvider", formValue(r, "provider"))
	if provider == "" {
		provider = "s3"
	}

	schedule := formDefault(r, "backupSchedule", formValue(r, "schedule"))
	if schedule == "" {
		schedule = "0 */6 * * *"
	}

	restoreOnStartup := formBool(r, "backupRestoreOnStartup")
	if !restoreOnStartup {
		restoreOnStartup = formBool(r, "restoreOnStartup")
	}

	accessKeySelection := normalizeExternalSecretSelection(formValue(r, "backupAccessKeySecret"))
	secretKeySelection := normalizeExternalSecretSelection(formValue(r, "backupSecretKeySecret"))
	accessKeySecretName, secretKeySecretName := h.resolveBackupCredentialTargetSecrets(r.Context(), accessKeySelection, secretKeySelection)

	config := service.BackupConfig{
		Enabled:           enabled,
		RestoreOnStartup:  restoreOnStartup,
		Schedule:          schedule,
		Provider:          provider,
		SecretName:        formValue(r, "secretName"),
		CredentialsSecret: formValue(r, "credentialsSecret"),
		Credentials: service.BackupCredential{
			AccessKeyIDSecretRef: service.SecretKeyRef{
				Name: accessKeySecretName,
				Key:  "value",
			},
			SecretAccessKeySecretRef: service.SecretKeyRef{
				Name: secretKeySecretName,
				Key:  "value",
			},
		},
		S3: service.S3Config{
			Endpoint: formDefault(r, "bkS3Endpoint", formValue(r, "s3Endpoint")),
			Bucket:   formDefault(r, "bkS3Bucket", formValue(r, "s3Bucket")),
			Region:   formDefault(r, "bkS3Region", formDefault(r, "s3Region", "us-east-1")),
			Prefix:   formDefault(r, "bkS3Prefix", formValue(r, "s3Prefix")),
		},
		GitHub: service.GitHubConfig{
			Repository: formDefault(r, "bkGithubRepo", formValue(r, "githubRepository")),
			Branch:     formDefault(r, "bkGithubBranch", formDefault(r, "githubBranch", "backups")),
			Path:       formDefault(r, "bkGithubPath", formValue(r, "githubPath")),
		},
	}

	if config.Enabled {
		switch config.Provider {
		case "s3":
			if strings.TrimSpace(config.S3.Bucket) == "" {
				htmxError(w, r, "S3 bucket is required when backup is enabled.", http.StatusBadRequest)
				return
			}
		case "github":
			if strings.TrimSpace(config.GitHub.Repository) == "" {
				htmxError(w, r, "GitHub repository is required when backup is enabled.", http.StatusBadRequest)
				return
			}
		default:
			htmxError(w, r, "Backup provider must be s3 or github.", http.StatusBadRequest)
			return
		}
	}

	slog.Info("backup: saving config", "release", name, "enabled", config.Enabled, "provider", config.Provider, "schedule", config.Schedule)
	if err := h.backup.ConfigureBackup(r.Context(), name, defaultNamespace(), service.BotType(info.BotType), config); err != nil {
		slog.Error("backup: save failed", "release", name, "error", err)
		if isHTMX(r) {
			w.WriteHeader(http.StatusInternalServerError)
			if _, writeErr := w.Write([]byte(`<div class="alert alert-error mb-4">Failed to save backup configuration. Check server logs for details.</div>`)); writeErr != nil {
				slog.Warn("backup: failed to write HTMX error response", "release", name, "error", writeErr)
			}
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("backup: config saved", "release", name)
	if isHTMX(r) {
		if _, err := w.Write([]byte(`<div class="alert alert-success mb-4">Backup configuration saved.</div>`)); err != nil {
			slog.Warn("backup: failed to write HTMX success response", "release", name, "error", err)
		}
		return
	}
	http.Redirect(w, r, "/bots/"+name+"/backup", http.StatusSeeOther)
}

func (h *BackupHandler) resolveBackupCredentialTargetSecrets(ctx context.Context, accessSelection, secretSelection string) (string, string) {
	access := strings.TrimSpace(accessSelection)
	secret := strings.TrimSpace(secretSelection)
	if h.secrets == nil || (access == "" && secret == "") {
		return access, secret
	}

	all, err := h.secrets.ListExternalSecrets(ctx, defaultNamespace())
	if err != nil {
		return access, secret
	}

	byName := make(map[string]string, len(all))
	for _, s := range all {
		if s.Status != "Synced" || s.TargetSecret == "" {
			continue
		}
		byName[s.Name] = s.TargetSecret
	}
	if target := byName[access]; target != "" {
		access = target
	}
	if target := byName[secret]; target != "" {
		secret = target
	}
	return access, secret
}

func boolFormValue(v bool) string {
	if v {
		return "on"
	}
	return "false"
}
