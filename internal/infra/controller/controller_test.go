package controller

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/chranama/MealCheck/internal/infra/provider"
	"github.com/chranama/MealCheck/internal/infra/provider/fake"
	"github.com/chranama/MealCheck/internal/infra/spec"
	"github.com/chranama/MealCheck/internal/infra/state"
	"testing"
	"time"
)

func setup(t *testing.T) (*Controller, *fake.Provider) {
	t.Helper()
	s, e := state.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	_, e = s.Apply(spec.Document{APIVersion: spec.Version, DeploymentID: "lab", DesiredState: "Running", Spec: spec.Workload{Profile: "cpu-local-model-v1"}})
	if e != nil {
		t.Fatal(e)
	}
	p := fake.New()
	return &Controller{Store: s, Provider: p}, p
}
func converge(t *testing.T, c *Controller, phase string) {
	t.Helper()
	for i := 0; i < 30; i++ {
		current, _ := c.Store.Get()
		if !current.Status.NextRetry.IsZero() && c.now().Before(current.Status.NextRetry) {
			next := current.Status.NextRetry
			c.Now = func() time.Time { return next }
		}
		if e := c.Step(context.Background()); e != nil {
			t.Fatal(e)
		}
		r, e := c.Store.Get()
		if e != nil {
			t.Fatal(e)
		}
		if r.Status.Phase == phase {
			return
		}
	}
	r, _ := c.Store.Get()
	t.Fatalf("did not converge %s: %+v", phase, r.Status)
}
func TestLifecycleAndIdempotency(t *testing.T) {
	c, p := setup(t)
	converge(t, c, "Ready")
	if len(p.Resources) != 6 {
		t.Fatal(len(p.Resources))
	}
	calls := len(p.Calls)
	for i := 0; i < 5; i++ {
		if e := c.Step(context.Background()); e != nil {
			t.Fatal(e)
		}
	}
	if len(p.Calls) != calls {
		t.Fatal("additional mutations")
	}
	c.Store.Transition("Stopped")
	converge(t, c, "Stopped")
	for _, r := range p.Resources {
		if r.Running {
			t.Fatal("still running")
		}
	}
	c.Store.Transition("Running")
	converge(t, c, "Ready")
	c.Store.Transition("Deleted")
	converge(t, c, "Deleted")
	if len(p.Resources) != 2 {
		t.Fatal("volumes not retained")
	}
	c.Store.Transition("Deleted")
	converge(t, c, "Deleted")
}
func TestCrashAndUnknownCreate(t *testing.T) {
	for _, crash := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "crash"}[crash], func(t *testing.T) {
			c, p := setup(t)
			if crash {
				c.AfterMutation = func() error { return errors.New("crash") }
				if e := c.Step(context.Background()); e == nil {
					t.Fatal("expected crash")
				}
				c.AfterMutation = nil
			} else {
				p.TimeoutAfterEnsure = true
			}
			converge(t, c, "Ready")
			if p.Calls["ensure:network"] != 1 {
				t.Fatal("duplicate network create")
			}
			ops, e := c.Store.Operations()
			if e != nil {
				t.Fatal(e)
			}
			for _, op := range ops {
				if op.Outcome == "" {
					t.Fatal("unresolved intent")
				}
			}
		})
	}
}
func TestDeleteDuringCreate(t *testing.T) {
	c, p := setup(t)
	p.BeforeMutation = func(_ provider.Resource, _ string) {
		p.BeforeMutation = nil
		if _, e := c.Store.Transition("Deleted"); e != nil {
			t.Fatal(e)
		}
	}
	if e := c.Step(context.Background()); e != nil {
		t.Fatal(e)
	}
	r, _ := c.Store.Get()
	if r.Status.Phase == "Ready" {
		t.Fatal("stale ready")
	}
	converge(t, c, "Deleted")
	if len(p.Resources) != 0 {
		t.Fatal("delete did not win")
	}
}
func TestUnavailableCollisionAndDataLoss(t *testing.T) {
	t.Run("unknown", func(t *testing.T) {
		c, p := setup(t)
		p.Unavailable = true
		c.Step(context.Background())
		r, _ := c.Store.Get()
		if r.Status.Phase != "Degraded" || len(p.Calls) > 0 {
			t.Fatal("speculative creation")
		}
		p.Unavailable = false
		converge(t, c, "Ready")
	})
	t.Run("collision", func(t *testing.T) {
		c, p := setup(t)
		r, _ := c.Store.Get()
		res := Resources(c.Store.InstallationID, r)[0]
		p.Resources[res.Name] = provider.Observed{ID: "foreign", Name: res.Name, Owner: "someone-else"}
		c.Step(context.Background())
		r, _ = c.Store.Get()
		if r.Status.Phase != "Blocked" || len(p.Calls) > 0 {
			t.Fatal("foreign mutation")
		}
	})
	t.Run("missing-volume", func(t *testing.T) {
		c, p := setup(t)
		converge(t, c, "Ready")
		r, _ := c.Store.Get()
		res := Resources(c.Store.InstallationID, r)[1]
		delete(p.Resources, res.Name)
		c.Step(context.Background())
		r, _ = c.Store.Get()
		if r.Status.Phase != "Blocked" {
			t.Fatal("data silently recreated")
		}
		if p.Calls["ensure:database-volume"] != 1 {
			t.Fatal("new empty volume")
		}
	})
}

