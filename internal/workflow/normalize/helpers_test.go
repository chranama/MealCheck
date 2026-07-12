package normalize

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

type fakeProvider struct {
	responses []string
	calls     int
	messages  [][]Message
	configs   []ProviderConfig
}

func (p *fakeProvider) Complete(_ context.Context, config ProviderConfig, request CompletionRequest) (string, error) {
	if config.APIKey != "" {
		for _, message := range request.Messages {
			if strings.Contains(message.Content, config.APIKey) {
				return "", fmt.Errorf("provider key leaked into prompt")
			}
		}
	}
	p.configs = append(p.configs, config)
	p.messages = append(p.messages, append([]Message(nil), request.Messages...))
	if p.calls >= len(p.responses) {
		return "", fmt.Errorf("fake provider response exhausted")
	}
	response := p.responses[p.calls]
	p.calls++
	return response, nil
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
		t.Fatalf("decode JSON: %v\n%s", err, string(data))
	}
}

func seededCase(t *testing.T, root string) checker.Case {
	t.Helper()
	var c checker.Case
	decodeJSON(t, readFile(t, filepath.Join(root, "examples/seeded-one-day-peanut-allergy/case.json")), &c)
	return c
}
