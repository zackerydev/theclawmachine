//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// ════════════════════════════════════════════════════════════════════════════
// JSON API tests — require ClawMachine running with --web flag
// ════════════════════════════════════════════════════════════════════════════

func TestAPIHealthReturnsOK(t *testing.T) {
	skipIfNoServer(t)

	resp, err := httpClient.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "OK" {
		t.Errorf("expected body %q, got %q", "OK", string(body))
	}
}

func TestAPIGetBotsJSON(t *testing.T) {
	skipIfNoServer(t)

	resp, err := httpClient.Get(baseURL + "/bots")
	if err != nil {
		t.Fatalf("GET /bots: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON content-type, got %q", ct)
	}
}

func TestAPIPostBotsEmptyBody(t *testing.T) {
	skipIfNoServer(t)

	req, _ := http.NewRequest("POST", baseURL+"/bots", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("POST /bots: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 400 {
		t.Errorf("expected status >= 400 for empty body, got %d", resp.StatusCode)
	}
}

func TestAPIPostBotsInvalidJSON(t *testing.T) {
	skipIfNoServer(t)

	req, _ := http.NewRequest("POST", baseURL+"/bots", strings.NewReader("not json at all {{{"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("POST /bots: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 400 {
		t.Errorf("expected status >= 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestAPIGetNonexistentBot(t *testing.T) {
	skipIfNoServer(t)

	resp, err := httpClient.Get(baseURL + "/bots/nonexistent-bot-12345")
	if err != nil {
		t.Fatalf("GET /bots/nonexistent-bot-12345: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 400 {
		t.Errorf("expected status >= 400 for nonexistent bot, got %d", resp.StatusCode)
	}
}

func TestAPIDeleteNonexistentBot(t *testing.T) {
	skipIfNoServer(t)

	req, _ := http.NewRequest("DELETE", baseURL+"/bots/nonexistent-bot-12345", nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /bots/nonexistent-bot-12345: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 400 {
		t.Errorf("expected status >= 400 for nonexistent bot, got %d", resp.StatusCode)
	}
}

func TestAPIBotEnvReturnsJSON(t *testing.T) {
	skipIfNoServer(t)

	resp, err := httpClient.Get(baseURL + "/api/botenv")
	if err != nil {
		t.Fatalf("GET /api/botenv: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("GET /api/botenv = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON, got Content-Type %q", ct)
	}
}

func TestAPIModelsProxy(t *testing.T) {
	skipIfNoServer(t)

	resp, err := httpClient.Get(baseURL + "/api/models")
	if err != nil {
		t.Fatalf("GET /api/models: %v", err)
	}
	defer resp.Body.Close()

	// 200 (cached) or 502 (upstream unreachable in CI) are both acceptable
	if resp.StatusCode != 200 && resp.StatusCode != 502 {
		t.Errorf("GET /api/models = %d, want 200 or 502", resp.StatusCode)
	}
}
