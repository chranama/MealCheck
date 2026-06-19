package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	if err := validateProfileAndConstraints(input.Profile, input.Constraints); err != nil {
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
	case "manual_structured":
		if input.CandidatePlan == nil {
			return PreparedRun{}, fmt.Errorf("candidate_plan is required for manual_structured")
		}
		plan = *input.CandidatePlan
		events = append(events, normalizationEvent("manual_plan_received", "manual structured meal plan received"))
	case "profile_generation", "prompt_generation":
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

func normalizeGeneratedPlanPostDecode(ctx context.Context, config Config, provider Provider, run Run, input PendingRunInput, plan checker.Plan, llmOutput string, initialOutput string, initialErr error, repairOutput string, repairErr error, repairAttempted bool, events []NormalizationEvent) (checker.Plan, string, string, error, bool, []NormalizationEvent, error) {
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

	if err := validateGeneratedPlanAgainstConstraints(plan, input.Constraints); err != nil {
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
		if err := validateGeneratedPlanAgainstConstraints(plan, input.Constraints); err != nil {
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

func generationMessages(input PendingRunInput) ([]ProviderMessage, error) {
	if input.Mode == "prompt_generation" && strings.TrimSpace(input.GenerationPrompt) == "" {
		return nil, fmt.Errorf("generation_prompt is required for prompt_generation")
	}
	system := strings.Join([]string{
		"You generate normalized MealCheck meal-plan JSON only.",
		"Return one JSON object matching schema_version 0.1.",
		mealPlanContractPromptBlock(),
		fmt.Sprintf("Return exactly %d day object(s), and every day must contain exactly %d meal object(s).", input.Constraints.Days, input.Constraints.MealsPerDay),
		"Do not copy the shape instructions as the answer; generate a complete meal plan that satisfies the requested counts.",
		"Do not include nutrient totals, calories, or compliance judgments.",
		"Every food item must include either quantity plus unit, or quantity_text with resolution_status unresolved and unresolved_reason.",
		"Allowed units are g, oz, cup, tbsp, tsp, and serving.",
		"Do not override declared allergies, excluded foods, or constraints.",
		"Do not provide medical claims.",
	}, " ")
	payload := map[string]any{
		"settings": providerPromptSettings(input.Profile, input.Constraints),
		"required_counts": map[string]int{
			"days":          input.Constraints.Days,
			"meals_per_day": input.Constraints.MealsPerDay,
		},
		"required_shape": mealPlanShapeInstructions(input.Constraints),
		"alias_rules":    mealPlanAliasRules(),
	}
	if input.Mode == "prompt_generation" {
		payload["user_prompt"] = input.GenerationPrompt
	}
	payloadJSON, _ := json.MarshalIndent(payload, "", "  ")
	return []ProviderMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: string(payloadJSON)},
	}, nil
}

func repairMessages(input PendingRunInput, original string, decodeErr error) []ProviderMessage {
	system := strings.Join([]string{
		"Repair MealCheck meal-plan JSON syntax or minor schema shape only.",
		mealPlanContractPromptBlock(),
		fmt.Sprintf("The repaired output must contain exactly %d day object(s), and every day must contain exactly %d meal object(s).", input.Constraints.Days, input.Constraints.MealsPerDay),
		"Do not invent nutrition totals or compliance judgments.",
		"If day or meal count is wrong, add or remove meal objects while preserving declared allergies, excluded foods, constraints, and any valid existing foods.",
		"If a quantity is vague or missing, preserve it as quantity_text with resolution_status unresolved and unresolved_reason vague_quantity.",
		"Remove invalid alias fields after mapping them to allowed MealCheck fields.",
		"Return only one JSON object.",
	}, " ")
	payload := map[string]any{
		"settings":        providerPromptSettings(input.Profile, input.Constraints),
		"decode_error":    decodeErr.Error(),
		"required_shape":  mealPlanShapeInstructions(input.Constraints),
		"alias_rules":     mealPlanAliasRules(),
		"original_output": original,
	}
	payloadJSON, _ := json.MarshalIndent(payload, "", "  ")
	return []ProviderMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: string(payloadJSON)},
	}
}

type providerPromptNutritionTargets struct {
	CalorieTargetKcal int `json:"calorie_target_kcal,omitempty"`
	ProteinTargetG    int `json:"protein_target_g,omitempty"`
}

