package artifacts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/chranama/MealCheck/internal/workflow/recommend"
)

func TestWriteBundleIncludesRecommendationArtifactAndSchema(t *testing.T) {
	root := repoRoot(t)
	outDir := filepath.Join(t.TempDir(), "bundle")

	result, err := WriteBundle(BundleOptions{
		Root:     root,
		CasePath: "examples/seeded-one-day-peanut-allergy/case.json",
		OutDir:   outDir,
		Mode:     "test",
	})
	if err != nil {
		t.Fatalf("WriteBundle: %v", err)
	}
	if result.Decision.ArtifactPaths["recommendation"] != "recommendation.json" {
		t.Fatalf("decision recommendation artifact path = %q, want recommendation.json", result.Decision.ArtifactPaths["recommendation"])
	}

	var recommendation recommend.Document
	decodeJSON(t, readFile(t, filepath.Join(outDir, "recommendation.json")), &recommendation)
	if recommendation.SchemaVersion != "0.1" || recommendation.SourcePlanID == "" {
		t.Fatalf("recommendation artifact = %+v, want schema version and source plan", recommendation)
	}

	if _, err := os.Stat(filepath.Join(outDir, "schemas", "recommendation.schema.json")); err != nil {
		t.Fatalf("recommendation schema was not copied: %v", err)
	}
	var manifest manifestDocument
	decodeJSON(t, readFile(t, filepath.Join(outDir, "manifest.json")), &manifest)
	if !containsArtifact(manifest.Artifacts, "recommendation.json") || !containsArtifact(manifest.Artifacts, "schemas/recommendation.schema.json") {
		t.Fatalf("manifest artifacts = %v, want recommendation artifact and schema", manifest.Artifacts)
	}
}

func containsArtifact(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func decodeJSON(t *testing.T, data []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, data)
	}
}
