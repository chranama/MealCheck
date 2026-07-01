package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

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
		if writeMealPlanNotVerifiableError(w, r, err) {
			return
		}
		if isLocalModelAvailabilityError(err) {
			writeError(w, r, http.StatusServiceUnavailable, "local_model_unavailable", err.Error(), nil)
			return
		}
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
		run.InputMode = pendingInput.Mode
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
func (s *Server) getRun(w http.ResponseWriter, r *http.Request, runID string) {
	run, err := s.Store.GetRun(r.Context(), runID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	events, err := s.Store.ListEvents(r.Context(), runID, 0)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	for i := range events {
		events[i] = publicRunEvent(events[i])
	}
	writeJSON(w, http.StatusOK, runDocument(run, events))
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
		event = publicRunEvent(event)
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
func linksForRun(runID string) RunLinks {
	return RunLinks{
		Self:      "/api/runs/" + runID,
		Events:    "/api/runs/" + runID + "/events",
		Report:    "/api/runs/" + runID + "/report",
		Artifacts: "/api/runs/" + runID + "/artifacts",
	}
}
