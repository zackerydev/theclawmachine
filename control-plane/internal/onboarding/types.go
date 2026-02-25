package onboarding

import "github.com/zackerydev/clawmachine/control-plane/internal/service"

// CapabilityID identifies a reusable onboarding capability.
type CapabilityID string

const (
	CapabilityLLM             CapabilityID = "llm"
	CapabilityGateway         CapabilityID = "gateway"
	CapabilityChannelDiscord  CapabilityID = "channels.discord"
	CapabilityChannelTelegram CapabilityID = "channels.telegram"
	CapabilityChannelSlack    CapabilityID = "channels.slack"
	CapabilityChannelWhatsApp CapabilityID = "channels.whatsapp"
	CapabilityChannelLine     CapabilityID = "channels.line"
	CapabilityChannelDingTalk CapabilityID = "channels.dingtalk"
	CapabilityChannelFeishu   CapabilityID = "channels.feishu"
	CapabilityStoragePostgres CapabilityID = "storage.postgres"
	CapabilityAgent           CapabilityID = "agent"
	CapabilitySafety          CapabilityID = "safety"
	CapabilityInfra           CapabilityID = "infra"
	CapabilityAdvanced        CapabilityID = "advanced"
)

// QuestionTier controls where a question appears in onboarding.
type QuestionTier string

const (
	TierGuided   QuestionTier = "guided"
	TierAdvanced QuestionTier = "advanced"
)

// QuestionType is the input type used by the TUI/API onboarding client.
type QuestionType string

const (
	QuestionText     QuestionType = "text"
	QuestionPassword QuestionType = "password"
	QuestionSelect   QuestionType = "select"
	QuestionCheckbox QuestionType = "checkbox"
	QuestionTextarea QuestionType = "textarea"
	QuestionModel    QuestionType = "model"
)

// Condition expresses a field dependency (show this when another answer matches).
type Condition struct {
	QuestionID string `json:"questionId"`
	Equals     string `json:"equals"`
}

// QuestionOption is a select option.
type QuestionOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// QuestionTarget describes where an answer is mapped in runtime config.
type QuestionTarget struct {
	ValuesPath string `json:"valuesPath,omitempty"`
	ConfigPath string `json:"configPath,omitempty"`
	EnvVar     string `json:"envVar,omitempty"`
	Secret     bool   `json:"secret,omitempty"`
}

// Question describes one onboarding prompt.
type Question struct {
	ID          string           `json:"id"`
	Label       string           `json:"label"`
	Type        QuestionType     `json:"type"`
	SectionKey  string           `json:"sectionKey,omitempty"`
	SectionName string           `json:"sectionName,omitempty"`
	Placeholder string           `json:"placeholder,omitempty"`
	HelpText    string           `json:"helpText,omitempty"`
	Required    bool             `json:"required"`
	Default     string           `json:"default,omitempty"`
	Secret      bool             `json:"secret,omitempty"`
	Monospace   bool             `json:"monospace,omitempty"`
	ShowWhen    *Condition       `json:"showWhen,omitempty"`
	Options     []QuestionOption `json:"options,omitempty"`
	Capability  CapabilityID     `json:"capability"`
	Tier        QuestionTier     `json:"tier"`
	Targets     []QuestionTarget `json:"targets,omitempty"`
}

// CapabilityMeta describes a capability used by a bot profile.
type CapabilityMeta struct {
	ID          CapabilityID `json:"id"`
	Label       string       `json:"label"`
	Description string       `json:"description,omitempty"`
}

// SectionMeta groups profile questions.
type SectionMeta struct {
	Key                string   `json:"key"`
	Label              string   `json:"label"`
	Description        string   `json:"description,omitempty"`
	QuestionIDs        []string `json:"questionIds"`
	CollapsedByDefault bool     `json:"collapsedByDefault,omitempty"`
}

// Preset is an opinionated quick-start path.
type Preset struct {
	ID                     string            `json:"id"`
	Label                  string            `json:"label"`
	Description            string            `json:"description,omitempty"`
	Capabilities           []CapabilityID    `json:"capabilities,omitempty"`
	DefaultAnswerOverrides map[string]string `json:"defaultAnswerOverrides,omitempty"`
}

// OnboardingProfile is the public, canonical onboarding contract for one bot type.
type OnboardingProfile struct {
	Version             string           `json:"version"`
	BotType             service.BotType  `json:"botType"`
	DisplayName         string           `json:"displayName"`
	Capabilities        []CapabilityMeta `json:"capabilities"`
	Sections            []SectionMeta    `json:"sections"`
	Questions           []Question       `json:"questions"`
	GuidedQuestionIDs   []string         `json:"guidedQuestionIds"`
	AdvancedQuestionIDs []string         `json:"advancedQuestionIds"`
	Presets             []Preset         `json:"presets,omitempty"`
}

// SecretRef points to a synchronized K8s Secret key.
type SecretRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// EnvSecretBinding maps one env var to a secret reference.
type EnvSecretBinding struct {
	FieldKey  string    `json:"fieldKey,omitempty"`
	EnvVar    string    `json:"envVar"`
	SecretRef SecretRef `json:"secretRef"`
}

// ConfigFileOutput holds generated config file output.
type ConfigFileOutput struct {
	Enabled bool   `json:"enabled"`
	Format  string `json:"format"`
	Content string `json:"content"`
}

// CompileResult is the deterministic compiler output from canonical answers.
type CompileResult struct {
	BotType      service.BotType    `json:"botType"`
	CleanAnswers map[string]string  `json:"cleanAnswers,omitempty"`
	SecretRefs   map[string]string  `json:"secretRefs,omitempty"`
	ValuesPatch  map[string]any     `json:"valuesPatch,omitempty"`
	ConfigFile   *ConfigFileOutput  `json:"configFile,omitempty"`
	EnvSecrets   []EnvSecretBinding `json:"envSecrets,omitempty"`
}
