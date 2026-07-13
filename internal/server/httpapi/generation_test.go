package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

func TestProfileGenerationUsesBYOKProviderAndRedactsSecret(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	seeded := seededCase(t, root)
	provider := &fakeProvider{responses: []string{string(readFile(t, filepath.Join(root, "examples/seeded-one-day-peanut-allergy/plans/candidate.json")))}}
	secret := "sk-test-secret"

	body := marshalJSON(t, CreateRunRequest{
		InputMode: "profile_generation",
		Settings:  seeded.Settings,
		Provider: ProviderConfig{
			Type:   "openai_compatible",
			Model:  "fake-model",
			APIKey: secret,
		},
	})
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created CreateRunResponse
	decodeJSON(t, createResp.Body.Bytes(), &created)

	worker := NewWorker(config, store, pending, func(config ProviderConfig) (Completer, error) {
		return provider, nil
	})
	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatalf("process run: %v", err)
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
	if run.Status != StatusCompleted {
		t.Fatalf("run status = %q, want completed", run.Status)
	}

	var redacted RedactedProviderConfig
	decodeJSON(t, readFile(t, filepath.Join(run.ArtifactDir, "configs", "redacted-provider.json")), &redacted)
	if redacted.APIKey != "redacted" {
		t.Fatalf("redacted provider api_key = %q, want redacted", redacted.APIKey)
	}
	assertFileTreeDoesNotContain(t, config.DataDir, secret)

	artifactListResp := doRequest(t, server, http.MethodGet, "/api/runs/"+created.RunID+"/artifacts", "")
	if artifactListResp.Code != http.StatusOK {
		t.Fatalf("artifact list status = %d body=%s", artifactListResp.Code, artifactListResp.Body.String())
	}
	for _, expected := range []string{"optional/llm-output.json", "optional/normalization-events.json"} {
		if !strings.Contains(artifactListResp.Body.String(), expected) {
			t.Fatalf("artifact list missing %q: %s", expected, artifactListResp.Body.String())
		}
	}
}

func TestProfileGenerationRedactsSuccessfulLLMOutputArtifact(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	seeded := seededCase(t, root)
	secret := "sk-success-output-secret"
	candidate := string(readFile(t, filepath.Join(root, "examples/seeded-one-day-peanut-allergy/plans/candidate.json")))
	provider := &fakeProvider{responses: []string{secret + "\n" + candidate}}

	body := marshalJSON(t, CreateRunRequest{
		InputMode: "profile_generation",
		Settings:  seeded.Settings,
		Provider: ProviderConfig{
			Type:   ProviderTypeOpenAI,
			Model:  "fake-model",
			APIKey: secret,
		},
	})
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created CreateRunResponse
	decodeJSON(t, createResp.Body.Bytes(), &created)

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Completer, error) {
		return provider, nil
	}).ProcessOne(context.Background())
	if err != nil {
		t.Fatalf("process run: %v", err)
	}
	if !processed {
		t.Fatal("expected worker to process one run")
	}
	run, err := store.GetRun(context.Background(), created.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != StatusCompleted {
		t.Fatalf("run status = %q, want completed", run.Status)
	}
	llmOutputBytes := readFile(t, filepath.Join(run.ArtifactDir, "optional", "llm-output.json"))
	if bytes.Contains(llmOutputBytes, []byte(secret)) {
		t.Fatalf("llm output artifact contains provider secret:\n%s", string(llmOutputBytes))
	}
	var llmOutput struct {
		Output string `json:"output"`
	}
	decodeJSON(t, llmOutputBytes, &llmOutput)
	if !strings.Contains(llmOutput.Output, "[redacted]") {
		t.Fatalf("llm output artifact missing redaction marker: %s", llmOutput.Output)
	}
	assertFileTreeDoesNotContain(t, config.DataDir, secret)
}

