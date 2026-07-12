package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/chranama/MealCheck/internal/llm/inference"
)

func providerRequestError(providerType string, err error, apiKey string) error {
	message := sanitizeProviderErrorText(err.Error(), apiKey)
	if message == "" {
		message = "request failed before the provider returned a response"
	}
	return fmt.Errorf("%s provider request failed: %s", providerLabel(providerType), message)
}

func providerHTTPError(providerType string, statusCode int, body []byte, apiKey string) error {
	message := sanitizeProviderErrorText(extractProviderErrorMessage(body), apiKey)
	if message == "" {
		message = defaultProviderHTTPMessage(statusCode)
	}
	return HTTPError{
		Provider:   providerLabel(providerType),
		StatusCode: statusCode,
		Message:    message,
	}
}

func extractProviderErrorMessage(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return ""
	}

	var root map[string]any
	if err := json.Unmarshal(body, &root); err == nil {
		if message := errorMessageFromObject(root); message != "" {
			return message
		}
	}
	return string(body)
}

func errorMessageFromObject(root map[string]any) string {
	if errValue, ok := root["error"]; ok {
		switch errObj := errValue.(type) {
		case string:
			return errObj
		case map[string]any:
			return errorMessageFields(errObj)
		}
	}
	if message, ok := stringField(root, "message"); ok {
		return message
	}
	return ""
}

func errorMessageFields(errObj map[string]any) string {
	message, _ := stringField(errObj, "message")
	var labels []string
	for _, key := range []string{"status", "type", "code"} {
		if value, ok := stringLikeField(errObj, key); ok && value != "" {
			labels = append(labels, value)
		}
	}
	if message == "" {
		return strings.Join(labels, ", ")
	}
	if len(labels) == 0 {
		return message
	}
	return fmt.Sprintf("%s (%s)", message, strings.Join(labels, ", "))
}

func stringField(values map[string]any, key string) (string, bool) {
	value, ok := values[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}

func stringLikeField(values map[string]any, key string) (string, bool) {
	value, ok := values[key]
	if !ok {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed), strings.TrimSpace(typed) != ""
	case float64:
		return fmt.Sprintf("%.0f", typed), true
	default:
		return "", false
	}
}

func defaultProviderHTTPMessage(statusCode int) string {
	switch statusCode {
	case http.StatusUnauthorized:
		return "authentication failed; check the provider API key"
	case http.StatusForbidden:
		return "request was forbidden; check key permissions, project access, billing, or model availability"
	case http.StatusNotFound:
		return "model or endpoint was not found; check the selected provider and model"
	case http.StatusTooManyRequests:
		return "rate limit or quota was exceeded; retry later or check provider quota and billing"
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return "provider service is temporarily unavailable; retry later"
	}
	if statusCode >= 500 {
		return "provider service failed; retry later"
	}
	if statusCode >= 400 {
		return "provider rejected the request; check provider, model, and request settings"
	}
	return ""
}

func sanitizeProviderErrorText(message, apiKey string) string {
	return inference.SanitizeErrorText(message, apiKey)
}

func SanitizeProviderErrorText(message, apiKey string) string {
	return sanitizeProviderErrorText(message, apiKey)
}

func providerLabel(providerType string) string {
	switch providerType {
	case ProviderTypeOpenAI:
		return "OpenAI"
	case ProviderTypeAnthropic:
		return "Anthropic"
	case ProviderTypeGemini:
		return "Gemini"
	case ProviderTypeLocalLlama:
		return "local llama"
	case ProviderTypeOpenAICompatible, "":
		return "OpenAI-compatible"
	default:
		return providerType
	}
}
