package hosted

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/checker"
)

type Server struct {
	Config          Config
	Store           Store
	Pending         *PendingInputs
	Policy          *PolicyLimiter
	ProviderFactory ProviderFactory
	mux             *http.ServeMux
}

func NewServer(config Config, store Store, pending ...*PendingInputs) *Server {
	pendingInputs := NewPendingInputs()
	if len(pending) > 0 && pending[0] != nil {
		pendingInputs = pending[0]
	}
	s := &Server{Config: config, Store: store, Pending: pendingInputs, Policy: NewPolicyLimiter(), ProviderFactory: DefaultProviderFactory, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.withMiddleware(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/demo-runs", s.handleDemoRuns)
	s.mux.HandleFunc("/api/demo-runs/", s.handleDemoRun)
	s.mux.HandleFunc("/api/qualify", s.handleQualify)
	s.mux.HandleFunc("/api/runs", s.handleRuns)
	s.mux.HandleFunc("/api/runs/", s.handleRun)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	stats, err := s.Store.Stats(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_unavailable", "store unavailable", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":                      "ok",
		"store":                       s.Config.StoreKind,
		"access_mode":                 accessMode(s.Config),
		"hosted_mode":                 hostedMode(s.Config),
		"queued_runs":                 stats.Queued,
		"running_runs":                stats.Running,
		"queue_size":                  s.Config.QueueSize,
		"active_run_limit":            1,
		"retention_days":              int(s.Config.Retention.Hours() / 24),
		"public_openai_compatible":    s.Config.PublicOpenAICompatible,
		"max_candidate_text_chars":    s.Config.MaxCandidateTextChars,
		"max_generation_prompt_chars": s.Config.MaxGenerationPromptChars,
		"local_model":                 s.localModelHealth(r.Context()),
		"policy": map[string]any{
			"public_request_limit":      s.Config.PublicRequestLimit,
			"public_request_window_sec": int(s.Config.PublicRequestWindow.Seconds()),
			"public_daily_run_limit":    s.Config.PublicDailyRunLimit,
			"queue_size":                s.Config.QueueSize,
			"active_run_limit":          1,
		},
	})
}

func (s *Server) handleDemoRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	index, err := s.loadDemoIndex()
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "demo_index_unavailable", err.Error(), nil)
		return
	}
	type demoResponse struct {
		DemoRun
		Links RunLinks `json:"links"`
	}
	response := struct {
		SchemaVersion string         `json:"schema_version"`
		DemoRuns      []demoResponse `json:"demo_runs"`
	}{SchemaVersion: index.SchemaVersion}
	for _, demo := range index.DemoRuns {
		response.DemoRuns = append(response.DemoRuns, demoResponse{
			DemoRun: demo,
			Links: RunLinks{
				Self:      "/api/demo-runs/" + demo.ID,
				Report:    "/api/demo-runs/" + demo.ID + "/report",
				Artifacts: "/api/demo-runs/" + demo.ID + "/artifacts",
			},
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleDemoRun(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/demo-runs/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, r, http.StatusNotFound, "not_found", "demo run not found", nil)
		return
	}
	demo, ok, err := s.findDemo(parts[0])
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "demo_index_unavailable", err.Error(), nil)
		return
	}
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "demo run not found", nil)
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		decision, err := readJSONFile(filepath.Join(s.Config.DemoArtifactRoot, demo.BasePath, "decision.json"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "demo_unavailable", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"demo": demo, "decision": decision})
		return
	}
	switch parts[1] {
	case "report":
		s.serveDemoReport(w, r, demo)
	case "artifacts":
		s.serveDemoArtifact(w, r, demo, strings.Join(parts[2:], "/"))
	default:
		writeError(w, r, http.StatusNotFound, "not_found", "demo route not found", nil)
	}
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createRun(w, r)
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleQualify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if _, err := s.authorizeRun(r); errors.Is(err, ErrInviteRunLimit) {
		writeError(w, r, http.StatusTooManyRequests, "invite_limit_reached", "access code run limit reached", nil)
		return
	} else if err != nil {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "valid access code required", nil)
		return
	}
	if err := s.enforcePublicRequestPolicy(w, r); err != nil {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.Config.MaxUploadBytes)
	var request MealPlanQualificationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON request", nil)
		return
	}
	providerSupplied := hasProviderConfigFields(request.Provider)
	request.Text = strings.TrimSpace(request.Text)
	request.Provider = normalizeProviderConfig(request.Provider)
	if request.Text == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "text is required", nil)
		return
	}
	textLimit := s.Config.MaxCandidateTextChars
	if hostedMode(s.Config) == HostedModeLocalModel {
		textLimit = s.Config.LocalModelMaxInputChars
	}
	if err := validateTextLength("text", request.Text, textLimit); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if err := validateSettings(request.Settings); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if hostedMode(s.Config) == HostedModeLocalModel {
		if providerSupplied {
			writeError(w, r, http.StatusBadRequest, "invalid_request", "hosted local model mode does not accept client provider configuration", nil)
			return
		}
		if err := validateLocalModelAvailable(s.Config); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "local_model_unavailable", err.Error(), nil)
			return
		}
		if err := validateLocalModelSettings(request.Settings); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		request.Provider = localModelProviderConfig(s.Config)
	} else if providerSupplied {
		if err := validatePublicProviderPolicy(s.Config, request.Provider); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
	}

	providerFactory := s.ProviderFactory
	if providerFactory == nil {
		providerFactory = DefaultProviderFactory
	}
	qualification, err := QualifyMealPlanText(r.Context(), providerFactory, request)
	if err != nil {
		if isProviderConfigError(err) {
			writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		writeError(w, r, http.StatusBadGateway, "provider_error", sanitizeProviderErrorText(err.Error(), request.Provider.APIKey), nil)
		return
	}
	writeJSON(w, http.StatusOK, QualifyMealPlanResponse{Qualification: qualification})
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/runs/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, r, http.StatusNotFound, "not_found", "run not found", nil)
		return
	}
	runID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			s.getRun(w, r, runID)
		case http.MethodDelete:
			s.deleteRun(w, r, runID)
		default:
			writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		}
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	switch parts[1] {
	case "events":
		s.runEvents(w, r, runID)
	case "report":
		s.runReport(w, r, runID)
	case "artifacts":
		s.runArtifact(w, r, runID, strings.Join(parts[2:], "/"))
	default:
		writeError(w, r, http.StatusNotFound, "not_found", "run route not found", nil)
	}
}

