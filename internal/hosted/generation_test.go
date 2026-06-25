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

func TestLocalModelExtractionMessagesInferCountsWhenUnset(t *testing.T) {
	messages, err := localModelExtractionMessages(PendingRunInput{
		CandidateText: strings.Join([]string{
			"Day 1 breakfast: 1 cup cooked oatmeal.",
			"Day 2 dinner: 4 oz salmon.",
		}, "\n"),
	})
	if err != nil {
		t.Fatalf("localModelExtractionMessages error: %v", err)
	}
	userPrompt := messages[1].Content
	for _, unwanted := range []string{
		"Use day numbers 1..",
		"Each day must contain exactly",
		"Use exactly these meal codes",
	} {
		if strings.Contains(userPrompt, unwanted) {
			t.Fatalf("user prompt contains %q:\n%s", unwanted, userPrompt)
		}
	}
	for _, want := range []string{
		"1 | day=1 | meal_code=b | source_text=1 cup cooked oatmeal",
		"2 | day=2 | meal_code=d | source_text=4 oz salmon",
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

func TestLocalModelExtractionMessagesNormalizesSliceUnitAndSlicedOrange(t *testing.T) {
	messages, err := localModelExtractionMessages(PendingRunInput{
		CandidateText: "Day 2 breakfast: 1 serving boiled egg, 2 slices whole wheat bread, and 1 cup sliced oranges.",
	})
	if err != nil {
		t.Fatalf("localModelExtractionMessages error: %v", err)
	}
	userPrompt := messages[1].Content
	for _, want := range []string{
		"The source contains exactly 3 resolved food item line(s); return exactly 3 row(s).",
		"1 | day=2 | meal_code=b | source_text=1 serving boiled egg",
		"2 | day=2 | meal_code=b | source_text=2 slice whole wheat bread",
		"3 | day=2 | meal_code=b | source_text=1 cup sliced oranges",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing %q:\n%s", want, userPrompt)
		}
	}
}

func TestLocalModelDaySectionsRewritesEachDayForSingleDayExtraction(t *testing.T) {
	sections, ok := localModelDaySections(strings.Join([]string{
		"Day 1 breakfast: 1 cup cooked oatmeal, 1 cup blueberries, and 1 cup plain Greek yogurt.",
		"Day 1 lunch: 4 oz chicken breast, 1 cup brown rice, and 1 cup broccoli.",
		"Day 1 dinner: 4 oz salmon, 1 cup sweet potato, and 1 cup spinach.",
		"Day 2 breakfast: 2 eggs, 1 cup whole wheat toast, and 1 cup orange segments.",
		"Day 2 lunch: 4 oz tuna, 2 cups mixed greens, and 1 tsp vinaigrette.",
		"Day 2 dinner: 5 oz turkey meatballs, 1 cup whole wheat pasta, and 1 cup tomato sauce.",
	}, "\n"))
	if !ok {
		t.Fatal("localModelDaySections ok = false, want true")
	}
	if len(sections) != 2 {
		t.Fatalf("sections length = %d, want 2", len(sections))
	}
	if sections[0].Day != 1 || sections[1].Day != 2 {
		t.Fatalf("section days = %d/%d, want 1/2", sections[0].Day, sections[1].Day)
	}
	if strings.Contains(sections[1].Text, "Day 2") {
		t.Fatalf("second section was not rewritten for one-day extraction:\n%s", sections[1].Text)
	}
	if !strings.Contains(sections[1].Text, "Day 1 breakfast") {
		t.Fatalf("second section missing rewritten day marker:\n%s", sections[1].Text)
	}
	if got := localLlamaExpectedResolvedItemCount(sections[1].Text); got != 9 {
		t.Fatalf("second section resolved item count = %d, want 9", got)
	}
}

func TestLocalModelDaySectionsUsesObservedDayCoverage(t *testing.T) {
	sections, ok := localModelDaySections(strings.Join([]string{
		"Day 1 breakfast: 1 cup cooked oatmeal.",
		"Day 3 breakfast: 1 cup cooked oatmeal.",
	}, "\n"))
	if !ok {
		t.Fatal("localModelDaySections ok = false, want true for observed day labels")
	}
	if len(sections) != 2 || sections[0].Day != 1 || sections[1].Day != 3 {
		t.Fatalf("sections = %+v, want day 1 and day 3", sections)
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
