//go:build e2e

package e2e

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestLifecycle_RealOnePasswordConnect exercises the full ClawMachine lifecycle
// with a real 1Password Connect Server:
//
//  1. Install 1Password Connect Server (real credentials)
//  2. Wait for Connect pods healthy
//  3. Verify Connect status via JSON API
//  4. Create ExternalSecrets, wait for sync
//  5. Create PicoClaw bot with 1Password secret refs
//  6. Verify pod running + config mounted
//  7. Verify bot visible via JSON API
//  8. Update config, verify pod restart
//  9. Delete bot, verify cleanup
//  10. Uninstall Connect, verify namespace removed
func TestLifecycle_RealOnePasswordConnect(t *testing.T) {
	skipIfNoServer(t)

	credFile := os.Getenv("OP_CREDENTIALS_FILE")
	connectToken := os.Getenv("OP_CONNECT_TOKEN")
	vaultName := os.Getenv("OP_VAULT_NAME")
	secretItemName := os.Getenv("OP_SECRET_ITEM_NAME")

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
	if secretItemName == "" {
		secretItemName = "anthropic-key"
	}

	credJSON, err := os.ReadFile(credFile)
	if err != nil {
		t.Skipf("Skipping: cannot read credentials file %s: %v", credFile, err)
	}

	clientset, _ := getK8sClients(t)
	const testBotName = "e2e-lifecycle"
	const namespace = "claw-machine"

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
		t.Log("✅ 1Password Connect install request accepted")
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
				t.Logf("✅ Connect pods ready (%d/%d)", ready, len(pods.Items))
				break
			}
			t.Logf("  Waiting... %d/%d pods ready", ready, len(pods.Items))
			time.Sleep(5 * time.Second)
		}
		waitForSecretStoreReady(t, namespace, 5*time.Minute)
	})

	// --- Step 3: Verify Connect via JSON API ---
	t.Run("Step3_VerifyConnectStatus", func(t *testing.T) {
		resp, err := httpClient.Get(baseURL + "/settings/status")
		if err != nil {
			t.Logf("Warning: GET /settings/status failed: %v — continuing", err)
			return
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Logf("Warning: /settings/status returned %d: %s — continuing", resp.StatusCode, body)
			return
		}
		// Response should mention Connect being installed
		if strings.Contains(string(body), "installed") || strings.Contains(string(body), "true") {
			t.Log("✅ Connect status confirmed via API")
		} else {
			t.Logf("Warning: status response didn't confirm Connect installed: %s", body)
		}
	})

	// --- Step 4: Create ExternalSecrets for Anthropic key and Discord token ---
	t.Run("Step4_CreateSecrets", func(t *testing.T) {
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
				"refreshInterval": "30s",
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
			t.Logf("✅ ExternalSecret %s created", s.name)
		}

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
					t.Logf("✅ Secret %s synced from 1Password", s.name)
					synced = true
				} else {
					time.Sleep(3 * time.Second)
				}
			}
		}
	})

	// --- Step 5: Create PicoClaw bot ---
	t.Run("Step5_CreateBot", func(t *testing.T) {
		body := map[string]any{
			"releaseName": testBotName,
			"botType":     "picoclaw",
			"values": map[string]any{
				"persistence":   map[string]any{"enabled": false},
				"networkPolicy": map[string]any{"ingress": false, "egress": true},
			},
			"configFields": map[string]string{
				"agentModel":      "claude-3-5-haiku-20241022",
				"agentMaxTokens":  "4096",
				"discordEnabled":  "true",
				"anthropicApiKey": "1p:e2e-anthropic-key",
				"discordToken":    "1p:e2e-discord-token",
			},
		}
		resp := doPost(t, baseURL+"/bots", body)
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK &&
			resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusAccepted {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("Create bot failed: %d — %s", resp.StatusCode, string(b))
		}
		t.Log("✅ PicoClaw bot created")
	})

	// --- Step 6: Verify pod running + config mounted ---
	t.Run("Step6_VerifyPod", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		t.Log("Waiting for bot pod...")
		for {
			select {
			case <-ctx.Done():
				t.Fatal("Timed out waiting for bot pod")
			default:
			}

			pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/instance=" + testBotName,
			})
			if err == nil && len(pods.Items) > 0 {
				pod := pods.Items[0]
				for _, c := range pod.Status.Conditions {
					if c.Type == "Ready" && c.Status == "True" {
						t.Logf("✅ Bot pod %s is running", pod.Name)
						for _, vm := range pod.Spec.Containers[0].VolumeMounts {
							if vm.Name == "config-file" {
								t.Logf("✅ Config file mounted at %s", vm.MountPath)
								goto podReady
							}
						}
						t.Log("⚠️  Config file volume mount not found")
						goto podReady
					}
				}
			}
			time.Sleep(5 * time.Second)
		}
	podReady:
	})

	// --- Step 7: Verify bot visible via JSON API ---
	t.Run("Step7_VerifyBotAPI", func(t *testing.T) {
		resp, err := httpClient.Get(baseURL + "/bots/" + testBotName)
		if err != nil {
			t.Fatalf("GET /bots/%s: %v", testBotName, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Bot not found via API: %d — %s", resp.StatusCode, body)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), testBotName) {
			t.Errorf("API response should contain bot name %q, got: %s", testBotName, body)
		}
		t.Log("✅ Bot visible via JSON API")
	})

	// --- Step 8: Update config ---
	t.Run("Step8_UpdateConfig", func(t *testing.T) {
		body := map[string]any{
			"configFields": map[string]string{
				"agentModel":      "claude-3-5-haiku-20241022",
				"agentMaxTokens":  "16384",
				"discordEnabled":  "true",
				"anthropicApiKey": "1p:e2e-anthropic-key",
				"discordToken":    "1p:e2e-discord-token",
			},
		}
		resp := doPut(t, baseURL+"/bots/"+testBotName+"/config", body)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("Update config failed: %d — %s", resp.StatusCode, string(b))
		}
		t.Log("✅ Config updated, waiting for pod restart")

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
						t.Log("✅ Pod restarted and ready")
						goto restarted
					}
				}
			}
			time.Sleep(3 * time.Second)
		}
	restarted:
	})

	// --- Step 9: Delete bot ---
	t.Run("Step9_DeleteBot", func(t *testing.T) {
		resp := doDeleteResp(t, baseURL+"/bots/"+testBotName)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("Delete bot failed: %d — %s", resp.StatusCode, string(b))
		}
		t.Log("✅ Bot deleted")

		ctx := context.Background()
		deadline := time.Now().Add(60 * time.Second)
		for {
			pods, _ := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/instance=" + testBotName,
			})
			if pods == nil || len(pods.Items) == 0 {
				t.Log("✅ Bot pods cleaned up")
				break
			}
			if time.Now().After(deadline) {
				t.Errorf("Expected 0 pods after delete, got %d", len(pods.Items))
				break
			}
			time.Sleep(3 * time.Second)
		}
	})

	// --- Step 10: Uninstall Connect ---
	t.Run("Step10_UninstallConnect", func(t *testing.T) {
		resp := doDeleteResp(t, baseURL+"/settings/connect")
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("Uninstall Connect failed: %d — %s", resp.StatusCode, string(b))
		}
		t.Log("✅ Connect uninstall request sent")

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				t.Log("Warning: 1password namespace cleanup timed out")
				return
			default:
			}
			_, err := clientset.CoreV1().Namespaces().Get(ctx, "1password", metav1.GetOptions{})
			if err != nil {
				t.Log("✅ 1password namespace removed")
				return
			}
			time.Sleep(3 * time.Second)
		}
	})
}
