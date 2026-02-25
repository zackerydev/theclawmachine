package handler

import (
	"testing"
)

func TestParseHubbleJSON_Empty(t *testing.T) {
	flows := parseHubbleJSON("")
	if len(flows) != 0 {
		t.Errorf("expected 0 flows, got %d", len(flows))
	}
}

func TestParseHubbleJSON_InvalidJSON(t *testing.T) {
	flows := parseHubbleJSON("not json\nalso not json\n")
	if len(flows) != 0 {
		t.Errorf("expected 0 flows for invalid JSON, got %d", len(flows))
	}
}

func TestParseHubbleJSON_ValidFlow(t *testing.T) {
	input := `{"time":"2026-01-01T00:00:00Z","flow":{"verdict":"FORWARDED","Type":"L3/L4","source":{"namespace":"claw-machine","pod_name":"mybot-abc","labels":["app.kubernetes.io/instance=mybot"]},"destination":{"namespace":"default","pod_name":"svc-xyz"},"l4":{"TCP":{"destination_port":443}}}}`
	flows := parseHubbleJSON(input)
	if len(flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(flows))
	}
	f := flows[0]
	if f.Verdict != "FORWARDED" {
		t.Errorf("Verdict = %q, want FORWARDED", f.Verdict)
	}
	if f.Source != "mybot" {
		t.Errorf("Source = %q, want mybot", f.Source)
	}
	if f.Protocol != "TCP" {
		t.Errorf("Protocol = %q, want TCP", f.Protocol)
	}
	if f.Port != "443" {
		t.Errorf("Port = %q, want 443", f.Port)
	}
}

func TestParseHubbleJSON_DNSFlow(t *testing.T) {
	input := `{"time":"2026-01-01T00:00:00Z","flow":{"verdict":"FORWARDED","source":{"namespace":"claw-machine","pod_name":"mybot-abc"},"destination":{"namespace":"kube-system","pod_name":"coredns","domain":"api.openai.com"},"l7":{"dns":{"query":"api.openai.com"}}}}`
	flows := parseHubbleJSON(input)
	if len(flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(flows))
	}
	if flows[0].Type != "DNS" {
		t.Errorf("Type = %q, want DNS", flows[0].Type)
	}
	if flows[0].DNSQuery != "api.openai.com" {
		t.Errorf("DNSQuery = %q, want api.openai.com", flows[0].DNSQuery)
	}
	if flows[0].Destination != "api.openai.com" {
		t.Errorf("Destination = %q, want api.openai.com (domain override)", flows[0].Destination)
	}
}

func TestParseHubbleJSON_MultiLine(t *testing.T) {
	input := `{"time":"t1","flow":{"verdict":"FORWARDED","source":{},"destination":{}}}
{"time":"t2","flow":{"verdict":"DROPPED","source":{},"destination":{}}}
`
	flows := parseHubbleJSON(input)
	if len(flows) != 2 {
		t.Fatalf("expected 2 flows, got %d", len(flows))
	}
	if flows[0].Verdict != "FORWARDED" {
		t.Errorf("flows[0].Verdict = %q", flows[0].Verdict)
	}
	if flows[1].Verdict != "DROPPED" {
		t.Errorf("flows[1].Verdict = %q", flows[1].Verdict)
	}
}

func TestParseHubbleJSON_UsesDestinationNamesForExternalHost(t *testing.T) {
	input := `{"time":"2026-01-01T00:00:00Z","flow":{"verdict":"DROPPED","source":{"namespace":"claw-machine","pod_name":"dorothy-abc"},"destination":{"namespace":"","pod_name":""},"destination_names":["heartland.dev"],"l4":{"TCP":{"destination_port":443}}}}`
	flows := parseHubbleJSON(input)
	if len(flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(flows))
	}
	if flows[0].Destination != "heartland.dev" {
		t.Fatalf("Destination = %q, want heartland.dev", flows[0].Destination)
	}
	if flows[0].Port != "443" {
		t.Fatalf("Port = %q, want 443", flows[0].Port)
	}
}

func TestCollapseFlows_DeduplicatesByDestinationAndVerdict(t *testing.T) {
	in := []Flow{
		{Time: "2026-01-01T00:00:00Z", Verdict: "DROPPED", Destination: "heartland.dev", Port: "443", Protocol: "TCP"},
		{Time: "2026-01-01T00:00:01Z", Verdict: "DROPPED", Destination: "heartland.dev", Port: "443", Protocol: "TCP"},
		{Time: "2026-01-01T00:00:02Z", Verdict: "FORWARDED", Destination: "google.com", Port: "443", Protocol: "TCP"},
	}
	out := collapseFlows(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 collapsed flows, got %d", len(out))
	}
	if out[0].Destination != "heartland.dev" || out[0].Count != 2 {
		t.Fatalf("first collapsed flow = %#v, want heartland.dev with Count=2", out[0])
	}
	if out[0].Time != "2026-01-01T00:00:01Z" {
		t.Fatalf("first collapsed flow time = %q, want newest time", out[0].Time)
	}
	if out[1].Destination != "google.com" || out[1].Count != 1 {
		t.Fatalf("second collapsed flow = %#v, want google.com with Count=1", out[1])
	}
}

func TestIsExternalDestination(t *testing.T) {
	tests := []struct {
		dest string
		want bool
	}{
		{dest: "heartland.dev", want: true},
		{dest: "heartland.dev:443", want: true},
		{dest: "104.21.2.3", want: true},
		{dest: "10.0.0.88:33644", want: false},
		{dest: "172.20.1.10:443", want: false},
		{dest: "192.168.1.12:443", want: false},
		{dest: "127.0.0.1:8080", want: false},
		{dest: "external", want: true},
		{dest: "claw-machine/dorothy-openclaw-abc", want: false},
		{dest: "kube-system/coredns-xyz", want: false},
		{dest: "", want: false},
		{dest: "svc-name", want: false},
	}

	for _, tt := range tests {
		got := isExternalDestination(tt.dest)
		if got != tt.want {
			t.Errorf("isExternalDestination(%q) = %v, want %v", tt.dest, got, tt.want)
		}
	}
}

func TestGetNestedString(t *testing.T) {
	m := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "deep",
			},
		},
		"top": "shallow",
	}

	tests := []struct {
		keys []string
		want string
	}{
		{[]string{"top"}, "shallow"},
		{[]string{"a", "b", "c"}, "deep"},
		{[]string{"missing"}, ""},
		{[]string{"a", "missing"}, ""},
		{[]string{"a", "b", "missing"}, ""},
	}

	for _, tt := range tests {
		got := getNestedString(m, tt.keys...)
		if got != tt.want {
			t.Errorf("getNestedString(%v) = %q, want %q", tt.keys, got, tt.want)
		}
	}
}

func TestGetStr(t *testing.T) {
	m := map[string]any{
		"str":    "hello",
		"num":    42,
		"nested": map[string]any{},
	}

	if got := getStr(m, "str"); got != "hello" {
		t.Errorf("getStr(str) = %q", got)
	}
	if got := getStr(m, "num"); got != "" {
		t.Errorf("getStr(num) = %q, want empty (not a string)", got)
	}
	if got := getStr(m, "missing"); got != "" {
		t.Errorf("getStr(missing) = %q", got)
	}
}
