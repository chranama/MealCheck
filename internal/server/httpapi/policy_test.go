package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHostedQueueLimit(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.QueueSize = 1
	store := NewMemoryStore()
	server := NewServer(config, store)

	body := `{"case_path":"examples/seeded-one-day-peanut-allergy/case.json"}`
	first := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first create status = %d body=%s", first.Code, first.Body.String())
	}
	second := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second create status = %d, want 429 body=%s", second.Code, second.Body.String())
	}
}

func TestPublicRequestRateLimit(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.PublicRequestLimit = 1
	config.PublicRequestWindow = time.Hour
	server := NewServer(config, NewMemoryStore())
	seeded := seededCase(t, root)
	body := marshalJSON(t, MealPlanQualificationRequest{
		Text:     testMealPlanJSON(false),
		Settings: seeded.Settings,
	})

	first := doRequest(t, server, http.MethodPost, "/api/qualify", body)
	if first.Code != http.StatusOK {
		t.Fatalf("first qualify status = %d, want 200 body=%s", first.Code, first.Body.String())
	}
	second := doRequest(t, server, http.MethodPost, "/api/qualify", body)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second qualify status = %d, want 429 body=%s", second.Code, second.Body.String())
	}
	if got := second.Header().Get("Retry-After"); got == "" {
		t.Fatal("rate-limited response missing Retry-After")
	}
}

func TestPublicDailyRunLimit(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.PublicDailyRunLimit = 1
	store := NewMemoryStore()
	server := NewServer(config, store)

	body := `{"case_path":"examples/seeded-one-day-peanut-allergy/case.json"}`
	first := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first create status = %d, want 202 body=%s", first.Code, first.Body.String())
	}
	second := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second create status = %d, want 429 body=%s", second.Code, second.Body.String())
	}
	if !strings.Contains(second.Body.String(), "daily public run limit") {
		t.Fatalf("daily limit body missing reason: %s", second.Body.String())
	}
}

func TestPublicModeRejectsOpenAICompatibleByDefault(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.PublicOpenAICompatible = false
	server := NewServer(config, NewMemoryStore())
	seeded := seededCase(t, root)

	body := marshalJSON(t, CreateRunRequest{
		InputMode: "profile_generation",
		Settings:  seeded.Settings,
		Provider: ProviderConfig{
			Type:    ProviderTypeOpenAICompatible,
			BaseURL: "https://router.example/v1",
			Model:   "custom",
			APIKey:  "secret",
		},
	})
	resp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want 400 body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "openai_compatible providers are disabled") {
		t.Fatalf("body missing openai_compatible policy message: %s", resp.Body.String())
	}
}

func TestPublicModeRejectsLocalOpenAICompatibleBaseURLWhenEnabled(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.PublicOpenAICompatible = true
	server := NewServer(config, NewMemoryStore())
	seeded := seededCase(t, root)

	body := marshalJSON(t, CreateRunRequest{
		InputMode: "profile_generation",
		Settings:  seeded.Settings,
		Provider: ProviderConfig{
			Type:    ProviderTypeOpenAICompatible,
			BaseURL: "https://127.0.0.1/v1",
			Model:   "custom",
			APIKey:  "secret",
		},
	})
	resp := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want 400 body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "private or local IP") {
		t.Fatalf("body missing local IP policy message: %s", resp.Body.String())
	}
}

