//go:build darwin || linux

package cachefs

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
	"golang.org/x/sys/unix"
)

// RealPurger owns the sole foreground production unlink capability.
type RealPurger struct{}

// NewPurger constructs the short-lived foreground purge executor.
func NewPurger() *RealPurger { return &RealPurger{} }

// Purge permanently unlinks one exact regular quarantine child.
func (p *RealPurger) Purge(ctx context.Context, root, quarantinePath string, expectedRoot, expected cacheartifact.Identity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rootFD, rootID, err := openRoot(root)
	if err != nil {
		return err
	}
	defer closeFD(rootFD)
	if !rootID.SameObject(expectedRoot) || !safeQuarantinePath(quarantinePath) {
		return ErrUnsafePath
	}
	qfd, err := openQuarantine(rootFD, rootID, false)
	if err != nil {
		return err
	}
	defer closeFD(qfd)
	name := filepath.Base(quarantinePath)
	current, err := statAt(qfd, name)
	if err != nil {
		return err
	}
	if !current.Equal(expected) || current.EntryType != "regular" || current.Nlink != 1 {
		return ErrChangedIdentity
	}
	if err := unix.Unlinkat(qfd, name, 0); err != nil {
		return fmt.Errorf("cache filesystem: purge: %w", err)
	}
	return nil
}
