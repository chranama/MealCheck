package planextract

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/workflow/checker"
)

var (
	localModelAnyDayPattern      = regexp.MustCompile(`(?i)\bday\s*([0-9]+)\b`)
	localModelWeeklyPattern      = regexp.MustCompile(`(?i)\b(?:multi[- ]?day|weekly|week(?:long)?\b|[2-9][-\s]?day\b|(?:[2-9]|[1-9][0-9]+)\s+days?\b|seven[- ]?day\b)`)
	localModelRecipePattern      = regexp.MustCompile(`(?i)\b(?:recipe|instructions?|directions?|preheat|simmer|cook\s+until|mix\s+until|stir\s+until)\b`)
	localModelGroceryListPattern = regexp.MustCompile(`(?i)\b(?:grocery|shopping)\s+list\b|\binventory\b`)
)

type LocalModelInputContractError struct {
	Qualification MealPlanQualificationResult
}

func (e LocalModelInputContractError) Error() string {
	if e.Qualification.Reason != "" {
		return e.Qualification.Reason
	}
	return "candidate_text is outside the hosted local_model input contract"
}

type localModelInputContractError = LocalModelInputContractError

type QualificationRejectionError struct {
	Qualification MealPlanQualificationResult
}

func (e QualificationRejectionError) Error() string {
	if e.Qualification.Reason != "" {
		return e.Qualification.Reason
	}
	return "candidate text is not ready for verification"
}

type qualificationRejectionError = QualificationRejectionError

func normalizeLocalModelSettings(settings checker.Settings) checker.Settings {
	if settings.VerificationConstraints.Days == 0 {
		settings.VerificationConstraints.Days = 1
	}
	return settings
}

func NormalizeLocalModelSettings(settings checker.Settings) checker.Settings {
	return normalizeLocalModelSettings(settings)
}

func ValidateLocalModelSettings(settings checker.Settings) error {
	if settings.VerificationConstraints.Days != 1 {
		return fmt.Errorf("hosted local_model requires settings verification_constraints days to be exactly 1")
	}
	return nil
}

func validateLocalModelInputContract(config Config, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("candidate_text is required for local_model")
	}
	if err := validateLocalModelOneDayText(text); err != nil {
		return err
	}
	if localModelRecipePattern.MatchString(text) {
		return localModelInputContractError{Qualification: qualificationResult(
			QualificationStatusRecipeOrMenuNeedsDecompose,
			"MealCheck needs one day of meal-plan text, not a recipe or cooking instructions. Rewrite it as meal labels with ingredient amounts before verification.",
			[]string{"meals", "ingredient_items", "quantities", "units"},
		)}
	}
	if localModelGroceryListPattern.MatchString(text) {
		return localModelInputContractError{Qualification: qualificationResult(
			QualificationStatusOutsideHostedContract,
			"MealCheck needs one day of meal-plan text, not a grocery list, shopping list, or source inventory. Split inventories into meal-labeled one-day text before verification.",
			[]string{"meals", "source_item_limit"},
		)}
	}
	chunks := localLlamaExtractionMealChunks(text)
	itemCount := 0
	for _, chunk := range chunks {
		if strings.TrimSpace(chunk.MealCode) == "" || chunk.MealCode == "infer" {
			return localModelInputContractError{Qualification: qualificationResult(
				QualificationStatusMealPlanTooVague,
				"Each source food item must be attached to a meal label such as breakfast, lunch, dinner, or snack.",
				[]string{"meals"},
			)}
		}
		itemCount += len(chunk.Items)
	}
	if itemCount == 0 {
		return localModelInputContractError{Qualification: qualificationResult(
			QualificationStatusMealPlanTooVague,
			"MealCheck needs one day of meal-labeled ingredient text with at least one source food item.",
			[]string{"ingredient_items", "meals"},
		)}
	}
	itemLimit := config.LocalModelMaxSourceItems
	if itemLimit > 0 {
		if itemCount > itemLimit {
			return localModelInputContractError{Qualification: qualificationResult(
				QualificationStatusOutsideHostedContract,
				fmt.Sprintf("MealCheck found %d source food items. The hosted one-day local-model path accepts at most %d; split long inventories before verification.", itemCount, itemLimit),
				[]string{"source_item_limit"},
			)}
		}
	}
	return nil
}

func ValidateLocalModelInputContract(config Config, text string) error {
	return validateLocalModelInputContract(config, text)
}

func validateLocalModelMealPlanPreflight(config Config, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("candidate_text is required for local_model")
	}
	if err := validateLocalModelExplicitContractMarkers(text); err != nil {
		return err
	}
	qualification := core.ClassifyLocalModelCandidateMealPlanText(text)
	if core.IsTerminalQualificationFailure(qualification) {
		if qualification.Status == QualificationStatusNotMealPlan && localModelHasSourceItems(text) {
			return validateLocalModelInputContract(config, text)
		}
		return qualificationRejectionError{Qualification: qualification}
	}
	return validateLocalModelInputContract(config, text)
}

func ValidateLocalModelMealPlanPreflight(config Config, text string) error {
	return validateLocalModelMealPlanPreflight(config, text)
}

func validateLocalModelExplicitContractMarkers(text string) error {
	if err := validateLocalModelOneDayText(text); err != nil {
		return err
	}
	if localModelRecipePattern.MatchString(text) {
		return localModelInputContractError{Qualification: qualificationResult(
			QualificationStatusRecipeOrMenuNeedsDecompose,
			"MealCheck needs one day of meal-plan text, not a recipe or cooking instructions. Rewrite it as meal labels with ingredient amounts before verification.",
			[]string{"meals", "ingredient_items", "quantities", "units"},
		)}
	}
	if localModelGroceryListPattern.MatchString(text) {
		return localModelInputContractError{Qualification: qualificationResult(
			QualificationStatusOutsideHostedContract,
			"MealCheck needs one day of meal-plan text, not a grocery list, shopping list, or source inventory. Split inventories into meal-labeled one-day text before verification.",
			[]string{"meals", "source_item_limit"},
		)}
	}
	return nil
}

func localModelHasSourceItems(text string) bool {
	for _, chunk := range localLlamaExtractionMealChunks(text) {
		if len(chunk.Items) > 0 {
			return true
		}
	}
	return false
}

func validateLocalModelOneDayText(text string) error {
	dayNumbers := localModelDayNumbers(text)
	if len(dayNumbers) > 0 {
		for _, day := range dayNumbers {
			if day != 1 {
				return localModelInputContractError{Qualification: qualificationResult(
					QualificationStatusOutsideHostedContract,
					fmt.Sprintf("MealCheck checks one day at a time in the hosted local-model path. Remove Day %d content or submit that day separately.", day),
					[]string{"one_day"},
				)}
			}
		}
	}
	if localModelWeeklyPattern.MatchString(text) {
		return localModelInputContractError{Qualification: qualificationResult(
			QualificationStatusOutsideHostedContract,
			"MealCheck checks one day at a time in the hosted local-model path. Weekly and multi-day plans must be split before verification.",
			[]string{"one_day"},
		)}
	}
	return nil
}

func localModelDayNumbers(text string) []int {
	matches := localModelAnyDayPattern.FindAllStringSubmatch(text, -1)
	seen := map[int]bool{}
	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		var day int
		if _, err := fmt.Sscanf(match[1], "%d", &day); err != nil {
			continue
		}
		seen[day] = true
	}
	days := make([]int, 0, len(seen))
	for day := range seen {
		days = append(days, day)
	}
	sort.Ints(days)
	return days
}
