package normalization

import (
	"strconv"
	"strings"
)

type ParsedSourceMeasurement struct {
	Food     string  `json:"food,omitempty"`
	Quantity float64 `json:"quantity,omitempty"`
	Unit     string  `json:"unit,omitempty"`
	Status   string  `json:"status"`
	Reason   string  `json:"reason,omitempty"`
}

func ParseSourceMeasurement(sourceText string) ParsedSourceMeasurement {
	text := strings.TrimSpace(strings.Trim(sourceText, " \t\r\n.;,"))
	if text == "" {
		return ParsedSourceMeasurement{Status: "failed", Reason: "empty_source_text"}
	}
	fields := strings.Fields(text)
	quantity, consumed, ok := parseQuantityPrefixFields(fields)
	if !ok || quantity <= 0 {
		return ParsedSourceMeasurement{Status: "failed", Reason: "missing_numeric_quantity"}
	}
	if consumed >= len(fields) {
		return ParsedSourceMeasurement{Status: "failed", Reason: "missing_unit"}
	}
	unit := NormalizeSourceUnit(strings.Trim(fields[consumed], " ,.;:()"))
	if !AllowedUnit(unit) {
		return ParsedSourceMeasurement{Status: "failed", Reason: "unsupported_unit"}
	}
	food := strings.TrimSpace(strings.Join(fields[consumed+1:], " "))
	food = strings.TrimSpace(strings.Trim(food, " \t\r\n.;,"))
	food = strings.TrimSpace(leadingOf.ReplaceAllString(food, ""))
	if food == "" {
		return ParsedSourceMeasurement{Status: "failed", Reason: "missing_food"}
	}
	return ParsedSourceMeasurement{
		Food:     food,
		Quantity: quantity,
		Unit:     unit,
		Status:   "parsed",
	}
}

func AllowedUnit(unit string) bool {
	switch unit {
	case "g", "oz", "cup", "tbsp", "tsp", "slice", "serving":
		return true
	default:
		return false
	}
}

func parseQuantityPrefixFields(fields []string) (float64, int, bool) {
	if len(fields) == 0 {
		return 0, 0, false
	}
	first, ok := parseQuantityToken(fields[0])
	if !ok {
		return 0, 0, false
	}
	if len(fields) > 1 && strings.Contains(fields[1], "/") {
		fraction, ok := parseQuantityToken(fields[1])
		if ok {
			return first + fraction, 2, true
		}
	}
	if len(fields) > 2 && fields[1] == "/" {
		denominator, ok := parseQuantityToken(fields[2])
		if ok && denominator != 0 {
			return first / denominator, 3, true
		}
	}
	if len(fields) > 3 && strings.Contains(fields[2], "/") {
		fraction, ok := parseQuantityToken(fields[2])
		if ok {
			return first + fraction, 3, true
		}
	}
	return first, 1, true
}

func parseQuantityToken(token string) (float64, bool) {
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
