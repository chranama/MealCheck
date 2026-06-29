package normalization

import (
	"context"
	"fmt"
	"strings"

	"github.com/chranama/MealCheck/internal/assist"
	"github.com/chranama/MealCheck/internal/checker"
)

const (
	MethodDeterministic               = "deterministic"
	MethodDeterministicWithLLMAssist  = "deterministic_with_llm_assist"
	MethodFailedPreModel              = "failed_pre_model"
	MethodFailedPostAssistValidation  = "failed_post_assist_validation"
	MethodLocalModelFallback          = "local_model_fallback"
	ReviewFlagLLMAssistUnavailable    = "llm_assist_unavailable"
	ReviewFlagLLMAssistProviderFailed = "llm_assist_provider_failed"
	ReviewFlagLLMAssistSchemaInvalid  = "llm_assist_schema_invalid"
	ReviewFlagLLMAssistIncomplete     = "llm_assist_incomplete"
	ReviewFlagLLMAssistInvalidMerge   = "llm_assist_invalid_merge"
)

type Request struct {
	Text   string
	PlanID string
	Policy Policy
}

type Policy struct {
	AssistEnabled          bool `json:"assist_enabled"`
	MaxSourceItemsPerChunk int  `json:"max_source_items_per_chunk,omitempty"`
}

type Engine struct {
	Policy Policy
	Assist assist.Client
}

type Result struct {
	SchemaVersion      string                   `json:"schema_version"`
	Method             string                   `json:"method"`
	PlanID             string                   `json:"plan_id,omitempty"`
	Plan               checker.Plan             `json:"-"`
	SourceItems        []SourceItem             `json:"source_items,omitempty"`
	ParsedItems        []ParsedSourceItem       `json:"parsed_items,omitempty"`
	UnresolvedItems    []UnresolvedItem         `json:"unresolved_items,omitempty"`
	AssistPolicy       Policy                   `json:"assist_policy"`
	AssistChunks       []AssistChunk            `json:"assist_chunks,omitempty"`
	AssistRequests     []AssistRequestPayload   `json:"assist_requests,omitempty"`
	AssistResponses    []AssistResponseArtifact `json:"assist_responses,omitempty"`
	AcceptedAssistRows []AssistAcceptedRow      `json:"accepted_assist_rows,omitempty"`
	RejectedAssistRows []AssistRejectedRow      `json:"rejected_assist_rows,omitempty"`
	AssistUsed         bool                     `json:"assist_used"`
	ProviderUsed       bool                     `json:"provider_used"`
	ReviewFlags        []string                 `json:"review_flags,omitempty"`
	DeterministicError string                   `json:"deterministic_error,omitempty"`
}

type UnresolvedItem struct {
	SourceItem SourceItem `json:"source_item"`
	Reason     string     `json:"reason"`
}

func (e Engine) Normalize(ctx context.Context, request Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	policy := request.Policy
	if policy == (Policy{}) {
		policy = e.Policy
	}
	result := Analyze(request.Text, request.PlanID, policy)
	if result.Method == MethodDeterministic {
		return result, nil
	}
	if policy.AssistEnabled {
		return e.assistNormalize(ctx, result)
	}
	return result, fmt.Errorf("%s", result.DeterministicError)
}

