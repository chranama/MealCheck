package planextract

import "strings"

func localLlamaInlineSourceItems(line string, day int, mealCode string, startID int, includeUnresolved bool) []localLlamaSourceItem {
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
	var items []localLlamaSourceItem
	for _, phrase := range localLlamaSplitInlineItemPhrases(remainder) {
		sourceText, ok := localLlamaNormalizeInlineItemPhrase(phrase)
		if !ok && includeUnresolved {
			sourceText, ok = localLlamaNormalizeUnresolvedInlineItemPhrase(phrase)
		}
		if !ok {
			continue
		}
		item := localLlamaSourceItemFromText(startID+len(items), day, mealCode, sourceText, includeUnresolved)
		if item.ParseStatus != "" {
			items = append(items, item)
		}
	}
	return items
}

func localLlamaNormalizeUnresolvedInlineItemPhrase(phrase string) (string, bool) {
	text := strings.TrimSpace(strings.Trim(phrase, " \t\r\n.;,"))
	text = localLlamaInlineLeadingAnd.ReplaceAllString(text, "")
	text = localLlamaCleanMealContextPrefix(text)
	if text == "" {
		return "", false
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, ":") || strings.HasPrefix(lower, "day ") {
		return "", false
	}
	return text, true
}

func localLlamaUnresolvedItemLine(line string) (string, bool) {
	matches := localLlamaAnyItemLinePattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return "", false
	}
	text := strings.TrimSpace(strings.Trim(matches[1], " \t\r\n.;,"))
	if text == "" || strings.Contains(text, ":") {
		return "", false
	}
	return text, true
}

func localLlamaSplitInlineItemPhrases(text string) []string {
	normalized := strings.ReplaceAll(text, ";", ",")
	parts := localLlamaSplitCommaItemParts(normalized)
	phrases := make([]string, 0, len(parts))
	for _, part := range parts {
		for _, subpart := range localLlamaSplitInlineAndQuantified(part) {
			phrase := strings.TrimSpace(strings.Trim(subpart, "."))
			phrase = localLlamaInlineLeadingAnd.ReplaceAllString(phrase, "")
			if phrase != "" {
				phrases = append(phrases, phrase)
			}
		}
	}
	return phrases
}

