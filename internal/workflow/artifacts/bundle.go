package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chranama/MealCheck/internal/workflow/checker"
	"github.com/chranama/MealCheck/internal/workflow/recommend"
)

type BundleOptions struct {
	Root              string
	CasePath          string
	OutDir            string
	Mode              string
	FNDDSFallbackPath string
}

type BundleResult struct {
	Decision checker.DecisionDocument
	OutDir   string
}

type manifestDocument struct {
	SchemaVersion string            `json:"schema_version"`
	CaseID        string            `json:"case_id"`
	Mode          string            `json:"mode"`
	GeneratedAt   string            `json:"generated_at"`
	MealCheck     map[string]string `json:"mealcheck"`
	Inputs        map[string]string `json:"inputs"`
	Artifacts     []string          `json:"artifacts"`
}

func WriteBundle(opts BundleOptions) (BundleResult, error) {
	if opts.Root == "" {
		opts.Root = "."
	}
	if opts.CasePath == "" {
		return BundleResult{}, fmt.Errorf("case path is required")
	}
	if opts.OutDir == "" {
		opts.OutDir = filepath.Join("artifacts", "latest")
	}
	if opts.Mode == "" {
		opts.Mode = "validate"
	}

	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return BundleResult{}, err
	}
	outDir := opts.OutDir
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(root, outDir)
	}
	outDir = filepath.Clean(outDir)
	if outDir == root || outDir == string(filepath.Separator) {
		return BundleResult{}, fmt.Errorf("artifact output directory must not be the repository root")
	}

	c, plan, catalog, err := checker.LoadCase(root, opts.CasePath)
	if err != nil {
		return BundleResult{}, err
	}
	var fallback checker.FNDDSReference
	if opts.FNDDSFallbackPath != "" {
		fallbackPath := opts.FNDDSFallbackPath
		if !filepath.IsAbs(fallbackPath) {
			fallbackPath = filepath.Join(root, fallbackPath)
		}
		ref, err := checker.OpenSQLiteFNDDSReference(fallbackPath)
		if err != nil {
			return BundleResult{}, err
		}
		defer ref.Close()
		fallback = ref
	}
	evaluation, err := checker.EvaluateWithFallback(c, plan, catalog, fallback)
	if err != nil {
		return BundleResult{}, err
	}
	decision := evaluation.DecisionDocument(c)
	decision.ArtifactPaths["case"] = opts.CasePath
	decision.ArtifactPaths["recommendation"] = "recommendation.json"

	if err := os.RemoveAll(outDir); err != nil {
		return BundleResult{}, err
	}
	for _, dir := range []string{
		outDir,
		filepath.Join(outDir, "configs"),
		filepath.Join(outDir, "guideline-pack"),
		filepath.Join(outDir, "schemas"),
		filepath.Join(outDir, "optional"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return BundleResult{}, err
		}
	}

	report := buildReport(c, evaluation)
	failures := failedChecks(evaluation.Checks)
	metrics := buildMetrics(evaluation)
	recommendation := recommend.Generate(c, plan, catalog, evaluation)

	writes := []struct {
		path string
		data any
	}{
		{"decision.json", decision},
		{"recommendation.json", recommendation},
		{"report.json", report},
		{"daily-totals.json", evaluation.DailyTotals},
		{"resolved-foods.json", evaluation.ResolvedItems},
		{"unresolved-foods.json", evaluation.UnresolvedItems},
		{"excluded-unresolved-foods.json", evaluation.ExcludedUnresolvedItems},
		{"metrics.json", metrics},
		{"normalized-plan.json", plan},
		{"configs/run.json", c},
		{"configs/redacted-provider.json", map[string]any{"provider": "none", "secrets": "not_applicable"}},
	}
	for _, write := range writes {
		if err := writeJSON(filepath.Join(outDir, write.path), write.data); err != nil {
			return BundleResult{}, err
		}
	}

	if err := writeJSONL(filepath.Join(outDir, "failures.jsonl"), failures); err != nil {
		return BundleResult{}, err
	}
	if err := writeMarkdown(filepath.Join(outDir, "report.md"), report, evaluation); err != nil {
		return BundleResult{}, err
	}
	if err := writeHTML(filepath.Join(outDir, "report.html"), report, evaluation); err != nil {
		return BundleResult{}, err
	}
	if err := writePDF(filepath.Join(outDir, "report.pdf"), report, evaluation); err != nil {
		return BundleResult{}, err
	}
	if err := copyFile(filepath.Join(root, c.GuidelinePackPath), filepath.Join(outDir, "guideline-pack", "pack.json")); err != nil {
		return BundleResult{}, err
	}
	sourceRegistryPath := filepath.Join(filepath.Dir(c.GuidelinePackPath), "source-registry.json")
	if err := copyFile(filepath.Join(root, sourceRegistryPath), filepath.Join(outDir, "guideline-pack", "citations.json")); err != nil {
		return BundleResult{}, err
	}
	for _, schema := range []string{
		"decision.schema.json",
		"report.schema.json",
		"meal-plan.schema.json",
		"guideline-pack.schema.json",
		"nutrient-catalog.schema.json",
	} {
		if err := copyFile(filepath.Join(root, "schemas", schema), filepath.Join(outDir, "schemas", schema)); err != nil {
			return BundleResult{}, err
		}
	}

	manifest := manifestDocument{
		SchemaVersion: "0.1",
		CaseID:        c.CaseID,
		Mode:          opts.Mode,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		MealCheck:     map[string]string{"version": "dev"},
		Inputs: map[string]string{
			"case":                  opts.CasePath,
			"baseline_plan":         c.BaselinePlan,
			"candidate_plan":        c.CandidatePlan,
			"guideline_pack_path":   c.GuidelinePackPath,
			"nutrient_catalog_path": c.NutrientCatalogPath,
		},
		Artifacts: []string{
			"decision.json",
			"recommendation.json",
			"report.json",
			"report.html",
			"report.pdf",
			"report.md",
			"failures.jsonl",
			"daily-totals.json",
			"resolved-foods.json",
			"unresolved-foods.json",
			"excluded-unresolved-foods.json",
			"metrics.json",
			"manifest.json",
			"normalized-plan.json",
			"configs/run.json",
			"configs/redacted-provider.json",
			"guideline-pack/pack.json",
			"guideline-pack/citations.json",
			"schemas/decision.schema.json",
			"schemas/meal-plan.schema.json",
			"schemas/guideline-pack.schema.json",
			"schemas/nutrient-catalog.schema.json",
			"schemas/report.schema.json",
		},
	}
	if err := writeJSON(filepath.Join(outDir, "manifest.json"), manifest); err != nil {
		return BundleResult{}, err
	}

	return BundleResult{Decision: decision, OutDir: outDir}, nil
}
