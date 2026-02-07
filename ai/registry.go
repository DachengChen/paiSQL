package ai

import (
	"fmt"

	"github.com/DachengChen/paiSQL/config"
)

// SupportedProviders lists available provider names for display.
var SupportedProviders = []string{"openai", "anthropic", "gemini", "ollama", "placeholder"}

// NewProvider creates an AI provider from the application config.
// Falls back to placeholder if the selected provider isn't configured.
func NewProvider(cfg config.AIConfig) (Provider, error) {
	switch cfg.Provider {
	case "openai":
		if cfg.OpenAI.APIKey == "" {
			return nil, fmt.Errorf("OpenAI API key not set. Set OPENAI_API_KEY env var or add it to ~/.paisql/config.json")
		}
		return NewOpenAI(cfg.OpenAI.APIKey, cfg.OpenAI.Model), nil

	case "anthropic":
		if cfg.Anthropic.APIKey == "" {
			return nil, fmt.Errorf("Anthropic API key not set. Set ANTHROPIC_API_KEY env var or add it to ~/.paisql/config.json")
		}
		return NewAnthropic(cfg.Anthropic.APIKey, cfg.Anthropic.Model), nil

	case "gemini":
		if cfg.Gemini.APIKey == "" {
			return nil, fmt.Errorf("Gemini API key not set. Set GEMINI_API_KEY env var or add it to ~/.paisql/config.json")
		}
		return NewGemini(cfg.Gemini.APIKey, cfg.Gemini.Model), nil

	case "ollama":
		return NewOllama(cfg.Ollama.Host, cfg.Ollama.Model), nil

	case "placeholder", "":
		return NewPlaceholder(), nil

	default:
		return nil, fmt.Errorf("unknown AI provider %q. Supported: openai, anthropic, ollama", cfg.Provider)
	}
}
