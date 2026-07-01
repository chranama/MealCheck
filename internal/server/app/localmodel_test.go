package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

func TestLocalModelRunUsesServerOwnedProvider(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.HostedMode = HostedModeLocalModel
	config.LocalModelEnabled = true
	config.LocalModelBaseURL = "http://127.0.0.1:11435/v1"
	config.LocalModelName = "/Users/chranama-server/MealCheck-data/models/Qwen3-0.6B-Q4_K_M.gguf"
	config.LocalModelMaxInputChars = 1_000
	config.LocalModelMaxOutputTokens = 160
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	provider := &fakeProvider{responses: compactLocalMealPlanJSONResponses()}
	seeded := seededCase(t, root)
	settings := localModelTestSettings(seeded.Settings)

	body := marshalJSON(t, CreateRunRequest{
		InputMode:     InputModeLocalModel,
		Settings:      settings,
		CandidateText: "Breakfast: 1 cup cooked oatmeal, 1 cup blueberries, 1 cup plain Greek yogurt.\nLunch: 4 oz chicken breast, 1 cup brown rice, 1 cup broccoli.\nDinner: 4 oz salmon, 1 cup sweet potato, 1 cup spinach.",
	})
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created CreateRunResponse
	decodeJSON(t, createResp.Body.Bytes(), &created)

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Provider, error) {
		if config.Type != ProviderTypeLocalLlama {
			t.Fatalf("provider type = %q, want %q", config.Type, ProviderTypeLocalLlama)
		}
		if config.APIKey != "" {
			t.Fatalf("local provider api key = %q, want empty", config.APIKey)
		}
		if config.BaseURL != "http://127.0.0.1:11435/v1" {
			t.Fatalf("local provider base URL = %q", config.BaseURL)
		}
		if config.Model != "/Users/chranama-server/MealCheck-data/models/Qwen3-0.6B-Q4_K_M.gguf" {
			t.Fatalf("local provider model = %q", config.Model)
		}
		if config.MaxTokens != 160 {
			t.Fatalf("local provider max tokens = %d, want 160", config.MaxTokens)
		}
		return provider, nil
	}).ProcessOne(context.Background())
	if err != nil {
		t.Fatalf("process run: %v", err)
	}
	if !processed {
		t.Fatal("expected worker to process one run")
	}
	if provider.calls != 3 {
		t.Fatalf("provider calls = %d, want 3", provider.calls)
	}

	run, err := store.GetRun(context.Background(), created.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != StatusCompleted {
		t.Fatalf("run status = %q, want completed; error=%s", run.Status, run.Error)
	}

	var redacted RedactedProviderConfig
	decodeJSON(t, readFile(t, filepath.Join(run.ArtifactDir, "configs", "redacted-provider.json")), &redacted)
	if redacted.Type != ProviderTypeLocalLlama {
		t.Fatalf("redacted provider type = %q, want %q", redacted.Type, ProviderTypeLocalLlama)
	}
	if redacted.Model != "Qwen3-0.6B-Q4_K_M.gguf" {
		t.Fatalf("redacted local model = %q, want basename only", redacted.Model)
	}
	if redacted.BaseURL != "" {
		t.Fatalf("redacted local base URL = %q, want empty", redacted.BaseURL)
	}
	if redacted.APIKey != "not_applicable" {
		t.Fatalf("redacted local api key = %q, want not_applicable", redacted.APIKey)
	}

	var events []NormalizationEvent
	decodeJSON(t, readFile(t, filepath.Join(run.ArtifactDir, "optional", "normalization-events.json")), &events)
	if !hasNormalizationEvent(events, "json_decoded") {
		t.Fatalf("normalization events missing json_decoded: %+v", events)
	}

	var chunkArtifact LocalModelExtractionArtifact
	decodeJSON(t, readFile(t, filepath.Join(run.ArtifactDir, "optional", "local-model-chunks.json")), &chunkArtifact)
	if chunkArtifact.SchemaVersion != "0.1" {
		t.Fatalf("chunk artifact schema_version = %q, want 0.1", chunkArtifact.SchemaVersion)
	}
	if chunkArtifact.ChunkCount != 3 || chunkArtifact.SourceItemCount != 9 || len(chunkArtifact.Chunks) != 3 {
		t.Fatalf("chunk artifact counts = chunks:%d sources:%d len:%d, want 3/9/3", chunkArtifact.ChunkCount, chunkArtifact.SourceItemCount, len(chunkArtifact.Chunks))
	}
	if chunkArtifact.Provider.Type != ProviderTypeLocalLlama || chunkArtifact.Provider.APIKey != "not_applicable" {
		t.Fatalf("chunk artifact provider = %+v, want redacted local llama", chunkArtifact.Provider)
	}
	if chunkArtifact.RepeatRunInstability.Measured {
		t.Fatalf("chunk artifact repeat instability should not be measured for a single hosted run: %+v", chunkArtifact.RepeatRunInstability)
	}
	breakfast := chunkArtifact.Chunks[0]
	if breakfast.MealCode != "b" || breakfast.MealLabel != "breakfast" {
		t.Fatalf("breakfast chunk meal = %s/%s, want b/breakfast", breakfast.MealCode, breakfast.MealLabel)
	}
	if got := fmt.Sprint(breakfast.SourceItemIDs); got != "[1 2 3]" {
		t.Fatalf("breakfast source ids = %s, want [1 2 3]", got)
	}
	if breakfast.Prompt.MessageCount != 2 || !strings.Contains(breakfast.Prompt.Messages[1].Content, "Meal text:") {
		t.Fatalf("breakfast prompt artifact = %+v, want system/user prompt with meal text", breakfast.Prompt)
	}
	if !strings.Contains(breakfast.RawOutput, `"cooked oatmeal"`) {
		t.Fatalf("breakfast raw compact output = %q, want cooked oatmeal row", breakfast.RawOutput)
	}
	if len(breakfast.DecodedRows) != 3 || !breakfast.DecodedRows[0].Resolved || breakfast.DecodedRows[0].Food != "cooked oatmeal" {
		t.Fatalf("breakfast decoded rows = %+v, want resolved cooked oatmeal row", breakfast.DecodedRows)
	}
	if got := fmt.Sprint(breakfast.Reconciliation.DecodedSourceItemIDs); got != "[1 2 3]" {
		t.Fatalf("breakfast decoded source ids = %s, want [1 2 3]", got)
	}
	if breakfast.StageTimings.PromptBuildMS < 0 || breakfast.StageTimings.ProviderRequestMS < 0 || breakfast.StageTimings.DecodeReconcileMS < 0 || breakfast.StageTimings.TotalMS < 0 {
		t.Fatalf("breakfast stage timings must be non-negative: %+v", breakfast.StageTimings)
	}

	manifestBytes := readFile(t, filepath.Join(run.ArtifactDir, "manifest.json"))
	if !bytes.Contains(manifestBytes, []byte("optional/local-model-chunks.json")) {
		t.Fatalf("manifest missing optional/local-model-chunks.json:\n%s", string(manifestBytes))
	}
}

