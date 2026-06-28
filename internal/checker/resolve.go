package checker

import (
	"fmt"
	"sort"
	"strings"
)

type resolver struct {
	foods       map[string]CatalogFood
	fallback    FNDDSReference
	constraints VerificationConstraints
}

func newResolver(catalog NutrientCatalog) resolver {
	return newResolverWithFallback(catalog, nil)
}

func newResolverWithFallback(catalog NutrientCatalog, fallback FNDDSReference, constraints ...VerificationConstraints) resolver {
	foods := map[string]CatalogFood{}
	for _, food := range catalog.Foods {
		foods[normalizeName(food.Name)] = food
		for _, alias := range food.Aliases {
			foods[normalizeName(alias)] = food
		}
	}
	var constraintSet VerificationConstraints
	if len(constraints) > 0 {
		constraintSet = constraints[0]
	}
	return resolver{foods: foods, fallback: fallback, constraints: constraintSet}
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
		if filter.Reason == unresolvedAmbiguousFood {
			if resolved, unresolved, ok, err := r.resolveApproximationCandidate(day, meal, item, broadFoodName(item.Food)); err != nil || ok || unresolved.UnresolvedReason != "" {
				return resolved, unresolved, ok, err
			}
		}
		if filter.Reason == unresolvedComposedFoodNeedsDecomposition {
			if resolved, unresolved, ok, err := r.resolveDecompositionCandidate(day, meal, item); err != nil || ok || unresolved.UnresolvedReason != "" {
				return resolved, unresolved, ok, err
			}
		}
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
		if resolved, unresolved, ok, err := r.resolveApproximationCandidate(day, meal, item, true); err != nil || ok || unresolved.UnresolvedReason != "" {
			return resolved, unresolved, ok, err
		}
		u := unresolvedItem(day, meal, item)
		u.UnresolvedReason = unresolvedUnknownFood
		return ResolvedItem{}, u, false, nil
	}
	return resolveKnownFood(day, meal, item, food)
}

func (r resolver) resolveApproximationCandidate(day int, meal string, item FoodItem, allowTextLookup bool) (ResolvedItem, UnresolvedItem, bool, error) {
	if r.fallback == nil || item.Quantity == nil {
		return ResolvedItem{}, UnresolvedItem{}, false, nil
	}
	proxy, ok, err := r.lookupApproximationProxy(item, allowTextLookup)
	if err != nil {
		return ResolvedItem{}, UnresolvedItem{}, false, err
	}
	if !ok {
		return ResolvedItem{}, UnresolvedItem{}, false, nil
	}
	if len(r.constraints.Allergies) > 0 && !proxy.AllowWhenAllergiesPresent {
		return ResolvedItem{}, UnresolvedItem{}, false, nil
	}
	if len(r.constraints.ExcludedFoods) > 0 && !proxy.AllowWhenExclusionsPresent {
		return ResolvedItem{}, UnresolvedItem{}, false, nil
	}
	resolved, unresolved, ok, err := resolveKnownFood(day, meal, item, proxy.Food)
	if err != nil || !ok {
		return resolved, unresolved, ok, err
	}
	resolved.ResolutionMethod = "estimated"
	resolved.Confidence = proxy.Confidence
	resolved.ProxyFoodID = proxy.Food.FoodID
	resolved.ProxyFood = proxy.Food.Name
	resolved.EstimateReason = proxy.EstimateReason
	return resolved, UnresolvedItem{}, true, nil
}

func (r resolver) lookupApproximationProxy(item FoodItem, allowTextLookup bool) (FNDDSApproximationProxy, bool, error) {
	if item.SourceFoodCode != "" {
		proxy, ok, err := r.fallback.LookupApproximationProxyBySourceFoodCode(item.SourceFoodCode)
		if err != nil || ok {
			return proxy, ok, err
		}
	}
	if allowTextLookup {
		return r.fallback.LookupApproximationProxy(item.Food)
	}
	return FNDDSApproximationProxy{}, false, nil
}

