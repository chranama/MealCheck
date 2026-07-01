package recommend

import (
	"fmt"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

const safetyPrepNote = "Refrigerate leftovers within 2 hours."

func addPrepSafetyNote(plan *checker.Plan, source checker.Evaluation) (Change, bool) {
	if !hasFailedCheck(source.Checks, "prep_safety_mentions_present") || containsSafetyNote(plan.PrepNotes) {
		return Change{}, false
	}
	plan.PrepNotes = append(plan.PrepNotes, safetyPrepNote)
	return Change{
		Operation:       "add_prep_note",
		PrepNote:        safetyPrepNote,
		Reason:          "Add explicit leftover refrigeration guidance.",
		AddressesChecks: []string{"prep_safety_mentions_present"},
	}, true
}

func replaceBlockedFoods(c checker.Case, plan *checker.Plan, catalog checker.NutrientCatalog, source checker.Evaluation) []Change {
	var changes []Change
	checks := map[string]bool{
		"allergens_absent":      hasFailedCheck(source.Checks, "allergens_absent"),
		"excluded_foods_absent": hasFailedCheck(source.Checks, "excluded_foods_absent"),
	}
	if !checks["allergens_absent"] && !checks["excluded_foods_absent"] {
		return changes
	}

	for _, check := range source.Checks {
		if !checks[check.CheckID] {
			continue
		}
		for _, evidence := range check.Evidence {
			day, dayOK := intEvidence(evidence, "day")
			meal, mealOK := stringEvidence(evidence, "meal")
			food, foodOK := stringEvidence(evidence, "food")
			if !dayOK || !mealOK || !foodOK {
				continue
			}
			item, ok := findPlanItem(plan, day, meal, food)
			if !ok {
				continue
			}
			replacement, replacementOK := chooseReplacement(c, catalog, *item)
			if !replacementOK {
				continue
			}
			from := *item
			*item = replacement
			changes = append(changes, Change{
				Operation:       "replace_item",
				Day:             day,
				Meal:            meal,
				From:            &from,
				To:              &replacement,
				Reason:          fmt.Sprintf("Replace %s with a catalog item that avoids configured allergies and exclusions.", from.Food),
				AddressesChecks: []string{check.CheckID},
			})
		}
	}
	return changes
}

func addMissingVegetables(c checker.Case, plan *checker.Plan, catalog checker.NutrientCatalog, source checker.Evaluation) []Change {
	if !hasFailedCheck(source.Checks, "food_group_coverage") {
		return nil
	}
	vegetable, ok := catalogFoodByName(catalog, "broccoli")
	if !ok || !safeCatalogFood(c, vegetable) || !supportsUnit(vegetable, "cup") {
		return nil
	}

	var changes []Change
	for _, check := range source.Checks {
		if check.CheckID != "food_group_coverage" {
			continue
		}
		for _, evidence := range check.Evidence {
			day, ok := intEvidence(evidence, "day")
			if !ok || planDayHasFoodGroup(catalog, plan, day, "vegetables") {
				continue
			}
			meal := preferredMeal(plan, day)
			if meal == nil {
				continue
			}
			quantity := 1.0
			item := checker.FoodItem{Food: vegetable.Name, Quantity: &quantity, Unit: "cup"}
			meal.Items = append(meal.Items, item)
			changes = append(changes, Change{
				Operation:       "add_item",
				Day:             day,
				Meal:            meal.Name,
				To:              &item,
				Reason:          "Add a resolved vegetable item for daily food-group coverage.",
				AddressesChecks: []string{"food_group_coverage"},
			})
		}
	}
	return changes
}

func chooseReplacement(c checker.Case, catalog checker.NutrientCatalog, item checker.FoodItem) (checker.FoodItem, bool) {
	sourceFood, sourceOK := catalogFoodByName(catalog, item.Food)
	candidates := sortedCatalogFoods(catalog)
	var fallback *checker.CatalogFood
	for _, candidate := range candidates {
		if normalizeName(candidate.Name) == normalizeName(item.Food) || !safeCatalogFood(c, candidate) {
			continue
		}
		if fallback == nil && supportsAnyReplacementUnit(candidate, item) {
			c := candidate
			fallback = &c
		}
		if sourceOK && !sharesFoodGroup(sourceFood, candidate) {
			continue
		}
		if replacement, ok := replacementItem(candidate, item); ok {
			return replacement, true
		}
	}
	if fallback != nil {
		return replacementItem(*fallback, item)
	}
	return checker.FoodItem{}, false
}

func replacementItem(candidate checker.CatalogFood, original checker.FoodItem) (checker.FoodItem, bool) {
	quantity := 1.0
	unit := "serving"
	if original.Quantity != nil && supportsUnit(candidate, original.Unit) {
		quantity = *original.Quantity
		unit = original.Unit
	} else if !supportsUnit(candidate, unit) {
		return checker.FoodItem{}, false
	}
	return checker.FoodItem{
		Food:        candidate.Name,
		Quantity:    &quantity,
		Unit:        unit,
		Preparation: original.Preparation,
	}, true
}

func supportsAnyReplacementUnit(food checker.CatalogFood, original checker.FoodItem) bool {
	return supportsUnit(food, original.Unit) || supportsUnit(food, "serving")
}