func (s *Server) createRun(w http.ResponseWriter, r *http.Request) {
	inviteTokenID, err := s.authorizeRun(r)
	if errors.Is(err, ErrInviteRunLimit) {
		writeError(w, r, http.StatusTooManyRequests, "invite_limit_reached", "access code run limit reached", nil)
		return
	}
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "valid access code required", nil)
		return
	}
	if err := s.enforcePublicRequestPolicy(w, r); err != nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.Config.MaxUploadBytes)
	var request CreateRunRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON request", nil)
		return
	}

	casePath, pendingInput, hasPending, err := requestRunInput(s.Config, request)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	ip := requestClientIP(r)
	if accessMode(s.Config) == AccessModePublicBYOK {
		if err := s.Policy.CheckDailyRunLimit(ip, time.Now().UTC(), s.Config.PublicDailyRunLimit); err != nil {
			writePolicyError(w, r, err)
			return
		}
	}

	run := newRun(s.Config, casePath)
	if hasPending {
		run.CasePath = runtimeCasePath(s.Config, run.ID)
		s.Pending.Put(run.ID, pendingInput, pendingInputExpiresAt(s.Config, run))
	}
	if err := s.Store.CreateRun(r.Context(), run, s.Config.QueueSize, inviteTokenID); err != nil {
		if hasPending {
			s.Pending.Delete(run.ID)
		}
		if errors.Is(err, ErrQueueFull) {
			writeError(w, r, http.StatusTooManyRequests, "queue_full", "run queue is full", map[string]any{"queue_size": s.Config.QueueSize})
			return
		}
		if errors.Is(err, ErrInviteRunLimit) {
			writeError(w, r, http.StatusTooManyRequests, "invite_limit_reached", "access code run limit reached", nil)
			return
		}
		if errors.Is(err, ErrInviteUnavailable) {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", "valid access code required", nil)
			return
		}
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	if accessMode(s.Config) == AccessModePublicBYOK {
		s.Policy.RecordRun(ip, run.CreatedAt)
	}
	if err := s.Store.AppendEvent(r.Context(), run.ID, EventQueued, "run queued", time.Now().UTC()); err != nil {
		if hasPending {
			s.Pending.Delete(run.ID)
		}
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusAccepted, CreateRunResponse{
		RunID:     run.ID,
		Status:    run.Status,
		ExpiresAt: run.ExpiresAt,
		Links:     linksForRun(run.ID),
	})
}

