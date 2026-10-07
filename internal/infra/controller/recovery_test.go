package controller

import (
	"context"
	"errors"
	"github.com/chranama/MealCheck/internal/infra/provider"
	"github.com/chranama/MealCheck/internal/infra/provider/fake"
	"github.com/chranama/MealCheck/internal/infra/state"
	"strings"
	"testing"
	"time"
)

func TestPersistedRestartBudgetAndExplicitRetry(t *testing.T) {
	c, p := setup(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Now = func() time.Time { return now }
	converge(t, c, "Ready")
	r, _ := c.Store.Get()
	api := Resources(c.Store.InstallationID, r)[5]
	for i := 0; i < 5; i++ {
		v := p.Resources[api.Name]
		v.Running = false
		v.Ready = false
		p.Resources[api.Name] = v
		if e := c.Step(context.Background()); e != nil {
			t.Fatal(e)
		}
	}
	r, _ = c.Store.Get()
	if r.Status.Phase != "Blocked" || p.Calls["start:api"] != 5 {
		t.Fatal(r.Status, p.Calls)
	}
	if _, e := c.Store.Apply(r.Document); e != nil {
		t.Fatal(e)
	}
	c.Step(context.Background())
	if p.Calls["start:api"] != 5 {
		t.Fatal("identical apply reset budget")
	}
	replacement := &Controller{Store: c.Store, Provider: p, Now: func() time.Time { return now }}
	replacement.Step(context.Background())
	if p.Calls["start:api"] != 5 {
		t.Fatal("controller restart reset budget")
	}
	now = now.Add(10 * time.Minute)
	replacement.Step(context.Background())
	if p.Calls["start:api"] != 6 {
		t.Fatal("window did not reset")
	}
	if _, e := c.Store.Retry(); e != nil {
		t.Fatal(e)
	}
	r, _ = c.Store.Get()
	if len(r.Status.Starts) != 0 || r.Status.RetryCount != 0 {
		t.Fatal("retry not reset")
	}
}
func TestReadinessWindowRequiresExplicitRetry(t *testing.T) {
	c, p := setup(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Now = func() time.Time { return now }
	converge(t, c, "Ready")
	r, _ := c.Store.Get()
	model := Resources(c.Store.InstallationID, r)[4]
	v := p.Resources[model.Name]
	v.Ready = false
	p.Resources[model.Name] = v
	c.Step(context.Background())
	now = now.Add(120 * time.Second)
	c.Step(context.Background())
	r, _ = c.Store.Get()
	if r.Status.Phase != "Blocked" || r.Status.Failure != "modelReadinessBudgetExhausted" {
		t.Fatal(r.Status)
	}
	v.Ready = true
	p.Resources[model.Name] = v
	c.Step(context.Background())
	r, _ = c.Store.Get()
	if r.Status.Phase != "Blocked" {
		t.Fatal("readiness budget evaded")
	}
	c.Store.Retry()
	converge(t, c, "Ready")
}
func TestBackoffUnknownObservationAndSanitization(t *testing.T) {
	c, p := setup(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Now = func() time.Time { return now }
	c.Jitter = func(d time.Duration) time.Duration { return d }
	p.Unavailable = true
	for _, delay := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second} {
		c.Step(context.Background())
		r, _ := c.Store.Get()
		if r.Status.NextRetry.Sub(now) != delay {
			t.Fatal(r.Status.NextRetry.Sub(now), delay)
		}
		if len(p.Calls) != 0 {
			t.Fatal("mutation during outage")
		}
		now = r.Status.NextRetry
	}
	p.Unavailable = false
	converge(t, c, "Ready")
	r, _ := c.Store.Get()
	if r.Status.RetryCount != 0 || !r.Status.NextRetry.IsZero() {
		t.Fatal(r.Status)
	}
}

type faultProvider struct {
	*fake.Provider
	failure error
	wait    bool
}

func (p faultProvider) Ensure(ctx context.Context, r provider.Resource) error {
	if p.wait {
		<-ctx.Done()
		return ctx.Err()
	}
	return p.failure
}
func TestBoundedCallsPermanentFailuresAndPrivacy(t *testing.T) {
	c, p := setup(t)
	c.Provider = faultProvider{Provider: p, wait: true}
	c.Deadline = time.Millisecond
	start := time.Now()
	if e := c.Step(context.Background()); e != nil {
		t.Fatal(e)
	}
	if time.Since(start) > time.Second {
		t.Fatal("provider deadline not enforced")
	}
	r, _ := c.Store.Get()
	if r.Status.Phase != "Degraded" || r.Status.NextRetry.IsZero() {
		t.Fatal(r.Status)
	}
	c.Store.Retry()
	c.Provider = faultProvider{Provider: p, failure: &provider.Error{Reason: "InvalidMount"}}
	c.Step(context.Background())
	r, _ = c.Store.Get()
	if r.Status.Phase != "Blocked" {
		t.Fatal(r.Status)
	}
	c.Step(context.Background())
	c.Store.Retry()
	c.Provider = faultProvider{Provider: p, failure: errors.New("password=do-not-persist")}
	c.Step(context.Background())
	r, _ = c.Store.Get()
	events, _ := c.Store.Events()
	if strings.Contains(r.Status.Conditions[0].Reason, "password") || strings.Contains(events[len(events)-1].Reason, "password") {
		t.Fatal("raw error leaked")
	}
}
func TestStableConditionTransitionTime(t *testing.T) {
	c, _ := setup(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Now = func() time.Time { return now }
	converge(t, c, "Ready")
	r, _ := c.Store.Get()
	first := r.Status.Conditions[0].TransitionTime
	now = now.Add(time.Minute)
	c.Step(context.Background())
	r, _ = c.Store.Get()
	if !r.Status.Conditions[0].TransitionTime.Equal(first) {
		t.Fatal("poll rewrote transition time")
	}
}
func TestRetryEpochPreventsLostOperatorReset(t *testing.T) {
	c, _ := setup(t)
	r, _ := c.Store.Get()
	stale := r.Status
	stale.Starts = map[string][]time.Time{"api": {time.Now()}}
	c.Store.Retry()
	if e := c.Store.SetStatus(r.Generation, stale); e != nil {
		t.Fatal(e)
	}
	r, _ = c.Store.Get()
	if len(r.Status.Starts) != 0 || r.Status.RetryEpoch == 0 {
		t.Fatal("inflight status erased retry")
	}
}

var _ = state.Status{}

func TestConditionsSeparateResourcesDependenciesApplicationAndProgress(t *testing.T) {
	c, p := setup(t)
	if e := c.Step(context.Background()); e != nil {
		t.Fatal(e)
	}
	r, _ := c.Store.Get()
	conditions := func(r state.Record) map[string]string {
		out := map[string]string{}
		for _, v := range r.Status.Conditions {
			out[v.Type] = v.Status
		}
		return out
	}
	if values := conditions(r); values["Progressing"] != "True" || values["ResourcesReady"] != "False" {
		t.Fatal(values)
	}
	converge(t, c, "Ready")
	r, _ = c.Store.Get()
	model := Resources(c.Store.InstallationID, r)[4]
	v := p.Resources[model.Name]
	v.Ready = false
	p.Resources[model.Name] = v
	c.Step(context.Background())
	r, _ = c.Store.Get()
	values := conditions(r)
	if values["ResourcesReady"] != "True" || values["DependenciesReady"] != "False" || values["ApplicationReady"] != "True" || values["Progressing"] != "True" {
		t.Fatal(values)
	}
	p.Unavailable = true
	c.Step(context.Background())
	r, _ = c.Store.Get()
	values = conditions(r)
	if values["ResourcesReady"] != "Unknown" || values["DependenciesReady"] != "Unknown" || values["ApplicationReady"] != "Unknown" || values["Progressing"] != "False" {
		t.Fatal(values)
	}
}
