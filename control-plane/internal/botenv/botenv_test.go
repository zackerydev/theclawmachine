package botenv

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestNewRegistry(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}

	// Should have both embedded bot types
	types := r.BotTypes()
	if len(types) < 2 {
		t.Fatalf("expected at least 2 bot types, got %d: %v", len(types), types)
	}

	if !r.IsValid("picoclaw") {
		t.Error("expected picoclaw to be valid")
	}
	if !r.IsValid("ironclaw") {
		t.Error("expected ironclaw to be valid")
	}
	if r.IsValid("nonexistent") {
		t.Error("expected nonexistent to be invalid")
	}
}

func TestRegistryOrder(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}

	types := r.BotTypes()
	if types[0] != "picoclaw" {
		t.Errorf("expected picoclaw first (order 10), got %s", types[0])
	}
	if types[1] != "ironclaw" {
		t.Errorf("expected ironclaw second (order 20), got %s", types[1])
	}
}

func TestIronClawFields(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}

	cfg := r.Get("ironclaw")
	if cfg == nil {
		t.Fatal("ironclaw config not found")
	}

	if cfg.DisplayName != "IronClaw" {
		t.Errorf("expected DisplayName=IronClaw, got %s", cfg.DisplayName)
	}

	allFields := cfg.AllFields()
	if len(allFields) == 0 {
		t.Fatal("expected ironclaw to have fields")
	}

	// Check gatewayAuthToken field
	var gatewayField *Field
	for i := range allFields {
		if allFields[i].Key == "gatewayAuthToken" {
			gatewayField = &allFields[i]
			break
		}
	}
	if gatewayField == nil {
		t.Fatal("expected gatewayAuthToken field")
	}
	if gatewayField.ValuesPath != "secrets.gatewayAuthToken" {
		t.Errorf("expected valuesPath=secrets.gatewayAuthToken, got %s", gatewayField.ValuesPath)
	}
	if !gatewayField.Required {
		t.Error("expected gatewayAuthToken to be required")
	}
	if !gatewayField.Secret {
		t.Error("expected gatewayAuthToken to be secret")
	}

	// Check conditional field
	var nearaiField *Field
	for i := range allFields {
		if allFields[i].Key == "nearaiSessionToken" {
			nearaiField = &allFields[i]
			break
		}
	}
	if nearaiField == nil {
		t.Fatal("expected nearaiSessionToken field")
	}
	if nearaiField.ShowWhen != "llmBackend=nearai" {
		t.Errorf("expected showWhen=llmBackend=nearai, got %s", nearaiField.ShowWhen)
	}
}

func TestPicoClawNoFields(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}

	cfg := r.Get("picoclaw")
	if cfg == nil {
		t.Fatal("picoclaw config not found")
	}
	if len(cfg.Sections) == 0 {
		t.Error("expected picoclaw to have sections")
	}
}

func TestOpenClawValuesOnlyConfig(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}

	cfg := r.Get("openclaw")
	if cfg == nil {
		t.Fatal("openclaw config not found")
	}
	if cfg.HasConfigFile() {
		t.Fatal("openclaw should be values/env-only and must not generate configFile content")
	}

	allFields := cfg.AllFields()
	var authChoice *Field
	for i := range allFields {
		if allFields[i].Key == "authChoice" {
			authChoice = &allFields[i]
			break
		}
	}
	if authChoice == nil {
		t.Fatal("openclaw authChoice field not found")
	}
	if authChoice.ConfigPath != "" {
		t.Fatalf("openclaw authChoice configPath = %q, want empty", authChoice.ConfigPath)
	}
}

// TestPicoClawConfigFile ensures picoclaw is configured as a JSON config-file bot
// with model providers. If either regresses, the chart will fail to generate the
// config Secret (bug: mount path targets a file that never gets created).
func TestPicoClawConfigFile(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}

	cfg := r.Get("picoclaw")
	if cfg == nil {
		t.Fatal("picoclaw config not found")
	}

	if cfg.ConfigFormat != "json" {
		t.Errorf("picoclaw configFormat = %q, want \"json\" (required for config file generation)", cfg.ConfigFormat)
	}
	if !cfg.HasConfigFile() {
		t.Error("picoclaw HasConfigFile() = false; bot must generate a config file")
	}
	if len(cfg.ModelProviders) == 0 {
		t.Error("picoclaw must have ModelProviders for model_list generation")
	}
	if cfg.AgentModelField == "" {
		t.Error("picoclaw AgentModelField must be set for model selection to work")
	}

	// Verify at least anthropic provider is registered with the expected prefix.
	var found bool
	for _, p := range cfg.ModelProviders {
		if p.Name == "anthropic" && p.ModelPrefix == "anthropic/" && p.APIKeyField == "anthropicApiKey" {
			found = true
			break
		}
	}
	if !found {
		t.Error("picoclaw missing anthropic ModelProvider with expected prefix and APIKeyField")
	}
}

func TestAll(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}

	all := r.All()
	if len(all) < 2 {
		t.Fatalf("expected at least 2 configs, got %d", len(all))
	}
	// Should be sorted by order
	if all[0].Name != "picoclaw" {
		t.Errorf("expected first config to be picoclaw, got %s", all[0].Name)
	}
}

func TestRegistryFromFS_MissingName(t *testing.T) {
	fsys := fstest.MapFS{
		"bots/bad.yaml": &fstest.MapFile{
			Data: []byte("displayName: Bad Bot\n"),
		},
	}
	_, err := newRegistryFromFS(fsys, "bots")
	if err == nil {
		t.Error("expected error for missing name field")
	}
}

func TestRegistryFromFS_InvalidYAML(t *testing.T) {
	fsys := fstest.MapFS{
		"bots/bad.yaml": &fstest.MapFile{
			Data: []byte("{{invalid yaml"),
		},
	}
	_, err := newRegistryFromFS(fsys, "bots")
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestRegistryFromFS_EmptyDir(t *testing.T) {
	fsys := fstest.MapFS{
		"bots/readme.txt": &fstest.MapFile{
			Data: []byte("not a yaml file"),
		},
	}
	r, err := newRegistryFromFS(fsys, "bots")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.BotTypes()) != 0 {
		t.Error("expected no bot types")
	}
}

func TestRegistryFromFS_MissingDir(t *testing.T) {
	fsys := fstest.MapFS{}
	_, err := newRegistryFromFS(fs.FS(fsys), "bots")
	if err == nil {
		t.Error("expected error for missing directory")
	}
}
