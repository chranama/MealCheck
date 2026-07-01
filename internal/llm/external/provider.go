package llm

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const (
	ProviderTypeOpenAICompatible = "openai_compatible"
	ProviderTypeOpenAI           = "openai"
	ProviderTypeAnthropic        = "anthropic"
	ProviderTypeGemini           = "gemini"
	ProviderTypeLocalLlama       = "local_llama"
)

type Provider interface {
	Complete(ctx context.Context, config ProviderConfig, messages []ProviderMessage) (string, error)
}

type ProviderFactory func(config ProviderConfig) (Provider, error)

type ProviderMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ProviderHTTPError struct {
	Provider   string
	StatusCode int
	Message    string
}

func (e ProviderHTTPError) Error() string {
	statusText := http.StatusText(e.StatusCode)
	status := fmt.Sprintf("HTTP %d", e.StatusCode)
	if statusText != "" {
		status = status + " " + statusText
	}
	if e.Message == "" {
		return fmt.Sprintf("%s provider returned %s", e.Provider, status)
	}
	return fmt.Sprintf("%s provider returned %s: %s", e.Provider, status, e.Message)
}

func DefaultProviderFactory(config ProviderConfig) (Provider, error) {
	providerType := config.Type
	if providerType == "" {
		providerType = ProviderTypeOpenAICompatible
	}
	timeout := 90 * time.Second
	if providerType == ProviderTypeLocalLlama && config.Timeout > 0 {
		timeout = config.Timeout
	}
	client := &http.Client{Timeout: timeout}
	switch providerType {
	case ProviderTypeOpenAICompatible:
		return OpenAICompatibleProvider{Client: client}, nil
	case ProviderTypeOpenAI:
		return OpenAIProvider{Client: client}, nil
	case ProviderTypeAnthropic:
		return AnthropicProvider{Client: client}, nil
	case ProviderTypeGemini:
		return GeminiProvider{Client: client}, nil
	case ProviderTypeLocalLlama:
		return LocalLlamaProvider{Client: client}, nil
	default:
		return nil, fmt.Errorf("unsupported provider type %q", providerType)
	}
}

func ValidateProviderConfig(config ProviderConfig) error {
	switch config.Type {
	case ProviderTypeOpenAICompatible, ProviderTypeOpenAI, ProviderTypeAnthropic, ProviderTypeGemini, ProviderTypeLocalLlama:
	default:
		return fmt.Errorf("unsupported provider type %q", config.Type)
	}
	if config.Model == "" {
		return fmt.Errorf("provider model is required")
	}
	if config.Type == ProviderTypeLocalLlama {
		return nil
	}
	if config.APIKey == "" {
		return fmt.Errorf("provider api_key is required")
	}
	return nil
}
