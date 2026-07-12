package planextract

import (
	"regexp"
	"strconv"
	"strings"
)

type localLlamaSourceItem struct {
	ID          int
	Day         int
	MealCode    string
	Text        string
	ParseStatus localLlamaSourceParseStatus
}

type localLlamaSourceParseStatus string

const (
	localLlamaSourceResolved        localLlamaSourceParseStatus = "resolved"
	localLlamaSourceNeedsModelParse localLlamaSourceParseStatus = "needs_model_parse"
)

type localLlamaMealChunk struct {
	Day       int
	MealCode  string
	MealLabel string
	MealText  string
	Items     []localLlamaSourceItem
}

// LocalLlamaSourceItem is the deterministic source-item inventory used before
// compact local-model extraction.
type LocalLlamaSourceItem struct {
	ID          int
	Day         int
	MealCode    string
	Text        string
	ParseStatus string
}

var (
	localLlamaResolvedItemLinePattern           = regexp.MustCompile(`(?i)^\s*(?:[-*]|\d+[.)])\s+(?:\d+(?:\.\d+)?|\d+\s*/\s*\d+|\d+\s+\d+\s*/\s*\d+)\s*(?:g|grams?|oz|ounces?|cups?|tbsp|tablespoons?|tsp|teaspoons?|slices?|servings?)\b`)
	localLlamaAnyItemLinePattern                = regexp.MustCompile(`^\s*(?:[-*]|\d+[.)])\s+(.+?)\s*$`)
	localLlamaInlineItemPattern                 = regexp.MustCompile(`(?i)^\s*((?:\d+(?:\.\d+)?)|(?:\d+\s*/\s*\d+)|(?:\d+\s+\d+\s*/\s*\d+))\s+((?:g|grams?|oz|ounces?|cups?|tbsp|tablespoons?|tsp|teaspoons?|slices?|servings?)\s+)?(.+?)\s*$`)
	localLlamaReverseItemPattern                = regexp.MustCompile(`(?i)^\s*(.+?)\s*(?:,|-|\()\s*((?:\d+(?:\.\d+)?)|(?:\d+\s*/\s*\d+)|(?:\d+\s+\d+\s*/\s*\d+))\s+(g|grams?|oz|ounces?|cups?|tbsp|tablespoons?|tsp|teaspoons?|slices?|servings?)\)?\s*$`)
	localLlamaMeasurementOnlyPattern            = regexp.MustCompile(`(?i)^\s*(?:\d+(?:\.\d+)?|\d+\s*/\s*\d+|\d+\s+\d+\s*/\s*\d+)\s+(?:g|grams?|oz|ounces?|cups?|tbsp|tablespoons?|tsp|teaspoons?|slices?|servings?)\s*$`)
	localLlamaUnsupportedInlineItemPattern      = regexp.MustCompile(`(?i)^\s*((?:\d+(?:\.\d+)?)|(?:\d+\s*/\s*\d+)|(?:\d+\s+\d+\s*/\s*\d+))\s+(bowls?|plates?|handfuls?|scoops?|packets?|packages?|cans?|jars?|bottles?|loaves|loaf|pieces?|wedges?|bars?|containers?|cartons?|boxes?|bags?)\s+(.+?)\s*$`)
	localLlamaUnsupportedReverseItemPattern     = regexp.MustCompile(`(?i)^\s*(.+?)\s*(?:,|-|\()\s*((?:\d+(?:\.\d+)?)|(?:\d+\s*/\s*\d+)|(?:\d+\s+\d+\s*/\s*\d+))\s+(bowls?|plates?|handfuls?|scoops?|packets?|packages?|cans?|jars?|bottles?|loaves|loaf|pieces?|wedges?|bars?|containers?|cartons?|boxes?|bags?)\)?\s*$`)
	localLlamaUnsupportedMeasurementOnlyPattern = regexp.MustCompile(`(?i)^\s*(?:\d+(?:\.\d+)?|\d+\s*/\s*\d+|\d+\s+\d+\s*/\s*\d+)\s+(?:bowls?|plates?|handfuls?|scoops?|packets?|packages?|cans?|jars?|bottles?|loaves|loaf|pieces?|wedges?|bars?|containers?|cartons?|boxes?|bags?)\s*$`)
	localLlamaInlineAndItemBoundary             = regexp.MustCompile(`(?i)\s+\b(?:and|with|plus)\s+((?:\d+(?:\.\d+)?|\d+\s*/\s*\d+|\d+\s+\d+\s*/\s*\d+)\s+)`)
	localLlamaInlineLeadingAnd                  = regexp.MustCompile(`(?i)^\s*(?:and|with|plus)\s+`)
	localLlamaMealPhrasePrefix                  = regexp.MustCompile(`(?i)^\s*(?:i\s+(?:will\s+)?(?:have|had|ate|eat)|will\s+have|had|have|ate|eat|includes?|include|was|is|were|are)\s+`)
	localLlamaTrailingMealVerb                  = regexp.MustCompile(`(?i)\b(?:includes?|include|was|is|were|are|has|had|have)\s*$`)
	localLlamaLeadingOf                         = regexp.MustCompile(`(?i)^of\s+`)
	localLlamaSourceItemMarkerPattern           = regexp.MustCompile(`^\s*(?:[-*]|\d+[.)])\s+`)
	localLlamaDayHeadingPattern                 = regexp.MustCompile(`(?i)\bday\s*([1-7])\b`)
	localLlamaParagraphMealPattern              = regexp.MustCompile(`(?i)\b(?:for|at|as|my|the)?\s*(morning snack|afternoon snack|evening snack|breakfast|lunch|dinner|snack)\b`)
	localLlamaParagraphQuantityStart            = regexp.MustCompile(`(?i)(?:\d+(?:\.\d+)?|\d+\s*/\s*\d+|\d+\s+\d+\s*/\s*\d+)`)
)

