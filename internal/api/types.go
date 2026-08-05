// Package api defines the local control interface: the request and response
// types, the Unix-domain-socket server, and the client the CLI uses.
//
// The socket is the only interface. No TCP port is opened, by default or
// otherwise, and the socket is created with owner-only permissions.
package api

import (
	"context"
)

// APIVersion is the path prefix for every endpoint.
const APIVersion = "v1"

// Health is the daemon's self-reported condition.
type Health string

const (
	HealthHealthy  Health = "healthy"
	HealthDegraded Health = "degraded"
	HealthStarting Health = "starting"
)

// StatusResponse backs `ghostgc status`.
type StatusResponse struct {
	Health                 Health         `json:"health"`
	Mode                   string         `json:"mode"`
	Version                string         `json:"version"`
	Platform               string         `json:"platform"`
	PID                    int            `json:"pid"`
	StartedNs              int64          `json:"started_ns"`
	UptimeSeconds          float64        `json:"uptime_seconds"`
	Agents                 []string       `json:"agents"`
	SessionsByState        map[string]int `json:"sessions_by_state"`
	ClassificationsByState map[string]int `json:"classifications_by_state"`
	Sessions               int            `json:"sessions"`
	CleanupCandidates      int            `json:"cleanup_candidates"`
	// SignallingEnabled is the compatibility alias for manual cleanup.
	SignallingEnabled       bool           `json:"signalling_enabled"`
	ManualCleanupEnabled    bool           `json:"manual_cleanup_enabled"`
	AutomaticCleanupEnabled bool           `json:"automatic_cleanup_enabled"`
	CacheEnabled            bool           `json:"cache_enabled"`
	CacheMode               string         `json:"cache_mode"`
	CacheCandidates         int            `json:"cache_candidates"`
	CacheQuarantined        int            `json:"cache_quarantined"`
	WorktreesByState        map[string]int `json:"worktrees_by_state"`
	StaleWorktrees          int            `json:"stale_worktrees"`
	ProtectedWorktrees      int            `json:"protected_worktrees"`
	LastScan                *ScanSummary   `json:"last_scan,omitempty"`
	Degraded                []string       `json:"degraded_reasons,omitempty"`
}

// ScanSummary describes the most recent observation cycle.
type ScanSummary struct {
	StartedNs           int64   `json:"started_ns"`
	AgeSeconds          float64 `json:"age_seconds"`
	DurationMs          float64 `json:"duration_ms"`
	VisibleProcesses    int     `json:"visible_processes"`
	InspectedProcesses  int     `json:"inspected_processes"`
	AttributedProcesses int     `json:"attributed_processes"`
	Error               string  `json:"error,omitempty"`
}

// Doctor check statuses.
const (
	CheckOK    = "ok"
	CheckWarn  = "warn"
	CheckError = "error"
)

// ListOptions narrows a listing request.
type ListOptions struct {
	SessionID string
	AgentID   string
	State     string
	Limit     int
	All       bool
}

// LogOptions narrows an audit-log request.
type LogOptions struct {
	Limit   int
	Kind    string
	Subject string
	SinceNs int64
}

// ActivityOptions narrows activity history.
type ActivityOptions struct {
	ProcUID   string
	SessionID string
	SinceNs   int64
	Limit     int
}

// ClassificationOptions narrows deterministic classification history.
type ClassificationOptions struct {
	ProcUID   string
	SessionID string
	State     string
	SinceNs   int64
	Limit     int
	Latest    bool
}

// Backend is what the daemon implements to serve the API. Defining it here
// keeps the transport unaware of the daemon and the daemon unaware of HTTP.
type Backend interface {
	Status(ctx context.Context) (StatusResponse, error)
	Sessions(ctx context.Context, opts ListOptions) (SessionsResponse, error)
	Session(ctx context.Context, idOrPrefix string) (SessionDetail, error)
	Processes(ctx context.Context, opts ListOptions) (ProcessesResponse, error)
	Explain(ctx context.Context, pid int) (ExplainResponse, error)
	Candidates(ctx context.Context) (CandidatesResponse, error)
	Policies(ctx context.Context) (PoliciesResponse, error)
	CleanupPreview(ctx context.Context, req CleanupPreviewRequest) (CleanupPreviewResponse, error)
	CleanupApply(ctx context.Context, req CleanupApplyRequest) (CleanupApplyResponse, error)
	Actions(ctx context.Context, opts ActionOptions) (ActionsResponse, error)
	Logs(ctx context.Context, opts LogOptions) (LogsResponse, error)
	Doctor(ctx context.Context) (DoctorResponse, error)
	Metrics(ctx context.Context) (MetricsResponse, error)
	Activity(ctx context.Context, opts ActivityOptions) (ActivityResponse, error)
	Classifications(ctx context.Context, opts ClassificationOptions) (ClassificationsResponse, error)
	CacheArtifacts(ctx context.Context, opts CacheArtifactOptions) (CacheArtifactsResponse, error)
	CacheArtifact(ctx context.Context, id string) (CacheArtifactResponse, error)
	CacheCandidates(ctx context.Context) (CacheArtifactsResponse, error)
	CacheCleanupPreview(ctx context.Context, req CachePreviewRequest) (CachePreviewResponse, error)
	CacheCleanupApply(ctx context.Context, req CacheApplyRequest) (CacheApplyResponse, error)
	CacheQuarantines(ctx context.Context) (CacheQuarantinesResponse, error)
	CacheRestorePreview(ctx context.Context, req CachePreviewRequest) (CachePreviewResponse, error)
	CacheRestoreApply(ctx context.Context, req CacheApplyRequest) (CacheApplyResponse, error)
	CachePurgePreview(ctx context.Context, req CachePreviewRequest) (CachePreviewResponse, error)
	CachePurgeApply(ctx context.Context, req CacheApplyRequest) (CachePurgePrepareResponse, error)
	CachePurgeComplete(ctx context.Context, req CachePurgeCompleteRequest) (CacheApplyResponse, error)
	CacheActions(ctx context.Context, opts CacheActionOptions) (CacheActionsResponse, error)
	Worktrees(ctx context.Context, opts WorktreeOptions) (WorktreesResponse, error)
	Worktree(ctx context.Context, idOrPrefix string) (WorktreeView, error)
	WorktreeRemovalPreview(ctx context.Context, req WorktreeRemovalPreviewRequest) (WorktreeRemovalPreviewResponse, error)
	WorktreeRemovalApply(ctx context.Context, req WorktreeRemovalApplyRequest) (WorktreeRemovalApplyResponse, error)
	WorktreeRestorePreview(ctx context.Context, req WorktreeRemovalPreviewRequest) (WorktreeRemovalPreviewResponse, error)
	WorktreeRestoreApply(ctx context.Context, req WorktreeRemovalApplyRequest) (WorktreeRemovalApplyResponse, error)
	WorktreePurgePreview(ctx context.Context, req WorktreeRemovalPreviewRequest) (WorktreeRemovalPreviewResponse, error)
	WorktreePurgeApply(ctx context.Context, req WorktreeRemovalApplyRequest) (WorktreePurgePrepareResponse, error)
	WorktreePurgeComplete(ctx context.Context, req WorktreePurgeCompleteRequest) (WorktreeRemovalApplyResponse, error)
	WorktreeActions(ctx context.Context, opts WorktreeActionOptions) (WorktreeActionsResponse, error)
}

// ErrorResponse is returned for any non-200 status.
type ErrorResponse struct {
	Error string `json:"error"`
}
