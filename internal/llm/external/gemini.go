package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type GeminiProvider struct {
	Client  *http.Client
	BaseURL string
}

func geminiSystemInstruction(messages []ProviderMessage) map[string]any {
	var parts []map[string]string
	for _, message := range messages {
		if (message.Role == "system" || message.Role == "developer") && strings.TrimSpace(message.Content) != "" {
			parts = append(parts, map[string]string{"text": message.Content})
		}
	}
	return map[string]any{"parts": parts}
}

func geminiContents(messages []ProviderMessage) []map[string]any {
	var result []map[string]any
	for _, message := range messages {
		if message.Role == "system" || message.Role == "developer" || strings.TrimSpace(message.Content) == "" {
			continue
		}
		role := "user"
		if message.Role == "assistant" {
			role = "model"
		}
		result = append(result, map[string]any{
			"role":  role,
			"parts": []map[string]string{{"text": message.Content}},
		})
	}
	return result
}

func (p GeminiProvider) Complete(ctx context.Context, config ProviderConfig, messages []ProviderMessage) (string, error) {
	if config.APIKey == "" {
		return "", fmt.Errorf("provider api_key is required")
	}
	if config.Model == "" {
		return "", fmt.Errorf("provider model is required")
	}
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}

	payload := map[string]any{
		"systemInstruction": geminiSystemInstruction(messages),
		"contents":          geminiContents(messages),
		"generationConfig": map[string]any{
			"temperature":      0,
			"responseMimeType": "application/json",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	baseURL := strings.TrimRight(p.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", baseURL, config.Model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-goog-api-key", config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", providerRequestError(config.Type, err, config.APIKey)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2_000_000))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", providerHTTPError(config.Type, resp.StatusCode, responseBody, config.APIKey)
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Candidates) == 0 {
		return "", fmt.Errorf("provider returned no message content")
	}
	var parts []string
	for _, part := range parsed.Candidates[0].Content.Parts {
		if strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
		}
	}
	content := strings.TrimSpace(strings.Join(parts, "\n"))
	if content == "" {
		return "", fmt.Errorf("provider returned no message content")
	}
	return content, nil
}
