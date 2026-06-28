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

	tests := []struct {
		input              string
		wantProxyFoodCode  string
		wantFoodID         string
		wantTextLookupFlag bool
	}{
		{input: " Rice ", wantProxyFoodCode: "56205001", wantFoodID: "fndds_56205001", wantTextLookupFlag: true},
		{input: "Mango", wantProxyFoodCode: "63129010", wantFoodID: "fndds_63129010", wantTextLookupFlag: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			proxy, ok, err := ref.LookupApproximationProxy(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Fatalf("%s approximation proxy did not resolve", tt.input)
			}
			if proxy.ProxyFoodCode != tt.wantProxyFoodCode {
				t.Fatalf("ProxyFoodCode = %q, want %s", proxy.ProxyFoodCode, tt.wantProxyFoodCode)
			}
			if proxy.Food.FoodID != tt.wantFoodID {
				t.Fatalf("proxy FoodID = %q, want %s", proxy.Food.FoodID, tt.wantFoodID)
			}
			if proxy.TextLookupEnabled != tt.wantTextLookupFlag {
				t.Fatalf("TextLookupEnabled = %v, want %v", proxy.TextLookupEnabled, tt.wantTextLookupFlag)
			}
		})
	}

	if proxy, ok, err := ref.LookupApproximationProxy("Peppers"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatalf("Peppers resolved unexpectedly through approximation proxy: %+v", proxy)
	}
}

func TestSQLiteFNDDSReferenceLookupApproximationProxyBySourceFoodCode(t *testing.T) {
	ref := openTestFNDDSReference(t)

	tests := []struct {
		name              string
		sourceFoodCode    string
		wantInputKey      string
		wantProxyFoodCode string
		wantFoodID        string
	}{
		{name: "curated nfs", sourceFoodCode: "14010000", wantInputKey: "cheese", wantProxyFoodCode: "14010000", wantFoodID: "fndds_14010000"},
		{name: "generated fruit", sourceFoodCode: "63129010", wantInputKey: "Mango", wantProxyFoodCode: "63129010", wantFoodID: "fndds_63129010"},
		{name: "generated beverage", sourceFoodCode: "93401010", wantInputKey: "Wine, red", wantProxyFoodCode: "93401010", wantFoodID: "fndds_93401010"},
		{name: "generated nuts", sourceFoodCode: "42111200", wantInputKey: "Peanuts, dry roasted, salted", wantProxyFoodCode: "42111200", wantFoodID: "fndds_42111200"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy, ok, err := ref.LookupApproximationProxyBySourceFoodCode(tt.sourceFoodCode)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Fatalf("%s source-code approximation proxy did not resolve", tt.sourceFoodCode)
			}
			if proxy.InputKey != tt.wantInputKey || proxy.ProxyFoodCode != tt.wantProxyFoodCode {
				t.Fatalf("proxy = %+v, want %s proxy for %s", proxy, tt.wantInputKey, tt.wantProxyFoodCode)
			}
			if proxy.Food.FoodID != tt.wantFoodID {
				t.Fatalf("proxy FoodID = %q, want %s", proxy.Food.FoodID, tt.wantFoodID)
			}
		})
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

