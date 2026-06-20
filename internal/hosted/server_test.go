package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chranama/MealCheck/internal/checker"
)

func TestHostedRunLifecycle(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	server := NewServer(config, store)

	createBody := `{"case_path":"examples/seeded-3-day-peanut-allergy/case.json"}`
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", createBody)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}

	var created CreateRunResponse
	decodeJSON(t, createResp.Body.Bytes(), &created)
	if created.RunID == "" {
		t.Fatal("expected run id")
	}

	processed, err := NewWorker(config, store, nil, nil).ProcessOne(context.Background())
	if err != nil {
		t.Fatalf("process run: %v", err)
	}
	if !processed {
		t.Fatal("expected worker to process one run")
	}

	runResp := doRequest(t, server, http.MethodGet, "/api/runs/"+created.RunID, "")
	if runResp.Code != http.StatusOK {
		t.Fatalf("run status = %d body=%s", runResp.Code, runResp.Body.String())
	}
	var runDoc struct {
		Run Run `json:"run"`
	}
	decodeJSON(t, runResp.Body.Bytes(), &runDoc)
	if runDoc.Run.Status != StatusCompleted {
		t.Fatalf("run status = %q, want completed", runDoc.Run.Status)
	}
	if runDoc.Run.Decision != "block" {
		t.Fatalf("run decision = %q, want block", runDoc.Run.Decision)
	}

	reportResp := doRequest(t, server, http.MethodGet, "/api/runs/"+created.RunID+"/report", "")
	if reportResp.Code != http.StatusOK {
		t.Fatalf("report status = %d body=%s", reportResp.Code, reportResp.Body.String())
	}
	var report map[string]any
	decodeJSON(t, reportResp.Body.Bytes(), &report)
	if report["decision"] != "block" {
		t.Fatalf("report decision = %v, want block", report["decision"])
	}

	artifactListResp := doRequest(t, server, http.MethodGet, "/api/runs/"+created.RunID+"/artifacts", "")
	if artifactListResp.Code != http.StatusOK {
		t.Fatalf("artifact list status = %d body=%s", artifactListResp.Code, artifactListResp.Body.String())
	}
	if !strings.Contains(artifactListResp.Body.String(), "decision.json") {
		t.Fatalf("artifact list missing decision.json: %s", artifactListResp.Body.String())
	}

	artifactResp := doRequest(t, server, http.MethodGet, "/api/runs/"+created.RunID+"/artifacts/decision.json", "")
	if artifactResp.Code != http.StatusOK {
		t.Fatalf("artifact status = %d body=%s", artifactResp.Code, artifactResp.Body.String())
	}
	var decision map[string]any
	decodeJSON(t, artifactResp.Body.Bytes(), &decision)
	if decision["decision"] != "block" {
		t.Fatalf("artifact decision = %v, want block", decision["decision"])
	}

	eventsResp := doRequest(t, server, http.MethodGet, "/api/runs/"+created.RunID+"/events", "")
	if eventsResp.Code != http.StatusOK {
		t.Fatalf("events status = %d body=%s", eventsResp.Code, eventsResp.Body.String())
	}
	for _, expected := range []string{"event: queued", "event: started", "event: artifact_written", "event: completed"} {
		if !strings.Contains(eventsResp.Body.String(), expected) {
			t.Fatalf("events missing %q:\n%s", expected, eventsResp.Body.String())
		}
	}
}

