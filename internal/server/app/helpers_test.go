package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/chranama/MealCheck/internal/core"
	llm "github.com/chranama/MealCheck/internal/llm/external"
	localmodel "github.com/chranama/MealCheck/internal/llm/local"
	"github.com/chranama/MealCheck/internal/server/access"
	"github.com/chranama/MealCheck/internal/server/store"
	"github.com/chranama/MealCheck/internal/workflow/checker"
	"github.com/chranama/MealCheck/internal/workflow/mealplan"
	"github.com/chranama/MealCheck/internal/workflow/normalize"
)

func doRequest(t *testing.T, server *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, req)
	return recorder
}

func decodeJSON(t *testing.T, data []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, string(data))
	}
}

func marshalJSON(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return string(b)
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func seededCase(t *testing.T, root string) checker.Case {
	t.Helper()
	var c checker.Case
	decodeJSON(t, readFile(t, filepath.Join(root, "examples/seeded-3-day-peanut-allergy/case.json")), &c)
	return c
}

func hasNormalizationEvent(events []NormalizationEvent, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func assertFileTreeDoesNotContain(t *testing.T, root, secret string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(b, []byte(secret)) {
			return fmt.Errorf("%s contains secret", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type fakeProvider struct {
	responses []string
	calls     int
	messages  [][]ProviderMessage
	configs   []ProviderConfig
}

type errorProvider struct {
	err error
}

type roundTripFunc func(*http.Request) (*http.Response, error)

const InviteTokenPrefix = access.InviteTokenPrefix

const (
	QualificationStatusNotMealPlan                = core.QualificationStatusNotMealPlan
	QualificationStatusMealPlanTooVague           = core.QualificationStatusMealPlanTooVague
	QualificationStatusRecipeOrMenuNeedsDecompose = core.QualificationStatusRecipeOrMenuNeedsDecompose
	QualificationStatusOutsideHostedContract      = core.QualificationStatusOutsideHostedContract
	QualificationStatusEligibleForVerification    = core.QualificationStatusEligibleForVerification
)

type decodePlanResult = mealplan.DecodePlanResult
type MemoryStore = store.MemoryStore
type ProviderMessage = llm.ProviderMessage
type LocalModelExtractionArtifact = localmodel.LocalModelExtractionArtifact

type normalizationFailureArtifact struct {
	SchemaVersion        string                                   `json:"schema_version"`
	RunID                string                                   `json:"run_id"`
	CreatedAt            string                                   `json:"created_at"`
	Provider             RedactedProviderConfig                   `json:"provider"`
	NormalizationEvents  []NormalizationEvent                     `json:"normalization_events"`
	InitialOutput        string                                   `json:"initial_output,omitempty"`
	InitialError         string                                   `json:"initial_error,omitempty"`
	RepairOutput         string                                   `json:"repair_output,omitempty"`
	RepairError          string                                   `json:"repair_error,omitempty"`
	FinalError           string                                   `json:"final_error,omitempty"`
	LocalModelExtraction *localmodel.LocalModelExtractionArtifact `json:"local_model_extraction,omitempty"`
}

func NewMemoryStore() *store.MemoryStore {
	return store.NewMemoryStore()
}

func GenerateInviteToken(label string, expiresAt *time.Time, maxRuns *int, now time.Time) (access.GeneratedInviteToken, error) {
	return access.GenerateInviteToken(label, expiresAt, maxRuns, now)
}

func generationMessages(input PendingRunInput) ([]ProviderMessage, error) {
	return normalize.GenerationMessages(input)
}

func qualificationMessages(request MealPlanQualificationRequest) []ProviderMessage {
	return normalize.QualificationMessages(request)
}

func decodePlanText(text string) (checker.Plan, error) {
	return mealplan.DecodePlanText(text)
}

func decodePlanTextDetailed(text string) (decodePlanResult, error) {
	return mealplan.DecodePlanTextDetailed(text)
}

func countMealPlanItems(plan checker.Plan) int {
	total := len(plan.ShoppingList)
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			total += len(meal.Items)
		}
	}
	return total
}

func testConfig(t *testing.T, root string) Config {
	t.Helper()
	dataDir := t.TempDir()
	return Config{
		Root:                     root,
		DataDir:                  dataDir,
		ArtifactDir:              filepath.Join(dataDir, "artifacts"),
		Addr:                     "127.0.0.1:0",
		StoreKind:                "memory",
		AccessMode:               AccessModePublicBYOK,
		PublicOpenAICompatible:   true,
		PublicRequestLimit:       60,
		PublicRequestWindow:      time.Minute,
		PublicDailyRunLimit:      20,
		MaxCandidateTextChars:    20_000,
		MaxGenerationPromptChars: 4_000,
		LocalModelMaxSourceItems: 20,
		QueueSize:                3,
		MaxCasesPerRun:           20,
		MaxUploadBytes:           1_000_000,
		RunTimeout:               10 * time.Minute,
		PendingInputTTL:          30 * time.Minute,
		Retention:                7 * 24 * time.Hour,
		WorkerPoll:               time.Millisecond,
		CleanupInterval:          time.Hour,
		DemoIndexPath:            filepath.Join(root, "examples", "seeded-3-day-peanut-allergy", "artifacts", "demo-runs", "index.json"),
		DemoArtifactRoot:         filepath.Join(root, "examples", "seeded-3-day-peanut-allergy", "artifacts"),
	}
}

func localModelTestSettings(settings checker.Settings) checker.Settings {
	settings.VerificationConstraints.Days = 1
	settings.VerificationConstraints.MealsPerDay = 0
	settings.VerificationConstraints.RequiresPrepSafetyNotes = false
	return settings
}

func qualificationFromErrorResponse(t *testing.T, response ErrorResponse) MealPlanQualificationResult {
	t.Helper()
	qualificationValue, ok := response.Error.Details["qualification"]
	if !ok {
		t.Fatalf("error details missing qualification: %+v", response.Error.Details)
	}
	qualificationBytes, err := json.Marshal(qualificationValue)
	if err != nil {
		t.Fatalf("marshal qualification detail: %v", err)
	}
	var qualification MealPlanQualificationResult
	decodeJSON(t, qualificationBytes, &qualification)
	return qualification
}

func compactLocalMealPlanJSONResponses() []string {
	return []string{
		`{"i":[[1,"cooked oatmeal",1,"cup"],[2,"blueberries",1,"cup"],[3,"plain Greek yogurt",1,"cup"]]}`,
		`{"i":[[4,"chicken breast",4,"oz"],[5,"brown rice",1,"cup"],[6,"broccoli",1,"cup"]]}`,
		`{"i":[[7,"salmon",4,"oz"],[8,"sweet potato",1,"cup"],[9,"spinach",1,"cup"]]}`,
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func testMealPlanJSON(unresolved bool) string {
	item := `{
              "food": "cooked oatmeal",
              "quantity": 1,
              "unit": "cup",
              "preparation": "plain"
            }`
	if unresolved {
		item = `{
              "food": "seasoning blend",
              "quantity_text": "some",
              "resolution_status": "unresolved",
              "unresolved_reason": "vague_quantity"
            }`
	}
	return `{
  "schema_version": "0.1",
  "plan_id": "qualification-test-plan",
  "description": "One day test plan.",
  "days": [
    {
      "day": 1,
      "meals": [
        {
          "name": "breakfast",
          "items": [
            ` + item + `
          ]
        }
      ]
    }
  ],
  "shopping_list": [],
  "prep_notes": ["Refrigerate leftovers promptly."]
}`
}
