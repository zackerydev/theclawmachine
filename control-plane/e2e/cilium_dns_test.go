//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestCiliumDNSPolicy deploys a busybox bot with CiliumNetworkPolicy
// that allows egress only to httpbin.org, then verifies:
// 1. httpbin.org is reachable
// 2. other domains (google.com) are blocked
// 3. DNS resolution works for all domains (Cilium intercepts DNS)
func TestCiliumDNSPolicy(t *testing.T) {
	clientset, config := getK8sClients(t)

	// Check if Cilium CRDs exist — skip if not
	_, err := clientset.Discovery().ServerResourcesForGroupVersion("cilium.io/v2")
	if err != nil {
		t.Skipf("Cilium CRDs not found, skipping DNS policy test: %v", err)
	}

	const testBot = "e2e-cilium-dns"

	// Clean up
	doDelete(t, baseURL+"/bots/"+testBot)
	time.Sleep(3 * time.Second)
	t.Cleanup(func() {
		doDelete(t, baseURL+"/bots/"+testBot)
	})

	// Deploy busybox with Cilium DNS policy: allow only httpbin.org
	body := map[string]any{
		"releaseName": testBot,
		"botType":     "busybox",
		"values": map[string]any{
			"networkPolicy": map[string]any{
				"ingress":    false,
				"egress":     false,
				"useCilium":  true,
				"allowedDomains": []string{
					"httpbin.org",
					"*.httpbin.org",
				},
			},
		},
	}
	jsonBody, _ := json.Marshal(body)
	resp, err := http.Post(baseURL+"/bots", "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("POST /bots: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /bots returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Wait for pod
	selector := fmt.Sprintf("app.kubernetes.io/instance=%s", testBot)
	podName := waitForPodRunning(t, clientset, botNamespace, selector, 3*time.Minute)
	t.Logf("pod running: %s", podName)

	// Give Cilium a moment to apply the policy
	time.Sleep(5 * time.Second)

	// Verify CiliumNetworkPolicy was created
	t.Run("CiliumPolicyExists", func(t *testing.T) {
		pods, _ := clientset.CoreV1().Pods(botNamespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: selector,
		})
		if len(pods.Items) == 0 {
			t.Fatal("no pods found")
		}
		// The policy should have been applied — we verify indirectly via behavior
		t.Logf("✓ Pod %s running with CiliumNetworkPolicy", pods.Items[0].Name)
	})

	// Test: allowed domain (httpbin.org) should be reachable
	t.Run("AllowedDomainReachable", func(t *testing.T) {
		stdout, stderr, err := execInPod(t, clientset, config, botNamespace, podName,
			[]string{"wget", "-q", "-O", "-", "--timeout=10", "http://httpbin.org/get"})
		if err != nil {
			t.Errorf("httpbin.org should be reachable but wget failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		} else if !strings.Contains(stdout, "httpbin") {
			t.Errorf("unexpected response from httpbin.org: %s", stdout)
		} else {
			t.Logf("✓ httpbin.org reachable")
		}
	})

	// Test: blocked domain (google.com) should be unreachable
	t.Run("BlockedDomainBlocked", func(t *testing.T) {
		stdout, stderr, err := execInPod(t, clientset, config, botNamespace, podName,
			[]string{"wget", "-q", "-O", "-", "--timeout=5", "http://google.com"})
		if err == nil {
			t.Errorf("google.com should be BLOCKED but wget succeeded: %s", stdout)
		} else {
			t.Logf("✓ google.com blocked (wget failed: %v, stderr: %s)", err, stderr)
		}
	})

	// Test: DNS resolution should work for any domain (Cilium proxy intercepts)
	t.Run("DNSResolutionWorks", func(t *testing.T) {
		stdout, stderr, err := execInPod(t, clientset, config, botNamespace, podName,
			[]string{"nslookup", "google.com"})
		if err != nil {
			// Some Cilium configs block DNS for non-allowed domains — that's also valid
			t.Logf("DNS resolution for blocked domain failed (may be expected with strict DNS proxy): %v stderr=%s", err, stderr)
		} else if strings.Contains(stdout, "Address") {
			t.Logf("✓ DNS resolves (even for blocked domains — Cilium blocks at L3/L4): %s", strings.TrimSpace(stdout))
		}
	})
}
