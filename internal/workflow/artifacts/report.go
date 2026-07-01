package artifacts

import (
	"fmt"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

type reportDocument struct {
	SchemaVersion     string                          `json:"schema_version"`
	CaseID            string                          `json:"case_id"`
	Decision          string                          `json:"decision"`
	ProfileSummary    checker.NutritionTargets        `json:"profile_summary"`
	ConstraintSummary checker.VerificationConstraints `json:"constraint_summary"`
	GuidelinePackID   string                          `json:"guideline_pack_id"`
	GuidelinePackName string                          `json:"guideline_pack_name,omitempty"`
	Sections          []reportSection                 `json:"sections"`
	Disclaimer        string                          `json:"disclaimer"`
}

type reportSection struct {
	Title string           `json:"title"`
	Body  string           `json:"body"`
	Items []map[string]any `json:"items,omitempty"`
}

type metricsDocument struct {
	CaseID                  string `json:"case_id"`
	Decision                string `json:"decision"`
	ResolvedItems           int    `json:"resolved_items"`
	ExactResolvedItems      int    `json:"exact_resolved_items"`
	EstimatedItems          int    `json:"estimated_items"`
	DecomposedItems         int    `json:"decomposed_items"`
	UnresolvedItems         int    `json:"unresolved_items"`
	ExcludedUnresolvedItems int    `json:"excluded_unresolved_items"`
	CheckCount              int    `json:"check_count"`
	BlockCount              int    `json:"block_count"`
	WarnCount               int    `json:"warn_count"`
}

func buildReport(c checker.Case, e checker.Evaluation) reportDocument {
	failures := failedChecks(e.Checks)
	return reportDocument{
		SchemaVersion:     "0.1",
		CaseID:            c.CaseID,
		Decision:          e.Decision,
		ProfileSummary:    c.Settings.NutritionTargets,
		ConstraintSummary: c.Settings.VerificationConstraints,
		GuidelinePackID:   c.GuidelinePackID,
		Sections: []reportSection{
			{
				Title: "Summary",
				Body:  e.Summary,
			},
			{
				Title: "Failed Or Warning Checks",
				Body:  fmt.Sprintf("%d checks require attention.", len(failures)),
				Items: checkItems(failures),
			},
			{
				Title: "Unresolved Foods",
				Body:  fmt.Sprintf("%d unresolved food or quantity items.", len(e.UnresolvedItems)),
				Items: unresolvedItems(e.UnresolvedItems),
			},
			{
				Title: "Excluded Unresolved Foods",
				Body:  fmt.Sprintf("%d de minimis unresolved items excluded from nutrition totals.", len(e.ExcludedUnresolvedItems)),
				Items: excludedUnresolvedItems(e.ExcludedUnresolvedItems),
			},
			{
				Title: "Estimated And Decomposed Foods",
				Body:  fmt.Sprintf("%d estimated or decomposed food items.", approximateResolutionCount(e.ResolvedItems)),
				Items: approximateResolutionItems(e.ResolvedItems),
			},
			{
				Title: "Daily Totals",
				Body:  "Calculated from resolved, estimated, and decomposed food items.",
				Items: dailyTotalItems(e.DailyTotals),
			},
		},
		Disclaimer: "MealCheck checks bounded guideline-derived rules. It does not provide medical nutrition advice.",
	}
}

func buildMetrics(e checker.Evaluation) metricsDocument {
	blockCount := 0
	warnCount := 0
	for _, check := range e.Checks {
		switch check.Status {
		case "block":
			blockCount++
		case "warn":
			warnCount++
		}
	}
	exactCount := 0
	estimatedCount := 0
	decomposedCount := 0
	for _, item := range e.ResolvedItems {
		switch item.ResolutionMethod {
		case "estimated":
			estimatedCount++
		case "decomposed":
			decomposedCount++
		default:
			exactCount++
		}
	}
	return metricsDocument{
		CaseID:                  e.CaseID,
		Decision:                e.Decision,
		ResolvedItems:           len(e.ResolvedItems),
		ExactResolvedItems:      exactCount,
		EstimatedItems:          estimatedCount,
		DecomposedItems:         decomposedCount,
		UnresolvedItems:         len(e.UnresolvedItems),
		ExcludedUnresolvedItems: len(e.ExcludedUnresolvedItems),
		CheckCount:              len(e.Checks),
		BlockCount:              blockCount,
		WarnCount:               warnCount,
	}
}

func failedChecks(checks []checker.CheckResult) []checker.CheckResult {
	var result []checker.CheckResult
	for _, check := range checks {
		if check.Status == "block" || check.Status == "warn" {
			result = append(result, check)
		}
	}
	return result
}

func checkItems(checks []checker.CheckResult) []map[string]any {
	items := make([]map[string]any, 0, len(checks))
	for _, check := range checks {
		items = append(items, map[string]any{
			"check_id": check.CheckID,
			"status":   check.Status,
			"severity": check.Severity,
			"message":  check.Message,
		})
	}
	return items
}

func unresolvedItems(unresolved []checker.UnresolvedItem) []map[string]any {
	items := make([]map[string]any, 0, len(unresolved))
	for _, item := range unresolved {
		items = append(items, map[string]any{
			"day":               item.Day,
			"meal":              item.Meal,
			"food":              item.Food,
			"source_food_code":  item.SourceFoodCode,
			"quantity":          item.Quantity,
			"quantity_text":     item.QuantityText,
			"unit":              item.Unit,
			"unresolved_reason": item.UnresolvedReason,
		})
	}
	return items
}

func excludedUnresolvedItems(excluded []checker.ExcludedUnresolvedItem) []map[string]any {
	items := make([]map[string]any, 0, len(excluded))
	for _, item := range excluded {
		items = append(items, map[string]any{
			"day":                 item.Day,
			"meal":                item.Meal,
			"food":                item.Food,
			"quantity":            item.Quantity,
			"unit":                item.Unit,
			"deterministic_grams": item.DeterministicGrams,
			"unresolved_reason":   item.UnresolvedReason,
			"exclusion_reason":    item.ExclusionReason,
			"policy_id":           item.PolicyID,
		})
	}
	return items
}

func approximateResolutionCount(resolved []checker.ResolvedItem) int {
	count := 0
	for _, item := range resolved {
		if item.ResolutionMethod == "estimated" || item.ResolutionMethod == "decomposed" {
			count++
		}
	}
	return count
}

func approximateResolutionItems(resolved []checker.ResolvedItem) []map[string]any {
	items := make([]map[string]any, 0, approximateResolutionCount(resolved))
	for _, item := range resolved {
		if item.ResolutionMethod != "estimated" && item.ResolutionMethod != "decomposed" {
			continue
		}
		entry := map[string]any{
			"day":               item.Day,
			"meal":              item.Meal,
			"food":              item.Food,
			"source_food_code":  item.SourceFoodCode,
			"resolution_method": item.ResolutionMethod,
			"confidence":        item.Confidence,
			"estimate_reason":   item.EstimateReason,
		}
		if item.ProxyFood != "" {
			entry["proxy_food"] = item.ProxyFood
			entry["proxy_food_id"] = item.ProxyFoodID
		}
		if len(item.Components) > 0 {
			entry["component_count"] = len(item.Components)
		}
		items = append(items, entry)
	}
	return items
}

func dailyTotalItems(totals []checker.DailyTotal) []map[string]any {
	items := make([]map[string]any, 0, len(totals))
	for _, total := range totals {
		items = append(items, map[string]any{
			"day":                          total.Day,
			"energy_kcal":                  total.Nutrients.EnergyKcal,
			"protein_g":                    total.Nutrients.ProteinG,
			"sodium_mg":                    total.Nutrients.SodiumMG,
			"saturated_fat_pct_calories":   total.SaturatedFatPctCalories,
			"added_sugar_g":                total.Nutrients.AddedSugarG,
			"resolved_food_groups_present": total.FoodGroups,
		})
	}
	return items
}
