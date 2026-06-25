package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/checker"
)

const (
	defaultGuidelinePackID     = "dga-2025-2030-us-adult-general-v1"
	defaultGuidelinePackPath   = "data/guidelines/dga-2025-2030-us-adult-general-v1/guideline-pack.json"
	defaultNutrientCatalogID   = "fixture-catalog-v1"
	defaultNutrientCatalogPath = "data/nutrients/fixture-catalog-v1.json"
)

type PreparedRun struct {
	CasePath            string
	LLMOutput           string
	NormalizationEvents []NormalizationEvent
	RedactedProvider    RedactedProviderConfig
	UsedProvider        bool
}

type NormalizationEvent struct {
	Type      string `json:"type"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

func PrepareRunInput(ctx context.Context, config Config, providerFactory ProviderFactory, run Run, input PendingRunInput) (PreparedRun, error) {
	if input.Mode == "" {
		return PreparedRun{}, fmt.Errorf("input mode is required")
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
	var provider Provider
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
		provider, err = providerFactory(input.Provider)
		if err != nil {
			return PreparedRun{}, err
		}
		providerRedacted = redactProvider(input.Provider)
		usedProvider = true
		messages, err := generationMessages(input)
		if err != nil {
			return PreparedRun{}, err
		}
		llmOutput, err = provider.Complete(ctx, input.Provider, messages)
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
			repairOutput, repairErr = provider.Complete(ctx, input.Provider, repairMessages(input, sanitizeDebugArtifactText(llmOutput, input.Provider.APIKey), repairDecodeErr))
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
		provider, err = providerFactory(input.Provider)
		if err != nil {
			return PreparedRun{}, err
		}
		providerRedacted = redactProvider(input.Provider)
		usedProvider = true
		plan, llmOutput, events, err = prepareLocalModelExtraction(ctx, config, provider, run, input, events)
		if err != nil {
			return PreparedRun{}, err
		}
	default:
		return PreparedRun{}, fmt.Errorf("unsupported input_mode %q", input.Mode)
	}

	if usedProvider {
		var err error
		plan, llmOutput, repairOutput, repairErr, repairAttempted, events, err = normalizeGeneratedPlanPostDecode(ctx, config, provider, run, input, plan, llmOutput, initialOutput, initialErr, repairOutput, repairErr, repairAttempted, events)
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
		CasePath:            casePath,
		LLMOutput:           sanitizeDebugArtifactText(llmOutput, input.Provider.APIKey),
		NormalizationEvents: events,
		RedactedProvider:    providerRedacted,
		UsedProvider:        usedProvider,
	}, nil
}

type localModelSegmentOutput struct {
	Day    int    `json:"day,omitempty"`
	Output string `json:"output"`
}

type localModelExtractionFailureStage string

const (
	localModelFailureProvider     localModelExtractionFailureStage = "provider"
	localModelFailureDecode       localModelExtractionFailureStage = "decode"
	localModelFailureCompleteness localModelExtractionFailureStage = "completeness"
)

func prepareLocalModelExtraction(ctx context.Context, config Config, provider Provider, run Run, input PendingRunInput, events []NormalizationEvent) (checker.Plan, string, []NormalizationEvent, error) {
	if sections, ok := localModelDaySections(input.CandidateText); ok && len(sections) > 1 {
		return prepareDecomposedLocalModelExtraction(ctx, config, provider, run, input, sections, events)
	}
	output, plan, stage, err := requestLocalModelExtraction(ctx, provider, input.Provider, input, "local-model-"+run.ID)
	if err != nil {
		events = append(events, localModelFailureEvent(stage))
		return checker.Plan{}, output, events, writeLocalModelNormalizationFailureAndReturn(config, run, input, events, normalizationFailureDebug{
			InitialOutput: output,
			InitialError:  err,
			FinalError:    err,
		})
	}
	events = append(events, normalizationEvent("llm_output_received", "local model returned compact meal-plan JSON"))
	events = append(events, normalizationEvent("json_decoded", "local model compact output decoded into normalized MealCheck JSON"))
	return plan, output, events, nil
}

func prepareDecomposedLocalModelExtraction(ctx context.Context, config Config, provider Provider, run Run, input PendingRunInput, sections []localModelDaySection, events []NormalizationEvent) (checker.Plan, string, []NormalizationEvent, error) {
	events = append(events, normalizationEvent("local_model_decomposed", fmt.Sprintf("candidate text was split into %d per-day local model extraction calls", len(sections))))
	outputs := make([]localModelSegmentOutput, 0, len(sections))
	days := make([]checker.PlanDay, 0, len(sections))
	for _, section := range sections {
		sectionInput := input
		sectionInput.CandidateText = section.Text
		sectionInput.Settings.VerificationConstraints.Days = 1
		sectionInput.Settings.VerificationConstraints.MealsPerDay = 0
		output, plan, stage, err := requestLocalModelExtraction(ctx, provider, input.Provider, sectionInput, fmt.Sprintf("local-model-%s-day-%d", run.ID, section.Day))
		outputs = append(outputs, localModelSegmentOutput{Day: section.Day, Output: output})
		combinedOutput := formatLocalModelSegmentOutputs(outputs, true)
		if err != nil {
			err = fmt.Errorf("day %d local model extraction failed: %w", section.Day, err)
			events = append(events, localModelFailureEvent(stage))
			return checker.Plan{}, combinedOutput, events, writeLocalModelNormalizationFailureAndReturn(config, run, input, events, normalizationFailureDebug{
				InitialOutput: combinedOutput,
				InitialError:  err,
				FinalError:    err,
			})
		}
		if len(plan.Days) != 1 {
			err := fmt.Errorf("day %d local model extraction returned %d day object(s), want 1", section.Day, len(plan.Days))
			events = append(events, normalizationEvent("plan_constraints_failed", "per-day local model output did not preserve one day"))
			return checker.Plan{}, combinedOutput, events, writeLocalModelNormalizationFailureAndReturn(config, run, input, events, normalizationFailureDebug{
				InitialOutput: combinedOutput,
				InitialError:  err,
				FinalError:    err,
			})
		}
		day := plan.Days[0]
		day.Day = section.Day
		days = append(days, day)
		events = append(events, normalizationEvent("llm_output_received", fmt.Sprintf("local model returned compact meal-plan JSON for day %d", section.Day)))
	}
	plan := checker.Plan{
		SchemaVersion: "0.1",
		PlanID:        "local-model-" + run.ID,
		Days:          days,
	}
	combinedOutput := formatLocalModelSegmentOutputs(outputs, true)
	if err := validateLocalModelExtractionCompleteness(plan, input.CandidateText); err != nil {
		events = append(events, localModelFailureEvent(localModelFailureCompleteness))
		return checker.Plan{}, combinedOutput, events, writeLocalModelNormalizationFailureAndReturn(config, run, input, events, normalizationFailureDebug{
			InitialOutput: combinedOutput,
			InitialError:  err,
			FinalError:    err,
		})
	}
	events = append(events, normalizationEvent("json_decoded", "per-day local model compact outputs decoded and merged into normalized MealCheck JSON"))
	return plan, combinedOutput, events, nil
}

func requestLocalModelExtraction(ctx context.Context, provider Provider, providerConfig ProviderConfig, input PendingRunInput, planID string) (string, checker.Plan, localModelExtractionFailureStage, error) {
	messages, err := localModelExtractionMessages(input)
	if err != nil {
		return "", checker.Plan{}, localModelFailureDecode, err
	}
	output, err := provider.Complete(ctx, providerConfig, messages)
	if err != nil {
		return output, checker.Plan{}, localModelFailureProvider, err
	}
	plan, err := DecodeLocalLlamaCompactPlan(output, planID)
	if err != nil {
		return output, checker.Plan{}, localModelFailureDecode, err
	}
	if err := validateLocalModelExtractionCompleteness(plan, input.CandidateText); err != nil {
		return output, checker.Plan{}, localModelFailureCompleteness, err
	}
	return output, plan, "", nil
}

func localModelFailureEvent(stage localModelExtractionFailureStage) NormalizationEvent {
	switch stage {
	case localModelFailureProvider:
		return normalizationEvent("provider_request_failed", "local model request failed before returning compact meal-plan JSON")
	case localModelFailureCompleteness:
		return normalizationEvent("item_count_failed", "local model output did not preserve the resolved source item count")
	default:
		return normalizationEvent("json_decode_failed", "local model output was not valid compact meal-plan JSON")
	}
}

func formatLocalModelSegmentOutputs(outputs []localModelSegmentOutput, decomposed bool) string {
	if !decomposed && len(outputs) == 1 {
		return outputs[0].Output
	}
	payload := map[string]any{
		"mode":    "per_day",
		"outputs": outputs,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(b)
}

func normalizeGeneratedPlanPostDecode(ctx context.Context, config Config, provider Provider, run Run, input PendingRunInput, plan checker.Plan, llmOutput string, initialOutput string, initialErr error, repairOutput string, repairErr error, repairAttempted bool, events []NormalizationEvent) (checker.Plan, string, string, error, bool, []NormalizationEvent, error) {
	if normalizedPlan, changed := markUnsupportedUnitsUnresolved(plan); changed {
		plan = normalizedPlan
		events = append(events, normalizationEvent("unsupported_units_marked_unresolved", "provider numeric items with unsupported units were preserved as unresolved quantities"))
	}
	if err := validatePlan(plan); err != nil {
		events = append(events, normalizationEvent("plan_validation_failed", "decoded provider output failed MealCheck plan validation"))
		return checker.Plan{}, llmOutput, repairOutput, repairErr, repairAttempted, events, writeNormalizationFailureAndReturn(config, run, input.Provider, events, normalizationFailureDebug{
			InitialOutput: initialOutput,
			InitialError:  initialErr,
			RepairOutput:  repairOutput,
			RepairError:   repairErr,
			FinalError:    err,
		})
	}

	if err := validateGeneratedPlanAgainstConstraints(plan, input.Settings.VerificationConstraints); err != nil {
		events = append(events, normalizationEvent("plan_constraints_failed", "decoded provider output did not satisfy requested day and meal counts"))
		if !input.RepairJSON || repairAttempted {
			return checker.Plan{}, llmOutput, repairOutput, repairErr, repairAttempted, events, writeNormalizationFailureAndReturn(config, run, input.Provider, events, normalizationFailureDebug{
				InitialOutput: initialOutput,
				InitialError:  initialErr,
				RepairOutput:  repairOutput,
				RepairError:   repairErr,
				FinalError:    err,
			})
		}

		repairAttempted = true
		repairOutput, repairErr = provider.Complete(ctx, input.Provider, repairMessages(input, sanitizeDebugArtifactText(llmOutput, input.Provider.APIKey), sanitizeRepairPromptError(err, input.Provider.APIKey)))
		if repairErr != nil {
			return checker.Plan{}, llmOutput, repairOutput, repairErr, repairAttempted, events, writeNormalizationFailureAndReturn(config, run, input.Provider, events, normalizationFailureDebug{
				InitialOutput: initialOutput,
				InitialError:  initialErr,
				RepairError:   repairErr,
				FinalError:    repairErr,
			})
		}

		events = append(events, normalizationEvent("repair_attempted", "one bounded JSON repair attempt was made"))
		llmOutput = repairOutput
		decodeResult, decodeErr := decodePlanTextDetailed(repairOutput)
		if decodeErr != nil {
			return checker.Plan{}, llmOutput, repairOutput, decodeErr, repairAttempted, events, writeNormalizationFailureAndReturn(config, run, input.Provider, events, normalizationFailureDebug{
				InitialOutput: initialOutput,
				InitialError:  initialErr,
				RepairOutput:  repairOutput,
				RepairError:   decodeErr,
				FinalError:    decodeErr,
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
				InitialOutput: initialOutput,
				InitialError:  initialErr,
				RepairOutput:  repairOutput,
				RepairError:   err,
				FinalError:    err,
			})
		}
		if err := validateGeneratedPlanAgainstConstraints(plan, input.Settings.VerificationConstraints); err != nil {
			events = append(events, normalizationEvent("plan_constraints_failed", "repair output did not satisfy requested day and meal counts"))
			return checker.Plan{}, llmOutput, repairOutput, err, repairAttempted, events, writeNormalizationFailureAndReturn(config, run, input.Provider, events, normalizationFailureDebug{
				InitialOutput: initialOutput,
				InitialError:  initialErr,
				RepairOutput:  repairOutput,
				RepairError:   err,
				FinalError:    err,
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

func generationMessages(input PendingRunInput) ([]ProviderMessage, error) {
	if input.Mode == InputModePromptGeneration && strings.TrimSpace(input.GenerationPrompt) == "" {
		return nil, fmt.Errorf("generation_prompt is required for prompt_generation")
	}
	constraints := input.Settings.VerificationConstraints
	systemParts := []string{
		"You generate normalized MealCheck meal-plan JSON only.",
		"Return one JSON object matching schema_version 0.1.",
		mealPlanContractPromptBlock(),
		"Do not copy the shape instructions as the answer; generate a complete meal plan that satisfies the requested structure.",
		"Do not include nutrient totals, calories, or compliance judgments.",
		"Every food item must include either quantity plus unit, or quantity_text with resolution_status unresolved and unresolved_reason.",
		"Allowed units are g, oz, cup, tbsp, tsp, slice, and serving.",
		"Do not override declared allergies, excluded foods, or constraints.",
		"Do not provide medical claims.",
	}
	if instruction := generationCountInstruction(constraints); instruction != "" {
		systemParts = append(systemParts[:3], append([]string{instruction}, systemParts[3:]...)...)
	} else {
		systemParts = append(systemParts[:3], append([]string{"Return one or more day objects with one or more meal objects per day, using the day and meal structure requested by the user prompt or source text."}, systemParts[3:]...)...)
	}
	system := strings.Join(systemParts, " ")
	payload := map[string]any{
		"settings":       input.Settings,
		"required_shape": mealPlanShapeInstructions(constraints),
		"alias_rules":    mealPlanAliasRules(),
	}
	if counts := explicitCountPayload(constraints); len(counts) > 0 {
		payload["required_counts"] = counts
	}
	if input.Mode == InputModePromptGeneration {
		payload["user_prompt"] = input.GenerationPrompt
	}
	payloadJSON, _ := json.MarshalIndent(payload, "", "  ")
	return []ProviderMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: string(payloadJSON)},
	}, nil
}

func localModelExtractionMessages(input PendingRunInput) ([]ProviderMessage, error) {
	text := strings.TrimSpace(input.CandidateText)
	if text == "" {
		return nil, fmt.Errorf("candidate_text is required for local_model")
	}
	constraints := input.Settings.VerificationConstraints
	system := strings.Join([]string{
		"Extract meal-plan ingredients into compact MealCheck local JSON only.",
		"Return one minified JSON object.",
		"Shape: {\"i\":[[source_item_id,day,meal_code,food,quantity,unit]]}.",
		"Meal codes: b=breakfast, m=morning snack, l=lunch, a=afternoon snack, d=dinner, s=snack, e=evening snack.",
		"When the user states the exact allowed meal codes, use only those meal codes.",
		"Allowed units: g, oz, cup, tbsp, tsp, slice, serving.",
	}, " ")
	userParts := []string{
		"Extract this meal plan into compact row JSON.",
		"Use only the numbered source items below.",
		localLlamaItemCountInstruction(text),
		"Convert every numbered source item into exactly one [source_item_id, day, meal_code, food, quantity, unit] tuple.",
		"Copy each source_item_id exactly once in ascending order; do not skip or duplicate source_item_id values.",
		"Use the provided day value for each numbered source item.",
		"When meal_code is one of b, m, l, a, d, s, or e, use that provided meal_code. When meal_code is infer, infer the closest supported meal code from the source context.",
		"Do not omit, merge, summarize, or invent items.",
		"Parse only food, numeric quantity, and unit from each source_text.",
		"The food value must be the food name only; do not include the leading quantity or unit in the food value.",
		"Do not include other keys or text.",
		"",
		localLlamaSourceItemsPromptBlock(text),
	}
	if constraints.Days > 0 {
		userParts = append(userParts[:2], append([]string{fmt.Sprintf("Use day numbers 1..%d.", constraints.Days)}, userParts[2:]...)...)
	}
	if constraints.MealsPerDay > 0 {
		userParts = append(userParts[:3], append([]string{
			fmt.Sprintf("Each day must contain exactly %d distinct meal code(s).", constraints.MealsPerDay),
			localLlamaMealCodeInstruction(constraints.MealsPerDay),
		}, userParts[3:]...)...)
	}
	user := strings.Join(userParts, "\n")
	return []ProviderMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, nil
}

func generationCountInstruction(constraints checker.VerificationConstraints) string {
	switch {
	case constraints.Days > 0 && constraints.MealsPerDay > 0:
		return fmt.Sprintf("Return exactly %d day object(s), and every day must contain exactly %d meal object(s).", constraints.Days, constraints.MealsPerDay)
	case constraints.Days > 0:
		return fmt.Sprintf("Return exactly %d day object(s), with meal objects that match the user prompt or source text.", constraints.Days)
	case constraints.MealsPerDay > 0:
		return fmt.Sprintf("Return one or more day objects, and every day must contain exactly %d meal object(s).", constraints.MealsPerDay)
	default:
		return ""
	}
}

func explicitCountPayload(constraints checker.VerificationConstraints) map[string]int {
	counts := map[string]int{}
	if constraints.Days > 0 {
		counts["days"] = constraints.Days
	}
	if constraints.MealsPerDay > 0 {
		counts["meals_per_day"] = constraints.MealsPerDay
	}
	return counts
}

func localLlamaMealCodeInstruction(mealsPerDay int) string {
	codes := localLlamaMealCodesForCount(mealsPerDay)
	return fmt.Sprintf("Use exactly these meal codes for every day: %s. Do not use any other meal codes.", strings.Join(codes, ", "))
}

func localLlamaItemCountInstruction(text string) string {
	expected := len(localLlamaResolvedSourceItems(text))
	if expected == 0 {
		return "Preserve every resolved food item that has a numeric quantity plus supported unit."
	}
	return fmt.Sprintf("The source contains exactly %d resolved food item line(s); return exactly %d row(s).", expected, expected)
}

type localLlamaSourceItem struct {
	ID       int
	Day      int
	MealCode string
	Text     string
}

type localModelDaySection struct {
	Day  int
	Text string
}

var (
	localLlamaResolvedItemLinePattern = regexp.MustCompile(`(?i)^\s*(?:[-*]|\d+[.)])\s+(?:\d+(?:\.\d+)?|\d+\s*/\s*\d+|\d+\s+\d+\s*/\s*\d+)\s*(?:g|grams?|oz|ounces?|cups?|tbsp|tablespoons?|tsp|teaspoons?|slices?|servings?)\b`)
	localLlamaInlineItemPattern       = regexp.MustCompile(`(?i)^\s*((?:\d+(?:\.\d+)?)|(?:\d+\s*/\s*\d+)|(?:\d+\s+\d+\s*/\s*\d+))\s+((?:g|grams?|oz|ounces?|cups?|tbsp|tablespoons?|tsp|teaspoons?|slices?|servings?)\s+)?(.+?)\s*$`)
	localLlamaInlineAndItemBoundary   = regexp.MustCompile(`(?i)\s+\band\s+((?:\d+(?:\.\d+)?|\d+\s*/\s*\d+|\d+\s+\d+\s*/\s*\d+)\s+)`)
	localLlamaInlineLeadingAnd        = regexp.MustCompile(`(?i)^\s*and\s+`)
	localLlamaSourceItemMarkerPattern = regexp.MustCompile(`^\s*(?:[-*]|\d+[.)])\s+`)
	localLlamaDayHeadingPattern       = regexp.MustCompile(`(?i)\bday\s*([1-7])\b`)
)

func localLlamaExpectedResolvedItemCount(text string) int {
	return len(localLlamaResolvedSourceItems(text))
}

func localModelDaySections(text string) ([]localModelDaySection, bool) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) == 0 {
		return nil, false
	}

	var sections []localModelDaySection
	var current strings.Builder
	currentDay := 0
	seen := map[int]bool{}
	sawDayMarker := false

	flush := func() bool {
		if currentDay == 0 {
			return true
		}
		sectionText := strings.TrimSpace(current.String())
		if sectionText == "" || localLlamaExpectedResolvedItemCount(sectionText) == 0 {
			return false
		}
		sections = append(sections, localModelDaySection{Day: currentDay, Text: sectionText})
		seen[currentDay] = true
		current.Reset()
		return true
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if currentDay != 0 {
				current.WriteString("\n")
			}
			continue
		}
		day := localLlamaDayFromHeading(trimmed)
		if day > 0 {
			sawDayMarker = true
			if currentDay == 0 {
				currentDay = day
			} else if day != currentDay {
				if seen[day] || !flush() {
					return nil, false
				}
				currentDay = day
			}
			line = localLlamaRewriteDayHeading(line, 1)
		} else if currentDay == 0 {
			return nil, false
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	if !sawDayMarker || !flush() {
		return nil, false
	}
	return sections, len(sections) > 1
}

func localLlamaRewriteDayHeading(line string, day int) string {
	return localLlamaDayHeadingPattern.ReplaceAllString(line, fmt.Sprintf("Day %d", day))
}

func localLlamaSourceItemsPromptBlock(text string) string {
	sourceItems := localLlamaResolvedSourceItems(text)
	if len(sourceItems) == 0 {
		return "Source meal plan text:\n" + text
	}
	var b strings.Builder
	b.WriteString("Numbered resolved source items:\n")
	for _, item := range sourceItems {
		mealCode := item.MealCode
		if mealCode == "" {
			mealCode = "infer"
		}
		fmt.Fprintf(&b, "%d | day=%d | meal_code=%s | source_text=%s\n", item.ID, item.Day, mealCode, item.Text)
	}
	return strings.TrimRight(b.String(), "\n")
}

func localLlamaResolvedSourceItems(text string) []localLlamaSourceItem {
	var items []localLlamaSourceItem
	currentDay := 1
	currentMealCode := ""
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		isItemLine := localLlamaResolvedItemLinePattern.MatchString(line)
		if !isItemLine {
			if day := localLlamaDayFromHeading(trimmed); day > 0 {
				currentDay = day
			}
			if mealCode := localLlamaMealCodeFromHeading(trimmed); mealCode != "" {
				currentMealCode = mealCode
			}
			inlineItems := localLlamaInlineSourceItems(trimmed, currentDay, currentMealCode, len(items)+1)
			if len(inlineItems) > 0 {
				items = append(items, inlineItems...)
			}
			continue
		}
		items = append(items, localLlamaSourceItem{
			ID:       len(items) + 1,
			Day:      currentDay,
			MealCode: currentMealCode,
			Text:     localLlamaCleanSourceItemLine(line),
		})
	}
	return items
}

func localLlamaInlineSourceItems(line string, day int, mealCode string, startID int) []localLlamaSourceItem {
	if !strings.Contains(line, ":") {
		return nil
	}
	_, remainder, found := strings.Cut(line, ":")
	if !found {
		return nil
	}
	remainder = strings.TrimSpace(remainder)
	if remainder == "" {
		return nil
	}
	var items []localLlamaSourceItem
	for _, phrase := range localLlamaSplitInlineItemPhrases(remainder) {
		sourceText, ok := localLlamaNormalizeInlineItemPhrase(phrase)
		if !ok {
			continue
		}
		items = append(items, localLlamaSourceItem{
			ID:       startID + len(items),
			Day:      day,
			MealCode: mealCode,
			Text:     sourceText,
		})
	}
	return items
}

func localLlamaSplitInlineItemPhrases(text string) []string {
	normalized := strings.ReplaceAll(text, ";", ",")
	parts := strings.Split(normalized, ",")
	phrases := make([]string, 0, len(parts))
	for _, part := range parts {
		for _, subpart := range localLlamaSplitInlineAndQuantified(part) {
			phrase := strings.TrimSpace(strings.Trim(subpart, "."))
			phrase = localLlamaInlineLeadingAnd.ReplaceAllString(phrase, "")
			if phrase != "" {
				phrases = append(phrases, phrase)
			}
		}
	}
	return phrases
}

func localLlamaSplitInlineAndQuantified(part string) []string {
	remaining := strings.TrimSpace(part)
	if remaining == "" {
		return nil
	}
	var phrases []string
	for {
		matches := localLlamaInlineAndItemBoundary.FindStringSubmatchIndex(remaining)
		if len(matches) == 0 {
			return append(phrases, remaining)
		}
		if left := strings.TrimSpace(remaining[:matches[0]]); left != "" {
			phrases = append(phrases, left)
		}
		remaining = strings.TrimSpace(remaining[matches[2]:])
		if remaining == "" {
			return phrases
		}
	}
}

func localLlamaNormalizeInlineItemPhrase(phrase string) (string, bool) {
	matches := localLlamaInlineItemPattern.FindStringSubmatch(strings.TrimSpace(phrase))
	if len(matches) != 4 {
		return "", false
	}
	quantity := strings.Join(strings.Fields(matches[1]), " ")
	unit := strings.TrimSpace(matches[2])
	food := strings.TrimSpace(matches[3])
	if quantity == "" || food == "" {
		return "", false
	}
	if unit == "" {
		unit = "serving"
	}
	unit = localLlamaNormalizeSourceUnit(unit)
	return strings.TrimSpace(quantity + " " + unit + " " + food), true
}

func localLlamaNormalizeSourceUnit(unit string) string {
	normalized := strings.ToLower(strings.TrimSpace(unit))
	switch normalized {
	case "gram", "grams":
		return "g"
	case "ounce", "ounces":
		return "oz"
	case "cups":
		return "cup"
	case "tablespoon", "tablespoons":
		return "tbsp"
	case "teaspoon", "teaspoons":
		return "tsp"
	case "slices":
		return "slice"
	case "servings":
		return "serving"
	default:
		return normalized
	}
}

func localLlamaDayFromHeading(line string) int {
	matches := localLlamaDayHeadingPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return 0
	}
	day, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return day
}

func localLlamaMealCodeFromHeading(line string) string {
	heading := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(line, ":")))
	switch {
	case strings.Contains(heading, "breakfast"):
		return "b"
	case strings.Contains(heading, "morning snack"):
		return "m"
	case strings.Contains(heading, "lunch"):
		return "l"
	case strings.Contains(heading, "afternoon snack"):
		return "a"
	case strings.Contains(heading, "dinner"):
		return "d"
	case strings.Contains(heading, "evening snack"):
		return "e"
	case strings.Contains(heading, "snack"):
		return "s"
	default:
		return ""
	}
}

func localLlamaCleanSourceItemLine(line string) string {
	return strings.TrimSpace(localLlamaSourceItemMarkerPattern.ReplaceAllString(line, ""))
}

func validateLocalModelExtractionCompleteness(plan checker.Plan, sourceText string) error {
	expected := localLlamaExpectedResolvedItemCount(sourceText)
	if expected == 0 {
		return nil
	}
	got := countMealPlanItems(plan)
	if got != expected {
		return fmt.Errorf("local model extracted %d resolved food item(s); expected %d from source resolved food item lines", got, expected)
	}
	return nil
}

func countMealPlanItems(plan checker.Plan) int {
	count := 0
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			count += len(meal.Items)
		}
	}
	return count
}

func localLlamaMealCodesForCount(mealsPerDay int) []string {
	switch mealsPerDay {
	case 1:
		return []string{"d"}
	case 2:
		return []string{"b", "d"}
	case 3:
		return []string{"b", "l", "d"}
	case 4:
		return []string{"b", "l", "a", "d"}
	case 5:
		return []string{"b", "m", "l", "a", "d"}
	case 6:
		return []string{"b", "m", "l", "a", "d", "e"}
	default:
		return []string{"b", "l", "d"}
	}
}

func repairMessages(input PendingRunInput, original string, decodeErr error) []ProviderMessage {
	constraints := input.Settings.VerificationConstraints
	systemParts := []string{
		"Repair MealCheck meal-plan JSON syntax or minor schema shape only.",
		mealPlanContractPromptBlock(),
		"Do not invent nutrition totals or compliance judgments.",
		"If a quantity is vague or missing, preserve it as quantity_text with resolution_status unresolved and unresolved_reason vague_quantity.",
		"Remove invalid alias fields after mapping them to allowed MealCheck fields.",
		"Return only one JSON object.",
	}
	if instruction := generationCountInstruction(constraints); instruction != "" {
		systemParts = append(systemParts[:2], append([]string{strings.Replace(instruction, "Return", "The repaired output must contain", 1)}, systemParts[2:]...)...)
		systemParts = append(systemParts[:4], append([]string{"If day or meal count is wrong, add or remove meal objects while preserving declared allergies, excluded foods, constraints, and any valid existing foods."}, systemParts[4:]...)...)
	} else {
		systemParts = append(systemParts[:2], append([]string{"Preserve the day and meal structure from the original output; do not add or remove days or meals to satisfy a default count."}, systemParts[2:]...)...)
	}
	system := strings.Join(systemParts, " ")
	payload := map[string]any{
		"settings":        input.Settings,
		"decode_error":    decodeErr.Error(),
		"required_shape":  mealPlanShapeInstructions(constraints),
		"alias_rules":     mealPlanAliasRules(),
		"original_output": original,
	}
	payloadJSON, _ := json.MarshalIndent(payload, "", "  ")
	return []ProviderMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: string(payloadJSON)},
	}
}

func extractJSONObject(text string) (string, error) {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 3 {
			trimmed = strings.Join(lines[1:len(lines)-1], "\n")
		}
		trimmed = strings.TrimSpace(trimmed)
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start < 0 || end < start {
		return "", fmt.Errorf("no JSON object found in provider output")
	}
	return trimmed[start : end+1], nil
}

func validateSettings(settings checker.Settings) error {
	return checker.ValidateSettings(settings)
}

func validatePlan(plan checker.Plan) error {
	if plan.SchemaVersion != "0.1" {
		return fmt.Errorf("meal plan schema_version must be 0.1")
	}
	if strings.TrimSpace(plan.PlanID) == "" {
		return fmt.Errorf("meal plan plan_id is required")
	}
	if len(plan.Days) == 0 {
		return fmt.Errorf("meal plan days are required")
	}
	for _, day := range plan.Days {
		if day.Day < 1 {
			return fmt.Errorf("meal plan day must be positive")
		}
		if len(day.Meals) == 0 {
			return fmt.Errorf("meal plan day %d has no meals", day.Day)
		}
		for _, meal := range day.Meals {
			if strings.TrimSpace(meal.Name) == "" {
				return fmt.Errorf("meal plan day %d has a meal without a name", day.Day)
			}
			if len(meal.Items) == 0 {
				return fmt.Errorf("meal plan day %d meal %s has no items", day.Day, meal.Name)
			}
			for _, item := range meal.Items {
				if strings.TrimSpace(item.Food) == "" {
					return fmt.Errorf("meal plan day %d meal %s has an item without food", day.Day, meal.Name)
				}
				if item.Quantity != nil {
					if *item.Quantity <= 0 {
						return fmt.Errorf("meal plan item %s quantity must be positive", item.Food)
					}
					if !allowedUnit(item.Unit) {
						return fmt.Errorf("meal plan item %s has unsupported unit %q", item.Food, item.Unit)
					}
					continue
				}
				if item.QuantityText == "" || item.ResolutionStatus != "unresolved" || item.UnresolvedReason == "" {
					return fmt.Errorf("meal plan item %s must include quantity/unit or unresolved quantity fields", item.Food)
				}
			}
		}
	}
	return nil
}

func validateGeneratedPlanAgainstConstraints(plan checker.Plan, constraints checker.VerificationConstraints) error {
	if constraints.Days > 0 && len(plan.Days) != constraints.Days {
		return fmt.Errorf("meal plan must include exactly %d day(s); got %d", constraints.Days, len(plan.Days))
	}
	seenDays := make(map[int]bool, len(plan.Days))
	for _, day := range plan.Days {
		if day.Day < 1 {
			return fmt.Errorf("meal plan day number %d is outside expected range 1..N", day.Day)
		}
		if constraints.Days > 0 && day.Day > constraints.Days {
			return fmt.Errorf("meal plan day number %d is outside expected range 1..%d", day.Day, constraints.Days)
		}
		if seenDays[day.Day] {
			return fmt.Errorf("meal plan includes duplicate day %d", day.Day)
		}
		seenDays[day.Day] = true
		if constraints.MealsPerDay > 0 && len(day.Meals) != constraints.MealsPerDay {
			return fmt.Errorf("meal plan day %d must include exactly %d meal(s); got %d", day.Day, constraints.MealsPerDay, len(day.Meals))
		}
	}
	if constraints.Days > 0 {
		for day := 1; day <= constraints.Days; day++ {
			if !seenDays[day] {
				return fmt.Errorf("meal plan is missing day %d", day)
			}
		}
	}
	return nil
}

func allowedUnit(unit string) bool {
	switch unit {
	case "g", "oz", "cup", "tbsp", "tsp", "slice", "serving":
		return true
	default:
		return false
	}
}

func writeRuntimeCase(config Config, run Run, input PendingRunInput, plan checker.Plan) (string, error) {
	inputDir := runtimeInputDir(config, run.ID)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		return "", err
	}
	planPath := filepath.Join(inputDir, "candidate-plan.json")
	casePath := runtimeCasePath(config, run.ID)
	if err := writeJSONFile(planPath, plan); err != nil {
		return "", err
	}
	c := checker.Case{
		SchemaVersion:       "0.1",
		CaseID:              run.ID,
		InputMode:           input.Mode,
		Settings:            input.Settings,
		GuidelinePackID:     defaultGuidelinePackID,
		GuidelinePackPath:   defaultGuidelinePackPath,
		NutrientCatalogID:   defaultNutrientCatalogID,
		NutrientCatalogPath: defaultNutrientCatalogPath,
		CandidatePlan:       planPath,
		Expectations:        checker.Expectations{},
		Tags:                []string{"hosted", input.Mode},
	}
	if input.GenerationPrompt != "" {
		c.GenerationPrompt = input.GenerationPrompt
	}
	if input.CandidateText != "" {
		c.GenerationPrompt = "[local_model candidate_text omitted from runtime case]"
	}
	if err := writeJSONFile(casePath, c); err != nil {
		return "", err
	}
	return casePath, nil
}

func runtimeInputDir(config Config, runID string) string {
	return filepath.Join(config.DataDir, "run-inputs", runID)
}

func runtimeCasePath(config Config, runID string) string {
	return filepath.Join(runtimeInputDir(config, runID), "case.json")
}

func writeOptionalArtifacts(outDir string, prepared PreparedRun) error {
	if prepared.UsedProvider {
		if err := writeJSONFile(filepath.Join(outDir, "optional", "llm-output.json"), map[string]any{"output": prepared.LLMOutput}); err != nil {
			return err
		}
	}
	if len(prepared.NormalizationEvents) > 0 {
		if err := writeJSONFile(filepath.Join(outDir, "optional", "normalization-events.json"), prepared.NormalizationEvents); err != nil {
			return err
		}
	}
	if prepared.UsedProvider {
		if err := writeJSONFile(filepath.Join(outDir, "configs", "redacted-provider.json"), prepared.RedactedProvider); err != nil {
			return err
		}
	}
	if prepared.UsedProvider || len(prepared.NormalizationEvents) > 0 {
		return updateManifestOptionals(outDir, prepared)
	}
	return nil
}

type normalizationFailureDebug struct {
	InitialOutput string
	InitialError  error
	RepairOutput  string
	RepairError   error
	FinalError    error
}

type normalizationFailureArtifact struct {
	SchemaVersion       string                 `json:"schema_version"`
	RunID               string                 `json:"run_id"`
	CreatedAt           string                 `json:"created_at"`
	Provider            RedactedProviderConfig `json:"provider"`
	NormalizationEvents []NormalizationEvent   `json:"normalization_events"`
	InitialOutput       string                 `json:"initial_output,omitempty"`
	InitialError        string                 `json:"initial_error,omitempty"`
	RepairOutput        string                 `json:"repair_output,omitempty"`
	RepairError         string                 `json:"repair_error,omitempty"`
	FinalError          string                 `json:"final_error,omitempty"`
}

func writeNormalizationFailureAndReturn(config Config, run Run, provider ProviderConfig, events []NormalizationEvent, failure normalizationFailureDebug) error {
	finalErr := failure.FinalError
	if finalErr == nil {
		finalErr = failure.RepairError
	}
	if finalErr == nil {
		finalErr = failure.InitialError
	}
	if finalErr == nil {
		finalErr = fmt.Errorf("provider output failed normalization")
	}
	if err := writeNormalizationFailureDebug(config, run, provider, events, failure); err != nil {
		return fmt.Errorf("%w; additionally failed to write normalization debug artifact: %v", finalErr, err)
	}
	return finalErr
}

func writeLocalModelNormalizationFailureAndReturn(config Config, run Run, input PendingRunInput, events []NormalizationEvent, failure normalizationFailureDebug) error {
	result := classifyCandidateMealPlanText(input.CandidateText)
	if isTerminalQualificationFailure(result) {
		events = append(events, normalizationEvent("qualification_failed_post_model", "candidate text was classified as not ready for verification after local model normalization failed"))
	} else {
		events = append(events, normalizationEvent("normalization_graceful_failed", "local model output could not be normalized into a verifiable meal plan"))
	}
	if failure.FinalError == nil {
		failure.FinalError = failure.InitialError
	}
	if failure.FinalError == nil {
		failure.FinalError = fmt.Errorf("local model output failed normalization")
	}
	publicMessage := localModelPublicFailureMessage(result)
	if err := writeNormalizationFailureDebug(config, run, input.Provider, events, failure); err != nil {
		return fmt.Errorf("%s; additionally failed to write normalization debug artifact: %v", publicMessage, err)
	}
	return fmt.Errorf("%s", publicMessage)
}

func localModelPublicFailureMessage(result MealPlanQualificationResult) string {
	if isTerminalQualificationFailure(result) && strings.TrimSpace(result.Reason) != "" {
		return result.Reason
	}
	return "MealCheck could not normalize this text into a verifiable meal plan. Use clear day labels, meal labels, food names, numeric quantities, and supported units."
}

func writeNormalizationFailureDebug(config Config, run Run, provider ProviderConfig, events []NormalizationEvent, failure normalizationFailureDebug) error {
	debugDir := filepath.Join(run.ArtifactDir, "debug")
	if err := os.MkdirAll(debugDir, 0o755); err != nil {
		return err
	}
	artifact := normalizationFailureArtifact{
		SchemaVersion:       "0.1",
		RunID:               run.ID,
		CreatedAt:           time.Now().UTC().Format(time.RFC3339),
		Provider:            redactProvider(provider),
		NormalizationEvents: append([]NormalizationEvent(nil), events...),
		InitialOutput:       sanitizeDebugArtifactText(failure.InitialOutput, provider.APIKey),
		InitialError:        sanitizeDebugError(failure.InitialError, provider.APIKey),
		RepairOutput:        sanitizeDebugArtifactText(failure.RepairOutput, provider.APIKey),
		RepairError:         sanitizeDebugError(failure.RepairError, provider.APIKey),
		FinalError:          sanitizeDebugError(failure.FinalError, provider.APIKey),
	}
	return writeJSONFile(filepath.Join(debugDir, "normalization-failure.json"), artifact)
}

func sanitizeDebugError(err error, apiKey string) string {
	if err == nil {
		return ""
	}
	return sanitizeProviderErrorText(err.Error(), apiKey)
}

func sanitizeRepairPromptError(err error, apiKey string) error {
	message := sanitizeDebugError(err, apiKey)
	if message == "" {
		message = "provider output failed MealCheck JSON decode"
	}
	return fmt.Errorf("%s", message)
}

func sanitizeDebugArtifactText(text, apiKey string) string {
	if text == "" {
		return ""
	}
	if apiKey != "" {
		text = strings.ReplaceAll(text, apiKey, "[redacted]")
	}
	const maxDebugArtifactTextLength = 200_000
	if len(text) > maxDebugArtifactTextLength {
		return text[:maxDebugArtifactTextLength] + "\n[truncated]"
	}
	return text
}

func updateManifestOptionals(outDir string, prepared PreparedRun) error {
	manifestPath := filepath.Join(outDir, "manifest.json")
	var manifest struct {
		SchemaVersion string            `json:"schema_version"`
		CaseID        string            `json:"case_id"`
		Mode          string            `json:"mode"`
		GeneratedAt   string            `json:"generated_at"`
		MealCheck     map[string]string `json:"mealcheck"`
		Inputs        map[string]string `json:"inputs"`
		Artifacts     []string          `json:"artifacts"`
	}
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		return err
	}
	if prepared.UsedProvider {
		manifest.Artifacts = appendIfMissing(manifest.Artifacts, "optional/llm-output.json")
	}
	if len(prepared.NormalizationEvents) > 0 {
		manifest.Artifacts = appendIfMissing(manifest.Artifacts, "optional/normalization-events.json")
	}
	return writeJSONFile(manifestPath, manifest)
}

func appendIfMissing(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func writeJSONFile(path string, data any) error {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

func normalizationEvent(eventType, message string) NormalizationEvent {
	return NormalizationEvent{
		Type:      eventType,
		Message:   message,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func redactProvider(config ProviderConfig) RedactedProviderConfig {
	providerType := config.Type
	if providerType == "" {
		providerType = ProviderTypeOpenAICompatible
	}
	if providerType == ProviderTypeLocalLlama {
		return RedactedProviderConfig{
			Type:   providerType,
			Model:  filepath.Base(config.Model),
			APIKey: "not_applicable",
		}
	}
	return RedactedProviderConfig{
		Type:    providerType,
		BaseURL: config.BaseURL,
		Model:   config.Model,
		APIKey:  "redacted",
	}
}
