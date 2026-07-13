package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

func TestHostedRunLifecycle(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	server := NewServer(config, store)

	createBody := `{"case_path":"examples/seeded-one-day-peanut-allergy/case.json"}`
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
		Run      Run         `json:"run"`
		Progress RunProgress `json:"progress"`
	}
	decodeJSON(t, runResp.Body.Bytes(), &runDoc)
	if runDoc.Run.Status != StatusCompleted {
		t.Fatalf("run status = %q, want completed", runDoc.Run.Status)
	}
	if runDoc.Run.Decision != "block" {
		t.Fatalf("run decision = %q, want block", runDoc.Run.Decision)
	}
	if runDoc.Progress.State != "ready" || runDoc.Progress.Label != "Report ready" {
		t.Fatalf("run progress = %+v, want ready report", runDoc.Progress)
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

func TestRunProgressClassifiesFailuresAndRedactsMessages(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	server := NewServer(config, store)
	run := newRun(config, "examples/seeded-one-day-peanut-allergy/case.json")
	if err := store.CreateRun(context.Background(), run, config.QueueSize, ""); err != nil {
		t.Fatalf("create run: %v", err)
	}
	now := time.Now().UTC()
	if err := store.FailRun(context.Background(), run.ID, "run timed out while reading /Users/example/MealCheck-data/secret.txt with sk-test-secret", now); err != nil {
		t.Fatalf("fail run: %v", err)
	}
	if err := store.AppendEvent(context.Background(), run.ID, EventFailed, "failed at /Users/example/private/path with sk-test-secret", now); err != nil {
		t.Fatalf("append event: %v", err)
	}

	runResp := doRequest(t, server, http.MethodGet, "/api/runs/"+run.ID, "")
	if runResp.Code != http.StatusOK {
		t.Fatalf("run status = %d body=%s", runResp.Code, runResp.Body.String())
	}
	var runDoc struct {
		Run      Run         `json:"run"`
		Progress RunProgress `json:"progress"`
	}
	decodeJSON(t, runResp.Body.Bytes(), &runDoc)
	if runDoc.Progress.State != "failed" {
		t.Fatalf("progress state = %q, want failed", runDoc.Progress.State)
	}
	if runDoc.Progress.Recovery == nil || runDoc.Progress.Recovery.Title != "Report timed out" {
		t.Fatalf("progress recovery = %+v, want timeout recovery", runDoc.Progress.Recovery)
	}
	if strings.Contains(runDoc.Progress.Message, "/Users/example") || strings.Contains(runDoc.Progress.Message, "sk-test-secret") {
		t.Fatalf("progress message was not redacted: %q", runDoc.Progress.Message)
	}
	if strings.Contains(runDoc.Run.Error, "/Users/example") || strings.Contains(runDoc.Run.Error, "sk-test-secret") {
		t.Fatalf("run error was not redacted: %q", runDoc.Run.Error)
	}

	eventsResp := doRequest(t, server, http.MethodGet, "/api/runs/"+run.ID+"/events", "")
	if eventsResp.Code != http.StatusOK {
		t.Fatalf("events status = %d body=%s", eventsResp.Code, eventsResp.Body.String())
	}
	if strings.Contains(eventsResp.Body.String(), "/Users/example") || strings.Contains(eventsResp.Body.String(), "sk-test-secret") {
		t.Fatalf("events were not redacted: %s", eventsResp.Body.String())
	}
}

func statusComponentByID(t *testing.T, components []StatusComponent, id string) StatusComponent {
	t.Helper()
	for _, component := range components {
		if component.ID == id {
			return component
		}
	}
	t.Fatalf("status component %q not found in %+v", id, components)
	return StatusComponent{}
}

type failingStatsStore struct {
	*MemoryStore
}

func (s failingStatsStore) Stats(context.Context) (StoreStats, error) {
	return StoreStats{}, fmt.Errorf("stats unavailable")
}

func seededPlan(t *testing.T, root string) checker.Plan {
	t.Helper()
	var plan checker.Plan
	decodeJSON(t, readFile(t, filepath.Join(root, "examples/seeded-one-day-peanut-allergy/plans/candidate.json")), &plan)
	return plan
}

func ptr[T any](value T) *T {
	return &value
}

func readAll(t *testing.T, r *http.Request) []byte {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (p *fakeProvider) Complete(_ context.Context, config ProviderConfig, request CompletionRequest) (string, error) {
	if config.APIKey != "" {
		for _, message := range request.Messages {
			if strings.Contains(message.Content, config.APIKey) {
				return "", fmt.Errorf("provider key leaked into prompt")
			}
		}
	}
	p.configs = append(p.configs, config)
	p.messages = append(p.messages, append([]Message(nil), request.Messages...))
	if p.calls >= len(p.responses) {
		return "", fmt.Errorf("fake provider response exhausted")
	}
	response := p.responses[p.calls]
	p.calls++
	return response, nil
}

func (p errorProvider) Complete(context.Context, ProviderConfig, CompletionRequest) (string, error) {
	return "", p.err
}

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
