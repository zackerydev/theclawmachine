package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zackerydev/clawmachine/control-plane/internal/botenv"
	"github.com/zackerydev/clawmachine/control-plane/internal/service"
)

func seedOpenRouterCacheForTest(models []openRouterModel) func() {
	openRouterModelCache.mu.Lock()
	prevModels := openRouterModelCache.models
	prevFetched := openRouterModelCache.fetched
	openRouterModelCache.models = models
	openRouterModelCache.fetched = time.Now()
	openRouterModelCache.mu.Unlock()
	return func() {
		openRouterModelCache.mu.Lock()
		openRouterModelCache.models = prevModels
		openRouterModelCache.fetched = prevFetched
		openRouterModelCache.mu.Unlock()
	}
}

func TestValidateOpenClawDefaultModel_CanonicalizesLegacyAlias(t *testing.T) {
	got, err := validateOpenClawDefaultModel("apiKey", "anthropic/claude-sonnet-4.6")
	if err != nil {
		t.Fatalf("validateOpenClawDefaultModel returned error: %v", err)
	}
	if got != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("canonical model = %q, want anthropic/claude-sonnet-4-6", got)
	}
}

func TestValidateOpenClawDefaultModel_RejectsInvalidModel(t *testing.T) {
	_, err := validateOpenClawDefaultModel("apiKey", "anthropic/not-real")
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	var modelErr *openClawModelValidationError
	if !errors.As(err, &modelErr) {
		t.Fatalf("error type = %T, want *openClawModelValidationError", err)
	}
}

func TestModelsHandler_ServeHTTP_OpenClawUsesProviderCatalog(t *testing.T) {
	h := NewModelsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/models?botType=openclaw&authChoice=apiKey", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var payload []modelOption
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected non-empty payload")
	}
	if payload[0].Source == "" {
		t.Fatalf("expected source for openclaw models, got empty in %+v", payload[0])
	}
}

func TestModelsHandler_ServeHTTP_DefaultUsesOpenRouterCatalog(t *testing.T) {
	restore := seedOpenRouterCacheForTest([]openRouterModel{
		{ID: "anthropic/claude-sonnet-4-6", Name: "Claude Sonnet 4.6"},
	})
	defer restore()

	h := NewModelsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var payload []modelOption
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("len(payload) = %d, want 1", len(payload))
	}
	if payload[0].ID != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("id = %q, want anthropic/claude-sonnet-4-6", payload[0].ID)
	}
}

func TestCompileConfigFields_OpenClawCanonicalizesDefaultModel(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	h := NewHelmHandler(&mockHelm{}, &mockTemplate{}, nil, nil, nil, reg, false)

	compiled, err := h.compileConfigFields(context.Background(), service.BotTypeOpenClaw, map[string]string{
		"authChoice":   "apiKey",
		"defaultModel": "anthropic/claude-sonnet-4.6",
	})
	if err != nil {
		t.Fatalf("compileConfigFields returned error: %v", err)
	}
	agent, ok := compiled.ValuesPatch["agent"].(map[string]any)
	if !ok {
		t.Fatalf("agent values missing: %#v", compiled.ValuesPatch)
	}
	if agent["defaultModel"] != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("agent.defaultModel = %v, want anthropic/claude-sonnet-4-6", agent["defaultModel"])
	}
}

func TestCompileConfigFields_OpenClawRejectsInvalidDefaultModel(t *testing.T) {
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	h := NewHelmHandler(&mockHelm{}, &mockTemplate{}, nil, nil, nil, reg, false)

	_, err = h.compileConfigFields(context.Background(), service.BotTypeOpenClaw, map[string]string{
		"authChoice":   "apiKey",
		"defaultModel": "anthropic/not-real",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var modelErr *openClawModelValidationError
	if !errors.As(err, &modelErr) {
		t.Fatalf("error type = %T, want *openClawModelValidationError", err)
	}
}
