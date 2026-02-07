package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OpenAI implements the Provider interface for OpenAI's Chat API.
type OpenAI struct {
	apiKey string
	model  string
}

var _ Provider = (*OpenAI)(nil)

// NewOpenAI creates an OpenAI provider.
func NewOpenAI(apiKey, model string) *OpenAI {
	if model == "" {
		model = "gpt-4o"
	}
	return &OpenAI{apiKey: apiKey, model: model}
}

func (o *OpenAI) Name() string {
	return fmt.Sprintf("OpenAI (%s)", o.model)
}

func (o *OpenAI) Chat(ctx context.Context, messages []Message) (string, error) {
	return o.call(ctx, messages)
}

func (o *OpenAI) SuggestIndexes(ctx context.Context, query string, explainJSON string) (string, error) {
	messages := []Message{
		{Role: "system", Content: systemPromptIndex},
		{Role: "user", Content: fmt.Sprintf("Query:\n%s\n\nEXPLAIN output:\n%s", query, explainJSON)},
	}
	return o.call(ctx, messages)
}

func (o *OpenAI) call(ctx context.Context, messages []Message) (string, error) {
	type chatMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	// Prepend system prompt for chat
	apiMsgs := make([]chatMsg, 0, len(messages)+1)
	hasSystem := false
	for _, m := range messages {
		if m.Role == "system" {
			hasSystem = true
		}
		apiMsgs = append(apiMsgs, chatMsg(m))
	}
	if !hasSystem {
		apiMsgs = append([]chatMsg{{Role: "system", Content: systemPromptChat}}, apiMsgs...)
	}

	body := map[string]interface{}{
		"model":    o.model,
		"messages": apiMsgs,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("openai parse error: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices")
	}

	return result.Choices[0].Message.Content, nil
}
