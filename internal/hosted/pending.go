package hosted

import "sync"

type PendingInputs struct {
	mu     sync.Mutex
	inputs map[string]PendingRunInput
}

func NewPendingInputs() *PendingInputs {
	return &PendingInputs{inputs: map[string]PendingRunInput{}}
}

func (p *PendingInputs) Put(runID string, input PendingRunInput) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inputs[runID] = input
}

func (p *PendingInputs) Take(runID string) (PendingRunInput, bool) {
	if p == nil {
		return PendingRunInput{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	input, ok := p.inputs[runID]
	if ok {
		delete(p.inputs, runID)
	}
	return input, ok
}

func (p *PendingInputs) Delete(runID string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.inputs, runID)
}
