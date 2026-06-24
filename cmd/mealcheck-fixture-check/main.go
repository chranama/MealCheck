package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

type validationTarget struct {
	schemaPath   string
	instancePath string
}

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()

	if err := run(*root); err != nil {
		fmt.Fprintf(os.Stderr, "fixture check failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("fixture check passed")
}

func run(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	targets := []validationTarget{
		{"schemas/case.schema.json", "examples/seeded-3-day-peanut-allergy/case.json"},
		{"schemas/meal-plan.schema.json", "examples/seeded-3-day-peanut-allergy/plans/baseline.json"},
		{"schemas/meal-plan.schema.json", "examples/seeded-3-day-peanut-allergy/plans/candidate.json"},
		{"schemas/source-registry.schema.json", "data/guidelines/dga-2025-2030-us-adult-general-v1/source-registry.json"},
		{"schemas/guideline-pack.schema.json", "data/guidelines/dga-2025-2030-us-adult-general-v1/guideline-pack.json"},
		{"schemas/nutrient-catalog.schema.json", "data/nutrients/fixture-catalog-v1.json"},
		{"schemas/decision.schema.json", "examples/seeded-3-day-peanut-allergy/expected-decision.json"},
		{"schemas/decision.schema.json", "examples/seeded-3-day-peanut-allergy/artifacts/demo-runs/seeded-3-day-peanut-allergy/decision.json"},
		{"schemas/report.schema.json", "examples/seeded-3-day-peanut-allergy/artifacts/demo-runs/seeded-3-day-peanut-allergy/report.json"},
	}

	for _, target := range targets {
		if err := validateAgainstSchema(root, target); err != nil {
			return err
		}
	}

	if err := validateCaseLinks(root); err != nil {
		return err
	}

	if err := validateGuidelineReferences(root); err != nil {
		return err
	}

	if err := validateNutrientCatalogQuality(root); err != nil {
		return err
	}

	if err := validateExpectedDecision(root); err != nil {
		return err
	}

	if err := validateStaticDemo(root); err != nil {
		return err
	}

	if err := validateEvaluationDataset(root); err != nil {
		return err
	}

	return nil
}

func validateAgainstSchema(root string, target validationTarget) error {
	schemaFile := filepath.Join(root, target.schemaPath)
	instanceFile := filepath.Join(root, target.instancePath)

	var schema jsonschema.Schema
	if err := readJSON(schemaFile, &schema); err != nil {
		return fmt.Errorf("read schema %s: %w", target.schemaPath, err)
	}

	resolved, err := schema.Resolve(nil)
	if err != nil {
		return fmt.Errorf("resolve schema %s: %w", target.schemaPath, err)
	}

	var instance any
	if err := readJSON(instanceFile, &instance); err != nil {
		return fmt.Errorf("read instance %s: %w", target.instancePath, err)
	}

	if err := resolved.Validate(instance); err != nil {
		return fmt.Errorf("validate %s against %s: %w", target.instancePath, target.schemaPath, err)
	}

	return nil
}

func validateCaseLinks(root string) error {
	caseDoc, err := readObject(filepath.Join(root, "examples/seeded-3-day-peanut-allergy/case.json"))
	if err != nil {
		return err
	}

	pathFields := []string{
		"guideline_pack_path",
		"nutrient_catalog_path",
		"baseline_plan",
		"candidate_plan",
	}
	for _, field := range pathFields {
		path, err := stringField(caseDoc, field)
		if err != nil {
			return err
		}
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			return fmt.Errorf("case field %s points to missing path %q: %w", field, path, err)
		}
	}

	pack, err := readObject(filepath.Join(root, mustString(caseDoc, "guideline_pack_path")))
	if err != nil {
		return err
	}
	catalog, err := readObject(filepath.Join(root, mustString(caseDoc, "nutrient_catalog_path")))
	if err != nil {
		return err
	}

	if err := fieldMatches(caseDoc, "guideline_pack_id", pack, "pack_id"); err != nil {
		return err
	}
	if err := fieldMatches(caseDoc, "nutrient_catalog_id", catalog, "catalog_id"); err != nil {
		return err
	}

	return nil
}

