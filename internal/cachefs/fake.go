package cachefs

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
)

// Fake is a deterministic, metadata-only filesystem for policy and daemon tests.
type Fake struct {
	mu      sync.Mutex
	Roots   map[string]*FakeRoot
	Errors  map[string]error
	Partial bool
}

// FakeRoot contains one provider root and exact relative entry identities.
type FakeRoot struct {
	Identity cacheartifact.Identity
	Entries  map[string]cacheartifact.Identity
}

// NewFake constructs an empty deterministic filesystem.
func NewFake() *Fake {
	return &Fake{Roots: make(map[string]*FakeRoot), Errors: make(map[string]error)}
}

// SetRoot replaces one fake provider root.
func (f *Fake) SetRoot(path string, identity cacheartifact.Identity) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Roots[path] = &FakeRoot{Identity: identity, Entries: make(map[string]cacheartifact.Identity)}
}

// Put adds or replaces one relative entry.
func (f *Fake) Put(root, relative string, identity cacheartifact.Identity) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Roots[root].Entries[relative] = identity
}

// Delete removes one exact fake entry.
func (f *Fake) Delete(root, relative string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Roots[root].Entries, relative)
}

// Exists reports whether an exact fake entry exists.
func (f *Fake) Exists(root, relative string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.Roots[root].Entries[relative]
	return ok
}

// Snapshot implements Filesystem without traversing nested entries.
func (f *Fake) Snapshot(ctx context.Context, root string, limit int) (Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.operationError("snapshot"); err != nil {
		return Snapshot{}, err
	}
	item := f.Roots[root]
	if item == nil {
		return Snapshot{}, fmt.Errorf("%w: %s", ErrUnsafePath, root)
	}
	names := make([]string, 0, len(item.Entries))
	for name := range item.Entries {
		if !strings.ContainsRune(name, filepath.Separator) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	complete := len(names) <= limit
	if len(names) > limit {
		names = names[:limit]
	}
	entries := make([]Entry, 0, len(names))
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		entries = append(entries, Entry{Name: name, Identity: item.Entries[name]})
	}
	return Snapshot{Root: item.Identity, Entries: entries, Complete: complete}, nil
}

// Quarantine implements an atomic fake rename.
func (f *Fake) Quarantine(ctx context.Context, root, relativePath, destination string, expectedRoot, expected cacheartifact.Identity) (cacheartifact.Identity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.operationError("quarantine"); err != nil {
		return cacheartifact.Identity{}, err
	}
	if err := ctx.Err(); err != nil {
		return cacheartifact.Identity{}, err
	}
	item := f.Roots[root]
	if item == nil || !item.Identity.SameObject(expectedRoot) {
		return cacheartifact.Identity{}, ErrChangedIdentity
	}
	current, ok := item.Entries[relativePath]
	target := filepath.Join(cacheartifact.QuarantineDirectory, destination)
	if !ok || !current.Equal(expected) {
		return cacheartifact.Identity{}, ErrChangedIdentity
	}
	if _, exists := item.Entries[target]; exists {
		return cacheartifact.Identity{}, ErrDestination
	}
	delete(item.Entries, relativePath)
	item.Entries[target] = current
	return current, nil
}

// Restore implements an atomic fake rename to an absent destination.
func (f *Fake) Restore(ctx context.Context, root, quarantinePath, destination string, expectedRoot, expected cacheartifact.Identity) (cacheartifact.Identity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.operationError("restore"); err != nil {
		return cacheartifact.Identity{}, err
	}
	item := f.Roots[root]
	if item == nil || !item.Identity.SameObject(expectedRoot) {
		return cacheartifact.Identity{}, ErrChangedIdentity
	}
	current, ok := item.Entries[quarantinePath]
	if !ok || !current.Equal(expected) {
		return cacheartifact.Identity{}, ErrChangedIdentity
	}
	if _, exists := item.Entries[destination]; exists {
		return cacheartifact.Identity{}, ErrDestination
	}
	delete(item.Entries, quarantinePath)
	item.Entries[destination] = current
	return current, nil
}

// QuarantineEntry observes one exact fake quarantine child.
func (f *Fake) QuarantineEntry(ctx context.Context, root, quarantinePath string, expectedRoot cacheartifact.Identity) (cacheartifact.Identity, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.operationError("quarantine-entry"); err != nil {
		return cacheartifact.Identity{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return cacheartifact.Identity{}, false, err
	}
	item := f.Roots[root]
	if item == nil || !item.Identity.SameObject(expectedRoot) {
		return cacheartifact.Identity{}, false, ErrChangedIdentity
	}
	current, ok := item.Entries[quarantinePath]
	return current, ok, nil
}

// Purge is the fake permanent-deletion seam.
func (f *Fake) Purge(ctx context.Context, root, quarantinePath string, expectedRoot, expected cacheartifact.Identity) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.operationError("purge"); err != nil {
		return err
	}
	item := f.Roots[root]
	if item == nil || !item.Identity.SameObject(expectedRoot) {
		return ErrChangedIdentity
	}
	current, ok := item.Entries[quarantinePath]
	if !ok || !current.Equal(expected) {
		return ErrChangedIdentity
	}
	delete(item.Entries, quarantinePath)
	if f.Partial {
		return ErrPartialPurge
	}
	return nil
}

func (f *Fake) operationError(kind string) error {
	if f.Errors == nil {
		return nil
	}
	return f.Errors[kind]
}
