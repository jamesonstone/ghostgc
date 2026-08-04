package sessions

import (
	"fmt"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// RelationshipKind names why a process is connected to a session.
//
// Specification section 9 requires a session to be modelled as a graph rather
// than only a process tree, because operating-system reparenting destroys tree
// edges. These are the edge types. Each one is an independent reason a process
// belongs, so losing one does not lose the session.
type RelationshipKind string

const (
	// RelParentChild is the live parent link, recorded only when it is
	// chronologically possible.
	RelParentChild RelationshipKind = "parent-child"
	// RelOriginalParent is who actually created the process, captured at first
	// observation and never rewritten.
	RelOriginalParent RelationshipKind = "original-parent"
	// RelReparented marks the moment a live parent link was lost.
	RelReparented RelationshipKind = "reparenting"
	// RelLaunch connects a session root to the non-agent process that started
	// it: an editor extension host, a shell, a CI runner.
	RelLaunch RelationshipKind = "launch"
	// RelProcessGroup marks shared process-group membership with the root.
	RelProcessGroup RelationshipKind = "process-group"
	// RelTerminal marks a shared controlling terminal or POSIX session.
	//
	// This is an annotation, never ownership. A POSIX session leader is
	// usually the user's interactive shell, so every unrelated command that
	// shell ever ran shares the identifier.
	RelTerminal RelationshipKind = "terminal"
	// RelRepository connects a process to the repository it is working in.
	RelRepository RelationshipKind = "repository"
	// RelEnvironment marks a process whose environment carries the agent's own
	// session identifier.
	RelEnvironment RelationshipKind = "environment"

	// RelSocket and RelFileLock record socket and open-file inspection evidence.
	RelSocket   RelationshipKind = "socket"
	RelFileLock RelationshipKind = "file-lock"
)

// AttributingKinds are the relationship kinds that may establish ownership.
//
// Terminal, process-group and repository edges are context. Two processes
// sharing a terminal says only that a human was at the same keyboard; two
// processes in the same repository says only that a directory is popular.
// Treating either as ownership would attribute half a developer's machine to
// whichever agent happened to be running.
var AttributingKinds = map[RelationshipKind]bool{
	RelParentChild:    true,
	RelOriginalParent: true,
	RelLaunch:         false,
	RelEnvironment:    true,
	RelReparented:     false,
	RelProcessGroup:   false,
	RelTerminal:       false,
	RelRepository:     false,
	RelSocket:         false,
	RelFileLock:       false,
}

// Relationship is one typed edge with the evidence behind it.
type Relationship struct {
	Kind   RelationshipKind `json:"kind"`
	From   process.Key      `json:"-"`
	To     process.Key      `json:"-"`
	Detail string           `json:"detail"`
	AtNs   int64            `json:"at_ns"`
}

// Record converts a relationship into its storage form.
func (r Relationship) Record(sessionID string, nowNs int64) storage.RelationshipRecord {
	to := ""
	if !r.To.Zero() {
		to = r.To.UID()
	}
	return storage.RelationshipRecord{
		SessionID:   sessionID,
		Kind:        string(r.Kind),
		FromProcUID: r.From.UID(),
		ToProcUID:   to,
		Detail:      r.Detail,
		FirstSeenNs: r.AtNs,
		LastSeenNs:  nowNs,
	}
}

// LaunchContext describes the non-agent process that started a session root.
//
// Without it, six Codex servers started by six editor windows and six
// abandoned Codex servers left behind by a crashed script look identical.
type LaunchContext struct {
	Key      process.Key `json:"-"`
	PID      int         `json:"pid,omitempty"`
	Name     string      `json:"name,omitempty"`
	ExecPath string      `json:"exec_path,omitempty"`
	// Observed reports whether the launching process was actually seen. When
	// it is false the session root was already reparented by the time ghostgc
	// first looked, and the launcher is genuinely unknown.
	Observed bool `json:"observed"`
}

// Describe renders the launch context for display and evidence.
func (l LaunchContext) Describe() string {
	if !l.Observed {
		return "unknown: the session root was already reparented when ghostgc first observed it"
	}
	return fmt.Sprintf("%s (pid %d)", l.Name, l.PID)
}

// TransitionEvidence builds the evidence for a state change.
func TransitionEvidence(from, to State, reason string) []adapters.Evidence {
	return []adapters.Evidence{{
		Kind:   adapters.EvidenceProcessInfo,
		Detail: fmt.Sprintf("session state %s -> %s: %s", nonEmptyState(from), to, reason),
	}}
}

func nonEmptyState(s State) State {
	if s == "" {
		return StateUnknown
	}
	return s
}
