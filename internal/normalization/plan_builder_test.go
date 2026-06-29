package normalization

import (
	"context"
	"strings"
	"testing"

	"github.com/chranama/MealCheck/internal/assist"
)

func TestBuildDeterministicPlanFromNaturalMealText(t *testing.T) {
	plan, parsedItems, err := BuildDeterministicPlan(strings.Join([]string{
		"Day 1 breakfast: 1 cup cooked oatmeal, 0.5 cup blueberries, and 1 cup plain Greek yogurt.",
		"Day 1 lunch: 4 oz grilled chicken breast, 1 cup brown rice, and 1 cup steamed broccoli.",
		"Day 2 dinner: 5 oz turkey meatballs, 1 cup whole wheat pasta, and 1 cup tomato sauce.",
	}, "\n"), "test-plan")
	if err != nil {
		t.Fatalf("BuildDeterministicPlan error: %v", err)
	}
	if len(parsedItems) != 9 {
		t.Fatalf("parsed item count = %d, want 9", len(parsedItems))
	}
	if len(plan.Days) != 2 {
		t.Fatalf("day count = %d, want 2", len(plan.Days))
	}
	if plan.Days[0].Day != 1 || plan.Days[1].Day != 2 {
		t.Fatalf("days = %+v, want day 1 then day 2", plan.Days)
	}
	if got := plan.Days[0].Meals[0].Name; got != "breakfast" {
		t.Fatalf("first meal name = %q, want breakfast", got)
	}
	item := plan.Days[0].Meals[0].Items[1]
	if item.Food != "blueberries" || item.Quantity == nil || *item.Quantity != 0.5 || item.Unit != "cup" {
		t.Fatalf("blueberry item = %+v, want 0.5 cup blueberries", item)
	}
}

func TestBuildDeterministicPlanRejectsMissingMealCode(t *testing.T) {
	_, _, err := BuildDeterministicPlan("- 1 cup cooked oatmeal", "test-plan")
	if err == nil {
		t.Fatal("BuildDeterministicPlan error = nil, want missing meal code error")
	}
}

func TestEngineNormalizeReturnsDeterministicResult(t *testing.T) {
	result, err := (Engine{}).Normalize(context.Background(), Request{
		Text:   "Day 1 breakfast: 1 cup cooked oatmeal.",
		PlanID: "engine-test",
	})
	if err != nil {
		t.Fatalf("Normalize error: %v", err)
	}
	if result.Method != MethodDeterministic {
		t.Fatalf("method = %q, want %q", result.Method, MethodDeterministic)
	}
	if len(result.SourceItems) != 1 || len(result.ParsedItems) != 1 {
		t.Fatalf("source/parsed counts = %d/%d, want 1/1", len(result.SourceItems), len(result.ParsedItems))
	}
	if result.Plan.PlanID != "engine-test" || len(result.Plan.Days) != 1 {
		t.Fatalf("plan = %+v, want deterministic plan", result.Plan)
	}
}

func TestEngineNormalizeReportsPreModelFailure(t *testing.T) {
	result, err := (Engine{}).Normalize(context.Background(), Request{
		Text:   "- 1 cup cooked oatmeal",
		PlanID: "engine-test",
	})
	if err == nil {
		t.Fatal("Normalize error = nil, want deterministic failure")
	}
	if result.Method != MethodFailedPreModel {
		t.Fatalf("method = %q, want %q", result.Method, MethodFailedPreModel)
	}
	if len(result.UnresolvedItems) != 1 || result.UnresolvedItems[0].Reason != "missing_or_unsupported_meal_code" {
		t.Fatalf("unresolved = %+v, want missing meal code", result.UnresolvedItems)
	}
}

func TestEngineAnalyzeBuildsAssistChunksWhenEnabled(t *testing.T) {
	result := Analyze(strings.Join([]string{
		"- 1 cup cooked oatmeal",
		"- 1 cup blueberries",
		"- 1 cup yogurt",
	}, "\n"), "assist-test", Policy{
		AssistEnabled:          true,
		MaxSourceItemsPerChunk: 2,
	})
	if result.Method != MethodDeterministicWithLLMAssist {
		t.Fatalf("method = %q, want %q", result.Method, MethodDeterministicWithLLMAssist)
	}
	if len(result.AssistChunks) != 2 {
		t.Fatalf("assist chunks = %+v, want 2 chunks", result.AssistChunks)
	}
	if got := result.AssistChunks[0].SourceItemIDs; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("first chunk ids = %+v, want [1 2]", got)
	}
}

func TestEngineNormalizeUsesAssistForMissingMealCode(t *testing.T) {
	result, err := (Engine{
		Policy: Policy{AssistEnabled: true},
		Assist: staticAssistClient{response: `{"items":[{"source_item_id":1,"action":"propose_row","day":1,"meal_code":"b","food":"cooked oatmeal","quantity":1,"unit":"cup","confidence":"high","message":""}]}`},
	}).Normalize(context.Background(), Request{
		Text:   "- 1 cup cooked oatmeal",
		PlanID: "assist-test",
	})
	if err != nil {
		t.Fatalf("Normalize error: %v", err)
	}
	if result.Method != MethodDeterministicWithLLMAssist {
		t.Fatalf("method = %q, want %q", result.Method, MethodDeterministicWithLLMAssist)
	}
	if !result.AssistUsed || !result.ProviderUsed {
		t.Fatalf("assist/provider used = %v/%v, want true/true", result.AssistUsed, result.ProviderUsed)
	}
	if len(result.AcceptedAssistRows) != 1 || len(result.RejectedAssistRows) != 0 {
		t.Fatalf("accepted/rejected rows = %d/%d, want 1/0", len(result.AcceptedAssistRows), len(result.RejectedAssistRows))
	}
	if got := result.Plan.Days[0].Meals[0].Items[0].Food; got != "cooked oatmeal" {
		t.Fatalf("assisted food = %q, want cooked oatmeal", got)
	}
}

func TestDecodeAndValidateP0AssistResponseRejectsInventedSourceID(t *testing.T) {
	payload := AssistRequestPayload{
		Task:        P0AssistTask,
		ChunkID:     "chunk_1",
		SourceItems: []SourceItem{{ID: 1, Day: 1, Text: "1 cup oatmeal"}},
	}
	validation, err := DecodeAndValidateP0AssistResponse(`{"items":[{"source_item_id":99,"action":"propose_row","day":1,"meal_code":"b","food":"oatmeal","quantity":1,"unit":"cup","confidence":"high","message":""}]}`, payload)
	if err != nil {
		t.Fatalf("DecodeAndValidateP0AssistResponse error: %v", err)
	}
	if len(validation.Accepted) != 0 || len(validation.Rejected) != 1 {
		t.Fatalf("accepted/rejected = %d/%d, want 0/1", len(validation.Accepted), len(validation.Rejected))
	}
	if validation.Rejected[0].Reason != "invented_source_item_id" {
		t.Fatalf("reject reason = %q, want invented_source_item_id", validation.Rejected[0].Reason)
	}
}

type staticAssistClient struct {
	response string
}

func (c staticAssistClient) Complete(_ context.Context, _ assist.Request) (assist.Response, error) {
	return assist.Response{RawText: c.response}, nil
}
