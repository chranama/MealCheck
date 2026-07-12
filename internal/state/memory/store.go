package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/chranama/MealCheck/internal/state"
	"github.com/chranama/MealCheck/internal/workflow/checker"
)

type Store struct {
	mu          sync.Mutex
	runs        map[string]state.Run
	events      map[string][]state.RunEvent
	invites     map[string]state.InviteToken
	nextEventID int64
}

var _ state.Store = (*Store)(nil)

func New() *Store {
	return &Store{
		runs:    map[string]state.Run{},
		events:  map[string][]state.RunEvent{},
		invites: map[string]state.InviteToken{},
	}
}

func (s *Store) CreateRun(_ context.Context, run state.Run, queueSize int, inviteTokenID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.runs[run.ID]; exists {
		return state.ErrConflict
	}
	queued := 0
	for _, existing := range s.runs {
		if existing.Status == state.StatusQueued {
			queued++
		}
	}
	if queued >= queueSize {
		return state.ErrQueueFull
	}
	if inviteTokenID != "" {
		invite, ok := s.invites[inviteTokenID]
		if !ok {
			return state.ErrInviteUnavailable
		}
		if invite.RevokedAt != nil {
			return state.ErrInviteUnavailable
		}
		if invite.ExpiresAt != nil && !invite.ExpiresAt.After(run.CreatedAt) {
			return state.ErrInviteUnavailable
		}
		if invite.MaxRuns != nil && invite.UsedRuns >= *invite.MaxRuns {
			return state.ErrInviteRunLimit
		}
		invite.UsedRuns++
		usedAt := run.CreatedAt
		invite.LastUsedAt = &usedAt
		s.invites[inviteTokenID] = invite
	}
	s.runs[run.ID] = run
	return nil
}

func (s *Store) GetRun(_ context.Context, id string) (state.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok || run.Status == state.StatusDeleted {
		return state.Run{}, state.ErrNotFound
	}
	return run, nil
}

func (s *Store) ClaimNextRun(_ context.Context, _ string, _ time.Time) (state.Run, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var queued []state.Run
	localModelRunning := false
	for _, run := range s.runs {
		if run.Status == state.StatusRunning && run.InputMode == state.InputModeLocalModel {
			localModelRunning = true
			break
		}
	}
	for _, run := range s.runs {
		if run.Status == state.StatusQueued && (!localModelRunning || run.InputMode != state.InputModeLocalModel) {
			queued = append(queued, run)
		}
	}
	if len(queued) == 0 {
		return state.Run{}, false, nil
	}
	sort.Slice(queued, func(i, j int) bool { return queued[i].CreatedAt.Before(queued[j].CreatedAt) })
	run := queued[0]
	now := time.Now().UTC()
	run.Status = state.StatusRunning
	run.UpdatedAt = now
	run.StartedAt = &now
	s.runs[run.ID] = run
	return run, true, nil
}

func (s *Store) MarkRunAwaitingReview(_ context.Context, id string, summary string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return state.ErrNotFound
	}
	run.Status = state.StatusAwaitingReview
	run.Summary = summary
	run.UpdatedAt = at
	s.runs[id] = run
	return nil
}

func (s *Store) StartReviewRun(_ context.Context, id string, _ string, _ time.Time, at time.Time) (state.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return state.Run{}, state.ErrNotFound
	}
	if run.Status != state.StatusAwaitingReview {
		return state.Run{}, state.ErrConflict
	}
	run.Status = state.StatusRunning
	run.UpdatedAt = at
	if run.StartedAt == nil {
		startedAt := at
		run.StartedAt = &startedAt
	}
	s.runs[id] = run
	return run, nil
}

func (s *Store) CompleteRun(_ context.Context, id string, decision checker.DecisionDocument, completedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return state.ErrNotFound
	}
	run.Status = state.StatusCompleted
	run.Decision = decision.Decision
	run.RiskLevel = decision.RiskLevel
	run.Summary = decision.Summary
	run.UpdatedAt = completedAt
	run.CompletedAt = &completedAt
	s.runs[id] = run
	return nil
}

func (s *Store) FailRun(_ context.Context, id string, message string, completedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return state.ErrNotFound
	}
	run.Status = state.StatusFailed
	run.Error = message
	run.UpdatedAt = completedAt
	run.CompletedAt = &completedAt
	s.runs[id] = run
	return nil
}

func (s *Store) DeleteRun(_ context.Context, id string) (state.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return state.Run{}, state.ErrNotFound
	}
	run.Status = state.StatusDeleted
	run.UpdatedAt = time.Now().UTC()
	s.runs[id] = run
	return run, nil
}

func (s *Store) ExpiredRuns(_ context.Context, now time.Time, limit int) ([]state.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var expired []state.Run
	for _, run := range s.runs {
		if len(expired) >= limit {
			break
		}
		if run.Status != state.StatusDeleted && !run.ExpiresAt.After(now) {
			run.Status = state.StatusDeleted
			run.UpdatedAt = now
			s.runs[run.ID] = run
			expired = append(expired, run)
		}
	}
	return expired, nil
}

func (s *Store) AppendEvent(_ context.Context, runID, eventType, message string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextEventID++
	event := state.RunEvent{
		ID:        s.nextEventID,
		RunID:     runID,
		Type:      eventType,
		Message:   message,
		CreatedAt: at,
	}
	s.events[runID] = append(s.events[runID], event)
	return nil
}

func (s *Store) ListEvents(_ context.Context, runID string, afterID int64) ([]state.RunEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var events []state.RunEvent
	for _, event := range s.events[runID] {
		if event.ID > afterID {
			events = append(events, event)
		}
	}
	return events, nil
}

func (s *Store) Stats(_ context.Context) (state.StoreStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var stats state.StoreStats
	for _, run := range s.runs {
		switch run.Status {
		case state.StatusQueued:
			stats.Queued++
		case state.StatusRunning:
			stats.Running++
		}
	}
	return stats, nil
}

func (s *Store) CreateInviteToken(_ context.Context, invite state.InviteToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.invites[invite.ID]; exists {
		return state.ErrConflict
	}
	s.invites[invite.ID] = invite
	return nil
}

func (s *Store) GetInviteToken(_ context.Context, id string) (state.InviteToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	invite, ok := s.invites[id]
	if !ok {
		return state.InviteToken{}, state.ErrNotFound
	}
	return invite, nil
}

func (s *Store) ListInviteTokens(_ context.Context) ([]state.InviteToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	invites := make([]state.InviteToken, 0, len(s.invites))
	for _, invite := range s.invites {
		invites = append(invites, invite)
	}
	sort.Slice(invites, func(i, j int) bool { return invites[i].CreatedAt.Before(invites[j].CreatedAt) })
	return invites, nil
}

func (s *Store) RevokeInviteToken(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	invite, ok := s.invites[id]
	if !ok {
		return state.ErrNotFound
	}
	revokedAt := at.UTC()
	invite.RevokedAt = &revokedAt
	s.invites[id] = invite
	return nil
}

func (s *Store) Close() error {
	return nil
}
