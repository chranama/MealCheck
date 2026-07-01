package localmodel

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

type localModelExtractionFailureStage string

const (
	localModelFailureProvider     localModelExtractionFailureStage = "provider"
	localModelFailureDecode       localModelExtractionFailureStage = "decode"
	localModelFailureCompleteness localModelExtractionFailureStage = "completeness"
)

// RunLocalModelExtraction normalizes hosted local-model text through the same
// chunked extraction path used by live run creation.
func RunLocalModelExtraction(ctx context.Context, provider Provider, providerConfig ProviderConfig, input PendingRunInput, planID string) (string, checker.Plan, []LocalLlamaNormalizationRepair, string, error) {
	output, plan, repairs, _, stage, err := requestLocalModelExtractionWithArtifacts(ctx, provider, providerConfig, input, planID)
	return output, plan, repairs, string(stage), err
}

// RunLocalModelExtractionWithArtifacts normalizes hosted local-model text and
// returns the chunk-level evidence artifact captured during extraction.
func RunLocalModelExtractionWithArtifacts(ctx context.Context, provider Provider, providerConfig ProviderConfig, input PendingRunInput, planID string) (string, checker.Plan, []LocalLlamaNormalizationRepair, *LocalModelExtractionArtifact, string, error) {
	output, plan, repairs, extraction, stage, err := requestLocalModelExtractionWithArtifacts(ctx, provider, providerConfig, input, planID)
	return output, plan, repairs, extraction, string(stage), err
}

func requestLocalModelExtraction(ctx context.Context, provider Provider, providerConfig ProviderConfig, input PendingRunInput, planID string) (string, checker.Plan, []LocalLlamaNormalizationRepair, localModelExtractionFailureStage, error) {
	output, plan, repairs, _, stage, err := requestLocalModelExtractionWithArtifacts(ctx, provider, providerConfig, input, planID)
	return output, plan, repairs, stage, err
}

