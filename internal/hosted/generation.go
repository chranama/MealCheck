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

type localModelExtractionFailureStage string

const (
	localModelFailureProvider     localModelExtractionFailureStage = "provider"
	localModelFailureDecode       localModelExtractionFailureStage = "decode"
	localModelFailureCompleteness localModelExtractionFailureStage = "completeness"
)

func prepareLocalModelExtraction(ctx context.Context, config Config, provider Provider, run Run, input PendingRunInput, events []NormalizationEvent) (checker.Plan, string, []NormalizationEvent, error) {
	output, plan, repairs, stage, err := requestLocalModelExtraction(ctx, provider, input.Provider, input, "local-model-"+run.ID)
	if err != nil {
		events = append(events, localModelFailureEvent(stage))
		return checker.Plan{}, output, events, writeLocalModelNormalizationFailureAndReturn(config, run, input, events, normalizationFailureDebug{
			InitialOutput: output,
			InitialError:  err,
			FinalError:    err,
		})
	}
	events = append(events, normalizationEvent("llm_output_received", "local model returned compact meal-plan JSON"))
	if len(repairs) > 0 {
		events = append(events, normalizationEvent("source_measurements_reconciled", fmt.Sprintf("local model compact rows were repaired from %d deterministic source field(s)", len(repairs))))
	}
	events = append(events, normalizationEvent("json_decoded", "local model compact output decoded into normalized MealCheck JSON"))
	return plan, output, events, nil
}

// RunLocalModelExtraction normalizes hosted local-model text through the same
// chunked extraction path used by live run creation.
func RunLocalModelExtraction(ctx context.Context, provider Provider, providerConfig ProviderConfig, input PendingRunInput, planID string) (string, checker.Plan, []LocalLlamaNormalizationRepair, string, error) {
	output, plan, repairs, stage, err := requestLocalModelExtraction(ctx, provider, providerConfig, input, planID)
	return output, plan, repairs, string(stage), err
}

func requestLocalModelExtraction(ctx context.Context, provider Provider, providerConfig ProviderConfig, input PendingRunInput, planID string) (string, checker.Plan, []LocalLlamaNormalizationRepair, localModelExtractionFailureStage, error) {
	chunks := localLlamaExtractionMealChunks(input.CandidateText)
	if len(chunks) == 0 {
		return "", checker.Plan{}, nil, localModelFailureDecode, fmt.Errorf("candidate_text must identify at least one meal chunk with source food items")
	}
	var outputs []string
	var rows []localLlamaRowItem
	var repairs []LocalLlamaNormalizationRepair
	for _, chunk := range chunks {
		messages, err := localModelExtractionMessagesForMealChunk(input, chunk)
		if err != nil {
			return localLlamaCombinedChunkOutput(outputs), checker.Plan{}, repairs, localModelFailureDecode, err
		}
		output, err := provider.Complete(ctx, providerConfig, messages)
		outputs = append(outputs, localLlamaChunkOutputBlock(chunk, output))
		if err != nil {
			return localLlamaCombinedChunkOutput(outputs), checker.Plan{}, repairs, localModelFailureProvider, err
		}
		chunkRows, chunkRepairs, err := decodeLocalLlamaMealChunkRows(output, chunk)
		repairs = append(repairs, chunkRepairs...)
		if err != nil {
			return localLlamaCombinedChunkOutput(outputs), checker.Plan{}, repairs, localModelFailureDecode, err
		}
		rows = append(rows, chunkRows...)
	}
	output := localLlamaCombinedChunkOutput(outputs)
	plan, err := expandLocalLlamaRows(rows, planID)
	if err != nil {
		return output, checker.Plan{}, repairs, localModelFailureDecode, err
	}
	if err := validateLocalModelExtractionCompleteness(plan, input.CandidateText); err != nil {
		return output, checker.Plan{}, repairs, localModelFailureCompleteness, err
	}
	return output, plan, repairs, "", nil
}

func localLlamaChunkOutputBlock(chunk localLlamaMealChunk, output string) string {
	return fmt.Sprintf("Meal chunk day=%d meal_code=%s source_ids=%s\n%s", chunk.Day, chunk.MealCode, localLlamaChunkSourceIDList(chunk), strings.TrimSpace(output))
}

func localLlamaCombinedChunkOutput(outputs []string) string {
	return strings.TrimSpace(strings.Join(outputs, "\n\n"))
}

