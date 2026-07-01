package normalize

import (
	"context"
	"fmt"

	localmodel "github.com/chranama/MealCheck/internal/llm/local"
	"github.com/chranama/MealCheck/internal/workflow/checker"
)

func prepareLocalModelExtraction(ctx context.Context, config Config, provider Provider, run Run, input PendingRunInput, events []NormalizationEvent) (checker.Plan, string, []NormalizationEvent, *LocalModelExtractionArtifact, error) {
	output, plan, repairs, extraction, stage, err := localmodel.RunLocalModelExtractionWithArtifacts(ctx, provider, input.Provider, input, "local-model-"+run.ID)
	if err != nil {
		events = append(events, localModelFailureEvent(stage))
		return checker.Plan{}, output, events, extraction, writeLocalModelNormalizationFailureAndReturn(config, run, input, events, normalizationFailureDebug{
			InitialOutput:        output,
			InitialError:         err,
			FinalError:           err,
			LocalModelExtraction: extraction,
		})
	}
	events = append(events, normalizationEvent("llm_output_received", "local model returned compact meal-plan JSON"))
	if len(repairs) > 0 {
		events = append(events, normalizationEvent("source_measurements_reconciled", fmt.Sprintf("local model compact rows were repaired from %d deterministic source field(s)", len(repairs))))
	}
	events = append(events, normalizationEvent("json_decoded", "local model compact output decoded into normalized MealCheck JSON"))
	return plan, output, events, extraction, nil
}

func localModelFailureEvent(stage string) NormalizationEvent {
	switch stage {
	case "provider":
		return normalizationEvent("provider_request_failed", "local model request failed before returning compact meal-plan JSON")
	case "completeness":
		return normalizationEvent("item_count_failed", "local model output did not preserve the numbered source item count")
	default:
		return normalizationEvent("json_decode_failed", "local model output was not valid compact meal-plan JSON")
	}
}
