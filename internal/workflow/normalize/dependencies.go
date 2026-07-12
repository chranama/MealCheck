package normalize

import (
	"context"

	"github.com/chranama/MealCheck/internal/llm/inference"
	planextract "github.com/chranama/MealCheck/internal/llm/planextract"
	"github.com/chranama/MealCheck/internal/workflow/checker"
	"github.com/chranama/MealCheck/internal/workflow/mealplan"
)

type decodePlanResult = mealplan.DecodePlanResult

func mealPlanCompletionRequest(messages []Message) CompletionRequest {
	return CompletionRequest{
		Messages: messages,
		StructuredOutput: &StructuredOutput{
			Name:           "mealcheck_meal_plan",
			StrictSchema:   mealplan.StrictMealPlanResponseSchema(),
			PortableSchema: mealplan.PortableMealPlanResponseSchema(),
		},
	}
}

func validateProviderConfig(config ProviderConfig) error {
	return inference.ValidateProviderConfig(config)
}

func sanitizeProviderErrorText(message, apiKey string) string {
	return inference.SanitizeErrorText(message, apiKey)
}

func normalizeLocalModelSettings(settings checker.Settings) checker.Settings {
	return planextract.NormalizeLocalModelSettings(settings)
}

func validateLocalModelSettings(settings checker.Settings) error {
	return planextract.ValidateLocalModelSettings(settings)
}

func validateLocalModelInputContract(config Config, text string) error {
	return planextract.ValidateLocalModelInputContract(config, text)
}

func requestLocalModelExtraction(ctx context.Context, completer Completer, providerConfig ProviderConfig, input PendingRunInput, planID string) (string, checker.Plan, []LocalLlamaNormalizationRepair, string, error) {
	return planextract.Extract(ctx, completer, providerConfig, input, planID)
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
