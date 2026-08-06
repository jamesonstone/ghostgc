package daemon

import (
	"context"

	"github.com/jamesonstone/ghostgc/internal/config"
)

// EffectiveConfig returns the immutable configuration loaded at daemon start.
func (d *Daemon) EffectiveConfig(context.Context) (config.Config, error) {
	return d.cfg, nil
}
