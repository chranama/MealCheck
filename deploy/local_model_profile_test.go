package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalModelDeploymentProfileScaffold(t *testing.T) {
	for _, path := range []string{
		"local-model/README.md",
		"local-model/compose.postgres.dev.yml",
		"local-model/mealcheck-server.env.example",
		"local-model/mealcheck-llama.env.example",
		"../scripts/run-local-model-deployment-profile.sh",
	} {
		if info, err := os.Stat(path); err != nil {
			t.Fatalf("missing local-model deployment profile file %s: %v", path, err)
		} else if info.Size() == 0 {
			t.Fatalf("local-model deployment profile file %s is empty", path)
		}
	}

	for _, path := range []string{
		"../scripts/run-local-model-deployment-profile.sh",
		"../scripts/test-deployed-local-model-live.sh",
		"macos/mealcheck-llama-server.sh",
	} {
		assertExecutable(t, path)
		assertBashSyntax(t, path)
	}
}

func TestLocalModelDeploymentProfileContract(t *testing.T) {
	serverEnv := readProfileEnv(t, "local-model/mealcheck-server.env.example")
	llamaEnv := readProfileEnv(t, "local-model/mealcheck-llama.env.example")
	compose := readText(t, "local-model/compose.postgres.dev.yml")
	runner := readText(t, "../scripts/run-local-model-deployment-profile.sh")
	gitignore := readText(t, "../.gitignore")

	assertEnv(t, serverEnv, "DATABASE_URL", "postgres://mealcheck:mealcheck@127.0.0.1:5432/mealcheck?sslmode=disable")
	assertEnv(t, serverEnv, "MEALCHECK_ADDR", "127.0.0.1:8080")
	assertEnv(t, serverEnv, "MEALCHECK_STORE", "postgres")
	assertEnv(t, serverEnv, "MEALCHECK_DATA_DIR", ".mealcheck-local-model")
	assertEnv(t, serverEnv, "MEALCHECK_ARTIFACT_DIR", ".mealcheck-local-model/artifacts")
	assertEnv(t, serverEnv, "MEALCHECK_HOSTED_MODE", "local_model")
	assertEnv(t, serverEnv, "MEALCHECK_LOCAL_MODEL_ENABLED", "true")
	assertEnv(t, serverEnv, "MEALCHECK_LOCAL_MODEL_BASE_URL", "http://127.0.0.1:11435/v1")
	assertEnv(t, serverEnv, "MEALCHECK_LOCAL_MODEL_NAME", "auto")

	assertEnv(t, llamaEnv, "LLAMA_SERVER_BIN", "llama-server")
	assertEnv(t, llamaEnv, "LLAMA_MODEL_PATH", ".mealcheck-local-model/models/Qwen3-0.6B-Q4_K_M.gguf")
	assertEnv(t, llamaEnv, "LLAMA_LOG_DIR", ".mealcheck-local-model/logs")
	assertEnv(t, llamaEnv, "LLAMA_HOST", "127.0.0.1")
	assertEnv(t, llamaEnv, "LLAMA_PORT", "11435")

	assertContains(t, compose, "image: postgres:17-alpine")
	assertContains(t, compose, "name: mealcheck-local-model-dev-postgres")
	assertContains(t, compose, `POSTGRES_DB: mealcheck`)
	assertContains(t, compose, `"127.0.0.1:5433:5432"`)
	assertContains(t, compose, `pg_isready -U mealcheck -d mealcheck`)

	assertContains(t, runner, `MEALCHECK_PROFILE_ENV_FILE`)
	assertContains(t, runner, `MEALCHECK_LOCAL_MODEL_NAME:-}" != "auto"`)
	assertContains(t, runner, `"$base_url/models"`)
	assertContains(t, runner, `jq -r '.data[0].id // empty'`)
	assertContains(t, runner, `pg_isready -d "$DATABASE_URL"`)
	assertContains(t, runner, `go build -o "$ROOT_DIR/bin/mealcheck-server" ./cmd/mealcheck-server`)
	assertContains(t, runner, `exec "$SERVER_BIN"`)
	assertContains(t, runner, `-store "$MEALCHECK_STORE"`)

	assertContains(t, gitignore, ".mealcheck-local-model/")
}

func TestLocalModelDeploymentProfileDocsMatchRunner(t *testing.T) {
	readme := readText(t, "local-model/README.md")
	runbook := readText(t, "../docs/runbook.md")
	deployReadme := readText(t, "README.md")
	priorities := readText(t, "../docs/current-priorities.md")

	for _, doc := range []struct {
		name string
		text string
	}{
		{name: "deploy/local-model/README.md", text: readme},
		{name: "docs/runbook.md", text: runbook},
		{name: "deploy/README.md", text: deployReadme},
	} {
		assertContainsNamed(t, doc.name, doc.text, "pg_isready -d 'postgres://mealcheck:mealcheck@127.0.0.1:5432/mealcheck?sslmode=disable'")
		assertContainsNamed(t, doc.name, doc.text, "scripts/run-local-model-deployment-profile.sh")
		assertContainsNamed(t, doc.name, doc.text, "MEALCHECK_DEPLOYED_API_URL=http://127.0.0.1:8080")
		assertContainsNamed(t, doc.name, doc.text, "scripts/test-deployed-local-model-live.sh")
	}

	assertContains(t, readme, "deploy/local-model/compose.postgres.dev.yml up -d")
	assertContains(t, readme, "127.0.0.1:11435/v1")
	assertContains(t, readme, "Do not expose")
	assertContainsNormalized(t, runbook, "without Cloudflare")
	assertContains(t, runbook, "deploy/local-model/compose.postgres.dev.yml up -d")
	assertContains(t, deployReadme, "optional disposable Postgres fallback")
	assertContains(t, priorities, "A local-model deployment profile now lives under `deploy/local-model/`")
	assertContains(t, priorities, "model id can")
	assertContains(t, priorities, "be resolved from `/v1/models`")
}

func assertExecutable(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat executable %s: %v", path, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("%s is not executable: mode %s", path, info.Mode())
	}
}

func assertBashSyntax(t *testing.T, path string) {
	t.Helper()
	cmd := exec.Command("bash", "-n", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash syntax check failed for %s: %v\n%s", path, err, string(output))
	}
}

func readProfileEnv(t *testing.T, path string) map[string]string {
	t.Helper()
	lines := strings.Split(readText(t, path), "\n")
	values := make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("%s contains non-assignment line %q", path, line)
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `'"`)
	}
	return values
}

func assertEnv(t *testing.T, values map[string]string, key, want string) {
	t.Helper()
	if got := values[key]; got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func assertContains(t *testing.T, text, want string) {
	t.Helper()
	assertContainsNamed(t, "text", text, want)
}

func assertContainsNamed(t *testing.T, name, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("%s does not contain %q", name, want)
	}
}

func assertContainsNormalized(t *testing.T, text, want string) {
	t.Helper()
	normalized := strings.Join(strings.Fields(text), " ")
	if !strings.Contains(normalized, want) {
		t.Fatalf("text does not contain %q after whitespace normalization", want)
	}
}
