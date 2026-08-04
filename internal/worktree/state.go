package worktree

import "time"

// Classify advances the continuous-inactivity state machine.
func Classify(previous Record, obs Observation, now, daemonStarted time.Time,
	staleAfter, scanInterval time.Duration, active bool, complete bool) Conclusion {
	protection := append([]string(nil), obs.Protection...)
	lastActivity := previous.LastActivity
	if lastActivity.IsZero() {
		lastActivity = now
	}
	reset := func(state State) Conclusion {
		return Conclusion{State: state, LastActivity: now, Protection: protection}
	}
	if !obs.Present || obs.Prunable {
		return reset(StateMissing)
	}
	if !complete || !obs.Complete || !obs.Canonical {
		return reset(StateUnknown)
	}
	if active {
		return reset(StateActive)
	}
	if len(protection) > 0 {
		return reset(StateProtected)
	}
	continuousState := previous.State == StateObserving || previous.State == StateStale
	continuous := continuousState && previous.ID == obs.ID && previous.DaemonStarted.Equal(daemonStarted) &&
		!previous.LastSeen.IsZero() && !now.Before(previous.LastSeen) &&
		now.Sub(previous.LastSeen) <= 2*scanInterval && previous.HEAD == obs.HEAD &&
		previous.Ref == obs.Ref && previous.StatusFingerprint == obs.Status.Fingerprint
	if !continuous {
		return Conclusion{State: StateObserving, LastActivity: now, InactiveSince: now}
	}
	inactiveSince := previous.InactiveSince
	if inactiveSince.IsZero() || now.Before(inactiveSince) {
		inactiveSince = now
	}
	state := StateObserving
	if now.Sub(inactiveSince) >= staleAfter {
		state = StateStale
	}
	return Conclusion{State: state, LastActivity: lastActivity, InactiveSince: inactiveSince}
}
