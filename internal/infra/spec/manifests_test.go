package spec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func manifestFixture(t *testing.T) (System, Deployment, Policy) {
	t.Helper()
	root := t.TempDir()
	model := filepath.Join(root, "model.gguf")
	if e := os.WriteFile(model, []byte("model"), 0600); e != nil {
		t.Fatal(e)
	}
	pin := "lab/image@sha256:" + strings.Repeat("a", 64)
	min, max := float64(1024), float64(65535)
	s := System{APIVersion: SystemVersion, Name: "mealcheck", Version: "v1", Architecture: "arm64", ApplicationRole: "api", Parameters: map[string]Parameter{"modelPath": {Type: "string"}, "secretProfile": {Type: "string", Default: "lab"}, "apiPort": {Type: "integer", Default: float64(18080), Minimum: &min, Maximum: &max}}}
	s.Resources = []ResolvedResource{{Role: "network", Kind: "network"}, {Role: "database-volume", Kind: "volume"}, {Role: "artifact-volume", Kind: "volume"}}
	for _, role := range []string{"postgres", "model", "api"} {
		c := &ResolvedContainer{Image: pin, NanoCPUs: 1000000000, MemoryBytes: 1024, NetworkRole: "network", Probe: Probe{Kind: "exec", Command: []string{"true"}}}
		deps := []string{"network"}
		if role == "api" {
			c.Ports = []Port{{ContainerPort: 8080, HostIP: "127.0.0.1", HostPortParameter: "apiPort"}}
		}
		if role == "model" {
			c.Mounts = []Mount{{Kind: "model", Source: "${modelPath}", Target: "/model.gguf", ReadOnly: true}}
		}
		s.Resources = append(s.Resources, ResolvedResource{Role: role, Kind: "container", DependsOn: deps, Container: c})
	}
	d := Deployment{APIVersion: DeploymentVersion, DeploymentID: "lab", DesiredState: "Running", System: SystemReference{Name: s.Name, Version: s.Version, Digest: s.Digest()}, Parameters: map[string]any{"modelPath": model}}
	return s, d, Policy{ModelRoots: []string{root}, Registries: []string{"lab"}}
}
func TestManifestResolutionDeterministicAndBounded(t *testing.T) {
	s, d, p := manifestFixture(t)
	cap := Capacity{Architecture: "arm64", CPUs: 1, MemoryBytes: 4096}
	a, e := Resolve(d, s, p, cap)
	if e != nil {
		t.Fatal(e)
	}
	b, e := Resolve(d, s, p, cap)
	if e != nil || a != b {
		t.Fatalf("not deterministic: %v", e)
	}
	if a.Spec.APIHostPort != 18080 || a.DeploymentJSON == "" || a.Spec.ResolvedJSON == "" {
		t.Fatal("missing defaults or durable documents")
	}
	d.Parameters["apiPort"] = float64(1)
	if _, e = Resolve(d, s, p, cap); e == nil {
		t.Fatal("accepted out of bounds port")
	}
	delete(d.Parameters, "apiPort")
	d.Parameters["undeclared"] = "x"
	if _, e = Resolve(d, s, p, cap); e == nil {
		t.Fatal("accepted unknown parameter")
	}
	delete(d.Parameters, "undeclared")
	if _, e = Resolve(d, s, p, Capacity{Architecture: "amd64", CPUs: 1, MemoryBytes: 4096}); e == nil {
		t.Fatal("accepted wrong architecture")
	}
	if _, e = Resolve(d, s, p, Capacity{Architecture: "arm64", CPUs: 1, MemoryBytes: 1024}); e == nil {
		t.Fatal("accepted insufficient memory")
	}
}
func TestManifestGraphAndTrust(t *testing.T) {
	s, d, p := manifestFixture(t)
	root := t.TempDir()
	path := filepath.Join(root, "mealcheck--v1.json")
	b, _ := json.Marshal(s)
	os.WriteFile(path, b, 0600)
	if _, e := (Catalog{Root: root}).Load(d.System); e != nil {
		t.Fatal(e)
	}
	os.Chmod(path, 0666)
	if _, e := (Catalog{Root: root}).Load(d.System); e == nil {
		t.Fatal("accepted writable catalog")
	}
	os.Chmod(path, 0600)
	ref := d.System
	ref.Digest = "sha256:" + strings.Repeat("b", 64)
	if _, e := (Catalog{Root: root}).Load(ref); e == nil {
		t.Fatal("accepted digest mismatch")
	}
	s.Resources[0].DependsOn = []string{"api"}
	d.System.Digest = s.Digest()
	if _, e := Resolve(d, s, p, Capacity{Architecture: "arm64", CPUs: 1, MemoryBytes: 4096}); e == nil {
		t.Fatal("accepted cycle")
	}
	s.Resources[0].DependsOn = []string{"missing"}
	d.System.Digest = s.Digest()
	if _, e := Resolve(d, s, p, Capacity{Architecture: "arm64", CPUs: 1, MemoryBytes: 4096}); e == nil {
		t.Fatal("accepted missing dependency")
	}
}
func TestStrictManifestDecoding(t *testing.T) {
	for _, b := range []string{`{"unknown":true}`, `{} {}`, `{"apiVersion":"a","apiVersion":"b"}`} {
		if _, e := DecodeDeployment([]byte(b)); e == nil {
			t.Fatal("deployment accepted malformed JSON")
		}
		if _, e := DecodeSystem([]byte(b)); e == nil {
			t.Fatal("system accepted malformed JSON")
		}
	}
}

