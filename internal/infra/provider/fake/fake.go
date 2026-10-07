package fake

import (
	"context"
	"fmt"
	"github.com/chranama/MealCheck/internal/infra/provider"
	"sync"
)

type Provider struct {
	mu                 sync.Mutex
	Resources          map[string]provider.Observed
	Unavailable        bool
	TimeoutAfterEnsure bool
	Calls              map[string]int
	BeforeMutation     func(provider.Resource, string)
}

func New() *Provider {
	return &Provider{Resources: map[string]provider.Observed{}, Calls: map[string]int{}}
}
func (p *Provider) Observe(ctx context.Context, rs []provider.Resource) (provider.Observation, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return provider.Observation{}, e
	}
	if p.Unavailable {
		return provider.Observation{}, &provider.Error{Reason: "EngineUnavailable", Transient: true}
	}
	out := provider.Observation{Resources: map[string]provider.Observed{}}
	for _, r := range rs {
		if v, ok := p.Resources[r.Name]; ok {
			out.Resources[r.Role] = v
		}
	}
	return out, nil
}
func (p *Provider) mutate(ctx context.Context, r provider.Resource, action string) error {
	if p.BeforeMutation != nil {
		p.BeforeMutation(r, action)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return e
	}
	if p.Unavailable {
		return &provider.Error{Reason: "EngineUnavailable", Transient: true}
	}
	p.Calls[action+":"+r.Role]++
	v, exists := p.Resources[r.Name]
	if exists && !provider.Compatible(r, v) {
		return &provider.Error{Reason: "OwnershipConflict"}
	}
	switch action {
	case "ensure":
		if !exists {
			p.Resources[r.Name] = provider.Observed{ID: fmt.Sprintf("id-%s", r.Name), Name: r.Name, Role: r.Role, Owner: r.InstallationID, DeploymentID: r.DeploymentID, Fingerprint: r.Fingerprint, Ready: r.Kind != "container"}
		}
		if p.TimeoutAfterEnsure {
			return context.DeadlineExceeded
		}
	case "start":
		v.Running = true
		v.Ready = true
		p.Resources[r.Name] = v
	case "stop":
		v.Running = false
		v.Ready = false
		p.Resources[r.Name] = v
	case "remove":
		delete(p.Resources, r.Name)
	}
	return nil
}
func (p *Provider) Ensure(ctx context.Context, r provider.Resource) error {
	return p.mutate(ctx, r, "ensure")
}
func (p *Provider) Start(ctx context.Context, r provider.Resource) error {
	return p.mutate(ctx, r, "start")
}
func (p *Provider) Stop(ctx context.Context, r provider.Resource) error {
	return p.mutate(ctx, r, "stop")
}
func (p *Provider) Remove(ctx context.Context, r provider.Resource) error {
	return p.mutate(ctx, r, "remove")
}
