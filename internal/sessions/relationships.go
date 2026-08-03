package sessions

import (
	"fmt"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
)

// launchContext finds the nearest ancestor of a session root that is not
// itself an agent process.
func (r *Reconciler) launchContext(root adapters.AgentRoot, graph adapters.Graph) LaunchContext {
	if graph.Tree.Link(root.Process.PID) != process.LinkIntact {
		// Already reparented, or the parent is not in the snapshot. The
		// launcher is genuinely unknown and must be reported as such.
		return LaunchContext{}
	}
	for _, ancestor := range graph.Tree.Ancestors(root.Process.PID) {
		if _, isAgent := graph.Roots[ancestor]; isAgent {
			continue
		}
		p, ok := graph.Snapshot.ByPID(ancestor)
		if !ok {
			break
		}
		return LaunchContext{
			Key:      p.Key(),
			PID:      p.PID,
			Name:     p.Name(),
			ExecPath: p.ExecPath,
			Observed: true,
		}
	}
	return LaunchContext{}
}

func launchUID(l LaunchContext) string {
	if !l.Observed {
		return ""
	}
	return l.Key.UID()
}

// recordRelationships emits the typed edges for one attributed process.
func (r *Reconciler) recordRelationships(res *Result, p process.Process, attr Attribution,
	snap *process.Snapshot, tree *process.Tree, graph adapters.Graph, live map[string]sessionRootInfo, nowNs int64) {

	key := p.Key()
	add := func(rel Relationship) {
		res.Relationships = append(res.Relationships, rel.Record(attr.SessionID, nowNs))
	}

	switch tree.Link(p.PID) {
	case process.LinkIntact:
		if parent, ok := snap.ByPID(p.PPID); ok {
			add(Relationship{
				Kind:   RelParentChild,
				From:   key,
				To:     parent.Key(),
				Detail: fmt.Sprintf("live parent is pid %d (%s)", parent.PID, parent.Name()),
				AtNs:   nowNs,
			})
		}
	case process.LinkReparented:
		add(Relationship{
			Kind: RelReparented,
			From: key,
			Detail: fmt.Sprintf("parent link lost; the process was reparented to pid %d. Detached is not orphaned: this records that the original parent exited, nothing about whether work remains",
				process.InitPID),
			AtNs: nowNs,
		})
	}

	if attr.OriginalParentObserved && attr.OriginalPPID > 0 {
		add(Relationship{
			Kind:   RelOriginalParent,
			From:   key,
			Detail: fmt.Sprintf("created by pid %d, observed alive at the time", attr.OriginalPPID),
			AtNs:   nowNs,
		})
	}

	if attr.Relation == adapters.RelationEnvironment {
		add(Relationship{
			Kind:   RelEnvironment,
			From:   key,
			Detail: "environment carries the agent's own session identifier",
			AtNs:   nowNs,
		})
	}

	if attr.RepositoryPath != "" {
		add(Relationship{
			Kind:   RelRepository,
			From:   key,
			Detail: "working directory is inside " + attr.RepositoryPath,
			AtNs:   nowNs,
		})
	}

	// Terminal and process-group edges are context, never ownership. See
	// AttributingKinds.
	if info, ok := live[attr.SessionID]; ok {
		if p.SID != 0 && p.SID == info.sid && p.PID != info.root.Process.PID {
			add(Relationship{
				Kind: RelTerminal,
				From: key,
				To:   info.rootKey,
				Detail: fmt.Sprintf("shares POSIX session %d with the session root. This is context, not ownership: a session leader is usually the user's shell, so unrelated commands share it too",
					p.SID),
				AtNs: nowNs,
			})
		}
		if p.PGID != 0 && p.PGID == info.root.Process.PGID && p.PID != info.root.Process.PID {
			add(Relationship{
				Kind:   RelProcessGroup,
				From:   key,
				To:     info.rootKey,
				Detail: fmt.Sprintf("shares process group %d with the session root", p.PGID),
				AtNs:   nowNs,
			})
		}
	}
}
