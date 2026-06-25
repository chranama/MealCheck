package checker

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSeededCaseEvaluatesToExpectedDecision(t *testing.T) {
	root := repoRoot(t)
	c, plan, catalog, err := LoadCase(root, "examples/seeded-3-day-peanut-allergy/case.json")
	if err != nil {
		t.Fatal(err)
	}

	got := Evaluate(c, plan, catalog)
	doc := got.DecisionDocument(c)
	if got.Decision != "block" {
		t.Fatalf("Decision = %q, want block", got.Decision)
	}
	if got.RiskLevel != "high" {
		t.Fatalf("RiskLevel = %q, want high", got.RiskLevel)
	}

	status := checkStatus(got.Checks)
	wantStatuses := map[string]string{
		"allergens_absent":             "block",
		"quantities_resolvable":        "block",
		"sodium_under_limit":           "warn",
		"calories_within_tolerance":    "warn",
		"prep_safety_mentions_present": "warn",
	}
	for checkID, want := range wantStatuses {
		if status[checkID] != want {
			t.Fatalf("%s status = %q, want %q", checkID, status[checkID], want)
		}
	}

	var expected DecisionDocument
	if err := readJSON(filepath.Join(root, "examples/seeded-3-day-peanut-allergy/expected-decision.json"), &expected); err != nil {
		t.Fatal(err)
	}
	if doc.Decision != expected.Decision {
		t.Fatalf("DecisionDocument.Decision = %q, want %q", doc.Decision, expected.Decision)
	}
	if !sameStringSet(doc.FailedChecks, expected.FailedChecks) {
		t.Fatalf("DecisionDocument.FailedChecks = %v, want %v", doc.FailedChecks, expected.FailedChecks)
	}

	if len(got.UnresolvedItems) != 1 {
		t.Fatalf("len(UnresolvedItems) = %d, want 1", len(got.UnresolvedItems))
	}
	unresolved := got.UnresolvedItems[0]
	if unresolved.Food != "seasoning blend" || unresolved.UnresolvedReason != "vague_quantity" {
		t.Fatalf("unexpected unresolved item: %+v", unresolved)
	}

	day2 := dailyTotal(t, got.DailyTotals, 2)
	if day2.Nutrients.SodiumMG <= float64(c.Settings.VerificationConstraints.MaxSodiumMGPerDay) {
		t.Fatalf("day 2 sodium = %.1f, want above %d", day2.Nutrients.SodiumMG, c.Settings.VerificationConstraints.MaxSodiumMGPerDay)
	}
	if day2.Nutrients.EnergyKcal <= 0 {
		t.Fatalf("day 2 energy = %.1f, want positive computed calories", day2.Nutrients.EnergyKcal)
	}
}