func TestProfileGenerationUsesBYOKProviderAndRedactsSecret(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	seeded := seededCase(t, root)
	provider := &fakeProvider{responses: []string{string(readFile(t, filepath.Join(root, "examples/seeded-3-day-peanut-allergy/plans/candidate.json")))}}
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

	worker := NewWorker(config, store, pending, func(config ProviderConfig) (Provider, error) {
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
	candidate := string(readFile(t, filepath.Join(root, "examples/seeded-3-day-peanut-allergy/plans/candidate.json")))
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

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Provider, error) {
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

func TestQualifyEndpointRequiresInviteAndReturnsStructuredQualification(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.AccessMode = AccessModeInviteRequired
	config.InviteToken = "invite-secret"
	store := NewMemoryStore()
	server := NewServer(config, store)
	seeded := seededCase(t, root)

	body := marshalJSON(t, MealPlanQualificationRequest{
		Text:     testMealPlanJSON(false),
		Settings: seeded.Settings,
	})
	unauthorized := doRequest(t, server, http.MethodPost, "/api/qualify", body)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401 body=%s", unauthorized.Code, unauthorized.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/qualify", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MealCheck-Invite-Token", "invite-secret")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("qualify status = %d, want 200 body=%s", recorder.Code, recorder.Body.String())
	}
	var response QualifyMealPlanResponse
	decodeJSON(t, recorder.Body.Bytes(), &response)
	if response.Qualification.Status != QualificationStatusEligibleForVerification {
		t.Fatalf("qualification status = %q, want %q", response.Qualification.Status, QualificationStatusEligibleForVerification)
	}
	if response.Qualification.NormalizedPlan == nil {
		t.Fatal("normalized plan missing")
	}
	if response.Qualification.ProviderUsed {
		t.Fatal("provider_used = true, want false for already-normalized JSON")
	}
}

func TestQualifyEndpointRequiresProviderOnlyWhenNormalizationIsNeeded(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	server := NewServer(config, store)
	seeded := seededCase(t, root)

	body := marshalJSON(t, MealPlanQualificationRequest{
		Text:     "Day 1 breakfast: 1 cup cooked oatmeal.",
		Settings: seeded.Settings,
	})
	resp := doRequest(t, server, http.MethodPost, "/api/qualify", body)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("qualify status = %d, want 400 body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "provider model is required") {
		t.Fatalf("provider validation body missing model error: %s", resp.Body.String())
	}
}

func TestQualifyEndpointUsesBYOKProviderForTextNormalization(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	server := NewServer(config, store)
	seeded := seededCase(t, root)
	secret := "sk-qualify-endpoint-secret"
	provider := &fakeProvider{responses: []string{testMealPlanJSON(false)}}
	server.ProviderFactory = func(config ProviderConfig) (Provider, error) {
		if config.Type != ProviderTypeOpenAI {
			t.Fatalf("provider type = %q, want openai", config.Type)
		}
		if config.APIKey != secret {
			t.Fatalf("provider api key = %q, want secret", config.APIKey)
		}
		return provider, nil
	}

	body := marshalJSON(t, MealPlanQualificationRequest{
		Text: "Day 1 / Breakfast / cooked oatmeal / 1 / cup\n" + secret,
		Settings: checker.Settings{
			NutritionTargets: seeded.Settings.NutritionTargets,
			VerificationConstraints: checker.VerificationConstraints{
				Days:        1,
				MealsPerDay: 1,
			},
		},
		Provider: ProviderConfig{
			Type:   ProviderTypeOpenAI,
			Model:  "fake-model",
			APIKey: secret,
		},
	})
	resp := doRequest(t, server, http.MethodPost, "/api/qualify", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("qualify status = %d, want 200 body=%s", resp.Code, resp.Body.String())
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	var response QualifyMealPlanResponse
	decodeJSON(t, resp.Body.Bytes(), &response)
	if response.Qualification.Status != QualificationStatusEligibleForVerification {
		t.Fatalf("qualification status = %q, want %q", response.Qualification.Status, QualificationStatusEligibleForVerification)
	}
	if !response.Qualification.ProviderUsed {
		t.Fatal("provider_used = false, want true")
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
		string(readFile(t, filepath.Join(root, "examples/seeded-3-day-peanut-allergy/plans/candidate.json"))),
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

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Provider, error) {
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

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Provider, error) {
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
		`{"schema_version":"0.1","plan_id":"unsupported-unit","days":[{"day":1,"meals":[{"name":"breakfast","items":[{"food":"Whole Wheat Bread","quantity":1,"unit":"slice"}]},{"name":"lunch","items":[{"food":"chicken breast","quantity":4,"unit":"oz"}]},{"name":"dinner","items":[{"food":"broccoli","quantity":1,"unit":"cup"}]}]}]}`,
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

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Provider, error) {
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
	if item.Quantity != nil || item.Unit != "" || item.QuantityText != "1 slice" || item.ResolutionStatus != "unresolved" || item.UnresolvedReason != "unsupported_unit" {
		t.Fatalf("unsupported unit item was not preserved as unresolved: %+v", item)
	}
	var decision checker.DecisionDocument
	decodeJSON(t, readFile(t, filepath.Join(run.ArtifactDir, "decision.json")), &decision)
	if len(decision.UnresolvedItems) != 1 {
		t.Fatalf("len(unresolved_items) = %d, want 1: %+v", len(decision.UnresolvedItems), decision.UnresolvedItems)
	}
	unresolved := decision.UnresolvedItems[0]
	if unresolved.Food != "Whole Wheat Bread" || unresolved.QuantityText != "1 slice" || unresolved.UnresolvedReason != "unsupported_unit" {
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

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Provider, error) {
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

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Provider, error) {
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

	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Provider, error) {
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
	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Provider, error) {
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

func TestRequestRunInputAcceptsNativeProviderTypes(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	seeded := seededCase(t, root)

	for _, providerType := range []string{ProviderTypeOpenAI, ProviderTypeAnthropic, ProviderTypeGemini, ProviderTypeOpenAICompatible} {
		t.Run(providerType, func(t *testing.T) {
			_, input, ok, err := requestRunInput(config, CreateRunRequest{
				InputMode: "profile_generation",
				Settings:  seeded.Settings,
				Provider: ProviderConfig{
					Type:    providerType,
					BaseURL: "https://example.invalid/v1",
					Model:   "test-model",
					APIKey:  "test-key",
				},
			})
			if err != nil {
				t.Fatalf("requestRunInput error: %v", err)
			}
			if !ok {
				t.Fatal("requestRunInput ok = false, want true")
			}
			if input.Provider.Type != providerType {
				t.Fatalf("provider type = %q, want %q", input.Provider.Type, providerType)
			}
			if providerType == ProviderTypeOpenAICompatible {
				if input.Provider.BaseURL == "" {
					t.Fatal("openai_compatible base_url was unexpectedly cleared")
				}
			} else if input.Provider.BaseURL != "" {
				t.Fatalf("native provider base_url = %q, want empty", input.Provider.BaseURL)
			}
		})
	}
}

func TestProviderPromptSettingsExposeOnlyVerifierUsedFields(t *testing.T) {
	root := repoRoot(t)
	seeded := seededCase(t, root)
	genMessages, err := generationMessages(PendingRunInput{
		Mode:     "profile_generation",
		Settings: seeded.Settings,
	})
	if err != nil {
		t.Fatalf("generationMessages error: %v", err)
	}
	qualMessages := qualificationMessages(MealPlanQualificationRequest{
		Text:     "Day 1 breakfast: 1 cup oatmeal.",
		Settings: seeded.Settings,
	})
	for name, content := range map[string]string{
		"generation":    genMessages[1].Content,
		"qualification": qualMessages[1].Content,
	} {
		for _, want := range []string{`"nutrition_targets"`, `"calorie_target_kcal"`, `"protein_target_g"`, `"verification_constraints"`, `"days"`, `"meals_per_day"`, `"allergies"`, `"excluded_foods"`, `"max_sodium_mg_per_day"`, `"requires_prep_safety_notes"`} {
			if !strings.Contains(content, want) {
				t.Fatalf("%s provider prompt missing %s: %s", name, want, content)
			}
		}
		for _, unwanted := range []string{`"age"`, `"sex"`, `"height_cm"`, `"weight_kg"`, `"activity_level"`, `"goal"`, `"diet_pattern"`, `"requires_shopping_list"`} {
			if strings.Contains(content, unwanted) {
				t.Fatalf("%s provider prompt contains unused field %s: %s", name, unwanted, content)
			}
		}
	}
}

func TestOpenAIProviderRequestAndResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-openai" {
			t.Fatalf("authorization = %q", got)
		}
		var payload map[string]any
		decodeJSON(t, readAll(t, r), &payload)
		if payload["model"] != "gpt-test" {
			t.Fatalf("model = %v", payload["model"])
		}
		format, ok := payload["response_format"].(map[string]any)
		if !ok || format["type"] != "json_schema" {
			t.Fatalf("response_format = %#v", payload["response_format"])
		}
		jsonSchema, ok := format["json_schema"].(map[string]any)
		if !ok || jsonSchema["name"] != "mealcheck_meal_plan" || jsonSchema["strict"] != true {
			t.Fatalf("json_schema = %#v", format["json_schema"])
		}
		schema, ok := jsonSchema["schema"].(map[string]any)
		if !ok || schema["additionalProperties"] != false {
			t.Fatalf("schema = %#v", jsonSchema["schema"])
		}
		days := schema["properties"].(map[string]any)["days"].(map[string]any)
		day := days["items"].(map[string]any)
		dayValue := day["properties"].(map[string]any)["day"].(map[string]any)
		if dayValue["minimum"] != float64(1) && dayValue["minimum"] != 1 {
			t.Fatalf("strict schema day minimum = %#v", dayValue["minimum"])
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"schema_version\":\"0.1\"}"}}]}`))
	}))
	defer server.Close()

	provider := OpenAIProvider{Client: server.Client(), BaseURL: server.URL + "/v1"}
	got, err := provider.Complete(context.Background(), ProviderConfig{
		Type:   ProviderTypeOpenAI,
		Model:  "gpt-test",
		APIKey: "sk-openai",
	}, []ProviderMessage{{Role: "system", Content: "Return JSON."}, {Role: "user", Content: "Plan."}})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if got != `{"schema_version":"0.1"}` {
		t.Fatalf("content = %q", got)
	}
}

func TestOpenAICompatibleProviderKeepsJSONMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q, want /chat/completions", r.URL.Path)
		}
		var payload map[string]any
		decodeJSON(t, readAll(t, r), &payload)
		format, ok := payload["response_format"].(map[string]any)
		if !ok || format["type"] != "json_object" {
			t.Fatalf("response_format = %#v", payload["response_format"])
		}
		if _, ok := format["json_schema"]; ok {
			t.Fatalf("openai-compatible response_format unexpectedly included json_schema: %#v", format)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"schema_version\":\"0.1\"}"}}]}`))
	}))
	defer server.Close()

	provider := OpenAICompatibleProvider{Client: server.Client()}
	got, err := provider.Complete(context.Background(), ProviderConfig{
		Type:    ProviderTypeOpenAICompatible,
		BaseURL: server.URL,
		Model:   "compatible-test",
		APIKey:  "sk-compatible",
	}, []ProviderMessage{{Role: "user", Content: "Plan."}})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if got != `{"schema_version":"0.1"}` {
		t.Fatalf("content = %q", got)
	}
}

func TestAnthropicProviderRequestAndResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %q, want /v1/messages", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "sk-anthropic" {
			t.Fatalf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got == "" {
			t.Fatal("missing anthropic-version header")
		}
		var payload map[string]any
		decodeJSON(t, readAll(t, r), &payload)
		if payload["model"] != "claude-test" {
			t.Fatalf("model = %v", payload["model"])
		}
		if !strings.Contains(fmt.Sprint(payload["system"]), "Return JSON") {
			t.Fatalf("system = %v", payload["system"])
		}
		if _, ok := payload["messages"].([]any); !ok {
			t.Fatalf("messages = %#v", payload["messages"])
		}
		outputConfig, ok := payload["output_config"].(map[string]any)
		if !ok {
			t.Fatalf("output_config = %#v", payload["output_config"])
		}
		format, ok := outputConfig["format"].(map[string]any)
		if !ok || format["type"] != "json_schema" {
			t.Fatalf("output_config.format = %#v", outputConfig["format"])
		}
		schema, ok := format["schema"].(map[string]any)
		if !ok || schema["additionalProperties"] != false {
			t.Fatalf("schema = %#v", format["schema"])
		}
		days := schema["properties"].(map[string]any)["days"].(map[string]any)
		day := days["items"].(map[string]any)
		dayValue := day["properties"].(map[string]any)["day"].(map[string]any)
		if _, ok := dayValue["minimum"]; ok {
			t.Fatalf("portable Anthropic schema included unsupported minimum: %#v", dayValue)
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"{\"schema_version\":\"0.1\"}"}]}`))
	}))
	defer server.Close()

	provider := AnthropicProvider{Client: server.Client(), BaseURL: server.URL}
	got, err := provider.Complete(context.Background(), ProviderConfig{
		Type:   ProviderTypeAnthropic,
		Model:  "claude-test",
		APIKey: "sk-anthropic",
	}, []ProviderMessage{{Role: "system", Content: "Return JSON."}, {Role: "user", Content: "Plan."}})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if got != `{"schema_version":"0.1"}` {
		t.Fatalf("content = %q", got)
	}
}

func TestGeminiProviderRequestAndResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-test:generateContent" {
			t.Fatalf("path = %q, want Gemini generateContent path", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "sk-gemini" {
			t.Fatalf("x-goog-api-key = %q", got)
		}
		var payload map[string]any
		decodeJSON(t, readAll(t, r), &payload)
		if _, ok := payload["systemInstruction"].(map[string]any); !ok {
			t.Fatalf("systemInstruction = %#v", payload["systemInstruction"])
		}
		config, ok := payload["generationConfig"].(map[string]any)
		if !ok {
			t.Fatalf("generationConfig = %#v", payload["generationConfig"])
		}
		if config["responseMimeType"] != "application/json" {
			t.Fatalf("responseMimeType = %#v", config["responseMimeType"])
		}
		if _, ok := config["responseFormat"]; ok {
			t.Fatalf("Gemini payload should not use responseFormat after live API rejection: %#v", config["responseFormat"])
		}
		if _, ok := config["responseSchema"]; ok {
			t.Fatalf("Gemini payload should not use responseSchema after live API rejection: %#v", config["responseSchema"])
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"schema_version\":\"0.1\"}"}]}}]}`))
	}))
	defer server.Close()

	provider := GeminiProvider{Client: server.Client(), BaseURL: server.URL}
	got, err := provider.Complete(context.Background(), ProviderConfig{
		Type:   ProviderTypeGemini,
		Model:  "gemini-test",
		APIKey: "sk-gemini",
	}, []ProviderMessage{{Role: "system", Content: "Return JSON."}, {Role: "user", Content: "Plan."}})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if got != `{"schema_version":"0.1"}` {
		t.Fatalf("content = %q", got)
	}
}

