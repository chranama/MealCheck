// Package runinput stores sensitive, non-persistent input while a run waits
// for a worker. Values are removed when taken or when their TTL expires.
package runinput

import (
	"sync"
	"time"

	"github.com/chranama/MealCheck/internal/core"
)

type Vault struct {
	mu     sync.Mutex
	inputs map[string]pendingInputEntry
}

type pendingInputEntry struct {
	Input     core.PendingRunInput
	CreatedAt time.Time
	ExpiresAt time.Time
}

func New() *Vault {
	return &Vault{inputs: map[string]pendingInputEntry{}}
}

func (p *Vault) Put(runID string, input core.PendingRunInput, expiresAt time.Time) {
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

func (p *Vault) Take(runID string) (core.PendingRunInput, bool) {
	if p == nil {
		return core.PendingRunInput{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.inputs[runID]
	if ok {
		delete(p.inputs, runID)
	}
	if !ok {
		return core.PendingRunInput{}, false
	}
	if !entry.ExpiresAt.IsZero() && !time.Now().UTC().Before(entry.ExpiresAt) {
		return core.PendingRunInput{}, false
	}
	return entry.Input, true
}

func (p *Vault) Delete(runID string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.inputs, runID)
}

func (p *Vault) DeleteExpired(now time.Time) int {
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

func (p *Vault) Count() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.inputs)
}
