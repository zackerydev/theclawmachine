package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/zackerydev/clawmachine/control-plane/internal/service"
)

// NetworkHandler serves network flow data from Hubble for bot pods.
type NetworkHandler struct {
	k8s  *service.KubernetesService
	tmpl TemplateRenderer
}

func NewNetworkHandler(k8s *service.KubernetesService, tmpl TemplateRenderer) *NetworkHandler {
	return &NetworkHandler{k8s: k8s, tmpl: tmpl}
}

// Flow represents a simplified Hubble network flow for display.
type Flow struct {
	Time        string `json:"time"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Port        string `json:"port"`
	Protocol    string `json:"protocol"`
	Verdict     string `json:"verdict"` // FORWARDED, DROPPED, ERROR
	DNSQuery    string `json:"dnsQuery,omitempty"`
	Type        string `json:"type"` // L3/L4, DNS, L7
	Count       int    `json:"count,omitempty"`
}

// Flows returns recent network flows for a bot's pods via Hubble CLI.
// GET /bots/{name}/network
func (h *NetworkHandler) Flows(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "missing bot name", http.StatusBadRequest)
		return
	}

	namespace := defaultNamespace()
	selector := fmt.Sprintf("app.kubernetes.io/instance=%s", name)
	htmx := isHTMX(r)

	// Get pod names for this bot
	pods, err := h.k8s.Clientset().CoreV1().Pods(namespace).List(
		r.Context(), metav1.ListOptions{LabelSelector: selector},
	)
	if err != nil || len(pods.Items) == 0 {
		if htmx {
			if !renderOrError(w, r, h.tmpl, "network-flows", map[string]any{
				"Error": "No pods found for " + name,
				"Flows": []Flow{},
			}, true) {
				return
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"error": "no pods found", "flows": []Flow{}}); err != nil {
			slog.Warn("network: failed to encode empty pod response", "release", name, "error", err)
		}
		return
	}

	// Query Hubble via kubectl exec into the cilium agent on the same node as the bot pod
	nodeName := pods.Items[0].Spec.NodeName
	flows, err := h.queryHubbleFlows(r.Context(), namespace, name, nodeName)
	if err != nil {
		if htmx {
			if !renderOrError(w, r, h.tmpl, "network-flows", map[string]any{
				"Error": "Hubble unavailable: " + err.Error(),
				"Flows": []Flow{},
			}, true) {
				return
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if encodeErr := json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "flows": []Flow{}}); encodeErr != nil {
			slog.Warn("network: failed to encode error response", "release", name, "error", encodeErr)
		}
		return
	}

	// Filter out kube-dns internal flows and simplify
	var simplified []Flow
	for _, f := range flows {
		// Skip internal DNS proxy traffic
		if f.Destination == "kube-system/" || strings.Contains(f.Destination, "kube-dns") {
			continue
		}
		simplified = append(simplified, f)
	}

	// Collapse repeated flows into a single row so noisy retries don't hide the hostname.
	simplified = collapseFlows(simplified)

	var allowed, blocked []Flow
	var internalAllowed, internalBlocked []Flow
	for _, f := range simplified {
		if !isExternalDestination(f.Destination) {
			if f.Verdict == "DROPPED" {
				internalBlocked = append(internalBlocked, f)
			} else {
				internalAllowed = append(internalAllowed, f)
			}
			continue
		}
		if f.Verdict == "DROPPED" {
			blocked = append(blocked, f)
		} else {
			allowed = append(allowed, f)
		}
	}

	data := map[string]any{
		"BotName":              name,
		"Allowed":              allowed,
		"Blocked":              blocked,
		"AllowedCount":         len(allowed),
		"BlockedCount":         len(blocked),
		"InternalAllowed":      internalAllowed,
		"InternalBlocked":      internalBlocked,
		"InternalAllowedCount": len(internalAllowed),
		"InternalBlockedCount": len(internalBlocked),
		"InternalTotalCount":   len(internalAllowed) + len(internalBlocked),
	}

	if htmx {
		if !renderOrError(w, r, h.tmpl, "network-flows", data, true) {
			return
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Warn("network: failed to encode flow response", "release", name, "error", err)
	}
}

// queryHubbleFlows uses kubectl exec to run hubble observe in the cilium agent pod
// on the same node as the bot pod. Each cilium agent only sees flows for its own node,
// so we must target the correct one.
func (h *NetworkHandler) queryHubbleFlows(ctx context.Context, namespace, botName, nodeName string) ([]Flow, error) {
	// Find the cilium agent pod on the same node as the bot pod
	ciliumPods, err := h.k8s.Clientset().CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{
		LabelSelector: "k8s-app=cilium",
		FieldSelector: "spec.nodeName=" + nodeName,
		Limit:         1,
	})
	if err != nil || len(ciliumPods.Items) == 0 {
		return nil, fmt.Errorf("no cilium agent pods found")
	}
	ciliumPod := ciliumPods.Items[0].Name

	// Run hubble observe with pod filter and JSON output
	cmd := []string{
		"hubble", "observe",
		"--namespace", namespace,
		"--pod", namespace + "/" + botName,
		"--last", "50",
		"--output", "json",
	}

	stdout, _, err := execInK8sPod(ctx, h.k8s, "kube-system", ciliumPod, cmd)
	if err != nil {
		// Try with label selector instead of exact pod name
		cmd = []string{
			"hubble", "observe",
			"--namespace", namespace,
			"--label", "app.kubernetes.io/instance=" + botName,
			"--last", "50",
			"--output", "json",
		}
		var stderr string
		stdout, stderr, err = execInK8sPod(ctx, h.k8s, "kube-system", ciliumPod, cmd)
		if err != nil {
			return nil, fmt.Errorf("hubble observe failed: %v (stderr: %s)", err, stderr)
		}
	}

	return parseHubbleJSON(stdout), nil
}

// execInK8sPod runs a command in a k8s pod and returns stdout/stderr.
func execInK8sPod(ctx context.Context, k8s *service.KubernetesService, namespace, pod string, command []string) (string, string, error) {
	execCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req := k8s.Clientset().CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: command,
			Stdout:  true,
			Stderr:  true,
		}, scheme.ParameterCodec)

	restConfig := k8s.RESTConfig()
	if restConfig == nil {
		return "", "", fmt.Errorf("no REST config available")
	}

	exec, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("creating executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(execCtx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	return stdout.String(), stderr.String(), err
}

// parseHubbleJSON parses newline-delimited JSON from hubble observe.
func parseHubbleJSON(output string) []Flow {
	var flows []Flow
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		f := Flow{
			Time:    getNestedString(raw, "time"),
			Verdict: getNestedString(raw, "flow", "verdict"),
			Type:    getNestedString(raw, "flow", "Type"),
		}

		// Source
		if src, ok := raw["flow"].(map[string]any); ok {
			if s, ok := src["source"].(map[string]any); ok {
				f.Source = formatNamespacedWorkload(getStr(s, "namespace"), getStr(s, "pod_name"))
				if labels, ok := s["labels"].([]any); ok {
					for _, l := range labels {
						if ls, ok := l.(string); ok && strings.HasPrefix(ls, "app.kubernetes.io/instance=") {
							f.Source = strings.TrimPrefix(ls, "app.kubernetes.io/instance=")
						}
					}
				}
			}
			if d, ok := src["destination"].(map[string]any); ok {
				destNS := getStr(d, "namespace")
				destPod := getStr(d, "pod_name")
				f.Destination = formatNamespacedWorkload(destNS, destPod)
				if domain := getStr(d, "domain"); domain != "" {
					f.Destination = domain
				}
			}
			// L4 info
			if l4, ok := src["l4"].(map[string]any); ok {
				for proto, v := range l4 {
					f.Protocol = strings.ToUpper(proto)
					if vm, ok := v.(map[string]any); ok {
						if port, ok := vm["destination_port"].(float64); ok {
							f.Port = fmt.Sprintf("%d", int(port))
						}
					}
				}
			}
			// DNS
			if dns, ok := src["l7"].(map[string]any); ok {
				if dnsInfo, ok := dns["dns"].(map[string]any); ok {
					f.Type = "DNS"
					f.DNSQuery = getStr(dnsInfo, "query")
				}
			}

			// External traffic may not populate destination namespace/pod.
			// Prefer resolved names, then DNS query, then destination IP.
			if f.Destination == "external" {
				if names := getStringSlice(src, "destination_names"); len(names) > 0 {
					f.Destination = names[0]
				} else if f.DNSQuery != "" {
					f.Destination = f.DNSQuery
				} else if ip, ok := src["IP"].(map[string]any); ok {
					if dst := getStr(ip, "destination"); dst != "" {
						f.Destination = dst
					}
				} else if ip, ok := src["ip"].(map[string]any); ok {
					if dst := getStr(ip, "destination"); dst != "" {
						f.Destination = dst
					}
				}
			}
		}

		if f.Type == "" {
			f.Type = "L3/L4"
		}
		flows = append(flows, f)
	}
	return flows
}

func getNestedString(m map[string]any, keys ...string) string {
	current := m
	for i, k := range keys {
		if i == len(keys)-1 {
			return getStr(current, k)
		}
		if next, ok := current[k].(map[string]any); ok {
			current = next
		} else {
			return ""
		}
	}
	return ""
}

func getStr(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getStringSlice(m map[string]any, key string) []string {
	raw, ok := m[key]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func formatNamespacedWorkload(namespace, pod string) string {
	switch {
	case namespace != "" && pod != "":
		return namespace + "/" + pod
	case pod != "":
		return pod
	case namespace != "":
		return namespace
	default:
		return "external"
	}
}

func collapseFlows(flows []Flow) []Flow {
	out := make([]Flow, 0, len(flows))
	indexByKey := make(map[string]int, len(flows))
	for _, f := range flows {
		key := strings.Join([]string{
			f.Verdict,
			f.Destination,
			f.Port,
			f.Protocol,
			f.DNSQuery,
		}, "|")
		if idx, ok := indexByKey[key]; ok {
			out[idx].Count++
			if f.Time > out[idx].Time {
				out[idx].Time = f.Time
			}
			continue
		}
		f.Count = 1
		indexByKey[key] = len(out)
		out = append(out, f)
	}
	return out
}

func isExternalDestination(dest string) bool {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return false
	}
	if dest == "external" {
		return true
	}
	if strings.Contains(dest, "/") {
		return false
	}
	host := dest
	if i := strings.Index(host, ":"); i > 0 {
		host = host[:i]
	}
	if ip := net.ParseIP(host); ip != nil {
		// Private/localhost/link-local IPs are internal noise for this UI.
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return false
		}
		return true
	}
	return strings.Contains(host, ".")
}
