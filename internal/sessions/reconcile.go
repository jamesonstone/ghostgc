package sessions

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// Reconcile derives the session graph, attributions and audit entries from a
// snapshot.
func (r *Reconciler) Reconcile(ctx context.Context, snap *process.Snapshot, tree *process.Tree, storeCommandLines bool) (*Result, error) {
	now := snap.Taken
	nowNs := now.UnixNano()

	graph := adapters.Graph{Snapshot: snap, Tree: tree, Roots: map[int]adapters.AgentRoot{}}

	// Step one: every adapter identifies its own session roots. Adapters do
	// not see each other's conclusions, so a process claimed by two adapters
	// is a conflict to be recorded, not a race to be won silently.
	for _, a := range r.reg.All() {
		roots, err := a.DetectRootProcesses(ctx, graph)
		if err != nil {
			return nil, fmt.Errorf("sessions: adapter %s detection: %w", a.ID(), err)
		}
		for _, root := range roots {
			existing, clash := graph.Roots[root.Process.PID]
			if !clash {
				graph.Roots[root.Process.PID] = root
				continue
			}
			if root.Confidence > existing.Confidence {
				root.Conflicts = append(root.Conflicts, adapters.Evidence{
					Kind:   adapters.EvidenceProcessInfo,
					Detail: fmt.Sprintf("process was also claimed by agent %q with confidence %.2f", existing.AgentID, existing.Confidence),
				})
				graph.Roots[root.Process.PID] = root
			} else {
				existing.Conflicts = append(existing.Conflicts, adapters.Evidence{
					Kind:   adapters.EvidenceProcessInfo,
					Detail: fmt.Sprintf("process is also claimed by agent %q with confidence %.2f", root.AgentID, root.Confidence),
				})
				graph.Roots[root.Process.PID] = existing
			}
		}
	}

	res := &Result{
		Roots:               graph.Roots,
		Attributions:        make(map[string]Attribution),
		Launch:              make(map[string]LaunchContext),
		pendingOwnership:    make(map[string]storage.OwnershipRecord),
		pendingSessionState: make(map[string]State),
		pendingSessionRoot:  make(map[string]process.Key),
		pendingNativeIndex:  make(map[string]string),
	}

	// Step two: turn detected roots into sessions before anything else is
	// attributed, so that a process carrying a brand-new session's identifier
	// can be matched to it in the same cycle.
	live, secondary := r.buildSessions(ctx, graph, res, nowNs)

	// Step three: attribute every inspected process.
	for _, p := range snap.Processes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !p.Detailed || p.UID != r.selfUID {
			continue
		}
		attr := r.attribute(ctx, p, graph, tree)
		if attr.SessionID == "" {
			continue
		}
		if sessionID, isSecondary := secondary[p.PID]; isSecondary && sessionID == attr.SessionID {
			// This process claimed to be the session root but another
			// claimant started earlier. It belongs to the session; it is not
			// the session.
			attr.Relation = adapters.RelationEnvironment
			attr.Evidence = append(attr.Evidence, adapters.Evidence{
				Kind: adapters.EvidenceProcessInfo,
				Detail: fmt.Sprintf("this process names session %s but pid %d started earlier and is the session root; recorded as a member rather than a second root",
					sessionID, live[sessionID].root.Process.PID),
			})
		}
		r.enrich(&attr, p, tree)
		res.Attributions[p.Key().UID()] = attr
		res.AttributedCount++
		r.recordProcess(res, p, attr, nowNs, storeCommandLines)
		r.recordRelationships(res, p, attr, snap, tree, graph, live, nowNs)
	}

	// Step four: end sessions whose exact root process is no longer present.
	// The check is by process key, not PID: a recycled PID must not keep a
	// finished session alive.
	r.endDepartedSessions(snap, res, now, nowNs)

	sort.Slice(res.Sessions, func(i, j int) bool { return res.Sessions[i].SessionID < res.Sessions[j].SessionID })
	sort.Slice(res.Ended, func(i, j int) bool { return res.Ended[i].SessionID < res.Ended[j].SessionID })
	return res, nil
}

func (r *Reconciler) endDepartedSessions(snap *process.Snapshot, res *Result, now time.Time, nowNs int64) {
	seen := make(map[string]bool, len(res.Sessions))
	for _, s := range res.Sessions {
		seen[s.SessionID] = true
	}

	ids := make([]string, 0, len(r.sessionRoot))
	for id := range r.sessionRoot {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, sessionID := range ids {
		rootKey := r.sessionRoot[sessionID]
		if seen[sessionID] {
			continue
		}
		from := r.sessionState[sessionID]
		if !from.Live() {
			continue
		}
		if _, alive := snap.ByKey(rootKey); alive {
			continue
		}

		reason := fmt.Sprintf("root process %s was not found in the snapshot taken at %s",
			rootKey.UID(), now.Format(time.RFC3339))
		evidence := TransitionEvidence(from, StateCompleted, reason)
		if p, pidInUse := snap.ByPID(rootKey.PID); pidInUse {
			evidence = append(evidence, adapters.Evidence{
				Kind: adapters.EvidenceProcessInfo,
				Detail: fmt.Sprintf("pid %d is in use by a different process (started %s, %s); a recycled pid does not keep a session alive",
					rootKey.PID, p.StartTime.Format(time.RFC3339), p.Name()),
			})
		}

		res.Ended = append(res.Ended, EndedSession{
			SessionID: sessionID, From: from, State: StateCompleted, EndedNs: nowNs,
		})
		res.Audit = append(res.Audit, auditEntry(nowNs, AuditSessionEnded, sessionID,
			fmt.Sprintf("session %s ended: %s", sessionID, reason), evidence))
		res.pendingSessionState[sessionID] = StateCompleted
	}
}
