package version

import "testing"

func TestNormalizeRuntimeImageTag(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "stable", in: "0.1.6", want: "0.1.6"},
		{name: "stable with v", in: "v0.1.6", want: "0.1.6"},
		{name: "prerelease", in: "v0.1.7-rc.1", want: "0.1.7-rc.1"},
		{name: "dev", in: "dev", want: ""},
		{name: "empty", in: "", want: ""},
		{name: "invalid", in: "dirty-build", wantErr: true},
		{name: "metadata", in: "0.1.6+sha.123", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeRuntimeImageTag(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizeRuntimeImageTag(%q) err=%v wantErr=%t", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("NormalizeRuntimeImageTag(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveRuntimeOrFallbackImageTag(t *testing.T) {
	tests := []struct {
		name         string
		runtime      string
		fallback     string
		wantTag      string
		wantFallback bool
		wantErr      bool
	}{
		{name: "runtime stable", runtime: "0.1.6", fallback: "0.1.0", wantTag: "0.1.6", wantFallback: false},
		{name: "runtime prerelease", runtime: "v0.1.7-rc.1", fallback: "0.1.0", wantTag: "0.1.7-rc.1", wantFallback: false},
		{name: "dev fallback", runtime: "dev", fallback: "0.1.0", wantTag: "0.1.0", wantFallback: true},
		{name: "empty fallback", runtime: "", fallback: "0.1.0", wantTag: "0.1.0", wantFallback: true},
		{name: "invalid runtime", runtime: "dirty-build", fallback: "0.1.0", wantErr: true},
		{name: "dev missing fallback", runtime: "dev", fallback: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTag, gotFallback, err := ResolveRuntimeOrFallbackImageTag(tt.runtime, tt.fallback)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveRuntimeOrFallbackImageTag(%q,%q) err=%v wantErr=%t", tt.runtime, tt.fallback, err, tt.wantErr)
			}
			if gotTag != tt.wantTag {
				t.Fatalf("ResolveRuntimeOrFallbackImageTag(%q,%q) tag=%q want=%q", tt.runtime, tt.fallback, gotTag, tt.wantTag)
			}
			if !tt.wantErr && gotFallback != tt.wantFallback {
				t.Fatalf("ResolveRuntimeOrFallbackImageTag(%q,%q) fallback=%t want=%t", tt.runtime, tt.fallback, gotFallback, tt.wantFallback)
			}
		})
	}
}
