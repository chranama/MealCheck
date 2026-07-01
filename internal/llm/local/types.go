package localmodel

import (
	"github.com/chranama/MealCheck/internal/core"
	llm "github.com/chranama/MealCheck/internal/llm/external"
)

type Config = core.Config
type PendingRunInput = core.PendingRunInput
type ProviderConfig = core.ProviderConfig
type RedactedProviderConfig = core.RedactedProviderConfig
type Provider = llm.Provider
type ProviderMessage = llm.ProviderMessage
type MealPlanQualificationResult = core.MealPlanQualificationResult

const (
	QualificationStatusNotMealPlan                = core.QualificationStatusNotMealPlan
	QualificationStatusMealPlanTooVague           = core.QualificationStatusMealPlanTooVague
	QualificationStatusRecipeOrMenuNeedsDecompose = core.QualificationStatusRecipeOrMenuNeedsDecompose
	QualificationStatusOutsideHostedContract      = core.QualificationStatusOutsideHostedContract
)

func qualificationResult(status, reason string, missingFields []string) MealPlanQualificationResult {
	return core.QualificationResult(status, reason, append([]string(nil), missingFields...))
}
