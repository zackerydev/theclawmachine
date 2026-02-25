package onboarding

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"

	"github.com/zackerydev/clawmachine/control-plane/internal/botenv"
	"github.com/zackerydev/clawmachine/control-plane/internal/service"
)

const ProfileVersion = "v1"

// Adapter compiles normalized onboarding answers for one bot type.
type Adapter interface {
	BotType() service.BotType
	Profile() (*OnboardingProfile, error)
	Compile(answers map[string]string) (*CompileResult, error)
}

// Engine provides canonical onboarding profiles and deterministic compilers.
type Engine struct {
	adapters map[service.BotType]Adapter
}

// NewEngine builds an onboarding engine from the bot registry.
func NewEngine(reg *botenv.Registry) *Engine {
	adapters := make(map[service.BotType]Adapter)
	if reg == nil {
		return &Engine{adapters: adapters}
	}
	for _, cfg := range reg.All() {
		bt := service.BotType(cfg.Name)
		adapters[bt] = newBotAdapter(bt, cfg)
	}
	return &Engine{adapters: adapters}
}

// SupportedBotTypes returns all bot types with onboarding adapters.
func (e *Engine) SupportedBotTypes() []service.BotType {
	types := make([]service.BotType, 0, len(e.adapters))
	for bt := range e.adapters {
		types = append(types, bt)
	}
	slices.Sort(types)
	return types
}

// Profile returns the canonical onboarding profile for a bot type.
func (e *Engine) Profile(botType service.BotType) (*OnboardingProfile, error) {
	adapter, ok := e.adapters[botType]
	if !ok {
		return nil, fmt.Errorf("unsupported bot type: %s", botType)
	}
	return adapter.Profile()
}

// Compile compiles normalized answers into install/runtime outputs.
func (e *Engine) Compile(botType service.BotType, answers map[string]string) (*CompileResult, error) {
	adapter, ok := e.adapters[botType]
	if !ok {
		return nil, fmt.Errorf("unsupported bot type: %s", botType)
	}
	return adapter.Compile(answers)
}

// SplitAnswers splits answer values into plaintext answers and secret references.
// Secret references are values prefixed with "1p:".
func SplitAnswers(answers map[string]string) (clean map[string]string, secretRefs map[string]string) {
	clean = make(map[string]string)
	secretRefs = make(map[string]string)
	for k, v := range answers {
		if after, ok := strings.CutPrefix(v, "1p:"); ok {
			secretRefs[k] = after
			continue
		}
		clean[k] = v
	}
	return clean, secretRefs
}

// MergeAnswers reassembles answers from plaintext and secret refs.
func MergeAnswers(clean, secretRefs map[string]string) map[string]string {
	out := make(map[string]string, len(clean)+len(secretRefs))
	maps.Copy(out, clean)
	for k, ref := range secretRefs {
		out[k] = "1p:" + ref
	}
	return out
}

// DeepMergeValues merges patch into dst recursively.
func DeepMergeValues(dst, patch map[string]any) {
	for k, v := range patch {
		if childPatch, ok := v.(map[string]any); ok {
			if childDst, ok := dst[k].(map[string]any); ok {
				DeepMergeValues(childDst, childPatch)
			} else {
				clone := make(map[string]any)
				DeepMergeValues(clone, childPatch)
				dst[k] = clone
			}
			continue
		}
		dst[k] = v
	}
}

type botAdapter struct {
	botType         service.BotType
	bot             *botenv.BotConfig
	envVarOverrides map[string]string
}

func newBotAdapter(botType service.BotType, cfg *botenv.BotConfig) *botAdapter {
	return &botAdapter{
		botType:         botType,
		bot:             cfg,
		envVarOverrides: envVarOverridesFor(botType),
	}
}

func (a *botAdapter) BotType() service.BotType {
	return a.botType
}