func TestHostedManualStructuredRunIsRejected(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	seeded := seededCase(t, root)

	body := marshalJSON(t, CreateRunRequest{
		InputMode:     "manual_structured",
		Settings:      seeded.Settings,
		CandidatePlan: ptr(seededPlan(t, root)),
	})
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want 400 body=%s", createResp.Code, createResp.Body.String())
	}
	if !strings.Contains(createResp.Body.String(), "local CLI/debug workflow") {
		t.Fatalf("manual rejection body missing local CLI/debug guidance: %s", createResp.Body.String())
	}
	if pending.Count() != 0 {
		t.Fatalf("pending count = %d, want 0", pending.Count())
	}
}

func TestPromptGenerationAllowsOneBoundedRepair(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	seeded := seededCase(t, root)
	provider := &fakeProvider{responses: []string{
		"this is not json",
		string(readFile(t, filepath.Join(root, "examples/seeded-one-day-peanut-allergy/plans/candidate.json"))),
	}}

	body := marshalJSON(t, CreateRunRequest{
		InputMode:        "prompt_generation",
		Settings:         seeded.Settings,
		GenerationPrompt: "Create a simple 3 day meal plan that avoids shellfish.",
		Provider: ProviderConfig{
			Type:   "openai_compatible",
			Model:  "fake-model",
			APIKey: "sk-repair-secret",
		},
	})
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created CreateRunResponse
	decodeJSON(t, createResp.Body.Bytes(), &created)

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Completer, error) {
		return provider, nil
	}).ProcessOne(context.Background())
	if err != nil {
		t.Fatalf("process run: %v", err)
	}
	if !processed {
		t.Fatal("expected worker to process one run")
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want initial call plus one repair", provider.calls)
	}

	run, err := store.GetRun(context.Background(), created.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	var events []NormalizationEvent
	decodeJSON(t, readFile(t, filepath.Join(run.ArtifactDir, "optional", "normalization-events.json")), &events)
	if !hasNormalizationEvent(events, "json_decode_failed") || !hasNormalizationEvent(events, "repair_attempted") || !hasNormalizationEvent(events, "repair_succeeded") {
		t.Fatalf("normalization events missing repair lifecycle: %+v", events)
	}
}

func TestPromptGenerationRepairsGeneratedPlanCountMismatch(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	seeded := seededCase(t, root)
	seeded.Settings.VerificationConstraints.Days = 1
	seeded.Settings.VerificationConstraints.MealsPerDay = 3
	provider := &fakeProvider{responses: []string{
		`{"schema_version":"0.1","plan_id":"one-meal","days":[{"day":1,"meals":[{"name":"breakfast","items":[{"food":"cooked oatmeal","quantity":1,"unit":"cup"}]}]}]}`,
		`{"schema_version":"0.1","plan_id":"three-meals","days":[{"day":1,"meals":[{"name":"breakfast","items":[{"food":"cooked oatmeal","quantity":1,"unit":"cup"}]},{"name":"lunch","items":[{"food":"chicken breast","quantity":4,"unit":"oz"}]},{"name":"dinner","items":[{"food":"salmon","quantity":4,"unit":"oz"}]}]}]}`,
	}}

	body := marshalJSON(t, CreateRunRequest{
		InputMode:        "prompt_generation",
		Settings:         seeded.Settings,
		GenerationPrompt: "Create a simple 1 day meal plan.",
		Provider: ProviderConfig{
			Type:   "openai_compatible",
			Model:  "fake-model",
			APIKey: "sk-count-repair-secret",
		},
	})
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created CreateRunResponse
	decodeJSON(t, createResp.Body.Bytes(), &created)

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Completer, error) {
		return provider, nil
	}).ProcessOne(context.Background())
	if err != nil {
		t.Fatalf("process run: %v", err)
	}
	if !processed {
		t.Fatal("expected worker to process one run")
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want initial response plus one repair", provider.calls)
	}

	run, err := store.GetRun(context.Background(), created.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != StatusCompleted {
		t.Fatalf("run status = %q, want completed", run.Status)
	}
	var events []NormalizationEvent
	decodeJSON(t, readFile(t, filepath.Join(run.ArtifactDir, "optional", "normalization-events.json")), &events)
	for _, eventType := range []string{"plan_constraints_failed", "repair_attempted", "repair_succeeded"} {
		if !hasNormalizationEvent(events, eventType) {
			t.Fatalf("normalization events missing %q: %+v", eventType, events)
		}
	}
	var plan checker.Plan
	decodeJSON(t, readFile(t, filepath.Join(run.ArtifactDir, "normalized-plan.json")), &plan)
	if len(plan.Days) != 1 || len(plan.Days[0].Meals) != 3 {
		t.Fatalf("normalized plan days/meals = %d/%d, want 1/3", len(plan.Days), len(plan.Days[0].Meals))
	}
}

