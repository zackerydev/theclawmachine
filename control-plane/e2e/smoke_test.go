//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// ════════════════════════════════════════════════════════════════════════════
// API health (requires --web server)
// ════════════════════════════════════════════════════════════════════════════

func TestHealthEndpoint(t *testing.T) {
	skipIfNoServer(t)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /health: expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "OK" {
		t.Errorf("GET /health: expected body %q, got %q", "OK", string(body))
	}
}

// ════════════════════════════════════════════════════════════════════════════
// CLI smoke tests (binary must be in PATH or $CLAWMACHINE_BIN)
// ════════════════════════════════════════════════════════════════════════════

// clawmachineBin returns the clawmachine binary path for CLI tests.
// Defaults to "clawmachine" in PATH; override with $CLAWMACHINE_BIN.
func clawmachineBin(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("clawmachine")
	if err != nil {
		t.Skipf("clawmachine binary not found in PATH (set CLAWMACHINE_BIN or build first): %v", err)
	}
	return bin
}

func TestCLI_Version(t *testing.T) {
	bin := clawmachineBin(t)
	out, err := exec.Command(bin, "version").Output()
	if err != nil {
		t.Fatalf("clawmachine version: %v", err)
	}
	if !strings.Contains(string(out), "clawmachine") {
		t.Errorf("version output should contain 'clawmachine', got: %q", string(out))
	}
}

func TestCLI_HelpNoError(t *testing.T) {
	bin := clawmachineBin(t)
	cmd := exec.Command(bin, "--help")
	out, err := cmd.CombinedOutput()
	// --help exits 0
	if err != nil {
		t.Fatalf("clawmachine --help: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "ClawMachine") {
		t.Errorf("help output should mention ClawMachine, got: %q", string(out))
	}
	if !strings.Contains(string(out), "upgrade") {
		t.Errorf("help output should contain 'upgrade', got: %q", string(out))
	}
}

func TestCLI_ServeCommandVisible(t *testing.T) {
	bin := clawmachineBin(t)
	out, _ := exec.Command(bin, "--help").CombinedOutput()
	if !strings.Contains(string(out), "serve") {
		t.Error("'serve' command should appear in help output")
	}
}

func TestCLI_TUICommandRemoved(t *testing.T) {
	bin := clawmachineBin(t)
	out, _ := exec.Command(bin, "--help").CombinedOutput()
	if strings.Contains(string(out), "tui") {
		t.Error("'tui' command should not appear in help output")
	}
}

func TestCLI_CompletionSubcommands(t *testing.T) {
	bin := clawmachineBin(t)
	out, err := exec.Command(bin, "completion", "bash").Output()
	if err != nil {
		t.Fatalf("clawmachine completion bash: %v", err)
	}
	if len(out) == 0 {
		t.Error("completion bash should produce output")
	}
}
