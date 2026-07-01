package mealplan

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/workflow/checker"
)

type decodePlanResult struct {
	Plan          checker.Plan
	Canonicalized bool
}

type DecodePlanResult = decodePlanResult

func decodePlanText(text string) (checker.Plan, error) {
	result, err := decodePlanTextDetailed(text)
	if err != nil {
		return checker.Plan{}, err
	}
	return result.Plan, nil
}

func DecodePlanText(text string) (checker.Plan, error) {
	return decodePlanText(text)
}

func decodePlanTextDetailed(text string) (decodePlanResult, error) {
	var plan checker.Plan
	jsonText, err := core.ExtractJSONObject(text)
	if err != nil {
		return decodePlanResult{}, err
	}
	jsonText, canonicalized, err := canonicalizePlanJSON(jsonText)
	if err != nil {
		return decodePlanResult{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(jsonText))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return decodePlanResult{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return decodePlanResult{}, fmt.Errorf("meal plan JSON contains multiple values")
	}
	return decodePlanResult{Plan: plan, Canonicalized: canonicalized}, nil
}

func DecodePlanTextDetailed(text string) (DecodePlanResult, error) {
	return decodePlanTextDetailed(text)
}
