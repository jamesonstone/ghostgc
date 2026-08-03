// Package sessions turns a process snapshot into a session graph, durable
// ownership and an audit trail.
//
// This is where the specification's central rule is enforced: ownership is
// recorded the first time it is observed and is not re-derived from scratch
// every cycle. When the operating system reparents a process after its parent
// exits, the live process tree loses the relationship — but the daemon does
// not, because it wrote it down.
//
// Delivery phase 2 turns that flat association into a graph. Each reason a
// process belongs is a separate typed edge, so losing one reason does not lose
// the session; and each edge is explicit about whether it may establish
// ownership at all.
package sessions

import (
	"time"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/repository"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// State is the lifecycle state of a session.
//
// Delivery phase 2 produces starting, active, completed and unknown. The
// remaining states need the activity deltas that arrive in phase 3 and the
// classification engine in phase 4. A state is never guessed at: a session the
// daemon cannot describe is unknown, and unknown is protected.
type State string

const (
	StateStarting   State = "starting"
	StateActive     State = "active"
	StateIdle       State = "idle"
	StateWaiting    State = "waiting"
	StateSuspicious State = "suspicious"
	StateHung       State = "hung"
	StateCompleted  State = "completed"
	StateCrashed    State = "crashed"
	StateOrphaned   State = "orphaned"
	StateUnknown    State = "unknown"
)

// Live reports whether a state describes a session that is still running.
func (s State) Live() bool {
	switch s {
	case StateCompleted, StateCrashed, StateOrphaned, StateUnknown, "":
		return false
	}
	return true
}

// StartingGrace is how long after a root process starts the session is
// reported as starting rather than active. It is one default scan interval:
// a session observed in its first cycle has not yet had a chance to do
// anything, and saying "active" would overstate what was seen.
const StartingGrace = 15 * time.Second

// Audit entry kinds.
const (
	AuditSessionStarted    = "session.started"
	AuditSessionEnded      = "session.ended"
	AuditSessionState      = "session.state-changed"
	AuditProcessAttributed = "process.attributed"
	AuditOwnershipRetained = "process.ownership-retained"
	AuditAttributionChange = "process.attribution-changed"
)

// Attribution is the daemon's per-process conclusion, with its evidence.
type Attribution struct {
	AgentID    string              `json:"agent_id,omitempty"`
	SessionID  string              `json:"session_id,omitempty"`
	Relation   adapters.Relation   `json:"relation,omitempty"`
	Confidence float64             `json:"confidence"`
	Evidence   []adapters.Evidence `json:"evidence,omitempty"`
	Conflicts  []adapters.Evidence `json:"conflicts,omitempty"`
	LinkState  process.LinkState   `json:"parent_link"`
	Key        process.Key         `json:"-"`

	// OriginalPPID is the parent recorded at first observation.
	OriginalPPID int `json:"original_ppid,omitempty"`
	// OriginalParentObserved reports whether that parent was actually seen
	// alive. When false, OriginalPPID is only what the kernel reported after
	// reparenting and must not be presented as the creator.
	OriginalParentObserved bool `json:"original_parent_observed"`
	// RepositoryPath is the repository the process is working inside.
	RepositoryPath string `json:"repository_path,omitempty"`
}

// Attributed reports whether the attribution clears the reporting threshold.
// Below it, the process is unknown, and unknown is protected.
func (a Attribution) Attributed() bool {
	return a.SessionID != "" && a.Confidence >= adapters.ConfidenceAttributable
}

// Result is everything one reconciliation produced.
type Result struct {
	Sessions      []storage.SessionRecord
	Ended         []EndedSession
	Processes     []storage.ProcessRecord
	Observations  []storage.ObservationRecord
	Ownership     []storage.OwnershipRecord
	Relationships []storage.RelationshipRecord
	Audit         []storage.AuditRecord

	// Roots maps root PID to the detected agent root for this snapshot.
	Roots map[int]adapters.AgentRoot
	// Attributions is keyed by proc_uid and covers every attributed process.
	Attributions map[string]Attribution
	// Launch describes how each session root was started, keyed by session id.
	Launch map[string]LaunchContext

	AttributedCount int

	// Pending cross-cycle state. It is applied to the Reconciler only by
	// Commit, which the daemon calls after the transaction that persists this
	// result succeeds. Advancing in-memory state before the write lands would
	// suppress the audit entry for a change that was never recorded.
	pendingOwnership    map[string]storage.OwnershipRecord
	pendingSessionState map[string]State
	pendingSessionRoot  map[string]process.Key
	pendingNativeIndex  map[string]string
}

// Commit applies the result's cross-cycle state to the Reconciler. Call it
// only after the result has been persisted.
func (r *Reconciler) Commit(res *Result) {
	for uid, own := range res.pendingOwnership {
		r.ownership[uid] = own
	}
	for id, state := range res.pendingSessionState {
		r.sessionState[id] = state
	}
	for id, key := range res.pendingSessionRoot {
		r.sessionRoot[id] = key
	}
	for native, id := range res.pendingNativeIndex {
		r.nativeIndex[native] = id
	}
}

// EndedSession records a session whose root process is gone.
type EndedSession struct {
	SessionID string
	From      State
	State     State
	EndedNs   int64
}

// Reconciler holds the cross-cycle state needed to emit audit entries only on
// change and to keep ownership durable.
type Reconciler struct {
	reg     *adapters.Registry
	selfPID int
	selfUID uint32
	repos   *repository.Finder

	// ownership is the durable session/process association, keyed by proc_uid.
	ownership map[string]storage.OwnershipRecord
	// sessionState is the last recorded state per session id.
	sessionState map[string]State
	// sessionRoot maps session id to its root process key, so that a session
	// is only ended when that exact process is gone rather than when its PID
	// stops appearing.
	sessionRoot map[string]process.Key
	// nativeIndex maps "agent|native-session-id" to the ghostgc session id, so
	// a process carrying an agent's own identifier can be attributed to the
	// session that owns it — including a session that has already finished,
	// which is the interesting case.
	nativeIndex map[string]string
}

// New constructs a Reconciler.
func New(reg *adapters.Registry, selfPID int, selfUID uint32, repos *repository.Finder) *Reconciler {
	if repos == nil {
		repos = repository.NewFinder()
	}
	return &Reconciler{
		reg:          reg,
		selfPID:      selfPID,
		selfUID:      selfUID,
		repos:        repos,
		ownership:    make(map[string]storage.OwnershipRecord),
		sessionState: make(map[string]State),
		sessionRoot:  make(map[string]process.Key),
		nativeIndex:  make(map[string]string),
	}
}

// Seed restores cross-cycle state from storage after a daemon restart.
func (r *Reconciler) Seed(sessions []storage.SessionRecord, ownership map[string]storage.OwnershipRecord) {
	for uid, rec := range ownership {
		r.ownership[uid] = rec
	}
	for _, s := range sessions {
		r.sessionState[s.SessionID] = State(s.State)
		if key, err := process.ParseKey(s.RootProcUID); err == nil {
			r.sessionRoot[s.SessionID] = key
		}
		if s.NativeSessionID != "" {
			r.nativeIndex[nativeKey(s.AgentID, s.NativeSessionID)] = s.SessionID
		}
	}
}

// Ownership exposes the durable associations for read-only use.
func (r *Reconciler) Ownership() map[string]storage.OwnershipRecord {
	out := make(map[string]storage.OwnershipRecord, len(r.ownership))
	for k, v := range r.ownership {
		out[k] = v
	}
	return out
}

func nativeKey(agentID, nativeID string) string { return agentID + "|" + nativeID }
