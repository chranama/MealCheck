package hosted

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/chranama/MealCheck/internal/checker"
)

type decodePlanResult struct {
	Plan          checker.Plan
	Canonicalized bool
}

func mealPlanContractPromptBlock() string {
	return strings.Join([]string{
		"Use exactly these MealCheck JSON field names.",
		"Top level: schema_version, plan_id, description, days, shopping_list, prep_notes.",
		"Day: day, meals.",
		"Meal: name, items.",
		"Food item: food, quantity, unit, quantity_text, preparation, brand, resolution_status, unresolved_reason.",
		"Do not use aliases meal, meal_type, type, food_items, foods, ingredients, food_item, item, or item-level name.",
		"Do not include additional fields.",
	}, " ")
}

func mealPlanAliasRules() []string {
	return []string{
		"Meal object aliases meal, meal_type, and type map to name.",
		"Meal item-array aliases food_items, foods, and ingredients map to items.",
		"Food item aliases food_item, item, and item-level name map to food.",
		"After mapping aliases, remove the alias fields and keep only MealCheck fields.",
	}
}

func mealPlanExampleShape() map[string]any {
	return map[string]any{
		"schema_version": "0.1",
		"plan_id":        "provider-generated-plan",
		"description":    "brief description",
		"days": []map[string]any{
			{
				"day": 1,
				"meals": []map[string]any{
					{
						"name": "breakfast",
						"items": []map[string]any{
							{
								"food":     "plain oatmeal",
								"quantity": 1,
								"unit":     "cup",
							},
							{
								"food":              "mixed fruit",
								"quantity_text":     "one small bowl",
								"resolution_status": "unresolved",
								"unresolved_reason": "vague_quantity",
							},
						},
					},
				},
			},
		},
		"shopping_list": []map[string]any{
			{
				"food":     "plain oatmeal",
				"quantity": 1,
				"unit":     "cup",
			},
		},
		"prep_notes": []string{"Keep cold foods refrigerated."},
	}
}

func mealPlanResponseSchema() map[string]any {
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
					"enum": []any{"g", "oz", "cup", "tbsp", "tsp", "serving", nil},
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
					"enum": []any{"unknown_food", "vague_quantity", "unsupported_unit", "missing_conversion", nil},
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

func decodePlanText(text string) (checker.Plan, error) {
	result, err := decodePlanTextDetailed(text)
	if err != nil {
		return checker.Plan{}, err
	}
	return result.Plan, nil
}

func decodePlanTextDetailed(text string) (decodePlanResult, error) {
	var plan checker.Plan
	jsonText, err := extractJSONObject(text)
	if err != nil {
		return decodePlanResult{}, err
	}
	jsonText, canonicalized, err := canonicalizePlanJSON(jsonText)
	if err != nil {
		return decodePlanResult{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(jsonText))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return decodePlanResult{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return decodePlanResult{}, fmt.Errorf("meal plan JSON contains multiple values")
	}
	return decodePlanResult{Plan: plan, Canonicalized: canonicalized}, nil
}

func canonicalizePlanJSON(jsonText string) (string, bool, error) {
	decoder := json.NewDecoder(strings.NewReader(jsonText))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", false, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", false, fmt.Errorf("meal plan JSON contains multiple values")
	}
	root, ok := value.(map[string]any)
	if !ok {
		return "", false, fmt.Errorf("meal plan JSON root must be an object")
	}
	canonicalized := canonicalizeMealPlanRoot(root)
	if !canonicalized {
		return jsonText, false, nil
	}
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(root); err != nil {
		return "", false, err
	}
	return strings.TrimSpace(b.String()), true, nil
}

func canonicalizeMealPlanRoot(root map[string]any) bool {
	changed := false
	changed = deleteNullFields(root, "description") || changed
	if days, ok := root["days"].([]any); ok {
		changed = canonicalizeDays(days) || changed
	}
	if shoppingList, ok := root["shopping_list"].([]any); ok {
		changed = canonicalizeFoodItems(shoppingList) || changed
	}
	return changed
}

func canonicalizeDays(days []any) bool {
	changed := false
	for _, day := range days {
		dayObj, ok := day.(map[string]any)
		if !ok {
			continue
		}
		meals, ok := dayObj["meals"].([]any)
		if !ok {
			continue
		}
		changed = canonicalizeMeals(meals) || changed
	}
	return changed
}

func canonicalizeMeals(meals []any) bool {
	changed := false
	for _, meal := range meals {
		mealObj, ok := meal.(map[string]any)
		if !ok {
			continue
		}
		changed = mapAliasField(mealObj, "name", "meal", "meal_type", "type") || changed
		changed = mapAliasField(mealObj, "items", "food_items", "foods", "ingredients") || changed
		if items, ok := mealObj["items"].([]any); ok {
			changed = canonicalizeFoodItems(items) || changed
		}
	}
	return changed
}

func canonicalizeFoodItems(items []any) bool {
	changed := false
	for _, item := range items {
		itemObj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		changed = mapAliasField(itemObj, "food", "food_item", "item", "name") || changed
		changed = deleteNullFields(itemObj, "quantity_text", "unit", "preparation", "brand", "resolution_status", "unresolved_reason") || changed
	}
	return changed
}

func mapAliasField(obj map[string]any, target string, aliases ...string) bool {
	changed := false
	hasTarget := hasUsableFieldValue(obj[target])
	for _, alias := range aliases {
		value, ok := obj[alias]
		if !ok {
			continue
		}
		if !hasTarget && value != nil {
			obj[target] = value
			hasTarget = true
		}
		delete(obj, alias)
		changed = true
	}
	return changed
}

func hasUsableFieldValue(value any) bool {
	if value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	return true
}

func deleteNullFields(obj map[string]any, fields ...string) bool {
	changed := false
	for _, field := range fields {
		if value, ok := obj[field]; ok && value == nil {
			delete(obj, field)
			changed = true
		}
	}
	return changed
}
