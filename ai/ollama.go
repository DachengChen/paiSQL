package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Ollama implements the Provider interface for local Ollama instances.
type Ollama struct {
	host  string
	model string
}

var _ Provider = (*Ollama)(nil)

// NewOllama creates an Ollama provider.
func NewOllama(host, model string) *Ollama {
	if host == "" {
		host = "http://localhost:11434"
	}
	if model == "" {
		model = "llama3.2"
	}
	return &Ollama{host: host, model: model}
}

func (o *Ollama) Name() string {
	return fmt.Sprintf("Ollama (%s)", o.model)
}

func (o *Ollama) Chat(ctx context.Context, messages []Message) (string, error) {
	return o.call(ctx, messages)
}

func (o *Ollama) SuggestIndexes(ctx context.Context, query string, explainJSON string) (string, error) {
	messages := []Message{
		{Role: "system", Content: systemPromptIndex},
		{Role: "user", Content: fmt.Sprintf("Query:\n%s\n\nEXPLAIN output:\n%s", query, explainJSON)},
	}
	return o.call(ctx, messages)
}

func (o *Ollama) GenerateQueryPlan(ctx context.Context, schemaContext string, userQuestion string, dataViewState string) (string, error) {
	userContent := fmt.Sprintf("Schema:\n%s\n\nData view state:\n%s\n\nUser question: %s", schemaContext, dataViewState, userQuestion)
	messages := []Message{
		{Role: "system", Content: systemPromptQueryPlan},
		{Role: "user", Content: userContent},
	}
	return o.call(ctx, messages)
}

func (o *Ollama) call(ctx context.Context, messages []Message) (string, error) {
	type chatMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

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
		"stream":   false,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	url := o.host + "/api/chat"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request failed (is Ollama running at %s?): %w", o.host, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("ollama parse error: %w", err)
	}

	if result.Message.Content == "" {
		return "", fmt.Errorf("ollama returned empty response")
	}

	return result.Message.Content, nil
}
