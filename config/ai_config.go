// Package config — AI provider configuration.
//
// AI settings are stored in ~/.paisql/config.json alongside
// connection profiles. API keys can also be set via environment
// variables (OPENAI_API_KEY, ANTHROPIC_API_KEY).
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// AIConfig holds the AI provider selection and credentials.
type AIConfig struct {
	Provider  string          `json:"provider"` // "openai", "anthropic", "gemini", "ollama", "placeholder"
	OpenAI    OpenAIConfig    `json:"openai"`
	Anthropic AnthropicConfig `json:"anthropic"`
	Gemini    GeminiConfig    `json:"gemini"`
	Ollama    OllamaConfig    `json:"ollama"`
}

// OpenAIConfig holds OpenAI-specific settings.
type OpenAIConfig struct {
	APIKey string `json:"api_key,omitempty"`
	Model  string `json:"model"`
}

// AnthropicConfig holds Anthropic-specific settings.
type AnthropicConfig struct {
	APIKey string `json:"api_key,omitempty"`
	Model  string `json:"model"`
}

// GeminiConfig holds Google Gemini-specific settings.
type GeminiConfig struct {
	APIKey string `json:"api_key,omitempty"`
	Model  string `json:"model"`
}

// OllamaConfig holds Ollama-specific settings.
type OllamaConfig struct {
	Host  string `json:"host"`
	Model string `json:"model"`
}

// AppConfig is the top-level config file structure (~/.paisql/config.json).
type AppConfig struct {
	AI AIConfig `json:"ai"`
}

// DefaultAIConfig returns sensible defaults.
func DefaultAIConfig() AIConfig {
	return AIConfig{
		Provider: "placeholder",
		OpenAI: OpenAIConfig{
			Model: "gpt-4o",
		},
		Anthropic: AnthropicConfig{
			Model: "claude-sonnet-4-20250514",
		},
		Gemini: GeminiConfig{
			Model: "gemini-2.0-flash",
		},
		Ollama: OllamaConfig{
			Host:  "http://localhost:11434",
			Model: "llama3.2",
		},
	}
}

// LoadAppConfig reads ~/.paisql/config.json; returns defaults if not found.
func LoadAppConfig() (*AppConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return defaultAppConfig(), nil
	}

	path := filepath.Join(homeDir, ".paisql", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultAppConfig(), nil
		}
		return nil, err
	}

	cfg := defaultAppConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Env vars override file config
	if envKey := os.Getenv("OPENAI_API_KEY"); envKey != "" {
		cfg.AI.OpenAI.APIKey = envKey
	}
	if envKey := os.Getenv("ANTHROPIC_API_KEY"); envKey != "" {
		cfg.AI.Anthropic.APIKey = envKey
	}
	if envKey := os.Getenv("GEMINI_API_KEY"); envKey != "" {
		cfg.AI.Gemini.APIKey = envKey
	}
	if envHost := os.Getenv("OLLAMA_HOST"); envHost != "" {
		cfg.AI.Ollama.Host = envHost
	}

	return cfg, nil
}

// SaveAppConfig writes the config to ~/.paisql/config.json.
func SaveAppConfig(cfg *AppConfig) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dir := filepath.Join(homeDir, ".paisql")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0600)
}

func defaultAppConfig() *AppConfig {
	return &AppConfig{
		AI: DefaultAIConfig(),
	}
}
