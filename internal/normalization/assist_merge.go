package normalization

import "fmt"

func MergeAssistRows(result Result) (checkerPlanRows []NormalizedRow, missing []UnresolvedItem) {
	rows := DeterministicRows(result.ParsedItems)
	covered := map[int]bool{}
	for _, row := range rows {
		if row.SourceItemID > 0 {
			covered[row.SourceItemID] = true
		}
	}
	for _, accepted := range result.AcceptedAssistRows {
		rows = append(rows, NormalizedRow{
			SourceItemID: accepted.SourceItem.ID,
			SourceText:   accepted.SourceItem.Text,
			Day:          accepted.Day,
			MealCode:     accepted.MealCode,
			Food:         accepted.Food,
			Quantity:     accepted.Quantity,
			Unit:         accepted.Unit,
			Source:       "llm_assist",
			Confidence:   accepted.Confidence,
		})
		covered[accepted.SourceItem.ID] = true
	}
	for _, unresolved := range result.UnresolvedItems {
		if unresolved.SourceItem.ID > 0 && !covered[unresolved.SourceItem.ID] {
			missing = append(missing, unresolved)
		}
	}
	return rows, missing
}

func BuildAssistedPlan(result Result) (Result, error) {
	rows, missing := MergeAssistRows(result)
	if len(missing) > 0 {
		result.Method = MethodFailedPostAssistValidation
		result.UnresolvedItems = missing
		result.ReviewFlags = appendIfMissingString(result.ReviewFlags, ReviewFlagLLMAssistIncomplete)
		return result, fmt.Errorf("normalization LLM assist left %d source item(s) unresolved", len(missing))
	}
	if len(result.AcceptedAssistRows) == 0 {
		result.Method = MethodFailedPostAssistValidation
		result.ReviewFlags = appendIfMissingString(result.ReviewFlags, ReviewFlagLLMAssistIncomplete)
		return result, fmt.Errorf("normalization LLM assist returned no accepted rows")
	}
	plan, err := BuildPlanFromRows(rows, result.PlanID)
	if err != nil {
		result.Method = MethodFailedPostAssistValidation
		result.ReviewFlags = appendIfMissingString(result.ReviewFlags, ReviewFlagLLMAssistInvalidMerge)
		return result, err
	}
	result.Plan = plan
	result.Method = MethodDeterministicWithLLMAssist
	result.UnresolvedItems = nil
	return result, nil
}
