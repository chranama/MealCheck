// Package submission validates hosted run requests and creates queued runs.
package submission

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/access"
	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/llm/inference"
	"github.com/chranama/MealCheck/internal/llm/planextract"
	"github.com/chranama/MealCheck/internal/runs/runinput"
	"github.com/chranama/MealCheck/internal/state"
	"github.com/chranama/MealCheck/internal/workflow/checker"
	"github.com/chranama/MealCheck/internal/workflow/normalize"
)

type Service struct {
	Config core.Config
	Store  state.Store
	Inputs *runinput.Vault
	Policy *access.PolicyLimiter
}

func (s Service) Submit(ctx context.Context, request core.CreateRunRequest, inviteTokenID, clientIP string, now time.Time) (core.Run, error) {
	casePath, pendingInput, hasPending, err := PrepareInput(s.Config, request)
	if err != nil {
		return core.Run{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if access.Mode(s.Config) == core.AccessModePublicBYOK {
		if err := s.Policy.CheckDailyRunLimit(clientIP, now, s.Config.PublicDailyRunLimit); err != nil {
			return core.Run{}, err
		}
	}

	run := NewRun(s.Config, casePath, now)
	if hasPending {
		run.InputMode = pendingInput.Mode
		run.CasePath = RuntimeCasePath(s.Config, run.ID)
		s.Inputs.Put(run.ID, pendingInput, PendingInputExpiresAt(s.Config, run))
	}
	if err := s.Store.CreateRun(ctx, run, s.Config.QueueSize, inviteTokenID); err != nil {
		if hasPending {
			s.Inputs.Delete(run.ID)
		}
		return core.Run{}, err
	}
	if access.Mode(s.Config) == core.AccessModePublicBYOK {
		s.Policy.RecordRun(clientIP, now)
	}
	if err := s.Store.AppendEvent(ctx, run.ID, core.EventQueued, "run queued", now); err != nil {
		if hasPending {
			s.Inputs.Delete(run.ID)
		}
		return core.Run{}, err
	}
	return run, nil
}

func PrepareInput(config core.Config, request core.CreateRunRequest) (string, core.PendingRunInput, bool, error) {
	inputMode := strings.TrimSpace(request.InputMode)
	if inputMode == "" {
		if hasDynamicRunFields(request) {
			return "", core.PendingRunInput{}, false, fmt.Errorf("input_mode is required for hosted meal-plan input")
		}
		casePath, err := CleanCasePath(config.Root, request.CasePath)
		if err != nil {
			return "", core.PendingRunInput{}, false, err
		}
		return casePath, core.PendingRunInput{}, false, nil
	}
	if strings.TrimSpace(request.CasePath) != "" {
		return "", core.PendingRunInput{}, false, fmt.Errorf("case_path cannot be combined with input_mode")
	}

	repairJSON := inputMode == core.InputModeProfileGeneration || inputMode == core.InputModePromptGeneration
	if request.RepairJSON != nil {
		repairJSON = *request.RepairJSON
	}
	pendingInput := core.PendingRunInput{
		Mode:             inputMode,
		Settings:         request.Settings,
		CandidatePlan:    request.CandidatePlan,
		GenerationPrompt: strings.TrimSpace(request.GenerationPrompt),
		CandidateText:    strings.TrimSpace(request.CandidateText),
		Provider:         NormalizeProviderConfig(request.Provider),
		RepairJSON:       repairJSON,
	}
	if err := checker.ValidateSettings(pendingInput.Settings); err != nil {
		return "", core.PendingRunInput{}, false, err
	}
	if err := ValidateTextLength("generation_prompt", pendingInput.GenerationPrompt, config.MaxGenerationPromptChars); err != nil {
		return "", core.PendingRunInput{}, false, err
	}

	switch inputMode {
	case core.InputModeManualStructured:
		return "", core.PendingRunInput{}, false, fmt.Errorf("manual_structured is supported only by the local CLI/debug workflow; hosted live runs require model-backed generation")
	case core.InputModeProfileGeneration:
		if HostedMode(config) == core.HostedModeLocalModel {
			return "", core.PendingRunInput{}, false, fmt.Errorf("profile_generation is disabled in hosted local model mode; use local_model candidate_text input")
		}
		if err := ValidateProviderConfig(pendingInput.Provider); err != nil {
			return "", core.PendingRunInput{}, false, err
		}
		if err := access.ValidatePublicProviderPolicy(config, pendingInput.Provider); err != nil {
			return "", core.PendingRunInput{}, false, err
		}
	case core.InputModePromptGeneration:
		if HostedMode(config) == core.HostedModeLocalModel {
			return "", core.PendingRunInput{}, false, fmt.Errorf("prompt_generation is disabled in hosted local model mode; use local_model candidate_text input")
		}
		if pendingInput.GenerationPrompt == "" {
			return "", core.PendingRunInput{}, false, fmt.Errorf("generation_prompt is required for prompt_generation")
		}
		if err := ValidateProviderConfig(pendingInput.Provider); err != nil {
			return "", core.PendingRunInput{}, false, err
		}
		if err := access.ValidatePublicProviderPolicy(config, pendingInput.Provider); err != nil {
			return "", core.PendingRunInput{}, false, err
		}
	case core.InputModeLocalModel:
		pendingInput.Settings = planextract.NormalizeLocalModelSettings(pendingInput.Settings)
		if err := checker.ValidateSettings(pendingInput.Settings); err != nil {
			return "", core.PendingRunInput{}, false, err
		}
		if err := ValidateLocalModelAvailable(config); err != nil {
			return "", core.PendingRunInput{}, false, err
		}
		if HasProviderConfigFields(request.Provider) {
			return "", core.PendingRunInput{}, false, fmt.Errorf("local_model uses the server-owned local model; omit provider")
		}
		if pendingInput.CandidateText == "" {
			return "", core.PendingRunInput{}, false, fmt.Errorf("candidate_text is required for local_model")
		}
		if err := ValidateTextLength("candidate_text", pendingInput.CandidateText, config.LocalModelMaxInputChars); err != nil {
			return "", core.PendingRunInput{}, false, err
		}
		if err := ValidateLocalModelSettings(pendingInput.Settings); err != nil {
			return "", core.PendingRunInput{}, false, err
		}
		if err := planextract.ValidateLocalModelMealPlanPreflight(config, pendingInput.CandidateText); err != nil {
			return "", core.PendingRunInput{}, false, err
		}
		pendingInput.Provider = LocalModelProviderConfig(config)
		pendingInput.RepairJSON = false
	default:
		return "", core.PendingRunInput{}, false, fmt.Errorf("unsupported input_mode %q", inputMode)
	}
	return "", pendingInput, true, nil
}

func PendingInputExpiresAt(config core.Config, run core.Run) time.Time {
	ttl := config.PendingInputTTL
	if ttl == 0 {
		ttl = defaultPendingInputTTL(config.QueueSize, config.RunTimeout)
	}
	expiresAt := run.CreatedAt.Add(ttl)
	if !run.ExpiresAt.IsZero() && run.ExpiresAt.Before(expiresAt) {
		return run.ExpiresAt
	}
	return expiresAt
}

func NewRun(config core.Config, casePath string, now time.Time) core.Run {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	id := "run_" + newID()
	return core.Run{
		ID:          id,
		CasePath:    casePath,
		Status:      core.StatusQueued,
		ArtifactDir: filepath.Join(config.ArtifactDir, id),
		CreatedAt:   now,
		UpdatedAt:   now,
		ExpiresAt:   now.Add(config.Retention),
	}
}

func CleanCasePath(root, casePath string) (string, error) {
	if casePath == "" {
		return "", fmt.Errorf("case_path is required")
	}
	if filepath.IsAbs(casePath) {
		return "", fmt.Errorf("case_path must be relative")
	}
	cleaned := filepath.Clean(casePath)
	if cleaned == "." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return "", fmt.Errorf("case_path must stay inside the repository")
	}
	if !strings.HasPrefix(cleaned, "examples"+string(filepath.Separator)) {
		return "", fmt.Errorf("case_path must reference a checked-in example for Milestone 4")
	}
	if _, err := os.Stat(filepath.Join(root, cleaned)); err != nil {
		return "", err
	}
	return cleaned, nil
}

func RuntimeCasePath(config core.Config, runID string) string {
	return normalize.RuntimeCasePath(config, runID)
}

func HasProviderConfigFields(config core.ProviderConfig) bool {
	return config.Type != "" || config.BaseURL != "" || config.Model != "" || config.APIKey != "" || config.MaxTokens != 0
}

func NormalizeProviderConfig(config core.ProviderConfig) core.ProviderConfig {
	providerType := strings.TrimSpace(config.Type)
	if providerType == "" {
		providerType = inference.ProviderTypeOpenAICompatible
	}
	normalized := core.ProviderConfig{
		Type: providerType, BaseURL: strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		Model: strings.TrimSpace(config.Model), APIKey: strings.TrimSpace(config.APIKey),
		MaxTokens: config.MaxTokens, Timeout: config.Timeout,
	}
	if providerType != inference.ProviderTypeOpenAICompatible && providerType != inference.ProviderTypeLocalLlama {
		normalized.BaseURL = ""
	}
	return normalized
}

func ValidateProviderConfig(config core.ProviderConfig) error {
	switch config.Type {
	case inference.ProviderTypeOpenAICompatible, inference.ProviderTypeOpenAI, inference.ProviderTypeAnthropic, inference.ProviderTypeGemini, inference.ProviderTypeLocalLlama:
	default:
		return fmt.Errorf("unsupported provider type %q", config.Type)
	}
	if config.Model == "" {
		return fmt.Errorf("provider model is required")
	}
	if config.Type != inference.ProviderTypeLocalLlama && config.APIKey == "" {
		return fmt.Errorf("provider api_key is required")
	}
	return nil
}

func ValidateTextLength(field, value string, limit int) error {
	if limit > 0 && value != "" && len([]rune(value)) > limit {
		return fmt.Errorf("%s exceeds maximum length of %d characters", field, limit)
	}
	return nil
}

func IsProviderConfigError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.HasPrefix(message, "unsupported provider type ") || message == "provider model is required" ||
		message == "provider base_url is required" || message == "provider api_key is required"
}

