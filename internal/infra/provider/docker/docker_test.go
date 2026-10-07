package docker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chranama/MealCheck/internal/infra/provider"
	"github.com/chranama/MealCheck/internal/infra/spec"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

func fixture(t *testing.T) (*Docker, provider.Resource) {
	t.Helper()
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	os.Mkdir(filepath.Join(root, "lab"), 0700)
	for _, name := range []string{"database-url", "postgres-password"} {
		os.WriteFile(filepath.Join(root, "lab", name), []byte("private-value"), 0600)
	}
	model := filepath.Join(root, "model.gguf")
	os.WriteFile(model, []byte("model"), 0600)
	return &Docker{secretRoot: root}, provider.Resource{Role: "api", Kind: "container", Name: "mc-test-lab-api", InstallationID: "owner", DeploymentID: "lab", Fingerprint: "spec", Workload: spec.Workload{APIImage: "api@sha256:" + strings.Repeat("a", 64), ModelPath: model, SecretProfile: "lab", APIHostPort: 18080}}
}
func TestProfileConfigurationAndSecretSafety(t *testing.T) {
	d, r := fixture(t)
	opts, e := d.config(r)
	if e != nil {
		t.Fatal(e)
	}
	if opts.Config.User != "10001:10001" || opts.HostConfig.RestartPolicy.Name != "no" {
		t.Fatal("runtime contract")
	}
	p := network.MustParsePort("8080/tcp")
	if !opts.HostConfig.PortBindings[p][0].HostIP.IsLoopback() {
		t.Fatal("public binding")
	}
	for _, v := range opts.Config.Env {
		if strings.Contains(v, "private-value") {
			t.Fatal("secret entered config")
		}
	}
	os.Chmod(filepath.Join(d.secretRoot, "lab", "database-url"), 0644)
	_, e = d.config(r)
	var pe *provider.Error
	if !errors.As(classify(e), &pe) || pe.Transient {
		t.Fatal("unsafe secret must block")
	}
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret"), []byte("private"), 0600)
	os.Remove(filepath.Join(d.secretRoot, "lab", "database-url"))
	os.Symlink(filepath.Join(outside, "secret"), filepath.Join(d.secretRoot, "lab", "database-url"))
	if _, e = d.config(r); e == nil {
		t.Fatal("secret escape")
	}
}
func TestActualConfigurationMustMatch(t *testing.T) {
	d, r := fixture(t)
	want, e := d.config(r)
	if e != nil {
		t.Fatal(e)
	}
	a := container.InspectResponse{Config: want.Config, HostConfig: want.HostConfig, NetworkSettings: &container.NetworkSettings{Networks: want.NetworkingConfig.EndpointsConfig}}
	for _, m := range want.HostConfig.Mounts {
		name := ""
		if m.Type == mount.TypeVolume {
			name = m.Source
		}
		a.Mounts = append(a.Mounts, container.MountPoint{Type: m.Type, Name: name, Source: m.Source, Destination: m.Target, RW: !m.ReadOnly})
	}
	if !compatibleConfig(a, want) {
		t.Fatal("compatible rejected")
	}
	a.HostConfig.Privileged = true
	if compatibleConfig(a, want) {
		t.Fatal("privileged accepted")
	}
	a.HostConfig.Privileged = false
	a.Mounts[0].RW = true
	if compatibleConfig(a, want) {
		t.Fatal("writable secret accepted")
	}
	a.Mounts[0].RW = false
	a.NetworkSettings.Networks = map[string]*network.EndpointSettings{"other": {}}
	if compatibleConfig(a, want) {
		t.Fatal("wrong network accepted")
	}
}
func TestUnavailableEngineSanitized(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "mc-docker-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	d, e := New("unix://"+filepath.Join(root, "missing.sock"), root)
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	_, e = d.Observe(context.Background(), nil)
	var pe *provider.Error
	if !errors.As(e, &pe) || !pe.Transient || pe.Reason != "EngineUnavailable" {
		t.Fatalf("error=%v", e)
	}
	if _, e = New("tcp://remote:2375", root); e == nil {
		t.Fatal("remote accepted")
	}
}

