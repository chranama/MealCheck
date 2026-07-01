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
		{"schemas/case.schema.json", "examples/seeded-one-day-peanut-allergy/case.json"},
		{"schemas/meal-plan.schema.json", "examples/seeded-one-day-peanut-allergy/plans/baseline.json"},
		{"schemas/meal-plan.schema.json", "examples/seeded-one-day-peanut-allergy/plans/candidate.json"},
		{"schemas/source-registry.schema.json", "data/guidelines/dga-2025-2030-us-adult-general-v1/source-registry.json"},
		{"schemas/guideline-pack.schema.json", "data/guidelines/dga-2025-2030-us-adult-general-v1/guideline-pack.json"},
		{"schemas/nutrient-catalog.schema.json", "data/nutrients/fixture-catalog-v1.json"},
		{"schemas/decision.schema.json", "examples/seeded-one-day-peanut-allergy/expected-decision.json"},
		{"schemas/decision.schema.json", "examples/seeded-one-day-peanut-allergy/artifacts/demo-runs/seeded-one-day-peanut-allergy/decision.json"},
		{"schemas/report.schema.json", "examples/seeded-one-day-peanut-allergy/artifacts/demo-runs/seeded-one-day-peanut-allergy/report.json"},
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
	caseDoc, err := readObject(filepath.Join(root, "examples/seeded-one-day-peanut-allergy/case.json"))
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
	caseDoc, err := readObject(filepath.Join(root, "examples/seeded-one-day-peanut-allergy/case.json"))
	if err != nil {
		return err
	}
	expected, err := readObject(filepath.Join(root, "examples/seeded-one-day-peanut-allergy/expected-decision.json"))
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
	index, err := readObject(filepath.Join(root, "examples/seeded-one-day-peanut-allergy/artifacts/demo-runs/index.json"))
	if err != nil {
		return err
	}
	demos := objectSlice(index, "demo_runs")
	if len(demos) != 1 {
		return fmt.Errorf("ui demo index should contain exactly one seeded demo, got %d", len(demos))
	}

	demo := demos[0]
	if id := mustString(demo, "id"); id != "seeded-one-day-peanut-allergy" {
		return fmt.Errorf("unexpected UI demo id %q", id)
	}
	basePath := mustString(demo, "base_path")
	repoBasePath := filepath.Join("examples", "seeded-one-day-peanut-allergy", "artifacts", basePath)
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
	if err := validateP0NormalizationDataset(root); err != nil {
		return err
	}

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

