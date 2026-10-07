package docker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chranama/MealCheck/internal/infra/provider"
	"github.com/chranama/MealCheck/internal/infra/spec"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
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
