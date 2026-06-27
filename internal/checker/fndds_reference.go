package checker

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type FNDDSReference interface {
	LookupEligibleByDescription(description string) (CatalogFood, bool, error)
}

type SQLiteFNDDSReference struct {
	db *sql.DB
}

func OpenSQLiteFNDDSReference(path string) (*SQLiteFNDDSReference, error) {
	if path == "" {
		return nil, fmt.Errorf("FNDDS fallback path is required")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("open FNDDS fallback database: %w", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	uri := (&url.URL{Scheme: "file", Path: abs, RawQuery: "mode=ro&immutable=1"}).String()
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	ref := &SQLiteFNDDSReference{db: db}
	if err := db.Ping(); err != nil {
		ref.Close()
		return nil, fmt.Errorf("open FNDDS fallback database: %w", err)
	}
	return ref, nil
}

func (r *SQLiteFNDDSReference) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *SQLiteFNDDSReference) LookupEligibleByDescription(description string) (CatalogFood, bool, error) {
	if r == nil || r.db == nil {
		return CatalogFood{}, false, nil
	}
	normalized := normalizeFNDDSMatchKey(description)
	rows, err := r.db.Query(`
		select distinct
			f.food_code,
			f.main_description,
			n.energy_kcal,
			n.protein_g,
			n.carbohydrate_g,
			n.fat_g,
			n.saturated_fat_g,
			n.sodium_mg,
			n.total_sugar_g,
			n.fiber_g
		from fndds_foods f
		join fndds_nutrients n on n.food_code = f.food_code
		join fndds_match_keys key on key.food_code = f.food_code
		where key.normalized_match_key = ?
		  and key.resolver_status = 'auto'
		  and key.confidence in ('exact', 'high')
		order by f.food_code
		limit 2
	`, normalized)
	if err != nil {
		return CatalogFood{}, false, err
	}
	defer rows.Close()

	type match struct {
		foodCode    string
		name        string
		nutrients   Nutrients
		totalSugarG float64
	}
	var matches []match
	for rows.Next() {
		var m match
		if err := rows.Scan(
			&m.foodCode,
			&m.name,
			&m.nutrients.EnergyKcal,
			&m.nutrients.ProteinG,
			&m.nutrients.CarbohydrateG,
			&m.nutrients.FatG,
			&m.nutrients.SaturatedFatG,
			&m.nutrients.SodiumMG,
			&m.totalSugarG,
			&m.nutrients.FiberG,
		); err != nil {
			return CatalogFood{}, false, err
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return CatalogFood{}, false, err
	}
	if err := rows.Close(); err != nil {
		return CatalogFood{}, false, err
	}
	if len(matches) != 1 {
		return CatalogFood{}, false, nil
	}

	m := matches[0]
	nutrients := m.nutrients
	// FNDDS At A Glance does not expose added-sugar grams. For fallback
	// rows, use total sugar as a conservative proxy until a reviewed
	// added-sugar source is added.
	nutrients.AddedSugarG = m.totalSugarG
	allergens, err := r.lookupStrings("fndds_allergens", "allergen", m.foodCode)
	if err != nil {
		return CatalogFood{}, false, err
	}
	foodGroups, err := r.lookupStrings("fndds_food_groups", "food_group", m.foodCode)
	if err != nil {
		return CatalogFood{}, false, err
	}
	unitConversions, err := r.lookupUnitConversions(m.foodCode)
	if err != nil {
		return CatalogFood{}, false, err
	}
	return CatalogFood{
		FoodID:           "fndds_" + m.foodCode,
		Name:             m.name,
		Aliases:          nil,
		BaseQuantityG:    100,
		NutrientsPer100G: nutrients,
		UnitConversions:  unitConversions,
		Allergens:        allergens,
		FoodGroups:       foodGroups,
		SourceRefs: []CatalogSourceRef{
			{
				Source:   "fndds-2021-2023",
				SourceID: m.foodCode,
				DataType: "FNDDS SQLite fallback",
				Note:     "Auto match-key fallback with source-backed unit conversions.",
			},
		},
	}, true, nil
}

func (r *SQLiteFNDDSReference) lookupStrings(table, column, foodCode string) ([]string, error) {
	rows, err := r.db.Query(fmt.Sprintf("select %s from %s where food_code = ? order by %s", column, table, column), foodCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r *SQLiteFNDDSReference) lookupUnitConversions(foodCode string) (map[string]float64, error) {
	rows, err := r.db.Query(`
		select normalized_unit, grams
		from fndds_unit_conversions
		where food_code = ?
		order by normalized_unit
	`, foodCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	conversions := map[string]float64{}
	for rows.Next() {
		var unit string
		var grams float64
		if err := rows.Scan(&unit, &grams); err != nil {
			return nil, err
		}
		conversions[unit] = grams
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if conversions["g"] == 0 {
		conversions["g"] = 1
	}
	if conversions["gram"] == 0 {
		conversions["gram"] = 1
	}
	if conversions["grams"] == 0 {
		conversions["grams"] = 1
	}
	return conversions, nil
}

func normalizeFNDDSMatchKey(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func normalizeUnit(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}