func validateP0NormalizationDataset(root string) error {
	base := filepath.Join(root, "data/evaluation/p0-normalization")
	sourceManifest, err := readObject(filepath.Join(base, "source-manifest.json"))
	if err != nil {
		return err
	}
	if got := mustString(sourceManifest, "schema_version"); got != "0.1" {
		return fmt.Errorf("p0 source-manifest schema_version = %q, want 0.1", got)
	}
	if got := mustString(sourceManifest, "source_manifest_id"); got != "p0-normalization-sources-v1" {
		return fmt.Errorf("p0 source_manifest_id = %q, want p0-normalization-sources-v1", got)
	}
	if len(objectSlice(sourceManifest, "sources")) == 0 {
		return errors.New("p0 source-manifest must define sources")
	}

	manifest, err := readObject(filepath.Join(base, "manifest.json"))
	if err != nil {
		return err
	}
	if got := mustString(manifest, "schema_version"); got != "0.1" {
		return fmt.Errorf("p0 manifest schema_version = %q, want 0.1", got)
	}
	if got := mustString(manifest, "dataset_id"); got != "p0-normalization-v1" {
		return fmt.Errorf("p0 manifest dataset_id = %q, want p0-normalization-v1", got)
	}
	if _, ok := manifest["release_gate"].(bool); !ok {
		return errors.New("p0 manifest release_gate must be boolean")
	}
	if len(stringSlice(manifest, "supported_units")) == 0 {
		return errors.New("p0 manifest supported_units must not be empty")
	}

	caseFiles, err := p0ManifestFiles(manifest, "case_files")
	if err != nil {
		return err
	}
	if len(caseFiles) == 0 {
		return errors.New("p0 manifest case_files must not be empty")
	}
	failureFiles, err := p0ManifestFiles(manifest, "failure_case_files")
	if err != nil {
		return err
	}
	if len(failureFiles) == 0 {
		return errors.New("p0 manifest failure_case_files must not be empty")
	}
	quarantineFiles, err := p0ManifestFiles(manifest, "quarantine_files")
	if err != nil {
		return err
	}

	successRows := []map[string]any{}
	for _, file := range caseFiles {
		rows, err := readJSONLObjects(filepath.Join(base, file.Path))
		if err != nil {
			return err
		}
		successRows = append(successRows, rows...)
	}
	failureRows := []map[string]any{}
	for _, file := range failureFiles {
		rows, err := readJSONLObjects(filepath.Join(base, file.Path))
		if err != nil {
			return err
		}
		failureRows = append(failureRows, rows...)
	}
	quarantineRows := []map[string]any{}
	for _, file := range quarantineFiles {
		rows, err := readJSONLObjects(filepath.Join(base, file.Path))
		if err != nil {
			return err
		}
		quarantineRows = append(quarantineRows, rows...)
	}
	if len(successRows) == 0 {
		return errors.New("p0 normalization success cases must not be empty")
	}
	if len(failureRows) == 0 {
		return errors.New("p0 normalization failure cases must not be empty")
	}

	seenIDs := map[string]bool{}
	totalExpectedItems := 0
	for _, row := range successRows {
		id := mustString(row, "id")
		if seenIDs[id] {
			return fmt.Errorf("p0 normalization has duplicate id %s", id)
		}
		seenIDs[id] = true
		if got := mustString(row, "schema_version"); got != "0.1" {
			return fmt.Errorf("p0 case %s schema_version = %q, want 0.1", id, got)
		}
		if _, err := stringField(row, "source_dataset"); err != nil {
			return fmt.Errorf("p0 case %s: %w", id, err)
		}
		if _, err := stringField(row, "input_text"); err != nil {
			return fmt.Errorf("p0 case %s: %w", id, err)
		}
		expected, ok := row["expected"].(map[string]any)
		if !ok {
			return fmt.Errorf("p0 case %s must define expected", id)
		}
		items := objectSlice(expected, "source_items")
		if len(items) == 0 {
			return fmt.Errorf("p0 case %s expected.source_items must not be empty", id)
		}
		totalExpectedItems += len(items)
		for index, item := range items {
			if sourceID, err := numericField(item, "source_item_id"); err != nil || int(sourceID) != index+1 {
				return fmt.Errorf("p0 case %s source_items[%d] source_item_id must be %d", id, index, index+1)
			}
			if day, err := numericField(item, "day"); err != nil || day < 1 || day > 7 {
				return fmt.Errorf("p0 case %s source_items[%d] day must be 1..7", id, index)
			}
			if !validP0MealCode(mustString(item, "meal_code")) {
				return fmt.Errorf("p0 case %s source_items[%d] has invalid meal_code %q", id, index, mustString(item, "meal_code"))
			}
			if _, err := stringField(item, "source_text"); err != nil {
				return fmt.Errorf("p0 case %s source_items[%d]: %w", id, index, err)
			}
			if _, err := stringField(item, "food"); err != nil {
				return fmt.Errorf("p0 case %s source_items[%d]: %w", id, index, err)
			}
			if quantity, err := numericField(item, "quantity"); err != nil || quantity <= 0 {
				return fmt.Errorf("p0 case %s source_items[%d] quantity must be positive", id, index)
			}
			if !validP0Unit(mustString(item, "unit")) {
				return fmt.Errorf("p0 case %s source_items[%d] has invalid unit %q", id, index, mustString(item, "unit"))
			}
		}
	}
	for _, row := range failureRows {
		id := mustString(row, "id")
		if seenIDs[id] {
			return fmt.Errorf("p0 normalization has duplicate id %s", id)
		}
		seenIDs[id] = true
		if got := mustString(row, "schema_version"); got != "0.1" {
			return fmt.Errorf("p0 failure case %s schema_version = %q, want 0.1", id, got)
		}
		if _, err := stringField(row, "input_text"); err != nil {
			return fmt.Errorf("p0 failure case %s: %w", id, err)
		}
		expected, ok := row["expected_failure"].(map[string]any)
		if !ok {
			return fmt.Errorf("p0 failure case %s must define expected_failure", id)
		}
		stage := mustString(expected, "stage")
		switch stage {
		case "qualification":
			if _, err := stringField(expected, "status"); err != nil {
				return fmt.Errorf("p0 failure case %s expected_failure: %w", id, err)
			}
		case "source_inventory":
			if _, err := stringField(expected, "reason"); err != nil {
				return fmt.Errorf("p0 failure case %s expected_failure: %w", id, err)
			}
		default:
			return fmt.Errorf("p0 failure case %s has invalid stage %q", id, stage)
		}
	}
	for _, row := range quarantineRows {
		id := mustString(row, "id")
		if seenIDs[id] {
			return fmt.Errorf("p0 normalization has duplicate id %s", id)
		}
		seenIDs[id] = true
		if got := mustString(row, "schema_version"); got != "0.1" {
			return fmt.Errorf("p0 quarantine case %s schema_version = %q, want 0.1", id, got)
		}
		if _, err := stringField(row, "source_dataset"); err != nil {
			return fmt.Errorf("p0 quarantine case %s: %w", id, err)
		}
		if _, err := stringField(row, "raw_text"); err != nil {
			return fmt.Errorf("p0 quarantine case %s: %w", id, err)
		}
		if _, err := stringField(row, "quarantine_reason"); err != nil {
			return fmt.Errorf("p0 quarantine case %s: %w", id, err)
		}
	}

	summary, ok := manifest["summary"].(map[string]any)
	if !ok {
		return errors.New("p0 manifest must define summary")
	}
	if got, err := numericField(summary, "success_cases"); err != nil || int(got) != len(successRows) {
		return fmt.Errorf("p0 manifest summary.success_cases must equal %d", len(successRows))
	}
	if got, err := numericField(summary, "failure_cases"); err != nil || int(got) != len(failureRows) {
		return fmt.Errorf("p0 manifest summary.failure_cases must equal %d", len(failureRows))
	}
	if got, err := numericField(summary, "total_expected_source_items"); err != nil || int(got) != totalExpectedItems {
		return fmt.Errorf("p0 manifest summary.total_expected_source_items must equal %d", totalExpectedItems)
	}
	if len(quarantineRows) > 0 {
		if got, err := numericField(summary, "quarantine_cases"); err != nil || int(got) != len(quarantineRows) {
			return fmt.Errorf("p0 manifest summary.quarantine_cases must equal %d", len(quarantineRows))
		}
	}
	return nil
}

