// Package docker implements resource mechanics through the Docker Engine API.
package docker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/infra/provider"
	"github.com/containerd/errdefs"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

const labelPrefix = "dev.mealcheck.controller."

type Docker struct {
	cli        *client.Client
	secretRoot string
}

func New(endpoint, secretRoot string) (*Docker, error) {
	if !strings.HasPrefix(endpoint, "unix:///") {
		return nil, errors.New("engine endpoint must be a local Unix socket")
	}
	root, e := filepath.EvalSymlinks(secretRoot)
	if e != nil {
		return nil, errors.New("secret root cannot be resolved")
	}
	c, e := client.New(client.WithHost(endpoint))
	if e != nil {
		return nil, e
	}
	return &Docker{c, root}, nil
}
func (d *Docker) Close() error { return d.cli.Close() }
func classify(e error) error {
	if e == nil {
		return nil
	}
	var pe *provider.Error
	if errors.As(e, &pe) {
		return pe
	}
	reason, transient := "EngineUnavailable", true
	if errdefs.IsConflict(e) {
		reason = "ResourceConflict"
		transient = false
	}
	if errdefs.IsInvalidArgument(e) || errdefs.IsPermissionDenied(e) || errdefs.IsUnauthorized(e) {
		reason = "InvalidResourceConfiguration"
		transient = false
	}
	if errdefs.IsNotFound(e) {
		reason = "ImageOrDependencyMissing"
	}
	return &provider.Error{Reason: reason, Transient: transient}
}
func labels(r provider.Resource) map[string]string {
	return map[string]string{labelPrefix + "owner": r.InstallationID, labelPrefix + "deployment": r.DeploymentID, labelPrefix + "role": r.Role, labelPrefix + "fingerprint": r.Fingerprint, labelPrefix + "generation": strconv.FormatInt(r.Generation, 10)}
}
func observed(r provider.Resource, id string, l map[string]string) provider.Observed {
	return provider.Observed{ID: id, Name: r.Name, Role: l[labelPrefix+"role"], Owner: l[labelPrefix+"owner"], DeploymentID: l[labelPrefix+"deployment"], Fingerprint: l[labelPrefix+"fingerprint"]}
}
func (d *Docker) inspect(ctx context.Context, r provider.Resource) (provider.Observed, error) {
	switch r.Kind {
	case "network":
		x, e := d.cli.NetworkInspect(ctx, r.Name, client.NetworkInspectOptions{})
		if e != nil {
			return provider.Observed{}, e
		}
		o := observed(r, x.Network.ID, x.Network.Labels)
		o.Ready = true
		return o, nil
	case "volume":
		x, e := d.cli.VolumeInspect(ctx, r.Name, client.VolumeInspectOptions{})
		if e != nil {
			return provider.Observed{}, e
		}
		o := observed(r, x.Volume.Labels[labelPrefix+"incarnation"], x.Volume.Labels)
		if o.ID == "" {
			o.Fingerprint = "missing-volume-incarnation"
		}
		o.Ready = true
		return o, nil
	case "container":
		x, e := d.cli.ContainerInspect(ctx, r.Name, client.ContainerInspectOptions{})
		if e != nil {
			return provider.Observed{}, e
		}
		o := observed(r, x.Container.ID, x.Container.Config.Labels)
		if x.Container.State != nil {
			o.Running = x.Container.State.Running
			o.Ready = false
			resolved, resolvedErr := r.Workload.Resolved()
			if resolvedErr != nil {
				return provider.Observed{}, &provider.Error{Reason: "InvalidResolvedConfiguration"}
			}
			if resolved != nil {
				for _, resource := range resolved.Resources {
					if resource.Role != r.Role || resource.Container == nil {
						continue
					}
					p := resource.Container.Probe
					if p.Kind == "application" && o.Running {
						hostPort, portErr := applicationHostPort(resource.Container)
						if portErr != nil {
							return provider.Observed{}, portErr
						}
						o.Ready = probeApplication(ctx, hostPort, p.Path, p.Components)
					} else if x.Container.State.Health != nil {
						o.Ready = o.Running && x.Container.State.Health.Status == "healthy"
					}
				}
			} else if r.Role == "api" && o.Running {
				o.Ready = ProbeApplication(ctx, r.Workload.APIHostPort)
			}
			if resolved == nil && r.Role != "api" && x.Container.State.Health != nil {
				o.Ready = o.Running && x.Container.State.Health.Status == "healthy"
			}
		}
		// A copied ownership label is not sufficient to accept a changed image.
		image := ""
		switch r.Role {
		case "api":
			image = r.Workload.APIImage
		case "postgres":
			image = r.Workload.PostgresImage
		case "model":
			image = r.Workload.ModelImage
		}
		desired, configErr := d.config(r)
		if configErr != nil {
			return provider.Observed{}, configErr
		}
		if r.Workload.ResolvedJSON != "" {
			image = desired.Config.Image
		}
		if x.Container.Config.Image != image || !compatibleConfig(x.Container, desired) {
			o.Fingerprint = "incompatible-configuration"
		}
		return o, nil
	}
	return provider.Observed{}, &provider.Error{Reason: "UnsupportedResource"}
}
func (d *Docker) Observe(ctx context.Context, rs []provider.Resource) (provider.Observation, error) {
	out := provider.Observation{Resources: map[string]provider.Observed{}}
	// Successful ping distinguishes a missing resource from an unavailable engine.
	if _, e := d.cli.Ping(ctx, client.PingOptions{}); e != nil {
		return out, classify(e)
	}
	for _, r := range rs {
		o, e := d.inspect(ctx, r)
		if errdefs.IsNotFound(e) {
			continue
		}
		if e != nil {
			return out, classify(e)
		}
		out.Resources[r.Role] = o
	}
	return out, nil
}
func (d *Docker) Ensure(ctx context.Context, r provider.Resource) error {
	o, e := d.inspect(ctx, r)
	if e == nil {
		if !provider.Compatible(r, o) {
			return &provider.Error{Reason: "OwnershipConflict"}
		}
		return nil
	}
	if !errdefs.IsNotFound(e) {
		return classify(e)
	}
	switch r.Kind {
	case "network":
		_, e = d.cli.NetworkCreate(ctx, r.Name, client.NetworkCreateOptions{Driver: "bridge", Labels: labels(r)})
	case "volume":
		l := labels(r)
		token := make([]byte, 16)
		if _, e = rand.Read(token); e != nil {
			return &provider.Error{Reason: "IdentityUnavailable"}
		}
		l[labelPrefix+"incarnation"] = hex.EncodeToString(token)
		_, e = d.cli.VolumeCreate(ctx, client.VolumeCreateOptions{Name: r.Name, Driver: "local", Labels: l})
	case "container":
		var opts client.ContainerCreateOptions
		opts, e = d.config(r)
		if e == nil {
			_, e = d.cli.ContainerCreate(ctx, opts)
		}
	default:
		return &provider.Error{Reason: "UnsupportedResource"}
	}
	if e != nil && !errdefs.IsConflict(e) {
		return classify(e)
	}
	// Includes raced create conflicts, and verifies volume-create's idempotent return.
	o, inspectErr := d.inspect(ctx, r)
	if inspectErr != nil {
		return classify(inspectErr)
	}
	if !provider.Compatible(r, o) {
		return &provider.Error{Reason: "OwnershipConflict"}
	}
	return nil
}
func (d *Docker) checked(ctx context.Context, r provider.Resource) (provider.Observed, error) {
	o, e := d.inspect(ctx, r)
	if e != nil {
		return o, classify(e)
	}
	if !provider.Compatible(r, o) {
		return o, &provider.Error{Reason: "OwnershipConflict"}
	}
	return o, nil
}
func (d *Docker) Start(ctx context.Context, r provider.Resource) error {
	o, e := d.checked(ctx, r)
	if e != nil {
		return e
	}
	if o.Running {
		return nil
	}
	_, e = d.cli.ContainerStart(ctx, o.ID, client.ContainerStartOptions{})
	return classify(e)
}
func (d *Docker) Stop(ctx context.Context, r provider.Resource) error {
	o, e := d.checked(ctx, r)
	if e != nil {
		return e
	}
	if !o.Running {
		return nil
	}
	t := 10
	_, e = d.cli.ContainerStop(ctx, o.ID, client.ContainerStopOptions{Timeout: &t})
	return classify(e)
}
func (d *Docker) Remove(ctx context.Context, r provider.Resource) error {
	o, e := d.inspect(ctx, r)
	if errdefs.IsNotFound(e) {
		return nil
	}
	if e != nil {
		return classify(e)
	}
	if !provider.Compatible(r, o) {
		return &provider.Error{Reason: "OwnershipConflict"}
	}
	switch r.Kind {
	case "container":
		_, e = d.cli.ContainerRemove(ctx, o.ID, client.ContainerRemoveOptions{})
	case "network":
		_, e = d.cli.NetworkRemove(ctx, o.ID, client.NetworkRemoveOptions{})
	case "volume":
		return &provider.Error{Reason: "DataRetentionRequired"}
	default:
		return &provider.Error{Reason: "UnsupportedResource"}
	}
	return classify(e)
}
func (d *Docker) secret(profile, file string) (string, error) {
	p := filepath.Join(d.secretRoot, profile, file)
	resolved, e := filepath.EvalSymlinks(p)
	if e != nil {
		return "", &provider.Error{Reason: "SecretFileUnavailable"}
	}
	rel, e := filepath.Rel(d.secretRoot, resolved)
	if e != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", &provider.Error{Reason: "SecretPathOutsideRoot"}
	}
	info, e := os.Stat(resolved)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", &provider.Error{Reason: "UnsafeSecretPermissions"}
	}
	return resolved, nil
}

