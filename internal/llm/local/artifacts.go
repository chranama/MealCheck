package localmodel

import (
	"strings"
	"time"
)

type LocalModelExtractionArtifact struct {
	SchemaVersion        string                                 `json:"schema_version"`
	CreatedAt            string                                 `json:"created_at"`
	Provider             RedactedProviderConfig                 `json:"provider"`
	PlanID               string                                 `json:"plan_id"`
	ChunkCount           int                                    `json:"chunk_count"`
	SourceItemCount      int                                    `json:"source_item_count"`
	StageTimings         LocalModelExtractionStageTimings       `json:"stage_timings"`
	RepeatRunInstability LocalModelRepeatRunInstabilityArtifact `json:"repeat_run_instability"`
	FailureStage         string                                 `json:"failure_stage,omitempty"`
	Error                string                                 `json:"error,omitempty"`
	Chunks               []LocalModelChunkArtifact              `json:"chunks"`
}

type LocalModelExtractionStageTimings struct {
	ChunkingMS          int64 `json:"chunking_ms"`
	ExpansionMS         int64 `json:"expansion_ms"`
	CompletenessCheckMS int64 `json:"completeness_check_ms"`
	TotalMS             int64 `json:"total_ms"`
}

type LocalModelRepeatRunInstabilityArtifact struct {
	Measured bool   `json:"measured"`
	Reason   string `json:"reason,omitempty"`
}

type LocalModelChunkArtifact struct {
	Index          int                                   `json:"index"`
	Day            int                                   `json:"day"`
	MealCode       string                                `json:"meal_code"`
	MealLabel      string                                `json:"meal_label,omitempty"`
	MealText       string                                `json:"meal_text"`
	SourceItemIDs  []int                                 `json:"source_item_ids"`
	SourceItems    []LocalModelChunkSourceItemArtifact   `json:"source_items"`
	Prompt         LocalModelPromptArtifact              `json:"prompt"`
	RawOutput      string                                `json:"raw_compact_output,omitempty"`
	DecodedRows    []LocalModelChunkDecodedRowArtifact   `json:"decoded_rows,omitempty"`
	Reconciliation LocalModelChunkReconciliationArtifact `json:"reconciliation"`
	StageTimings   LocalModelChunkStageTimings           `json:"stage_timings"`
	FailureStage   string                                `json:"failure_stage,omitempty"`
	Error          string                                `json:"error,omitempty"`
}

type LocalModelChunkSourceItemArtifact struct {
	ID          int    `json:"id"`
	Day         int    `json:"day"`
	MealCode    string `json:"meal_code"`
	Text        string `json:"text"`
	ParseStatus string `json:"parse_status"`
}

type LocalModelPromptArtifact struct {
	MessageCount int                               `json:"message_count"`
	Messages     []LocalModelPromptMessageArtifact `json:"messages"`
}

type LocalModelPromptMessageArtifact struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LocalModelChunkDecodedRowArtifact struct {
	SourceItemID     int      `json:"source_item_id"`
	Day              int      `json:"day"`
	MealCode         string   `json:"meal_code"`
	Food             string   `json:"food"`
	Resolved         bool     `json:"resolved"`
	Quantity         *float64 `json:"quantity,omitempty"`
	Unit             string   `json:"unit,omitempty"`
	QuantityText     string   `json:"quantity_text,omitempty"`
	UnresolvedReason string   `json:"unresolved_reason,omitempty"`
}

type LocalModelChunkReconciliationArtifact struct {
	DecodedSourceItemIDs []int                           `json:"decoded_source_item_ids,omitempty"`
	RepairCount          int                             `json:"repair_count"`
	Repairs              []LocalLlamaNormalizationRepair `json:"repairs,omitempty"`
}

type LocalModelChunkStageTimings struct {
	PromptBuildMS     int64 `json:"prompt_build_ms"`
	ProviderRequestMS int64 `json:"provider_request_ms"`
	DecodeReconcileMS int64 `json:"decode_reconcile_ms"`
	TotalMS           int64 `json:"total_ms"`
}

