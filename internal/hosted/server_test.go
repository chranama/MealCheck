package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

	processed, err := NewWorker(config, store).ProcessOne(context.Background())
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
		DemoIndexPath:    filepath.Join(root, "ui", "demo-runs", "index.json"),
		DemoArtifactRoot: filepath.Join(root, "ui"),
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
