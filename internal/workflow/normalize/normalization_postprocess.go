package normalize

import (
	"context"
	"fmt"
	"strings"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

func normalizeGeneratedPlanPostDecode(ctx context.Context, config Config, provider Provider, run Run, input PendingRunInput, plan checker.Plan, llmOutput string, initialOutput string, initialErr error, repairOutput string, repairErr error, repairAttempted bool, events []NormalizationEvent, localModelExtraction *LocalModelExtractionArtifact) (checker.Plan, string, string, error, bool, []NormalizationEvent, error) {
	if normalizedPlan, changed := markUnsupportedUnitsUnresolved(plan); changed {
		plan = normalizedPlan
		events = append(events, normalizationEvent("unsupported_units_marked_unresolved", "provider numeric items with unsupported units were preserved as unresolved quantities"))
	}
	if err := validatePlan(plan); err != nil {
		events = append(events, normalizationEvent("plan_validation_failed", "decoded provider output failed MealCheck plan validation"))
		return checker.Plan{}, llmOutput, repairOutput, repairErr, repairAttempted, events, writeNormalizationFailureAndReturn(config, run, input.Provider, events, normalizationFailureDebug{
			InitialOutput:        initialOutput,
			InitialError:         initialErr,
			RepairOutput:         repairOutput,
			RepairError:          repairErr,
			FinalError:           err,
			LocalModelExtraction: localModelExtraction,
		})
	}

	if err := validateGeneratedPlanAgainstConstraints(plan, input.Settings.VerificationConstraints); err != nil {
		events = append(events, normalizationEvent("plan_constraints_failed", "decoded provider output did not satisfy requested day and meal counts"))
		if !input.RepairJSON || repairAttempted {
			return checker.Plan{}, llmOutput, repairOutput, repairErr, repairAttempted, events, writeNormalizationFailureAndReturn(config, run, input.Provider, events, normalizationFailureDebug{
				InitialOutput:        initialOutput,
				InitialError:         initialErr,
				RepairOutput:         repairOutput,
				RepairError:          repairErr,
				FinalError:           err,
				LocalModelExtraction: localModelExtraction,
			})
		}

		repairAttempted = true
		repairOutput, repairErr = provider.Complete(ctx, input.Provider, repairMessages(input, sanitizeDebugArtifactText(llmOutput, input.Provider.APIKey), sanitizeRepairPromptError(err, input.Provider.APIKey)))
		if repairErr != nil {
			return checker.Plan{}, llmOutput, repairOutput, repairErr, repairAttempted, events, writeNormalizationFailureAndReturn(config, run, input.Provider, events, normalizationFailureDebug{
				InitialOutput:        initialOutput,
				InitialError:         initialErr,
				RepairError:          repairErr,
				FinalError:           repairErr,
				LocalModelExtraction: localModelExtraction,
			})
		}

		events = append(events, normalizationEvent("repair_attempted", "one bounded JSON repair attempt was made"))
		llmOutput = repairOutput
		decodeResult, decodeErr := decodePlanTextDetailed(repairOutput)
		if decodeErr != nil {
			return checker.Plan{}, llmOutput, repairOutput, decodeErr, repairAttempted, events, writeNormalizationFailureAndReturn(config, run, input.Provider, events, normalizationFailureDebug{
				InitialOutput:        initialOutput,
				InitialError:         initialErr,
				RepairOutput:         repairOutput,
				RepairError:          decodeErr,
				FinalError:           decodeErr,
				LocalModelExtraction: localModelExtraction,
			})
		}
		plan = decodeResult.Plan
		if normalizedPlan, changed := markUnsupportedUnitsUnresolved(plan); changed {
			plan = normalizedPlan
			events = append(events, normalizationEvent("unsupported_units_marked_unresolved", "provider numeric items with unsupported units were preserved as unresolved quantities"))
		}
		if decodeResult.Canonicalized {
			events = append(events, normalizationEvent("json_canonicalized", "repair output used bounded alias canonicalization before strict decode"))
		}
		events = append(events, normalizationEvent("repair_succeeded", "repair output decoded as normalized meal-plan JSON"))
		if err := validatePlan(plan); err != nil {
			events = append(events, normalizationEvent("plan_validation_failed", "repair output failed MealCheck plan validation"))
			return checker.Plan{}, llmOutput, repairOutput, err, repairAttempted, events, writeNormalizationFailureAndReturn(config, run, input.Provider, events, normalizationFailureDebug{
				InitialOutput:        initialOutput,
				InitialError:         initialErr,
				RepairOutput:         repairOutput,
				RepairError:          err,
				FinalError:           err,
				LocalModelExtraction: localModelExtraction,
			})
		}
		if err := validateGeneratedPlanAgainstConstraints(plan, input.Settings.VerificationConstraints); err != nil {
			events = append(events, normalizationEvent("plan_constraints_failed", "repair output did not satisfy requested day and meal counts"))
			return checker.Plan{}, llmOutput, repairOutput, err, repairAttempted, events, writeNormalizationFailureAndReturn(config, run, input.Provider, events, normalizationFailureDebug{
				InitialOutput:        initialOutput,
				InitialError:         initialErr,
				RepairOutput:         repairOutput,
				RepairError:          err,
				FinalError:           err,
				LocalModelExtraction: localModelExtraction,
			})
		}
	}

	return plan, llmOutput, repairOutput, repairErr, repairAttempted, events, nil
}
func markUnsupportedUnitsUnresolved(plan checker.Plan) (checker.Plan, bool) {
	changed := false
	for dayIndex := range plan.Days {
		for mealIndex := range plan.Days[dayIndex].Meals {
			for itemIndex := range plan.Days[dayIndex].Meals[mealIndex].Items {
				if markUnsupportedItemUnitUnresolved(&plan.Days[dayIndex].Meals[mealIndex].Items[itemIndex]) {
					changed = true
				}
			}
		}
	}
	for itemIndex := range plan.ShoppingList {
		if markUnsupportedItemUnitUnresolved(&plan.ShoppingList[itemIndex]) {
			changed = true
		}
	}
	return plan, changed
}
func markUnsupportedItemUnitUnresolved(item *checker.FoodItem) bool {
	if item.Quantity == nil || allowedUnit(item.Unit) {
		return false
	}
	if strings.TrimSpace(item.QuantityText) == "" {
		item.QuantityText = unsupportedUnitQuantityText(item.Quantity, item.Unit)
	}
	item.Quantity = nil
	item.Unit = ""
	item.ResolutionStatus = "unresolved"
	item.UnresolvedReason = "unsupported_unit"
	return true
}
func unsupportedUnitQuantityText(quantity *float64, unit string) string {
	if quantity == nil {
		return strings.TrimSpace(unit)
	}
	amount := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", *quantity), "0"), ".")
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return amount
	}
	return amount + " " + unit
}
