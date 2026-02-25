//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// baseURL is the ClawMachine HTTP API base URL (--web mode).
// Used by API and lifecycle tests that exercise the JSON endpoints.
// TUI interaction is not tested here — the TUI is an interactive terminal app.
var baseURL string

func TestMain(m *testing.M) {
	baseURL = os.Getenv("CLAWMACHINE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// Check if the server is reachable; inform but don't abort.
	// Lifecycle tests that talk to K8s directly can still run without the server.
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		fmt.Fprintf(os.Stderr, "INFO: ClawMachine server not reachable at %s — API tests will self-skip\n", baseURL)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "WARN: /health returned %d\n", resp.StatusCode)
		}
	}

	os.Exit(m.Run())
}

// skipIfNoServer skips the calling test if the ClawMachine API server is not up.
func skipIfNoServer(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		t.Skipf("ClawMachine API server not reachable at %s (start with: clawmachine --web): %v", baseURL, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("ClawMachine /health returned %d", resp.StatusCode)
	}
}