func TestLocalModelRunFastFailsNonVerifiableTextBeforeQueue(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.HostedMode = HostedModeLocalModel
	config.LocalModelEnabled = true
	config.LocalModelBaseURL = "http://127.0.0.1:11435/v1"
	config.LocalModelName = "/Users/chranama-server/MealCheck-data/models/Qwen3-0.6B-Q4_K_M.gguf"
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	settings := localModelTestSettings(seededCase(t, root).Settings)

	tests := []struct {
		name       string
		text       string
		wantStatus string
	}{
		{
			name:       "non meal text",
			text:       "Please draft a short meeting agenda for tomorrow afternoon.",
			wantStatus: QualificationStatusNotMealPlan,
		},
		{
			name:       "recipe needs decomposition",
			text:       "Make a healthy chicken bowl with rice, vegetables, and sauce. Cook until warm.",
			wantStatus: QualificationStatusRecipeOrMenuNeedsDecompose,
		},
		{
			name:       "source items missing meal labels",
			text:       "1 cup cooked oatmeal, 1 cup blueberries, and 4 oz chicken breast.",
			wantStatus: QualificationStatusMealPlanTooVague,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := marshalJSON(t, CreateRunRequest{
				InputMode:     InputModeLocalModel,
				Settings:      settings,
				CandidateText: tt.text,
			})
			resp := doRequest(t, server, http.MethodPost, "/api/runs", body)
			if resp.Code != http.StatusUnprocessableEntity {
				t.Fatalf("create status = %d, want 422 body=%s", resp.Code, resp.Body.String())
			}
			var errorResponse ErrorResponse
			decodeJSON(t, resp.Body.Bytes(), &errorResponse)
			if errorResponse.Error.Code != "meal_plan_not_verifiable" {
				t.Fatalf("error code = %q, want meal_plan_not_verifiable", errorResponse.Error.Code)
			}
			qualificationValue, ok := errorResponse.Error.Details["qualification"]
			if !ok {
				t.Fatalf("error details missing qualification: %+v", errorResponse.Error.Details)
			}
			qualificationBytes, err := json.Marshal(qualificationValue)
			if err != nil {
				t.Fatalf("marshal qualification detail: %v", err)
			}
			var qualification MealPlanQualificationResult
			decodeJSON(t, qualificationBytes, &qualification)
			if qualification.Status != tt.wantStatus {
				t.Fatalf("qualification status = %q, want %q", qualification.Status, tt.wantStatus)
			}
			if qualification.ProviderUsed {
				t.Fatal("provider_used = true, want false")
			}
			if pending.Count() != 0 {
				t.Fatalf("pending count = %d, want 0", pending.Count())
			}
			stats, err := store.Stats(context.Background())
			if err != nil {
				t.Fatalf("store stats: %v", err)
			}
			if stats.Queued != 0 || stats.Running != 0 {
				t.Fatalf("store stats queued/running = %d/%d, want 0/0", stats.Queued, stats.Running)
			}
		})
	}
}

