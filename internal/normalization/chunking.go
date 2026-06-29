package normalization

import "fmt"

type AssistChunk struct {
	ID            string       `json:"id"`
	Day           int          `json:"day,omitempty"`
	MealCode      string       `json:"meal_code,omitempty"`
	SourceItemIDs []int        `json:"source_item_ids"`
	SourceItems   []SourceItem `json:"source_items"`
}

func ChunkSourceItems(sourceItems []SourceItem, policy Policy) []AssistChunk {
	if len(sourceItems) == 0 {
		return nil
	}
	maxItems := policy.MaxSourceItemsPerChunk
	if maxItems <= 0 {
		maxItems = 6
	}

	var chunks []AssistChunk
	var current []SourceItem
	currentDay := 0
	currentMealCode := ""
	flush := func() {
		if len(current) == 0 {
			return
		}
		chunks = append(chunks, makeAssistChunk(len(chunks)+1, currentDay, currentMealCode, current))
		current = nil
	}

	for _, item := range sourceItems {
		itemDay := item.Day
		itemMealCode := item.MealCode
		if len(current) > 0 && (itemDay != currentDay || itemMealCode != currentMealCode || len(current) >= maxItems) {
			flush()
		}
		if len(current) == 0 {
			currentDay = itemDay
			currentMealCode = itemMealCode
		}
		current = append(current, item)
	}
	flush()
	return chunks
}

func makeAssistChunk(index int, day int, mealCode string, sourceItems []SourceItem) AssistChunk {
	ids := make([]int, 0, len(sourceItems))
	copiedItems := append([]SourceItem(nil), sourceItems...)
	for _, item := range copiedItems {
		ids = append(ids, item.ID)
	}
	return AssistChunk{
		ID:            fmt.Sprintf("chunk_%d", index),
		Day:           day,
		MealCode:      mealCode,
		SourceItemIDs: ids,
		SourceItems:   copiedItems,
	}
}
