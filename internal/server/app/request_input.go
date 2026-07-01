package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

func requestRunInput(config Config, request CreateRunRequest) (string, PendingRunInput, bool, error) {
	inputMode := strings.TrimSpace(request.InputMode)
	if inputMode == "" {
		if hasDynamicRunFields(request) {
			return "", PendingRunInput{}, false, fmt.Errorf("input_mode is required for hosted meal-plan input")
		}
		casePath, err := cleanCasePath(config.Root, request.CasePath)
		if err != nil {
			return "", PendingRunInput{}, false, err
		}
		return casePath, PendingRunInput{}, false, nil
	}
	if strings.TrimSpace(request.CasePath) != "" {
		return "", PendingRunInput{}, false, fmt.Errorf("case_path cannot be combined with input_mode")
	}

	repairJSON := inputMode == InputModeProfileGeneration || inputMode == InputModePromptGeneration
	if request.RepairJSON != nil {
		repairJSON = *request.RepairJSON
	}
	pendingInput := PendingRunInput{
		Mode:             inputMode,
		Settings:         request.Settings,
		CandidatePlan:    request.CandidatePlan,
		GenerationPrompt: strings.TrimSpace(request.GenerationPrompt),
		CandidateText:    strings.TrimSpace(request.CandidateText),
		Provider:         normalizeProviderConfig(request.Provider),
		RepairJSON:       repairJSON,
	}
	if err := validateSettings(pendingInput.Settings); err != nil {
		return "", PendingRunInput{}, false, err
	}
	if err := validateTextLength("generation_prompt", pendingInput.GenerationPrompt, config.MaxGenerationPromptChars); err != nil {
		return "", PendingRunInput{}, false, err
	}

	switch inputMode {
	case InputModeManualStructured:
		return "", PendingRunInput{}, false, fmt.Errorf("manual_structured is supported only by the local CLI/debug workflow; hosted live runs require model-backed generation")
	case InputModeProfileGeneration:
		if hostedMode(config) == HostedModeLocalModel {
			return "", PendingRunInput{}, false, fmt.Errorf("profile_generation is disabled in hosted local model mode; use local_model candidate_text input")
		}
		if err := validateProviderConfig(pendingInput.Provider); err != nil {
			return "", PendingRunInput{}, false, err
		}
		if err := validatePublicProviderPolicy(config, pendingInput.Provider); err != nil {
			return "", PendingRunInput{}, false, err
		}
	case InputModePromptGeneration:
		if hostedMode(config) == HostedModeLocalModel {
			return "", PendingRunInput{}, false, fmt.Errorf("prompt_generation is disabled in hosted local model mode; use local_model candidate_text input")
		}
		if pendingInput.GenerationPrompt == "" {
			return "", PendingRunInput{}, false, fmt.Errorf("generation_prompt is required for prompt_generation")
		}
		if err := validateProviderConfig(pendingInput.Provider); err != nil {
			return "", PendingRunInput{}, false, err
		}
		if err := validatePublicProviderPolicy(config, pendingInput.Provider); err != nil {
			return "", PendingRunInput{}, false, err
		}
	case InputModeLocalModel:
		pendingInput.Settings = normalizeLocalModelSettings(pendingInput.Settings)
		if err := validateSettings(pendingInput.Settings); err != nil {
			return "", PendingRunInput{}, false, err
		}
		if err := validateLocalModelAvailable(config); err != nil {
			return "", PendingRunInput{}, false, err
		}
		if hasProviderConfigFields(request.Provider) {
			return "", PendingRunInput{}, false, fmt.Errorf("local_model uses the server-owned local model; omit provider")
		}
		if pendingInput.CandidateText == "" {
			return "", PendingRunInput{}, false, fmt.Errorf("candidate_text is required for local_model")
		}
		if err := validateTextLength("candidate_text", pendingInput.CandidateText, config.LocalModelMaxInputChars); err != nil {
			return "", PendingRunInput{}, false, err
		}
		if err := validateLocalModelSettings(pendingInput.Settings); err != nil {
			return "", PendingRunInput{}, false, err
		}
		if err := validateLocalModelMealPlanPreflight(config, pendingInput.CandidateText); err != nil {
			return "", PendingRunInput{}, false, err
		}
		pendingInput.Provider = localModelProviderConfig(config)
		pendingInput.RepairJSON = false
	default:
		return "", PendingRunInput{}, false, fmt.Errorf("unsupported input_mode %q", inputMode)
	}
	return "", pendingInput, true, nil
}

func RequestRunInput(config Config, request CreateRunRequest) (string, PendingRunInput, bool, error) {
	return requestRunInput(config, request)
}

