package localsmoke

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/commands/localmodelsummary"
	"github.com/chranama/MealCheck/internal/core"
	llm "github.com/chranama/MealCheck/internal/llm/external"
	"github.com/chranama/MealCheck/internal/server/app"
	"github.com/chranama/MealCheck/internal/server/store"
	"github.com/chranama/MealCheck/internal/workflow/checker"
)

const (
	seededCasePath = "examples/seeded-one-day-peanut-allergy/case.json"
	seededPlanPath = "examples/seeded-one-day-peanut-allergy/plans/candidate.json"
	inviteToken    = "local-smoke-invite-token"
	allowedOrigin  = "http://127.0.0.1:4173"
	providerSecret = "local-smoke-provider-key"
)

type runner struct {
	root    string
	workDir string
	stdout  io.Writer
	logs    bytes.Buffer
}

func Run(args []string, stdout, stderr io.Writer) int {
	if err := run(args, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "mealcheck local-smoke failed: %v\n", err)
		return 1
	}
	return 0
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("local-smoke", flag.ContinueOnError)
	flags.SetOutput(stderr)
	rootFlag := flags.String("root", ".", "repository root")
	workDirFlag := flags.String("work-dir", "", "optional smoke-test work directory")
	keepWorkDir := flags.Bool("keep-work-dir", false, "keep the smoke-test work directory")
	if err := flags.Parse(args); err != nil {
		return err
	}

	root, err := filepath.Abs(*rootFlag)
	if err != nil {
		return err
	}
	workDir := *workDirFlag
	if workDir == "" {
		workDir, err = os.MkdirTemp("", "mealcheck-smoke-*")
		if err != nil {
			return err
		}
	} else if err := os.MkdirAll(workDir, 0o755); err != nil {
		return err
	}
	if !*keepWorkDir {
		defer os.RemoveAll(workDir)
	}

	r := &runner{root: root, workDir: workDir, stdout: stdout}
	r.logf("work_dir=%s", workDir)
	if err := r.cliDeploymentSmoke(); err != nil {
		return err
	}
	if err := r.hostedSmoke(); err != nil {
		return err
	}
	if err := r.p2OperationalSmoke(); err != nil {
		return err
	}
	if strings.Contains(r.logs.String(), providerSecret) {
		return fmt.Errorf("smoke logs contain provider secret")
	}
	r.logf("local smoke passed")
	return nil
}

func (r *runner) logf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	fmt.Fprintln(r.stdout, message)
	r.logs.WriteString(message + "\n")
}

