package normalization

import (
	"encoding/json"
	"fmt"

	"github.com/chranama/MealCheck/internal/assist"
)

func BuildP0AssistRequestPayload(result Result, chunk AssistChunk) AssistRequestPayload {
	return AssistRequestPayload{
		Task:               P0AssistTask,
		ChunkID:            chunk.ID,
		SourceItems:        append([]SourceItem(nil), chunk.SourceItems...),
		AllowedUnits:       AllowedAssistUnits(),
		AllowedMealCodes:   AllowedAssistMealCodes(),
		FixedSourceItemIDs: deterministicSourceItemIDs(result.ParsedItems),
	}
}

func P0AssistMessages(payload AssistRequestPayload) []assist.Message {
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		encoded = []byte(fmt.Sprintf(`{"task":%q,"chunk_id":%q}`, payload.Task, payload.ChunkID))
	}
	return []assist.Message{
		{
			Role: "system",
			Content: stringsJoinLines(
				"You repair MealCheck meal-plan source items into canonical rows.",
				"Return JSON only.",
				"Use only source_item_id values present in the request.",
				"Do not invent foods, nutrient values, source ids, or additional rows.",
				"For each source item, choose propose_row, needs_clarification, or abstain.",
				"For propose_row, use one allowed unit and one allowed meal_code.",
				"For needs_clarification or abstain, leave day as 0, meal_code empty, food empty, quantity 0, and unit empty.",
			),
		},
		{
			Role:    "user",
			Content: string(encoded),
		},
	}
}

func deterministicSourceItemIDs(parsedItems []ParsedSourceItem) []int {
	rows := DeterministicRows(parsedItems)
	ids := make([]int, 0, len(rows))
	for _, row := range rows {
		if row.SourceItemID > 0 {
			ids = append(ids, row.SourceItemID)
		}
	}
	return ids
}

func stringsJoinLines(lines ...string) string {
	if len(lines) == 0 {
		return ""
	}
	total := 0
	for _, line := range lines {
		total += len(line) + 1
	}
	buf := make([]byte, 0, total)
	for i, line := range lines {
		if i > 0 {
			buf = append(buf, '\n')
		}
		buf = append(buf, line...)
	}
	return string(buf)
}
