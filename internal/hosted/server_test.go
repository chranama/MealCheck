package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
		InputMode:   "profile_generation",
		Profile:     seeded.Profile,
		Constraints: seeded.Constraints,
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

func TestManualStructuredRunDoesNotRequireProvider(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	pending := NewPendingInputs()
	server := NewServer(config, store, pending)
	seeded := seededCase(t, root)

	body := marshalJSON(t, CreateRunRequest{
		InputMode:     "manual_structured",
		Profile:       seeded.Profile,
		Constraints:   seeded.Constraints,
		CandidatePlan: ptr(seededPlan(t, root)),
	})
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created CreateRunResponse
	decodeJSON(t, createResp.Body.Bytes(), &created)

	providerCalled := false
	processed, err := NewWorker(config, store, pending, func(config ProviderConfig) (Provider, error) {
		providerCalled = true
		return nil, fmt.Errorf("provider should not be called")
	}).ProcessOne(context.Background())
	if err != nil {
		t.Fatalf("process run: %v", err)
	}
	if !processed {
		t.Fatal("expected worker to process one run")
	}
	if providerCalled {
		t.Fatal("manual_structured run called provider")
	}

	run, err := store.GetRun(context.Background(), created.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != StatusCompleted {
		t.Fatalf("run status = %q, want completed", run.Status)
	}
	if _, err := os.Stat(filepath.Join(run.ArtifactDir, "optional", "llm-output.json")); !os.IsNotExist(err) {
		t.Fatalf("manual run wrote llm output or unexpected stat error: %v", err)
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
		Profile:          seeded.Profile,
		Constraints:      seeded.Constraints,
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
		Profile:          seeded.Profile,
		Constraints:      seeded.Constraints,
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

func TestInviteTokenGate(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
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
	if err := store.CreateRun(context.Background(), run, config.QueueSize); err != nil {
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

func testConfig(t *testing.T, root string) Config {
	t.Helper()
	dataDir := t.TempDir()
	return Config{
		Root:             root,
		DataDir:          dataDir,
		ArtifactDir:      filepath.Join(dataDir, "artifacts"),
		Addr:             "127.0.0.1:0",
		StoreKind:        "memory",
		QueueSize:        3,
		MaxCasesPerRun:   20,
		MaxUploadBytes:   1_000_000,
		RunTimeout:       10 * time.Minute,
		Retention:        7 * 24 * time.Hour,
		WorkerPoll:       time.Millisecond,
		CleanupInterval:  time.Hour,
		DemoIndexPath:    filepath.Join(root, "ui", "public", "demo-runs", "index.json"),
		DemoArtifactRoot: filepath.Join(root, "ui", "public"),
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
