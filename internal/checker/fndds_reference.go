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
	LookupApproximationProxy(inputKey string) (FNDDSApproximationProxy, bool, error)
	LookupApproximationProxyBySourceFoodCode(foodCode string) (FNDDSApproximationProxy, bool, error)
	LookupDecompositionTemplate(description string) (FNDDSDecompositionTemplate, bool, error)
	LookupFoodByCode(foodCode string) (CatalogFood, bool, error)
}

type SQLiteFNDDSReference struct {
	db *sql.DB
}

type FNDDSApproximationProxy struct {
	InputKey                   string
	ProxyFoodCode              string
	ProxyDescription           string
	Confidence                 string
	AllowWhenAllergiesPresent  bool
	AllowWhenExclusionsPresent bool
	EstimateReason             string
	Food                       CatalogFood
}

type FNDDSDecompositionTemplate struct {
	TemplateID string
	Pattern    string
	Confidence string
	Notes      string
	Components []FNDDSDecompositionComponent
}

type FNDDSDecompositionComponent struct {
	FoodCode string
	Role     string
	Fraction float64
	Required bool
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
			key.food_code
		from fndds_match_keys key
		where key.normalized_match_key = ?
		  and key.resolver_status = 'auto'
		  and key.confidence in ('exact', 'high')
		order by key.food_code
		limit 2
	`, normalized)
	if err != nil {
		return CatalogFood{}, false, err
	}
	defer rows.Close()

	var matches []string
	for rows.Next() {
		var foodCode string
		if err := rows.Scan(&foodCode); err != nil {
			return CatalogFood{}, false, err
		}
		matches = append(matches, foodCode)
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
	return r.lookupCatalogFoodByCode(matches[0])
}

func (r *SQLiteFNDDSReference) LookupApproximationProxy(inputKey string) (FNDDSApproximationProxy, bool, error) {
	if r == nil || r.db == nil {
		return FNDDSApproximationProxy{}, false, nil
	}
	normalized := normalizeFNDDSMatchKey(inputKey)
	return r.lookupApproximationProxy(`
		select proxy.input_key, proxy.proxy_food_code, proxy.proxy_description, proxy.confidence,
		       proxy.allow_when_allergies_present, proxy.allow_when_exclusions_present,
		       proxy.estimate_reason
		  from fndds_approximation_proxies proxy
		 where proxy.normalized_input_key = ?
		 order by proxy.input_key
		 limit 1
	`, normalized)
}

func (r *SQLiteFNDDSReference) LookupApproximationProxyBySourceFoodCode(foodCode string) (FNDDSApproximationProxy, bool, error) {
	if r == nil || r.db == nil {
		return FNDDSApproximationProxy{}, false, nil
	}
	foodCode = strings.TrimPrefix(strings.TrimSpace(foodCode), "fndds_")
	if foodCode == "" {
		return FNDDSApproximationProxy{}, false, nil
	}
	return r.lookupApproximationProxy(`
		select proxy.input_key, proxy.proxy_food_code, proxy.proxy_description, proxy.confidence,
		       proxy.allow_when_allergies_present, proxy.allow_when_exclusions_present,
		       proxy.estimate_reason
		  from fndds_approximation_proxy_source_codes source
		  join fndds_approximation_proxies proxy on proxy.input_key = source.input_key
		 where source.source_food_code = ?
		 order by proxy.input_key
		 limit 1
	`, foodCode)
}

func (r *SQLiteFNDDSReference) lookupApproximationProxy(query string, arg string) (FNDDSApproximationProxy, bool, error) {
	var proxy FNDDSApproximationProxy
	var allowAllergies int
	var allowExclusions int
	err := r.db.QueryRow(query, arg).Scan(
		&proxy.InputKey,
		&proxy.ProxyFoodCode,
		&proxy.ProxyDescription,
		&proxy.Confidence,
		&allowAllergies,
		&allowExclusions,
		&proxy.EstimateReason,
	)
	if err == sql.ErrNoRows {
		return FNDDSApproximationProxy{}, false, nil
	}
	if err != nil {
		return FNDDSApproximationProxy{}, false, err
	}
	food, ok, err := r.lookupCatalogFoodByCode(proxy.ProxyFoodCode)
	if err != nil || !ok {
		return FNDDSApproximationProxy{}, ok, err
	}
	proxy.AllowWhenAllergiesPresent = allowAllergies != 0
	proxy.AllowWhenExclusionsPresent = allowExclusions != 0
	proxy.Food = food
	return proxy, true, nil
}

func (r *SQLiteFNDDSReference) LookupDecompositionTemplate(description string) (FNDDSDecompositionTemplate, bool, error) {
	if r == nil || r.db == nil {
		return FNDDSDecompositionTemplate{}, false, nil
	}
	normalized := normalizeFNDDSMatchKey(description)
	var template FNDDSDecompositionTemplate
	var notes sql.NullString
	err := r.db.QueryRow(`
		select template_id, pattern, confidence, notes
		  from fndds_decomposition_templates
		 where normalized_pattern = ?
		 order by template_id
		 limit 1
	`, normalized).Scan(&template.TemplateID, &template.Pattern, &template.Confidence, &notes)
	if err == sql.ErrNoRows {
		return FNDDSDecompositionTemplate{}, false, nil
	}
	if err != nil {
		return FNDDSDecompositionTemplate{}, false, err
	}
	if notes.Valid {
		template.Notes = notes.String
	}
	rows, err := r.db.Query(`
		select food_code, role, fraction, required
		  from fndds_decomposition_components
		 where template_id = ?
		 order by position
	`, template.TemplateID)
	if err != nil {
		return FNDDSDecompositionTemplate{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var component FNDDSDecompositionComponent
		var required int
		if err := rows.Scan(&component.FoodCode, &component.Role, &component.Fraction, &required); err != nil {
			return FNDDSDecompositionTemplate{}, false, err
		}
		component.Required = required != 0
		template.Components = append(template.Components, component)
	}
	if err := rows.Err(); err != nil {
		return FNDDSDecompositionTemplate{}, false, err
	}
	if len(template.Components) == 0 {
		return FNDDSDecompositionTemplate{}, false, nil
	}
	return template, true, nil
}

func (r *SQLiteFNDDSReference) LookupFoodByCode(foodCode string) (CatalogFood, bool, error) {
	if r == nil || r.db == nil {
		return CatalogFood{}, false, nil
	}
	return r.lookupCatalogFoodByCode(foodCode)
}

func (r *SQLiteFNDDSReference) lookupCatalogFoodByCode(foodCode string) (CatalogFood, bool, error) {
	foodCode = strings.TrimPrefix(strings.TrimSpace(foodCode), "fndds_")
	var name string
	var nutrients Nutrients
	var totalSugarG float64
	err := r.db.QueryRow(`
		select
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
		where f.food_code = ?
	`, foodCode).Scan(
		&name,
		&nutrients.EnergyKcal,
		&nutrients.ProteinG,
		&nutrients.CarbohydrateG,
		&nutrients.FatG,
		&nutrients.SaturatedFatG,
		&nutrients.SodiumMG,
		&totalSugarG,
		&nutrients.FiberG,
	)
	if err == sql.ErrNoRows {
		return CatalogFood{}, false, nil
	}
	if err != nil {
		return CatalogFood{}, false, err
	}

	// FNDDS At A Glance does not expose added-sugar grams. For fallback
	// rows, use total sugar as a conservative proxy until a reviewed
	// added-sugar source is added.
	nutrients.AddedSugarG = totalSugarG
	allergens, err := r.lookupStrings("fndds_allergens", "allergen", foodCode)
	if err != nil {
		return CatalogFood{}, false, err
	}
	foodGroups, err := r.lookupStrings("fndds_food_groups", "food_group", foodCode)
	if err != nil {
		return CatalogFood{}, false, err
	}
	unitConversions, err := r.lookupUnitConversions(foodCode)
	if err != nil {
		return CatalogFood{}, false, err
	}
	return CatalogFood{
		FoodID:           "fndds_" + foodCode,
		Name:             name,
		Aliases:          nil,
		BaseQuantityG:    100,
		NutrientsPer100G: nutrients,
		UnitConversions:  unitConversions,
		Allergens:        allergens,
		FoodGroups:       foodGroups,
		SourceRefs: []CatalogSourceRef{
			{
				Source:   "fndds-2021-2023",
				SourceID: foodCode,
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
