package hosted

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/artifacts"
)

type Worker struct {
	Config          Config
	Store           Store
	Pending         *PendingInputs
	ProviderFactory ProviderFactory
	ID              string
}

func NewWorker(config Config, store Store, pending *PendingInputs, providerFactory ProviderFactory) *Worker {
	if pending == nil {
		pending = NewPendingInputs()
	}
	if providerFactory == nil {
		providerFactory = DefaultProviderFactory
	}
	return &Worker{
		Config:          config,
		Store:           store,
		Pending:         pending,
		ProviderFactory: providerFactory,
		ID:              "worker-" + newID(),
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.Config.WorkerPoll)
	defer ticker.Stop()
	for {
		if _, err := w.ProcessOne(ctx); err != nil {
			// Keep the worker alive. Errors are recorded on the run when possible.
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	run, ok, err := w.Store.ClaimNextRun(ctx, w.ID, time.Now().UTC().Add(w.Config.RunTimeout))
	if err != nil || !ok {
		return ok, err
	}

	runCtx, cancel := context.WithTimeout(ctx, w.Config.RunTimeout)
	defer cancel()

	if err := w.Store.AppendEvent(runCtx, run.ID, EventStarted, "worker started run", time.Now().UTC()); err != nil {
		return true, err
	}

	done := make(chan error, 1)
	var result artifacts.BundleResult
	var processedPending bool
	go func() {
		var err error
		var prepared PreparedRun
		casePath := run.CasePath
		if pendingInput, ok := w.Pending.Take(run.ID); ok {
			processedPending = true
			prepared, err = PrepareRunInput(runCtx, w.Config, w.ProviderFactory, run, pendingInput)
			if err != nil {
				done <- err
				return
			}
			casePath = prepared.CasePath
		}
		result, err = artifacts.WriteBundle(artifacts.BundleOptions{
			Root:     w.Config.Root,
			CasePath: casePath,
			OutDir:   run.ArtifactDir,
			Mode:     "hosted",
		})
		if err == nil && processedPending {
			err = writeOptionalArtifacts(result.OutDir, prepared)
		}
		done <- err
	}()

	select {
	case <-runCtx.Done():
		now := time.Now().UTC()
		message := "run timed out"
		_ = w.Store.FailRun(context.Background(), run.ID, message, now)
		_ = w.Store.AppendEvent(context.Background(), run.ID, EventFailed, message, now)
		return true, runCtx.Err()
	case err := <-done:
		now := time.Now().UTC()
		if err != nil {
			_ = w.Store.FailRun(ctx, run.ID, err.Error(), now)
			_ = w.Store.AppendEvent(ctx, run.ID, EventFailed, err.Error(), now)
			return true, err
		}
		if processedPending {
			if err := w.Store.AppendEvent(ctx, run.ID, EventPlanNormalized, "meal plan normalized", now); err != nil {
				return true, err
			}
		}
		if err := w.Store.AppendEvent(ctx, run.ID, EventArtifactWritten, "artifact bundle written", now); err != nil {
			return true, err
		}
		if err := w.Store.CompleteRun(ctx, run.ID, result.Decision, now); err != nil {
			return true, err
		}
		if err := w.Store.AppendEvent(ctx, run.ID, EventCompleted, result.Decision.Decision, now); err != nil {
			return true, err
		}
		return true, nil
	}
}

type CleanupJob struct {
	Config  Config
	Store   Store
	Pending *PendingInputs
}

func (j CleanupJob) Run(ctx context.Context) {
	ticker := time.NewTicker(j.Config.CleanupInterval)
	defer ticker.Stop()
	for {
		_ = j.RunOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (j CleanupJob) RunOnce(ctx context.Context) error {
	runs, err := j.Store.ExpiredRuns(ctx, time.Now().UTC(), 50)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.ArtifactDir == "" {
			continue
		}
		if err := removeArtifactDir(j.Config.ArtifactDir, run.ArtifactDir); err != nil {
			return err
		}
		if j.Pending != nil {
			j.Pending.Delete(run.ID)
		}
		_ = j.Store.AppendEvent(ctx, run.ID, "expired", "expired run artifacts deleted", time.Now().UTC())
	}
	return nil
}

func newRun(config Config, casePath string) Run {
	now := time.Now().UTC()
	id := "run_" + newID()
	return Run{
		ID:          id,
		CasePath:    casePath,
		Status:      StatusQueued,
		ArtifactDir: filepath.Join(config.ArtifactDir, id),
		CreatedAt:   now,
		UpdatedAt:   now,
		ExpiresAt:   now.Add(config.Retention),
	}
}

func newID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func cleanCasePath(root, casePath string) (string, error) {
	if casePath == "" {
		return "", fmt.Errorf("case_path is required")
	}
	if filepath.IsAbs(casePath) {
		return "", fmt.Errorf("case_path must be relative")
	}
	cleaned := filepath.Clean(casePath)
	if cleaned == "." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return "", fmt.Errorf("case_path must stay inside the repository")
	}
	if !strings.HasPrefix(cleaned, "examples"+string(filepath.Separator)) {
		return "", fmt.Errorf("case_path must reference a checked-in example for Milestone 4")
	}
	if _, err := os.Stat(filepath.Join(root, cleaned)); err != nil {
		return "", err
	}
	return cleaned, nil
}

func removeArtifactDir(root, target string) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if target == root || target == string(filepath.Separator) {
		return fmt.Errorf("refusing to delete artifact root")
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return fmt.Errorf("artifact directory escapes artifact root")
	}
	return os.RemoveAll(target)
}