func validateGuidelineReferences(root string) error {
	pack, err := readObject(filepath.Join(root, "data/guidelines/dga-2025-2030-us-adult-general-v1/guideline-pack.json"))
	if err != nil {
		return err
	}
	registry, err := readObject(filepath.Join(root, "data/guidelines/dga-2025-2030-us-adult-general-v1/source-registry.json"))
	if err != nil {
		return err
	}

	sourceIDs := map[string]bool{}
	for _, source := range objectSlice(pack, "source_documents") {
		sourceIDs[mustString(source, "source_id")] = true
	}

	claimIDs := map[string]bool{}
	for _, source := range objectSlice(registry, "sources") {
		for _, claim := range objectSlice(source, "claims_used") {
			claimIDs[mustString(claim, "claim_id")] = true
		}
	}

	for _, rule := range objectSlice(pack, "rules") {
		ruleID := mustString(rule, "rule_id")
		for _, ref := range stringSlice(rule, "source_refs") {
			if !sourceIDs[ref] {
				return fmt.Errorf("rule %s references unknown source %s", ruleID, ref)
			}
		}
		for _, claim := range stringSlice(rule, "source_claims") {
			if !claimIDs[claim] {
				return fmt.Errorf("rule %s references unknown source claim %s", ruleID, claim)
			}
		}
		if len(stringSlice(rule, "source_refs")) > 0 && len(stringSlice(rule, "source_claims")) == 0 {
			return fmt.Errorf("rule %s has source_refs but no source_claims", ruleID)
		}
	}

	return nil
}

func validateExpectedDecision(root string) error {
	caseDoc, err := readObject(filepath.Join(root, "examples/seeded-3-day-peanut-allergy/case.json"))
	if err != nil {
		return err
	}
	expected, err := readObject(filepath.Join(root, "examples/seeded-3-day-peanut-allergy/expected-decision.json"))
	if err != nil {
		return err
	}

	expectations, ok := caseDoc["expectations"].(map[string]any)
	if !ok {
		return errors.New("case expectations must be an object")
	}

	expectedDecision := mustString(expectations, "expected_decision")
	if got := mustString(expected, "decision"); got != expectedDecision {
		return fmt.Errorf("expected-decision decision %q does not match case expectation %q", got, expectedDecision)
	}

	checkIDs := map[string]bool{}
	for _, check := range objectSlice(expected, "checks") {
		checkIDs[mustString(check, "check_id")] = true
	}

	for _, checkID := range stringSlice(expectations, "expected_block_checks") {
		if !checkIDs[checkID] {
			return fmt.Errorf("expected block check %s is missing from expected-decision checks", checkID)
		}
	}
	for _, checkID := range stringSlice(expectations, "expected_warn_checks") {
		if !checkIDs[checkID] {
			return fmt.Errorf("expected warn check %s is missing from expected-decision checks", checkID)
		}
	}

	candidate, err := readObject(filepath.Join(root, mustString(caseDoc, "candidate_plan")))
	if err != nil {
		return err
	}
	if !planContainsFood(candidate, "peanut sauce") {
		return errors.New("candidate fixture must include peanut sauce for the allergen block")
	}
	if !planContainsUnresolvedReason(candidate, "vague_quantity") {
		return errors.New("candidate fixture must include a vague quantity unresolved item")
	}

	return nil
}

func validateStaticDemo(root string) error {
	index, err := readObject(filepath.Join(root, "examples/seeded-3-day-peanut-allergy/artifacts/demo-runs/index.json"))
	if err != nil {
		return err
	}
	demos := objectSlice(index, "demo_runs")
	if len(demos) != 1 {
		return fmt.Errorf("ui demo index should contain exactly one seeded demo, got %d", len(demos))
	}

	demo := demos[0]
	if id := mustString(demo, "id"); id != "seeded-3-day-peanut-allergy" {
		return fmt.Errorf("unexpected UI demo id %q", id)
	}
	basePath := mustString(demo, "base_path")
	repoBasePath := filepath.Join("examples", "seeded-3-day-peanut-allergy", "artifacts", basePath)
	requiredFiles := []string{
		"decision.json",
		"report.json",
		"daily-totals.json",
		"resolved-foods.json",
		"unresolved-foods.json",
		"manifest.json",
		"guideline-pack/pack.json",
		"guideline-pack/citations.json",
	}
	for _, file := range requiredFiles {
		if _, err := os.Stat(filepath.Join(root, repoBasePath, file)); err != nil {
			return fmt.Errorf("static demo is missing %s: %w", filepath.Join(basePath, file), err)
		}
	}

	decision, err := readObject(filepath.Join(root, repoBasePath, "decision.json"))
	if err != nil {
		return err
	}
	if got := mustString(decision, "decision"); got != "block" {
		return fmt.Errorf("static demo decision = %q, want block", got)
	}

	manifest, err := readObject(filepath.Join(root, repoBasePath, "manifest.json"))
	if err != nil {
		return err
	}
	if got := mustString(manifest, "mode"); got != "validate" {
		return fmt.Errorf("static demo manifest mode = %q, want validate", got)
	}
	if len(stringSlice(manifest, "artifacts")) == 0 {
		return errors.New("static demo manifest must list artifacts")
	}

	return nil
}