func TestLoadCaseRejectsLLMSuppliedNutrientTotals(t *testing.T) {
	root := repoRoot(t)
	temp := t.TempDir()
	c, _, _, err := LoadCase(root, "examples/seeded-3-day-peanut-allergy/case.json")
	if err != nil {
		t.Fatal(err)
	}

	badPlan := filepath.Join(temp, "bad-plan.json")
	if err := os.WriteFile(badPlan, []byte(`{
  "schema_version": "0.1",
  "plan_id": "bad-llm-totals",
  "nutrition_totals": {"energy_kcal": 9999},
  "days": [
    {
      "day": 1,
      "meals": [
        {
          "name": "breakfast",
          "items": [
            {"food": "cooked oatmeal", "quantity": 1, "unit": "cup"}
          ]
        }
      ]
    }
  ]
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	caseCopy := c
	caseCopy.CandidatePlan = badPlan
	if _, err := loadPlan(caseCopy.CandidatePlan); err == nil {
		t.Fatal("loadPlan accepted LLM-supplied nutrition_totals; want rejection")
	}
}

func TestLoadCaseRejectsOldProfileAndConstraintsContract(t *testing.T) {
	temp := t.TempDir()
	casePath := filepath.Join(temp, "case.json")
	if err := os.WriteFile(casePath, []byte(`{
  "schema_version": "0.1",
  "case_id": "old-contract",
  "input_mode": "prompt_generation",
  "profile": {"calorie_target_kcal": 2000, "protein_target_g": 98},
  "constraints": {"days": 1, "meals_per_day": 3},
  "guideline_pack_id": "test",
  "candidate_plan": "plan.json",
  "nutrient_catalog_path": "catalog.json",
  "expectations": {"expected_decision": "pass"}
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, _, err := LoadCase(temp, "case.json")
	if err == nil {
		t.Fatal("LoadCase accepted old profile/constraints contract")
	}
	if got := err.Error(); !strings.Contains(got, `unknown field "profile"`) {
		t.Fatalf("LoadCase error = %q, want unknown profile field", got)
	}
}

func TestLoadCaseRequiresSettingsContract(t *testing.T) {
	temp := t.TempDir()
	casePath := filepath.Join(temp, "case.json")
	if err := os.WriteFile(casePath, []byte(`{
  "schema_version": "0.1",
  "case_id": "missing-settings",
  "input_mode": "prompt_generation",
  "guideline_pack_id": "test",
  "candidate_plan": "plan.json",
  "nutrient_catalog_path": "catalog.json",
  "expectations": {"expected_decision": "pass"}
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, _, err := LoadCase(temp, "case.json")
	if err == nil {
		t.Fatal("LoadCase accepted missing settings")
	}
	if got := err.Error(); !strings.Contains(got, "settings nutrition_targets calorie_target_kcal must be positive") {
		t.Fatalf("LoadCase error = %q, want settings validation error", got)
	}
}

func TestValidateSettingsRejectsIncompleteUnresolvedPolicy(t *testing.T) {
	settings := deMinimisCase().Settings
	settings.VerificationConstraints.UnresolvedPolicy = UnresolvedPolicy{DeMinimisEnabled: true}

	err := ValidateSettings(settings)
	if err == nil {
		t.Fatal("ValidateSettings accepted enabled unresolved policy without caps")
	}
	if got := err.Error(); !strings.Contains(got, "max_item_grams must be positive") {
		t.Fatalf("ValidateSettings error = %q, want max_item_grams validation", got)
	}
}

func TestUnresolvedPolicyDefaultsToBlocking(t *testing.T) {
	c := deMinimisCase()
	plan := deMinimisPlan(1, "g")
	got := Evaluate(c, plan, deMinimisCatalog())

	if got.Decision != "block" {
		t.Fatalf("Decision = %q, want block", got.Decision)
	}
	if len(got.UnresolvedItems) != 1 {
		t.Fatalf("len(UnresolvedItems) = %d, want 1", len(got.UnresolvedItems))
	}
	if len(got.ExcludedUnresolvedItems) != 0 {
		t.Fatalf("ExcludedUnresolvedItems = %+v, want none", got.ExcludedUnresolvedItems)
	}
	if checkStatus(got.Checks)["quantities_resolvable"] != "block" {
		t.Fatalf("quantities_resolvable status = %q, want block", checkStatus(got.Checks)["quantities_resolvable"])
	}
}

func TestUnresolvedPolicyExcludesDeMinimisMassAsWarn(t *testing.T) {
	c := deMinimisCase()
	c.Settings.VerificationConstraints.UnresolvedPolicy = UnresolvedPolicy{
		DeMinimisEnabled:    true,
		MaxItemGrams:        2,
		MaxTotalGramsPerDay: 5,
		MaxItemsPerDay:      3,
	}
	plan := deMinimisPlan(1, "g")
	got := Evaluate(c, plan, deMinimisCatalog())

	if got.Decision != "warn" {
		t.Fatalf("Decision = %q, want warn", got.Decision)
	}
	if len(got.UnresolvedItems) != 0 {
		t.Fatalf("UnresolvedItems = %+v, want none", got.UnresolvedItems)
	}
	if len(got.ExcludedUnresolvedItems) != 1 {
		t.Fatalf("len(ExcludedUnresolvedItems) = %d, want 1", len(got.ExcludedUnresolvedItems))
	}
	excluded := got.ExcludedUnresolvedItems[0]
	if excluded.Food != "sumac" || excluded.DeterministicGrams != 1 || excluded.PolicyID != "de_minimis_unresolved_v1" {
		t.Fatalf("unexpected excluded item: %+v", excluded)
	}
	if checkStatus(got.Checks)["quantities_resolvable"] != "warn" {
		t.Fatalf("quantities_resolvable status = %q, want warn", checkStatus(got.Checks)["quantities_resolvable"])
	}
	day1 := dailyTotal(t, got.DailyTotals, 1)
	if day1.Nutrients.EnergyKcal != 10 {
		t.Fatalf("day 1 energy = %.1f, want only resolved catalog item energy", day1.Nutrients.EnergyKcal)
	}
}

func TestUnresolvedPolicyBlocksItemsOverCap(t *testing.T) {
	c := deMinimisCase()
	c.Settings.VerificationConstraints.UnresolvedPolicy = UnresolvedPolicy{
		DeMinimisEnabled:    true,
		MaxItemGrams:        2,
		MaxTotalGramsPerDay: 5,
		MaxItemsPerDay:      3,
	}
	got := Evaluate(c, deMinimisPlan(3, "g"), deMinimisCatalog())

	if got.Decision != "block" {
		t.Fatalf("Decision = %q, want block", got.Decision)
	}
	if len(got.UnresolvedItems) != 1 || len(got.ExcludedUnresolvedItems) != 0 {
		t.Fatalf("unresolved = %+v excluded = %+v, want one blocking unresolved", got.UnresolvedItems, got.ExcludedUnresolvedItems)
	}
}

func TestUnresolvedPolicyBlocksDayCapOverflow(t *testing.T) {
	c := deMinimisCase()
	c.Settings.VerificationConstraints.UnresolvedPolicy = UnresolvedPolicy{
		DeMinimisEnabled:    true,
		MaxItemGrams:        2,
		MaxTotalGramsPerDay: 1.5,
		MaxItemsPerDay:      3,
	}
	plan := deMinimisPlan(1, "g")
	qty := 1.0
	plan.Days[0].Meals[0].Items = append(plan.Days[0].Meals[0].Items, FoodItem{Food: "zaatar", Quantity: &qty, Unit: "g"})

	got := Evaluate(c, plan, deMinimisCatalog())
	if got.Decision != "block" {
		t.Fatalf("Decision = %q, want block", got.Decision)
	}
	if len(got.UnresolvedItems) != 2 || len(got.ExcludedUnresolvedItems) != 0 {
		t.Fatalf("unresolved = %+v excluded = %+v, want both day candidates blocking", got.UnresolvedItems, got.ExcludedUnresolvedItems)
	}
}

func TestUnresolvedPolicyBlocksItemCountOverflow(t *testing.T) {
	c := deMinimisCase()
	c.Settings.VerificationConstraints.UnresolvedPolicy = UnresolvedPolicy{
		DeMinimisEnabled:    true,
		MaxItemGrams:        2,
		MaxTotalGramsPerDay: 5,
		MaxItemsPerDay:      1,
	}
	plan := deMinimisPlan(1, "g")
	qty := 1.0
	plan.Days[0].Meals[0].Items = append(plan.Days[0].Meals[0].Items, FoodItem{Food: "zaatar", Quantity: &qty, Unit: "g"})

	got := Evaluate(c, plan, deMinimisCatalog())
	if got.Decision != "block" {
		t.Fatalf("Decision = %q, want block", got.Decision)
	}
	if len(got.UnresolvedItems) != 2 || len(got.ExcludedUnresolvedItems) != 0 {
		t.Fatalf("unresolved = %+v excluded = %+v, want item-count overflow to keep both blocking", got.UnresolvedItems, got.ExcludedUnresolvedItems)
	}
}

func TestUnresolvedPolicyDoesNotExcludeVagueQuantities(t *testing.T) {
	c := deMinimisCase()
	c.Settings.VerificationConstraints.UnresolvedPolicy = UnresolvedPolicy{
		DeMinimisEnabled:    true,
		MaxItemGrams:        2,
		MaxTotalGramsPerDay: 5,
		MaxItemsPerDay:      3,
	}
	plan := deMinimisPlan(1, "g")
	plan.Days[0].Meals[0].Items[1] = FoodItem{Food: "sumac", QuantityText: "a pinch", ResolutionStatus: "unresolved", UnresolvedReason: "vague_quantity"}

	got := Evaluate(c, plan, deMinimisCatalog())
	if got.Decision != "block" {
		t.Fatalf("Decision = %q, want block", got.Decision)
	}
	if len(got.UnresolvedItems) != 1 || got.UnresolvedItems[0].UnresolvedReason != "vague_quantity" {
		t.Fatalf("UnresolvedItems = %+v, want vague quantity blocking", got.UnresolvedItems)
	}
	if len(got.ExcludedUnresolvedItems) != 0 {
		t.Fatalf("ExcludedUnresolvedItems = %+v, want none", got.ExcludedUnresolvedItems)
	}
}

func TestUnresolvedPolicyDisabledWhenAllergiesConfigured(t *testing.T) {
	c := deMinimisCase()
	c.Settings.VerificationConstraints.Allergies = []string{"peanuts"}
	c.Settings.VerificationConstraints.UnresolvedPolicy = UnresolvedPolicy{
		DeMinimisEnabled:    true,
		MaxItemGrams:        2,
		MaxTotalGramsPerDay: 5,
		MaxItemsPerDay:      3,
	}
	got := Evaluate(c, deMinimisPlan(1, "g"), deMinimisCatalog())

	if got.Decision != "block" {
		t.Fatalf("Decision = %q, want block", got.Decision)
	}
	if len(got.UnresolvedItems) != 1 || len(got.ExcludedUnresolvedItems) != 0 {
		t.Fatalf("unresolved = %+v excluded = %+v, want allergy context to block", got.UnresolvedItems, got.ExcludedUnresolvedItems)
	}
}

func deMinimisCase() Case {
	return Case{
		CaseID: "de-minimis-test",
		Settings: Settings{
			NutritionTargets: NutritionTargets{
				CalorieTargetKcal: 2000,
				ProteinTargetG:    1,
			},
			VerificationConstraints: VerificationConstraints{
				Days:                       1,
				MealsPerDay:                1,
				MaxSodiumMGPerDay:          0,
				MaxAddedSugarGPerMeal:      0,
				MaxSaturatedFatPctCalories: 0,
				CalorieTolerancePct:        0,
			},
		},
	}
}

func deMinimisPlan(quantity float64, unit string) Plan {
	return Plan{
		Days: []PlanDay{
			{
				Day: 1,
				Meals: []Meal{
					{
						Name: "breakfast",
						Items: []FoodItem{
							{Food: "broccoli", Quantity: ptrFloat(100), Unit: "g"},
							{Food: "sumac", Quantity: &quantity, Unit: unit},
						},
					},
				},
			},
		},
	}
}

func deMinimisCatalog() NutrientCatalog {
	return NutrientCatalog{
		Foods: []CatalogFood{
			{
				FoodID:          "broccoli",
				Name:            "broccoli",
				BaseQuantityG:   100,
				UnitConversions: map[string]float64{"g": 1},
				NutrientsPer100G: Nutrients{
					EnergyKcal: 10,
					ProteinG:   2,
				},
				FoodGroups: []string{"vegetables"},
			},
		},
	}
}

func ptrFloat(value float64) *float64 {
	return &value
}

func loadPlan(path string) (Plan, error) {
	var plan Plan
	err := readJSON(path, &plan)
	return plan, err
}

func checkStatus(checks []CheckResult) map[string]string {
	result := map[string]string{}
	for _, check := range checks {
		result[check.CheckID] = check.Status
	}
	return result
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := map[string]int{}
	for _, value := range left {
		seen[value]++
	}
	for _, value := range right {
		seen[value]--
		if seen[value] < 0 {
			return false
		}
	}
	return true
}

func dailyTotal(t *testing.T, totals []DailyTotal, day int) DailyTotal {
	t.Helper()
	for _, total := range totals {
		if total.Day == day {
			return total
		}
	}
	t.Fatalf("missing daily total for day %d", day)
	return DailyTotal{}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
