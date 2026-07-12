package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIRequestAndResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-openai" {
			t.Fatalf("authorization = %q", got)
		}
		var payload map[string]any
		decodeJSON(t, readAll(t, r), &payload)
		if payload["model"] != "gpt-test" {
			t.Fatalf("model = %v", payload["model"])
		}
		format, ok := payload["response_format"].(map[string]any)
		if !ok || format["type"] != "json_schema" {
			t.Fatalf("response_format = %#v", payload["response_format"])
		}
		jsonSchema, ok := format["json_schema"].(map[string]any)
		if !ok || jsonSchema["name"] != "mealcheck_meal_plan" || jsonSchema["strict"] != true {
			t.Fatalf("json_schema = %#v", format["json_schema"])
		}
		schema, ok := jsonSchema["schema"].(map[string]any)
		if !ok || schema["additionalProperties"] != false {
			t.Fatalf("schema = %#v", jsonSchema["schema"])
		}
		days := schema["properties"].(map[string]any)["days"].(map[string]any)
		day := days["items"].(map[string]any)
		dayValue := day["properties"].(map[string]any)["day"].(map[string]any)
		if dayValue["minimum"] != float64(1) && dayValue["minimum"] != 1 {
			t.Fatalf("strict schema day minimum = %#v", dayValue["minimum"])
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"schema_version\":\"0.1\"}"}}]}`))
	}))
	defer server.Close()

	provider := OpenAI{Client: server.Client(), BaseURL: server.URL + "/v1"}
	got, err := provider.Complete(context.Background(), ProviderConfig{
		Type:   ProviderTypeOpenAI,
		Model:  "gpt-test",
		APIKey: "sk-openai",
	}, mealPlanRequest([]Message{{Role: "system", Content: "Return JSON."}, {Role: "user", Content: "Plan."}}))
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if got != `{"schema_version":"0.1"}` {
		t.Fatalf("content = %q", got)
	}
}

func TestOpenAICompatibleKeepsJSONMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q, want /chat/completions", r.URL.Path)
		}
		var payload map[string]any
		decodeJSON(t, readAll(t, r), &payload)
		format, ok := payload["response_format"].(map[string]any)
		if !ok || format["type"] != "json_object" {
			t.Fatalf("response_format = %#v", payload["response_format"])
		}
		if _, ok := format["json_schema"]; ok {
			t.Fatalf("openai-compatible response_format unexpectedly included json_schema: %#v", format)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"schema_version\":\"0.1\"}"}}]}`))
	}))
	defer server.Close()

	provider := OpenAICompatible{Client: server.Client()}
	got, err := provider.Complete(context.Background(), ProviderConfig{
		Type:    ProviderTypeOpenAICompatible,
		BaseURL: server.URL,
		Model:   "compatible-test",
		APIKey:  "sk-compatible",
	}, mealPlanRequest([]Message{{Role: "user", Content: "Plan."}}))
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if got != `{"schema_version":"0.1"}` {
		t.Fatalf("content = %q", got)
	}
}