func newLocalModelExtractionArtifact(provider ProviderConfig, planID string) *LocalModelExtractionArtifact {
	return &LocalModelExtractionArtifact{
		SchemaVersion: "0.1",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		Provider:      redactProvider(provider),
		PlanID:        planID,
		RepeatRunInstability: LocalModelRepeatRunInstabilityArtifact{
			Measured: false,
			Reason:   "single hosted run; repeat instability is measured by eval-normalization local_model_case_repeat_summary",
		},
	}
}

func localModelChunkArtifact(index int, chunk localLlamaMealChunk) LocalModelChunkArtifact {
	mealLabel := strings.TrimSpace(chunk.MealLabel)
	if mealLabel == "" {
		mealLabel, _ = localLlamaMealName(chunk.MealCode)
	}
	sourceIDs, sourceItems := localModelChunkSourceItemArtifacts(chunk)
	return LocalModelChunkArtifact{
		Index:         index,
		Day:           chunk.Day,
		MealCode:      chunk.MealCode,
		MealLabel:     mealLabel,
		MealText:      strings.TrimSpace(chunk.MealText),
		SourceItemIDs: sourceIDs,
		SourceItems:   sourceItems,
		Reconciliation: LocalModelChunkReconciliationArtifact{
			RepairCount: 0,
		},
	}
}

func localModelChunkSourceItemArtifacts(chunk localLlamaMealChunk) ([]int, []LocalModelChunkSourceItemArtifact) {
	sourceIDs := make([]int, 0, len(chunk.Items))
	sourceItems := make([]LocalModelChunkSourceItemArtifact, 0, len(chunk.Items))
	for _, item := range chunk.Items {
		sourceIDs = append(sourceIDs, item.ID)
		sourceItems = append(sourceItems, LocalModelChunkSourceItemArtifact{
			ID:          item.ID,
			Day:         item.Day,
			MealCode:    item.MealCode,
			Text:        item.Text,
			ParseStatus: string(item.ParseStatus),
		})
	}
	return sourceIDs, sourceItems
}

func localModelPromptArtifact(messages []ProviderMessage, apiKey string) LocalModelPromptArtifact {
	prompt := LocalModelPromptArtifact{
		MessageCount: len(messages),
		Messages:     make([]LocalModelPromptMessageArtifact, 0, len(messages)),
	}
	for _, message := range messages {
		prompt.Messages = append(prompt.Messages, LocalModelPromptMessageArtifact{
			Role:    message.Role,
			Content: sanitizeDebugArtifactText(message.Content, apiKey),
		})
	}
	return prompt
}

func localModelDecodedRowArtifacts(rows []localLlamaRowItem) []LocalModelChunkDecodedRowArtifact {
	artifacts := make([]LocalModelChunkDecodedRowArtifact, 0, len(rows))
	for _, row := range rows {
		artifact := LocalModelChunkDecodedRowArtifact{
			SourceItemID:     row.SourceItemID,
			Day:              row.Day,
			MealCode:         row.MealCode,
			Food:             row.Food,
			Unit:             row.Unit,
			QuantityText:     row.QuantityText,
			UnresolvedReason: row.UnresolvedReason,
		}
		if strings.TrimSpace(row.QuantityText) == "" && strings.TrimSpace(row.UnresolvedReason) == "" {
			quantity := row.Quantity
			artifact.Quantity = &quantity
			artifact.Resolved = true
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts
}

func localModelDecodedSourceItemIDs(rows []localLlamaRowItem) []int {
	ids := make([]int, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.SourceItemID)
	}
	return ids
}

func finishLocalModelExtractionArtifact(artifact *LocalModelExtractionArtifact, started time.Time, stage localModelExtractionFailureStage, err error, apiKey string) {
	if artifact == nil {
		return
	}
	artifact.StageTimings.TotalMS = elapsedMillis(started)
	if stage != "" {
		artifact.FailureStage = string(stage)
	}
	if err != nil {
		artifact.Error = sanitizeDebugError(err, apiKey)
	}
}

func elapsedMillis(started time.Time) int64 {
	if started.IsZero() {
		return 0
	}
	elapsed := time.Since(started).Milliseconds()
	if elapsed < 0 {
		return 0
	}
	return elapsed
}
