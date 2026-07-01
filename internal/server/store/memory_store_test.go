package store

import (
	"context"
	"testing"
	"time"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

func TestMemoryStoreClaimBlocksSecondLocalModelRun(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	first := testRun("run_local_1", InputModeLocalModel, now)
	second := testRun("run_local_2", InputModeLocalModel, now.Add(time.Second))
	if err := store.CreateRun(ctx, first, 10, ""); err != nil {
		t.Fatalf("create first run: %v", err)
	}
	if err := store.CreateRun(ctx, second, 10, ""); err != nil {
		t.Fatalf("create second run: %v", err)
	}

	claimed, ok, err := store.ClaimNextRun(ctx, "worker-1", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("claim first run: %v", err)
	}
	if !ok || claimed.ID != first.ID {
		t.Fatalf("first claim = %s ok=%t, want %s", claimed.ID, ok, first.ID)
	}

	blocked, ok, err := store.ClaimNextRun(ctx, "worker-2", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("claim blocked run: %v", err)
	}
	if ok {
		t.Fatalf("second local-model claim = %s, want no claim while %s is running", blocked.ID, first.ID)
	}

	if err := store.CompleteRun(ctx, first.ID, checker.DecisionDocument{Decision: "pass"}, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("complete first run: %v", err)
	}
	claimed, ok, err = store.ClaimNextRun(ctx, "worker-3", now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("claim released run: %v", err)
	}
	if !ok || claimed.ID != second.ID {
		t.Fatalf("released claim = %s ok=%t, want %s", claimed.ID, ok, second.ID)
	}
}

func TestMemoryStoreClaimAllowsNonLocalRunWhileLocalModelRuns(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	localRunning := testRun("run_local_1", InputModeLocalModel, now)
	localQueued := testRun("run_local_2", InputModeLocalModel, now.Add(time.Second))
	byokQueued := testRun("run_byok", "prompt_generation", now.Add(2*time.Second))
	for _, run := range []Run{localRunning, localQueued, byokQueued} {
		if err := store.CreateRun(ctx, run, 10, ""); err != nil {
			t.Fatalf("create %s: %v", run.ID, err)
		}
	}

	claimed, ok, err := store.ClaimNextRun(ctx, "worker-1", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("claim local run: %v", err)
	}
	if !ok || claimed.ID != localRunning.ID {
		t.Fatalf("first claim = %s ok=%t, want %s", claimed.ID, ok, localRunning.ID)
	}

	claimed, ok, err = store.ClaimNextRun(ctx, "worker-2", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("claim non-local run: %v", err)
	}
	if !ok || claimed.ID != byokQueued.ID {
		t.Fatalf("second claim = %s ok=%t, want non-local run %s", claimed.ID, ok, byokQueued.ID)
	}

	local, err := store.GetRun(ctx, localQueued.ID)
	if err != nil {
		t.Fatalf("get queued local run: %v", err)
	}
	if local.Status != StatusQueued {
		t.Fatalf("queued local status = %q, want queued", local.Status)
	}
}

func testRun(id, inputMode string, createdAt time.Time) Run {
	return Run{
		ID:          id,
		CasePath:    "examples/seeded-one-day-peanut-allergy/case.json",
		InputMode:   inputMode,
		Status:      StatusQueued,
		ArtifactDir: "/tmp/" + id,
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
		ExpiresAt:   createdAt.Add(time.Hour),
	}
}
