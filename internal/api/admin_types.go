package api

import (
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// CandidateEntry is one policy match. None can exist in this delivery phase.
type CandidateEntry struct {
	PID      int    `json:"pid"`
	PolicyID string `json:"policy_id"`
	Result   string `json:"result"`
	Reason   string `json:"reason"`
	Command  string `json:"would_execute,omitempty"`
}

// CandidatesResponse backs `ghostgc candidates`.
type CandidatesResponse struct {
	Enforceable []CandidateEntry `json:"enforceable"`
	Audited     []CandidateEntry `json:"audited"`
	Note        string           `json:"note"`
}

// PolicySummary describes a loaded policy.
type PolicySummary struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Mode        string `json:"mode"`
}

// PoliciesResponse backs `ghostgc policies`.
type PoliciesResponse struct {
	GlobalMode string          `json:"global_mode"`
	Policies   []PolicySummary `json:"policies"`
	Note       string          `json:"note"`
}

// LogsResponse backs `ghostgc logs`.
type LogsResponse struct {
	Entries []storage.AuditRecord `json:"entries"`
}

// DoctorCheck is one diagnostic result.
type DoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Remedy string `json:"remedy,omitempty"`
}

// DoctorResponse backs `ghostgc doctor`.
type DoctorResponse struct {
	Checks []DoctorCheck `json:"checks"`
	OK     bool          `json:"ok"`
}

// MetricsResponse backs `ghostgc metrics` and the observability requirements.
type MetricsResponse struct {
	ScanCount            int64          `json:"scan_count"`
	ScanFailures         int64          `json:"scan_failures"`
	LastScanDurationMs   float64        `json:"last_scan_duration_ms"`
	MeanScanDurationMs   float64        `json:"mean_scan_duration_ms"`
	MaxScanDurationMs    float64        `json:"max_scan_duration_ms"`
	LastReconcileMs      float64        `json:"last_reconcile_ms"`
	LastPersistMs        float64        `json:"last_persist_ms"`
	VisibleProcesses     int            `json:"visible_processes"`
	InspectedProcesses   int            `json:"inspected_processes"`
	AttributedProcesses  int            `json:"attributed_processes"`
	ActiveSessions       int            `json:"active_sessions"`
	SuspiciousSessions   int            `json:"suspicious_sessions"`
	CleanupCandidates    int            `json:"cleanup_candidates"`
	ActionsAttempted     int64          `json:"actions_attempted"`
	ActionsRejected      int64          `json:"actions_rejected"`
	ActionsCompleted     int64          `json:"actions_completed"`
	DatabaseBytes        int64          `json:"database_bytes"`
	DatabaseCounts       storage.Counts `json:"database_counts"`
	RSSBytes             uint64         `json:"rss_bytes"`
	Goroutines           int            `json:"goroutines"`
	RetentionRuns        int64          `json:"retention_runs"`
	LastRetentionDeleted int64          `json:"last_retention_deleted"`
}
