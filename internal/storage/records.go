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

// ActivityRecord is one phase-3 targeted activity sample. Each availability
// flag is independent so unavailable evidence can never masquerade as zero.
type ActivityRecord struct {
	ID         int64  `json:"id"`
	ProcUID    string `json:"proc_uid"`
	SessionID  string `json:"session_id"`
	TsNs       int64  `json:"ts_ns"`
	IntervalNs int64  `json:"interval_ns"`
	BaselineOK bool   `json:"baseline_ok"`

	CPUPercent float64 `json:"cpu_percent"`
	CPUDeltaNs int64   `json:"cpu_delta_ns"`
	CPUKnown   bool    `json:"cpu_known"`

	DiskReadBytes    int64 `json:"disk_read_bytes"`
	DiskWrittenBytes int64 `json:"disk_written_bytes"`
	IOKnown          bool  `json:"io_known"`
	RSSBytes         int64 `json:"rss_bytes"`

	OpenFiles               int  `json:"open_files"`
	WritableRepositoryFiles int  `json:"writable_repository_files"`
	FilesKnown              bool `json:"files_known"`

	Sockets           int    `json:"sockets"`
	ConnectedSockets  int    `json:"connected_sockets"`
	ReceiveQueueBytes int64  `json:"receive_queue_bytes"`
	SendQueueBytes    int64  `json:"send_queue_bytes"`
	NetworkChanged    bool   `json:"network_changed"`
	SocketsKnown      bool   `json:"sockets_known"`
	Note              string `json:"note,omitempty"`
}

// ClassificationRecord is one deterministic conclusion over an activity
// sample. State is intentionally independent of session lifecycle state.
type ClassificationRecord struct {
	ID            int64  `json:"id"`
	ProcUID       string `json:"proc_uid"`
	SessionID     string `json:"session_id"`
	TsNs          int64  `json:"ts_ns"`
	ActivityTsNs  int64  `json:"activity_ts_ns"`
	State         string `json:"state"`
	BasisState    string `json:"basis_state"`
	Detached      bool   `json:"detached"`
	SessionEnded  bool   `json:"session_ended"`
	StableSinceNs int64  `json:"stable_since_ns"`
	EvidenceJSON  string `json:"evidence"`
}

// PolicyDecisionRecord is one bounded policy match, refusal or cooldown.
type PolicyDecisionRecord struct {
	ID                  int64  `json:"id"`
	EvaluationID        int64  `json:"evaluation_id"`
	PolicyID            string `json:"policy_id"`
	ProcUID             string `json:"proc_uid"`
	SessionID           string `json:"session_id"`
	TsNs                int64  `json:"ts_ns"`
	ClassificationTsNs  int64  `json:"classification_ts_ns"`
	ClassificationState string `json:"classification_state"`
	Result              string `json:"result"`
	Reason              string `json:"reason"`
	CooldownUntilNs     int64  `json:"cooldown_until_ns,omitempty"`
	EvidenceJSON        string `json:"-"`
}

// ActionRecord is one durable manual action request and its latest outcome.
type ActionRecord struct {
	ID           int64  `json:"id"`
	ActionID     string `json:"action_id"`
	PolicyID     string `json:"policy_id"`
	ProcUID      string `json:"proc_uid"`
	SessionID    string `json:"session_id"`
	RequestedNs  int64  `json:"requested_ns"`
	UpdatedNs    int64  `json:"updated_ns"`
	Result       string `json:"result"`
	Signal       string `json:"signal"`
	Reason       string `json:"reason"`
	EvidenceJSON string `json:"evidence"`
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
