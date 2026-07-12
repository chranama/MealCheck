package planextract

import (
	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/llm/inference"
)

type Config = core.Config
type PendingRunInput = core.PendingRunInput
type ProviderConfig = core.ProviderConfig
type RedactedProviderConfig = core.RedactedProviderConfig
type Completer = inference.Completer
type Message = inference.Message
type Request = inference.Request
type StructuredOutput = inference.StructuredOutput
type MealPlanQualificationResult = core.MealPlanQualificationResult

const (
	QualificationStatusNotMealPlan                = core.QualificationStatusNotMealPlan
	QualificationStatusMealPlanTooVague           = core.QualificationStatusMealPlanTooVague
	QualificationStatusRecipeOrMenuNeedsDecompose = core.QualificationStatusRecipeOrMenuNeedsDecompose
	QualificationStatusUnsupportedUnits           = core.QualificationStatusUnsupportedUnits
	QualificationStatusOutsideHostedContract      = core.QualificationStatusOutsideHostedContract
)

func qualificationResult(status, reason string, missingFields []string) MealPlanQualificationResult {
	return core.QualificationResult(status, reason, append([]string(nil), missingFields...))
}