func (r *runner) cliDeploymentSmoke() error {
	r.logf("cli: build mealcheck binary")
	binDir := filepath.Join(r.workDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	binName := "mealcheck"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(binDir, binName)
	if output, err := runCommand(r.root, 0, "go", "build", "-o", binPath, "./cmd/mealcheck"); err != nil {
		return fmt.Errorf("build CLI: %w\n%s", err, output)
	}

	r.logf("cli: validate seeded case and expect block exit")
	outDir := filepath.Join(r.workDir, "cli-artifacts")
	output, err := runCommand(r.root, 1, binPath, "validate", "--root", r.root, "--case", seededCasePath, "--out", outDir)
	if err != nil {
		return fmt.Errorf("validate seeded case: %w\n%s", err, output)
	}
	if !strings.Contains(output, "decision: block") {
		return fmt.Errorf("validate output missing block decision:\n%s", output)
	}
	var decision checker.DecisionDocument
	if err := readJSON(filepath.Join(outDir, "decision.json"), &decision); err != nil {
		return err
	}
	if decision.Decision != "block" {
		return fmt.Errorf("decision.json decision = %q, want block", decision.Decision)
	}

	r.logf("cli: inspect decision and expect same block exit")
	output, err = runCommand(r.root, 1, binPath, "decision", filepath.Join(outDir, "decision.json"))
	if err != nil {
		return fmt.Errorf("decision command: %w\n%s", err, output)
	}
	if !strings.Contains(output, "decision: block") {
		return fmt.Errorf("decision output missing block decision:\n%s", output)
	}
	return nil
}

func (r *runner) hostedSmoke() error {
	r.logf("hosted: start in-memory API harness")
	config := smokeConfig(r.root, r.workDir)
	store := store.NewMemoryStore()
	pending := app.NewPendingInputs()
	server := app.NewServer(config, store, pending)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := httpServer.Client()

	if err := r.verifyCORS(client, httpServer.URL); err != nil {
		return err
	}
	if err := expectStatus(client, http.MethodGet, httpServer.URL+"/api/health", "", "", http.StatusOK); err != nil {
		return err
	}
	if err := expectStatus(client, http.MethodPost, httpServer.URL+"/api/runs", `{"case_path":"`+seededCasePath+`"}`, "", http.StatusUnauthorized); err != nil {
		return err
	}

	var seeded checker.Case
	if err := readJSON(filepath.Join(r.root, seededCasePath), &seeded); err != nil {
		return err
	}
	seededRunID, err := r.createRun(client, httpServer.URL, core.CreateRunRequest{
		CasePath: seededCasePath,
	})
	if err != nil {
		return err
	}
	r.logf("hosted: process checked-in seeded run")
	if err := processOne(config, store, pending, llm.DefaultProviderFactory); err != nil {
		return err
	}
	if err := verifyCompletedRun(client, httpServer.URL, seededRunID); err != nil {
		return err
	}
	if err := deleteRun(client, httpServer.URL, seededRunID); err != nil {
		return err
	}

	r.logf("hosted: process BYOK run with fake provider")
	response, err := os.ReadFile(filepath.Join(r.root, seededPlanPath))
	if err != nil {
		return err
	}
	byokRunID, err := r.createRun(client, httpServer.URL, core.CreateRunRequest{
		InputMode: "profile_generation",
		Settings:  seeded.Settings,
		Provider: core.ProviderConfig{
			Type:    "openai_compatible",
			BaseURL: "https://fake-provider.local/v1",
			Model:   "fake-meal-plan",
			APIKey:  providerSecret,
		},
	})
	if err != nil {
		return err
	}
	if err := processOne(config, store, pending, llm.StaticResponseProviderFactory(string(response))); err != nil {
		return err
	}
	if err := verifyCompletedRun(client, httpServer.URL, byokRunID); err != nil {
		return err
	}
	if err := verifyRedaction(client, httpServer.URL, config.DataDir, byokRunID); err != nil {
		return err
	}
	return deleteRun(client, httpServer.URL, byokRunID)
}

func (r *runner) p2OperationalSmoke() error {
	r.logf("p2: verify queue-full response")
	queueConfig := smokeConfig(r.root, filepath.Join(r.workDir, "p2-queue"))
	queueConfig.QueueSize = 1
	queueStore := store.NewMemoryStore()
	queueServer := httptest.NewServer(app.NewServer(queueConfig, queueStore, app.NewPendingInputs()).Handler())
	defer queueServer.Close()
	client := queueServer.Client()
	body := `{"case_path":"` + seededCasePath + `"}`
	if err := expectStatus(client, http.MethodPost, queueServer.URL+"/api/runs", body, inviteToken, http.StatusAccepted); err != nil {
		return err
	}
	if err := expectErrorCode(client, http.MethodPost, queueServer.URL+"/api/runs", body, inviteToken, http.StatusTooManyRequests, "queue_full"); err != nil {
		return err
	}

	r.logf("p2: verify local-model unavailable response")
	unavailableConfig := localModelSmokeConfig(r.root, filepath.Join(r.workDir, "p2-unavailable"))
	unavailableConfig.LocalModelEnabled = false
	unavailableServer := httptest.NewServer(app.NewServer(unavailableConfig, store.NewMemoryStore(), app.NewPendingInputs()).Handler())
	defer unavailableServer.Close()
	var seeded checker.Case
	if err := readJSON(filepath.Join(r.root, seededCasePath), &seeded); err != nil {
		return err
	}
	localBody := localModelRunBody(seeded.Settings)
	if err := expectErrorCode(unavailableServer.Client(), http.MethodPost, unavailableServer.URL+"/api/runs", localBody, inviteToken, http.StatusServiceUnavailable, "local_model_unavailable"); err != nil {
		return err
	}

	r.logf("p2: verify one active local-model claim")
	activeConfig := localModelSmokeConfig(r.root, filepath.Join(r.workDir, "p2-active-local-model"))
	activeStore := store.NewMemoryStore()
	activePending := app.NewPendingInputs()
	activeServer := httptest.NewServer(app.NewServer(activeConfig, activeStore, activePending).Handler())
	defer activeServer.Close()
	firstLocalRunID, err := r.createRun(activeServer.Client(), activeServer.URL, localModelRunRequest(seeded.Settings))
	if err != nil {
		return err
	}
	secondLocalRunID, err := r.createRun(activeServer.Client(), activeServer.URL, localModelRunRequest(seeded.Settings))
	if err != nil {
		return err
	}
	claimed, ok, err := activeStore.ClaimNextRun(context.Background(), "local-smoke-worker-1", time.Now().Add(time.Minute))
	if err != nil {
		return err
	}
	if !ok || claimed.ID != firstLocalRunID {
		return fmt.Errorf("first active local-model claim = %s ok=%t, want %s", claimed.ID, ok, firstLocalRunID)
	}
	blocked, ok, err := activeStore.ClaimNextRun(context.Background(), "local-smoke-worker-2", time.Now().Add(time.Minute))
	if err != nil {
		return err
	}
	if ok {
		return fmt.Errorf("second active local-model claim = %s, want blocked while %s is running", blocked.ID, firstLocalRunID)
	}
	if err := activeStore.FailRun(context.Background(), firstLocalRunID, "local-smoke released active local model", time.Now().UTC()); err != nil {
		return err
	}
	claimed, ok, err = activeStore.ClaimNextRun(context.Background(), "local-smoke-worker-3", time.Now().Add(time.Minute))
	if err != nil {
		return err
	}
	if !ok || claimed.ID != secondLocalRunID {
		return fmt.Errorf("released local-model claim = %s ok=%t, want %s", claimed.ID, ok, secondLocalRunID)
	}

	r.logf("p2: verify timeout failure progress")
	timeoutConfig := localModelSmokeConfig(r.root, filepath.Join(r.workDir, "p2-timeout"))
	timeoutConfig.RunTimeout = 20 * time.Millisecond
	timeoutStore := store.NewMemoryStore()
	timeoutPending := app.NewPendingInputs()
	timeoutServer := httptest.NewServer(app.NewServer(timeoutConfig, timeoutStore, timeoutPending).Handler())
	defer timeoutServer.Close()
	timeoutRunID, err := r.createRun(timeoutServer.Client(), timeoutServer.URL, localModelRunRequest(seeded.Settings))
	if err != nil {
		return err
	}
	slowProvider := &sleepingProvider{delay: 200 * time.Millisecond, done: make(chan struct{})}
	processed, err := app.NewWorker(timeoutConfig, timeoutStore, timeoutPending, func(core.ProviderConfig) (llm.Provider, error) {
		return slowProvider, nil
	}).ProcessOne(context.Background())
	if !processed {
		return fmt.Errorf("expected timeout worker to process one run")
	}
	if err == nil {
		return fmt.Errorf("expected timeout worker error")
	}
	if err := slowProvider.wait(); err != nil {
		return err
	}
	timeoutRunBody, err := requestBody(timeoutServer.Client(), http.MethodGet, timeoutServer.URL+"/api/runs/"+timeoutRunID, nil, http.StatusOK)
	if err != nil {
		return err
	}
	var timeoutDoc struct {
		Progress core.RunProgress `json:"progress"`
	}
	if err := json.Unmarshal(timeoutRunBody, &timeoutDoc); err != nil {
		return err
	}
	if timeoutDoc.Progress.State != "failed" || timeoutDoc.Progress.Recovery == nil || timeoutDoc.Progress.Recovery.Title != "Report timed out" {
		return fmt.Errorf("timeout progress = %+v, want failed timeout recovery", timeoutDoc.Progress)
	}

	r.logf("p2: verify local-model artifact writes and summary")
	localConfig := localModelSmokeConfig(r.root, filepath.Join(r.workDir, "p2-local-model"))
	localStore := store.NewMemoryStore()
	localPending := app.NewPendingInputs()
	localServer := httptest.NewServer(app.NewServer(localConfig, localStore, localPending).Handler())
	defer localServer.Close()
	localRunID, err := r.createRun(localServer.Client(), localServer.URL, localModelRunRequest(seeded.Settings))
	if err != nil {
		return err
	}
	responses := []string{
		`{"i":[[1,"cooked oatmeal",1,"cup"],[2,"blueberries",1,"cup"],[3,"plain Greek yogurt",1,"cup"]]}`,
		`{"i":[[4,"chicken breast",4,"oz"],[5,"brown rice",1,"cup"],[6,"broccoli",1,"cup"]]}`,
		`{"i":[[7,"salmon",4,"oz"],[8,"sweet potato",1,"cup"],[9,"spinach",1,"cup"]]}`,
	}
	if err := processOne(localConfig, localStore, localPending, func(core.ProviderConfig) (llm.Provider, error) {
		return &sequenceProvider{responses: responses}, nil
	}); err != nil {
		return err
	}
	if _, err := requestBody(localServer.Client(), http.MethodGet, localServer.URL+"/api/runs/"+localRunID+"/artifacts/optional/local-model-chunks.json", nil, http.StatusOK); err != nil {
		return err
	}
	summary, err := localmodelsummary.Build(localConfig.ArtifactDir)
	if err != nil {
		return err
	}
	if summary.RunCount != 1 || summary.ChunkCount != 3 || summary.Runs[0].SourceItemCount != 9 {
		return fmt.Errorf("local-model summary = %+v, want one 3-chunk 9-source run", summary)
	}
	return nil
}

func (r *runner) verifyCORS(client *http.Client, baseURL string) error {
	r.logf("hosted: verify CORS allowed and disallowed origins")
	allowedReq, err := http.NewRequest(http.MethodOptions, baseURL+"/api/runs", nil)
	if err != nil {
		return err
	}
	allowedReq.Header.Set("Origin", allowedOrigin)
	allowedReq.Header.Set("Access-Control-Request-Method", http.MethodPost)
	allowedResp, err := client.Do(allowedReq)
	if err != nil {
		return err
	}
	allowedResp.Body.Close()
	if allowedResp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("allowed preflight status = %d, want 204", allowedResp.StatusCode)
	}
	if got := allowedResp.Header.Get("Access-Control-Allow-Origin"); got != allowedOrigin {
		return fmt.Errorf("allowed origin header = %q, want %q", got, allowedOrigin)
	}

	disallowedReq, err := http.NewRequest(http.MethodOptions, baseURL+"/api/runs", nil)
	if err != nil {
		return err
	}
	disallowedReq.Header.Set("Origin", "https://example.invalid")
	disallowedReq.Header.Set("Access-Control-Request-Method", http.MethodPost)
	disallowedResp, err := client.Do(disallowedReq)
	if err != nil {
		return err
	}
	disallowedResp.Body.Close()
	if disallowedResp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("disallowed preflight status = %d, want 204", disallowedResp.StatusCode)
	}
	if got := disallowedResp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		return fmt.Errorf("disallowed origin header = %q, want empty", got)
	}
	return nil
}

