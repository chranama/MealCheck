package checker

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

func Evaluate(c Case, plan Plan, catalog NutrientCatalog) Evaluation {
	resolved, unresolved := newResolver(catalog).resolvePlan(plan)
	dailyTotals, mealTotals := calculateTotals(resolved)

	checks := []CheckResult{
		checkSchemaValid(),
		checkRequiredMeals(c, plan),
		checkQuantitiesResolvable(unresolved),
		checkAllergensAbsent(c, resolved),
		checkExcludedFoodsAbsent(c, resolved),
		checkCaloriesWithinTolerance(c, dailyTotals),
		checkSodiumUnderLimit(c, dailyTotals),
		checkAddedSugarUnderLimit(c, mealTotals),
		checkSaturatedFatUnderLimit(c, dailyTotals),
		checkProteinMinimumMet(c, dailyTotals),
		checkFoodGroupCoverage(dailyTotals),
		checkPrepSafety(c, plan),
	}

	decision, risk := aggregateDecision(checks)
	return Evaluation{
		CaseID:            c.CaseID,
		Decision:          decision,
		RiskLevel:         risk,
		Summary:           summary(decision),
		RecommendedAction: recommendedAction(decision),
		Checks:            checks,
		ResolvedItems:     resolved,
		UnresolvedItems:   unresolved,
		DailyTotals:       dailyTotals,
		MealTotals:        mealTotals,
	}
}

func calculateTotals(items []ResolvedItem) ([]DailyTotal, []MealTotal) {
	daily := map[int]*DailyTotal{}
	meals := map[string]*MealTotal{}
	for _, item := range items {
		dayTotal := daily[item.Day]
		if dayTotal == nil {
			dayTotal = &DailyTotal{Day: item.Day, FoodGroups: map[string]bool{}}
			daily[item.Day] = dayTotal
		}
		dayTotal.Nutrients = addNutrients(dayTotal.Nutrients, item.Nutrients)
		for _, group := range item.FoodGroups {
			dayTotal.FoodGroups[group] = true
		}

		key := fmt.Sprintf("%d:%s", item.Day, item.Meal)
		mealTotal := meals[key]
		if mealTotal == nil {
			mealTotal = &MealTotal{Day: item.Day, Meal: item.Meal}
			meals[key] = mealTotal
		}
		mealTotal.Nutrients = addNutrients(mealTotal.Nutrients, item.Nutrients)
	}

	dailyTotals := make([]DailyTotal, 0, len(daily))
	for _, total := range daily {
		if total.Nutrients.EnergyKcal > 0 {
			total.SaturatedFatPctCalories = total.Nutrients.SaturatedFatG * 9 / total.Nutrients.EnergyKcal * 100
		}
		dailyTotals = append(dailyTotals, *total)
	}
	sort.Slice(dailyTotals, func(i, j int) bool { return dailyTotals[i].Day < dailyTotals[j].Day })

	mealTotals := make([]MealTotal, 0, len(meals))
	for _, total := range meals {
		mealTotals = append(mealTotals, *total)
	}
	sort.Slice(mealTotals, func(i, j int) bool {
		if mealTotals[i].Day != mealTotals[j].Day {
			return mealTotals[i].Day < mealTotals[j].Day
		}
		return mealTotals[i].Meal < mealTotals[j].Meal
	})

	return dailyTotals, mealTotals
}

func addNutrients(a, b Nutrients) Nutrients {
	return Nutrients{
		EnergyKcal:    a.EnergyKcal + b.EnergyKcal,
		ProteinG:      a.ProteinG + b.ProteinG,
		CarbohydrateG: a.CarbohydrateG + b.CarbohydrateG,
		FatG:          a.FatG + b.FatG,
		SaturatedFatG: a.SaturatedFatG + b.SaturatedFatG,
		SodiumMG:      a.SodiumMG + b.SodiumMG,
		AddedSugarG:   a.AddedSugarG + b.AddedSugarG,
		FiberG:        a.FiberG + b.FiberG,
	}
}

func checkSchemaValid() CheckResult {
	return CheckResult{
		CheckID:  "meal_plan_schema_valid",
		Status:   "pass",
		Severity: "info",
		Message:  "The candidate plan was loaded as normalized meal-plan JSON.",
	}
}

