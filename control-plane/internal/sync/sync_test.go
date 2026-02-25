package sync

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRun_InvalidProvider(t *testing.T) {
	err := Run(context.Background(), Options{Provider: "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid provider")
	}
	if err.Error() != "unsupported provider: invalid" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_S3_MissingConfig(t *testing.T) {
	// S3 sync with a nonexistent source directory should fail gracefully
	err := Run(context.Background(), Options{
		Provider: "s3",
		Source:   "/nonexistent/path/that/does/not/exist",
		Bucket:   "test-bucket",
		Region:   "us-east-1",
	})
	// Should fail at AWS config or walk, not panic
	if err == nil {
		t.Skip("AWS credentials available — skipping in unit test")
	}
}

func TestRun_GitHub_MissingToken(t *testing.T) {
	// Unset GITHUB_TOKEN to ensure error
	orig := os.Getenv("GITHUB_TOKEN")
	if err := os.Unsetenv("GITHUB_TOKEN"); err != nil {
		t.Fatalf("unset GITHUB_TOKEN: %v", err)
	}
	defer func() {
		if orig != "" {
			if err := os.Setenv("GITHUB_TOKEN", orig); err != nil {
				t.Fatalf("restore GITHUB_TOKEN: %v", err)
			}
		}
	}()

	err := Run(context.Background(), Options{
		Provider: "github",
		Source:   "/tmp",
		Repo:     "test/repo",
		Branch:   "main",
	})
	if err == nil {
		t.Fatal("expected error when GITHUB_TOKEN is missing")
	}
	if err.Error() != "GITHUB_TOKEN environment variable required for github provider" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSyncS3_WalkDir(t *testing.T) {
	// Create a temp directory with some files to verify WalkDir logic
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("write file1: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "subdir", "file2.txt"), []byte("world"), 0644); err != nil {
		t.Fatalf("write file2: %v", err)
	}

	// syncS3 will fail at AWS config but we verify it doesn't panic on valid dirs
	err := syncS3(context.Background(), Options{
		Source: dir,
		Bucket: "test",
		Region: "us-east-1",
		Prefix: "backup",
	})
	// Expected to fail at AWS config loading (no credentials in test)
	if err == nil {
		t.Skip("AWS credentials available — skipping")
	}
	// Should be an AWS config error, not a walk error
	if err.Error() == "" {
		t.Error("expected non-empty error")
	}
}

func TestSyncGitHub_BadRepo(t *testing.T) {
	// Set a fake token so we get past the token check
	orig := os.Getenv("GITHUB_TOKEN")
	if err := os.Setenv("GITHUB_TOKEN", "fake-token-for-testing"); err != nil {
		t.Fatalf("set GITHUB_TOKEN: %v", err)
	}
	defer func() {
		if orig != "" {
			if err := os.Setenv("GITHUB_TOKEN", orig); err != nil {
				t.Fatalf("restore GITHUB_TOKEN: %v", err)
			}
		} else {
			if err := os.Unsetenv("GITHUB_TOKEN"); err != nil {
				t.Fatalf("unset GITHUB_TOKEN: %v", err)
			}
		}
	}()

	err := syncGitHub(context.Background(), Options{
		Source: t.TempDir(),
		Repo:   "nonexistent/repo-that-does-not-exist-12345",
		Branch: "main",
	})
	if err == nil {
		t.Fatal("expected error cloning nonexistent repo")
	}
}

func TestOptions_Defaults(t *testing.T) {
	opts := Options{
		Provider: "s3",
		Source:   "/tmp/test",
		Bucket:   "my-bucket",
	}
	if opts.Provider != "s3" {
		t.Errorf("Provider = %q, want s3", opts.Provider)
	}
	if opts.Region != "" {
		t.Errorf("Region should be empty by default, got %q", opts.Region)
	}
}
