package normalization

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// SourceItem is a deterministic inventory row extracted from meal-plan text.
type SourceItem struct {
	ID       int
	Day      int
	MealCode string
	Text     string
}

type DaySection struct {
	Day  int
	Text string
}

var (
	resolvedItemLinePattern = regexp.MustCompile(`(?i)^\s*(?:[-*]|\d+[.)])\s+(?:\d+(?:\.\d+)?|\d+\s*/\s*\d+|\d+\s+\d+\s*/\s*\d+)\s*(?:g|grams?|oz|ounces?|cups?|tbsp|tablespoons?|tsp|teaspoons?|slices?|servings?)\b`)
	inlineItemPattern       = regexp.MustCompile(`(?i)^\s*((?:\d+(?:\.\d+)?)|(?:\d+\s*/\s*\d+)|(?:\d+\s+\d+\s*/\s*\d+))\s+((?:g|grams?|oz|ounces?|cups?|tbsp|tablespoons?|tsp|teaspoons?|slices?|servings?)\s+)?(.+?)\s*$`)
	inlineAndItemBoundary   = regexp.MustCompile(`(?i)\s+\b(?:and|with|plus)\s+((?:\d+(?:\.\d+)?|\d+\s*/\s*\d+|\d+\s+\d+\s*/\s*\d+)\s+)`)
	inlineLeadingAnd        = regexp.MustCompile(`(?i)^\s*and\s+`)
	leadingOf               = regexp.MustCompile(`(?i)^of\s+`)
	sourceItemMarkerPattern = regexp.MustCompile(`^\s*(?:[-*]|\d+[.)])\s+`)
	dayHeadingPattern       = regexp.MustCompile(`(?i)\bday\s*([1-7])\b`)
)

func ExpectedResolvedItemCount(text string) int {
	return len(ResolvedSourceItems(text))
}

func ResolvedSourceItems(text string) []SourceItem {
	var items []SourceItem
	currentDay := 1
	currentMealCode := ""
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		isItemLine := resolvedItemLinePattern.MatchString(line)
		if !isItemLine {
			if day := DayFromHeading(trimmed); day > 0 {
				currentDay = day
			}
			if mealCode := MealCodeFromHeading(trimmed); mealCode != "" {
				currentMealCode = mealCode
			}
			inlineItems := inlineSourceItems(trimmed, currentDay, currentMealCode, len(items)+1)
			if len(inlineItems) > 0 {
				items = append(items, inlineItems...)
			}
			continue
		}
		items = append(items, SourceItem{
			ID:       len(items) + 1,
			Day:      currentDay,
			MealCode: currentMealCode,
			Text:     cleanSourceItemLine(line),
		})
	}
	return items
}

func DaySections(text string) ([]DaySection, bool) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) == 0 {
		return nil, false
	}

	var sections []DaySection
	var current strings.Builder
	currentDay := 0
	seen := map[int]bool{}
	sawDayMarker := false

	flush := func() bool {
		if currentDay == 0 {
			return true
		}
		sectionText := strings.TrimSpace(current.String())
		if sectionText == "" || ExpectedResolvedItemCount(sectionText) == 0 {
			return false
		}
		sections = append(sections, DaySection{Day: currentDay, Text: sectionText})
		seen[currentDay] = true
		current.Reset()
		return true
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if currentDay != 0 {
				current.WriteString("\n")
			}
			continue
		}
		day := DayFromHeading(trimmed)
		if day > 0 {
			sawDayMarker = true
			if currentDay == 0 {
				currentDay = day
			} else if day != currentDay {
				if seen[day] || !flush() {
					return nil, false
				}
				currentDay = day
			}
			line = RewriteDayHeading(line, 1)
		} else if currentDay == 0 {
			if ExpectedResolvedItemCount(trimmed) == 0 {
				continue
			}
			return nil, false
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	if !sawDayMarker || !flush() {
		return nil, false
	}
	return sections, len(sections) > 1
}

func RewriteDayHeading(line string, day int) string {
	return dayHeadingPattern.ReplaceAllString(line, fmt.Sprintf("Day %d", day))
}

