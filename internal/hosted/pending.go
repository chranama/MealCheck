package hosted

import (
	"sync"
	"time"
)

type PendingInputs struct {
	mu     sync.Mutex
	inputs map[string]pendingInputEntry
}

type pendingInputEntry struct {
	Input     PendingRunInput
	CreatedAt time.Time
	ExpiresAt time.Time
}

func NewPendingInputs() *PendingInputs {
	return &PendingInputs{inputs: map[string]pendingInputEntry{}}
}

func (p *PendingInputs) Put(runID string, input PendingRunInput, expiresAt time.Time) {
	if p == nil {
		return
	}
	now := time.Now().UTC()
	if expiresAt.IsZero() {
		expiresAt = now.Add(15 * time.Minute)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inputs[runID] = pendingInputEntry{
		Input:     input,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}
}

func (p *PendingInputs) Take(runID string) (PendingRunInput, bool) {
	if p == nil {
		return PendingRunInput{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.inputs[runID]
	if ok {
		delete(p.inputs, runID)
	}
	if !ok {
		return PendingRunInput{}, false
	}
	if !entry.ExpiresAt.IsZero() && !time.Now().UTC().Before(entry.ExpiresAt) {
		return PendingRunInput{}, false
	}
	return entry.Input, true
}

func (p *PendingInputs) Delete(runID string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.inputs, runID)
}

func (p *PendingInputs) DeleteExpired(now time.Time) int {
	if p == nil {
		return 0
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	deleted := 0
	for runID, entry := range p.inputs {
		if !entry.ExpiresAt.IsZero() && !now.Before(entry.ExpiresAt) {
			delete(p.inputs, runID)
			deleted++
		}
	}
	return deleted
}

func (p *PendingInputs) Count() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.inputs)
}
