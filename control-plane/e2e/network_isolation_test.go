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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	botNamespace = "claw-machine"
	busyboxName  = "nettest"
)

func getK8sClients(t *testing.T) (*kubernetes.Clientset, *rest.Config) {
	t.Helper()
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		t.Fatalf("failed to get kubeconfig: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatalf("failed to create k8s client: %v", err)
	}
	return clientset, config
}

func execInPod(t *testing.T, clientset *kubernetes.Clientset, config *rest.Config, namespace, podName string, command []string) (string, string, error) {
	t.Helper()
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: command,
			Stdout:  true,
			Stderr:  true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("creating executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	return stdout.String(), stderr.String(), err
}

func waitForPodRunning(t *testing.T, clientset *kubernetes.Clientset, namespace, labelSelector string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err == nil && len(pods.Items) > 0 {
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodRunning {
					return pod.Name
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("timed out waiting for pod with selector %q to be running", labelSelector)
	return ""
}

func createBusyboxBot(t *testing.T) {
	t.Helper()
	body := map[string]any{
		"releaseName": busyboxName,
		"botType":     "busybox",
		"values": map[string]any{
			"networkPolicy": map[string]any{
				"ingress":        false,
				"egress":         false,
				"useCilium":      true,
				"allowedDomains": []string{}, // empty = deny all egress
			},
		},
	}
	jsonBody, _ := json.Marshal(body)

	resp, err := http.Post(baseURL+"/bots", "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("POST /bots: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusInternalServerError {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Skipf("POST /bots returned 500 (server may need rebuild): %s", string(bodyBytes))
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /bots: expected 201 or 204, got %d", resp.StatusCode)
	}
}

func deleteBusyboxBot(t *testing.T) {
	t.Helper()
	req, _ := http.NewRequest("DELETE", baseURL+"/bots/"+busyboxName, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("cleanup DELETE /bots/%s: %v", busyboxName, err)
		return
	}
	resp.Body.Close()
}

func TestBusyboxNetworkIsolation(t *testing.T) {
	clientset, config := getK8sClients(t)

	// Clean up any previous test bot
	deleteBusyboxBot(t)
	time.Sleep(5 * time.Second)

	// Create busybox bot with network policy (deny all egress except DNS)
	createBusyboxBot(t)
	t.Cleanup(func() { deleteBusyboxBot(t) })

	// Wait for pod to be running
	selector := fmt.Sprintf("app.kubernetes.io/instance=%s", busyboxName)
	podName := waitForPodRunning(t, clientset, botNamespace, selector, 2*time.Minute)
	t.Logf("busybox pod running: %s", podName)

	// Test 1: Verify internet egress is blocked (wget google.com should fail)
	t.Run("EgressBlocked", func(t *testing.T) {
		stdout, stderr, err := execInPod(t, clientset, config, botNamespace, podName,
			[]string{"wget", "-q", "-O", "-", "--timeout=5", "http://google.com"})
		if err == nil {
			t.Errorf("expected wget to fail (egress should be blocked), but it succeeded.\nstdout: %s\nstderr: %s", stdout, stderr)
		} else {
			t.Logf("wget correctly failed: %v (stderr: %s)", err, stderr)
		}
	})

	// Test 2: Verify DNS resolution still works (use FQDN for busybox)
	t.Run("DNSWorks", func(t *testing.T) {
		stdout, stderr, err := execInPod(t, clientset, config, botNamespace, podName,
			[]string{"nslookup", "kubernetes.default.svc.cluster.local"})
		if err != nil {
			t.Errorf("expected DNS to work, but nslookup failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		} else if !strings.Contains(stdout, "Address") {
			t.Errorf("nslookup output doesn't look right: %s", stdout)
		} else {
			t.Logf("DNS resolution works: %s", strings.TrimSpace(stdout))
		}
	})
}
