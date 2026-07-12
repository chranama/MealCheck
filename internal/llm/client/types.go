package client

import "github.com/chranama/MealCheck/internal/llm/inference"

type ProviderConfig = inference.ProviderConfig
type Completer = inference.Completer
type CompleterFactory = inference.CompleterFactory
type Message = inference.Message
type Request = inference.Request

const (
	ProviderTypeOpenAICompatible = inference.ProviderTypeOpenAICompatible
	ProviderTypeOpenAI           = inference.ProviderTypeOpenAI
	ProviderTypeAnthropic        = inference.ProviderTypeAnthropic
	ProviderTypeGemini           = inference.ProviderTypeGemini
	ProviderTypeLocalLlama       = inference.ProviderTypeLocalLlama
)