func (r *runner) createRun(client *http.Client, baseURL string, body core.CreateRunRequest) (string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/runs", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", allowedOrigin)
	req.Header.Set("X-MealCheck-Invite-Token", inviteToken)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("create run status = %d body=%s", resp.StatusCode, string(data))
	}
	var created core.CreateRunResponse
	if err := json.Unmarshal(data, &created); err != nil {
		return "", err
	}
	if created.RunID == "" {
		return "", fmt.Errorf("create run returned empty run_id")
	}
	return created.RunID, nil
}

func smokeConfig(root, workDir string) core.Config {
	dataDir := filepath.Join(workDir, "hosted-data")
	return core.Config{
		Root:             root,
		DataDir:          dataDir,
		ArtifactDir:      filepath.Join(dataDir, "artifacts"),
		Addr:             "127.0.0.1:0",
		StoreKind:        "memory",
		AllowedOrigin:    allowedOrigin,
		InviteToken:      inviteToken,
		QueueSize:        3,
		MaxCasesPerRun:   20,
		MaxUploadBytes:   1_000_000,
		RunTimeout:       10 * time.Minute,
		Retention:        7 * 24 * time.Hour,
		WorkerPoll:       time.Millisecond,
		CleanupInterval:  time.Hour,
		DemoIndexPath:    filepath.Join(root, "examples", "seeded-one-day-peanut-allergy", "artifacts", "demo-runs", "index.json"),
		DemoArtifactRoot: filepath.Join(root, "examples", "seeded-one-day-peanut-allergy", "artifacts"),
	}
}

