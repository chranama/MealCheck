package checker

type Case struct {
	SchemaVersion       string       `json:"schema_version"`
	CaseID              string       `json:"case_id"`
	InputMode           string       `json:"input_mode"`
	Settings            Settings     `json:"settings"`
	GuidelinePackID     string       `json:"guideline_pack_id"`
	GuidelinePackPath   string       `json:"guideline_pack_path"`
	NutrientCatalogID   string       `json:"nutrient_catalog_id"`
	NutrientCatalogPath string       `json:"nutrient_catalog_path"`
	BaselinePlan        string       `json:"baseline_plan,omitempty"`
	CandidatePlan       string       `json:"candidate_plan"`
	GenerationPrompt    string       `json:"generation_prompt,omitempty"`
	Expectations        Expectations `json:"expectations"`
	Tags                []string     `json:"tags"`
}

type Settings struct {
	NutritionTargets        NutritionTargets        `json:"nutrition_targets"`
	VerificationConstraints VerificationConstraints `json:"verification_constraints"`
}

type NutritionTargets struct {
	CalorieTargetKcal int `json:"calorie_target_kcal"`
	ProteinTargetG    int `json:"protein_target_g"`
}

type VerificationConstraints struct {
	Days                       int              `json:"days,omitempty"`
	MealsPerDay                int              `json:"meals_per_day,omitempty"`
	Allergies                  []string         `json:"allergies"`
	ExcludedFoods              []string         `json:"excluded_foods"`
	MaxSodiumMGPerDay          int              `json:"max_sodium_mg_per_day"`
	MaxAddedSugarGPerMeal      float64          `json:"max_added_sugar_g_per_meal"`
	MaxSaturatedFatPctCalories float64          `json:"max_saturated_fat_pct_calories"`
	CalorieTolerancePct        float64          `json:"calorie_tolerance_pct"`
	RequiresPrepSafetyNotes    bool             `json:"requires_prep_safety_notes"`
	UnresolvedPolicy           UnresolvedPolicy `json:"unresolved_policy,omitempty"`
}

type UnresolvedPolicy struct {
	DeMinimisEnabled    bool    `json:"de_minimis_enabled,omitempty"`
	MaxItemGrams        float64 `json:"max_item_grams,omitempty"`
	MaxTotalGramsPerDay float64 `json:"max_total_grams_per_day,omitempty"`
	MaxItemsPerDay      int     `json:"max_items_per_day,omitempty"`
}

type Expectations struct {
	ExpectedDecision    string   `json:"expected_decision"`
	ExpectedBlockChecks []string `json:"expected_block_checks"`
	ExpectedWarnChecks  []string `json:"expected_warn_checks"`
}

type Plan struct {
	SchemaVersion string     `json:"schema_version"`
	PlanID        string     `json:"plan_id"`
	Description   string     `json:"description"`
	Days          []PlanDay  `json:"days"`
	ShoppingList  []FoodItem `json:"shopping_list"`
	PrepNotes     []string   `json:"prep_notes"`
}

type PlanDay struct {
	Day   int    `json:"day"`
	Meals []Meal `json:"meals"`
}

type Meal struct {
	Name  string     `json:"name"`
	Items []FoodItem `json:"items"`
}

type FoodItem struct {
	Food             string   `json:"food"`
	Quantity         *float64 `json:"quantity,omitempty"`
	QuantityText     string   `json:"quantity_text,omitempty"`
	Unit             string   `json:"unit,omitempty"`
	Preparation      string   `json:"preparation,omitempty"`
	Brand            string   `json:"brand,omitempty"`
	ResolutionStatus string   `json:"resolution_status,omitempty"`
	UnresolvedReason string   `json:"unresolved_reason,omitempty"`
}

type NutrientCatalog struct {
	SchemaVersion string        `json:"schema_version"`
	CatalogID     string        `json:"catalog_id"`
	Source        string        `json:"source"`
	FixtureNote   string        `json:"fixture_note"`
	Foods         []CatalogFood `json:"foods"`
}

type CatalogFood struct {
	FoodID           string             `json:"food_id"`
	Name             string             `json:"name"`
	Aliases          []string           `json:"aliases"`
	BaseQuantityG    float64            `json:"base_quantity_g"`
	NutrientsPer100G Nutrients          `json:"nutrients_per_100g"`
	UnitConversions  map[string]float64 `json:"unit_conversions"`
	Allergens        []string           `json:"allergens"`
	FoodGroups       []string           `json:"food_groups"`
	SourceRefs       []CatalogSourceRef `json:"source_refs,omitempty"`
}

