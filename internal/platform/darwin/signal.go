//go:build darwin

package darwin

import (
	"context"
	"fmt"
	"syscall"

	"github.com/jamesonstone/ghostgc/internal/process"
)

// SignalProcess performs the final exact-identity check and sends the sole
// allowed signal. Authority is established by the daemon before this method;
// keeping the identity check beside the syscall narrows the PID-reuse window.
func (c *Collector) SignalProcess(ctx context.Context, key process.Key,
	executable process.ExecutableIdentity, sig syscall.Signal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if sig != syscall.SIGTERM {
		return fmt.Errorf("darwin: only SIGTERM is allowed")
	}
	observed, err := c.InspectProcess(ctx, key.PID)
	if err != nil {
		return fmt.Errorf("darwin: signal target %s unavailable: %w", key, err)
	}
	if observed.UID != c.uid || observed.Key() != key {
		return fmt.Errorf("darwin: signal target changed from %s to %s", key, observed.Key())
	}
	identity, ok := observed.Executable()
	if !ok || identity != executable {
		return fmt.Errorf("darwin: signal target executable changed or is unavailable")
	}
	if err := syscall.Kill(key.PID, syscall.SIGTERM); err != nil {
		return fmt.Errorf("darwin: signalling exact target %s: %w", key, err)
	}
	return nil
}
