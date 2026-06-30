package hosted

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
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
		"The source contains exactly 4 numbered source item(s); return exactly 4 row(s).",
		"Convert every numbered source item into exactly one row.",
		"1 | day=1 | meal_code=b | status=resolved | source_text=1 cup oatmeal",
		"2 | day=1 | meal_code=b | status=resolved | source_text=1/2 cup blueberries",
		"3 | day=1 | meal_code=b | status=unresolved | source_text=salt to taste",
		"4 | day=1 | meal_code=l | status=resolved | source_text=4 oz chicken",
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
		"The source contains exactly 9 numbered source item(s); return exactly 9 row(s).",
		"1 | day=1 | meal_code=b | status=resolved | source_text=1 cup cooked oatmeal",
		"2 | day=1 | meal_code=b | status=resolved | source_text=1 cup blueberries",
		"3 | day=1 | meal_code=b | status=resolved | source_text=1 cup plain Greek yogurt",
		"4 | day=1 | meal_code=l | status=resolved | source_text=6 oz chicken breast",
		"9 | day=1 | meal_code=d | status=resolved | source_text=1 cup spinach",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing %q:\n%s", want, userPrompt)
		}
	}
}

func TestLocalModelExtractionMessagesDefaultsToOneDayWhenSettingsUnset(t *testing.T) {
	messages, err := localModelExtractionMessages(PendingRunInput{
		CandidateText: strings.Join([]string{
			"Day 1 breakfast: 1 cup cooked oatmeal.",
			"Day 1 dinner: 4 oz salmon.",
		}, "\n"),
	})
	if err != nil {
		t.Fatalf("localModelExtractionMessages error: %v", err)
	}
	userPrompt := messages[1].Content
	for _, unwanted := range []string{
		"Each day must contain exactly",
		"Use exactly these meal codes",
	} {
		if strings.Contains(userPrompt, unwanted) {
			t.Fatalf("user prompt contains %q:\n%s", unwanted, userPrompt)
		}
	}
	for _, want := range []string{
		"Use day numbers 1..1.",
		"1 | day=1 | meal_code=b | status=resolved | source_text=1 cup cooked oatmeal",
		"2 | day=1 | meal_code=d | status=resolved | source_text=4 oz salmon",
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
		"The source contains exactly 3 numbered source item(s); return exactly 3 row(s).",
		"1 | day=1 | meal_code=d | status=resolved | source_text=1 cup macaroni and cheese",
		"2 | day=1 | meal_code=d | status=resolved | source_text=1 serving banana",
		"3 | day=1 | meal_code=d | status=resolved | source_text=1 cup broccoli",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing %q:\n%s", want, userPrompt)
		}
	}
}

func TestLocalModelExtractionMessagesNormalizesSliceUnitAndSlicedOrange(t *testing.T) {
	messages, err := localModelExtractionMessages(PendingRunInput{
		CandidateText: "Day 1 breakfast: 1 serving boiled egg, 2 slices whole wheat bread, and 1 cup sliced oranges.",
	})
	if err != nil {
		t.Fatalf("localModelExtractionMessages error: %v", err)
	}
	userPrompt := messages[1].Content
	for _, want := range []string{
		"The source contains exactly 3 numbered source item(s); return exactly 3 row(s).",
		"1 | day=1 | meal_code=b | status=resolved | source_text=1 serving boiled egg",
		"2 | day=1 | meal_code=b | status=resolved | source_text=2 slice whole wheat bread",
		"3 | day=1 | meal_code=b | status=resolved | source_text=1 cup sliced oranges",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing %q:\n%s", want, userPrompt)
		}
	}
}