func checkRequiredMeals(c Case, plan Plan) CheckResult {
	var evidence []map[string]any
	constraints := c.Settings.VerificationConstraints
	if len(plan.Days) != constraints.Days {
		evidence = append(evidence, map[string]any{"expected_days": constraints.Days, "actual_days": len(plan.Days)})
	}
	for _, day := range plan.Days {
		if len(day.Meals) != constraints.MealsPerDay {
			evidence = append(evidence, map[string]any{"day": day.Day, "expected_meals": constraints.MealsPerDay, "actual_meals": len(day.Meals)})
		}
	}
	if len(evidence) > 0 {
		return CheckResult{CheckID: "required_meals_present", Status: "block", Severity: "block", Message: "The candidate is missing required meal structure.", Evidence: evidence}
	}
	return CheckResult{CheckID: "required_meals_present", Status: "pass", Severity: "info", Message: "The candidate includes the required number of days and meals per day."}
}

func checkQuantitiesResolvable(unresolved []UnresolvedItem) CheckResult {
	if len(unresolved) == 0 {
		return CheckResult{CheckID: "quantities_resolvable", Status: "pass", Severity: "info", Message: "All candidate food quantities resolved."}
	}
	evidence := make([]map[string]any, 0, len(unresolved))
	var days []int
	var meals []string
	for _, item := range unresolved {
		evidence = append(evidence, map[string]any{
			"day":               item.Day,
			"meal":              item.Meal,
			"food":              item.Food,
			"quantity_text":     item.QuantityText,
			"unit":              item.Unit,
			"unresolved_reason": item.UnresolvedReason,
		})
		days = appendUniqueInt(days, item.Day)
		meals = appendUniqueString(meals, item.Meal)
	}
	return CheckResult{CheckID: "quantities_resolvable", Status: "block", Severity: "block", Message: "The candidate includes unresolved food quantities or units.", Evidence: evidence, AffectedDays: days, AffectedMeals: meals}
}

func checkAllergensAbsent(c Case, items []ResolvedItem) CheckResult {
	allergies := normalizedSet(c.Settings.VerificationConstraints.Allergies)
	var evidence []map[string]any
	var days []int
	var meals []string
	for _, item := range items {
		for _, allergen := range item.Allergens {
			if allergies[normalizeAllergen(allergen)] {
				evidence = append(evidence, map[string]any{"day": item.Day, "meal": item.Meal, "food": item.Food, "matched_allergen": allergen})
				days = appendUniqueInt(days, item.Day)
				meals = appendUniqueString(meals, item.Meal)
			}
		}
	}
	if len(evidence) > 0 {
		return CheckResult{CheckID: "allergens_absent", Status: "block", Severity: "block", Message: "The candidate includes a declared allergen.", Evidence: evidence, SourceRefs: []string{"fda-food-allergies"}, AffectedDays: days, AffectedMeals: meals}
	}
	return CheckResult{CheckID: "allergens_absent", Status: "pass", Severity: "info", Message: "No declared allergen was found in resolved foods."}
}

func checkExcludedFoodsAbsent(c Case, items []ResolvedItem) CheckResult {
	excluded := normalizedSet(c.Settings.VerificationConstraints.ExcludedFoods)
	var evidence []map[string]any
	for _, item := range items {
		if excluded[normalizeName(item.Food)] {
			evidence = append(evidence, map[string]any{"day": item.Day, "meal": item.Meal, "food": item.Food})
		}
	}
	if len(evidence) > 0 {
		return CheckResult{CheckID: "excluded_foods_absent", Status: "block", Severity: "block", Message: "The candidate includes a user-excluded food.", Evidence: evidence}
	}
	return CheckResult{CheckID: "excluded_foods_absent", Status: "pass", Severity: "info", Message: "No user-excluded food was found in resolved foods."}
}

func checkCaloriesWithinTolerance(c Case, totals []DailyTotal) CheckResult {
	target := float64(c.Settings.NutritionTargets.CalorieTargetKcal)
	tolerance := c.Settings.VerificationConstraints.CalorieTolerancePct
	if target <= 0 || tolerance <= 0 {
		return CheckResult{CheckID: "calories_within_tolerance", Status: "not_applicable", Severity: "not_applicable", Message: "No calorie target and tolerance were configured."}
	}
	low := target * (1 - tolerance/100)
	high := target * (1 + tolerance/100)
	var evidence []map[string]any
	for _, total := range totals {
		if total.Nutrients.EnergyKcal < low || total.Nutrients.EnergyKcal > high {
			evidence = append(evidence, map[string]any{"day": total.Day, "energy_kcal": round1(total.Nutrients.EnergyKcal), "target_kcal": target, "tolerance_pct": tolerance})
		}
	}
	if len(evidence) > 0 {
		return CheckResult{CheckID: "calories_within_tolerance", Status: "warn", Severity: "warn", Message: "One or more days are outside the configured calorie tolerance.", Evidence: evidence}
	}
	return CheckResult{CheckID: "calories_within_tolerance", Status: "pass", Severity: "info", Message: "Daily calories are within the configured tolerance."}
}

