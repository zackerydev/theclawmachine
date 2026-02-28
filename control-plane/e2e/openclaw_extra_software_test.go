//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func waitForPodReadyBySelector(t *testing.T, namespace, labelSelector string, timeout time.Duration) string {
	t.Helper()
	clientset, _ := getK8sClients(t)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err == nil && len(pods.Items) > 0 {
			for _, pod := range pods.Items {
				for _, cond := range pod.Status.Conditions {
					if cond.Type == "Ready" && cond.Status == "True" {
						return pod.Name
					}
				}
			}
		}
		time.Sleep(3 * time.Second)
	}

	t.Fatalf("timed out waiting for ready pod with selector %q", labelSelector)
	return ""
}

func TestOpenClawExtraSoftwareInstallsClaude(t *testing.T) {
	skipIfNoServer(t)

	clientset, config := getK8sClients(t)
	const namespace = "claw-machine"
	const testBotName = "e2e-openclaw-mise"

	doDelete(t, baseURL+"/bots/"+testBotName)
	time.Sleep(2 * time.Second)

	t.Cleanup(func() {
		doDelete(t, baseURL+"/bots/"+testBotName)
	})

	body := map[string]any{
		"releaseName": testBotName,
		"botType":     "openclaw",
		"values": map[string]any{
			"persistence": map[string]any{
				"enabled": true,
				"size":    "1Gi",
			},
			"networkPolicy": map[string]any{
				"ingress": false,
				"egress":  true,
			},
			"extraSoftware": map[string]any{
				"toolVersions": "claude latest",
			},
		},
	}

	resp := doPost(t, baseURL+"/bots", body)
	if resp.StatusCode != 201 && resp.StatusCode != 200 && resp.StatusCode != 204 && resp.StatusCode != 202 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create bot failed: %d — %s", resp.StatusCode, string(b))
	}
	resp.Body.Close()

	selector := fmt.Sprintf("app.kubernetes.io/instance=%s", testBotName)
	waitForPodRunning(t, clientset, namespace, selector, 4*time.Minute)
	podName := waitForPodReadyBySelector(t, namespace, selector, 4*time.Minute)
	t.Logf("openclaw pod ready: %s", podName)

	stdout, stderr, err := execInPod(t, clientset, config, namespace, podName, []string{
		"sh", "-lc", "cat /root/.openclaw/workspace/.tool-versions",
	})
	if err != nil {
		t.Fatalf("read .tool-versions failed: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "claude latest") {
		t.Fatalf(".tool-versions missing expected entry; got:\n%s", stdout)
	}

	stdout, stderr, err = execInPod(t, clientset, config, namespace, podName, []string{
		"sh", "-lc", "cd /root/.openclaw/workspace && CLAUDE_BIN=\"$(mise which claude 2>/dev/null)\" && [ -n \"$CLAUDE_BIN\" ] && [ -x \"$CLAUDE_BIN\" ] && echo \"$CLAUDE_BIN\"",
	})
	if err != nil {
		t.Fatalf("claude binary lookup failed: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	t.Logf("claude binary found: %s", strings.TrimSpace(stdout))
}
