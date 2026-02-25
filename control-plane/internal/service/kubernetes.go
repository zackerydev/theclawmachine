package service

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

type KubernetesService struct {
	clientset      kubernetes.Interface
	dynamic        dynamic.Interface
	restConfig     *rest.Config
	currentContext string
	kubeConfigPath string
	inCluster      bool
}

func NewKubernetesService(kubeContext string) (*KubernetesService, error) {
	// Try in-cluster config first (running inside a pod)
	restConfig, err := rest.InClusterConfig()
	if err == nil {
		return newK8sFromConfig(restConfig, "in-cluster", "", true)
	}

	// Fall back to kubeconfig (local development)
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	if kubeContext != "" {
		configOverrides.CurrentContext = kubeContext
	}

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	restConfig, err = kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("building kubeconfig: %w", err)
	}

	rawConfig, err := kubeConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("reading raw kubeconfig: %w", err)
	}

	return newK8sFromConfig(restConfig, rawConfig.CurrentContext, loadingRules.GetDefaultFilename(), false)
}

func newK8sFromConfig(restConfig *rest.Config, context, configPath string, inCluster bool) (*KubernetesService, error) {
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes clientset: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}

	return &KubernetesService{
		clientset:      clientset,
		dynamic:        dynClient,
		restConfig:     restConfig,
		currentContext: context,
		kubeConfigPath: configPath,
		inCluster:      inCluster,
	}, nil
}

func (k *KubernetesService) Clientset() kubernetes.Interface  { return k.clientset }
func (k *KubernetesService) DynamicClient() dynamic.Interface { return k.dynamic }
func (k *KubernetesService) RESTConfig() *rest.Config         { return k.restConfig }
func (k *KubernetesService) GetCurrentContext() string        { return k.currentContext }
func (k *KubernetesService) GetKubeConfigPath() string        { return k.kubeConfigPath }
func (k *KubernetesService) InCluster() bool                  { return k.inCluster }

// GetBackupLastSuccess returns the most recent successful backup run time for a release.
func (k *KubernetesService) GetBackupLastSuccess(ctx context.Context, namespace, releaseName string) (time.Time, bool, error) {
	cronJobs, err := k.clientset.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName),
	})
	if err != nil {
		return time.Time{}, false, fmt.Errorf("listing backup cronjobs for %q: %w", releaseName, err)
	}

	cronJob := selectBackupCronJob(cronJobs.Items, releaseName)
	if cronJob == nil {
		return time.Time{}, false, nil
	}

	if cronJob.Status.LastSuccessfulTime != nil {
		return cronJob.Status.LastSuccessfulTime.Time, true, nil
	}

	jobs, err := k.clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName),
	})
	if err != nil {
		return time.Time{}, false, fmt.Errorf("listing backup jobs for %q: %w", releaseName, err)
	}

	var latest time.Time
	var found bool
	for _, job := range jobs.Items {
		if !jobOwnedByCronJob(job, cronJob.Name) || job.Status.Succeeded < 1 {
			continue
		}

		completeAt := jobCompletionTime(job)
		if completeAt.IsZero() {
			continue
		}
		if !found || completeAt.After(latest) {
			latest = completeAt
			found = true
		}
	}

	return latest, found, nil
}

// HasCRD checks whether a CRD exists by looking for its API group in the cluster.
// The name should be in standard CRD format: "resource.group" (e.g., "ciliumnetworkpolicies.cilium.io").
func (k *KubernetesService) HasCRD(name string) bool {
	// Extract API group from CRD name (e.g., "ciliumnetworkpolicies.cilium.io" → "cilium.io")
	idx := 0
	for i, c := range name {
		if c == '.' {
			idx = i + 1
			break
		}
	}
	if idx == 0 || idx >= len(name) {
		return false
	}
	group := name[idx:]

	groups, err := k.clientset.Discovery().ServerGroups()
	if err != nil {
		return false
	}
	for _, g := range groups.Groups {
		if g.Name == group {
			return true
		}
	}
	return false
}

// GetPodLogs returns the last tailLines of logs for the first pod matching the release.
func (k *KubernetesService) GetPodLogs(ctx context.Context, namespace, releaseName string, tailLines int64) (string, error) {
	pods, err := k.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName),
	})
	if err != nil {
		return "", fmt.Errorf("listing pods for %q: %w", releaseName, err)
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no pods found for release %q", releaseName)
	}

	pod := pods.Items[0]
	container := selectLogContainer(pod, releaseName)
	opts := &corev1.PodLogOptions{
		TailLines: &tailLines,
		Container: container,
	}
	req := k.clientset.CoreV1().Pods(namespace).GetLogs(pod.Name, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("streaming logs for pod %q: %w", pod.Name, err)
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			slog.Warn("failed to close pod log stream", "pod", pod.Name, "namespace", namespace, "error", closeErr)
		}
	}()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(stream); err != nil {
		return "", fmt.Errorf("reading logs: %w", err)
	}
	return buf.String(), nil
}

