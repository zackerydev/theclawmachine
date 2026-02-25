package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// ModelsHandler proxies and caches the OpenRouter models API.
type ModelsHandler struct{}

func NewModelsHandler() *ModelsHandler {
	return &ModelsHandler{}
}

func (h *ModelsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var models []modelOption
	var err error

	botType := r.URL.Query().Get("botType")
	if botType == "openclaw" {
		authChoice := r.URL.Query().Get("authChoice")
		models, err = getOpenClawModels(authChoice)
	} else {
		models, err = h.getModels()
	}
	if err != nil {
		http.Error(w, "failed to fetch models: "+err.Error(), http.StatusBadGateway)
		return
	}

	data, err := json.Marshal(models)
	if err != nil {
		http.Error(w, "failed to encode models: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=900")
	if _, err := w.Write(data); err != nil {
		slog.Warn("models: failed writing response", "error", err)
	}
}

func (h *ModelsHandler) getModels() ([]modelOption, error) {
	slog.Debug("models: serving openrouter catalog")
	rawModels, err := getOpenRouterModelsCached()
	if err != nil {
		slog.Warn("models: fetch failed", "error", err)
		return nil, err
	}
	models := make([]modelOption, 0, len(rawModels))
	for _, m := range rawModels {
		if m.ID == "" {
			continue
		}
		models = append(models, modelOption{ID: m.ID, Name: m.Name})
	}
	slog.Info("models: cached", "count", len(models))
	return models, nil
}