func TestLlamaCPPUsesCompactSchemaWithoutAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("authorization header = %q, want empty", got)
		}
		var payload map[string]any
		decodeJSON(t, readAll(t, r), &payload)
		if payload["model"] != "local-model" {
			t.Fatalf("model = %v", payload["model"])
		}
		if payload["max_tokens"] != float64(128) {
			t.Fatalf("max_tokens = %v, want 128", payload["max_tokens"])
		}
		format, ok := payload["response_format"].(map[string]any)
		if !ok || format["type"] != "json_schema" {
			t.Fatalf("response_format = %#v", payload["response_format"])
		}
		jsonSchema, ok := format["json_schema"].(map[string]any)
		if !ok || jsonSchema["name"] != "mealcheck_compact_meal_plan" || jsonSchema["strict"] != true {
			t.Fatalf("json_schema = %#v", format["json_schema"])
		}
		schema, ok := jsonSchema["schema"].(map[string]any)
		if !ok {
			t.Fatalf("schema = %#v", jsonSchema["schema"])
		}
		required, ok := schema["required"].([]any)
		if !ok || len(required) != 1 || required[0] != "i" {
			t.Fatalf("compact schema required = %#v", schema["required"])
		}
		templateArgs, ok := payload["chat_template_kwargs"].(map[string]any)
		if !ok || templateArgs["enable_thinking"] != false {
			t.Fatalf("chat_template_kwargs = %#v", payload["chat_template_kwargs"])
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"i\":[[1,\"b\",\"oatmeal\",1,\"cup\"],[1,\"l\",\"chicken\",4,\"oz\"],[1,\"d\",\"salmon\",4,\"oz\"]]}"}}]}`))
	}))
	defer server.Close()

	provider := LlamaCPP{Client: server.Client()}
	got, err := provider.Complete(context.Background(), ProviderConfig{
		Type:      ProviderTypeLocalLlama,
		BaseURL:   server.URL + "/v1",
		Model:     "local-model",
		MaxTokens: 128,
	}, compactMealPlanRequest([]Message{{Role: "user", Content: "Plan."}}))
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if !strings.Contains(got, `"i"`) {
		t.Fatalf("content = %q", got)
	}
}

func TestAnthropicRequestAndResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %q, want /v1/messages", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "sk-anthropic" {
			t.Fatalf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got == "" {
			t.Fatal("missing anthropic-version header")
		}
		var payload map[string]any
		decodeJSON(t, readAll(t, r), &payload)
		if payload["model"] != "claude-test" {
			t.Fatalf("model = %v", payload["model"])
		}
		if !strings.Contains(fmt.Sprint(payload["system"]), "Return JSON") {
			t.Fatalf("system = %v", payload["system"])
		}
		if _, ok := payload["messages"].([]any); !ok {
			t.Fatalf("messages = %#v", payload["messages"])
		}
		outputConfig, ok := payload["output_config"].(map[string]any)
		if !ok {
			t.Fatalf("output_config = %#v", payload["output_config"])
		}
		format, ok := outputConfig["format"].(map[string]any)
		if !ok || format["type"] != "json_schema" {
			t.Fatalf("output_config.format = %#v", outputConfig["format"])
		}
		schema, ok := format["schema"].(map[string]any)
		if !ok || schema["additionalProperties"] != false {
			t.Fatalf("schema = %#v", format["schema"])
		}
		days := schema["properties"].(map[string]any)["days"].(map[string]any)
		day := days["items"].(map[string]any)
		dayValue := day["properties"].(map[string]any)["day"].(map[string]any)
		if _, ok := dayValue["minimum"]; ok {
			t.Fatalf("portable Anthropic schema included unsupported minimum: %#v", dayValue)
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"{\"schema_version\":\"0.1\"}"}]}`))
	}))
	defer server.Close()

	provider := Anthropic{Client: server.Client(), BaseURL: server.URL}
	got, err := provider.Complete(context.Background(), ProviderConfig{
		Type:   ProviderTypeAnthropic,
		Model:  "claude-test",
		APIKey: "sk-anthropic",
	}, mealPlanRequest([]Message{{Role: "system", Content: "Return JSON."}, {Role: "user", Content: "Plan."}}))
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if got != `{"schema_version":"0.1"}` {
		t.Fatalf("content = %q", got)
	}
}

func TestGeminiRequestAndResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-test:generateContent" {
			t.Fatalf("path = %q, want Gemini generateContent path", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "sk-gemini" {
			t.Fatalf("x-goog-api-key = %q", got)
		}
		var payload map[string]any
		decodeJSON(t, readAll(t, r), &payload)
		if _, ok := payload["systemInstruction"].(map[string]any); !ok {
			t.Fatalf("systemInstruction = %#v", payload["systemInstruction"])
		}
		config, ok := payload["generationConfig"].(map[string]any)
		if !ok {
			t.Fatalf("generationConfig = %#v", payload["generationConfig"])
		}
		if config["responseMimeType"] != "application/json" {
			t.Fatalf("responseMimeType = %#v", config["responseMimeType"])
		}
		if _, ok := config["responseFormat"]; ok {
			t.Fatalf("Gemini payload should not use responseFormat after live API rejection: %#v", config["responseFormat"])
		}
		if _, ok := config["responseSchema"]; ok {
			t.Fatalf("Gemini payload should not use responseSchema after live API rejection: %#v", config["responseSchema"])
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"schema_version\":\"0.1\"}"}]}}]}`))
	}))
	defer server.Close()

	provider := Gemini{Client: server.Client(), BaseURL: server.URL}
	got, err := provider.Complete(context.Background(), ProviderConfig{
		Type:   ProviderTypeGemini,
		Model:  "gemini-test",
		APIKey: "sk-gemini",
	}, mealPlanRequest([]Message{{Role: "system", Content: "Return JSON."}, {Role: "user", Content: "Plan."}}))
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if got != `{"schema_version":"0.1"}` {
		t.Fatalf("content = %q", got)
	}
}

func TestProviderHTTPErrorMessagesAreActionableAndRedacted(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		provider   Completer
		config     ProviderConfig
		want       []string
		wantAbsent []string
	}{
		{
			name:   "openai quota",
			status: http.StatusTooManyRequests,
			body:   `{"error":{"message":"Quota exceeded for sk-openai","type":"rate_limit_error","code":"rate_limit_exceeded"}}`,
			provider: OpenAI{
				BaseURL: "REPLACED",
			},
			config: ProviderConfig{
				Type:   ProviderTypeOpenAI,
				Model:  "gpt-test",
				APIKey: "sk-openai",
			},
			want: []string{
				"OpenAI provider returned HTTP 429 Too Many Requests",
				"Quota exceeded for [redacted]",
				"rate_limit_error",
				"rate_limit_exceeded",
			},
			wantAbsent: []string{"sk-openai"},
		},
		{
			name:   "anthropic overloaded",
			status: 529,
			body:   `{"type":"error","error":{"type":"overloaded_error","message":"Anthropic is overloaded"}}`,
			provider: Anthropic{
				BaseURL: "REPLACED",
			},
			config: ProviderConfig{
				Type:   ProviderTypeAnthropic,
				Model:  "claude-test",
				APIKey: "sk-ant",
			},
			want: []string{
				"Anthropic provider returned HTTP 529",
				"Anthropic is overloaded",
				"overloaded_error",
			},
		},
		{
			name:   "gemini unavailable",
			status: http.StatusServiceUnavailable,
			body:   `{"error":{"code":503,"message":"The model is overloaded. Please try again later.","status":"UNAVAILABLE"}}`,
			provider: Gemini{
				BaseURL: "REPLACED",
			},
			config: ProviderConfig{
				Type:   ProviderTypeGemini,
				Model:  "gemini-test",
				APIKey: "sk-gemini",
			},
			want: []string{
				"Gemini provider returned HTTP 503 Service Unavailable",
				"The model is overloaded. Please try again later.",
				"UNAVAILABLE",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			switch provider := tt.provider.(type) {
			case OpenAI:
				provider.Client = server.Client()
				provider.BaseURL = server.URL + "/v1"
				tt.provider = provider
			case Anthropic:
				provider.Client = server.Client()
				provider.BaseURL = server.URL
				tt.provider = provider
			case Gemini:
				provider.Client = server.Client()
				provider.BaseURL = server.URL
				tt.provider = provider
			}

			_, err := tt.provider.Complete(context.Background(), tt.config, mealPlanRequest([]Message{{Role: "user", Content: "Plan."}}))
			if err == nil {
				t.Fatal("Complete error = nil, want provider error")
			}
			got := err.Error()
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("error %q does not contain %q", got, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Fatalf("error %q contains secret %q", got, absent)
				}
			}
		})
	}
}

func TestProviderRequestErrorRedactsKey(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial failed with sk-secret")
	})}
	provider := OpenAI{Client: client, BaseURL: "https://example.invalid/v1"}

	_, err := provider.Complete(context.Background(), ProviderConfig{
		Type:   ProviderTypeOpenAI,
		Model:  "gpt-test",
		APIKey: "sk-secret",
	}, mealPlanRequest([]Message{{Role: "user", Content: "Plan."}}))
	if err == nil {
		t.Fatal("Complete error = nil, want request error")
	}
	if strings.Contains(err.Error(), "sk-secret") {
		t.Fatalf("error %q contains secret", err.Error())
	}
	if !strings.Contains(err.Error(), "OpenAI provider request failed") {
		t.Fatalf("error %q does not identify provider request failure", err.Error())
	}
}