func requestLocalModelExtractionWithArtifacts(ctx context.Context, provider Provider, providerConfig ProviderConfig, input PendingRunInput, planID string) (string, checker.Plan, []LocalLlamaNormalizationRepair, *LocalModelExtractionArtifact, localModelExtractionFailureStage, error) {
	started := time.Now()
	extraction := newLocalModelExtractionArtifact(providerConfig, planID)
	chunkingStarted := time.Now()
	chunks := localLlamaExtractionMealChunks(input.CandidateText)
	extraction.StageTimings.ChunkingMS = elapsedMillis(chunkingStarted)
	extraction.ChunkCount = len(chunks)
	extraction.SourceItemCount = localLlamaMealChunkItemCount(chunks)
	if len(chunks) == 0 {
		err := fmt.Errorf("candidate_text must identify at least one meal chunk with source food items")
		finishLocalModelExtractionArtifact(extraction, started, localModelFailureDecode, err, providerConfig.APIKey)
		return "", checker.Plan{}, nil, extraction, localModelFailureDecode, err
	}
	var outputs []string
	var rows []localLlamaRowItem
	var repairs []LocalLlamaNormalizationRepair
	for index, chunk := range chunks {
		chunkStarted := time.Now()
		chunkArtifact := localModelChunkArtifact(index+1, chunk)
		promptStarted := time.Now()
		messages, err := localModelExtractionMessagesForMealChunk(input, chunk)
		chunkArtifact.StageTimings.PromptBuildMS = elapsedMillis(promptStarted)
		if err != nil {
			chunkArtifact.FailureStage = string(localModelFailureDecode)
			chunkArtifact.Error = sanitizeDebugError(err, providerConfig.APIKey)
			chunkArtifact.StageTimings.TotalMS = elapsedMillis(chunkStarted)
			extraction.Chunks = append(extraction.Chunks, chunkArtifact)
			output := localLlamaCombinedChunkOutput(outputs)
			finishLocalModelExtractionArtifact(extraction, started, localModelFailureDecode, err, providerConfig.APIKey)
			return output, checker.Plan{}, repairs, extraction, localModelFailureDecode, err
		}
		chunkArtifact.Prompt = localModelPromptArtifact(messages, providerConfig.APIKey)
		providerStarted := time.Now()
		output, err := provider.Complete(ctx, providerConfig, messages)
		chunkArtifact.StageTimings.ProviderRequestMS = elapsedMillis(providerStarted)
		chunkArtifact.RawOutput = sanitizeDebugArtifactText(strings.TrimSpace(output), providerConfig.APIKey)
		outputs = append(outputs, localLlamaChunkOutputBlock(chunk, output))
		if err != nil {
			chunkArtifact.FailureStage = string(localModelFailureProvider)
			chunkArtifact.Error = sanitizeDebugError(err, providerConfig.APIKey)
			chunkArtifact.StageTimings.TotalMS = elapsedMillis(chunkStarted)
			extraction.Chunks = append(extraction.Chunks, chunkArtifact)
			finishLocalModelExtractionArtifact(extraction, started, localModelFailureProvider, err, providerConfig.APIKey)
			return localLlamaCombinedChunkOutput(outputs), checker.Plan{}, repairs, extraction, localModelFailureProvider, err
		}
		decodeStarted := time.Now()
		chunkRows, chunkRepairs, err := decodeLocalLlamaMealChunkRows(output, chunk)
		chunkArtifact.StageTimings.DecodeReconcileMS = elapsedMillis(decodeStarted)
		repairs = append(repairs, chunkRepairs...)
		if err != nil {
			chunkArtifact.FailureStage = string(localModelFailureDecode)
			chunkArtifact.Error = sanitizeDebugError(err, providerConfig.APIKey)
			chunkArtifact.StageTimings.TotalMS = elapsedMillis(chunkStarted)
			extraction.Chunks = append(extraction.Chunks, chunkArtifact)
			finishLocalModelExtractionArtifact(extraction, started, localModelFailureDecode, err, providerConfig.APIKey)
			return localLlamaCombinedChunkOutput(outputs), checker.Plan{}, repairs, extraction, localModelFailureDecode, err
		}
		chunkArtifact.DecodedRows = localModelDecodedRowArtifacts(chunkRows)
		chunkArtifact.Reconciliation = LocalModelChunkReconciliationArtifact{
			DecodedSourceItemIDs: localModelDecodedSourceItemIDs(chunkRows),
			RepairCount:          len(chunkRepairs),
			Repairs:              chunkRepairs,
		}
		chunkArtifact.StageTimings.TotalMS = elapsedMillis(chunkStarted)
		extraction.Chunks = append(extraction.Chunks, chunkArtifact)
		rows = append(rows, chunkRows...)
	}
	output := localLlamaCombinedChunkOutput(outputs)
	expansionStarted := time.Now()
	plan, err := expandLocalLlamaRows(rows, planID)
	extraction.StageTimings.ExpansionMS = elapsedMillis(expansionStarted)
	if err != nil {
		finishLocalModelExtractionArtifact(extraction, started, localModelFailureDecode, err, providerConfig.APIKey)
		return output, checker.Plan{}, repairs, extraction, localModelFailureDecode, err
	}
	completenessStarted := time.Now()
	if err := validateLocalModelExtractionCompleteness(plan, input.CandidateText); err != nil {
		extraction.StageTimings.CompletenessCheckMS = elapsedMillis(completenessStarted)
		finishLocalModelExtractionArtifact(extraction, started, localModelFailureCompleteness, err, providerConfig.APIKey)
		return output, checker.Plan{}, repairs, extraction, localModelFailureCompleteness, err
	}
	extraction.StageTimings.CompletenessCheckMS = elapsedMillis(completenessStarted)
	finishLocalModelExtractionArtifact(extraction, started, "", nil, providerConfig.APIKey)
	return output, plan, repairs, extraction, "", nil
}
func localLlamaChunkOutputBlock(chunk localLlamaMealChunk, output string) string {
	return fmt.Sprintf("Meal chunk day=%d meal_code=%s source_ids=%s\n%s", chunk.Day, chunk.MealCode, localLlamaChunkSourceIDList(chunk), strings.TrimSpace(output))
}
func localLlamaCombinedChunkOutput(outputs []string) string {
	return strings.TrimSpace(strings.Join(outputs, "\n\n"))
}
