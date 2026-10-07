package control

import (
	"bytes"
	"encoding/json"
	"github.com/chranama/MealCheck/internal/infra/spec"
	"github.com/chranama/MealCheck/internal/infra/state"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandlerLifecycleAndRejections(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model")
	os.WriteFile(model, []byte("synthetic"), 0600)
	s, e := state.Open(filepath.Join(dir, "state"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	h := Handler(s, spec.Policy{ModelRoots: []string{dir}, Registries: []string{"lab"}})
	call := func(q Request) Response {
		t.Helper()
		b, _ := json.Marshal(q)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/v1", bytes.NewReader(b)))
		var out Response
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	d := spec.Document{APIVersion: spec.Version, DeploymentID: "lab", DesiredState: "Running", Spec: spec.Workload{Profile: "cpu-local-model-v1", APIImage: "lab/api@sha256:" + strings.Repeat("a", 64), PostgresImage: "lab/pg@sha256:" + strings.Repeat("b", 64), ModelImage: "lab/model@sha256:" + strings.Repeat("c", 64), ModelPath: model, SecretProfile: "lab", APIHostPort: 18080, DataPolicy: "Retain"}}
	b, _ := json.Marshal(d)
	for _, command := range []string{"apply", "get", "stop", "start", "events", "delete", "delete"} {
		q := Request{Version: spec.Version, Command: command}
		if command == "apply" {
			q.Document = b
		}
		if out := call(q); out.Error != "" {
			t.Fatal(command, out.Error)
		}
	}
	if out := call(Request{Version: spec.Version, Command: "start"}); out.Error == "" {
		t.Fatal("resurrected deleted state")
	}
	for _, q := range []Request{{Version: "unknown", Command: "get"}, {Version: spec.Version, Command: "shell"}, {Version: spec.Version, Command: "apply", Document: json.RawMessage(`{"secret":"sensitive"}`)}} {
		if out := call(q); out.Error == "" {
			t.Fatal("invalid request accepted")
		}
	}
}