func TestSQLiteFNDDSReferenceLookupDecompositionRuleBySourceFoodCode(t *testing.T) {
	ref := openTestFNDDSReference(t)

	tests := []struct {
		name          string
		sourceCode    string
		wantRuleID    string
		wantFoodCode  string
		wantComponent float64
	}{
		{
			name:          "pasta tomato meat",
			sourceCode:    "58146322",
			wantRuleID:    "pasta_tomato_meat_v1",
			wantFoodCode:  "56130000",
			wantComponent: 0.58,
		},
		{
			name:          "tuna salad sandwich",
			sourceCode:    "27550720",
			wantRuleID:    "tuna_salad_sandwich_white_v1",
			wantFoodCode:  "51101000",
			wantComponent: 0.50,
		},
		{
			name:          "burrito beef beans rice cheese",
			sourceCode:    "58102340",
			wantRuleID:    "burrito_beef_beans_rice_cheese_v1",
			wantFoodCode:  "52215200",
			wantComponent: 0.25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, ok, err := ref.LookupDecompositionRuleBySourceFoodCode(tt.sourceCode)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Fatalf("source code %s decomposition rule did not resolve", tt.sourceCode)
			}
			if rule.RuleID != tt.wantRuleID {
				t.Fatalf("RuleID = %q, want %s", rule.RuleID, tt.wantRuleID)
			}
			if len(rule.Components) == 0 {
				t.Fatal("rule has no components")
			}
			if rule.Components[0].FoodCode != tt.wantFoodCode || rule.Components[0].Fraction != tt.wantComponent {
				t.Fatalf("first component = %+v, want %s at %.2f", rule.Components[0], tt.wantFoodCode, tt.wantComponent)
			}
		})
	}
}

