package service

import "testing"

func TestWorkspaceConfig_Validate_Disabled(t *testing.T) {
	w := &WorkspaceConfig{Enabled: false}
	if err := w.Validate(); err != nil {
		t.Errorf("disabled config should be valid: %v", err)
	}
}

func TestWorkspaceConfig_Validate_S3_Valid(t *testing.T) {
	w := &WorkspaceConfig{
		Enabled:  true,
		Provider: "s3",
		S3:       S3Config{Bucket: "my-bucket", SecretName: "aws-creds"},
	}
	if err := w.Validate(); err != nil {
		t.Errorf("valid S3 config should pass: %v", err)
	}
}

func TestWorkspaceConfig_Validate_S3_MissingBucket(t *testing.T) {
	w := &WorkspaceConfig{
		Enabled:  true,
		Provider: "s3",
		S3:       S3Config{SecretName: "aws-creds"},
	}
	if err := w.Validate(); err == nil {
		t.Error("S3 without bucket should fail")
	}
}

func TestWorkspaceConfig_Validate_S3_MissingSecret(t *testing.T) {
	w := &WorkspaceConfig{
		Enabled:  true,
		Provider: "s3",
		S3:       S3Config{Bucket: "my-bucket"},
	}
	if err := w.Validate(); err == nil {
		t.Error("S3 without secretName should fail")
	}
}

func TestWorkspaceConfig_Validate_GitHub_Valid(t *testing.T) {
	w := &WorkspaceConfig{
		Enabled:  true,
		Provider: "github",
		GitHub:   GitHubConfig{Repository: "user/repo", SecretName: "gh-token"},
	}
	if err := w.Validate(); err != nil {
		t.Errorf("valid GitHub config should pass: %v", err)
	}
}

func TestWorkspaceConfig_Validate_GitHub_MissingRepo(t *testing.T) {
	w := &WorkspaceConfig{
		Enabled:  true,
		Provider: "github",
		GitHub:   GitHubConfig{SecretName: "gh-token"},
	}
	if err := w.Validate(); err == nil {
		t.Error("GitHub without repository should fail")
	}
}

func TestWorkspaceConfig_Validate_GitHub_MissingSecret(t *testing.T) {
	w := &WorkspaceConfig{
		Enabled:  true,
		Provider: "github",
		GitHub:   GitHubConfig{Repository: "user/repo"},
	}
	if err := w.Validate(); err == nil {
		t.Error("GitHub without secretName should fail")
	}
}

func TestWorkspaceConfig_Validate_InvalidProvider(t *testing.T) {
	w := &WorkspaceConfig{Enabled: true, Provider: "azure"}
	if err := w.Validate(); err == nil {
		t.Error("invalid provider should fail")
	}
}

func TestWorkspaceConfig_ToHelmValues_Disabled(t *testing.T) {
	w := &WorkspaceConfig{Enabled: false}
	vals := w.ToHelmValues()
	if vals["enabled"] != false {
		t.Errorf("disabled should have enabled=false")
	}
}

func TestWorkspaceConfig_ToHelmValues_S3(t *testing.T) {
	w := &WorkspaceConfig{
		Enabled:  true,
		Provider: "s3",
		S3:       S3Config{Bucket: "my-bucket", Region: "us-west-2", Prefix: "backups", SecretName: "aws-creds"},
	}
	vals := w.ToHelmValues()
	if vals["enabled"] != true {
		t.Error("should be enabled")
	}
	if vals["provider"] != "s3" {
		t.Errorf("provider = %v", vals["provider"])
	}
	s3 := vals["s3"].(map[string]any)
	if s3["bucket"] != "my-bucket" {
		t.Errorf("bucket = %v", s3["bucket"])
	}
	if s3["region"] != "us-west-2" {
		t.Errorf("region = %v", s3["region"])
	}
}

func TestWorkspaceConfig_ToHelmValues_S3_DefaultRegion(t *testing.T) {
	w := &WorkspaceConfig{
		Enabled:  true,
		Provider: "s3",
		S3:       S3Config{Bucket: "b", SecretName: "s"},
	}
	vals := w.ToHelmValues()
	s3 := vals["s3"].(map[string]any)
	if s3["region"] != "us-east-1" {
		t.Errorf("default region = %v, want us-east-1", s3["region"])
	}
}

func TestWorkspaceConfig_ToHelmValues_GitHub(t *testing.T) {
	w := &WorkspaceConfig{
		Enabled:  true,
		Provider: "github",
		GitHub:   GitHubConfig{Repository: "user/repo", Branch: "develop", Path: "workspace", SecretName: "gh"},
	}
	vals := w.ToHelmValues()
	gh := vals["github"].(map[string]any)
	if gh["repository"] != "user/repo" {
		t.Errorf("repository = %v", gh["repository"])
	}
	if gh["branch"] != "develop" {
		t.Errorf("branch = %v", gh["branch"])
	}
}

func TestWorkspaceConfig_ToHelmValues_GitHub_DefaultBranch(t *testing.T) {
	w := &WorkspaceConfig{
		Enabled:  true,
		Provider: "github",
		GitHub:   GitHubConfig{Repository: "user/repo", SecretName: "gh"},
	}
	vals := w.ToHelmValues()
	gh := vals["github"].(map[string]any)
	if gh["branch"] != "main" {
		t.Errorf("default branch = %v, want main", gh["branch"])
	}
}
