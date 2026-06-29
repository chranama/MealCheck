package normalization

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chranama/MealCheck/internal/checker"
)

type ParsedSourceItem struct {
	SourceItem  SourceItem              `json:"source_item"`
	Measurement ParsedSourceMeasurement `json:"measurement"`
}

type NormalizedRow struct {
	SourceItemID int     `json:"source_item_id,omitempty"`
	SourceText   string  `json:"source_text,omitempty"`
	Day          int     `json:"day"`
	MealCode     string  `json:"meal_code"`
	Food         string  `json:"food"`
	Quantity     float64 `json:"quantity"`
	Unit         string  `json:"unit"`
	Source       string  `json:"source,omitempty"`
	Confidence   string  `json:"confidence,omitempty"`
}

func BuildDeterministicPlan(text string, planID string) (checker.Plan, []ParsedSourceItem, error) {
	sourceItems := ResolvedSourceItems(text)
	if len(sourceItems) == 0 {
		return checker.Plan{}, nil, fmt.Errorf("no resolved source items")
	}
	if strings.TrimSpace(planID) == "" {
		planID = "deterministic-normalized"
	}

	rows := make([]NormalizedRow, 0, len(sourceItems))
	parsedItems := make([]ParsedSourceItem, 0, len(sourceItems))
	for _, sourceItem := range sourceItems {
		if sourceItem.Day < 1 || sourceItem.Day > 7 {
			return checker.Plan{}, parsedItems, fmt.Errorf("source item %d day %d is outside supported range 1..7", sourceItem.ID, sourceItem.Day)
		}
		if _, ok := MealName(sourceItem.MealCode); !ok {
			return checker.Plan{}, parsedItems, fmt.Errorf("source item %d has missing or unsupported meal code %q", sourceItem.ID, sourceItem.MealCode)
		}
		measurement := ParseSourceMeasurement(sourceItem.Text)
		parsedItems = append(parsedItems, ParsedSourceItem{
			SourceItem:  sourceItem,
			Measurement: measurement,
		})
		if measurement.Status != "parsed" {
			return checker.Plan{}, parsedItems, fmt.Errorf("source item %d could not be parsed: %s", sourceItem.ID, measurement.Reason)
		}
		rows = append(rows, NormalizedRow{
			SourceItemID: sourceItem.ID,
			SourceText:   sourceItem.Text,
			Day:          sourceItem.Day,
			MealCode:     sourceItem.MealCode,
			Food:         measurement.Food,
			Quantity:     measurement.Quantity,
			Unit:         measurement.Unit,
			Source:       "deterministic",
		})
	}
	plan, err := BuildPlanFromRows(rows, planID)
	return plan, parsedItems, err
}

func DeterministicRows(parsedItems []ParsedSourceItem) []NormalizedRow {
	rows := make([]NormalizedRow, 0, len(parsedItems))
	for _, parsed := range parsedItems {
		if parsed.Measurement.Status != "parsed" {
			continue
		}
		if _, ok := MealName(parsed.SourceItem.MealCode); !ok {
			continue
		}
		if parsed.SourceItem.Day < 1 || parsed.SourceItem.Day > 7 {
			continue
		}
		rows = append(rows, NormalizedRow{
			SourceItemID: parsed.SourceItem.ID,
			SourceText:   parsed.SourceItem.Text,
			Day:          parsed.SourceItem.Day,
			MealCode:     parsed.SourceItem.MealCode,
			Food:         parsed.Measurement.Food,
			Quantity:     parsed.Measurement.Quantity,
			Unit:         parsed.Measurement.Unit,
			Source:       "deterministic",
		})
	}
	return rows
}

func BuildPlanFromRows(rows []NormalizedRow, planID string) (checker.Plan, error) {
	if len(rows) == 0 {
		return checker.Plan{}, fmt.Errorf("no normalized rows")
	}
	if strings.TrimSpace(planID) == "" {
		planID = "deterministic-normalized"
	}
	rows = append([]NormalizedRow(nil), rows...)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].SourceItemID == rows[j].SourceItemID {
			return i < j
		}
		if rows[i].SourceItemID == 0 {
			return false
		}
		if rows[j].SourceItemID == 0 {
			return true
		}
		return rows[i].SourceItemID < rows[j].SourceItemID
	})

	dayMeals := map[int]map[string][]checker.FoodItem{}
	for _, row := range rows {
		if row.Day < 1 || row.Day > 7 {
			return checker.Plan{}, fmt.Errorf("source item %d day %d is outside supported range 1..7", row.SourceItemID, row.Day)
		}
		if _, ok := MealName(row.MealCode); !ok {
			return checker.Plan{}, fmt.Errorf("source item %d has missing or unsupported meal code %q", row.SourceItemID, row.MealCode)
		}
		unit := NormalizeSourceUnit(row.Unit)
		if !AllowedUnit(unit) {
			return checker.Plan{}, fmt.Errorf("source item %d has unsupported unit %q", row.SourceItemID, row.Unit)
		}
		food := strings.TrimSpace(row.Food)
		if food == "" {
			return checker.Plan{}, fmt.Errorf("source item %d has empty food", row.SourceItemID)
		}
		if row.Quantity <= 0 {
			return checker.Plan{}, fmt.Errorf("source item %d has non-positive quantity", row.SourceItemID)
		}
		quantity := row.Quantity
		if dayMeals[row.Day] == nil {
			dayMeals[row.Day] = map[string][]checker.FoodItem{}
		}
		dayMeals[row.Day][row.MealCode] = append(dayMeals[row.Day][row.MealCode], checker.FoodItem{
			Food:     food,
			Quantity: &quantity,
			Unit:     unit,
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
			return MealRank(mealCodes[i]) < MealRank(mealCodes[j])
		})

		meals := make([]checker.Meal, 0, len(mealCodes))
		for _, mealCode := range mealCodes {
			mealName, _ := MealName(mealCode)
			meals = append(meals, checker.Meal{
				Name:  mealName,
				Items: dayMeals[dayNumber][mealCode],
			})
		}
		days = append(days, checker.PlanDay{Day: dayNumber, Meals: meals})
	}

	return checker.Plan{
		SchemaVersion: "0.1",
		PlanID:        planID,
		Days:          days,
	}, nil
}

func MealName(code string) (string, bool) {
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

func MealRank(code string) int {
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
