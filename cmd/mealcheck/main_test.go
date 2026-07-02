package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestValidateWritesArtifactsAndReturnsBlockExit(t *testing.T) {
	root := repoRoot(t)
	out := filepath.Join(t.TempDir(), "artifacts")

	code := run([]string{
		"validate",
		"--root", root,
		"--case", "examples/seeded-one-day-peanut-allergy/case.json",
		"--out", out,
	}, &bytes.Buffer{}, &bytes.Buffer{})

	if code != 1 {
		t.Fatalf("validate exit code = %d, want 1 for block decision", code)
	}

	for _, path := range requiredArtifactPaths() {
		if _, err := os.Stat(filepath.Join(out, path)); err != nil {
			t.Fatalf("missing artifact %s: %v", path, err)
		}
	}

	var decision struct {
		Decision      string            `json:"decision"`
		ArtifactPaths map[string]string `json:"artifact_paths"`
	}
	readJSON(t, filepath.Join(out, "decision.json"), &decision)
	if decision.Decision != "block" {
		t.Fatalf("decision = %q, want block", decision.Decision)
	}
	if got := decision.ArtifactPaths["case"]; got != "examples/seeded-one-day-peanut-allergy/case.json" {
		t.Fatalf("decision case artifact path = %q", got)
	}
	validateAgainstSchema(t, filepath.Join(root, "schemas/decision.schema.json"), filepath.Join(out, "decision.json"))
	validateAgainstSchema(t, filepath.Join(root, "schemas/report.schema.json"), filepath.Join(out, "report.json"))

	var manifest struct {
		Mode      string   `json:"mode"`
		Artifacts []string `json:"artifacts"`
	}
	readJSON(t, filepath.Join(out, "manifest.json"), &manifest)
	if manifest.Mode != "validate" {
		t.Fatalf("manifest mode = %q, want validate", manifest.Mode)
	}
	if len(manifest.Artifacts) < len(requiredArtifactPaths()) {
		t.Fatalf("manifest has %d artifacts, want at least %d", len(manifest.Artifacts), len(requiredArtifactPaths()))
	}
}

func TestCompareWritesCompareManifestAndReturnsBlockExit(t *testing.T) {
	root := repoRoot(t)
	out := filepath.Join(t.TempDir(), "artifacts")

	code := run([]string{
		"compare",
		"--root", root,
		"--case", "examples/seeded-one-day-peanut-allergy/case.json",
		"--out", out,
	}, &bytes.Buffer{}, &bytes.Buffer{})

	if code != 1 {
		t.Fatalf("compare exit code = %d, want 1 for block decision", code)
	}

	var manifest struct {
		Mode string `json:"mode"`
	}
	readJSON(t, filepath.Join(out, "manifest.json"), &manifest)
	if manifest.Mode != "compare" {
		t.Fatalf("manifest mode = %q, want compare", manifest.Mode)
	}
}

