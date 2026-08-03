package api

import (
	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/classification"
	"github.com/jamesonstone/ghostgc/internal/protection"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// SessionSummary is one row of `ghostgc sessions`.
type SessionSummary struct {
	SessionID       string         `json:"session_id"`
	ShortID         string         `json:"short_id"`
	AgentID         string         `json:"agent_id"`
	Repository      string         `json:"repository"`
	RepositoryPath  string         `json:"repository_path,omitempty"`
	WorkingDir      string         `json:"working_dir,omitempty"`
	State           string         `json:"state"`
	Confidence      float64        `json:"confidence"`
	RootPID         int            `json:"root_pid"`
	TTY             string         `json:"tty,omitempty"`
	AgeSeconds      float64        `json:"age_seconds"`
	Processes       int            `json:"processes"`
	LiveProcesses   int            `json:"live_processes"`
	Classifications map[string]int `json:"classifications"`
	StartedNs       int64          `json:"started_ns"`
	LastSeenNs      int64          `json:"last_seen_ns"`
	EndedNs         *int64         `json:"ended_ns,omitempty"`

	NativeSessionID string `json:"native_session_id,omitempty"`
	PreviousState   string `json:"previous_state,omitempty"`
	StateChangedNs  int64  `json:"state_changed_ns,omitempty"`
	Branch          string `json:"branch,omitempty"`
	RepositoryBusy  bool   `json:"repository_busy,omitempty"`
	TerminalSID     int    `json:"terminal_sid,omitempty"`

	// LaunchedBy describes the non-agent process that started the session
	// root: an editor extension host, a shell, a CI runner.
	LaunchedBy     string `json:"launched_by,omitempty"`
	LaunchedByPID  int    `json:"launched_by_pid,omitempty"`
	LaunchedByPath string `json:"launched_by_path,omitempty"`
}

// RelationshipView is one edge of a session graph.
type RelationshipView struct {
	Kind    string `json:"kind"`
	From    string `json:"from"`
	FromPID int    `json:"from_pid"`
	To      string `json:"to,omitempty"`
	ToPID   int    `json:"to_pid,omitempty"`
	Detail  string `json:"detail"`
	// Attributing reports whether this kind of edge may establish ownership.
	// Terminal, process-group and repository edges are context only.
	Attributing bool  `json:"attributing"`
	FirstSeenNs int64 `json:"first_seen_ns"`
	LastSeenNs  int64 `json:"last_seen_ns"`
}

// SessionsResponse backs `ghostgc sessions`.
type SessionsResponse struct {
	Sessions []SessionSummary `json:"sessions"`
	Note     string           `json:"note,omitempty"`
}

// ProcessSummary is one row of `ghostgc processes`.
type ProcessSummary struct {
	ProcUID        string   `json:"proc_uid"`
	PID            int      `json:"pid"`
	PPID           int      `json:"ppid"`
	OriginalPPID   int      `json:"original_ppid"`
	Name           string   `json:"name"`
	ExecPath       string   `json:"exec_path,omitempty"`
	Cmdline        []string `json:"cmdline,omitempty"`
	CWD            string   `json:"cwd,omitempty"`
	TTY            string   `json:"tty,omitempty"`
	AgentID        string   `json:"agent_id,omitempty"`
	SessionID      string   `json:"session_id,omitempty"`
	ShortID        string   `json:"short_session_id,omitempty"`
	Relation       string   `json:"relation,omitempty"`
	Confidence     float64  `json:"confidence"`
	RepositoryPath string   `json:"repository_path,omitempty"`
	// OriginalParentObserved reports whether the creator was actually seen.
	OriginalParentObserved bool    `json:"original_parent_observed"`
	State                  string  `json:"state"`
	AgeSeconds             float64 `json:"age_seconds"`
	RSSBytes               uint64  `json:"rss_bytes,omitempty"`
	CPUSeconds             float64 `json:"cpu_seconds,omitempty"`
	Threads                int     `json:"threads,omitempty"`
	Live                   bool    `json:"live"`
	ActivityState          string  `json:"activity_state,omitempty"`
	Detached               bool    `json:"detached"`
	ClassificationTsNs     int64   `json:"classification_ts_ns,omitempty"`
}

// ProcessesResponse backs `ghostgc processes`.
type ProcessesResponse struct {
	Processes []ProcessSummary `json:"processes"`
	Note      string           `json:"note,omitempty"`
}

// SessionDetail backs `ghostgc session show`.
type SessionDetail struct {
	Session       SessionSummary        `json:"session"`
	Evidence      []adapters.Evidence   `json:"evidence,omitempty"`
	Processes     []ProcessSummary      `json:"processes"`
	Relationships []RelationshipView    `json:"relationships,omitempty"`
	Audit         []storage.AuditRecord `json:"audit,omitempty"`
}

// ExplainResponse backs `ghostgc explain`.
type ExplainResponse struct {
	Found            bool                      `json:"found"`
	PID              int                       `json:"pid"`
	ProcUID          string                    `json:"proc_uid,omitempty"`
	Name             string                    `json:"name,omitempty"`
	ExecPath         string                    `json:"exec_path,omitempty"`
	Cmdline          []string                  `json:"cmdline,omitempty"`
	Classification   string                    `json:"classification"`
	ActivityState    string                    `json:"activity_state,omitempty"`
	Detached         bool                      `json:"detached"`
	ActivityEvidence []classification.Evidence `json:"activity_evidence,omitempty"`
	AgentID          string                    `json:"agent_id,omitempty"`
	SessionID        string                    `json:"session_id,omitempty"`
	SessionState     string                    `json:"session_state,omitempty"`
	Relation         string                    `json:"relation,omitempty"`
	Confidence       float64                   `json:"confidence"`
	ParentLink       string                    `json:"parent_link"`
	OriginalPPID     int                       `json:"original_ppid,omitempty"`
	// OriginalParentObserved reports whether the creator was actually seen. When
	// false, OriginalPPID says nothing about who created the process.
	OriginalParentObserved bool   `json:"original_parent_observed"`
	RepositoryPath         string `json:"repository_path,omitempty"`
	// EnvironmentReadable reports whether the operating system let ghostgc see
	// this process's environment. When it is false, the absence of agent
	// variables proves nothing.
	EnvironmentReadable bool                `json:"environment_readable"`
	Relationships       []RelationshipView  `json:"relationships,omitempty"`
	Descendants         []int               `json:"descendants,omitempty"`
	Evidence            []adapters.Evidence `json:"evidence,omitempty"`
	Conflicts           []adapters.Evidence `json:"conflicts,omitempty"`
	Protection          protection.Result   `json:"protection"`
	PolicyNote          string              `json:"policy_note"`
	Message             string              `json:"message,omitempty"`
}
