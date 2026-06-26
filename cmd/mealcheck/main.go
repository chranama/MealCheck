package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/chranama/MealCheck/internal/artifacts"
	"github.com/chranama/MealCheck/internal/checker"
	"github.com/chranama/MealCheck/internal/commands/eval"
	"github.com/chranama/MealCheck/internal/commands/fixturecheck"
	"github.com/chranama/MealCheck/internal/commands/localsmoke"
	"github.com/chranama/MealCheck/internal/hosted"
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
	case "eval":
		return eval.Run(args[1:], stdout, stderr)
	case "fixture-check":
		return fixturecheck.Run(args[1:], stdout, stderr)
	case "invite":
		return runInviteCommand(args[1:], stdout, stderr)
	case "local-smoke":
		return localsmoke.Run(args[1:], stdout, stderr)
	case "local-llama":
		return runLocalLlamaCommand(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runLocalLlamaCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printLocalLlamaUsage(stderr)
		return 2
	}
	switch args[0] {
	case "normalize":
		return runLocalLlamaNormalizeCommand(args[1:], stdout, stderr)
	case "schema":
		return runLocalLlamaSchemaCommand(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown local-llama command %q\n\n", args[0])
		printLocalLlamaUsage(stderr)
		return 2
	}
}

func runLocalLlamaNormalizeCommand(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("local-llama normalize", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inputPath := flags.String("input", "", "compact local llama JSON path")
	outPath := flags.String("out", "", "canonical MealCheck plan JSON output path; stdout when empty")
	planID := flags.String("plan-id", "local-llama-normalized", "canonical MealCheck plan_id")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "local-llama normalize does not accept positional arguments")
		return 2
	}
	if *inputPath == "" {
		fmt.Fprintln(stderr, "local-llama normalize requires --input")
		return 2
	}
	input, err := os.ReadFile(*inputPath)
	if err != nil {
		fmt.Fprintf(stderr, "local-llama normalize failed: %v\n", err)
		return 2
	}
	plan, err := hosted.DecodeLocalLlamaCompactPlan(string(input), *planID)
	if err != nil {
		fmt.Fprintf(stderr, "local-llama normalize failed: %v\n", err)
		return 2
	}
	if *outPath == "" {
		if err := writeJSON(stdout, plan); err != nil {
			fmt.Fprintf(stderr, "local-llama normalize failed: %v\n", err)
			return 2
		}
		return 0
	}
	if err := writeJSONPath(*outPath, plan); err != nil {
		fmt.Fprintf(stderr, "local-llama normalize failed: %v\n", err)
		return 2
	}
	return 0
}

func runLocalLlamaSchemaCommand(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("local-llama schema", flag.ContinueOnError)
	flags.SetOutput(stderr)
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "local-llama schema does not accept positional arguments")
		return 2
	}
	if err := writeJSON(stdout, hosted.LocalLlamaCompactResponseSchema()); err != nil {
		fmt.Fprintf(stderr, "local-llama schema failed: %v\n", err)
		return 2
	}
	return 0
}

func runInviteCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printInviteUsage(stderr)
		return 2
	}
	switch args[0] {
	case "create":
		return runInviteCreateCommand(args[1:], stdout, stderr)
	case "list":
		return runInviteListCommand(args[1:], stdout, stderr)
	case "revoke":
		return runInviteRevokeCommand(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown invite command %q\n\n", args[0])
		printInviteUsage(stderr)
		return 2
	}
}

func runInviteCreateCommand(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("invite create", flag.ContinueOnError)
	flags.SetOutput(stderr)
	databaseURL := flags.String("database-url", os.Getenv("DATABASE_URL"), "Postgres database URL")
	label := flags.String("label", "", "reviewer label for the access code")
	expires := flags.String("expires", "", "optional expiry date as YYYY-MM-DD or RFC3339")
	expiresIn := flags.Duration("expires-in", 0, "optional expiry duration, such as 720h")
	maxRuns := flags.Int("max-runs", 0, "optional maximum run count")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "invite create does not accept positional arguments")
		return 2
	}
	if *label == "" {
		fmt.Fprintln(stderr, "invite create requires --label")
		return 2
	}
	if *expires != "" && *expiresIn != 0 {
		fmt.Fprintln(stderr, "invite create accepts only one of --expires or --expires-in")
		return 2
	}
	now := time.Now().UTC()
	expiry, err := parseInviteExpiry(*expires, *expiresIn, now)
	if err != nil {
		fmt.Fprintf(stderr, "invite create failed: %v\n", err)
		return 2
	}
	var max *int
	if *maxRuns < 0 {
		fmt.Fprintln(stderr, "invite create requires --max-runs to be non-negative")
		return 2
	}
	if *maxRuns > 0 {
		max = maxRuns
	}
	generated, err := hosted.GenerateInviteToken(*label, expiry, max, now)
	if err != nil {
		fmt.Fprintf(stderr, "invite create failed: %v\n", err)
		return 2
	}
	store, err := openInviteStore(*databaseURL)
	if err != nil {
		fmt.Fprintf(stderr, "invite create failed: %v\n", err)
		return 2
	}
	defer store.Close()
	if err := store.CreateInviteToken(context.Background(), generated.Invite); err != nil {
		fmt.Fprintf(stderr, "invite create failed: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "id: %s\n", generated.Invite.ID)
	fmt.Fprintf(stdout, "label: %s\n", generated.Invite.Label)
	if generated.Invite.ExpiresAt != nil {
		fmt.Fprintf(stdout, "expires_at: %s\n", generated.Invite.ExpiresAt.Format(time.RFC3339))
	}
	if generated.Invite.MaxRuns != nil {
		fmt.Fprintf(stdout, "max_runs: %d\n", *generated.Invite.MaxRuns)
	}
	fmt.Fprintf(stdout, "access_code: %s\n", generated.Token)
	fmt.Fprintln(stdout, "store this access code now; MealCheck stores only its hash.")
	return 0
}

func runInviteListCommand(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("invite list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	databaseURL := flags.String("database-url", os.Getenv("DATABASE_URL"), "Postgres database URL")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "invite list does not accept positional arguments")
		return 2
	}
	store, err := openInviteStore(*databaseURL)
	if err != nil {
		fmt.Fprintf(stderr, "invite list failed: %v\n", err)
		return 2
	}
	defer store.Close()
	invites, err := store.ListInviteTokens(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "invite list failed: %v\n", err)
		return 2
	}
	for _, invite := range invites {
		fmt.Fprintf(stdout, "%s\t%s\tused=%d", invite.ID, invite.Label, invite.UsedRuns)
		if invite.MaxRuns != nil {
			fmt.Fprintf(stdout, "/%d", *invite.MaxRuns)
		}
		if invite.ExpiresAt != nil {
			fmt.Fprintf(stdout, "\texpires=%s", invite.ExpiresAt.Format(time.RFC3339))
		}
		if invite.RevokedAt != nil {
			fmt.Fprintf(stdout, "\trevoked=%s", invite.RevokedAt.Format(time.RFC3339))
		}
		if invite.LastUsedAt != nil {
			fmt.Fprintf(stdout, "\tlast_used=%s", invite.LastUsedAt.Format(time.RFC3339))
		}
		fmt.Fprintln(stdout)
	}
	return 0
}

