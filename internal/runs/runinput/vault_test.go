package runinput

import (
	"testing"
	"time"

	"github.com/chranama/MealCheck/internal/core"
)

func TestPendingInputsTakeDeletesAndRejectsExpiredInputs(t *testing.T) {
	pending := New()
	now := time.Now().UTC()

	pending.Put("fresh", core.PendingRunInput{Mode: "profile_generation"}, now.Add(time.Minute))
	input, ok := pending.Take("fresh")
	if !ok {
		t.Fatal("Take fresh input ok = false, want true")
	}
	if input.Mode != "profile_generation" {
		t.Fatalf("input mode = %q, want profile_generation", input.Mode)
	}
	if pending.Count() != 0 {
		t.Fatalf("pending count after fresh take = %d, want 0", pending.Count())
	}

	pending.Put("expired", core.PendingRunInput{Mode: "prompt_generation"}, now.Add(-time.Second))
	if _, ok := pending.Take("expired"); ok {
		t.Fatal("Take expired input ok = true, want false")
	}
	if pending.Count() != 0 {
		t.Fatalf("pending count after expired take = %d, want 0", pending.Count())
	}
}

func TestPendingInputsDeleteExpiredKeepsFreshInputs(t *testing.T) {
	pending := New()
	now := time.Now().UTC()
	pending.Put("fresh", core.PendingRunInput{Mode: "profile_generation"}, now.Add(time.Minute))
	pending.Put("expired", core.PendingRunInput{Mode: "prompt_generation"}, now.Add(-time.Second))

	if deleted := pending.DeleteExpired(now); deleted != 1 {
		t.Fatalf("deleted expired count = %d, want 1", deleted)
	}
	if pending.Count() != 1 {
		t.Fatalf("pending count = %d, want 1", pending.Count())
	}
	input, ok := pending.Take("fresh")
	if !ok {
		t.Fatal("fresh input missing after DeleteExpired")
	}
	if input.Mode != "profile_generation" {
		t.Fatalf("fresh input mode = %q, want profile_generation", input.Mode)
	}
}
