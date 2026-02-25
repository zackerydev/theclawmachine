//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// ════════════════════════════════════════════════════════════════════════════
// Connect / provider API tests — require ClawMachine running with --web flag
// ════════════════════════════════════════════════════════════════════════════

// TestInstallConnectMissingCredentials verifies that POSTing without credentials returns 400.
func TestInstallConnectMissingCredentials(t *testing.T) {
	skipIfNoServer(t)

	req, _ := http.NewRequest("POST", baseURL+"/settings/connect/install",
		strings.NewReader("token=tok&vaultName=vault"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("POST /settings/connect/install: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 400, got %d: %s", resp.StatusCode, body)
	}
}

// TestInstallConnectMissingToken verifies that POSTing without a token returns 400.
func TestInstallConnectMissingToken(t *testing.T) {
	skipIfNoServer(t)

	req, _ := http.NewRequest("POST", baseURL+"/settings/connect/install",
		strings.NewReader(`{"credentialsJson":"{\"version\":\"2\"}","vaultName":"vault"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("POST /settings/connect/install: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 400, got %d: %s", resp.StatusCode, body)
	}
}

// TestProviderMissingFields verifies that creating a secret store with incomplete data returns 400.
func TestProviderMissingFields(t *testing.T) {
	skipIfNoServer(t)

	req, _ := http.NewRequest("POST", baseURL+"/settings/provider",
		strings.NewReader("connectHost=https://connect.example.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("POST /settings/provider: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 400, got %d: %s", resp.StatusCode, body)
	}
}

// TestConnectStatusEndpoint verifies the Connect status endpoint returns JSON.
func TestConnectStatusEndpoint(t *testing.T) {
	skipIfNoServer(t)

	resp, err := httpClient.Get(baseURL + "/settings/status")
	if err != nil {
		t.Fatalf("GET /settings/status: %v", err)
	}
	defer resp.Body.Close()

	// 200 with JSON body expected
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200, got %d: %s", resp.StatusCode, body)
	}
}
