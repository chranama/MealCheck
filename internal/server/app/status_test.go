package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestHealthReportsHostedLocalModelMode(t *testing.T) {
	root := repoRoot(t)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("local model health path = %q, want /models", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": []map[string]string{{"id": "test-model"}}})
	}))
	t.Cleanup(modelServer.Close)

	config := testConfig(t, root)
	config.HostedMode = HostedModeLocalModel
	config.LocalModelEnabled = true
	config.LocalModelBaseURL = modelServer.URL
	config.LocalModelName = "/Users/chranama-server/MealCheck-data/models/Qwen3-0.6B-Q4_K_M.gguf"
	config.LocalModelMaxInputChars = 2048
	config.LocalModelMaxOutputTokens = 128
	server := NewServer(config, NewMemoryStore())

	resp := doRequest(t, server, http.MethodGet, "/api/health", "")
	if resp.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200 body=%s", resp.Code, resp.Body.String())
	}
	var health map[string]any
	decodeJSON(t, resp.Body.Bytes(), &health)
	if health["hosted_mode"] != HostedModeLocalModel {
		t.Fatalf("hosted_mode = %v, want %q", health["hosted_mode"], HostedModeLocalModel)
	}
	localModel, ok := health["local_model"].(map[string]any)
	if !ok {
		t.Fatalf("local_model missing or wrong type: %#v", health["local_model"])
	}
	if localModel["enabled"] != true || localModel["ready"] != true {
		t.Fatalf("local_model health = %#v, want enabled and ready", localModel)
	}
	if localModel["model"] != "Qwen3-0.6B-Q4_K_M.gguf" {
		t.Fatalf("local_model model = %v, want basename only", localModel["model"])
	}
	if localModel["max_input_chars"] != float64(2048) {
		t.Fatalf("local_model max_input_chars = %v, want 2048", localModel["max_input_chars"])
	}
	if localModel["max_source_items"] != float64(20) {
		t.Fatalf("local_model max_source_items = %v, want 20", localModel["max_source_items"])
	}
	if localModel["supported_days"] != float64(1) {
		t.Fatalf("local_model supported_days = %v, want 1", localModel["supported_days"])
	}
}

func TestStatusReportsUserVisibleOperationalComponents(t *testing.T) {
	root := repoRoot(t)
	server := NewServer(testConfig(t, root), NewMemoryStore())

	resp := doRequest(t, server, http.MethodGet, "/api/status", "")
	if resp.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200 body=%s", resp.Code, resp.Body.String())
	}
	var status PublicStatusResponse
	decodeJSON(t, resp.Body.Bytes(), &status)
	if status.SchemaVersion != "0.1" {
		t.Fatalf("schema_version = %q, want 0.1", status.SchemaVersion)
	}
	if status.Overall.State != StatusStateOperational {
		t.Fatalf("overall state = %q, want operational", status.Overall.State)
	}
	for _, id := range []string{"meal_check_submission", "ai_meal_normalization", "nutrition_allergen_checking", "report_generation", "sample_report"} {
		component := statusComponentByID(t, status.Components, id)
		if component.State != StatusStateOperational {
			t.Fatalf("%s state = %q, want operational", id, component.State)
		}
	}
	if status.Links.SampleReport != "/api/demo-runs/seeded-one-day-peanut-allergy/report" {
		t.Fatalf("sample report link = %q", status.Links.SampleReport)
	}
	for _, rawField := range []string{"queue_size", "store", "local_model", "public_request_limit", "Qwen3"} {
		if strings.Contains(resp.Body.String(), rawField) {
			t.Fatalf("public status leaked raw field %q: %s", rawField, resp.Body.String())
		}
	}
}

