package localmodel

import (
	"math"
	"strconv"
	"strings"
)

func allowedLocalLlamaUnresolvedReason(reason string) bool {
	switch reason {
	case "missing_quantity", "vague_quantity", "unsupported_unit":
		return true
	default:
		return false
	}
}
func localLlamaStripDuplicateQuantityPrefix(food string, quantity float64, unit string) (string, string) {
	measurement := localLlamaParseSourceMeasurement(food)
	if measurement.Status != "parsed" || math.Abs(measurement.Quantity-quantity) > 0.0001 {
		return food, unit
	}
	return measurement.Food, measurement.Unit
}
func localLlamaParseQuantityPrefixFields(fields []string) (float64, int, bool) {
	if len(fields) == 0 {
		return 0, 0, false
	}
	first, ok := localLlamaParseQuantityToken(fields[0])
	if !ok {
		return 0, 0, false
	}
	if len(fields) > 1 && strings.Contains(fields[1], "/") {
		fraction, ok := localLlamaParseQuantityToken(fields[1])
		if ok {
			return first + fraction, 2, true
		}
	}
	if len(fields) > 2 && fields[1] == "/" {
		denominator, ok := localLlamaParseQuantityToken(fields[2])
		if ok && denominator != 0 {
			return first / denominator, 3, true
		}
	}
	if len(fields) > 3 && strings.Contains(fields[2], "/") {
		fraction, ok := localLlamaParseQuantityToken(fields[2])
		if ok {
			return first + fraction, 3, true
		}
	}
	return first, 1, true
}
func localLlamaParseQuantityToken(token string) (float64, bool) {
	token = strings.Trim(strings.TrimSpace(token), "(),")
	if token == "" {
		return 0, false
	}
	if strings.Contains(token, "/") {
		parts := strings.Split(token, "/")
		if len(parts) != 2 {
			return 0, false
		}
		numerator, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			return 0, false
		}
		denominator, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil || denominator == 0 {
			return 0, false
		}
		return numerator / denominator, true
	}
	value, err := strconv.ParseFloat(token, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}
func LocalLlamaParseSourceMeasurement(sourceText string) LocalLlamaParsedSourceMeasurement {
	return localLlamaParseSourceMeasurement(sourceText)
}

func allowedUnit(unit string) bool {
	switch unit {
	case "g", "oz", "cup", "tbsp", "tsp", "slice", "serving":
		return true
	default:
		return false
	}
}

func localLlamaParseSourceMeasurement(sourceText string) LocalLlamaParsedSourceMeasurement {
	text := strings.TrimSpace(strings.Trim(sourceText, " \t\r\n.;,"))
	if text == "" {
		return LocalLlamaParsedSourceMeasurement{Status: "failed", Reason: "empty_source_text"}
	}
	fields := strings.Fields(text)
	quantity, consumed, ok := localLlamaParseQuantityPrefixFields(fields)
	if !ok || quantity <= 0 {
		return LocalLlamaParsedSourceMeasurement{Status: "failed", Reason: "missing_numeric_quantity"}
	}
	if consumed >= len(fields) {
		return LocalLlamaParsedSourceMeasurement{Status: "failed", Reason: "missing_unit"}
	}
	unit := localLlamaNormalizeSourceUnit(strings.Trim(fields[consumed], " ,.;:()"))
	if !allowedUnit(unit) {
		return LocalLlamaParsedSourceMeasurement{Status: "failed", Reason: "unsupported_unit"}
	}
	food := strings.TrimSpace(strings.Join(fields[consumed+1:], " "))
	food = strings.TrimSpace(strings.Trim(food, " \t\r\n.;,"))
	food = strings.TrimSpace(localLlamaLeadingOf.ReplaceAllString(food, ""))
	if food == "" {
		return LocalLlamaParsedSourceMeasurement{Status: "failed", Reason: "missing_food"}
	}
	return LocalLlamaParsedSourceMeasurement{
		Food:     food,
		Quantity: quantity,
		Unit:     unit,
		Status:   "parsed",
	}
}
