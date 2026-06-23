package recommend

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chranama/MealCheck/internal/checker"
)

const safetyPrepNote = "Refrigerate leftovers within 2 hours."

type Document struct {
	SchemaVersion     string                    `json:"schema_version"`
	Status            string                    `json:"status"`
	Reason            string                    `json:"reason"`
	SourceDecision    string                    `json:"source_decision"`
	SourcePlanID      string                    `json:"source_plan_id"`
	BlockingChecks    []string                  `json:"blocking_checks,omitempty"`
	Changes           []Change                  `json:"changes,omitempty"`
	ModifiedPlan      *checker.Plan             `json:"modified_plan,omitempty"`
	ProjectedDecision *checker.DecisionDocument `json:"projected_decision,omitempty"`
}

type Change struct {
	Operation       string            `json:"operation"`
	Day             int               `json:"day,omitempty"`
	Meal            string            `json:"meal,omitempty"`
	From            *checker.FoodItem `json:"from,omitempty"`
	To              *checker.FoodItem `json:"to,omitempty"`
	PrepNote        string            `json:"prep_note,omitempty"`
	Reason          string            `json:"reason"`
	AddressesChecks []string          `json:"addresses_checks"`
}

func Generate(c checker.Case, plan checker.Plan, catalog checker.NutrientCatalog, source checker.Evaluation) Document {
	doc := Document{
		SchemaVersion:  "0.1",
		Status:         "unavailable",
		Reason:         "No deterministic modification is available.",
		SourceDecision: source.Decision,
		SourcePlanID:   plan.PlanID,
		BlockingChecks: failedCheckIDs(source.Checks),
	}

	if source.Decision == "pass" {
		doc.Reason = "Recommendation is only attempted for block or warn decisions."
		return doc
	}
	if hasFailedCheck(source.Checks, "required_meals_present") {
		doc.Reason = "No deterministic recommendation is available because the meal structure is incomplete."
		return doc
	}
	if hasFailedCheck(source.Checks, "quantities_resolvable") {
		doc.Reason = "No deterministic recommendation is available because one or more food quantities or units are unresolved."
		return doc
	}

	modified := clonePlan(plan)
	changes := make([]Change, 0)
	if changeSet := replaceBlockedFoods(c, modified, catalog, source); len(changeSet) > 0 {
		changes = append(changes, changeSet...)
	}
	if changeSet := addMissingVegetables(c, modified, catalog, source); len(changeSet) > 0 {
		changes = append(changes, changeSet...)
	}
	if change, ok := addPrepSafetyNote(modified, source); ok {
		changes = append(changes, change)
	}
	if len(changes) == 0 {
		doc.Reason = "No supported deterministic edit matched the failed checks."
		return doc
	}

	projected := checker.Evaluate(c, *modified, catalog)
	if projected.Decision != "pass" {
		doc.Reason = "A deterministic edit was attempted, but the modified meal plan still does not pass all configured checks."
		doc.BlockingChecks = failedCheckIDs(projected.Checks)
		return doc
	}

	projectedDecision := projected.DecisionDocument(c)
	doc.Status = "available"
	doc.Reason = "A deterministic modified meal plan passed the configured checks."
	doc.BlockingChecks = nil
	doc.Changes = changes
	doc.ModifiedPlan = modified
	doc.ProjectedDecision = &projectedDecision
	return doc
}

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
