package hosted

import (
	"context"
	"fmt"

	"github.com/chranama/MealCheck/internal/assist"
)

type AssistProviderAdapter struct {
	Provider Provider
	Config   ProviderConfig
}

func (a AssistProviderAdapter) Complete(ctx context.Context, request assist.Request) (assist.Response, error) {
	if a.Provider == nil {
		return assist.Response{}, fmt.Errorf("assist provider is not configured")
	}
	config := a.Config
	if request.ResponseSchema != nil {
		config.ResponseSchemaName = request.SchemaName
		config.ResponseSchema = request.ResponseSchema
	}
	messages := make([]ProviderMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		messages = append(messages, ProviderMessage{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	raw, err := a.Provider.Complete(ctx, config, messages)
	if err != nil {
		return assist.Response{}, err
	}
	return assist.Response{RawText: raw}, nil
}