func TestResolvedRuntimeDefinitionDrivesConfiguration(t *testing.T) {
	d, r := fixture(t)
	resolved := spec.ResolvedSystem{Architecture: "arm64", ApplicationRole: "api", Resources: []spec.ResolvedResource{{Role: "api", Kind: "container", DependsOn: []string{"network"}, Container: &spec.ResolvedContainer{Image: r.Workload.APIImage, User: "10001:10001", Args: []string{"serve", "--port", "9000"}, Environment: []string{"EXAMPLE=engineer-defined"}, NanoCPUs: 500000000, MemoryBytes: 256 << 20, NetworkRole: "network", Aliases: []string{"backend"}, Mounts: []spec.Mount{{Kind: "secret", Source: "database-url", Target: "/run/secrets/database-url", ReadOnly: true}, {Kind: "volume", Source: "artifact-volume", Target: "/data"}}, Ports: []spec.Port{{ContainerPort: 9000, HostPort: 18090, HostIP: "127.0.0.1"}}, Probe: spec.Probe{Kind: "exec", Command: []string{"check", "ready"}}}}}}
	encoded, _ := json.Marshal(resolved)
	r.Workload.ResolvedJSON = string(encoded)
	opts, err := d.config(r)
	if err != nil {
		t.Fatal(err)
	}
	if opts.HostConfig.NanoCPUs != 500000000 || opts.HostConfig.Memory != 256<<20 || opts.Config.Cmd[1] != "--port" || opts.Config.Env[0] != "EXAMPLE=engineer-defined" {
		t.Fatal("system config ignored")
	}
	p := network.MustParsePort("9000/tcp")
	if opts.HostConfig.PortBindings[p][0].HostPort != "18090" || opts.Config.Healthcheck.Test[1] != "check" || opts.NetworkingConfig.EndpointsConfig[sibling(r, "network")].Aliases[0] != "backend" {
		t.Fatal("system topology/probe ignored")
	}
	for _, v := range opts.Config.Env {
		if strings.Contains(v, "private-value") {
			t.Fatal("secret leaked")
		}
	}
	// Copied ownership labels cannot conceal engineer configuration drift.
	actual := container.InspectResponse{Config: opts.Config, HostConfig: opts.HostConfig, NetworkSettings: &container.NetworkSettings{Networks: opts.NetworkingConfig.EndpointsConfig}}
	for _, m := range opts.HostConfig.Mounts {
		name := ""
		if m.Type == mount.TypeVolume {
			name = m.Source
		}
		actual.Mounts = append(actual.Mounts, container.MountPoint{Type: m.Type, Name: name, Source: m.Source, Destination: m.Target, RW: !m.ReadOnly})
	}
	if !compatibleConfig(actual, opts) {
		t.Fatal("resolved runtime rejected")
	}
	changed := *actual.Config
	actual.Config = &changed
	actual.Config.Cmd = []string{"unexpected"}
	if compatibleConfig(actual, opts) {
		t.Fatal("command drift accepted")
	}
}

func TestRequiredEnvironmentCannotBeOverridden(t *testing.T) {
	d, r := fixture(t)
	opts, err := d.config(r)
	if err != nil {
		t.Fatal(err)
	}
	actual := container.InspectResponse{Config: opts.Config, HostConfig: opts.HostConfig, NetworkSettings: &container.NetworkSettings{Networks: opts.NetworkingConfig.EndpointsConfig}}
	for _, m := range opts.HostConfig.Mounts {
		name := ""
		if m.Type == mount.TypeVolume {
			name = m.Source
		}
		actual.Mounts = append(actual.Mounts, container.MountPoint{Type: m.Type, Name: name, Source: m.Source, Destination: m.Target, RW: !m.ReadOnly})
	}
	cfg := *actual.Config
	actual.Config = &cfg
	actual.Config.Env = append(append([]string{}, cfg.Env...), "MEALCHECK_HOSTED_MODE=changed")
	if compatibleConfig(actual, opts) {
		t.Fatal("duplicate key overrides trusted configuration")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestEngineCapacityAndActualImageArchitecture(t *testing.T) {
	architecture := "arm64"
	cli, err := client.New(client.WithAPIVersion("1.56"), client.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"Architecture":"aarch64","NCPU":4,"MemTotal":4294967296}`
		if strings.Contains(r.URL.Path, "/images/") {
			body = `{"Architecture":"` + architecture + `","Os":"linux"}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	d := &Docker{cli: cli}
	info, err := d.EngineInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Architecture != "arm64" || info.CPUs != 4 || info.MemoryBytes != 4<<30 {
		t.Fatalf("capacity=%+v", info)
	}
	b, _ := json.Marshal(spec.ResolvedSystem{Architecture: "arm64", Resources: []spec.ResolvedResource{{Role: "api", Kind: "container", Container: &spec.ResolvedContainer{Image: "api@sha256:" + strings.Repeat("a", 64)}}}})
	w := spec.Workload{ResolvedJSON: string(b)}
	if err = d.ValidateImages(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	architecture = "amd64"
	var pe *provider.Error
	if err = d.ValidateImages(context.Background(), w); !errors.As(err, &pe) || pe.Reason != "ImageArchitectureMismatch" {
		t.Fatalf("mismatch=%v", err)
	}
}

func TestApplicationProbeUsesResolvedPublishedPort(t *testing.T) {
	c := &spec.ResolvedContainer{Probe: spec.Probe{Kind: "application", Port: 8080}, Ports: []spec.Port{{ContainerPort: 8080, HostPort: 18081, HostIP: "127.0.0.1"}}}
	port, err := applicationHostPort(c)
	if err != nil || port != 18081 {
		t.Fatalf("resolved probe port=%d err=%v", port, err)
	}
	c.Probe.Port = 8081
	if _, err = applicationHostPort(c); err == nil {
		t.Fatal("unpublished port accepted")
	}
	c.Probe.Port = 8080
	c.Ports = append(c.Ports, c.Ports[0])
	if _, err = applicationHostPort(c); err == nil {
		t.Fatal("ambiguous published binding accepted")
	}
	c.Ports = c.Ports[:1]
	c.Ports[0].HostIP = "0.0.0.0"
	if _, err = applicationHostPort(c); err == nil {
		t.Fatal("public probe binding accepted")
	}
}