func (a *botAdapter) Profile() (*OnboardingProfile, error) {
	if a.bot == nil {
		return nil, fmt.Errorf("bot config missing for %s", a.botType)
	}

	profile := &OnboardingProfile{
		Version:     ProfileVersion,
		BotType:     a.botType,
		DisplayName: a.bot.DisplayName,
	}

	capSet := make(map[CapabilityID]struct{})
	questionByID := make(map[string]Question)
	sections := make([]SectionMeta, 0, len(a.bot.Sections))

	addQuestion := func(sectionKey, sectionLabel string, field botenv.Field) {
		cap := classifyCapability(field)
		tier := classifyTier(field, cap)
		q := Question{
			ID:          field.Key,
			Label:       field.Label,
			Type:        normalizeQuestionType(field.Type),
			SectionKey:  sectionKey,
			SectionName: sectionLabel,
			Placeholder: field.Placeholder,
			HelpText:    field.HelpText,
			Required:    field.Required,
			Default:     field.Default,
			Secret:      field.Secret,
			Monospace:   field.FontMono,
			Capability:  cap,
			Tier:        tier,
			Targets: []QuestionTarget{{
				ValuesPath: field.ValuesPath,
				ConfigPath: field.ConfigPath,
				EnvVar:     a.envVarForField(field),
				Secret:     field.Secret,
			}},
		}
		if cond := parseShowWhen(field.ShowWhen); cond != nil {
			q.ShowWhen = cond
		}
		if len(field.Options) > 0 {
			q.Options = make([]QuestionOption, len(field.Options))
			for i, opt := range field.Options {
				q.Options[i] = QuestionOption{Label: opt.Label, Value: opt.Value}
			}
		}
		questionByID[q.ID] = q
		capSet[cap] = struct{}{}
	}

	if len(a.bot.Sections) > 0 {
		for _, sec := range a.bot.Sections {
			meta := SectionMeta{
				Key:                sec.Key,
				Label:              sec.Label,
				Description:        sec.Description,
				CollapsedByDefault: sec.CollapsedByDefault,
			}
			for _, f := range sec.Fields {
				addQuestion(sec.Key, sec.Label, f)
				meta.QuestionIDs = append(meta.QuestionIDs, f.Key)
			}
			sections = append(sections, meta)
		}
	} else {
		meta := SectionMeta{Key: "general", Label: "General"}
		for _, f := range a.bot.Fields {
			addQuestion(meta.Key, meta.Label, f)
			meta.QuestionIDs = append(meta.QuestionIDs, f.Key)
		}
		sections = append(sections, meta)
	}

	questionOrder := make([]string, 0, len(questionByID))
	for _, sec := range sections {
		questionOrder = append(questionOrder, sec.QuestionIDs...)
	}

	profile.Questions = make([]Question, 0, len(questionOrder))
	for _, qid := range questionOrder {
		q := questionByID[qid]
		profile.Questions = append(profile.Questions, q)
		if q.Tier == TierGuided {
			profile.GuidedQuestionIDs = append(profile.GuidedQuestionIDs, q.ID)
		} else {
			profile.AdvancedQuestionIDs = append(profile.AdvancedQuestionIDs, q.ID)
		}
	}

	caps := make([]CapabilityID, 0, len(capSet))
	for cap := range capSet {
		caps = append(caps, cap)
	}
	slices.Sort(caps)
	for _, cap := range caps {
		profile.Capabilities = append(profile.Capabilities, capabilityMeta(cap))
	}
	profile.Sections = sections
	profile.Presets = defaultPresets(profile.BotType)

	return profile, nil
}

func (a *botAdapter) Compile(answers map[string]string) (*CompileResult, error) {
	if a.bot == nil {
		return nil, fmt.Errorf("bot config missing for %s", a.botType)
	}

	clean, secretRefs := SplitAnswers(answers)
	result := &CompileResult{
		BotType:      a.botType,
		CleanAnswers: clean,
		SecretRefs:   secretRefs,
		ValuesPatch:  make(map[string]any),
	}

	for _, field := range a.bot.AllFields() {
		val, ok := clean[field.Key]
		if !ok || val == "" {
			continue
		}
		if field.ValuesPath != "" && field.ValuesPath != "configFile.content" {
			setNestedValue(result.ValuesPatch, field.ValuesPath, valueForValuesPath(field, val))
		}
	}

	if a.bot.HasConfigFile() {
		content, err := botenv.BuildConfigContent(a.bot, clean)
		if err != nil {
			return nil, err
		}
		if content != "" {
			result.ConfigFile = &ConfigFileOutput{
				Enabled: content != "{}",
				Format:  a.bot.ConfigFormat,
				Content: content,
			}
		}
	}

	for fieldKey, secretName := range secretRefs {
		field := findField(a.bot, fieldKey)
		if field == nil {
			continue
		}
		envVar := a.envVarForField(*field)
		if envVar == "" {
			continue
		}
		result.EnvSecrets = append(result.EnvSecrets, EnvSecretBinding{
			FieldKey: fieldKey,
			EnvVar:   envVar,
			SecretRef: SecretRef{
				Name: secretName,
				Key:  "value",
			},
		})
	}

	if len(result.ValuesPatch) == 0 {
		result.ValuesPatch = nil
	}
	return result, nil
}

func findField(cfg *botenv.BotConfig, key string) *botenv.Field {
	for _, field := range cfg.AllFields() {
		if field.Key == key {
			f := field
			return &f
		}
	}
	return nil
}

func parseShowWhen(showWhen string) *Condition {
	if showWhen == "" {
		return nil
	}
	lhs, rhs, ok := strings.Cut(showWhen, "=")
	if !ok || lhs == "" {
		return nil
	}
	return &Condition{QuestionID: lhs, Equals: rhs}
}