func localModelFailureEvent(stage localModelExtractionFailureStage) NormalizationEvent {
	switch stage {
	case localModelFailureProvider:
		return normalizationEvent("provider_request_failed", "local model request failed before returning compact meal-plan JSON")
	case localModelFailureCompleteness:
		return normalizationEvent("item_count_failed", "local model output did not preserve the numbered source item count")
	default:
		return normalizationEvent("json_decode_failed", "local model output was not valid compact meal-plan JSON")
	}
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
	chunks := localLlamaExtractionMealChunks(text)
	if len(chunks) == 0 {
		return nil, fmt.Errorf("candidate_text must identify at least one meal chunk with source food items")
	}
	return localModelExtractionMessagesForMealChunk(input, chunks[0])
}

func localModelExtractionMessagesForMealChunk(input PendingRunInput, chunk localLlamaMealChunk) ([]ProviderMessage, error) {
	input.Settings = normalizeLocalModelSettings(input.Settings)
	constraints := input.Settings.VerificationConstraints
	system := strings.Join([]string{
		"Extract meal-plan ingredients into compact MealCheck local JSON only.",
		"Return one minified JSON object.",
		"Shape: {\"i\":[[source_item_id,food,quantity,unit]]}.",
		"For a food without a usable quantity, use [source_item_id,food,null,\"\",quantity_text,unresolved_reason].",
		"Allowed units: g, oz, cup, tbsp, tsp, slice, serving.",
		"Allowed unresolved_reason values: missing_quantity, vague_quantity, unsupported_unit.",
	}, " ")
	mealLabel := chunk.MealLabel
	if mealLabel == "" {
		mealLabel, _ = localLlamaMealName(chunk.MealCode)
	}
	userParts := []string{
		"Extract this meal chunk into compact row JSON.",
		fmt.Sprintf("Meal: day=%d meal_code=%s meal_label=%s.", chunk.Day, chunk.MealCode, mealLabel),
		localLlamaChunkItemCountInstruction(chunk),
		"Use only the numbered source items below.",
		"Use the meal text only as context for resolving the listed source items.",
		"Do not add foods from the meal text unless they appear in Source items.",
		"Convert every numbered source item into exactly one row.",
		"Copy each source_item_id exactly once in ascending order; do not skip or duplicate source_item_id values.",
		"The numbered source item list is authoritative for source_item_id and source wording.",
		"The server already knows day and meal_code; do not output day or meal_code.",
		"Do not omit, merge, summarize, or invent items.",
		"Parse food, numeric quantity, and unit from each source_text when present.",
		"For resolved source_text, copy quantity and unit from the leading measurement; preserve fractions such as 1/2 as 0.5.",
		"For needs_model_parse source_text, infer food and usable quantity from the source_text and meal text when possible; otherwise put the visible amount phrase in quantity_text when one exists, or use \"missing quantity\".",
		"Do not change tbsp to tsp or tsp to tbsp.",
		"The food value must be the food name only; do not include the leading quantity, unit, quantity_text, or unresolved_reason in the food value.",
		"Preserve source food wording, including preparation words such as cooked, plain, grilled, steamed, baked, roasted, sliced, or mixed.",
		"Do not include other keys or text.",
		"",
		localLlamaMealChunkPromptBlock(chunk),
	}
	if constraints.Days > 0 {
		userParts = append(userParts[:2], append([]string{fmt.Sprintf("The full request is limited to day numbers 1..%d.", constraints.Days)}, userParts[2:]...)...)
	}
	if constraints.MealsPerDay > 0 {
		userParts = append(userParts[:3], append([]string{
			fmt.Sprintf("The full request contains exactly %d distinct meal chunk(s) for the day.", constraints.MealsPerDay),
		}, userParts[3:]...)...)
	}
	user := strings.Join(userParts, "\n")
	return []ProviderMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, nil
}

// LocalModelExtractionMessages returns the compact local-model extraction
// prompt used by the hosted local-model path.
func LocalModelExtractionMessages(input PendingRunInput) ([]ProviderMessage, error) {
	return localModelExtractionMessages(input)
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
	expected := len(localLlamaExtractionSourceItems(text))
	if expected == 0 {
		return "Preserve every food item that can be tied to a day or meal in the source text."
	}
	return fmt.Sprintf("The source contains exactly %d numbered source item(s); return exactly %d row(s).", expected, expected)
}

