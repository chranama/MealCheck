package planextract

import "strings"

func localLlamaParagraphMealChunks(line string, day int, startID int, includeUnresolved bool) []localLlamaMealChunk {
	matches := localLlamaParagraphMealPattern.FindAllStringSubmatchIndex(line, -1)
	if len(matches) == 0 {
		return nil
	}
	var chunks []localLlamaMealChunk
	nextID := startID
	for index, match := range matches {
		if len(match) < 4 {
			continue
		}
		mealLabel := strings.ToLower(strings.TrimSpace(line[match[2]:match[3]]))
		mealCode := localLlamaMealCodeFromHeading(mealLabel)
		if mealCode == "" {
			continue
		}
		sectionStart := match[1]
		sectionEnd := len(line)
		if index+1 < len(matches) {
			sectionEnd = matches[index+1][0]
		}
		section := strings.TrimSpace(line[sectionStart:sectionEnd])
		if section == "" {
			continue
		}
		chunk := localLlamaMealChunk{
			Day:       day,
			MealCode:  mealCode,
			MealLabel: mealLabel,
			MealText:  strings.TrimSpace(line[match[0]:sectionEnd]),
		}
		for _, sourceText := range localLlamaParagraphSourceTexts(section, includeUnresolved) {
			item := localLlamaSourceItemFromText(nextID, day, mealCode, sourceText, includeUnresolved)
			if item.ParseStatus == "" {
				continue
			}
			chunk.Items = append(chunk.Items, item)
			nextID++
		}
		if len(chunk.Items) > 0 {
			chunks = append(chunks, chunk)
		}
	}
	return chunks
}

func localLlamaParagraphSourceTexts(section string, includeUnresolved bool) []string {
	normalized := strings.ReplaceAll(section, ";", ",")
	parts := localLlamaSplitCommaItemParts(normalized)
	sourceTexts := make([]string, 0, len(parts))
	for _, part := range parts {
		for _, phrase := range localLlamaSplitInlineAndQuantified(part) {
			sourceText, ok := localLlamaNormalizeParagraphItemPhrase(phrase)
			if !ok && includeUnresolved {
				sourceText, ok = localLlamaNormalizeUnresolvedParagraphItemPhrase(phrase)
			}
			if ok {
				sourceTexts = append(sourceTexts, sourceText)
			}
		}
	}
	return sourceTexts
}

func localLlamaNormalizeParagraphItemPhrase(phrase string) (string, bool) {
	text := strings.TrimSpace(strings.Trim(phrase, " \t\r\n.;,"))
	text = localLlamaInlineLeadingAnd.ReplaceAllString(text, "")
	if text == "" {
		return "", false
	}
	if sourceText, ok := localLlamaNormalizeInlineItemPhrase(text); ok {
		return sourceText, true
	}
	if match := localLlamaParagraphQuantityStart.FindStringIndex(text); len(match) == 2 {
		text = strings.TrimSpace(text[match[0]:])
	}
	return localLlamaNormalizeInlineItemPhrase(text)
}

func localLlamaNormalizeUnresolvedParagraphItemPhrase(phrase string) (string, bool) {
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
