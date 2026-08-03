// Package classification turns activity evidence into deterministic state.
// It does not evaluate protection or policy and cannot authorise an action.
package classification

import (
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/process"
)

// State is one activity conclusion for an exact process.
type State string

const (
	StateActive     State = "active"
	StateWaiting    State = "waiting"
	StateIdle       State = "idle"
	StateSuspicious State = "suspicious"
	StateHung       State = "hung"
	StateCrashed    State = "crashed"
	StateOrphaned   State = "orphaned"
	StateUnknown    State = "unknown"
)

// StrongConclusionWindow is the minimum continuous evidence required for a
// hung or orphaned conclusion.
const StrongConclusionWindow = 5 * time.Minute

// Evidence is a persistable explanation of one classification.
type Evidence struct {
	Rule   string `json:"rule"`
	Detail string `json:"detail"`
}

// Activity is the subset of a Phase 3 sample used by the classifier.
type Activity struct {
	Taken                   time.Time
	BaselineOK              bool
	CPUPercent              float64
	CPUKnown                bool
	DiskReadBytes           int64
	DiskWrittenBytes        int64
	IOKnown                 bool
	WritableRepositoryFiles int
	FilesKnown              bool
	ConnectedSockets        int
	NetworkChanged          bool
	SocketsKnown            bool
}

// Previous is the prior committed result for the exact process.
type Previous struct {
	Key           process.Key
	Basis         State
	Detached      bool
	SessionEnded  bool
	ProcessStatus process.Status
	StableSince   time.Time
}

// Input contains only observations, never policy configuration.
type Input struct {
	Key          process.Key
	Status       process.Status
	Detached     bool
	SessionEnded bool
	Activity     Activity
	Previous     Previous
}

// Result contains the visible state and its unmodified basis state.
type Result struct {
	State        State
	Basis        State
	Detached     bool
	SessionEnded bool
	StableSince  time.Time
	Evidence     []Evidence
}

// Classify deterministically evaluates one current sample.
func Classify(in Input) Result {
	result := Result{
		State: StateUnknown, Basis: StateUnknown, Detached: in.Detached,
		SessionEnded: in.SessionEnded, StableSince: in.Activity.Taken,
	}
	if in.Status == process.StatusZombie {
		result.State, result.Basis = StateCrashed, StateCrashed
		result.Evidence = []Evidence{{Rule: "kernel-zombie-v1", Detail: "the kernel reported zombie state"}}
		return result
	}
	if reason := incompleteReason(in.Activity); reason != "" {
		result.Evidence = []Evidence{{Rule: "activity-unknown-v1", Detail: reason}}
		return result
	}

	result.Basis, result.Evidence = basis(in.Activity)
	if sameWindow(in) {
		result.StableSince = in.Previous.StableSince
	}
	stableFor := in.Activity.Taken.Sub(result.StableSince)
	result.State = result.Basis

	switch {
	case in.SessionEnded && in.Detached && (result.Basis == StateActive || result.Basis == StateWaiting):
		result.State = StateSuspicious
		result.Evidence = append(result.Evidence, Evidence{Rule: "post-session-work-v1", Detail: "a detached process retained activity or live resources after its session ended"})
	case in.SessionEnded && in.Detached && result.Basis == StateIdle && stableFor >= StrongConclusionWindow:
		result.State = StateOrphaned
		result.Evidence = append(result.Evidence, Evidence{Rule: "stable-orphan-v1", Detail: fmt.Sprintf("detached known-idle evidence remained stable for %s", stableFor)})
	case in.Status == process.StatusStopped &&
		(result.Basis == StateIdle || result.Basis == StateWaiting) && stableFor >= StrongConclusionWindow:
		result.State = StateHung
		result.Evidence = append(result.Evidence, Evidence{Rule: "stable-stopped-v1", Detail: fmt.Sprintf("stopped known-inactive evidence remained stable for %s", stableFor)})
	}
	return result
}

func incompleteReason(a Activity) string {
	switch {
	case a.Taken.IsZero():
		return "activity sample time is unavailable"
	case !a.BaselineOK:
		return "activity has no valid exact-key baseline"
	case !a.CPUKnown:
		return "CPU activity is unavailable"
	case !a.IOKnown:
		return "disk activity is unavailable"
	case !a.FilesKnown:
		return "open-file evidence is unavailable"
	case !a.SocketsKnown:
		return "socket evidence is unavailable"
	default:
		return ""
	}
}

func basis(a Activity) (State, []Evidence) {
	if a.CPUPercent > 0 || a.DiskReadBytes > 0 || a.DiskWrittenBytes > 0 || a.NetworkChanged {
		return StateActive, []Evidence{{Rule: "progress-observed-v1", Detail: "CPU, disk or socket-queue progress was observed"}}
	}
	if a.WritableRepositoryFiles > 0 || a.ConnectedSockets > 0 {
		return StateWaiting, []Evidence{{Rule: "live-resource-wait-v1", Detail: "no progress was observed while writable repository files or connected sockets remained"}}
	}
	return StateIdle, []Evidence{{Rule: "complete-inactivity-v1", Detail: "complete CPU, disk, file and socket evidence showed no progress or live resource hold"}}
}

func sameWindow(in Input) bool {
	p := in.Previous
	return p.Key == in.Key && !p.StableSince.IsZero() && p.Basis != StateUnknown &&
		p.Basis == basisState(in.Activity) && p.Detached == in.Detached &&
		p.SessionEnded == in.SessionEnded && p.ProcessStatus == in.Status
}

func basisState(a Activity) State {
	state, _ := basis(a)
	return state
}