func validateNutrientCatalogQuality(root string) error {
	catalog, err := readObject(filepath.Join(root, "data/nutrients/fixture-catalog-v1.json"))
	if err != nil {
		return err
	}
	foods := objectSlice(catalog, "foods")
	if len(foods) < 100 {
		return fmt.Errorf("fixture nutrient catalog has %d foods, want at least 100", len(foods))
	}

	allowedAllergens := map[string]bool{
		"milk": true, "eggs": true, "fish": true, "crustacean shellfish": true,
		"tree nuts": true, "peanuts": true, "wheat": true, "soybeans": true, "sesame": true,
	}
	allowedGroups := map[string]bool{
		"beverages": true, "condiments": true, "dairy": true, "fats": true, "fruits": true,
		"protein": true, "refined_grains": true, "snacks": true, "vegetables": true, "whole_grains": true,
	}
	labels := map[string]string{}
	ids := map[string]bool{}
	for _, food := range foods {
		foodID := mustString(food, "food_id")
		if ids[foodID] {
			return fmt.Errorf("duplicate catalog food_id %s", foodID)
		}
		ids[foodID] = true

		name := mustString(food, "name")
		if err := registerCatalogLabel(labels, foodID, name); err != nil {
			return err
		}
		for _, alias := range stringSlice(food, "aliases") {
			if err := registerCatalogLabel(labels, foodID, alias); err != nil {
				return err
			}
		}

		conversions, ok := food["unit_conversions"].(map[string]any)
		if !ok || len(conversions) == 0 {
			return fmt.Errorf("catalog food %s must define unit_conversions", foodID)
		}
		if grams, ok := conversions["g"].(float64); !ok || grams <= 0 {
			return fmt.Errorf("catalog food %s must define a positive g conversion", foodID)
		}
		for unit, value := range conversions {
			number, ok := value.(float64)
			if !ok || number <= 0 {
				return fmt.Errorf("catalog food %s has invalid conversion for %s", foodID, unit)
			}
		}

		groups := stringSlice(food, "food_groups")
		if len(groups) == 0 {
			return fmt.Errorf("catalog food %s must define at least one food group", foodID)
		}
		for _, group := range groups {
			if !allowedGroups[group] {
				return fmt.Errorf("catalog food %s has invalid food group %s", foodID, group)
			}
		}
		for _, allergen := range stringSlice(food, "allergens") {
			if !allowedAllergens[allergen] {
				return fmt.Errorf("catalog food %s has invalid allergen %s", foodID, allergen)
			}
		}
		for _, source := range objectSlice(food, "source_refs") {
			if _, err := stringField(source, "source"); err != nil {
				return fmt.Errorf("catalog food %s has invalid source_refs: %w", foodID, err)
			}
		}
	}
	return nil
}

func registerCatalogLabel(labels map[string]string, foodID, label string) error {
	key := normalizeLabel(label)
	if key == "" {
		return fmt.Errorf("catalog food %s has an empty name or alias", foodID)
	}
	if existing := labels[key]; existing != "" && existing != foodID {
		return fmt.Errorf("catalog label %q is used by both %s and %s", label, existing, foodID)
	}
	labels[key] = foodID
	return nil
}

func validateEvaluationDataset(root string) error {
	if err := validateEvaluationDatasetFile(root, "data/evaluation/fndds-grounded-meal-plans-v1.json", "fndds-grounded-meal-plans-v1", []string{
		"balanced_common", "vegetarian", "vegan", "high_sodium", "high_added_sugar",
		"allergen_risk", "low_protein", "long_tail_unresolved", "vague_quantity",
	}); err != nil {
		return err
	}

	if err := validateEvaluationDatasetFile(root, "data/evaluation/wweia-nhanes-real-recalls-v1.json", "wweia-nhanes-real-recalls-v1", []string{
		"wweia_common_recall_day", "wweia_high_sodium", "wweia_low_protein", "wweia_resolved_eating_occasion",
	}); err != nil {
		return err
	}
	return nil
}

