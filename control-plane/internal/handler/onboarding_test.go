package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zackerydev/clawmachine/control-plane/internal/botenv"
	"github.com/zackerydev/clawmachine/control-plane/internal/onboarding"
)

func newOnboardingHandler(t *testing.T) *OnboardingHandler {
	t.Helper()
	reg, err := botenv.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return NewOnboardingHandler(onboarding.NewEngine(reg))
}

func TestOnboardingHandler_Profile(t *testing.T) {
	h := newOnboardingHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/onboarding/profile?type=picoclaw", nil)
	rec := httptest.NewRecorder()
	h.Profile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var body struct {
		BotType string `json:"botType"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.BotType != "picoclaw" {
		t.Fatalf("botType=%q want picoclaw", body.BotType)
	}
}

func TestOnboardingHandler_Preview(t *testing.T) {
	h := newOnboardingHandler(t)

	payload := map[string]any{
		"botType": "openclaw",
		"answers": map[string]string{
			"authChoice":      "apiKey",
			"anthropicApiKey": "1p:anthropic-prod",
		},
		"values": map[string]any{
			"persistence": map[string]any{"enabled": true},
		},
	}
	buf, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/onboarding/preview", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Preview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var body struct {
		EnvSecrets []struct {
			EnvVar string `json:"envVar"`
		} `json:"envSecrets"`
		EffectiveValues map[string]any `json:"effectiveValues"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.EnvSecrets) == 0 {
		t.Fatalf("expected envSecrets in preview")
	}
	if _, ok := body.EffectiveValues["persistence"]; !ok {
		t.Fatalf("expected effectiveValues to include provided base values")
	}
}
