package main

import (
	"context"
	"strings"
	"testing"

	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/state/memory"
)

func TestOpenStateStoreSelectsMemoryAdapter(t *testing.T) {
	store, err := openStateStore(context.Background(), core.Config{StoreKind: "memory"})
	if err != nil {
		t.Fatalf("openStateStore error: %v", err)
	}
	defer store.Close()

	if _, ok := store.(*memory.Store); !ok {
		t.Fatalf("openStateStore type = %T, want *memory.Store", store)
	}
}

func TestOpenStateStoreRejectsUnsupportedAdapter(t *testing.T) {
	_, err := openStateStore(context.Background(), core.Config{StoreKind: "unknown"})
	if err == nil || !strings.Contains(err.Error(), `unsupported store kind "unknown"`) {
		t.Fatalf("openStateStore error = %v, want unsupported store kind", err)
	}
}
