package localmodel

import (
	"fmt"
	"strings"
)

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

func localLlamaItemCountInstruction(text string) string {
	expected := localLlamaExpectedExtractionItemCount(text)
	if expected == 0 {
		return "Preserve every food item that can be tied to a day or meal in the source text."
	}
	return fmt.Sprintf("The source contains exactly %d numbered source item(s); return exactly %d row(s).", expected, expected)
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

func LocalModelExtractionMessages(input PendingRunInput) ([]ProviderMessage, error) {
	return localModelExtractionMessages(input)
}
