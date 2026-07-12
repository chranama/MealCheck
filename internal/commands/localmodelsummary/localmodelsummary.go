package localmodelsummary

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	planextract "github.com/chranama/MealCheck/internal/llm/planextract"
)

type Summary struct {
	SchemaVersion      string       `json:"schema_version"`
	GeneratedAt        string       `json:"generated_at"`
	ArtifactRoot       string       `json:"artifact_root"`
	RunCount           int          `json:"run_count"`
	ChunkCount         int          `json:"chunk_count"`
	FailedRunCount     int          `json:"failed_run_count"`
	TimeoutCount       int          `json:"timeout_count"`
	DecodeFailureCount int          `json:"decode_failure_count"`
	RepairCount        int          `json:"repair_count"`
	Runs               []RunSummary `json:"runs"`
}

type RunSummary struct {
	RunID              string                                       `json:"run_id"`
	ArtifactDir        string                                       `json:"artifact_dir"`
	Status             string                                       `json:"status"`
	Model              string                                       `json:"model,omitempty"`
	ProviderType       string                                       `json:"provider_type,omitempty"`
	MealCheckVersion   string                                       `json:"mealcheck_version,omitempty"`
	PlanID             string                                       `json:"plan_id,omitempty"`
	SourceItemCount    int                                          `json:"source_item_count"`
	ChunkCount         int                                          `json:"chunk_count"`
	FailureStage       string                                       `json:"failure_stage,omitempty"`
	Error              string                                       `json:"error,omitempty"`
	Timeout            bool                                         `json:"timeout"`
	RepairCount        int                                          `json:"repair_count"`
	DecodeFailureCount int                                          `json:"decode_failure_count"`
	StageTimings       planextract.LocalModelExtractionStageTimings `json:"stage_timings"`
	Chunks             []ChunkSummary                               `json:"chunks"`
}

type ChunkSummary struct {
	Index             int                                     `json:"index"`
	Day               int                                     `json:"day"`
	MealCode          string                                  `json:"meal_code"`
	MealLabel         string                                  `json:"meal_label,omitempty"`
	SourceItemCount   int                                     `json:"source_item_count"`
	DecodedRowCount   int                                     `json:"decoded_row_count"`
	RepairCount       int                                     `json:"repair_count"`
	DecodeFailure     bool                                    `json:"decode_failure"`
	FailureStage      string                                  `json:"failure_stage,omitempty"`
	StageTimings      planextract.LocalModelChunkStageTimings `json:"stage_timings"`
	ProviderRequestMS int64                                   `json:"provider_request_ms"`
	TotalMS           int64                                   `json:"total_ms"`
}

type normalizationFailureArtifact struct {
	RunID                string                                    `json:"run_id"`
	Provider             planextract.RedactedProviderConfig        `json:"provider"`
	FinalError           string                                    `json:"final_error,omitempty"`
	LocalModelExtraction *planextract.LocalModelExtractionArtifact `json:"local_model_extraction,omitempty"`
}

type manifestArtifact struct {
	MealCheck map[string]string `json:"mealcheck"`
}

func Run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("local-model-summary", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("artifact-root", filepath.Join(".mealcheck-data", "artifacts"), "artifact root or single local-model artifact path")
	format := flags.String("format", "text", "output format: text or json")
	out := flags.String("out", "", "optional output path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "local-model-summary does not accept positional arguments")
		return 2
	}
	summary, err := Build(*root)
	if err != nil {
		fmt.Fprintf(stderr, "local-model-summary failed: %v\n", err)
		return 2
	}
	var writer io.Writer = stdout
	var file *os.File
	if *out != "" {
		file, err = os.Create(*out)
		if err != nil {
			fmt.Fprintf(stderr, "local-model-summary failed: %v\n", err)
			return 2
		}
		defer file.Close()
		writer = file
	}
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(summary); err != nil {
			fmt.Fprintf(stderr, "local-model-summary failed: %v\n", err)
			return 2
		}
	case "text":
		writeText(writer, summary)
	default:
		fmt.Fprintf(stderr, "local-model-summary failed: unsupported format %q\n", *format)
		return 2
	}
	return 0
}

