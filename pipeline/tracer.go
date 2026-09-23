// tracer.go: append-only step log for the UI accordion.
package pipeline

import (
	"sync"
	"time"
)

// Step is one event in a chat turn.
// At is RFC3339 text so Wails bindings do not need time.Time.
type Step struct {
	At     string `json:"at"`
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
}

// Tracer collects steps for one Run.
type Tracer struct {
	mu    sync.Mutex
	steps []Step
}

// NewTracer returns an empty tracer.
func NewTracer() *Tracer { return &Tracer{} }

// Add appends a step (kind examples: route_decision, model_request, tool_call, tool_result, error).
func (t *Tracer) Add(kind, label, detail string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.steps = append(t.steps, Step{
		At:     time.Now().UTC().Format(time.RFC3339),
		Kind:   kind,
		Label:  label,
		Detail: detail,
	})
}

// Snapshot returns a copy of recorded steps.
func (t *Tracer) Snapshot() []Step {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Step, len(t.steps))
	copy(out, t.steps)
	return out
}