type localLlamaSourceItem struct {
	ID          int
	Day         int
	MealCode    string
	Text        string
	ParseStatus localLlamaSourceParseStatus
}

type localLlamaSourceParseStatus string

const (
	localLlamaSourceResolved        localLlamaSourceParseStatus = "resolved"
	localLlamaSourceNeedsModelParse localLlamaSourceParseStatus = "needs_model_parse"
)

type localLlamaMealChunk struct {
	Day       int
	MealCode  string
	MealLabel string
	MealText  string
	Items     []localLlamaSourceItem
}

// LocalLlamaSourceItem is the deterministic source-item inventory used before
// compact local-model extraction.
type LocalLlamaSourceItem struct {
	ID          int
	Day         int
	MealCode    string
	Text        string
	ParseStatus string
}

var (
	localLlamaResolvedItemLinePattern = regexp.MustCompile(`(?i)^\s*(?:[-*]|\d+[.)])\s+(?:\d+(?:\.\d+)?|\d+\s*/\s*\d+|\d+\s+\d+\s*/\s*\d+)\s*(?:g|grams?|oz|ounces?|cups?|tbsp|tablespoons?|tsp|teaspoons?|slices?|servings?)\b`)
	localLlamaAnyItemLinePattern      = regexp.MustCompile(`^\s*(?:[-*]|\d+[.)])\s+(.+?)\s*$`)
	localLlamaInlineItemPattern       = regexp.MustCompile(`(?i)^\s*((?:\d+(?:\.\d+)?)|(?:\d+\s*/\s*\d+)|(?:\d+\s+\d+\s*/\s*\d+))\s+((?:g|grams?|oz|ounces?|cups?|tbsp|tablespoons?|tsp|teaspoons?|slices?|servings?)\s+)?(.+?)\s*$`)
	localLlamaReverseItemPattern      = regexp.MustCompile(`(?i)^\s*(.+?)\s*(?:,|-|\()\s*((?:\d+(?:\.\d+)?)|(?:\d+\s*/\s*\d+)|(?:\d+\s+\d+\s*/\s*\d+))\s+(g|grams?|oz|ounces?|cups?|tbsp|tablespoons?|tsp|teaspoons?|slices?|servings?)\)?\s*$`)
	localLlamaMeasurementOnlyPattern  = regexp.MustCompile(`(?i)^\s*(?:\d+(?:\.\d+)?|\d+\s*/\s*\d+|\d+\s+\d+\s*/\s*\d+)\s+(?:g|grams?|oz|ounces?|cups?|tbsp|tablespoons?|tsp|teaspoons?|slices?|servings?)\s*$`)
	localLlamaInlineAndItemBoundary   = regexp.MustCompile(`(?i)\s+\b(?:and|with|plus)\s+((?:\d+(?:\.\d+)?|\d+\s*/\s*\d+|\d+\s+\d+\s*/\s*\d+)\s+)`)
	localLlamaInlineLeadingAnd        = regexp.MustCompile(`(?i)^\s*(?:and|with|plus)\s+`)
	localLlamaMealPhrasePrefix        = regexp.MustCompile(`(?i)^\s*(?:i\s+(?:will\s+)?(?:have|had|ate|eat)|will\s+have|had|have|ate|eat|includes?|include|was|is|were|are)\s+`)
	localLlamaTrailingMealVerb        = regexp.MustCompile(`(?i)\b(?:includes?|include|was|is|were|are|has|had|have)\s*$`)
	localLlamaLeadingOf               = regexp.MustCompile(`(?i)^of\s+`)
	localLlamaSourceItemMarkerPattern = regexp.MustCompile(`^\s*(?:[-*]|\d+[.)])\s+`)
	localLlamaDayHeadingPattern       = regexp.MustCompile(`(?i)\bday\s*([1-7])\b`)
	localLlamaParagraphMealPattern    = regexp.MustCompile(`(?i)\b(?:for|at|as|my|the)?\s*(morning snack|afternoon snack|evening snack|breakfast|lunch|dinner|snack)\b`)
	localLlamaParagraphQuantityStart  = regexp.MustCompile(`(?i)(?:\d+(?:\.\d+)?|\d+\s*/\s*\d+|\d+\s+\d+\s*/\s*\d+)`)
)

func localLlamaExpectedResolvedItemCount(text string) int {
	return len(localLlamaResolvedSourceItems(text))
}

