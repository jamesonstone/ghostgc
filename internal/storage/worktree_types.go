package storage

// WorktreeRecord is the durable current inventory or a removal tombstone.
type WorktreeRecord struct {
	WorktreeID        string `json:"worktree_id"`
	Path              string `json:"path"`
	PathDevice        uint64 `json:"path_device"`
	PathInode         uint64 `json:"path_inode"`
	CommonGitDir      string `json:"common_git_dir"`
	AdminGitDir       string `json:"admin_git_dir"`
	HEAD              string `json:"head"`
	Ref               string `json:"ref"`
	Branch            string `json:"branch"`
	SourcesJSON       string `json:"-"`
	State             string `json:"state"`
	FirstSeenNs       int64  `json:"first_seen_ns"`
	LastSeenNs        int64  `json:"last_seen_ns"`
	LastActivityNs    int64  `json:"last_activity_ns"`
	InactiveSinceNs   int64  `json:"inactive_since_ns"`
	DaemonStartedNs   int64  `json:"daemon_started_ns"`
	StatusFingerprint string `json:"-"`
	ProtectionJSON    string `json:"-"`
	EvidenceJSON      string `json:"-"`
	ApprovedLinksJSON string `json:"-"`
	GitIdentityJSON   string `json:"-"`
	Registered        bool   `json:"registered"`
	Complete          bool   `json:"complete"`
	RemovedNs         *int64 `json:"removed_ns,omitempty"`
	RecreateCommand   string `json:"recreate_command,omitempty"`
	OriginalPath      string `json:"original_path,omitempty"`
	RetiredNs         *int64 `json:"retired_ns,omitempty"`
	RetirementGraceNs int64  `json:"retirement_grace_until_ns,omitempty"`
}

// WorktreeActionRecord is one durable manual removal attempt.
type WorktreeActionRecord struct {
	ID              int64  `json:"id"`
	ActionID        string `json:"action_id"`
	WorktreeID      string `json:"worktree_id"`
	Path            string `json:"path"`
	Branch          string `json:"branch"`
	RequestedNs     int64  `json:"requested_ns"`
	UpdatedNs       int64  `json:"updated_ns"`
	Result          string `json:"result"`
	Reason          string `json:"reason"`
	EvidenceJSON    string `json:"-"`
	RecreateCommand string `json:"recreate_command,omitempty"`
}

// WorktreeFilter narrows inventory.
type WorktreeFilter struct {
	State, Source string
	Limit         int
}

// WorktreeActionFilter narrows removal history.
type WorktreeActionFilter struct {
	WorktreeID, Result string
	Limit              int
}
