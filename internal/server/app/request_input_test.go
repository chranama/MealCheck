package app

import (
	"strings"
	"testing"
)

func TestRequestRunInputAcceptsNativeProviderTypes(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	seeded := seededCase(t, root)

	for _, providerType := range []string{ProviderTypeOpenAI, ProviderTypeAnthropic, ProviderTypeGemini, ProviderTypeOpenAICompatible} {
		t.Run(providerType, func(t *testing.T) {
			_, input, ok, err := requestRunInput(config, CreateRunRequest{
				InputMode: "profile_generation",
				Settings:  seeded.Settings,
				Provider: ProviderConfig{
					Type:    providerType,
					BaseURL: "https://example.invalid/v1",
					Model:   "test-model",
					APIKey:  "test-key",
				},
			})
			if err != nil {
				t.Fatalf("requestRunInput error: %v", err)
			}
			if !ok {
				t.Fatal("requestRunInput ok = false, want true")
			}
			if input.Provider.Type != providerType {
				t.Fatalf("provider type = %q, want %q", input.Provider.Type, providerType)
			}
			if providerType == ProviderTypeOpenAICompatible {
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
	seeded := seededCase(t, root)
	genMessages, err := generationMessages(PendingRunInput{
		Mode:     "profile_generation",
		Settings: seeded.Settings,
	})
	if err != nil {
		t.Fatalf("generationMessages error: %v", err)
	}
	qualMessages := qualificationMessages(MealPlanQualificationRequest{
		Text:     "Day 1 breakfast: 1 cup oatmeal.",
		Settings: seeded.Settings,
	})
	for name, content := range map[string]string{
		"generation":    genMessages[1].Content,
		"qualification": qualMessages[1].Content,
	} {
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
