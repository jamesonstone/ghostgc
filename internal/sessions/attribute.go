package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// attribute asks every adapter, then falls back to durable ownership, then to
// environment membership.
func (r *Reconciler) attribute(ctx context.Context, p process.Process, graph adapters.Graph, tree *process.Tree) Attribution {
	uid := p.Key().UID()
	best := Attribution{Key: p.Key(), LinkState: tree.Link(p.PID)}

	for _, a := range r.reg.All() {
		got := a.AttributeProcess(ctx, p, graph)
		if got.SessionID == "" || got.Confidence <= best.Confidence {
			continue
		}
		best.AgentID = got.AgentID
		best.SessionID = got.SessionID
		best.Confidence = got.Confidence
		best.Evidence = got.Evidence
		best.Conflicts = got.Conflicts
		best.Relation = got.Relation
	}
	if best.SessionID != "" {
		return best
	}

	// No live evidence. If this exact process was previously observed to
	// belong to a session, that fact stands. Losing a parent link is a
	// reparenting event, not a change of ownership.
	if prior, ok := r.ownership[uid]; ok {
		best.AgentID = prior.AgentID
		best.SessionID = prior.SessionID
		best.Confidence = prior.Confidence
		best.Relation = adapters.RelationRecorded
		best.Evidence = []adapters.Evidence{
			{
				Kind: adapters.EvidenceRecorded,
				Detail: fmt.Sprintf("ownership by session %s was recorded at first observation (%s) with confidence %.2f",
					prior.SessionID, time.Unix(0, prior.FirstSeenNs).Format(time.RFC3339), prior.Confidence),
				Weight: prior.Confidence,
			},
			originalParentEvidence(prior.OriginalPPID, prior.OriginalParentObserved, best.LinkState),
		}
		return best
	}

	// Nothing structural claims it. If it carries an agent's own session
	// identifier it descends from that session, which is worth saying — but
	// capped, because every descendant inherits the variable forever.
	return r.attributeByEnvironment(p, best)
}

func (r *Reconciler) attributeByEnvironment(p process.Process, best Attribution) Attribution {
	for _, a := range r.reg.All() {
		native, ok := a.NativeSessionID(p)
		if !ok || native == "" {
			continue
		}
		sessionID, known := r.nativeIndex[nativeKey(a.ID(), native)]
		if !known {
			continue
		}
		confidence := adapters.ConfidenceEnvironmentMembership
		best.AgentID = a.ID()
		best.SessionID = sessionID
		best.Confidence = confidence
		best.Relation = adapters.RelationEnvironment
		best.Evidence = []adapters.Evidence{
			{
				Kind: adapters.EvidenceEnvironment,
				Detail: fmt.Sprintf("environment carries %s session identifier %q, which names session %s",
					a.ID(), native, sessionID),
				Weight: confidence,
			},
			{
				Kind:   adapters.EvidenceEnvironment,
				Detail: "an environment variable is inherited by every descendant, so this establishes that the process descends from that session and nothing more; it is capped below the policy-eligible threshold for that reason",
			},
		}
		return best
	}
	return best
}

// enrich fills in the parts of an attribution that come from the daemon's own
// records rather than from an adapter.
func (r *Reconciler) enrich(attr *Attribution, p process.Process, tree *process.Tree) {
	uid := p.Key().UID()
	if prior, ok := r.ownership[uid]; ok {
		attr.OriginalPPID = prior.OriginalPPID
		attr.OriginalParentObserved = prior.OriginalParentObserved
	} else {
		// First observation. The claimed parent is only the creator if it was
		// actually there and could plausibly have created this process.
		attr.OriginalPPID = p.PPID
		attr.OriginalParentObserved = tree.Link(p.PID) == process.LinkIntact
	}
	attr.RepositoryPath = r.repos.Root(p.CWD)
}

func (r *Reconciler) adapterFor(id string) adapters.AgentAdapter {
	for _, a := range r.reg.All() {
		if a.ID() == id {
			return a
		}
	}
	return nil
}