func TestProviderHTTPErrorMessagesAreActionableAndRedacted(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		provider   Provider
		config     ProviderConfig
		want       []string
		wantAbsent []string
	}{
		{
			name:   "openai quota",
			status: http.StatusTooManyRequests,
			body:   `{"error":{"message":"Quota exceeded for sk-openai","type":"rate_limit_error","code":"rate_limit_exceeded"}}`,
			provider: OpenAIProvider{
				BaseURL: "REPLACED",
			},
			config: ProviderConfig{
				Type:   ProviderTypeOpenAI,
				Model:  "gpt-test",
				APIKey: "sk-openai",
			},
			want: []string{
				"OpenAI provider returned HTTP 429 Too Many Requests",
				"Quota exceeded for [redacted]",
				"rate_limit_error",
				"rate_limit_exceeded",
			},
			wantAbsent: []string{"sk-openai"},
		},
		{
			name:   "anthropic overloaded",
			status: 529,
			body:   `{"type":"error","error":{"type":"overloaded_error","message":"Anthropic is overloaded"}}`,
			provider: AnthropicProvider{
				BaseURL: "REPLACED",
			},
			config: ProviderConfig{
				Type:   ProviderTypeAnthropic,
				Model:  "claude-test",
				APIKey: "sk-ant",
			},
			want: []string{
				"Anthropic provider returned HTTP 529",
				"Anthropic is overloaded",
				"overloaded_error",
			},
		},
		{
			name:   "gemini unavailable",
			status: http.StatusServiceUnavailable,
			body:   `{"error":{"code":503,"message":"The model is overloaded. Please try again later.","status":"UNAVAILABLE"}}`,
			provider: GeminiProvider{
				BaseURL: "REPLACED",
			},
			config: ProviderConfig{
				Type:   ProviderTypeGemini,
				Model:  "gemini-test",
				APIKey: "sk-gemini",
			},
			want: []string{
				"Gemini provider returned HTTP 503 Service Unavailable",
				"The model is overloaded. Please try again later.",
				"UNAVAILABLE",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			switch provider := tt.provider.(type) {
			case OpenAIProvider:
				provider.Client = server.Client()
				provider.BaseURL = server.URL + "/v1"
				tt.provider = provider
			case AnthropicProvider:
				provider.Client = server.Client()
				provider.BaseURL = server.URL
				tt.provider = provider
			case GeminiProvider:
				provider.Client = server.Client()
				provider.BaseURL = server.URL
				tt.provider = provider
			}

			_, err := tt.provider.Complete(context.Background(), tt.config, []ProviderMessage{{Role: "user", Content: "Plan."}})
			if err == nil {
				t.Fatal("Complete error = nil, want provider error")
			}
			got := err.Error()
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("error %q does not contain %q", got, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Fatalf("error %q contains secret %q", got, absent)
				}
			}
		})
	}
}

