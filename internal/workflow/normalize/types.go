package normalize

import (
	"github.com/chranama/MealCheck/internal/core"
	llm "github.com/chranama/MealCheck/internal/llm/external"
	localmodel "github.com/chranama/MealCheck/internal/llm/local"
)

type Config = core.Config
type Run = core.Run
type PendingRunInput = core.PendingRunInput
type ProviderConfig = core.ProviderConfig
type RedactedProviderConfig = core.RedactedProviderConfig
type Provider = llm.Provider
type ProviderFactory = llm.ProviderFactory
type ProviderMessage = llm.ProviderMessage
type LocalModelExtractionArtifact = localmodel.LocalModelExtractionArtifact
type LocalLlamaNormalizationRepair = localmodel.LocalLlamaNormalizationRepair

const (
	InputModeManualStructured  = core.InputModeManualStructured
	InputModeProfileGeneration = core.InputModeProfileGeneration
	InputModePromptGeneration  = core.InputModePromptGeneration
	InputModeLocalModel        = core.InputModeLocalModel

	ProviderTypeOpenAI           = llm.ProviderTypeOpenAI
	ProviderTypeLocalLlama       = llm.ProviderTypeLocalLlama
	ProviderTypeOpenAICompatible = llm.ProviderTypeOpenAICompatible
)