type CatalogSourceRef struct {
	Source   string `json:"source"`
	SourceID string `json:"source_id,omitempty"`
	DataType string `json:"data_type,omitempty"`
	Note     string `json:"note,omitempty"`
}

type Nutrients struct {
	EnergyKcal    float64 `json:"energy_kcal"`
	ProteinG      float64 `json:"protein_g"`
	CarbohydrateG float64 `json:"carbohydrate_g"`
	FatG          float64 `json:"fat_g"`
	SaturatedFatG float64 `json:"saturated_fat_g"`
	SodiumMG      float64 `json:"sodium_mg"`
	AddedSugarG   float64 `json:"added_sugar_g"`
	FiberG        float64 `json:"fiber_g"`
}

type Evaluation struct {
	CaseID                  string
	Decision                string
	RiskLevel               string
	Summary                 string
	RecommendedAction       string
	Checks                  []CheckResult
	ResolvedItems           []ResolvedItem
	UnresolvedItems         []UnresolvedItem
	ExcludedUnresolvedItems []ExcludedUnresolvedItem
	DailyTotals             []DailyTotal
	MealTotals              []MealTotal
}

type DecisionDocument struct {
	SchemaVersion           string                   `json:"schema_version"`
	CaseID                  string                   `json:"case_id"`
	Decision                string                   `json:"decision"`
	Summary                 string                   `json:"summary"`
	RiskLevel               string                   `json:"risk_level"`
	FailedChecks            []string                 `json:"failed_checks"`
	UnresolvedItems         []UnresolvedItem         `json:"unresolved_items"`
	ExcludedUnresolvedItems []ExcludedUnresolvedItem `json:"excluded_unresolved_items"`
	RecommendedAction       string                   `json:"recommended_action"`
	GuidelinePackID         string                   `json:"guideline_pack_id"`
	ArtifactPaths           map[string]string        `json:"artifact_paths"`
	Checks                  []CheckResult            `json:"checks"`
}

type CheckResult struct {
	CheckID       string           `json:"check_id"`
	Status        string           `json:"status"`
	Severity      string           `json:"severity"`
	Message       string           `json:"message"`
	Evidence      []map[string]any `json:"evidence,omitempty"`
	SourceRefs    []string         `json:"source_refs,omitempty"`
	AffectedDays  []int            `json:"affected_days,omitempty"`
	AffectedMeals []string         `json:"affected_meals,omitempty"`
}

type ResolvedItem struct {
	Day        int       `json:"day"`
	Meal       string    `json:"meal"`
	Food       string    `json:"food"`
	FoodID     string    `json:"food_id"`
	Quantity   float64   `json:"quantity"`
	Unit       string    `json:"unit"`
	Grams      float64   `json:"grams"`
	Nutrients  Nutrients `json:"nutrients"`
	Allergens  []string  `json:"allergens"`
	FoodGroups []string  `json:"food_groups"`
}

type UnresolvedItem struct {
	Day              int      `json:"day"`
	Meal             string   `json:"meal"`
	Food             string   `json:"food"`
	Quantity         *float64 `json:"quantity,omitempty"`
	QuantityText     string   `json:"quantity_text,omitempty"`
	Unit             string   `json:"unit,omitempty"`
	UnresolvedReason string   `json:"unresolved_reason"`
}

type ExcludedUnresolvedItem struct {
	Day                int     `json:"day"`
	Meal               string  `json:"meal"`
	Food               string  `json:"food"`
	Quantity           float64 `json:"quantity"`
	Unit               string  `json:"unit"`
	DeterministicGrams float64 `json:"deterministic_grams"`
	UnresolvedReason   string  `json:"unresolved_reason"`
	ExclusionReason    string  `json:"exclusion_reason"`
	PolicyID           string  `json:"policy_id"`
}

type DailyTotal struct {
	Day                     int             `json:"day"`
	Nutrients               Nutrients       `json:"nutrients"`
	SaturatedFatPctCalories float64         `json:"saturated_fat_pct_calories"`
	FoodGroups              map[string]bool `json:"food_groups"`
}

type MealTotal struct {
	Day       int       `json:"day"`
	Meal      string    `json:"meal"`
	Nutrients Nutrients `json:"nutrients"`
}
