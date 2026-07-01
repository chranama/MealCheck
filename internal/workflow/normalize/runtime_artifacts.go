package normalize

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

func writeRuntimeCase(config Config, run Run, input PendingRunInput, plan checker.Plan) (string, error) {
	inputDir := runtimeInputDir(config, run.ID)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		return "", err
	}
	planPath := filepath.Join(inputDir, "candidate-plan.json")
	casePath := runtimeCasePath(config, run.ID)
	if err := writeJSONFile(planPath, plan); err != nil {
		return "", err
	}
	c := checker.Case{
		SchemaVersion:       "0.1",
		CaseID:              run.ID,
		InputMode:           input.Mode,
		Settings:            input.Settings,
		GuidelinePackID:     defaultGuidelinePackID,
		GuidelinePackPath:   defaultGuidelinePackPath,
		NutrientCatalogID:   defaultNutrientCatalogID,
		NutrientCatalogPath: defaultNutrientCatalogPath,
		CandidatePlan:       planPath,
		Expectations:        checker.Expectations{},
		Tags:                []string{"hosted", input.Mode},
	}
	if input.GenerationPrompt != "" {
		c.GenerationPrompt = input.GenerationPrompt
	}
	if input.CandidateText != "" {
		c.GenerationPrompt = "[local_model candidate_text omitted from runtime case]"
	}
	if err := writeJSONFile(casePath, c); err != nil {
		return "", err
	}
	return casePath, nil
}
func runtimeInputDir(config Config, runID string) string {
	return filepath.Join(config.DataDir, "run-inputs", runID)
}
func runtimeCasePath(config Config, runID string) string {
	return filepath.Join(runtimeInputDir(config, runID), "case.json")
}

func RuntimeCasePath(config Config, runID string) string {
	return runtimeCasePath(config, runID)
}

func writeOptionalArtifacts(outDir string, prepared PreparedRun) error {
	if prepared.UsedProvider {
		if err := writeJSONFile(filepath.Join(outDir, "optional", "llm-output.json"), map[string]any{"output": prepared.LLMOutput}); err != nil {
			return err
		}
	}
	if len(prepared.NormalizationEvents) > 0 {
		if err := writeJSONFile(filepath.Join(outDir, "optional", "normalization-events.json"), prepared.NormalizationEvents); err != nil {
			return err
		}
	}
	if prepared.LocalModelExtraction != nil {
		if err := writeJSONFile(filepath.Join(outDir, "optional", "local-model-chunks.json"), prepared.LocalModelExtraction); err != nil {
			return err
		}
	}
	if prepared.UsedProvider {
		if err := writeJSONFile(filepath.Join(outDir, "configs", "redacted-provider.json"), prepared.RedactedProvider); err != nil {
			return err
		}
	}
	if prepared.UsedProvider || len(prepared.NormalizationEvents) > 0 || prepared.LocalModelExtraction != nil {
		return updateManifestOptionals(outDir, prepared)
	}
	return nil
}

func WriteOptionalArtifacts(outDir string, prepared PreparedRun) error {
	return writeOptionalArtifacts(outDir, prepared)
}

type normalizationFailureDebug struct {
	InitialOutput        string
	InitialError         error
	RepairOutput         string
	RepairError          error
	FinalError           error
	LocalModelExtraction *LocalModelExtractionArtifact
}
type normalizationFailureArtifact struct {
	SchemaVersion        string                        `json:"schema_version"`
	RunID                string                        `json:"run_id"`
	CreatedAt            string                        `json:"created_at"`
	Provider             RedactedProviderConfig        `json:"provider"`
	NormalizationEvents  []NormalizationEvent          `json:"normalization_events"`
	InitialOutput        string                        `json:"initial_output,omitempty"`
	InitialError         string                        `json:"initial_error,omitempty"`
	RepairOutput         string                        `json:"repair_output,omitempty"`
	RepairError          string                        `json:"repair_error,omitempty"`
	FinalError           string                        `json:"final_error,omitempty"`
	LocalModelExtraction *LocalModelExtractionArtifact `json:"local_model_extraction,omitempty"`
}

