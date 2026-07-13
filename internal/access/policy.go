package access

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/llm/inference"
)

type PolicyLimiter struct {
	mu       sync.Mutex
	requests map[string]requestBucket
	runs     map[string]dailyRunBucket
}

type requestBucket struct {
	WindowStart time.Time
	Count       int
}

type dailyRunBucket struct {
	Day   string
	Count int
}

type PolicyError struct {
	Status     int
	Code       string
	Message    string
	RetryAfter time.Duration
	Details    map[string]any
}

func (e PolicyError) Error() string {
	return e.Message
}

func NewPolicyLimiter() *PolicyLimiter {
	return &PolicyLimiter{
		requests: map[string]requestBucket{},
		runs:     map[string]dailyRunBucket{},
	}
}

func (p *PolicyLimiter) AllowRequest(ip string, now time.Time, limit int, window time.Duration) error {
	if p == nil || limit <= 0 {
		return nil
	}
	if window <= 0 {
		window = time.Minute
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	bucket := p.requests[ip]
	if bucket.WindowStart.IsZero() || !now.Before(bucket.WindowStart.Add(window)) {
		bucket = requestBucket{WindowStart: now}
	}
	if bucket.Count >= limit {
		retryAfter := bucket.WindowStart.Add(window).Sub(now)
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return PolicyError{
			Status:     429,
			Code:       "rate_limited",
			Message:    "too many requests; retry later",
			RetryAfter: retryAfter,
			Details: map[string]any{
				"limit":               limit,
				"window_seconds":      int(window.Seconds()),
				"retry_after_seconds": int(retryAfter.Seconds()),
			},
		}
	}
	bucket.Count++
	p.requests[ip] = bucket
	return nil
}

func (p *PolicyLimiter) CheckDailyRunLimit(ip string, now time.Time, limit int) error {
	if p == nil || limit <= 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	day := now.UTC().Format("2006-01-02")
	bucket := p.runs[ip]
	if bucket.Day != day {
		bucket = dailyRunBucket{Day: day}
	}
	if bucket.Count >= limit {
		nextDay := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day()+1, 0, 0, 0, 0, time.UTC)
		retryAfter := nextDay.Sub(now.UTC())
		return PolicyError{
			Status:     429,
			Code:       "daily_run_limit_reached",
			Message:    "daily public run limit reached; retry tomorrow",
			RetryAfter: retryAfter,
			Details: map[string]any{
				"limit":               limit,
				"retry_after_seconds": int(retryAfter.Seconds()),
			},
		}
	}
	p.runs[ip] = bucket
	return nil
}

func (p *PolicyLimiter) RecordRun(ip string, now time.Time) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	day := now.UTC().Format("2006-01-02")
	bucket := p.runs[ip]
	if bucket.Day != day {
		bucket = dailyRunBucket{Day: day}
	}
	bucket.Count++
	p.runs[ip] = bucket
}

func validatePublicProviderPolicy(config core.Config, provider core.ProviderConfig) error {
	if provider.Type == inference.ProviderTypeLocalLlama {
		return fmt.Errorf("local_llama is a server-owned provider; use input_mode local_model")
	}
	if Mode(config) != core.AccessModePublicBYOK || provider.Type != inference.ProviderTypeOpenAICompatible {
		return nil
	}
	if !config.PublicOpenAICompatible {
		return fmt.Errorf("openai_compatible providers are disabled on the public hosted service; run MealCheck locally for custom endpoints")
	}
	return validatePublicOpenAICompatibleBaseURL(provider.BaseURL)
}

func ValidatePublicProviderPolicy(config core.Config, provider core.ProviderConfig) error {
	return validatePublicProviderPolicy(config, provider)
}

func Mode(config core.Config) string {
	switch config.AccessMode {
	case core.AccessModePublicBYOK, core.AccessModeInviteRequired:
		return config.AccessMode
	case "":
		if config.InviteToken != "" || config.InviteRequired {
			return core.AccessModeInviteRequired
		}
		return core.AccessModePublicBYOK
	default:
		return core.AccessModeInviteRequired
	}
}

func validatePublicOpenAICompatibleBaseURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("openai_compatible base_url must be an absolute HTTPS URL")
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("openai_compatible base_url must use HTTPS")
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("openai_compatible base_url host is required")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return fmt.Errorf("openai_compatible base_url must use the default HTTPS port")
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return fmt.Errorf("openai_compatible base_url cannot target localhost")
	}
	if ip := net.ParseIP(host); ip != nil && !publicRoutableIP(ip) {
		return fmt.Errorf("openai_compatible base_url cannot target private or local IP addresses")
	}
	return nil
}

func publicRoutableIP(ip net.IP) bool {
	return !(ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast())
}
