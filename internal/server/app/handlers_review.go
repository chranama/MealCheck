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
	"github.com/chranama/MealCheck/internal/workflow/checker"
	"github.com/chranama/MealCheck/internal/workflow/normalize"
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
	RowIndex      *int   `json:"row_index,omitempty"`
	SourceItemID  *int   `json:"source_item_id,omitempty"`
	SourceText    string `json:"source_text,omitempty"`
	Before        any    `json:"before,omitempty"`
	After         any    `json:"after,omitempty"`
}

type reviewCorrectionRequest struct {
	RowIndex         *int     `json:"row_index"`
	SourceItemID     int      `json:"source_item_id"`
	Food             string   `json:"food"`
	Quantity         *float64 `json:"quantity,omitempty"`
	Unit             string   `json:"unit,omitempty"`
	QuantityText     string   `json:"quantity_text,omitempty"`
	ResolutionStatus string   `json:"resolution_status,omitempty"`
	UnresolvedReason string   `json:"unresolved_reason,omitempty"`
	Reason           string   `json:"reason,omitempty"`
}

type reviewCorrectedItem struct {
	Food             string   `json:"food"`
	Quantity         *float64 `json:"quantity,omitempty"`
	Unit             string   `json:"unit,omitempty"`
	QuantityText     string   `json:"quantity_text,omitempty"`
	ResolutionStatus string   `json:"resolution_status,omitempty"`
	UnresolvedReason string   `json:"unresolved_reason,omitempty"`
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

func (s *Server) correctReview(w http.ResponseWriter, r *http.Request, runID string) {
	run, err := s.Store.GetRun(r.Context(), runID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	if run.Status != StatusAwaitingReview {
		writeError(w, r, http.StatusConflict, "review_not_available", "normalized-plan review is not awaiting correction", nil)
		return
	}
	request, err := decodeReviewCorrectionRequest(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	correctedItem, err := correctedReviewItem(request)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_correction", err.Error(), nil)
		return
	}

	artifact, err := readReviewArtifactFile(run.ArtifactDir)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "review_artifacts_unavailable", err.Error(), nil)
		return
	}
	rowIndex, row, err := reviewCorrectionRow(artifact, request)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_correction_target", err.Error(), nil)
		return
	}
	plan := artifact.NormalizedPlan
	beforeItem, err := applyReviewCorrectionToPlan(&plan, row, correctedItem)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_correction_target", err.Error(), nil)
		return
	}
	beforeRow := reviewCorrectedItemFromRow(row)
	if beforeItem != nil {
		beforeRow = reviewCorrectedItemFromFoodItem(*beforeItem)
	}
	afterRow := reviewCorrectedItemFromFoodItem(correctedItem)
	artifact.NormalizedPlan = plan
	artifact.Rows[rowIndex] = correctedReviewRow(row, correctedItem)
	artifact.Status = "awaiting_confirmation"
	artifact.TrustSignals = recalculateReviewTrustSignals(artifact)
	artifact.RequiresConfirmation = reviewRequiresConfirmation(artifact)

	if err := writeRuntimeCandidatePlan(run.CasePath, plan); err != nil {
		writeError(w, r, http.StatusInternalServerError, "correction_write_failed", err.Error(), nil)
		return
	}
	if err := writeReviewArtifactFile(run.ArtifactDir, artifact); err != nil {
		writeError(w, r, http.StatusInternalServerError, "correction_write_failed", err.Error(), nil)
		return
	}
	now := time.Now().UTC()
	reason := firstNonEmpty(request.Reason, "Normalized row corrected before checking.")
	if err := appendReviewCorrectionAction(run.ArtifactDir, run.ID, rowIndex, row.SourceItemID, row.SourceText, beforeRow, afterRow, reason, now); err != nil {
		writeError(w, r, http.StatusInternalServerError, "review_action_failed", err.Error(), nil)
		return
	}
	if err := s.Store.AppendEvent(r.Context(), run.ID, EventReviewCorrected, "normalized plan row corrected before checking", now); err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, artifact)
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

func decodeReviewCorrectionRequest(r *http.Request) (reviewCorrectionRequest, error) {
	if r.Body == nil {
		return reviewCorrectionRequest{}, fmt.Errorf("request body is required")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8192))
	if err != nil {
		return reviewCorrectionRequest{}, err
	}
	if strings.TrimSpace(string(body)) == "" {
		return reviewCorrectionRequest{}, fmt.Errorf("request body is required")
	}
	var request reviewCorrectionRequest
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return reviewCorrectionRequest{}, fmt.Errorf("invalid JSON request")
	}
	return request, nil
}