func writeNormalizationFailureAndReturn(config Config, run Run, provider ProviderConfig, events []NormalizationEvent, failure normalizationFailureDebug) error {
	finalErr := failure.FinalError
	if finalErr == nil {
		finalErr = failure.RepairError
	}
	if finalErr == nil {
		finalErr = failure.InitialError
	}
	if finalErr == nil {
		finalErr = fmt.Errorf("provider output failed normalization")
	}
	if err := writeNormalizationFailureDebug(config, run, provider, events, failure); err != nil {
		return fmt.Errorf("%w; additionally failed to write normalization debug artifact: %v", finalErr, err)
	}
	return finalErr
}
func writeLocalModelNormalizationFailureAndReturn(config Config, run Run, input PendingRunInput, events []NormalizationEvent, failure normalizationFailureDebug) error {
	result := classifyLocalModelCandidateMealPlanText(input.CandidateText)
	if isTerminalQualificationFailure(result) {
		events = append(events, normalizationEvent("qualification_failed_post_model", "candidate text was classified as not ready for verification after local model normalization failed"))
	} else {
		events = append(events, normalizationEvent("normalization_graceful_failed", "local model output could not be normalized into a verifiable meal plan"))
	}
	if failure.FinalError == nil {
		failure.FinalError = failure.InitialError
	}
	if failure.FinalError == nil {
		failure.FinalError = fmt.Errorf("local model output failed normalization")
	}
	publicMessage := localModelPublicFailureMessage(result)
	if err := writeNormalizationFailureDebug(config, run, input.Provider, events, failure); err != nil {
		return fmt.Errorf("%s; additionally failed to write normalization debug artifact: %v", publicMessage, err)
	}
	return fmt.Errorf("%s", publicMessage)
}
func localModelPublicFailureMessage(result MealPlanQualificationResult) string {
	if isTerminalQualificationFailure(result) && strings.TrimSpace(result.Reason) != "" {
		return result.Reason
	}
	return "MealCheck could not normalize this text into a verifiable meal plan. Use clear day labels, meal labels, food names, numeric quantities, and supported units."
}
func writeNormalizationFailureDebug(config Config, run Run, provider ProviderConfig, events []NormalizationEvent, failure normalizationFailureDebug) error {
	debugDir := filepath.Join(run.ArtifactDir, "debug")
	if err := os.MkdirAll(debugDir, 0o755); err != nil {
		return err
	}
	artifact := normalizationFailureArtifact{
		SchemaVersion:        "0.1",
		RunID:                run.ID,
		CreatedAt:            time.Now().UTC().Format(time.RFC3339),
		Provider:             redactProvider(provider),
		NormalizationEvents:  append([]NormalizationEvent(nil), events...),
		InitialOutput:        sanitizeDebugArtifactText(failure.InitialOutput, provider.APIKey),
		InitialError:         sanitizeDebugError(failure.InitialError, provider.APIKey),
		RepairOutput:         sanitizeDebugArtifactText(failure.RepairOutput, provider.APIKey),
		RepairError:          sanitizeDebugError(failure.RepairError, provider.APIKey),
		FinalError:           sanitizeDebugError(failure.FinalError, provider.APIKey),
		LocalModelExtraction: failure.LocalModelExtraction,
	}
	return writeJSONFile(filepath.Join(debugDir, "normalization-failure.json"), artifact)
}
func sanitizeDebugError(err error, apiKey string) string {
	if err == nil {
		return ""
	}
	return sanitizeProviderErrorText(err.Error(), apiKey)
}
func sanitizeRepairPromptError(err error, apiKey string) error {
	message := sanitizeDebugError(err, apiKey)
	if message == "" {
		message = "provider output failed MealCheck JSON decode"
	}
	return fmt.Errorf("%s", message)
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
func updateManifestOptionals(outDir string, prepared PreparedRun) error {
	manifestPath := filepath.Join(outDir, "manifest.json")
	var manifest struct {
		SchemaVersion string            `json:"schema_version"`
		CaseID        string            `json:"case_id"`
		Mode          string            `json:"mode"`
		GeneratedAt   string            `json:"generated_at"`
		MealCheck     map[string]string `json:"mealcheck"`
		Inputs        map[string]string `json:"inputs"`
		Artifacts     []string          `json:"artifacts"`
	}
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		return err
	}
	if prepared.UsedProvider {
		manifest.Artifacts = appendIfMissing(manifest.Artifacts, "optional/llm-output.json")
	}
	if len(prepared.NormalizationEvents) > 0 {
		manifest.Artifacts = appendIfMissing(manifest.Artifacts, "optional/normalization-events.json")
	}
	if prepared.LocalModelExtraction != nil {
		manifest.Artifacts = appendIfMissing(manifest.Artifacts, "optional/local-model-chunks.json")
	}
	return writeJSONFile(manifestPath, manifest)
}
func appendIfMissing(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
func writeJSONFile(path string, data any) error {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}
func normalizationEvent(eventType, message string) NormalizationEvent {
	return NormalizationEvent{
		Type:      eventType,
		Message:   message,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}
func redactProvider(config ProviderConfig) RedactedProviderConfig {
	providerType := config.Type
	if providerType == "" {
		providerType = ProviderTypeOpenAICompatible
	}
	if providerType == ProviderTypeLocalLlama {
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
