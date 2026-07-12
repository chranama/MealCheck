package normalize

import (
	"context"
	"fmt"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

const (
	defaultGuidelinePackID     = "dga-2025-2030-us-adult-general-v1"
	defaultGuidelinePackPath   = "data/guidelines/dga-2025-2030-us-adult-general-v1/guideline-pack.json"
	defaultNutrientCatalogID   = "fixture-catalog-v1"
	defaultNutrientCatalogPath = "data/nutrients/fixture-catalog-v1.json"
)

type PreparedRun struct {
	CasePath             string
	Plan                 checker.Plan
	LLMOutput            string
	NormalizationEvents  []NormalizationEvent
	RedactedProvider     RedactedProviderConfig
	LocalModelExtraction *LocalModelExtractionArtifact
	UsedProvider         bool
}

type NormalizationEvent struct {
	Type      string `json:"type"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

func PrepareRunInput(ctx context.Context, config Config, completerFactory CompleterFactory, run Run, input PendingRunInput) (PreparedRun, error) {
	if input.Mode == "" {
		return PreparedRun{}, fmt.Errorf("input mode is required")
	}
	if input.Mode != InputModeManualStructured && completerFactory == nil {
		return PreparedRun{}, fmt.Errorf("inference completer factory is required")
	}
	if input.Mode == InputModeLocalModel {
		input.Settings = normalizeLocalModelSettings(input.Settings)
		if err := validateLocalModelSettings(input.Settings); err != nil {
			return PreparedRun{}, err
		}
		if err := validateLocalModelInputContract(config, input.CandidateText); err != nil {
			return PreparedRun{}, err
		}
	}
	if err := validateSettings(input.Settings); err != nil {
		return PreparedRun{}, err
	}

	var plan checker.Plan
	var llmOutput string
	var initialOutput string
	var initialErr error
	var repairOutput string
	var repairErr error
	var repairAttempted bool
	var events []NormalizationEvent
	var providerRedacted RedactedProviderConfig
	var completer Completer
	var localModelExtraction *LocalModelExtractionArtifact
	usedProvider := false

	switch input.Mode {
	case InputModeManualStructured:
		if input.CandidatePlan == nil {
			return PreparedRun{}, fmt.Errorf("candidate_plan is required for manual_structured")
		}
		plan = *input.CandidatePlan
		events = append(events, normalizationEvent("manual_plan_received", "manual structured meal plan received"))
	case InputModeProfileGeneration, InputModePromptGeneration:
		var err error
		completer, err = completerFactory(input.Provider)
		if err != nil {
			return PreparedRun{}, err
		}
		providerRedacted = redactProvider(input.Provider)
		usedProvider = true
		messages, err := generationMessages(input)
		if err != nil {
			return PreparedRun{}, err
		}
		llmOutput, err = completer.Complete(ctx, input.Provider, mealPlanCompletionRequest(messages))
		if err != nil {
			events = append(events, normalizationEvent("provider_request_failed", "provider request failed before returning meal-plan JSON"))
			return PreparedRun{}, writeNormalizationFailureAndReturn(config, run, input.Provider, events, normalizationFailureDebug{
				FinalError: err,
			})
		}
		initialOutput = llmOutput
		events = append(events, normalizationEvent("llm_output_received", "provider returned candidate meal-plan JSON"))
		decodeResult, err := decodePlanTextDetailed(llmOutput)
		if err != nil {
			initialErr = err
			events = append(events, normalizationEvent("json_decode_failed", "initial provider output was not valid normalized meal-plan JSON"))
			if !input.RepairJSON {
				return PreparedRun{}, writeNormalizationFailureAndReturn(config, run, input.Provider, events, normalizationFailureDebug{
					InitialOutput: initialOutput,
					InitialError:  initialErr,
					FinalError:    initialErr,
				})
			}
			repairDecodeErr := sanitizeRepairPromptError(err, input.Provider.APIKey)
			repairAttempted = true
			repairOutput, repairErr = completer.Complete(ctx, input.Provider, mealPlanCompletionRequest(repairMessages(input, sanitizeDebugArtifactText(llmOutput, input.Provider.APIKey), repairDecodeErr)))
			if repairErr != nil {
				return PreparedRun{}, writeNormalizationFailureAndReturn(config, run, input.Provider, events, normalizationFailureDebug{
					InitialOutput: initialOutput,
					InitialError:  initialErr,
					RepairError:   repairErr,
					FinalError:    repairErr,
				})
			}
			events = append(events, normalizationEvent("repair_attempted", "one bounded JSON repair attempt was made"))
			llmOutput = repairOutput
			decodeResult, err = decodePlanTextDetailed(repairOutput)
			if err != nil {
				return PreparedRun{}, writeNormalizationFailureAndReturn(config, run, input.Provider, events, normalizationFailureDebug{
					InitialOutput: initialOutput,
					InitialError:  initialErr,
					RepairOutput:  repairOutput,
					RepairError:   err,
					FinalError:    err,
				})
			}
			plan = decodeResult.Plan
			if decodeResult.Canonicalized {
				events = append(events, normalizationEvent("json_canonicalized", "repair output used bounded alias canonicalization before strict decode"))
			}
			events = append(events, normalizationEvent("repair_succeeded", "repair output decoded as normalized meal-plan JSON"))
		} else {
			plan = decodeResult.Plan
			if decodeResult.Canonicalized {
				events = append(events, normalizationEvent("json_canonicalized", "provider output used bounded alias canonicalization before strict decode"))
			}
			events = append(events, normalizationEvent("json_decoded", "provider output decoded as normalized meal-plan JSON"))
		}
	case InputModeLocalModel:
		var err error
		completer, err = completerFactory(input.Provider)
		if err != nil {
			return PreparedRun{}, err
		}
		providerRedacted = redactProvider(input.Provider)
		usedProvider = true
		plan, llmOutput, events, localModelExtraction, err = prepareLocalModelExtraction(ctx, config, completer, run, input, events)
		if err != nil {
			return PreparedRun{}, err
		}
	default:
		return PreparedRun{}, fmt.Errorf("unsupported input_mode %q", input.Mode)
	}

	if usedProvider {
		var err error
		plan, llmOutput, repairOutput, repairErr, repairAttempted, events, err = normalizeGeneratedPlanPostDecode(ctx, config, completer, run, input, plan, llmOutput, initialOutput, initialErr, repairOutput, repairErr, repairAttempted, events, localModelExtraction)
		if err != nil {
			return PreparedRun{}, err
		}
	} else if err := validatePlan(plan); err != nil {
		return PreparedRun{}, err
	}

	casePath, err := writeRuntimeCase(config, run, input, plan)
	if err != nil {
		return PreparedRun{}, err
	}
	return PreparedRun{
		CasePath:             casePath,
		Plan:                 plan,
		LLMOutput:            sanitizeDebugArtifactText(llmOutput, input.Provider.APIKey),
		NormalizationEvents:  events,
		RedactedProvider:     providerRedacted,
		LocalModelExtraction: localModelExtraction,
		UsedProvider:         usedProvider,
	}, nil
}