func TestApplicationProbeRequiresPublishedPort(t *testing.T) {
	s, d, p := manifestFixture(t)
	for i := range s.Resources {
		if s.Resources[i].Role == "api" {
			s.Resources[i].Container.Probe = Probe{Kind: "application", Port: 9090, Path: "/api/status", Components: []string{"meal_check_submission"}}
		}
	}
	d.System.Digest = s.Digest()
	if _, err := Resolve(d, s, p, Capacity{Architecture: "arm64", CPUs: 4, MemoryBytes: 4096}); err == nil {
		t.Fatal("accepted an application probe with no published port")
	}
}

func TestManifestStructuralRequirements(t *testing.T) {
	s, d, p := manifestFixture(t)
	b, _ := json.Marshal(d)
	var fields map[string]any
	json.Unmarshal(b, &fields)
	for _, key := range []string{"parameters", "system", "deploymentID"} {
		copy := map[string]any{}
		for k, v := range fields {
			copy[k] = v
		}
		delete(copy, key)
		bad, _ := json.Marshal(copy)
		if _, err := DecodeDeployment(bad); err == nil {
			t.Fatalf("accepted omitted %s", key)
		}
	}
	fields["parameters"] = nil
	b, _ = json.Marshal(fields)
	if _, err := DecodeDeployment(b); err == nil {
		t.Fatal("accepted null parameters")
	}
	// Authoring requirements must be checked before Go defaults erase omission.
	b, _ = json.Marshal(s)
	json.Unmarshal(b, &fields)
	resources := fields["resources"].([]any)
	model := resources[4].(map[string]any)["container"].(map[string]any)
	delete(model["mounts"].([]any)[0].(map[string]any), "readOnly")
	b, _ = json.Marshal(fields)
	if _, err := DecodeSystem(b); err == nil {
		t.Fatal("accepted omitted mount readOnly")
	}
	s.Resources[3].DependsOn = []string{"network", "network"}
	d.System.Digest = s.Digest()
	if _, err := Resolve(d, s, p, Capacity{Architecture: "arm64", CPUs: 4, MemoryBytes: 4096}); err == nil {
		t.Fatal("accepted duplicate dependencies")
	}
	s.Resources[3].DependsOn = []string{"network"}
	s.Architecture = "unsupported"
	d.System.Digest = s.Digest()
	if _, err := Resolve(d, s, p, Capacity{Architecture: "unsupported", CPUs: 4, MemoryBytes: 4096}); err == nil {
		t.Fatal("accepted unsupported architecture")
	}
}

