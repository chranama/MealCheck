package checker

import (
	"path/filepath"
	"testing"
)

func TestSQLiteFNDDSReferenceLookupEligibleExactMatch(t *testing.T) {
	ref := openTestFNDDSReference(t)

	food, ok, err := ref.LookupEligibleByDescription("  WATER, tap  ")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Water, tap did not resolve through FNDDS fallback")
	}
	if food.FoodID != "fndds_94000100" {
		t.Fatalf("FoodID = %q, want fndds_94000100", food.FoodID)
	}
	if food.Name != "Water, tap" {
		t.Fatalf("Name = %q, want Water, tap", food.Name)
	}
	if food.UnitConversions["g"] != 1 || food.UnitConversions["gram"] != 1 || food.UnitConversions["grams"] != 1 {
		t.Fatalf("unexpected gram conversions: %#v", food.UnitConversions)
	}
	if food.UnitConversions["cup"] != 240 || food.UnitConversions["cups"] != 240 {
		t.Fatalf("unexpected cup conversions: %#v", food.UnitConversions)
	}
	if food.NutrientsPer100G.SodiumMG < 0 {
		t.Fatalf("SodiumMG = %.1f, want non-negative", food.NutrientsPer100G.SodiumMG)
	}
}

func TestSQLiteFNDDSReferenceLookupMatchKeyAliases(t *testing.T) {
	ref := openTestFNDDSReference(t)

	tests := []struct {
		description string
		foodID      string
		name        string
	}{
		{
			description: "instant coffee",
			foodID:      "fndds_92103000",
			name:        "Coffee, instant, reconstituted",
		},
		{
			description: "granulated sugar",
			foodID:      "fndds_91101010",
			name:        "Sugar, white, granulated or lump",
		},
		{
			description: "lettuce",
			foodID:      "fndds_89902020",
			name:        "Lettuce, for use on a sandwich",
		},
		{
			description: "cooked white rice",
			foodID:      "fndds_56205008",
			name:        "Rice, white, cooked, no added fat",
		},
	}
	for _, tc := range tests {
		food, ok, err := ref.LookupEligibleByDescription(tc.description)
		if err != nil {
			t.Fatalf("%s lookup returned error: %v", tc.description, err)
		}
		if !ok {
			t.Fatalf("%s did not resolve through FNDDS fallback", tc.description)
		}
		if food.FoodID != tc.foodID || food.Name != tc.name {
			t.Fatalf("%s resolved to %s %q, want %s %q", tc.description, food.FoodID, food.Name, tc.foodID, tc.name)
		}
	}
}

func TestSQLiteFNDDSReferenceRejectsQuarantinedAndReviewRequiredRows(t *testing.T) {
	ref := openTestFNDDSReference(t)

	for _, description := range []string{"Milk, NFS", "Milk, human"} {
		if food, ok, err := ref.LookupEligibleByDescription(description); err != nil {
			t.Fatalf("%s lookup returned error: %v", description, err)
		} else if ok {
			t.Fatalf("%s resolved unexpectedly: %+v", description, food)
		}
	}
}

func TestSQLiteFNDDSReferenceLookupApproximationProxy(t *testing.T) {
	ref := openTestFNDDSReference(t)

	proxy, ok, err := ref.LookupApproximationProxy(" Rice ")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("rice approximation proxy did not resolve")
	}
	if proxy.ProxyFoodCode != "56205001" {
		t.Fatalf("ProxyFoodCode = %q, want 56205001", proxy.ProxyFoodCode)
	}
	if proxy.Food.FoodID != "fndds_56205001" {
		t.Fatalf("proxy FoodID = %q, want fndds_56205001", proxy.Food.FoodID)
	}
}

