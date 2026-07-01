package mealplan

import (
	"fmt"
	"strings"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

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

func MealPlanContractPromptBlock() string {
	return mealPlanContractPromptBlock()
}

func mealPlanAliasRules() []string {
	return []string{
		"Meal object aliases meal, meal_type, and type map to name.",
		"Meal item-array aliases food_items, foods, and ingredients map to items.",
		"Food item aliases food_item, item, and item-level name map to food.",
		"After mapping aliases, remove the alias fields and keep only MealCheck fields.",
	}
}

func MealPlanAliasRules() []string {
	return mealPlanAliasRules()
}

func mealPlanShapeInstructions(constraints checker.VerificationConstraints) map[string]any {
	daysInstruction := "array with one or more day object(s); preserve or infer day numbers from the prompt or source text"
	if constraints.Days > 0 {
		daysInstruction = fmt.Sprintf("array with exactly %d day object(s); day numbers must run from 1 through %d", constraints.Days, constraints.Days)
	}
	mealsInstruction := "array with one or more meal object(s) per day; preserve or infer meal labels from the prompt or source text"
	if constraints.MealsPerDay > 0 {
		mealsInstruction = fmt.Sprintf("array with exactly %d meal object(s) per day", constraints.MealsPerDay)
	}
	return map[string]any{
		"schema_version": "0.1",
		"plan_id":        "provider-generated-plan",
		"description":    "brief description",
		"days":           daysInstruction,
		"day": map[string]any{
			"day":   "integer day number",
			"meals": mealsInstruction,
		},
		"meal": map[string]any{
			"name":  "meal name, such as breakfast, lunch, dinner, or snack",
			"items": "array of food item objects",
		},
		"food_item_numeric_quantity": map[string]any{
			"food":     "food name",
			"quantity": "positive number",
			"unit":     "g, oz, cup, tbsp, tsp, slice, or serving",
		},
		"food_item_unresolved_quantity": map[string]any{
			"food":              "food name",
			"quantity_text":     "original vague quantity text",
			"resolution_status": "unresolved",
			"unresolved_reason": "vague_quantity, unknown_food, unsupported_unit, missing_conversion, ambiguous_food, composed_food_needs_decomposition, restaurant_or_branded_food, branded_food_unavailable, preparation_unclear, non_food_text, or model_normalization_failed",
		},
		"suggested_meal_names": suggestedMealNames(constraints.MealsPerDay),
		"shopping_list":        "array of food item objects using the same food item fields",
		"prep_notes":           []string{"Keep cold foods refrigerated."},
	}
}

func MealPlanShapeInstructions(constraints checker.VerificationConstraints) map[string]any {
	return mealPlanShapeInstructions(constraints)
}

func suggestedMealNames(mealsPerDay int) []string {
	names := []string{"breakfast", "lunch", "dinner", "snack_1", "snack_2", "snack_3"}
	if mealsPerDay < 1 {
		return nil
	}
	if mealsPerDay > len(names) {
		mealsPerDay = len(names)
	}
	return names[:mealsPerDay]
}
