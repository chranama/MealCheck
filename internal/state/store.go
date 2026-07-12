// Package state defines MealCheck's hosted operational-state contract.
package state

import (
	"context"
	"errors"
	"time"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

var (
	ErrQueueFull         = errors.New("queue full")
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("conflict")
	ErrInviteUnavailable = errors.New("invite token unavailable")
	ErrInviteRunLimit    = errors.New("invite token run limit reached")
)

type Store interface {
	CreateRun(ctx context.Context, run Run, queueSize int, inviteTokenID string) error
	GetRun(ctx context.Context, id string) (Run, error)
	ClaimNextRun(ctx context.Context, workerID string, leaseUntil time.Time) (Run, bool, error)
	MarkRunAwaitingReview(ctx context.Context, id string, summary string, at time.Time) error
	StartReviewRun(ctx context.Context, id string, workerID string, leaseUntil time.Time, at time.Time) (Run, error)
	CompleteRun(ctx context.Context, id string, decision checker.DecisionDocument, completedAt time.Time) error
	FailRun(ctx context.Context, id string, message string, completedAt time.Time) error
	DeleteRun(ctx context.Context, id string) (Run, error)
	ExpiredRuns(ctx context.Context, now time.Time, limit int) ([]Run, error)
	AppendEvent(ctx context.Context, runID, eventType, message string, at time.Time) error
	ListEvents(ctx context.Context, runID string, afterID int64) ([]RunEvent, error)
	Stats(ctx context.Context) (StoreStats, error)
	CreateInviteToken(ctx context.Context, invite InviteToken) error
	GetInviteToken(ctx context.Context, id string) (InviteToken, error)
	ListInviteTokens(ctx context.Context) ([]InviteToken, error)
	RevokeInviteToken(ctx context.Context, id string, at time.Time) error
	Close() error
}

type StoreStats struct {
	Queued  int `json:"queued"`
	Running int `json:"running"`
}