func TestDecisionCommandReturnsDecisionExit(t *testing.T) {
	root := repoRoot(t)
	out := filepath.Join(t.TempDir(), "artifacts")

	validateCode := run([]string{
		"validate",
		"--root", root,
		"--case", "examples/seeded-one-day-peanut-allergy/case.json",
		"--out", out,
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if validateCode != 1 {
		t.Fatalf("validate setup exit code = %d, want 1", validateCode)
	}

	var stdout bytes.Buffer
	code := run([]string{"decision", filepath.Join(out, "decision.json")}, &stdout, &bytes.Buffer{})
	if code != 1 {
		t.Fatalf("decision exit code = %d, want 1 for block decision", code)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("decision: block")) {
		t.Fatalf("decision output missing block decision:\n%s", stdout.String())
	}
}

func TestEvalCheckerCommandWritesResult(t *testing.T) {
	root := repoRoot(t)
	out := filepath.Join(t.TempDir(), "eval-checker-result.json")

	code := run([]string{
		"eval-checker",
		"-root", root,
		"-out", out,
	}, &bytes.Buffer{}, &bytes.Buffer{})

	if code != 0 {
		t.Fatalf("eval-checker exit code = %d, want 0", code)
	}

	var result struct {
		SchemaVersion       string `json:"schema_version"`
		DatasetID           string `json:"dataset_id"`
		TotalCases          int    `json:"total_cases"`
		CasesWithMismatches int    `json:"cases_with_mismatches"`
	}
	readJSON(t, out, &result)
	if result.SchemaVersion != "0.1" {
		t.Fatalf("schema_version = %q, want 0.1", result.SchemaVersion)
	}
	if result.DatasetID != "fndds-grounded-meal-plans-v1" {
		t.Fatalf("dataset_id = %q, want fndds-grounded-meal-plans-v1", result.DatasetID)
	}
	if result.TotalCases != 100 {
		t.Fatalf("total_cases = %d, want 100", result.TotalCases)
	}
	if result.CasesWithMismatches != 0 {
		t.Fatalf("cases_with_mismatches = %d, want 0", result.CasesWithMismatches)
	}
}

func TestEvalCheckerCommandWritesPortableExports(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "eval-checker-rows.jsonl")
	csvPath := filepath.Join(dir, "eval-checker-rows.csv")
	out := filepath.Join(dir, "eval-checker-result.json")

	code := run([]string{
		"eval-checker",
		"-root", root,
		"-out", out,
		"-export-jsonl", jsonlPath,
		"-export-csv", csvPath,
	}, &bytes.Buffer{}, &bytes.Buffer{})

	if code != 0 {
		t.Fatalf("eval-checker exit code = %d, want 0", code)
	}

	type row struct {
		EvalType      string  `json:"eval_type"`
		DatasetID     string  `json:"dataset_id"`
		CatalogID     string  `json:"catalog_id"`
		CaseID        string  `json:"case_id"`
		Passed        bool    `json:"passed"`
		FoodItems     int     `json:"food_items"`
		ResolvedItems int     `json:"resolved_items"`
		ResolvedRate  float64 `json:"resolved_rate"`
	}
	rows := readJSONL[row](t, jsonlPath)
	if len(rows) != 100 {
		t.Fatalf("JSONL row count = %d, want 100", len(rows))
	}
	first := rows[0]
	if first.EvalType != "checker" || first.DatasetID != "fndds-grounded-meal-plans-v1" || first.CaseID != "balanced_common-001" {
		t.Fatalf("first checker export row = %+v", first)
	}
	if !first.Passed || first.CatalogID == "" || first.FoodItems == 0 || first.ResolvedItems == 0 || first.ResolvedRate == 0 {
		t.Fatalf("first checker export metrics = %+v", first)
	}

	csvRows := readCSV(t, csvPath)
	if len(csvRows) != 101 {
		t.Fatalf("CSV row count = %d, want 101 including header", len(csvRows))
	}
	wantHeader := []string{"eval_type", "dataset_id", "catalog_id", "case_id", "category"}
	for i, want := range wantHeader {
		if got := csvRows[0][i]; got != want {
			t.Fatalf("CSV header[%d] = %q, want %q", i, got, want)
		}
	}
}

func TestEvalNormalizationCommandWritesResult(t *testing.T) {
	root := repoRoot(t)
	out := filepath.Join(t.TempDir(), "p0-eval-result.json")

	code := run([]string{
		"eval-normalization",
		"-root", root,
		"-out", out,
	}, &bytes.Buffer{}, &bytes.Buffer{})

	if code != 0 {
		t.Fatalf("eval-normalization exit code = %d, want 0", code)
	}

	var result struct {
		SchemaVersion              string  `json:"schema_version"`
		DatasetID                  string  `json:"dataset_id"`
		Mode                       string  `json:"mode"`
		TotalCases                 int     `json:"total_cases"`
		SuccessCases               int     `json:"success_cases"`
		FailureCases               int     `json:"failure_cases"`
		CasesWithMismatches        int     `json:"cases_with_mismatches"`
		SourceItemPreservationRate float64 `json:"source_item_preservation_rate"`
	}
	readJSON(t, out, &result)
	if result.SchemaVersion != "0.1" {
		t.Fatalf("schema_version = %q, want 0.1", result.SchemaVersion)
	}
	if result.DatasetID != "p0-normalization-v1" {
		t.Fatalf("dataset_id = %q, want p0-normalization-v1", result.DatasetID)
	}
	if result.Mode != "deterministic" {
		t.Fatalf("mode = %q, want deterministic", result.Mode)
	}
	if result.TotalCases != 14 || result.SuccessCases != 9 || result.FailureCases != 5 {
		t.Fatalf("case counts = total %d success %d failure %d, want 14/9/5", result.TotalCases, result.SuccessCases, result.FailureCases)
	}
	if result.CasesWithMismatches != 0 {
		t.Fatalf("cases_with_mismatches = %d, want 0", result.CasesWithMismatches)
	}
	if result.SourceItemPreservationRate != 1 {
		t.Fatalf("source_item_preservation_rate = %f, want 1", result.SourceItemPreservationRate)
	}
}

func TestEvalNormalizationCommandWritesPortableExports(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "p0-eval-rows.jsonl")
	csvPath := filepath.Join(dir, "p0-eval-rows.csv")
	out := filepath.Join(dir, "p0-eval-result.json")

	code := run([]string{
		"eval-normalization",
		"-root", root,
		"-out", out,
		"-export-jsonl", jsonlPath,
		"-export-csv", csvPath,
	}, &bytes.Buffer{}, &bytes.Buffer{})

	if code != 0 {
		t.Fatalf("eval-normalization exit code = %d, want 0", code)
	}

	type row struct {
		EvalType                   string   `json:"eval_type"`
		DatasetID                  string   `json:"dataset_id"`
		Mode                       string   `json:"mode"`
		CaseID                     string   `json:"case_id"`
		CaseType                   string   `json:"case_type"`
		Tags                       []string `json:"tags"`
		Passed                     bool     `json:"passed"`
		ExpectedSourceItems        int      `json:"expected_source_items"`
		SourceItemsMatched         int      `json:"source_items_matched"`
		SourceItemPreservationRate float64  `json:"source_item_preservation_rate"`
	}
	rows := readJSONL[row](t, jsonlPath)
	if len(rows) != 14 {
		t.Fatalf("JSONL row count = %d, want 14", len(rows))
	}
	first := rows[0]
	if first.EvalType != "normalization" || first.DatasetID != "p0-normalization-v1" || first.Mode != "deterministic" {
		t.Fatalf("first normalization export row = %+v", first)
	}
	if first.CaseID != "robustness_one_day_canonical_bullets" || first.CaseType != "success" || !first.Passed {
		t.Fatalf("first normalization case row = %+v", first)
	}
	if first.ExpectedSourceItems != 9 || first.SourceItemsMatched != 9 || first.SourceItemPreservationRate != 1 {
		t.Fatalf("first normalization metrics = %+v", first)
	}
	if !containsString(first.Tags, "reviewed_seed") {
		t.Fatalf("first normalization tags = %v, want reviewed_seed", first.Tags)
	}

	csvRows := readCSV(t, csvPath)
	if len(csvRows) != 15 {
		t.Fatalf("CSV row count = %d, want 15 including header", len(csvRows))
	}
	wantHeader := []string{"eval_type", "dataset_id", "mode", "case_id", "case_type"}
	for i, want := range wantHeader {
		if got := csvRows[0][i]; got != want {
			t.Fatalf("CSV header[%d] = %q, want %q", i, got, want)
		}
	}
}

