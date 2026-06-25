package hosted

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
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

type localLlamaRowPlan struct {
	Items []localLlamaRowItem `json:"i"`
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

type localLlamaRowItem struct {
	SourceItemID int
	Day          int
	MealCode     string
	Food         string
	Quantity     float64
	Unit         string
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

func (item *localLlamaRowItem) UnmarshalJSON(data []byte) error {
	var values []json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return fmt.Errorf("local llama row item must be [source_item_id, day, meal_code, food, quantity, unit]: %w", err)
	}
	if len(values) != 5 && len(values) != 6 {
		return fmt.Errorf("local llama row item must have exactly 5 legacy values or 6 source-id values")
	}
	offset := 0
	if len(values) == 6 {
		if err := json.Unmarshal(values[0], &item.SourceItemID); err != nil {
			return fmt.Errorf("local llama row item source_item_id must be an integer: %w", err)
		}
		offset = 1
	}
	if err := json.Unmarshal(values[offset], &item.Day); err != nil {
		return fmt.Errorf("local llama row item day must be an integer: %w", err)
	}
	if err := json.Unmarshal(values[offset+1], &item.MealCode); err != nil {
		return fmt.Errorf("local llama row item meal_code must be a string: %w", err)
	}
	if err := json.Unmarshal(values[offset+2], &item.Food); err != nil {
		return fmt.Errorf("local llama row item food must be a string: %w", err)
	}
	if err := json.Unmarshal(values[offset+3], &item.Quantity); err != nil {
		return fmt.Errorf("local llama row item quantity must be a number: %w", err)
	}
	if err := json.Unmarshal(values[offset+4], &item.Unit); err != nil {
		return fmt.Errorf("local llama row item unit must be a string: %w", err)
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
	if localLlamaJSONUsesRowKeys(jsonText) {
		return decodeLocalLlamaRowPlanJSON(jsonText, planID)
	}
	if localLlamaJSONUsesTupleKeys(jsonText) {
		return decodeLocalLlamaTuplePlanJSON(jsonText, planID)
	}
	return decodeLocalLlamaLegacyCompactPlanJSON(jsonText, planID)
}

func localLlamaJSONUsesRowKeys(jsonText string) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonText), &fields); err != nil {
		return false
	}
	_, hasItems := fields["i"]
	return hasItems
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

