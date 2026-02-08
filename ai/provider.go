// Package ai defines the interface for AI assistant providers
// and a placeholder implementation.
//
// Design decisions:
//   - Provider is an interface so we can swap backends (OpenAI, Anthropic,
//     Ollama, etc.) without changing TUI code.
//   - All methods accept context for cancellation (async-friendly).
//   - The placeholder provider returns canned responses for development.
package ai

import (
	"context"
)

// Message represents a chat message.
type Message struct {
	Role    string // "user", "assistant", "system"
	Content string
}

// Provider is the interface all AI backends must implement.
type Provider interface {
	// Chat sends a conversation and returns the assistant's reply.
	Chat(ctx context.Context, messages []Message) (string, error)

	// SuggestIndexes analyzes a query and suggests indexes.
	SuggestIndexes(ctx context.Context, query string, explainJSON string) (string, error)

	// GenerateQueryPlan takes a schema context and user question,
	// returning a structured JSON query plan (not raw SQL).
	GenerateQueryPlan(ctx context.Context, schemaContext string, userQuestion string, dataViewState string) (string, error)

	// Name returns the provider name for display.
	Name() string
}