// sibling names derive from the role suffix; the controller fixes this naming contract.
func sibling(r provider.Resource, role string) string {
	return strings.TrimSuffix(r.Name, "-"+r.Role) + "-" + role
}
func (d *Docker) config(r provider.Resource) (client.ContainerCreateOptions, error) {
	if resolved, err := r.Workload.Resolved(); err != nil {
		return client.ContainerCreateOptions{}, &provider.Error{Reason: "InvalidResolvedConfiguration"}
	} else if resolved != nil {
		return d.resolvedConfig(r, resolved)
	}
	w := r.Workload
	cfg := &container.Config{Labels: labels(r)}
	host := &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "no"}, Resources: container.Resources{Memory: 2 << 30, NanoCPUs: 2_000_000_000}}
	bind := func(src, dst string, ro bool) {
		host.Mounts = append(host.Mounts, mount.Mount{Type: mount.TypeBind, Source: src, Target: dst, ReadOnly: ro})
	}
	vol := func(role, dst string) {
		host.Mounts = append(host.Mounts, mount.Mount{Type: mount.TypeVolume, Source: sibling(r, role), Target: dst})
	}
	switch r.Role {
	case "postgres":
		s, e := d.secret(w.SecretProfile, "postgres-password")
		if e != nil {
			return client.ContainerCreateOptions{}, e
		}
		bind(s, "/run/secrets/postgres-password", true)
		cfg.Image = w.PostgresImage
		cfg.Env = []string{"POSTGRES_DB=mealcheck", "POSTGRES_USER=mealcheck", "POSTGRES_PASSWORD_FILE=/run/secrets/postgres-password"}
		vol("database-volume", "/var/lib/postgresql/data")
		host.Memory = 1 << 30
		cfg.Healthcheck = &container.HealthConfig{Test: []string{"CMD-SHELL", "pg_isready -U mealcheck -d mealcheck"}, Interval: 2 * time.Second, Timeout: 2 * time.Second, Retries: 3}
	case "model":
		resolved, pathErr := filepath.EvalSymlinks(w.ModelPath)
		if pathErr != nil || resolved != w.ModelPath {
			return client.ContainerCreateOptions{}, &provider.Error{Reason: "ModelPathMustBeCanonical"}
		}
		info, e := os.Stat(w.ModelPath)
		if e != nil || !info.Mode().IsRegular() {
			return client.ContainerCreateOptions{}, &provider.Error{Reason: "ModelFileUnavailable"}
		}
		cfg.Image = w.ModelImage
		cfg.Cmd = []string{"--model", "/models/model.gguf", "--host", "0.0.0.0", "--port", "8080", "--alias", "mealcheck-lab-model", "--ctx-size", "8192", "--threads", "8", "--n-gpu-layers", "0", "--jinja"}
		cfg.Healthcheck = &container.HealthConfig{Test: []string{"CMD", "curl", "-f", "http://localhost:8080/health"}, Interval: 2 * time.Second, Timeout: 2 * time.Second, Retries: 3}
		bind(w.ModelPath, "/models/model.gguf", true)
		host.Memory = 4 << 30
		host.NanoCPUs = 8_000_000_000
	case "api":
		s, e := d.secret(w.SecretProfile, "database-url")
		if e != nil {
			return client.ContainerCreateOptions{}, e
		}
		bind(s, "/run/secrets/database-url", true)
		cfg.Image = w.APIImage
		cfg.User = "10001:10001"
		cfg.Env = []string{"MEALCHECK_ADDR=0.0.0.0:8080", "MEALCHECK_STORE=postgres", "MEALCHECK_HOSTED_MODE=local_model", "MEALCHECK_LOCAL_MODEL_ENABLED=true", "MEALCHECK_LOCAL_MODEL_BASE_URL=http://model:8080/v1", "MEALCHECK_LOCAL_MODEL_NAME=mealcheck-lab-model", "MEALCHECK_DATA_DIR=/var/lib/mealcheck", "MEALCHECK_ARTIFACT_DIR=/var/lib/mealcheck/artifacts", "MEALCHECK_FNDDS_FALLBACK_PATH=/opt/mealcheck/data/reference/fndds-2021-2023/fndds.sqlite"}
		vol("artifact-volume", "/var/lib/mealcheck/artifacts")
		p := network.MustParsePort("8080/tcp")
		cfg.ExposedPorts = network.PortSet{p: struct{}{}}
		host.PortBindings = network.PortMap{p: []network.PortBinding{{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: strconv.Itoa(w.APIHostPort)}}}
	default:
		return client.ContainerCreateOptions{}, &provider.Error{Reason: "UnsupportedResource"}
	}
	return client.ContainerCreateOptions{Name: r.Name, Config: cfg, HostConfig: host, NetworkingConfig: &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{sibling(r, "network"): {Aliases: []string{r.Role}}}}}, nil
}

