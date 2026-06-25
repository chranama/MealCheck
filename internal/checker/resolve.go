package checker

import (
	"fmt"
	"sort"
	"strings"
)

type resolver struct {
	foods    map[string]CatalogFood
	fallback FNDDSReference
}

func newResolver(catalog NutrientCatalog) resolver {
	return newResolverWithFallback(catalog, nil)
}

func newResolverWithFallback(catalog NutrientCatalog, fallback FNDDSReference) resolver {
	foods := map[string]CatalogFood{}
	for _, food := range catalog.Foods {
		foods[normalizeName(food.Name)] = food
		for _, alias := range food.Aliases {
			foods[normalizeName(alias)] = food
		}
	}
	return resolver{foods: foods, fallback: fallback}
}

func (r resolver) resolvePlan(plan Plan) ([]ResolvedItem, []UnresolvedItem) {
	resolved, unresolved, _ := r.resolvePlanWithError(plan)
	return resolved, unresolved
}

func (r resolver) resolvePlanWithError(plan Plan) ([]ResolvedItem, []UnresolvedItem, error) {
	var resolved []ResolvedItem
	var unresolved []UnresolvedItem

	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			for _, item := range meal.Items {
				resolvedItem, unresolvedItem, ok, err := r.resolveItem(day.Day, meal.Name, item)
				if err != nil {
					return nil, nil, err
				}
				if ok {
					resolved = append(resolved, resolvedItem)
					continue
				}
				unresolved = append(unresolved, unresolvedItem)
			}
		}
	}

	sort.Slice(resolved, func(i, j int) bool {
		if resolved[i].Day != resolved[j].Day {
			return resolved[i].Day < resolved[j].Day
		}
		if resolved[i].Meal != resolved[j].Meal {
			return resolved[i].Meal < resolved[j].Meal
		}
		return resolved[i].Food < resolved[j].Food
	})

	return resolved, unresolved, nil
}

func (r resolver) resolveItem(day int, meal string, item FoodItem) (ResolvedItem, UnresolvedItem, bool, error) {
	if item.ResolutionStatus == "unresolved" {
		if item.UnresolvedReason != unresolvedUnknownFood || item.Quantity == nil {
			return ResolvedItem{}, unresolvedItem(day, meal, item), false, nil
		}
		if food, ok := r.foods[normalizeName(item.Food)]; ok {
			return resolveKnownFood(day, meal, item, food)
		}
		return r.resolveFallbackCandidate(day, meal, item)
	}
	if item.Quantity == nil {
		u := unresolvedItem(day, meal, item)
		if u.UnresolvedReason == "" {
			u.UnresolvedReason = unresolvedVagueQuantity
		}
		return ResolvedItem{}, u, false, nil
	}

	food, ok := r.foods[normalizeName(item.Food)]
	if !ok {
		return r.resolveFallbackCandidate(day, meal, item)
	}

	return resolveKnownFood(day, meal, item, food)
}

func (r resolver) resolveFallbackCandidate(day int, meal string, item FoodItem) (ResolvedItem, UnresolvedItem, bool, error) {
	filter := filterFallbackLookupCandidate(item)
	if !filter.LookupAllowed {
		u := unresolvedItem(day, meal, item)
		u.UnresolvedReason = filter.Reason
		return ResolvedItem{}, u, false, nil
	}
	if r.fallback == nil {
		u := unresolvedItem(day, meal, item)
		u.UnresolvedReason = unresolvedUnknownFood
		return ResolvedItem{}, u, false, nil
	}
	food, ok, err := r.fallback.LookupEligibleByDescription(filter.Query)
	if err != nil {
		return ResolvedItem{}, UnresolvedItem{}, false, err
	}
	if !ok {
		u := unresolvedItem(day, meal, item)
		u.UnresolvedReason = unresolvedUnknownFood
		return ResolvedItem{}, u, false, nil
	}
	return resolveKnownFood(day, meal, item, food)
}

func resolveKnownFood(day int, meal string, item FoodItem, food CatalogFood) (ResolvedItem, UnresolvedItem, bool, error) {
	gramsPerUnit, ok := food.UnitConversions[item.Unit]
	if !ok {
		u := unresolvedItem(day, meal, item)
		u.UnresolvedReason = fmt.Sprintf("missing_conversion:%s", item.Unit)
		return ResolvedItem{}, u, false, nil
	}
	grams := *item.Quantity * gramsPerUnit
	factor := grams / 100
	return ResolvedItem{
		Day:        day,
		Meal:       meal,
		Food:       item.Food,
		FoodID:     food.FoodID,
		Quantity:   *item.Quantity,
		Unit:       item.Unit,
		Grams:      grams,
		Nutrients:  scaleNutrients(food.NutrientsPer100G, factor),
		Allergens:  append([]string(nil), food.Allergens...),
		FoodGroups: append([]string(nil), food.FoodGroups...),
	}, UnresolvedItem{}, true, nil
}

func unresolvedItem(day int, meal string, item FoodItem) UnresolvedItem {
	return UnresolvedItem{
		Day:              day,
		Meal:             meal,
		Food:             item.Food,
		Quantity:         item.Quantity,
		QuantityText:     item.QuantityText,
		Unit:             item.Unit,
		UnresolvedReason: item.UnresolvedReason,
	}
}

func scaleNutrients(n Nutrients, factor float64) Nutrients {
	return Nutrients{
		EnergyKcal:    n.EnergyKcal * factor,
		ProteinG:      n.ProteinG * factor,
		CarbohydrateG: n.CarbohydrateG * factor,
		FatG:          n.FatG * factor,
		SaturatedFatG: n.SaturatedFatG * factor,
		SodiumMG:      n.SodiumMG * factor,
		AddedSugarG:   n.AddedSugarG * factor,
		FiberG:        n.FiberG * factor,
	}
}

func normalizeName(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}
