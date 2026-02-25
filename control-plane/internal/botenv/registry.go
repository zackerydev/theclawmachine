// Package botenv provides a registry of environment variables used by each
// bot type, categorized by purpose. The UI uses this to present smart defaults
// when users create bots — instead of asking users to guess env var names.
package botenv

// Category groups env vars by purpose.
type Category string

const (
	CategoryLLM     Category = "llm"     // LLM provider API keys
	CategoryChannel Category = "channel" // Chat channel tokens
	CategoryTool    Category = "tool"    // Tool API keys (search, TTS, etc.)
	CategorySystem  Category = "system"  // Internal / system config
)

// EnvVar describes a single environment variable a bot recognizes.
type EnvVar struct {
	// Name is the env var name (e.g. "ANTHROPIC_API_KEY")
	Name string `json:"name"`

	// Label is a human-friendly label for the UI
	Label string `json:"label"`

	// Description explains what this var does
	Description string `json:"description"`

	// Category groups vars for UI sections
	Category Category `json:"category"`

	// Secret indicates this value should be stored as a K8s Secret
	// (not a ConfigMap). Most API keys and tokens are secrets.
	Secret bool `json:"secret"`

	// Required means the bot won't start without this var
	Required bool `json:"required"`

	// Default is the default value (empty string = no default)
	Default string `json:"default,omitempty"`

	// Channel links this var to a specific channel (e.g. "discord", "telegram").
	// Empty means it's not channel-specific.
	Channel string `json:"channel,omitempty"`

	// Provider links this var to a specific LLM provider (e.g. "anthropic", "openai").
	Provider string `json:"provider,omitempty"`
}

// BotType is one of the supported bot types.
type BotType string

const (
	PicoClaw BotType = "picoclaw"
	IronClaw BotType = "ironclaw"
	OpenClaw BotType = "openclaw"
	BusyBox  BotType = "busybox"
)

// Registry returns the full env var list for a given bot type.
func GetEnvVars(botType BotType) []EnvVar {
	switch botType {
	case PicoClaw:
		return picoClawVars
	case IronClaw:
		return ironClawVars
	case OpenClaw:
		return openClawVars
	case BusyBox:
		return busyBoxVars
	default:
		return nil
	}
}

// AllBotTypes returns the list of bot types.
func AllBotTypes() []BotType {
	return []BotType{PicoClaw, IronClaw, OpenClaw, BusyBox}
}

// ByCategory filters env vars by category.
func ByCategory(vars []EnvVar, cat Category) []EnvVar {
	var out []EnvVar
	for _, v := range vars {
		if v.Category == cat {
			out = append(out, v)
		}
	}
	return out
}

// ByChannel filters env vars for a specific channel.
func ByChannel(vars []EnvVar, channel string) []EnvVar {
	var out []EnvVar
	for _, v := range vars {
		if v.Channel == channel {
			out = append(out, v)
		}
	}
	return out
}

// Secrets returns only the secret env vars.
func Secrets(vars []EnvVar) []EnvVar {
	var out []EnvVar
	for _, v := range vars {
		if v.Secret {
			out = append(out, v)
		}
	}
	return out
}

// ────────────────────────────────────────────────────────────────
// PicoClaw env vars
// Source: picoclaw/pkg/config/config.go (.env tags)
// Also accepts shorthand env vars from .env.example
// ────────────────────────────────────────────────────────────────