func localLlamaExpectedExtractionItemCount(text string) int {
	return len(localLlamaExtractionSourceItems(text))
}

// LocalLlamaExpectedResolvedItemCount returns the number of deterministic
// source items MealCheck expects the local model to preserve.
func LocalLlamaExpectedResolvedItemCount(text string) int {
	return localLlamaExpectedResolvedItemCount(text)
}

// LocalLlamaResolvedSourceItems returns the deterministic source-item inventory
// used in local-model prompts.
func LocalLlamaResolvedSourceItems(text string) []LocalLlamaSourceItem {
	internal := localLlamaResolvedSourceItems(text)
	items := make([]LocalLlamaSourceItem, 0, len(internal))
	for _, item := range internal {
		items = append(items, LocalLlamaSourceItem{
			ID:          item.ID,
			Day:         item.Day,
			MealCode:    item.MealCode,
			Text:        item.Text,
			ParseStatus: string(item.ParseStatus),
		})
	}
	return items
}

func localLlamaSourceItemsPromptBlock(text string) string {
	sourceItems := localLlamaExtractionSourceItems(text)
	if len(sourceItems) == 0 {
		return "Source meal plan text:\n" + text
	}
	var b strings.Builder
	b.WriteString("Numbered source items:\n")
	for _, item := range sourceItems {
		mealCode := item.MealCode
		if mealCode == "" {
			mealCode = "infer"
		}
		fmt.Fprintf(&b, "%d | day=%d | meal_code=%s | status=%s | source_text=%s\n", item.ID, item.Day, mealCode, item.ParseStatus, item.Text)
	}
	return strings.TrimRight(b.String(), "\n")
}

func localLlamaMealChunkPromptBlock(chunk localLlamaMealChunk) string {
	var b strings.Builder
	b.WriteString("Meal text:\n")
	b.WriteString(strings.TrimSpace(chunk.MealText))
	b.WriteString("\n\nSource items:\n")
	for _, item := range chunk.Items {
		fmt.Fprintf(&b, "%d | status=%s | source_text=%s\n", item.ID, item.ParseStatus, item.Text)
	}
	return strings.TrimRight(b.String(), "\n")
}

func localLlamaChunkItemCountInstruction(chunk localLlamaMealChunk) string {
	return fmt.Sprintf("This meal chunk contains exactly %d numbered source item(s); return exactly %d row(s).", len(chunk.Items), len(chunk.Items))
}

func localLlamaChunkSourceIDList(chunk localLlamaMealChunk) string {
	ids := make([]string, 0, len(chunk.Items))
	for _, item := range chunk.Items {
		ids = append(ids, strconv.Itoa(item.ID))
	}
	return strings.Join(ids, ",")
}

func localLlamaResolvedSourceItems(text string) []localLlamaSourceItem {
	return localLlamaSourceItems(text, false)
}

func localLlamaExtractionSourceItems(text string) []localLlamaSourceItem {
	return localLlamaSourceItems(text, true)
}

func localLlamaSourceItems(text string, includeUnresolved bool) []localLlamaSourceItem {
	chunks := localLlamaMealChunks(text, includeUnresolved)
	var items []localLlamaSourceItem
	for _, chunk := range chunks {
		items = append(items, chunk.Items...)
	}
	return items
}

func localLlamaExtractionMealChunks(text string) []localLlamaMealChunk {
	return localLlamaMealChunks(text, true)
}