func TestSQLiteFNDDSReferenceLookupDecompositionRuleByDescription(t *testing.T) {
	ref := openTestFNDDSReference(t)

	rule, ok, err := ref.LookupDecompositionRuleByDescription("pasta with tomato sauce and meat")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("pasta tomato meat decomposition rule did not resolve by description")
	}
	if rule.RuleID != "pasta_tomato_meat_v1" {
		t.Fatalf("RuleID = %q, want pasta_tomato_meat_v1", rule.RuleID)
	}

	if _, ok, err := ref.LookupDecompositionRuleByDescription("pasta with cream sauce and meat"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("tomato-meat rule matched cream sauce text")
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

func TestResolverGateBlocksUnproxiedBroadFallbackLookup(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 100.0
	plan := singleItemPlan("Meat", &qty, "g")

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

func TestResolverUsesApproximationProxyForCuratedGenericFood(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 28.0
	plan := singleItemPlan("Cheese", &qty, "g")

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
	if item.ResolutionMethod != "estimated" || item.ProxyFoodID != "fndds_14010000" {
		t.Fatalf("resolved = %+v, want estimated generic cheese proxy", item)
	}
}

func TestResolverUsesSourceFoodCodeApproximationForCuratedNFSFood(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 28.0
	plan := singleItemPlan("Cheese, NFS", &qty, "g")
	plan.Days[0].Meals[0].Items[0].SourceFoodCode = "14010000"

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
	if item.ResolutionMethod != "estimated" || item.ProxyFoodID != "fndds_14010000" || item.SourceFoodCode != "14010000" {
		t.Fatalf("resolved = %+v, want source-code-backed estimated cheese proxy", item)
	}
}

func TestResolverUsesGeneratedSourceFoodCodeApproximation(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 100.0
	plan := singleItemPlan("Mango, raw", &qty, "g")
	plan.Days[0].Meals[0].Items[0].SourceFoodCode = "63129010"

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
	if item.ResolutionMethod != "estimated" || item.ProxyFoodID != "fndds_63129010" || item.SourceFoodCode != "63129010" {
		t.Fatalf("resolved = %+v, want source-code-backed estimated mango proxy", item)
	}
}

func TestResolverUsesGeneratedNutApproximation(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 100.0
	plan := singleItemPlan("Peanuts, dry roasted, salted", &qty, "g")
	plan.Days[0].Meals[0].Items[0].SourceFoodCode = "42111200"

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
	if item.ResolutionMethod != "estimated" || item.ProxyFoodID != "fndds_42111200" || item.SourceFoodCode != "42111200" {
		t.Fatalf("resolved = %+v, want source-code-backed estimated peanut proxy", item)
	}
}

func TestResolverAllowsVitaminCDrinkFallback(t *testing.T) {
	ref := openTestFNDDSReference(t)
	qty := 100.0
	tests := []struct {
		name       string
		food       string
		sourceCode string
		wantFoodID string
	}{
		{
			name:       "fruit drink",
			food:       "Fruit flavored drink, with high vitamin C, powdered, reconstituted",
			sourceCode: "92542000",
			wantFoodID: "fndds_92542000",
		},
		{
			name:       "vegetable fruit juice",
			food:       "Vegetable and fruit juice, 100% juice, with high vitamin C",
			sourceCode: "78101000",
			wantFoodID: "fndds_78101000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := singleItemPlan(tt.food, &qty, "g")
			plan.Days[0].Meals[0].Items[0].SourceFoodCode = tt.sourceCode

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
			if item.ResolutionMethod != "exact" || item.FoodID != tt.wantFoodID {
				t.Fatalf("resolved = %+v, want exact %s", item, tt.wantFoodID)
			}
		})
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

func TestResolverUsesDecompositionRuleForSourceCodedComposedFood(t *testing.T) {
	ref := openTestFNDDSReference(t)
	tests := []struct {
		name           string
		food           string
		sourceCode     string
		wantFoodID     string
		wantComponents int
		wantFirstGrams float64
	}{
		{
			name:           "pasta tomato meat",
			food:           "Pasta with tomato-based sauce and meat, home recipe",
			sourceCode:     "58146322",
			wantFoodID:     "decomposed_rule_pasta_tomato_meat_v1",
			wantComponents: 3,
			wantFirstGrams: 58,
		},
		{
			name:           "tuna salad sandwich",
			food:           "Tuna salad sandwich on white",
			sourceCode:     "27550720",
			wantFoodID:     "decomposed_rule_tuna_salad_sandwich_white_v1",
			wantComponents: 2,
			wantFirstGrams: 50,
		},
		{
			name:           "burrito beef beans rice cheese",
			food:           "Burrito, beef, with beans and rice, cheese",
			sourceCode:     "58102340",
			wantFoodID:     "decomposed_rule_burrito_beef_beans_rice_cheese_v1",
			wantComponents: 5,
			wantFirstGrams: 25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qty := 100.0
			plan := Plan{
				Days: []PlanDay{
					{
						Day: 1,
						Meals: []Meal{
							{
								Name: "dinner",
								Items: []FoodItem{
									{
										Food:           tt.food,
										Quantity:       &qty,
										Unit:           "g",
										SourceFoodCode: tt.sourceCode,
									},
								},
							},
						},
					},
				},
			}

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
			if item.ResolutionMethod != "decomposed" || item.FoodID != tt.wantFoodID {
				t.Fatalf("resolved = %+v, want decomposed rule %s", item, tt.wantFoodID)
			}
			if len(item.Components) != tt.wantComponents {
				t.Fatalf("len(Components) = %d, want %d", len(item.Components), tt.wantComponents)
			}
			if diff := item.Components[0].Grams - tt.wantFirstGrams; diff < -0.0001 || diff > 0.0001 {
				t.Fatalf("first component grams = %.1f, want %.1f", item.Components[0].Grams, tt.wantFirstGrams)
			}
		})
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

func (r *recordingFNDDSReference) LookupApproximationProxyBySourceFoodCode(foodCode string) (FNDDSApproximationProxy, bool, error) {
	return FNDDSApproximationProxy{}, false, nil
}

func (r *recordingFNDDSReference) LookupDecompositionTemplate(description string) (FNDDSDecompositionTemplate, bool, error) {
	return FNDDSDecompositionTemplate{}, false, nil
}

func (r *recordingFNDDSReference) LookupDecompositionRuleBySourceFoodCode(foodCode string) (FNDDSDecompositionRule, bool, error) {
	return FNDDSDecompositionRule{}, false, nil
}

func (r *recordingFNDDSReference) LookupDecompositionRuleByDescription(description string) (FNDDSDecompositionRule, bool, error) {
	return FNDDSDecompositionRule{}, false, nil
}

func (r *recordingFNDDSReference) LookupFoodByCode(foodCode string) (CatalogFood, bool, error) {
	return CatalogFood{}, false, nil
}