var picoClawVars = []EnvVar{
	// LLM Providers — PicoClaw supports many via config, but env var shortcuts exist
	{Name: "ANTHROPIC_API_KEY", Label: "Anthropic API Key", Description: "Claude models (sonnet, opus, haiku)", Category: CategoryLLM, Secret: true, Provider: "anthropic"},
	{Name: "OPENAI_API_KEY", Label: "OpenAI API Key", Description: "GPT-4o, o1, etc.", Category: CategoryLLM, Secret: true, Provider: "openai"},
	{Name: "OPENROUTER_API_KEY", Label: "OpenRouter API Key", Description: "Access 200+ models via OpenRouter", Category: CategoryLLM, Secret: true, Provider: "openrouter"},
	{Name: "GEMINI_API_KEY", Label: "Gemini API Key", Description: "Google Gemini models", Category: CategoryLLM, Secret: true, Provider: "gemini"},
	{Name: "GROQ_API_KEY", Label: "Groq API Key", Description: "Fast inference + free Whisper voice transcription", Category: CategoryLLM, Secret: true, Provider: "groq"},
	{Name: "DEEPSEEK_API_KEY", Label: "DeepSeek API Key", Description: "DeepSeek models (direct)", Category: CategoryLLM, Secret: true, Provider: "deepseek"},
	{Name: "ZHIPU_API_KEY", Label: "Zhipu API Key", Description: "GLM-4 models", Category: CategoryLLM, Secret: true, Provider: "zhipu"},

	// Channels
	{Name: "DISCORD_BOT_TOKEN", Label: "Discord Bot Token", Description: "Bot token from Discord Developer Portal", Category: CategoryChannel, Secret: true, Channel: "discord"},
	{Name: "TELEGRAM_BOT_TOKEN", Label: "Telegram Bot Token", Description: "Bot token from @BotFather", Category: CategoryChannel, Secret: true, Channel: "telegram"},
	{Name: "SLACK_BOT_TOKEN", Label: "Slack Bot Token", Description: "xoxb-... bot OAuth token", Category: CategoryChannel, Secret: true, Channel: "slack"},
	{Name: "SLACK_APP_TOKEN", Label: "Slack App Token", Description: "xapp-... app-level token for Socket Mode", Category: CategoryChannel, Secret: true, Channel: "slack"},
	{Name: "LINE_CHANNEL_SECRET", Label: "LINE Channel Secret", Description: "LINE Messaging API channel secret", Category: CategoryChannel, Secret: true, Channel: "line"},
	{Name: "LINE_CHANNEL_ACCESS_TOKEN", Label: "LINE Channel Access Token", Description: "LINE Messaging API access token", Category: CategoryChannel, Secret: true, Channel: "line"},

	// Tools
	{Name: "BRAVE_SEARCH_API_KEY", Label: "Brave Search API Key", Description: "Web search via Brave Search API", Category: CategoryTool, Secret: true},

	// System
	{Name: "PICOCLAW_AGENTS_DEFAULTS_MODEL", Label: "Default Model", Description: "Model name (e.g. claude-sonnet-4-20250514)", Category: CategorySystem},
	{Name: "PICOCLAW_AGENTS_DEFAULTS_PROVIDER", Label: "Default Provider", Description: "LLM provider name (anthropic, openai, openrouter, etc.)", Category: CategorySystem},
	{Name: "TZ", Label: "Timezone", Description: "IANA timezone (e.g. America/Chicago)", Category: CategorySystem, Default: "UTC"},
}

// ────────────────────────────────────────────────────────────────
// IronClaw env vars
// Source: ironclaw/.env.example + src/config.rs
// ────────────────────────────────────────────────────────────────

