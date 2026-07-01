package localmodelsummary

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	localmodel "github.com/chranama/MealCheck/internal/llm/local"
)

func TestBuildSummarizesCompletedAndFailedLocalModelArtifacts(t *testing.T) {
	root := t.TempDir()
	completedDir := filepath.Join(root, "run_completed")
	failedDir := filepath.Join(root, "run_failed")
	writeJSON(t, filepath.Join(completedDir, "manifest.json"), manifestArtifact{MealCheck: map[string]string{"version": "abc123"}})
	writeJSON(t, filepath.Join(completedDir, "decision.json"), map[string]string{"decision": "warn"})
	writeJSON(t, filepath.Join(completedDir, "optional", "local-model-chunks.json"), localmodel.LocalModelExtractionArtifact{
		SchemaVersion:   "0.1",
		PlanID:          "local-model-run_completed",
		Provider:        localmodel.RedactedProviderConfig{Type: "local_llama", Model: "Qwen3-0.6B-Q4_K_M.gguf"},
		ChunkCount:      1,
		SourceItemCount: 2,
		StageTimings:    localmodel.LocalModelExtractionStageTimings{TotalMS: 1200},
		Chunks: []localmodel.LocalModelChunkArtifact{
			{
				Index:         0,
				Day:           1,
				MealCode:      "b",
				SourceItemIDs: []int{1, 2},
				DecodedRows: []localmodel.LocalModelChunkDecodedRowArtifact{
					{SourceItemID: 1, Food: "oatmeal", Resolved: true},
					{SourceItemID: 2, Food: "blueberries", Resolved: true},
				},
				Reconciliation: localmodel.LocalModelChunkReconciliationArtifact{RepairCount: 3},
				StageTimings:   localmodel.LocalModelChunkStageTimings{ProviderRequestMS: 900, TotalMS: 1000},
			},
		},
	})
	writeJSON(t, filepath.Join(failedDir, "debug", "normalization-failure.json"), normalizationFailureArtifact{
		RunID:      "run_failed",
		FinalError: "run timed out",
		LocalModelExtraction: &localmodel.LocalModelExtractionArtifact{
			SchemaVersion:   "0.1",
			PlanID:          "local-model-run_failed",
			Provider:        localmodel.RedactedProviderConfig{Type: "local_llama", Model: "Qwen3-0.6B-Q4_K_M.gguf"},
			ChunkCount:      1,
			SourceItemCount: 1,
			FailureStage:    "decode",
			Error:           "context deadline exceeded",
			Chunks: []localmodel.LocalModelChunkArtifact{
				{
					Index:         0,
					Day:           1,
					MealCode:      "l",
					SourceItemIDs: []int{1},
					FailureStage:  "decode",
					Error:         "decode failed",
				},
			},
		},
	})

	summary, err := Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if summary.RunCount != 2 || summary.ChunkCount != 2 {
		t.Fatalf("summary counts = runs %d chunks %d, want 2/2", summary.RunCount, summary.ChunkCount)
	}
	if summary.FailedRunCount != 1 || summary.TimeoutCount != 1 {
		t.Fatalf("failure counts = failed %d timeout %d, want 1/1", summary.FailedRunCount, summary.TimeoutCount)
	}
	if summary.DecodeFailureCount != 1 || summary.RepairCount != 3 {
		t.Fatalf("decode/repair counts = %d/%d, want 1/3", summary.DecodeFailureCount, summary.RepairCount)
	}
	completed := summary.Runs[0]
	if completed.RunID != "run_completed" || completed.Status != "completed" || completed.MealCheckVersion != "abc123" {
		t.Fatalf("completed run summary = %+v", completed)
	}
	failed := summary.Runs[1]
	if failed.RunID != "run_failed" || failed.Status != "failed" || !failed.Timeout || failed.DecodeFailureCount != 1 {
		t.Fatalf("failed run summary = %+v", failed)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
