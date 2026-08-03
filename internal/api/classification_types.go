package api

import "github.com/jamesonstone/ghostgc/internal/classification"

// ClassificationView is one API-safe deterministic conclusion.
type ClassificationView struct {
	ID            int64                     `json:"id"`
	ProcUID       string                    `json:"proc_uid"`
	SessionID     string                    `json:"session_id"`
	TsNs          int64                     `json:"ts_ns"`
	ActivityTsNs  int64                     `json:"activity_ts_ns"`
	State         string                    `json:"state"`
	BasisState    string                    `json:"basis_state"`
	Detached      bool                      `json:"detached"`
	SessionEnded  bool                      `json:"session_ended"`
	StableSinceNs int64                     `json:"stable_since_ns"`
	Evidence      []classification.Evidence `json:"evidence"`
}

// ClassificationsResponse backs `ghostgc classifications`.
type ClassificationsResponse struct {
	Classifications []ClassificationView `json:"classifications"`
}
