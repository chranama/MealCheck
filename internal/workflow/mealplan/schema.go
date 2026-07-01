package mealplan

func strictMealPlanResponseSchema() map[string]any {
	foodItem := func() map[string]any {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required": []string{
				"food",
				"quantity",
				"quantity_text",
				"unit",
				"preparation",
				"brand",
				"resolution_status",
				"unresolved_reason",
			},
			"properties": map[string]any{
				"food": map[string]any{
					"type":        "string",
					"description": "Food name only; do not use food_item, item, or name.",
				},
				"quantity": map[string]any{
					"type":        []string{"number", "null"},
					"description": "Positive numeric quantity when known; otherwise null.",
				},
				"quantity_text": map[string]any{
					"type":        []string{"string", "null"},
					"description": "Original vague quantity text when numeric quantity is unavailable; otherwise null.",
				},
				"unit": map[string]any{
					"type": []string{"string", "null"},
					"enum": []any{"g", "oz", "cup", "tbsp", "tsp", "slice", "serving", nil},
				},
				"preparation": map[string]any{
					"type": []string{"string", "null"},
				},
				"brand": map[string]any{
					"type": []string{"string", "null"},
				},
				"resolution_status": map[string]any{
					"type": []string{"string", "null"},
					"enum": []any{"pending", "unresolved", nil},
				},
				"unresolved_reason": map[string]any{
					"type": []string{"string", "null"},
					"enum": []any{
						"unknown_food",
						"vague_quantity",
						"unsupported_unit",
						"missing_conversion",
						"ambiguous_food",
						"composed_food_needs_decomposition",
						"restaurant_or_branded_food",
						"branded_food_unavailable",
						"preparation_unclear",
						"non_food_text",
						"model_normalization_failed",
						nil,
					},
				},
			},
		}
	}
	meal := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"name", "items"},
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Meal name; do not use meal, meal_type, or type.",
			},
			"items": map[string]any{
				"type":        "array",
				"minItems":    1,
				"description": "Food items; do not use food_items, foods, or ingredients.",
				"items":       foodItem(),
			},
		},
	}
	day := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"day", "meals"},
		"properties": map[string]any{
			"day": map[string]any{
				"type":    "integer",
				"minimum": 1,
			},
			"meals": map[string]any{
				"type":     "array",
				"minItems": 1,
				"items":    meal,
			},
		},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required": []string{
			"schema_version",
			"plan_id",
			"description",
			"days",
			"shopping_list",
			"prep_notes",
		},
		"properties": map[string]any{
			"schema_version": map[string]any{
				"type": "string",
				"enum": []string{"0.1"},
			},
			"plan_id": map[string]any{
				"type": "string",
			},
			"description": map[string]any{
				"type": "string",
			},
			"days": map[string]any{
				"type":     "array",
				"minItems": 1,
				"items":    day,
			},
			"shopping_list": map[string]any{
				"type":  "array",
				"items": foodItem(),
			},
			"prep_notes": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
		},
	}
}

func StrictMealPlanResponseSchema() map[string]any {
	return strictMealPlanResponseSchema()
}

func portableMealPlanResponseSchema() map[string]any {
	foodItem := func() map[string]any {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"food"},
			"properties": map[string]any{
				"food": map[string]any{
					"type": "string",
				},
				"quantity": map[string]any{
					"type": "number",
				},
				"quantity_text": map[string]any{
					"type": "string",
				},
				"unit": map[string]any{
					"type": "string",
				},
				"preparation": map[string]any{
					"type": "string",
				},
				"brand": map[string]any{
					"type": "string",
				},
				"resolution_status": map[string]any{
					"type": "string",
				},
				"unresolved_reason": map[string]any{
					"type": "string",
				},
			},
		}
	}
	meal := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"name", "items"},
		"properties": map[string]any{
			"name": map[string]any{
				"type": "string",
			},
			"items": map[string]any{
				"type":  "array",
				"items": foodItem(),
			},
		},
	}
	day := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"day", "meals"},
		"properties": map[string]any{
			"day": map[string]any{
				"type": "integer",
			},
			"meals": map[string]any{
				"type":  "array",
				"items": meal,
			},
		},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"schema_version", "plan_id", "days"},
		"properties": map[string]any{
			"schema_version": map[string]any{
				"type": "string",
			},
			"plan_id": map[string]any{
				"type": "string",
			},
			"description": map[string]any{
				"type": "string",
			},
			"days": map[string]any{
				"type":  "array",
				"items": day,
			},
			"shopping_list": map[string]any{
				"type":  "array",
				"items": foodItem(),
			},
			"prep_notes": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
		},
	}
}

func PortableMealPlanResponseSchema() map[string]any {
	return portableMealPlanResponseSchema()
}
