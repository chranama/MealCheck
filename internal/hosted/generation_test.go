package hosted

import (
	"strings"
	"testing"

	"github.com/chranama/MealCheck/internal/checker"
)

func TestLocalModelExtractionMessagesIncludeResolvedItemCount(t *testing.T) {
	messages, err := localModelExtractionMessages(PendingRunInput{
		CandidateText: strings.Join([]string{
			"Breakfast:",
			"- 1 cup oatmeal",
			"- 1/2 cup blueberries",
			"- salt to taste",
			"Lunch:",
			"- 4 oz chicken",
		}, "\n"),
		Settings: checker.Settings{
			VerificationConstraints: checker.VerificationConstraints{
				Days:        1,
				MealsPerDay: 3,
			},
		},
	})
	if err != nil {
		t.Fatalf("localModelExtractionMessages error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages length = %d, want 2", len(messages))
	}
	userPrompt := messages[1].Content
	for _, want := range []string{
		"Use exactly these meal codes for every day: b, l, d.",
		"The source contains exactly 3 resolved food item line(s); return exactly 3 row(s).",
		"Convert every numbered source item into exactly one [source_item_id, day, meal_code, food, quantity, unit] tuple.",
		"1 | day=1 | meal_code=b | source_text=1 cup oatmeal",
		"2 | day=1 | meal_code=b | source_text=1/2 cup blueberries",
		"3 | day=1 | meal_code=l | source_text=4 oz chicken",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing %q:\n%s", want, userPrompt)
		}
	}
}

func TestLocalModelExtractionMessagesNumberInlineMealItems(t *testing.T) {
	messages, err := localModelExtractionMessages(PendingRunInput{
		CandidateText: strings.Join([]string{
			"Day 1 breakfast: 1 cup cooked oatmeal, 1 cup blueberries, and 1 cup plain Greek yogurt.",
			"Day 1 lunch: 6 oz chicken breast, 1 cup brown rice, and 1 cup broccoli.",
			"Day 1 dinner: 6 oz salmon, 1 cup sweet potato, and 1 cup spinach.",
		}, "\n"),
		Settings: checker.Settings{
			VerificationConstraints: checker.VerificationConstraints{
				Days:        1,
				MealsPerDay: 3,
			},
		},
	})
	if err != nil {
		t.Fatalf("localModelExtractionMessages error: %v", err)
	}
	userPrompt := messages[1].Content
	for _, want := range []string{
		"The source contains exactly 9 resolved food item line(s); return exactly 9 row(s).",
		"1 | day=1 | meal_code=b | source_text=1 cup cooked oatmeal",
		"2 | day=1 | meal_code=b | source_text=1 cup blueberries",
		"3 | day=1 | meal_code=b | source_text=1 cup plain Greek yogurt",
		"4 | day=1 | meal_code=l | source_text=6 oz chicken breast",
		"9 | day=1 | meal_code=d | source_text=1 cup spinach",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing %q:\n%s", want, userPrompt)
		}
	}
}

func TestLocalModelExtractionMessagesPreservesAndInsideFoodNames(t *testing.T) {
	messages, err := localModelExtractionMessages(PendingRunInput{
		CandidateText: "Day 1 dinner: 1 cup macaroni and cheese, 1 banana, and 1 cup broccoli.",
		Settings: checker.Settings{
			VerificationConstraints: checker.VerificationConstraints{
				Days:        1,
				MealsPerDay: 1,
			},
		},
	})
	if err != nil {
		t.Fatalf("localModelExtractionMessages error: %v", err)
	}
	userPrompt := messages[1].Content
	for _, want := range []string{
		"The source contains exactly 3 resolved food item line(s); return exactly 3 row(s).",
		"1 | day=1 | meal_code=d | source_text=1 cup macaroni and cheese",
		"2 | day=1 | meal_code=d | source_text=1 serving banana",
		"3 | day=1 | meal_code=d | source_text=1 cup broccoli",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing %q:\n%s", want, userPrompt)
		}
	}
}

func TestValidateLocalModelExtractionCompletenessRejectsOmittedItems(t *testing.T) {
	source := strings.Join([]string{
		"Breakfast:",
		"- 1 cup oatmeal",
		"- 1/2 cup blueberries",
		"Lunch:",
		"- 4 oz chicken",
	}, "\n")
	quantity := 1.0
	plan := checker.Plan{
		SchemaVersion: "0.1",
		PlanID:        "local-model-test",
		Days: []checker.PlanDay{
			{
				Day: 1,
				Meals: []checker.Meal{
					{
						Name: "breakfast",
						Items: []checker.FoodItem{
							{Food: "oatmeal", Quantity: &quantity, Unit: "cup"},
							{Food: "blueberries", Quantity: &quantity, Unit: "cup"},
						},
					},
				},
			},
		},
	}
	err := validateLocalModelExtractionCompleteness(plan, source)
	if err == nil {
		t.Fatal("validateLocalModelExtractionCompleteness error = nil, want omitted-item error")
	}
	if !strings.Contains(err.Error(), "expected 3") {
		t.Fatalf("error = %q, want expected item count", err)
	}
}