func TestStatusReportsQueueCapacityAsDegradedPerformance(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.QueueSize = 1
	store := NewMemoryStore()
	server := NewServer(config, store)

	body := `{"case_path":"examples/seeded-one-day-peanut-allergy/case.json"}`
	createResp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, want 202 body=%s", createResp.Code, createResp.Body.String())
	}

	resp := doRequest(t, server, http.MethodGet, "/api/status", "")
	if resp.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200 body=%s", resp.Code, resp.Body.String())
	}
	var status PublicStatusResponse
	decodeJSON(t, resp.Body.Bytes(), &status)
	if status.Overall.State != StatusStateDegraded {
		t.Fatalf("overall state = %q, want degraded", status.Overall.State)
	}
	for _, id := range []string{"meal_check_submission", "report_generation"} {
		component := statusComponentByID(t, status.Components, id)
		if component.State != StatusStateDegraded {
			t.Fatalf("%s state = %q, want degraded", id, component.State)
		}
	}
}

func TestStatusReportsLocalModelUnavailableWithoutLeakingDetails(t *testing.T) {
	root := repoRoot(t)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(modelServer.Close)

	config := testConfig(t, root)
	config.HostedMode = HostedModeLocalModel
	config.LocalModelEnabled = true
	config.LocalModelBaseURL = modelServer.URL
	config.LocalModelName = "/Users/chranama-server/MealCheck-data/models/Qwen3-0.6B-Q4_K_M.gguf"
	server := NewServer(config, NewMemoryStore())

	resp := doRequest(t, server, http.MethodGet, "/api/status", "")
	if resp.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200 body=%s", resp.Code, resp.Body.String())
	}
	var status PublicStatusResponse
	decodeJSON(t, resp.Body.Bytes(), &status)
	if status.Overall.State != StatusStatePartialOutage {
		t.Fatalf("overall state = %q, want partial_outage", status.Overall.State)
	}
	for _, id := range []string{"ai_meal_normalization", "report_generation"} {
		component := statusComponentByID(t, status.Components, id)
		if component.State != StatusStatePartialOutage {
			t.Fatalf("%s state = %q, want partial_outage", id, component.State)
		}
	}
	for _, rawDetail := range []string{modelServer.URL, "Qwen3-0.6B-Q4_K_M.gguf", "local model endpoint returned"} {
		if strings.Contains(resp.Body.String(), rawDetail) {
			t.Fatalf("public status leaked raw local model detail %q: %s", rawDetail, resp.Body.String())
		}
	}
}

func TestStatusReportsSampleReportOutageWithoutFailingEndpoint(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.DemoIndexPath = filepath.Join(t.TempDir(), "missing-index.json")
	server := NewServer(config, NewMemoryStore())

	resp := doRequest(t, server, http.MethodGet, "/api/status", "")
	if resp.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200 body=%s", resp.Code, resp.Body.String())
	}
	var status PublicStatusResponse
	decodeJSON(t, resp.Body.Bytes(), &status)
	if status.Overall.State != StatusStatePartialOutage {
		t.Fatalf("overall state = %q, want partial_outage", status.Overall.State)
	}
	component := statusComponentByID(t, status.Components, "sample_report")
	if component.State != StatusStatePartialOutage {
		t.Fatalf("sample report state = %q, want partial_outage", component.State)
	}
	if status.Links.SampleReport != "" {
		t.Fatalf("sample report link = %q, want empty", status.Links.SampleReport)
	}
}

func TestStatusReportsStoreFailureAsMajorOutage(t *testing.T) {
	root := repoRoot(t)
	server := NewServer(testConfig(t, root), failingStatsStore{MemoryStore: NewMemoryStore()})

	resp := doRequest(t, server, http.MethodGet, "/api/status", "")
	if resp.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200 body=%s", resp.Code, resp.Body.String())
	}
	var status PublicStatusResponse
	decodeJSON(t, resp.Body.Bytes(), &status)
	if status.Overall.State != StatusStateMajorOutage {
		t.Fatalf("overall state = %q, want major_outage", status.Overall.State)
	}
	for _, id := range []string{"meal_check_submission", "ai_meal_normalization", "nutrition_allergen_checking", "report_generation"} {
		component := statusComponentByID(t, status.Components, id)
		if component.State != StatusStateMajorOutage {
			t.Fatalf("%s state = %q, want major_outage", id, component.State)
		}
	}
	if strings.Contains(resp.Body.String(), "stats unavailable") {
		t.Fatalf("public status leaked store error: %s", resp.Body.String())
	}
}