func TestSQLiteFNDDSReferenceLookupDecompositionTemplate(t *testing.T) {
	ref := openTestFNDDSReference(t)

	template, ok, err := ref.LookupDecompositionTemplate("Ham sandwich on white, with cheese")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ham sandwich decomposition template did not resolve")
	}
	if template.TemplateID != "ham_sandwich_white_cheese_v1" {
		t.Fatalf("TemplateID = %q, want ham_sandwich_white_cheese_v1", template.TemplateID)
	}
	if len(template.Components) != 3 {
		t.Fatalf("len(Components) = %d, want 3", len(template.Components))
	}
	if template.Components[0].FoodCode != "51101000" || template.Components[0].Fraction != 0.40 {
		t.Fatalf("first component = %+v, want white bread at 0.40", template.Components[0])
	}
}

func TestResolverUsesReviewedCatalogBeforeFNDDSFallback(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 100.0
	catalog := NutrientCatalog{
		Foods: []CatalogFood{
			{
				FoodID:          "reviewed_water",
				Name:            "Water, tap",
				BaseQuantityG:   100,
				UnitConversions: map[string]float64{"g": 1},
				NutrientsPer100G: Nutrients{
					EnergyKcal: 7,
				},
			},
		},
	}
	plan := Plan{
		Days: []PlanDay{
			{
				Day: 1,
				Meals: []Meal{
					{
						Name: "breakfast",
						Items: []FoodItem{
							{Food: "Water, tap", Quantity: &qty, Unit: "g"},
						},
					},
				},
			},
		},
	}

	resolved, unresolved, err := newResolverWithFallback(catalog, ref).resolvePlanWithError(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %+v, want none", unresolved)
	}
	if len(resolved) != 1 {
		t.Fatalf("len(resolved) = %d, want 1", len(resolved))
	}
	if resolved[0].FoodID != "reviewed_water" {
		t.Fatalf("FoodID = %q, want reviewed catalog match", resolved[0].FoodID)
	}
	if resolved[0].Nutrients.EnergyKcal != 7 {
		t.Fatalf("EnergyKcal = %.1f, want reviewed catalog nutrients", resolved[0].Nutrients.EnergyKcal)
	}
}

func TestResolverReviewedCatalogBypassesFallbackGate(t *testing.T) {
	qty := 28.0
	fallback := &recordingFNDDSReference{}
	catalog := NutrientCatalog{
		Foods: []CatalogFood{
			{
				FoodID:          "reviewed_cheese",
				Name:            "Cheese",
				BaseQuantityG:   100,
				UnitConversions: map[string]float64{"g": 1},
				NutrientsPer100G: Nutrients{
					EnergyKcal: 400,
				},
			},
		},
	}
	plan := singleItemPlan("Cheese", &qty, "g")

	resolved, unresolved, err := newResolverWithFallback(catalog, fallback).resolvePlanWithError(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %+v, want none", unresolved)
	}
	if len(resolved) != 1 {
		t.Fatalf("len(resolved) = %d, want 1", len(resolved))
	}
	if resolved[0].FoodID != "reviewed_cheese" {
		t.Fatalf("FoodID = %q, want reviewed catalog match", resolved[0].FoodID)
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback calls = %d, want 0", fallback.calls)
	}
}

func TestResolverUsesReviewedCatalogForSlicesAndSlicedOrangeAlias(t *testing.T) {
	var catalog NutrientCatalog
	if err := readJSON(filepath.Join(repoRoot(t), "data/nutrients/fixture-catalog-v1.json"), &catalog); err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	breadQuantity := 2.0
	orangeQuantity := 1.0
	plan := Plan{
		Days: []PlanDay{
			{
				Day: 1,
				Meals: []Meal{
					{
						Name: "breakfast",
						Items: []FoodItem{
							{Food: "whole wheat bread", Quantity: &breadQuantity, Unit: "slice"},
							{Food: "sliced oranges", Quantity: &orangeQuantity, Unit: "cup"},
						},
					},
				},
			},
		},
	}

	resolved, unresolved, err := newResolver(catalog).resolvePlanWithError(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %+v, want none", unresolved)
	}
	if len(resolved) != 2 {
		t.Fatalf("len(resolved) = %d, want 2", len(resolved))
	}
	resolvedByID := map[string]ResolvedItem{}
	for _, item := range resolved {
		resolvedByID[item.FoodID] = item
	}
	if item, ok := resolvedByID["whole_wheat_bread"]; !ok || item.Grams != 32 {
		t.Fatalf("bread resolved = %+v, want whole_wheat_bread at 32g", item)
	}
	if item, ok := resolvedByID["orange_raw"]; !ok || item.Grams != 180 {
		t.Fatalf("orange resolved = %+v, want orange_raw at 180g", item)
	}
}

