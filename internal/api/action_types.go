package api

import (
	"encoding/json"

	"github.com/jamesonstone/ghostgc/internal/policy"
)

// CleanupPreviewRequest selects one exact current recommendation.
type CleanupPreviewRequest struct {
	PolicyID string `json:"policy_id"`
	ProcUID  string `json:"proc_uid"`
}

// CleanupPreviewResponse carries short-lived, single-use manual authority.
type CleanupPreviewResponse struct {
	Approval     string         `json:"approval"`
	ExpiresNs    int64          `json:"expires_ns"`
	Candidate    CandidateEntry `json:"candidate"`
	Signal       string         `json:"signal"`
	Command      string         `json:"command"`
	Revalidation []string       `json:"revalidation"`
	Note         string         `json:"note"`
}

// CleanupApplyRequest consumes one approval.
type CleanupApplyRequest struct {
	Approval string `json:"approval"`
}

// CleanupApplyResponse reports the durable outcome of one request.
type CleanupApplyResponse struct {
	ActionID  string          `json:"action_id"`
	Authority string          `json:"authority"`
	PolicyID  string          `json:"policy_id"`
	ProcUID   string          `json:"proc_uid"`
	Result    string          `json:"result"`
	Signal    string          `json:"signal"`
	AtNs      int64           `json:"at_ns"`
	Reason    string          `json:"reason"`
	Evidence  json.RawMessage `json:"evidence"`
}

// ActionOptions narrows durable action history.
type ActionOptions struct {
	ProcUID  string
	PolicyID string
	Result   string
	Limit    int
}

// ActionsResponse backs `ghostgc actions`.
type ActionsResponse struct {
	Actions []ActionView `json:"actions"`
}

// ActionView exposes structured action evidence instead of encoded JSON text.
type ActionView struct {
	ActionID    string            `json:"action_id"`
	Authority   string            `json:"authority"`
	PolicyID    string            `json:"policy_id"`
	ProcUID     string            `json:"proc_uid"`
	SessionID   string            `json:"session_id"`
	RequestedNs int64             `json:"requested_ns"`
	UpdatedNs   int64             `json:"updated_ns"`
	Result      string            `json:"result"`
	Signal      string            `json:"signal"`
	Reason      string            `json:"reason"`
	Evidence    []policy.Evidence `json:"evidence"`
}
