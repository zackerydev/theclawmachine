package sync

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// Options holds sync command configuration.
type Options struct {
	Provider string
	Source   string

	// S3 options
	Bucket string
	Region string
	Prefix string

	// GitHub options
	Repo   string
	Branch string
	Path   string
}

// Run executes the sync operation based on provider.
func Run(ctx context.Context, opts Options) error {
	switch opts.Provider {
	case "s3":
		return syncS3(ctx, opts)
	case "github":
		return syncGitHub(ctx, opts)
	default:
		return fmt.Errorf("unsupported provider: %s", opts.Provider)
	}
}

func syncS3(ctx context.Context, opts Options) error {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(opts.Region))
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg)

	return filepath.WalkDir(opts.Source, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, err := filepath.Rel(opts.Source, path)
		if err != nil {
			return err
		}

		key := rel
		if opts.Prefix != "" {
			key = opts.Prefix + "/" + rel
		}

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("opening %s: %w", path, err)
		}

		slog.Info("uploading", "file", rel, "bucket", opts.Bucket, "key", key)
		_, err = client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: &opts.Bucket,
			Key:    &key,
			Body:   f,
		})
		if closeErr := f.Close(); closeErr != nil {
			slog.Warn("sync s3: failed to close file", "path", path, "error", closeErr)
			if err == nil {
				return closeErr
			}
		}
		return err
	})
}

func syncGitHub(ctx context.Context, opts Options) error {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable required for github provider")
	}

	dir, err := os.MkdirTemp("", "clawmachine-sync-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(dir); removeErr != nil {
			slog.Warn("sync github: failed to remove temp dir", "path", dir, "error", removeErr)
		}
	}()

	repoURL := "https://github.com/" + opts.Repo + ".git"
	auth := &http.BasicAuth{Username: "x-access-token", Password: token}

	repo, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL:           repoURL,
		Auth:          auth,
		ReferenceName: plumbing.NewBranchReferenceName(opts.Branch),
		SingleBranch:  true,
		Depth:         1,
	})
	if err != nil {
		// Branch may not exist yet; clone default and create branch
		repo, err = git.PlainClone(dir, false, &git.CloneOptions{
			URL:   repoURL,
			Auth:  auth,
			Depth: 1,
		})
		if err != nil {
			return fmt.Errorf("cloning repo: %w", err)
		}

		wt, err := repo.Worktree()
		if err != nil {
			return err
		}
		err = wt.Checkout(&git.CheckoutOptions{
			Branch: plumbing.NewBranchReferenceName(opts.Branch),
			Create: true,
		})
		if err != nil {
			return fmt.Errorf("creating branch %s: %w", opts.Branch, err)
		}
	}

	// Copy files from source to repo path
	destDir := dir
	if opts.Path != "" {
		destDir = filepath.Join(dir, opts.Path)
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return err
		}
	}

	err = filepath.WalkDir(opts.Source, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(opts.Source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(destDir, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	})
	if err != nil {
		return fmt.Errorf("copying files: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	if _, err := wt.Add("."); err != nil {
		return fmt.Errorf("staging files: %w", err)
	}

	_, err = wt.Commit(fmt.Sprintf("backup: %s", time.Now().UTC().Format(time.RFC3339)), &git.CommitOptions{
		Author: &object.Signature{
			Name:  "ClawMachine Backup",
			Email: "backup@clawmachine.dev",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	err = repo.Push(&git.PushOptions{
		Auth: auth,
		RefSpecs: []gitconfig.RefSpec{
			gitconfig.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", opts.Branch, opts.Branch)),
		},
	})
	if err != nil {
		return fmt.Errorf("pushing: %w", err)
	}

	slog.Info("pushed backup to github", "repo", opts.Repo, "branch", opts.Branch)
	return nil
}
