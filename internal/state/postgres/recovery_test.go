package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/chranama/MealCheck/internal/state"
)

// Run against a dedicated disposable database only: this test clears its runs.
func TestExpiredLeaseFailsWithoutReplayingInput(t *testing.T) {
	url := os.Getenv("MEALCHECK_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("dedicated MEALCHECK_TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.ExecContext(ctx, "truncate run_events, runs cascade"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, id := range []string{"abandoned", "next"} {
		err = s.CreateRun(ctx, state.Run{ID: id, CasePath: "/synthetic/case.json", InputMode: state.InputModeLocalModel, Status: state.StatusQueued, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Hour)}, 3, "")
		if err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Millisecond)
	}
	run, ok, err := s.ClaimNextRun(ctx, "first", time.Now().Add(time.Hour))
	if err != nil || !ok || run.ID != "abandoned" {
		t.Fatalf("first claim: %v %v %v", run, ok, err)
	}
	// A healthy lease must keep the serialized model queue blocked.
	_, ok, err = s.ClaimNextRun(ctx, "other", time.Now().Add(time.Hour))
	if err != nil || ok {
		t.Fatalf("active lease bypassed: %v %v", ok, err)
	}
	if _, err = s.db.ExecContext(ctx, "update runs set lease_expires_at=$1 where id='abandoned'", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	run, ok, err = s.ClaimNextRun(ctx, "replacement", time.Now().Add(time.Hour))
	if err != nil || !ok || run.ID != "next" {
		t.Fatalf("queue remained blocked: %v %v %v", run, ok, err)
	}
	abandoned, err := s.GetRun(ctx, "abandoned")
	if err != nil || abandoned.Status != state.StatusFailed {
		t.Fatalf("abandoned work replayed: %v %v", abandoned, err)
	}
	events, err := s.ListEvents(ctx, "abandoned", 0)
	if err != nil || len(events) != 1 || events[0].Type != "failed" {
		t.Fatalf("recovery diagnostic: %v %v", events, err)
	}
	// No queued rows: the recovery transaction must still commit.
	if _, err = s.db.ExecContext(ctx, "update runs set lease_expires_at=$1 where id='next'", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	_, ok, err = s.ClaimNextRun(ctx, "replacement", time.Now().Add(time.Hour))
	if err != nil || ok {
		t.Fatalf("unexpected claim: %v %v", ok, err)
	}
	next, err := s.GetRun(ctx, "next")
	if err != nil || next.Status != state.StatusFailed {
		t.Fatalf("empty-queue recovery not committed: %v %v", next, err)
	}
}
