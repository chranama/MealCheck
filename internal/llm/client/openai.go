package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type OpenAICompatible struct {
	Client *http.Client
}

type OpenAI struct {
	Client  *http.Client
	BaseURL string
}

type LlamaCPP struct {
	Client  *http.Client
	BaseURL string
}

func completeOpenAIChat(ctx context.Context, client *http.Client, config ProviderConfig, request Request, baseURLOverride string) (string, error) {
	if config.Type != ProviderTypeLocalLlama && config.APIKey == "" {
		return "", fmt.Errorf("provider api_key is required")
	}
	if config.Model == "" {
		return "", fmt.Errorf("provider model is required")
	}
	baseURL := strings.TrimRight(baseURLOverride, "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(config.BaseURL, "/")
	}
	if baseURL == "" {
		if config.Type == ProviderTypeLocalLlama {
			baseURL = "http://127.0.0.1:11435/v1"
		} else {
			baseURL = "https://api.openai.com/v1"
		}
	}
	if client == nil {
		client = http.DefaultClient
	}

	responseFormat := map[string]any{
		"type": "json_object",
	}
	if config.Type == ProviderTypeOpenAI && request.StructuredOutput != nil {
		responseFormat = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   request.StructuredOutput.Name,
				"strict": true,
				"schema": request.StructuredOutput.StrictSchema,
			},
		}
	}
	if config.Type == ProviderTypeLocalLlama && request.StructuredOutput != nil {
		responseFormat = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   request.StructuredOutput.Name,
				"strict": true,
				"schema": request.StructuredOutput.StrictSchema,
			},
		}
	}
	payload := map[string]any{
		"model":           config.Model,
		"messages":        request.Messages,
		"temperature":     0,
		"response_format": responseFormat,
	}
	if config.Type == ProviderTypeLocalLlama {
		maxTokens := config.MaxTokens
		if maxTokens <= 0 {
			maxTokens = 512
		}
		payload["max_tokens"] = maxTokens
		payload["stream"] = false
		payload["chat_template_kwargs"] = map[string]any{"enable_thinking": false}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	if config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	}
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

func (p OpenAICompatible) Complete(ctx context.Context, config ProviderConfig, request Request) (string, error) {
	return completeOpenAIChat(ctx, p.Client, config, request, config.BaseURL)
}

func (p OpenAI) Complete(ctx context.Context, config ProviderConfig, request Request) (string, error) {
	config.BaseURL = ""
	config.Type = ProviderTypeOpenAI
	return completeOpenAIChat(ctx, p.Client, config, request, p.BaseURL)
}

func (p LlamaCPP) Complete(ctx context.Context, config ProviderConfig, request Request) (string, error) {
	config.Type = ProviderTypeLocalLlama
	return completeOpenAIChat(ctx, p.Client, config, request, p.BaseURL)
}