func TestMealPlanInputRobustnessManifestSourceInventory(t *testing.T) {
	root := repoRoot(t)
	manifestPath := filepath.Join(root, "examples/meal-plan-input-robustness/manifest.json")
	var manifest struct {
		Cases []struct {
			ID                     string              `json:"id"`
			File                   string              `json:"file"`
			ExpectedDays           int                 `json:"expected_days"`
			ExpectedMealCodesByDay map[string][]string `json:"expected_meal_codes_by_day"`
			ExpectedItemCount      int                 `json:"expected_item_count"`
		} `json:"cases"`
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read robustness manifest: %v", err)
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("decode robustness manifest: %v", err)
	}
	if len(manifest.Cases) == 0 {
		t.Fatal("robustness manifest has no cases")
	}

	for _, tc := range manifest.Cases {
		t.Run(tc.ID, func(t *testing.T) {
			casePath := filepath.Join(root, "examples/meal-plan-input-robustness", tc.File)
			caseBytes, err := os.ReadFile(casePath)
			if err != nil {
				t.Fatalf("read robustness case: %v", err)
			}
			text := string(caseBytes)
			items := localLlamaResolvedSourceItems(text)
			if len(items) != tc.ExpectedItemCount {
				t.Fatalf("resolved source item count = %d, want %d\nitems=%+v", len(items), tc.ExpectedItemCount, items)
			}
			itemCountInstruction := localLlamaItemCountInstruction(text)
			if !strings.Contains(itemCountInstruction, "exactly "+strconv.Itoa(tc.ExpectedItemCount)+" numbered source item") {
				t.Fatalf("item count instruction does not include expected count %d: %s", tc.ExpectedItemCount, itemCountInstruction)
			}

			seen := map[int]map[string]int{}
			for index, item := range items {
				if item.ID != index+1 {
					t.Fatalf("source item id at index %d = %d, want %d", index, item.ID, index+1)
				}
				if item.Day < 1 {
					t.Fatalf("source item has invalid day: %+v", item)
				}
				if item.MealCode == "" {
					t.Fatalf("source item missing meal code: %+v", item)
				}
				if seen[item.Day] == nil {
					seen[item.Day] = map[string]int{}
				}
				seen[item.Day][item.MealCode]++
			}

			if tc.ID == "default_hosted_natural_rewrite" {
				sourceTexts := map[string]bool{}
				for _, item := range items {
					sourceTexts[item.Text] = true
				}
				for _, want := range []string{
					"1 cup cooked oatmeal",
					"2 slice whole wheat bread",
					"1 cup sliced oranges",
				} {
					if !sourceTexts[want] {
						t.Fatalf("natural rewrite missing normalized source text %q; got=%v", want, sourceTexts)
					}
				}
			}

			for dayText, codes := range tc.ExpectedMealCodesByDay {
				day, err := strconv.Atoi(dayText)
				if err != nil {
					t.Fatalf("invalid expected day %q: %v", dayText, err)
				}
				for _, code := range codes {
					if seen[day][code] == 0 {
						t.Fatalf("missing source items for day %d meal code %s; seen=%v", day, code, seen)
					}
				}
			}

			if tc.ExpectedDays > 1 {
				err := validateLocalModelInputContract(Config{LocalModelMaxSourceItems: 100}, text)
				if err == nil {
					t.Fatalf("multi-day case passed hosted local-model input contract")
				}
				if !strings.Contains(err.Error(), "one day") {
					t.Fatalf("multi-day contract error = %q, want one-day rejection", err)
				}
			}
		})
	}
}

func TestValidateLocalModelInputContractRejectsMultiDayText(t *testing.T) {
	err := validateLocalModelInputContract(Config{LocalModelMaxSourceItems: 20}, strings.Join([]string{
		"Day 1 breakfast: 1 cup cooked oatmeal.",
		"Day 2 breakfast: 1 cup cooked oatmeal.",
	}, "\n"))
	if err == nil {
		t.Fatal("validateLocalModelInputContract error = nil, want multi-day rejection")
	}
	if !strings.Contains(err.Error(), "one day only") {
		t.Fatalf("error = %q, want one-day contract message", err)
	}
}

func TestValidateLocalModelInputContractRejectsTooManySourceItems(t *testing.T) {
	text := strings.Join([]string{
		"Day 1 breakfast:",
		"- 1 cup cooked oatmeal",
		"- 1 cup blueberries",
		"- 1 cup plain Greek yogurt",
	}, "\n")
	err := validateLocalModelInputContract(Config{LocalModelMaxSourceItems: 2}, text)
	if err == nil {
		t.Fatal("validateLocalModelInputContract error = nil, want item limit rejection")
	}
	if !strings.Contains(err.Error(), "at most 2") {
		t.Fatalf("error = %q, want item limit", err)
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