func localModelSmokeConfig(root, workDir string) core.Config {
	config := smokeConfig(root, workDir)
	config.HostedMode = core.HostedModeLocalModel
	config.LocalModelEnabled = true
	config.LocalModelBaseURL = "http://127.0.0.1:11435/v1"
	config.LocalModelName = "/tmp/MealCheck/models/Qwen3-0.6B-Q4_K_M.gguf"
	config.LocalModelTimeout = 2 * time.Second
	config.LocalModelMaxInputChars = 3_000
	config.LocalModelMaxSourceItems = 20
	config.LocalModelMaxOutputTokens = 160
	return config
}

func localModelRunRequest(settings checker.Settings) core.CreateRunRequest {
	settings.VerificationConstraints.Days = 1
	settings.VerificationConstraints.MealsPerDay = 0
	settings.VerificationConstraints.RequiresPrepSafetyNotes = false
	return core.CreateRunRequest{
		InputMode:     core.InputModeLocalModel,
		Settings:      settings,
		CandidateText: "Breakfast: 1 cup cooked oatmeal, 1 cup blueberries, 1 cup plain Greek yogurt.\nLunch: 4 oz chicken breast, 1 cup brown rice, 1 cup broccoli.\nDinner: 4 oz salmon, 1 cup sweet potato, 1 cup spinach.",
	}
}

