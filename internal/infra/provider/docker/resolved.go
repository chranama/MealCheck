package docker

import (
	"context"
	"github.com/chranama/MealCheck/internal/infra/provider"
	"github.com/chranama/MealCheck/internal/infra/spec"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func (d *Docker) EngineInfo(ctx context.Context) (spec.EngineInfo, error) {
	x, err := d.cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return spec.EngineInfo{}, classify(err)
	}
	architecture := x.Info.Architecture
	switch architecture {
	case "x86_64":
		architecture = "amd64"
	case "aarch64":
		architecture = "arm64"
	}
	return spec.EngineInfo{Architecture: architecture, CPUs: x.Info.NCPU, MemoryBytes: x.Info.MemTotal}, nil
}

// ValidateImages verifies preloaded immutable image references against the target
// engine. It never pulls images, accesses registry credentials, or mutates Docker.
func (d *Docker) ValidateImages(ctx context.Context, w spec.Workload) error {
	resolved, err := w.Resolved()
	if err != nil {
		return &provider.Error{Reason: "InvalidResolvedConfiguration"}
	}
	if resolved == nil {
		return nil
	}
	for _, resource := range resolved.Resources {
		if resource.Container == nil {
			continue
		}
		inspected, err := d.cli.ImageInspect(ctx, resource.Container.Image)
		if err != nil {
			return classify(err)
		}
		if inspected.Os != "linux" || inspected.Architecture != resolved.Architecture {
			return &provider.Error{Reason: "ImageArchitectureMismatch"}
		}
	}
	return nil
}

func (d *Docker) resolvedConfig(r provider.Resource, system *spec.ResolvedSystem) (client.ContainerCreateOptions, error) {
	var resolved *spec.ResolvedContainer
	for _, resource := range system.Resources {
		if resource.Role == r.Role {
			resolved = resource.Container
		}
	}
	if resolved == nil {
		return client.ContainerCreateOptions{}, &provider.Error{Reason: "UnsupportedResource"}
	}
	cfg := &container.Config{Labels: labels(r), Image: resolved.Image, User: resolved.User, Cmd: resolved.Args, Env: resolved.Environment}
	host := &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "no"}, Resources: container.Resources{Memory: resolved.MemoryBytes, NanoCPUs: resolved.NanoCPUs}}
	for _, m := range resolved.Mounts {
		source := m.Source
		kind := mount.TypeBind
		switch m.Kind {
		case "volume":
			kind = mount.TypeVolume
			source = sibling(r, m.Source)
		case "secret":
			var err error
			source, err = d.secret(r.Workload.SecretProfile, m.Source)
			if err != nil {
				return client.ContainerCreateOptions{}, err
			}
		case "model":
			canonical, err := filepath.EvalSymlinks(source)
			if err != nil || canonical != source {
				return client.ContainerCreateOptions{}, &provider.Error{Reason: "ModelPathMustBeCanonical"}
			}
			info, err := os.Stat(source)
			if err != nil || !info.Mode().IsRegular() {
				return client.ContainerCreateOptions{}, &provider.Error{Reason: "ModelFileUnavailable"}
			}
		default:
			return client.ContainerCreateOptions{}, &provider.Error{Reason: "UnsupportedMount"}
		}
		host.Mounts = append(host.Mounts, mount.Mount{Type: kind, Source: source, Target: m.Target, ReadOnly: m.ReadOnly})
	}
	for _, p := range resolved.Ports {
		port := network.MustParsePort(strconv.Itoa(p.ContainerPort) + "/tcp")
		ip, err := netip.ParseAddr(p.HostIP)
		if err != nil || !ip.IsLoopback() {
			return client.ContainerCreateOptions{}, &provider.Error{Reason: "UnsafePortBinding"}
		}
		if cfg.ExposedPorts == nil {
			cfg.ExposedPorts = network.PortSet{}
		}
		cfg.ExposedPorts[port] = struct{}{}
		if host.PortBindings == nil {
			host.PortBindings = network.PortMap{}
		}
		host.PortBindings[port] = []network.PortBinding{{HostIP: ip, HostPort: strconv.Itoa(p.HostPort)}}
	}
	if resolved.Probe.Kind == "exec" {
		cfg.Healthcheck = &container.HealthConfig{Test: append([]string{"CMD"}, resolved.Probe.Command...), Interval: 2 * time.Second, Timeout: 2 * time.Second, Retries: 3}
	}
	if resolved.Probe.Kind == "http" || resolved.Probe.Kind == "model" {
		cfg.Healthcheck = &container.HealthConfig{Test: []string{"CMD", "curl", "-f", "http://localhost:" + strconv.Itoa(resolved.Probe.Port) + resolved.Probe.Path}, Interval: 2 * time.Second, Timeout: 2 * time.Second, Retries: 3}
	}
	return client.ContainerCreateOptions{Name: r.Name, Config: cfg, HostConfig: host, NetworkingConfig: &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{sibling(r, resolved.NetworkRole): {Aliases: resolved.Aliases}}}}, nil
}

// Application probes refer to container ports. Resolve the accepted published
// binding, including its deployment parameter, before probing host loopback.
func applicationHostPort(c *spec.ResolvedContainer) (int, error) {
	port := 0
	for _, binding := range c.Ports {
		if binding.ContainerPort != c.Probe.Port {
			continue
		}
		if port != 0 || binding.HostIP != "127.0.0.1" || binding.HostPort < 1 || binding.HostPort > 65535 {
			return 0, &provider.Error{Reason: "InvalidApplicationProbeBinding"}
		}
		port = binding.HostPort
	}
	if port == 0 {
		return 0, &provider.Error{Reason: "InvalidApplicationProbeBinding"}
	}
	return port, nil
}