func TestResolverUsesFNDDSFallbackForEligibleExactMatch(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 100.0
	plan := singleItemPlan("Water, tap", &qty, "g")

	resolved, unresolved, err := newResolverWithFallback(NutrientCatalog{}, ref).resolvePlanWithError(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %+v, want none", unresolved)
	}
	if len(resolved) != 1 {
		t.Fatalf("len(resolved) = %d, want 1", len(resolved))
	}
	if resolved[0].FoodID != "fndds_94000100" {
		t.Fatalf("FoodID = %q, want fndds_94000100", resolved[0].FoodID)
	}
}

func TestResolverRetriesExplicitUnknownFoodThroughFNDDSFallback(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 100.0
	plan := singleItemPlan("Water, tap", &qty, "g")
	plan.Days[0].Meals[0].Items[0].ResolutionStatus = "unresolved"
	plan.Days[0].Meals[0].Items[0].UnresolvedReason = "unknown_food"

	resolved, unresolved, err := newResolverWithFallback(NutrientCatalog{}, ref).resolvePlanWithError(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %+v, want none", unresolved)
	}
	if len(resolved) != 1 {
		t.Fatalf("len(resolved) = %d, want 1", len(resolved))
	}
	if resolved[0].FoodID != "fndds_94000100" {
		t.Fatalf("FoodID = %q, want fndds_94000100", resolved[0].FoodID)
	}
}

func TestResolverUsesFNDDSFallbackPortionConversions(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 1.0
	plan := singleItemPlan("Water, tap", &qty, "cup")

	resolved, unresolved, err := newResolverWithFallback(NutrientCatalog{}, ref).resolvePlanWithError(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %+v, want none", unresolved)
	}
	if len(resolved) != 1 {
		t.Fatalf("len(resolved) = %d, want 1", len(resolved))
	}
	if resolved[0].FoodID != "fndds_94000100" || resolved[0].Grams != 240 {
		t.Fatalf("resolved = %+v, want water at 240g", resolved[0])
	}
}

