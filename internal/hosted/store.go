package hosted

import (
	"context"
	"errors"
	"time"

	"github.com/chranama/MealCheck/internal/checker"
)

var (
	ErrQueueFull = errors.New("queue full")
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
)

type Store interface {
	CreateRun(ctx context.Context, run Run, queueSize int) error
	GetRun(ctx context.Context, id string) (Run, error)
	ClaimNextRun(ctx context.Context, workerID string, leaseUntil time.Time) (Run, bool, error)
	CompleteRun(ctx context.Context, id string, decision checker.DecisionDocument, completedAt time.Time) error
	FailRun(ctx context.Context, id string, message string, completedAt time.Time) error
	DeleteRun(ctx context.Context, id string) (Run, error)
	ExpiredRuns(ctx context.Context, now time.Time, limit int) ([]Run, error)
	AppendEvent(ctx context.Context, runID, eventType, message string, at time.Time) error
	ListEvents(ctx context.Context, runID string, afterID int64) ([]RunEvent, error)
	Stats(ctx context.Context) (StoreStats, error)
	Close() error
}

type StoreStats struct {
	Queued  int `json:"queued"`
	Running int `json:"running"`
}
