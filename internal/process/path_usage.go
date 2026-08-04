package process

// PathUsage is aggregate same-user process evidence for one exact directory.
// Discovered descriptor and working-directory paths never cross the platform
// boundary; only PID-reuse-safe process keys and counts do.
type PathUsage struct {
	Complete           bool  `json:"complete"`
	InspectedProcesses int   `json:"inspected_processes"`
	CWDReferences      int   `json:"cwd_references"`
	OpenVnodes         int   `json:"open_vnodes"`
	ProcessKeys        []Key `json:"process_keys"`
}