func localLlamaExpectedResolvedItemCount(text string) int {
	return len(localLlamaResolvedSourceItems(text))
}
func localLlamaExpectedExtractionItemCount(text string) int {
	return len(localLlamaExtractionSourceItems(text))
}

// LocalLlamaExpectedResolvedItemCount returns the number of deterministic
// source items MealCheck expects the local model to preserve.
func LocalLlamaExpectedResolvedItemCount(text string) int {
	return localLlamaExpectedResolvedItemCount(text)
}

func LocalLlamaExpectedExtractionItemCount(text string) int {
	return localLlamaExpectedExtractionItemCount(text)
}

// LocalLlamaResolvedSourceItems returns the deterministic source-item inventory
// used in local-model prompts.
func LocalLlamaResolvedSourceItems(text string) []LocalLlamaSourceItem {
	internal := localLlamaResolvedSourceItems(text)
	items := make([]LocalLlamaSourceItem, 0, len(internal))
	for _, item := range internal {
		items = append(items, LocalLlamaSourceItem{
			ID:          item.ID,
			Day:         item.Day,
			MealCode:    item.MealCode,
			Text:        item.Text,
			ParseStatus: string(item.ParseStatus),
		})
	}
	return items
}
func localLlamaResolvedSourceItems(text string) []localLlamaSourceItem {
	return localLlamaSourceItems(text, false)
}
func localLlamaExtractionSourceItems(text string) []localLlamaSourceItem {
	return localLlamaSourceItems(text, true)
}
func localLlamaSourceItems(text string, includeUnresolved bool) []localLlamaSourceItem {
	chunks := localLlamaMealChunks(text, includeUnresolved)
	var items []localLlamaSourceItem
	for _, chunk := range chunks {
		items = append(items, chunk.Items...)
	}
	return items
}
func localLlamaExtractionMealChunks(text string) []localLlamaMealChunk {
	return localLlamaMealChunks(text, true)
}
func localLlamaMealChunks(text string, includeUnresolved bool) []localLlamaMealChunk {
	var chunks []localLlamaMealChunk
	currentDay := 1
	currentMealCode := ""
	currentChunkIndex := -1
	nextID := 1
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		isItemLine := localLlamaResolvedItemLinePattern.MatchString(line)
		if !isItemLine {
			if day := localLlamaDayFromHeading(trimmed); day > 0 {
				currentDay = day
			}
			if mealCode := localLlamaMealCodeFromHeading(trimmed); mealCode != "" {
				currentMealCode = mealCode
				currentChunkIndex = localLlamaEnsureMealChunk(&chunks, currentDay, currentMealCode, trimmed)
			}
			inlineItems := localLlamaInlineSourceItems(trimmed, currentDay, currentMealCode, nextID, includeUnresolved)
			paragraphChunks := localLlamaParagraphMealChunks(trimmed, currentDay, nextID, includeUnresolved)
			if localLlamaMealChunkItemCount(paragraphChunks) > len(inlineItems) {
				chunks = append(chunks, paragraphChunks...)
				nextID += localLlamaMealChunkItemCount(paragraphChunks)
				if len(paragraphChunks) > 0 {
					last := paragraphChunks[len(paragraphChunks)-1]
					currentDay = last.Day
					currentMealCode = last.MealCode
					currentChunkIndex = -1
				}
				continue
			}
			if len(inlineItems) > 0 {
				if currentMealCode == "" {
					currentMealCode = "infer"
				}
				currentChunkIndex = localLlamaEnsureMealChunk(&chunks, currentDay, currentMealCode, trimmed)
				chunks[currentChunkIndex].Items = append(chunks[currentChunkIndex].Items, inlineItems...)
				nextID += len(inlineItems)
				continue
			}
			if len(paragraphChunks) > 0 {
				chunks = append(chunks, paragraphChunks...)
				nextID += localLlamaMealChunkItemCount(paragraphChunks)
				last := paragraphChunks[len(paragraphChunks)-1]
				currentDay = last.Day
				currentMealCode = last.MealCode
				currentChunkIndex = -1
				continue
			}
			if includeUnresolved {
				if text, ok := localLlamaUnresolvedItemLine(line); ok {
					if currentMealCode == "" {
						currentMealCode = "infer"
					}
					currentChunkIndex = localLlamaEnsureMealChunk(&chunks, currentDay, currentMealCode, trimmed)
					chunks[currentChunkIndex].Items = append(chunks[currentChunkIndex].Items, localLlamaSourceItem{
						ID:          nextID,
						Day:         currentDay,
						MealCode:    currentMealCode,
						Text:        text,
						ParseStatus: localLlamaSourceNeedsModelParse,
					})
					nextID++
				}
			}
			continue
		}
		if currentMealCode == "" {
			currentMealCode = "infer"
		}
		currentChunkIndex = localLlamaEnsureMealChunk(&chunks, currentDay, currentMealCode, trimmed)
		cleaned := localLlamaCleanSourceItemLine(line)
		item := localLlamaSourceItemFromText(nextID, currentDay, currentMealCode, cleaned, includeUnresolved)
		if item.ParseStatus != "" {
			chunks[currentChunkIndex].Items = append(chunks[currentChunkIndex].Items, item)
			nextID++
		}
	}
	return localLlamaNonEmptyMealChunks(chunks)
}
func localLlamaEnsureMealChunk(chunks *[]localLlamaMealChunk, day int, mealCode string, mealText string) int {
	if mealCode == "" {
		mealCode = "infer"
	}
	for index := len(*chunks) - 1; index >= 0; index-- {
		chunk := &(*chunks)[index]
		if chunk.Day == day && chunk.MealCode == mealCode {
			localLlamaAppendMealText(chunk, mealText)
			return index
		}
	}
	mealLabel, _ := localLlamaMealName(mealCode)
	if mealLabel == "" {
		mealLabel = mealCode
	}
	*chunks = append(*chunks, localLlamaMealChunk{
		Day:       day,
		MealCode:  mealCode,
		MealLabel: mealLabel,
		MealText:  strings.TrimSpace(mealText),
	})
	return len(*chunks) - 1
}
func localLlamaAppendMealText(chunk *localLlamaMealChunk, mealText string) {
	mealText = strings.TrimSpace(mealText)
	if mealText == "" {
		return
	}
	if chunk.MealText == "" {
		chunk.MealText = mealText
		return
	}
	if strings.Contains(chunk.MealText, mealText) {
		return
	}
	chunk.MealText = strings.TrimSpace(chunk.MealText + "\n" + mealText)
}
func localLlamaNonEmptyMealChunks(chunks []localLlamaMealChunk) []localLlamaMealChunk {
	filtered := make([]localLlamaMealChunk, 0, len(chunks))
	for _, chunk := range chunks {
		if len(chunk.Items) > 0 {
			filtered = append(filtered, chunk)
		}
	}
	return filtered
}
func localLlamaMealChunkItemCount(chunks []localLlamaMealChunk) int {
	count := 0
	for _, chunk := range chunks {
		count += len(chunk.Items)
	}
	return count
}
func localLlamaSourceItemFromText(id int, day int, mealCode string, sourceText string, includeUnresolved bool) localLlamaSourceItem {
	sourceText = strings.TrimSpace(sourceText)
	if sourceText == "" {
		return localLlamaSourceItem{}
	}
	status := localLlamaSourceNeedsModelParse
	if localLlamaParseSourceMeasurement(sourceText).Status == "parsed" {
		status = localLlamaSourceResolved
	} else if !includeUnresolved {
		return localLlamaSourceItem{}
	}
	return localLlamaSourceItem{
		ID:          id,
		Day:         day,
		MealCode:    mealCode,
		Text:        sourceText,
		ParseStatus: status,
	}
}
func localLlamaDayFromHeading(line string) int {
	matches := localLlamaDayHeadingPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return 0
	}
	day, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return day
}
func localLlamaMealCodeFromHeading(line string) string {
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
func localLlamaCleanSourceItemLine(line string) string {
	return strings.TrimSpace(localLlamaSourceItemMarkerPattern.ReplaceAllString(line, ""))
}
