package execution

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/runs/runinput"
	"github.com/chranama/MealCheck/internal/state/memory"
	"github.com/chranama/MealCheck/internal/workflow/normalize"
)

func TestMissingLocalModelInputFailsWithoutRequestingProviderKey(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	config := core.Config{DataDir: t.TempDir(), RunTimeout: time.Second}
	now := time.Now().UTC()
	id := "interrupted-input"
	err := store.CreateRun(ctx, core.Run{ID: id, CasePath: normalize.RuntimeCasePath(config, id), InputMode: core.InputModeLocalModel, Status: core.StatusQueued, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Hour)}, 3, "")
	if err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(config, store, runinput.New(), nil)
	_, err = worker.ProcessOne(ctx)
	if err == nil {
		t.Fatal("missing volatile input was replayed")
	}
	run, err := store.GetRun(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != core.StatusFailed || !strings.Contains(run.Error, "resubmit") || strings.Contains(run.Error, "API key") {
		t.Fatalf("incorrect recovery: %+v", run)
	}
}