func normalizeQuestionType(t string) QuestionType {
	switch t {
	case "password":
		return QuestionPassword
	case "select":
		return QuestionSelect
	case "checkbox":
		return QuestionCheckbox
	case "textarea":
		return QuestionTextarea
	case "model":
		return QuestionModel
	default:
		return QuestionText
	}
}

func classifyTier(field botenv.Field, cap CapabilityID) QuestionTier {
	if field.Required {
		return TierGuided
	}
	k := strings.ToLower(field.Key)
	if strings.Contains(k, "model") || strings.Contains(k, "backend") || strings.Contains(k, "authchoice") {
		return TierGuided
	}
	if cap == CapabilityGateway || cap == CapabilityLLM {
		return TierGuided
	}
	if strings.HasSuffix(field.Key, "Enabled") {
		switch cap {
		case CapabilityChannelDiscord,
			CapabilityChannelTelegram,
			CapabilityChannelSlack,
			CapabilityChannelWhatsApp,
			CapabilityChannelLine,
			CapabilityChannelDingTalk,
			CapabilityChannelFeishu:
			return TierGuided
		}
	}
	return TierAdvanced
}

func classifyCapability(field botenv.Field) CapabilityID {
	key := strings.ToLower(field.Key)
	path := strings.ToLower(field.ValuesPath + "." + field.ConfigPath)

	switch {
	case strings.Contains(key, "discord") || strings.Contains(path, "channels.discord"):
		return CapabilityChannelDiscord
	case strings.Contains(key, "telegram") || strings.Contains(path, "channels.telegram"):
		return CapabilityChannelTelegram
	case strings.Contains(key, "slack") || strings.Contains(path, "channels.slack"):
		return CapabilityChannelSlack
	case strings.Contains(key, "whatsapp") || strings.Contains(path, "channels.whatsapp"):
		return CapabilityChannelWhatsApp
	case strings.Contains(key, "line") || strings.Contains(path, "channels.line"):
		return CapabilityChannelLine
	case strings.Contains(key, "dingtalk") || strings.Contains(path, "channels.dingtalk"):
		return CapabilityChannelDingTalk
	case strings.Contains(key, "feishu") || strings.Contains(path, "channels.feishu"):
		return CapabilityChannelFeishu
	case strings.Contains(key, "gateway") || strings.Contains(path, "gateway"):
		return CapabilityGateway
	case strings.Contains(key, "llm") || strings.Contains(key, "model") || strings.Contains(key, "api") || strings.Contains(path, "auth"):
		return CapabilityLLM
	case strings.Contains(key, "database") || strings.Contains(key, "postgres") || strings.Contains(path, "database") || strings.Contains(path, "postgres"):
		return CapabilityStoragePostgres
	case strings.Contains(key, "safety") || strings.Contains(key, "heartbeat"):
		return CapabilitySafety
	case strings.Contains(key, "agent") || strings.Contains(path, "agent"):
		return CapabilityAgent
	case strings.Contains(path, "backup") || strings.Contains(path, "workspace"):
		return CapabilityInfra
	default:
		return CapabilityAdvanced
	}
}

func capabilityMeta(cap CapabilityID) CapabilityMeta {
	switch cap {
	case CapabilityLLM:
		return CapabilityMeta{ID: cap, Label: "LLM", Description: "Provider and model configuration"}
	case CapabilityGateway:
		return CapabilityMeta{ID: cap, Label: "Gateway", Description: "Ingress and bot API access controls"}
	case CapabilityChannelDiscord:
		return CapabilityMeta{ID: cap, Label: "Discord"}
	case CapabilityChannelTelegram:
		return CapabilityMeta{ID: cap, Label: "Telegram"}
	case CapabilityChannelSlack:
		return CapabilityMeta{ID: cap, Label: "Slack"}
	case CapabilityChannelWhatsApp:
		return CapabilityMeta{ID: cap, Label: "WhatsApp"}
	case CapabilityChannelLine:
		return CapabilityMeta{ID: cap, Label: "LINE"}
	case CapabilityChannelDingTalk:
		return CapabilityMeta{ID: cap, Label: "DingTalk"}
	case CapabilityChannelFeishu:
		return CapabilityMeta{ID: cap, Label: "Feishu/Lark"}
	case CapabilityStoragePostgres:
		return CapabilityMeta{ID: cap, Label: "PostgreSQL"}
	case CapabilityAgent:
		return CapabilityMeta{ID: cap, Label: "Agent"}
	case CapabilitySafety:
		return CapabilityMeta{ID: cap, Label: "Safety"}
	case CapabilityInfra:
		return CapabilityMeta{ID: cap, Label: "Infrastructure"}
	default:
		return CapabilityMeta{ID: cap, Label: "Advanced"}
	}
}