func TestCopiedLabelsCannotReplaceBoundVolume(t *testing.T) {
	c, p := setup(t)
	converge(t, c, "Ready")
	r, _ := c.Store.Get()
	res := Resources(c.Store.InstallationID, r)[1]
	replacement := p.Resources[res.Name]
	replacement.ID = "different-volume"
	p.Resources[res.Name] = replacement
	if e := c.Step(context.Background()); e != nil {
		t.Fatal(e)
	}
	r, _ = c.Store.Get()
	if r.Status.Phase != "Blocked" || r.Status.Conditions[0].Reason != "PersistentVolumeIdentityChanged" {
		t.Fatal(r.Status)
	}
}
func TestLifecycleClearsReadyConditions(t *testing.T) {
	c, _ := setup(t)
	converge(t, c, "Ready")
	for _, desired := range []string{"Stopped", "Deleted"} {
		if _, e := c.Store.Transition(desired); e != nil {
			t.Fatal(e)
		}
		r, _ := c.Store.Get()
		for _, condition := range r.Status.Conditions {
			if condition.Status == "True" {
				t.Fatal("stale ready after acceptance")
			}
		}
		phase := map[string]string{"Stopped": "Stopped", "Deleted": "Deleted"}[desired]
		converge(t, c, phase)
		r, _ = c.Store.Get()
		for _, condition := range r.Status.Conditions {
			if condition.Status == "True" {
				t.Fatal("stale readiness after convergence")
			}
		}
	}
}
func TestCrashReopensDurableStore(t *testing.T) {
	dir := t.TempDir()
	s, e := state.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Apply(spec.Document{APIVersion: spec.Version, DeploymentID: "lab", DesiredState: "Running", Spec: spec.Workload{Profile: "cpu-local-model-v1"}})
	if e != nil {
		t.Fatal(e)
	}
	p := fake.New()
	first := &Controller{Store: s, Provider: p, AfterMutation: func() error { return errors.New("crash") }}
	if e = first.Step(context.Background()); e == nil {
		t.Fatal("no crash")
	}
	s.Close()
	reopened, e := state.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	second := &Controller{Store: reopened, Provider: p}
	converge(t, second, "Ready")
	if p.Calls["ensure:network"] != 1 {
		t.Fatal("duplicate after restart")
	}
}

