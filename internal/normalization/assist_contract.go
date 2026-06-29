package normalization

const (
	P0AssistTask       = "p0_normalization_assist"
	P0AssistSchemaName = "mealcheck_p0_normalization_assist"

	AssistActionProposeRow         = "propose_row"
	AssistActionNeedsClarification = "needs_clarification"
	AssistActionAbstain            = "abstain"

	AssistConfidenceHigh   = "high"
	AssistConfidenceMedium = "medium"
	AssistConfidenceLow    = "low"
)

type AssistRequestPayload struct {
	Task               string       `json:"task"`
	ChunkID            string       `json:"chunk_id"`
	SourceItems        []SourceItem `json:"source_items"`
	AllowedUnits       []string     `json:"allowed_units"`
	AllowedMealCodes   []string     `json:"allowed_meal_codes"`
	FixedSourceItemIDs []int        `json:"fixed_source_item_ids,omitempty"`
}

type AssistResponse struct {
	Items []AssistResponseItem `json:"items"`
}

type AssistResponseItem struct {
	SourceItemID int     `json:"source_item_id"`
	Action       string  `json:"action"`
	Day          int     `json:"day"`
	MealCode     string  `json:"meal_code"`
	Food         string  `json:"food"`
	Quantity     float64 `json:"quantity"`
	Unit         string  `json:"unit"`
	Confidence   string  `json:"confidence"`
	Message      string  `json:"message"`
}

type AssistResponseArtifact struct {
	ChunkID string               `json:"chunk_id"`
	RawText string               `json:"raw_text,omitempty"`
	Items   []AssistResponseItem `json:"items,omitempty"`
	Error   string               `json:"error,omitempty"`
}

type AssistAcceptedRow struct {
	ChunkID    string     `json:"chunk_id"`
	SourceItem SourceItem `json:"source_item"`
	Day        int        `json:"day"`
	MealCode   string     `json:"meal_code"`
	Food       string     `json:"food"`
	Quantity   float64    `json:"quantity"`
	Unit       string     `json:"unit"`
	Confidence string     `json:"confidence"`
	Message    string     `json:"message,omitempty"`
}

type AssistRejectedRow struct {
	ChunkID      string `json:"chunk_id"`
	SourceItemID int    `json:"source_item_id,omitempty"`
	Action       string `json:"action,omitempty"`
	Reason       string `json:"reason"`
	Message      string `json:"message,omitempty"`
}

type AssistValidationResult struct {
	Response AssistResponse
	Accepted []AssistAcceptedRow
	Rejected []AssistRejectedRow
}

func P0AssistResponseSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"source_item_id": map[string]any{"type": "integer", "minimum": 1},
						"action": map[string]any{
							"type": "string",
							"enum": []any{AssistActionProposeRow, AssistActionNeedsClarification, AssistActionAbstain},
						},
						"day":       map[string]any{"type": "integer", "minimum": 0, "maximum": 7},
						"meal_code": map[string]any{"type": "string", "enum": []any{"", "b", "m", "l", "a", "d", "s", "e"}},
						"food":      map[string]any{"type": "string"},
						"quantity":  map[string]any{"type": "number", "minimum": 0},
						"unit":      map[string]any{"type": "string", "enum": []any{"", "g", "oz", "cup", "tbsp", "tsp", "slice", "serving"}},
						"confidence": map[string]any{
							"type": "string",
							"enum": []any{AssistConfidenceHigh, AssistConfidenceMedium, AssistConfidenceLow},
						},
						"message": map[string]any{"type": "string"},
					},
					"required": []any{"source_item_id", "action", "day", "meal_code", "food", "quantity", "unit", "confidence", "message"},
				},
			},
		},
		"required": []any{"items"},
	}
}

func AllowedAssistUnits() []string {
	return []string{"g", "oz", "cup", "tbsp", "tsp", "slice", "serving"}
}

func AllowedAssistMealCodes() []string {
	return []string{"b", "m", "l", "a", "d", "s", "e"}
}
