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
		{"schemas/decision.schema.json", "ui/demo-runs/seeded-3-day-peanut-allergy/decision.json"},
		{"schemas/report.schema.json", "ui/demo-runs/seeded-3-day-peanut-allergy/report.json"},
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

	if err := validateExpectedDecision(root); err != nil {
		return err
	}

	if err := validateStaticDemo(root); err != nil {
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
	index, err := readObject(filepath.Join(root, "ui/demo-runs/index.json"))
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
	repoBasePath := filepath.Join("ui", basePath)
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
