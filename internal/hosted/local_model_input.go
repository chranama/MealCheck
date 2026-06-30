package hosted

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/chranama/MealCheck/internal/checker"
)

var (
	localModelAnyDayPattern      = regexp.MustCompile(`(?i)\bday\s*([0-9]+)\b`)
	localModelWeeklyPattern      = regexp.MustCompile(`(?i)\b(?:multi[- ]?day|weekly|week(?:long)?\b|[2-9][-\s]?day\b|(?:[2-9]|[1-9][0-9]+)\s+days?\b|seven[- ]?day\b)`)
	localModelRecipePattern      = regexp.MustCompile(`(?i)\b(?:recipe|instructions?|directions?|preheat|simmer|cook\s+until|mix\s+until|stir\s+until)\b`)
	localModelGroceryListPattern = regexp.MustCompile(`(?i)\b(?:grocery|shopping)\s+list\b`)
)

func normalizeLocalModelSettings(settings checker.Settings) checker.Settings {
	if settings.VerificationConstraints.Days == 0 {
		settings.VerificationConstraints.Days = 1
	}
	return settings
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
		return fmt.Errorf("candidate_text must be a one-day meal plan, not a recipe or cooking instructions")
	}
	if localModelGroceryListPattern.MatchString(text) {
		return fmt.Errorf("candidate_text must be a meal plan, not a grocery or shopping list")
	}
	chunks := localLlamaExtractionMealChunks(text)
	itemCount := 0
	for _, chunk := range chunks {
		if strings.TrimSpace(chunk.MealCode) == "" || chunk.MealCode == "infer" {
			return fmt.Errorf("candidate_text must identify each source food item under a meal label such as breakfast, lunch, dinner, or snack")
		}
		itemCount += len(chunk.Items)
	}
	if itemCount == 0 {
		return fmt.Errorf("candidate_text must identify at least one source food item with a meal label")
	}
	itemLimit := config.LocalModelMaxSourceItems
	if itemLimit > 0 {
		if itemCount > itemLimit {
			return fmt.Errorf("candidate_text includes %d source food item(s); hosted local_model accepts at most %d", itemCount, itemLimit)
		}
	}
	return nil
}

func validateLocalModelOneDayText(text string) error {
	dayNumbers := localModelDayNumbers(text)
	if len(dayNumbers) > 0 {
		for _, day := range dayNumbers {
			if day != 1 {
				return fmt.Errorf("candidate_text must describe one day only; remove Day %d content", day)
			}
		}
	}
	if localModelWeeklyPattern.MatchString(text) {
		return fmt.Errorf("candidate_text must describe one day only; weekly and multi-day plans are outside the hosted local_model contract")
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