func pendingInputExpiresAt(config Config, run Run) time.Time {
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
func hasDynamicRunFields(request CreateRunRequest) bool {
	return request.CandidatePlan != nil ||
		request.CandidateText != "" ||
		hasSettingsFields(request.Settings) ||
		request.GenerationPrompt != "" ||
		request.Provider.Type != "" ||
		request.Provider.BaseURL != "" ||
		request.Provider.Model != "" ||
		request.Provider.APIKey != "" ||
		request.RepairJSON != nil
}
func hasSettingsFields(settings checker.Settings) bool {
	targets := settings.NutritionTargets
	constraints := settings.VerificationConstraints
	return targets.CalorieTargetKcal != 0 ||
		targets.ProteinTargetG != 0 ||
		constraints.Days != 0 ||
		constraints.MealsPerDay != 0 ||
		len(constraints.Allergies) > 0 ||
		len(constraints.ExcludedFoods) > 0 ||
		constraints.MaxSodiumMGPerDay != 0 ||
		constraints.MaxAddedSugarGPerMeal != 0 ||
		constraints.MaxSaturatedFatPctCalories != 0 ||
		constraints.CalorieTolerancePct != 0 ||
		constraints.RequiresPrepSafetyNotes
}
func hasProviderConfigFields(config ProviderConfig) bool {
	return config.Type != "" ||
		config.BaseURL != "" ||
		config.Model != "" ||
		config.APIKey != "" ||
		config.MaxTokens != 0
}
func normalizeProviderConfig(config ProviderConfig) ProviderConfig {
	providerType := strings.TrimSpace(config.Type)
	if providerType == "" {
		providerType = ProviderTypeOpenAICompatible
	}
	normalized := ProviderConfig{
		Type:      providerType,
		BaseURL:   strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		Model:     strings.TrimSpace(config.Model),
		APIKey:    strings.TrimSpace(config.APIKey),
		MaxTokens: config.MaxTokens,
		Timeout:   config.Timeout,
	}
	if providerType != ProviderTypeOpenAICompatible && providerType != ProviderTypeLocalLlama {
		normalized.BaseURL = ""
	}
	return normalized
}
func validateProviderConfig(config ProviderConfig) error {
	switch config.Type {
	case ProviderTypeOpenAICompatible, ProviderTypeOpenAI, ProviderTypeAnthropic, ProviderTypeGemini, ProviderTypeLocalLlama:
	default:
		return fmt.Errorf("unsupported provider type %q", config.Type)
	}
	if config.Model == "" {
		return fmt.Errorf("provider model is required")
	}
	if config.Type == ProviderTypeLocalLlama {
		return nil
	}
	if config.APIKey == "" {
		return fmt.Errorf("provider api_key is required")
	}
	return nil
}
func validateTextLength(field, value string, limit int) error {
	if limit <= 0 || value == "" {
		return nil
	}
	if len([]rune(value)) > limit {
		return fmt.Errorf("%s exceeds maximum length of %d characters", field, limit)
	}
	return nil
}
func isProviderConfigError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.HasPrefix(message, "unsupported provider type ") ||
		message == "provider model is required" ||
		message == "provider base_url is required" ||
		message == "provider api_key is required"
}
func hostedMode(config Config) string {
	switch config.HostedMode {
	case HostedModeBYOK, HostedModeLocalModel:
		return config.HostedMode
	default:
		return HostedModeBYOK
	}
}
func accessMode(config Config) string {
	switch config.AccessMode {
	case AccessModePublicBYOK, AccessModeInviteRequired:
		return config.AccessMode
	default:
		if config.InviteToken != "" || config.InviteRequired {
			return AccessModeInviteRequired
		}
		return AccessModePublicBYOK
	}
}
func localModelProviderConfig(config Config) ProviderConfig {
	return ProviderConfig{
		Type:      ProviderTypeLocalLlama,
		BaseURL:   strings.TrimRight(strings.TrimSpace(config.LocalModelBaseURL), "/"),
		Model:     strings.TrimSpace(config.LocalModelName),
		MaxTokens: config.LocalModelMaxOutputTokens,
		Timeout:   config.LocalModelTimeout,
	}
}
func validateLocalModelAvailable(config Config) error {
	if !config.LocalModelEnabled {
		return fmt.Errorf("local model provider is not enabled")
	}
	provider := localModelProviderConfig(config)
	if provider.BaseURL == "" {
		return fmt.Errorf("local model base URL is not configured")
	}
	if provider.Model == "" {
		return fmt.Errorf("local model name is not configured")
	}
	return nil
}
func isLocalModelAvailabilityError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return message == "local model provider is not enabled" ||
		message == "local model base URL is not configured" ||
		message == "local model name is not configured"
}
func validateLocalModelSettings(settings checker.Settings) error {
	if settings.VerificationConstraints.Days != 1 {
		return fmt.Errorf("hosted local_model requires settings verification_constraints days to be exactly 1")
	}
	return nil
}