type providerPromptConstraints struct {
	Days                       int      `json:"days,omitempty"`
	MealsPerDay                int      `json:"meals_per_day,omitempty"`
	Allergies                  []string `json:"allergies,omitempty"`
	ExcludedFoods              []string `json:"excluded_foods,omitempty"`
	MaxSodiumMGPerDay          int      `json:"max_sodium_mg_per_day,omitempty"`
	MaxAddedSugarGPerMeal      float64  `json:"max_added_sugar_g_per_meal,omitempty"`
	MaxSaturatedFatPctCalories float64  `json:"max_saturated_fat_pct_calories,omitempty"`
	CalorieTolerancePct        float64  `json:"calorie_tolerance_pct,omitempty"`
	RequiresPrepSafetyNotes    bool     `json:"requires_prep_safety_notes"`
}

func providerPromptSettings(profile checker.Profile, constraints checker.Constraints) map[string]any {
	return map[string]any{
		"nutrition_targets": providerPromptNutritionTargets{
			CalorieTargetKcal: profile.CalorieTargetKcal,
			ProteinTargetG:    profile.ProteinTargetG,
		},
		"verification_constraints": providerPromptConstraints{
			Days:                       constraints.Days,
			MealsPerDay:                constraints.MealsPerDay,
			Allergies:                  constraints.Allergies,
			ExcludedFoods:              constraints.ExcludedFoods,
			MaxSodiumMGPerDay:          constraints.MaxSodiumMGPerDay,
			MaxAddedSugarGPerMeal:      constraints.MaxAddedSugarGPerMeal,
			MaxSaturatedFatPctCalories: constraints.MaxSaturatedFatPctCalories,
			CalorieTolerancePct:        constraints.CalorieTolerancePct,
			RequiresPrepSafetyNotes:    constraints.RequiresPrepSafetyNotes,
		},
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

func validateProfileAndConstraints(profile checker.Profile, constraints checker.Constraints) error {
	if profile.Age < 18 {
		return fmt.Errorf("profile age must be at least 18")
	}
	if profile.Sex != "male" && profile.Sex != "female" {
		return fmt.Errorf("profile sex must be male or female")
	}
	if profile.HeightCM <= 0 || profile.WeightKG <= 0 {
		return fmt.Errorf("profile height_cm and weight_kg must be positive")
	}
	switch profile.ActivityLevel {
	case "inactive", "low_active", "moderate", "active", "very_active":
	default:
		return fmt.Errorf("profile activity_level must be inactive, low_active, moderate, active, or very_active")
	}
	if constraints.Days < 1 || constraints.Days > 7 {
		return fmt.Errorf("constraints days must be between 1 and 7")
	}
	if constraints.MealsPerDay < 1 || constraints.MealsPerDay > 6 {
		return fmt.Errorf("constraints meals_per_day must be between 1 and 6")
	}
	return nil
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

func validateGeneratedPlanAgainstConstraints(plan checker.Plan, constraints checker.Constraints) error {
	if len(plan.Days) != constraints.Days {
		return fmt.Errorf("meal plan must include exactly %d day(s); got %d", constraints.Days, len(plan.Days))
	}
	seenDays := make(map[int]bool, len(plan.Days))
	for _, day := range plan.Days {
		if day.Day < 1 || day.Day > constraints.Days {
			return fmt.Errorf("meal plan day number %d is outside expected range 1..%d", day.Day, constraints.Days)
		}
		if seenDays[day.Day] {
			return fmt.Errorf("meal plan includes duplicate day %d", day.Day)
		}
		seenDays[day.Day] = true
		if len(day.Meals) != constraints.MealsPerDay {
			return fmt.Errorf("meal plan day %d must include exactly %d meal(s); got %d", day.Day, constraints.MealsPerDay, len(day.Meals))
		}
	}
	for day := 1; day <= constraints.Days; day++ {
		if !seenDays[day] {
			return fmt.Errorf("meal plan is missing day %d", day)
		}
	}
	return nil
}

func allowedUnit(unit string) bool {
	switch unit {
	case "g", "oz", "cup", "tbsp", "tsp", "serving":
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
		Profile:             input.Profile,
		Constraints:         input.Constraints,
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
	return RedactedProviderConfig{
		Type:    providerType,
		BaseURL: config.BaseURL,
		Model:   config.Model,
		APIKey:  "redacted",
	}
}