func requestRunInput(config Config, request CreateRunRequest) (string, PendingRunInput, bool, error) {
	inputMode := strings.TrimSpace(request.InputMode)
	if inputMode == "" {
		if hasDynamicRunFields(request) {
			return "", PendingRunInput{}, false, fmt.Errorf("input_mode is required for hosted meal-plan input")
		}
		casePath, err := cleanCasePath(config.Root, request.CasePath)
		if err != nil {
			return "", PendingRunInput{}, false, err
		}
		return casePath, PendingRunInput{}, false, nil
	}
	if strings.TrimSpace(request.CasePath) != "" {
		return "", PendingRunInput{}, false, fmt.Errorf("case_path cannot be combined with input_mode")
	}

	repairJSON := inputMode == InputModeProfileGeneration || inputMode == InputModePromptGeneration
	if request.RepairJSON != nil {
		repairJSON = *request.RepairJSON
	}
	pendingInput := PendingRunInput{
		Mode:             inputMode,
		Settings:         request.Settings,
		CandidatePlan:    request.CandidatePlan,
		GenerationPrompt: strings.TrimSpace(request.GenerationPrompt),
		CandidateText:    strings.TrimSpace(request.CandidateText),
		Provider:         normalizeProviderConfig(request.Provider),
		RepairJSON:       repairJSON,
	}
	if err := validateSettings(pendingInput.Settings); err != nil {
		return "", PendingRunInput{}, false, err
	}
	if err := validateTextLength("generation_prompt", pendingInput.GenerationPrompt, config.MaxGenerationPromptChars); err != nil {
		return "", PendingRunInput{}, false, err
	}

	switch inputMode {
	case InputModeManualStructured:
		return "", PendingRunInput{}, false, fmt.Errorf("manual_structured is supported only by the local CLI/debug workflow; hosted live runs require model-backed generation")
	case InputModeProfileGeneration:
		if hostedMode(config) == HostedModeLocalModel {
			return "", PendingRunInput{}, false, fmt.Errorf("profile_generation is disabled in hosted local model mode; use local_model candidate_text input")
		}
		if err := validateProviderConfig(pendingInput.Provider); err != nil {
			return "", PendingRunInput{}, false, err
		}
		if err := validatePublicProviderPolicy(config, pendingInput.Provider); err != nil {
			return "", PendingRunInput{}, false, err
		}
	case InputModePromptGeneration:
		if hostedMode(config) == HostedModeLocalModel {
			return "", PendingRunInput{}, false, fmt.Errorf("prompt_generation is disabled in hosted local model mode; use local_model candidate_text input")
		}
		if pendingInput.GenerationPrompt == "" {
			return "", PendingRunInput{}, false, fmt.Errorf("generation_prompt is required for prompt_generation")
		}
		if err := validateProviderConfig(pendingInput.Provider); err != nil {
			return "", PendingRunInput{}, false, err
		}
		if err := validatePublicProviderPolicy(config, pendingInput.Provider); err != nil {
			return "", PendingRunInput{}, false, err
		}
	case InputModeLocalModel:
		if err := validateLocalModelAvailable(config); err != nil {
			return "", PendingRunInput{}, false, err
		}
		if hasProviderConfigFields(request.Provider) {
			return "", PendingRunInput{}, false, fmt.Errorf("local_model uses the server-owned local model; omit provider")
		}
		if pendingInput.CandidateText == "" {
			return "", PendingRunInput{}, false, fmt.Errorf("candidate_text is required for local_model")
		}
		if err := validateTextLength("candidate_text", pendingInput.CandidateText, config.LocalModelMaxInputChars); err != nil {
			return "", PendingRunInput{}, false, err
		}
		if err := validateLocalModelSettings(pendingInput.Settings); err != nil {
			return "", PendingRunInput{}, false, err
		}
		pendingInput.Provider = localModelProviderConfig(config)
		pendingInput.RepairJSON = false
	default:
		return "", PendingRunInput{}, false, fmt.Errorf("unsupported input_mode %q", inputMode)
	}
	return "", pendingInput, true, nil
}