func (e Engine) assistNormalize(ctx context.Context, result Result) (Result, error) {
	if len(result.AssistChunks) == 0 {
		result.Method = MethodFailedPostAssistValidation
		result.ReviewFlags = appendIfMissingString(result.ReviewFlags, ReviewFlagLLMAssistIncomplete)
		return result, fmt.Errorf("normalization LLM assist has no eligible source items")
	}
	if e.Assist == nil {
		result.Method = MethodFailedPostAssistValidation
		result.ReviewFlags = appendIfMissingString(result.ReviewFlags, ReviewFlagLLMAssistUnavailable)
		return result, fmt.Errorf("normalization LLM assist client is not configured")
	}
	for _, chunk := range result.AssistChunks {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		payload := BuildP0AssistRequestPayload(result, chunk)
		result.AssistRequests = append(result.AssistRequests, payload)
		response, err := e.Assist.Complete(ctx, assist.Request{
			Task:           P0AssistTask,
			SchemaName:     P0AssistSchemaName,
			ResponseSchema: P0AssistResponseSchema(),
			Messages:       P0AssistMessages(payload),
		})
		result.ProviderUsed = true
		result.AssistUsed = true
		artifact := AssistResponseArtifact{ChunkID: chunk.ID}
		if err != nil {
			artifact.Error = err.Error()
			result.AssistResponses = append(result.AssistResponses, artifact)
			result.Method = MethodFailedPostAssistValidation
			result.ReviewFlags = appendIfMissingString(result.ReviewFlags, ReviewFlagLLMAssistProviderFailed)
			return result, fmt.Errorf("normalization LLM assist provider failed: %w", err)
		}
		artifact.RawText = response.RawText
		validation, err := DecodeAndValidateP0AssistResponse(response.RawText, payload)
		if err != nil {
			artifact.Error = err.Error()
			result.AssistResponses = append(result.AssistResponses, artifact)
			result.Method = MethodFailedPostAssistValidation
			result.ReviewFlags = appendIfMissingString(result.ReviewFlags, ReviewFlagLLMAssistSchemaInvalid)
			return result, err
		}
		artifact.Items = validation.Response.Items
		result.AssistResponses = append(result.AssistResponses, artifact)
		result.AcceptedAssistRows = append(result.AcceptedAssistRows, validation.Accepted...)
		result.RejectedAssistRows = append(result.RejectedAssistRows, validation.Rejected...)
	}
	return BuildAssistedPlan(result)
}

func Analyze(text string, planID string, policy Policy) Result {
	sourceItems := ResolvedSourceItems(text)
	result := Result{
		SchemaVersion: "0.1",
		Method:        MethodFailedPreModel,
		PlanID:        strings.TrimSpace(planID),
		SourceItems:   sourceItems,
		AssistPolicy:  policy,
	}
	if result.PlanID == "" {
		result.PlanID = "deterministic-normalized"
	}
	plan, parsedItems, err := BuildDeterministicPlan(text, result.PlanID)
	result.ParsedItems = parsedItems
	if err == nil {
		result.Method = MethodDeterministic
		result.Plan = plan
		return result
	}
	result.DeterministicError = err.Error()
	result.UnresolvedItems = unresolvedItemsForFailure(sourceItems, parsedItems, err)
	if policy.AssistEnabled {
		result.AssistChunks = ChunkSourceItems(unresolvedSourceItems(result.UnresolvedItems), policy)
		if len(result.AssistChunks) > 0 {
			result.Method = MethodDeterministicWithLLMAssist
		}
	}
	return result
}

func unresolvedItemsForFailure(sourceItems []SourceItem, parsedItems []ParsedSourceItem, err error) []UnresolvedItem {
	if len(sourceItems) == 0 {
		return nil
	}
	parsedByID := map[int]ParsedSourceItem{}
	for _, parsed := range parsedItems {
		parsedByID[parsed.SourceItem.ID] = parsed
	}
	unresolved := make([]UnresolvedItem, 0)
	for _, sourceItem := range sourceItems {
		parsed, ok := parsedByID[sourceItem.ID]
		if !ok {
			if _, mealOK := MealName(sourceItem.MealCode); !mealOK {
				unresolved = append(unresolved, UnresolvedItem{
					SourceItem: sourceItem,
					Reason:     "missing_or_unsupported_meal_code",
				})
				continue
			}
			unresolved = append(unresolved, UnresolvedItem{
				SourceItem: sourceItem,
				Reason:     "not_parsed",
			})
			continue
		}
		if parsed.Measurement.Status != "parsed" {
			unresolved = append(unresolved, UnresolvedItem{
				SourceItem: sourceItem,
				Reason:     parsed.Measurement.Reason,
			})
			continue
		}
		if _, ok := MealName(sourceItem.MealCode); !ok {
			unresolved = append(unresolved, UnresolvedItem{
				SourceItem: sourceItem,
				Reason:     "missing_or_unsupported_meal_code",
			})
		}
	}
	if len(unresolved) == 0 && err != nil {
		unresolved = append(unresolved, UnresolvedItem{Reason: err.Error()})
	}
	return unresolved
}

func unresolvedSourceItems(unresolved []UnresolvedItem) []SourceItem {
	items := make([]SourceItem, 0, len(unresolved))
	for _, item := range unresolved {
		if item.SourceItem.ID > 0 {
			items = append(items, item.SourceItem)
		}
	}
	return items
}

func appendIfMissingString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
