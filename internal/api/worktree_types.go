package api

import "encoding/json"

// WorktreeOptions narrows inventory.
type WorktreeOptions struct {
	State, Source string
	Limit         int
}

// WorktreesResponse backs `ghostgc worktrees`.
type WorktreesResponse struct {
	Worktrees []WorktreeView `json:"worktrees"`
}

// WorktreeView is one public inventory conclusion.
type WorktreeView struct {
	WorktreeID      string          `json:"worktree_id"`
	ShortID         string          `json:"short_id"`
	Path            string          `json:"path"`
	Branch          string          `json:"branch,omitempty"`
	HEAD            string          `json:"head"`
	Ref             string          `json:"ref,omitempty"`
	Sources         []string        `json:"sources"`
	State           string          `json:"state"`
	FirstSeenNs     int64           `json:"first_seen_ns"`
	LastSeenNs      int64           `json:"last_seen_ns"`
	LastActivityNs  int64           `json:"last_activity_ns"`
	InactiveSinceNs int64           `json:"inactive_since_ns,omitempty"`
	InactiveSeconds float64         `json:"inactive_seconds"`
	Protection      []string        `json:"protection_reasons"`
	Evidence        json.RawMessage `json:"evidence"`
	Complete        bool            `json:"complete"`
	RemovedNs       *int64          `json:"removed_ns,omitempty"`
	RecreateCommand string          `json:"recreate_command,omitempty"`
}

// WorktreeRemovalPreviewRequest selects one exact inventory identity or prefix.
type WorktreeRemovalPreviewRequest struct {
	WorktreeID string `json:"worktree_id"`
}

// WorktreeRemovalPreviewResponse carries ephemeral manual authority.
type WorktreeRemovalPreviewResponse struct {
	Approval     string       `json:"approval"`
	ExpiresNs    int64        `json:"expires_ns"`
	Worktree     WorktreeView `json:"worktree"`
	Command      string       `json:"command"`
	Revalidation []string     `json:"revalidation"`
	Note         string       `json:"note"`
}

// WorktreeRemovalApplyRequest consumes one approval.
type WorktreeRemovalApplyRequest struct {
	Approval string `json:"approval"`
}

// WorktreeRemovalApplyResponse reports one durable result.
type WorktreeRemovalApplyResponse struct {
	ActionID        string          `json:"action_id"`
	WorktreeID      string          `json:"worktree_id"`
	Path            string          `json:"path"`
	Branch          string          `json:"branch,omitempty"`
	Result          string          `json:"result"`
	AtNs            int64           `json:"at_ns"`
	Reason          string          `json:"reason"`
	Evidence        json.RawMessage `json:"evidence"`
	RecreateCommand string          `json:"recreate_command"`
}

// WorktreeActionOptions narrows durable removal history.
type WorktreeActionOptions struct {
	WorktreeID, Result string
	Limit              int
}

// WorktreeActionsResponse backs `ghostgc worktree actions`.
type WorktreeActionsResponse struct {
	Actions []WorktreeActionView `json:"actions"`
}

// WorktreeActionView is one durable removal attempt.
type WorktreeActionView struct {
	ActionID        string          `json:"action_id"`
	WorktreeID      string          `json:"worktree_id"`
	Path            string          `json:"path"`
	Branch          string          `json:"branch,omitempty"`
	RequestedNs     int64           `json:"requested_ns"`
	UpdatedNs       int64           `json:"updated_ns"`
	Result          string          `json:"result"`
	Reason          string          `json:"reason"`
	Evidence        json.RawMessage `json:"evidence"`
	RecreateCommand string          `json:"recreate_command"`
}
