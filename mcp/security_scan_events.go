package mcp

import (
	"context"
	"time"
)

// SecurityScanEvent is a live, read-only update from a ClamAV scan.
type SecurityScanEvent struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"`
	Path    string    `json:"path,omitempty"`
	Message string    `json:"message,omitempty"`
	At      time.Time `json:"at"`
}

type securityScanObserverKey struct{}

// WithSecurityScanObserver attaches a callback for live scan updates to a request context.
func WithSecurityScanObserver(ctx context.Context, observer func(SecurityScanEvent)) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, securityScanObserverKey{}, observer)
}

func securityScanObserver(ctx context.Context) func(SecurityScanEvent) {
	if ctx == nil {
		return nil
	}
	observer, _ := ctx.Value(securityScanObserverKey{}).(func(SecurityScanEvent))
	return observer
}
