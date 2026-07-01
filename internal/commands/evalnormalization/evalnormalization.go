package evalnormalization

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/core"
	llm "github.com/chranama/MealCheck/internal/llm/external"
	localmodel "github.com/chranama/MealCheck/internal/llm/local"
	"github.com/chranama/MealCheck/internal/workflow/checker"
	"github.com/chranama/MealCheck/internal/workflow/normalize"
)

const (
	modeDeterministic = "deterministic"
	modeLocalLlama    = "local_llama"
)

type manifest struct {
	SchemaVersion    string         `json:"schema_version"`
	DatasetID        string         `json:"dataset_id"`
	Description      string         `json:"description,omitempty"`
	CaseFiles        []manifestFile `json:"case_files,omitempty"`
	FailureCaseFiles []manifestFile `json:"failure_case_files,omitempty"`
	QuarantineFiles  []manifestFile `json:"quarantine_files,omitempty"`
	SourceRefs       []sourceRef    `json:"source_refs,omitempty"`
	Summary          map[string]any `json:"summary,omitempty"`
}

type manifestFile struct {
	Path          string `json:"path"`
	SourceDataset string `json:"source_dataset,omitempty"`
	Gate          string `json:"gate,omitempty"`
}

func (f *manifestFile) UnmarshalJSON(data []byte) error {
	var path string
	if err := json.Unmarshal(data, &path); err == nil {
		f.Path = path
		f.Gate = "strict"
		return nil
	}
	var object struct {
		Path          string `json:"path"`
		SourceDataset string `json:"source_dataset,omitempty"`
		Gate          string `json:"gate,omitempty"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	f.Path = object.Path
	f.SourceDataset = object.SourceDataset
	f.Gate = object.Gate
	if f.Gate == "" {
		f.Gate = "strict"
	}
	return nil
}

type sourceRef struct {
	Source   string `json:"source"`
	SourceID string `json:"source_id,omitempty"`
	DataType string `json:"data_type,omitempty"`
	Note     string `json:"note,omitempty"`
}

type successCase struct {
	SchemaVersion string         `json:"schema_version"`
	ID            string         `json:"id"`
	SourceDataset string         `json:"source_dataset"`
	SourceRef     map[string]any `json:"source_ref,omitempty"`
	InputText     string         `json:"input_text"`
	Expected      expectedBlock  `json:"expected"`
	Tags          []string       `json:"tags,omitempty"`
}

type expectedBlock struct {
	Days        []int                `json:"days,omitempty"`
	SourceItems []expectedSourceItem `json:"source_items"`
}

type expectedSourceItem struct {
	SourceItemID int     `json:"source_item_id"`
	Day          int     `json:"day"`
	MealCode     string  `json:"meal_code"`
	SourceText   string  `json:"source_text"`
	Food         string  `json:"food"`
	Quantity     float64 `json:"quantity"`
	Unit         string  `json:"unit"`
}

type failureCase struct {
	SchemaVersion   string          `json:"schema_version"`
	ID              string          `json:"id"`
	SourceDataset   string          `json:"source_dataset"`
	SourceRef       map[string]any  `json:"source_ref,omitempty"`
	InputText       string          `json:"input_text"`
	ExpectedFailure expectedFailure `json:"expected_failure"`
	Tags            []string        `json:"tags,omitempty"`
}

type expectedFailure struct {
	Stage  string `json:"stage"`
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type result struct {
	SchemaVersion               string              `json:"schema_version"`
	DatasetID                   string              `json:"dataset_id"`
	Mode                        string              `json:"mode"`
	TotalCases                  int                 `json:"total_cases"`
	CasesPassed                 int                 `json:"cases_passed"`
	CasesWithMismatches         int                 `json:"cases_with_mismatches"`
	SuccessCases                int                 `json:"success_cases"`
	SuccessCasesPassed          int                 `json:"success_cases_passed"`
	FailureCases                int                 `json:"failure_cases"`
	FailureCasesPassed          int                 `json:"failure_cases_passed"`
	TotalExpectedSourceItems    int                 `json:"total_expected_source_items"`
	SourceItemsMatched          int                 `json:"source_items_matched"`
	SourceItemPreservationRate  float64             `json:"source_item_preservation_rate"`
	AdapterValidCases           int                 `json:"adapter_valid_cases"`
	LocalModelRepeatsRequested  int                 `json:"local_model_repeats_requested,omitempty"`
	LocalModelSuccessCasesRun   int                 `json:"local_model_success_cases_run,omitempty"`
	LocalModelSuccessCasesPass  int                 `json:"local_model_success_cases_pass,omitempty"`
	LocalModelExpectedItems     int                 `json:"local_model_expected_items,omitempty"`
	LocalModelRowsMatched       int                 `json:"local_model_rows_matched,omitempty"`
	LocalModelRowMatchRate      float64             `json:"local_model_row_match_rate,omitempty"`
	LocalModelDayMatched        int                 `json:"local_model_day_matched,omitempty"`
	LocalModelDayAccuracy       float64             `json:"local_model_day_accuracy,omitempty"`
	LocalModelMealMatched       int                 `json:"local_model_meal_matched,omitempty"`
	LocalModelMealAccuracy      float64             `json:"local_model_meal_accuracy,omitempty"`
	LocalModelFoodMatched       int                 `json:"local_model_food_matched,omitempty"`
	LocalModelFoodAccuracy      float64             `json:"local_model_food_accuracy,omitempty"`
	LocalModelQuantityMatched   int                 `json:"local_model_quantity_matched,omitempty"`
	LocalModelQuantityAccuracy  float64             `json:"local_model_quantity_accuracy,omitempty"`
	LocalModelUnitMatched       int                 `json:"local_model_unit_matched,omitempty"`
	LocalModelUnitAccuracy      float64             `json:"local_model_unit_accuracy,omitempty"`
	LocalModelSourceRepairs     int                 `json:"local_model_source_repairs,omitempty"`
	LocalModelRepairCases       int                 `json:"local_model_repair_cases,omitempty"`
	LocalModelProviderFailures  int                 `json:"local_model_provider_failures,omitempty"`
	LocalModelDecodeFailures    int                 `json:"local_model_decode_failures,omitempty"`
	LocalModelUnstableCases     int                 `json:"local_model_unstable_cases,omitempty"`
	QualificationFailuresRun    int                 `json:"qualification_failures_run"`
	QualificationFailuresPass   int                 `json:"qualification_failures_pass"`
	LocalModelRepeatSummary     []repeatSummary     `json:"local_model_repeat_summary,omitempty"`
	LocalModelCaseRepeatSummary []caseRepeatSummary `json:"local_model_case_repeat_summary,omitempty"`
	GateSummary                 []gateSummary       `json:"gate_summary,omitempty"`
	SourceDatasetSummary        []datasetSummary    `json:"source_dataset_summary,omitempty"`
	QuarantineSummary           quarantineSummary   `json:"quarantine_summary,omitempty"`
	TagSummary                  []tagSummary        `json:"tag_summary,omitempty"`
	FailureSummary              []rankedCount       `json:"failure_summary,omitempty"`
	Mismatches                  []caseMismatch      `json:"mismatches,omitempty"`
}

type tagSummary struct {
	Tag    string  `json:"tag"`
	Cases  int     `json:"cases"`
	Passed int     `json:"passed"`
	Rate   float64 `json:"rate"`
}

type repeatSummary struct {
	Repeat              int     `json:"repeat"`
	SuccessCasesRun     int     `json:"success_cases_run"`
	SuccessCasesPass    int     `json:"success_cases_pass"`
	CasesWithMismatches int     `json:"cases_with_mismatches"`
	ExpectedItems       int     `json:"expected_items"`
	RowsMatched         int     `json:"rows_matched"`
	RowMatchRate        float64 `json:"row_match_rate"`
	DayAccuracy         float64 `json:"day_accuracy"`
	MealAccuracy        float64 `json:"meal_accuracy"`
	FoodAccuracy        float64 `json:"food_accuracy"`
	QuantityAccuracy    float64 `json:"quantity_accuracy"`
	UnitAccuracy        float64 `json:"unit_accuracy"`
	SourceRepairs       int     `json:"source_repairs,omitempty"`
	RepairCases         int     `json:"repair_cases,omitempty"`
	ProviderFailures    int     `json:"provider_failures,omitempty"`
	DecodeFailures      int     `json:"decode_failures,omitempty"`
}

type caseRepeatSummary struct {
	CaseID           string  `json:"case_id"`
	Repeats          int     `json:"repeats"`
	Passes           int     `json:"passes"`
	Failures         int     `json:"failures"`
	MinRowMatchRate  float64 `json:"min_row_match_rate"`
	MeanRowMatchRate float64 `json:"mean_row_match_rate"`
	MaxRowMatchRate  float64 `json:"max_row_match_rate"`
}

type gateSummary struct {
	Gate   string  `json:"gate"`
	Cases  int     `json:"cases"`
	Passed int     `json:"passed"`
	Rate   float64 `json:"rate"`
}

type datasetSummary struct {
	SourceDataset string  `json:"source_dataset"`
	Cases         int     `json:"cases"`
	Passed        int     `json:"passed"`
	Rate          float64 `json:"rate"`
}

type quarantineSummary struct {
	Files int `json:"files,omitempty"`
	Rows  int `json:"rows,omitempty"`
}

type rankedCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type caseMismatch struct {
	CaseID   string   `json:"case_id"`
	CaseType string   `json:"case_type"`
	Tags     []string `json:"tags,omitempty"`
	Messages []string `json:"messages"`
}

type tagAccumulator struct {
	cases  int
	passed int
}

type loadedSuccessCase struct {
	Case successCase
	Gate string
}

type loadedFailureCase struct {
	Case failureCase
	Gate string
}

type runOptions struct {
	Root              string
	ManifestPath      string
	DatasetPath       string
	FailurePath       string
	Gate              string
	SourceDataset     string
	Mode              string
	LocalModelRepeats int
	ProviderConfig    core.ProviderConfig
	ProviderFactory   llm.ProviderFactory
}

// Run executes the P0 meal-plan normalization evaluation.
func Run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("eval-normalization", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root")
	manifestPath := flags.String("manifest", "data/evaluation/p0-normalization/manifest.json", "P0 normalization manifest path")
	datasetPath := flags.String("dataset", "", "optional P0 success-case JSONL path override")
	failurePath := flags.String("failures", "", "optional P0 failure-case JSONL path override")
	gate := flags.String("gate", "strict", "manifest gate to run: strict, exploratory, or all")
	sourceDataset := flags.String("source-dataset", "", "optional source_dataset filter for manifest-driven runs")
	outPath := flags.String("out", "", "optional path to write JSON results")
	mode := flags.String("mode", modeDeterministic, "evaluation mode: deterministic or local-llama")
	localModelBaseURL := flags.String("local-model-base-url", "", "local llama OpenAI-compatible base URL; defaults to MEALCHECK_LOCAL_MODEL_BASE_URL")
	localModelName := flags.String("local-model-name", "", "local llama model name; defaults to MEALCHECK_LOCAL_MODEL_NAME")
	localModelMaxOutputTokens := flags.Int("local-model-max-output-tokens", 0, "local llama max output tokens; defaults to MEALCHECK_LOCAL_MODEL_MAX_OUTPUT_TOKENS")
	localModelTimeout := flags.Duration("local-model-timeout", 0, "local llama request timeout; defaults to MEALCHECK_LOCAL_MODEL_TIMEOUT")
	localModelRepeats := flags.Int("local-model-repeats", envPositiveInt("MEALCHECK_P0_REPEATS", 1), "local llama repeats per success case; defaults to MEALCHECK_P0_REPEATS or 1")
	allowMismatch := flags.Bool("allow-mismatch", false, "exit successfully even when expected outcomes mismatch")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "eval-normalization does not accept positional arguments")
		return 2
	}

	envConfig := core.ConfigFromEnv(*root)
	providerConfig := core.ProviderConfig{
		Type:      llm.ProviderTypeLocalLlama,
		BaseURL:   firstNonEmpty(*localModelBaseURL, envConfig.LocalModelBaseURL),
		Model:     firstNonEmpty(*localModelName, envConfig.LocalModelName),
		MaxTokens: firstPositiveInt(*localModelMaxOutputTokens, envConfig.LocalModelMaxOutputTokens),
		Timeout:   firstPositiveDuration(*localModelTimeout, envConfig.LocalModelTimeout),
	}
	result, err := run(runOptions{
		Root:              *root,
		ManifestPath:      *manifestPath,
		DatasetPath:       *datasetPath,
		FailurePath:       *failurePath,
		Gate:              *gate,
		SourceDataset:     *sourceDataset,
		Mode:              *mode,
		LocalModelRepeats: *localModelRepeats,
		ProviderConfig:    providerConfig,
		ProviderFactory:   llm.DefaultProviderFactory,
	})
	if err != nil {
		fmt.Fprintf(stderr, "mealcheck eval-normalization failed: %v\n", err)
		return 1
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "encode normalization eval result: %v\n", err)
		return 1
	}
	encoded = append(encoded, '\n')
	if *outPath != "" {
		if err := os.WriteFile(resolvePath(*root, *outPath), encoded, 0o644); err != nil {
			fmt.Fprintf(stderr, "write normalization eval result: %v\n", err)
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

func run(opts runOptions) (result, error) {
	mode, err := normalizeMode(opts.Mode)
	if err != nil {
		return result{}, err
	}
	if mode == modeLocalLlama {
		if opts.ProviderConfig.Type == "" {
			opts.ProviderConfig.Type = llm.ProviderTypeLocalLlama
		}
		if opts.ProviderConfig.Type != llm.ProviderTypeLocalLlama {
			return result{}, fmt.Errorf("local-llama mode requires provider type %q", llm.ProviderTypeLocalLlama)
		}
		if strings.TrimSpace(opts.ProviderConfig.Model) == "" {
			return result{}, fmt.Errorf("local-llama mode requires -local-model-name or MEALCHECK_LOCAL_MODEL_NAME")
		}
	}
	localModelRepeats, err := normalizeLocalModelRepeats(mode, opts.LocalModelRepeats)
	if err != nil {
		return result{}, err
	}
	if opts.ProviderFactory == nil {
		opts.ProviderFactory = llm.DefaultProviderFactory
	}

	var m manifest
	if err := readJSON(resolvePath(opts.Root, opts.ManifestPath), &m); err != nil {
		return result{}, fmt.Errorf("read manifest: %w", err)
	}
	if m.SchemaVersion != "0.1" {
		return result{}, fmt.Errorf("manifest schema_version = %q, want 0.1", m.SchemaVersion)
	}
	if strings.TrimSpace(m.DatasetID) == "" {
		return result{}, fmt.Errorf("manifest dataset_id is required")
	}
	successCases, failureCases, quarantine, err := loadEvaluationCases(opts, m)
	if err != nil {
		return result{}, err
	}
	if len(successCases)+len(failureCases) == 0 {
		return result{}, fmt.Errorf("normalization evaluation has no cases")
	}

	r := result{
		SchemaVersion:     "0.1",
		DatasetID:         m.DatasetID,
		Mode:              mode,
		SuccessCases:      len(successCases),
		FailureCases:      len(failureCases),
		TotalCases:        len(successCases) + len(failureCases),
		QuarantineSummary: quarantine,
	}
	if mode == modeLocalLlama {
		r.LocalModelRepeatsRequested = localModelRepeats
		r.LocalModelRepeatSummary = make([]repeatSummary, localModelRepeats)
		for i := range r.LocalModelRepeatSummary {
			r.LocalModelRepeatSummary[i].Repeat = i + 1
		}
	}
	tagCounts := map[string]*tagAccumulator{}
	gateCounts := map[string]*tagAccumulator{}
	datasetCounts := map[string]*tagAccumulator{}
	failureCounts := map[string]int{}

	for _, loaded := range successCases {
		c := loaded.Case
		mismatch, matchedItems, adapterOK := evaluateSuccessCase(c)
		r.TotalExpectedSourceItems += len(c.Expected.SourceItems)
		r.SourceItemsMatched += matchedItems
		if adapterOK {
			r.AdapterValidCases++
		}
		if mode == modeLocalLlama {
			caseSummary := caseRepeatSummary{
				CaseID:  c.ID,
				Repeats: localModelRepeats,
			}
			for repeatIndex := 1; repeatIndex <= localModelRepeats; repeatIndex++ {
				localMessages, localMetrics, localRepairs, localFailure := evaluateLocalModelSuccessCase(c, opts.ProviderFactory, opts.ProviderConfig)
				repeat := &r.LocalModelRepeatSummary[repeatIndex-1]
				recordLocalModelAttempt(&r, repeat, c, localMetrics, localRepairs, localMessages, localFailure)
				rowRate := ratio(localMetrics.MatchedRows, len(c.Expected.SourceItems))
				recordCaseRepeatAttempt(&caseSummary, rowRate, len(localMessages) == 0)
				if len(localMessages) != 0 {
					for _, message := range localMessages {
						mismatch.Messages = append(mismatch.Messages, fmt.Sprintf("repeat_%d_%s", repeatIndex, message))
					}
				}
			}
			finalizeCaseRepeatSummary(&caseSummary)
			if shouldReportCaseRepeat(caseSummary) {
				r.LocalModelCaseRepeatSummary = append(r.LocalModelCaseRepeatSummary, caseSummary)
			}
			if isUnstableCaseRepeat(caseSummary) {
				r.LocalModelUnstableCases++
			}
		}
		passed := len(mismatch.Messages) == 0
		if passed {
			r.SuccessCasesPassed++
		} else {
			r.Mismatches = append(r.Mismatches, mismatch)
			for _, message := range mismatch.Messages {
				failureCounts[failureCategory(message)]++
			}
		}
		recordTags(tagCounts, c.Tags, passed)
		recordAccumulator(gateCounts, loaded.Gate, passed)
		recordAccumulator(datasetCounts, c.SourceDataset, passed)
	}

	for _, loaded := range failureCases {
		c := loaded.Case
		mismatch := evaluateFailureCase(c)
		passed := len(mismatch.Messages) == 0
		if c.ExpectedFailure.Stage == "qualification" {
			r.QualificationFailuresRun++
			if passed {
				r.QualificationFailuresPass++
			}
		}
		if passed {
			r.FailureCasesPassed++
		} else {
			r.Mismatches = append(r.Mismatches, mismatch)
			for _, message := range mismatch.Messages {
				failureCounts[failureCategory(message)]++
			}
		}
		recordTags(tagCounts, c.Tags, passed)
		recordAccumulator(gateCounts, loaded.Gate, passed)
		recordAccumulator(datasetCounts, c.SourceDataset, passed)
	}

	r.CasesPassed = r.SuccessCasesPassed + r.FailureCasesPassed
	r.CasesWithMismatches = len(r.Mismatches)
	r.SourceItemPreservationRate = ratio(r.SourceItemsMatched, r.TotalExpectedSourceItems)
	r.LocalModelRowMatchRate = ratio(r.LocalModelRowsMatched, r.LocalModelExpectedItems)
	r.LocalModelDayAccuracy = ratio(r.LocalModelDayMatched, r.LocalModelExpectedItems)
	r.LocalModelMealAccuracy = ratio(r.LocalModelMealMatched, r.LocalModelExpectedItems)
	r.LocalModelFoodAccuracy = ratio(r.LocalModelFoodMatched, r.LocalModelExpectedItems)
	r.LocalModelQuantityAccuracy = ratio(r.LocalModelQuantityMatched, r.LocalModelExpectedItems)
	r.LocalModelUnitAccuracy = ratio(r.LocalModelUnitMatched, r.LocalModelExpectedItems)
	finalizeRepeatSummaries(r.LocalModelRepeatSummary)
	r.GateSummary = gateSummaries(gateCounts)
	r.SourceDatasetSummary = datasetSummaries(datasetCounts)
	r.TagSummary = tagSummaries(tagCounts)
	r.FailureSummary = rankedCounts(failureCounts)
	return r, nil
}

func loadEvaluationCases(opts runOptions, m manifest) ([]loadedSuccessCase, []loadedFailureCase, quarantineSummary, error) {
	if opts.DatasetPath != "" || opts.FailurePath != "" {
		datasetPath := opts.DatasetPath
		if datasetPath == "" {
			datasetPath = "data/evaluation/p0-normalization/cases-v1.jsonl"
		}
		failurePath := opts.FailurePath
		if failurePath == "" {
			failurePath = "data/evaluation/p0-normalization/failure-cases-v1.jsonl"
		}
		successCases, err := readSuccessCases(resolvePath(opts.Root, datasetPath))
		if err != nil {
			return nil, nil, quarantineSummary{}, err
		}
		failureCases, err := readFailureCases(resolvePath(opts.Root, failurePath))
		if err != nil {
			return nil, nil, quarantineSummary{}, err
		}
		loadedSuccess := make([]loadedSuccessCase, 0, len(successCases))
		for _, c := range successCases {
			loadedSuccess = append(loadedSuccess, loadedSuccessCase{Case: c, Gate: "direct"})
		}
		loadedFailure := make([]loadedFailureCase, 0, len(failureCases))
		for _, c := range failureCases {
			loadedFailure = append(loadedFailure, loadedFailureCase{Case: c, Gate: "direct"})
		}
		return loadedSuccess, loadedFailure, quarantineSummary{}, nil
	}

	gate, err := normalizeGate(opts.Gate)
	if err != nil {
		return nil, nil, quarantineSummary{}, err
	}
	manifestPath := resolvePath(opts.Root, opts.ManifestPath)
	manifestDir := filepath.Dir(manifestPath)

	successFiles := selectedManifestFiles(m.CaseFiles, gate, opts.SourceDataset)
	failureFiles := selectedManifestFiles(m.FailureCaseFiles, gate, opts.SourceDataset)
	if len(successFiles)+len(failureFiles) == 0 {
		return nil, nil, quarantineSummary{}, fmt.Errorf("manifest has no P0 files for gate %q source_dataset %q", gate, opts.SourceDataset)
	}

	var successCases []loadedSuccessCase
	for _, file := range successFiles {
		rows, err := readSuccessCases(resolveManifestPath(manifestDir, file.Path))
		if err != nil {
			return nil, nil, quarantineSummary{}, err
		}
		for _, row := range rows {
			successCases = append(successCases, loadedSuccessCase{Case: row, Gate: file.Gate})
		}
	}
	var failureCases []loadedFailureCase
	for _, file := range failureFiles {
		rows, err := readFailureCases(resolveManifestPath(manifestDir, file.Path))
		if err != nil {
			return nil, nil, quarantineSummary{}, err
		}
		for _, row := range rows {
			failureCases = append(failureCases, loadedFailureCase{Case: row, Gate: file.Gate})
		}
	}

	var quarantine quarantineSummary
	for _, file := range selectedManifestFiles(m.QuarantineFiles, gate, opts.SourceDataset) {
		count, err := countQuarantineRows(resolveManifestPath(manifestDir, file.Path))
		if err != nil {
			return nil, nil, quarantineSummary{}, err
		}
		quarantine.Files++
		quarantine.Rows += count
	}
	return successCases, failureCases, quarantine, nil
}

func normalizeGate(gate string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(gate)) {
	case "", "strict":
		return "strict", nil
	case "exploratory":
		return "exploratory", nil
	case "all":
		return "all", nil
	default:
		return "", fmt.Errorf("unsupported gate %q", gate)
	}
}

func selectedManifestFiles(files []manifestFile, gate string, sourceDataset string) []manifestFile {
	var selected []manifestFile
	for _, file := range files {
		if strings.TrimSpace(file.Path) == "" {
			continue
		}
		fileGate := strings.TrimSpace(file.Gate)
		if fileGate == "" {
			fileGate = "strict"
		}
		if gate != "all" && fileGate != gate {
			continue
		}
		if sourceDataset != "" && file.SourceDataset != sourceDataset {
			continue
		}
		file.Gate = fileGate
		selected = append(selected, file)
	}
	return selected
}

func resolveManifestPath(manifestDir string, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(manifestDir, path)
}

func countQuarantineRows(path string) (int, error) {
	count := 0
	err := readJSONL(path, func(line []byte) error {
		var row map[string]any
		if err := json.Unmarshal(line, &row); err != nil {
			return err
		}
		count++
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("read quarantine rows: %w", err)
	}
	return count, nil
}

func evaluateSuccessCase(c successCase) (caseMismatch, int, bool) {
	mismatch := caseMismatch{CaseID: c.ID, CaseType: "success", Tags: append([]string(nil), c.Tags...)}
	var matchedItems int

	if c.SchemaVersion != "0.1" {
		mismatch.Messages = append(mismatch.Messages, fmt.Sprintf("schema_version = %q, want 0.1", c.SchemaVersion))
	}
	if strings.TrimSpace(c.ID) == "" {
		mismatch.Messages = append(mismatch.Messages, "id is required")
	}
	if strings.TrimSpace(c.InputText) == "" {
		mismatch.Messages = append(mismatch.Messages, "input_text is required")
	}
	if len(c.Expected.SourceItems) == 0 {
		mismatch.Messages = append(mismatch.Messages, "expected.source_items must not be empty")
	}

	actualItems := localmodel.LocalLlamaResolvedSourceItems(c.InputText)
	if len(actualItems) != len(c.Expected.SourceItems) {
		mismatch.Messages = append(mismatch.Messages, fmt.Sprintf("source_inventory_count_failed: got %d source item(s), want %d", len(actualItems), len(c.Expected.SourceItems)))
	}
	for index, expected := range c.Expected.SourceItems {
		if index >= len(actualItems) {
			continue
		}
		actual := actualItems[index]
		messages := compareSourceItem(actual, expected)
		if len(messages) == 0 {
			matchedItems++
			continue
		}
		for _, message := range messages {
			mismatch.Messages = append(mismatch.Messages, fmt.Sprintf("source_item_%d_%s", expected.SourceItemID, message))
		}
	}

	adapterOK := true
	plan, err := localmodel.DecodeLocalLlamaCompactPlan(compactRowsJSON(c.Expected.SourceItems), "p0-normalization-eval")
	if err != nil {
		adapterOK = false
		mismatch.Messages = append(mismatch.Messages, fmt.Sprintf("adapter_validation_failed: %v", err))
	} else {
		for _, message := range comparePlanRows(plan, c.Expected.SourceItems) {
			mismatch.Messages = append(mismatch.Messages, message)
		}
	}
	return mismatch, matchedItems, adapterOK
}

func evaluateLocalModelSuccessCase(c successCase, providerFactory llm.ProviderFactory, providerConfig core.ProviderConfig) ([]string, rowComparisonMetrics, int, string) {
	provider, err := providerFactory(providerConfig)
	if err != nil {
		return []string{fmt.Sprintf("local_model_provider_failed: %v", err)}, rowComparisonMetrics{}, 0, "provider"
	}
	ctx := context.Background()
	var cancel context.CancelFunc
	if providerConfig.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, providerConfig.Timeout)
		defer cancel()
	}
	_, plan, repairs, stage, err := localmodel.RunLocalModelExtraction(ctx, provider, providerConfig, core.PendingRunInput{
		Mode:          core.InputModeLocalModel,
		CandidateText: c.InputText,
		Provider:      providerConfig,
	}, "p0-normalization-local-model-eval")
	if err != nil {
		failure := "decode"
		if stage == "provider" {
			failure = "provider"
		} else if stage == "completeness" {
			failure = "decode"
		}
		return []string{fmt.Sprintf("local_model_%s_failed: %v", failure, err)}, rowComparisonMetrics{}, len(repairs), failure
	}
	compareMessages, metrics := comparePlanRowsDetailed(plan, c.Expected.SourceItems)
	for i := range compareMessages {
		compareMessages[i] = "local_model_" + compareMessages[i]
	}
	return compareMessages, metrics, len(repairs), ""
}

func recordLocalModelAttempt(r *result, repeat *repeatSummary, c successCase, metrics rowComparisonMetrics, repairs int, messages []string, failure string) {
	expectedItems := len(c.Expected.SourceItems)
	r.LocalModelSuccessCasesRun++
	r.LocalModelExpectedItems += expectedItems
	r.LocalModelRowsMatched += metrics.MatchedRows
	r.LocalModelDayMatched += metrics.DayMatched
	r.LocalModelMealMatched += metrics.MealMatched
	r.LocalModelFoodMatched += metrics.FoodMatched
	r.LocalModelQuantityMatched += metrics.QuantityMatched
	r.LocalModelUnitMatched += metrics.UnitMatched
	r.LocalModelSourceRepairs += repairs

	repeat.SuccessCasesRun++
	repeat.ExpectedItems += expectedItems
	repeat.RowsMatched += metrics.MatchedRows
	repeat.DayAccuracy += float64(metrics.DayMatched)
	repeat.MealAccuracy += float64(metrics.MealMatched)
	repeat.FoodAccuracy += float64(metrics.FoodMatched)
	repeat.QuantityAccuracy += float64(metrics.QuantityMatched)
	repeat.UnitAccuracy += float64(metrics.UnitMatched)
	repeat.SourceRepairs += repairs

	if repairs > 0 {
		r.LocalModelRepairCases++
		repeat.RepairCases++
	}
	if len(messages) == 0 {
		r.LocalModelSuccessCasesPass++
		repeat.SuccessCasesPass++
		return
	}
	repeat.CasesWithMismatches++
	switch failure {
	case "provider":
		r.LocalModelProviderFailures++
		repeat.ProviderFailures++
	case "decode":
		r.LocalModelDecodeFailures++
		repeat.DecodeFailures++
	}
}

func recordCaseRepeatAttempt(summary *caseRepeatSummary, rowMatchRate float64, passed bool) {
	if passed {
		summary.Passes++
	} else {
		summary.Failures++
	}
	if summary.Passes+summary.Failures == 1 {
		summary.MinRowMatchRate = rowMatchRate
		summary.MaxRowMatchRate = rowMatchRate
	} else {
		if rowMatchRate < summary.MinRowMatchRate {
			summary.MinRowMatchRate = rowMatchRate
		}
		if rowMatchRate > summary.MaxRowMatchRate {
			summary.MaxRowMatchRate = rowMatchRate
		}
	}
	summary.MeanRowMatchRate += rowMatchRate
}

func shouldReportCaseRepeat(summary caseRepeatSummary) bool {
	if summary.Repeats <= 1 {
		return false
	}
	return summary.Failures > 0 || summary.MinRowMatchRate != summary.MaxRowMatchRate
}

func isUnstableCaseRepeat(summary caseRepeatSummary) bool {
	if summary.Repeats <= 1 {
		return false
	}
	return (summary.Passes > 0 && summary.Failures > 0) || summary.MinRowMatchRate != summary.MaxRowMatchRate
}

func finalizeCaseRepeatSummary(summary *caseRepeatSummary) {
	attempts := summary.Passes + summary.Failures
	if attempts == 0 {
		return
	}
	summary.MeanRowMatchRate = summary.MeanRowMatchRate / float64(attempts)
}

func finalizeRepeatSummaries(summaries []repeatSummary) {
	for i := range summaries {
		summary := &summaries[i]
		expectedItems := summary.ExpectedItems
		summary.RowMatchRate = ratio(summary.RowsMatched, expectedItems)
		summary.DayAccuracy = ratioFloat(summary.DayAccuracy, expectedItems)
		summary.MealAccuracy = ratioFloat(summary.MealAccuracy, expectedItems)
		summary.FoodAccuracy = ratioFloat(summary.FoodAccuracy, expectedItems)
		summary.QuantityAccuracy = ratioFloat(summary.QuantityAccuracy, expectedItems)
		summary.UnitAccuracy = ratioFloat(summary.UnitAccuracy, expectedItems)
	}
}

func compareSourceItem(actual localmodel.LocalLlamaSourceItem, expected expectedSourceItem) []string {
	var messages []string
	if actual.ID != expected.SourceItemID {
		messages = append(messages, fmt.Sprintf("id_failed: got %d, want %d", actual.ID, expected.SourceItemID))
	}
	if actual.Day != expected.Day {
		messages = append(messages, fmt.Sprintf("day_failed: got %d, want %d", actual.Day, expected.Day))
	}
	if actual.MealCode != expected.MealCode {
		messages = append(messages, fmt.Sprintf("meal_failed: got %q, want %q", actual.MealCode, expected.MealCode))
	}
	if actual.Text != expected.SourceText {
		messages = append(messages, fmt.Sprintf("source_text_failed: got %q, want %q", actual.Text, expected.SourceText))
	}
	return messages
}

func evaluateFailureCase(c failureCase) caseMismatch {
	mismatch := caseMismatch{CaseID: c.ID, CaseType: "failure", Tags: append([]string(nil), c.Tags...)}
	if c.SchemaVersion != "0.1" {
		mismatch.Messages = append(mismatch.Messages, fmt.Sprintf("schema_version = %q, want 0.1", c.SchemaVersion))
	}
	if strings.TrimSpace(c.ID) == "" {
		mismatch.Messages = append(mismatch.Messages, "id is required")
	}
	if strings.TrimSpace(c.InputText) == "" {
		mismatch.Messages = append(mismatch.Messages, "input_text is required")
	}
	switch c.ExpectedFailure.Stage {
	case "qualification":
		qualification, err := normalize.QualifyMealPlanText(context.Background(), nil, core.MealPlanQualificationRequest{
			Text: c.InputText,
		})
		if err != nil {
			mismatch.Messages = append(mismatch.Messages, fmt.Sprintf("qualification_failed: %v", err))
			return mismatch
		}
		if c.ExpectedFailure.Status != "" && qualification.Status != c.ExpectedFailure.Status {
			mismatch.Messages = append(mismatch.Messages, fmt.Sprintf("qualification_status_failed: got %q, want %q", qualification.Status, c.ExpectedFailure.Status))
		}
	case "source_inventory":
		items := localmodel.LocalLlamaResolvedSourceItems(c.InputText)
		if len(items) != 0 {
			mismatch.Messages = append(mismatch.Messages, fmt.Sprintf("source_inventory_failed: got %d source item(s), want 0", len(items)))
		}
	default:
		mismatch.Messages = append(mismatch.Messages, fmt.Sprintf("unsupported_failure_stage: %q", c.ExpectedFailure.Stage))
	}
	return mismatch
}

func compactRowsJSON(items []expectedSourceItem) string {
	rows := make([][]any, 0, len(items))
	for _, item := range items {
		rows = append(rows, []any{item.SourceItemID, item.Day, item.MealCode, item.Food, item.Quantity, item.Unit})
	}
	doc := map[string]any{"i": rows}
	encoded, _ := json.Marshal(doc)
	return string(encoded)
}

type compactComparableRow struct {
	Day      int
	MealCode string
	Food     string
	Quantity float64
	Unit     string
}

type rowComparisonMetrics struct {
	ComparedRows    int
	MatchedRows     int
	DayMatched      int
	MealMatched     int
	FoodMatched     int
	QuantityMatched int
	UnitMatched     int
}

func comparePlanRows(plan checker.Plan, expected []expectedSourceItem) []string {
	messages, _ := comparePlanRowsDetailed(plan, expected)
	return messages
}

func comparePlanRowsDetailed(plan checker.Plan, expected []expectedSourceItem) ([]string, rowComparisonMetrics) {
	actual := flattenPlanRows(plan)
	var messages []string
	var metrics rowComparisonMetrics
	if len(actual) != len(expected) {
		messages = append(messages, fmt.Sprintf("adapter_item_count_failed: got %d row(s), want %d", len(actual), len(expected)))
	}
	limit := len(expected)
	if len(actual) < limit {
		limit = len(actual)
	}
	metrics.ComparedRows = limit
	for i := 0; i < limit; i++ {
		got := actual[i]
		want := expected[i]
		rowMessages := []string{}
		if got.Day != want.Day {
			rowMessages = append(rowMessages, fmt.Sprintf("adapter_row_%d_day_failed: got %d, want %d", i+1, got.Day, want.Day))
		} else {
			metrics.DayMatched++
		}
		if got.MealCode != want.MealCode {
			rowMessages = append(rowMessages, fmt.Sprintf("adapter_row_%d_meal_failed: got %q, want %q", i+1, got.MealCode, want.MealCode))
		} else {
			metrics.MealMatched++
		}
		if got.Food != want.Food {
			rowMessages = append(rowMessages, fmt.Sprintf("adapter_row_%d_food_failed: got %q, want %q", i+1, got.Food, want.Food))
		} else {
			metrics.FoodMatched++
		}
		if math.Abs(got.Quantity-want.Quantity) > 0.0001 {
			rowMessages = append(rowMessages, fmt.Sprintf("adapter_row_%d_quantity_failed: got %.4f, want %.4f", i+1, got.Quantity, want.Quantity))
		} else {
			metrics.QuantityMatched++
		}
		if got.Unit != want.Unit {
			rowMessages = append(rowMessages, fmt.Sprintf("adapter_row_%d_unit_failed: got %q, want %q", i+1, got.Unit, want.Unit))
		} else {
			metrics.UnitMatched++
		}
		if len(rowMessages) == 0 {
			metrics.MatchedRows++
		} else {
			messages = append(messages, rowMessages...)
		}
	}
	return messages, metrics
}

func flattenPlanRows(plan checker.Plan) []compactComparableRow {
	var rows []compactComparableRow
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			code := mealCodeForName(meal.Name)
			for _, item := range meal.Items {
				quantity := 0.0
				if item.Quantity != nil {
					quantity = *item.Quantity
				}
				rows = append(rows, compactComparableRow{
					Day:      day.Day,
					MealCode: code,
					Food:     item.Food,
					Quantity: quantity,
					Unit:     item.Unit,
				})
			}
		}
	}
	return rows
}

func mealCodeForName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "breakfast":
		return "b"
	case "morning snack":
		return "m"
	case "lunch":
		return "l"
	case "afternoon snack":
		return "a"
	case "dinner":
		return "d"
	case "snack":
		return "s"
	case "evening snack":
		return "e"
	default:
		return ""
	}
}

func readSuccessCases(path string) ([]successCase, error) {
	var cases []successCase
	if err := readJSONL(path, func(line []byte) error {
		var c successCase
		if err := json.Unmarshal(line, &c); err != nil {
			return err
		}
		cases = append(cases, c)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("read success cases: %w", err)
	}
	return cases, nil
}

func readFailureCases(path string) ([]failureCase, error) {
	var cases []failureCase
	if err := readJSONL(path, func(line []byte) error {
		var c failureCase
		if err := json.Unmarshal(line, &c); err != nil {
			return err
		}
		cases = append(cases, c)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("read failure cases: %w", err)
	}
	return cases, nil
}

func readJSONL(path string, decode func([]byte) error) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if err := decode([]byte(text)); err != nil {
			return fmt.Errorf("%s line %d: %w", path, lineNumber, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return err
	}
	return nil
}

func resolvePath(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func recordTags(counts map[string]*tagAccumulator, tags []string, passed bool) {
	if len(tags) == 0 {
		tags = []string{"untagged"}
	}
	for _, tag := range tags {
		recordAccumulator(counts, tag, passed)
	}
}

func recordAccumulator(counts map[string]*tagAccumulator, key string, passed bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "unknown"
	}
	acc := counts[key]
	if acc == nil {
		acc = &tagAccumulator{}
		counts[key] = acc
	}
	acc.cases++
	if passed {
		acc.passed++
	}
}

func tagSummaries(counts map[string]*tagAccumulator) []tagSummary {
	tags := make([]string, 0, len(counts))
	for tag := range counts {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	result := make([]tagSummary, 0, len(tags))
	for _, tag := range tags {
		acc := counts[tag]
		result = append(result, tagSummary{
			Tag:    tag,
			Cases:  acc.cases,
			Passed: acc.passed,
			Rate:   ratio(acc.passed, acc.cases),
		})
	}
	return result
}

func gateSummaries(counts map[string]*tagAccumulator) []gateSummary {
	gates := make([]string, 0, len(counts))
	for gate := range counts {
		gates = append(gates, gate)
	}
	sort.Strings(gates)
	result := make([]gateSummary, 0, len(gates))
	for _, gate := range gates {
		acc := counts[gate]
		result = append(result, gateSummary{
			Gate:   gate,
			Cases:  acc.cases,
			Passed: acc.passed,
			Rate:   ratio(acc.passed, acc.cases),
		})
	}
	return result
}

func datasetSummaries(counts map[string]*tagAccumulator) []datasetSummary {
	datasets := make([]string, 0, len(counts))
	for dataset := range counts {
		datasets = append(datasets, dataset)
	}
	sort.Strings(datasets)
	result := make([]datasetSummary, 0, len(datasets))
	for _, dataset := range datasets {
		acc := counts[dataset]
		result = append(result, datasetSummary{
			SourceDataset: dataset,
			Cases:         acc.cases,
			Passed:        acc.passed,
			Rate:          ratio(acc.passed, acc.cases),
		})
	}
	return result
}

func rankedCounts(counts map[string]int) []rankedCount {
	values := make([]string, 0, len(counts))
	for value := range counts {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if counts[values[i]] == counts[values[j]] {
			return values[i] < values[j]
		}
		return counts[values[i]] > counts[values[j]]
	})
	result := make([]rankedCount, 0, len(values))
	for _, value := range values {
		result = append(result, rankedCount{Value: value, Count: counts[value]})
	}
	return result
}

func failureCategory(message string) string {
	if index := strings.Index(message, ":"); index > 0 {
		message = message[:index]
	}
	if index := strings.Index(message, "_failed"); index > 0 {
		return message[:index+len("_failed")]
	}
	return message
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func ratioFloat(numerator float64, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return numerator / float64(denominator)
}

func normalizeMode(mode string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "", modeDeterministic:
		return modeDeterministic, nil
	case "local-llama", modeLocalLlama:
		return modeLocalLlama, nil
	default:
		return "", fmt.Errorf("unsupported normalization eval mode %q", mode)
	}
}

func normalizeLocalModelRepeats(mode string, repeats int) (int, error) {
	if mode != modeLocalLlama {
		return 0, nil
	}
	if repeats == 0 {
		return 1, nil
	}
	if repeats < 0 {
		return 0, fmt.Errorf("local-model repeats must be a positive integer")
	}
	return repeats, nil
}

func envPositiveInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstPositiveDuration(values ...time.Duration) time.Duration {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
