// Package botenv provides a data-driven registry of bot types and their
// configuration fields. Bot definitions are loaded from embedded YAML files
// so that adding a new bot type requires only a new YAML file — no Go changes.
package botenv

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed bots/*.yaml
var botsFS embed.FS

// Field describes a single configuration field in the bot install form.
type Field struct {
	// Key is the unique identifier for form binding.
	Key string `yaml:"key" json:"key"`
	// Label is the human-readable label shown in the form.
	Label string `yaml:"label" json:"label"`
	// Type is the HTML input type: text, password, select, checkbox, textarea.
	Type string `yaml:"type" json:"type"`
	// Placeholder is shown when the field is empty.
	Placeholder string `yaml:"placeholder,omitempty" json:"placeholder,omitempty"`
	// HelpText is displayed below the field.
	HelpText string `yaml:"helpText,omitempty" json:"helpText,omitempty"`
	// Required marks the field as required in the form.
	Required bool `yaml:"required,omitempty" json:"required,omitempty"`
	// Default is the default value for the field.
	Default string `yaml:"default,omitempty" json:"default,omitempty"`
	// Options is used for select fields.
	Options []SelectOption `yaml:"options,omitempty" json:"options,omitempty"`
	// ShowWhen makes this field conditionally visible.
	// Format: "fieldKey=value" — shown only when another field has the given value.
	ShowWhen string `yaml:"showWhen,omitempty" json:"showWhen,omitempty"`
	// ValuesPath is the dot-separated Helm values path (e.g. "database.url").
	ValuesPath string `yaml:"valuesPath" json:"valuesPath"`
	// ConfigPath is the dot-separated path in the generated config file (e.g. "channels.discord.token").
	// If empty, the field is only mapped to Helm values (not the config file).
	ConfigPath string `yaml:"configPath,omitempty" json:"configPath,omitempty"`
	// FontMono renders the input in monospace font.
	FontMono bool `yaml:"fontMono,omitempty" json:"fontMono,omitempty"`
	// Secret marks this field as sensitive — stored in K8s Secret, masked in UI.
	Secret bool `yaml:"secret,omitempty" json:"secret,omitempty"`
	// EnvVar is the upstream environment variable name used when wiring secret refs.
	// Optional: if empty, onboarding adapters infer from field key/valuesPath.
	EnvVar string `yaml:"envVar,omitempty" json:"envVar,omitempty"`
}

// Section groups related fields under a collapsible heading in the form.
type Section struct {
	// Key is the unique identifier for the section.
	Key string `yaml:"key" json:"key"`
	// Label is the section heading.
	Label string `yaml:"label" json:"label"`
	// Description is optional help text shown below the heading.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Icon is an optional SVG icon name (heroicons).
	Icon string `yaml:"icon,omitempty" json:"icon,omitempty"`
	// Collapsible makes the section collapsible (default open).
	Collapsible bool `yaml:"collapsible,omitempty" json:"collapsible,omitempty"`
	// CollapsedByDefault starts the section collapsed.
	CollapsedByDefault bool `yaml:"collapsedByDefault,omitempty" json:"collapsedByDefault,omitempty"`
	// Fields are the configuration fields in this section.
	Fields []Field `yaml:"fields" json:"fields"`
}

// SelectOption is a single option in a select field.
type SelectOption struct {
	Label string `yaml:"label" json:"label"`
	Value string `yaml:"value" json:"value"`
}

// ModelProvider maps a form field (API key) to a model_list entry for bots
// that use the LiteLLM model_list format (e.g. PicoClaw).
type ModelProvider struct {
	// Name is a human identifier (e.g. "anthropic").
	Name string `yaml:"name"`
	// ModelFilters is a list of model-value prefixes that belong to this provider.
	// If the selected model starts with any of these, it is used as model_name.
	ModelFilters []string `yaml:"modelFilters"`
	// ModelPrefix is prepended to the model value to form the "model" field
	// in the model_list entry (e.g. "anthropic/" → "anthropic/claude-sonnet-4-20250514").
	ModelPrefix string `yaml:"modelPrefix"`
	// APIBase is the provider's API base URL.
	APIBase string `yaml:"apiBase,omitempty"`
	// APIKeyField is the form field key whose value becomes the api_key.
	APIKeyField string `yaml:"apiKeyField"`
	// DefaultModel is used as model_name when the selected model doesn't match
	// this provider's filters but the user has still provided an API key.
	DefaultModel string `yaml:"defaultModel,omitempty"`
}

// BotConfig defines a bot type's metadata and configuration fields.
type BotConfig struct {
	// Name is the bot type identifier (e.g. "picoclaw").
	Name string `yaml:"name" json:"name"`
	// DisplayName is the human-readable name (e.g. "PicoClaw").
	DisplayName string `yaml:"displayName" json:"displayName"`
	// Description is a short description of the bot.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Order controls the display order in the UI (lower = first).
	Order int `yaml:"order,omitempty" json:"order,omitempty"`
	// ConfigFormat is the format of the generated config file: json, toml, or env.
	// Empty means the bot uses env vars only (no config file).
	ConfigFormat string `yaml:"configFormat,omitempty" json:"configFormat,omitempty"`
	// Fields are the bot-specific configuration fields shown in the install form (flat, legacy).
	Fields []Field `yaml:"fields,omitempty" json:"fields"`
	// Sections are grouped configuration fields for the step-2 form.
	// When present, these are used instead of Fields for the bot config step.
	Sections []Section `yaml:"sections,omitempty" json:"sections,omitempty"`
	// AgentModelField is the form field key whose value is the selected default model.
	// Used together with ModelProviders to build the LiteLLM model_list config array.
	AgentModelField string `yaml:"agentModelField,omitempty" json:"agentModelField,omitempty"`
	// ModelProviders maps form fields (API keys) to model_list entries for bots
	// that use the LiteLLM model_list format (e.g. PicoClaw).
	ModelProviders []ModelProvider `yaml:"modelProviders,omitempty" json:"modelProviders,omitempty"`
}

// AllFields returns all fields from both Fields and Sections (for backward compat).
func (bc *BotConfig) AllFields() []Field {
	if len(bc.Sections) == 0 {
		return bc.Fields
	}
	var all []Field
	all = append(all, bc.Fields...)
	for _, s := range bc.Sections {
		all = append(all, s.Fields...)
	}
	return all
}

// HasConfigFile returns true if this bot type generates a config file.
func (bc *BotConfig) HasConfigFile() bool {
	return bc.ConfigFormat == "json" || bc.ConfigFormat == "toml"
}

// Registry holds all known bot configurations.
type Registry struct {
	bots map[string]*BotConfig
}

// NewRegistry loads all bot YAML files from the embedded filesystem.
func NewRegistry() (*Registry, error) {
	return newRegistryFromFS(botsFS, "bots")
}

func newRegistryFromFS(fsys fs.FS, dir string) (*Registry, error) {
	r := &Registry{bots: make(map[string]*BotConfig)}

	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("reading bot configs dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		data, err := fs.ReadFile(fsys, filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		var cfg BotConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}

		if cfg.Name == "" {
			return nil, fmt.Errorf("%s: missing required 'name' field", entry.Name())
		}

		r.bots[cfg.Name] = &cfg
	}

	return r, nil
}

// Get returns the config for a bot type, or nil if not found.
func (r *Registry) Get(name string) *BotConfig {
	return r.bots[name]
}

// BotTypes returns all bot type names, sorted by Order then Name.
func (r *Registry) BotTypes() []string {
	configs := make([]*BotConfig, 0, len(r.bots))
	for _, c := range r.bots {
		configs = append(configs, c)
	}
	sort.Slice(configs, func(i, j int) bool {
		if configs[i].Order != configs[j].Order {
			return configs[i].Order < configs[j].Order
		}
		return configs[i].Name < configs[j].Name
	})
	names := make([]string, len(configs))
	for i, c := range configs {
		names[i] = c.Name
	}
	return names
}

// All returns all bot configs, sorted by Order then Name.
func (r *Registry) All() []*BotConfig {
	names := r.BotTypes()
	configs := make([]*BotConfig, len(names))
	for i, n := range names {
		configs[i] = r.bots[n]
	}
	return configs
}

// IsValid returns true if the given name is a known bot type.
func (r *Registry) IsValid(name string) bool {
	_, ok := r.bots[name]
	return ok
}
