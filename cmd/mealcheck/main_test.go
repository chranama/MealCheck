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
		"report.json",
		"report.html",
		"report.pdf",
		"report.md",
		"failures.jsonl",
		"daily-totals.json",
		"resolved-foods.json",
		"unresolved-foods.json",
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
