package spec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"syscall"
)

const SystemVersion = "mealcheck.dev/system/v1alpha1"
const DeploymentVersion = "mealcheck.dev/deployment/v1alpha1"

type SystemReference struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}
type Parameter struct {
	Type    string   `json:"type"`
	Default any      `json:"default,omitempty"`
	Allowed []any    `json:"allowed,omitempty"`
	Minimum *float64 `json:"minimum,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`
}
type System struct {
	APIVersion      string               `json:"apiVersion"`
	Name            string               `json:"name"`
	Version         string               `json:"version"`
	Architecture    string               `json:"architecture"`
	ApplicationRole string               `json:"applicationRole"`
	Parameters      map[string]Parameter `json:"parameters"`
	Resources       []ResolvedResource   `json:"resources"`
}
type Deployment struct {
	APIVersion   string          `json:"apiVersion"`
	DeploymentID string          `json:"deploymentID"`
	DesiredState string          `json:"desiredState"`
	System       SystemReference `json:"system"`
	Parameters   map[string]any  `json:"parameters"`
}
type ResolvedSystem struct {
	System          SystemReference    `json:"system"`
	Architecture    string             `json:"architecture"`
	ApplicationRole string             `json:"applicationRole"`
	Resources       []ResolvedResource `json:"resources"`
}
type ResolvedResource struct {
	Role      string             `json:"role"`
	Kind      string             `json:"kind"`
	DependsOn []string           `json:"dependsOn,omitempty"`
	Container *ResolvedContainer `json:"container,omitempty"`
}
type ResolvedContainer struct {
	Image           string   `json:"image"`
	User            string   `json:"user,omitempty"`
	Args            []string `json:"args,omitempty"`
	Environment     []string `json:"environment,omitempty"`
	NanoCPUs        int64    `json:"nanoCPUs"`
	MemoryBytes     int64    `json:"memoryBytes"`
	NetworkRole     string   `json:"networkRole"`
	Aliases         []string `json:"aliases,omitempty"`
	Mounts          []Mount  `json:"mounts,omitempty"`
	Ports           []Port   `json:"ports,omitempty"`
	Probe           Probe    `json:"probe"`
	CPUParameter    string   `json:"cpuParameter,omitempty"`
	MemoryParameter string   `json:"memoryParameter,omitempty"`
}
type Mount struct {
	Kind     string `json:"kind"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readOnly"`
}
type Port struct {
	ContainerPort     int    `json:"containerPort"`
	HostPort          int    `json:"hostPort"`
	HostIP            string `json:"hostIP"`
	HostPortParameter string `json:"hostPortParameter,omitempty"`
}
type Probe struct {
	Kind       string   `json:"kind"`
	Command    []string `json:"command,omitempty"`
	Path       string   `json:"path,omitempty"`
	Port       int      `json:"port,omitempty"`
	Components []string `json:"components,omitempty"`
}
type EngineInfo struct {
	Architecture string
	CPUs         int
	MemoryBytes  int64
}
type Capacity = EngineInfo
type Catalog struct{ Root string }

func noDuplicates(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	var walk func() error
	walk = func() error {
		token, e := dec.Token()
		if e != nil {
			return e
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				key, e := dec.Token()
				if e != nil {
					return e
				}
				name, ok := key.(string)
				if !ok {
					return errors.New("invalid object key")
				}
				if seen[name] {
					return fmt.Errorf("duplicate JSON field %s", name)
				}
				seen[name] = true
				if e = walk(); e != nil {
					return e
				}
			}
			_, e = dec.Token()
			return e
		case '[':
			for dec.More() {
				if e = walk(); e != nil {
					return e
				}
			}
			_, e = dec.Token()
			return e
		default:
			return errors.New("invalid JSON delimiter")
		}
	}
	if e := walk(); e != nil {
		return e
	}
	if _, e := dec.Token(); e != io.EOF {
		return errors.New("expected one JSON document")
	}
	return nil
}
func strict(b []byte, v any) error {
	if e := noDuplicates(b); e != nil {
		return e
	}

	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if e := dec.Decode(v); e != nil {
		return e
	}
	if e := dec.Decode(new(any)); e != io.EOF {
		return errors.New("expected one JSON document")
	}
	return nil
}
func DecodeDeployment(b []byte) (Deployment, error) {
	var d Deployment
	e := strict(b, &d)
	if e == nil {
		e = requiredFields(b, "apiVersion", "deploymentID", "desiredState", "system", "parameters")
	}
	if e == nil && d.Parameters == nil {
		e = errors.New("deployment parameters must be an object")
	}
	if e == nil {
		var fields map[string]json.RawMessage
		json.Unmarshal(b, &fields)
		e = requiredFields(fields["system"], "name", "version", "digest")
	}
	return d, e
}
func DecodeSystem(b []byte) (System, error) {
	var s System
	e := strict(b, &s)
	if e == nil {
		e = requiredFields(b, "apiVersion", "name", "version", "architecture", "applicationRole", "parameters", "resources")
	}
	if e == nil && (s.Parameters == nil || s.Resources == nil) {
		e = errors.New("system parameters and resources must be objects and arrays")
	}
	if e == nil {
		e = systemStructure(b)
	}
	return s, e
}

