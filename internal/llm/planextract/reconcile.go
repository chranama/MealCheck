package planextract

import (
	"math"
	"strconv"
	"strings"
)

func reconcileLocalLlamaRowsWithSource(rows []localLlamaRowItem, sourceText string) ([]localLlamaRowItem, []LocalLlamaNormalizationRepair) {
	if len(rows) == 0 {
		return rows, nil
	}
	for _, row := range rows {
		if row.SourceItemID == 0 {
			return rows, nil
		}
	}
	return reconcileLocalLlamaRowsWithSourceItems(rows, localLlamaExtractionSourceItems(sourceText))
}
func reconcileLocalLlamaRowsWithSourceItems(rows []localLlamaRowItem, sourceItems []localLlamaSourceItem) ([]localLlamaRowItem, []LocalLlamaNormalizationRepair) {
	sourceByID := map[int]localLlamaSourceItem{}
	for _, item := range sourceItems {
		sourceByID[item.ID] = item
	}
	if len(sourceByID) == 0 {
		return rows, nil
	}

	reconciled := append([]localLlamaRowItem(nil), rows...)
	var repairs []LocalLlamaNormalizationRepair
	for index := range reconciled {
		row := &reconciled[index]
		sourceItem, ok := sourceByID[row.SourceItemID]
		if !ok {
			continue
		}
		if sourceItem.Day > 0 && row.Day != sourceItem.Day {
			repairs = append(repairs, localLlamaRepair(row.SourceItemID, "day", strconv.Itoa(row.Day), strconv.Itoa(sourceItem.Day), "source_inventory"))
			row.Day = sourceItem.Day
		}
		if sourceItem.MealCode != "" && row.MealCode != sourceItem.MealCode {
			repairs = append(repairs, localLlamaRepair(row.SourceItemID, "meal_code", row.MealCode, sourceItem.MealCode, "source_inventory"))
			row.MealCode = sourceItem.MealCode
		}
		if sourceItem.ParseStatus != localLlamaSourceResolved {
			continue
		}
		measurement := localLlamaParseSourceMeasurement(sourceItem.Text)
		if measurement.Status != "parsed" {
			continue
		}
		if strings.TrimSpace(row.QuantityText) != "" || strings.TrimSpace(row.UnresolvedReason) != "" {
			repairs = append(repairs, localLlamaRepair(row.SourceItemID, "resolution_status", "unresolved", "resolved", "source_measurement"))
			row.QuantityText = ""
			row.UnresolvedReason = ""
		}
		if math.Abs(row.Quantity-measurement.Quantity) > 0.0001 {
			repairs = append(repairs, localLlamaRepair(row.SourceItemID, "quantity", formatLocalLlamaQuantity(row.Quantity), formatLocalLlamaQuantity(measurement.Quantity), "source_measurement"))
			row.Quantity = measurement.Quantity
		}
		normalizedUnit := localLlamaNormalizeSourceUnit(row.Unit)
		if normalizedUnit != measurement.Unit {
			repairs = append(repairs, localLlamaRepair(row.SourceItemID, "unit", row.Unit, measurement.Unit, "source_measurement"))
			row.Unit = measurement.Unit
		} else if row.Unit != normalizedUnit {
			repairs = append(repairs, localLlamaRepair(row.SourceItemID, "unit", row.Unit, normalizedUnit, "unit_alias"))
			row.Unit = normalizedUnit
		}
		if strings.TrimSpace(row.Food) != measurement.Food {
			repairs = append(repairs, localLlamaRepair(row.SourceItemID, "food", row.Food, measurement.Food, "source_measurement"))
			row.Food = measurement.Food
		}
	}
	return reconciled, repairs
}
func localLlamaRepair(sourceItemID int, field string, from string, to string, reason string) LocalLlamaNormalizationRepair {
	return LocalLlamaNormalizationRepair{
		SourceItemID: sourceItemID,
		Field:        field,
		From:         from,
		To:           to,
		Reason:       reason,
	}
}
func formatLocalLlamaQuantity(quantity float64) string {
	return strconv.FormatFloat(quantity, 'f', -1, 64)
}