var ironClawVars = []EnvVar{
	// System (required)
	{Name: "DATABASE_URL", Label: "Database URL", Description: "PostgreSQL connection string (auto-set when built-in DB is enabled)", Category: CategorySystem, Secret: true, Required: true, Default: ""},
	{Name: "GATEWAY_AUTH_TOKEN", Label: "Gateway Auth Token", Description: "Protects the IronClaw gateway API", Category: CategorySystem, Secret: true, Required: true},

	// LLM Providers
	{Name: "LLM_BACKEND", Label: "LLM Backend", Description: "Provider: nearai, anthropic, openai, ollama, openai_compatible", Category: CategoryLLM, Default: "nearai"},
	{Name: "NEARAI_SESSION_TOKEN", Label: "NEAR AI Session Token", Description: "Session token for NEAR AI inference", Category: CategoryLLM, Secret: true, Provider: "nearai"},
	{Name: "ANTHROPIC_API_KEY", Label: "Anthropic API Key", Description: "Direct Anthropic API access", Category: CategoryLLM, Secret: true, Provider: "anthropic"},
	{Name: "OPENAI_API_KEY", Label: "OpenAI API Key", Description: "Direct OpenAI API access", Category: CategoryLLM, Secret: true, Provider: "openai"},
	{Name: "LLM_API_KEY", Label: "LLM API Key", Description: "Generic API key for openai_compatible backends", Category: CategoryLLM, Secret: true, Provider: "openai_compatible"},
	{Name: "LLM_BASE_URL", Label: "LLM Base URL", Description: "Base URL for openai_compatible backends", Category: CategoryLLM, Provider: "openai_compatible"},
	{Name: "LLM_EXTRA_HEADERS", Label: "LLM Extra Headers", Description: "Comma-separated key:value pairs (e.g. for OpenRouter)", Category: CategoryLLM, Provider: "openai_compatible"},
	{Name: "NEARAI_API_KEY", Label: "NEAR AI API Key", Description: "API key from cloud.near.ai (alternative to session token)", Category: CategoryLLM, Secret: true, Provider: "nearai"},
	{Name: "NEARAI_BASE_URL", Label: "NEAR AI Base URL", Description: "NEAR AI inference endpoint", Category: CategoryLLM, Default: "https://private.near.ai", Provider: "nearai"},
	{Name: "NEARAI_MODEL", Label: "NEAR AI Model", Description: "Model to use via NEAR AI", Category: CategoryLLM, Default: "zai-org/GLM-5-FP8", Provider: "nearai"},
	{Name: "LLM_MODEL", Label: "LLM Model", Description: "Model for openai_compatible backends", Category: CategoryLLM, Provider: "openai_compatible"},
	{Name: "OLLAMA_MODEL", Label: "Ollama Model", Description: "Model for local Ollama inference", Category: CategoryLLM, Provider: "ollama"},
	{Name: "OLLAMA_BASE_URL", Label: "Ollama Base URL", Description: "Ollama server URL", Category: CategoryLLM, Default: "http://localhost:11434", Provider: "ollama"},

	// Channels
	{Name: "TELEGRAM_BOT_TOKEN", Label: "Telegram Bot Token", Description: "Bot token from @BotFather", Category: CategoryChannel, Secret: true, Channel: "telegram"},
	{Name: "SLACK_BOT_TOKEN", Label: "Slack Bot Token", Description: "xoxb-... bot OAuth token", Category: CategoryChannel, Secret: true, Channel: "slack"},
	{Name: "SLACK_APP_TOKEN", Label: "Slack App Token", Description: "xapp-... app-level token for Socket Mode", Category: CategoryChannel, Secret: true, Channel: "slack"},
	{Name: "SLACK_SIGNING_SECRET", Label: "Slack Signing Secret", Description: "Used to verify Slack webhook requests", Category: CategoryChannel, Secret: true, Channel: "slack"},

	// System
	{Name: "HTTP_WEBHOOK_SECRET", Label: "HTTP Webhook Secret", Description: "Secret for webhook verification", Category: CategoryChannel, Secret: true, Channel: "http"},

	// System
	{Name: "DATABASE_POOL_SIZE", Label: "Database Pool Size", Description: "Number of database connections", Category: CategorySystem, Default: "10"},
	{Name: "AGENT_NAME", Label: "Agent Name", Description: "Display name for the agent", Category: CategorySystem, Default: "ironclaw"},
	{Name: "AGENT_MAX_PARALLEL_JOBS", Label: "Max Parallel Jobs", Description: "Concurrent job limit", Category: CategorySystem, Default: "5"},
	{Name: "AGENT_JOB_TIMEOUT_SECS", Label: "Job Timeout (seconds)", Description: "Max duration per job", Category: CategorySystem, Default: "3600"},
	{Name: "AGENT_USE_PLANNING", Label: "Enable Planning", Description: "Planning phase before tool execution", Category: CategorySystem, Default: "true"},
	{Name: "SAFETY_INJECTION_CHECK_ENABLED", Label: "Injection Check", Description: "Detect prompt injection attempts", Category: CategorySystem, Default: "true"},
	{Name: "HTTP_PORT", Label: "HTTP Port", Description: "Webhook server port", Category: CategorySystem, Default: "8080"},
}

// ────────────────────────────────────────────────────────────────
// OpenClaw env vars
// Source: openclaw source + podman env + config schema
// ────────────────────────────────────────────────────────────────

