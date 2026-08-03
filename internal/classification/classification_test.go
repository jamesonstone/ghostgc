package classification

import (
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/process"
)

var (
	classKey = process.Key{PID: 42, StartTimeNs: 100}
	classT0  = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
)

func complete() Activity {
	return Activity{Taken: classT0, BaselineOK: true, CPUKnown: true, IOKnown: true, FilesKnown: true, SocketsKnown: true}
}

func TestClassifyImmediateStates(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want State
	}{
		{name: "unknown baseline", in: Input{Key: classKey, Activity: Activity{Taken: classT0}}, want: StateUnknown},
		{name: "active CPU", in: withActivity(func(a *Activity) { a.CPUPercent = 0.1 }), want: StateActive},
		{name: "active disk", in: withActivity(func(a *Activity) { a.DiskWrittenBytes = 1 }), want: StateActive},
		{name: "waiting file", in: withActivity(func(a *Activity) { a.WritableRepositoryFiles = 1 }), want: StateWaiting},
		{name: "waiting socket", in: withActivity(func(a *Activity) { a.ConnectedSockets = 1 }), want: StateWaiting},
		{name: "idle", in: withActivity(func(a *Activity) {}), want: StateIdle},
		{name: "crashed", in: Input{Key: classKey, Status: process.StatusZombie}, want: StateCrashed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.in); got.State != tt.want || len(got.Evidence) == 0 {
				t.Fatalf("Classify() = %+v, want %s with evidence", got, tt.want)
			}
		})
	}
}

func TestDetachedIsIndependentAndPostSessionWorkIsSuspicious(t *testing.T) {
	in := withActivity(func(a *Activity) { a.CPUPercent = 1 })
	in.Detached = true
	if got := Classify(in); got.State != StateActive || !got.Detached {
		t.Fatalf("a detached working process is still active: %+v", got)
	}
	in.SessionEnded = true
	if got := Classify(in); got.State != StateSuspicious || !got.Detached {
		t.Fatalf("post-session detached work = %+v, want suspicious and detached", got)
	}
}

func TestStrongStatesRequireFiveContinuousMinutes(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*Input)
		before    State
		after     State
	}{
		{name: "orphaned", configure: func(in *Input) { in.Detached, in.SessionEnded = true, true }, before: StateIdle, after: StateOrphaned},
		{name: "hung", configure: func(in *Input) { in.Status = process.StatusStopped }, before: StateIdle, after: StateHung},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := withActivity(func(a *Activity) {})
			tt.configure(&first)
			got := Classify(first)
			previous := Previous{Key: classKey, Basis: got.Basis, Detached: got.Detached,
				SessionEnded: got.SessionEnded, ProcessStatus: first.Status, StableSince: classT0}

			before := first
			before.Activity.Taken = classT0.Add(StrongConclusionWindow - time.Nanosecond)
			before.Previous = previous
			if got := Classify(before); got.State != tt.before {
				t.Fatalf("before window = %s, want %s", got.State, tt.before)
			}
			after := before
			after.Activity.Taken = classT0.Add(StrongConclusionWindow)
			if got := Classify(after); got.State != tt.after {
				t.Fatalf("at window = %s, want %s", got.State, tt.after)
			}
		})
	}
}

func TestEvidenceGapAndIdentityChangeResetStableWindow(t *testing.T) {
	in := withActivity(func(a *Activity) {})
	in.Detached, in.SessionEnded = true, true
	in.Activity.Taken = classT0.Add(10 * time.Minute)
	in.Previous = Previous{Key: classKey, Basis: StateIdle, Detached: true,
		SessionEnded: true, StableSince: classT0}

	reused := in
	reused.Key.StartTimeNs++
	if got := Classify(reused); got.State != StateIdle || got.StableSince != reused.Activity.Taken {
		t.Fatalf("reused PID inherited a strong window: %+v", got)
	}
	missing := in
	missing.Activity.IOKnown = false
	if got := Classify(missing); got.State != StateUnknown {
		t.Fatalf("missing evidence = %s, want unknown", got.State)
	}
}

func withActivity(change func(*Activity)) Input {
	a := complete()
	change(&a)
	return Input{Key: classKey, Status: process.StatusSleeping, Activity: a}
}
