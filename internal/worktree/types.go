// Package worktree discovers and evaluates local Git worktrees without using
// network state or granting automatic removal authority.
package worktree

import "time"

// State is a durable inventory conclusion.
type State string

const (
	StateActive    State = "active"
	StateObserving State = "observing"
	StateStale     State = "stale"
	StateProtected State = "protected"
	StateUnknown   State = "unknown"
	StateMissing   State = "missing"
	StateRetired   State = "retired"
	StateRemoved   State = "removed"
)

// Source identifies how ghostgc gained read-only discovery authority.
type Source string

const (
	SourceSession Source = "session"
	SourceRoot    Source = "configured_root"
)

// FileIdentity binds an absolute path to its current filesystem object.
type FileIdentity struct {
	Path      string `json:"path"`
	Device    uint64 `json:"device"`
	Inode     uint64 `json:"inode"`
	Size      int64  `json:"size"`
	ModTimeNs int64  `json:"mod_time_ns"`
}

// GitIdentity binds authority to the exact Git executable.
type GitIdentity struct {
	FileIdentity
	Digest string `json:"sha256"`
}

// Registration is one entry from `git worktree list --porcelain -z`.
type Registration struct {
	Path         string
	CommonGitDir string
	AdminGitDir  string
	HEAD         string
	Ref          string
	Branch       string
	Detached     bool
	Bare         bool
	Locked       bool
	Prunable     bool
}

// ApprovedLink is the only ignored/untracked material removal may unlink.
type ApprovedLink struct {
	Name     string       `json:"name"`
	LinkText string       `json:"link_text"`
	Target   FileIdentity `json:"target"`
}

// StatusEvidence contains counts only. Paths are discarded after parsing.
type StatusEvidence struct {
	Staged      int    `json:"staged"`
	Tracked     int    `json:"tracked"`
	Conflicted  int    `json:"conflicted"`
	Untracked   int    `json:"untracked"`
	Ignored     int    `json:"ignored"`
	Fingerprint string `json:"fingerprint"`
}

// Clean reports whether no content category contains material.
func (s StatusEvidence) Clean() bool {
	return s.Staged+s.Tracked+s.Conflicted+s.Untracked+s.Ignored == 0
}

// Observation is a complete local evidence snapshot for one registration.
type Observation struct {
	ID                string
	Path              string
	PathIdentity      FileIdentity
	CommonIdentity    FileIdentity
	AdminIdentity     FileIdentity
	CommonGitDir      string
	AdminGitDir       string
	HEAD              string
	Ref               string
	Branch            string
	Detached          bool
	Primary           bool
	Present           bool
	Canonical         bool
	Locked            bool
	Prunable          bool
	Complete          bool
	Status            StatusEvidence
	Operations        []string
	ApprovedLinks     []ApprovedLink
	Published         bool
	DetachedReachable bool
	Submodules        bool
	Protection        []string
	ObservedAt        time.Time
}

// Record is the state needed by the continuous-inactivity transition.
type Record struct {
	ID                string
	State             State
	HEAD              string
	Ref               string
	StatusFingerprint string
	LastSeen          time.Time
	LastActivity      time.Time
	InactiveSince     time.Time
	DaemonStarted     time.Time
}

// Conclusion is one state-machine result.
type Conclusion struct {
	State         State
	LastActivity  time.Time
	InactiveSince time.Time
	Protection    []string
}
