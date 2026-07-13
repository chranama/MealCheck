package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/runs/progress"
	"github.com/chranama/MealCheck/internal/runs/review"
	"github.com/chranama/MealCheck/internal/state"
)

type reviewActionRequest struct {
	Reason string `json:"reason,omitempty"`
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
	case "correction":
		s.correctReview(w, r, runID)
	case "reject":
		s.finishReview(w, r, runID, "rejected", core.EventReviewRejected, "Normalized plan rejected before checking.")
	case "rewrite":
		s.finishReview(w, r, runID, "rewrite_requested", core.EventReviewRewrite, "Source text rewrite requested before checking.")
	default:
		writeError(w, r, http.StatusNotFound, "not_found", "review route not found", nil)
	}
}

func (s *Server) runReviewArtifact(w http.ResponseWriter, r *http.Request, runID string) {
	path, err := s.reviewService().Artifact(r.Context(), runID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	serveArtifactFile(w, r, s.Config.ArtifactDir, path)
}

func (s *Server) confirmReview(w http.ResponseWriter, r *http.Request, runID string) {
	request, err := decodeReviewActionRequest(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON request", nil)
		return
	}
	if err := s.reviewService().Confirm(r.Context(), runID, request.Reason, time.Now().UTC()); err != nil {
		s.writeReviewError(w, r, err)
		return
	}
	s.writeRunDocument(w, r, runID)
}

func (s *Server) correctReview(w http.ResponseWriter, r *http.Request, runID string) {
	request, err := decodeReviewCorrectionRequest(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	artifact, err := s.reviewService().Correct(r.Context(), runID, request, time.Now().UTC())
	if err != nil {
		s.writeReviewError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, artifact)
}

func (s *Server) finishReview(w http.ResponseWriter, r *http.Request, runID, action, eventType, message string) {
	request, err := decodeReviewActionRequest(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON request", nil)
		return
	}
	if err := s.reviewService().Finish(r.Context(), runID, action, eventType, message, request.Reason, time.Now().UTC()); err != nil {
		s.writeReviewError(w, r, err)
		return
	}
	s.writeRunDocument(w, r, runID)
}

func (s *Server) reviewService() review.Service {
	return review.Service{Config: s.Config, Store: s.Store}
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
		events[i] = progress.Event(events[i])
	}
	writeJSON(w, http.StatusOK, progress.Document(run, events, linksForRun(run.ID)))
}

func (s *Server) writeReviewError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, review.ErrNotAvailable) || errors.Is(err, state.ErrConflict) {
		writeError(w, r, http.StatusConflict, "review_not_available", "normalized-plan review is not awaiting action", nil)
		return
	}
	var operationErr review.OperationError
	if errors.As(err, &operationErr) {
		status := http.StatusInternalServerError
		if strings.HasPrefix(operationErr.Code, "invalid_") {
			status = http.StatusBadRequest
		}
		writeError(w, r, status, operationErr.Code, operationErr.Error(), nil)
		return
	}
	writeStoreError(w, r, err)
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

func decodeReviewCorrectionRequest(r *http.Request) (review.Correction, error) {
	if r.Body == nil {
		return review.Correction{}, fmt.Errorf("request body is required")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8192))
	if err != nil {
		return review.Correction{}, err
	}
	if strings.TrimSpace(string(body)) == "" {
		return review.Correction{}, fmt.Errorf("request body is required")
	}
	var request review.Correction
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return review.Correction{}, fmt.Errorf("invalid JSON request")
	}
	return request, nil
}
