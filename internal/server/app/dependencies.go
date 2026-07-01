package app

import (
	"context"
	"encoding/json"
	"time"

	llm "github.com/chranama/MealCheck/internal/llm/external"
	localmodel "github.com/chranama/MealCheck/internal/llm/local"
	"github.com/chranama/MealCheck/internal/server/access"
	"github.com/chranama/MealCheck/internal/workflow/checker"
	"github.com/chranama/MealCheck/internal/workflow/normalize"
)

type PreparedRun = normalize.PreparedRun
type NormalizedPlanReviewArtifact = normalize.NormalizedPlanReviewArtifact
type qualificationRejectionError = localmodel.QualificationRejectionError
type localModelInputContractError = localmodel.LocalModelInputContractError

func DefaultProviderFactory(config ProviderConfig) (Provider, error) {
	return llm.DefaultProviderFactory(config)
}

func NewPolicyLimiter() *PolicyLimiter {
	return access.NewPolicyLimiter()
}

func validateSettings(settings checker.Settings) error {
	return checker.ValidateSettings(settings)
}

func QualifyMealPlanText(ctx context.Context, providerFactory ProviderFactory, request MealPlanQualificationRequest) (MealPlanQualificationResult, error) {
	return normalize.QualifyMealPlanText(ctx, providerFactory, request)
}

func PrepareRunInput(ctx context.Context, config Config, providerFactory ProviderFactory, run Run, input PendingRunInput) (PreparedRun, error) {
	return normalize.PrepareRunInput(ctx, config, providerFactory, run, input)
}

func writeOptionalArtifacts(outDir string, prepared PreparedRun) error {
	return normalize.WriteOptionalArtifacts(outDir, prepared)
}

func writeReviewArtifacts(outDir string, runID string, prepared PreparedRun) error {
	return normalize.WriteReviewArtifacts(outDir, runID, prepared)
}

func updateManifestArtifacts(outDir string, paths ...string) error {
	return normalize.UpdateManifestArtifacts(outDir, paths...)
}

func runtimeCasePath(config Config, runID string) string {
	return normalize.RuntimeCasePath(config, runID)
}

func normalizeLocalModelSettings(settings checker.Settings) checker.Settings {
	return localmodel.NormalizeLocalModelSettings(settings)
}

func validateLocalModelMealPlanPreflight(config Config, text string) error {
	return localmodel.ValidateLocalModelMealPlanPreflight(config, text)
}

func validatePublicProviderPolicy(config Config, provider ProviderConfig) error {
	return access.ValidatePublicProviderPolicy(config, provider)
}

func ParseInviteToken(value string) (id string, secret string, ok bool) {
	return access.ParseInviteToken(value)
}

func ValidateInviteToken(invite InviteToken, secret string, now time.Time) error {
	return access.ValidateInviteToken(invite, secret, now)
}

func clientIP(raddr string, headers map[string]string) string {
	return access.ClientIP(raddr, headers)
}

func retryAfterHeader(duration time.Duration) string {
	return access.RetryAfterHeader(duration)
}

func sanitizeProviderErrorText(message, apiKey string) string {
	return llm.SanitizeProviderErrorText(message, apiKey)
}

func jsonRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func defaultPendingInputTTL(queueSize int, runTimeout time.Duration) time.Duration {
	if queueSize < 1 {
		queueSize = 1
	}
	if runTimeout <= 0 {
		runTimeout = 10 * time.Minute
	}
	ttl := time.Duration(queueSize+1) * runTimeout
	if ttl < 15*time.Minute {
		return 15 * time.Minute
	}
	return ttl
}
