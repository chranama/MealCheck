package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

func TestQualifyEndpointRequiresInviteAndReturnsStructuredQualification(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.AccessMode = AccessModeInviteRequired
	config.InviteToken = "invite-secret"
	store := NewMemoryStore()
	server := NewServer(config, store)
	seeded := seededCase(t, root)

	body := marshalJSON(t, MealPlanQualificationRequest{
		Text:     testMealPlanJSON(false),
		Settings: seeded.Settings,
	})
	unauthorized := doRequest(t, server, http.MethodPost, "/api/qualify", body)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401 body=%s", unauthorized.Code, unauthorized.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/qualify", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MealCheck-Invite-Token", "invite-secret")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("qualify status = %d, want 200 body=%s", recorder.Code, recorder.Body.String())
	}
	var response QualifyMealPlanResponse
	decodeJSON(t, recorder.Body.Bytes(), &response)
	if response.Qualification.Status != QualificationStatusEligibleForVerification {
		t.Fatalf("qualification status = %q, want %q", response.Qualification.Status, QualificationStatusEligibleForVerification)
	}
	if response.Qualification.NormalizedPlan == nil {
		t.Fatal("normalized plan missing")
	}
	if response.Qualification.ProviderUsed {
		t.Fatal("provider_used = true, want false for already-normalized JSON")
	}
}

func TestQualifyEndpointRequiresProviderOnlyWhenNormalizationIsNeeded(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	server := NewServer(config, store)
	seeded := seededCase(t, root)

	body := marshalJSON(t, MealPlanQualificationRequest{
		Text:     "Day 1 breakfast: 1 cup cooked oatmeal.",
		Settings: seeded.Settings,
	})
	resp := doRequest(t, server, http.MethodPost, "/api/qualify", body)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("qualify status = %d, want 400 body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "provider model is required") {
		t.Fatalf("provider validation body missing model error: %s", resp.Body.String())
	}
}

func TestQualifyEndpointUsesBYOKProviderForTextNormalization(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	store := NewMemoryStore()
	server := NewServer(config, store)
	seeded := seededCase(t, root)
	secret := "sk-qualify-endpoint-secret"
	provider := &fakeProvider{responses: []string{testMealPlanJSON(false)}}
	server.ProviderFactory = func(config ProviderConfig) (Provider, error) {
		if config.Type != ProviderTypeOpenAI {
			t.Fatalf("provider type = %q, want openai", config.Type)
		}
		if config.APIKey != secret {
			t.Fatalf("provider api key = %q, want secret", config.APIKey)
		}
		return provider, nil
	}

	body := marshalJSON(t, MealPlanQualificationRequest{
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
	resp := doRequest(t, server, http.MethodPost, "/api/qualify", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("qualify status = %d, want 200 body=%s", resp.Code, resp.Body.String())
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	var response QualifyMealPlanResponse
	decodeJSON(t, resp.Body.Bytes(), &response)
	if response.Qualification.Status != QualificationStatusEligibleForVerification {
		t.Fatalf("qualification status = %q, want %q", response.Qualification.Status, QualificationStatusEligibleForVerification)
	}
	if !response.Qualification.ProviderUsed {
		t.Fatal("provider_used = false, want true")
	}
}

func TestQualifyEndpointUsesHostedLocalModelWithoutClientProvider(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.HostedMode = HostedModeLocalModel
	config.LocalModelEnabled = true
	config.LocalModelBaseURL = "http://127.0.0.1:11435/v1"
	config.LocalModelName = "/Users/chranama-server/MealCheck-data/models/Qwen3-0.6B-Q4_K_M.gguf"
	store := NewMemoryStore()
	server := NewServer(config, store)
	seeded := seededCase(t, root)
	provider := &fakeProvider{responses: compactLocalMealPlanJSONResponses()}
	server.ProviderFactory = func(config ProviderConfig) (Provider, error) {
		if config.Type != ProviderTypeLocalLlama {
			t.Fatalf("provider type = %q, want %q", config.Type, ProviderTypeLocalLlama)
		}
		if config.APIKey != "" {
			t.Fatalf("local provider api key = %q, want empty", config.APIKey)
		}
		return provider, nil
	}

	body := marshalJSON(t, MealPlanQualificationRequest{
		Text:     "Breakfast: 1 cup cooked oatmeal, 1 cup blueberries, 1 cup plain Greek yogurt.\nLunch: 4 oz chicken breast, 1 cup brown rice, 1 cup broccoli.\nDinner: 4 oz salmon, 1 cup sweet potato, 1 cup spinach.",
		Settings: localModelTestSettings(seeded.Settings),
	})
	resp := doRequest(t, server, http.MethodPost, "/api/qualify", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("qualify status = %d, want 200 body=%s", resp.Code, resp.Body.String())
	}
	if provider.calls != 3 {
		t.Fatalf("provider calls = %d, want 3", provider.calls)
	}
	var response QualifyMealPlanResponse
	decodeJSON(t, resp.Body.Bytes(), &response)
	if response.Qualification.Status != QualificationStatusEligibleForVerification {
		t.Fatalf("qualification status = %q, want %q", response.Qualification.Status, QualificationStatusEligibleForVerification)
	}
	if !response.Qualification.ProviderUsed {
		t.Fatal("provider_used = false, want true")
	}
	if response.Qualification.NormalizedPlan == nil {
		t.Fatal("normalized plan missing")
	}
}

func TestQualifyEndpointReturnsQualificationForHostedLocalModelContractFailure(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.HostedMode = HostedModeLocalModel
	config.LocalModelEnabled = true
	config.LocalModelBaseURL = "http://127.0.0.1:11435/v1"
	config.LocalModelName = "/Users/chranama-server/MealCheck-data/models/Qwen3-0.6B-Q4_K_M.gguf"
	store := NewMemoryStore()
	server := NewServer(config, store)
	seeded := seededCase(t, root)

	body := marshalJSON(t, MealPlanQualificationRequest{
		Text: strings.Join([]string{
			"Day 1 breakfast: 1 cup cooked oatmeal.",
			"Day 2 breakfast: 1 cup cooked oatmeal.",
		}, "\n"),
		Settings: localModelTestSettings(seeded.Settings),
	})
	resp := doRequest(t, server, http.MethodPost, "/api/qualify", body)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("qualify status = %d, want 422 body=%s", resp.Code, resp.Body.String())
	}
	var errorResponse ErrorResponse
	decodeJSON(t, resp.Body.Bytes(), &errorResponse)
	if errorResponse.Error.Code != "meal_plan_not_verifiable" {
		t.Fatalf("error code = %q, want meal_plan_not_verifiable", errorResponse.Error.Code)
	}
	qualification := qualificationFromErrorResponse(t, errorResponse)
	if qualification.Status != QualificationStatusOutsideHostedContract {
		t.Fatalf("qualification status = %q, want %q", qualification.Status, QualificationStatusOutsideHostedContract)
	}
	if qualification.ProviderUsed {
		t.Fatal("provider_used = true, want false")
	}
}
