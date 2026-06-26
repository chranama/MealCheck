package fixturecheck

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	_ "modernc.org/sqlite"
)

type validationTarget struct {
	schemaPath   string
	instancePath string
}

func Run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("fixture-check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if err := run(*root); err != nil {
		fmt.Fprintf(stderr, "fixture check failed: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "fixture check passed")
	return 0
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

	if err := validateFNDDSReference(root); err != nil {
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
		"excluded-unresolved-foods.json",
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

func validateFNDDSReference(root string) error {
	base := filepath.Join(root, "data/reference/fndds-2021-2023")
	requiredFiles := []string{
		"foods.jsonl",
		"nutrients.jsonl",
		"portions.jsonl",
		"resolver-candidates.jsonl",
		"quarantined-foods.jsonl",
		"review-required-foods.jsonl",
		"food-index.json",
		"classification-summary.json",
		"manifest.json",
		"fndds.sqlite",
	}
	for _, file := range requiredFiles {
		if _, err := os.Stat(filepath.Join(base, file)); err != nil {
			return fmt.Errorf("FNDDS reference is missing %s: %w", file, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "data/reference/fndds/source-manifest.json")); err != nil {
		return fmt.Errorf("FNDDS source manifest missing: %w", err)
	}

	foods, err := readJSONLObjects(filepath.Join(base, "foods.jsonl"))
	if err != nil {
		return err
	}
	nutrients, err := readJSONLObjects(filepath.Join(base, "nutrients.jsonl"))
	if err != nil {
		return err
	}
	portions, err := readJSONLObjects(filepath.Join(base, "portions.jsonl"))
	if err != nil {
		return err
	}
	candidates, err := readJSONLObjects(filepath.Join(base, "resolver-candidates.jsonl"))
	if err != nil {
		return err
	}
	quarantined, err := readJSONLObjects(filepath.Join(base, "quarantined-foods.jsonl"))
	if err != nil {
		return err
	}
	reviewRequired, err := readJSONLObjects(filepath.Join(base, "review-required-foods.jsonl"))
	if err != nil {
		return err
	}
	summary, err := readObject(filepath.Join(base, "classification-summary.json"))
	if err != nil {
		return err
	}
	index, err := readObject(filepath.Join(base, "food-index.json"))
	if err != nil {
		return err
	}

	if len(foods) < 5000 {
		return fmt.Errorf("FNDDS reference has %d foods, want at least 5000", len(foods))
	}
	if len(nutrients) != len(foods) {
		return fmt.Errorf("FNDDS nutrients has %d rows, want %d", len(nutrients), len(foods))
	}
	if len(portions) < len(foods) {
		return fmt.Errorf("FNDDS portions has %d rows, want at least %d", len(portions), len(foods))
	}
	if got := int(mustNumber(summary, "food_count")); got != len(foods) {
		return fmt.Errorf("FNDDS summary food_count = %d, want %d", got, len(foods))
	}
	if got := int(mustNumber(summary, "resolver_candidate_count")); got != len(candidates) {
		return fmt.Errorf("FNDDS summary resolver_candidate_count = %d, want %d", got, len(candidates))
	}
	if got := int(mustNumber(summary, "quarantined_count")); got != len(quarantined) {
		return fmt.Errorf("FNDDS summary quarantined_count = %d, want %d", got, len(quarantined))
	}
	if got := int(mustNumber(summary, "review_required_count")); got != len(reviewRequired) {
		return fmt.Errorf("FNDDS summary review_required_count = %d, want %d", got, len(reviewRequired))
	}
	if got := int(mustNumber(index, "food_count")); got != len(foods) {
		return fmt.Errorf("FNDDS food-index food_count = %d, want %d", got, len(foods))
	}

	validStatuses := map[string]bool{
		"eligible_specific":               true,
		"eligible_generic":                true,
		"review_required":                 true,
		"quarantined_ambiguous":           true,
		"quarantined_mixed_dish":          true,
		"quarantined_restaurant_or_brand": true,
		"quarantined_preparation_unclear": true,
	}
	validFlags := map[string]bool{
		"nfs": true, "not_further_specified": true, "not_specified_as_to": true,
		"generic_other": true, "generic_name": true, "mixed_dish": true,
		"sandwich": true, "pizza": true, "burrito": true, "taco": true,
		"casserole": true, "soup_or_stew": true, "restaurant_or_fast_food": true,
		"home_recipe": true, "brand_or_product_style": true, "preparation_unclear": true,
		"added_fat_unspecified": true, "multi_component_allergen_risk": true,
		"missing_required_nutrients": true, "missing_portion_data": true,
	}
	hardQuarantineFlags := map[string]bool{
		"nfs": true, "not_further_specified": true, "not_specified_as_to": true,
		"generic_other": true, "mixed_dish": true, "sandwich": true,
		"pizza": true, "burrito": true, "taco": true, "casserole": true,
		"soup_or_stew": true, "restaurant_or_fast_food": true,
		"home_recipe": true, "brand_or_product_style": true,
		"preparation_unclear": true, "added_fat_unspecified": true,
		"multi_component_allergen_risk": true, "missing_required_nutrients": true,
	}

	foodByCode := map[string]map[string]any{}
	for _, food := range foods {
		code := mustString(food, "food_code")
		if foodByCode[code] != nil {
			return fmt.Errorf("duplicate FNDDS food_code %s", code)
		}
		foodByCode[code] = food
		if got := mustString(food, "release"); got != "2021-2023" {
			return fmt.Errorf("FNDDS food %s release = %q, want 2021-2023", code, got)
		}
		if _, err := stringField(food, "main_description"); err != nil {
			return fmt.Errorf("FNDDS food %s: %w", code, err)
		}
		status := mustString(food, "candidate_status")
		if !validStatuses[status] {
			return fmt.Errorf("FNDDS food %s has invalid candidate_status %s", code, status)
		}
		for _, flag := range stringSlice(food, "ambiguity_flags") {
			if !validFlags[flag] {
				return fmt.Errorf("FNDDS food %s has invalid ambiguity flag %s", code, flag)
			}
			if isEligibleStatus(status) && hardQuarantineFlags[flag] {
				return fmt.Errorf("FNDDS eligible food %s has hard quarantine flag %s", code, flag)
			}
		}
		if len(objectSlice(food, "source_refs")) == 0 {
			return fmt.Errorf("FNDDS food %s must define source_refs", code)
		}
		if err := validateReferenceNutrients(code, food, isEligibleStatus(status)); err != nil {
			return err
		}
		for _, portion := range objectSlice(food, "portion_options") {
			if _, err := stringField(portion, "description"); err != nil {
				return fmt.Errorf("FNDDS food %s portion: %w", code, err)
			}
			grams, err := numericField(portion, "grams")
			if err != nil || grams <= 0 {
				return fmt.Errorf("FNDDS food %s has invalid portion grams", code)
			}
		}
	}

	if err := validateReferenceSplit("resolver-candidates.jsonl", candidates, foodByCode, func(status string) bool { return isEligibleStatus(status) }); err != nil {
		return err
	}
	if err := validateReferenceSplit("quarantined-foods.jsonl", quarantined, foodByCode, func(status string) bool { return strings.HasPrefix(status, "quarantined_") }); err != nil {
		return err
	}
	if err := validateReferenceSplit("review-required-foods.jsonl", reviewRequired, foodByCode, func(status string) bool { return status == "review_required" }); err != nil {
		return err
	}

	expectedStatuses := map[string]string{
		"11000000": "review_required",
		"11100000": "quarantined_ambiguous",
		"94000100": "eligible_generic",
		"94100100": "eligible_generic",
	}
	for code, want := range expectedStatuses {
		food := foodByCode[code]
		if food == nil {
			return fmt.Errorf("FNDDS reference missing known example %s", code)
		}
		if got := mustString(food, "candidate_status"); got != want {
			return fmt.Errorf("FNDDS food %s candidate_status = %s, want %s", code, got, want)
		}
	}
	if err := validateFNDDSReferenceSQLite(base, foods, nutrients, portions); err != nil {
		return err
	}
	return nil
}

func validateFNDDSReferenceSQLite(base string, foods, nutrients, portions []map[string]any) error {
	sqlitePath := filepath.Join(base, "fndds.sqlite")
	abs, err := filepath.Abs(sqlitePath)
	if err != nil {
		return err
	}
	uri := (&url.URL{Scheme: "file", Path: abs, RawQuery: "mode=ro&immutable=1"}).String()
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return fmt.Errorf("open FNDDS SQLite reference: %w", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return fmt.Errorf("open FNDDS SQLite reference: %w", err)
	}

	flagCount := 0
	allergenCount := 0
	foodGroupCount := 0
	for _, food := range foods {
		flagCount += len(stringSlice(food, "ambiguity_flags"))
		allergenCount += len(stringSlice(food, "allergens"))
		foodGroupCount += len(stringSlice(food, "food_groups"))
	}

	expectedCounts := map[string]int{
		"fndds_foods":           len(foods),
		"fndds_nutrients":       len(nutrients),
		"fndds_portions":        len(portions),
		"fndds_ambiguity_flags": flagCount,
		"fndds_allergens":       allergenCount,
		"fndds_food_groups":     foodGroupCount,
	}
	for table, want := range expectedCounts {
		got, err := sqliteCount(db, fmt.Sprintf("select count(*) from %s", table))
		if err != nil {
			return fmt.Errorf("count FNDDS SQLite table %s: %w", table, err)
		}
		if got != want {
			return fmt.Errorf("FNDDS SQLite table %s has %d rows, want %d", table, got, want)
		}
	}

	expectedStatuses := map[string]string{
		"11000000": "review_required",
		"11100000": "quarantined_ambiguous",
		"94000100": "eligible_generic",
	}
	for code, want := range expectedStatuses {
		got, err := sqliteString(db, "select candidate_status from fndds_foods where food_code = ?", code)
		if err != nil {
			return fmt.Errorf("read FNDDS SQLite food %s status: %w", code, err)
		}
		if got != want {
			return fmt.Errorf("FNDDS SQLite food %s candidate_status = %s, want %s", code, got, want)
		}
	}

	resolverExamples := map[string]int{
		"water, tap":  1,
		"milk, nfs":   0,
		"milk, human": 0,
	}
	for description, want := range resolverExamples {
		got, err := sqliteCount(
			db,
			`select count(*)
			   from fndds_foods f
			  where f.normalized_description = ?
			    and f.candidate_status in ('eligible_specific', 'eligible_generic')
			    and not exists (
			      select 1 from fndds_ambiguity_flags flag where flag.food_code = f.food_code
			    )`,
			description,
		)
		if err != nil {
			return fmt.Errorf("validate FNDDS SQLite resolver example %q: %w", description, err)
		}
		if got != want {
			return fmt.Errorf("FNDDS SQLite resolver example %q matched %d rows, want %d", description, got, want)
		}
	}

	for _, indexName := range []string{
		"idx_fndds_foods_normalized_description",
		"idx_fndds_foods_candidate_status",
		"idx_fndds_portions_food_code",
		"idx_fndds_flags_food_code",
		"idx_fndds_allergens_food_code",
		"idx_fndds_food_groups_food_code",
	} {
		got, err := sqliteCount(db, "select count(*) from sqlite_master where type = 'index' and name = ?", indexName)
		if err != nil {
			return fmt.Errorf("validate FNDDS SQLite index %s: %w", indexName, err)
		}
		if got != 1 {
			return fmt.Errorf("FNDDS SQLite index %s count = %d, want 1", indexName, got)
		}
	}

	return nil
}

func sqliteCount(db *sql.DB, query string, args ...any) (int, error) {
	var count int
	err := db.QueryRow(query, args...).Scan(&count)
	return count, err
}

func sqliteString(db *sql.DB, query string, args ...any) (string, error) {
	var value string
	err := db.QueryRow(query, args...).Scan(&value)
	return value, err
}

func validateReferenceNutrients(code string, food map[string]any, requireComplete bool) error {
	nutrients, ok := food["nutrients_per_100g"].(map[string]any)
	if !ok {
		return fmt.Errorf("FNDDS food %s must define nutrients_per_100g", code)
	}
	required := []string{
		"energy_kcal", "protein_g", "carbohydrate_g", "fat_g",
		"saturated_fat_g", "sodium_mg", "total_sugar_g", "fiber_g",
	}
	for _, field := range required {
		value := nutrients[field]
		if value == nil {
			if requireComplete {
				return fmt.Errorf("FNDDS eligible food %s has null nutrient %s", code, field)
			}
			continue
		}
		number, ok := value.(float64)
		if !ok || number < 0 {
			return fmt.Errorf("FNDDS food %s has invalid nutrient %s", code, field)
		}
	}
	return nil
}

func validateReferenceSplit(name string, rows []map[string]any, foodByCode map[string]map[string]any, matches func(string) bool) error {
	seen := map[string]bool{}
	for _, row := range rows {
		code := mustString(row, "food_code")
		if seen[code] {
			return fmt.Errorf("%s contains duplicate food_code %s", name, code)
		}
		seen[code] = true
		source := foodByCode[code]
		if source == nil {
			return fmt.Errorf("%s references unknown food_code %s", name, code)
		}
		status := mustString(row, "candidate_status")
		if status != mustString(source, "candidate_status") {
			return fmt.Errorf("%s food %s status = %s, source status = %s", name, code, status, mustString(source, "candidate_status"))
		}
		if !matches(status) {
			return fmt.Errorf("%s food %s has unexpected status %s", name, code, status)
		}
	}
	return nil
}

func isEligibleStatus(status string) bool {
	return status == "eligible_specific" || status == "eligible_generic"
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

func readJSONLObjects(path string) ([]map[string]any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	var rows []map[string]any
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(text), &row); err != nil {
			return nil, fmt.Errorf("parse %s line %d: %w", path, line, err)
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return rows, nil
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

func mustNumber(doc map[string]any, field string) float64 {
	value, ok := doc[field].(float64)
	if !ok {
		panic(fmt.Sprintf("field %s must be a number", field))
	}
	return value
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
