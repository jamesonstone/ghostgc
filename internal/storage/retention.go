package storage

import (
	"context"
	"fmt"
	"time"
)

// RetentionPolicy bounds how much history is kept.
type RetentionPolicy struct {
	RawObservations time.Duration
	Scans           time.Duration
	Audit           time.Duration
	PolicyDecisions time.Duration
	// ExitedProcesses is how long a process row survives after the process
	// exited. Ownership records for exited processes are removed with them.
	ExitedProcesses time.Duration
	// EndedSessions is how long an ended session is kept.
	EndedSessions time.Duration
	// MaxDatabaseBytes triggers an aggressive pass when exceeded.
	MaxDatabaseBytes int64
}

// RetentionResult reports what a compaction pass removed.
type RetentionResult struct {
	Observations     int64 `json:"observations_deleted"`
	ActivitySamples  int64 `json:"activity_samples_deleted"`
	Classifications  int64 `json:"classifications_deleted"`
	PolicyDecisions  int64 `json:"policy_decisions_deleted"`
	Scans            int64 `json:"scans_deleted"`
	Audit            int64 `json:"audit_deleted"`
	Processes        int64 `json:"processes_deleted"`
	Ownership        int64 `json:"ownership_deleted"`
	Sessions         int64 `json:"sessions_deleted"`
	Relationships    int64 `json:"relationships_deleted"`
	Aggressive       bool  `json:"aggressive"`
	SizeBeforeBytes  int64 `json:"size_before_bytes"`
	SizeAfterBytes   int64 `json:"size_after_bytes"`
	OverBudgetBefore bool  `json:"over_budget_before"`
}

// Total returns the number of rows removed.
func (r RetentionResult) Total() int64 {
	return r.Observations + r.ActivitySamples + r.Classifications + r.PolicyDecisions + r.Scans + r.Audit + r.Processes + r.Ownership + r.Sessions + r.Relationships
}

// Compact enforces the retention policy.
//
// When the database is over its size budget the retention windows are halved
// and the pass runs again. Storage growth is bounded by construction rather
// than by hoping the defaults were generous enough.
func (s *Store) Compact(ctx context.Context, p RetentionPolicy, now time.Time) (RetentionResult, error) {
	res := RetentionResult{SizeBeforeBytes: s.SizeBytes()}
	res.OverBudgetBefore = p.MaxDatabaseBytes > 0 && res.SizeBeforeBytes > p.MaxDatabaseBytes

	if err := s.compactOnce(ctx, p, now, &res); err != nil {
		return res, err
	}

	if res.OverBudgetBefore {
		res.Aggressive = true
		tighter := p
		tighter.RawObservations /= 2
		tighter.Scans /= 2
		tighter.Audit /= 2
		tighter.PolicyDecisions /= 2
		tighter.ExitedProcesses /= 2
		tighter.EndedSessions /= 2
		if err := s.compactOnce(ctx, tighter, now, &res); err != nil {
			return res, err
		}
	}

	// VACUUM cannot run inside a transaction and reclaims the space that the
	// deletes above only marked free.
	if res.Total() > 0 {
		if _, err := s.db.ExecContext(ctx, `VACUUM`); err != nil {
			return res, fmt.Errorf("storage: vacuum: %w", err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return res, fmt.Errorf("storage: wal checkpoint: %w", err)
	}

	res.SizeAfterBytes = s.SizeBytes()
	return res, nil
}

func (s *Store) compactOnce(ctx context.Context, p RetentionPolicy, now time.Time, res *RetentionResult) error {
	cutoff := func(d time.Duration) int64 {
		if d <= 0 {
			d = time.Hour
		}
		return now.Add(-d).UnixNano()
	}

	return s.WithTx(ctx, func(t *Tx) error {
		steps := []struct {
			dst *int64
			q   string
			arg int64
		}{
			{&res.Observations, `DELETE FROM process_observations WHERE ts_ns < ?`, cutoff(p.RawObservations)},
			{&res.ActivitySamples, `DELETE FROM process_activity WHERE ts_ns < ?`, cutoff(p.RawObservations)},
			{&res.Classifications, `DELETE FROM process_classifications WHERE ts_ns < ?`, cutoff(p.RawObservations)},
			{&res.PolicyDecisions, `DELETE FROM policy_decisions WHERE ts_ns < ?`, cutoff(p.PolicyDecisions)},
			{&res.Scans, `DELETE FROM scans WHERE started_ns < ?`, cutoff(p.Scans)},
			{&res.Audit, `DELETE FROM audit_log WHERE ts_ns < ?`, cutoff(p.Audit)},
			{&res.Ownership, `DELETE FROM session_processes WHERE proc_uid IN (
				SELECT proc_uid FROM processes WHERE exited_at_ns IS NOT NULL AND exited_at_ns < ?)`, cutoff(p.ExitedProcesses)},
			{&res.Processes, `DELETE FROM processes WHERE exited_at_ns IS NOT NULL AND exited_at_ns < ?`, cutoff(p.ExitedProcesses)},
			{&res.Sessions, `DELETE FROM sessions WHERE ended_ns IS NOT NULL AND ended_ns < ?`, cutoff(p.EndedSessions)},
		}
		for _, step := range steps {
			r, err := t.tx.ExecContext(ctx, step.q, step.arg)
			if err != nil {
				return fmt.Errorf("storage: retention step failed: %w", err)
			}
			n, err := r.RowsAffected()
			if err != nil {
				return err
			}
			*step.dst += n
		}
		// Observations whose process row is gone are unreachable; drop them so
		// the time series cannot outlive its subject.
		r, err := t.tx.ExecContext(ctx,
			`DELETE FROM process_observations WHERE proc_uid NOT IN (SELECT proc_uid FROM processes)`)
		if err != nil {
			return fmt.Errorf("storage: pruning orphaned observations: %w", err)
		}
		n, err := r.RowsAffected()
		if err != nil {
			return err
		}
		res.Observations += n

		r, err = t.tx.ExecContext(ctx,
			`DELETE FROM process_activity WHERE proc_uid NOT IN (SELECT proc_uid FROM processes)`)
		if err != nil {
			return fmt.Errorf("storage: pruning orphaned activity: %w", err)
		}
		n, err = r.RowsAffected()
		if err != nil {
			return err
		}
		res.ActivitySamples += n

		r, err = t.tx.ExecContext(ctx,
			`DELETE FROM process_classifications WHERE proc_uid NOT IN (SELECT proc_uid FROM processes)`)
		if err != nil {
			return fmt.Errorf("storage: pruning orphaned classifications: %w", err)
		}
		n, err = r.RowsAffected()
		if err != nil {
			return err
		}
		res.Classifications += n

		// Edges whose session is gone are unreachable.
		r, err = t.tx.ExecContext(ctx,
			`DELETE FROM session_relationships WHERE session_id NOT IN (SELECT session_id FROM sessions)`)
		if err != nil {
			return fmt.Errorf("storage: pruning orphaned relationships: %w", err)
		}
		n, err = r.RowsAffected()
		if err != nil {
			return err
		}
		res.Relationships += n
		return nil
	})
}
