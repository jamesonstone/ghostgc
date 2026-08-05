package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
)

// PersistCacheEvaluation commits one complete cache projection atomically.
func (s *Store) PersistCacheEvaluation(ctx context.Context, evaluation cacheartifact.Evaluation, artifacts []cacheartifact.Artifact, decisions []cacheartifact.Decision) (int64, error) {
	var evaluationID int64
	err := s.WithTx(ctx, func(tx *Tx) error {
		result, err := tx.tx.ExecContext(ctx, `INSERT INTO cache_evaluations
			(observed_ns, configuration_digest, complete, inspected, protected, candidates, error)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, evaluation.ObservedNs, evaluation.ConfigurationDigest,
			evaluation.Complete, evaluation.Inspected, evaluation.Protected, evaluation.Candidates, evaluation.Error)
		if err != nil {
			return fmt.Errorf("storage: inserting cache evaluation: %w", err)
		}
		evaluationID, err = result.LastInsertId()
		if err != nil {
			return err
		}
		for i := range artifacts {
			artifacts[i].EvaluationID = evaluationID
			if err := tx.upsertCacheArtifact(artifacts[i]); err != nil {
				return err
			}
			if err := tx.insertCacheObservation(evaluationID, artifacts[i], evaluation.Complete); err != nil {
				return err
			}
		}
		for i := range decisions {
			decisions[i].EvaluationID = evaluationID
			if err := tx.insertCacheDecision(decisions[i]); err != nil {
				return err
			}
		}
		return tx.SetMeta("last_cache_scan_ns", fmt.Sprint(evaluation.ObservedNs))
	})
	return evaluationID, err
}

func (tx *Tx) upsertCacheArtifact(artifact cacheartifact.Artifact) error {
	identityJSON, _ := json.Marshal(artifact.Identity)
	rootIdentityJSON, _ := json.Marshal(artifact.RootIdentity)
	evidenceJSON, _ := json.Marshal(artifact.Evidence)
	_, err := tx.tx.ExecContext(tx.ctx, `INSERT INTO cache_artifacts (
		artifact_id, provider, agent_id, session_id, artifact_kind, root_path, relative_path,
		identity_json, root_identity_json, identity_digest, manifest_digest, first_observed_ns,
		last_observed_ns, stable_since_ns, lifecycle, reason, evidence, configuration_digest,
		evaluation_id, policy_id, quarantine_path, quarantined_at_ns, quarantine_digest
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(artifact_id) DO UPDATE SET
		provider=excluded.provider, agent_id=excluded.agent_id, session_id=excluded.session_id,
		artifact_kind=excluded.artifact_kind, root_path=excluded.root_path,
		relative_path=excluded.relative_path, identity_json=excluded.identity_json,
		root_identity_json=excluded.root_identity_json, identity_digest=excluded.identity_digest,
		manifest_digest=excluded.manifest_digest, first_observed_ns=MIN(cache_artifacts.first_observed_ns, excluded.first_observed_ns),
		last_observed_ns=excluded.last_observed_ns, stable_since_ns=excluded.stable_since_ns,
		lifecycle=excluded.lifecycle, reason=excluded.reason, evidence=excluded.evidence,
		configuration_digest=excluded.configuration_digest, evaluation_id=excluded.evaluation_id,
		policy_id=excluded.policy_id, quarantine_path=excluded.quarantine_path,
		quarantined_at_ns=excluded.quarantined_at_ns, quarantine_digest=excluded.quarantine_digest`,
		artifact.ID, artifact.Provider, artifact.Agent, artifact.SessionID, artifact.Kind,
		artifact.RootPath, artifact.RelativePath, string(identityJSON), string(rootIdentityJSON),
		artifact.IdentityDigest, artifact.ManifestDigest, artifact.FirstObservedNs,
		artifact.LastObservedNs, artifact.StableSinceNs, artifact.Lifecycle, artifact.Reason,
		string(evidenceJSON), artifact.Configuration, artifact.EvaluationID, artifact.PolicyID,
		artifact.QuarantinePath, artifact.QuarantinedAtNs, artifact.QuarantineDigest)
	if err != nil {
		return fmt.Errorf("storage: upserting cache artifact %s: %w", artifact.ID, err)
	}
	return nil
}

func (tx *Tx) insertCacheObservation(evaluationID int64, artifact cacheartifact.Artifact, complete bool) error {
	evidenceJSON, _ := json.Marshal(artifact.Evidence)
	_, err := tx.tx.ExecContext(tx.ctx, `INSERT INTO cache_observations
		(artifact_id, evaluation_id, observed_ns, identity_digest, manifest_digest, lifecycle, complete, evidence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, artifact.ID, evaluationID, artifact.LastObservedNs,
		artifact.IdentityDigest, artifact.ManifestDigest, artifact.Lifecycle, complete, string(evidenceJSON))
	if err != nil {
		return fmt.Errorf("storage: inserting cache observation: %w", err)
	}
	return nil
}

func (tx *Tx) insertCacheDecision(decision cacheartifact.Decision) error {
	evidenceJSON, _ := json.Marshal(decision.Evidence)
	_, err := tx.tx.ExecContext(tx.ctx, `INSERT INTO cache_decisions
		(evaluation_id, artifact_id, policy_id, result, reason, evidence) VALUES (?, ?, ?, ?, ?, ?)`,
		decision.EvaluationID, decision.ArtifactID, decision.PolicyID, decision.Result,
		decision.Reason, string(evidenceJSON))
	if err != nil {
		return fmt.Errorf("storage: inserting cache decision: %w", err)
	}
	return nil
}

// BeginCacheAction commits pre-side-effect evidence.
func (s *Store) BeginCacheAction(ctx context.Context, action cacheartifact.Action) error {
	evidenceJSON, _ := json.Marshal(action.Evidence)
	_, err := s.db.ExecContext(ctx, `INSERT INTO cache_actions
		(action_id, artifact_id, kind, policy_id, requested_ns, updated_ns, result, reason, evidence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, action.ActionID, action.ArtifactID, action.Kind,
		action.PolicyID, action.RequestedNs, action.UpdatedNs, action.Result, action.Reason, string(evidenceJSON))
	if err != nil {
		return fmt.Errorf("storage: beginning cache action: %w", err)
	}
	return nil
}

// FinishCacheAction records a refusal or failure without claiming a side effect.
func (s *Store) FinishCacheAction(ctx context.Context, actionID, result, reason string, evidence []string, nowNs int64) error {
	evidenceJSON, _ := json.Marshal(evidence)
	return s.WithTx(ctx, func(tx *Tx) error {
		res, err := tx.tx.ExecContext(ctx, `UPDATE cache_actions SET updated_ns=?, result=?, reason=?, evidence=? WHERE action_id=?`,
			nowNs, result, reason, string(evidenceJSON), actionID)
		if err != nil {
			return fmt.Errorf("storage: finishing cache action: %w", err)
		}
		rows, _ := res.RowsAffected()
		if rows != 1 {
			return ErrNotFound
		}
		if result == "partial" {
			_, err = tx.tx.ExecContext(ctx, `UPDATE cache_artifacts SET lifecycle=?, reason=?
				WHERE artifact_id=(SELECT artifact_id FROM cache_actions WHERE action_id=?)`,
				cacheartifact.StatePartial, reason, actionID)
		}
		return err
	})
}
