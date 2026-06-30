package hosted

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/chranama/MealCheck/internal/checker"
)

const (
	QualificationStatusNotMealPlan                 = "not_meal_plan"
	QualificationStatusMealPlanTooVague            = "meal_plan_too_vague"
	QualificationStatusRecipeOrMenuNeedsDecompose  = "recipe_or_menu_needs_decomposition"
	QualificationStatusOutsideHostedContract       = "meal_plan_outside_hosted_contract"
	QualificationStatusEligibleForVerification     = "eligible_for_verification"
	QualificationStatusEligibleWithUnresolvedItems = "eligible_with_unresolved_items"
)

type MealPlanQualificationRequest struct {
	Text     string           `json:"text"`
	Settings checker.Settings `json:"settings,omitempty"`
	Provider ProviderConfig   `json:"provider,omitempty"`
}

type MealPlanQualificationResult struct {
	SchemaVersion  string        `json:"schema_version"`
	Status         string        `json:"status"`
	Reason         string        `json:"reason"`
	MissingFields  []string      `json:"missing_fields,omitempty"`
	NormalizedPlan *checker.Plan `json:"normalized_plan,omitempty"`
	ProviderUsed   bool          `json:"provider_used"`
	Canonicalized  bool          `json:"canonicalized,omitempty"`
}

type qualificationRejectionError struct {
	Qualification MealPlanQualificationResult
}

func (e qualificationRejectionError) Error() string {
	if e.Qualification.Reason != "" {
		return e.Qualification.Reason
	}
	return "candidate text is not ready for verification"
}

func QualifyStructuredMealPlanText(text string) MealPlanQualificationResult {
	return qualifyMealPlanJSON(text)
}

func QualifyMealPlanText(ctx context.Context, providerFactory ProviderFactory, request MealPlanQualificationRequest) (MealPlanQualificationResult, error) {
	text := strings.TrimSpace(request.Text)
	if text == "" {
		return qualificationResult(QualificationStatusNotMealPlan, "No candidate meal-plan text was provided.", []string{"text"}), nil
	}

	if result := qualifyMealPlanJSON(text); result.Status == QualificationStatusEligibleForVerification || result.Status == QualificationStatusEligibleWithUnresolvedItems {
		return result, nil
	}

	classification := classifyCandidateMealPlanText(text)
	if request.Provider.Type == ProviderTypeLocalLlama {
		classification = classifyLocalModelCandidateMealPlanText(text)
	}
	if classification.Status != QualificationStatusEligibleForVerification {
		if request.Provider.Type == ProviderTypeLocalLlama && classification.Status == QualificationStatusEligibleWithUnresolvedItems {
			// Continue so the local model can preserve missing quantities as
			// unresolved rows in the normalized plan.
		} else {
			return classification, nil
		}
	}

	if providerFactory == nil {
		providerFactory = DefaultProviderFactory
	}
	if err := validateProviderConfig(request.Provider); err != nil {
		return MealPlanQualificationResult{}, err
	}
	provider, err := providerFactory(request.Provider)
	if err != nil {
		return MealPlanQualificationResult{}, err
	}

	if request.Provider.Type == ProviderTypeLocalLlama {
		_, plan, _, _, decodeErr := requestLocalModelExtraction(ctx, provider, request.Provider, PendingRunInput{
			Mode:          InputModeLocalModel,
			Settings:      request.Settings,
			CandidateText: text,
			Provider:      request.Provider,
		}, defaultLocalLlamaPlanID)
		if decodeErr != nil {
			return MealPlanQualificationResult{
				SchemaVersion: "0.1",
				Status:        QualificationStatusMealPlanTooVague,
				Reason:        "The candidate text looked like a meal plan, but the local model did not return compact MealCheck meal-plan JSON.",
				MissingFields: []string{"normalized_plan"},
				ProviderUsed:  true,
			}, nil
		}
		result := qualificationResultForPlan(plan, false)
		result.ProviderUsed = true
		return result, nil
	}
	output, err := provider.Complete(ctx, request.Provider, qualificationMessages(request))
	if err != nil {
		return MealPlanQualificationResult{}, err
	}
	result := qualifyMealPlanJSON(output)
	result.ProviderUsed = true
	if result.Status == QualificationStatusEligibleForVerification || result.Status == QualificationStatusEligibleWithUnresolvedItems {
		return result, nil
	}
	result.Status = QualificationStatusMealPlanTooVague
	result.Reason = "The candidate text looked like a meal plan, but the provider did not return normalized MealCheck meal-plan JSON."
	result.MissingFields = appendIfMissingString(result.MissingFields, "normalized_plan")
	return result, nil
}

