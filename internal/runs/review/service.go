// Package review owns normalized-plan review actions and their artifact and
// run-state transitions.
package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/state"
	"github.com/chranama/MealCheck/internal/workflow/artifacts"
	"github.com/chranama/MealCheck/internal/workflow/checker"
	"github.com/chranama/MealCheck/internal/workflow/normalize"
)

const (
	ArtifactPath = "review/normalized-plan-review.json"
	actionsPath  = "review/review-actions.jsonl"
)

var ErrNotAvailable = errors.New("normalized-plan review is not awaiting action")

type OperationError struct {
	Code string
	Err  error
}

func (e OperationError) Error() string { return e.Err.Error() }
func (e OperationError) Unwrap() error { return e.Err }

type Service struct {
	Config core.Config
	Store  state.Store
}

type Correction struct {
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

func (s Service) Artifact(ctx context.Context, runID string) (string, error) {
	run, err := s.Store.GetRun(ctx, runID)
	if err != nil {
		return "", err
	}
	return filepath.Join(run.ArtifactDir, ArtifactPath), nil
}

func (s Service) Confirm(ctx context.Context, runID, reason string, now time.Time) error {
	run, err := s.awaitingRun(ctx, runID)
	if err != nil {
		return err
	}
	reason = firstNonEmpty(reason, "User confirmed normalized plan for checking.")
	if err := appendAction(run.ArtifactDir, run.ID, "confirmed", reason, now); err != nil {
		return operationError("review_action_failed", err)
	}
	snapshot, err := snapshotArtifacts(run.ArtifactDir)
	if err != nil {
		return operationError("review_artifacts_unavailable", err)
	}
	run, err = s.Store.StartReviewRun(ctx, run.ID, "review-confirm", now.Add(s.Config.RunTimeout), now)
	if err != nil {
		return err
	}
	if err := s.Store.AppendEvent(ctx, run.ID, core.EventReviewConfirmed, "normalized plan confirmed for checking", now); err != nil {
		return err
	}

	result, err := artifacts.WriteBundle(artifacts.BundleOptions{
		Root: s.Config.Root, CasePath: run.CasePath, OutDir: run.ArtifactDir, Mode: "hosted",
		FNDDSFallbackPath: s.Config.FNDDSFallbackPath,
	})
	restoreErr := restoreArtifacts(run.ArtifactDir, snapshot)
	if err != nil {
		s.fail(ctx, run.ID, fmt.Errorf("checking failed after review confirmation: %w", err), now)
		return operationError("bundle_failed", err)
	}
	if restoreErr != nil {
		s.fail(ctx, run.ID, restoreErr, now)
		return operationError("review_artifact_restore_failed", restoreErr)
	}
	if err := normalize.UpdateManifestArtifacts(result.OutDir, snapshot.paths()...); err != nil {
		s.fail(ctx, run.ID, err, now)
		return operationError("manifest_update_failed", err)
	}
	if err := s.Store.AppendEvent(ctx, run.ID, core.EventArtifactWritten, "artifact bundle written", now); err != nil {
		return err
	}
	if err := s.Store.CompleteRun(ctx, run.ID, result.Decision, now); err != nil {
		return err
	}
	return s.Store.AppendEvent(ctx, run.ID, core.EventCompleted, result.Decision.Decision, now)
}

func (s Service) Correct(ctx context.Context, runID string, request Correction, now time.Time) (normalize.NormalizedPlanReviewArtifact, error) {
	run, err := s.awaitingRun(ctx, runID)
	if err != nil {
		return normalize.NormalizedPlanReviewArtifact{}, err
	}
	corrected, err := correctedItem(request)
	if err != nil {
		return normalize.NormalizedPlanReviewArtifact{}, operationError("invalid_correction", err)
	}
	artifact, err := readArtifact(run.ArtifactDir)
	if err != nil {
		return artifact, operationError("review_artifacts_unavailable", err)
	}
	rowIndex, row, err := correctionRow(artifact, request)
	if err != nil {
		return artifact, operationError("invalid_correction_target", err)
	}
	plan := artifact.NormalizedPlan
	beforeItem, err := applyCorrection(&plan, row, corrected)
	if err != nil {
		return artifact, operationError("invalid_correction_target", err)
	}
	before := correctedItemFromRow(row)
	if beforeItem != nil {
		before = correctedItemFromFood(*beforeItem)
	}
	after := correctedItemFromFood(corrected)
	artifact.NormalizedPlan = plan
	artifact.Rows[rowIndex] = correctedRow(row, corrected)
	artifact.Status = "awaiting_confirmation"
	artifact.TrustSignals = recalculateTrustSignals(artifact)
	artifact.RequiresConfirmation = artifact.TrustSignals.UnresolvedItemCount > 0 || artifact.TrustSignals.RepairCount > 0 || artifact.TrustSignals.FailedChunkCount > 0

	if err := writeRuntimeCandidatePlan(run.CasePath, plan); err != nil {
		return artifact, operationError("correction_write_failed", err)
	}
	if err := writeArtifact(run.ArtifactDir, artifact); err != nil {
		return artifact, operationError("correction_write_failed", err)
	}
	reason := firstNonEmpty(request.Reason, "Normalized row corrected before checking.")
	if err := appendCorrectionAction(run.ArtifactDir, run.ID, rowIndex, row.SourceItemID, row.SourceText, before, after, reason, now); err != nil {
		return artifact, operationError("review_action_failed", err)
	}
	if err := s.Store.AppendEvent(ctx, run.ID, core.EventReviewCorrected, "normalized plan row corrected before checking", now); err != nil {
		return artifact, err
	}
	return artifact, nil
}

func (s Service) Finish(ctx context.Context, runID, action, eventType, message, reason string, now time.Time) error {
	run, err := s.awaitingRun(ctx, runID)
	if err != nil {
		return err
	}
	if err := appendAction(run.ArtifactDir, run.ID, action, firstNonEmpty(reason, message), now); err != nil {
		return operationError("review_action_failed", err)
	}
	if err := s.Store.AppendEvent(ctx, run.ID, eventType, message, now); err != nil {
		return err
	}
	if err := s.Store.FailRun(ctx, run.ID, message, now); err != nil {
		return err
	}
	return s.Store.AppendEvent(ctx, run.ID, core.EventFailed, message, now)
}

func (s Service) awaitingRun(ctx context.Context, runID string) (core.Run, error) {
	run, err := s.Store.GetRun(ctx, runID)
	if err != nil {
		return core.Run{}, err
	}
	if run.Status != core.StatusAwaitingReview {
		return core.Run{}, ErrNotAvailable
	}
	return run, nil
}

func (s Service) fail(ctx context.Context, runID string, err error, at time.Time) {
	_ = s.Store.FailRun(ctx, runID, err.Error(), at)
	_ = s.Store.AppendEvent(ctx, runID, core.EventFailed, err.Error(), at)
}

func operationError(code string, err error) error { return OperationError{Code: code, Err: err} }

type actionArtifact struct {
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

type correctedItemValue struct {
	Food             string   `json:"food"`
	Quantity         *float64 `json:"quantity,omitempty"`
	Unit             string   `json:"unit,omitempty"`
	QuantityText     string   `json:"quantity_text,omitempty"`
	ResolutionStatus string   `json:"resolution_status,omitempty"`
	UnresolvedReason string   `json:"unresolved_reason,omitempty"`
}

func correctedItem(request Correction) (checker.FoodItem, error) {
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
		if !allowedUnit(unit) {
			return checker.FoodItem{}, fmt.Errorf("unit %q is not supported", unit)
		}
		quantity := *request.Quantity
		item.Quantity, item.Unit = &quantity, unit
		return item, nil
	}
	quantityText := strings.TrimSpace(request.QuantityText)
	unresolvedReason := strings.TrimSpace(request.UnresolvedReason)
	resolutionStatus := firstNonEmpty(strings.TrimSpace(request.ResolutionStatus), "unresolved")
	if quantityText == "" || resolutionStatus != "unresolved" || unresolvedReason == "" {
		return checker.FoodItem{}, fmt.Errorf("unresolved corrections require quantity_text, resolution_status unresolved, and unresolved_reason")
	}
	item.QuantityText, item.ResolutionStatus, item.UnresolvedReason = quantityText, "unresolved", unresolvedReason
	item.Unit = strings.TrimSpace(request.Unit)
	if item.Unit != "" && !allowedUnit(item.Unit) {
		return checker.FoodItem{}, fmt.Errorf("unit %q is not supported", item.Unit)
	}
	return item, nil
}

func allowedUnit(unit string) bool {
	switch unit {
	case "g", "oz", "cup", "tbsp", "tsp", "slice", "serving":
		return true
	default:
		return false
	}
}

func correctionRow(artifact normalize.NormalizedPlanReviewArtifact, request Correction) (int, normalize.NormalizedPlanReviewRow, error) {
	if request.RowIndex == nil {
		return 0, normalize.NormalizedPlanReviewRow{}, fmt.Errorf("row_index is required")
	}
	index := *request.RowIndex
	if index < 0 || index >= len(artifact.Rows) {
		return 0, normalize.NormalizedPlanReviewRow{}, fmt.Errorf("row_index is outside the review rows")
	}
	row := artifact.Rows[index]
	if request.SourceItemID <= 0 {
		return 0, normalize.NormalizedPlanReviewRow{}, fmt.Errorf("source_item_id is required")
	}
	if row.SourceItemID != request.SourceItemID {
		return 0, normalize.NormalizedPlanReviewRow{}, fmt.Errorf("source_item_id does not match row_index")
	}
	return index, row, nil
}

func applyCorrection(plan *checker.Plan, row normalize.NormalizedPlanReviewRow, corrected checker.FoodItem) (*checker.FoodItem, error) {
	meal := planMeal(plan, row)
	if meal == nil {
		return nil, fmt.Errorf("matching meal was not found in normalized plan")
	}
	if strings.TrimSpace(row.NormalizedFood) == "" {
		meal.Items = append(meal.Items, corrected)
		return nil, nil
	}
	matches := matchingItems(meal, row)
	if len(matches) != 1 {
		return nil, fmt.Errorf("correction target matched %d normalized plan items", len(matches))
	}
	before := meal.Items[matches[0]]
	meal.Items[matches[0]] = corrected
	return &before, nil
}

func planMeal(plan *checker.Plan, row normalize.NormalizedPlanReviewRow) *checker.Meal {
	for dayIndex := range plan.Days {
		if plan.Days[dayIndex].Day != row.Day {
			continue
		}
		for mealIndex := range plan.Days[dayIndex].Meals {
			meal := &plan.Days[dayIndex].Meals[mealIndex]
			if mealMatches(meal.Name, row) {
				return meal
			}
		}
	}
	return nil
}

func mealMatches(name string, row normalize.NormalizedPlanReviewRow) bool {
	name = normalizedText(name)
	for _, candidate := range []string{row.MealLabel, row.MealCode, mealCodeLabel(row.MealCode)} {
		if name != "" && name == normalizedText(candidate) {
			return true
		}
	}
	return false
}

func mealCodeLabel(code string) string {
	switch normalizedText(code) {
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

func matchingItems(meal *checker.Meal, row normalize.NormalizedPlanReviewRow) []int {
	var matches []int
	for index, item := range meal.Items {
		if normalizedText(item.Food) != normalizedText(row.NormalizedFood) {
			continue
		}
		if row.Quantity != nil {
			if item.Quantity == nil || *item.Quantity != *row.Quantity || normalizedText(item.Unit) != normalizedText(row.Unit) {
				continue
			}
		} else if row.QuantityText != "" && normalizedText(item.QuantityText) != normalizedText(row.QuantityText) {
			continue
		}
		matches = append(matches, index)
	}
	return matches
}

func correctedRow(row normalize.NormalizedPlanReviewRow, item checker.FoodItem) normalize.NormalizedPlanReviewRow {
	row.NormalizedFood, row.Quantity, row.Unit = item.Food, item.Quantity, item.Unit
	row.QuantityText, row.UnresolvedReason = item.QuantityText, item.UnresolvedReason
	row.Resolved = item.Quantity != nil && item.UnresolvedReason == ""
	return row
}

func recalculateTrustSignals(artifact normalize.NormalizedPlanReviewArtifact) normalize.NormalizedPlanReviewTrustSignals {
	signals := artifact.TrustSignals
	seenSources := map[int]bool{}
	signals.NormalizedRowCount, signals.UnresolvedItemCount = 0, 0
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

func readArtifact(dir string) (normalize.NormalizedPlanReviewArtifact, error) {
	var artifact normalize.NormalizedPlanReviewArtifact
	b, err := os.ReadFile(filepath.Join(dir, ArtifactPath))
	if err != nil {
		return artifact, err
	}
	return artifact, json.Unmarshal(b, &artifact)
}

func writeArtifact(dir string, artifact normalize.NormalizedPlanReviewArtifact) error {
	b, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ArtifactPath), append(b, '\n'), 0o644)
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
	return os.WriteFile(planPath, append(b, '\n'), 0o644)
}

func appendAction(dir, runID, action, reason string, at time.Time) error {
	return appendActionArtifact(dir, actionArtifact{SchemaVersion: "0.1", RunID: runID, Action: action, Reason: reason, CreatedAt: at.Format(time.RFC3339)})
}

func appendCorrectionAction(dir, runID string, rowIndex, sourceItemID int, sourceText string, before, after correctedItemValue, reason string, at time.Time) error {
	return appendActionArtifact(dir, actionArtifact{
		SchemaVersion: "0.1", RunID: runID, Action: "corrected", Reason: reason, CreatedAt: at.Format(time.RFC3339),
		RowIndex: &rowIndex, SourceItemID: &sourceItemID, SourceText: sourceText, Before: before, After: after,
	})
}

func appendActionArtifact(dir string, artifact actionArtifact) error {
	path := filepath.Join(dir, actionsPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(artifact)
}

func correctedItemFromRow(row normalize.NormalizedPlanReviewRow) correctedItemValue {
	status := "unresolved"
	if row.Resolved && strings.TrimSpace(row.UnresolvedReason) == "" {
		status = ""
	}
	return correctedItemValue{Food: row.NormalizedFood, Quantity: row.Quantity, Unit: row.Unit, QuantityText: row.QuantityText, ResolutionStatus: status, UnresolvedReason: row.UnresolvedReason}
}

func correctedItemFromFood(item checker.FoodItem) correctedItemValue {
	return correctedItemValue{Food: item.Food, Quantity: item.Quantity, Unit: item.Unit, QuantityText: item.QuantityText, ResolutionStatus: item.ResolutionStatus, UnresolvedReason: item.UnresolvedReason}
}

type artifactSnapshot map[string][]byte

func snapshotArtifacts(dir string) (artifactSnapshot, error) {
	paths := []string{ArtifactPath, actionsPath, "optional/llm-output.json", "optional/normalization-events.json", "optional/local-model-chunks.json", "configs/redacted-provider.json"}
	snapshot := artifactSnapshot{}
	for _, path := range paths {
		b, err := os.ReadFile(filepath.Join(dir, path))
		if errors.Is(err, os.ErrNotExist) {
			if path == ArtifactPath {
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

func restoreArtifacts(dir string, snapshot artifactSnapshot) error {
	for path, b := range snapshot {
		target := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (s artifactSnapshot) paths() []string {
	paths := make([]string, 0, len(s))
	for path := range s {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func normalizedText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