func TestPromptGenerationMarksUnsupportedUnitsUnresolved(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	seeded := seededCase(t, root)
	seeded.Settings.VerificationConstraints.Days = 1
	seeded.Settings.VerificationConstraints.MealsPerDay = 3
	provider := &fakeProvider{responses: []string{
		`{"schema_version":"0.1","plan_id":"unsupported-unit","days":[{"day":1,"meals":[{"name":"breakfast","items":[{"food":"Whole Wheat Bread","quantity":1,"unit":"loaf"}]},{"name":"lunch","items":[{"food":"chicken breast","quantity":4,"unit":"oz"}]},{"name":"dinner","items":[{"food":"broccoli","quantity":1,"unit":"cup"}]}]}]}`,
	}}

	body := marshalJSON(t, CreateRunRequest{
		InputMode:        "prompt_generation",
		Settings:         seeded.Settings,
		GenerationPrompt: "Create a simple 1 day meal plan.",
		Provider: ProviderConfig{
			Type:   "openai_compatible",
			Model:  "fake-model",
			APIKey: "sk-unsupported-unit-secret",
		},
	})
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created CreateRunResponse
	decodeJSON(t, createResp.Body.Bytes(), &created)

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Completer, error) {
		return provider, nil
	}).ProcessOne(context.Background())
	if err != nil {
		t.Fatalf("process run: %v", err)
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
	if run.Status != StatusCompleted {
		t.Fatalf("run status = %q, want completed", run.Status)
	}
	if run.Decision != "block" {
		t.Fatalf("run decision = %q, want block", run.Decision)
	}
	var events []NormalizationEvent
	decodeJSON(t, readFile(t, filepath.Join(run.ArtifactDir, "optional", "normalization-events.json")), &events)
	if !hasNormalizationEvent(events, "unsupported_units_marked_unresolved") {
		t.Fatalf("normalization events missing unsupported_units_marked_unresolved: %+v", events)
	}
	var plan checker.Plan
	decodeJSON(t, readFile(t, filepath.Join(run.ArtifactDir, "normalized-plan.json")), &plan)
	item := plan.Days[0].Meals[0].Items[0]
	if item.Quantity != nil || item.Unit != "" || item.QuantityText != "1 loaf" || item.ResolutionStatus != "unresolved" || item.UnresolvedReason != "unsupported_unit" {
		t.Fatalf("unsupported unit item was not preserved as unresolved: %+v", item)
	}
	var decision checker.DecisionDocument
	decodeJSON(t, readFile(t, filepath.Join(run.ArtifactDir, "decision.json")), &decision)
	if len(decision.UnresolvedItems) != 1 {
		t.Fatalf("len(unresolved_items) = %d, want 1: %+v", len(decision.UnresolvedItems), decision.UnresolvedItems)
	}
	unresolved := decision.UnresolvedItems[0]
	if unresolved.Food != "Whole Wheat Bread" || unresolved.QuantityText != "1 loaf" || unresolved.UnresolvedReason != "unsupported_unit" {
		t.Fatalf("unexpected unresolved item: %+v", unresolved)
	}
}

