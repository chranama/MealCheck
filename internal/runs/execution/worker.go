// Package execution owns background run processing and retention jobs.
package execution

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/llm/inference"
	"github.com/chranama/MealCheck/internal/runs/runinput"
	"github.com/chranama/MealCheck/internal/state"
	"github.com/chranama/MealCheck/internal/workflow/artifacts"
	"github.com/chranama/MealCheck/internal/workflow/normalize"
)

type Worker struct {
	Config           core.Config
	Store            state.Store
	Inputs           *runinput.Vault
	CompleterFactory inference.CompleterFactory
	ID               string
}

func NewWorker(config core.Config, store state.Store, inputs *runinput.Vault, completerFactory inference.CompleterFactory) *Worker {
	if inputs == nil {
		inputs = runinput.New()
	}
	return &Worker{
		Config: config, Store: store, Inputs: inputs, CompleterFactory: completerFactory,
		ID: "worker-" + newID(),
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.Config.WorkerPoll)
	defer ticker.Stop()
	for {
		if _, err := w.ProcessOne(ctx); err != nil {
			// Errors are recorded on the run when possible; the worker stays alive.
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
	if err := w.Store.AppendEvent(runCtx, run.ID, core.EventStarted, "worker started run", time.Now().UTC()); err != nil {
		return true, err
	}

	done := make(chan workerProcessResult, 1)
	go func() {
		var prepared normalize.PreparedRun
		var result artifacts.BundleResult
		var err error
		var processedInput bool
		casePath := run.CasePath
		if input, ok := w.Inputs.Take(run.ID); ok {
			processedInput = true
			prepared, err = normalize.PrepareRunInput(runCtx, w.Config, w.CompleterFactory, run, input)
			if err != nil {
				done <- workerProcessResult{err: err}
				return
			}
			casePath = prepared.CasePath
			if input.Mode == core.InputModeLocalModel {
				if err := normalize.WriteReviewArtifacts(run.ArtifactDir, run.ID, prepared); err != nil {
					done <- workerProcessResult{err: err}
					return
				}
				done <- workerProcessResult{processedInput: true, awaitingReview: true}
				return
			}
		} else if run.CasePath == normalize.RuntimeCasePath(w.Config, run.ID) {
			done <- workerProcessResult{err: fmt.Errorf("pending BYOK run input expired before processing; resubmit with a fresh provider API key")}
			return
		}

		result, err = artifacts.WriteBundle(artifacts.BundleOptions{
			Root: w.Config.Root, CasePath: casePath, OutDir: run.ArtifactDir, Mode: "hosted",
			FNDDSFallbackPath: w.Config.FNDDSFallbackPath,
		})
		if err == nil && processedInput {
			err = normalize.WriteOptionalArtifacts(result.OutDir, prepared)
		}
		done <- workerProcessResult{result: result, processedInput: processedInput, err: err}
	}()

	select {
	case <-runCtx.Done():
		now := time.Now().UTC()
		message := "run timed out"
		_ = w.Store.FailRun(context.Background(), run.ID, message, now)
		_ = w.Store.AppendEvent(context.Background(), run.ID, core.EventFailed, message, now)
		return true, runCtx.Err()
	case outcome := <-done:
		now := time.Now().UTC()
		if outcome.err != nil {
			_ = w.Store.FailRun(ctx, run.ID, outcome.err.Error(), now)
			_ = w.Store.AppendEvent(ctx, run.ID, core.EventFailed, outcome.err.Error(), now)
			return true, outcome.err
		}
		if outcome.processedInput {
			if err := w.Store.AppendEvent(ctx, run.ID, core.EventPlanNormalized, "meal plan normalized", now); err != nil {
				return true, err
			}
		}
		if outcome.awaitingReview {
			if err := w.Store.MarkRunAwaitingReview(ctx, run.ID, "MealCheck normalized this plan for source-linked review.", now); err != nil {
				return true, err
			}
			if err := w.Store.AppendEvent(ctx, run.ID, core.EventReviewReady, "normalized plan ready for review", now); err != nil {
				return true, err
			}
			return true, nil
		}
		if err := w.Store.AppendEvent(ctx, run.ID, core.EventArtifactWritten, "artifact bundle written", now); err != nil {
			return true, err
		}
		if err := w.Store.CompleteRun(ctx, run.ID, outcome.result.Decision, now); err != nil {
			return true, err
		}
		if err := w.Store.AppendEvent(ctx, run.ID, core.EventCompleted, outcome.result.Decision.Decision, now); err != nil {
			return true, err
		}
		return true, nil
	}
}

type workerProcessResult struct {
	result         artifacts.BundleResult
	processedInput bool
	awaitingReview bool
	err            error
}

type CleanupJob struct {
	Config core.Config
	Store  state.Store
	Inputs *runinput.Vault
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
	if j.Inputs != nil {
		j.Inputs.DeleteExpired(time.Now().UTC())
	}
	runs, err := j.Store.ExpiredRuns(ctx, time.Now().UTC(), 50)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.ArtifactDir == "" {
			continue
		}
		if err := RemoveArtifactDir(j.Config.ArtifactDir, run.ArtifactDir); err != nil {
			return err
		}
		if j.Inputs != nil {
			j.Inputs.Delete(run.ID)
		}
		_ = j.Store.AppendEvent(ctx, run.ID, "expired", "expired run artifacts deleted", time.Now().UTC())
	}
	return nil
}

func RemoveArtifactDir(root, target string) error {
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

func newID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
