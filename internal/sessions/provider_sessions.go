package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

type providerSessionInfo struct {
	session adapters.ProviderSession
}

func (r *Reconciler) knownProviderSessions() []adapters.KnownSession {
	out := make([]adapters.KnownSession, 0, len(r.nativeIndex))
	for key, sessionID := range r.nativeIndex {
		agentID, nativeID, ok := strings.Cut(key, "|")
		rootKey, hasRoot := r.sessionRoot[sessionID]
		if !ok || nativeID == "" || !hasRoot || !r.sessionState[sessionID].Live() {
			continue
		}
		out = append(out, adapters.KnownSession{
			AgentID: agentID, NativeID: nativeID, SessionID: sessionID,
			RootKey: rootKey, Metadata: r.sessionMetadata[sessionID],
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AgentID != out[j].AgentID {
			return out[i].AgentID < out[j].AgentID
		}
		return out[i].NativeID < out[j].NativeID
	})
	return out
}

func (r *Reconciler) discoverProviderSessions(ctx context.Context, graph adapters.Graph) map[string]providerSessionInfo {
	out := make(map[string]providerSessionInfo)
	ambiguous := make(map[string]bool)
	for _, adapter := range r.reg.All() {
		discoverer, ok := adapter.(adapters.SessionLifecycleAdapter)
		if !ok {
			continue
		}
		for _, session := range discoverer.DiscoverProviderSessions(ctx, graph) {
			key := nativeKey(session.AgentID, session.NativeID)
			root, rootOK := graph.Roots[session.Root.Process.PID]
			validState := session.State == adapters.ProviderSessionActive ||
				session.State == adapters.ProviderSessionCompleted
			if session.AgentID != adapter.ID() || session.NativeID == "" || session.SessionID == "" ||
				!validState || !rootOK || root.Key != session.Root.Key || root.AgentID != adapter.ID() ||
				session.StartedAt.IsZero() || session.ChangedAt.Before(session.StartedAt) ||
				session.ChangedAt.After(graph.Snapshot.Taken) {
				continue
			}
			if prior, exists := out[key]; exists &&
				(prior.session.SessionID != session.SessionID || prior.session.Root.Key != session.Root.Key) {
				delete(out, key)
				ambiguous[key] = true
				continue
			}
			if !ambiguous[key] {
				out[key] = providerSessionInfo{session: session}
			}
		}
	}
	return out
}

func (r *Reconciler) buildProviderSessions(graph adapters.Graph, providers map[string]providerSessionInfo,
	res *Result, roots map[string]sessionRootInfo, nowNs int64) {
	ids := make([]string, 0, len(providers))
	for _, info := range providers {
		ids = append(ids, info.session.SessionID)
	}
	sort.Strings(ids)
	seen := make(map[string]bool, len(res.Sessions))
	for _, session := range res.Sessions {
		seen[session.SessionID] = true
	}
	for _, sessionID := range ids {
		var provider adapters.ProviderSession
		for _, info := range providers {
			if info.session.SessionID == sessionID {
				provider = info.session
				break
			}
		}
		if seen[sessionID] {
			continue
		}
		seen[sessionID] = true
		state := State(provider.State)
		from := r.sessionState[sessionID]
		changed := from != state
		changedNs := int64(0)
		if changed {
			changedNs = provider.ChangedAt.UnixNano()
		}
		workingDir := provider.Metadata.WorkingDir
		if workingDir == "" {
			workingDir = provider.Root.Process.CWD
		}
		repo := r.repos.Describe(workingDir)
		launch := r.launchContext(provider.Root, graph)
		metadata, _ := json.Marshal(provider.Metadata)
		evidence, _ := json.Marshal(provider.Evidence)
		var endedNs *int64
		if state == StateCompleted {
			value := provider.ChangedAt.UnixNano()
			endedNs = &value
		}
		record := storage.SessionRecord{
			SessionID: sessionID, AgentID: provider.AgentID,
			RootProcUID: provider.Root.Key.UID(), RootPID: provider.Root.Process.PID,
			State: string(state), Confidence: provider.Root.Confidence,
			WorkingDir: workingDir, RepositoryPath: repo.Root, TTY: provider.Root.Process.TTY,
			Invocation: provider.Metadata.Invocation, MetadataJSON: string(metadata), EvidenceJSON: string(evidence),
			StartedNs: provider.StartedAt.UnixNano(), LastSeenNs: nowNs, EndedNs: endedNs,
			NativeSessionID: provider.NativeID, PreviousState: string(from), StateChangedNs: changedNs,
			HostProcUID: launchUID(launch), HostPID: launch.PID, HostName: launch.Name, HostExecPath: launch.ExecPath,
			Branch: repo.Branch, RepositoryBusy: repo.Busy(), TerminalSID: provider.Root.Process.SID,
		}
		res.Sessions = append(res.Sessions, record)
		res.Launch[sessionID] = launch
		if changed {
			reason := fmt.Sprintf("Codex rollout lifecycle reported task %s", state)
			kind := AuditSessionState
			summary := fmt.Sprintf("provider task session %s: %s -> %s", sessionID, nonEmptyState(from), state)
			if from == "" && state == StateActive {
				kind = AuditSessionStarted
				summary = fmt.Sprintf("Codex task session %s started under host pid %d", sessionID, provider.Root.Process.PID)
			}
			if state == StateCompleted {
				kind = AuditSessionEnded
				summary = fmt.Sprintf("Codex task session %s completed from provider lifecycle evidence", sessionID)
				res.Ended = append(res.Ended, EndedSession{
					SessionID: sessionID, From: from, State: state, EndedNs: provider.ChangedAt.UnixNano(),
				})
			}
			auditEvidence := append(TransitionEvidence(from, state, reason), provider.Evidence...)
			res.Audit = append(res.Audit, auditEntry(provider.ChangedAt.UnixNano(), kind, sessionID, summary, auditEvidence))
		}
		res.pendingSessionState[sessionID] = state
		res.pendingSessionRoot[sessionID] = provider.Root.Key
		res.pendingNativeIndex[nativeKey(provider.AgentID, provider.NativeID)] = sessionID
		res.pendingMetadata[sessionID] = provider.Metadata
		roots[sessionID] = sessionRootInfo{
			root: provider.Root, agentID: provider.AgentID, rootKey: provider.Root.Key,
			sid: provider.Root.Process.SID, tty: provider.Root.Process.TTY,
		}
	}
}

func (r *Reconciler) attributeByProviderSession(p process.Process, graph adapters.Graph,
	providers map[string]providerSessionInfo) Attribution {
	for _, adapter := range r.reg.All() {
		nativeID, ok := adapter.NativeSessionID(p)
		if !ok {
			continue
		}
		info, known := providers[nativeKey(adapter.ID(), nativeID)]
		if !known {
			continue
		}
		root := info.session.Root
		for depth, ancestor := range graph.Tree.Ancestors(p.PID) {
			if ancestor != root.Process.PID {
				continue
			}
			observed, exact := graph.Snapshot.ByKey(root.Key)
			if !exact || observed.UID != p.UID {
				return Attribution{}
			}
			evidence := append([]adapters.Evidence(nil), info.session.Evidence...)
			evidence = append(evidence, adapters.Evidence{
				Kind:   adapters.EvidenceAncestry,
				Detail: fmt.Sprintf("process is a descendant of the provider task host pid %d through %d intact parent link(s)", ancestor, depth+1),
			})
			return Attribution{
				AgentID: adapter.ID(), SessionID: info.session.SessionID,
				Confidence: root.Confidence, Evidence: evidence,
				Relation: adapters.RelationDescendant,
			}
		}
	}
	return Attribution{}
}
