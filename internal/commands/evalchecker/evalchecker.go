package evalchecker

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/chranama/MealCheck/internal/checker"
)

type evalDataset struct {
	SchemaVersion string         `json:"schema_version"`
	DatasetID     string         `json:"dataset_id"`
	Description   string         `json:"description"`
	CatalogPath   string         `json:"catalog_path"`
	SourceRefs    []sourceRef    `json:"source_refs,omitempty"`
	Cases         []evalCase     `json:"cases"`
	Summary       map[string]any `json:"summary,omitempty"`
}

type sourceRef struct {
	Source   string `json:"source"`
	SourceID string `json:"source_id,omitempty"`
	DataType string `json:"data_type,omitempty"`
	Note     string `json:"note,omitempty"`
}

type evalCase struct {
	CaseID        string           `json:"case_id"`
	Category      string           `json:"category"`
	Description   string           `json:"description"`
	SourceText    string           `json:"source_text"`
	SourceRefs    []sourceRef      `json:"source_refs,omitempty"`
	Settings      checker.Settings `json:"settings"`
	Plan          checker.Plan     `json:"plan"`
	Expected      evalExpected     `json:"expected"`
	Tags          []string         `json:"tags"`
	SourceMetrics map[string]any   `json:"source_metrics,omitempty"`
}

type evalExpected struct {
	Decision           string   `json:"decision,omitempty"`
	UnresolvedCount    *int     `json:"unresolved_count,omitempty"`
	BlockChecks        []string `json:"block_checks,omitempty"`
	WarnChecks         []string `json:"warn_checks,omitempty"`
	AllowExtraWarnings bool     `json:"allow_extra_warnings,omitempty"`
}

type evalResult struct {
	SchemaVersion        string            `json:"schema_version"`
	DatasetID            string            `json:"dataset_id"`
	CatalogID            string            `json:"catalog_id"`
	CatalogFoodCount     int               `json:"catalog_food_count"`
	FNDDSFallbackEnabled bool              `json:"fndds_fallback_enabled"`
	FNDDSFallbackPath    string            `json:"fndds_fallback_path,omitempty"`
	ExpectedComparison   string            `json:"expected_comparison"`
	TotalCases           int               `json:"total_cases"`
	TotalFoodItems       int               `json:"total_food_items"`
	ResolvedItems        int               `json:"resolved_items"`
	ExactResolvedItems   int               `json:"exact_resolved_items"`
	EstimatedItems       int               `json:"estimated_items"`
	DecomposedItems      int               `json:"decomposed_items"`
	UnresolvedItems      int               `json:"unresolved_items"`
	ResolvedRate         float64           `json:"resolved_rate"`
	CasesWithMismatches  int               `json:"cases_with_mismatches"`
	CategorySummary      []categorySummary `json:"category_summary"`
	TopUnresolvedFoods   []rankedCount     `json:"top_unresolved_foods"`
	TopUnresolvedUnits   []rankedCount     `json:"top_unresolved_units"`
	Mismatches           []caseMismatch    `json:"mismatches,omitempty"`
}

type categorySummary struct {
	Category           string  `json:"category"`
	Cases              int     `json:"cases"`
	FoodItems          int     `json:"food_items"`
	ResolvedItems      int     `json:"resolved_items"`
	ExactResolvedItems int     `json:"exact_resolved_items"`
	EstimatedItems     int     `json:"estimated_items"`
	DecomposedItems    int     `json:"decomposed_items"`
	UnresolvedItems    int     `json:"unresolved_items"`
	ResolvedRate       float64 `json:"resolved_rate"`
}

type rankedCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type caseMismatch struct {
	CaseID   string   `json:"case_id"`
	Category string   `json:"category"`
	Messages []string `json:"messages"`
}

type categoryAccumulator struct {
	cases      int
	total      int
	resolved   int
	exact      int
	estimated  int
	decomposed int
	unresolved int
}

