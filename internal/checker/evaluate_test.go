package checker

import (
	"os"
	"path/filepath"
	"runtime"
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
	if day2.Nutrients.SodiumMG <= float64(c.Constraints.MaxSodiumMGPerDay) {
		t.Fatalf("day 2 sodium = %.1f, want above %d", day2.Nutrients.SodiumMG, c.Constraints.MaxSodiumMGPerDay)
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