func TestInviteModeAllowsOpenAICompatibleWhenPublicCustomEndpointsDisabled(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.AccessMode = AccessModeInviteRequired
	config.InviteToken = "invite-secret"
	config.PublicOpenAICompatible = false
	server := NewServer(config, NewMemoryStore())
	seeded := seededCase(t, root)

	body := marshalJSON(t, CreateRunRequest{
		InputMode: "profile_generation",
		Settings:  seeded.Settings,
		Provider: ProviderConfig{
			Type:   ProviderTypeOpenAICompatible,
			Model:  "fake-model",
			APIKey: "secret",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MealCheck-Invite-Token", "invite-secret")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, want 202 body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestInviteTokenGate(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.AccessMode = AccessModeInviteRequired
	config.InviteToken = "invite-secret"
	server := NewServer(config, NewMemoryStore())

	body := `{"case_path":"examples/seeded-one-day-peanut-allergy/case.json"}`
	unauthorized := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401 body=%s", unauthorized.Code, unauthorized.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MealCheck-Invite-Token", "invite-secret")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("authorized status = %d, want 202 body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPerUserInviteTokenGate(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.AccessMode = AccessModeInviteRequired
	config.InviteRequired = true
	store := NewMemoryStore()
	maxRuns := 1
	generated, err := GenerateInviteToken("reviewer-chris", nil, &maxRuns, time.Now().UTC())
	if err != nil {
		t.Fatalf("generate invite token: %v", err)
	}
	if err := store.CreateInviteToken(context.Background(), generated.Invite); err != nil {
		t.Fatalf("create invite token: %v", err)
	}
	server := NewServer(config, store)

	body := `{"case_path":"examples/seeded-one-day-peanut-allergy/case.json"}`
	missing := doRequest(t, server, http.MethodPost, "/api/runs", body)
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing invite status = %d, want 401 body=%s", missing.Code, missing.Body.String())
	}

	wrong := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader([]byte(body)))
	wrong.Header.Set("Content-Type", "application/json")
	wrong.Header.Set("X-MealCheck-Invite-Token", InviteTokenPrefix+generated.Invite.ID+".wrong-secret")
	wrongRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(wrongRecorder, wrong)
	if wrongRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("wrong invite status = %d, want 401 body=%s", wrongRecorder.Code, wrongRecorder.Body.String())
	}

	allowed := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader([]byte(body)))
	allowed.Header.Set("Content-Type", "application/json")
	allowed.Header.Set("X-MealCheck-Invite-Token", generated.Token)
	allowedRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(allowedRecorder, allowed)
	if allowedRecorder.Code != http.StatusAccepted {
		t.Fatalf("per-user invite status = %d, want 202 body=%s", allowedRecorder.Code, allowedRecorder.Body.String())
	}
	invite, err := store.GetInviteToken(context.Background(), generated.Invite.ID)
	if err != nil {
		t.Fatalf("get invite token: %v", err)
	}
	if invite.UsedRuns != 1 || invite.LastUsedAt == nil {
		t.Fatalf("invite usage = %d last_used=%v, want one recorded use", invite.UsedRuns, invite.LastUsedAt)
	}

	exhausted := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader([]byte(body)))
	exhausted.Header.Set("Content-Type", "application/json")
	exhausted.Header.Set("X-MealCheck-Invite-Token", generated.Token)
	exhaustedRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(exhaustedRecorder, exhausted)
	if exhaustedRecorder.Code != http.StatusTooManyRequests {
		t.Fatalf("exhausted invite status = %d, want 429 body=%s", exhaustedRecorder.Code, exhaustedRecorder.Body.String())
	}
}

func TestCORSAllowsOnlyConfiguredOrigin(t *testing.T) {
	root := repoRoot(t)
	config := testConfig(t, root)
	config.AllowedOrigin = "http://127.0.0.1:4173"
	server := NewServer(config, NewMemoryStore())

	allowed := httptest.NewRequest(http.MethodOptions, "/api/runs", nil)
	allowed.Header.Set("Origin", config.AllowedOrigin)
	allowed.Header.Set("Access-Control-Request-Method", http.MethodPost)
	allowedRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(allowedRecorder, allowed)
	if allowedRecorder.Code != http.StatusNoContent {
		t.Fatalf("allowed preflight status = %d, want 204", allowedRecorder.Code)
	}
	if got := allowedRecorder.Header().Get("Access-Control-Allow-Origin"); got != config.AllowedOrigin {
		t.Fatalf("allowed origin header = %q, want %q", got, config.AllowedOrigin)
	}
	if got := allowedRecorder.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-MealCheck-Invite-Token") {
		t.Fatalf("allowed headers missing invite token header: %q", got)
	}

	disallowed := httptest.NewRequest(http.MethodOptions, "/api/runs", nil)
	disallowed.Header.Set("Origin", "https://example.invalid")
	disallowed.Header.Set("Access-Control-Request-Method", http.MethodPost)
	disallowedRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(disallowedRecorder, disallowed)
	if disallowedRecorder.Code != http.StatusNoContent {
		t.Fatalf("disallowed preflight status = %d, want 204", disallowedRecorder.Code)
	}
	if got := disallowedRecorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("disallowed origin header = %q, want empty", got)
	}
	if got := disallowedRecorder.Header().Values("Vary"); len(got) == 0 {
		t.Fatal("expected Vary: Origin for configured CORS")
	}

	noOrigin := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	noOriginRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(noOriginRecorder, noOrigin)
	if got := noOriginRecorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("no-origin CORS header = %q, want empty", got)
	}
}
