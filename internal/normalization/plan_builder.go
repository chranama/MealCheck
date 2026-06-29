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

func BuildDeterministicPlan(text string, planID string) (checker.Plan, []ParsedSourceItem, error) {
	sourceItems := ResolvedSourceItems(text)
	if len(sourceItems) == 0 {
		return checker.Plan{}, nil, fmt.Errorf("no resolved source items")
	}
	if strings.TrimSpace(planID) == "" {
		planID = "deterministic-normalized"
	}

	dayMeals := map[int]map[string][]checker.FoodItem{}
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
		quantity := measurement.Quantity
		if dayMeals[sourceItem.Day] == nil {
			dayMeals[sourceItem.Day] = map[string][]checker.FoodItem{}
		}
		dayMeals[sourceItem.Day][sourceItem.MealCode] = append(dayMeals[sourceItem.Day][sourceItem.MealCode], checker.FoodItem{
			Food:     measurement.Food,
			Quantity: &quantity,
			Unit:     measurement.Unit,
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
	}, parsedItems, nil
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
