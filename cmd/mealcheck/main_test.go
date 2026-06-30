package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestValidateWritesArtifactsAndReturnsBlockExit(t *testing.T) {
	root := repoRoot(t)
	out := filepath.Join(t.TempDir(), "artifacts")

	code := run([]string{
		"validate",
		"--root", root,
		"--case", "examples/seeded-3-day-peanut-allergy/case.json",
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
	if got := decision.ArtifactPaths["case"]; got != "examples/seeded-3-day-peanut-allergy/case.json" {
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
		"--case", "examples/seeded-3-day-peanut-allergy/case.json",
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
		"--case", "examples/seeded-3-day-peanut-allergy/case.json",
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
	if result.TotalCases != 11 || result.SuccessCases != 8 || result.FailureCases != 3 {
		t.Fatalf("case counts = total %d success %d failure %d, want 11/8/3", result.TotalCases, result.SuccessCases, result.FailureCases)
	}
	if result.CasesWithMismatches != 0 {
		t.Fatalf("cases_with_mismatches = %d, want 0", result.CasesWithMismatches)
	}
	if result.SourceItemPreservationRate != 1 {
		t.Fatalf("source_item_preservation_rate = %f, want 1", result.SourceItemPreservationRate)
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