func TestProviderRequestErrorRedactsKey(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial failed with sk-secret")
	})}
	provider := OpenAIProvider{Client: client, BaseURL: "https://example.invalid/v1"}

	_, err := provider.Complete(context.Background(), ProviderConfig{
		Type:   ProviderTypeOpenAI,
		Model:  "gpt-test",
		APIKey: "sk-secret",
	}, []ProviderMessage{{Role: "user", Content: "Plan."}})
	if err == nil {
		t.Fatal("Complete error = nil, want request error")
	}
	if strings.Contains(err.Error(), "sk-secret") {
		t.Fatalf("error %q contains secret", err.Error())
	}
	if !strings.Contains(err.Error(), "OpenAI provider request failed") {
		t.Fatalf("error %q does not identify provider request failure", err.Error())
	}
}

func TestHostedQueueLimit(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.QueueSize = 1
	store := NewMemoryStore()
	server := NewServer(config, store)

	body := `{"case_path":"examples/seeded-3-day-peanut-allergy/case.json"}`
	first := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first create status = %d body=%s", first.Code, first.Body.String())
	}
	second := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second create status = %d, want 429 body=%s", second.Code, second.Body.String())
	}
}

func TestHealthReportsPublicAccessPolicy(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.PublicOpenAICompatible = false
	config.PublicDailyRunLimit = 7
	server := NewServer(config, NewMemoryStore())

	resp := doRequest(t, server, http.MethodGet, "/api/health", "")
	if resp.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200 body=%s", resp.Code, resp.Body.String())
	}
	var health map[string]any
	decodeJSON(t, resp.Body.Bytes(), &health)
	if health["access_mode"] != AccessModePublicBYOK {
		t.Fatalf("access_mode = %v, want %q", health["access_mode"], AccessModePublicBYOK)
	}
	if health["public_openai_compatible"] != false {
		t.Fatalf("public_openai_compatible = %v, want false", health["public_openai_compatible"])
	}
	policy, ok := health["policy"].(map[string]any)
	if !ok {
		t.Fatalf("policy missing or wrong type: %#v", health["policy"])
	}
	if policy["public_daily_run_limit"] != float64(7) {
		t.Fatalf("public_daily_run_limit = %v, want 7", policy["public_daily_run_limit"])
	}
}

