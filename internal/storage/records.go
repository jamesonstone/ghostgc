package storage

import (
	_ "modernc.org/sqlite"
)

// ProcessRecord is a persisted process row.
type ProcessRecord struct {
	ProcUID      string  `json:"proc_uid"`
	PID          int     `json:"pid"`
	PPID         int     `json:"ppid"`
	OriginalPPID int     `json:"original_ppid"`
	PGID         int     `json:"pgid"`
	SID          int     `json:"sid"`
	UID          int64   `json:"uid"`
	StartTimeNs  int64   `json:"start_time_ns"`
	Comm         string  `json:"comm"`
	ExecPath     string  `json:"exec_path"`
	Cmdline      string  `json:"cmdline"`
	CWD          string  `json:"cwd"`
	TTY          string  `json:"tty"`
	AgentID      string  `json:"agent_id,omitempty"`
	SessionID    string  `json:"session_id,omitempty"`
	Relation     string  `json:"relation,omitempty"`
	Confidence   float64 `json:"attribution_confidence"`
	EvidenceJSON string  `json:"-"`
	FirstSeenNs  int64   `json:"first_seen_ns"`
	LastSeenNs   int64   `json:"last_seen_ns"`
	ExitedAtNs   *int64  `json:"exited_at_ns,omitempty"`

	RepositoryPath string `json:"repository_path,omitempty"`
	// OriginalParentObserved reports whether the daemon actually saw the
	// original parent alive. When it is false, OriginalPPID is only what the
	// kernel reported after reparenting, and must not be presented as the
	// process's creator.
	OriginalParentObserved bool `json:"original_parent_observed"`
}

// ObservationRecord is one time-series sample.
type ObservationRecord struct {
	ProcUID   string
	ScanID    int64
	TsNs      int64
	Status    string
	PPID      int
	CPUTimeNs int64
	RSSBytes  int64
	VSZBytes  int64
	Threads   int
}

// SessionRecord is a persisted session row.
type SessionRecord struct {
	SessionID      string  `json:"session_id"`
	AgentID        string  `json:"agent_id"`
	RootProcUID    string  `json:"root_proc_uid"`
	RootPID        int     `json:"root_pid"`
	State          string  `json:"state"`
	Confidence     float64 `json:"confidence"`
	WorkingDir     string  `json:"working_dir,omitempty"`
	RepositoryPath string  `json:"repository_path,omitempty"`
	TTY            string  `json:"tty,omitempty"`
	Invocation     string  `json:"invocation,omitempty"`
	MetadataJSON   string  `json:"-"`
	EvidenceJSON   string  `json:"-"`
	StartedNs      int64   `json:"started_ns"`
	LastSeenNs     int64   `json:"last_seen_ns"`
	EndedNs        *int64  `json:"ended_ns,omitempty"`

	// NativeSessionID is the agent's own identifier for the session, when it
	// exposes one.
	NativeSessionID string `json:"native_session_id,omitempty"`
	PreviousState   string `json:"previous_state,omitempty"`
	StateChangedNs  int64  `json:"state_changed_ns,omitempty"`

	// Launch context: the nearest ancestor of the root that is not itself an
	// agent process.
	HostProcUID  string `json:"host_proc_uid,omitempty"`
	HostPID      int    `json:"host_pid,omitempty"`
	HostName     string `json:"host_name,omitempty"`
	HostExecPath string `json:"host_exec_path,omitempty"`

	Branch         string `json:"branch,omitempty"`
	RepositoryBusy bool   `json:"repository_busy,omitempty"`
	TerminalSID    int    `json:"terminal_sid,omitempty"`
}

// OwnershipRecord is a durable session/process association.
type OwnershipRecord struct {
	SessionID    string  `json:"session_id"`
	ProcUID      string  `json:"proc_uid"`
	AgentID      string  `json:"agent_id"`
	Relation     string  `json:"relation"`
	Confidence   float64 `json:"confidence"`
	EvidenceJSON string  `json:"-"`
	OriginalPPID int     `json:"original_ppid"`
	// OriginalParentObserved reports whether the original parent was seen
	// alive. See ProcessRecord.
	OriginalParentObserved bool  `json:"original_parent_observed"`
	FirstSeenNs            int64 `json:"first_seen_ns"`
	LastSeenNs             int64 `json:"last_seen_ns"`
}

// RelationshipRecord is one typed edge in a session graph.
type RelationshipRecord struct {
	SessionID   string `json:"session_id"`
	Kind        string `json:"kind"`
	FromProcUID string `json:"from_proc_uid"`
	ToProcUID   string `json:"to_proc_uid,omitempty"`
	Detail      string `json:"detail,omitempty"`
	FirstSeenNs int64  `json:"first_seen_ns"`
	LastSeenNs  int64  `json:"last_seen_ns"`
}

// AuditRecord is one entry in the audit trail.
type AuditRecord struct {
	ID           int64  `json:"id"`
	TsNs         int64  `json:"ts_ns"`
	Kind         string `json:"kind"`
	Subject      string `json:"subject"`
	Summary      string `json:"summary"`
	EvidenceJSON string `json:"evidence,omitempty"`
}

// ScanRecord summarises one observation cycle.
type ScanRecord struct {
	ID                  int64  `json:"id"`
	StartedNs           int64  `json:"started_ns"`
	DurationUs          int64  `json:"duration_us"`
	VisibleProcesses    int    `json:"visible_processes"`
	InspectedProcesses  int    `json:"inspected_processes"`
	AttributedProcesses int    `json:"attributed_processes"`
	Sessions            int    `json:"sessions"`
	Error               string `json:"error,omitempty"`
}
