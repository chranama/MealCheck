package evalnormalization

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chranama/MealCheck/internal/hosted"
)

func TestRunLocalLlamaModeScoresProviderOutput(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data", "evaluation", "p0-normalization")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	manifestPath := filepath.Join(dataDir, "manifest.json")
	datasetPath := filepath.Join(dataDir, "cases-v1.jsonl")
	failurePath := filepath.Join(dataDir, "failure-cases-v1.jsonl")
	if err := os.WriteFile(manifestPath, []byte(`{"schema_version":"0.1","dataset_id":"p0-local-test"}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	successCase := `{"schema_version":"0.1","id":"one-item","source_dataset":"test","input_text":"Day 1 breakfast:\n- 1 cup oatmeal","expected":{"days":[1],"source_items":[{"source_item_id":1,"day":1,"meal_code":"b","source_text":"1 cup oatmeal","food":"oatmeal","quantity":1,"unit":"cup"}]},"tags":["unit_test"]}` + "\n"
	if err := os.WriteFile(datasetPath, []byte(successCase), 0o644); err != nil {
		t.Fatalf("write dataset: %v", err)
	}
	if err := os.WriteFile(failurePath, nil, 0o644); err != nil {
		t.Fatalf("write failures: %v", err)
	}

	result, err := run(runOptions{
		Root:         root,
		ManifestPath: manifestPath,
		DatasetPath:  datasetPath,
		FailurePath:  failurePath,
		Mode:         "local-llama",
		ProviderConfig: hosted.ProviderConfig{
			Type:  hosted.ProviderTypeLocalLlama,
			Model: "test-model",
		},
		ProviderFactory: hosted.StaticResponseProviderFactory(`{"i":[[1,1,"b","oatmeal",1,"cup"]]}`),
	})
	if err != nil {
		t.Fatalf("run local llama eval: %v", err)
	}
	if result.Mode != modeLocalLlama {
		t.Fatalf("mode = %q, want %q", result.Mode, modeLocalLlama)
	}
	if result.TotalCases != 1 || result.CasesPassed != 1 || result.CasesWithMismatches != 0 {
		t.Fatalf("unexpected result counts: total=%d passed=%d mismatches=%d", result.TotalCases, result.CasesPassed, result.CasesWithMismatches)
	}
	if result.LocalModelSuccessCasesRun != 1 || result.LocalModelSuccessCasesPass != 1 {
		t.Fatalf("local model pass counts = %d/%d, want 1/1", result.LocalModelSuccessCasesPass, result.LocalModelSuccessCasesRun)
	}
	if result.LocalModelRowsMatched != 1 || result.LocalModelRowMatchRate != 1 {
		t.Fatalf("local model row match = %d rate %.3f, want 1 and 1", result.LocalModelRowsMatched, result.LocalModelRowMatchRate)
	}
}
