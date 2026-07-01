package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
func requestClientIP(r *http.Request) string {
	headers := map[string]string{
		"CF-Connecting-IP": r.Header.Get("CF-Connecting-IP"),
		"X-Forwarded-For":  r.Header.Get("X-Forwarded-For"),
		"X-Real-IP":        r.Header.Get("X-Real-IP"),
	}
	return clientIP(r.RemoteAddr, headers)
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
func writeMealPlanNotVerifiableError(w http.ResponseWriter, r *http.Request, err error) bool {
	var qualificationErr qualificationRejectionError
	if errors.As(err, &qualificationErr) {
		writeError(w, r, http.StatusUnprocessableEntity, "meal_plan_not_verifiable", qualificationErr.Error(), map[string]any{
			"qualification": qualificationErr.Qualification,
		})
		return true
	}
	var contractErr localModelInputContractError
	if errors.As(err, &contractErr) {
		writeError(w, r, http.StatusUnprocessableEntity, "meal_plan_not_verifiable", contractErr.Error(), map[string]any{
			"qualification": contractErr.Qualification,
		})
		return true
	}
	return false
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