func TestFixtureCheckCommandReturnsOK(t *testing.T) {
	code := run([]string{
		"fixture-check",
		"-root", repoRoot(t),
	}, &bytes.Buffer{}, &bytes.Buffer{})

	if code != 0 {
		t.Fatalf("fixture-check exit code = %d, want 0", code)
	}
}

func TestLocalLlamaNormalizeCommandWritesCanonicalPlan(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "compact.json")
	out := filepath.Join(dir, "normalized-plan.json")
	if err := os.WriteFile(input, []byte(`{"i":[[1,1,"b","cooked oatmeal",1,"cup"],[2,1,"l","grilled chicken breast",4,"oz"],[3,1,"d","baked salmon",4,"oz"]]}`), 0o644); err != nil {
		t.Fatalf("write compact input: %v", err)
	}

	code := run([]string{
		"local-llama",
		"normalize",
		"--input", input,
		"--out", out,
		"--plan-id", "cli-compact-test",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("local-llama normalize exit code = %d, want 0", code)
	}

	var plan struct {
		SchemaVersion string `json:"schema_version"`
		PlanID        string `json:"plan_id"`
		Days          []struct {
			Day   int `json:"day"`
			Meals []struct {
				Name  string `json:"name"`
				Items []struct {
					Food string   `json:"food"`
					Qty  *float64 `json:"quantity"`
					Unit string   `json:"unit"`
				} `json:"items"`
			} `json:"meals"`
		} `json:"days"`
	}
	readJSON(t, out, &plan)
	if plan.SchemaVersion != "0.1" {
		t.Fatalf("schema_version = %q, want 0.1", plan.SchemaVersion)
	}
	if plan.PlanID != "cli-compact-test" {
		t.Fatalf("plan_id = %q, want cli-compact-test", plan.PlanID)
	}
	if len(plan.Days) != 1 || len(plan.Days[0].Meals) != 3 {
		t.Fatalf("plan days/meals = %d/%d, want 1/3", len(plan.Days), len(plan.Days[0].Meals))
	}
	if plan.Days[0].Meals[0].Items[0].Qty == nil {
		t.Fatal("first item quantity = nil")
	}
}

