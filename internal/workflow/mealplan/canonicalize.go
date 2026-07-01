package mealplan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

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

func CanonicalizePlanJSON(jsonText string) (string, bool, error) {
	return canonicalizePlanJSON(jsonText)
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
