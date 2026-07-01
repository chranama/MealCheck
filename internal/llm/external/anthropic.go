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

type AnthropicProvider struct {
	Client  *http.Client
	BaseURL string
}

func anthropicMessagePayload(messages []ProviderMessage) (string, []map[string]string) {
	var systemParts []string
	var result []map[string]string
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		content := message.Content
		if strings.TrimSpace(content) == "" {
			continue
		}
		switch role {
		case "system", "developer":
			systemParts = append(systemParts, content)
		case "assistant":
			result = append(result, map[string]string{"role": "assistant", "content": content})
		default:
			result = append(result, map[string]string{"role": "user", "content": content})
		}
	}
	return strings.Join(systemParts, "\n\n"), result
}

func (p AnthropicProvider) Complete(ctx context.Context, config ProviderConfig, messages []ProviderMessage) (string, error) {
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

	system, anthropicMessages := anthropicMessagePayload(messages)
	payload := map[string]any{
		"model":       config.Model,
		"max_tokens":  4096,
		"temperature": 0,
		"messages":    anthropicMessages,
		"output_config": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"schema": portableMealPlanResponseSchema(),
			},
		},
	}
	if system != "" {
		payload["system"] = system
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	baseURL := strings.TrimRight(p.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
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
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return "", err
	}
	var parts []string
	for _, block := range parsed.Content {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	content := strings.TrimSpace(strings.Join(parts, "\n"))
	if content == "" {
		return "", fmt.Errorf("provider returned no message content")
	}
	return content, nil
}