func SourceItemsPromptBlock(text string) string {
	sourceItems := ResolvedSourceItems(text)
	if len(sourceItems) == 0 {
		return "Source meal plan text:\n" + text
	}
	var b strings.Builder
	b.WriteString("Numbered resolved source items:\n")
	for _, item := range sourceItems {
		mealCode := item.MealCode
		if mealCode == "" {
			mealCode = "infer"
		}
		fmt.Fprintf(&b, "%d | day=%d | meal_code=%s | source_text=%s\n", item.ID, item.Day, mealCode, item.Text)
	}
	return strings.TrimRight(b.String(), "\n")
}

func inlineSourceItems(line string, day int, mealCode string, startID int) []SourceItem {
	if !strings.Contains(line, ":") {
		return nil
	}
	_, remainder, found := strings.Cut(line, ":")
	if !found {
		return nil
	}
	remainder = strings.TrimSpace(remainder)
	if remainder == "" {
		return nil
	}
	var items []SourceItem
	for _, phrase := range splitInlineItemPhrases(remainder) {
		sourceText, ok := normalizeInlineItemPhrase(phrase)
		if !ok {
			continue
		}
		items = append(items, SourceItem{
			ID:       startID + len(items),
			Day:      day,
			MealCode: mealCode,
			Text:     sourceText,
		})
	}
	return items
}

func splitInlineItemPhrases(text string) []string {
	normalized := strings.ReplaceAll(text, ";", ",")
	parts := strings.Split(normalized, ",")
	phrases := make([]string, 0, len(parts))
	for _, part := range parts {
		for _, subpart := range splitInlineAndQuantified(part) {
			phrase := strings.TrimSpace(strings.Trim(subpart, "."))
			phrase = inlineLeadingAnd.ReplaceAllString(phrase, "")
			if phrase != "" {
				phrases = append(phrases, phrase)
			}
		}
	}
	return phrases
}

func splitInlineAndQuantified(part string) []string {
	remaining := strings.TrimSpace(part)
	if remaining == "" {
		return nil
	}
	var phrases []string
	for {
		matches := inlineAndItemBoundary.FindStringSubmatchIndex(remaining)
		if len(matches) == 0 {
			return append(phrases, remaining)
		}
		if left := strings.TrimSpace(remaining[:matches[0]]); left != "" {
			phrases = append(phrases, left)
		}
		remaining = strings.TrimSpace(remaining[matches[2]:])
		if remaining == "" {
			return phrases
		}
	}
}

func normalizeInlineItemPhrase(phrase string) (string, bool) {
	matches := inlineItemPattern.FindStringSubmatch(strings.TrimSpace(phrase))
	if len(matches) != 4 {
		return "", false
	}
	quantity := strings.Join(strings.Fields(matches[1]), " ")
	unit := strings.TrimSpace(matches[2])
	food := strings.TrimSpace(matches[3])
	if quantity == "" || food == "" {
		return "", false
	}
	if unit != "" {
		food = strings.TrimSpace(leadingOf.ReplaceAllString(food, ""))
	}
	if unit == "" {
		unit = "serving"
	}
	unit = NormalizeSourceUnit(unit)
	return strings.TrimSpace(quantity + " " + unit + " " + food), true
}

func NormalizeSourceUnit(unit string) string {
	normalized := strings.ToLower(strings.TrimSpace(unit))
	switch normalized {
	case "gram", "grams":
		return "g"
	case "ounce", "ounces":
		return "oz"
	case "cups":
		return "cup"
	case "tablespoon", "tablespoons":
		return "tbsp"
	case "teaspoon", "teaspoons":
		return "tsp"
	case "slices":
		return "slice"
	case "servings":
		return "serving"
	default:
		return normalized
	}
}

func DayFromHeading(line string) int {
	matches := dayHeadingPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return 0
	}
	day, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return day
}

func MealCodeFromHeading(line string) string {
	heading := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(line, ":")))
	switch {
	case strings.Contains(heading, "breakfast"):
		return "b"
	case strings.Contains(heading, "morning snack"):
		return "m"
	case strings.Contains(heading, "lunch"):
		return "l"
	case strings.Contains(heading, "afternoon snack"):
		return "a"
	case strings.Contains(heading, "dinner"):
		return "d"
	case strings.Contains(heading, "evening snack"):
		return "e"
	case strings.Contains(heading, "snack"):
		return "s"
	default:
		return ""
	}
}

func cleanSourceItemLine(line string) string {
	return strings.TrimSpace(sourceItemMarkerPattern.ReplaceAllString(line, ""))
}
