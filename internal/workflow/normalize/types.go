package normalize

import (
	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/llm/inference"
	planextract "github.com/chranama/MealCheck/internal/llm/planextract"
)

type Config = core.Config
type Run = core.Run
type PendingRunInput = core.PendingRunInput
type ProviderConfig = core.ProviderConfig
type RedactedProviderConfig = core.RedactedProviderConfig
type Completer = inference.Completer
type CompleterFactory = inference.CompleterFactory
type Message = inference.Message
type CompletionRequest = inference.Request
type StructuredOutput = inference.StructuredOutput
type LocalModelExtractionArtifact = planextract.LocalModelExtractionArtifact
type LocalModelChunkSourceItemArtifact = planextract.LocalModelChunkSourceItemArtifact
type LocalLlamaNormalizationRepair = planextract.LocalLlamaNormalizationRepair

const (
	InputModeManualStructured  = core.InputModeManualStructured
	InputModeProfileGeneration = core.InputModeProfileGeneration
	InputModePromptGeneration  = core.InputModePromptGeneration
	InputModeLocalModel        = core.InputModeLocalModel

	ProviderTypeOpenAI           = inference.ProviderTypeOpenAI
	ProviderTypeLocalLlama       = inference.ProviderTypeLocalLlama
	ProviderTypeOpenAICompatible = inference.ProviderTypeOpenAICompatible
)
