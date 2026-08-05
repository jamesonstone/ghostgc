package daemon

import (
	"path/filepath"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/adapters/codex"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/repository"
)

// BuildRegistry constructs the adapter registry from configuration.
func BuildRegistry(cfg config.Config, repos *repository.Finder) *adapters.Registry {
	var list []adapters.AgentAdapter
	for id, agent := range cfg.Agents {
		if !agent.Enabled {
			continue
		}
		switch id {
		case codex.ID:
			list = append(list, codex.New(repos))
		default:
			// An unknown adapter id is a configuration statement the daemon
			// cannot honour. It is reported by `ghostgc doctor` rather than
			// silently ignored, but it must not stop observation.
		}
	}
	return adapters.NewRegistry(list...)
}

// AdapterEnvKeys returns the environment variables the enabled adapters need.
// The daemon command calls this before constructing the platform so that the
// collector extracts nothing else.
func AdapterEnvKeys(cfg config.Config) []string {
	return BuildRegistry(cfg, repository.NewFinder()).EnvKeys()
}

func worktreeSnapshotDir(paths config.Paths) (string, error) {
	root := paths.StateDir
	if root == "" {
		root = filepath.Dir(paths.Database)
	}
	return filepath.Abs(filepath.Join(root, "git-exec"))
}