func qualifyMealPlanJSON(text string) MealPlanQualificationResult {
	decodeResult, err := decodePlanTextDetailed(text)
	if err != nil {
		return qualificationResult(QualificationStatusMealPlanTooVague, "Candidate text is not normalized MealCheck meal-plan JSON.", []string{"normalized_plan"})
	}
	if err := validatePlan(decodeResult.Plan); err != nil {
		return qualificationResult(QualificationStatusMealPlanTooVague, fmt.Sprintf("Candidate meal-plan JSON is not verifiable: %s.", err), missingFieldsForPlanError(err))
	}
	return qualificationResultForPlan(decodeResult.Plan, decodeResult.Canonicalized)
}

func qualificationResultForPlan(plan checker.Plan, canonicalized bool) MealPlanQualificationResult {
	status := QualificationStatusEligibleForVerification
	reason := "The content is normalized ingredient-level MealCheck JSON and can be verified."
	if planHasUnresolvedItems(plan) {
		status = QualificationStatusEligibleWithUnresolvedItems
		reason = "The content is normalized MealCheck JSON and can be verified, but unresolved foods or quantities must remain visible in the report."
	}
	return MealPlanQualificationResult{
		SchemaVersion:  "0.1",
		Status:         status,
		Reason:         reason,
		NormalizedPlan: &plan,
		Canonicalized:  canonicalized,
	}
}

func classifyCandidateMealPlanText(text string) MealPlanQualificationResult {
	lower := strings.ToLower(text)
	hasMealStructure := hasMealStructureSignal(lower)
	hasQuantity := hasQuantitySignal(lower)
	isRecipeLike := hasRecipeSignal(lower)

	if !hasMealStructure && !isRecipeLike {
		return qualificationResult(QualificationStatusNotMealPlan, "The text does not describe days, meals, recipes, or ingredient-level meal-plan content.", []string{"meal_plan_content"})
	}
	if isRecipeLike && !hasMealStructure {
		return qualificationResult(QualificationStatusRecipeOrMenuNeedsDecompose, "The text is recipe-like, but it needs to be decomposed into day, meal, ingredient, quantity, and unit fields before verification.", []string{"days", "meals", "ingredient_items"})
	}
	if !hasQuantity {
		return qualificationResult(QualificationStatusMealPlanTooVague, "The text resembles a meal plan but lacks ingredient quantities and units needed for verification.", []string{"quantities", "units"})
	}
	return qualificationResult(QualificationStatusEligibleForVerification, "The text appears to contain meal structure and ingredient quantities; a BYOK provider can attempt normalization into MealCheck JSON.", nil)
}

func classifyLocalModelCandidateMealPlanText(text string) MealPlanQualificationResult {
	classification := classifyCandidateMealPlanText(text)
	if classification.Status == QualificationStatusMealPlanTooVague && hasMealStructureSignal(strings.ToLower(text)) {
		return qualificationResult(
			QualificationStatusEligibleWithUnresolvedItems,
			"The text appears to contain one-day meal structure, but some food quantities may need to remain unresolved after local-model normalization.",
			[]string{"quantities", "units"},
		)
	}
	return classification
}