func validateEvaluationDatasetFile(root, datasetPath, datasetID string, requiredCategories []string) error {
	dataset, err := readObject(filepath.Join(root, datasetPath))
	if err != nil {
		return err
	}
	if got := mustString(dataset, "schema_version"); got != "0.1" {
		return fmt.Errorf("%s schema_version = %q, want 0.1", datasetPath, got)
	}
	if got := mustString(dataset, "dataset_id"); got != datasetID {
		return fmt.Errorf("%s dataset_id = %q, want %s", datasetPath, got, datasetID)
	}
	if got := mustString(dataset, "catalog_path"); got != "data/nutrients/fixture-catalog-v1.json" {
		return fmt.Errorf("%s catalog_path = %q, want data/nutrients/fixture-catalog-v1.json", datasetPath, got)
	}
	if len(objectSlice(dataset, "source_refs")) == 0 {
		return fmt.Errorf("%s must define source_refs", datasetPath)
	}
	cases := objectSlice(dataset, "cases")
	if len(cases) != 100 {
		return fmt.Errorf("%s has %d cases, want 100", datasetPath, len(cases))
	}

	seenCaseIDs := map[string]bool{}
	categories := map[string]int{}
	for _, c := range cases {
		caseID := mustString(c, "case_id")
		if seenCaseIDs[caseID] {
			return fmt.Errorf("%s has duplicate case_id %s", datasetPath, caseID)
		}
		seenCaseIDs[caseID] = true
		category := mustString(c, "category")
		categories[category]++
		if _, err := stringField(c, "source_text"); err != nil {
			return fmt.Errorf("%s case %s: %w", datasetPath, caseID, err)
		}
		if datasetID == "wweia-nhanes-real-recalls-v1" && len(objectSlice(c, "source_refs")) == 0 {
			return fmt.Errorf("%s case %s must define source_refs", datasetPath, caseID)
		}
		if datasetID == "wweia-nhanes-real-recalls-v1" {
			metrics, ok := c["source_metrics"].(map[string]any)
			if !ok {
				return fmt.Errorf("%s case %s must define source_metrics", datasetPath, caseID)
			}
			if _, err := numericField(metrics, "food_items"); err != nil {
				return fmt.Errorf("%s case %s source_metrics: %w", datasetPath, caseID, err)
			}
			if _, err := numericField(metrics, "known_local_items"); err != nil {
				return fmt.Errorf("%s case %s source_metrics: %w", datasetPath, caseID, err)
			}
		}
		expected, ok := c["expected"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s case %s must define expected outcomes", datasetPath, caseID)
		}
		if _, ok := expected["unresolved_count"].(float64); !ok {
			return fmt.Errorf("%s case %s expected.unresolved_count must be present", datasetPath, caseID)
		}
		plan, ok := c["plan"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s case %s must define a plan", datasetPath, caseID)
		}
		if !planHasAnyFood(plan) {
			return fmt.Errorf("%s case %s plan must contain food items", datasetPath, caseID)
		}
	}

	for _, category := range requiredCategories {
		if categories[category] == 0 {
			return fmt.Errorf("%s is missing category %s", datasetPath, category)
		}
	}
	return nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return err
	}
	return nil
}

func readObject(path string) (map[string]any, error) {
	var doc map[string]any
	if err := readJSON(path, &doc); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return doc, nil
}

func stringField(doc map[string]any, field string) (string, error) {
	value, ok := doc[field].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("field %s must be a non-empty string", field)
	}
	return value, nil
}

func numericField(doc map[string]any, field string) (float64, error) {
	value, ok := doc[field].(float64)
	if !ok {
		return 0, fmt.Errorf("field %s must be a number", field)
	}
	return value, nil
}

func mustString(doc map[string]any, field string) string {
	value, ok := doc[field].(string)
	if !ok {
		panic(fmt.Sprintf("field %s must be a string", field))
	}
	return value
}

func fieldMatches(left map[string]any, leftField string, right map[string]any, rightField string) error {
	leftValue, err := stringField(left, leftField)
	if err != nil {
		return err
	}
	rightValue, err := stringField(right, rightField)
	if err != nil {
		return err
	}
	if leftValue != rightValue {
		return fmt.Errorf("%s %q does not match %s %q", leftField, leftValue, rightField, rightValue)
	}
	return nil
}

func objectSlice(doc map[string]any, field string) []map[string]any {
	values, _ := doc[field].([]any)
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if object, ok := value.(map[string]any); ok {
			result = append(result, object)
		}
	}
	return result
}

func stringSlice(doc map[string]any, field string) []string {
	values, _ := doc[field].([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if s, ok := value.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func planContainsFood(plan map[string]any, food string) bool {
	return walkFoodItems(plan, func(item map[string]any) bool {
		return strings.EqualFold(mustString(item, "food"), food)
	})
}

func planContainsUnresolvedReason(plan map[string]any, reason string) bool {
	return walkFoodItems(plan, func(item map[string]any) bool {
		value, ok := item["unresolved_reason"].(string)
		return ok && value == reason
	})
}

func walkFoodItems(plan map[string]any, predicate func(map[string]any) bool) bool {
	for _, day := range objectSlice(plan, "days") {
		for _, meal := range objectSlice(day, "meals") {
			for _, item := range objectSlice(meal, "items") {
				if predicate(item) {
					return true
				}
			}
		}
	}
	return false
}

func planHasAnyFood(plan map[string]any) bool {
	return walkFoodItems(plan, func(item map[string]any) bool {
		_, ok := item["food"].(string)
		return ok
	})
}

func normalizeLabel(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}
