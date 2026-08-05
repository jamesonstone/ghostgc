package api

import "github.com/jamesonstone/ghostgc/internal/cacheartifact"

// CacheArtifactOptions filters cache artifact projections.
type CacheArtifactOptions struct {
	Lifecycle string
	Current   bool
}

// CacheArtifactsResponse backs cache artifact and candidate listings.
type CacheArtifactsResponse struct {
	Artifacts []cacheartifact.Artifact `json:"artifacts"`
	Note      string                   `json:"note"`
}

// CacheArtifactResponse explains one exact artifact.
type CacheArtifactResponse struct {
	Artifact cacheartifact.Artifact `json:"artifact"`
	Note     string                 `json:"note"`
}

// CachePreviewRequest requests authority for one exact artifact action.
type CachePreviewRequest struct {
	ArtifactID string `json:"artifact_id"`
	PolicyID   string `json:"policy_id,omitempty"`
}

// CachePreviewResponse returns one short-lived, single-use approval.
type CachePreviewResponse struct {
	Action       string                    `json:"action"`
	Approval     string                    `json:"approval"`
	ExpiresNs    int64                     `json:"expires_ns"`
	Artifact     cacheartifact.Artifact    `json:"artifact"`
	Quarantine   *cacheartifact.Quarantine `json:"quarantine,omitempty"`
	Destination  string                    `json:"destination"`
	Command      string                    `json:"command"`
	Revalidation []string                  `json:"revalidation"`
	Note         string                    `json:"note"`
}

// CacheApplyRequest consumes one exact approval.
type CacheApplyRequest struct {
	Approval     string `json:"approval"`
	Confirmation string `json:"confirmation,omitempty"`
}

// CacheApplyResponse reports the durable action result.
type CacheApplyResponse struct {
	ActionID   string   `json:"action_id"`
	ArtifactID string   `json:"artifact_id"`
	Action     string   `json:"action"`
	Result     string   `json:"result"`
	Reason     string   `json:"reason"`
	AtNs       int64    `json:"at_ns"`
	Evidence   []string `json:"evidence"`
}

// CachePurgePlan is one exact foreground-only deletion capability.
type CachePurgePlan struct {
	ActionID       string                 `json:"action_id"`
	ArtifactID     string                 `json:"artifact_id"`
	RootPath       string                 `json:"root_path"`
	QuarantinePath string                 `json:"quarantine_path"`
	RootIdentity   cacheartifact.Identity `json:"root_identity"`
	Identity       cacheartifact.Identity `json:"identity"`
	Configuration  string                 `json:"configuration_digest"`
	ExpiresNs      int64                  `json:"expires_ns"`
	Completion     string                 `json:"completion"`
}

// CachePurgePrepareResponse commits intent and returns a foreground plan.
type CachePurgePrepareResponse struct {
	Action CacheApplyResponse `json:"action"`
	Plan   CachePurgePlan     `json:"plan"`
}

// CachePurgeCompleteRequest reports foreground execution for verification.
type CachePurgeCompleteRequest struct {
	ActionID       string `json:"action_id"`
	Completion     string `json:"completion"`
	ExecutionError string `json:"execution_error,omitempty"`
}

// CacheQuarantinesResponse lists reversible quarantined artifacts.
type CacheQuarantinesResponse struct {
	Artifacts []cacheartifact.Quarantine `json:"artifacts"`
	Note      string                     `json:"note"`
}

// CacheActionOptions filters durable cache actions.
type CacheActionOptions struct {
	ArtifactID string
	Kind       string
	Result     string
	Limit      int
}

// CacheActionsResponse lists durable cache actions.
type CacheActionsResponse struct {
	Actions []cacheartifact.Action `json:"actions"`
}