func (r *Reconciler) recordProcess(res *Result, p process.Process, attr Attribution, nowNs int64, storeCommandLines bool) {
	uid := p.Key().UID()
	prior, known := r.ownership[uid]

	firstSeenNs := nowNs
	if known {
		firstSeenNs = prior.FirstSeenNs
	}

	cmdline := "[]"
	if storeCommandLines {
		if b, err := json.Marshal(process.RedactArgs(p.Args)); err == nil {
			cmdline = string(b)
		}
	} else if len(p.Args) > 0 {
		if b, err := json.Marshal([]string{p.Name()}); err == nil {
			cmdline = string(b)
		}
	}

	evidenceJSON, _ := json.Marshal(attr.Evidence)

	res.Processes = append(res.Processes, storage.ProcessRecord{
		ProcUID:                uid,
		PID:                    p.PID,
		PPID:                   p.PPID,
		OriginalPPID:           attr.OriginalPPID,
		OriginalParentObserved: attr.OriginalParentObserved,
		PGID:                   p.PGID,
		SID:                    p.SID,
		UID:                    int64(p.UID),
		StartTimeNs:            p.StartTime.UnixNano(),
		Comm:                   p.Comm,
		ExecPath:               p.ExecPath,
		Cmdline:                cmdline,
		CWD:                    p.CWD,
		TTY:                    p.TTY,
		RepositoryPath:         attr.RepositoryPath,
		AgentID:                attr.AgentID,
		SessionID:              attr.SessionID,
		Relation:               string(attr.Relation),
		Confidence:             attr.Confidence,
		EvidenceJSON:           string(evidenceJSON),
		FirstSeenNs:            firstSeenNs,
		LastSeenNs:             nowNs,
	})

	res.Observations = append(res.Observations, storage.ObservationRecord{
		ProcUID:   uid,
		TsNs:      nowNs,
		Status:    string(p.Status),
		PPID:      p.PPID,
		CPUTimeNs: int64(p.CPUTime),
		RSSBytes:  int64(p.RSSBytes),
		VSZBytes:  int64(p.VSZBytes),
		Threads:   p.Threads,
	})

	own := storage.OwnershipRecord{
		SessionID:              attr.SessionID,
		ProcUID:                uid,
		AgentID:                attr.AgentID,
		Relation:               string(attr.Relation),
		Confidence:             attr.Confidence,
		EvidenceJSON:           string(evidenceJSON),
		OriginalPPID:           attr.OriginalPPID,
		OriginalParentObserved: attr.OriginalParentObserved,
		FirstSeenNs:            firstSeenNs,
		LastSeenNs:             nowNs,
	}
	res.Ownership = append(res.Ownership, own)
	res.pendingOwnership[uid] = own

	switch {
	case !known:
		res.Audit = append(res.Audit, auditEntry(nowNs, AuditProcessAttributed, uid,
			fmt.Sprintf("pid %d (%s) attributed to %s session %s as %s with confidence %.2f",
				p.PID, p.Name(), attr.AgentID, attr.SessionID, attr.Relation, attr.Confidence),
			attr.Evidence))
	case prior.SessionID != attr.SessionID:
		res.Audit = append(res.Audit, auditEntry(nowNs, AuditAttributionChange, uid,
			fmt.Sprintf("pid %d (%s) moved from session %s to session %s",
				p.PID, p.Name(), prior.SessionID, attr.SessionID),
			attr.Evidence))
	case attr.Relation == adapters.RelationRecorded && prior.Relation != string(adapters.RelationRecorded):
		res.Audit = append(res.Audit, auditEntry(nowNs, AuditOwnershipRetained, uid,
			fmt.Sprintf("pid %d (%s) lost its live parent link (%s) and retains recorded ownership by session %s",
				p.PID, p.Name(), attr.LinkState, attr.SessionID),
			attr.Evidence))
	}
}
