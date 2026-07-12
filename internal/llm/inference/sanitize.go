package inference

import "strings"

func SanitizeErrorText(message, apiKey string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	message = strings.Join(strings.Fields(message), " ")
	if apiKey != "" {
		message = strings.ReplaceAll(message, apiKey, "[redacted]")
	}
	const maxErrorLength = 700
	if len(message) > maxErrorLength {
		return message[:maxErrorLength] + "..."
	}
	return message
}