func decodeLocalLlamaRowPlanJSON(jsonText string, planID string) (checker.Plan, error) {
	decoder := json.NewDecoder(strings.NewReader(jsonText))
	decoder.DisallowUnknownFields()
	var rowPlan localLlamaRowPlan
	if err := decoder.Decode(&rowPlan); err != nil {
		return checker.Plan{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return checker.Plan{}, fmt.Errorf("local llama row JSON contains multiple values")
	}
	return expandLocalLlamaRows(rowPlan.Items, planID)
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

func expandLocalLlamaRows(rows []localLlamaRowItem, planID string) (checker.Plan, error) {
	if len(rows) == 0 {
		return checker.Plan{}, fmt.Errorf("local llama row JSON has no items")
	}
	if strings.TrimSpace(planID) == "" {
		planID = defaultLocalLlamaPlanID
	}
	if err := validateLocalLlamaSourceItemIDs(rows); err != nil {
		return checker.Plan{}, err
	}

	dayMeals := map[int]map[string][]localLlamaCompactItem{}
	for _, row := range rows {
		if row.Day < 1 || row.Day > 7 {
			return checker.Plan{}, fmt.Errorf("local llama row day %d is outside supported range 1..7", row.Day)
		}
		mealCode := strings.TrimSpace(row.MealCode)
		if _, ok := localLlamaMealName(mealCode); !ok {
			return checker.Plan{}, fmt.Errorf("local llama row has unsupported meal code %q", row.MealCode)
		}
		if dayMeals[row.Day] == nil {
			dayMeals[row.Day] = map[string][]localLlamaCompactItem{}
		}
		dayMeals[row.Day][mealCode] = append(dayMeals[row.Day][mealCode], localLlamaCompactItem{
			Food:     row.Food,
			Quantity: row.Quantity,
			Unit:     row.Unit,
		})
	}

	dayNumbers := make([]int, 0, len(dayMeals))
	for day := range dayMeals {
		dayNumbers = append(dayNumbers, day)
	}
	sort.Ints(dayNumbers)

	days := make([]checker.PlanDay, 0, len(dayNumbers))
	for _, dayNumber := range dayNumbers {
		mealCodes := make([]string, 0, len(dayMeals[dayNumber]))
		for mealCode := range dayMeals[dayNumber] {
			mealCodes = append(mealCodes, mealCode)
		}
		sort.Slice(mealCodes, func(i, j int) bool {
			return localLlamaMealRank(mealCodes[i]) < localLlamaMealRank(mealCodes[j])
		})

		meals := make([]checker.Meal, 0, len(mealCodes))
		for _, mealCode := range mealCodes {
			mealName, _ := localLlamaMealName(mealCode)
			items, err := expandLocalLlamaCompactItems(mealName, dayMeals[dayNumber][mealCode])
			if err != nil {
				return checker.Plan{}, err
			}
			meals = append(meals, checker.Meal{Name: mealName, Items: items})
		}
		days = append(days, checker.PlanDay{Day: dayNumber, Meals: meals})
	}

	return checker.Plan{
		SchemaVersion: "0.1",
		PlanID:        planID,
		Days:          days,
	}, nil
}

func validateLocalLlamaSourceItemIDs(rows []localLlamaRowItem) error {
	withSourceID := 0
	seen := make(map[int]bool, len(rows))
	for _, row := range rows {
		if row.SourceItemID == 0 {
			continue
		}
		withSourceID++
		if row.SourceItemID < 1 {
			return fmt.Errorf("local llama row source_item_id %d is outside supported range 1..N", row.SourceItemID)
		}
		if seen[row.SourceItemID] {
			return fmt.Errorf("local llama row source_item_id %d is duplicated", row.SourceItemID)
		}
		seen[row.SourceItemID] = true
	}
	if withSourceID == 0 {
		return nil
	}
	if withSourceID != len(rows) {
		return fmt.Errorf("local llama rows must either all include source_item_id or all use the legacy row shape")
	}
	for id := 1; id <= len(rows); id++ {
		if !seen[id] {
			return fmt.Errorf("local llama row source_item_id %d is missing", id)
		}
	}
	return nil
}

func localLlamaMealName(code string) (string, bool) {
	switch code {
	case "b":
		return "breakfast", true
	case "m":
		return "morning snack", true
	case "l":
		return "lunch", true
	case "a":
		return "afternoon snack", true
	case "d":
		return "dinner", true
	case "s":
		return "snack", true
	case "e":
		return "evening snack", true
	default:
		return "", false
	}
}

func localLlamaMealRank(code string) int {
	switch code {
	case "b":
		return 0
	case "m":
		return 1
	case "l":
		return 2
	case "a":
		return 3
	case "d":
		return 4
	case "s":
		return 5
	case "e":
		return 6
	default:
		return 99
	}
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
		quantity := compact.Quantity
		unit := localLlamaNormalizeSourceUnit(compact.Unit)
		food, unit = localLlamaStripDuplicateQuantityPrefix(food, quantity, unit)
		if !allowedUnit(unit) {
			return nil, fmt.Errorf("local llama compact item %s has unsupported unit %q", food, compact.Unit)
		}
		items = append(items, checker.FoodItem{
			Food:     food,
			Quantity: &quantity,
			Unit:     unit,
		})
	}
	return items, nil
}

func localLlamaStripDuplicateQuantityPrefix(food string, quantity float64, unit string) (string, string) {
	fields := strings.Fields(food)
	if len(fields) < 3 {
		return food, unit
	}
	prefixQuantity, consumed, ok := localLlamaParseQuantityPrefixFields(fields)
	if !ok || math.Abs(prefixQuantity-quantity) > 0.0001 {
		return food, unit
	}
	if len(fields) < consumed+2 {
		return food, unit
	}
	prefixUnit := localLlamaNormalizeSourceUnit(fields[consumed])
	if !allowedUnit(prefixUnit) {
		return food, unit
	}
	rest := strings.TrimSpace(strings.Join(fields[consumed+1:], " "))
	if rest == "" {
		return food, unit
	}
	return rest, prefixUnit
}

func localLlamaParseQuantityPrefixFields(fields []string) (float64, int, bool) {
	if len(fields) == 0 {
		return 0, 0, false
	}
	first, ok := localLlamaParseQuantityToken(fields[0])
	if !ok {
		return 0, 0, false
	}
	if len(fields) > 1 && strings.Contains(fields[1], "/") {
		fraction, ok := localLlamaParseQuantityToken(fields[1])
		if ok {
			return first + fraction, 2, true
		}
	}
	return first, 1, true
}

func localLlamaParseQuantityToken(token string) (float64, bool) {
	token = strings.Trim(strings.TrimSpace(token), "(),")
	if token == "" {
		return 0, false
	}
	if strings.Contains(token, "/") {
		parts := strings.Split(token, "/")
		if len(parts) != 2 {
			return 0, false
		}
		numerator, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			return 0, false
		}
		denominator, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil || denominator == 0 {
			return 0, false
		}
		return numerator / denominator, true
	}
	value, err := strconv.ParseFloat(token, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func LocalLlamaCompactResponseSchema() map[string]any {
	item := map[string]any{
		"type":     "array",
		"minItems": 6,
		"maxItems": 6,
		"items": []map[string]any{
			{
				"type":    "integer",
				"minimum": 1,
			},
			{
				"type":    "integer",
				"minimum": 1,
				"maximum": 7,
			},
			{
				"type": "string",
				"enum": []string{"b", "m", "l", "a", "d", "s", "e"},
			},
			{
				"type": "string",
			},
			{
				"type":             "number",
				"exclusiveMinimum": 0,
			},
			{
				"type": "string",
				"enum": []string{"g", "oz", "cup", "tbsp", "tsp", "slice", "serving"},
			},
		},
		"additionalItems": false,
	}
	rows := map[string]any{
		"type":     "array",
		"minItems": 1,
		"items":    item,
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