func TestPublicRequestRateLimit(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.PublicRequestLimit = 1
	config.PublicRequestWindow = time.Hour
	server := NewServer(config, NewMemoryStore())
	seeded := seededCase(t, root)
	body := marshalJSON(t, MealPlanQualificationRequest{
		Text:     testMealPlanJSON(false),
		Settings: seeded.Settings,
	})

	first := doRequest(t, server, http.MethodPost, "/api/qualify", body)
	if first.Code != http.StatusOK {
		t.Fatalf("first qualify status = %d, want 200 body=%s", first.Code, first.Body.String())
	}
	second := doRequest(t, server, http.MethodPost, "/api/qualify", body)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second qualify status = %d, want 429 body=%s", second.Code, second.Body.String())
	}
	if got := second.Header().Get("Retry-After"); got == "" {
		t.Fatal("rate-limited response missing Retry-After")
	}
}

func TestPublicDailyRunLimit(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.PublicDailyRunLimit = 1
	store := NewMemoryStore()
	server := NewServer(config, store)

	body := `{"case_path":"examples/seeded-3-day-peanut-allergy/case.json"}`
	first := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first create status = %d, want 202 body=%s", first.Code, first.Body.String())
	}
	second := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second create status = %d, want 429 body=%s", second.Code, second.Body.String())
	}
	if !strings.Contains(second.Body.String(), "daily public run limit") {
		t.Fatalf("daily limit body missing reason: %s", second.Body.String())
	}
}

func TestPublicModeRejectsOpenAICompatibleByDefault(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.PublicOpenAICompatible = false
	server := NewServer(config, NewMemoryStore())
	seeded := seededCase(t, root)

	body := marshalJSON(t, CreateRunRequest{
		InputMode: "profile_generation",
		Settings:  seeded.Settings,
		Provider: ProviderConfig{
			Type:    ProviderTypeOpenAICompatible,
			BaseURL: "https://router.example/v1",
			Model:   "custom",
			APIKey:  "secret",
		},
	})
	resp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want 400 body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "openai_compatible providers are disabled") {
		t.Fatalf("body missing openai_compatible policy message: %s", resp.Body.String())
	}
}

func TestPublicModeRejectsLocalOpenAICompatibleBaseURLWhenEnabled(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.PublicOpenAICompatible = true
	server := NewServer(config, NewMemoryStore())
	seeded := seededCase(t, root)

	body := marshalJSON(t, CreateRunRequest{
		InputMode: "profile_generation",
		Settings:  seeded.Settings,
		Provider: ProviderConfig{
			Type:    ProviderTypeOpenAICompatible,
			BaseURL: "https://127.0.0.1/v1",
			Model:   "custom",
			APIKey:  "secret",
		},
	})
	resp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want 400 body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "private or local IP") {
		t.Fatalf("body missing local IP policy message: %s", resp.Body.String())
	}
}