func localLlamaSplitInlineAndQuantified(part string) []string {
	remaining := strings.TrimSpace(part)
	if remaining == "" {
		return nil
	}
	var phrases []string
	for {
		matches := localLlamaInlineAndItemBoundary.FindStringSubmatchIndex(remaining)
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

func localLlamaSplitCommaItemParts(text string) []string {
	rawParts := strings.Split(text, ",")
	parts := make([]string, 0, len(rawParts))
	for index := 0; index < len(rawParts); index++ {
		part := strings.TrimSpace(rawParts[index])
		if part == "" {
			continue
		}
		if index+1 < len(rawParts) {
			next := strings.TrimSpace(rawParts[index+1])
			if localLlamaShouldMergeReverseMeasurementParts(part, next) {
				parts = append(parts, part+", "+next)
				index++
				continue
			}
		}
		parts = append(parts, part)
	}
	return parts
}

func localLlamaShouldMergeReverseMeasurementParts(foodPart string, measurementPart string) bool {
	if foodPart == "" || measurementPart == "" {
		return false
	}
	if localLlamaParagraphQuantityStart.MatchString(foodPart) {
		return false
	}
	measurementPart = strings.Trim(measurementPart, " .;")
	return localLlamaMeasurementOnlyPattern.MatchString(measurementPart) || localLlamaUnsupportedMeasurementOnlyPattern.MatchString(measurementPart)
}

func localLlamaNormalizeInlineItemPhrase(phrase string) (string, bool) {
	if sourceText, ok := localLlamaNormalizeReverseItemPhrase(phrase); ok {
		return sourceText, true
	}
	if sourceText, ok := localLlamaNormalizeUnsupportedInlineItemPhrase(phrase); ok {
		return sourceText, true
	}
	matches := localLlamaInlineItemPattern.FindStringSubmatch(strings.TrimSpace(phrase))
	if len(matches) != 4 {
		return "", false
	}
	quantity := strings.Join(strings.Fields(matches[1]), " ")
	unit := strings.TrimSpace(matches[2])
	food := localLlamaCleanMealContextPrefix(strings.TrimSpace(matches[3]))
	if quantity == "" || food == "" {
		return "", false
	}
	if unit != "" {
		food = strings.TrimSpace(localLlamaLeadingOf.ReplaceAllString(food, ""))
	}
	if unit == "" {
		unit = "serving"
	}
	unit = localLlamaNormalizeSourceUnit(unit)
	return strings.TrimSpace(quantity + " " + unit + " " + food), true
}

func localLlamaNormalizeReverseItemPhrase(phrase string) (string, bool) {
	phrase = strings.TrimSpace(strings.Trim(phrase, " \t\r\n.;"))
	matches := localLlamaReverseItemPattern.FindStringSubmatch(phrase)
	if len(matches) != 4 {
		return localLlamaNormalizeUnsupportedReverseItemPhrase(phrase)
	}
	food := localLlamaCleanMealContextPrefix(matches[1])
	food = strings.TrimSpace(strings.Trim(food, " \t\r\n.;:-()"))
	if food == "" {
		return "", false
	}
	quantity := strings.Join(strings.Fields(matches[2]), " ")
	unit := localLlamaNormalizeSourceUnit(matches[3])
	return strings.TrimSpace(quantity + " " + unit + " " + food), true
}

func localLlamaNormalizeUnsupportedInlineItemPhrase(phrase string) (string, bool) {
	matches := localLlamaUnsupportedInlineItemPattern.FindStringSubmatch(strings.TrimSpace(strings.Trim(phrase, " \t\r\n.;")))
	if len(matches) != 4 {
		return "", false
	}
	quantity := strings.Join(strings.Fields(matches[1]), " ")
	unit := localLlamaNormalizeUnsupportedSourceUnit(matches[2])
	food := localLlamaCleanMealContextPrefix(strings.TrimSpace(matches[3]))
	food = strings.TrimSpace(localLlamaLeadingOf.ReplaceAllString(food, ""))
	if quantity == "" || unit == "" || food == "" {
		return "", false
	}
	return strings.TrimSpace(quantity + " " + unit + " " + food), true
}

func localLlamaNormalizeUnsupportedReverseItemPhrase(phrase string) (string, bool) {
	matches := localLlamaUnsupportedReverseItemPattern.FindStringSubmatch(phrase)
	if len(matches) != 4 {
		return "", false
	}
	food := localLlamaCleanMealContextPrefix(matches[1])
	food = strings.TrimSpace(strings.Trim(food, " \t\r\n.;:-()"))
	if food == "" {
		return "", false
	}
	quantity := strings.Join(strings.Fields(matches[2]), " ")
	unit := localLlamaNormalizeUnsupportedSourceUnit(matches[3])
	if quantity == "" || unit == "" {
		return "", false
	}
	return strings.TrimSpace(quantity + " " + unit + " " + food), true
}

func localLlamaCleanMealContextPrefix(text string) string {
	text = strings.TrimSpace(text)
	for {
		cleaned := strings.TrimSpace(localLlamaMealPhrasePrefix.ReplaceAllString(text, ""))
		cleaned = strings.TrimSpace(localLlamaTrailingMealVerb.ReplaceAllString(cleaned, ""))
		if cleaned == text {
			break
		}
		text = cleaned
	}
	return strings.TrimSpace(text)
}

func localLlamaNormalizeSourceUnit(unit string) string {
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

func localLlamaNormalizeUnsupportedSourceUnit(unit string) string {
	normalized := strings.ToLower(strings.TrimSpace(unit))
	switch normalized {
	case "bowls":
		return "bowl"
	case "plates":
		return "plate"
	case "handfuls":
		return "handful"
	case "scoops":
		return "scoop"
	case "packets":
		return "packet"
	case "packages":
		return "package"
	case "cans":
		return "can"
	case "jars":
		return "jar"
	case "bottles":
		return "bottle"
	case "loaves":
		return "loaf"
	case "pieces":
		return "piece"
	case "wedges":
		return "wedge"
	case "bars":
		return "bar"
	case "containers":
		return "container"
	case "cartons":
		return "carton"
	case "boxes":
		return "box"
	case "bags":
		return "bag"
	default:
		return normalized
	}
}