func defaultPresets(botType service.BotType) []Preset {
	switch botType {
	case service.BotTypePicoClaw:
		return []Preset{
			{
				ID:           "quickstart",
				Label:        "Quickstart",
				Description:  "Model + one channel with safe defaults",
				Capabilities: []CapabilityID{CapabilityLLM, CapabilityChannelDiscord},
			},
		}
	case service.BotTypeIronClaw:
		return []Preset{
			{
				ID:           "nearai-default",
				Label:        "NEAR AI Default",
				Description:  "NEAR AI backend with built-in PostgreSQL",
				Capabilities: []CapabilityID{CapabilityLLM, CapabilityStoragePostgres, CapabilityGateway},
			},
		}
	case service.BotTypeOpenClaw:
		return []Preset{
			{
				ID:           "gateway-discord",
				Label:        "Gateway + Discord",
				Description:  "API gateway with Discord channel enabled",
				Capabilities: []CapabilityID{CapabilityGateway, CapabilityLLM, CapabilityChannelDiscord},
			},
		}
	default:
		return nil
	}
}

func valueForValuesPath(field botenv.Field, val string) any {
	if field.Type == "checkbox" {
		return val == "true" || val == "on" || val == "1"
	}
	return val
}

func setNestedValue(dst map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	for i, part := range parts {
		if i == len(parts)-1 {
			dst[part] = value
			return
		}
		next, ok := dst[part].(map[string]any)
		if !ok {
			next = make(map[string]any)
			dst[part] = next
		}
		dst = next
	}
}

func envVarOverridesFor(botType service.BotType) map[string]string {
	base := map[string]string{
		"gatewayToken":         "OPENCLAW_GATEWAY_TOKEN",
		"gatewayAuthToken":     "GATEWAY_AUTH_TOKEN",
		"compatApiKey":         "LLM_API_KEY",
		"compatBaseUrl":        "LLM_BASE_URL",
		"compatExtraHeaders":   "LLM_EXTRA_HEADERS",
		"lineAccessToken":      "LINE_CHANNEL_ACCESS_TOKEN",
		"lineChannelSecret":    "LINE_CHANNEL_SECRET",
		"telegramToken":        "TELEGRAM_BOT_TOKEN",
		"discordToken":         "DISCORD_BOT_TOKEN",
		"braveApiKey":          "BRAVE_API_KEY",
		"ollamaApiBase":        "OLLAMA_BASE_URL",
		"httpWebhookSecret":    "HTTP_WEBHOOK_SECRET",
		"dingtalkClientSecret": "DINGTALK_CLIENT_SECRET",
		"feishuAppSecret":      "FEISHU_APP_SECRET",
	}

	switch botType {
	case service.BotTypeOpenClaw:
		base["discordBotToken"] = "DISCORD_BOT_TOKEN"
		base["telegramBotToken"] = "TELEGRAM_BOT_TOKEN"
		base["slackBotToken"] = "SLACK_BOT_TOKEN"
		base["slackAppToken"] = "SLACK_APP_TOKEN"
	case service.BotTypeIronClaw:
		base["nearaiSessionToken"] = "NEARAI_SESSION_TOKEN"
		base["nearaiApiKey"] = "NEARAI_API_KEY"
		base["anthropicApiKey"] = "ANTHROPIC_API_KEY"
		base["openaiApiKey"] = "OPENAI_API_KEY"
		base["slackSigningSecret"] = "SLACK_SIGNING_SECRET"
	case service.BotTypePicoClaw:
		base["slackBotToken"] = "SLACK_BOT_TOKEN"
		base["slackAppToken"] = "SLACK_APP_TOKEN"
		base["telegramToken"] = "TELEGRAM_BOT_TOKEN"
		base["discordToken"] = "DISCORD_BOT_TOKEN"
	}

	return base
}

func (a *botAdapter) envVarForField(field botenv.Field) string {
	if field.EnvVar != "" {
		return field.EnvVar
	}
	if v, ok := a.envVarOverrides[field.Key]; ok {
		return v
	}

	if after, ok := strings.CutPrefix(field.ValuesPath, "secrets."); ok {
		suffix := after
		if suffix != "" {
			return toEnvVar(suffix)
		}
	}
	if field.Secret {
		return toEnvVar(field.Key)
	}
	return ""
}

func toEnvVar(s string) string {
	var b strings.Builder
	for i, r := range s {
		switch {
		case r == '-' || r == '.' || r == ' ':
			b.WriteRune('_')
		case unicode.IsUpper(r):
			if i > 0 {
				prev := rune(s[i-1])
				if prev != '_' && prev != '-' && prev != '.' && prev != ' ' && !unicode.IsUpper(prev) {
					b.WriteRune('_')
				}
			}
			b.WriteRune(unicode.ToUpper(r))
		default:
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	return b.String()
}
