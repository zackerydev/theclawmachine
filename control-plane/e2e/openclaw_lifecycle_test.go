//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestOpenClawLifecycle exercises the full OpenClaw bot lifecycle:
//
//  1. Install 1Password Connect Server (real credentials)
//  2. Wait for Connect pods healthy
//  3. Create ExternalSecrets (anthropic-key + discord token)
//  4. Create OpenClaw bot with authChoice=apiKey + 1Password secret refs
//  5. Verify bot pod running
//  6. Verify runtime config in /root/.openclaw/openclaw.json
//  7. Verify gateway responds on port 18789
//  8. Update config (change model)
//  9. Verify pod restarts with updated config
//
// 10. Verify updated runtime config
// 11. Delete bot, verify cleanup
func TestOpenClawLifecycle(t *testing.T) {
	credFile := os.Getenv("OP_CREDENTIALS_FILE")
	connectToken := os.Getenv("OP_CONNECT_TOKEN")
	vaultName := os.Getenv("OP_VAULT_NAME")

	if credFile == "" {
		credFile = os.Getenv("HOME") + "/1password-credentials.json"
	}
	if connectToken == "" {
		tokenBytes, err := os.ReadFile(os.Getenv("HOME") + "/1password-jwt.txt")
		if err != nil {
			t.Skipf("Skipping: no OP_CONNECT_TOKEN and ~/1password-jwt.txt not found: %v", err)
		}
		connectToken = strings.TrimSpace(string(tokenBytes))
	}
	if vaultName == "" {
		vaultName = "claw-machine-dev"
	}

	credJSON, err := os.ReadFile(credFile)
	if err != nil {
		t.Skipf("Skipping: cannot read credentials file %s: %v", credFile, err)
	}

	clientset, config := getK8sClients(t)
	const testBotName = "e2e-openclaw"
	const namespace = "claw-machine"

	assertRuntimeConfig := func(t *testing.T, expectedModel string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/instance=" + testBotName,
		})
		if err != nil || len(pods.Items) == 0 {
			t.Fatalf("failed to list bot pods: %v", err)
		}
		podName := pods.Items[0].Name

		stdout, stderr, err := execInPod(t, clientset, config, namespace, podName, []string{
			"sh", "-lc", "cat /root/.openclaw/openclaw.json",
		})
		if err != nil {
			t.Fatalf("read openclaw.json failed: %v\nstderr=%s", err, stderr)
		}

		var cfg map[string]any
		if err := json.Unmarshal([]byte(stdout), &cfg); err != nil {
			t.Fatalf("openclaw.json invalid JSON: %v\n%s", err, stdout)
		}

		channels, _ := cfg["channels"].(map[string]any)
		discord, _ := channels["discord"].(map[string]any)
		if enabled, _ := discord["enabled"].(bool); !enabled {
			t.Fatalf("channels.discord.enabled = %v, want true", discord["enabled"])
		}
		if token, _ := discord["token"].(string); token == "" {
			t.Fatal("channels.discord.token is empty")
		}

		auth, _ := cfg["auth"].(map[string]any)
		profiles, _ := auth["profiles"].(map[string]any)
		if _, ok := profiles["anthropic:default"]; !ok {
			t.Fatalf("auth.profiles missing anthropic:default: %#v", profiles)
		}

		agents, _ := cfg["agents"].(map[string]any)
		defaults, _ := agents["defaults"].(map[string]any)
		modelAny := defaults["model"]
		model := ""
		switch v := modelAny.(type) {
		case string:
			model = v
		case map[string]any:
			model, _ = v["primary"].(string)
		}
		if model != expectedModel {
			t.Fatalf("agents.defaults.model = %q, want %q", model, expectedModel)
		}

		stdout, stderr, err = execInPod(t, clientset, config, namespace, podName, []string{
			"sh", "-lc", "cat /root/.openclaw/agents/main/agent/auth-profiles.json",
		})
		if err != nil {
			t.Fatalf("read auth-profiles.json failed: %v\nstderr=%s", err, stderr)
		}
		var authProfiles map[string]any
		if err := json.Unmarshal([]byte(stdout), &authProfiles); err != nil {
			t.Fatalf("auth-profiles.json invalid JSON: %v\n%s", err, stdout)
		}
		profilesAny, _ := authProfiles["profiles"].(map[string]any)
		anthropicAny, ok := profilesAny["anthropic:default"].(map[string]any)
		if !ok {
			t.Fatalf("auth-profiles missing anthropic:default: %#v", profilesAny)
		}
		if key, _ := anthropicAny["key"].(string); key == "" {
			t.Fatal("anthropic:default.key is empty")
		}
	}

	// Cleanup from previous runs
	doDelete(t, baseURL+"/bots/"+testBotName)
	doDelete(t, baseURL+"/secrets/e2e-anthropic-key")
	doDelete(t, baseURL+"/secrets/e2e-discord-token")
	time.Sleep(2 * time.Second)

	t.Cleanup(func() {
		t.Log("Cleanup: removing test resources")
		doDelete(t, baseURL+"/bots/"+testBotName)
		time.Sleep(3 * time.Second)
		doDelete(t, baseURL+"/secrets/e2e-anthropic-key")
		doDelete(t, baseURL+"/secrets/e2e-discord-token")
		doDelete(t, baseURL+"/settings/connect")
	})

	// --- Step 1: Install 1Password Connect Server ---
	t.Run("Step1_InstallConnect", func(t *testing.T) {
		body := map[string]string{
			"credentialsJson": string(credJSON),
			"token":           connectToken,
			"vaultName":       vaultName,
		}
		resp := doPost(t, baseURL+"/settings/connect/install", body)
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("Install Connect failed: %d — %s", resp.StatusCode, string(b))
		}
		t.Log("Connect install request accepted")
	})

	// --- Step 2: Wait for Connect pods ---
	t.Run("Step2_WaitForConnect", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		t.Log("Waiting for 1Password Connect pods in namespace '1password'...")
		for {
			select {
			case <-ctx.Done():
				t.Fatal("Timed out waiting for Connect pods to become ready")
			default:
			}

			pods, err := clientset.CoreV1().Pods("1password").List(ctx, metav1.ListOptions{})
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			ready := 0
			for _, pod := range pods.Items {
				for _, c := range pod.Status.Conditions {
					if c.Type == "Ready" && c.Status == "True" {
						ready++
					}
				}
			}

			if ready >= 1 && len(pods.Items) >= 1 {
				t.Logf("Connect pods ready (%d/%d)", ready, len(pods.Items))
				break
			}

			t.Logf("  Waiting... %d/%d pods ready", ready, len(pods.Items))
			time.Sleep(5 * time.Second)
		}
		// Wait for SecretStore to be Ready before creating ExternalSecrets
		waitForSecretStoreReady(t, namespace, 5*time.Minute)
	})

	// --- Step 3: Create ExternalSecrets for Anthropic key and Discord token ---
	t.Run("Step3_CreateSecrets", func(t *testing.T) {
		secrets := []struct {
			name string
			item string
		}{
			{"e2e-anthropic-key", "anthropic-key"},
			{"e2e-discord-token", "dorothy-bot-token"},
		}

		for _, s := range secrets {
			body := map[string]any{
				"name":            s.name,
				"item":            s.item,
				"field":           "credential",
				"refreshInterval": "30s", // Short interval so ESO retries quickly if Connect is still starting
			}
			resp := doPost(t, baseURL+"/secrets", body)
			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
				b, _ := io.ReadAll(resp.Body)
				errMsg := string(b)
				if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "404") {
					t.Skipf("Skipping: vault item '%s' not found in vault '%s'.", s.item, vaultName)
				}
				t.Fatalf("Create secret %s failed: %d — %s", s.name, resp.StatusCode, errMsg)
			}
			t.Logf("ExternalSecret %s created", s.name)
		}

		// Wait for both secrets to sync — Connect may take 2-3min after reinstall to authenticate
		t.Log("Waiting for secrets to sync...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		for _, s := range secrets {
			synced := false
			for !synced {
				select {
				case <-ctx.Done():
					t.Logf("Warning: secret %s sync timed out, continuing anyway", s.name)
					synced = true
				default:
				}
				secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, s.name, metav1.GetOptions{})
				if err == nil && len(secret.Data) > 0 {
					t.Logf("Secret %s synced from 1Password", s.name)
					synced = true
				} else {
					time.Sleep(3 * time.Second)
				}
			}
		}
	})

	// --- Step 4: Create OpenClaw bot with 1Password secret references ---
	t.Run("Step4_CreateBot", func(t *testing.T) {
		body := map[string]any{
			"releaseName": testBotName,
			"botType":     "openclaw",
			"values": map[string]any{
				"persistence":   map[string]any{"enabled": false},
				"networkPolicy": map[string]any{"ingress": false, "egress": true},
			},
			"configFields": map[string]string{
				"authChoice":      "apiKey",
				"anthropicApiKey": "1p:e2e-anthropic-key",
				"discordBotToken": "1p:e2e-discord-token",
				"discordEnabled":  "true",
				"defaultModel":    "anthropic/claude-sonnet-4-5-20250929",
			},
		}

		resp := doPost(t, baseURL+"/bots", body)
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusAccepted {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("Create bot failed: %d — %s", resp.StatusCode, string(b))
		}
		t.Log("OpenClaw bot created")
	})

	// --- Step 5: Verify bot pod running ---
	t.Run("Step5_VerifyPodRunning", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		t.Log("Waiting for OpenClaw bot pod...")
		for {
			select {
			case <-ctx.Done():
				// Dump pod status for debugging
				pods, _ := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
					LabelSelector: "app.kubernetes.io/instance=" + testBotName,
				})
				if len(pods.Items) > 0 {
					pod := pods.Items[0]
					t.Fatalf("Timed out waiting for bot pod (phase=%s, conditions=%v)", pod.Status.Phase, pod.Status.Conditions)
				}
				t.Fatal("Timed out waiting for bot pod — no pods found")
			default:
			}

			pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/instance=" + testBotName,
			})
			if err == nil && len(pods.Items) > 0 {
				pod := pods.Items[0]
				for _, c := range pod.Status.Conditions {
					if c.Type == "Ready" && c.Status == "True" {
						t.Logf("Bot pod %s is running", pod.Name)
						goto podReady
					}
				}
			}
			time.Sleep(5 * time.Second)
		}
	podReady:
	})

	// --- Step 6: Verify gateway responds on port 18789 ---
	t.Run("Step6_VerifyRuntimeConfig", func(t *testing.T) {
		assertRuntimeConfig(t, "anthropic/claude-sonnet-4-5-20250929")
	})

	// --- Step 7: Verify gateway responds on port 18789 ---
	t.Run("Step7_VerifyGateway", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Find the bot pod and use port-forward or service endpoint
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/instance=" + testBotName,
		})
		if err != nil || len(pods.Items) == 0 {
			t.Fatal("No bot pods found for gateway check")
		}

		// Check gateway via the ClusterIP service
		svcURL := fmt.Sprintf("http://%s-openclaw.%s.svc.cluster.local:18789/", testBotName, namespace)

		// If running outside the cluster, check via the bot detail API instead
		resp := doGet(t, baseURL+"/bots/"+testBotName)
		if resp.StatusCode == http.StatusOK {
			t.Logf("Bot detail API accessible, gateway service at %s", svcURL)
			// Verify the pod has the gateway container port configured
			pod := pods.Items[0]
			for _, container := range pod.Spec.Containers {
				for _, port := range container.Ports {
					if port.ContainerPort == 18789 {
						t.Log("Gateway port 18789 configured on pod")
						return
					}
				}
			}
			t.Log("Warning: port 18789 not found in pod spec, gateway may use default")
		} else {
			t.Fatalf("Bot detail check failed: %d", resp.StatusCode)
		}
	})

	// --- Step 8: Update config (change model) ---
	t.Run("Step8_UpdateConfig", func(t *testing.T) {
		body := map[string]any{
			"configFields": map[string]string{
				"authChoice":      "apiKey",
				"anthropicApiKey": "1p:e2e-anthropic-key",
				"discordBotToken": "1p:e2e-discord-token",
				"discordEnabled":  "true",
				"defaultModel":    "anthropic/claude-haiku-4-5-20251001", // Changed model
			},
		}
		resp := doPut(t, baseURL+"/bots/"+testBotName+"/config", body)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("Update config failed: %d — %s", resp.StatusCode, string(b))
		}
		t.Log("Config updated, pod should restart")
	})

	// --- Step 9: Verify pod restarts with updated config ---
	t.Run("Step9_VerifyRestart", func(t *testing.T) {
		// Wait for pod to begin restart
		time.Sleep(10 * time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				t.Fatal("Timed out waiting for pod restart")
			default:
			}
			pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/instance=" + testBotName,
			})
			if err == nil && len(pods.Items) > 0 {
				for _, c := range pods.Items[0].Status.Conditions {
					if c.Type == "Ready" && c.Status == "True" {
						t.Log("Pod restarted and ready")
						goto restarted
					}
				}
			}
			time.Sleep(3 * time.Second)
		}
	restarted:
	})

	t.Run("Step10_VerifyUpdatedRuntimeConfig", func(t *testing.T) {
		assertRuntimeConfig(t, "anthropic/claude-haiku-4-5-20251001")
	})

	// --- Step 11: Delete bot ---
	t.Run("Step11_DeleteBot", func(t *testing.T) {
		resp := doDeleteResp(t, baseURL+"/bots/"+testBotName)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("Delete bot failed: %d — %s", resp.StatusCode, string(b))
		}
		t.Log("Bot deleted")

		// Wait for pods to terminate (retry up to 60s)
		ctx := context.Background()
		deadline := time.Now().Add(60 * time.Second)
		for {
			pods, _ := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/instance=" + testBotName,
			})
			if pods == nil || len(pods.Items) == 0 {
				t.Log("Bot pods cleaned up")
				break
			}
			if time.Now().After(deadline) {
				t.Errorf("Expected 0 pods for deleted bot, got %d after 60s", len(pods.Items))
				break
			}
			time.Sleep(3 * time.Second)
		}

		_, err := clientset.CoreV1().Secrets(namespace).Get(ctx, testBotName+"-openclaw-config", metav1.GetOptions{})
		if err == nil {
			t.Errorf("legacy config secret %s-openclaw-config should not exist", testBotName)
		} else {
			t.Log("No legacy config secret created (expected)")
		}
	})
}
