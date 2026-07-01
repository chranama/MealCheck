package llm

import (
	"github.com/chranama/MealCheck/internal/workflow/mealplan"
)

func strictMealPlanResponseSchema() map[string]any {
	return mealplan.StrictMealPlanResponseSchema()
}

func portableMealPlanResponseSchema() map[string]any {
	return mealplan.PortableMealPlanResponseSchema()
}

func LocalLlamaMealChunkResponseSchema() map[string]any {
	sourceIDField := map[string]any{"type": "integer", "minimum": 1}
	foodField := map[string]any{"type": "string"}
	quantityField := map[string]any{"type": "number", "exclusiveMinimum": 0}
	unitField := map[string]any{"type": "string", "enum": []string{"g", "oz", "cup", "tbsp", "tsp", "slice", "serving"}}
	resolvedItem := map[string]any{
		"type":            "array",
		"minItems":        4,
		"maxItems":        4,
		"items":           []map[string]any{sourceIDField, foodField, quantityField, unitField},
		"additionalItems": false,
	}
	unresolvedItem := map[string]any{
		"type":     "array",
		"minItems": 6,
		"maxItems": 6,
		"items": []map[string]any{
			sourceIDField,
			foodField,
			{"type": []string{"number", "null"}},
			{"type": "string"},
			{"type": "string"},
			{"type": "string", "enum": []string{"missing_quantity", "vague_quantity", "unsupported_unit"}},
		},
		"additionalItems": false,
	}
	rows := map[string]any{
		"type":     "array",
		"minItems": 1,
		"items": map[string]any{
			"oneOf": []map[string]any{resolvedItem, unresolvedItem},
		},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"i"},
		"properties": map[string]any{
			"i": rows,
		},
	}
}
