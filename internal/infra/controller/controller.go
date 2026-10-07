// Package controller implements serial, observation-first resource reconciliation.
package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/chranama/MealCheck/internal/infra/provider"
	"github.com/chranama/MealCheck/internal/infra/spec"
	"github.com/chranama/MealCheck/internal/infra/state"
	"github.com/google/uuid"
	"math/rand/v2"
	"sync"
	"time"
)

type Controller struct {
	mu                  sync.Mutex
	Store               *state.Store
	Provider            provider.Provider
	Now                 func() time.Time
	Deadline            time.Duration
	AfterMutation       func() error
	StartupWindow       time.Duration
	RetryBase, RetryMax time.Duration
	StartLimit          int
	StartWindow         time.Duration
	Jitter              func(time.Duration) time.Duration
	Wake                <-chan struct{}
}

func Resources(owner string, r state.Record) []provider.Resource {
	roles := []string{"network", "database-volume", "artifact-volume", "postgres", "model", "api"}
	kinds := map[string]string{}
	dependencies := map[string][]string{}
	if resolved, err := r.Document.Spec.Resolved(); err == nil && resolved != nil {
		roles = nil
		// Stable topological ordering also makes reverse teardown safe when
		// engineers author resources in a different order.
		pending := append([]spec.ResolvedResource(nil), resolved.Resources...)
		ordered := []spec.ResolvedResource{}
		visited := map[string]bool{}
		for len(pending) > 0 {
			progress := false
			next := []spec.ResolvedResource{}
			for _, resource := range pending {
				ready := true
				for _, dependency := range resource.DependsOn {
					if !visited[dependency] {
						ready = false
					}
				}
				if !ready {
					next = append(next, resource)
					continue
				}
				ordered = append(ordered, resource)
				visited[resource.Role] = true
				progress = true
			}
			if !progress {
				break
			} // Invalid graphs are rejected before acceptance.
			pending = next
		}
		for _, resource := range ordered {
			roles = append(roles, resource.Role)
			kinds[resource.Role] = resource.Kind
			dependencies[resource.Role] = resource.DependsOn
		}
	}
	out := make([]provider.Resource, 0, len(roles))
	for _, role := range roles {
		kind := "container"
		if role == "network" {
			kind = "network"
		}
		if role == "database-volume" || role == "artifact-volume" {
			kind = "volume"
		}
		if configured, ok := kinds[role]; ok {
			kind = configured
		}
		out = append(out, provider.Resource{DependsOn: dependencies[role], Role: role, Kind: kind, Name: fmt.Sprintf("mc-%s-%s-%s", owner[:8], r.Document.DeploymentID, role), InstallationID: owner, DeploymentID: r.Document.DeploymentID, Fingerprint: r.Document.Spec.Fingerprint(), Generation: r.Generation, Workload: r.Document.Spec})
	}
	return out
}
func (c *Controller) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now().UTC()
}
func (c *Controller) observe(ctx context.Context, resources []provider.Resource) (provider.Observation, error) {
	d := c.Deadline
	if d == 0 {
		d = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return c.Provider.Observe(ctx, resources)
}
func (c *Controller) Step(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, e := c.Store.Get()
	if e != nil {
		return e
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !r.Status.NextRetry.IsZero() && c.now().Before(r.Status.NextRetry) {
		return nil
	}
	if _, err := r.Document.Spec.Resolved(); err != nil {
		return fmt.Errorf("invalid persisted system snapshot: %w", err)
	}
	resources := Resources(c.Store.InstallationID, r)
	o, e := c.observe(ctx, resources)
	status := r.Status
	if status.Resources == nil {
		status.Resources = map[string]state.Binding{}
	}
	if status.Starts == nil {
		status.Starts = map[string][]time.Time{}
	}
	if status.StartupSince == nil {
		status.StartupSince = map[string]time.Time{}
	}
	status.ObservedGeneration = r.Generation
	status.Conditions = c.conditions(r.Generation, "False", "Reconciling")
	if e != nil {
		status.Phase = "Degraded"
		status.Conditions = c.conditions(r.Generation, "Unknown", "EngineObservationUnavailable")
		c.backoff(&status)
		return c.Store.SetStatus(r.Generation, status)
	}
	status.LastObservation = c.now()
	status.Conditions = c.conditions(r.Generation, "False", "Reconciling")
	for _, res := range resources {
		actual, exists := o.Resources[res.Role]
		if exists && !provider.Compatible(res, actual) {
			status.Phase = "Blocked"
			status.Conditions = c.conditions(r.Generation, "False", "OwnershipConflict")
			return c.Store.SetStatus(r.Generation, status)
		}
		if res.Kind == "volume" && !exists {
			if _, bound := status.Resources[res.Role]; bound {
				status.Phase = "Blocked"
				status.Conditions = c.conditions(r.Generation, "False", "PersistentVolumeMissing")
				return c.Store.SetStatus(r.Generation, status)
			}
		}
		if exists && res.Kind == "volume" {
			if previous, bound := status.Resources[res.Role]; bound && previous.ID != actual.ID {
				status.Phase = "Blocked"
				status.Conditions = c.conditions(r.Generation, "False", "PersistentVolumeIdentityChanged")
				return c.Store.SetStatus(r.Generation, status)
			}
		}
		if exists {
			status.Resources[res.Role] = state.Binding{ID: actual.ID, Name: res.Name, Retained: res.Kind == "volume" && r.Document.DesiredState == "Deleted"}
		}
	}
	if status.Failure != "" && r.Document.DesiredState == "Running" {
		status.Phase = "Blocked"
		status.Conditions = c.conditions(r.Generation, "False", status.Failure)
		return c.Store.SetStatus(r.Generation, status)
	}
	// Resolve old intents using known observation before selecting another action.
	ops, e := c.Store.Operations()
	if e != nil {
		return e
	}
	for _, op := range ops {
		if op.Outcome != "" {
			continue
		}
		actual, exists := o.Resources[op.Role]
		confirmed := (op.Action == "ensure" && exists) || (op.Action == "start" && exists && actual.Running) || (op.Action == "stop" && exists && !actual.Running) || (op.Action == "remove" && !exists)
		if confirmed {
			op.Outcome = "Confirmed"
		} else {
			op.Outcome = "ObservedNotApplied"
		}
		if e = c.Store.Journal(op); e != nil {
			return e
		}
	}
	var target *provider.Resource
	action := ""
	switch r.Document.DesiredState {
	case "Running":
		status.Phase = "Provisioning"
		for i := range resources {
			res := &resources[i]
			actual, exists := o.Resources[res.Role]
			dependenciesReady := true
			for _, dependency := range res.DependsOn {
				observed, ok := o.Resources[dependency]
				if !ok || !observed.Ready {
					dependenciesReady = false
				}
			}
			if !dependenciesReady {
				continue
			}
			if !exists {
				target = res
				action = "ensure"
				break
			}
			if res.Kind == "container" {
				if !actual.Running {
					target = res
					action = "start"
					break
				}
				if !actual.Ready {
					status.Phase = "Degraded"
					status.Conditions = c.conditions(r.Generation, "False", res.Role+"NotReady")
					since, ok := status.StartupSince[res.Role]
					if !ok {
						since = c.now()
						status.StartupSince[res.Role] = since
					}
					window := c.StartupWindow
					if window == 0 {
						window = 120 * time.Second
					}
					if c.now().Sub(since) >= window {
						status.Phase = "Blocked"
						status.Failure = res.Role + "ReadinessBudgetExhausted"
						status.Conditions = c.conditions(r.Generation, "False", status.Failure)
					}
					break
				}
				delete(status.StartupSince, res.Role)
			}
		}
		allReady := true
		for _, resource := range resources {
			actual, exists := o.Resources[resource.Role]
			if !exists || !actual.Ready || (resource.Kind == "container" && !actual.Running) {
				allReady = false
			}
		}
		if target == nil && status.Phase == "Provisioning" && !allReady {
			status.Phase = "Degraded"
			status.Conditions = c.conditions(r.Generation, "False", "DependenciesNotReady")
		}
		if target == nil && status.Phase == "Provisioning" && allReady {
			status.Phase = "Ready"
			status.RetryCount = 0
			status.NextRetry = time.Time{}
			status.Conditions = c.conditions(r.Generation, "True", "ObservedReady")
			status.Conditions[3].Status = "False"
		}
	case "Stopped", "Deleted":
		status.Phase = "Stopped"
		status.Conditions = c.conditions(r.Generation, "False", r.Document.DesiredState)
		if r.Document.DesiredState == "Deleted" {
			status.Phase = "Deleting"
		}
		for i := len(resources) - 1; i >= 0; i-- {
			res := &resources[i]
			actual, exists := o.Resources[res.Role]
			if !exists || res.Kind == "volume" {
				continue
			}
			if res.Kind == "container" && actual.Running {
				target = res
				action = "stop"
				break
			}
			if r.Document.DesiredState == "Deleted" {
				target = res
				action = "remove"
				break
			}
		}
		if target != nil && r.Document.DesiredState == "Stopped" {
			status.Phase = "Provisioning"
		}
		if target == nil && r.Document.DesiredState == "Deleted" {
			status.Phase = "Deleted"
		}
	}
	if target != nil && action == "start" {
		window := c.StartWindow
		if window == 0 {
			window = 10 * time.Minute
		}
		limit := c.StartLimit
		if limit == 0 {
			limit = 5
		}
		recent := []time.Time{}
		for _, stamp := range status.Starts[target.Role] {
			if c.now().Sub(stamp) < window {
				recent = append(recent, stamp)
			}
		}
		status.Starts[target.Role] = recent
		if len(recent) >= limit {
			status.Phase = "Blocked"
			status.Conditions = c.conditions(r.Generation, "False", target.Role+"RestartBudgetExhausted")
			status.NextRetry = recent[0].Add(window)
			target = nil
		}
	}
	c.observedConditions(&status, r, resources, o)
	if e = c.Store.SetStatus(r.Generation, status); e != nil {
		return e
	}
	if target == nil {
		return nil
	}
	latest, e := c.Store.Get()
	if e != nil {
		return e
	}
	if latest.Generation != r.Generation {
		return nil
	}
	op := state.Operation{ID: uuid.NewString(), Generation: r.Generation, Role: target.Role, Action: action, Fingerprint: target.Fingerprint}
	if e = c.Store.Journal(op); e != nil {
		return e
	}
	if action == "start" {
		status.Starts[target.Role] = append(status.Starts[target.Role], c.now())
		status.StartupSince[target.Role] = c.now()
		if err := c.Store.SetStatus(r.Generation, status); err != nil {
			return err
		}
	}
	deadline := c.Deadline
	if deadline == 0 {
		deadline = 15 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, deadline)
	switch action {
	case "ensure":
		e = c.Provider.Ensure(callCtx, *target)
	case "start":
		e = c.Provider.Start(callCtx, *target)
	case "stop":
		e = c.Provider.Stop(callCtx, *target)
	case "remove":
		e = c.Provider.Remove(callCtx, *target)
	}
	cancel()
	if c.AfterMutation != nil {
		if failure := c.AfterMutation(); failure != nil {
			return failure
		}
	}
	observed, obsErr := c.observe(ctx, resources)
	if obsErr != nil {
		status.Phase = "Degraded"
		status.Conditions = c.conditions(r.Generation, "Unknown", "MutationOutcomeUnknown")
		c.backoff(&status)
		return c.Store.SetStatus(r.Generation, status)
	}
	actual, exists := observed.Resources[target.Role]
	confirmed := (action == "ensure" && exists && provider.Compatible(*target, actual)) || (action == "start" && exists && actual.Running) || (action == "stop" && exists && !actual.Running) || (action == "remove" && !exists)
	if confirmed {
		status.RetryCount = 0
		status.NextRetry = time.Time{}
		op.Outcome = "Confirmed"
		if exists {
			status.Resources[target.Role] = state.Binding{ID: actual.ID, Name: target.Name, Retained: target.Kind == "volume"}
		} else {
			delete(status.Resources, target.Role)
		}
	} else {
		op.Outcome = "ObservedNotApplied"
		status.Phase = "Degraded"
		reason := "ProviderOperationFailed"
		var pe *provider.Error
		if errors.As(e, &pe) && !pe.Transient {
			status.Phase = "Blocked"
			reason = pe.Reason
			status.Failure = reason
		}
		status.Conditions = c.conditions(r.Generation, "False", reason)
		if status.Phase != "Blocked" {
			c.backoff(&status)
		}
	}
	c.observedConditions(&status, r, resources, observed)
	if err := c.Store.Journal(op); err != nil {
		return err
	}
	if err := c.Store.AddEvent(action+":"+op.Outcome, r.Generation); err != nil {
		return err
	}
	return c.Store.SetStatus(r.Generation, status)
}
func (c *Controller) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, e := c.Store.Get(); e == nil {
			if e = c.Step(ctx); e != nil && ctx.Err() == nil {
				return e
			}
		} else if e != sql.ErrNoRows {
			return e
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		case <-c.Wake:
		}
	}
}

