package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	routes "github.com/zackerydev/clawmachine/control-plane/internal"
	"github.com/zackerydev/clawmachine/control-plane/internal/botenv"
	"github.com/zackerydev/clawmachine/control-plane/internal/handler"
	"github.com/zackerydev/clawmachine/control-plane/internal/middleware"
	"github.com/zackerydev/clawmachine/control-plane/internal/onboarding"
	"github.com/zackerydev/clawmachine/control-plane/internal/service"
)

// setupLogging configures the global slog logger.
// In dev mode: text handler with DEBUG level.
// In production: JSON handler with INFO level (override via LOG_LEVEL env var).
func setupLogging(dev bool) {
	level := slog.LevelInfo
	if dev {
		level = slog.LevelDebug
	}
	if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		switch strings.ToLower(lvl) {
		case "debug":
			level = slog.LevelDebug
		case "info":
			level = slog.LevelInfo
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
	}

	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if dev {
		h = slog.NewTextHandler(os.Stderr, opts)
	} else {
		h = slog.NewJSONHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(h))
	slog.Info("logging configured", "level", level.String(), "format", map[bool]string{true: "text", false: "json"}[dev])
}

func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the ClawMachine web server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd)
		},
	}
	cmd.Flags().Bool("dev", false, "Enable dev mode (template reload, imagePullPolicy=Never)")
	return cmd
}

func runServe(cmd *cobra.Command) error {
	kubeContext, _ := cmd.Flags().GetString("context")
	dev, _ := cmd.Flags().GetBool("dev")
	port := getenv("PORT", "8080")

	// Configure structured logging
	setupLogging(dev)

	printLogo()
	styledPrintf(accentStyle, "Starting ClawMachine server on port %s...", port)

	k8s, err := service.NewKubernetesService(kubeContext)
	if err != nil {
		return err
	}
	slog.Info("connected to kubernetes", "context", k8s.GetCurrentContext())

	mux := http.NewServeMux()

	tmpl, err := service.NewTemplateService(dev)
	if err != nil {
		return err
	}
	helmSvc := service.NewHelmService(k8s.GetKubeConfigPath(), k8s.GetCurrentContext(), k8s.InCluster(), dev, k8s.Clientset())
	secretsSvc := service.NewSecretsService(k8s.Clientset(), k8s.DynamicClient())
	connectSvc := service.NewConnectService(k8s.Clientset(), k8s.GetKubeConfigPath(), k8s.GetCurrentContext(), k8s.InCluster())
	botSecretsSvc := service.NewBotSecretsService(k8s)

	botReg, err := botenv.NewRegistry()
	if err != nil {
		return err
	}

	backupSvc := service.NewBackupService(helmSvc)
	onboardingEngine := onboarding.NewEngine(botReg)

	helmHandler := handler.NewHelmHandlerWithVersion(helmSvc, tmpl, botSecretsSvc, secretsSvc, k8s, botReg, dev, version)

	routes.Setup(mux, &routes.Handlers{
		Helm:       helmHandler,
		Secrets:    handler.NewSecretsHandler(secretsSvc, connectSvc, tmpl),
		Network:    handler.NewNetworkHandler(k8s, tmpl),
		Backup:     handler.NewBackupHandler(backupSvc, helmSvc, tmpl, k8s, secretsSvc),
		Onboarding: handler.NewOnboardingHandler(onboardingEngine),
	})

	// Apply middleware chain (order: outermost runs first)
	var handler http.Handler = mux
	handler = middleware.LimitBody(handler)
	handler = middleware.SecurityHeaders(handler)
	handler = middleware.RequestLogger(handler)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGTERM/SIGINT (important for Kubernetes)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-ctx.Done()
		slog.Info("shutting down gracefully...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown error", "error", err)
		}
	}()

	styledPrintf(successStyle, "ClawMachine is running at http://localhost:%s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
