package normalize

import (
	"context"
	"strings"
	"testing"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

func TestQualifyMealPlanTextRejectsNonMealText(t *testing.T) {
	called := false
	result, err := QualifyMealPlanText(context.Background(), func(config ProviderConfig) (Completer, error) {
		called = true
		return &fakeProvider{}, nil
	}, MealPlanQualificationRequest{
		Text: "The weather should be warm tomorrow.",
	})
	if err != nil {
		t.Fatalf("QualifyMealPlanText error: %v", err)
	}
	if called {
		t.Fatal("provider was called for non-meal text")
	}
	if result.Status != QualificationStatusNotMealPlan {
		t.Fatalf("status = %q, want %q", result.Status, QualificationStatusNotMealPlan)
	}
	if result.NormalizedPlan != nil {
		t.Fatal("normalized plan present for non-meal text")
	}
}

func TestQualifyMealPlanTextRejectsVagueMealOutline(t *testing.T) {
	result, err := QualifyMealPlanText(context.Background(), nil, MealPlanQualificationRequest{
		Text: "Breakfast: oatmeal\nLunch: salad\nDinner: chicken bowl",
	})
	if err != nil {
		t.Fatalf("QualifyMealPlanText error: %v", err)
	}
	if result.Status != QualificationStatusMealPlanTooVague {
		t.Fatalf("status = %q, want %q", result.Status, QualificationStatusMealPlanTooVague)
	}
	for _, field := range []string{"quantities", "units"} {
		if !containsString(result.MissingFields, field) {
			t.Fatalf("missing fields = %v, want %q", result.MissingFields, field)
		}
	}
}

func TestQualifyMealPlanTextIdentifiesRecipeNeedingDecomposition(t *testing.T) {
	result, err := QualifyMealPlanText(context.Background(), nil, MealPlanQualificationRequest{
		Text: "Make a healthy chicken bowl with rice, vegetables, and sauce. Cook until warm.",
	})
	if err != nil {
		t.Fatalf("QualifyMealPlanText error: %v", err)
	}
	if result.Status != QualificationStatusRecipeOrMenuNeedsDecompose {
		t.Fatalf("status = %q, want %q", result.Status, QualificationStatusRecipeOrMenuNeedsDecompose)
	}
	if result.NormalizedPlan != nil {
		t.Fatal("normalized plan present for recipe-like text")
	}
}

func TestQualifyMealPlanTextRejectsUnsupportedPortionUnits(t *testing.T) {
	result, err := QualifyMealPlanText(context.Background(), nil, MealPlanQualificationRequest{
		Text: "Day 1 breakfast: 1 bowl cereal, 1 cup milk.\nLunch: chicken, 1 plate, and 1 cup rice.",
	})
	if err != nil {
		t.Fatalf("QualifyMealPlanText error: %v", err)
	}
	if result.Status != QualificationStatusUnsupportedUnits {
		t.Fatalf("status = %q, want %q", result.Status, QualificationStatusUnsupportedUnits)
	}
	if !containsString(result.MissingFields, "supported_units") {
		t.Fatalf("missing fields = %v, want supported_units", result.MissingFields)
	}
	if !strings.Contains(result.Reason, "bowl") || !strings.Contains(result.Reason, "plate") {
		t.Fatalf("reason = %q, want unsupported units", result.Reason)
	}
}

func TestQualifyStructuredMealPlanTextAcceptsEligiblePlan(t *testing.T) {
	result := QualifyStructuredMealPlanText(testMealPlanJSON(false))
	if result.Status != QualificationStatusEligibleForVerification {
		t.Fatalf("status = %q, want %q", result.Status, QualificationStatusEligibleForVerification)
	}
	if result.NormalizedPlan == nil {
		t.Fatal("normalized plan missing")
	}
	if result.NormalizedPlan.PlanID != "qualification-test-plan" {
		t.Fatalf("plan_id = %q, want qualification-test-plan", result.NormalizedPlan.PlanID)
	}
}

func TestQualifyStructuredMealPlanTextAcceptsEligiblePlanWithUnresolvedItems(t *testing.T) {
	result := QualifyStructuredMealPlanText(testMealPlanJSON(true))
	if result.Status != QualificationStatusEligibleWithUnresolvedItems {
		t.Fatalf("status = %q, want %q", result.Status, QualificationStatusEligibleWithUnresolvedItems)
	}
	if result.NormalizedPlan == nil {
		t.Fatal("normalized plan missing")
	}
}

func TestQualifyMealPlanTextUsesBYOKProviderForEligibleTextNormalization(t *testing.T) {
	root := repoRoot(t)
	seeded := seededCase(t, root)
	secret := "sk-qualification-secret"
	provider := &fakeProvider{responses: []string{testMealPlanJSON(false)}}

	result, err := QualifyMealPlanText(context.Background(), func(config ProviderConfig) (Completer, error) {
		if config.APIKey != secret {
			t.Fatalf("provider api key = %q, want secret", config.APIKey)
		}
		return provider, nil
	}, MealPlanQualificationRequest{
		Text: "Day 1 / Breakfast / cooked oatmeal / 1 / cup\n" + secret,
		Settings: checker.Settings{
			NutritionTargets: seeded.Settings.NutritionTargets,
			VerificationConstraints: checker.VerificationConstraints{
				Days:        1,
				MealsPerDay: 1,
			},
		},
		Provider: ProviderConfig{
			Type:   ProviderTypeOpenAI,
			Model:  "fake-model",
			APIKey: secret,
		},
	})
	if err != nil {
		t.Fatalf("QualifyMealPlanText error: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if result.Status != QualificationStatusEligibleForVerification {
		t.Fatalf("status = %q, want %q", result.Status, QualificationStatusEligibleForVerification)
	}
	if !result.ProviderUsed {
		t.Fatal("provider_used = false, want true")
	}
	if result.NormalizedPlan == nil {
		t.Fatal("normalized plan missing")
	}
	for _, call := range provider.messages {
		for _, message := range call {
			if strings.Contains(message.Content, secret) {
				t.Fatalf("qualification prompt leaked provider secret: %s", message.Content)
			}
		}
	}
}

func testMealPlanJSON(unresolved bool) string {
	item := `{
              "food": "cooked oatmeal",
              "quantity": 1,
              "unit": "cup",
              "preparation": "plain"
            }`
	if unresolved {
		item = `{
              "food": "seasoning blend",
              "quantity_text": "some",
              "resolution_status": "unresolved",
              "unresolved_reason": "vague_quantity"
            }`
	}
	return `{
  "schema_version": "0.1",
  "plan_id": "qualification-test-plan",
  "description": "One day test plan.",
  "days": [
    {
      "day": 1,
      "meals": [
        {
          "name": "breakfast",
          "items": [
            ` + item + `
          ]
        }
      ]
    }
  ],
  "shopping_list": [],
  "prep_notes": ["Refrigerate leftovers promptly."]
}`
}

func containsString(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}