func localLlamaMealChunks(text string, includeUnresolved bool) []localLlamaMealChunk {
	var chunks []localLlamaMealChunk
	currentDay := 1
	currentMealCode := ""
	currentChunkIndex := -1
	nextID := 1
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
				currentChunkIndex = localLlamaEnsureMealChunk(&chunks, currentDay, currentMealCode, trimmed)
			}
			inlineItems := localLlamaInlineSourceItems(trimmed, currentDay, currentMealCode, nextID, includeUnresolved)
			paragraphChunks := localLlamaParagraphMealChunks(trimmed, currentDay, nextID, includeUnresolved)
			if localLlamaMealChunkItemCount(paragraphChunks) > len(inlineItems) {
				chunks = append(chunks, paragraphChunks...)
				nextID += localLlamaMealChunkItemCount(paragraphChunks)
				if len(paragraphChunks) > 0 {
					last := paragraphChunks[len(paragraphChunks)-1]
					currentDay = last.Day
					currentMealCode = last.MealCode
					currentChunkIndex = -1
				}
				continue
			}
			if len(inlineItems) > 0 {
				if currentMealCode == "" {
					currentMealCode = "infer"
				}
				currentChunkIndex = localLlamaEnsureMealChunk(&chunks, currentDay, currentMealCode, trimmed)
				chunks[currentChunkIndex].Items = append(chunks[currentChunkIndex].Items, inlineItems...)
				nextID += len(inlineItems)
				continue
			}
			if len(paragraphChunks) > 0 {
				chunks = append(chunks, paragraphChunks...)
				nextID += localLlamaMealChunkItemCount(paragraphChunks)
				last := paragraphChunks[len(paragraphChunks)-1]
				currentDay = last.Day
				currentMealCode = last.MealCode
				currentChunkIndex = -1
				continue
			}
			if includeUnresolved {
				if text, ok := localLlamaUnresolvedItemLine(line); ok {
					if currentMealCode == "" {
						currentMealCode = "infer"
					}
					currentChunkIndex = localLlamaEnsureMealChunk(&chunks, currentDay, currentMealCode, trimmed)
					chunks[currentChunkIndex].Items = append(chunks[currentChunkIndex].Items, localLlamaSourceItem{
						ID:          nextID,
						Day:         currentDay,
						MealCode:    currentMealCode,
						Text:        text,
						ParseStatus: localLlamaSourceNeedsModelParse,
					})
					nextID++
				}
			}
			continue
		}
		if currentMealCode == "" {
			currentMealCode = "infer"
		}
		currentChunkIndex = localLlamaEnsureMealChunk(&chunks, currentDay, currentMealCode, trimmed)
		cleaned := localLlamaCleanSourceItemLine(line)
		item := localLlamaSourceItemFromText(nextID, currentDay, currentMealCode, cleaned, includeUnresolved)
		if item.ParseStatus != "" {
			chunks[currentChunkIndex].Items = append(chunks[currentChunkIndex].Items, item)
			nextID++
		}
	}
	return localLlamaNonEmptyMealChunks(chunks)
}

func localLlamaEnsureMealChunk(chunks *[]localLlamaMealChunk, day int, mealCode string, mealText string) int {
	if mealCode == "" {
		mealCode = "infer"
	}
	for index := len(*chunks) - 1; index >= 0; index-- {
		chunk := &(*chunks)[index]
		if chunk.Day == day && chunk.MealCode == mealCode {
			localLlamaAppendMealText(chunk, mealText)
			return index
		}
	}
	mealLabel, _ := localLlamaMealName(mealCode)
	if mealLabel == "" {
		mealLabel = mealCode
	}
	*chunks = append(*chunks, localLlamaMealChunk{
		Day:       day,
		MealCode:  mealCode,
		MealLabel: mealLabel,
		MealText:  strings.TrimSpace(mealText),
	})
	return len(*chunks) - 1
}

func localLlamaAppendMealText(chunk *localLlamaMealChunk, mealText string) {
	mealText = strings.TrimSpace(mealText)
	if mealText == "" {
		return
	}
	if chunk.MealText == "" {
		chunk.MealText = mealText
		return
	}
	if strings.Contains(chunk.MealText, mealText) {
		return
	}
	chunk.MealText = strings.TrimSpace(chunk.MealText + "\n" + mealText)
}

func localLlamaNonEmptyMealChunks(chunks []localLlamaMealChunk) []localLlamaMealChunk {
	filtered := make([]localLlamaMealChunk, 0, len(chunks))
	for _, chunk := range chunks {
		if len(chunk.Items) > 0 {
			filtered = append(filtered, chunk)
		}
	}
	return filtered
}

func localLlamaMealChunkItemCount(chunks []localLlamaMealChunk) int {
	count := 0
	for _, chunk := range chunks {
		count += len(chunk.Items)
	}
	return count
}

func localLlamaSourceItemFromText(id int, day int, mealCode string, sourceText string, includeUnresolved bool) localLlamaSourceItem {
	sourceText = strings.TrimSpace(sourceText)
	if sourceText == "" {
		return localLlamaSourceItem{}
	}
	status := localLlamaSourceNeedsModelParse
	if localLlamaParseSourceMeasurement(sourceText).Status == "parsed" {
		status = localLlamaSourceResolved
	} else if !includeUnresolved {
		return localLlamaSourceItem{}
	}
	return localLlamaSourceItem{
		ID:          id,
		Day:         day,
		MealCode:    mealCode,
		Text:        sourceText,
		ParseStatus: status,
	}
}