func TestResolverKeepsUnsupportedFallbackUnitsUnresolved(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 1.0
	plan := singleItemPlan("Water, tap", &qty, "bunch")

	resolved, unresolved, err := newResolverWithFallback(NutrientCatalog{}, ref).resolvePlanWithError(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 0 {
		t.Fatalf("resolved = %+v, want none", resolved)
	}
	if len(unresolved) != 1 {
		t.Fatalf("len(unresolved) = %d, want 1", len(unresolved))
	}
	if unresolved[0].UnresolvedReason != unresolvedUnsupportedUnit {
		t.Fatalf("UnresolvedReason = %q, want %s", unresolved[0].UnresolvedReason, unresolvedUnsupportedUnit)
	}
}

func TestResolverGateBlocksBroadFallbackLookup(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 100.0
	plan := singleItemPlan("Cheese", &qty, "g")

	resolved, unresolved, err := newResolverWithFallback(NutrientCatalog{}, ref).resolvePlanWithError(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 0 {
		t.Fatalf("resolved = %+v, want none", resolved)
	}
	if len(unresolved) != 1 {
		t.Fatalf("len(unresolved) = %d, want 1", len(unresolved))
	}
	if unresolved[0].UnresolvedReason != unresolvedAmbiguousFood {
		t.Fatalf("UnresolvedReason = %q, want %s", unresolved[0].UnresolvedReason, unresolvedAmbiguousFood)
	}
}

func TestResolverUsesApproximationProxyForCuratedBroadFood(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 100.0
	plan := singleItemPlan("Rice", &qty, "g")

	resolved, unresolved, err := newResolverWithFallback(NutrientCatalog{}, ref).resolvePlanWithError(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %+v, want none", unresolved)
	}
	if len(resolved) != 1 {
		t.Fatalf("len(resolved) = %d, want 1", len(resolved))
	}
	item := resolved[0]
	if item.ResolutionMethod != "estimated" || item.ProxyFoodID != "fndds_56205001" {
		t.Fatalf("resolved = %+v, want estimated rice proxy", item)
	}
	if item.Food != "Rice" || item.ProxyFood == "" {
		t.Fatalf("resolved food/proxy = %q/%q, want original food plus proxy label", item.Food, item.ProxyFood)
	}
}

func TestResolverBlocksApproximationProxyWhenExcludedFoodsAreConfigured(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 100.0
	plan := singleItemPlan("Rice", &qty, "g")
	constraints := VerificationConstraints{ExcludedFoods: []string{"broccoli"}}

	resolved, unresolved, err := newResolverWithFallback(NutrientCatalog{}, ref, constraints).resolvePlanWithError(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 0 {
		t.Fatalf("resolved = %+v, want none", resolved)
	}
	if len(unresolved) != 1 || unresolved[0].UnresolvedReason != unresolvedAmbiguousFood {
		t.Fatalf("unresolved = %+v, want ambiguous broad-food block", unresolved)
	}
}

func TestResolverGateBlocksMixedDishFallbackLookup(t *testing.T) {
	fallback := &recordingFNDDSReference{}
	qty := 100.0
	plan := singleItemPlan("Ham sandwich on white, with cheese", &qty, "g")

	resolved, unresolved, err := newResolverWithFallback(NutrientCatalog{}, fallback).resolvePlanWithError(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 0 {
		t.Fatalf("resolved = %+v, want none", resolved)
	}
	if len(unresolved) != 1 {
		t.Fatalf("len(unresolved) = %d, want 1", len(unresolved))
	}
	if unresolved[0].UnresolvedReason != unresolvedComposedFoodNeedsDecomposition {
		t.Fatalf("UnresolvedReason = %q, want %s", unresolved[0].UnresolvedReason, unresolvedComposedFoodNeedsDecomposition)
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback calls = %d, want 0", fallback.calls)
	}
}

func TestResolverUsesDecompositionTemplateForCuratedComposedFood(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 100.0
	plan := singleItemPlan("Ham sandwich on white, with cheese", &qty, "g")

	resolved, unresolved, err := newResolverWithFallback(NutrientCatalog{}, ref).resolvePlanWithError(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %+v, want none", unresolved)
	}
	if len(resolved) != 1 {
		t.Fatalf("len(resolved) = %d, want 1", len(resolved))
	}
	item := resolved[0]
	if item.ResolutionMethod != "decomposed" || item.FoodID != "decomposed_ham_sandwich_white_cheese_v1" {
		t.Fatalf("resolved = %+v, want decomposed ham sandwich", item)
	}
	if len(item.Components) != 3 {
		t.Fatalf("len(Components) = %d, want 3", len(item.Components))
	}
	if item.Components[0].Grams != 40 {
		t.Fatalf("first component grams = %.1f, want 40", item.Components[0].Grams)
	}
}

func TestEvaluateWarnsWhenApproximateResolutionIsUsed(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 100.0
	plan := singleItemPlan("Rice", &qty, "g")
	c := Case{
		CaseID: "approximation-warning",
		Settings: Settings{
			NutritionTargets: NutritionTargets{CalorieTargetKcal: 2000, ProteinTargetG: 50},
		},
	}

	evaluation, err := EvaluateWithFallback(c, plan, NutrientCatalog{}, ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(evaluation.UnresolvedItems) != 0 {
		t.Fatalf("UnresolvedItems = %+v, want none", evaluation.UnresolvedItems)
	}
	if checkStatus(evaluation.Checks)["estimated_or_decomposed_foods"] != "warn" {
		t.Fatalf("estimated_or_decomposed_foods = %q, want warn", checkStatus(evaluation.Checks)["estimated_or_decomposed_foods"])
	}
}

func TestResolverGateBlocksBrandedFallbackLookup(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 100.0
	plan := singleItemPlan("French fries", &qty, "g")
	plan.Days[0].Meals[0].Items[0].Brand = "McDonald's"

	resolved, unresolved, err := newResolverWithFallback(NutrientCatalog{}, ref).resolvePlanWithError(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 0 {
		t.Fatalf("resolved = %+v, want none", resolved)
	}
	if len(unresolved) != 1 {
		t.Fatalf("len(unresolved) = %d, want 1", len(unresolved))
	}
	if unresolved[0].UnresolvedReason != unresolvedRestaurantOrBrandedFood {
		t.Fatalf("UnresolvedReason = %q, want %s", unresolved[0].UnresolvedReason, unresolvedRestaurantOrBrandedFood)
	}
}

func TestResolverGateBlocksNonFoodFallbackLookup(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 1.0
	plan := singleItemPlan("Vitamin supplement", &qty, "g")

	resolved, unresolved, err := newResolverWithFallback(NutrientCatalog{}, ref).resolvePlanWithError(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 0 {
		t.Fatalf("resolved = %+v, want none", resolved)
	}
	if len(unresolved) != 1 {
		t.Fatalf("len(unresolved) = %d, want 1", len(unresolved))
	}
	if unresolved[0].UnresolvedReason != unresolvedNonFoodText {
		t.Fatalf("UnresolvedReason = %q, want %s", unresolved[0].UnresolvedReason, unresolvedNonFoodText)
	}
}

func TestResolverDoesNotFallbackExplicitlyUnresolvedItems(t *testing.T) {
	ref := openTestFNDDSReference(t)
	plan := singleItemPlan("Water, tap", nil, "")
	plan.Days[0].Meals[0].Items[0].QuantityText = "some water"
	plan.Days[0].Meals[0].Items[0].ResolutionStatus = "unresolved"
	plan.Days[0].Meals[0].Items[0].UnresolvedReason = "vague_quantity"

	resolved, unresolved, err := newResolverWithFallback(NutrientCatalog{}, ref).resolvePlanWithError(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 0 {
		t.Fatalf("resolved = %+v, want none", resolved)
	}
	if len(unresolved) != 1 {
		t.Fatalf("len(unresolved) = %d, want 1", len(unresolved))
	}
	if unresolved[0].UnresolvedReason != "vague_quantity" {
		t.Fatalf("UnresolvedReason = %q, want vague_quantity", unresolved[0].UnresolvedReason)
	}
}

func openTestFNDDSReference(t *testing.T) *SQLiteFNDDSReference {
	t.Helper()
	ref, err := OpenSQLiteFNDDSReference(filepath.Join(repoRoot(t), "data/reference/fndds-2021-2023/fndds.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ref.Close() })
	return ref
}

func singleItemPlan(food string, quantity *float64, unit string) Plan {
	return Plan{
		Days: []PlanDay{
			{
				Day: 1,
				Meals: []Meal{
					{
						Name: "breakfast",
						Items: []FoodItem{
							{Food: food, Quantity: quantity, Unit: unit},
						},
					},
				},
			},
		},
	}
}

type recordingFNDDSReference struct {
	calls int
}

func (r *recordingFNDDSReference) LookupEligibleByDescription(description string) (CatalogFood, bool, error) {
	r.calls++
	return CatalogFood{}, false, nil
}

func (r *recordingFNDDSReference) LookupApproximationProxy(inputKey string) (FNDDSApproximationProxy, bool, error) {
	return FNDDSApproximationProxy{}, false, nil
}

func (r *recordingFNDDSReference) LookupDecompositionTemplate(description string) (FNDDSDecompositionTemplate, bool, error) {
	return FNDDSDecompositionTemplate{}, false, nil
}

func (r *recordingFNDDSReference) LookupFoodByCode(foodCode string) (CatalogFood, bool, error) {
	return CatalogFood{}, false, nil
}
