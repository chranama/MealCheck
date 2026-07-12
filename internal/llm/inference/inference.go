// Package inference defines the model-completion capability used by MealCheck.
// Concrete provider clients implement these contracts in internal/llm/client.
package inference

import (
	"context"
	"fmt"

	"github.com/chranama/MealCheck/internal/core"
)

const (
	ProviderTypeOpenAICompatible = "openai_compatible"
	ProviderTypeOpenAI           = "openai"
	ProviderTypeAnthropic        = "anthropic"
	ProviderTypeGemini           = "gemini"
	ProviderTypeLocalLlama       = "local_llama"
)

type ProviderConfig = core.ProviderConfig

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type StructuredOutput struct {
	Name           string
	StrictSchema   map[string]any
	PortableSchema map[string]any
}

type Request struct {
	Messages         []Message
	StructuredOutput *StructuredOutput
}

type Completer interface {
	Complete(ctx context.Context, config ProviderConfig, request Request) (string, error)
}

type CompleterFactory func(config ProviderConfig) (Completer, error)

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
