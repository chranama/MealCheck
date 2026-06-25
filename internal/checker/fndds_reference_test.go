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
	if food.NutrientsPer100G.SodiumMG < 0 {
		t.Fatalf("SodiumMG = %.1f, want non-negative", food.NutrientsPer100G.SodiumMG)
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

func TestResolverKeepsFallbackHouseholdUnitsUnresolved(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 1.0
	plan := singleItemPlan("Water, tap", &qty, "cup")

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