func correctedReviewItem(request reviewCorrectionRequest) (checker.FoodItem, error) {
	food := strings.TrimSpace(request.Food)
	if food == "" {
		return checker.FoodItem{}, fmt.Errorf("food is required")
	}
	item := checker.FoodItem{Food: food}
	if request.Quantity != nil {
		if strings.TrimSpace(request.QuantityText) != "" || strings.TrimSpace(request.UnresolvedReason) != "" || strings.TrimSpace(request.ResolutionStatus) != "" {
			return checker.FoodItem{}, fmt.Errorf("resolved corrections cannot include unresolved fields")
		}
		if *request.Quantity <= 0 {
			return checker.FoodItem{}, fmt.Errorf("quantity must be positive")
		}
		unit := strings.TrimSpace(request.Unit)
		if !reviewCorrectionAllowedUnit(unit) {
			return checker.FoodItem{}, fmt.Errorf("unit %q is not supported", unit)
		}
		quantity := *request.Quantity
		item.Quantity = &quantity
		item.Unit = unit
		return item, nil
	}
	quantityText := strings.TrimSpace(request.QuantityText)
	unresolvedReason := strings.TrimSpace(request.UnresolvedReason)
	resolutionStatus := firstNonEmpty(strings.TrimSpace(request.ResolutionStatus), "unresolved")
	if quantityText == "" || resolutionStatus != "unresolved" || unresolvedReason == "" {
		return checker.FoodItem{}, fmt.Errorf("unresolved corrections require quantity_text, resolution_status unresolved, and unresolved_reason")
	}
	item.QuantityText = quantityText
	item.ResolutionStatus = "unresolved"
	item.UnresolvedReason = unresolvedReason
	item.Unit = strings.TrimSpace(request.Unit)
	if item.Unit != "" && !reviewCorrectionAllowedUnit(item.Unit) {
		return checker.FoodItem{}, fmt.Errorf("unit %q is not supported", item.Unit)
	}
	return item, nil
}

func reviewCorrectionAllowedUnit(unit string) bool {
	switch unit {
	case "g", "oz", "cup", "tbsp", "tsp", "slice", "serving":
		return true
	default:
		return false
	}
}

func readReviewArtifactFile(artifactDir string) (NormalizedPlanReviewArtifact, error) {
	var artifact NormalizedPlanReviewArtifact
	b, err := os.ReadFile(filepath.Join(artifactDir, reviewArtifactPath))
	if err != nil {
		return artifact, err
	}
	if err := json.Unmarshal(b, &artifact); err != nil {
		return artifact, err
	}
	return artifact, nil
}

func writeReviewArtifactFile(artifactDir string, artifact NormalizedPlanReviewArtifact) error {
	b, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(filepath.Join(artifactDir, reviewArtifactPath), b, 0o644)
}

func reviewCorrectionRow(artifact NormalizedPlanReviewArtifact, request reviewCorrectionRequest) (int, normalize.NormalizedPlanReviewRow, error) {
	if request.RowIndex == nil {
		return 0, normalize.NormalizedPlanReviewRow{}, fmt.Errorf("row_index is required")
	}
	rowIndex := *request.RowIndex
	if rowIndex < 0 || rowIndex >= len(artifact.Rows) {
		return 0, normalize.NormalizedPlanReviewRow{}, fmt.Errorf("row_index is outside the review rows")
	}
	row := artifact.Rows[rowIndex]
	if request.SourceItemID <= 0 {
		return 0, normalize.NormalizedPlanReviewRow{}, fmt.Errorf("source_item_id is required")
	}
	if row.SourceItemID != request.SourceItemID {
		return 0, normalize.NormalizedPlanReviewRow{}, fmt.Errorf("source_item_id does not match row_index")
	}
	return rowIndex, row, nil
}

func applyReviewCorrectionToPlan(plan *checker.Plan, row normalize.NormalizedPlanReviewRow, corrected checker.FoodItem) (*checker.FoodItem, error) {
	meal := reviewPlanMeal(plan, row)
	if meal == nil {
		return nil, fmt.Errorf("matching meal was not found in normalized plan")
	}
	if strings.TrimSpace(row.NormalizedFood) == "" {
		meal.Items = append(meal.Items, corrected)
		return nil, nil
	}
	matches := matchingPlanItems(meal, row)
	if len(matches) != 1 {
		return nil, fmt.Errorf("correction target matched %d normalized plan items", len(matches))
	}
	index := matches[0]
	before := meal.Items[index]
	meal.Items[index] = corrected
	return &before, nil
}

func reviewPlanMeal(plan *checker.Plan, row normalize.NormalizedPlanReviewRow) *checker.Meal {
	for dayIndex := range plan.Days {
		if plan.Days[dayIndex].Day != row.Day {
			continue
		}
		for mealIndex := range plan.Days[dayIndex].Meals {
			meal := &plan.Days[dayIndex].Meals[mealIndex]
			if reviewMealMatches(meal.Name, row) {
				return meal
			}
		}
	}
	return nil
}

