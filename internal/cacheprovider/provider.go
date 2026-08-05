// Package cacheprovider defines cache discovery separately from process adapters.
package cacheprovider

import (
	"context"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
	"github.com/jamesonstone/ghostgc/internal/cachefs"
)

// Session is the minimum durable session fact a cache provider may consume.
type Session struct {
	ID            string
	NativeID      string
	Agent         string
	State         string
	CodexHome     string
	LiveProcesses int
	Confidence    float64
}

// Result is one bounded provider observation.
type Result struct {
	Artifacts []cacheartifact.Artifact
	Complete  bool
	Inspected int
}

// Provider discovers one primary-source-backed cache family.
type Provider interface {
	ID() string
	Observe(ctx context.Context, sessions []Session, filesystem cachefs.Filesystem, maxEntries int) (Result, error)
}
