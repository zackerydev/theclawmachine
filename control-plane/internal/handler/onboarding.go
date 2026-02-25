package handler

import (
	"encoding/json"
	"net/http"

	"github.com/zackerydev/clawmachine/control-plane/internal/onboarding"
	"github.com/zackerydev/clawmachine/control-plane/internal/service"
)

// OnboardingHandler serves canonical onboarding profiles and compile previews.
type OnboardingHandler struct {
	engine *onboarding.Engine
}

func NewOnboardingHandler(engine *onboarding.Engine) *OnboardingHandler {
	return &OnboardingHandler{engine: engine}
}

// Profile returns the onboarding profile for a bot type.
// GET /api/onboarding/profile?type=picoclaw
func (h *OnboardingHandler) Profile(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		http.Error(w, "onboarding engine not configured", http.StatusServiceUnavailable)
		return
	}

	botType := service.BotType(r.URL.Query().Get("type"))
	if botType == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			BotTypes []service.BotType `json:"botTypes"`
		}{BotTypes: h.engine.SupportedBotTypes()})
		return
	}

	profile, err := h.engine.Profile(botType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(profile)
}

// Preview compiles onboarding answers into deterministic chart/config output.
// POST /api/onboarding/preview
func (h *OnboardingHandler) Preview(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		http.Error(w, "onboarding engine not configured", http.StatusServiceUnavailable)
		return
	}

	var body struct {
		BotType service.BotType   `json:"botType"`
		Answers map[string]string `json:"answers"`
		Values  map[string]any    `json:"values,omitempty"`
	}
	if err := parseRequest(r, &body); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.BotType == "" {
		http.Error(w, "botType is required", http.StatusBadRequest)
		return
	}

	compiled, err := h.engine.Compile(body.BotType, body.Answers)
	if err != nil {
		http.Error(w, "compile failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	effective := map[string]any{}
	if body.Values != nil {
		onboarding.DeepMergeValues(effective, body.Values)
	}
	if compiled.ValuesPatch != nil {
		onboarding.DeepMergeValues(effective, compiled.ValuesPatch)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		*onboarding.CompileResult
		EffectiveValues map[string]any `json:"effectiveValues,omitempty"`
	}{
		CompileResult:   compiled,
		EffectiveValues: effective,
	})
}