type p0ManifestFile struct {
	Path          string
	SourceDataset string
	Gate          string
}

func p0ManifestFiles(manifest map[string]any, key string) ([]p0ManifestFile, error) {
	raw, ok := manifest[key]
	if !ok {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("p0 manifest %s must be an array", key)
	}
	files := make([]p0ManifestFile, 0, len(values))
	for index, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) == "" {
				return nil, fmt.Errorf("p0 manifest %s[%d] path must not be empty", key, index)
			}
			files = append(files, p0ManifestFile{Path: typed, Gate: "strict"})
		case map[string]any:
			path := mustString(typed, "path")
			if strings.TrimSpace(path) == "" {
				return nil, fmt.Errorf("p0 manifest %s[%d] path must not be empty", key, index)
			}
			gate := mustString(typed, "gate")
			if gate == "" {
				gate = "strict"
			}
			if gate != "strict" && gate != "exploratory" {
				return nil, fmt.Errorf("p0 manifest %s[%d] has invalid gate %q", key, index, gate)
			}
			sourceDataset := mustString(typed, "source_dataset")
			if sourceDataset == "" && gate == "exploratory" {
				return nil, fmt.Errorf("p0 manifest %s[%d] exploratory file must define source_dataset", key, index)
			}
			files = append(files, p0ManifestFile{Path: path, SourceDataset: sourceDataset, Gate: gate})
		default:
			return nil, fmt.Errorf("p0 manifest %s[%d] must be a string or object", key, index)
		}
	}
	return files, nil
}

