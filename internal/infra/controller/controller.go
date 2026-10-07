// Package controller implements serial, observation-first resource reconciliation.
package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/chranama/MealCheck/internal/infra/provider"
	"github.com/chranama/MealCheck/internal/infra/state"
	"github.com/google/uuid"
	"sync"
	"time"
)

type Controller struct {
	mu            sync.Mutex
	Store         *state.Store
	Provider      provider.Provider
	Now           func() time.Time
	Deadline      time.Duration
	AfterMutation func() error
}

func Resources(owner string, r state.Record) []provider.Resource {
	roles := []string{"network", "database-volume", "artifact-volume", "postgres", "model", "api"}
	out := make([]provider.Resource, 0, len(roles))
	for _, role := range roles {
		kind := "container"
		if role == "network" {
			kind = "network"
		}
		if role == "database-volume" || role == "artifact-volume" {
			kind = "volume"
		}
		out = append(out, provider.Resource{Role: role, Kind: kind, Name: fmt.Sprintf("mc-%s-%s-%s", owner[:8], r.Document.DeploymentID, role), InstallationID: owner, DeploymentID: r.Document.DeploymentID, Fingerprint: r.Document.Spec.Fingerprint(), Generation: r.Generation, Workload: r.Document.Spec})
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
	resources := Resources(c.Store.InstallationID, r)
	o, e := c.observe(ctx, resources)
	status := r.Status
	if status.Resources == nil {
		status.Resources = map[string]state.Binding{}
	}
	status.ObservedGeneration = r.Generation
	status.Conditions = c.conditions(r.Generation, "False", "Reconciling")
	if e != nil {
		status.Phase = "Degraded"
		status.Conditions = c.conditions(r.Generation, "Unknown", "EngineObservationUnavailable")
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
					break
				}
			}
		}
		if target == nil && status.Phase == "Provisioning" {
			status.Phase = "Ready"
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
		return c.Store.SetStatus(r.Generation, status)
	}
	actual, exists := observed.Resources[target.Role]
	confirmed := (action == "ensure" && exists && provider.Compatible(*target, actual)) || (action == "start" && exists && actual.Running) || (action == "stop" && exists && !actual.Running) || (action == "remove" && !exists)
	if confirmed {
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
		}
		status.Conditions = c.conditions(r.Generation, "False", reason)
	}
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
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (c *Controller) conditions(g int64, value, reason string) []state.Condition {
	out := []state.Condition{}
	for _, kind := range []string{"ResourcesReady", "DependenciesReady", "ApplicationReady", "Progressing"} {
		out = append(out, state.Condition{Type: kind, Status: value, Reason: reason, Generation: g, TransitionTime: c.now()})
	}
	return out
}
