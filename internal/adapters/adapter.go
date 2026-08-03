// Package adapters defines the contract between the daemon and per-agent
// detection logic.
//
// An adapter answers two questions and nothing else: which processes are the
// entry points of an agent session, and which observed process belongs to
// which session. It never decides that anything should be terminated.
package adapters

import (
	"context"
	"sort"

	"github.com/jamesonstone/ghostgc/internal/process"
)

// Confidence thresholds shared by every adapter.
//
// These bound what a confidence score is allowed to unlock. Confidence is
// never the only safety control, but it is a necessary one.
const (
	// ConfidencePolicyEligible is the minimum for a process to be considered
	// by a cleanup policy at all. Nothing reaches this bar by name alone.
	ConfidencePolicyEligible = 0.95
	// ConfidenceAttributable is the minimum for a process to be shown as
	// belonging to a session. Between this and ConfidencePolicyEligible the
	// attribution is reportable but audit-only.
	ConfidenceAttributable = 0.75
	// ConfidenceRootDetection is the minimum for a process to be treated as a
	// session root. Detecting a session is not an action, so this bar sits
	// below ConfidenceAttributable deliberately: a weakly identified session
	// that is visible and labelled low-confidence is more useful, and safer,
	// than an unmodelled process tree.
	ConfidenceRootDetection = 0.5
	// ConfidenceEnvironmentMembership is the ceiling for a process attributed
	// solely because its environment carries an agent's session identifier.
	//
	// It sits deliberately below ConfidencePolicyEligible. An environment
	// variable is inherited by every descendant for the lifetime of the
	// machine, so it is excellent evidence of lineage and poor evidence of
	// anything else. A process cannot become eligible for automated action on
	// the strength of a variable it merely inherited.
	ConfidenceEnvironmentMembership = 0.90
)

// EvidenceKind labels where a piece of evidence came from.
type EvidenceKind string

const (
	EvidenceExecutable  EvidenceKind = "executable"
	EvidenceArgv        EvidenceKind = "argv"
	EvidenceEnvironment EvidenceKind = "environment"
	EvidenceAncestry    EvidenceKind = "ancestry"
	EvidenceWorkingDir  EvidenceKind = "working-directory"
	EvidenceTerminal    EvidenceKind = "terminal"
	EvidenceRecorded    EvidenceKind = "recorded-ownership"
	EvidenceProcessInfo EvidenceKind = "process-info"
)

// Evidence is one observable fact supporting or contradicting a conclusion.
// Every classification ghostgc reports carries these; a conclusion without
// evidence is a bug.
type Evidence struct {
	Kind   EvidenceKind `json:"kind"`
	Detail string       `json:"detail"`
	// Weight is the independent probability this single fact contributes.
	// Zero means the fact is contextual and adds no confidence on its own.
	Weight float64 `json:"weight,omitempty"`
}

// CombineWeights folds independent evidence weights into a single confidence
// using a noisy-or, and caps the result below 1.0.
//
// The cap matters: no amount of heuristic agreement should be reportable as
// certainty. Only a structurally authoritative identifier could justify 1.0,
// and no such identifier exists yet.
func CombineWeights(ev []Evidence) float64 {
	inverse := 1.0
	for _, e := range ev {
		if e.Weight <= 0 {
			continue
		}
		w := e.Weight
		if w > 0.99 {
			w = 0.99
		}
		inverse *= 1 - w
	}
	c := 1 - inverse
	if c > 0.99 {
		c = 0.99
	}
	return c
}

// SessionMetadata is what an adapter can say about a session beyond its root
// process.
type SessionMetadata struct {
	// SessionID is the agent's own identifier when one is discoverable.
	SessionID string `json:"session_id,omitempty"`
	// WorkingDir is the session root's working directory.
	WorkingDir string `json:"working_dir,omitempty"`
	// RepositoryPath is the enclosing repository root, when there is one.
	RepositoryPath string `json:"repository_path,omitempty"`
	// TTY is the controlling terminal of the session root.
	TTY string `json:"tty,omitempty"`
	// Invocation describes how the agent was launched, for display.
	Invocation string `json:"invocation,omitempty"`
	// Extra carries adapter-specific, already-redacted key/value pairs.
	Extra map[string]string `json:"extra,omitempty"`
}

