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
	if result.LocalModelFoodAccuracy != 1 || result.LocalModelQuantityAccuracy != 1 || result.LocalModelUnitAccuracy != 1 {
		t.Fatalf("local model field accuracies = food %.3f quantity %.3f unit %.3f, want all 1", result.LocalModelFoodAccuracy, result.LocalModelQuantityAccuracy, result.LocalModelUnitAccuracy)
	}
}

func TestRunLocalLlamaModeRecordsSourceRepairs(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data", "evaluation", "p0-normalization")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	manifestPath := filepath.Join(dataDir, "manifest.json")
	datasetPath := filepath.Join(dataDir, "cases-v1.jsonl")
	failurePath := filepath.Join(dataDir, "failure-cases-v1.jsonl")
	if err := os.WriteFile(manifestPath, []byte(`{"schema_version":"0.1","dataset_id":"p0-local-repair-test"}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	successCase := `{"schema_version":"0.1","id":"repair-item","source_dataset":"test","input_text":"Day 1 breakfast:\n- 1/2 cup blueberries\n- 1 tbsp olive oil","expected":{"days":[1],"source_items":[{"source_item_id":1,"day":1,"meal_code":"b","source_text":"1/2 cup blueberries","food":"blueberries","quantity":0.5,"unit":"cup"},{"source_item_id":2,"day":1,"meal_code":"b","source_text":"1 tbsp olive oil","food":"olive oil","quantity":1,"unit":"tbsp"}]},"tags":["unit_test"]}` + "\n"
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
		ProviderFactory: hosted.StaticResponseProviderFactory(`{"i":[[2,1,"b","1 tbsp olive oil",1,"tsp"],[1,1,"b","1/2 cup blueberries",1,"cup"]]}`),
	})
	if err != nil {
		t.Fatalf("run local llama eval: %v", err)
	}
	if result.CasesPassed != 1 || result.CasesWithMismatches != 0 {
		t.Fatalf("unexpected result counts: passed=%d mismatches=%d messages=%+v", result.CasesPassed, result.CasesWithMismatches, result.Mismatches)
	}
	if result.LocalModelSourceRepairs == 0 || result.LocalModelRepairCases != 1 {
		t.Fatalf("repair metrics = repairs %d cases %d, want repairs > 0 and cases 1", result.LocalModelSourceRepairs, result.LocalModelRepairCases)
	}
	if result.LocalModelRowsMatched != 2 || result.LocalModelRowMatchRate != 1 {
		t.Fatalf("local model row match = %d rate %.3f, want 2 and 1", result.LocalModelRowsMatched, result.LocalModelRowMatchRate)
	}
}

func TestRunManifestGateAndSourceDatasetFilters(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data", "evaluation", "p0-normalization")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	manifestPath := filepath.Join(dataDir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(`{
		"schema_version":"0.1",
		"dataset_id":"p0-manifest-filter-test",
		"case_files":[
			{"path":"strict-cases.jsonl","source_dataset":"mealcheck_input_robustness","gate":"strict"},
			{"path":"nyt-cases.jsonl","source_dataset":"nyt_ingredient_phrase_tagger","gate":"exploratory"}
		],
		"failure_case_files":[
			{"path":"strict-failures.jsonl","source_dataset":"mealcheck_input_robustness","gate":"strict"},
			{"path":"nyt-failures.jsonl","source_dataset":"nyt_ingredient_phrase_tagger","gate":"exploratory"}
		],
		"quarantine_files":[
			{"path":"nyt-quarantine.jsonl","source_dataset":"nyt_ingredient_phrase_tagger","gate":"exploratory"}
		]
	}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	strictCase := `{"schema_version":"0.1","id":"strict-item","source_dataset":"mealcheck_input_robustness","input_text":"Day 1 breakfast:\n- 1 cup oatmeal","expected":{"days":[1],"source_items":[{"source_item_id":1,"day":1,"meal_code":"b","source_text":"1 cup oatmeal","food":"oatmeal","quantity":1,"unit":"cup"}]},"tags":["strict"]}` + "\n"
	nytCase := `{"schema_version":"0.1","id":"nyt-item","source_dataset":"nyt_ingredient_phrase_tagger","input_text":"Day 1 breakfast:\n- 2 tbsp olive oil","expected":{"days":[1],"source_items":[{"source_item_id":1,"day":1,"meal_code":"b","source_text":"2 tbsp olive oil","food":"olive oil","quantity":2,"unit":"tbsp"}]},"tags":["external"]}` + "\n"
	if err := os.WriteFile(filepath.Join(dataDir, "strict-cases.jsonl"), []byte(strictCase), 0o644); err != nil {
		t.Fatalf("write strict cases: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "nyt-cases.jsonl"), []byte(nytCase), 0o644); err != nil {
		t.Fatalf("write nyt cases: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "strict-failures.jsonl"), nil, 0o644); err != nil {
		t.Fatalf("write strict failures: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "nyt-failures.jsonl"), nil, 0o644); err != nil {
		t.Fatalf("write nyt failures: %v", err)
	}
	quarantine := `{"schema_version":"0.1","id":"nyt-quarantine-1","source_dataset":"nyt_ingredient_phrase_tagger","raw_text":"optional parsley","quarantine_reason":"optional_or_alternative"}` + "\n"
	if err := os.WriteFile(filepath.Join(dataDir, "nyt-quarantine.jsonl"), []byte(quarantine), 0o644); err != nil {
		t.Fatalf("write quarantine: %v", err)
	}

	strictResult, err := run(runOptions{
		Root:         root,
		ManifestPath: manifestPath,
		Gate:         "strict",
		Mode:         "deterministic",
	})
	if err != nil {
		t.Fatalf("run strict eval: %v", err)
	}
	if strictResult.TotalCases != 1 || strictResult.SuccessCases != 1 {
		t.Fatalf("strict counts = total %d success %d, want 1/1", strictResult.TotalCases, strictResult.SuccessCases)
	}
	if len(strictResult.GateSummary) != 1 || strictResult.GateSummary[0].Gate != "strict" {
		t.Fatalf("strict gate summary = %+v, want only strict", strictResult.GateSummary)
	}

	externalResult, err := run(runOptions{
		Root:          root,
		ManifestPath:  manifestPath,
		Gate:          "exploratory",
		SourceDataset: "nyt_ingredient_phrase_tagger",
		Mode:          "deterministic",
	})
	if err != nil {
		t.Fatalf("run exploratory eval: %v", err)
	}
	if externalResult.TotalCases != 1 || externalResult.SuccessCases != 1 {
		t.Fatalf("external counts = total %d success %d, want 1/1", externalResult.TotalCases, externalResult.SuccessCases)
	}
	if externalResult.QuarantineSummary.Rows != 1 {
		t.Fatalf("quarantine rows = %d, want 1", externalResult.QuarantineSummary.Rows)
	}
	if len(externalResult.SourceDatasetSummary) != 1 || externalResult.SourceDatasetSummary[0].SourceDataset != "nyt_ingredient_phrase_tagger" {
		t.Fatalf("source dataset summary = %+v, want only NYT", externalResult.SourceDatasetSummary)
	}
}
