package localmodel

import (
	"fmt"
	"strconv"
	"strings"
)

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
	b.WriteString("Authoritative source items (return exactly one row for each line):\n")
	for _, item := range chunk.Items {
		fmt.Fprintf(&b, "%d | status=%s | source_text=%s\n", item.ID, item.ParseStatus, item.Text)
	}
	b.WriteString("\n\nContext-only meal text (use only to clarify listed source items; do not extract rows from this section):\n")
	b.WriteString(strings.TrimSpace(chunk.MealText))
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
