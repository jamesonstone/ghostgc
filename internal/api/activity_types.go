package api

import "github.com/jamesonstone/ghostgc/internal/storage"

// ActivityResponse backs `ghostgc activity`.
type ActivityResponse struct {
	Samples []storage.ActivityRecord `json:"samples"`
	Note    string                   `json:"note,omitempty"`
}