func TestPromptGenerationFailsWithoutRepairAfterInvalidJSON(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	seeded := seededCase(t, root)
	repairJSON := false
	provider := &fakeProvider{responses: []string{"not json"}}

	body := marshalJSON(t, CreateRunRequest{
		InputMode:        "prompt_generation",
		Settings:         seeded.Settings,
		GenerationPrompt: "Create a simple 3 day meal plan.",
		Provider: ProviderConfig{
			Type:   "openai_compatible",
			Model:  "fake-model",
			APIKey: "sk-no-repair-secret",
		},
		RepairJSON: &repairJSON,
	})
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created CreateRunResponse
	decodeJSON(t, createResp.Body.Bytes(), &created)

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Completer, error) {
		return provider, nil
	}).ProcessOne(context.Background())
	if err == nil {
		t.Fatal("expected invalid JSON without repair to fail")
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
}

func TestDecodePlanTextCanonicalizesKnownProviderAliases(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		wantMeal  string
		wantFood  string
		wantItems int
	}{
		{
			name: "food item alias",
			text: `{
				"schema_version": "0.1",
				"plan_id": "alias-food-item",
				"days": [{"day": 1, "meals": [{"name": "breakfast", "items": [{"food_item": "plain oatmeal", "quantity": 1, "unit": "cup"}]}]}]
			}`,
			wantMeal:  "breakfast",
			wantFood:  "plain oatmeal",
			wantItems: 1,
		},
		{
			name: "meal type alias",
			text: `{
				"schema_version": "0.1",
				"plan_id": "alias-meal-type",
				"days": [{"day": 1, "meals": [{"meal_type": "lunch", "items": [{"food": "brown rice", "quantity": 1, "unit": "cup"}]}]}]
			}`,
			wantMeal:  "lunch",
			wantFood:  "brown rice",
			wantItems: 1,
		},
		{
			name: "meal alias",
			text: `{
				"schema_version": "0.1",
				"plan_id": "alias-meal",
				"days": [{"day": 1, "meals": [{"meal": "dinner", "items": [{"food": "salmon", "quantity": 4, "unit": "oz"}]}]}]
			}`,
			wantMeal:  "dinner",
			wantFood:  "salmon",
			wantItems: 1,
		},
		{
			name: "food items array and item alias",
			text: `{
				"schema_version": "0.1",
				"plan_id": "alias-food-items",
				"days": [{"day": 1, "meals": [{"name": "snack", "food_items": [{"item": "apple", "quantity": 1, "unit": "serving"}]}]}]
			}`,
			wantMeal:  "snack",
			wantFood:  "apple",
			wantItems: 1,
		},
		{
			name: "foods array and item-level name alias",
			text: `{
				"schema_version": "0.1",
				"plan_id": "alias-foods",
				"days": [{"day": 1, "meals": [{"name": "snack", "foods": [{"name": "carrots", "quantity": 1, "unit": "serving"}]}]}]
			}`,
			wantMeal:  "snack",
			wantFood:  "carrots",
			wantItems: 1,
		},
		{
			name: "ingredients array alias",
			text: `{
				"schema_version": "0.1",
				"plan_id": "alias-ingredients",
				"days": [{"day": 1, "meals": [{"name": "lunch", "ingredients": [{"food": "lentils", "quantity": 1, "unit": "cup"}]}]}]
			}`,
			wantMeal:  "lunch",
			wantFood:  "lentils",
			wantItems: 1,
		},
		{
			name: "nullable provider schema fields",
			text: `{
				"schema_version": "0.1",
				"plan_id": "nullable-fields",
				"description": "test",
				"days": [{"day": 1, "meals": [{"name": "breakfast", "items": [{
					"food": "plain oatmeal",
					"quantity": 1,
					"unit": "cup",
					"quantity_text": null,
					"preparation": null,
					"brand": null,
					"resolution_status": null,
					"unresolved_reason": null
				}]}]}],
				"shopping_list": [],
				"prep_notes": []
			}`,
			wantMeal:  "breakfast",
			wantFood:  "plain oatmeal",
			wantItems: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := decodePlanTextDetailed(tt.text)
			if err != nil {
				t.Fatalf("decodePlanTextDetailed error: %v", err)
			}
			if !result.Canonicalized {
				t.Fatal("Canonicalized = false, want true")
			}
			meal := result.Plan.Days[0].Meals[0]
			if meal.Name != tt.wantMeal {
				t.Fatalf("meal name = %q, want %q", meal.Name, tt.wantMeal)
			}
			if len(meal.Items) != tt.wantItems {
				t.Fatalf("items = %d, want %d", len(meal.Items), tt.wantItems)
			}
			if meal.Items[0].Food != tt.wantFood {
				t.Fatalf("food = %q, want %q", meal.Items[0].Food, tt.wantFood)
			}
		})
	}
}