func (r resolver) resolveDecompositionCandidate(day int, meal string, item FoodItem) (ResolvedItem, UnresolvedItem, bool, error) {
	if r.fallback == nil || item.Quantity == nil {
		return ResolvedItem{}, UnresolvedItem{}, false, nil
	}
	template, ok, err := r.fallback.LookupDecompositionTemplate(item.Food)
	if err != nil || !ok {
		return ResolvedItem{}, UnresolvedItem{}, false, err
	}
	gramsPerUnit, ok := deterministicMassUnitFactor(item.Unit)
	if !ok {
		u := unresolvedItem(day, meal, item)
		u.UnresolvedReason = unresolvedUnsupportedUnit
		return ResolvedItem{}, u, false, nil
	}
	totalGrams := *item.Quantity * gramsPerUnit
	resolved := ResolvedItem{
		Day:              day,
		Meal:             meal,
		Food:             item.Food,
		FoodID:           "decomposed_" + template.TemplateID,
		SourceFoodCode:   item.SourceFoodCode,
		Quantity:         *item.Quantity,
		Unit:             item.Unit,
		Grams:            totalGrams,
		ResolutionMethod: "decomposed",
		Confidence:       template.Confidence,
		EstimateReason:   template.Notes,
	}
	for _, component := range template.Components {
		food, ok, err := r.fallback.LookupFoodByCode(component.FoodCode)
		if err != nil {
			return ResolvedItem{}, UnresolvedItem{}, false, err
		}
		if !ok {
			u := unresolvedItem(day, meal, item)
			u.UnresolvedReason = unresolvedUnknownFood
			return ResolvedItem{}, u, false, nil
		}
		componentGrams := totalGrams * component.Fraction
		componentQuantity := componentGrams
		componentItem := FoodItem{
			Food:     food.Name,
			Quantity: &componentQuantity,
			Unit:     "g",
		}
		resolvedComponent, unresolved, ok, err := resolveKnownFood(day, meal, componentItem, food)
		if err != nil || !ok {
			return ResolvedItem{}, unresolved, false, err
		}
		resolved.Nutrients = addNutrients(resolved.Nutrients, resolvedComponent.Nutrients)
		for _, allergen := range resolvedComponent.Allergens {
			resolved.Allergens = appendUniqueString(resolved.Allergens, allergen)
		}
		for _, group := range resolvedComponent.FoodGroups {
			resolved.FoodGroups = appendUniqueString(resolved.FoodGroups, group)
		}
		resolved.Components = append(resolved.Components, ResolvedComponent{
			Food:       resolvedComponent.Food,
			FoodID:     resolvedComponent.FoodID,
			Fraction:   component.Fraction,
			Grams:      componentGrams,
			Nutrients:  resolvedComponent.Nutrients,
			Allergens:  resolvedComponent.Allergens,
			FoodGroups: resolvedComponent.FoodGroups,
		})
	}
	return resolved, UnresolvedItem{}, true, nil
}

func resolveKnownFood(day int, meal string, item FoodItem, food CatalogFood) (ResolvedItem, UnresolvedItem, bool, error) {
	gramsPerUnit, ok := food.UnitConversions[item.Unit]
	if !ok {
		gramsPerUnit, ok = food.UnitConversions[normalizeUnit(item.Unit)]
	}
	if !ok {
		u := unresolvedItem(day, meal, item)
		u.UnresolvedReason = fmt.Sprintf("missing_conversion:%s", item.Unit)
		return ResolvedItem{}, u, false, nil
	}
	grams := *item.Quantity * gramsPerUnit
	factor := grams / 100
	return ResolvedItem{
		Day:              day,
		Meal:             meal,
		Food:             item.Food,
		FoodID:           food.FoodID,
		SourceFoodCode:   item.SourceFoodCode,
		Quantity:         *item.Quantity,
		Unit:             item.Unit,
		Grams:            grams,
		Nutrients:        scaleNutrients(food.NutrientsPer100G, factor),
		Allergens:        append([]string(nil), food.Allergens...),
		FoodGroups:       append([]string(nil), food.FoodGroups...),
		ResolutionMethod: "exact",
		Confidence:       "high",
	}, UnresolvedItem{}, true, nil
}

func unresolvedItem(day int, meal string, item FoodItem) UnresolvedItem {
	return UnresolvedItem{
		Day:              day,
		Meal:             meal,
		Food:             item.Food,
		SourceFoodCode:   item.SourceFoodCode,
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