func TestLocalModelRunAcceptsMissingQuantitiesAsUnresolvedRows(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.HostedMode = HostedModeLocalModel
	config.LocalModelEnabled = true
	config.LocalModelBaseURL = "http://127.0.0.1:11435/v1"
	config.LocalModelName = "/Users/chranama-server/MealCheck-data/models/Qwen3-0.6B-Q4_K_M.gguf"
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	provider := &fakeProvider{responses: []string{
		`{"i":[[1,"oatmeal",null,"","missing quantity","missing_quantity"]]}`,
		`{"i":[[2,"salad",null,"","missing quantity","missing_quantity"]]}`,
		`{"i":[[3,"chicken bowl",null,"","missing quantity","missing_quantity"]]}`,
	}}
	settings := localModelTestSettings(seededCase(t, root).Settings)

	body := marshalJSON(t, CreateRunRequest{
		InputMode:     InputModeLocalModel,
		Settings:      settings,
		CandidateText: "Breakfast: oatmeal\nLunch: salad\nDinner: chicken bowl",
	})
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created CreateRunResponse
	decodeJSON(t, createResp.Body.Bytes(), &created)

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Provider, error) {
		return provider, nil
	}).ProcessOne(context.Background())
	if err != nil {
		t.Fatalf("process run: %v", err)
	}
	if !processed {
		t.Fatal("expected worker to process one run")
	}
	if provider.calls != 3 {
		t.Fatalf("provider calls = %d, want 3", provider.calls)
	}
	run, err := store.GetRun(context.Background(), created.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != StatusCompleted {
		t.Fatalf("run status = %q, want completed; error=%s", run.Status, run.Error)
	}
	var plan checker.Plan
	decodeJSON(t, readFile(t, filepath.Join(run.ArtifactDir, "normalized-plan.json")), &plan)
	if got := countMealPlanItems(plan); got != 3 {
		t.Fatalf("plan item count = %d, want 3", got)
	}
	item := plan.Days[0].Meals[0].Items[0]
	if item.Quantity != nil || item.QuantityText != "missing quantity" || item.ResolutionStatus != "unresolved" || item.UnresolvedReason != "missing_quantity" {
		t.Fatalf("unresolved item = %+v", item)
	}
}