// GetReleasePodHealthy reports whether any non-terminating pod for the release is Running and Ready.
func (k *KubernetesService) GetReleasePodHealthy(ctx context.Context, namespace, releaseName string) (bool, error) {
	pods, err := k.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName),
	})
	if err != nil {
		return false, fmt.Errorf("listing pods for %q: %w", releaseName, err)
	}

	for _, pod := range pods.Items {
		if pod.DeletionTimestamp == nil && pod.Status.Phase == corev1.PodRunning && podReady(pod) {
			return true, nil
		}
	}

	return false, nil
}

// ReadSecretData returns the raw data map of a K8s Secret.
func (k *KubernetesService) ReadSecretData(ctx context.Context, namespace, name string) (map[string][]byte, error) {
	secret, err := k.clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("reading secret %q: %w", name, err)
	}
	return secret.Data, nil
}

// RestartBot deletes all pods for a release, letting the deployment recreate them.
func (k *KubernetesService) RestartBot(ctx context.Context, namespace, releaseName string) error {
	pods, err := k.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName),
	})
	if err != nil {
		return fmt.Errorf("listing pods for %q: %w", releaseName, err)
	}

	for _, pod := range pods.Items {
		if err := k.clientset.CoreV1().Pods(namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("deleting pod %q: %w", pod.Name, err)
		}
	}
	return nil
}

// ExecInReleasePod runs a command in a pod owned by the release and returns stdout/stderr.
func (k *KubernetesService) ExecInReleasePod(ctx context.Context, namespace, releaseName, container string, command []string) (string, string, error) {
	pods, err := k.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName),
	})
	if err != nil {
		return "", "", fmt.Errorf("listing pods for %q: %w", releaseName, err)
	}
	if len(pods.Items) == 0 {
		return "", "", fmt.Errorf("no pods found for release %q", releaseName)
	}

	pod := selectExecPod(pods.Items)
	if pod == nil {
		return "", "", fmt.Errorf("no suitable pods found for release %q", releaseName)
	}

	restConfig := k.RESTConfig()
	if restConfig == nil {
		return "", "", fmt.Errorf("no REST config available")
	}

	req := k.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("creating exec for pod %q: %w", pod.Name, err)
	}

	execCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(execCtx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return stdout.String(), stderr.String(), fmt.Errorf("running command in pod %q: %w", pod.Name, err)
	}
	return stdout.String(), stderr.String(), nil
}

func selectExecPod(pods []corev1.Pod) *corev1.Pod {
	for i := range pods {
		p := &pods[i]
		if p.DeletionTimestamp == nil && p.Status.Phase == corev1.PodRunning && podReady(*p) {
			return p
		}
	}
	for i := range pods {
		p := &pods[i]
		if p.DeletionTimestamp == nil && p.Status.Phase == corev1.PodRunning {
			return p
		}
	}
	for i := range pods {
		p := &pods[i]
		if p.DeletionTimestamp == nil {
			return p
		}
	}
	if len(pods) == 0 {
		return nil
	}
	return &pods[0]
}

func podReady(pod corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func selectBackupCronJob(cronJobs []batchv1.CronJob, releaseName string) *batchv1.CronJob {
	if len(cronJobs) == 0 {
		return nil
	}

	for i := range cronJobs {
		if cronJobs[i].Name == releaseName+"-backup" {
			return &cronJobs[i]
		}
	}

	for i := range cronJobs {
		if strings.HasSuffix(cronJobs[i].Name, "-backup") {
			return &cronJobs[i]
		}
	}

	return nil
}

func jobOwnedByCronJob(job batchv1.Job, cronJobName string) bool {
	for _, owner := range job.OwnerReferences {
		if owner.Kind == "CronJob" && owner.Name == cronJobName {
			return true
		}
	}
	return false
}

func jobCompletionTime(job batchv1.Job) time.Time {
	if job.Status.CompletionTime != nil {
		return job.Status.CompletionTime.Time
	}
	if job.Status.StartTime != nil {
		return job.Status.StartTime.Time
	}
	return job.CreationTimestamp.Time
}