// ProbeApplication bounds a host-loopback application readiness request.
func ProbeApplication(ctx context.Context, port int) bool {
	return probeApplication(ctx, port, "/api/status", []string{"meal_check_submission", "ai_meal_normalization", "nutrition_allergen_checking"})
}
func probeApplication(ctx context.Context, port int, path string, components []string) bool {
	req, e := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:"+strconv.Itoa(port)+path, nil)
	if e != nil {
		return false
	}
	c := &http.Client{Timeout: 2 * time.Second}
	resp, e := c.Do(req)
	if e != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var status struct {
		Components []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"components"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&status) != nil {
		return false
	}
	ready := map[string]bool{}
	for _, c := range status.Components {
		ready[c.ID] = c.State == "operational"
	}
	for _, component := range components {
		if !ready[component] {
			return false
		}
	}
	return len(components) > 0
}

// Labels assert ownership; actual settings must also satisfy the fixed profile.
func compatibleConfig(actual container.InspectResponse, desired client.ContainerCreateOptions) bool {
	c, h := actual.Config, actual.HostConfig
	if c == nil || h == nil || c.User != desired.Config.User || h.Privileged || len(h.CapAdd) > 0 || len(h.Devices) > 0 || len(h.SecurityOpt) > 0 || h.PidMode != "" || h.IpcMode == "host" || h.RestartPolicy.Name != "no" || h.Memory != desired.HostConfig.Memory || h.NanoCPUs != desired.HostConfig.NanoCPUs || !equalPorts(h.PortBindings, desired.HostConfig.PortBindings) {
		return false
	}
	if desired.Config.Healthcheck != nil && (c.Healthcheck == nil || !reflect.DeepEqual(c.Healthcheck.Test, desired.Config.Healthcheck.Test)) {
		return false
	}
	if len(desired.Config.Cmd) > 0 && !reflect.DeepEqual(c.Cmd, desired.Config.Cmd) {
		return false
	}
	for _, required := range desired.Config.Env {
		found := false
		key := strings.SplitN(required, "=", 2)[0]
		for _, value := range c.Env {
			if strings.SplitN(value, "=", 2)[0] == key && value != required {
				return false
			}
			if value == required {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if len(actual.Mounts) != len(desired.HostConfig.Mounts) {
		return false
	}
	for _, required := range desired.HostConfig.Mounts {
		found := false
		for _, m := range actual.Mounts {
			source := m.Source
			if m.Type == mount.TypeVolume {
				source = m.Name
			}
			if m.Type == required.Type && source == required.Source && m.Destination == required.Target && m.RW != required.ReadOnly {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if actual.NetworkSettings == nil || len(actual.NetworkSettings.Networks) != 1 {
		return false
	}
	for name := range desired.NetworkingConfig.EndpointsConfig {
		ep := actual.NetworkSettings.Networks[name]
		if ep == nil {
			return false
		}
		for _, alias := range desired.NetworkingConfig.EndpointsConfig[name].Aliases {
			found := false
			for _, value := range ep.Aliases {
				if value == alias {
					found = true
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

func equalPorts(a, b network.PortMap) bool {
	if len(a) != len(b) {
		return false
	}
	for p, v := range a {
		if !reflect.DeepEqual(v, b[p]) {
			return false
		}
	}
	return true
}
