package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"
)

func newBackupCmd() *cobra.Command {
	var (
		mode      string
		workspace string
		bucket    string
		endpoint  string
		region    string
		prefix    string
	)

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Back up bot workspace or database to S3",
		Long: `Create a backup and upload it to an S3-compatible storage service.

Two modes are supported:
  filesystem  Tar and gzip a workspace directory (OpenClaw, PicoClaw)
  pgdump      Run pg_dump against DATABASE_URL (IronClaw)

Credentials are read from AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY
environment variables, or from the default AWS credential chain.`,
		Example: `  # Backup a workspace directory
  clawmachine backup --bucket my-backups --workspace /workspace

  # Backup an IronClaw database
  clawmachine backup --mode pgdump --bucket my-backups

  # Backup to Cloudflare R2
  clawmachine backup --bucket my-backups --endpoint https://acct.r2.cloudflarestorage.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			s3Client, err := newS3Client(ctx, endpoint, region)
			if err != nil {
				return fmt.Errorf("creating S3 client: %w", err)
			}

			var tmpFile *os.File
			var ext string

			switch mode {
			case "filesystem":
				ext = "tar.gz"
				slog.Info("creating workspace archive", "workspace", workspace)
				tmpFile, err = os.CreateTemp("", "clawmachine-backup-*.tar.gz")
				if err != nil {
					return fmt.Errorf("creating temp file: %w", err)
				}
				defer func() {
					if closeErr := tmpFile.Close(); closeErr != nil {
						slog.Warn("failed to close temp file", "path", tmpFile.Name(), "error", closeErr)
					}
					if removeErr := os.Remove(tmpFile.Name()); removeErr != nil && !os.IsNotExist(removeErr) {
						slog.Warn("failed to remove temp file", "path", tmpFile.Name(), "error", removeErr)
					}
				}()

				if err := createTarGz(tmpFile, workspace); err != nil {
					return fmt.Errorf("creating archive: %w", err)
				}

			case "pgdump":
				ext = "sql.gz"
				dbURL := os.Getenv("DATABASE_URL")
				if dbURL == "" {
					return fmt.Errorf("DATABASE_URL environment variable is required for pgdump mode")
				}

				slog.Info("running pg_dump")
				tmpFile, err = os.CreateTemp("", "clawmachine-backup-*.sql.gz")
				if err != nil {
					return fmt.Errorf("creating temp file: %w", err)
				}
				defer func() {
					if closeErr := tmpFile.Close(); closeErr != nil {
						slog.Warn("failed to close temp file", "path", tmpFile.Name(), "error", closeErr)
					}
					if removeErr := os.Remove(tmpFile.Name()); removeErr != nil && !os.IsNotExist(removeErr) {
						slog.Warn("failed to remove temp file", "path", tmpFile.Name(), "error", removeErr)
					}
				}()

				if err := runPgDump(dbURL, tmpFile); err != nil {
					return fmt.Errorf("pg_dump: %w", err)
				}

			default:
				return fmt.Errorf("unknown backup mode %q (use filesystem or pgdump)", mode)
			}

			// Upload timestamped backup
			timestamp := time.Now().UTC().Format("20060102-150405")
			key := filepath.Join(prefix, fmt.Sprintf("%s.%s", timestamp, ext))

			if _, err := tmpFile.Seek(0, 0); err != nil {
				return fmt.Errorf("seeking temp file: %w", err)
			}

			slog.Info("uploading backup", "bucket", bucket, "key", key)
			if err := uploadToS3(ctx, s3Client, bucket, key, tmpFile); err != nil {
				return fmt.Errorf("uploading backup: %w", err)
			}

			// Upload latest pointer (overwrite)
			latestKey := filepath.Join(prefix, fmt.Sprintf("latest.%s", ext))
			if _, err := tmpFile.Seek(0, 0); err != nil {
				return fmt.Errorf("seeking temp file: %w", err)
			}

			slog.Info("uploading latest pointer", "bucket", bucket, "key", latestKey)
			if err := uploadToS3(ctx, s3Client, bucket, latestKey, tmpFile); err != nil {
				return fmt.Errorf("uploading latest: %w", err)
			}

			slog.Info("backup complete", "key", key)
			return nil
		},
	}

	cmd.Flags().StringVar(&mode, "mode", "filesystem", "Backup mode: filesystem or pgdump")
	cmd.Flags().StringVar(&workspace, "workspace", "/workspace", "Workspace directory to backup (filesystem mode)")
	cmd.Flags().StringVar(&bucket, "bucket", "", "S3 bucket name (required)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "S3-compatible endpoint URL")
	cmd.Flags().StringVar(&region, "region", "us-east-1", "S3 region")
	cmd.Flags().StringVar(&prefix, "prefix", "", "S3 key prefix")
	if err := cmd.MarkFlagRequired("bucket"); err != nil {
		panic(fmt.Errorf("marking required flag bucket: %w", err))
	}

	return cmd
}

// runPgDump executes pg_dump and writes gzipped output to w.
func runPgDump(dbURL string, w *os.File) error {
	cmd := exec.Command("pg_dump", "--no-owner", "--no-acl", "--clean", "--if-exists", dbURL)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting pg_dump: %w", err)
	}

	gzw := gzip.NewWriter(w)
	if _, err := io.Copy(gzw, stdout); err != nil {
		waitErr := cmd.Wait()
		if waitErr != nil {
			return fmt.Errorf("compressing pg_dump output: %w (wait: %v)", err, waitErr)
		}
		return fmt.Errorf("compressing pg_dump output: %w", err)
	}
	if err := gzw.Close(); err != nil {
		waitErr := cmd.Wait()
		if waitErr != nil {
			return fmt.Errorf("closing gzip writer: %w (wait: %v)", err, waitErr)
		}
		return fmt.Errorf("closing gzip writer: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("pg_dump exited with error: %w", err)
	}
	return nil
}

func newS3Client(ctx context.Context, endpoint, region string) (*s3.Client, error) {
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}

	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if accessKey != "" && secretKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}

	s3Opts := []func(*s3.Options){}
	if endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		})
	}

	return s3.NewFromConfig(cfg, s3Opts...), nil
}

func uploadToS3(ctx context.Context, client *s3.Client, bucket, key string, body io.ReadSeeker) error {
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   body,
	})
	return err
}

func createTarGz(w io.Writer, srcDir string) error {
	gzw := gzip.NewWriter(w)
	tw := tar.NewWriter(gzw)
	walkErr := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		if _, err := io.Copy(tw, f); err != nil {
			if closeErr := f.Close(); closeErr != nil {
				slog.Warn("failed to close file during archive copy", "path", path, "error", closeErr)
			}
			return err
		}
		return f.Close()
	})
	closeTarErr := tw.Close()
	closeGzipErr := gzw.Close()
	if walkErr != nil {
		return walkErr
	}
	if closeTarErr != nil {
		return closeTarErr
	}
	if closeGzipErr != nil {
		return closeGzipErr
	}
	return nil
}
