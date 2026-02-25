package service

import (
	"context"
	"fmt"

	"helm.sh/helm/v4/pkg/action"
)

// BackupConfig holds backup configuration for a bot release.
type BackupConfig struct {
	Enabled           bool   `json:"enabled"`
	RestoreOnStartup  bool   `json:"restoreOnStartup"`
	Schedule          string `json:"schedule"`
	Provider          string `json:"provider"` // "s3" or "github"
	S3                S3Config
	GitHub            GitHubConfig
	SecretName        string           `json:"secretName"`
	Credentials       BackupCredential `json:"credentials"`
	CredentialsSecret string           `json:"credentialsSecret"`
}

type BackupCredential struct {
	AccessKeyIDSecretRef     SecretKeyRef `json:"accessKeyIdSecretRef"`
	SecretAccessKeySecretRef SecretKeyRef `json:"secretAccessKeySecretRef"`
}

type SecretKeyRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// S3Config is declared in workspace.go; Endpoint is used for S3-compatible services (e.g. LocalStack).

// BackupService manages backup configuration for bot releases.
type BackupService struct {
	helm *HelmService
}

// NewBackupService creates a new BackupService.
func NewBackupService(helm *HelmService) *BackupService {
	return &BackupService{helm: helm}
}

// ConfigureBackup updates a release with backup configuration values.
func (s *BackupService) ConfigureBackup(ctx context.Context, name, namespace string, botType BotType, config BackupConfig) error {
	values := map[string]any{
		"backup": map[string]any{
			"enabled":          config.Enabled,
			"restoreOnStartup": config.RestoreOnStartup,
			"schedule":         config.Schedule,
			"provider":         config.Provider,
			"s3": map[string]any{
				"bucket":   config.S3.Bucket,
				"region":   config.S3.Region,
				"prefix":   config.S3.Prefix,
				"endpoint": config.S3.Endpoint,
			},
			"github": map[string]any{
				"repository": config.GitHub.Repository,
				"branch":     config.GitHub.Branch,
				"path":       config.GitHub.Path,
			},
			"secretName":        config.SecretName,
			"credentialsSecret": config.CredentialsSecret,
			"credentials": map[string]any{
				"accessKeyIdSecretRef": map[string]any{
					"name": config.Credentials.AccessKeyIDSecretRef.Name,
					"key":  config.Credentials.AccessKeyIDSecretRef.Key,
				},
				"secretAccessKeySecretRef": map[string]any{
					"name": config.Credentials.SecretAccessKeySecretRef.Name,
					"key":  config.Credentials.SecretAccessKeySecretRef.Key,
				},
			},
		},
	}

	_, err := s.helm.Upgrade(ctx, name, namespace, botType, values)
	if err != nil {
		return fmt.Errorf("configuring backup for %q: %w", name, err)
	}
	return nil
}

// GetBackupConfig retrieves the backup configuration for a release by reading its current values.
func (s *BackupService) GetBackupConfig(name, namespace string) (*BackupConfig, error) {
	cfg, err := s.helm.initActionConfig(namespace)
	if err != nil {
		return nil, err
	}

	client := action.NewGetValues(cfg)
	client.AllValues = true
	vals, err := client.Run(name)
	if err != nil {
		return nil, fmt.Errorf("getting values for %q: %w", name, err)
	}

	backup, _ := vals["backup"].(map[string]any)
	if backup == nil {
		return &BackupConfig{}, nil
	}

	config := &BackupConfig{
		Enabled:           getBool(backup, "enabled"),
		RestoreOnStartup:  getBool(backup, "restoreOnStartup"),
		Schedule:          getString(backup, "schedule"),
		Provider:          getString(backup, "provider"),
		SecretName:        getString(backup, "secretName"),
		CredentialsSecret: getString(backup, "credentialsSecret"),
	}

	if s3, ok := backup["s3"].(map[string]any); ok {
		config.S3.Bucket = getString(s3, "bucket")
		config.S3.Region = getString(s3, "region")
		config.S3.Prefix = getString(s3, "prefix")
		config.S3.Endpoint = getString(s3, "endpoint")
	}

	if gh, ok := backup["github"].(map[string]any); ok {
		config.GitHub.Repository = getString(gh, "repository")
		config.GitHub.Branch = getString(gh, "branch")
		config.GitHub.Path = getString(gh, "path")
	}

	if creds, ok := backup["credentials"].(map[string]any); ok {
		if accessRef, ok := creds["accessKeyIdSecretRef"].(map[string]any); ok {
			config.Credentials.AccessKeyIDSecretRef.Name = getString(accessRef, "name")
			config.Credentials.AccessKeyIDSecretRef.Key = getString(accessRef, "key")
		}
		if secretRef, ok := creds["secretAccessKeySecretRef"].(map[string]any); ok {
			config.Credentials.SecretAccessKeySecretRef.Name = getString(secretRef, "name")
			config.Credentials.SecretAccessKeySecretRef.Key = getString(secretRef, "key")
		}
	}

	return config, nil
}

func getString(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func getBool(m map[string]any, key string) bool {
	v, _ := m[key].(bool)
	return v
}