func checkSodiumUnderLimit(c Case, totals []DailyTotal) CheckResult {
	limit := float64(c.Settings.VerificationConstraints.MaxSodiumMGPerDay)
	if limit <= 0 {
		return CheckResult{CheckID: "sodium_under_limit", Status: "not_applicable", Severity: "not_applicable", Message: "No sodium limit was configured."}
	}
	var evidence []map[string]any
	var days []int
	for _, total := range totals {
		if total.Nutrients.SodiumMG >= limit {
			evidence = append(evidence, map[string]any{"day": total.Day, "sodium_mg": round1(total.Nutrients.SodiumMG), "limit_mg": limit})
			days = append(days, total.Day)
		}
	}
	if len(evidence) > 0 {
		return CheckResult{CheckID: "sodium_under_limit", Status: "warn", Severity: "warn", Message: "One or more days exceed the configured sodium limit.", Evidence: evidence, SourceRefs: []string{"dga-2025-2030"}, AffectedDays: days}
	}
	return CheckResult{CheckID: "sodium_under_limit", Status: "pass", Severity: "info", Message: "Daily sodium is below the configured limit."}
}

func checkAddedSugarUnderLimit(c Case, totals []MealTotal) CheckResult {
	limit := c.Settings.VerificationConstraints.MaxAddedSugarGPerMeal
	if limit <= 0 {
		return CheckResult{CheckID: "added_sugar_under_limit", Status: "not_applicable", Severity: "not_applicable", Message: "No added-sugar meal limit was configured."}
	}
	var evidence []map[string]any
	var days []int
	var meals []string
	for _, total := range totals {
		if total.Nutrients.AddedSugarG > limit {
			evidence = append(evidence, map[string]any{"day": total.Day, "meal": total.Meal, "added_sugar_g": round1(total.Nutrients.AddedSugarG), "limit_g": limit})
			days = appendUniqueInt(days, total.Day)
			meals = appendUniqueString(meals, total.Meal)
		}
	}
	if len(evidence) > 0 {
		return CheckResult{CheckID: "added_sugar_under_limit", Status: "warn", Severity: "warn", Message: "One or more meals exceed the configured added-sugar limit.", Evidence: evidence, SourceRefs: []string{"dga-2025-2030"}, AffectedDays: days, AffectedMeals: meals}
	}
	return CheckResult{CheckID: "added_sugar_under_limit", Status: "pass", Severity: "info", Message: "Meal added sugar is within the configured limit."}
}

func checkSaturatedFatUnderLimit(c Case, totals []DailyTotal) CheckResult {
	limit := c.Settings.VerificationConstraints.MaxSaturatedFatPctCalories
	if limit <= 0 {
		return CheckResult{CheckID: "saturated_fat_under_limit", Status: "not_applicable", Severity: "not_applicable", Message: "No saturated-fat limit was configured."}
	}
	var evidence []map[string]any
	for _, total := range totals {
		if total.SaturatedFatPctCalories > limit {
			evidence = append(evidence, map[string]any{"day": total.Day, "saturated_fat_pct_calories": round1(total.SaturatedFatPctCalories), "limit_pct": limit})
		}
	}
	if len(evidence) > 0 {
		return CheckResult{CheckID: "saturated_fat_under_limit", Status: "warn", Severity: "warn", Message: "One or more days exceed the configured saturated-fat limit.", Evidence: evidence, SourceRefs: []string{"dga-2025-2030"}}
	}
	return CheckResult{CheckID: "saturated_fat_under_limit", Status: "pass", Severity: "info", Message: "Daily saturated fat is within the configured limit."}
}

func checkProteinMinimumMet(c Case, totals []DailyTotal) CheckResult {
	limit := float64(c.Settings.NutritionTargets.ProteinTargetG)
	if limit <= 0 {
		return CheckResult{CheckID: "protein_minimum_met", Status: "not_applicable", Severity: "not_applicable", Message: "No protein minimum could be derived."}
	}
	var evidence []map[string]any
	for _, total := range totals {
		if total.Nutrients.ProteinG < limit {
			evidence = append(evidence, map[string]any{"day": total.Day, "protein_g": round1(total.Nutrients.ProteinG), "minimum_g": round1(limit)})
		}
	}
	if len(evidence) > 0 {
		return CheckResult{CheckID: "protein_minimum_met", Status: "warn", Severity: "warn", Message: "One or more days are below the configured protein minimum.", Evidence: evidence, SourceRefs: []string{"dga-2025-2030"}}
	}
	return CheckResult{CheckID: "protein_minimum_met", Status: "pass", Severity: "info", Message: "Daily protein meets the configured minimum."}
}

