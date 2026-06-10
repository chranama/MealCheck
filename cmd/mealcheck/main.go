package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/chranama/MealCheck/internal/artifacts"
	"github.com/chranama/MealCheck/internal/checker"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "validate":
		return runBundleCommand("validate", args[1:], stdout, stderr)
	case "compare":
		return runBundleCommand("compare", args[1:], stdout, stderr)
	case "decision":
		return runDecisionCommand(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runBundleCommand(mode string, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(mode, flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root")
	casePath := flags.String("case", "", "case JSON path, relative to root")
	outDir := flags.String("out", filepath.Join("artifacts", "latest"), "artifact output directory")
	strict := flags.Bool("strict", false, "treat warn decisions as failing")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "%s does not accept positional arguments\n", mode)
		return 2
	}
	if *casePath == "" {
		fmt.Fprintf(stderr, "%s requires --case\n", mode)
		return 2
	}

	result, err := artifacts.WriteBundle(artifacts.BundleOptions{
		Root:     *root,
		CasePath: *casePath,
		OutDir:   *outDir,
		Mode:     mode,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%s failed: %v\n", mode, err)
		return 2
	}

	printDecision(stdout, result.Decision)
	fmt.Fprintf(stdout, "artifacts: %s\n", result.OutDir)
	fmt.Fprintf(stdout, "report: %s\n", filepath.Join(result.OutDir, "report.md"))
	return exitCodeForDecision(result.Decision.Decision, *strict)
}

func runDecisionCommand(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("decision", flag.ContinueOnError)
	flags.SetOutput(stderr)
	strict := flags.Bool("strict", false, "treat warn decisions as failing")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "decision requires exactly one decision.json path")
		return 2
	}

	var decision checker.DecisionDocument
	f, err := os.Open(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "decision failed: %v\n", err)
		return 2
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decision); err != nil {
		fmt.Fprintf(stderr, "decision failed: %v\n", err)
		return 2
	}

	printDecision(stdout, decision)
	return exitCodeForDecision(decision.Decision, *strict)
}

func printDecision(w io.Writer, decision checker.DecisionDocument) {
	fmt.Fprintf(w, "case: %s\n", decision.CaseID)
	fmt.Fprintf(w, "decision: %s\n", decision.Decision)
	fmt.Fprintf(w, "risk: %s\n", decision.RiskLevel)
	fmt.Fprintf(w, "summary: %s\n", decision.Summary)
	if len(decision.FailedChecks) > 0 {
		fmt.Fprintln(w, "checks requiring attention:")
		for _, check := range decision.FailedChecks {
			fmt.Fprintf(w, "- %s\n", check)
		}
	}
}

func exitCodeForDecision(decision string, strict bool) int {
	switch decision {
	case "pass":
		return 0
	case "warn":
		if strict {
			return 1
		}
		return 0
	case "block":
		return 1
	default:
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  mealcheck validate --case <case.json> [--out artifacts/latest] [--strict]")
	fmt.Fprintln(w, "  mealcheck compare --case <case.json> [--out artifacts/latest] [--strict]")
	fmt.Fprintln(w, "  mealcheck decision [--strict] <decision.json>")
}
