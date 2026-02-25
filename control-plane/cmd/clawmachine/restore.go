package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/spf13/cobra"
)

func newRestoreCmd() *cobra.Command {
	var (
		mode      string
		workspace string
		bucket    string
		endpoint  string
		region    string
		prefix    string
	)

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore bot workspace or database from S3",
		Long: `Download and restore a backup from an S3-compatible storage service.

Two modes are supported:
  filesystem  Extract a tar.gz archive into the workspace directory (OpenClaw, PicoClaw)
  pgdump      Pipe a gzipped SQL dump into psql via DATABASE_URL (IronClaw)

If no backup exists in S3, the command exits successfully (fresh start).`,
		Example: `  # Restore a workspace directory
  clawmachine restore --bucket my-backups --workspace /workspace

  # Restore an IronClaw database
  clawmachine restore --mode pgdump --bucket my-backups`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			s3Client, err := newS3Client(ctx, endpoint, region)
			if err != nil {
				return fmt.Errorf("creating S3 client: %w", err)
			}

			var ext string
			switch mode {
			case "filesystem":
				ext = "tar.gz"
			case "pgdump":
				ext = "sql.gz"
			default:
				return fmt.Errorf("unknown restore mode %q (use filesystem or pgdump)", mode)
			}

			key := filepath.Join(prefix, fmt.Sprintf("latest.%s", ext))
			slog.Info("downloading backup", "bucket", bucket, "key", key, "mode", mode)

			resp, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(key),
			})
			if err != nil {
				var noSuchKey *types.NoSuchKey
				if errors.As(err, &noSuchKey) {
					slog.Info("no backup found, starting fresh")
					return nil
				}
				var notFound *types.NotFound
				if errors.As(err, &notFound) {
					slog.Info("no backup found, starting fresh")
					return nil
				}
				return fmt.Errorf("downloading backup: %w", err)
			}
			defer func() {
				if closeErr := resp.Body.Close(); closeErr != nil {
					slog.Warn("restore: failed to close response body", "error", closeErr)
				}
			}()

			switch mode {
			case "filesystem":
				slog.Info("extracting backup", "workspace", workspace)
				if err := extractTarGz(resp.Body, workspace); err != nil {
					return fmt.Errorf("extracting backup: %w", err)
				}

			case "pgdump":
				dbURL := os.Getenv("DATABASE_URL")
				if dbURL == "" {
					return fmt.Errorf("DATABASE_URL environment variable is required for pgdump mode")
				}
				slog.Info("restoring database from pg_dump")
				if err := runPsqlRestore(dbURL, resp.Body); err != nil {
					return fmt.Errorf("pg restore: %w", err)
				}
			}

			slog.Info("restore complete")
			return nil
		},
	}

	cmd.Flags().StringVar(&mode, "mode", "filesystem", "Restore mode: filesystem or pgdump")
	cmd.Flags().StringVar(&workspace, "workspace", "/workspace", "Workspace directory to restore into (filesystem mode)")
	cmd.Flags().StringVar(&bucket, "bucket", "", "S3 bucket name (required)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "S3-compatible endpoint URL")
	cmd.Flags().StringVar(&region, "region", "us-east-1", "S3 region")
	cmd.Flags().StringVar(&prefix, "prefix", "", "S3 key prefix")
	if err := cmd.MarkFlagRequired("bucket"); err != nil {
		panic(fmt.Errorf("marking required flag bucket: %w", err))
	}

	return cmd
}

// runPsqlRestore decompresses a gzipped SQL dump and pipes it into psql.
func runPsqlRestore(dbURL string, r io.Reader) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("decompressing backup: %w", err)
	}
	defer func() {
		if closeErr := gzr.Close(); closeErr != nil {
			slog.Warn("restore: failed to close gzip reader", "error", closeErr)
		}
	}()

	cmd := exec.Command("psql", dbURL)
	cmd.Stdin = gzr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql exited with error: %w", err)
	}
	return nil
}

func extractTarGz(r io.Reader, destDir string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := gzr.Close(); closeErr != nil {
			slog.Warn("restore: failed to close tar gzip reader", "error", closeErr)
		}
	}()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)

		if !filepath.IsLocal(header.Name) {
			return fmt.Errorf("invalid path in archive: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				if closeErr := f.Close(); closeErr != nil {
					slog.Warn("restore: failed to close extracted file", "path", target, "error", closeErr)
				}
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}
