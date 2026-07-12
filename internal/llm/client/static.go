package client

import (
	"context"
	"fmt"
	"os"
	"strings"
)

func StaticResponseFactory(response string) CompleterFactory {
	return func(_ ProviderConfig) (Completer, error) {
		return StaticResponse{Response: response}, nil
	}
}

func StaticResponseFactoryFromFile(path string) (CompleterFactory, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return StaticResponseFactory(string(b)), nil
}

type StaticResponse struct {
	Response string
}

func (p StaticResponse) Complete(_ context.Context, config ProviderConfig, request Request) (string, error) {
	if config.APIKey != "" {
		for _, message := range request.Messages {
			if strings.Contains(message.Content, config.APIKey) {
				return "", fmt.Errorf("provider api_key leaked into prompt")
			}
		}
	}
	return p.Response, nil
}