func TestInviteModeAllowsOpenAICompatibleWhenPublicCustomEndpointsDisabled(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.AccessMode = AccessModeInviteRequired
	config.InviteToken = "invite-secret"
	config.PublicOpenAICompatible = false
	server := NewServer(config, NewMemoryStore())
	seeded := seededCase(t, root)

	body := marshalJSON(t, CreateRunRequest{
		InputMode: "profile_generation",
		Settings:  seeded.Settings,
		Provider: ProviderConfig{
			Type:   ProviderTypeOpenAICompatible,
			Model:  "fake-model",
			APIKey: "secret",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MealCheck-Invite-Token", "invite-secret")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, want 202 body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestInviteTokenGate(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.AccessMode = AccessModeInviteRequired
	config.InviteToken = "invite-secret"
	server := NewServer(config, NewMemoryStore())

	body := `{"case_path":"examples/seeded-3-day-peanut-allergy/case.json"}`
	unauthorized := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401 body=%s", unauthorized.Code, unauthorized.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MealCheck-Invite-Token", "invite-secret")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("authorized status = %d, want 202 body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPerUserInviteTokenGate(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.AccessMode = AccessModeInviteRequired
	config.InviteRequired = true
	store := NewMemoryStore()
	maxRuns := 1
	generated, err := GenerateInviteToken("reviewer-chris", nil, &maxRuns, time.Now().UTC())
	if err != nil {
		t.Fatalf("generate invite token: %v", err)
	}
	if err := store.CreateInviteToken(context.Background(), generated.Invite); err != nil {
		t.Fatalf("create invite token: %v", err)
	}
	server := NewServer(config, store)

	body := `{"case_path":"examples/seeded-3-day-peanut-allergy/case.json"}`
	missing := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing invite status = %d, want 401 body=%s", missing.Code, missing.Body.String())
	}

	wrong := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader([]byte(body)))
	wrong.Header.Set("Content-Type", "application/json")
	wrong.Header.Set("X-MealCheck-Invite-Token", InviteTokenPrefix+generated.Invite.ID+".wrong-secret")
	wrongRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(wrongRecorder, wrong)
	if wrongRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("wrong invite status = %d, want 401 body=%s", wrongRecorder.Code, wrongRecorder.Body.String())
	}

	allowed := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader([]byte(body)))
	allowed.Header.Set("Content-Type", "application/json")
	allowed.Header.Set("X-MealCheck-Invite-Token", generated.Token)
	allowedRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(allowedRecorder, allowed)
	if allowedRecorder.Code != http.StatusAccepted {
		t.Fatalf("per-user invite status = %d, want 202 body=%s", allowedRecorder.Code, allowedRecorder.Body.String())
	}
	invite, err := store.GetInviteToken(context.Background(), generated.Invite.ID)
	if err != nil {
		t.Fatalf("get invite token: %v", err)
	}
	if invite.UsedRuns != 1 || invite.LastUsedAt == nil {
		t.Fatalf("invite usage = %d last_used=%v, want one recorded use", invite.UsedRuns, invite.LastUsedAt)
	}

	exhausted := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader([]byte(body)))
	exhausted.Header.Set("Content-Type", "application/json")
	exhausted.Header.Set("X-MealCheck-Invite-Token", generated.Token)
	exhaustedRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(exhaustedRecorder, exhausted)
	if exhaustedRecorder.Code != http.StatusTooManyRequests {
		t.Fatalf("exhausted invite status = %d, want 429 body=%s", exhaustedRecorder.Code, exhaustedRecorder.Body.String())
	}
}

func TestCORSAllowsOnlyConfiguredOrigin(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.AllowedOrigin = "http://127.0.0.1:4173"
	server := NewServer(config, NewMemoryStore())

	allowed := httptest.NewRequest(http.MethodOptions, "/api/runs", nil)
	allowed.Header.Set("Origin", config.AllowedOrigin)
	allowed.Header.Set("Access-Control-Request-Method", http.MethodPost)
	allowedRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(allowedRecorder, allowed)
	if allowedRecorder.Code != http.StatusNoContent {
		t.Fatalf("allowed preflight status = %d, want 204", allowedRecorder.Code)
	}
	if got := allowedRecorder.Header().Get("Access-Control-Allow-Origin"); got != config.AllowedOrigin {
		t.Fatalf("allowed origin header = %q, want %q", got, config.AllowedOrigin)
	}
	if got := allowedRecorder.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-MealCheck-Invite-Token") {
		t.Fatalf("allowed headers missing invite token header: %q", got)
	}

	disallowed := httptest.NewRequest(http.MethodOptions, "/api/runs", nil)
	disallowed.Header.Set("Origin", "https://example.invalid")
	disallowed.Header.Set("Access-Control-Request-Method", http.MethodPost)
	disallowedRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(disallowedRecorder, disallowed)
	if disallowedRecorder.Code != http.StatusNoContent {
		t.Fatalf("disallowed preflight status = %d, want 204", disallowedRecorder.Code)
	}
	if got := disallowedRecorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("disallowed origin header = %q, want empty", got)
	}
	if got := disallowedRecorder.Header().Values("Vary"); len(got) == 0 {
		t.Fatal("expected Vary: Origin for configured CORS")
	}

	noOrigin := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	noOriginRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(noOriginRecorder, noOrigin)
	if got := noOriginRecorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("no-origin CORS header = %q, want empty", got)
	}
}

func TestDemoEndpoints(t *testing.T) {
	root := repoRoot(t)
	server := NewServer(testConfig(t, root), NewMemoryStore())

	indexResp := doRequest(t, server, http.MethodGet, "/api/demo-runs", "")
	if indexResp.Code != http.StatusOK {
		t.Fatalf("demo index status = %d body=%s", indexResp.Code, indexResp.Body.String())
	}
	if !strings.Contains(indexResp.Body.String(), "seeded-3-day-peanut-allergy") {
		t.Fatalf("demo index missing seeded run: %s", indexResp.Body.String())
	}

	reportResp := doRequest(t, server, http.MethodGet, "/api/demo-runs/seeded-3-day-peanut-allergy/report", "")
	if reportResp.Code != http.StatusOK {
		t.Fatalf("demo report status = %d body=%s", reportResp.Code, reportResp.Body.String())
	}
	var report map[string]any
	decodeJSON(t, reportResp.Body.Bytes(), &report)
	if report["decision"] != "block" {
		t.Fatalf("demo report decision = %v, want block", report["decision"])
	}

	artifactResp := doRequest(t, server, http.MethodGet, "/api/demo-runs/seeded-3-day-peanut-allergy/artifacts/decision.json", "")
	if artifactResp.Code != http.StatusOK {
		t.Fatalf("demo artifact status = %d body=%s", artifactResp.Code, artifactResp.Body.String())
	}
}

