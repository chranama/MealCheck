package checker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func LoadCase(root, casePath string) (Case, Plan, NutrientCatalog, error) {
	var c Case
	if err := readJSON(resolvePath(root, casePath), &c); err != nil {
		return Case{}, Plan{}, NutrientCatalog{}, fmt.Errorf("load case: %w", err)
	}
	if err := validateCaseSettings(c.Settings); err != nil {
		return Case{}, Plan{}, NutrientCatalog{}, fmt.Errorf("load case: %w", err)
	}

	var plan Plan
	if err := readJSON(resolvePath(root, c.CandidatePlan), &plan); err != nil {
		return Case{}, Plan{}, NutrientCatalog{}, fmt.Errorf("load candidate plan: %w", err)
	}

	var catalog NutrientCatalog
	if err := readJSON(resolvePath(root, c.NutrientCatalogPath), &catalog); err != nil {
		return Case{}, Plan{}, NutrientCatalog{}, fmt.Errorf("load nutrient catalog: %w", err)
	}

	return c, plan, catalog, nil
}

func validateCaseSettings(settings Settings) error {
	targets := settings.NutritionTargets
	constraints := settings.VerificationConstraints
	if targets.CalorieTargetKcal <= 0 {
		return fmt.Errorf("settings nutrition_targets calorie_target_kcal must be positive")
	}
	if targets.ProteinTargetG <= 0 {
		return fmt.Errorf("settings nutrition_targets protein_target_g must be positive")
	}
	if constraints.Days < 1 || constraints.Days > 7 {
		return fmt.Errorf("settings verification_constraints days must be between 1 and 7")
	}
	if constraints.MealsPerDay < 1 || constraints.MealsPerDay > 6 {
		return fmt.Errorf("settings verification_constraints meals_per_day must be between 1 and 6")
	}
	return nil
}

func resolvePath(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func readJSON(path string, out any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	return nil
}
