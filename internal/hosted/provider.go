package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	ProviderTypeOpenAICompatible = "openai_compatible"
	ProviderTypeOpenAI           = "openai"
	ProviderTypeAnthropic        = "anthropic"
	ProviderTypeGemini           = "gemini"
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
		providerType = ProviderTypeOpenAICompatible
	}
	client := &http.Client{Timeout: 45 * time.Second}
	switch providerType {
	case ProviderTypeOpenAICompatible:
		return OpenAICompatibleProvider{Client: client}, nil
	case ProviderTypeOpenAI:
		return OpenAIProvider{Client: client}, nil
	case ProviderTypeAnthropic:
		return AnthropicProvider{Client: client}, nil
	case ProviderTypeGemini:
		return GeminiProvider{Client: client}, nil
	default:
		return nil, fmt.Errorf("unsupported provider type %q", providerType)
	}
}

func StaticResponseProviderFactory(response string) ProviderFactory {
	return func(_ ProviderConfig) (Provider, error) {
		return StaticResponseProvider{Response: response}, nil
	}
}

func StaticResponseProviderFactoryFromFile(path string) (ProviderFactory, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return StaticResponseProviderFactory(string(b)), nil
}

type StaticResponseProvider struct {
	Response string
}

func (p StaticResponseProvider) Complete(_ context.Context, config ProviderConfig, messages []ProviderMessage) (string, error) {
	if config.APIKey != "" {
		for _, message := range messages {
			if strings.Contains(message.Content, config.APIKey) {
				return "", fmt.Errorf("provider api_key leaked into prompt")
			}
		}
	}
	return p.Response, nil
}

type OpenAICompatibleProvider struct {
	Client *http.Client
}

func (p OpenAICompatibleProvider) Complete(ctx context.Context, config ProviderConfig, messages []ProviderMessage) (string, error) {
	return completeOpenAIChat(ctx, p.Client, config, messages, config.BaseURL)
}

type OpenAIProvider struct {
	Client  *http.Client
	BaseURL string
}

func (p OpenAIProvider) Complete(ctx context.Context, config ProviderConfig, messages []ProviderMessage) (string, error) {
	config.BaseURL = ""
	return completeOpenAIChat(ctx, p.Client, config, messages, p.BaseURL)
}

func completeOpenAIChat(ctx context.Context, client *http.Client, config ProviderConfig, messages []ProviderMessage, baseURLOverride string) (string, error) {
	if config.APIKey == "" {
		return "", fmt.Errorf("provider api_key is required")
	}
	if config.Model == "" {
		return "", fmt.Errorf("provider model is required")
	}
	baseURL := strings.TrimRight(baseURLOverride, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
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

type AnthropicProvider struct {
	Client  *http.Client
	BaseURL string
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
		"max_tokens":  8192,
		"temperature": 0,
		"messages":    anthropicMessages,
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

type GeminiProvider struct {
	Client  *http.Client
	BaseURL string
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
			"temperature":    0,
			"responseFormat": map[string]any{"text": map[string]string{"mimeType": "application/json"}},
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
