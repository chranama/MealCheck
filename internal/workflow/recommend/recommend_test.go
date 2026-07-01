package recommend

import (
	"testing"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

func TestGenerateReturnsUnavailableForPassingPlan(t *testing.T) {
	c := testCase()
	plan := passingPlan()
	catalog := testCatalog()
	evaluation := checker.Evaluate(c, plan, catalog)
	if evaluation.Decision != "pass" {
		t.Fatalf("setup decision = %q, want pass", evaluation.Decision)
	}

	got := Generate(c, plan, catalog, evaluation)
	if got.Status != "unavailable" {
		t.Fatalf("Status = %q, want unavailable", got.Status)
	}
	if got.ModifiedPlan != nil {
		t.Fatal("ModifiedPlan must be nil for unavailable recommendation")
	}
}

func TestGenerateAddsPrepSafetyNoteWhenItMakesPlanPass(t *testing.T) {
	c := testCase()
	plan := passingPlan()
	plan.PrepNotes = nil
	catalog := testCatalog()
	evaluation := checker.Evaluate(c, plan, catalog)
	if evaluation.Decision != "warn" {
		t.Fatalf("setup decision = %q, want warn", evaluation.Decision)
	}

	got := Generate(c, plan, catalog, evaluation)
	if got.Status != "available" {
		t.Fatalf("Status = %q, want available: %s", got.Status, got.Reason)
	}
	if got.ModifiedPlan == nil || !containsSafetyNote(got.ModifiedPlan.PrepNotes) {
		t.Fatalf("modified plan prep notes = %#v, want safety note", got.ModifiedPlan)
	}
	if got.ProjectedDecision == nil || got.ProjectedDecision.Decision != "pass" {
		t.Fatalf("projected decision = %#v, want pass", got.ProjectedDecision)
	}
	if len(got.Changes) != 1 || got.Changes[0].Operation != "add_prep_note" {
		t.Fatalf("changes = %#v, want one add_prep_note", got.Changes)
	}
}

func TestGenerateReplacesAllergenWhenItMakesPlanPass(t *testing.T) {
	c := testCase()
	c.Settings.VerificationConstraints.Allergies = []string{"peanuts"}
	c.Settings.VerificationConstraints.RequiresPrepSafetyNotes = false
	plan := passingPlan()
	quantity := 1.0
	plan.Days[0].Meals[1].Items = append(plan.Days[0].Meals[1].Items, checker.FoodItem{
		Food:     "peanut sauce",
		Quantity: &quantity,
		Unit:     "tbsp",
	})
	catalog := testCatalog()
	evaluation := checker.Evaluate(c, plan, catalog)
	if evaluation.Decision != "block" {
		t.Fatalf("setup decision = %q, want block", evaluation.Decision)
	}

	got := Generate(c, plan, catalog, evaluation)
	if got.Status != "available" {
		t.Fatalf("Status = %q, want available: %s", got.Status, got.Reason)
	}
	if got.ProjectedDecision == nil || got.ProjectedDecision.Decision != "pass" {
		t.Fatalf("projected decision = %#v, want pass", got.ProjectedDecision)
	}
	replacement := got.Changes[0].To
	if replacement == nil || replacement.Food == "peanut sauce" {
		t.Fatalf("replacement = %#v, want non-peanut substitute", replacement)
	}
}

func TestGenerateAddsMissingVegetableWhenItMakesPlanPass(t *testing.T) {
	c := testCase()
	plan := passingPlan()
	plan.Days[0].Meals[1].Items = nil
	catalog := testCatalog()
	evaluation := checker.Evaluate(c, plan, catalog)
	if evaluation.Decision != "warn" {
		t.Fatalf("setup decision = %q, want warn", evaluation.Decision)
	}

	got := Generate(c, plan, catalog, evaluation)
	if got.Status != "available" {
		t.Fatalf("Status = %q, want available: %s", got.Status, got.Reason)
	}
	if got.ProjectedDecision == nil || got.ProjectedDecision.Decision != "pass" {
		t.Fatalf("projected decision = %#v, want pass", got.ProjectedDecision)
	}
	if len(got.Changes) != 1 || got.Changes[0].Operation != "add_item" {
		t.Fatalf("changes = %#v, want one add_item", got.Changes)
	}
	added := got.Changes[0].To
	if added == nil || added.Food != "broccoli" || added.Quantity == nil || *added.Quantity != 1 || added.Unit != "cup" {
		t.Fatalf("added item = %#v, want 1 cup broccoli", added)
	}
}

func TestGenerateRefusesUnresolvedQuantities(t *testing.T) {
	c := testCase()
	plan := passingPlan()
	plan.Days[0].Meals[0].Items[0].Quantity = nil
	plan.Days[0].Meals[0].Items[0].QuantityText = "some"
	catalog := testCatalog()
	evaluation := checker.Evaluate(c, plan, catalog)
	if evaluation.Decision != "block" {
		t.Fatalf("setup decision = %q, want block", evaluation.Decision)
	}

	got := Generate(c, plan, catalog, evaluation)
	if got.Status != "unavailable" {
		t.Fatalf("Status = %q, want unavailable", got.Status)
	}
	if got.ModifiedPlan != nil || got.ProjectedDecision != nil {
		t.Fatalf("unavailable recommendation leaked plan/projected decision: %#v", got)
	}
	if len(got.BlockingChecks) == 0 || got.BlockingChecks[0] != "quantities_resolvable" {
		t.Fatalf("BlockingChecks = %v, want quantities_resolvable first", got.BlockingChecks)
	}
}

func testCase() checker.Case {
	return checker.Case{
		SchemaVersion:     "0.1",
		CaseID:            "recommend-test",
		GuidelinePackID:   "test-pack",
		GuidelinePackPath: "guidelines.json",
		Settings: checker.Settings{
			NutritionTargets: checker.NutritionTargets{
				CalorieTargetKcal: 1000,
				ProteinTargetG:    1,
			},
			VerificationConstraints: checker.VerificationConstraints{
				Days:                       1,
				MealsPerDay:                3,
				MaxSodiumMGPerDay:          10000,
				MaxAddedSugarGPerMeal:      100,
				MaxSaturatedFatPctCalories: 100,
				CalorieTolerancePct:        100,
				RequiresPrepSafetyNotes:    true,
			},
		},
	}
}

func passingPlan() checker.Plan {
	return checker.Plan{
		SchemaVersion: "0.1",
		PlanID:        "recommend-source",
		Days: []checker.PlanDay{
			{
				Day: 1,
				Meals: []checker.Meal{
					{Name: "breakfast", Items: []checker.FoodItem{item("cooked oatmeal", 1, "cup")}},
					{Name: "lunch", Items: []checker.FoodItem{item("broccoli", 1, "cup")}},
					{Name: "dinner", Items: []checker.FoodItem{item("chicken breast", 4, "oz")}},
				},
			},
		},
		PrepNotes: []string{safetyPrepNote},
	}
}

func item(food string, quantity float64, unit string) checker.FoodItem {
	return checker.FoodItem{Food: food, Quantity: &quantity, Unit: unit}
}

func testCatalog() checker.NutrientCatalog {
	return checker.NutrientCatalog{
		SchemaVersion: "0.1",
		CatalogID:     "recommend-catalog",
		Foods: []checker.CatalogFood{
			{
				FoodID:          "oatmeal",
				Name:            "cooked oatmeal",
				UnitConversions: map[string]float64{"cup": 240, "serving": 240},
				FoodGroups:      []string{"whole_grains"},
				NutrientsPer100G: checker.Nutrients{
					EnergyKcal: 70,
					ProteinG:   2,
				},
			},
			{
				FoodID:          "broccoli",
				Name:            "broccoli",
				UnitConversions: map[string]float64{"cup": 90, "serving": 90},
				FoodGroups:      []string{"vegetables"},
				NutrientsPer100G: checker.Nutrients{
					EnergyKcal: 35,
					ProteinG:   3,
				},
			},
			{
				FoodID:          "chicken",
				Name:            "chicken breast",
				UnitConversions: map[string]float64{"oz": 28.35, "serving": 113.4},
				FoodGroups:      []string{"protein"},
				NutrientsPer100G: checker.Nutrients{
					EnergyKcal: 165,
					ProteinG:   31,
				},
			},
			{
				FoodID:          "peanut-sauce",
				Name:            "peanut sauce",
				UnitConversions: map[string]float64{"tbsp": 16, "serving": 32},
				Allergens:       []string{"peanuts"},
				FoodGroups:      []string{"fats", "protein"},
				NutrientsPer100G: checker.Nutrients{
					EnergyKcal: 500,
					ProteinG:   15,
				},
			},
			{
				FoodID:          "olive-oil",
				Name:            "olive oil",
				UnitConversions: map[string]float64{"tbsp": 14, "serving": 14},
				FoodGroups:      []string{"fats"},
				NutrientsPer100G: checker.Nutrients{
					EnergyKcal: 884,
				},
			},
		},
	}
}