func runInviteRevokeCommand(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("invite revoke", flag.ContinueOnError)
	flags.SetOutput(stderr)
	databaseURL := flags.String("database-url", os.Getenv("DATABASE_URL"), "Postgres database URL")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "invite revoke requires exactly one access code id")
		return 2
	}
	store, err := openInviteStore(*databaseURL)
	if err != nil {
		fmt.Fprintf(stderr, "invite revoke failed: %v\n", err)
		return 2
	}
	defer store.Close()
	id := flags.Arg(0)
	if err := store.RevokeInviteToken(context.Background(), id, time.Now().UTC()); err != nil {
		fmt.Fprintf(stderr, "invite revoke failed: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "revoked: %s\n", id)
	return 0
}

func openInviteStore(databaseURL string) (hosted.Store, error) {
	return hosted.OpenPostgresStore(context.Background(), databaseURL)
}

func parseInviteExpiry(value string, duration time.Duration, now time.Time) (*time.Time, error) {
	if duration != 0 {
		if duration < 0 {
			return nil, fmt.Errorf("--expires-in must be positive")
		}
		expiresAt := now.Add(duration).UTC()
		return &expiresAt, nil
	}
	if value == "" {
		return nil, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		parsed = parsed.UTC()
		return &parsed, nil
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		parsed = parsed.UTC()
		return &parsed, nil
	}
	return nil, fmt.Errorf("invalid --expires %q; use YYYY-MM-DD or RFC3339", value)
}

func runBundleCommand(mode string, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(mode, flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root")
	casePath := flags.String("case", "", "case JSON path, relative to root")
	outDir := flags.String("out", filepath.Join("artifacts", "latest"), "artifact output directory")
	fnddsFallbackPath := flags.String("fndds-fallback", "", "optional FNDDS SQLite fallback database path")
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
		Root:              *root,
		CasePath:          *casePath,
		OutDir:            *outDir,
		Mode:              mode,
		FNDDSFallbackPath: *fnddsFallbackPath,
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
	fmt.Fprintln(w, "  mealcheck validate --case <case.json> [--out artifacts/latest] [--fndds-fallback fndds.sqlite] [--strict]")
	fmt.Fprintln(w, "  mealcheck compare --case <case.json> [--out artifacts/latest] [--fndds-fallback fndds.sqlite] [--strict]")
	fmt.Fprintln(w, "  mealcheck decision [--strict] <decision.json>")
	fmt.Fprintln(w, "  mealcheck eval [-dataset dataset.json] [-out results.json] [-fndds-fallback fndds.sqlite] [-skip-expected]")
	fmt.Fprintln(w, "  mealcheck fixture-check [-root repo-root]")
	fmt.Fprintln(w, "  mealcheck local-llama normalize --input compact.json [--out normalized-plan.json]")
	fmt.Fprintln(w, "  mealcheck local-llama schema")
	fmt.Fprintln(w, "  mealcheck local-smoke [-root repo-root] [-work-dir dir] [-keep-work-dir]")
	fmt.Fprintln(w, "  mealcheck invite create --label <label> [--expires YYYY-MM-DD] [--max-runs N]")
	fmt.Fprintln(w, "  mealcheck invite list")
	fmt.Fprintln(w, "  mealcheck invite revoke <access-code-id>")
}

func printLocalLlamaUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  mealcheck local-llama normalize --input compact.json [--out normalized-plan.json] [--plan-id ID]")
	fmt.Fprintln(w, "  mealcheck local-llama schema")
}

func printInviteUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  mealcheck invite create --label <label> [--database-url URL] [--expires YYYY-MM-DD] [--expires-in 720h] [--max-runs N]")
	fmt.Fprintln(w, "  mealcheck invite list [--database-url URL]")
	fmt.Fprintln(w, "  mealcheck invite revoke [--database-url URL] <access-code-id>")
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeJSONPath(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeJSON(f, value)
}
