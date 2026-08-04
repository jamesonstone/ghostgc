// Package cachefs owns the narrow descriptor-anchored cache filesystem boundary.
package cachefs

import (
	"context"
	"errors"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
)

// Entry is one immediate child observed without following links.
type Entry struct {
	Name     string                 `json:"name"`
	Identity cacheartifact.Identity `json:"identity"`
}

// Snapshot is one bounded provider-root observation.
type Snapshot struct {
	Root     cacheartifact.Identity `json:"root"`
	Entries  []Entry                `json:"entries"`
	Complete bool                   `json:"complete"`
}

// Filesystem is the only cache metadata and mutation seam.
type Filesystem interface {
	Snapshot(ctx context.Context, root string, limit int) (Snapshot, error)
	Quarantine(ctx context.Context, root, relativePath, destination string, expectedRoot, expected cacheartifact.Identity) (cacheartifact.Identity, error)
	Restore(ctx context.Context, root, quarantinePath, destination string, expectedRoot, expected cacheartifact.Identity) (cacheartifact.Identity, error)
	Purge(ctx context.Context, root, quarantinePath string, expectedRoot, expected cacheartifact.Identity) error
}

var (
	ErrUnsafePath      = errors.New("cache filesystem: unsafe path")
	ErrChangedIdentity = errors.New("cache filesystem: identity changed")
	ErrDestination     = errors.New("cache filesystem: destination exists")
	ErrCrossDevice     = errors.New("cache filesystem: entry crosses provider filesystem")
	ErrPartialPurge    = errors.New("cache filesystem: purge was partial")
)