func TestLocalModelRunRejectsClearMultiDayInputBeforeQueue(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.HostedMode = HostedModeLocalModel
	config.LocalModelEnabled = true
	config.LocalModelBaseURL = "http://127.0.0.1:11435/v1"
	config.LocalModelName = "/Users/chranama-server/MealCheck-data/models/Qwen3-0.6B-Q4_K_M.gguf"
	config.LocalModelMaxInputChars = 2_000
	config.LocalModelMaxOutputTokens = 160
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	seeded := seededCase(t, root)
	settings := localModelTestSettings(seeded.Settings)

	body := marshalJSON(t, CreateRunRequest{
		InputMode: InputModeLocalModel,
		Settings:  settings,
		CandidateText: strings.Join([]string{
			"Day 1 breakfast: 1 cup cooked oatmeal, 1 cup blueberries, and 1 cup plain Greek yogurt.",
			"Day 1 lunch: 4 oz chicken breast, 1 cup brown rice, and 1 cup broccoli.",
			"Day 1 dinner: 4 oz salmon, 1 cup sweet potato, and 1 cup spinach.",
			"Day 2 breakfast: 2 eggs, 1 cup whole wheat toast, and 1 cup orange segments.",
			"Day 2 lunch: 4 oz tuna, 2 cups mixed greens, and 1 tsp vinaigrette.",
			"Day 2 dinner: 5 oz turkey meatballs, 1 cup whole wheat pasta, and 1 cup tomato sauce.",
		}, "\n"),
	})
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("create status = %d, want 422 body=%s", createResp.Code, createResp.Body.String())
	}
	var errorResponse ErrorResponse
	decodeJSON(t, createResp.Body.Bytes(), &errorResponse)
	if errorResponse.Error.Code != "meal_plan_not_verifiable" || !strings.Contains(errorResponse.Error.Message, "one day") {
		t.Fatalf("error response = %+v, want one-day meal_plan_not_verifiable", errorResponse.Error)
	}
	qualification := qualificationFromErrorResponse(t, errorResponse)
	if qualification.Status != QualificationStatusOutsideHostedContract {
		t.Fatalf("qualification status = %q, want %q", qualification.Status, QualificationStatusOutsideHostedContract)
	}
	if qualification.ProviderUsed {
		t.Fatal("provider_used = true, want false")
	}
	if pending.Count() != 0 {
		t.Fatalf("pending count = %d, want 0", pending.Count())
	}
	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatalf("store stats: %v", err)
	}
	if stats.Queued != 0 || stats.Running != 0 {
		t.Fatalf("store stats queued/running = %d/%d, want 0/0", stats.Queued, stats.Running)
	}
}

func TestLocalModelRunRejectsSourceItemOverflowBeforeQueue(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.HostedMode = HostedModeLocalModel
	config.LocalModelEnabled = true
	config.LocalModelBaseURL = "http://127.0.0.1:11435/v1"
	config.LocalModelName = "/Users/chranama-server/MealCheck-data/models/Qwen3-0.6B-Q4_K_M.gguf"
	config.LocalModelMaxSourceItems = 2
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	settings := localModelTestSettings(seededCase(t, root).Settings)

	body := marshalJSON(t, CreateRunRequest{
		InputMode: InputModeLocalModel,
		Settings:  settings,
		CandidateText: strings.Join([]string{
			"Day 1 breakfast:",
			"- 1 cup cooked oatmeal",
			"- 1 cup blueberries",
			"- 1 cup plain Greek yogurt",
		}, "\n"),
	})
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("create status = %d, want 422 body=%s", createResp.Code, createResp.Body.String())
	}
	var errorResponse ErrorResponse
	decodeJSON(t, createResp.Body.Bytes(), &errorResponse)
	if errorResponse.Error.Code != "meal_plan_not_verifiable" || !strings.Contains(errorResponse.Error.Message, "at most 2") {
		t.Fatalf("error response = %+v, want source-cap meal_plan_not_verifiable", errorResponse.Error)
	}
	qualification := qualificationFromErrorResponse(t, errorResponse)
	if qualification.Status != QualificationStatusOutsideHostedContract {
		t.Fatalf("qualification status = %q, want %q", qualification.Status, QualificationStatusOutsideHostedContract)
	}
	if pending.Count() != 0 {
		t.Fatalf("pending count = %d, want 0", pending.Count())
	}
	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatalf("store stats: %v", err)
	}
	if stats.Queued != 0 || stats.Running != 0 {
		t.Fatalf("store stats queued/running = %d/%d, want 0/0", stats.Queued, stats.Running)
	}
}

