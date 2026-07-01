package core

import (
	"regexp"
	"strings"

	"github.com/chranama/MealCheck/internal/workflow/checker"
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

func QualificationResult(status, reason string, missingFields []string) MealPlanQualificationResult {
	return MealPlanQualificationResult{
		SchemaVersion: "0.1",
		Status:        status,
		Reason:        reason,
		MissingFields: missingFields,
		ProviderUsed:  false,
	}
}

func ClassifyCandidateMealPlanText(text string) MealPlanQualificationResult {
	lower := strings.ToLower(text)
	hasMealStructure := hasMealStructureSignal(lower)
	hasQuantity := hasQuantitySignal(lower)
	isRecipeLike := hasRecipeSignal(lower)

	if !hasMealStructure && !isRecipeLike {
		return QualificationResult(QualificationStatusNotMealPlan, "The text does not describe days, meals, recipes, or ingredient-level meal-plan content.", []string{"meal_plan_content"})
	}
	if isRecipeLike && !hasMealStructure {
		return QualificationResult(QualificationStatusRecipeOrMenuNeedsDecompose, "The text is recipe-like, but it needs to be decomposed into day, meal, ingredient, quantity, and unit fields before verification.", []string{"days", "meals", "ingredient_items"})
	}
	if !hasQuantity {
		return QualificationResult(QualificationStatusMealPlanTooVague, "The text resembles a meal plan but lacks ingredient quantities and units needed for verification.", []string{"quantities", "units"})
	}
	return QualificationResult(QualificationStatusEligibleForVerification, "The text appears to contain meal structure and ingredient quantities; a BYOK provider can attempt normalization into MealCheck JSON.", nil)
}

func ClassifyLocalModelCandidateMealPlanText(text string) MealPlanQualificationResult {
	classification := ClassifyCandidateMealPlanText(text)
	if classification.Status == QualificationStatusMealPlanTooVague && hasMealStructureSignal(strings.ToLower(text)) {
		return QualificationResult(
			QualificationStatusEligibleWithUnresolvedItems,
			"The text appears to contain one-day meal structure, but some food quantities may need to remain unresolved after local-model normalization.",
			[]string{"quantities", "units"},
		)
	}
	return classification
}

func IsTerminalQualificationFailure(result MealPlanQualificationResult) bool {
	switch result.Status {
	case QualificationStatusNotMealPlan, QualificationStatusMealPlanTooVague, QualificationStatusRecipeOrMenuNeedsDecompose:
		return true
	case QualificationStatusOutsideHostedContract:
		return true
	default:
		return false
	}
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