func Build(root string) (Summary, error) {
	artifactRoot, err := filepath.Abs(root)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{
		SchemaVersion: "0.1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		ArtifactRoot:  artifactRoot,
		Runs:          []RunSummary{},
	}
	paths, err := artifactEvidencePaths(artifactRoot)
	if err != nil {
		return Summary{}, err
	}
	for _, path := range paths {
		run, ok, err := readRunSummary(path)
		if err != nil {
			return Summary{}, err
		}
		if !ok {
			continue
		}
		summary.Runs = append(summary.Runs, run)
		summary.ChunkCount += run.ChunkCount
		summary.DecodeFailureCount += run.DecodeFailureCount
		summary.RepairCount += run.RepairCount
		if run.Status == "failed" {
			summary.FailedRunCount++
		}
		if run.Timeout {
			summary.TimeoutCount++
		}
	}
	sort.Slice(summary.Runs, func(i, j int) bool {
		return summary.Runs[i].RunID < summary.Runs[j].RunID
	})
	summary.RunCount = len(summary.Runs)
	return summary, nil
}

func artifactEvidencePaths(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if isEvidenceFile(root) {
			return []string{root}, nil
		}
		return nil, fmt.Errorf("%s is not a local-model evidence artifact", root)
	}
	seenRunDirs := map[string]string{}
	var paths []string
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !isEvidenceFile(path) {
			return nil
		}
		runDir := artifactRunDir(path)
		if existing, ok := seenRunDirs[runDir]; ok {
			if strings.HasSuffix(existing, filepath.Join("optional", "local-model-chunks.json")) {
				return nil
			}
			if strings.HasSuffix(path, filepath.Join("optional", "local-model-chunks.json")) {
				seenRunDirs[runDir] = path
			}
			return nil
		}
		seenRunDirs[runDir] = path
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, path := range seenRunDirs {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func isEvidenceFile(path string) bool {
	cleaned := filepath.Clean(path)
	return strings.HasSuffix(cleaned, filepath.Join("optional", "local-model-chunks.json")) ||
		strings.HasSuffix(cleaned, filepath.Join("debug", "normalization-failure.json")) ||
		filepath.Base(cleaned) == "local-model-chunks.json"
}

func readRunSummary(path string) (RunSummary, bool, error) {
	if strings.HasSuffix(filepath.Clean(path), filepath.Join("debug", "normalization-failure.json")) {
		var failure normalizationFailureArtifact
		if err := readJSON(path, &failure); err != nil {
			return RunSummary{}, false, err
		}
		if failure.LocalModelExtraction == nil {
			return RunSummary{}, false, nil
		}
		run := extractionRunSummary(path, *failure.LocalModelExtraction)
		run.Status = "failed"
		run.Error = firstNonEmpty(failure.FinalError, run.Error)
		if failure.RunID != "" {
			run.RunID = failure.RunID
		}
		if run.ProviderType == "" {
			run.ProviderType = failure.Provider.Type
		}
		if run.Model == "" {
			run.Model = failure.Provider.Model
		}
		run.Timeout = isTimeout(run.Error)
		return run, true, nil
	}
	var extraction planextract.LocalModelExtractionArtifact
	if err := readJSON(path, &extraction); err != nil {
		return RunSummary{}, false, err
	}
	run := extractionRunSummary(path, extraction)
	return run, true, nil
}

func extractionRunSummary(path string, extraction planextract.LocalModelExtractionArtifact) RunSummary {
	runDir := artifactRunDir(path)
	run := RunSummary{
		RunID:            runIDFrom(runDir, extraction.PlanID),
		ArtifactDir:      runDir,
		Status:           finalStatus(runDir, extraction),
		Model:            extraction.Provider.Model,
		ProviderType:     extraction.Provider.Type,
		PlanID:           extraction.PlanID,
		SourceItemCount:  extraction.SourceItemCount,
		ChunkCount:       extraction.ChunkCount,
		FailureStage:     extraction.FailureStage,
		Error:            extraction.Error,
		Timeout:          isTimeout(extraction.Error),
		StageTimings:     extraction.StageTimings,
		MealCheckVersion: manifestVersion(runDir),
	}
	for _, chunk := range extraction.Chunks {
		chunkSummary := ChunkSummary{
			Index:             chunk.Index,
			Day:               chunk.Day,
			MealCode:          chunk.MealCode,
			MealLabel:         chunk.MealLabel,
			SourceItemCount:   len(chunk.SourceItemIDs),
			DecodedRowCount:   len(chunk.DecodedRows),
			RepairCount:       chunk.Reconciliation.RepairCount,
			DecodeFailure:     chunk.FailureStage == "decode",
			FailureStage:      chunk.FailureStage,
			StageTimings:      chunk.StageTimings,
			ProviderRequestMS: chunk.StageTimings.ProviderRequestMS,
			TotalMS:           chunk.StageTimings.TotalMS,
		}
		if chunkSummary.DecodeFailure {
			run.DecodeFailureCount++
		}
		run.RepairCount += chunkSummary.RepairCount
		run.Chunks = append(run.Chunks, chunkSummary)
	}
	if run.ChunkCount == 0 {
		run.ChunkCount = len(run.Chunks)
	}
	if run.FailureStage == "decode" && run.DecodeFailureCount == 0 {
		run.DecodeFailureCount = 1
	}
	return run
}

func artifactRunDir(path string) string {
	cleaned := filepath.Clean(path)
	switch filepath.Base(filepath.Dir(cleaned)) {
	case "optional", "debug":
		return filepath.Dir(filepath.Dir(cleaned))
	default:
		return filepath.Dir(cleaned)
	}
}

func runIDFrom(runDir, planID string) string {
	if id := filepath.Base(runDir); id != "." && id != string(filepath.Separator) {
		return id
	}
	return strings.TrimPrefix(planID, "local-model-")
}

func finalStatus(runDir string, extraction planextract.LocalModelExtractionArtifact) string {
	if fileExists(filepath.Join(runDir, "decision.json")) {
		return "completed"
	}
	if extraction.FailureStage != "" || strings.TrimSpace(extraction.Error) != "" || fileExists(filepath.Join(runDir, "debug", "normalization-failure.json")) {
		return "failed"
	}
	return "unknown"
}

func manifestVersion(runDir string) string {
	var manifest manifestArtifact
	if err := readJSON(filepath.Join(runDir, "manifest.json"), &manifest); err != nil {
		return ""
	}
	return manifest.MealCheck["version"]
}

func writeText(w io.Writer, summary Summary) {
	fmt.Fprintf(w, "local model runs: %d\n", summary.RunCount)
	fmt.Fprintf(w, "chunks: %d  repairs: %d  decode_failures: %d  timeouts: %d\n", summary.ChunkCount, summary.RepairCount, summary.DecodeFailureCount, summary.TimeoutCount)
	for _, run := range summary.Runs {
		fmt.Fprintf(w, "\n%s status=%s model=%s sources=%d chunks=%d total_ms=%d repairs=%d decode_failures=%d timeout=%t",
			run.RunID, emptyAsDash(run.Status), emptyAsDash(run.Model), run.SourceItemCount, run.ChunkCount, run.StageTimings.TotalMS, run.RepairCount, run.DecodeFailureCount, run.Timeout)
		if run.FailureStage != "" {
			fmt.Fprintf(w, " failure_stage=%s", run.FailureStage)
		}
		fmt.Fprintln(w)
		for _, chunk := range run.Chunks {
			fmt.Fprintf(w, "  chunk=%d meal=%s sources=%d rows=%d provider_ms=%d total_ms=%d repairs=%d decode_failure=%t",
				chunk.Index, emptyAsDash(chunk.MealCode), chunk.SourceItemCount, chunk.DecodedRowCount, chunk.ProviderRequestMS, chunk.TotalMS, chunk.RepairCount, chunk.DecodeFailure)
			if chunk.FailureStage != "" {
				fmt.Fprintf(w, " failure_stage=%s", chunk.FailureStage)
			}
			fmt.Fprintln(w)
		}
	}
}

func readJSON(path string, out any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(out)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isTimeout(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "timed out") || strings.Contains(lower, "deadline exceeded")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func emptyAsDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
