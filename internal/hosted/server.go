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
)

type Server struct {
	Config  Config
	Store   Store
	Pending *PendingInputs
	mux     *http.ServeMux
}

func NewServer(config Config, store Store, pending ...*PendingInputs) *Server {
	pendingInputs := NewPendingInputs()
	if len(pending) > 0 && pending[0] != nil {
		pendingInputs = pending[0]
	}
	s := &Server{Config: config, Store: store, Pending: pendingInputs, mux: http.NewServeMux()}
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
		"status":           "ok",
		"store":            s.Config.StoreKind,
		"queued_runs":      stats.Queued,
		"running_runs":     stats.Running,
		"queue_size":       s.Config.QueueSize,
		"active_run_limit": 1,
		"retention_days":   int(s.Config.Retention.Hours() / 24),
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

	repairJSON := inputMode == "profile_generation" || inputMode == "prompt_generation"
	if request.RepairJSON != nil {
		repairJSON = *request.RepairJSON
	}
	pendingInput := PendingRunInput{
		Mode:             inputMode,
		Profile:          request.Profile,
		Constraints:      request.Constraints,
		CandidatePlan:    request.CandidatePlan,
		GenerationPrompt: strings.TrimSpace(request.GenerationPrompt),
		Provider:         normalizeProviderConfig(request.Provider),
		RepairJSON:       repairJSON,
	}
	if err := validateProfileAndConstraints(pendingInput.Profile, pendingInput.Constraints); err != nil {
		return "", PendingRunInput{}, false, err
	}

	switch inputMode {
	case "manual_structured":
		if pendingInput.CandidatePlan == nil {
			return "", PendingRunInput{}, false, fmt.Errorf("candidate_plan is required for manual_structured")
		}
		if err := validatePlan(*pendingInput.CandidatePlan); err != nil {
			return "", PendingRunInput{}, false, err
		}
	case "profile_generation":
		if err := validateProviderConfig(pendingInput.Provider); err != nil {
			return "", PendingRunInput{}, false, err
		}
	case "prompt_generation":
		if pendingInput.GenerationPrompt == "" {
			return "", PendingRunInput{}, false, fmt.Errorf("generation_prompt is required for prompt_generation")
		}
		if err := validateProviderConfig(pendingInput.Provider); err != nil {
			return "", PendingRunInput{}, false, err
		}
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
		request.GenerationPrompt != "" ||
		request.Provider.Type != "" ||
		request.Provider.BaseURL != "" ||
		request.Provider.Model != "" ||
		request.Provider.APIKey != "" ||
		request.RepairJSON != nil
}

func normalizeProviderConfig(config ProviderConfig) ProviderConfig {
	providerType := strings.TrimSpace(config.Type)
	if providerType == "" {
		providerType = ProviderTypeOpenAICompatible
	}
	normalized := ProviderConfig{
		Type:    providerType,
		BaseURL: strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		Model:   strings.TrimSpace(config.Model),
		APIKey:  strings.TrimSpace(config.APIKey),
	}
	if providerType != ProviderTypeOpenAICompatible {
		normalized.BaseURL = ""
	}
	return normalized
}

func validateProviderConfig(config ProviderConfig) error {
	switch config.Type {
	case ProviderTypeOpenAICompatible, ProviderTypeOpenAI, ProviderTypeAnthropic, ProviderTypeGemini:
	default:
		return fmt.Errorf("unsupported provider type %q", config.Type)
	}
	if config.Model == "" {
		return fmt.Errorf("provider model is required")
	}
	if config.APIKey == "" {
		return fmt.Errorf("provider api_key is required")
	}
	return nil
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