func localModelRunBody(settings checker.Settings) string {
	body, err := json.Marshal(localModelRunRequest(settings))
	if err != nil {
		panic(err)
	}
	return string(body)
}

func processOne(config core.Config, store store.Store, pending *app.PendingInputs, providerFactory llm.ProviderFactory) error {
	processed, err := app.NewWorker(config, store, pending, providerFactory).ProcessOne(context.Background())
	if err != nil {
		return err
	}
	if !processed {
		return fmt.Errorf("expected worker to process one run")
	}
	return nil
}

type sequenceProvider struct {
	responses []string
	index     int
}

func (p *sequenceProvider) Complete(_ context.Context, _ core.ProviderConfig, _ []llm.ProviderMessage) (string, error) {
	if p.index >= len(p.responses) {
		return "", fmt.Errorf("sequence provider exhausted")
	}
	response := p.responses[p.index]
	p.index++
	return response, nil
}

type sleepingProvider struct {
	delay time.Duration
	done  chan struct{}
}

func (p *sleepingProvider) Complete(ctx context.Context, _ core.ProviderConfig, _ []llm.ProviderMessage) (string, error) {
	if p.done != nil {
		defer close(p.done)
	}
	time.Sleep(p.delay)
	return "", ctx.Err()
}

func (p *sleepingProvider) wait() error {
	if p.done == nil {
		return nil
	}
	select {
	case <-p.done:
		return nil
	case <-time.After(time.Second):
		return fmt.Errorf("slow provider did not finish after timeout smoke")
	}
}