func pendingInputExpiresAt(config Config, run Run) time.Time {
	ttl := config.PendingInputTTL
	if ttl == 0 {
		ttl = defaultPendingInputTTL(config.QueueSize, config.RunTimeout)
	}
	expiresAt := run.CreatedAt.Add(ttl)
	if !run.ExpiresAt.IsZero() && run.ExpiresAt.Before(expiresAt) {
		return run.ExpiresAt
	}
	return expiresAt
}

func hasDynamicRunFields(request CreateRunRequest) bool {
	return request.CandidatePlan != nil ||
		request.CandidateText != "" ||
		hasSettingsFields(request.Settings) ||
		request.GenerationPrompt != "" ||
		request.Provider.Type != "" ||
		request.Provider.BaseURL != "" ||
		request.Provider.Model != "" ||
		request.Provider.APIKey != "" ||
		request.RepairJSON != nil
}

func hasSettingsFields(settings checker.Settings) bool {
	targets := settings.NutritionTargets
	constraints := settings.VerificationConstraints
	return targets.CalorieTargetKcal != 0 ||
		targets.ProteinTargetG != 0 ||
		constraints.Days != 0 ||
		constraints.MealsPerDay != 0 ||
		len(constraints.Allergies) > 0 ||
		len(constraints.ExcludedFoods) > 0 ||
		constraints.MaxSodiumMGPerDay != 0 ||
		constraints.MaxAddedSugarGPerMeal != 0 ||
		constraints.MaxSaturatedFatPctCalories != 0 ||
		constraints.CalorieTolerancePct != 0 ||
		constraints.RequiresPrepSafetyNotes
}

func hasProviderConfigFields(config ProviderConfig) bool {
	return config.Type != "" ||
		config.BaseURL != "" ||
		config.Model != "" ||
		config.APIKey != "" ||
		config.MaxTokens != 0
}

func normalizeProviderConfig(config ProviderConfig) ProviderConfig {
	providerType := strings.TrimSpace(config.Type)
	if providerType == "" {
		providerType = ProviderTypeOpenAICompatible
	}
	normalized := ProviderConfig{
		Type:      providerType,
		BaseURL:   strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		Model:     strings.TrimSpace(config.Model),
		APIKey:    strings.TrimSpace(config.APIKey),
		MaxTokens: config.MaxTokens,
		Timeout:   config.Timeout,
	}
	if providerType != ProviderTypeOpenAICompatible && providerType != ProviderTypeLocalLlama {
		normalized.BaseURL = ""
	}
	return normalized
}

func validateProviderConfig(config ProviderConfig) error {
	switch config.Type {
	case ProviderTypeOpenAICompatible, ProviderTypeOpenAI, ProviderTypeAnthropic, ProviderTypeGemini, ProviderTypeLocalLlama:
	default:
		return fmt.Errorf("unsupported provider type %q", config.Type)
	}
	if config.Model == "" {
		return fmt.Errorf("provider model is required")
	}
	if config.Type == ProviderTypeLocalLlama {
		return nil
	}
	if config.APIKey == "" {
		return fmt.Errorf("provider api_key is required")
	}
	return nil
}

func validateTextLength(field, value string, limit int) error {
	if limit <= 0 || value == "" {
		return nil
	}
	if len([]rune(value)) > limit {
		return fmt.Errorf("%s exceeds maximum length of %d characters", field, limit)
	}
	return nil
}

