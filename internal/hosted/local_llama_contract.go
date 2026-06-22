package hosted

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/chranama/MealCheck/internal/checker"
)

const defaultLocalLlamaPlanID = "local-llama-normalized"

type localLlamaCompactPlan struct {
	Breakfast []localLlamaCompactItem `json:"breakfast"`
	Lunch     []localLlamaCompactItem `json:"lunch"`
	Dinner    []localLlamaCompactItem `json:"dinner"`
}

type localLlamaCompactItem struct {
	Food     string  `json:"f"`
	Quantity float64 `json:"q"`
	Unit     string  `json:"u"`
}

// DecodeLocalLlamaCompactPlan expands the local llama compact extraction
// contract into canonical MealCheck plan JSON.
func DecodeLocalLlamaCompactPlan(text string, planID string) (checker.Plan, error) {
	jsonText, err := extractJSONObject(text)
	if err != nil {
		return checker.Plan{}, err
	}

	decoder := json.NewDecoder(strings.NewReader(jsonText))
	decoder.DisallowUnknownFields()
	var compact localLlamaCompactPlan
	if err := decoder.Decode(&compact); err != nil {
		return checker.Plan{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return checker.Plan{}, fmt.Errorf("local llama compact JSON contains multiple values")
	}

	plan, err := expandLocalLlamaCompactPlan(compact, planID)
	if err != nil {
		return checker.Plan{}, err
	}
	return plan, nil
}

func expandLocalLlamaCompactPlan(compact localLlamaCompactPlan, planID string) (checker.Plan, error) {
	if strings.TrimSpace(planID) == "" {
		planID = defaultLocalLlamaPlanID
	}

	meals := []checker.Meal{
		{Name: "breakfast"},
		{Name: "lunch"},
		{Name: "dinner"},
	}
	sources := [][]localLlamaCompactItem{
		compact.Breakfast,
		compact.Lunch,
		compact.Dinner,
	}
	for index, source := range sources {
		items, err := expandLocalLlamaCompactItems(meals[index].Name, source)
		if err != nil {
			return checker.Plan{}, err
		}
		meals[index].Items = items
	}

	return checker.Plan{
		SchemaVersion: "0.1",
		PlanID:        planID,
		Days: []checker.PlanDay{
			{
				Day:   1,
				Meals: meals,
			},
		},
	}, nil
}

func expandLocalLlamaCompactItems(mealName string, compactItems []localLlamaCompactItem) ([]checker.FoodItem, error) {
	if len(compactItems) == 0 {
		return nil, fmt.Errorf("local llama compact meal %s has no items", mealName)
	}

	items := make([]checker.FoodItem, 0, len(compactItems))
	for _, compact := range compactItems {
		food := strings.TrimSpace(compact.Food)
		if food == "" {
			return nil, fmt.Errorf("local llama compact meal %s has an item without food", mealName)
		}
		if compact.Quantity <= 0 {
			return nil, fmt.Errorf("local llama compact item %s quantity must be positive", food)
		}
		unit := strings.TrimSpace(compact.Unit)
		if !allowedUnit(unit) {
			return nil, fmt.Errorf("local llama compact item %s has unsupported unit %q", food, compact.Unit)
		}
		quantity := compact.Quantity
		items = append(items, checker.FoodItem{
			Food:     food,
			Quantity: &quantity,
			Unit:     unit,
		})
	}
	return items, nil
}

func LocalLlamaCompactResponseSchema() map[string]any {
	item := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"f", "q", "u"},
		"properties": map[string]any{
			"f": map[string]any{
				"type": "string",
			},
			"q": map[string]any{
				"type":             "number",
				"exclusiveMinimum": 0,
			},
			"u": map[string]any{
				"type": "string",
				"enum": []string{"g", "oz", "cup", "tbsp", "tsp", "serving"},
			},
		},
	}
	mealItems := map[string]any{
		"type":     "array",
		"minItems": 1,
		"items":    item,
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"breakfast", "lunch", "dinner"},
		"properties": map[string]any{
			"breakfast": mealItems,
			"lunch":     mealItems,
			"dinner":    mealItems,
		},
	}
}
