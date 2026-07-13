package httpapi

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDemoEndpoints(t *testing.T) {
	root := repoRoot(t)
	server := NewServer(testConfig(t, root), NewMemoryStore())

	indexResp := doRequest(t, server, http.MethodGet, "/api/demo-runs", "")
	if indexResp.Code != http.StatusOK {
		t.Fatalf("demo index status = %d body=%s", indexResp.Code, indexResp.Body.String())
	}
	if !strings.Contains(indexResp.Body.String(), "seeded-one-day-peanut-allergy") {
		t.Fatalf("demo index missing seeded run: %s", indexResp.Body.String())
	}

	reportResp := doRequest(t, server, http.MethodGet, "/api/demo-runs/seeded-one-day-peanut-allergy/report", "")
	if reportResp.Code != http.StatusOK {
		t.Fatalf("demo report status = %d body=%s", reportResp.Code, reportResp.Body.String())
	}
	var report map[string]any
	decodeJSON(t, reportResp.Body.Bytes(), &report)
	if report["decision"] != "block" {
		t.Fatalf("demo report decision = %v, want block", report["decision"])
	}

	artifactResp := doRequest(t, server, http.MethodGet, "/api/demo-runs/seeded-one-day-peanut-allergy/artifacts/decision.json", "")
	if artifactResp.Code != http.StatusOK {
		t.Fatalf("demo artifact status = %d body=%s", artifactResp.Code, artifactResp.Body.String())
	}
}

func TestCleanupDeletesExpiredArtifacts(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.Retention = -time.Hour
	store := NewMemoryStore()

	run := newRun(config, "examples/seeded-one-day-peanut-allergy/case.json")
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
