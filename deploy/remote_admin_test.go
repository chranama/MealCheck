package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteAdministrationChecker(t *testing.T) {
	checker := "macos/check-remote-service.sh"
	assertExecutable(t, checker)
	assertBashSyntax(t, checker)

	temp := t.TempDir()
	calls := filepath.Join(temp, "calls")
	fakeSSH := filepath.Join(temp, "ssh")
	script := `#!/bin/bash
printf '%s\n' "$*" >> "$MEALCHECK_TEST_CALLS"
if [ "${1:-}" = "-G" ]; then
  printf 'user chranama-server\n'
  printf 'hostname example.invalid\n'
  printf 'hostkeyalias trusted-origin\n'
  exit 0
fi
exit "${MEALCHECK_TEST_SSH_EXIT:-0}"
`
	if err := os.WriteFile(fakeSSH, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}

	run := func(extraEnv []string, args ...string) (string, int) {
		t.Helper()
		cmd := exec.Command(checker, args...)
		cmd.Env = append(os.Environ(),
			"MEALCHECK_REMOTE_SSH_BIN="+fakeSSH,
			"MEALCHECK_TEST_CALLS="+calls,
		)
		cmd.Env = append(cmd.Env, extraEnv...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return string(output), 0
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return string(output), exitErr.ExitCode()
		}
		t.Fatalf("run checker: %v", err)
		return "", -1
	}

	output, status := run(nil, "mealcheck-server-cf")
	if status != 0 || !strings.Contains(output, "Configuration-only check passed") {
		t.Fatalf("secondary config check failed: status=%d output=%s", status, output)
	}

	if err := os.WriteFile(calls, nil, 0o600); err != nil {
		t.Fatalf("reset calls: %v", err)
	}
	_, status = run([]string{"MEALCHECK_TEST_SSH_EXIT=23"}, "--connect", "mealcheck-server")
	if status != 23 {
		t.Fatalf("failed primary status = %d, want 23", status)
	}
	attempted, err := os.ReadFile(calls)
	if err != nil {
		t.Fatalf("read calls: %v", err)
	}
	if strings.Contains(string(attempted), "mealcheck-server-cf") {
		t.Fatalf("primary failure silently attempted secondary alias: %s", attempted)
	}

	if _, status = run(nil, "unsupported-host"); status == 0 {
		t.Fatal("unsupported alias was accepted")
	}
}