var openClawVars = []EnvVar{
	// System (required)
	{Name: "OPENCLAW_GATEWAY_TOKEN", Label: "Gateway Token", Description: "Auth token for the OpenClaw gateway API", Category: CategorySystem, Secret: true, Required: true},

	// LLM Providers
	{Name: "ANTHROPIC_API_KEY", Label: "Anthropic API Key", Description: "Claude models (primary provider)", Category: CategoryLLM, Secret: true, Provider: "anthropic"},
	{Name: "OPENAI_API_KEY", Label: "OpenAI API Key", Description: "GPT-4o, o1, etc.", Category: CategoryLLM, Secret: true, Provider: "openai"},
	{Name: "GEMINI_API_KEY", Label: "Gemini API Key", Description: "Google Gemini models", Category: CategoryLLM, Secret: true, Provider: "gemini"},
	{Name: "OPENROUTER_API_KEY", Label: "OpenRouter API Key", Description: "Access 200+ models via OpenRouter", Category: CategoryLLM, Secret: true, Provider: "openrouter"},
	{Name: "GROQ_API_KEY", Label: "Groq API Key", Description: "Fast inference (Llama, Mixtral)", Category: CategoryLLM, Secret: true, Provider: "groq"},
	{Name: "OLLAMA_API_KEY", Label: "Ollama API Key", Description: "Local Ollama inference", Category: CategoryLLM, Secret: true, Provider: "ollama"},

	// Channels
	{Name: "DISCORD_BOT_TOKEN", Label: "Discord Bot Token", Description: "Bot token from Discord Developer Portal", Category: CategoryChannel, Secret: true, Channel: "discord"},
	{Name: "TELEGRAM_BOT_TOKEN", Label: "Telegram Bot Token", Description: "Bot token from @BotFather", Category: CategoryChannel, Secret: true, Channel: "telegram"},
	{Name: "SLACK_BOT_TOKEN", Label: "Slack Bot Token", Description: "xoxb-... bot OAuth token", Category: CategoryChannel, Secret: true, Channel: "slack"},
	{Name: "SLACK_APP_TOKEN", Label: "Slack App Token", Description: "xapp-... app-level token for Socket Mode", Category: CategoryChannel, Secret: true, Channel: "slack"},
	{Name: "IRC_HOST", Label: "IRC Host", Description: "IRC server hostname", Category: CategoryChannel, Channel: "irc"},
	{Name: "IRC_NICK", Label: "IRC Nick", Description: "Bot nickname on IRC", Category: CategoryChannel, Channel: "irc"},
	{Name: "IRC_PASSWORD", Label: "IRC Password", Description: "IRC server password", Category: CategoryChannel, Secret: true, Channel: "irc"},
	{Name: "MATRIX_HOMESERVER", Label: "Matrix Homeserver", Description: "Matrix homeserver URL", Category: CategoryChannel, Channel: "matrix"},
	{Name: "MATRIX_ACCESS_TOKEN", Label: "Matrix Access Token", Description: "Matrix bot access token", Category: CategoryChannel, Secret: true, Channel: "matrix"},
	{Name: "MATTERMOST_URL", Label: "Mattermost URL", Description: "Mattermost server URL", Category: CategoryChannel, Channel: "mattermost"},
	{Name: "MATTERMOST_BOT_TOKEN", Label: "Mattermost Bot Token", Description: "Mattermost bot access token", Category: CategoryChannel, Secret: true, Channel: "mattermost"},

	// Tools
	{Name: "BRAVE_API_KEY", Label: "Brave Search API Key", Description: "Web search via Brave Search API", Category: CategoryTool, Secret: true},
	{Name: "ELEVENLABS_API_KEY", Label: "ElevenLabs API Key", Description: "Text-to-speech via ElevenLabs", Category: CategoryTool, Secret: true},
	{Name: "DEEPGRAM_API_KEY", Label: "Deepgram API Key", Description: "Speech-to-text via Deepgram", Category: CategoryTool, Secret: true},
	{Name: "FIRECRAWL_API_KEY", Label: "Firecrawl API Key", Description: "Web scraping via Firecrawl", Category: CategoryTool, Secret: true},
	{Name: "GH_TOKEN", Label: "GitHub Token", Description: "GitHub personal access token for gh CLI", Category: CategoryTool, Secret: true},

	// System
	{Name: "OPENCLAW_GATEWAY_PORT", Label: "Gateway Port", Description: "Gateway listen port", Category: CategorySystem, Default: "18789"},
}

// ────────────────────────────────────────────────────────────────
// BusyBox (toolbox) — no bot-specific vars, but users may want to
// set keys for the pre-installed CLIs they'll use inside the container.
// ────────────────────────────────────────────────────────────────

var busyBoxVars = []EnvVar{
	// LLM Providers (for whichever CLI they run inside)
	{Name: "ANTHROPIC_API_KEY", Label: "Anthropic API Key", Description: "For any CLI that needs Claude access", Category: CategoryLLM, Secret: true, Provider: "anthropic"},
	{Name: "OPENAI_API_KEY", Label: "OpenAI API Key", Description: "For any CLI that needs OpenAI access", Category: CategoryLLM, Secret: true, Provider: "openai"},

	// Channel tokens (in case user runs a bot manually)
	{Name: "DISCORD_BOT_TOKEN", Label: "Discord Bot Token", Description: "If running a bot CLI manually inside BusyBox", Category: CategoryChannel, Secret: true, Channel: "discord"},
	{Name: "TELEGRAM_BOT_TOKEN", Label: "Telegram Bot Token", Description: "If running a bot CLI manually inside BusyBox", Category: CategoryChannel, Secret: true, Channel: "telegram"},

	// Tools
	{Name: "GH_TOKEN", Label: "GitHub Token", Description: "GitHub personal access token", Category: CategoryTool, Secret: true},
}
