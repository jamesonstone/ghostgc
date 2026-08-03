package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// sessionRootInfo is the per-session context built from a detected root.
type sessionRootInfo struct {
	root    adapters.AgentRoot
	agentID string
	rootKey process.Key
	sid     int
	tty     string
}

// buildSessions turns detected roots into session records, resolving launch
// context, repository metadata and the state transition for each.
//
// Several detected processes can name the same session: an agent that exposes
// its session identifier through the environment passes it to every helper it
// starts, and a helper built from the same executable is detected on its own
// identity evidence too. A session has exactly one root, so the earliest-started
// claimant wins and the rest are recorded as members. Writing two roots for one
// session would leave "which process is this session" unanswerable, which is
// the question every later phase depends on.
func (r *Reconciler) buildSessions(ctx context.Context, graph adapters.Graph, res *Result, nowNs int64) (map[string]sessionRootInfo, map[int]string) {
	live := make(map[string]sessionRootInfo)
	secondary := make(map[int]string)

	pids := make([]int, 0, len(graph.Roots))
	for pid := range graph.Roots {
		pids = append(pids, pid)
	}
	sort.Ints(pids)

	// First pass: decide which claimant is the root of each session.
	chosen := make(map[string]int)
	sessionOf := make(map[int]string)
	for _, pid := range pids {
		root := graph.Roots[pid]
		adapter := r.adapterFor(root.AgentID)
		if adapter == nil {
			continue
		}
		attr := adapter.AttributeProcess(ctx, root.Process, graph)
		if attr.SessionID == "" {
			continue
		}
		sessionOf[pid] = attr.SessionID
		incumbent, taken := chosen[attr.SessionID]
		if !taken {
			chosen[attr.SessionID] = pid
			continue
		}
		if root.Process.StartTime.Before(graph.Roots[incumbent].Process.StartTime) {
			chosen[attr.SessionID] = pid
			secondary[incumbent] = attr.SessionID
		} else {
			secondary[pid] = attr.SessionID
		}
	}

	for _, pid := range pids {
		sessionID, ok := sessionOf[pid]
		if !ok || chosen[sessionID] != pid {
			continue
		}
		root := graph.Roots[pid]
		adapter := r.adapterFor(root.AgentID)

		launch := r.launchContext(root, graph)
		res.Launch[sessionID] = launch

		repo := r.repos.Describe(root.Process.CWD)
		native, _ := adapter.NativeSessionID(root.Process)

		state, from, changed, reason := r.transition(sessionID, root.Process, nowNs)

		evidence, _ := json.Marshal(root.Evidence)
		metadata, _ := json.Marshal(root.Metadata)
		rec := storage.SessionRecord{
			SessionID:       sessionID,
			AgentID:         root.AgentID,
			RootProcUID:     root.Key.UID(),
			RootPID:         root.Process.PID,
			State:           string(state),
			Confidence:      root.Confidence,
			WorkingDir:      root.Process.CWD,
			RepositoryPath:  repo.Root,
			TTY:             root.Process.TTY,
			Invocation:      root.Metadata.Invocation,
			MetadataJSON:    string(metadata),
			EvidenceJSON:    string(evidence),
			StartedNs:       root.Process.StartTime.UnixNano(),
			LastSeenNs:      nowNs,
			NativeSessionID: native,
			PreviousState:   string(from),
			HostProcUID:     launchUID(launch),
			HostPID:         launch.PID,
			HostName:        launch.Name,
			HostExecPath:    launch.ExecPath,
			Branch:          repo.Branch,
			RepositoryBusy:  repo.Busy(),
			TerminalSID:     root.Process.SID,
		}
		if changed {
			rec.StateChangedNs = nowNs
		}
		res.Sessions = append(res.Sessions, rec)

		if changed {
			kind := AuditSessionState
			summary := fmt.Sprintf("session %s: %s -> %s (%s)", sessionID, nonEmptyState(from), state, reason)
			if from == "" {
				kind = AuditSessionStarted
				summary = fmt.Sprintf("%s session %s observed with root pid %d (%s), launched by %s",
					root.AgentID, sessionID, root.Process.PID, root.Process.Name(), launch.Describe())
			}
			ev := append(TransitionEvidence(from, state, reason), root.Evidence...)
			ev = append(ev, launchEvidence(launch), repositoryEvidence(repo))
			res.Audit = append(res.Audit, auditEntry(nowNs, kind, sessionID, summary, compactEvidence(ev)))
		}

		res.pendingSessionState[sessionID] = state
		res.pendingSessionRoot[sessionID] = root.Key
		if native != "" {
			res.pendingNativeIndex[nativeKey(root.AgentID, native)] = sessionID
		}

		live[sessionID] = sessionRootInfo{
			root:    root,
			agentID: root.AgentID,
			rootKey: root.Key,
			sid:     root.Process.SID,
			tty:     root.Process.TTY,
		}

		// The launch edge is recorded from the session root outwards, so that
		// "who started this" survives the launcher's own exit.
		if launch.Observed {
			res.Relationships = append(res.Relationships, Relationship{
				Kind:   RelLaunch,
				From:   root.Key,
				To:     launch.Key,
				Detail: fmt.Sprintf("session root was started by %s", launch.Describe()),
				AtNs:   nowNs,
			}.Record(sessionID, nowNs))
		}
		if repo.Root != "" {
			res.Relationships = append(res.Relationships, Relationship{
				Kind:   RelRepository,
				From:   root.Key,
				Detail: repositoryDetail(repo),
				AtNs:   nowNs,
			}.Record(sessionID, nowNs))
		}
	}
	return live, secondary
}

// transition computes the next session state and whether it changed.
func (r *Reconciler) transition(sessionID string, root process.Process, nowNs int64) (to, from State, changed bool, reason string) {
	from = r.sessionState[sessionID]
	age := time.Duration(nowNs - root.StartTime.UnixNano())

	switch {
	case from == "" && age < StartingGrace:
		to = StateStarting
		reason = fmt.Sprintf("root process started %s ago, inside the first observation window", age.Truncate(time.Second))
	case from == "":
		to = StateActive
		reason = fmt.Sprintf("root process %d is running and was already %s old when first observed",
			root.PID, age.Truncate(time.Second))
	case from == StateStarting && age >= StartingGrace:
		to = StateActive
		reason = fmt.Sprintf("root process survived the first observation window and is still running after %s", age.Truncate(time.Second))
	case !from.Live():
		// The root was reported gone and is present again. That is either a
		// missed scan or a misobservation; either way it is worth an audit
		// entry rather than a silent correction.
		to = StateActive
		reason = fmt.Sprintf("root process %d was observed again after the session was reported %s", root.PID, from)
	default:
		to = from
	}
	return to, from, to != from, reason
}