func Run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("eval-checker", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root")
	datasetPath := flags.String("dataset", "data/evaluation/fndds-grounded-meal-plans-v1.json", "evaluation dataset path")
	catalogOverride := flags.String("catalog", "", "optional nutrient catalog path override")
	fallbackPath := flags.String("fndds-fallback", "", "optional FNDDS SQLite fallback database path")
	skipExpected := flags.Bool("skip-expected", false, "skip expected outcome comparisons and report coverage only")
	outPath := flags.String("out", "", "optional path to write JSON results")
	allowMismatch := flags.Bool("allow-mismatch", false, "exit successfully even when expected outcomes mismatch")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	result, err := run(*root, *datasetPath, *catalogOverride, *fallbackPath, *skipExpected)
	if err != nil {
		fmt.Fprintf(stderr, "mealcheck eval-checker failed: %v\n", err)
		return 1
	}

	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "encode eval-checker result: %v\n", err)
		return 1
	}
	encoded = append(encoded, '\n')

	if *outPath != "" {
		if err := os.WriteFile(resolvePath(*root, *outPath), encoded, 0o644); err != nil {
			fmt.Fprintf(stderr, "write eval-checker result: %v\n", err)
			return 1
		}
	} else {
		fmt.Fprint(stdout, string(encoded))
	}

	if result.CasesWithMismatches > 0 && !*allowMismatch {
		return 1
	}
	return 0
}

func run(root, datasetPath, catalogOverride, fallbackPath string, skipExpected bool) (evalResult, error) {
	var dataset evalDataset
	if err := readJSON(resolvePath(root, datasetPath), &dataset); err != nil {
		return evalResult{}, fmt.Errorf("read dataset: %w", err)
	}
	if dataset.SchemaVersion != "0.1" {
		return evalResult{}, fmt.Errorf("dataset schema_version = %q, want 0.1", dataset.SchemaVersion)
	}
	if len(dataset.Cases) == 0 {
		return evalResult{}, fmt.Errorf("dataset has no cases")
	}

	catalogPath := dataset.CatalogPath
	if catalogOverride != "" {
		catalogPath = catalogOverride
	}
	var catalog checker.NutrientCatalog
	if err := readJSON(resolvePath(root, catalogPath), &catalog); err != nil {
		return evalResult{}, fmt.Errorf("read catalog: %w", err)
	}

	var fallback checker.FNDDSReference
	if fallbackPath != "" {
		ref, err := checker.OpenSQLiteFNDDSReference(resolvePath(root, fallbackPath))
		if err != nil {
			return evalResult{}, err
		}
		defer ref.Close()
		fallback = ref
	}

	result := evalResult{
		SchemaVersion:        "0.1",
		DatasetID:            dataset.DatasetID,
		CatalogID:            catalog.CatalogID,
		CatalogFoodCount:     len(catalog.Foods),
		FNDDSFallbackEnabled: fallbackPath != "",
		FNDDSFallbackPath:    fallbackPath,
		ExpectedComparison:   "strict",
		TotalCases:           len(dataset.Cases),
	}
	if skipExpected {
		result.ExpectedComparison = "skipped"
	}
	categoryCounts := map[string]*categoryAccumulator{}
	unresolvedFoods := map[string]int{}
	unresolvedUnits := map[string]int{}

	for _, evalCase := range dataset.Cases {
		c := checker.Case{
			SchemaVersion:       "0.1",
			CaseID:              evalCase.CaseID,
			InputMode:           "manual",
			Settings:            evalCase.Settings,
			GuidelinePackID:     "dga-2025-2030-us-adult-general-v1",
			GuidelinePackPath:   "data/guidelines/dga-2025-2030-us-adult-general-v1/guideline-pack.json",
			NutrientCatalogID:   catalog.CatalogID,
			NutrientCatalogPath: catalogPath,
			CandidatePlan:       datasetPath,
		}
		evaluation, err := checker.EvaluateWithFallback(c, evalCase.Plan, catalog, fallback)
		if err != nil {
			return evalResult{}, fmt.Errorf("evaluate case %s: %w", evalCase.CaseID, err)
		}
		itemCount := countPlanItems(evalCase.Plan)
		resolvedCount := len(evaluation.ResolvedItems)
		unresolvedCount := len(evaluation.UnresolvedItems)
		exactCount, estimatedCount, decomposedCount := countResolutionMethods(evaluation.ResolvedItems)

		result.TotalFoodItems += itemCount
		result.ResolvedItems += resolvedCount
		result.ExactResolvedItems += exactCount
		result.EstimatedItems += estimatedCount
		result.DecomposedItems += decomposedCount
		result.UnresolvedItems += unresolvedCount

		acc := categoryCounts[evalCase.Category]
		if acc == nil {
			acc = &categoryAccumulator{}
			categoryCounts[evalCase.Category] = acc
		}
		acc.cases++
		acc.total += itemCount
		acc.resolved += resolvedCount
		acc.exact += exactCount
		acc.estimated += estimatedCount
		acc.decomposed += decomposedCount
		acc.unresolved += unresolvedCount

		for _, unresolved := range evaluation.UnresolvedItems {
			unresolvedFoods[normalizeKey(unresolved.Food)]++
			unit := unresolved.Unit
			if unit == "" {
				unit = "(missing)"
			}
			unresolvedUnits[unit]++
		}

		if !skipExpected {
			mismatch := compareExpected(evalCase, evaluation)
			if len(mismatch.Messages) > 0 {
				result.Mismatches = append(result.Mismatches, mismatch)
			}
		}
	}

	result.ResolvedRate = ratio(result.ResolvedItems, result.TotalFoodItems)
	result.CasesWithMismatches = len(result.Mismatches)
	result.CategorySummary = categorySummaries(categoryCounts)
	result.TopUnresolvedFoods = rankedCounts(unresolvedFoods, 20)
	result.TopUnresolvedUnits = rankedCounts(unresolvedUnits, 20)
	return result, nil
}