func TestDecodePlanTextRejectsUnknownFieldsOutsideAliasMap(t *testing.T) {
	_, err := decodePlanText(`{
		"schema_version": "0.1",
		"plan_id": "unknown-field",
		"days": [{"day": 1, "meals": [{"name": "breakfast", "items": [{"food": "plain oatmeal", "quantity": 1, "unit": "cup", "calories": 120}]}]}]
	}`)
	if err == nil {
		t.Fatal("decodePlanText error = nil, want unknown field error")
	}
	if !strings.Contains(err.Error(), `unknown field "calories"`) {
		t.Fatalf("error = %q, want unknown calories field", err.Error())
	}
}

func TestPromptGenerationWritesRedactedNormalizationDebugArtifact(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	seeded := seededCase(t, root)
	secret := "sk-debug-secret"
	provider := &fakeProvider{responses: []string{
		`{"schema_version":"0.1","plan_id":"bad-initial","debug":"sk-debug-secret","days":[{"day":1,"meals":[{"meal_type":"breakfast","items":[{"food_item":"plain oatmeal","quantity":1,"unit":"cup"}]}]}]}`,
		`{"schema_version":"0.1","plan_id":"bad-repair","debug":"sk-debug-secret","days":[{"day":1,"meals":[{"name":"breakfast","items":[{"food":"plain oatmeal","quantity":1,"unit":"cup"}]}]}]}`,
	}}

	body := marshalJSON(t, CreateRunRequest{
		InputMode:        "prompt_generation",
		Settings:         seeded.Settings,
		GenerationPrompt: "Create a simple 3 day meal plan.",
		Provider: ProviderConfig{
			Type:   "openai_compatible",
			Model:  "fake-model",
			APIKey: secret,
		},
	})
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created CreateRunResponse
	decodeJSON(t, createResp.Body.Bytes(), &created)

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Completer, error) {
		return provider, nil
	}).ProcessOne(context.Background())
	if err == nil {
		t.Fatal("expected prompt generation to fail")
	}
	if !processed {
		t.Fatal("expected worker to process one run")
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", provider.calls)
	}
	run, err := store.GetRun(context.Background(), created.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != StatusFailed {
		t.Fatalf("run status = %q, want failed", run.Status)
	}

	debugBytes := readFile(t, filepath.Join(run.ArtifactDir, "debug", "normalization-failure.json"))
	if bytes.Contains(debugBytes, []byte(secret)) {
		t.Fatalf("normalization debug artifact contains provider secret:\n%s", string(debugBytes))
	}
	var debug normalizationFailureArtifact
	decodeJSON(t, debugBytes, &debug)
	if debug.Provider.APIKey != "redacted" {
		t.Fatalf("debug provider api_key = %q, want redacted", debug.Provider.APIKey)
	}
	if !strings.Contains(debug.InitialOutput, "[redacted]") || !strings.Contains(debug.RepairOutput, "[redacted]") {
		t.Fatalf("debug outputs were not redacted: %+v", debug)
	}
	if !strings.Contains(debug.InitialError, `unknown field "debug"`) || !strings.Contains(debug.RepairError, `unknown field "debug"`) {
		t.Fatalf("debug errors missing strict decode details: %+v", debug)
	}
	if !hasNormalizationEvent(debug.NormalizationEvents, "repair_attempted") {
		t.Fatalf("debug events missing repair_attempted: %+v", debug.NormalizationEvents)
	}
}