func TestResolvedGraphUsesPersistedRolesAndDependencies(t *testing.T) {
	c, p := setup(t)
	// Install an accepted snapshot before any resources are created.
	r, _ := c.Store.Get()
	system := spec.ResolvedSystem{ApplicationRole: "api", Resources: []spec.ResolvedResource{{Role: "network", Kind: "network"}, {Role: "database-volume", Kind: "volume"}, {Role: "artifact-volume", Kind: "volume"}, {Role: "postgres", Kind: "container", DependsOn: []string{"network", "database-volume"}}, {Role: "model", Kind: "container", DependsOn: []string{"network"}}, {Role: "api", Kind: "container", DependsOn: []string{"postgres", "model", "artifact-volume"}}}}
	b, _ := json.Marshal(system)
	r.Document.Spec.ResolvedJSON = string(b)
	resources := Resources(c.Store.InstallationID, r)
	if len(resources) != 6 || len(resources[5].DependsOn) != 3 {
		t.Fatal("graph ignored")
	}
	// Resource identity keeps the legacy naming convention and snapshots carry
	// their own fingerprint, so catalog availability does not affect planning.
	if resources[0].Name != Resources(c.Store.InstallationID, state.Record{Document: spec.Document{DeploymentID: "lab"}})[0].Name {
		t.Fatal("ownership naming changed")
	}
	p.Resources["network"] = provider.Observed{ID: "network", Ready: true}
	// Directly test readiness aggregation against engineer-defined dependencies.
	status := state.Status{Phase: "Provisioning", Conditions: c.conditions(1, "False", "Reconciling")}
	obs := provider.Observation{Resources: map[string]provider.Observed{}}
	for _, res := range resources {
		obs.Resources[res.Role] = provider.Observed{Ready: true, Running: true}
	}
	c.observedConditions(&status, r, resources, obs)
	if status.Conditions[1].Status != "True" {
		t.Fatal("healthy dependencies rejected")
	}
	volume := obs.Resources["artifact-volume"]
	volume.Ready = false
	obs.Resources["artifact-volume"] = volume
	c.observedConditions(&status, r, resources, obs)
	if status.Conditions[1].Status != "False" {
		t.Fatal("engineer declared dependency ignored")
	}
}

func TestShuffledResolvedGraphLifecycle(t *testing.T) {
	s, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	system := spec.ResolvedSystem{ApplicationRole: "api", Resources: []spec.ResolvedResource{{Role: "api", Kind: "container", DependsOn: []string{"postgres", "model", "artifact-volume"}}, {Role: "model", Kind: "container", DependsOn: []string{"network"}}, {Role: "postgres", Kind: "container", DependsOn: []string{"network", "database-volume"}}, {Role: "artifact-volume", Kind: "volume"}, {Role: "database-volume", Kind: "volume"}, {Role: "network", Kind: "network"}}}
	b, _ := json.Marshal(system)
	_, err = s.Apply(spec.Document{APIVersion: spec.Version, DeploymentID: "lab", DesiredState: "Running", Spec: spec.Workload{Profile: "cpu-local-model-v1", ResolvedJSON: string(b)}})
	if err != nil {
		t.Fatal(err)
	}
	p := fake.New()
	c := &Controller{Store: s, Provider: p}
	converge(t, c, "Ready")
	r, _ := s.Get()
	resources := Resources(s.InstallationID, r)
	positions := map[string]int{}
	for i, res := range resources {
		positions[res.Role] = i
	}
	for _, res := range resources {
		for _, dep := range res.DependsOn {
			if positions[dep] >= positions[res.Role] {
				t.Fatal("unsafe ordering")
			}
		}
	}
	s.Transition("Stopped")
	converge(t, c, "Stopped")
	s.Transition("Running")
	converge(t, c, "Ready")
	s.Transition("Deleted")
	converge(t, c, "Deleted")
	if len(p.Resources) != 2 {
		t.Fatal("persistent data not retained")
	}
}

func TestResolvedUnreadyDependencyNeverClaimsReady(t *testing.T) {
	s, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	system := spec.ResolvedSystem{ApplicationRole: "api", Resources: []spec.ResolvedResource{{Role: "api", Kind: "container", DependsOn: []string{"network"}}, {Role: "network", Kind: "network"}}}
	b, _ := json.Marshal(system)
	_, err = s.Apply(spec.Document{APIVersion: spec.Version, DeploymentID: "lab", DesiredState: "Running", Spec: spec.Workload{ResolvedJSON: string(b)}})
	if err != nil {
		t.Fatal(err)
	}
	p := fake.New()
	c := &Controller{Store: s, Provider: p}
	r, _ := s.Get()
	resources := Resources(s.InstallationID, r)
	for _, res := range resources {
		p.Resources[res.Name] = provider.Observed{ID: res.Role, Owner: res.InstallationID, DeploymentID: res.DeploymentID, Role: res.Role, Fingerprint: res.Fingerprint, Ready: res.Kind == "container", Running: res.Kind == "container"}
	}
	if err = c.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	r, _ = s.Get()
	if r.Status.Phase == "Ready" {
		t.Fatal("unready dependency accepted")
	}
	if len(p.Calls) != 0 {
		t.Fatal("mutated despite blocked dependency")
	}
}
