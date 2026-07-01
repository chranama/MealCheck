package normalize

import (
	"fmt"
	"strings"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

func extractJSONObject(text string) (string, error) {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 3 {
			trimmed = strings.Join(lines[1:len(lines)-1], "\n")
		}
		trimmed = strings.TrimSpace(trimmed)
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start < 0 || end < start {
		return "", fmt.Errorf("no JSON object found in provider output")
	}
	return trimmed[start : end+1], nil
}
func validateSettings(settings checker.Settings) error {
	return checker.ValidateSettings(settings)
}
func validatePlan(plan checker.Plan) error {
	if plan.SchemaVersion != "0.1" {
		return fmt.Errorf("meal plan schema_version must be 0.1")
	}
	if strings.TrimSpace(plan.PlanID) == "" {
		return fmt.Errorf("meal plan plan_id is required")
	}
	if len(plan.Days) == 0 {
		return fmt.Errorf("meal plan days are required")
	}
	for _, day := range plan.Days {
		if day.Day < 1 {
			return fmt.Errorf("meal plan day must be positive")
		}
		if len(day.Meals) == 0 {
			return fmt.Errorf("meal plan day %d has no meals", day.Day)
		}
		for _, meal := range day.Meals {
			if strings.TrimSpace(meal.Name) == "" {
				return fmt.Errorf("meal plan day %d has a meal without a name", day.Day)
			}
			if len(meal.Items) == 0 {
				return fmt.Errorf("meal plan day %d meal %s has no items", day.Day, meal.Name)
			}
			for _, item := range meal.Items {
				if strings.TrimSpace(item.Food) == "" {
					return fmt.Errorf("meal plan day %d meal %s has an item without food", day.Day, meal.Name)
				}
				if item.Quantity != nil {
					if *item.Quantity <= 0 {
						return fmt.Errorf("meal plan item %s quantity must be positive", item.Food)
					}
					if !allowedUnit(item.Unit) {
						return fmt.Errorf("meal plan item %s has unsupported unit %q", item.Food, item.Unit)
					}
					continue
				}
				if item.QuantityText == "" || item.ResolutionStatus != "unresolved" || item.UnresolvedReason == "" {
					return fmt.Errorf("meal plan item %s must include quantity/unit or unresolved quantity fields", item.Food)
				}
			}
		}
	}
	return nil
}
func validateGeneratedPlanAgainstConstraints(plan checker.Plan, constraints checker.VerificationConstraints) error {
	if constraints.Days > 0 && len(plan.Days) != constraints.Days {
		return fmt.Errorf("meal plan must include exactly %d day(s); got %d", constraints.Days, len(plan.Days))
	}
	seenDays := make(map[int]bool, len(plan.Days))
	for _, day := range plan.Days {
		if day.Day < 1 {
			return fmt.Errorf("meal plan day number %d is outside expected range 1..N", day.Day)
		}
		if constraints.Days > 0 && day.Day > constraints.Days {
			return fmt.Errorf("meal plan day number %d is outside expected range 1..%d", day.Day, constraints.Days)
		}
		if seenDays[day.Day] {
			return fmt.Errorf("meal plan includes duplicate day %d", day.Day)
		}
		seenDays[day.Day] = true
		if constraints.MealsPerDay > 0 && len(day.Meals) != constraints.MealsPerDay {
			return fmt.Errorf("meal plan day %d must include exactly %d meal(s); got %d", day.Day, constraints.MealsPerDay, len(day.Meals))
		}
	}
	if constraints.Days > 0 {
		for day := 1; day <= constraints.Days; day++ {
			if !seenDays[day] {
				return fmt.Errorf("meal plan is missing day %d", day)
			}
		}
	}
	return nil
}
func allowedUnit(unit string) bool {
	switch unit {
	case "g", "oz", "cup", "tbsp", "tsp", "slice", "serving":
		return true
	default:
		return false
	}
}