func isProviderConfigError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.HasPrefix(message, "unsupported provider type ") ||
		message == "provider model is required" ||
		message == "provider base_url is required" ||
		message == "provider api_key is required"
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request, runID string) {
	run, err := s.Store.GetRun(r.Context(), runID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "links": linksForRun(run.ID)})
}

func (s *Server) deleteRun(w http.ResponseWriter, r *http.Request, runID string) {
	run, err := s.Store.DeleteRun(r.Context(), runID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	s.Pending.Delete(runID)
	if run.ArtifactDir != "" {
		if err := removeArtifactDir(s.Config.ArtifactDir, run.ArtifactDir); err != nil {
			writeError(w, r, http.StatusInternalServerError, "artifact_delete_failed", err.Error(), nil)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": StatusDeleted})
}

func (s *Server) runEvents(w http.ResponseWriter, r *http.Request, runID string) {
	afterID, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	events, err := s.Store.ListEvents(r.Context(), runID, afterID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	for _, event := range events {
		fmt.Fprintf(w, "id: %d\n", event.ID)
		fmt.Fprintf(w, "event: %s\n", event.Type)
		fmt.Fprintf(w, "data: %s\n\n", jsonRaw(event))
	}
}

func (s *Server) runReport(w http.ResponseWriter, r *http.Request, runID string) {
	run, err := s.Store.GetRun(r.Context(), runID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	serveArtifactFile(w, r, s.Config.ArtifactDir, filepath.Join(run.ArtifactDir, "report.json"))
}

func (s *Server) runArtifact(w http.ResponseWriter, r *http.Request, runID, artifactPath string) {
	run, err := s.Store.GetRun(r.Context(), runID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	if artifactPath == "" {
		s.listArtifacts(w, r, run)
		return
	}
	path, err := safeArtifactPath(run.ArtifactDir, artifactPath)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_artifact_path", err.Error(), nil)
		return
	}
	serveArtifactFile(w, r, s.Config.ArtifactDir, path)
}

func (s *Server) listArtifacts(w http.ResponseWriter, r *http.Request, run Run) {
	s.listArtifactManifest(w, r, run.ID, run.ArtifactDir, "/api/runs/"+run.ID+"/artifacts")
}

func (s *Server) listArtifactManifest(w http.ResponseWriter, r *http.Request, id, artifactDir, urlPrefix string) {
	manifest, err := readJSONFile(filepath.Join(artifactDir, "manifest.json"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "artifacts_unavailable", "artifact manifest is not available", nil)
		return
	}
	paths, _ := manifest["artifacts"].([]any)
	var response ArtifactList
	response.RunID = id
	for _, value := range paths {
		path, ok := value.(string)
		if !ok {
			continue
		}
		response.Artifacts = append(response.Artifacts, ArtifactListItem{
			Path: path,
			URL:  urlPrefix + "/" + path,
			Type: artifactType(path),
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) serveDemoReport(w http.ResponseWriter, r *http.Request, demo DemoRun) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	serveArtifactFile(w, r, s.Config.DemoArtifactRoot, filepath.Join(s.Config.DemoArtifactRoot, demo.BasePath, "report.json"))
}

func (s *Server) serveDemoArtifact(w http.ResponseWriter, r *http.Request, demo DemoRun, artifactPath string) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	base := filepath.Join(s.Config.DemoArtifactRoot, demo.BasePath)
	if artifactPath == "" {
		s.listArtifactManifest(w, r, demo.ID, base, "/api/demo-runs/"+demo.ID+"/artifacts")
		return
	}
	path, err := safeArtifactPath(base, artifactPath)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_artifact_path", err.Error(), nil)
		return
	}
	serveArtifactFile(w, r, s.Config.DemoArtifactRoot, path)
}

func (s *Server) loadDemoIndex() (DemoIndex, error) {
	var index DemoIndex
	b, err := os.ReadFile(s.Config.DemoIndexPath)
	if err != nil {
		return DemoIndex{}, err
	}
	if err := json.Unmarshal(b, &index); err != nil {
		return DemoIndex{}, err
	}
	return index, nil
}

func (s *Server) findDemo(id string) (DemoRun, bool, error) {
	index, err := s.loadDemoIndex()
	if err != nil {
		return DemoRun{}, false, err
	}
	for _, demo := range index.DemoRuns {
		if demo.ID == id {
			return demo, true, nil
		}
	}
	return DemoRun{}, false, nil
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = "req_" + newID()
		}
		w.Header().Set("X-Request-ID", requestID)
		if s.Config.AllowedOrigin != "" {
			w.Header().Add("Vary", "Origin")
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin == s.Config.AllowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-MealCheck-Invite-Token, X-Request-ID")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, requestID)))
	})
}

func (s *Server) authorizeRun(r *http.Request) (string, error) {
	if accessMode(s.Config) == AccessModePublicBYOK {
		return "", nil
	}
	value := strings.TrimSpace(r.Header.Get("X-MealCheck-Invite-Token"))
	if s.Config.InviteToken != "" && value == s.Config.InviteToken {
		return "", nil
	}
	if value != "" {
		id, secret, ok := ParseInviteToken(value)
		if !ok {
			return "", ErrInviteUnavailable
		}
		invite, err := s.Store.GetInviteToken(r.Context(), id)
		if err != nil {
			return "", ErrInviteUnavailable
		}
		if err := ValidateInviteToken(invite, secret, time.Now().UTC()); err != nil {
			return "", err
		}
		return invite.ID, nil
	}
	if s.Config.InviteToken == "" && !s.Config.InviteRequired {
		return "", nil
	}
	return "", ErrInviteUnavailable
}

func (s *Server) enforcePublicRequestPolicy(w http.ResponseWriter, r *http.Request) error {
	if accessMode(s.Config) != AccessModePublicBYOK {
		return nil
	}
	if s.Policy == nil {
		s.Policy = NewPolicyLimiter()
	}
	if err := s.Policy.AllowRequest(requestClientIP(r), time.Now().UTC(), s.Config.PublicRequestLimit, s.Config.PublicRequestWindow); err != nil {
		writePolicyError(w, r, err)
		return err
	}
	return nil
}

func hostedMode(config Config) string {
	switch config.HostedMode {
	case HostedModeBYOK, HostedModeLocalModel:
		return config.HostedMode
	default:
		return HostedModeBYOK
	}
}

func accessMode(config Config) string {
	switch config.AccessMode {
	case AccessModePublicBYOK, AccessModeInviteRequired:
		return config.AccessMode
	default:
		if config.InviteToken != "" || config.InviteRequired {
			return AccessModeInviteRequired
		}
		return AccessModePublicBYOK
	}
}

func localModelProviderConfig(config Config) ProviderConfig {
	return ProviderConfig{
		Type:      ProviderTypeLocalLlama,
		BaseURL:   strings.TrimRight(strings.TrimSpace(config.LocalModelBaseURL), "/"),
		Model:     strings.TrimSpace(config.LocalModelName),
		MaxTokens: config.LocalModelMaxOutputTokens,
		Timeout:   config.LocalModelTimeout,
	}
}

func validateLocalModelAvailable(config Config) error {
	if !config.LocalModelEnabled {
		return fmt.Errorf("local model provider is not enabled")
	}
	provider := localModelProviderConfig(config)
	if provider.BaseURL == "" {
		return fmt.Errorf("local model base URL is not configured")
	}
	if provider.Model == "" {
		return fmt.Errorf("local model name is not configured")
	}
	return nil
}

func validateLocalModelSettings(settings checker.Settings) error {
	return nil
}

func (s *Server) localModelHealth(ctx context.Context) map[string]any {
	modelName := strings.TrimSpace(s.Config.LocalModelName)
	if modelName != "" {
		modelName = filepath.Base(modelName)
	}
	status := map[string]any{
		"enabled":                 s.Config.LocalModelEnabled,
		"ready":                   false,
		"model":                   modelName,
		"max_input_chars":         s.Config.LocalModelMaxInputChars,
		"max_output_tokens":       s.Config.LocalModelMaxOutputTokens,
		"timeout_sec":             int(s.Config.LocalModelTimeout.Seconds()),
		"supported_days":          7,
		"supported_meals_per_day": 6,
	}
	if !s.Config.LocalModelEnabled {
		return status
	}
	if err := validateLocalModelAvailable(s.Config); err != nil {
		status["error"] = err.Error()
		return status
	}
	baseURL := strings.TrimRight(strings.TrimSpace(s.Config.LocalModelBaseURL), "/")
	readyCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(readyCtx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		status["error"] = "local model readiness request could not be built"
		return status
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		status["error"] = "local model endpoint is not reachable"
		return status
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		status["ready"] = true
		return status
	}
	status["error"] = fmt.Sprintf("local model endpoint returned HTTP %d", resp.StatusCode)
	return status
}

func requestClientIP(r *http.Request) string {
	headers := map[string]string{
		"CF-Connecting-IP": r.Header.Get("CF-Connecting-IP"),
		"X-Forwarded-For":  r.Header.Get("X-Forwarded-For"),
		"X-Real-IP":        r.Header.Get("X-Real-IP"),
	}
	return clientIP(r.RemoteAddr, headers)
}

func linksForRun(runID string) RunLinks {
	return RunLinks{
		Self:      "/api/runs/" + runID,
		Events:    "/api/runs/" + runID + "/events",
		Report:    "/api/runs/" + runID + "/report",
		Artifacts: "/api/runs/" + runID + "/artifacts",
	}
}

func safeArtifactPath(root, requested string) (string, error) {
	if requested == "" || filepath.IsAbs(requested) {
		return "", fmt.Errorf("artifact path must be relative")
	}
	cleaned := filepath.Clean(requested)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("artifact path must stay inside artifact bundle")
	}
	return filepath.Join(root, cleaned), nil
}

func serveArtifactFile(w http.ResponseWriter, r *http.Request, root, path string) {
	path = filepath.Clean(path)
	rel, err := filepath.Rel(filepath.Clean(root), path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		writeError(w, r, http.StatusBadRequest, "invalid_artifact_path", "artifact path escapes root", nil)
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeError(w, r, http.StatusNotFound, "not_found", "artifact not found", nil)
			return
		}
		writeError(w, r, http.StatusInternalServerError, "artifact_error", err.Error(), nil)
		return
	}
	if info.IsDir() {
		writeError(w, r, http.StatusNotFound, "not_found", "artifact not found", nil)
		return
	}
	http.ServeFile(w, r, path)
}

func readJSONFile(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeStoreError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "not_found", "run not found", nil)
		return
	}
	writeError(w, r, http.StatusInternalServerError, "store_error", err.Error(), nil)
}

func writePolicyError(w http.ResponseWriter, r *http.Request, err error) {
	var policyErr PolicyError
	if errors.As(err, &policyErr) {
		if policyErr.RetryAfter > 0 {
			w.Header().Set("Retry-After", retryAfterHeader(policyErr.RetryAfter))
		}
		writeError(w, r, policyErr.Status, policyErr.Code, policyErr.Message, policyErr.Details)
		return
	}
	writeError(w, r, http.StatusTooManyRequests, "rate_limited", err.Error(), nil)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string, details map[string]any) {
	requestID, _ := r.Context().Value(requestIDKey{}).(string)
	if requestID == "" {
		requestID = r.Header.Get("X-Request-ID")
	}
	writeJSON(w, status, ErrorResponse{
		Error: APIError{
			Code:      code,
			Message:   message,
			RequestID: requestID,
			Details:   details,
		},
	})
}

func artifactType(path string) string {
	switch {
	case strings.HasSuffix(path, ".jsonl"):
		return "jsonl"
	case strings.HasSuffix(path, ".json"):
		return "json"
	case strings.HasSuffix(path, ".html"):
		return "html"
	case strings.HasSuffix(path, ".md"):
		return "markdown"
	default:
		return "file"
	}
}

type requestIDKey struct{}
