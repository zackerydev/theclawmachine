package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/zackerydev/clawmachine/control-plane/internal/botenv"
)

// BotEnvHandler serves the env var registry as JSON for the bot creation UI.
type BotEnvHandler struct{}

func NewBotEnvHandler() *BotEnvHandler {
	return &BotEnvHandler{}
}

// ServeHTTP returns the env var registry for a given bot type.
// GET /api/botenv?type=picoclaw
func (h *BotEnvHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	botType := botenv.BotType(r.URL.Query().Get("type"))

	vars := botenv.GetEnvVars(botType)
	if vars == nil {
		// Return all bot types with their vars
		result := make(map[string][]botenv.EnvVar)
		for _, bt := range botenv.AllBotTypes() {
			result[string(bt)] = botenv.GetEnvVars(bt)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			slog.Warn("botenv: failed to encode full registry", "error", err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(vars); err != nil {
		slog.Warn("botenv: failed to encode env vars", "type", botType, "error", err)
	}
}
