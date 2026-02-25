package routes

import (
	"log/slog"
	"net/http"

	"github.com/zackerydev/clawmachine/control-plane/internal/handler"
	"github.com/zackerydev/clawmachine/control-plane/internal/middleware"
)

type Handlers struct {
	Helm       *handler.HelmHandler
	Secrets    *handler.SecretsHandler
	Network    *handler.NetworkHandler
	Backup     *handler.BackupHandler
	Onboarding *handler.OnboardingHandler
}

func Setup(mux *http.ServeMux, handlers *Handlers) {
	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			slog.Warn("health: failed to write response", "error", err)
		}
	})

	// Static files (no directory listing)
	mux.Handle("GET /static/", http.StripPrefix("/static/", middleware.NoListingFileServer("./static")))

	// Pages
	mux.HandleFunc("GET /{$}", handlers.Helm.ListPage)
	mux.HandleFunc("GET /bots/new", handlers.Helm.NewPage)
	mux.HandleFunc("POST /bots/new/infra", handlers.Helm.NewInfraPage)
	mux.HandleFunc("POST /bots/new/config", handlers.Helm.NewConfigPage)
	mux.HandleFunc("GET /bots/{name}/page", handlers.Helm.DetailPage)

	// Bot management API
	mux.HandleFunc("GET /bots", handlers.Helm.List)
	mux.HandleFunc("POST /bots", handlers.Helm.Install)
	mux.HandleFunc("GET /bots/{name}", handlers.Helm.Status)
	mux.HandleFunc("PUT /bots/{name}", handlers.Helm.Upgrade)
	mux.HandleFunc("DELETE /bots/{name}", handlers.Helm.Uninstall)
	mux.HandleFunc("GET /bots/{name}/logs", handlers.Helm.Logs)
	mux.HandleFunc("POST /bots/{name}/cli", handlers.Helm.ExecCLI)
	mux.HandleFunc("POST /bots/{name}/restart", handlers.Helm.Restart)
	mux.HandleFunc("PUT /bots/{name}/config", handlers.Helm.UpdateConfig)
	mux.HandleFunc("GET /bots/{name}/network", handlers.Network.Flows)
	mux.HandleFunc("GET /bots/{name}/backup", handlers.Backup.BackupConfigPage)
	mux.HandleFunc("POST /bots/{name}/backup", handlers.Backup.SaveBackupConfig)

	// Settings hub
	mux.HandleFunc("GET /settings", handlers.Secrets.SettingsPage)

	// Secret Providers
	mux.HandleFunc("GET /settings/providers", handlers.Secrets.ProvidersPage)
	mux.HandleFunc("GET /settings/status", handlers.Secrets.SettingsStatusPartial)
	mux.HandleFunc("POST /settings/provider", handlers.Secrets.CreateSecretStore)
	mux.HandleFunc("DELETE /settings/provider", handlers.Secrets.DeleteSecretStore)
	mux.HandleFunc("POST /settings/connect/install", handlers.Secrets.InstallConnect)
	mux.HandleFunc("DELETE /settings/connect", handlers.Secrets.UninstallConnect)

	// Bot env var registry
	mux.Handle("GET /api/botenv", handler.NewBotEnvHandler())

	// OpenRouter models proxy (cached)
	mux.Handle("GET /api/models", handler.NewModelsHandler())

	// Canonical onboarding profiles and compile previews
	if handlers.Onboarding != nil {
		mux.HandleFunc("GET /api/onboarding/profile", handlers.Onboarding.Profile)
		mux.HandleFunc("POST /api/onboarding/preview", handlers.Onboarding.Preview)
	}

	// Secrets
	mux.HandleFunc("GET /secrets", handlers.Secrets.SecretsListPage)
	mux.HandleFunc("GET /secrets/available", handlers.Secrets.AvailableSecrets)
	mux.HandleFunc("GET /secrets/status", handlers.Secrets.SecretsStatusPartial)
	mux.HandleFunc("GET /secrets/new", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/secrets", http.StatusSeeOther)
	})
	mux.HandleFunc("POST /secrets", handlers.Secrets.CreateExternalSecret)
	mux.HandleFunc("DELETE /secrets/{name}", handlers.Secrets.DeleteExternalSecret)

}
