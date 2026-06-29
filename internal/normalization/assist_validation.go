package normalization

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func DecodeAndValidateP0AssistResponse(raw string, payload AssistRequestPayload) (AssistValidationResult, error) {
	var response AssistResponse
	decoder := json.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return AssistValidationResult{}, fmt.Errorf("decode P0 assist response: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return AssistValidationResult{}, fmt.Errorf("decode P0 assist response: trailing JSON value")
	}
	result := AssistValidationResult{Response: response}
	sourceItems := map[int]SourceItem{}
	for _, sourceItem := range payload.SourceItems {
		sourceItems[sourceItem.ID] = sourceItem
	}
	fixed := map[int]bool{}
	for _, id := range payload.FixedSourceItemIDs {
		fixed[id] = true
	}
	seen := map[int]bool{}
	for _, item := range response.Items {
		accepted, rejected := validateP0AssistItem(item, payload.ChunkID, sourceItems, fixed, seen)
		if rejected != nil {
			result.Rejected = append(result.Rejected, *rejected)
			continue
		}
		result.Accepted = append(result.Accepted, accepted)
	}
	return result, nil
}

func validateP0AssistItem(item AssistResponseItem, chunkID string, sourceItems map[int]SourceItem, fixed map[int]bool, seen map[int]bool) (AssistAcceptedRow, *AssistRejectedRow) {
	reject := func(reason string) (AssistAcceptedRow, *AssistRejectedRow) {
		return AssistAcceptedRow{}, &AssistRejectedRow{
			ChunkID:      chunkID,
			SourceItemID: item.SourceItemID,
			Action:       strings.TrimSpace(item.Action),
			Reason:       reason,
			Message:      strings.TrimSpace(item.Message),
		}
	}
	if item.SourceItemID <= 0 {
		return reject("missing_source_item_id")
	}
	sourceItem, ok := sourceItems[item.SourceItemID]
	if !ok {
		return reject("invented_source_item_id")
	}
	if fixed[item.SourceItemID] {
		return reject("fixed_source_item_id")
	}
	if seen[item.SourceItemID] {
		return reject("duplicate_source_item_id")
	}
	seen[item.SourceItemID] = true

	action := strings.TrimSpace(item.Action)
	confidence := strings.TrimSpace(item.Confidence)
	if !allowedAssistConfidence(confidence) {
		return reject("invalid_confidence")
	}
	switch action {
	case AssistActionProposeRow:
		return validateP0ProposedRow(item, chunkID, sourceItem, confidence)
	case AssistActionNeedsClarification:
		if hasP0RowFields(item) {
			return reject("action_result_mismatch")
		}
		return reject("needs_clarification")
	case AssistActionAbstain:
		if hasP0RowFields(item) {
			return reject("action_result_mismatch")
		}
		return reject("abstain")
	default:
		return reject("invalid_action")
	}
}

func validateP0ProposedRow(item AssistResponseItem, chunkID string, sourceItem SourceItem, confidence string) (AssistAcceptedRow, *AssistRejectedRow) {
	reject := func(reason string) (AssistAcceptedRow, *AssistRejectedRow) {
		return AssistAcceptedRow{}, &AssistRejectedRow{
			ChunkID:      chunkID,
			SourceItemID: item.SourceItemID,
			Action:       strings.TrimSpace(item.Action),
			Reason:       reason,
			Message:      strings.TrimSpace(item.Message),
		}
	}
	if item.Day < 1 || item.Day > 7 {
		return reject("invalid_day")
	}
	if _, ok := MealName(strings.TrimSpace(item.MealCode)); !ok {
		return reject("unsupported_meal_code")
	}
	food := strings.TrimSpace(item.Food)
	if food == "" {
		return reject("missing_food")
	}
	if item.Quantity <= 0 {
		return reject("non_positive_quantity")
	}
	unit := NormalizeSourceUnit(strings.TrimSpace(item.Unit))
	if !AllowedUnit(unit) {
		return reject("unsupported_unit")
	}
	return AssistAcceptedRow{
		ChunkID:    chunkID,
		SourceItem: sourceItem,
		Day:        item.Day,
		MealCode:   strings.TrimSpace(item.MealCode),
		Food:       food,
		Quantity:   item.Quantity,
		Unit:       unit,
		Confidence: confidence,
		Message:    strings.TrimSpace(item.Message),
	}, nil
}

func hasP0RowFields(item AssistResponseItem) bool {
	return item.Day != 0 ||
		strings.TrimSpace(item.MealCode) != "" ||
		strings.TrimSpace(item.Food) != "" ||
		item.Quantity != 0 ||
		strings.TrimSpace(item.Unit) != ""
}

func allowedAssistConfidence(confidence string) bool {
	switch confidence {
	case AssistConfidenceHigh, AssistConfidenceMedium, AssistConfidenceLow:
		return true
	default:
		return false
	}
}
