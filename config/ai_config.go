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
	Provider    string            `json:"provider"` // "openai", "anthropic", "gemini", "groq", "ollama", "antigravity", "placeholder"
	OpenAI      OpenAIConfig      `json:"openai"`
	Anthropic   AnthropicConfig   `json:"anthropic"`
	Gemini      GeminiConfig      `json:"gemini"`
	Ollama      OllamaConfig      `json:"ollama"`
	Groq        GroqConfig        `json:"groq"`
	Antigravity AntigravityConfig `json:"antigravity"`
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

// AntigravityConfig holds Google Antigravity OAuth settings.
// No API key needed — uses Google OAuth2 login (same as Gemini CLI).
type AntigravityConfig struct {
	Model string `json:"model"`
}

// GroqConfig holds Groq-specific settings.
type GroqConfig struct {
	APIKey string `json:"api_key,omitempty"`
	Model  string `json:"model"`
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
		Antigravity: AntigravityConfig{
			Model: "gemini-2.0-flash",
		},
		Groq: GroqConfig{
			Model: "llama-3.1-8b-instant",
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
	if envModel := os.Getenv("ANTIGRAVITY_MODEL"); envModel != "" {
		cfg.AI.Antigravity.Model = envModel
	}
	if envKey := os.Getenv("GROQ_API_KEY"); envKey != "" {
		cfg.AI.Groq.APIKey = envKey
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