// Check required nested fields before zero-valued Go fields can hide omissions.
func systemStructure(b []byte) error {
	var fields struct {
		Parameters map[string]json.RawMessage `json:"parameters"`
		Resources  []json.RawMessage          `json:"resources"`
	}
	if err := json.Unmarshal(b, &fields); err != nil {
		return err
	}
	for _, parameter := range fields.Parameters {
		if err := requiredFields(parameter, "type"); err != nil {
			return err
		}
		var p map[string]json.RawMessage
		json.Unmarshal(parameter, &p)
		if allowed, ok := p["allowed"]; ok {
			var values []any
			if err := json.Unmarshal(allowed, &values); err != nil || len(values) == 0 {
				return errors.New("allowed parameter values must be a nonempty array")
			}
		}
	}
	for _, resource := range fields.Resources {
		if err := requiredFields(resource, "role", "kind"); err != nil {
			return err
		}
		var res struct {
			Container json.RawMessage `json:"container"`
		}
		json.Unmarshal(resource, &res)
		if len(res.Container) == 0 {
			continue
		}
		if err := requiredFields(res.Container, "image", "nanoCPUs", "memoryBytes", "networkRole", "probe"); err != nil {
			return err
		}
		var c struct {
			Mounts []json.RawMessage `json:"mounts"`
			Ports  []json.RawMessage `json:"ports"`
			Probe  json.RawMessage   `json:"probe"`
		}
		json.Unmarshal(res.Container, &c)
		for _, mount := range c.Mounts {
			if err := requiredFields(mount, "kind", "source", "target", "readOnly"); err != nil {
				return err
			}
		}
		for _, port := range c.Ports {
			if err := requiredFields(port, "containerPort", "hostPort", "hostIP"); err != nil {
				return err
			}
		}
		if err := requiredFields(c.Probe, "kind"); err != nil {
			return err
		}
	}
	return nil
}
func requiredFields(b []byte, keys ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return err
	}
	for _, key := range keys {
		value, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("required field %s is missing or null", key)
		}
	}
	return nil
}
func (s System) Digest() string {
	b, _ := json.Marshal(s)
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}
func trusted(path string, dir bool) error {
	fi, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if fi.Mode()&os.ModeSymlink != 0 || fi.Mode().Perm()&0022 != 0 {
		return errors.New("catalog must not be symlink or group/world writable")
	}
	if !dir && !fi.Mode().IsRegular() {
		return errors.New("catalog requires regular file")
	}
	if fi.Size() > 1024*1024 {
		return errors.New("catalog file too large")
	}
	if dir != fi.IsDir() {
		return errors.New("unexpected catalog file type")
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Getuid() {
		return errors.New("catalog must be owned by controller user")
	}
	return nil
}
func (c Catalog) Load(ref SystemReference) (System, error) {
	var s System
	if !id.MatchString(ref.Name) || !id.MatchString(ref.Version) {
		return s, errors.New("invalid system reference")
	}
	if e := trusted(c.Root, true); e != nil {
		return s, e
	}
	p := filepath.Join(c.Root, ref.Name+"--"+ref.Version+".json")
	if e := trusted(p, false); e != nil {
		return s, e
	}
	b, e := os.ReadFile(p)
	if e != nil {
		return s, e
	}
	s, e = DecodeSystem(b)
	if e != nil {
		return s, e
	}
	if s.Name != ref.Name || s.Version != ref.Version || s.Digest() != ref.Digest {
		return s, errors.New("system identity or digest mismatch")
	}
	return s, nil
}
func (w Workload) Resolved() (*ResolvedSystem, error) {
	if w.ResolvedJSON == "" {
		return nil, nil
	}
	var r ResolvedSystem
	if e := strict([]byte(w.ResolvedJSON), &r); e != nil {
		return nil, e
	}
	return &r, nil
}

var reference = regexp.MustCompile(`\$\{([a-zA-Z][a-zA-Z0-9_-]*)\}`)

func parameter(p Parameter, v any) error {
	switch p.Type {
	case "string":
		if _, ok := v.(string); !ok {
			return errors.New("expected string")
		}
	case "integer":
		f, ok := v.(float64)
		if !ok {
			switch n := v.(type) {
			case int:
				f = float64(n)
			case int64:
				f = float64(n)
			default:
				return errors.New("expected integer")
			}
		}
		if f != float64(int64(f)) {
			return errors.New("expected integer")
		}
		if p.Minimum != nil && f < *p.Minimum || p.Maximum != nil && f > *p.Maximum {
			return errors.New("parameter outside bounds")
		}
	default:
		return errors.New("unsupported parameter type")
	}
	if len(p.Allowed) > 0 {
		ok := false
		for _, a := range p.Allowed {
			if reflect.DeepEqual(a, v) {
				ok = true
			}
		}
		if !ok {
			return errors.New("parameter value not allowed")
		}
	}
	return nil
}
func Resolve(d Deployment, s System, p Policy, cap Capacity) (Document, error) {
	var out Document
	if d.APIVersion != DeploymentVersion || s.APIVersion != SystemVersion {
		return out, errors.New("unsupported manifest version")
	}
	if d.System.Name != s.Name || d.System.Version != s.Version || d.System.Digest != s.Digest() {
		return out, errors.New("system identity or digest mismatch")
	}
	if cap.Architecture != s.Architecture {
		return out, errors.New("engine architecture mismatch")
	}
	if !id.MatchString(s.Name) || !id.MatchString(s.Version) || s.Parameters == nil || d.Parameters == nil {
		return out, errors.New("invalid system identity or parameter object")
	}
	if cap.CPUs <= 0 || cap.MemoryBytes <= 0 {
		return out, errors.New("engine capacity must be known")
	}
	values := map[string]any{}
	for k, v := range s.Parameters {
		if !regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`).MatchString(k) {
			return out, errors.New("invalid parameter name")
		}
		if v.Minimum != nil && v.Maximum != nil && *v.Minimum > *v.Maximum {
			return out, errors.New("invalid parameter bounds")
		}
		if v.Type == "string" && (v.Minimum != nil || v.Maximum != nil) {
			return out, errors.New("string parameter cannot have numeric bounds")
		}
		if v.Allowed != nil && len(v.Allowed) == 0 {
			return out, errors.New("allowed parameter list cannot be empty")
		}
		for _, allowed := range v.Allowed {
			check := v
			check.Allowed = nil
			if err := parameter(check, allowed); err != nil {
				return out, fmt.Errorf("invalid allowed value for %s: %w", k, err)
			}
		}
		val, ok := d.Parameters[k]
		if !ok {
			val = v.Default
		}
		if e := parameter(v, val); e != nil {
			return out, fmt.Errorf("parameter %s: %w", k, e)
		}
		values[k] = val
	}
	for k := range d.Parameters {
		if _, ok := s.Parameters[k]; !ok {
			return out, fmt.Errorf("unknown parameter %s", k)
		}
	}
	if raw, ok := values["modelPath"].(string); ok {
		canonical, err := filepath.EvalSymlinks(raw)
		if err != nil {
			return out, errors.New("model file cannot be resolved")
		}
		values["modelPath"] = canonical
	}
	// Substitute only declared scalar references; no evaluation, shell, or template language.
	b, _ := json.Marshal(s.Resources)
	var tree any
	if e := json.Unmarshal(b, &tree); e != nil {
		return out, e
	}
	var subst func(any) (any, error)
	subst = func(v any) (any, error) {
		switch t := v.(type) {
		case string:
			var err error
			r := reference.ReplaceAllStringFunc(t, func(m string) string {
				k := reference.FindStringSubmatch(m)[1]
				x, ok := values[k]
				if !ok {
					err = fmt.Errorf("unknown parameter reference %s", k)
					return m
				}
				return fmt.Sprint(x)
			})
			if strings.Contains(r, "${") {
				return nil, errors.New("unresolved parameter reference")
			}
			return r, err
		case []any:
			for i, x := range t {
				y, e := subst(x)
				if e != nil {
					return nil, e
				}
				t[i] = y
			}
		case map[string]any:
			for k, x := range t {
				y, e := subst(x)
				if e != nil {
					return nil, e
				}
				t[k] = y
			}
		}
		return v, nil
	}
	tree, e := subst(tree)
	if e != nil {
		return out, e
	}
	b, _ = json.Marshal(tree)
	var resources []ResolvedResource
	if e = json.Unmarshal(b, &resources); e != nil {
		return out, e
	}
	integer := func(k string) (int64, error) {
		v, ok := values[k]
		if !ok || s.Parameters[k].Type != "integer" {
			return 0, errors.New("integer binding requires declared integer parameter")
		}
		switch n := v.(type) {
		case float64:
			return int64(n), nil
		case int:
			return int64(n), nil
		case int64:
			return n, nil
		}
		return 0, errors.New("invalid integer")
	}
	for i := range resources {
		c := resources[i].Container
		if c == nil {
			continue
		}
		if c.CPUParameter != "" {
			c.NanoCPUs, e = integer(c.CPUParameter)
			if e != nil {
				return out, e
			}
			c.CPUParameter = ""
		}
		if c.MemoryParameter != "" {
			c.MemoryBytes, e = integer(c.MemoryParameter)
			if e != nil {
				return out, e
			}
			c.MemoryParameter = ""
		}
		for j := range c.Ports {
			if c.Ports[j].HostPortParameter != "" {
				n, e := integer(c.Ports[j].HostPortParameter)
				if e != nil {
					return out, e
				}
				c.Ports[j].HostPort = int(n)
				c.Ports[j].HostPortParameter = ""
			}
		}
	}
	r := ResolvedSystem{System: d.System, Architecture: s.Architecture, ApplicationRole: s.ApplicationRole, Resources: resources}
	if e := r.Validate(p, cap); e != nil {
		return out, e
	}
	out = Document{APIVersion: Version, DeploymentID: d.DeploymentID, DesiredState: d.DesiredState, Spec: Workload{Profile: "cpu-local-model-v1", DataPolicy: "Retain"}}
	get := func(k string) string { return fmt.Sprint(values[k]) }
	out.Spec.ModelPath = get("modelPath")
	out.Spec.SecretProfile = get("secretProfile")
	for _, r := range resources {
		if r.Container == nil {
			continue
		}
		switch r.Role {
		case "api":
			out.Spec.APIImage = r.Container.Image
			for _, port := range r.Container.Ports {
				out.Spec.APIHostPort = port.HostPort
			}
		case "postgres":
			out.Spec.PostgresImage = r.Container.Image
		case "model":
			out.Spec.ModelImage = r.Container.Image
		}
	}
	b, _ = json.Marshal(r)
	out.Spec.ResolvedJSON = string(b)
	b, _ = json.Marshal(d)
	out.DeploymentJSON = string(b)
	for _, resource := range r.Resources {
		if resource.Container == nil {
			continue
		}
		for _, mount := range resource.Container.Mounts {
			if mount.Kind == "model" && mount.Source != out.Spec.ModelPath {
				return Document{}, errors.New("model mount must reference deployment modelPath")
			}
		}
	}
	if e = out.Validate(p); e != nil {
		return Document{}, e
	}
	return out, nil
}
func (r ResolvedSystem) Validate(p Policy, cap Capacity) error {
	if r.Architecture != "arm64" && r.Architecture != "amd64" {
		return errors.New("unsupported system architecture")
	}
	expected := map[string]string{"network": "network", "database-volume": "volume", "artifact-volume": "volume", "postgres": "container", "model": "container", "api": "container"}
	seen := map[string]ResolvedResource{}
	for _, res := range r.Resources {
		if expected[res.Role] != res.Kind {
			return fmt.Errorf("unsupported resource %s/%s", res.Role, res.Kind)
		}
		if _, ok := seen[res.Role]; ok {
			return errors.New("duplicate resource")
		}
		seen[res.Role] = res
		deps := map[string]bool{}
		for _, dependency := range res.DependsOn {
			if deps[dependency] {
				return errors.New("duplicate dependency")
			}
			deps[dependency] = true
		}
	}
	if len(seen) != 6 || r.ApplicationRole != "api" {
		return errors.New("system requires MealCheck six-resource graph and api application role")
	}
	vis := map[string]int{}
	var walk func(string) error
	walk = func(k string) error {
		if vis[k] == 1 {
			return errors.New("dependency cycle")
		}
		if vis[k] == 2 {
			return nil
		}
		res, ok := seen[k]
		if !ok {
			return errors.New("missing dependency")
		}
		vis[k] = 1
		for _, dep := range res.DependsOn {
			if e := walk(dep); e != nil {
				return e
			}
		}
		vis[k] = 2
		return nil
	}
	var totalCPU, totalMem int64
	for k, res := range seen {
		if e := walk(k); e != nil {
			return e
		}
		c := res.Container
		if res.Kind != "container" {
			if c != nil {
				return errors.New("non-container resource has configuration")
			}
			continue
		}
		if c == nil || !image.MatchString(c.Image) {
			return errors.New("container requires digest-pinned image")
		}
		allowed := false
		for _, reg := range p.Registries {
			if strings.HasPrefix(c.Image, reg+"/") || strings.HasPrefix(c.Image, reg+"@") {
				allowed = true
			}
		}
		if !allowed {
			return errors.New("image registry is not allowed")
		}
		if c.NanoCPUs <= 0 || c.MemoryBytes <= 0 {
			return errors.New("resource limits must be positive")
		}
		if c.NanoCPUs > totalCPU {
			totalCPU = c.NanoCPUs
		}
		if c.MemoryBytes > math.MaxInt64-totalMem {
			return errors.New("memory limits overflow")
		}
		totalMem += c.MemoryBytes
		hasDependency := func(role string) bool {
			for _, dep := range res.DependsOn {
				if dep == role {
					return true
				}
			}
			return false
		}
		if !hasDependency(c.NetworkRole) {
			return errors.New("container must depend on its network")
		}
		for _, mount := range c.Mounts {
			if mount.Kind == "volume" && !hasDependency(mount.Source) {
				return errors.New("container must depend on mounted volume")
			}
		}
		if c.NetworkRole != "network" {
			return errors.New("private network required")
		}
		for _, port := range c.Ports {
			min, max := p.MinPort, p.MaxPort
			if min == 0 {
				min = 1024
			}
			if max == 0 {
				max = 65535
			}
			if res.Role != "api" || port.HostIP != "127.0.0.1" || port.HostPort < min || port.HostPort > max || port.ContainerPort < 1 || port.ContainerPort > 65535 {
				return errors.New("only loopback API port publication is allowed")
			}
		}
		for _, m := range c.Mounts {
			if !filepath.IsAbs(m.Target) || filepath.Clean(m.Target) != m.Target {
				return errors.New("invalid container mount target")
			}
			switch m.Kind {
			case "volume":
				if expected[m.Source] != "volume" {
					return errors.New("unknown volume mount")
				}
			case "model":
				if !m.ReadOnly {
					return errors.New("model mount must be read-only")
				}
			case "secret":
				if !m.ReadOnly || (m.Source != "postgres-password" && m.Source != "database-url") {
					return errors.New("invalid secret file reference")
				}
			default:
				return errors.New("unsupported mount kind")
			}
		}
		switch c.Probe.Kind {
		case "exec":
			if len(c.Probe.Command) == 0 {
				return errors.New("exec probe requires command")
			}
		case "http", "model", "application":
			if c.Probe.Kind == "application" && len(c.Probe.Components) == 0 {
				return errors.New("application probe requires components")
			}
			if c.Probe.Kind == "application" {
				matches := 0
				for _, port := range c.Ports {
					if port.ContainerPort == c.Probe.Port {
						matches++
					}
				}
				if res.Role != r.ApplicationRole || matches != 1 {
					return errors.New("application probe requires exactly one matching published container port")
				}
			}
			if c.Probe.Port < 1 || c.Probe.Port > 65535 || !strings.HasPrefix(c.Probe.Path, "/") {
				return errors.New("invalid HTTP probe")
			}
		default:
			return errors.New("unsupported probe")
		}
	}
	if cap.CPUs > 0 && totalCPU > int64(cap.CPUs)*1000000000 || cap.MemoryBytes > 0 && totalMem > cap.MemoryBytes {
		return errors.New("system resource limits exceed engine capacity")
	}
	return nil
}