func TestInvalidCLIUsageReturnsConfigExit(t *testing.T) {
	code := run([]string{"validate"}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != 2 {
		t.Fatalf("validate without --case exit code = %d, want 2", code)
	}
	code = run([]string{"decision"}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != 2 {
		t.Fatalf("decision without path exit code = %d, want 2", code)
	}
}

func requiredArtifactPaths() []string {
	return []string{
		"decision.json",
		"recommendation.json",
		"report.json",
		"report.html",
		"report.pdf",
		"report.md",
		"failures.jsonl",
		"daily-totals.json",
		"resolved-foods.json",
		"unresolved-foods.json",
		"excluded-unresolved-foods.json",
		"metrics.json",
		"manifest.json",
		"normalized-plan.json",
		"configs/run.json",
		"configs/redacted-provider.json",
		"guideline-pack/pack.json",
		"guideline-pack/citations.json",
		"schemas/decision.schema.json",
		"schemas/meal-plan.schema.json",
		"schemas/guideline-pack.schema.json",
		"schemas/nutrient-catalog.schema.json",
		"schemas/report.schema.json",
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readJSON(t *testing.T, path string, out any) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func readJSONL[T any](t *testing.T, path string) []T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var rows []T
	for i, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var row T
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("decode %s line %d: %v", path, i+1, err)
		}
		rows = append(rows, row)
	}
	return rows
}

func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("read csv %s: %v", path, err)
	}
	return rows
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func validateAgainstSchema(t *testing.T, schemaPath, instancePath string) {
	t.Helper()

	var schema jsonschema.Schema
	readJSON(t, schemaPath, &schema)
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatalf("resolve schema %s: %v", schemaPath, err)
	}

	var instance any
	readJSON(t, instancePath, &instance)
	if err := resolved.Validate(instance); err != nil {
		t.Fatalf("validate %s against %s: %v", instancePath, schemaPath, err)
	}
}
