// Package provider separates engine mechanics from reconciliation policy.
package provider

import (
	"context"
	"github.com/chranama/MealCheck/internal/infra/spec"
)

type Resource struct {
	Role, Kind, Name, InstallationID, DeploymentID, Fingerprint string
	Generation                                                  int64
	DependsOn                                                   []string
	Workload                                                    spec.Workload
}
type Observed struct {
	ID, Name, Role, Owner, DeploymentID, Fingerprint string
	Running, Ready                                   bool
}
type Observation struct{ Resources map[string]Observed }
type Provider interface {
	Observe(context.Context, []Resource) (Observation, error)
	Ensure(context.Context, Resource) error
	Start(context.Context, Resource) error
	Stop(context.Context, Resource) error
	Remove(context.Context, Resource) error
}
type Error struct {
	Reason    string
	Transient bool
}

func (e *Error) Error() string { return e.Reason }
func Compatible(r Resource, o Observed) bool {
	return o.Owner == r.InstallationID && o.DeploymentID == r.DeploymentID && o.Role == r.Role && o.Fingerprint == r.Fingerprint
}
