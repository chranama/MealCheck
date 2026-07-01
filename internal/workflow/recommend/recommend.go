package recommend

import "github.com/chranama/MealCheck/internal/workflow/checker"

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
