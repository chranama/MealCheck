package localmodel

import (
	"path/filepath"
	"strings"

	llm "github.com/chranama/MealCheck/internal/llm/external"
)

func redactProvider(config ProviderConfig) RedactedProviderConfig {
	providerType := config.Type
	if providerType == "" {
		providerType = llm.ProviderTypeOpenAICompatible
	}
	if providerType == llm.ProviderTypeLocalLlama {
		return RedactedProviderConfig{
			Type:   providerType,
			Model:  filepath.Base(config.Model),
			APIKey: "not_applicable",
		}
	}
	return RedactedProviderConfig{
		Type:    providerType,
		BaseURL: config.BaseURL,
		Model:   config.Model,
		APIKey:  "redacted",
	}
}

func sanitizeDebugError(err error, apiKey string) string {
	if err == nil {
		return ""
	}
	return llm.SanitizeProviderErrorText(err.Error(), apiKey)
}

func sanitizeDebugArtifactText(text, apiKey string) string {
	if text == "" {
		return ""
	}
	if apiKey != "" {
		text = strings.ReplaceAll(text, apiKey, "[redacted]")
	}
	const maxDebugArtifactTextLength = 200_000
	if len(text) > maxDebugArtifactTextLength {
		return text[:maxDebugArtifactTextLength] + "\n[truncated]"
	}
	return text
}