// AgentRoot is a detected session entry point.
type AgentRoot struct {
	AgentID    string          `json:"agent_id"`
	Process    process.Process `json:"-"`
	Key        process.Key     `json:"-"`
	Metadata   SessionMetadata `json:"metadata"`
	Confidence float64         `json:"confidence"`
	Evidence   []Evidence      `json:"evidence"`
	Conflicts  []Evidence      `json:"conflicts,omitempty"`
}

// Attribution is an adapter's answer to "which session does this process
// belong to".
type Attribution struct {
	AgentID    string      `json:"agent_id,omitempty"`
	SessionID  string      `json:"session_id,omitempty"`
	Confidence float64     `json:"confidence"`
	Evidence   []Evidence  `json:"evidence,omitempty"`
	Conflicts  []Evidence  `json:"conflicts,omitempty"`
	Relation   Relation    `json:"relation,omitempty"`
	RootKey    process.Key `json:"-"`
}

// Attributed reports whether the attribution is strong enough to display.
func (a Attribution) Attributed() bool {
	return a.SessionID != "" && a.Confidence >= ConfidenceAttributable
}

// Relation names how a process is connected to its session.
type Relation string

const (
	RelationRoot       Relation = "root"
	RelationDescendant Relation = "descendant"
	RelationRecorded   Relation = "recorded"
	// RelationEnvironment is a process attributed because its environment
	// carries the agent's own session identifier.
	RelationEnvironment Relation = "environment"
	RelationNone        Relation = ""
)

// ProtectionRule declares a class of process an adapter insists must never be
// terminated automatically.
type ProtectionRule struct {
	ID          string   `json:"id"`
	Reason      string   `json:"reason"`
	ExecNames   []string `json:"exec_names,omitempty"`
	PathSegment string   `json:"path_segment,omitempty"`
	Always      bool     `json:"always,omitempty"`
}

// Graph is the read-only view of the current observation handed to adapters.
type Graph struct {
	Snapshot *process.Snapshot
	Tree     *process.Tree
	// Roots maps root PID to the detected root. It is populated before
	// AttributeProcess is called.
	Roots map[int]AgentRoot
}

// AgentAdapter is implemented once per agent runtime.
type AgentAdapter interface {
	// ID is the stable agent identifier, e.g. "codex".
	ID() string

	// EnvKeys lists the environment variables this adapter needs. The
	// collector extracts only the union of every adapter's keys, so an
	// adapter that asks for nothing costs nothing.
	EnvKeys() []string

	// DetectRootProcesses finds session entry points in a snapshot.
	DetectRootProcesses(ctx context.Context, g Graph) ([]AgentRoot, error)

	// ExtractSessionMetadata describes a session from its root process.
	ExtractSessionMetadata(ctx context.Context, p process.Process) (SessionMetadata, error)

	// AttributeProcess decides which session, if any, owns a process.
	AttributeProcess(ctx context.Context, p process.Process, g Graph) Attribution

	// NativeSessionID returns the agent's own session identifier carried by a
	// process, when it carries one.
	//
	// This is membership evidence and nothing more. Agents expose their
	// session identifier through the environment, and environments are
	// inherited by every descendant, so a match means the process descends
	// from that session — never that the process is the agent itself.
	NativeSessionID(p process.Process) (string, bool)

	// ProtectedPatterns lists classes this adapter refuses to see terminated.
	ProtectedPatterns() []ProtectionRule
}

// Registry holds the enabled adapters in a deterministic order.
type Registry struct {
	adapters []AgentAdapter
}

// NewRegistry builds a registry sorted by adapter id so that iteration order
// is stable across runs and evidence is reproducible.
func NewRegistry(as ...AgentAdapter) *Registry {
	sorted := make([]AgentAdapter, len(as))
	copy(sorted, as)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID() < sorted[j].ID() })
	return &Registry{adapters: sorted}
}

// All returns the registered adapters.
func (r *Registry) All() []AgentAdapter { return r.adapters }

// EnvKeys returns the deduplicated union of every adapter's required
// environment variables.
func (r *Registry) EnvKeys() []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range r.adapters {
		for _, k := range a.EnvKeys() {
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