func validP0MealCode(code string) bool {
	switch code {
	case "b", "m", "l", "a", "d", "s", "e":
		return true
	default:
		return false
	}
}

func validP0Unit(unit string) bool {
	switch unit {
	case "g", "oz", "cup", "tbsp", "tsp", "slice", "serving":
		return true
	default:
		return false
	}
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
		"resolver-match-keys.jsonl",
		"unit-conversions.jsonl",
		"quarantined-foods.jsonl",
		"review-required-foods.jsonl",
		"approximation-proxies.json",
		"decomposition-templates.json",
		"decomposition-rules.json",
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
	matchKeys, err := readJSONLObjects(filepath.Join(base, "resolver-match-keys.jsonl"))
	if err != nil {
		return err
	}
	unitConversions, err := readJSONLObjects(filepath.Join(base, "unit-conversions.jsonl"))
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
	approximationProxyDoc, err := readObject(filepath.Join(base, "approximation-proxies.json"))
	if err != nil {
		return err
	}
	decompositionTemplateDoc, err := readObject(filepath.Join(base, "decomposition-templates.json"))
	if err != nil {
		return err
	}
	decompositionRuleDoc, err := readObject(filepath.Join(base, "decomposition-rules.json"))
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
	if got := int(mustNumber(summary, "resolver_match_key_count")); got != len(matchKeys) {
		return fmt.Errorf("FNDDS summary resolver_match_key_count = %d, want %d", got, len(matchKeys))
	}
	if got := int(mustNumber(summary, "unit_conversion_count")); got != len(unitConversions) {
		return fmt.Errorf("FNDDS summary unit_conversion_count = %d, want %d", got, len(unitConversions))
	}
	approximationProxies := objectSlice(approximationProxyDoc, "proxies")
	decompositionTemplates := objectSlice(decompositionTemplateDoc, "templates")
	decompositionRules := objectSlice(decompositionRuleDoc, "rules")
	if got := int(mustNumber(summary, "approximation_proxy_count")); got != len(approximationProxies) {
		return fmt.Errorf("FNDDS summary approximation_proxy_count = %d, want %d", got, len(approximationProxies))
	}
	approximationProxySourceCodeCount := 0
	for _, proxy := range approximationProxies {
		approximationProxySourceCodeCount += len(stringSlice(proxy, "source_food_codes"))
	}
	if got := int(mustNumber(summary, "approximation_proxy_source_code_count")); got != approximationProxySourceCodeCount {
		return fmt.Errorf("FNDDS summary approximation_proxy_source_code_count = %d, want %d", got, approximationProxySourceCodeCount)
	}
	if got := int(mustNumber(summary, "decomposition_template_count")); got != len(decompositionTemplates) {
		return fmt.Errorf("FNDDS summary decomposition_template_count = %d, want %d", got, len(decompositionTemplates))
	}
	if got := int(mustNumber(summary, "decomposition_rule_count")); got != len(decompositionRules) {
		return fmt.Errorf("FNDDS summary decomposition_rule_count = %d, want %d", got, len(decompositionRules))
	}
	decompositionRuleSourceCodeCount := 0
	decompositionRuleComponentCount := 0
	for _, rule := range decompositionRules {
		decompositionRuleSourceCodeCount += len(stringSlice(rule, "source_food_codes"))
		decompositionRuleComponentCount += len(objectSlice(rule, "components"))
	}
	if got := int(mustNumber(summary, "decomposition_rule_source_code_count")); got != decompositionRuleSourceCodeCount {
		return fmt.Errorf("FNDDS summary decomposition_rule_source_code_count = %d, want %d", got, decompositionRuleSourceCodeCount)
	}
	if got := int(mustNumber(summary, "decomposition_rule_component_count")); got != decompositionRuleComponentCount {
		return fmt.Errorf("FNDDS summary decomposition_rule_component_count = %d, want %d", got, decompositionRuleComponentCount)
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
	validResolverStatuses := map[string]bool{
		"auto": true, "review": true, "decompose": true, "blocked": true,
	}
	validMatchConfidences := map[string]bool{
		"exact": true, "high": true, "review": true,
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
	if err := validateReferenceMatchKeys(matchKeys, foodByCode, validResolverStatuses, validMatchConfidences); err != nil {
		return err
	}
	if err := validateReferenceUnitConversions(unitConversions, foodByCode); err != nil {
		return err
	}
	if err := validateReferenceApproximationProxies(approximationProxies, foodByCode); err != nil {
		return err
	}
	if err := validateReferenceDecompositionTemplates(decompositionTemplates, foodByCode); err != nil {
		return err
	}
	if err := validateReferenceDecompositionRules(decompositionRules, foodByCode); err != nil {
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
	if err := validateFNDDSReferenceSQLite(base, foods, nutrients, portions, matchKeys, unitConversions, approximationProxies, decompositionTemplates, decompositionRules); err != nil {
		return err
	}
	return nil
}

func validateFNDDSReferenceSQLite(base string, foods, nutrients, portions, matchKeys, unitConversions, approximationProxies, decompositionTemplates, decompositionRules []map[string]any) error {
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
	decompositionComponentCount := 0
	for _, template := range decompositionTemplates {
		decompositionComponentCount += len(objectSlice(template, "components"))
	}
	decompositionRuleSourceCodeCount := 0
	decompositionRuleTermCount := 0
	decompositionRuleComponentCount := 0
	for _, rule := range decompositionRules {
		decompositionRuleSourceCodeCount += len(stringSlice(rule, "source_food_codes"))
		decompositionRuleTermCount += len(stringSlice(rule, "match_terms"))
		decompositionRuleTermCount += len(stringSlice(rule, "exclude_terms"))
		decompositionRuleComponentCount += len(objectSlice(rule, "components"))
	}
	approximationProxySourceCodeCount := 0
	for _, proxy := range approximationProxies {
		approximationProxySourceCodeCount += len(stringSlice(proxy, "source_food_codes"))
	}

	expectedCounts := map[string]int{
		"fndds_foods":                            len(foods),
		"fndds_nutrients":                        len(nutrients),
		"fndds_portions":                         len(portions),
		"fndds_match_keys":                       len(matchKeys),
		"fndds_unit_conversions":                 len(unitConversions),
		"fndds_approximation_proxies":            len(approximationProxies),
		"fndds_approximation_proxy_source_codes": approximationProxySourceCodeCount,
		"fndds_decomposition_templates":          len(decompositionTemplates),
		"fndds_decomposition_components":         decompositionComponentCount,
		"fndds_decomposition_rules":              len(decompositionRules),
		"fndds_decomposition_rule_source_codes":  decompositionRuleSourceCodeCount,
		"fndds_decomposition_rule_terms":         decompositionRuleTermCount,
		"fndds_decomposition_rule_components":    decompositionRuleComponentCount,
		"fndds_ambiguity_flags":                  flagCount,
		"fndds_allergens":                        allergenCount,
		"fndds_food_groups":                      foodGroupCount,
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
		"water tap":                      1,
		"instant coffee":                 1,
		"granulated sugar":               1,
		"lettuce":                        1,
		"rice white cooked no added fat": 1,
		"milk nfs":                       0,
		"milk human":                     0,
	}
	for description, want := range resolverExamples {
		got, err := sqliteCount(
			db,
			`select count(*)
			   from (
			     select distinct key.food_code
			       from fndds_match_keys key
			      where key.normalized_match_key = ?
			        and key.resolver_status = 'auto'
			        and key.confidence in ('exact', 'high')
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
		"idx_fndds_match_keys_normalized_status",
		"idx_fndds_match_keys_food_code",
		"idx_fndds_unit_conversions_food_code",
		"idx_fndds_approximation_proxies_normalized_input_key",
		"idx_fndds_approximation_proxy_source_codes_source",
		"idx_fndds_decomposition_templates_normalized_pattern",
		"idx_fndds_decomposition_components_template_id",
		"idx_fndds_decomposition_rule_source_codes_source",
		"idx_fndds_decomposition_rule_terms_type",
		"idx_fndds_decomposition_rule_components_rule_id",
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

func validateReferenceMatchKeys(rows []map[string]any, foodByCode map[string]map[string]any, validStatuses, validConfidences map[string]bool) error {
	seen := map[string]bool{}
	for _, row := range rows {
		code := mustString(row, "food_code")
		if foodByCode[code] == nil {
			return fmt.Errorf("resolver-match-keys.jsonl references unknown food_code %s", code)
		}
		normalized := mustString(row, "normalized_match_key")
		if strings.TrimSpace(normalized) == "" {
			return fmt.Errorf("FNDDS match key for food %s has empty normalized_match_key", code)
		}
		keyType := mustString(row, "key_type")
		if strings.TrimSpace(keyType) == "" {
			return fmt.Errorf("FNDDS match key for food %s has empty key_type", code)
		}
		dedupeKey := code + "\x00" + normalized + "\x00" + keyType
		if seen[dedupeKey] {
			return fmt.Errorf("duplicate FNDDS match key %s for food %s key_type %s", normalized, code, keyType)
		}
		seen[dedupeKey] = true
		status := mustString(row, "resolver_status")
		if !validStatuses[status] {
			return fmt.Errorf("FNDDS match key %s for food %s has invalid resolver_status %s", normalized, code, status)
		}
		confidence := mustString(row, "confidence")
		if !validConfidences[confidence] {
			return fmt.Errorf("FNDDS match key %s for food %s has invalid confidence %s", normalized, code, confidence)
		}
		if status == "auto" && mustString(row, "block_reason") != "" {
			return fmt.Errorf("FNDDS auto match key %s for food %s has block_reason", normalized, code)
		}
	}
	return nil
}

func validateReferenceUnitConversions(rows []map[string]any, foodByCode map[string]map[string]any) error {
	seen := map[string]bool{}
	for _, row := range rows {
		code := mustString(row, "food_code")
		if foodByCode[code] == nil {
			return fmt.Errorf("unit-conversions.jsonl references unknown food_code %s", code)
		}
		unit := mustString(row, "normalized_unit")
		if strings.TrimSpace(unit) == "" {
			return fmt.Errorf("FNDDS unit conversion for food %s has empty normalized_unit", code)
		}
		dedupeKey := code + "\x00" + unit
		if seen[dedupeKey] {
			return fmt.Errorf("duplicate FNDDS unit conversion %s for food %s", unit, code)
		}
		seen[dedupeKey] = true
		grams, err := numericField(row, "grams")
		if err != nil || grams <= 0 {
			return fmt.Errorf("FNDDS unit conversion %s for food %s has invalid grams", unit, code)
		}
		if _, err := stringField(row, "source_description"); err != nil {
			return fmt.Errorf("FNDDS unit conversion %s for food %s: %w", unit, code, err)
		}
	}
	return nil
}

func validateReferenceApproximationProxies(rows []map[string]any, foodByCode map[string]map[string]any) error {
	if len(rows) == 0 {
		return errors.New("approximation-proxies.json must define at least one proxy")
	}
	seen := map[string]bool{}
	validConfidences := map[string]bool{"low": true, "medium": true, "high": true}
	for _, row := range rows {
		inputKey := mustString(row, "input_key")
		normalized := mustString(row, "normalized_input_key")
		if normalized != normalizeMatchKey(inputKey) {
			return fmt.Errorf("approximation proxy %q normalized_input_key = %q, want %q", inputKey, normalized, normalizeMatchKey(inputKey))
		}
		if seen[normalized] {
			return fmt.Errorf("duplicate approximation proxy %q", normalized)
		}
		seen[normalized] = true
		code := mustString(row, "proxy_food_code")
		if foodByCode[code] == nil {
			return fmt.Errorf("approximation proxy %q references unknown food_code %s", inputKey, code)
		}
		seenSourceCodes := map[string]bool{}
		for _, sourceCode := range stringSlice(row, "source_food_codes") {
			if seenSourceCodes[sourceCode] {
				return fmt.Errorf("approximation proxy %q contains duplicate source_food_code %s", inputKey, sourceCode)
			}
			seenSourceCodes[sourceCode] = true
			if foodByCode[sourceCode] == nil {
				return fmt.Errorf("approximation proxy %q references unknown source_food_code %s", inputKey, sourceCode)
			}
		}
		if !validConfidences[mustString(row, "confidence")] {
			return fmt.Errorf("approximation proxy %q has invalid confidence %q", inputKey, mustString(row, "confidence"))
		}
		if _, err := stringField(row, "estimate_reason"); err != nil {
			return fmt.Errorf("approximation proxy %q: %w", inputKey, err)
		}
		if _, ok := row["allow_when_allergies_present"].(bool); !ok {
			return fmt.Errorf("approximation proxy %q allow_when_allergies_present must be boolean", inputKey)
		}
		if _, ok := row["allow_when_exclusions_present"].(bool); !ok {
			return fmt.Errorf("approximation proxy %q allow_when_exclusions_present must be boolean", inputKey)
		}
	}
	return nil
}

func validateReferenceDecompositionTemplates(rows []map[string]any, foodByCode map[string]map[string]any) error {
	if len(rows) == 0 {
		return errors.New("decomposition-templates.json must define at least one template")
	}
	seenIDs := map[string]bool{}
	seenPatterns := map[string]bool{}
	validConfidences := map[string]bool{"low": true, "medium": true, "high": true}
	for _, row := range rows {
		templateID := mustString(row, "template_id")
		if seenIDs[templateID] {
			return fmt.Errorf("duplicate decomposition template_id %s", templateID)
		}
		seenIDs[templateID] = true
		pattern := mustString(row, "pattern")
		normalized := mustString(row, "normalized_pattern")
		if normalized != normalizeMatchKey(pattern) {
			return fmt.Errorf("decomposition template %s normalized_pattern = %q, want %q", templateID, normalized, normalizeMatchKey(pattern))
		}
		if seenPatterns[normalized] {
			return fmt.Errorf("duplicate decomposition template pattern %q", normalized)
		}
		seenPatterns[normalized] = true
		if !validConfidences[mustString(row, "confidence")] {
			return fmt.Errorf("decomposition template %s has invalid confidence %q", templateID, mustString(row, "confidence"))
		}
		components := objectSlice(row, "components")
		if len(components) == 0 {
			return fmt.Errorf("decomposition template %s must define components", templateID)
		}
		totalFraction := 0.0
		for _, component := range components {
			code := mustString(component, "food_code")
			if foodByCode[code] == nil {
				return fmt.Errorf("decomposition template %s references unknown food_code %s", templateID, code)
			}
			if _, err := stringField(component, "role"); err != nil {
				return fmt.Errorf("decomposition template %s component %s: %w", templateID, code, err)
			}
			fraction, err := numericField(component, "fraction")
			if err != nil || fraction <= 0 || fraction >= 1 {
				return fmt.Errorf("decomposition template %s component %s has invalid fraction", templateID, code)
			}
			totalFraction += fraction
			if _, ok := component["required"].(bool); !ok {
				return fmt.Errorf("decomposition template %s component %s required must be boolean", templateID, code)
			}
		}
		if totalFraction < 0.999 || totalFraction > 1.001 {
			return fmt.Errorf("decomposition template %s fractions sum to %.3f", templateID, totalFraction)
		}
	}
	return nil
}

func validateReferenceDecompositionRules(rows []map[string]any, foodByCode map[string]map[string]any) error {
	if len(rows) == 0 {
		return errors.New("decomposition-rules.json must define at least one rule")
	}
	seenIDs := map[string]bool{}
	seenSourceCodes := map[string]bool{}
	validConfidences := map[string]bool{"low": true, "medium": true, "high": true}
	for _, row := range rows {
		ruleID := mustString(row, "rule_id")
		if seenIDs[ruleID] {
			return fmt.Errorf("duplicate decomposition rule_id %s", ruleID)
		}
		seenIDs[ruleID] = true
		if _, err := stringField(row, "family"); err != nil {
			return fmt.Errorf("decomposition rule %s: %w", ruleID, err)
		}
		priority, err := numericField(row, "priority")
		if err != nil || priority <= 0 {
			return fmt.Errorf("decomposition rule %s has invalid priority", ruleID)
		}
		if !validConfidences[mustString(row, "confidence")] {
			return fmt.Errorf("decomposition rule %s has invalid confidence %q", ruleID, mustString(row, "confidence"))
		}
		if _, err := stringField(row, "notes"); err != nil {
			return fmt.Errorf("decomposition rule %s: %w", ruleID, err)
		}

		sourceCodes := stringSlice(row, "source_food_codes")
		matchTerms := stringSlice(row, "match_terms")
		if len(sourceCodes) == 0 && len(matchTerms) == 0 {
			return fmt.Errorf("decomposition rule %s must define source_food_codes or match_terms", ruleID)
		}
		for _, sourceCode := range sourceCodes {
			if foodByCode[sourceCode] == nil {
				return fmt.Errorf("decomposition rule %s references unknown source_food_code %s", ruleID, sourceCode)
			}
			if seenSourceCodes[sourceCode] {
				return fmt.Errorf("decomposition source_food_code %s is assigned to multiple rules", sourceCode)
			}
			seenSourceCodes[sourceCode] = true
		}

		normalizedMatchTerms := stringSlice(row, "normalized_match_terms")
		if len(normalizedMatchTerms) != len(matchTerms) {
			return fmt.Errorf("decomposition rule %s normalized_match_terms count mismatch", ruleID)
		}
		if err := validateReferenceDecompositionRuleTerms(ruleID, "match_terms", matchTerms, normalizedMatchTerms); err != nil {
			return err
		}
		excludeTerms := stringSlice(row, "exclude_terms")
		normalizedExcludeTerms := stringSlice(row, "normalized_exclude_terms")
		if len(normalizedExcludeTerms) != len(excludeTerms) {
			return fmt.Errorf("decomposition rule %s normalized_exclude_terms count mismatch", ruleID)
		}
		if err := validateReferenceDecompositionRuleTerms(ruleID, "exclude_terms", excludeTerms, normalizedExcludeTerms); err != nil {
			return err
		}

		components := objectSlice(row, "components")
		if len(components) == 0 {
			return fmt.Errorf("decomposition rule %s must define components", ruleID)
		}
		totalFraction := 0.0
		for _, component := range components {
			code := mustString(component, "food_code")
			if foodByCode[code] == nil {
				return fmt.Errorf("decomposition rule %s references unknown food_code %s", ruleID, code)
			}
			if _, err := stringField(component, "role"); err != nil {
				return fmt.Errorf("decomposition rule %s component %s: %w", ruleID, code, err)
			}
			fraction, err := numericField(component, "fraction")
			if err != nil || fraction <= 0 || fraction >= 1 {
				return fmt.Errorf("decomposition rule %s component %s has invalid fraction", ruleID, code)
			}
			totalFraction += fraction
			if _, ok := component["required"].(bool); !ok {
				return fmt.Errorf("decomposition rule %s component %s required must be boolean", ruleID, code)
			}
		}
		if totalFraction < 0.999 || totalFraction > 1.001 {
			return fmt.Errorf("decomposition rule %s fractions sum to %.3f", ruleID, totalFraction)
		}
	}
	return nil
}

func validateReferenceDecompositionRuleTerms(ruleID string, field string, terms []string, normalizedTerms []string) error {
	seen := map[string]bool{}
	for i, term := range terms {
		normalized := normalizeMatchKey(term)
		if normalized == "" {
			return fmt.Errorf("decomposition rule %s has empty %s term", ruleID, field)
		}
		if normalizedTerms[i] != normalized {
			return fmt.Errorf("decomposition rule %s %s normalized term = %q, want %q", ruleID, field, normalizedTerms[i], normalized)
		}
		if seen[normalized] {
			return fmt.Errorf("decomposition rule %s has duplicate %s term %q", ruleID, field, normalized)
		}
		seen[normalized] = true
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

func normalizeMatchKey(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
