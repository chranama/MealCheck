package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Provider interface {
	Complete(ctx context.Context, config ProviderConfig, messages []ProviderMessage) (string, error)
}

type ProviderFactory func(config ProviderConfig) (Provider, error)

type ProviderMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func DefaultProviderFactory(config ProviderConfig) (Provider, error) {
	providerType := config.Type
	if providerType == "" {
		providerType = "openai_compatible"
	}
	if providerType != "openai_compatible" {
		return nil, fmt.Errorf("unsupported provider type %q", providerType)
	}
	return OpenAICompatibleProvider{Client: &http.Client{Timeout: 45 * time.Second}}, nil
}

type OpenAICompatibleProvider struct {
	Client *http.Client
}

func (p OpenAICompatibleProvider) Complete(ctx context.Context, config ProviderConfig, messages []ProviderMessage) (string, error) {
	if config.APIKey == "" {
		return "", fmt.Errorf("provider api_key is required")
	}
	if config.Model == "" {
		return "", fmt.Errorf("provider model is required")
	}
	baseURL := strings.TrimRight(config.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}

	payload := map[string]any{
		"model":       config.Model,
		"messages":    messages,
		"temperature": 0,
		"response_format": map[string]string{
			"type": "json_object",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2_000_000))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("provider returned status %d", resp.StatusCode)
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("provider returned no message content")
	}
	return parsed.Choices[0].Message.Content, nil
}