func compareExpected(evalCase evalCase, evaluation checker.Evaluation) caseMismatch {
	mismatch := caseMismatch{CaseID: evalCase.CaseID, Category: evalCase.Category}
	expected := evalCase.Expected
	if expected.Decision != "" && evaluation.Decision != expected.Decision {
		mismatch.Messages = append(mismatch.Messages, fmt.Sprintf("decision = %q, want %q", evaluation.Decision, expected.Decision))
	}
	if expected.UnresolvedCount != nil && len(evaluation.UnresolvedItems) != *expected.UnresolvedCount {
		mismatch.Messages = append(mismatch.Messages, fmt.Sprintf("unresolved_count = %d, want %d", len(evaluation.UnresolvedItems), *expected.UnresolvedCount))
	}

	statuses := map[string]string{}
	for _, check := range evaluation.Checks {
		statuses[check.CheckID] = check.Status
	}
	for _, checkID := range expected.BlockChecks {
		if statuses[checkID] != "block" {
			mismatch.Messages = append(mismatch.Messages, fmt.Sprintf("check %s = %q, want block", checkID, statuses[checkID]))
		}
	}
	for _, checkID := range expected.WarnChecks {
		if statuses[checkID] != "warn" {
			mismatch.Messages = append(mismatch.Messages, fmt.Sprintf("check %s = %q, want warn", checkID, statuses[checkID]))
		}
	}
	if !expected.AllowExtraWarnings {
		for _, check := range evaluation.Checks {
			if check.Status == "warn" && !contains(expected.WarnChecks, check.CheckID) {
				mismatch.Messages = append(mismatch.Messages, fmt.Sprintf("unexpected warning check %s", check.CheckID))
			}
		}
	}
	return mismatch
}

func countPlanItems(plan checker.Plan) int {
	total := 0
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			total += len(meal.Items)
		}
	}
	return total
}

func countResolutionMethods(items []checker.ResolvedItem) (int, int, int) {
	exact := 0
	estimated := 0
	decomposed := 0
	for _, item := range items {
		switch item.ResolutionMethod {
		case "estimated":
			estimated++
		case "decomposed":
			decomposed++
		default:
			exact++
		}
	}
	return exact, estimated, decomposed
}

func categorySummaries(counts map[string]*categoryAccumulator) []categorySummary {
	categories := make([]string, 0, len(counts))
	for category := range counts {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	summaries := make([]categorySummary, 0, len(categories))
	for _, category := range categories {
		count := counts[category]
		summaries = append(summaries, categorySummary{
			Category:           category,
			Cases:              count.cases,
			FoodItems:          count.total,
			ResolvedItems:      count.resolved,
			ExactResolvedItems: count.exact,
			EstimatedItems:     count.estimated,
			DecomposedItems:    count.decomposed,
			UnresolvedItems:    count.unresolved,
			ResolvedRate:       ratio(count.resolved, count.total),
		})
	}
	return summaries
}

func rankedCounts(counts map[string]int, limit int) []rankedCount {
	values := make([]rankedCount, 0, len(counts))
	for value, count := range counts {
		values = append(values, rankedCount{Value: value, Count: count})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Count != values[j].Count {
			return values[i].Count > values[j].Count
		}
		return values[i].Value < values[j].Value
	})
	if len(values) > limit {
		values = values[:limit]
	}
	return values
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return math.Round(float64(numerator)/float64(denominator)*10000) / 10000
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func normalizeKey(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func readJSON(path string, out any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func resolvePath(root, path string) string {
	if path == "" || strings.HasPrefix(path, "/") {
		return path
	}
	return strings.TrimRight(root, "/") + "/" + path
}