func localLlamaParagraphMealChunks(line string, day int, startID int, includeUnresolved bool) []localLlamaMealChunk {
	matches := localLlamaParagraphMealPattern.FindAllStringSubmatchIndex(line, -1)
	if len(matches) == 0 {
		return nil
	}
	var chunks []localLlamaMealChunk
	nextID := startID
	for index, match := range matches {
		if len(match) < 4 {
			continue
		}
		mealLabel := strings.ToLower(strings.TrimSpace(line[match[2]:match[3]]))
		mealCode := localLlamaMealCodeFromHeading(mealLabel)
		if mealCode == "" {
			continue
		}
		sectionStart := match[1]
		sectionEnd := len(line)
		if index+1 < len(matches) {
			sectionEnd = matches[index+1][0]
		}
		section := strings.TrimSpace(line[sectionStart:sectionEnd])
		if section == "" {
			continue
		}
		chunk := localLlamaMealChunk{
			Day:       day,
			MealCode:  mealCode,
			MealLabel: mealLabel,
			MealText:  strings.TrimSpace(line[match[0]:sectionEnd]),
		}
		for _, sourceText := range localLlamaParagraphSourceTexts(section, includeUnresolved) {
			item := localLlamaSourceItemFromText(nextID, day, mealCode, sourceText, includeUnresolved)
			if item.ParseStatus == "" {
				continue
			}
			chunk.Items = append(chunk.Items, item)
			nextID++
		}
		if len(chunk.Items) > 0 {
			chunks = append(chunks, chunk)
		}
	}
	return chunks
}

func localLlamaParagraphSourceTexts(section string, includeUnresolved bool) []string {
	normalized := strings.ReplaceAll(section, ";", ",")
	parts := localLlamaSplitCommaItemParts(normalized)
	sourceTexts := make([]string, 0, len(parts))
	for _, part := range parts {
		for _, phrase := range localLlamaSplitInlineAndQuantified(part) {
			sourceText, ok := localLlamaNormalizeParagraphItemPhrase(phrase)
			if !ok && includeUnresolved {
				sourceText, ok = localLlamaNormalizeUnresolvedParagraphItemPhrase(phrase)
			}
			if ok {
				sourceTexts = append(sourceTexts, sourceText)
			}
		}
	}
	return sourceTexts
}

func localLlamaNormalizeParagraphItemPhrase(phrase string) (string, bool) {
	text := strings.TrimSpace(strings.Trim(phrase, " \t\r\n.;,"))
	text = localLlamaInlineLeadingAnd.ReplaceAllString(text, "")
	if text == "" {
		return "", false
	}
	if sourceText, ok := localLlamaNormalizeInlineItemPhrase(text); ok {
		return sourceText, true
	}
	if match := localLlamaParagraphQuantityStart.FindStringIndex(text); len(match) == 2 {
		text = strings.TrimSpace(text[match[0]:])
	}
	return localLlamaNormalizeInlineItemPhrase(text)
}

func localLlamaInlineSourceItems(line string, day int, mealCode string, startID int, includeUnresolved bool) []localLlamaSourceItem {
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
		if !ok && includeUnresolved {
			sourceText, ok = localLlamaNormalizeUnresolvedInlineItemPhrase(phrase)
		}
		if !ok {
			continue
		}
		item := localLlamaSourceItemFromText(startID+len(items), day, mealCode, sourceText, includeUnresolved)
		if item.ParseStatus != "" {
			items = append(items, item)
		}
	}
	return items
}

func localLlamaNormalizeUnresolvedInlineItemPhrase(phrase string) (string, bool) {
	text := strings.TrimSpace(strings.Trim(phrase, " \t\r\n.;,"))
	text = localLlamaInlineLeadingAnd.ReplaceAllString(text, "")
	text = localLlamaCleanMealContextPrefix(text)
	if text == "" {
		return "", false
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, ":") || strings.HasPrefix(lower, "day ") {
		return "", false
	}
	return text, true
}

