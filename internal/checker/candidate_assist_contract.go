package checker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	CandidateAssistTask       = "p1_candidate_assist"
	CandidateAssistSchemaName = "mealcheck_p1_candidate_assist"

	CandidateAssistActionSelect    = "select_candidate"
	CandidateAssistActionAmbiguous = "ambiguous"
	CandidateAssistActionNoMatch   = "no_safe_match"
)

type CandidateAssistCandidate struct {
	CandidateID                string             `json:"candidate_id"`
	Name                       string             `json:"name"`
	Category                   string             `json:"category,omitempty"`
	ResolutionMethodIfSelected string             `json:"resolution_method_if_selected,omitempty"`
	Aliases                    []string           `json:"aliases,omitempty"`
	SupportedUnits             []string           `json:"supported_units,omitempty"`
	NutrientSummary            map[string]float64 `json:"nutrient_summary,omitempty"`
}

type CandidateAssistRequest struct {
	Task          string                     `json:"task"`
	SourceFood    string                     `json:"source_food"`
	SourceUnit    string                     `json:"source_unit,omitempty"`
	SourceContext string                     `json:"source_context,omitempty"`
	Candidates    []CandidateAssistCandidate `json:"candidates"`
}

type CandidateAssistResponse struct {
	Action      string `json:"action"`
	CandidateID string `json:"candidate_id"`
	Confidence  string `json:"confidence"`
	Reason      string `json:"reason"`
}

type CandidateAssistDecision struct {
	Action     string
	Candidate  CandidateAssistCandidate
	Confidence string
	Reason     string
}

func BuildCandidateAssistRequest(sourceFood string, sourceUnit string, sourceContext string, candidates []CandidateAssistCandidate) CandidateAssistRequest {
	return CandidateAssistRequest{
		Task:          CandidateAssistTask,
		SourceFood:    strings.TrimSpace(sourceFood),
		SourceUnit:    strings.TrimSpace(sourceUnit),
		SourceContext: strings.TrimSpace(sourceContext),
		Candidates:    append([]CandidateAssistCandidate(nil), candidates...),
	}
}

func CandidateAssistResponseSchema(candidateIDs []string) map[string]any {
	enumIDs := make([]any, 0, len(candidateIDs)+1)
	enumIDs = append(enumIDs, "")
	for _, id := range candidateIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			enumIDs = append(enumIDs, id)
		}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []any{CandidateAssistActionSelect, CandidateAssistActionAmbiguous, CandidateAssistActionNoMatch},
			},
			"candidate_id": map[string]any{"type": "string", "enum": enumIDs},
			"confidence": map[string]any{
				"type": "string",
				"enum": []any{"high", "medium", "low"},
			},
			"reason": map[string]any{"type": "string"},
		},
		"required": []any{"action", "candidate_id", "confidence", "reason"},
	}
}

func DecodeAndValidateCandidateAssistResponse(raw string, request CandidateAssistRequest) (CandidateAssistDecision, error) {
	var response CandidateAssistResponse
	decoder := json.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return CandidateAssistDecision{}, fmt.Errorf("decode candidate assist response: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return CandidateAssistDecision{}, fmt.Errorf("decode candidate assist response: trailing JSON value")
	}
	return ValidateCandidateAssistResponse(response, request.Candidates)
}

func ValidateCandidateAssistResponse(response CandidateAssistResponse, candidates []CandidateAssistCandidate) (CandidateAssistDecision, error) {
	action := strings.TrimSpace(response.Action)
	confidence := strings.TrimSpace(response.Confidence)
	candidateID := strings.TrimSpace(response.CandidateID)
	if !allowedCandidateAssistConfidence(confidence) {
		return CandidateAssistDecision{}, fmt.Errorf("invalid candidate assist confidence %q", response.Confidence)
	}
	candidateByID := map[string]CandidateAssistCandidate{}
	for _, candidate := range candidates {
		id := strings.TrimSpace(candidate.CandidateID)
		if id != "" {
			candidate.CandidateID = id
			candidateByID[id] = candidate
		}
	}
	switch action {
	case CandidateAssistActionSelect:
		if candidateID == "" {
			return CandidateAssistDecision{}, fmt.Errorf("candidate assist select_candidate requires candidate_id")
		}
		candidate, ok := candidateByID[candidateID]
		if !ok {
			return CandidateAssistDecision{}, fmt.Errorf("candidate assist selected unknown candidate_id %q", candidateID)
		}
		return CandidateAssistDecision{
			Action:     action,
			Candidate:  candidate,
			Confidence: confidence,
			Reason:     strings.TrimSpace(response.Reason),
		}, nil
	case CandidateAssistActionAmbiguous, CandidateAssistActionNoMatch:
		if candidateID != "" {
			return CandidateAssistDecision{}, fmt.Errorf("candidate assist %s must not include candidate_id", action)
		}
		return CandidateAssistDecision{
			Action:     action,
			Confidence: confidence,
			Reason:     strings.TrimSpace(response.Reason),
		}, nil
	default:
		return CandidateAssistDecision{}, fmt.Errorf("unsupported candidate assist action %q", response.Action)
	}
}

func allowedCandidateAssistConfidence(confidence string) bool {
	switch confidence {
	case "high", "medium", "low":
		return true
	default:
		return false
	}
}