func verifyCompletedRun(client *http.Client, baseURL, runID string) error {
	runBody, err := requestBody(client, http.MethodGet, baseURL+"/api/runs/"+runID, nil, http.StatusOK)
	if err != nil {
		return err
	}
	var runDoc struct {
		Run core.Run `json:"run"`
	}
	if err := json.Unmarshal(runBody, &runDoc); err != nil {
		return err
	}
	if runDoc.Run.Status != core.StatusCompleted {
		return fmt.Errorf("run %s status = %q, want completed", runID, runDoc.Run.Status)
	}

	reportBody, err := requestBody(client, http.MethodGet, baseURL+"/api/runs/"+runID+"/report", nil, http.StatusOK)
	if err != nil {
		return err
	}
	var report map[string]any
	if err := json.Unmarshal(reportBody, &report); err != nil {
		return err
	}
	if report["decision"] != "block" {
		return fmt.Errorf("run %s report decision = %v, want block", runID, report["decision"])
	}

	artifactBody, err := requestBody(client, http.MethodGet, baseURL+"/api/runs/"+runID+"/artifacts", nil, http.StatusOK)
	if err != nil {
		return err
	}
	if !bytes.Contains(artifactBody, []byte("decision.json")) {
		return fmt.Errorf("run %s artifact list missing decision.json", runID)
	}
	eventBody, err := requestBody(client, http.MethodGet, baseURL+"/api/runs/"+runID+"/events", nil, http.StatusOK)
	if err != nil {
		return err
	}
	for _, expected := range []string{"event: queued", "event: started", "event: artifact_written", "event: completed"} {
		if !bytes.Contains(eventBody, []byte(expected)) {
			return fmt.Errorf("run %s events missing %q", runID, expected)
		}
	}
	return nil
}

func verifyRedaction(client *http.Client, baseURL, dataDir, runID string) error {
	redactedBody, err := requestBody(client, http.MethodGet, baseURL+"/api/runs/"+runID+"/artifacts/configs/redacted-provider.json", nil, http.StatusOK)
	if err != nil {
		return err
	}
	var redacted core.RedactedProviderConfig
	if err := json.Unmarshal(redactedBody, &redacted); err != nil {
		return err
	}
	if redacted.APIKey != "redacted" {
		return fmt.Errorf("redacted provider api_key = %q, want redacted", redacted.APIKey)
	}
	if bytes.Contains(redactedBody, []byte(providerSecret)) {
		return fmt.Errorf("redacted provider artifact contains provider secret")
	}
	return assertFileTreeDoesNotContain(dataDir, providerSecret)
}

func deleteRun(client *http.Client, baseURL, runID string) error {
	if _, err := requestBody(client, http.MethodDelete, baseURL+"/api/runs/"+runID, nil, http.StatusOK); err != nil {
		return err
	}
	if _, err := requestBody(client, http.MethodGet, baseURL+"/api/runs/"+runID, nil, http.StatusNotFound); err != nil {
		return err
	}
	return nil
}

func expectStatus(client *http.Client, method, url, body, inviteToken string, status int) error {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if inviteToken != "" {
		req.Header.Set("X-MealCheck-Invite-Token", inviteToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s status = %d, want %d body=%s", method, url, resp.StatusCode, status, string(data))
	}
	return nil
}

func expectErrorCode(client *http.Client, method, url, body, inviteToken string, status int, code string) error {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	data, err := requestBodyWithInvite(client, method, url, reader, inviteToken, status)
	if err != nil {
		return err
	}
	var response core.ErrorResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return err
	}
	if response.Error.Code != code {
		return fmt.Errorf("%s %s error code = %q, want %q body=%s", method, url, response.Error.Code, code, string(data))
	}
	return nil
}

func requestBody(client *http.Client, method, url string, body io.Reader, status int) ([]byte, error) {
	return requestBodyWithInvite(client, method, url, body, "", status)
}

func requestBodyWithInvite(client *http.Client, method, url string, body io.Reader, inviteToken string, status int) ([]byte, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", allowedOrigin)
	if inviteToken != "" {
		req.Header.Set("X-MealCheck-Invite-Token", inviteToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != status {
		return nil, fmt.Errorf("%s %s status = %d, want %d body=%s", method, url, resp.StatusCode, status, string(data))
	}
	return data, nil
}

func runCommand(dir string, expectedExit int, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if err == nil {
		if expectedExit != 0 {
			return output.String(), fmt.Errorf("exit code = 0, want %d", expectedExit)
		}
		return output.String(), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == expectedExit {
		return output.String(), nil
	}
	return output.String(), err
}

func readJSON(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func assertFileTreeDoesNotContain(root, secret string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
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
			return fmt.Errorf("%s contains provider secret", path)
		}
		return nil
	})
}
