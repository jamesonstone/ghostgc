//go:build !darwin && !linux

package platform

import "fmt"

// New returns the Platform implementation for the host operating system.
func New(opts Options) (Platform, error) {
	return nil, fmt.Errorf("%w: ghostgc supports darwin and linux only", ErrNotSupported)
}
