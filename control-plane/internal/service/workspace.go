package service

import "fmt"

// WorkspaceConfig holds the workspace import configuration for a bot install.
type WorkspaceConfig struct {
	Enabled  bool             `json:"enabled"`
	Provider string           `json:"provider"` // "s3" or "github"
	S3       S3Config         `json:"s3"`
	GitHub   GitHubConfig     `json:"github"`
}

type S3Config struct {
	Bucket     string `json:"bucket"`
	Region     string `json:"region"`
	Prefix     string `json:"prefix"`
	Endpoint   string `json:"endpoint"`   // S3-compatible endpoint (e.g. LocalStack)
	SecretName string `json:"secretName"`
}

type GitHubConfig struct {
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
	Path       string `json:"path"`
	SecretName string `json:"secretName"`
}

// Validate checks that a workspace config is valid when enabled.
func (w *WorkspaceConfig) Validate() error {
	if !w.Enabled {
		return nil
	}

	switch w.Provider {
	case "s3":
		if w.S3.Bucket == "" {
			return fmt.Errorf("workspace: S3 bucket is required")
		}
		if w.S3.SecretName == "" {
			return fmt.Errorf("workspace: S3 secret name is required")
		}
	case "github":
		if w.GitHub.Repository == "" {
			return fmt.Errorf("workspace: GitHub repository is required")
		}
		if w.GitHub.SecretName == "" {
			return fmt.Errorf("workspace: GitHub secret name is required")
		}
	default:
		return fmt.Errorf("workspace: invalid provider %q, must be \"s3\" or \"github\"", w.Provider)
	}

	return nil
}

// ToHelmValues converts the workspace config into a map suitable for Helm values.
func (w *WorkspaceConfig) ToHelmValues() map[string]any {
	if !w.Enabled {
		return map[string]any{
			"enabled": false,
		}
	}

	vals := map[string]any{
		"enabled":  true,
		"provider": w.Provider,
	}

	switch w.Provider {
	case "s3":
		region := w.S3.Region
		if region == "" {
			region = "us-east-1"
		}
		vals["s3"] = map[string]any{
			"bucket":     w.S3.Bucket,
			"region":     region,
			"prefix":     w.S3.Prefix,
			"secretName": w.S3.SecretName,
		}
	case "github":
		branch := w.GitHub.Branch
		if branch == "" {
			branch = "main"
		}
		vals["github"] = map[string]any{
			"repository": w.GitHub.Repository,
			"branch":     branch,
			"path":       w.GitHub.Path,
			"secretName": w.GitHub.SecretName,
		}
	}

	return vals
}