func TestCleanupDeletesExpiredArtifacts(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.Retention = -time.Hour
	store := NewMemoryStore()

	run := newRun(config, "examples/seeded-3-day-peanut-allergy/case.json")
	if err := store.CreateRun(context.Background(), run, config.QueueSize, ""); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := os.MkdirAll(run.ArtifactDir, 0o755); err != nil {
		t.Fatalf("mkdir artifact dir: %v", err)
	}

	if err := (CleanupJob{Config: config, Store: store}).RunOnce(context.Background()); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(run.ArtifactDir); !os.IsNotExist(err) {
		t.Fatalf("artifact dir still exists or unexpected error: %v", err)
	}
	if _, err := store.GetRun(context.Background(), run.ID); err != ErrNotFound {
		t.Fatalf("expired run lookup err = %v, want ErrNotFound", err)
	}
}

func doRequest(t *testing.T, server *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, req)
	return recorder
}

func decodeJSON(t *testing.T, data []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, string(data))
	}
}

func marshalJSON(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return string(b)
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func seededCase(t *testing.T, root string) checker.Case {
	t.Helper()
	var c checker.Case
	decodeJSON(t, readFile(t, filepath.Join(root, "examples/seeded-3-day-peanut-allergy/case.json")), &c)
	return c
}

func seededPlan(t *testing.T, root string) checker.Plan {
	t.Helper()
	var plan checker.Plan
	decodeJSON(t, readFile(t, filepath.Join(root, "examples/seeded-3-day-peanut-allergy/plans/candidate.json")), &plan)
	return plan
}

func ptr[T any](value T) *T {
	return &value
}

func hasNormalizationEvent(events []NormalizationEvent, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func readAll(t *testing.T, r *http.Request) []byte {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func assertFileTreeDoesNotContain(t *testing.T, root, secret string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(b, []byte(secret)) {
			return fmt.Errorf("%s contains secret", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type fakeProvider struct {
	responses []string
	calls     int
	messages  [][]ProviderMessage
	configs   []ProviderConfig
}

func (p *fakeProvider) Complete(_ context.Context, config ProviderConfig, messages []ProviderMessage) (string, error) {
	if config.APIKey != "" {
		for _, message := range messages {
			if strings.Contains(message.Content, config.APIKey) {
				return "", fmt.Errorf("provider key leaked into prompt")
			}
		}
	}
	p.configs = append(p.configs, config)
	p.messages = append(p.messages, append([]ProviderMessage(nil), messages...))
	if p.calls >= len(p.responses) {
		return "", fmt.Errorf("fake provider response exhausted")
	}
	response := p.responses[p.calls]
	p.calls++
	return response, nil
}

type errorProvider struct {
	err error
}

func (p errorProvider) Complete(context.Context, ProviderConfig, []ProviderMessage) (string, error) {
	return "", p.err
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func testConfig(t *testing.T, root string) Config {
	t.Helper()
	dataDir := t.TempDir()
	return Config{
		Root:                     root,
		DataDir:                  dataDir,
		ArtifactDir:              filepath.Join(dataDir, "artifacts"),
		Addr:                     "127.0.0.1:0",
		StoreKind:                "memory",
		AccessMode:               AccessModePublicBYOK,
		PublicOpenAICompatible:   true,
		PublicRequestLimit:       60,
		PublicRequestWindow:      time.Minute,
		PublicDailyRunLimit:      20,
		MaxCandidateTextChars:    20_000,
		MaxGenerationPromptChars: 4_000,
		QueueSize:                3,
		MaxCasesPerRun:           20,
		MaxUploadBytes:           1_000_000,
		RunTimeout:               10 * time.Minute,
		PendingInputTTL:          30 * time.Minute,
		Retention:                7 * 24 * time.Hour,
		WorkerPoll:               time.Millisecond,
		CleanupInterval:          time.Hour,
		DemoIndexPath:            filepath.Join(root, "ui", "public", "demo-runs", "index.json"),
		DemoArtifactRoot:         filepath.Join(root, "ui", "public"),
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