func reviewMealMatches(name string, row normalize.NormalizedPlanReviewRow) bool {
	name = normalizeReviewText(name)
	if name == "" {
		return false
	}
	candidates := []string{
		row.MealLabel,
		row.MealCode,
		reviewMealCodeLabel(row.MealCode),
	}
	for _, candidate := range candidates {
		if name == normalizeReviewText(candidate) {
			return true
		}
	}
	return false
}

func reviewMealCodeLabel(code string) string {
	switch normalizeReviewText(code) {
	case "b":
		return "breakfast"
	case "l":
		return "lunch"
	case "d":
		return "dinner"
	case "s":
		return "snack"
	default:
		return code
	}
}

func matchingPlanItems(meal *checker.Meal, row normalize.NormalizedPlanReviewRow) []int {
	var matches []int
	for itemIndex, item := range meal.Items {
		if normalizeReviewText(item.Food) != normalizeReviewText(row.NormalizedFood) {
			continue
		}
		if row.Quantity != nil {
			if item.Quantity == nil || *item.Quantity != *row.Quantity {
				continue
			}
			if normalizeReviewText(item.Unit) != normalizeReviewText(row.Unit) {
				continue
			}
		} else if row.QuantityText != "" && normalizeReviewText(item.QuantityText) != normalizeReviewText(row.QuantityText) {
			continue
		}
		matches = append(matches, itemIndex)
	}
	return matches
}

func correctedReviewRow(row normalize.NormalizedPlanReviewRow, item checker.FoodItem) normalize.NormalizedPlanReviewRow {
	row.NormalizedFood = item.Food
	row.Quantity = item.Quantity
	row.Unit = item.Unit
	row.QuantityText = item.QuantityText
	row.UnresolvedReason = item.UnresolvedReason
	row.Resolved = item.Quantity != nil && item.UnresolvedReason == ""
	return row
}

func recalculateReviewTrustSignals(artifact NormalizedPlanReviewArtifact) normalize.NormalizedPlanReviewTrustSignals {
	signals := artifact.TrustSignals
	seenSources := map[int]bool{}
	signals.NormalizedRowCount = 0
	signals.UnresolvedItemCount = 0
	for _, row := range artifact.Rows {
		if row.SourceItemID > 0 {
			seenSources[row.SourceItemID] = true
		}
		if strings.TrimSpace(row.NormalizedFood) != "" {
			signals.NormalizedRowCount++
		}
		if !row.Resolved || strings.TrimSpace(row.UnresolvedReason) != "" {
			signals.UnresolvedItemCount++
		}
	}
	if len(seenSources) > 0 {
		signals.SourceItemCount = len(seenSources)
	}
	return signals
}

func reviewRequiresConfirmation(artifact NormalizedPlanReviewArtifact) bool {
	return artifact.TrustSignals.UnresolvedItemCount > 0 || artifact.TrustSignals.RepairCount > 0 || artifact.TrustSignals.FailedChunkCount > 0
}

func writeRuntimeCandidatePlan(casePath string, plan checker.Plan) error {
	var c checker.Case
	b, err := os.ReadFile(casePath)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return err
	}
	if strings.TrimSpace(c.CandidatePlan) == "" {
		return fmt.Errorf("runtime case has no candidate_plan")
	}
	planPath := c.CandidatePlan
	if !filepath.IsAbs(planPath) {
		planPath = filepath.Join(filepath.Dir(casePath), planPath)
	}
	b, err = json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(planPath, b, 0o644)
}

func reviewCorrectedItemFromRow(row normalize.NormalizedPlanReviewRow) reviewCorrectedItem {
	return reviewCorrectedItem{
		Food:             row.NormalizedFood,
		Quantity:         row.Quantity,
		Unit:             row.Unit,
		QuantityText:     row.QuantityText,
		ResolutionStatus: reviewRowResolutionStatus(row),
		UnresolvedReason: row.UnresolvedReason,
	}
}

func reviewCorrectedItemFromFoodItem(item checker.FoodItem) reviewCorrectedItem {
	return reviewCorrectedItem{
		Food:             item.Food,
		Quantity:         item.Quantity,
		Unit:             item.Unit,
		QuantityText:     item.QuantityText,
		ResolutionStatus: item.ResolutionStatus,
		UnresolvedReason: item.UnresolvedReason,
	}
}

func reviewRowResolutionStatus(row normalize.NormalizedPlanReviewRow) string {
	if row.Resolved && strings.TrimSpace(row.UnresolvedReason) == "" {
		return ""
	}
	return "unresolved"
}

func normalizeReviewText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
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

func appendReviewCorrectionAction(artifactDir string, runID string, rowIndex int, sourceItemID int, sourceText string, before reviewCorrectedItem, after reviewCorrectedItem, reason string, at time.Time) error {
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
		Action:        "corrected",
		Reason:        reason,
		CreatedAt:     at.Format(time.RFC3339),
		RowIndex:      &rowIndex,
		SourceItemID:  &sourceItemID,
		SourceText:    sourceText,
		Before:        before,
		After:         after,
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