func localLlamaUnresolvedItemLine(line string) (string, bool) {
	matches := localLlamaAnyItemLinePattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return "", false
	}
	text := strings.TrimSpace(strings.Trim(matches[1], " \t\r\n.;,"))
	if text == "" || strings.Contains(text, ":") {
		return "", false
	}
	return text, true
}

func localLlamaSplitInlineItemPhrases(text string) []string {
	normalized := strings.ReplaceAll(text, ";", ",")
	parts := localLlamaSplitCommaItemParts(normalized)
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

func localLlamaSplitCommaItemParts(text string) []string {
	rawParts := strings.Split(text, ",")
	parts := make([]string, 0, len(rawParts))
	for index := 0; index < len(rawParts); index++ {
		part := strings.TrimSpace(rawParts[index])
		if part == "" {
			continue
		}
		if index+1 < len(rawParts) {
			next := strings.TrimSpace(rawParts[index+1])
			if localLlamaShouldMergeReverseMeasurementParts(part, next) {
				parts = append(parts, part+", "+next)
				index++
				continue
			}
		}
		parts = append(parts, part)
	}
	return parts
}

func localLlamaShouldMergeReverseMeasurementParts(foodPart string, measurementPart string) bool {
	if foodPart == "" || measurementPart == "" {
		return false
	}
	if localLlamaParagraphQuantityStart.MatchString(foodPart) {
		return false
	}
	return localLlamaMeasurementOnlyPattern.MatchString(strings.Trim(measurementPart, " .;"))
}

func localLlamaNormalizeInlineItemPhrase(phrase string) (string, bool) {
	if sourceText, ok := localLlamaNormalizeReverseItemPhrase(phrase); ok {
		return sourceText, true
	}
	matches := localLlamaInlineItemPattern.FindStringSubmatch(strings.TrimSpace(phrase))
	if len(matches) != 4 {
		return "", false
	}
	quantity := strings.Join(strings.Fields(matches[1]), " ")
	unit := strings.TrimSpace(matches[2])
	food := localLlamaCleanMealContextPrefix(strings.TrimSpace(matches[3]))
	if quantity == "" || food == "" {
		return "", false
	}
	if unit != "" {
		food = strings.TrimSpace(localLlamaLeadingOf.ReplaceAllString(food, ""))
	}
	if unit == "" {
		unit = "serving"
	}
	unit = localLlamaNormalizeSourceUnit(unit)
	return strings.TrimSpace(quantity + " " + unit + " " + food), true
}

func localLlamaNormalizeReverseItemPhrase(phrase string) (string, bool) {
	matches := localLlamaReverseItemPattern.FindStringSubmatch(strings.TrimSpace(strings.Trim(phrase, " \t\r\n.;")))
	if len(matches) != 4 {
		return "", false
	}
	food := localLlamaCleanMealContextPrefix(matches[1])
	food = strings.TrimSpace(strings.Trim(food, " \t\r\n.;:-()"))
	if food == "" {
		return "", false
	}
	quantity := strings.Join(strings.Fields(matches[2]), " ")
	unit := localLlamaNormalizeSourceUnit(matches[3])
	return strings.TrimSpace(quantity + " " + unit + " " + food), true
}

func localLlamaNormalizeUnresolvedParagraphItemPhrase(phrase string) (string, bool) {
	text := strings.TrimSpace(strings.Trim(phrase, " \t\r\n.;,"))
	text = localLlamaInlineLeadingAnd.ReplaceAllString(text, "")
	text = localLlamaCleanMealContextPrefix(text)
	if text == "" {
		return "", false
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, ":") || strings.HasPrefix(lower, "day ") {
		return "", false
	}
	return text, true
}

func localLlamaCleanMealContextPrefix(text string) string {
	text = strings.TrimSpace(text)
	for {
		cleaned := strings.TrimSpace(localLlamaMealPhrasePrefix.ReplaceAllString(text, ""))
		cleaned = strings.TrimSpace(localLlamaTrailingMealVerb.ReplaceAllString(cleaned, ""))
		if cleaned == text {
			break
		}
		text = cleaned
	}
	return strings.TrimSpace(text)
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
	expected := localLlamaExpectedExtractionItemCount(sourceText)
	if expected == 0 {
		return nil
	}
	got := countMealPlanItems(plan)
	if got != expected {
		return fmt.Errorf("local model extracted %d food item(s); expected %d from numbered source items", got, expected)
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
	result := classifyLocalModelCandidateMealPlanText(input.CandidateText)
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