func TestPromptGenerationWritesRedactedDebugArtifactOnProviderError(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	seeded := seededCase(t, root)
	secret := "sk-provider-error-secret"

	body := marshalJSON(t, CreateRunRequest{
		InputMode:        "prompt_generation",
		Settings:         seeded.Settings,
		GenerationPrompt: "Create a simple 3 day meal plan.",
		Provider: ProviderConfig{
			Type:   ProviderTypeGemini,
			Model:  "gemini-test",
			APIKey: secret,
		},
	})
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created CreateRunResponse
	decodeJSON(t, createResp.Body.Bytes(), &created)

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Completer, error) {
		return errorProvider{err: fmt.Errorf("Gemini provider returned HTTP 400 Bad Request: schema rejected for %s", config.APIKey)}, nil
	}).ProcessOne(context.Background())
	if err == nil {
		t.Fatal("expected provider error")
	}
	if !processed {
		t.Fatal("expected worker to process one run")
	}

	run, err := store.GetRun(context.Background(), created.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != StatusFailed {
		t.Fatalf("run status = %q, want failed", run.Status)
	}
	debugBytes := readFile(t, filepath.Join(run.ArtifactDir, "debug", "normalization-failure.json"))
	if bytes.Contains(debugBytes, []byte(secret)) {
		t.Fatalf("provider error debug artifact contains provider secret:\n%s", string(debugBytes))
	}
	var debug normalizationFailureArtifact
	decodeJSON(t, debugBytes, &debug)
	if debug.Provider.APIKey != "redacted" {
		t.Fatalf("debug provider api_key = %q, want redacted", debug.Provider.APIKey)
	}
	if !strings.Contains(debug.FinalError, "[redacted]") {
		t.Fatalf("debug final error was not redacted: %+v", debug)
	}
	if !hasNormalizationEvent(debug.NormalizationEvents, "provider_request_failed") {
		t.Fatalf("debug events missing provider_request_failed: %+v", debug.NormalizationEvents)
	}
}

func TestBYOKRunFailsClosedWhenPendingInputExpiresBeforeWorkerClaim(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.PendingInputTTL = -time.Second
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	seeded := seededCase(t, root)

	body := marshalJSON(t, CreateRunRequest{
		InputMode:        "prompt_generation",
		Settings:         seeded.Settings,
		GenerationPrompt: "Create a simple 3 day meal plan.",
		Provider: ProviderConfig{
			Type:   ProviderTypeGemini,
			Model:  "gemini-test",
			APIKey: "sk-expiring-secret",
		},
	})
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	if pending.Count() != 1 {
		t.Fatalf("pending count after create = %d, want 1", pending.Count())
	}
	var created CreateRunResponse
	decodeJSON(t, createResp.Body.Bytes(), &created)

	providerCalled := false
	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Completer, error) {
		providerCalled = true
		return nil, fmt.Errorf("provider should not be called")
	}).ProcessOne(context.Background())
	if err == nil {
		t.Fatal("process run error = nil, want expired pending input error")
	}
	if !processed {
		t.Fatal("expected worker to process one run")
	}
	if providerCalled {
		t.Fatal("provider factory called after pending input expired")
	}
	if pending.Count() != 0 {
		t.Fatalf("pending count after expired worker claim = %d, want 0", pending.Count())
	}
	run, err := store.GetRun(context.Background(), created.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != StatusFailed {
		t.Fatalf("run status = %q, want failed", run.Status)
	}
	if !strings.Contains(run.Error, "pending BYOK run input expired") {
		t.Fatalf("run error = %q, want pending input expired message", run.Error)
	}
}
