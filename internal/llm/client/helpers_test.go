package client

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/chranama/MealCheck/internal/llm/inference"
	"github.com/chranama/MealCheck/internal/llm/planextract"
	"github.com/chranama/MealCheck/internal/workflow/mealplan"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func readAll(t *testing.T, r *http.Request) []byte {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	return body
}

func decodeJSON(t *testing.T, data []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, string(data))
	}
}

func mealPlanRequest(messages []Message) Request {
	return Request{
		Messages: messages,
		StructuredOutput: &inference.StructuredOutput{
			Name:           "mealcheck_meal_plan",
			StrictSchema:   mealplan.StrictMealPlanResponseSchema(),
			PortableSchema: mealplan.PortableMealPlanResponseSchema(),
		},
	}
}

func compactMealPlanRequest(messages []Message) Request {
	return Request{
		Messages: messages,
		StructuredOutput: &inference.StructuredOutput{
			Name:         "mealcheck_compact_meal_plan",
			StrictSchema: planextract.MealChunkResponseSchema(),
		},
	}
}
