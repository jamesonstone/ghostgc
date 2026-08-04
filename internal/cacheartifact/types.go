// Package cacheartifact defines cache identities, lifecycle and durable evidence.
package cacheartifact

// Lifecycle is the current fail-closed artifact state.
type Lifecycle string

const (
	StateObserved       Lifecycle = "observed"
	StateProtected      Lifecycle = "protected"
	StateSettling       Lifecycle = "settling"
	StateStaleCandidate Lifecycle = "stale_candidate"
	StateRecommended    Lifecycle = "recommended"
	StateQuarantined    Lifecycle = "quarantined"
	StateRestored       Lifecycle = "restored"
	StatePurged         Lifecycle = "purged"
	StatePartial        Lifecycle = "partial"
	StateFailed         Lifecycle = "failed"
)

const (
	ProviderCodexShellSnapshot = "codex-shell-snapshot-v1"
	AgentCodex                 = "codex"
	KindShellSnapshot          = "shell-snapshot"
	QuarantineDirectory        = ".ghostgc-quarantine"
)

// Artifact is the current projection of one exact cache entry.
type Artifact struct {
	ID               string    `json:"artifact_id"`
	Provider         string    `json:"provider"`
	Agent            string    `json:"agent"`
	SessionID        string    `json:"session_id"`
	Kind             string    `json:"artifact_kind"`
	RootPath         string    `json:"root_path"`
	RelativePath     string    `json:"relative_path"`
	Identity         Identity  `json:"identity"`
	RootIdentity     Identity  `json:"root_identity"`
	IdentityDigest   string    `json:"identity_digest"`
	ManifestDigest   string    `json:"manifest_digest"`
	FirstObservedNs  int64     `json:"first_observed_ns"`
	LastObservedNs   int64     `json:"last_observed_ns"`
	StableSinceNs    int64     `json:"stable_since_ns"`
	Lifecycle        Lifecycle `json:"lifecycle"`
	Reason           string    `json:"reason"`
	Evidence         []string  `json:"evidence"`
	Configuration    string    `json:"configuration_digest"`
	EvaluationID     int64     `json:"evaluation_id,omitempty"`
	PolicyID         string    `json:"policy_id,omitempty"`
	QuarantinePath   string    `json:"quarantine_path,omitempty"`
	QuarantinedAtNs  int64     `json:"quarantined_at_ns,omitempty"`
	QuarantineDigest string    `json:"quarantine_manifest_digest,omitempty"`
}

// Observation is one committed metadata-only sample.
type Observation struct {
	ArtifactID     string    `json:"artifact_id"`
	ObservedNs     int64     `json:"observed_ns"`
	IdentityDigest string    `json:"identity_digest"`
	ManifestDigest string    `json:"manifest_digest"`
	Lifecycle      Lifecycle `json:"lifecycle"`
	Complete       bool      `json:"complete"`
	Evidence       []string  `json:"evidence"`
}

// Evaluation records one bounded cache scan and configuration.
type Evaluation struct {
	ID                  int64  `json:"evaluation_id"`
	ObservedNs          int64  `json:"observed_ns"`
	ConfigurationDigest string `json:"configuration_digest"`
	Complete            bool   `json:"complete"`
	Inspected           int    `json:"inspected"`
	Protected           int    `json:"protected"`
	Candidates          int    `json:"candidates"`
	Error               string `json:"error,omitempty"`
}

// Decision is one exact policy result in an evaluation.
type Decision struct {
	ID           int64    `json:"decision_id"`
	EvaluationID int64    `json:"evaluation_id"`
	ArtifactID   string   `json:"artifact_id"`
	PolicyID     string   `json:"policy_id"`
	Result       string   `json:"result"`
	Reason       string   `json:"reason"`
	Evidence     []string `json:"evidence"`
}

// Action is durable evidence around one cache side effect.
type Action struct {
	ID          int64    `json:"id"`
	ActionID    string   `json:"action_id"`
	ArtifactID  string   `json:"artifact_id"`
	Kind        string   `json:"kind"`
	PolicyID    string   `json:"policy_id,omitempty"`
	RequestedNs int64    `json:"requested_ns"`
	UpdatedNs   int64    `json:"updated_ns"`
	Result      string   `json:"result"`
	Reason      string   `json:"reason"`
	Evidence    []string `json:"evidence"`
}

// Quarantine is the durable reversible location of one exact artifact.
type Quarantine struct {
	ArtifactID       string   `json:"artifact_id"`
	RootPath         string   `json:"root_path"`
	OriginalPath     string   `json:"original_path"`
	QuarantinePath   string   `json:"quarantine_path"`
	Identity         Identity `json:"identity"`
	ManifestDigest   string   `json:"manifest_digest"`
	QuarantinedNs    int64    `json:"quarantined_ns"`
	GraceUntilNs     int64    `json:"grace_until_ns"`
	Status           string   `json:"status"`
	UpdatedNs        int64    `json:"updated_ns"`
	Configuration    string   `json:"configuration_digest"`
	OriginalManifest string   `json:"original_manifest_digest"`
}
