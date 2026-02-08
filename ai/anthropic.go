package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Anthropic implements the Provider interface for the Anthropic Messages API.
type Anthropic struct {
	apiKey string
	model  string
}

var _ Provider = (*Anthropic)(nil)

// NewAnthropic creates an Anthropic provider.
func NewAnthropic(apiKey, model string) *Anthropic {
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	return &Anthropic{apiKey: apiKey, model: model}
}

func (a *Anthropic) Name() string {
	return fmt.Sprintf("Anthropic (%s)", a.model)
}

func (a *Anthropic) Chat(ctx context.Context, messages []Message) (string, error) {
	return a.call(ctx, systemPromptChat, messages)
}

func (a *Anthropic) SuggestIndexes(ctx context.Context, query string, explainJSON string) (string, error) {
	messages := []Message{
		{Role: "user", Content: fmt.Sprintf("Query:\n%s\n\nEXPLAIN output:\n%s", query, explainJSON)},
	}
	return a.call(ctx, systemPromptIndex, messages)
}

func (a *Anthropic) GenerateQueryPlan(ctx context.Context, schemaContext string, userQuestion string, dataViewState string) (string, error) {
	userContent := fmt.Sprintf("Schema:\n%s\n\nData view state:\n%s\n\nUser question: %s", schemaContext, dataViewState, userQuestion)
	messages := []Message{
		{Role: "user", Content: userContent},
	}
	return a.call(ctx, systemPromptQueryPlan, messages)
}

func (a *Anthropic) call(ctx context.Context, system string, messages []Message) (string, error) {
	type apiMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	// Anthropic doesn't use "system" role in messages — it's a top-level field.
	apiMsgs := make([]apiMsg, 0, len(messages))
	for _, m := range messages {
		if m.Role == "system" {
			system = m.Content
			continue
		}
		apiMsgs = append(apiMsgs, apiMsg(m))
	}

	// Ensure at least one user message
	if len(apiMsgs) == 0 {
		return "", fmt.Errorf("anthropic requires at least one user message")
	}

	body := map[string]interface{}{
		"model":      a.model,
		"max_tokens": 4096,
		"system":     system,
		"messages":   apiMsgs,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("anthropic parse error: %w", err)
	}

	// Concatenate all text blocks
	var text string
	for _, block := range result.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	if text == "" {
		return "", fmt.Errorf("anthropic returned no text content")
	}

	return text, nil
}
