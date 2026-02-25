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

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	picoName          = "lifecycle-test"
	localstackURL     = "http://localstack.claw-machine:4566"
	backupBucket      = "clawmachine-test-backups"
	backupRegion      = "us-east-1"
)

func createPicoClawBot(t *testing.T) {
	t.Helper()
	body := map[string]any{
		"releaseName": picoName,
		"botType":     "picoclaw",
		"values": map[string]any{
			"persistence": map[string]any{
				"enabled": true,
				"size":    "256Mi",
			},
			"backup": map[string]any{
				"enabled":  true,
				"schedule": "0 */6 * * *",
				"provider": "s3",
				"s3": map[string]any{
					"bucket":   backupBucket,
					"region":   backupRegion,
					"endpoint": localstackURL,
				},
			},
			"networkPolicy": map[string]any{
				"ingress": true,
				"egress":  true,
			},
		},
	}
	jsonBody, _ := json.Marshal(body)

	resp, err := http.Post(baseURL+"/bots", "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("POST /bots: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /bots: expected 201 or 204, got %d: %s", resp.StatusCode, string(bodyBytes))
	}
}

func deletePicoClawBot(t *testing.T) {
	t.Helper()
	req, _ := http.NewRequest("DELETE", baseURL+"/bots/"+picoName, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("cleanup DELETE /bots/%s: %v", picoName, err)
		return
	}
	resp.Body.Close()
}

func waitForPicoClawHealthy(t *testing.T, timeout time.Duration) {
	t.Helper()
	clientset, _ := getK8sClients(t)
	selector := fmt.Sprintf("app.kubernetes.io/instance=%s", picoName)

	// First wait for pod running
	podName := waitForPodRunning(t, clientset, botNamespace, selector, timeout)
	t.Logf("picoclaw pod running: %s", podName)

	// Then wait for the health endpoint via port-forward or service
	// We check the pod's readiness condition instead
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pod, err := clientset.CoreV1().Pods(botNamespace).Get(context.Background(), podName, metav1.GetOptions{})
		if err == nil {
			for _, cond := range pod.Status.Conditions {
				if cond.Type == "Ready" && cond.Status == "True" {
					t.Logf("picoclaw pod is ready")
					return
				}
			}
		}
		time.Sleep(3 * time.Second)
	}
	t.Fatalf("timed out waiting for picoclaw pod to be ready")
}

// createLocalStackBucket creates the S3 bucket in LocalStack.
// Uses the localstack endpoint accessible from the test runner (localhost:4566).
func createLocalStackBucket(t *testing.T) {
	t.Helper()
	// Use localhost since we're running from outside the cluster (port-forwarded)
	endpoint := "http://localhost:4566"

	req, _ := http.NewRequest("PUT", endpoint+"/"+backupBucket, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("LocalStack not available (is it deployed?): %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 409 {
		t.Fatalf("create bucket returned %d", resp.StatusCode)
	}
	t.Logf("LocalStack bucket %q ready", backupBucket)
}

// listLocalStackObjects lists objects in the backup bucket.
func listLocalStackObjects(t *testing.T) []string {
	t.Helper()
	endpoint := "http://localhost:4566"

	resp, err := http.Get(endpoint + "/" + backupBucket + "?list-type=2")
	if err != nil {
		t.Fatalf("failed to list LocalStack objects: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// Simple XML parse for <Key> elements
	var keys []string
	content := string(body)
	for {
		start := strings.Index(content, "<Key>")
		if start == -1 {
			break
		}
		end := strings.Index(content[start:], "</Key>")
		if end == -1 {
			break
		}
		keys = append(keys, content[start+5:start+end])
		content = content[start+end+6:]
	}
	return keys
}

// triggerBackup triggers the backup CronJob manually by creating a Job from it.
func triggerBackup(t *testing.T) {
	t.Helper()
	clientset, _ := getK8sClients(t)
	cronJobName := picoName + "-picoclaw-backup"

	// Get the CronJob
	cronJob, err := clientset.BatchV1().CronJobs(botNamespace).Get(context.Background(), cronJobName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get cronjob %s: %v", cronJobName, err)
	}

	// Create a Job from the CronJob spec
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cronJobName + "-manual-test",
			Namespace: botNamespace,
		},
		Spec: cronJob.Spec.JobTemplate.Spec,
	}

	_, err = clientset.BatchV1().Jobs(botNamespace).Create(context.Background(), job, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create backup job: %v", err)
	}

	// Wait for job completion
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		j, err := clientset.BatchV1().Jobs(botNamespace).Get(context.Background(), job.Name, metav1.GetOptions{})
		if err == nil && j.Status.Succeeded > 0 {
			t.Logf("backup job completed successfully")
			return
		}
		if err == nil && j.Status.Failed > 0 {
			t.Fatalf("backup job failed")
		}
		time.Sleep(3 * time.Second)
	}
	t.Fatalf("timed out waiting for backup job to complete")
}

func TestPicoClawLifecycle(t *testing.T) {
	// Clean up any previous test bot
	deletePicoClawBot(t)
	time.Sleep(5 * time.Second)

	// Step 1: Create LocalStack bucket
	createLocalStackBucket(t)

	// Step 2: Create PicoClaw bot
	t.Run("CreateBot", func(t *testing.T) {
		createPicoClawBot(t)
		t.Cleanup(func() { deletePicoClawBot(t) })
	})

	// Step 3: Wait for healthy
	t.Run("WaitForHealthy", func(t *testing.T) {
		waitForPicoClawHealthy(t, 3*time.Minute)
	})

	// Step 4: Trigger backup
	t.Run("TriggerBackup", func(t *testing.T) {
		triggerBackup(t)
	})

	// Step 5: Verify backup exists in LocalStack
	t.Run("VerifyBackupInS3", func(t *testing.T) {
		objects := listLocalStackObjects(t)
		if len(objects) == 0 {
			t.Error("expected at least one object in the backup bucket, got none")
		} else {
			t.Logf("found %d backup objects: %v", len(objects), objects)
		}
	})

	// Step 6: Delete the bot
	t.Run("DeleteBot", func(t *testing.T) {
		deletePicoClawBot(t)
		time.Sleep(10 * time.Second)
	})

	// Step 7: Recreate from scratch (simulating restore — same config, data from PVC if retained)
	t.Run("RestoreBot", func(t *testing.T) {
		createPicoClawBot(t)
		t.Cleanup(func() { deletePicoClawBot(t) })
		waitForPicoClawHealthy(t, 3*time.Minute)
		t.Logf("picoclaw restored and running")
	})
}
