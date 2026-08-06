package daemon

import (
	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

func (d *Daemon) candidateDiagnostics(sessions []storage.SessionRecord, classifications map[string]int,
	decisions []storage.PolicyDecisionRecord, snapshot *process.Snapshot) api.CandidateDiagnostics {
	diagnostics := api.CandidateDiagnostics{
		OrphanedClassifications: classifications["orphaned"],
		PolicyDecisions:         len(decisions),
	}
	for _, session := range sessions {
		if session.EndedNs == nil {
			diagnostics.ActiveSessions++
		}
	}
	policyExecutables := make(map[string]bool)
	for _, definition := range d.cfg.Policies {
		if !definition.Enabled {
			continue
		}
		for _, executable := range definition.Executables {
			policyExecutables[executable] = true
		}
	}
	if snapshot != nil {
		for _, observed := range snapshot.Processes {
			if policyExecutables[observed.Name()] {
				diagnostics.MatchingExecutables++
			}
		}
	}
	return diagnostics
}