func HostedMode(config core.Config) string {
	if config.HostedMode == core.HostedModeBYOK || config.HostedMode == core.HostedModeLocalModel {
		return config.HostedMode
	}
	return core.HostedModeBYOK
}

func LocalModelProviderConfig(config core.Config) core.ProviderConfig {
	return core.ProviderConfig{
		Type: inference.ProviderTypeLocalLlama, BaseURL: strings.TrimRight(strings.TrimSpace(config.LocalModelBaseURL), "/"),
		Model: strings.TrimSpace(config.LocalModelName), MaxTokens: config.LocalModelMaxOutputTokens, Timeout: config.LocalModelTimeout,
	}
}

func ValidateLocalModelAvailable(config core.Config) error {
	if !config.LocalModelEnabled {
		return fmt.Errorf("local model provider is not enabled")
	}
	provider := LocalModelProviderConfig(config)
	if provider.BaseURL == "" {
		return fmt.Errorf("local model base URL is not configured")
	}
	if provider.Model == "" {
		return fmt.Errorf("local model name is not configured")
	}
	return nil
}

func IsLocalModelAvailabilityError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return message == "local model provider is not enabled" || message == "local model base URL is not configured" || message == "local model name is not configured"
}

func ValidateLocalModelSettings(settings checker.Settings) error {
	if settings.VerificationConstraints.Days != 1 {
		return fmt.Errorf("hosted local_model requires settings verification_constraints days to be exactly 1")
	}
	return nil
}

func hasDynamicRunFields(request core.CreateRunRequest) bool {
	return request.CandidatePlan != nil || request.CandidateText != "" || hasSettingsFields(request.Settings) ||
		request.GenerationPrompt != "" || HasProviderConfigFields(request.Provider) || request.RepairJSON != nil
}

func hasSettingsFields(settings checker.Settings) bool {
	targets := settings.NutritionTargets
	constraints := settings.VerificationConstraints
	return targets.CalorieTargetKcal != 0 || targets.ProteinTargetG != 0 || constraints.Days != 0 ||
		constraints.MealsPerDay != 0 || len(constraints.Allergies) > 0 || len(constraints.ExcludedFoods) > 0 ||
		constraints.MaxSodiumMGPerDay != 0 || constraints.MaxAddedSugarGPerMeal != 0 ||
		constraints.MaxSaturatedFatPctCalories != 0 || constraints.CalorieTolerancePct != 0 || constraints.RequiresPrepSafetyNotes
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

func newID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
