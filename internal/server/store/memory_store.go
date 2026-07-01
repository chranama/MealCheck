package store

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

type MemoryStore struct {
	mu          sync.Mutex
	runs        map[string]Run
	events      map[string][]RunEvent
	invites     map[string]InviteToken
	nextEventID int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		runs:    map[string]Run{},
		events:  map[string][]RunEvent{},
		invites: map[string]InviteToken{},
	}
}

func (s *MemoryStore) CreateRun(_ context.Context, run Run, queueSize int, inviteTokenID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.runs[run.ID]; exists {
		return ErrConflict
	}
	queued := 0
	for _, existing := range s.runs {
		if existing.Status == StatusQueued {
			queued++
		}
	}
	if queued >= queueSize {
		return ErrQueueFull
	}
	if inviteTokenID != "" {
		invite, ok := s.invites[inviteTokenID]
		if !ok {
			return ErrInviteUnavailable
		}
		if invite.RevokedAt != nil {
			return ErrInviteUnavailable
		}
		if invite.ExpiresAt != nil && !invite.ExpiresAt.After(run.CreatedAt) {
			return ErrInviteUnavailable
		}
		if invite.MaxRuns != nil && invite.UsedRuns >= *invite.MaxRuns {
			return ErrInviteRunLimit
		}
		invite.UsedRuns++
		usedAt := run.CreatedAt
		invite.LastUsedAt = &usedAt
		s.invites[inviteTokenID] = invite
	}
	s.runs[run.ID] = run
	return nil
}

func (s *MemoryStore) GetRun(_ context.Context, id string) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok || run.Status == StatusDeleted {
		return Run{}, ErrNotFound
	}
	return run, nil
}

func (s *MemoryStore) ClaimNextRun(_ context.Context, _ string, _ time.Time) (Run, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var queued []Run
	localModelRunning := false
	for _, run := range s.runs {
		if run.Status == StatusRunning && run.InputMode == InputModeLocalModel {
			localModelRunning = true
			break
		}
	}
	for _, run := range s.runs {
		if run.Status == StatusQueued && (!localModelRunning || run.InputMode != InputModeLocalModel) {
			queued = append(queued, run)
		}
	}
	if len(queued) == 0 {
		return Run{}, false, nil
	}
	sort.Slice(queued, func(i, j int) bool { return queued[i].CreatedAt.Before(queued[j].CreatedAt) })
	run := queued[0]
	now := time.Now().UTC()
	run.Status = StatusRunning
	run.UpdatedAt = now
	run.StartedAt = &now
	s.runs[run.ID] = run
	return run, true, nil
}

func (s *MemoryStore) MarkRunAwaitingReview(_ context.Context, id string, summary string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return ErrNotFound
	}
	run.Status = StatusAwaitingReview
	run.Summary = summary
	run.UpdatedAt = at
	s.runs[id] = run
	return nil
}

func (s *MemoryStore) StartReviewRun(_ context.Context, id string, _ string, _ time.Time, at time.Time) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return Run{}, ErrNotFound
	}
	if run.Status != StatusAwaitingReview {
		return Run{}, ErrConflict
	}
	run.Status = StatusRunning
	run.UpdatedAt = at
	if run.StartedAt == nil {
		startedAt := at
		run.StartedAt = &startedAt
	}
	s.runs[id] = run
	return run, nil
}

func (s *MemoryStore) CompleteRun(_ context.Context, id string, decision checker.DecisionDocument, completedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return ErrNotFound
	}
	run.Status = StatusCompleted
	run.Decision = decision.Decision
	run.RiskLevel = decision.RiskLevel
	run.Summary = decision.Summary
	run.UpdatedAt = completedAt
	run.CompletedAt = &completedAt
	s.runs[id] = run
	return nil
}

func (s *MemoryStore) FailRun(_ context.Context, id string, message string, completedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return ErrNotFound
	}
	run.Status = StatusFailed
	run.Error = message
	run.UpdatedAt = completedAt
	run.CompletedAt = &completedAt
	s.runs[id] = run
	return nil
}

func (s *MemoryStore) DeleteRun(_ context.Context, id string) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return Run{}, ErrNotFound
	}
	run.Status = StatusDeleted
	run.UpdatedAt = time.Now().UTC()
	s.runs[id] = run
	return run, nil
}

func (s *MemoryStore) ExpiredRuns(_ context.Context, now time.Time, limit int) ([]Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var expired []Run
	for _, run := range s.runs {
		if len(expired) >= limit {
			break
		}
		if run.Status != StatusDeleted && !run.ExpiresAt.After(now) {
			run.Status = StatusDeleted
			run.UpdatedAt = now
			s.runs[run.ID] = run
			expired = append(expired, run)
		}
	}
	return expired, nil
}

func (s *MemoryStore) AppendEvent(_ context.Context, runID, eventType, message string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextEventID++
	event := RunEvent{
		ID:        s.nextEventID,
		RunID:     runID,
		Type:      eventType,
		Message:   message,
		CreatedAt: at,
	}
	s.events[runID] = append(s.events[runID], event)
	return nil
}

func (s *MemoryStore) ListEvents(_ context.Context, runID string, afterID int64) ([]RunEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var events []RunEvent
	for _, event := range s.events[runID] {
		if event.ID > afterID {
			events = append(events, event)
		}
	}
	return events, nil
}

func (s *MemoryStore) Stats(_ context.Context) (StoreStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var stats StoreStats
	for _, run := range s.runs {
		switch run.Status {
		case StatusQueued:
			stats.Queued++
		case StatusRunning:
			stats.Running++
		}
	}
	return stats, nil
}

func (s *MemoryStore) CreateInviteToken(_ context.Context, invite InviteToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.invites[invite.ID]; exists {
		return ErrConflict
	}
	s.invites[invite.ID] = invite
	return nil
}

func (s *MemoryStore) GetInviteToken(_ context.Context, id string) (InviteToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	invite, ok := s.invites[id]
	if !ok {
		return InviteToken{}, ErrNotFound
	}
	return invite, nil
}

func (s *MemoryStore) ListInviteTokens(_ context.Context) ([]InviteToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	invites := make([]InviteToken, 0, len(s.invites))
	for _, invite := range s.invites {
		invites = append(invites, invite)
	}
	sort.Slice(invites, func(i, j int) bool { return invites[i].CreatedAt.Before(invites[j].CreatedAt) })
	return invites, nil
}

func (s *MemoryStore) RevokeInviteToken(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	invite, ok := s.invites[id]
	if !ok {
		return ErrNotFound
	}
	revokedAt := at.UTC()
	invite.RevokedAt = &revokedAt
	s.invites[id] = invite
	return nil
}

func (s *MemoryStore) Close() error {
	return nil
}
