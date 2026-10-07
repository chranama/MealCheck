package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/chranama/MealCheck/internal/infra/spec"
	"github.com/chranama/MealCheck/internal/infra/state"
)

func checkPlan(store *state.Store, d spec.Document) error {
	old, err := store.Get()
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if old.Document.DeploymentID != d.DeploymentID {
		return errors.New("one deployment per controller")
	}
	if old.Tombstone {
		return errors.New("deployment is terminally deleted")
	}
	if old.Document.Spec != d.Spec {
		return errors.New("workload configuration is immutable")
	}
	return nil
}

// ManifestOptions grants catalog and engine access to the daemon, never to a
// deployment document. Engine reads happen only before accepting a new snapshot.
type ManifestOptions struct {
	Catalog        *spec.Catalog
	EngineInfo     func(context.Context) (spec.EngineInfo, error)
	ValidateImages func(context.Context, spec.Workload) error
}

func resolveRequest(ctx context.Context, store *state.Store, policy spec.Policy, options ManifestOptions, raw []byte) (spec.Document, error) {
	var header struct {
		APIVersion string `json:"apiVersion"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return spec.Document{}, errors.New("invalid manifest JSON")
	}
	if header.APIVersion == spec.Version {
		d, err := spec.Decode(raw)
		if err != nil {
			return d, err
		}
		// Snapshots are produced by resolution. Legacy clients cannot inject them.
		if d.Spec.ResolvedJSON != "" || d.DeploymentJSON != "" {
			return d, errors.New("resolved snapshots cannot be supplied by clients")
		}
		return d, d.Validate(policy)
	}
	d, err := spec.DecodeDeployment(raw)
	if err != nil {
		return spec.Document{}, err
	}
	if d.APIVersion != spec.DeploymentVersion || (d.DesiredState != "Running" && d.DesiredState != "Stopped" && d.DesiredState != "Deleted") {
		return spec.Document{}, errors.New("unsupported deployment version or desiredState")
	}
	// Re-applying the already accepted operator input needs neither the catalog
	// nor the engine. Lifecycle requests continue against the persisted snapshot.
	if old, err := store.Get(); err == nil && old.Document.DeploymentJSON != "" {
		accepted, err := spec.DecodeDeployment([]byte(old.Document.DeploymentJSON))
		if err != nil {
			return spec.Document{}, errors.New("invalid stored deployment document")
		}
		accepted.DesiredState = d.DesiredState
		a, _ := json.Marshal(accepted)
		b, _ := json.Marshal(d)
		if string(a) == string(b) {
			out := old.Document
			out.DesiredState = d.DesiredState
			out.DeploymentJSON = string(b)
			return out, nil
		}
	}
	if options.Catalog == nil || options.EngineInfo == nil || options.ValidateImages == nil {
		return spec.Document{}, errors.New("deployment manifests require a trusted catalog and engine validation")
	}
	system, err := options.Catalog.Load(d.System)
	if err != nil {
		return spec.Document{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	capacity, err := options.EngineInfo(ctx)
	if err != nil {
		return spec.Document{}, err
	}
	out, err := spec.Resolve(d, system, policy, capacity)
	if err != nil {
		return out, err
	}
	if err := options.ValidateImages(ctx, out.Spec); err != nil {
		return out, err
	}
	return out, nil
}

func preview(d spec.Document) (any, error) {
	resolved, err := d.Spec.Resolved()
	if err != nil {
		return nil, err
	}
	if resolved == nil {
		return struct {
			DeploymentID string        `json:"deploymentID"`
			DesiredState string        `json:"desiredState"`
			Legacy       spec.Workload `json:"legacyWorkload"`
		}{d.DeploymentID, d.DesiredState, d.Spec}, nil
	}
	return struct {
		DeploymentID string               `json:"deploymentID"`
		DesiredState string               `json:"desiredState"`
		System       *spec.ResolvedSystem `json:"resolvedSystem"`
	}{d.DeploymentID, d.DesiredState, resolved}, nil
}
