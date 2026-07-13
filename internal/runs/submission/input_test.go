package submission

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/llm/inference"
	"github.com/chranama/MealCheck/internal/workflow/checker"
	"github.com/chranama/MealCheck/internal/workflow/normalize"
)

func TestPrepareInputAcceptsNativeProviderTypes(t *testing.T) {
	root := repoRoot(t)
	config := core.Config{Root: root, MaxGenerationPromptChars: 1000, AccessMode: core.AccessModeInviteRequired}
	seeded, _, _, err := checker.LoadCase(root, "examples/seeded-one-day-peanut-allergy/case.json")
	if err != nil {
		t.Fatal(err)
	}

	providerTypes := []string{
		inference.ProviderTypeOpenAI,
		inference.ProviderTypeAnthropic,
		inference.ProviderTypeGemini,
		inference.ProviderTypeOpenAICompatible,
	}
	for _, providerType := range providerTypes {
		t.Run(providerType, func(t *testing.T) {
			_, input, ok, err := PrepareInput(config, core.CreateRunRequest{
				InputMode: core.InputModeProfileGeneration,
				Settings:  seeded.Settings,
				Provider: core.ProviderConfig{
					Type: providerType, BaseURL: "https://example.invalid/v1", Model: "test-model", APIKey: "test-key",
				},
			})
			if err != nil {
				t.Fatalf("PrepareInput error: %v", err)
			}
			if !ok {
				t.Fatal("PrepareInput ok = false, want true")
			}
			if input.Provider.Type != providerType {
				t.Fatalf("provider type = %q, want %q", input.Provider.Type, providerType)
			}
			if providerType == inference.ProviderTypeOpenAICompatible {
				if input.Provider.BaseURL == "" {
					t.Fatal("openai_compatible base_url was unexpectedly cleared")
				}
			} else if input.Provider.BaseURL != "" {
				t.Fatalf("native provider base_url = %q, want empty", input.Provider.BaseURL)
			}
		})
	}
}

func TestProviderPromptSettingsExposeOnlyVerifierUsedFields(t *testing.T) {
	root := repoRoot(t)
	seeded, _, _, err := checker.LoadCase(root, "examples/seeded-one-day-peanut-allergy/case.json")
	if err != nil {
		t.Fatal(err)
	}
	genMessages, err := normalize.GenerationMessages(core.PendingRunInput{Mode: core.InputModeProfileGeneration, Settings: seeded.Settings})
	if err != nil {
		t.Fatalf("GenerationMessages error: %v", err)
	}
	qualMessages := normalize.QualificationMessages(core.MealPlanQualificationRequest{
		Text: "Day 1 breakfast: 1 cup oatmeal.", Settings: seeded.Settings,
	})
	for name, content := range map[string]string{"generation": genMessages[1].Content, "qualification": qualMessages[1].Content} {
		for _, want := range []string{`"nutrition_targets"`, `"calorie_target_kcal"`, `"protein_target_g"`, `"verification_constraints"`, `"days"`, `"meals_per_day"`, `"allergies"`, `"excluded_foods"`, `"max_sodium_mg_per_day"`, `"requires_prep_safety_notes"`} {
			if !strings.Contains(content, want) {
				t.Fatalf("%s provider prompt missing %s: %s", name, want, content)
			}
		}
		for _, unwanted := range []string{`"age"`, `"sex"`, `"height_cm"`, `"weight_kg"`, `"activity_level"`, `"goal"`, `"diet_pattern"`, `"requires_shopping_list"`} {
			if strings.Contains(content, unwanted) {
				t.Fatalf("%s provider prompt contains unused field %s: %s", name, unwanted, content)
			}
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
