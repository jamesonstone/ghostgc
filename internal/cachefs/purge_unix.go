//go:build darwin || linux

package cachefs

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// authorizedPurge is the single production cache-deletion primitive.
func authorizedPurge(quarantineFD int, name string) error {
	if err := unix.Unlinkat(quarantineFD, name, 0); err != nil {
		return fmt.Errorf("cache filesystem: purge: %w", err)
	}
	return nil
}
