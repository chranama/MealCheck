package client

import (
	"fmt"
	"net/http"
	"time"

	"github.com/chranama/MealCheck/internal/llm/inference"
)

type HTTPError struct {
	Provider   string
	StatusCode int
	Message    string
}

func (e HTTPError) Error() string {
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

func New(config ProviderConfig) (Completer, error) {
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
		return OpenAICompatible{Client: client}, nil
	case ProviderTypeOpenAI:
		return OpenAI{Client: client}, nil
	case ProviderTypeAnthropic:
		return Anthropic{Client: client}, nil
	case ProviderTypeGemini:
		return Gemini{Client: client}, nil
	case ProviderTypeLocalLlama:
		return LlamaCPP{Client: client}, nil
	default:
		return nil, fmt.Errorf("unsupported provider type %q", providerType)
	}
}

func ValidateProviderConfig(config ProviderConfig) error {
	return inference.ValidateProviderConfig(config)
}
