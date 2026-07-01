package recommend

import (
	"sort"
	"strings"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

func safeCatalogFood(c checker.Case, food checker.CatalogFood) bool {
	allergies := normalizedSet(c.Settings.VerificationConstraints.Allergies)
	for _, allergen := range food.Allergens {
		if allergies[normalizeAllergen(allergen)] {
			return false
		}
	}
	excluded := normalizedSet(c.Settings.VerificationConstraints.ExcludedFoods)
	return !excluded[normalizeName(food.Name)]
}

func catalogFoodByName(catalog checker.NutrientCatalog, food string) (checker.CatalogFood, bool) {
	want := normalizeName(food)
	for _, candidate := range catalog.Foods {
		if normalizeName(candidate.Name) == want {
			return candidate, true
		}
		for _, alias := range candidate.Aliases {
			if normalizeName(alias) == want {
				return candidate, true
			}
		}
	}
	return checker.CatalogFood{}, false
}

func sortedCatalogFoods(catalog checker.NutrientCatalog) []checker.CatalogFood {
	foods := append([]checker.CatalogFood(nil), catalog.Foods...)
	sort.Slice(foods, func(i, j int) bool {
		return normalizeName(foods[i].Name) < normalizeName(foods[j].Name)
	})
	return foods
}

func sharesFoodGroup(left, right checker.CatalogFood) bool {
	groups := normalizedSet(left.FoodGroups)
	for _, group := range right.FoodGroups {
		if groups[normalizeName(group)] {
			return true
		}
	}
	return false
}

func planDayHasFoodGroup(catalog checker.NutrientCatalog, plan *checker.Plan, day int, group string) bool {
	for _, planDay := range plan.Days {
		if planDay.Day != day {
			continue
		}
		for _, meal := range planDay.Meals {
			for _, item := range meal.Items {
				food, ok := catalogFoodByName(catalog, item.Food)
				if !ok {
					continue
				}
				for _, foodGroup := range food.FoodGroups {
					if normalizeName(foodGroup) == normalizeName(group) {
						return true
					}
				}
			}
		}
	}
	return false
}

func preferredMeal(plan *checker.Plan, day int) *checker.Meal {
	var first *checker.Meal
	for dayIndex := range plan.Days {
		if plan.Days[dayIndex].Day != day {
			continue
		}
		for mealIndex := range plan.Days[dayIndex].Meals {
			meal := &plan.Days[dayIndex].Meals[mealIndex]
			if first == nil {
				first = meal
			}
			if normalizeName(meal.Name) == "dinner" {
				return meal
			}
		}
	}
	return first
}

func findPlanItem(plan *checker.Plan, day int, mealName, foodName string) (*checker.FoodItem, bool) {
	for dayIndex := range plan.Days {
		if plan.Days[dayIndex].Day != day {
			continue
		}
		for mealIndex := range plan.Days[dayIndex].Meals {
			meal := &plan.Days[dayIndex].Meals[mealIndex]
			if normalizeName(meal.Name) != normalizeName(mealName) {
				continue
			}
			for itemIndex := range meal.Items {
				item := &meal.Items[itemIndex]
				if normalizeName(item.Food) == normalizeName(foodName) {
					return item, true
				}
			}
		}
	}
	return nil, false
}

func clonePlan(plan checker.Plan) *checker.Plan {
	clone := checker.Plan{
		SchemaVersion: plan.SchemaVersion,
		PlanID:        plan.PlanID + "-recommended",
		Description:   plan.Description,
		PrepNotes:     append([]string(nil), plan.PrepNotes...),
	}
	clone.ShoppingList = cloneItems(plan.ShoppingList)
	clone.Days = make([]checker.PlanDay, 0, len(plan.Days))
	for _, day := range plan.Days {
		dayClone := checker.PlanDay{Day: day.Day, Meals: make([]checker.Meal, 0, len(day.Meals))}
		for _, meal := range day.Meals {
			dayClone.Meals = append(dayClone.Meals, checker.Meal{
				Name:  meal.Name,
				Items: cloneItems(meal.Items),
			})
		}
		clone.Days = append(clone.Days, dayClone)
	}
	return &clone
}

func cloneItems(items []checker.FoodItem) []checker.FoodItem {
	cloned := make([]checker.FoodItem, 0, len(items))
	for _, item := range items {
		itemClone := item
		if item.Quantity != nil {
			quantity := *item.Quantity
			itemClone.Quantity = &quantity
		}
		cloned = append(cloned, itemClone)
	}
	return cloned
}

func containsSafetyNote(notes []string) bool {
	joined := strings.ToLower(strings.Join(notes, " "))
	return strings.Contains(joined, "refrigerate") || strings.Contains(joined, "within 2 hours") || strings.Contains(joined, "leftover")
}

func failedCheckIDs(checks []checker.CheckResult) []string {
	var ids []string
	for _, check := range checks {
		if check.Status == "block" || check.Status == "warn" {
			ids = append(ids, check.CheckID)
		}
	}
	return ids
}

func hasFailedCheck(checks []checker.CheckResult, checkID string) bool {
	for _, check := range checks {
		if check.CheckID == checkID && (check.Status == "block" || check.Status == "warn") {
			return true
		}
	}
	return false
}

func supportsUnit(food checker.CatalogFood, unit string) bool {
	if unit == "" {
		return false
	}
	_, ok := food.UnitConversions[unit]
	return ok
}

func intEvidence(evidence map[string]any, key string) (int, bool) {
	switch value := evidence[key].(type) {
	case int:
		return value, true
	case float64:
		return int(value), true
	default:
		return 0, false
	}
}

func stringEvidence(evidence map[string]any, key string) (string, bool) {
	value, ok := evidence[key].(string)
	return value, ok && value != ""
}

func normalizedSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, value := range values {
		set[normalizeName(value)] = true
		set[normalizeAllergen(value)] = true
	}
	return set
}

func normalizeAllergen(value string) string {
	return strings.TrimSuffix(normalizeName(value), "s")
}

func normalizeName(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}
