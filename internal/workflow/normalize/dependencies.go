package normalize

import (
	"context"

	llm "github.com/chranama/MealCheck/internal/llm/external"
	localmodel "github.com/chranama/MealCheck/internal/llm/local"
	"github.com/chranama/MealCheck/internal/workflow/checker"
	"github.com/chranama/MealCheck/internal/workflow/mealplan"
)

type decodePlanResult = mealplan.DecodePlanResult

func DefaultProviderFactory(config ProviderConfig) (Provider, error) {
	return llm.DefaultProviderFactory(config)
}

func validateProviderConfig(config ProviderConfig) error {
	return llm.ValidateProviderConfig(config)
}

func sanitizeProviderErrorText(message, apiKey string) string {
	return llm.SanitizeProviderErrorText(message, apiKey)
}

func normalizeLocalModelSettings(settings checker.Settings) checker.Settings {
	return localmodel.NormalizeLocalModelSettings(settings)
}

func validateLocalModelSettings(settings checker.Settings) error {
	return localmodel.ValidateLocalModelSettings(settings)
}

func validateLocalModelInputContract(config Config, text string) error {
	return localmodel.ValidateLocalModelInputContract(config, text)
}

func requestLocalModelExtraction(ctx context.Context, provider Provider, providerConfig ProviderConfig, input PendingRunInput, planID string) (string, checker.Plan, []LocalLlamaNormalizationRepair, string, error) {
	return localmodel.RunLocalModelExtraction(ctx, provider, providerConfig, input, planID)
}

func mealPlanContractPromptBlock() string {
	return mealplan.MealPlanContractPromptBlock()
}

func mealPlanAliasRules() []string {
	return mealplan.MealPlanAliasRules()
}

func mealPlanShapeInstructions(constraints checker.VerificationConstraints) map[string]any {
	return mealplan.MealPlanShapeInstructions(constraints)
}

func decodePlanText(text string) (checker.Plan, error) {
	return mealplan.DecodePlanText(text)
}

func decodePlanTextDetailed(text string) (decodePlanResult, error) {
	return mealplan.DecodePlanTextDetailed(text)
}

func canonicalizePlanJSON(jsonText string) (string, bool, error) {
	return mealplan.CanonicalizePlanJSON(jsonText)
}