func checkFoodGroupCoverage(totals []DailyTotal) CheckResult {
	var evidence []map[string]any
	for _, total := range totals {
		if !total.FoodGroups["vegetables"] {
			evidence = append(evidence, map[string]any{"day": total.Day, "missing_food_group": "vegetables"})
		}
	}
	if len(evidence) > 0 {
		return CheckResult{CheckID: "food_group_coverage", Status: "warn", Severity: "warn", Message: "One or more days lack a resolved vegetable item.", Evidence: evidence, SourceRefs: []string{"dga-2025-2030"}}
	}
	return CheckResult{CheckID: "food_group_coverage", Status: "pass", Severity: "info", Message: "Each day includes a resolved vegetable item."}
}

func checkPrepSafety(c Case, plan Plan) CheckResult {
	if !c.Settings.VerificationConstraints.RequiresPrepSafetyNotes {
		return CheckResult{CheckID: "prep_safety_mentions_present", Status: "not_applicable", Severity: "not_applicable", Message: "Prep-safety notes were not required."}
	}
	joined := strings.ToLower(strings.Join(plan.PrepNotes, " "))
	if strings.Contains(joined, "refrigerate") || strings.Contains(joined, "within 2 hours") || strings.Contains(joined, "leftover") {
		return CheckResult{CheckID: "prep_safety_mentions_present", Status: "pass", Severity: "info", Message: "Prep notes include leftover refrigeration guidance.", SourceRefs: []string{"foodsafety-four-steps"}}
	}
	return CheckResult{CheckID: "prep_safety_mentions_present", Status: "warn", Severity: "warn", Message: "Prep notes do not mention prompt refrigeration or leftover handling.", SourceRefs: []string{"foodsafety-four-steps"}}
}

func aggregateDecision(checks []CheckResult) (string, string) {
	hasWarn := false
	for _, check := range checks {
		if check.Status == "block" {
			return "block", "high"
		}
		if check.Status == "warn" {
			hasWarn = true
		}
	}
	if hasWarn {
		return "warn", "medium"
	}
	return "pass", "low"
}

func summary(decision string) string {
	switch decision {
	case "block":
		return "The candidate plan has blocking issues and should be revised before use."
	case "warn":
		return "The candidate plan has review-needed warnings."
	default:
		return "The candidate plan passed the configured checks."
	}
}

func recommendedAction(decision string) string {
	switch decision {
	case "block":
		return "Revise blocking violations and rerun MealCheck."
	case "warn":
		return "Review warnings before using the plan."
	default:
		return "No required action."
	}
}

func (e Evaluation) DecisionDocument(c Case) DecisionDocument {
	return DecisionDocument{
		SchemaVersion:     "0.1",
		CaseID:            e.CaseID,
		Decision:          e.Decision,
		Summary:           e.Summary,
		RiskLevel:         e.RiskLevel,
		FailedChecks:      failedChecks(e.Checks),
		UnresolvedItems:   e.UnresolvedItems,
		RecommendedAction: e.RecommendedAction,
		GuidelinePackID:   c.GuidelinePackID,
		ArtifactPaths: map[string]string{
			"case":                  "examples/seeded-3-day-peanut-allergy/case.json",
			"baseline_plan":         c.BaselinePlan,
			"candidate_plan":        c.CandidatePlan,
			"guideline_pack_path":   c.GuidelinePackPath,
			"nutrient_catalog_path": c.NutrientCatalogPath,
		},
		Checks: e.Checks,
	}
}

func failedChecks(checks []CheckResult) []string {
	var result []string
	for _, check := range checks {
		if check.Status == "block" || check.Status == "warn" {
			result = append(result, check.CheckID)
		}
	}
	return result
}

func normalizedSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, value := range values {
		set[normalizeAllergen(value)] = true
		set[normalizeName(value)] = true
	}
	return set
}

func normalizeAllergen(value string) string {
	value = normalizeName(value)
	return strings.TrimSuffix(value, "s")
}

func appendUniqueInt(values []int, value int) []int {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func round1(value float64) float64 {
	return math.Round(value*10) / 10
}
