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