func (c *Controller) conditions(g int64, value, reason string) []state.Condition {
	out := []state.Condition{}
	for _, kind := range []string{"ResourcesReady", "DependenciesReady", "ApplicationReady", "Progressing"} {
		conditionValue := value
		if kind == "Progressing" && value == "Unknown" {
			conditionValue = "False"
		}
		out = append(out, state.Condition{Type: kind, Status: conditionValue, Reason: reason, Generation: g, TransitionTime: c.now()})
	}
	return out
}

func (c *Controller) backoff(s *state.Status) {
	s.RetryCount++
	base := c.RetryBase
	if base == 0 {
		base = time.Second
	}
	max := c.RetryMax
	if max == 0 {
		max = 30 * time.Second
	}
	delay := base
	for i := 1; i < s.RetryCount && delay < max; i++ {
		delay *= 2
	}
	if delay > max {
		delay = max
	}
	if c.Jitter != nil {
		delay = c.Jitter(delay)
	} else {
		delay = time.Duration(float64(delay) * (0.8 + rand.Float64()*0.4))
	}
	if delay < base {
		delay = base
	}
	if delay > max {
		delay = max
	}
	s.NextRetry = c.now().Add(delay)
}

// observedConditions separates existence, dependency health, application health,
// and controller progress when an authoritative engine observation is available.
func (c *Controller) observedConditions(status *state.Status, r state.Record, resources []provider.Resource, o provider.Observation) {
	if r.Document.DesiredState != "Running" || status.Phase == "Blocked" {
		return
	}
	allPresent := true
	for _, res := range resources {
		if _, ok := o.Resources[res.Role]; !ok {
			allPresent = false
		}
	}
	applicationRole := "api"
	dependencyRoles := []string{"postgres", "model"}
	if resolved, err := r.Document.Spec.Resolved(); err == nil && resolved != nil {
		applicationRole = resolved.ApplicationRole
		dependencyRoles = nil
		for _, resource := range resolved.Resources {
			if resource.Role == applicationRole {
				dependencyRoles = resource.DependsOn
			}
		}
	}
	dependenciesReady := true
	for _, role := range dependencyRoles {
		actual, ok := o.Resources[role]
		if !ok || !actual.Ready {
			dependenciesReady = false
		}
	}
	api, apiOK := o.Resources[applicationRole]
	values := map[string]bool{"ResourcesReady": allPresent, "DependenciesReady": dependenciesReady, "ApplicationReady": apiOK && api.Running && api.Ready, "Progressing": status.Phase != "Ready"}
	trueReasons := map[string]string{"ResourcesReady": "ResourcesPresent", "DependenciesReady": "DependenciesHealthy", "ApplicationReady": "ApplicationHealthy", "Progressing": "Reconciling"}
	falseReasons := map[string]string{"ResourcesReady": "ResourcesMissing", "DependenciesReady": "DependenciesNotReady", "ApplicationReady": "ApplicationNotReady", "Progressing": "Converged"}
	for i := range status.Conditions {
		kind := status.Conditions[i].Type
		if values[kind] {
			status.Conditions[i].Status = "True"
			status.Conditions[i].Reason = trueReasons[kind]
		} else {
			status.Conditions[i].Status = "False"
			status.Conditions[i].Reason = falseReasons[kind]
		}
	}
}