func TestCommittedManifestExamplesResolve(t *testing.T) {
	root := filepath.Join("..", "..", "..", "deploy", "controller")
	modelRoot := t.TempDir()
	model := filepath.Join(modelRoot, "model.gguf")
	if err := os.WriteFile(model, []byte("synthetic model"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, architecture := range []string{"arm64", "amd64"} {
		t.Run(architecture, func(t *testing.T) {
			read := func(path string) []byte {
				t.Helper()
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				return b
			}
			s, err := DecodeSystem(read(filepath.Join(root, "systems", "mealcheck-cpu-"+architecture+"--v1.json")))
			if err != nil {
				t.Fatal(err)
			}
			d, err := DecodeDeployment(read(filepath.Join(root, "examples", "deployment-"+architecture+".template.json")))
			if err != nil {
				t.Fatal(err)
			}
			if d.System.Digest != s.Digest() {
				t.Fatal("committed deployment references an outdated system digest")
			}
			d.Parameters["modelPath"] = model
			capacity := Capacity{Architecture: architecture, CPUs: 16, MemoryBytes: 8214056960}
			if architecture == "amd64" {
				capacity.CPUs = 4
				capacity.MemoryBytes = 4102107136
			}
			resolved, err := Resolve(d, s, Policy{ModelRoots: []string{modelRoot}, Registries: []string{"localhost:15000/mealcheck", "docker.io/library", "ghcr.io/ggml-org/llama.cpp"}}, capacity)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := resolved.Spec.Resolved()
			if err != nil || snapshot.Architecture != architecture || len(snapshot.Resources) != 6 {
				t.Fatal("incorrect example resolution", err)
			}
		})
	}
}

func TestResolvePolicyBoundaries(t *testing.T) {
	cases := []struct {
		name   string
		change func(*System, *Deployment, *Policy, *Capacity)
	}{
		{"unknown reference", func(s *System, d *Deployment, p *Policy, c *Capacity) {
			s.Resources[4].Container.Args = []string{"${missing}"}
			d.System.Digest = s.Digest()
		}},
		{"malformed reference", func(s *System, d *Deployment, p *Policy, c *Capacity) {
			s.Resources[4].Container.Args = []string{"${missing"}
			d.System.Digest = s.Digest()
		}},
		{"null parameter", func(s *System, d *Deployment, p *Policy, c *Capacity) { d.Parameters["modelPath"] = nil }},
		{"invalid bounds", func(s *System, d *Deployment, p *Policy, c *Capacity) {
			a, b := float64(10), float64(1)
			param := s.Parameters["apiPort"]
			param.Minimum = &a
			param.Maximum = &b
			s.Parameters["apiPort"] = param
			d.System.Digest = s.Digest()
		}},
		{"wrong allowed type", func(s *System, d *Deployment, p *Policy, c *Capacity) {
			param := s.Parameters["apiPort"]
			param.Allowed = []any{"invalid"}
			s.Parameters["apiPort"] = param
			d.System.Digest = s.Digest()
		}},
		{"secret path escape", func(s *System, d *Deployment, p *Policy, c *Capacity) { d.Parameters["secretProfile"] = "../other" }},
		{"model policy revoked", func(s *System, d *Deployment, p *Policy, c *Capacity) { p.ModelRoots = []string{t.TempDir()} }},
		{"registry policy revoked", func(s *System, d *Deployment, p *Policy, c *Capacity) { p.Registries = []string{"other"} }},
		{"public API port", func(s *System, d *Deployment, p *Policy, c *Capacity) {
			s.Resources[5].Container.Ports[0].HostIP = "0.0.0.0"
			d.System.Digest = s.Digest()
		}},
		{"CPU oversubscription", func(s *System, d *Deployment, p *Policy, c *Capacity) {
			s.Resources[4].Container.NanoCPUs = 2000000000
			d.System.Digest = s.Digest()
		}},
		{"unknown engine capacity", func(s *System, d *Deployment, p *Policy, c *Capacity) { c.CPUs = 0 }},
		{"changed system pin", func(s *System, d *Deployment, p *Policy, c *Capacity) {
			s.Resources[4].Container.Args = []string{"changed"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, d, p := manifestFixture(t)
			cap := Capacity{Architecture: "arm64", CPUs: 1, MemoryBytes: 4096}
			tc.change(&s, &d, &p, &cap)
			if _, e := Resolve(d, s, p, cap); e == nil {
				t.Fatal("unsafe manifest accepted")
			}
		})
	}
}
func TestCatalogRejectsSymlinkAndDigestCanonical(t *testing.T) {
	s, d, _ := manifestFixture(t)
	b, _ := json.MarshalIndent(s, "", "  ")
	decoded, e := DecodeSystem(b)
	if e != nil {
		t.Fatal(e)
	}
	if decoded.Digest() != s.Digest() {
		t.Fatal("formatting changed canonical digest")
	}
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "system.json")
	if e = os.WriteFile(outside, b, 0600); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(root, s.Name+"--"+s.Version+".json")
	if e = os.Symlink(outside, path); e != nil {
		t.Fatal(e)
	}
	if _, e = (Catalog{Root: root}).Load(d.System); e == nil {
		t.Fatal("accepted symlink system")
	}
	linkedRoot := filepath.Join(t.TempDir(), "catalog")
	if e = os.Symlink(root, linkedRoot); e != nil {
		t.Fatal(e)
	}
	if _, e = (Catalog{Root: linkedRoot}).Load(d.System); e == nil {
		t.Fatal("accepted symlink catalog")
	}
}
