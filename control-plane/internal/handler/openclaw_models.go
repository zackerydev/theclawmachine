package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

type modelOption struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source,omitempty"`
}

type openClawModelValidationError struct {
	msg string
}

func (e *openClawModelValidationError) Error() string {
	return e.msg
}

type openRouterModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var (
	openRouterModelCache = struct {
		mu      sync.RWMutex
		models  []openRouterModel
		fetched time.Time
		ttl     time.Duration
		client  *http.Client
	}{
		ttl:    15 * time.Minute,
		client: &http.Client{Timeout: 10 * time.Second},
	}
)

var openClawAnthropicModels = []modelOption{
	{ID: "anthropic/claude-opus-4-6", Name: "Claude Opus 4.6", Source: "openclaw-static"},
	{ID: "anthropic/claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Source: "openclaw-static"},
	{ID: "anthropic/claude-sonnet-4-5-20250929", Name: "Claude Sonnet 4.5 (20250929)", Source: "openclaw-static"},
	{ID: "anthropic/claude-opus-4-5", Name: "Claude Opus 4.5", Source: "openclaw-static"},
	{ID: "anthropic/claude-sonnet-4-5", Name: "Claude Sonnet 4.5", Source: "openclaw-static"},
	{ID: "anthropic/claude-haiku-4-5", Name: "Claude Haiku 4.5", Source: "openclaw-static"},
}

var openClawOpenAIModels = []modelOption{
	{ID: "openai/gpt-5.2", Name: "GPT-5.2", Source: "openclaw-static"},
	{ID: "openai/gpt-5-mini", Name: "GPT-5 Mini", Source: "openclaw-static"},
	{ID: "openai/gpt-5", Name: "GPT-5", Source: "openclaw-static"},
	{ID: "openai/gpt-4o", Name: "GPT-4o", Source: "openclaw-static"},
}

var openClawGeminiModels = []modelOption{
	{ID: "google/gemini-3-pro-preview", Name: "Gemini 3 Pro Preview", Source: "openclaw-static"},
	{ID: "google/gemini-3-flash-preview", Name: "Gemini 3 Flash Preview", Source: "openclaw-static"},
	{ID: "google/gemini-2.5-pro", Name: "Gemini 2.5 Pro", Source: "openclaw-static"},
	{ID: "google/gemini-2.5-flash", Name: "Gemini 2.5 Flash", Source: "openclaw-static"},
}

var openClawModelAliases = map[string]string{
	"claude-sonnet-4.6":                    "anthropic/claude-sonnet-4-6",
	"anthropic/claude-sonnet-4.6":          "anthropic/claude-sonnet-4-6",
	"anthropic/sonnet-4.6":                 "anthropic/claude-sonnet-4-6",
	"sonnet-4.6":                           "anthropic/claude-sonnet-4-6",
	"claude-opus-4.6":                      "anthropic/claude-opus-4-6",
	"anthropic/claude-opus-4.6":            "anthropic/claude-opus-4-6",
	"anthropic/opus-4.6":                   "anthropic/claude-opus-4-6",
	"opus-4.6":                             "anthropic/claude-opus-4-6",
	"anthropic/claude-sonnet-4.5":          "anthropic/claude-sonnet-4-5",
	"anthropic/claude-sonnet-4.5-20250929": "anthropic/claude-sonnet-4-5-20250929",
	"anthropic/sonnet-4.5":                 "anthropic/claude-sonnet-4-5",
	"anthropic/claude-opus-4.5":            "anthropic/claude-opus-4-5",
	"anthropic/opus-4.5":                   "anthropic/claude-opus-4-5",
	"anthropic/claude-haiku-4.5":           "anthropic/claude-haiku-4-5",
	"anthropic/haiku-4.5":                  "anthropic/claude-haiku-4-5",
}

func normalizeOpenClawAuthChoice(raw string) string {
	choice := strings.TrimSpace(raw)
	switch choice {
	case "", "apiKey":
		return "apiKey"
	case "openai":
		return "openai-api-key"
	case "openrouter":
		return "openrouter-api-key"
	default:
		return choice
	}
}

func canonicalizeOpenClawModel(raw string) (canonical string, changed bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	lower := strings.ToLower(trimmed)
	if mapped, ok := openClawModelAliases[lower]; ok {
		return mapped, mapped != trimmed
	}
	return trimmed, false
}

func getOpenRouterModelsCached() ([]openRouterModel, error) {
	openRouterModelCache.mu.RLock()
	if openRouterModelCache.models != nil && time.Since(openRouterModelCache.fetched) < openRouterModelCache.ttl {
		defer openRouterModelCache.mu.RUnlock()
		return slices.Clone(openRouterModelCache.models), nil
	}
	openRouterModelCache.mu.RUnlock()

	openRouterModelCache.mu.Lock()
	defer openRouterModelCache.mu.Unlock()

	if openRouterModelCache.models != nil && time.Since(openRouterModelCache.fetched) < openRouterModelCache.ttl {
		return slices.Clone(openRouterModelCache.models), nil
	}

	resp, err := openRouterModelCache.client.Get("https://openrouter.ai/api/v1/models")
	if err != nil {
		if openRouterModelCache.models != nil {
			return slices.Clone(openRouterModelCache.models), nil
		}
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Warn("openrouter: failed to close response body", "error", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if openRouterModelCache.models != nil {
			return slices.Clone(openRouterModelCache.models), nil
		}
		return nil, err
	}

	var raw struct {
		Data []openRouterModel `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		if openRouterModelCache.models != nil {
			return slices.Clone(openRouterModelCache.models), nil
		}
		return nil, err
	}

	openRouterModelCache.models = slices.Clone(raw.Data)
	openRouterModelCache.fetched = time.Now()
	return slices.Clone(openRouterModelCache.models), nil
}

func getOpenClawModels(authChoice string) ([]modelOption, error) {
	switch normalizeOpenClawAuthChoice(authChoice) {
	case "apiKey":
		return slices.Clone(openClawAnthropicModels), nil
	case "openai-api-key":
		return slices.Clone(openClawOpenAIModels), nil
	case "gemini-api-key":
		return slices.Clone(openClawGeminiModels), nil
	case "openrouter-api-key":
		rawModels, err := getOpenRouterModelsCached()
		if err != nil {
			return nil, err
		}
		out := make([]modelOption, 0, len(rawModels))
		for _, m := range rawModels {
			if m.ID == "" {
				continue
			}
			out = append(out, modelOption{
				ID:     "openrouter/" + m.ID,
				Name:   m.Name,
				Source: "openrouter-proxy",
			})
		}
		return out, nil
	default:
		return slices.Clone(openClawAnthropicModels), nil
	}
}

func validateOpenClawDefaultModel(authChoice, modelRaw string) (string, error) {
	trimmed := strings.TrimSpace(modelRaw)
	if trimmed == "" {
		return "", nil
	}

	canonical, canonicalized := canonicalizeOpenClawModel(trimmed)
	allowedModels, err := getOpenClawModels(authChoice)
	if err != nil {
		return "", fmt.Errorf("unable to load model catalog for validation: %w", err)
	}

	allowed := make(map[string]struct{}, len(allowedModels))
	for _, m := range allowedModels {
		allowed[m.ID] = struct{}{}
	}
	if _, ok := allowed[canonical]; ok {
		return canonical, nil
	}

	if canonicalized {
		return "", &openClawModelValidationError{
			msg: fmt.Sprintf("invalid model %q for provider %q. Did you mean %q?", trimmed, normalizeOpenClawAuthChoice(authChoice), canonical),
		}
	}
	return "", &openClawModelValidationError{
		msg: fmt.Sprintf("invalid model %q for provider %q. Select a model from the dropdown list", trimmed, normalizeOpenClawAuthChoice(authChoice)),
	}
}
