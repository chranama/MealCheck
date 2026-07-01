package normalize

import (
	"encoding/json"
	"fmt"
	"strings"

	localmodel "github.com/chranama/MealCheck/internal/llm/local"
	"github.com/chranama/MealCheck/internal/workflow/checker"
)

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

func GenerationMessages(input PendingRunInput) ([]ProviderMessage, error) {
	return generationMessages(input)
}

// LocalModelExtractionMessages returns the compact local-model extraction
// prompt used by the hosted local-model path.
func LocalModelExtractionMessages(input PendingRunInput) ([]ProviderMessage, error) {
	return localmodel.LocalModelExtractionMessages(input)
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
	expected := localmodel.LocalLlamaExpectedExtractionItemCount(text)
	if expected == 0 {
		return "Preserve every food item that can be tied to a day or meal in the source text."
	}
	return fmt.Sprintf("The source contains exactly %d numbered source item(s); return exactly %d row(s).", expected, expected)
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
