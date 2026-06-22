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

type localLlamaTuplePlan struct {
	Breakfast []localLlamaTupleItem `json:"b"`
	Lunch     []localLlamaTupleItem `json:"l"`
	Dinner    []localLlamaTupleItem `json:"d"`
}

type localLlamaCompactItem struct {
	Food     string  `json:"f"`
	Quantity float64 `json:"q"`
	Unit     string  `json:"u"`
}

type localLlamaTupleItem struct {
	Food     string
	Quantity float64
	Unit     string
}

func (item *localLlamaTupleItem) UnmarshalJSON(data []byte) error {
	var values []json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return fmt.Errorf("local llama tuple item must be [food, quantity, unit]: %w", err)
	}
	if len(values) != 3 {
		return fmt.Errorf("local llama tuple item must have exactly 3 values")
	}
	if err := json.Unmarshal(values[0], &item.Food); err != nil {
		return fmt.Errorf("local llama tuple item food must be a string: %w", err)
	}
	if err := json.Unmarshal(values[1], &item.Quantity); err != nil {
		return fmt.Errorf("local llama tuple item quantity must be a number: %w", err)
	}
	if err := json.Unmarshal(values[2], &item.Unit); err != nil {
		return fmt.Errorf("local llama tuple item unit must be a string: %w", err)
	}
	return nil
}

// DecodeLocalLlamaCompactPlan expands the local llama compact extraction
// contract into canonical MealCheck plan JSON.
func DecodeLocalLlamaCompactPlan(text string, planID string) (checker.Plan, error) {
	jsonText, err := extractJSONObject(text)
	if err != nil {
		return checker.Plan{}, err
	}
	if localLlamaJSONUsesTupleKeys(jsonText) {
		return decodeLocalLlamaTuplePlanJSON(jsonText, planID)
	}
	return decodeLocalLlamaLegacyCompactPlanJSON(jsonText, planID)
}

func localLlamaJSONUsesTupleKeys(jsonText string) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonText), &fields); err != nil {
		return false
	}
	_, hasBreakfast := fields["b"]
	_, hasLunch := fields["l"]
	_, hasDinner := fields["d"]
	return hasBreakfast || hasLunch || hasDinner
}

func decodeLocalLlamaLegacyCompactPlanJSON(jsonText string, planID string) (checker.Plan, error) {
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

func decodeLocalLlamaTuplePlanJSON(jsonText string, planID string) (checker.Plan, error) {
	decoder := json.NewDecoder(strings.NewReader(jsonText))
	decoder.DisallowUnknownFields()
	var tuple localLlamaTuplePlan
	if err := decoder.Decode(&tuple); err != nil {
		return checker.Plan{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return checker.Plan{}, fmt.Errorf("local llama tuple JSON contains multiple values")
	}

	return expandLocalLlamaCompactPlan(localLlamaCompactPlan{
		Breakfast: compactTupleItems(tuple.Breakfast),
		Lunch:     compactTupleItems(tuple.Lunch),
		Dinner:    compactTupleItems(tuple.Dinner),
	}, planID)
}

func compactTupleItems(tupleItems []localLlamaTupleItem) []localLlamaCompactItem {
	compactItems := make([]localLlamaCompactItem, 0, len(tupleItems))
	for _, tuple := range tupleItems {
		compactItems = append(compactItems, localLlamaCompactItem{
			Food:     tuple.Food,
			Quantity: tuple.Quantity,
			Unit:     tuple.Unit,
		})
	}
	return compactItems
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
		"type": "array",
		"items": []map[string]any{
			{
				"type": "string",
			},
			{
				"type":             "number",
				"exclusiveMinimum": 0,
			},
			{
				"type": "string",
				"enum": []string{"g", "oz", "cup", "tbsp", "tsp", "serving"},
			},
		},
		"additionalItems": false,
	}
	mealItems := map[string]any{
		"type":     "array",
		"minItems": 1,
		"items":    item,
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"b", "l", "d"},
		"properties": map[string]any{
			"b": mealItems,
			"l": mealItems,
			"d": mealItems,
		},
	}
}
