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
	var events []NormalizationEvent
	var providerRedacted RedactedProviderConfig
	usedProvider := false

	switch input.Mode {
	case "manual_structured":
		if input.CandidatePlan == nil {
			return PreparedRun{}, fmt.Errorf("candidate_plan is required for manual_structured")
		}
		plan = *input.CandidatePlan
		events = append(events, normalizationEvent("manual_plan_received", "manual structured meal plan received"))
	case "profile_generation", "prompt_generation":
		provider, err := providerFactory(input.Provider)
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
			return PreparedRun{}, err
		}
		events = append(events, normalizationEvent("llm_output_received", "provider returned candidate meal-plan JSON"))
		plan, err = decodePlanText(llmOutput)
		if err != nil {
			events = append(events, normalizationEvent("json_decode_failed", "initial provider output was not valid normalized meal-plan JSON"))
			if !input.RepairJSON {
				return PreparedRun{}, err
			}
			repairOutput, repairErr := provider.Complete(ctx, input.Provider, repairMessages(input, llmOutput, err))
			if repairErr != nil {
				return PreparedRun{}, repairErr
			}
			events = append(events, normalizationEvent("repair_attempted", "one bounded JSON repair attempt was made"))
			llmOutput = repairOutput
			plan, err = decodePlanText(repairOutput)
			if err != nil {
				return PreparedRun{}, err
			}
			events = append(events, normalizationEvent("repair_succeeded", "repair output decoded as normalized meal-plan JSON"))
		} else {
			events = append(events, normalizationEvent("json_decoded", "provider output decoded as normalized meal-plan JSON"))
		}
	default:
		return PreparedRun{}, fmt.Errorf("unsupported input_mode %q", input.Mode)
	}

	if err := validatePlan(plan); err != nil {
		return PreparedRun{}, err
	}

	casePath, err := writeRuntimeCase(config, run, input, plan)
	if err != nil {
		return PreparedRun{}, err
	}
	return PreparedRun{
		CasePath:            casePath,
		LLMOutput:           llmOutput,
		NormalizationEvents: events,
		RedactedProvider:    providerRedacted,
		UsedProvider:        usedProvider,
	}, nil
}

func generationMessages(input PendingRunInput) ([]ProviderMessage, error) {
	if input.Mode == "prompt_generation" && strings.TrimSpace(input.GenerationPrompt) == "" {
		return nil, fmt.Errorf("generation_prompt is required for prompt_generation")
	}
	system := strings.Join([]string{
		"You generate normalized MealCheck meal-plan JSON only.",
		"Return one JSON object matching schema_version 0.1.",
		"Do not include nutrient totals, calories, or compliance judgments.",
		"Every food item must include either quantity plus unit, or quantity_text with resolution_status unresolved and unresolved_reason.",
		"Allowed units are g, oz, cup, tbsp, tsp, and serving.",
		"Do not override declared allergies, excluded foods, or constraints.",
		"Do not provide medical claims.",
	}, " ")
	payload := map[string]any{
		"profile":     input.Profile,
		"constraints": input.Constraints,
		"required_shape": map[string]any{
			"schema_version": "0.1",
			"plan_id":        "string",
			"description":    "string",
			"days":           "array of day objects with meals and food items",
			"shopping_list":  "array of food items",
			"prep_notes":     "array of strings",
		},
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
		"Do not invent missing foods, quantities, units, nutrition totals, or compliance judgments.",
		"If a quantity is vague or missing, preserve it as quantity_text with resolution_status unresolved and unresolved_reason vague_quantity.",
		"Return only one JSON object.",
	}, " ")
	payload := map[string]any{
		"profile":         input.Profile,
		"constraints":     input.Constraints,
		"decode_error":    decodeErr.Error(),
		"original_output": original,
	}
	payloadJSON, _ := json.MarshalIndent(payload, "", "  ")
	return []ProviderMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: string(payloadJSON)},
	}
}

func decodePlanText(text string) (checker.Plan, error) {
	var plan checker.Plan
	jsonText, err := extractJSONObject(text)
	if err != nil {
		return checker.Plan{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(jsonText))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return checker.Plan{}, err
	}
	if decoder.More() {
		return checker.Plan{}, fmt.Errorf("meal plan JSON contains multiple values")
	}
	return plan, nil
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
		providerType = "openai_compatible"
	}
	return RedactedProviderConfig{
		Type:    providerType,
		BaseURL: config.BaseURL,
		Model:   config.Model,
		APIKey:  "redacted",
	}
}
