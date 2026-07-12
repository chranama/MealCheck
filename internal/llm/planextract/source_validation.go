package planextract

import (
	"fmt"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

func validateLocalModelExtractionCompleteness(plan checker.Plan, sourceText string) error {
	expected := localLlamaExpectedExtractionItemCount(sourceText)
	if expected == 0 {
		return nil
	}
	got := countMealPlanItems(plan)
	if got != expected {
		return fmt.Errorf("local model extracted %d food item(s); expected %d from numbered source items", got, expected)
	}
	return nil
}

func countMealPlanItems(plan checker.Plan) int {
	count := 0
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			count += len(meal.Items)
		}
	}
	return count
}
