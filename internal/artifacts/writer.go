package artifacts

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/checker"
	"github.com/chranama/MealCheck/internal/recommend"
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

type reportDocument struct {
	SchemaVersion     string                          `json:"schema_version"`
	CaseID            string                          `json:"case_id"`
	Decision          string                          `json:"decision"`
	ProfileSummary    checker.NutritionTargets        `json:"profile_summary"`
	ConstraintSummary checker.VerificationConstraints `json:"constraint_summary"`
	GuidelinePackID   string                          `json:"guideline_pack_id"`
	GuidelinePackName string                          `json:"guideline_pack_name,omitempty"`
	Sections          []reportSection                 `json:"sections"`
	Disclaimer        string                          `json:"disclaimer"`
}

type reportSection struct {
	Title string           `json:"title"`
	Body  string           `json:"body"`
	Items []map[string]any `json:"items,omitempty"`
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

type metricsDocument struct {
	CaseID          string `json:"case_id"`
	Decision        string `json:"decision"`
	ResolvedItems   int    `json:"resolved_items"`
	UnresolvedItems int    `json:"unresolved_items"`
	CheckCount      int    `json:"check_count"`
	BlockCount      int    `json:"block_count"`
	WarnCount       int    `json:"warn_count"`
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

func buildReport(c checker.Case, e checker.Evaluation) reportDocument {
	failures := failedChecks(e.Checks)
	return reportDocument{
		SchemaVersion:     "0.1",
		CaseID:            c.CaseID,
		Decision:          e.Decision,
		ProfileSummary:    c.Settings.NutritionTargets,
		ConstraintSummary: c.Settings.VerificationConstraints,
		GuidelinePackID:   c.GuidelinePackID,
		Sections: []reportSection{
			{
				Title: "Summary",
				Body:  e.Summary,
			},
			{
				Title: "Failed Or Warning Checks",
				Body:  fmt.Sprintf("%d checks require attention.", len(failures)),
				Items: checkItems(failures),
			},
			{
				Title: "Unresolved Foods",
				Body:  fmt.Sprintf("%d unresolved food or quantity items.", len(e.UnresolvedItems)),
				Items: unresolvedItems(e.UnresolvedItems),
			},
			{
				Title: "Daily Totals",
				Body:  "Calculated from the fixture nutrient catalog.",
				Items: dailyTotalItems(e.DailyTotals),
			},
		},
		Disclaimer: "MealCheck checks bounded guideline-derived rules. It does not provide medical nutrition advice.",
	}
}

func buildMetrics(e checker.Evaluation) metricsDocument {
	blockCount := 0
	warnCount := 0
	for _, check := range e.Checks {
		switch check.Status {
		case "block":
			blockCount++
		case "warn":
			warnCount++
		}
	}
	return metricsDocument{
		CaseID:          e.CaseID,
		Decision:        e.Decision,
		ResolvedItems:   len(e.ResolvedItems),
		UnresolvedItems: len(e.UnresolvedItems),
		CheckCount:      len(e.Checks),
		BlockCount:      blockCount,
		WarnCount:       warnCount,
	}
}

func failedChecks(checks []checker.CheckResult) []checker.CheckResult {
	var result []checker.CheckResult
	for _, check := range checks {
		if check.Status == "block" || check.Status == "warn" {
			result = append(result, check)
		}
	}
	return result
}

func checkItems(checks []checker.CheckResult) []map[string]any {
	items := make([]map[string]any, 0, len(checks))
	for _, check := range checks {
		items = append(items, map[string]any{
			"check_id": check.CheckID,
			"status":   check.Status,
			"severity": check.Severity,
			"message":  check.Message,
		})
	}
	return items
}

func unresolvedItems(unresolved []checker.UnresolvedItem) []map[string]any {
	items := make([]map[string]any, 0, len(unresolved))
	for _, item := range unresolved {
		items = append(items, map[string]any{
			"day":               item.Day,
			"meal":              item.Meal,
			"food":              item.Food,
			"quantity_text":     item.QuantityText,
			"unresolved_reason": item.UnresolvedReason,
		})
	}
	return items
}

func dailyTotalItems(totals []checker.DailyTotal) []map[string]any {
	items := make([]map[string]any, 0, len(totals))
	for _, total := range totals {
		items = append(items, map[string]any{
			"day":                          total.Day,
			"energy_kcal":                  total.Nutrients.EnergyKcal,
			"protein_g":                    total.Nutrients.ProteinG,
			"sodium_mg":                    total.Nutrients.SodiumMG,
			"saturated_fat_pct_calories":   total.SaturatedFatPctCalories,
			"added_sugar_g":                total.Nutrients.AddedSugarG,
			"resolved_food_groups_present": total.FoodGroups,
		})
	}
	return items
}

func writeJSON(path string, data any) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

func writeJSONL(path string, checks []checker.CheckResult) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, check := range checks {
		b, err := json.Marshal(check)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return w.Flush()
}

func writeMarkdown(path string, report reportDocument, e checker.Evaluation) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# MealCheck Report\n\n")
	fmt.Fprintf(&b, "- Case: `%s`\n", report.CaseID)
	fmt.Fprintf(&b, "- Decision: `%s`\n", report.Decision)
	fmt.Fprintf(&b, "- Guideline pack: `%s`\n\n", report.GuidelinePackID)
	fmt.Fprintf(&b, "## Summary\n\n%s\n\n", report.Sections[0].Body)
	fmt.Fprintf(&b, "## Checks Requiring Attention\n\n")
	for _, check := range failedChecks(e.Checks) {
		fmt.Fprintf(&b, "- `%s` %s: %s\n", check.CheckID, check.Status, check.Message)
	}
	fmt.Fprintf(&b, "\n## Unresolved Foods\n\n")
	if len(e.UnresolvedItems) == 0 {
		fmt.Fprintf(&b, "None.\n")
	} else {
		for _, item := range e.UnresolvedItems {
			fmt.Fprintf(&b, "- Day %d %s: `%s` (%s)\n", item.Day, item.Meal, item.Food, item.UnresolvedReason)
		}
	}
	fmt.Fprintf(&b, "\n## Daily Totals\n\n")
	for _, total := range e.DailyTotals {
		fmt.Fprintf(&b, "- Day %d: %.1f kcal, %.1f g protein, %.1f mg sodium\n", total.Day, total.Nutrients.EnergyKcal, total.Nutrients.ProteinG, total.Nutrients.SodiumMG)
	}
	fmt.Fprintf(&b, "\n## Disclaimer\n\n%s\n", report.Disclaimer)
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeHTML(path string, report reportDocument, e checker.Evaluation) error {
	var md bytes.Buffer
	md.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><title>MealCheck Report</title></head><body>")
	fmt.Fprintf(&md, "<h1>MealCheck Report</h1><p><strong>Case:</strong> %s</p><p><strong>Decision:</strong> %s</p>", html.EscapeString(report.CaseID), html.EscapeString(report.Decision))
	fmt.Fprintf(&md, "<h2>Summary</h2><p>%s</p>", html.EscapeString(report.Sections[0].Body))
	md.WriteString("<h2>Checks Requiring Attention</h2><ul>")
	for _, check := range failedChecks(e.Checks) {
		fmt.Fprintf(&md, "<li><code>%s</code> %s: %s</li>", html.EscapeString(check.CheckID), html.EscapeString(check.Status), html.EscapeString(check.Message))
	}
	md.WriteString("</ul><h2>Unresolved Foods</h2><ul>")
	for _, item := range e.UnresolvedItems {
		fmt.Fprintf(&md, "<li>Day %d %s: <code>%s</code> (%s)</li>", item.Day, html.EscapeString(item.Meal), html.EscapeString(item.Food), html.EscapeString(item.UnresolvedReason))
	}
	md.WriteString("</ul><h2>Daily Totals</h2><ul>")
	for _, total := range e.DailyTotals {
		fmt.Fprintf(&md, "<li>Day %d: %.1f kcal, %.1f g protein, %.1f mg sodium</li>", total.Day, total.Nutrients.EnergyKcal, total.Nutrients.ProteinG, total.Nutrients.SodiumMG)
	}
	fmt.Fprintf(&md, "</ul><h2>Disclaimer</h2><p>%s</p></body></html>\n", html.EscapeString(report.Disclaimer))
	return os.WriteFile(path, md.Bytes(), 0o644)
}

func writePDF(path string, report reportDocument, e checker.Evaluation) error {
	lines := []string{
		"MealCheck Report",
		"Decision: " + report.Decision,
		"Guideline: " + report.GuidelinePackID,
		"",
		"Summary",
		report.Sections[0].Body,
		"",
		"Checks Requiring Attention",
	}
	failures := failedChecks(e.Checks)
	if len(failures) == 0 {
		lines = append(lines, "None.")
	} else {
		for _, check := range failures {
			lines = append(lines, fmt.Sprintf("%s: %s", strings.ToUpper(check.Status), check.Message))
		}
	}
	lines = append(lines, "", "Unresolved Foods")
	if len(e.UnresolvedItems) == 0 {
		lines = append(lines, "None.")
	} else {
		for _, item := range e.UnresolvedItems {
			lines = append(lines, fmt.Sprintf("Day %d %s: %s (%s)", item.Day, item.Meal, item.Food, item.UnresolvedReason))
		}
	}
	lines = append(lines, "", "Daily Totals")
	for _, total := range e.DailyTotals {
		lines = append(lines, fmt.Sprintf("Day %d: %.1f kcal, %.1f g protein, %.1f mg sodium", total.Day, total.Nutrients.EnergyKcal, total.Nutrients.ProteinG, total.Nutrients.SodiumMG))
	}
	lines = append(lines, "", "Disclaimer", report.Disclaimer)
	return os.WriteFile(path, simplePDF(lines), 0o644)
}

func simplePDF(lines []string) []byte {
	var stream bytes.Buffer
	stream.WriteString("BT\n/F1 12 Tf\n72 760 Td\n14 TL\n")
	for _, line := range lines {
		for _, wrappedLine := range wrapPDFLine(line, 84) {
			fmt.Fprintf(&stream, "(%s) Tj\nT*\n", escapePDFText(wrappedLine))
		}
	}
	stream.WriteString("ET\n")
	content := stream.String()

	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content),
	}

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objects)+1)
	offsets = append(offsets, 0)
	for index, object := range objects {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xrefOffset := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n", len(objects)+1)
	out.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset)
	return out.Bytes()
}

func wrapPDFLine(line string, limit int) []string {
	if line == "" {
		return []string{""}
	}
	var wrapped []string
	words := strings.Fields(line)
	current := ""
	for _, word := range words {
		if current == "" {
			current = word
			continue
		}
		if len(current)+1+len(word) > limit {
			wrapped = append(wrapped, current)
			current = word
			continue
		}
		current += " " + word
	}
	if current != "" {
		wrapped = append(wrapped, current)
	}
	return wrapped
}

func escapePDFText(text string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)")
	return replacer.Replace(strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if r < 32 || r > 126 {
			return '?'
		}
		return r
	}, text))
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
