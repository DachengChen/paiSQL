package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Gemini implements the Provider interface for Google's Gemini API
// (used by Antigravity / Google DeepMind).
type Gemini struct {
	apiKey string
	model  string
}

var _ Provider = (*Gemini)(nil)

// NewGemini creates a Gemini provider.
func NewGemini(apiKey, model string) *Gemini {
	if model == "" {
		model = "gemini-2.0-flash"
	}
	return &Gemini{apiKey: apiKey, model: model}
}

func (g *Gemini) Name() string {
	return fmt.Sprintf("Gemini (%s)", g.model)
}

func (g *Gemini) Chat(ctx context.Context, messages []Message) (string, error) {
	return g.call(ctx, systemPromptChat, messages)
}

func (g *Gemini) SuggestIndexes(ctx context.Context, query string, explainJSON string) (string, error) {
	messages := []Message{
		{Role: "user", Content: fmt.Sprintf("Query:\n%s\n\nEXPLAIN output:\n%s", query, explainJSON)},
	}
	return g.call(ctx, systemPromptIndex, messages)
}

func (g *Gemini) call(ctx context.Context, system string, messages []Message) (string, error) {
	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Role  string `json:"role"`
		Parts []part `json:"parts"`
	}

	// Build contents array
	var contents []content
	for _, m := range messages {
		if m.Role == "system" {
			system = m.Content
			continue
		}
		role := m.Role
		if role == "assistant" {
			role = "model" // Gemini uses "model" instead of "assistant"
		}
		contents = append(contents, content{
			Role:  role,
			Parts: []part{{Text: m.Content}},
		})
	}

	body := map[string]interface{}{
		"contents": contents,
		"systemInstruction": map[string]interface{}{
			"parts": []part{{Text: system}},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		g.model, g.apiKey)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("gemini parse error: %w", err)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned no content")
	}

	// Concatenate all text parts
	var text string
	for _, p := range result.Candidates[0].Content.Parts {
		text += p.Text
	}

	return text, nil
}
