package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/workflow/artifacts"
)

const (
	reviewArtifactPath = "review/normalized-plan-review.json"
	reviewActionsPath  = "review/review-actions.jsonl"
)

type reviewActionRequest struct {
	Reason string `json:"reason,omitempty"`
}

type reviewActionArtifact struct {
	SchemaVersion string `json:"schema_version"`
	RunID         string `json:"run_id"`
	Action        string `json:"action"`
	Reason        string `json:"reason,omitempty"`
	CreatedAt     string `json:"created_at"`
}

func (s *Server) runReview(w http.ResponseWriter, r *http.Request, runID string, parts []string) {
	if len(parts) == 0 {
		if r.Method != http.MethodGet {
			writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		s.runReviewArtifact(w, r, runID)
		return
	}
	if len(parts) != 1 || r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	switch parts[0] {
	case "confirm":
		s.confirmReview(w, r, runID)
	case "reject":
		s.finishReviewWithoutChecking(w, r, runID, "rejected", EventReviewRejected, "Normalized plan rejected before checking.")
	case "rewrite":
		s.finishReviewWithoutChecking(w, r, runID, "rewrite_requested", EventReviewRewrite, "Source text rewrite requested before checking.")
	default:
		writeError(w, r, http.StatusNotFound, "not_found", "review route not found", nil)
	}
}

func (s *Server) runReviewArtifact(w http.ResponseWriter, r *http.Request, runID string) {
	run, err := s.Store.GetRun(r.Context(), runID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	serveArtifactFile(w, r, s.Config.ArtifactDir, filepath.Join(run.ArtifactDir, reviewArtifactPath))
}

func (s *Server) confirmReview(w http.ResponseWriter, r *http.Request, runID string) {
	run, err := s.Store.GetRun(r.Context(), runID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	if run.Status != StatusAwaitingReview {
		writeError(w, r, http.StatusConflict, "review_not_available", "normalized-plan review is not awaiting confirmation", nil)
		return
	}
	request, err := decodeReviewActionRequest(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON request", nil)
		return
	}
	now := time.Now().UTC()
	reason := firstNonEmpty(request.Reason, "User confirmed normalized plan for checking.")
	if err := appendReviewAction(run.ArtifactDir, run.ID, "confirmed", reason, now); err != nil {
		writeError(w, r, http.StatusInternalServerError, "review_action_failed", err.Error(), nil)
		return
	}
	snapshot, err := snapshotReviewArtifacts(run.ArtifactDir)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "review_artifacts_unavailable", err.Error(), nil)
		return
	}
	run, err = s.Store.StartReviewRun(r.Context(), run.ID, "review-confirm", now.Add(s.Config.RunTimeout), now)
	if err != nil {
		writeReviewStoreError(w, r, err)
		return
	}
	if err := s.Store.AppendEvent(r.Context(), run.ID, EventReviewConfirmed, "normalized plan confirmed for checking", now); err != nil {
		writeStoreError(w, r, err)
		return
	}

	result, err := artifacts.WriteBundle(artifacts.BundleOptions{
		Root:              s.Config.Root,
		CasePath:          run.CasePath,
		OutDir:            run.ArtifactDir,
		Mode:              "hosted",
		FNDDSFallbackPath: s.Config.FNDDSFallbackPath,
	})
	restoreErr := restoreReviewArtifacts(run.ArtifactDir, snapshot)
	if err != nil {
		s.failConfirmedReviewRun(r, run.ID, fmt.Errorf("checking failed after review confirmation: %w", err), now)
		writeError(w, r, http.StatusInternalServerError, "bundle_failed", err.Error(), nil)
		return
	}
	if restoreErr != nil {
		s.failConfirmedReviewRun(r, run.ID, restoreErr, now)
		writeError(w, r, http.StatusInternalServerError, "review_artifact_restore_failed", restoreErr.Error(), nil)
		return
	}
	if err := updateManifestArtifacts(result.OutDir, snapshot.paths()...); err != nil {
		s.failConfirmedReviewRun(r, run.ID, err, now)
		writeError(w, r, http.StatusInternalServerError, "manifest_update_failed", err.Error(), nil)
		return
	}
	if err := s.Store.AppendEvent(r.Context(), run.ID, EventArtifactWritten, "artifact bundle written", now); err != nil {
		writeStoreError(w, r, err)
		return
	}
	if err := s.Store.CompleteRun(r.Context(), run.ID, result.Decision, now); err != nil {
		writeStoreError(w, r, err)
		return
	}
	if err := s.Store.AppendEvent(r.Context(), run.ID, EventCompleted, result.Decision.Decision, now); err != nil {
		writeStoreError(w, r, err)
		return
	}
	s.writeRunDocument(w, r, run.ID)
}

func (s *Server) finishReviewWithoutChecking(w http.ResponseWriter, r *http.Request, runID string, action string, eventType string, message string) {
	run, err := s.Store.GetRun(r.Context(), runID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	if run.Status != StatusAwaitingReview {
		writeError(w, r, http.StatusConflict, "review_not_available", "normalized-plan review is not awaiting action", nil)
		return
	}
	request, err := decodeReviewActionRequest(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON request", nil)
		return
	}
	now := time.Now().UTC()
	reason := firstNonEmpty(request.Reason, message)
	if err := appendReviewAction(run.ArtifactDir, run.ID, action, reason, now); err != nil {
		writeError(w, r, http.StatusInternalServerError, "review_action_failed", err.Error(), nil)
		return
	}
	if err := s.Store.AppendEvent(r.Context(), run.ID, eventType, message, now); err != nil {
		writeStoreError(w, r, err)
		return
	}
	if err := s.Store.FailRun(r.Context(), run.ID, message, now); err != nil {
		writeStoreError(w, r, err)
		return
	}
	if err := s.Store.AppendEvent(r.Context(), run.ID, EventFailed, message, now); err != nil {
		writeStoreError(w, r, err)
		return
	}
	s.writeRunDocument(w, r, run.ID)
}

func (s *Server) writeRunDocument(w http.ResponseWriter, r *http.Request, runID string) {
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

func (s *Server) failConfirmedReviewRun(r *http.Request, runID string, err error, at time.Time) {
	message := err.Error()
	_ = s.Store.FailRun(r.Context(), runID, message, at)
	_ = s.Store.AppendEvent(r.Context(), runID, EventFailed, message, at)
}

func decodeReviewActionRequest(r *http.Request) (reviewActionRequest, error) {
	if r.Body == nil {
		return reviewActionRequest{}, nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		return reviewActionRequest{}, err
	}
	if strings.TrimSpace(string(body)) == "" {
		return reviewActionRequest{}, nil
	}
	var request reviewActionRequest
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return reviewActionRequest{}, err
	}
	return request, nil
}

func appendReviewAction(artifactDir string, runID string, action string, reason string, at time.Time) error {
	path := filepath.Join(artifactDir, reviewActionsPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(reviewActionArtifact{
		SchemaVersion: "0.1",
		RunID:         runID,
		Action:        action,
		Reason:        reason,
		CreatedAt:     at.Format(time.RFC3339),
	})
}

type reviewArtifactSnapshot map[string][]byte

func snapshotReviewArtifacts(artifactDir string) (reviewArtifactSnapshot, error) {
	paths := []string{
		reviewArtifactPath,
		reviewActionsPath,
		"optional/llm-output.json",
		"optional/normalization-events.json",
		"optional/local-model-chunks.json",
		"configs/redacted-provider.json",
	}
	snapshot := reviewArtifactSnapshot{}
	for _, path := range paths {
		b, err := os.ReadFile(filepath.Join(artifactDir, path))
		if errors.Is(err, os.ErrNotExist) {
			if path == reviewArtifactPath {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		snapshot[path] = b
	}
	return snapshot, nil
}

func restoreReviewArtifacts(artifactDir string, snapshot reviewArtifactSnapshot) error {
	for path, b := range snapshot {
		target := filepath.Join(artifactDir, path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (s reviewArtifactSnapshot) paths() []string {
	paths := make([]string, 0, len(s))
	for path := range s {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func writeReviewStoreError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrConflict) {
		writeError(w, r, http.StatusConflict, "review_not_available", "normalized-plan review is not awaiting confirmation", nil)
		return
	}
	writeStoreError(w, r, err)
}
