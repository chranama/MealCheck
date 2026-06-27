package checker

import (
	"regexp"
	"strings"
)

const (
	unresolvedUnknownFood                    = "unknown_food"
	unresolvedVagueQuantity                  = "vague_quantity"
	unresolvedUnsupportedUnit                = "unsupported_unit"
	unresolvedAmbiguousFood                  = "ambiguous_food"
	unresolvedComposedFoodNeedsDecomposition = "composed_food_needs_decomposition"
	unresolvedRestaurantOrBrandedFood        = "restaurant_or_branded_food"
	unresolvedPreparationUnclear             = "preparation_unclear"
	unresolvedNonFoodText                    = "non_food_text"
)

type lookupFilterResult struct {
	LookupAllowed bool
	Reason        string
	Query         string
}

var lookupTokenPattern = regexp.MustCompile(`[a-z0-9]+`)

var broadOneWordFoods = map[string]bool{
	"bread":    true,
	"cereal":   true,
	"cheese":   true,
	"coffee":   true,
	"fish":     true,
	"juice":    true,
	"meat":     true,
	"pasta":    true,
	"rice":     true,
	"sandwich": true,
	"sauce":    true,
	"soup":     true,
	"tea":      true,
	"water":    true,
}

func filterFallbackLookupCandidate(item FoodItem) lookupFilterResult {
	food := strings.TrimSpace(item.Food)
	if food == "" {
		return blockLookup(unresolvedNonFoodText)
	}
	if item.Quantity == nil {
		return blockLookup(unresolvedVagueQuantity)
	}

	text := lookupFilterText(item)
	tokens := lookupFilterTokens(text)
	switch {
	case hasAnyToken(tokens, "medicine", "medication", "pill", "supplement", "vitamin", "workout", "exercise", "fasting"):
		return blockLookup(unresolvedNonFoodText)
	case preparationUnclear(text, tokens):
		return blockLookup(unresolvedPreparationUnclear)
	case ambiguousFood(text, tokens):
		return blockLookup(unresolvedAmbiguousFood)
	case composedFood(food, text, tokens):
		return blockLookup(unresolvedComposedFoodNeedsDecomposition)
	case restaurantOrBrandedFood(item, text, tokens):
		return blockLookup(unresolvedRestaurantOrBrandedFood)
	case broadFoodName(food):
		return blockLookup(unresolvedAmbiguousFood)
	case !fallbackUnitAllowed(item.Unit):
		return blockLookup(unresolvedUnsupportedUnit)
	default:
		return lookupFilterResult{
			LookupAllowed: true,
			Query:         food,
		}
	}
}

func lookupFilterText(item FoodItem) string {
	parts := []string{
		item.Food,
		item.Preparation,
		item.Brand,
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func lookupFilterTokens(text string) map[string]bool {
	tokens := map[string]bool{}
	for _, token := range lookupTokenPattern.FindAllString(text, -1) {
		tokens[token] = true
	}
	return tokens
}

func blockLookup(reason string) lookupFilterResult {
	return lookupFilterResult{
		LookupAllowed: false,
		Reason:        reason,
	}
}

func fallbackUnitAllowed(unit string) bool {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "g", "gram", "grams",
		"oz", "ounce", "ounces",
		"fl oz", "fluid ounce", "fluid ounces",
		"cup", "cups",
		"tbsp", "tablespoon", "tablespoons",
		"tsp", "teaspoon", "teaspoons",
		"slice", "slices",
		"piece", "pieces",
		"container", "containers",
		"packet", "packets",
		"roll", "rolls",
		"cracker", "crackers",
		"patty", "patties",
		"link", "links":
		return true
	default:
		return false
	}
}

func ambiguousFood(text string, tokens map[string]bool) bool {
	if hasAnyToken(tokens, "nfs", "unknown", "other") {
		return true
	}
	return containsAny(text,
		"not further specified",
		"not specified",
		"ns as to",
		"variety not specified",
		"brand not specified",
	)
}

func composedFood(food, text string, tokens map[string]bool) bool {
	if sandwichComponentFood(food) {
		return false
	}
	if hasAnyToken(tokens, "sandwich", "pizza", "burrito", "taco", "casserole", "lasagna", "meal") {
		return true
	}
	return containsAny(text,
		"home recipe",
		"macaroni and cheese",
		"macaroni & cheese",
		"pasta with",
		"pasta in",
		"spaghetti with",
		"spaghetti in",
		"soup",
		"stew",
		" with cheese",
		" with meat",
		" with sauce",
		" with gravy",
		" with vegetables",
		" and cheese",
		" and meat",
		" and beans",
		" and rice",
	)
}

func restaurantOrBrandedFood(item FoodItem, text string, tokens map[string]bool) bool {
	if strings.TrimSpace(item.Brand) != "" {
		return true
	}
	if hasAnyToken(tokens, "restaurant", "cafeteria", "commercial", "mcdonald", "mcdonalds", "wendys", "burger", "king", "kfc", "taco", "bell", "subway", "chipotle", "chick", "fila") {
		return true
	}
	return containsAny(text,
		"fast food",
		"school lunch",
		"from a mix",
		"ready-to-serve",
		"ready to serve",
	)
}

func preparationUnclear(text string, tokens map[string]bool) bool {
	if containsAny(text, "fat added", "with added fat", "as ingredient") {
		return true
	}
	return tokens["fried"] && tokens["unknown"]
}

func sandwichComponentFood(food string) bool {
	food = normalizeFNDDSMatchKey(food)
	const suffix = " for use on a sandwich"
	if !strings.HasSuffix(food, suffix) {
		return false
	}
	switch strings.TrimSpace(strings.TrimSuffix(food, suffix)) {
	case "avocado", "bacon", "cucumber", "lettuce", "mushrooms", "onions", "pepper", "spinach", "tomatoes":
		return true
	default:
		return false
	}
}

func broadFoodName(food string) bool {
	tokens := lookupTokenPattern.FindAllString(strings.ToLower(strings.TrimSpace(food)), -1)
	return len(tokens) == 1 && broadOneWordFoods[tokens[0]]
}

func hasAnyToken(tokens map[string]bool, values ...string) bool {
	for _, value := range values {
		if tokens[value] {
			return true
		}
	}
	return false
}

func containsAny(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, value) {
			return true
		}
	}
	return false
}
