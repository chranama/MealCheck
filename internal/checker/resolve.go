package checker

import (
	"fmt"
	"sort"
	"strings"
)

type resolver struct {
	foods map[string]CatalogFood
}

func newResolver(catalog NutrientCatalog) resolver {
	foods := map[string]CatalogFood{}
	for _, food := range catalog.Foods {
		foods[normalizeName(food.Name)] = food
		for _, alias := range food.Aliases {
			foods[normalizeName(alias)] = food
		}
	}
	return resolver{foods: foods}
}

func (r resolver) resolvePlan(plan Plan) ([]ResolvedItem, []UnresolvedItem) {
	var resolved []ResolvedItem
	var unresolved []UnresolvedItem

	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			for _, item := range meal.Items {
				resolvedItem, unresolvedItem, ok := r.resolveItem(day.Day, meal.Name, item)
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

	return resolved, unresolved
}

func (r resolver) resolveItem(day int, meal string, item FoodItem) (ResolvedItem, UnresolvedItem, bool) {
	if item.ResolutionStatus == "unresolved" {
		return ResolvedItem{}, unresolvedItem(day, meal, item), false
	}
	if item.Quantity == nil {
		u := unresolvedItem(day, meal, item)
		if u.UnresolvedReason == "" {
			u.UnresolvedReason = "vague_quantity"
		}
		return ResolvedItem{}, u, false
	}

	food, ok := r.foods[normalizeName(item.Food)]
	if !ok {
		u := unresolvedItem(day, meal, item)
		u.UnresolvedReason = "unknown_food"
		return ResolvedItem{}, u, false
	}

	gramsPerUnit, ok := food.UnitConversions[item.Unit]
	if !ok {
		u := unresolvedItem(day, meal, item)
		u.UnresolvedReason = fmt.Sprintf("missing_conversion:%s", item.Unit)
		return ResolvedItem{}, u, false
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
	}, UnresolvedItem{}, true
}

func unresolvedItem(day int, meal string, item FoodItem) UnresolvedItem {
	return UnresolvedItem{
		Day:              day,
		Meal:             meal,
		Food:             item.Food,
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
