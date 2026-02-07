package ai

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Placeholder is a mock AI provider for development.
type Placeholder struct{}

var _ Provider = (*Placeholder)(nil)

func NewPlaceholder() *Placeholder {
	return &Placeholder{}
}

func (p *Placeholder) Name() string {
	return "placeholder"
}

func (p *Placeholder) Chat(ctx context.Context, messages []Message) (string, error) {
	// Simulate network latency
	select {
	case <-time.After(500 * time.Millisecond):
	case <-ctx.Done():
		return "", ctx.Err()
	}

	if len(messages) == 0 {
		return "No messages provided.", nil
	}

	last := messages[len(messages)-1].Content
	return fmt.Sprintf("🤖 [Placeholder AI]\n\nYou asked: %q\n\nThis is a placeholder response. "+
		"Configure a real AI provider (OpenAI, Anthropic, Ollama) to get actual assistance.\n\n"+
		"Tip: I can help with:\n"+
		"  • Query optimization\n"+
		"  • Index suggestions\n"+
		"  • Schema design\n"+
		"  • SQL debugging", last), nil
}

func (p *Placeholder) SuggestIndexes(ctx context.Context, query string, explainJSON string) (string, error) {
	select {
	case <-time.After(300 * time.Millisecond):
	case <-ctx.Done():
		return "", ctx.Err()
	}

	suggestions := []string{
		"📊 Index Suggestions (Placeholder)",
		"",
		fmt.Sprintf("Query: %s", truncate(query, 80)),
		"",
		"Suggested indexes:",
		"  1. CREATE INDEX idx_example ON table_name (column_name);",
		"  2. CREATE INDEX idx_composite ON table_name (col1, col2);",
		"",
		"Note: Connect a real AI provider for actual analysis.",
	}
	return strings.Join(suggestions, "\n"), nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