func TestLocalModelRunReportsFriendlyPostModelNormalizationFailure(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.HostedMode = HostedModeLocalModel
	config.LocalModelEnabled = true
	config.LocalModelBaseURL = "http://127.0.0.1:11435/v1"
	config.LocalModelName = "/Users/chranama-server/MealCheck-data/models/Qwen3-0.6B-Q4_K_M.gguf"
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	provider := &fakeProvider{responses: []string{"this is not compact json"}}
	settings := localModelTestSettings(seededCase(t, root).Settings)

	body := marshalJSON(t, CreateRunRequest{
		InputMode:     InputModeLocalModel,
		Settings:      settings,
		CandidateText: "Breakfast: 1 cup cooked oatmeal, 1 cup blueberries, 1 cup plain Greek yogurt.\nLunch: 4 oz chicken breast, 1 cup brown rice, 1 cup broccoli.\nDinner: 4 oz salmon, 1 cup sweet potato, 1 cup spinach.",
	})
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created CreateRunResponse
	decodeJSON(t, createResp.Body.Bytes(), &created)

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Provider, error) {
		return provider, nil
	}).ProcessOne(context.Background())
	if err == nil {
		t.Fatal("process run error = nil, want normalization failure")
	}
	if !processed {
		t.Fatal("expected worker to process one run")
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	run, err := store.GetRun(context.Background(), created.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != StatusFailed {
		t.Fatalf("run status = %q, want failed", run.Status)
	}
	for _, want := range []string{"could not normalize", "day labels", "meal labels", "quantities"} {
		if !strings.Contains(run.Error, want) {
			t.Fatalf("run error = %q, want %q", run.Error, want)
		}
	}
	for _, absent := range []string{"compact", "JSON", "source item", "invalid character"} {
		if strings.Contains(run.Error, absent) {
			t.Fatalf("run error = %q, should not expose %q", run.Error, absent)
		}
	}
	debugBytes := readFile(t, filepath.Join(run.ArtifactDir, "debug", "normalization-failure.json"))
	var debug normalizationFailureArtifact
	decodeJSON(t, debugBytes, &debug)
	if !hasNormalizationEvent(debug.NormalizationEvents, "json_decode_failed") || !hasNormalizationEvent(debug.NormalizationEvents, "normalization_graceful_failed") {
		t.Fatalf("debug events missing graceful local model failure events: %+v", debug.NormalizationEvents)
	}
	if !strings.Contains(debug.FinalError, "no JSON object found") {
		t.Fatalf("debug final error = %q, want decode detail", debug.FinalError)
	}
	if debug.LocalModelExtraction == nil {
		t.Fatal("debug artifact missing local model extraction evidence")
	}
	if debug.LocalModelExtraction.FailureStage != "decode" {
		t.Fatalf("debug local model failure stage = %q, want decode", debug.LocalModelExtraction.FailureStage)
	}
	if len(debug.LocalModelExtraction.Chunks) != 1 {
		t.Fatalf("debug local model chunk count = %d, want failed first chunk only", len(debug.LocalModelExtraction.Chunks))
	}
	failedChunk := debug.LocalModelExtraction.Chunks[0]
	if failedChunk.FailureStage != "decode" || !strings.Contains(failedChunk.RawOutput, "this is not compact json") {
		t.Fatalf("failed chunk artifact = %+v, want decode failure with raw output", failedChunk)
	}
	if failedChunk.Prompt.MessageCount != 2 || !strings.Contains(failedChunk.Prompt.Messages[1].Content, "Source items:") {
		t.Fatalf("failed chunk prompt artifact = %+v, want meal prompt evidence", failedChunk.Prompt)
	}
}