func isTerminalQualificationFailure(result MealPlanQualificationResult) bool {
	switch result.Status {
	case QualificationStatusNotMealPlan, QualificationStatusMealPlanTooVague, QualificationStatusRecipeOrMenuNeedsDecompose:
		return true
	case QualificationStatusOutsideHostedContract:
		return true
	default:
		return false
	}
}

func qualificationMessages(request MealPlanQualificationRequest) []ProviderMessage {
	system := strings.Join([]string{
		"Normalize candidate meal-plan text into MealCheck meal-plan JSON only.",
		"Return one JSON object matching schema_version 0.1.",
		mealPlanContractPromptBlock(),
		"Only use food items, quantities, units, and prep details present in the source text.",
		"If a quantity is vague, preserve it as quantity_text with resolution_status unresolved and unresolved_reason vague_quantity.",
		"Do not invent missing foods, quantities, days, meals, nutrition totals, or compliance judgments.",
		"Do not provide medical claims.",
	}, " ")
	payload := map[string]any{
		"settings":       request.Settings,
		"source_text":    sanitizeDebugArtifactText(request.Text, request.Provider.APIKey),
		"required_shape": mealPlanShapeInstructions(request.Settings.VerificationConstraints),
		"alias_rules":    mealPlanAliasRules(),
	}
	payloadJSON, _ := json.MarshalIndent(payload, "", "  ")
	return []ProviderMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: string(payloadJSON)},
	}
}

func qualificationResult(status, reason string, missingFields []string) MealPlanQualificationResult {
	return MealPlanQualificationResult{
		SchemaVersion: "0.1",
		Status:        status,
		Reason:        reason,
		MissingFields: append([]string(nil), missingFields...),
	}
}

func missingFieldsForPlanError(err error) []string {
	message := err.Error()
	switch {
	case strings.Contains(message, "schema_version"):
		return []string{"schema_version"}
	case strings.Contains(message, "plan_id"):
		return []string{"plan_id"}
	case strings.Contains(message, "days"):
		return []string{"days"}
	case strings.Contains(message, "meal"):
		return []string{"meals"}
	case strings.Contains(message, "food"):
		return []string{"food"}
	case strings.Contains(message, "quantity") || strings.Contains(message, "unit"):
		return []string{"quantities", "units"}
	default:
		return []string{"normalized_plan"}
	}
}

func planHasUnresolvedItems(plan checker.Plan) bool {
	for _, item := range plan.ShoppingList {
		if itemIsUnresolved(item) {
			return true
		}
	}
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			for _, item := range meal.Items {
				if itemIsUnresolved(item) {
					return true
				}
			}
		}
	}
	return false
}

func itemIsUnresolved(item checker.FoodItem) bool {
	return item.Quantity == nil || strings.TrimSpace(item.ResolutionStatus) == "unresolved" || strings.TrimSpace(item.UnresolvedReason) != ""
}

func hasMealStructureSignal(text string) bool {
	for _, token := range []string{"breakfast", "lunch", "dinner", "snack", "meal", "day 1", "day 2", "day 3", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func hasRecipeSignal(text string) bool {
	for _, token := range []string{"recipe", "ingredients", "instructions", "make ", "cook ", "bake ", "mix ", "stir ", "prep "} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

var quantitySignalPattern = regexp.MustCompile(`(?i)(\b\d+(\.\d+)?\b.*\b(g|gram|grams|oz|ounce|ounces|cup|cups|tbsp|tablespoon|tablespoons|tsp|teaspoon|teaspoons|slice|slices|serving|servings)\b)|(\b(g|oz|cup|cups|tbsp|tsp|slice|slices|serving|servings)\b.*\b\d+(\.\d+)?\b)`)

func hasQuantitySignal(text string) bool {
	return quantitySignalPattern.MatchString(text)
}

func appendIfMissingString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
