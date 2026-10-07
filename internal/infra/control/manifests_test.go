package control

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chranama/MealCheck/internal/infra/spec"
	"github.com/chranama/MealCheck/internal/infra/state"
)

func TestManifestPlanAcceptanceAndFrozenCatalog(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(model, []byte("test model"), 0600); err != nil {
		t.Fatal(err)
	}
	system := spec.System{APIVersion: spec.SystemVersion, Name: "mealcheck", Version: "v1", Architecture: "arm64", ApplicationRole: "api", Parameters: map[string]spec.Parameter{
		"modelPath": {Type: "string"}, "secretProfile": {Type: "string", Default: "lab"},
	}}
	for _, role := range []string{"network", "database-volume", "artifact-volume"} {
		kind := "volume"
		if role == "network" {
			kind = "network"
		}
		system.Resources = append(system.Resources, spec.ResolvedResource{Role: role, Kind: kind})
	}
	for _, role := range []string{"postgres", "model", "api"} {
		c := &spec.ResolvedContainer{Image: "lab/" + role + "@sha256:" + strings.Repeat("a", 64), NanoCPUs: 1000000000, MemoryBytes: 1 << 28, NetworkRole: "network", Aliases: []string{role}, Probe: spec.Probe{Kind: "exec", Command: []string{"true"}}}
		deps := []string{"network"}
		switch role {
		case "postgres":
			deps = append(deps, "database-volume")
			c.Mounts = []spec.Mount{{Kind: "volume", Source: "database-volume", Target: "/data"}, {Kind: "secret", Source: "postgres-password", Target: "/run/secrets/postgres-password", ReadOnly: true}}
		case "model":
			c.Mounts = []spec.Mount{{Kind: "model", Source: "${modelPath}", Target: "/model", ReadOnly: true}}
		case "api":
			deps = append(deps, "postgres", "model", "artifact-volume")
			c.Ports = []spec.Port{{ContainerPort: 8080, HostPort: 18080, HostIP: "127.0.0.1"}}
			c.Mounts = []spec.Mount{{Kind: "volume", Source: "artifact-volume", Target: "/artifacts"}, {Kind: "secret", Source: "database-url", Target: "/run/secrets/database-url", ReadOnly: true}}
			c.Probe = spec.Probe{Kind: "application", Port: 8080, Path: "/api/status", Components: []string{"meal_check_submission"}}
		}
		system.Resources = append(system.Resources, spec.ResolvedResource{Role: role, Kind: "container", DependsOn: deps, Container: c})
	}
	catalog := filepath.Join(dir, "catalog")
	if err := os.Mkdir(catalog, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(catalog, "mealcheck--v1.json")
	b, _ := json.Marshal(system)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	reads, notifies := 0, 0
	h := HandlerWithManifests(s, spec.Policy{ModelRoots: []string{dir}, Registries: []string{"lab"}}, ManifestOptions{
		Catalog: &spec.Catalog{Root: catalog}, EngineInfo: func(context.Context) (spec.EngineInfo, error) {
			reads++
			return spec.EngineInfo{Architecture: "arm64", CPUs: 4, MemoryBytes: 4 << 30}, nil
		},
		ValidateImages: func(context.Context, spec.Workload) error { return nil },
	}, func() { notifies++ })
	d := spec.Deployment{APIVersion: spec.DeploymentVersion, DeploymentID: "lab", DesiredState: "Running", System: spec.SystemReference{Name: system.Name, Version: system.Version, Digest: system.Digest()}, Parameters: map[string]any{"modelPath": model}}
	call := func(command string, input any) Response {
		t.Helper()
		b, _ := json.Marshal(input)
		q, _ := json.Marshal(Request{Version: spec.Version, Command: command, Document: b})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1", bytes.NewReader(q)))
		var out Response
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if out := call("plan", d); out.Error != "" {
		t.Fatal(out.Error)
	}
	if _, err := s.Get(); err != sql.ErrNoRows {
		t.Fatal("plan changed durable deployment", err)
	}
	events, _ := s.Events()
	if len(events) != 0 || notifies != 0 {
		t.Fatal("plan emitted events or woke reconciler")
	}
	if out := call("apply", d); out.Error != "" {
		t.Fatal(out.Error)
	}
	accepted, _ := s.Get()
	if accepted.Generation != 1 || accepted.Document.DeploymentJSON == "" || accepted.Document.Spec.ResolvedJSON == "" {
		t.Fatal("missing accepted snapshot", accepted)
	}
	other := d
	other.DeploymentID = "another"
	if out := call("plan", other); out.Error == "" {
		t.Fatal("plan accepted a second deployment")
	}
	unchanged, _ := s.Get()
	if unchanged.Generation != accepted.Generation {
		t.Fatal("rejected plan changed generation")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	readsBefore := reads
	if out := call("apply", d); out.Error != "" {
		t.Fatal("catalog deletion changed accepted apply", out.Error)
	}
	same, _ := s.Get()
	if same.Generation != accepted.Generation || reads != readsBefore {
		t.Fatal("accepted apply was not idempotent/offline")
	}
	d.DesiredState = "Stopped"
	if out := call("apply", d); out.Error != "" {
		t.Fatal("lifecycle required deleted catalog", out.Error)
	}
	stopped, _ := s.Get()
	if stopped.Generation != 2 || stopped.Document.Spec != accepted.Document.Spec {
		t.Fatal("snapshot changed on lifecycle transition")
	}
	d.DesiredState = "Impossible"
	if out := call("plan", d); out.Error == "" {
		t.Fatal("invalid lifecycle plan accepted")
	}
	d.DesiredState = "Running"
	d.Parameters["secretProfile"] = "different"
	if out := call("apply", d); out.Error == "" {
		t.Fatal("changed deployment accepted without catalog")
	}
	injected := accepted.Document
	if out := call("apply", injected); out.Error == "" {
		t.Fatal("client injected internal snapshot")
	}
	// Reopening uses only accepted state, with no catalog access or migration.
	installation := s.InstallationID
	s.Close()
	s, err = state.Open(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	reopened, _ := s.Get()
	if s.InstallationID != installation || reopened.Document.Spec != accepted.Document.Spec || reopened.Generation != 2 {
		t.Fatal("restart changed snapshot or installation")
	}
}
