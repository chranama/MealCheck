package localmodel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/chranama/MealCheck/internal/workflow/checker"
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
		"Meal: day=1 meal_code=b meal_label=breakfast.",
		"The full request contains exactly 3 distinct meal chunk(s) for the day.",
		"This meal chunk contains exactly 3 numbered source item(s); return exactly 3 row(s).",
		"Meal text:\nBreakfast:",
		"Convert every numbered source item into exactly one row.",
		"The server already knows day and meal_code; do not output day or meal_code.",
		"1 | status=resolved | source_text=1 cup oatmeal",
		"2 | status=resolved | source_text=1/2 cup blueberries",
		"3 | status=needs_model_parse | source_text=salt to taste",
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
		"This meal chunk contains exactly 3 numbered source item(s); return exactly 3 row(s).",
		"Meal text:\nDay 1 breakfast: 1 cup cooked oatmeal, 1 cup blueberries, and 1 cup plain Greek yogurt.",
		"1 | status=resolved | source_text=1 cup cooked oatmeal",
		"2 | status=resolved | source_text=1 cup blueberries",
		"3 | status=resolved | source_text=1 cup plain Greek yogurt",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing %q:\n%s", want, userPrompt)
		}
	}
	chunks := localLlamaExtractionMealChunks(strings.Join([]string{
		"Day 1 breakfast: 1 cup cooked oatmeal, 1 cup blueberries, and 1 cup plain Greek yogurt.",
		"Day 1 lunch: 6 oz chicken breast, 1 cup brown rice, and 1 cup broccoli.",
		"Day 1 dinner: 6 oz salmon, 1 cup sweet potato, and 1 cup spinach.",
	}, "\n"))
	if len(chunks) != 3 {
		t.Fatalf("meal chunks = %d, want 3", len(chunks))
	}
	if chunks[1].Items[0].ID != 4 || chunks[2].Items[2].ID != 9 {
		t.Fatalf("global source IDs not preserved across chunks: %+v", chunks)
	}
}

func TestLocalModelExtractionMessagesNumberParagraphMealItems(t *testing.T) {
	text := "For breakfast I will have 1 cup oatmeal with 0.5 cup blueberries and 1 cup plain Greek yogurt. For lunch I will have 4 oz chicken breast with 1 cup brown rice plus 1 cup broccoli. Dinner includes 4 oz salmon and 1 serving sweet potato."
	messages, err := localModelExtractionMessages(PendingRunInput{
		CandidateText: text,
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
		"This meal chunk contains exactly 3 numbered source item(s); return exactly 3 row(s).",
		"Meal text:\nFor breakfast I will have 1 cup oatmeal with 0.5 cup blueberries and 1 cup plain Greek yogurt.",
		"1 | status=resolved | source_text=1 cup oatmeal",
		"2 | status=resolved | source_text=0.5 cup blueberries",
		"3 | status=resolved | source_text=1 cup plain Greek yogurt",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing %q:\n%s", want, userPrompt)
		}
	}
	chunks := localLlamaExtractionMealChunks(text)
	if len(chunks) != 3 || chunks[1].Items[0].ID != 4 || chunks[2].Items[1].ID != 8 {
		t.Fatalf("paragraph chunks/source IDs = %+v, want three meal chunks with global IDs", chunks)
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
		"The full request is limited to day numbers 1..1.",
		"1 | status=resolved | source_text=1 cup cooked oatmeal",
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
		"This meal chunk contains exactly 3 numbered source item(s); return exactly 3 row(s).",
		"1 | status=resolved | source_text=1 cup macaroni and cheese",
		"2 | status=resolved | source_text=1 serving banana",
		"3 | status=resolved | source_text=1 cup broccoli",
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
		"This meal chunk contains exactly 3 numbered source item(s); return exactly 3 row(s).",
		"1 | status=resolved | source_text=1 serving boiled egg",
		"2 | status=resolved | source_text=2 slice whole wheat bread",
		"3 | status=resolved | source_text=1 cup sliced oranges",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing %q:\n%s", want, userPrompt)
		}
	}
}

func TestLocalModelExtractionMessagesNormalizesReverseQuantityOrder(t *testing.T) {
	messages, err := localModelExtractionMessages(PendingRunInput{
		CandidateText: "Day 1 lunch: chicken, 100 g, rice, 1 cup, and a side salad.",
	})
	if err != nil {
		t.Fatalf("localModelExtractionMessages error: %v", err)
	}
	userPrompt := messages[1].Content
	for _, want := range []string{
		"Meal: day=1 meal_code=l meal_label=lunch.",
		"1 | status=resolved | source_text=100 g chicken",
		"2 | status=resolved | source_text=1 cup rice",
		"3 | status=needs_model_parse | source_text=a side salad",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing %q:\n%s", want, userPrompt)
		}
	}
}

func TestLocalModelExtractionMessagesNormalizesParagraphReverseQuantityOrder(t *testing.T) {
	chunks := localLlamaExtractionMealChunks("For lunch I had chicken, 100 g, rice, 1 cup, and a side salad.")
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1: %+v", len(chunks), chunks)
	}
	if len(chunks[0].Items) != 3 {
		t.Fatalf("chunk items = %d, want 3: %+v", len(chunks[0].Items), chunks[0].Items)
	}
	wants := []struct {
		text   string
		status localLlamaSourceParseStatus
	}{
		{text: "100 g chicken", status: localLlamaSourceResolved},
		{text: "1 cup rice", status: localLlamaSourceResolved},
		{text: "a side salad", status: localLlamaSourceNeedsModelParse},
	}
	for index, want := range wants {
		item := chunks[0].Items[index]
		if item.Text != want.text || item.ParseStatus != want.status {
			t.Fatalf("item %d = %+v, want text=%q status=%s", index, item, want.text, want.status)
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
	if !strings.Contains(err.Error(), "one day") {
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

func TestValidateLocalModelInputContractAcceptsParagraphMealPlan(t *testing.T) {
	text := "For breakfast I will have 1 cup oatmeal with 0.5 cup blueberries. For lunch I will have 4 oz chicken with 1 cup rice. Dinner includes 4 oz salmon and 1 serving sweet potato."
	err := validateLocalModelInputContract(Config{LocalModelMaxSourceItems: 20}, text)
	if err != nil {
		t.Fatalf("validateLocalModelInputContract error = %v, want nil", err)
	}
}

func TestValidateLocalModelInputContractRejectsZeroInventoryMealText(t *testing.T) {
	text := "Please draft a short meeting agenda for tomorrow afternoon."
	err := validateLocalModelInputContract(Config{LocalModelMaxSourceItems: 20}, text)
	if err == nil {
		t.Fatal("validateLocalModelInputContract error = nil, want zero-inventory rejection")
	}
	if !strings.Contains(err.Error(), "source food item") {
		t.Fatalf("error = %q, want source item guidance", err)
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
