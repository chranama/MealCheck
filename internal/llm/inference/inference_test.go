package inference

import (
	"strings"
	"testing"
)

func TestValidateProviderConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  ProviderConfig
		wantErr string
	}{
		{
			name:   "local llama does not require an API key",
			config: ProviderConfig{Type: ProviderTypeLocalLlama, Model: "local-model"},
		},
		{
			name:    "remote provider requires an API key",
			config:  ProviderConfig{Type: ProviderTypeOpenAI, Model: "gpt-test"},
			wantErr: "provider api_key is required",
		},
		{
			name:    "model is required",
			config:  ProviderConfig{Type: ProviderTypeAnthropic, APIKey: "secret"},
			wantErr: "provider model is required",
		},
		{
			name:    "provider type is validated",
			config:  ProviderConfig{Type: "unknown", Model: "model", APIKey: "secret"},
			wantErr: `unsupported provider type "unknown"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProviderConfig(tt.config)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateProviderConfig error: %v", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("ValidateProviderConfig error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeErrorTextRedactsAndBoundsProviderErrors(t *testing.T) {
	const apiKey = "super-secret"
	message := "  request failed with\n" + apiKey + " " + strings.Repeat("x", 800)
	got := SanitizeErrorText(message, apiKey)

	if strings.Contains(got, apiKey) {
		t.Fatalf("sanitized error contains API key: %q", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("sanitized error does not contain redaction marker: %q", got)
	}
	if len(got) != 703 || !strings.HasSuffix(got, "...") {
		t.Fatalf("sanitized error length/suffix = %d/%q, want 703/ellipsis", len(got), got[len(got)-3:])
	}
}
