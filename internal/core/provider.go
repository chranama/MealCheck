package core

import "time"

type ProviderConfig struct {
	Type      string        `json:"type,omitempty"`
	BaseURL   string        `json:"base_url,omitempty"`
	Model     string        `json:"model,omitempty"`
	APIKey    string        `json:"api_key,omitempty"`
	MaxTokens int           `json:"max_tokens,omitempty"`
	Timeout   time.Duration `json:"-"`
}

type RedactedProviderConfig struct {
	Type    string `json:"type"`
	BaseURL string `json:"base_url,omitempty"`
	Model   string `json:"model,omitempty"`
	APIKey  string `json:"api_key"`
}
