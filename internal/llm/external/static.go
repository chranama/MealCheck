package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
)

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
