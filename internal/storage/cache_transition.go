package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
)

// RecordCacheQuarantined commits the moved identity and action completion.
func (s *Store) RecordCacheQuarantined(ctx context.Context, actionID string, item cacheartifact.Quarantine) error {
	identityJSON, _ := json.Marshal(item.Identity)
	return s.WithTx(ctx, func(tx *Tx) error {
		_, err := tx.tx.ExecContext(ctx, `INSERT INTO cache_quarantines
			(artifact_id, root_path, original_path, quarantine_path, identity_json, manifest_digest,
			 original_manifest_digest, quarantined_ns, grace_until_ns, status, updated_ns, configuration_digest)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(artifact_id) DO UPDATE SET root_path=excluded.root_path,
			original_path=excluded.original_path, quarantine_path=excluded.quarantine_path,
			identity_json=excluded.identity_json, manifest_digest=excluded.manifest_digest,
			original_manifest_digest=excluded.original_manifest_digest,
			quarantined_ns=excluded.quarantined_ns, grace_until_ns=excluded.grace_until_ns,
			status=excluded.status, updated_ns=excluded.updated_ns,
			configuration_digest=excluded.configuration_digest`,
			item.ArtifactID, item.RootPath, item.OriginalPath, item.QuarantinePath,
			string(identityJSON), item.ManifestDigest, item.OriginalManifest, item.QuarantinedNs,
			item.GraceUntilNs, item.Status, item.UpdatedNs, item.Configuration)
		if err != nil {
			return fmt.Errorf("storage: recording cache quarantine: %w", err)
		}
		_, err = tx.tx.ExecContext(ctx, `UPDATE cache_artifacts SET lifecycle=?, identity_json=?,
			identity_digest=?, manifest_digest=?, quarantine_path=?, quarantined_at_ns=?,
			quarantine_digest=?, reason=? WHERE artifact_id=?`, cacheartifact.StateQuarantined,
			string(identityJSON), item.Identity.Digest(), item.ManifestDigest, item.QuarantinePath,
			item.QuarantinedNs, item.ManifestDigest, "atomically quarantined on the provider filesystem", item.ArtifactID)
		if err != nil {
			return err
		}
		return finishCacheActionTx(tx, actionID, "quarantined", "atomic same-filesystem quarantine completed", item.UpdatedNs)
	})
}

// RecordCacheRestored commits one exact restoration.
func (s *Store) RecordCacheRestored(ctx context.Context, actionID, artifactID string, identity cacheartifact.Identity, manifest string, nowNs int64) error {
	identityJSON, _ := json.Marshal(identity)
	return s.WithTx(ctx, func(tx *Tx) error {
		if _, err := tx.tx.ExecContext(ctx, `UPDATE cache_quarantines SET status='restored', updated_ns=? WHERE artifact_id=?`, nowNs, artifactID); err != nil {
			return err
		}
		if _, err := tx.tx.ExecContext(ctx, `UPDATE cache_artifacts SET lifecycle=?, identity_json=?,
			identity_digest=?, manifest_digest=?, stable_since_ns=?, quarantine_path='', quarantined_at_ns=0,
			quarantine_digest='', reason=? WHERE artifact_id=?`, cacheartifact.StateRestored,
			string(identityJSON), identity.Digest(), manifest, nowNs, "restored to the exact absent original destination", artifactID); err != nil {
			return err
		}
		return finishCacheActionTx(tx, actionID, "restored", "atomic restoration completed", nowNs)
	})
}

// RecordCachePurged commits permanent quarantine-only deletion evidence.
func (s *Store) RecordCachePurged(ctx context.Context, actionID, artifactID string, nowNs int64) error {
	return s.WithTx(ctx, func(tx *Tx) error {
		if _, err := tx.tx.ExecContext(ctx, `UPDATE cache_quarantines SET status='purged', updated_ns=? WHERE artifact_id=?`, nowNs, artifactID); err != nil {
			return err
		}
		if _, err := tx.tx.ExecContext(ctx, `UPDATE cache_artifacts SET lifecycle=?, reason=? WHERE artifact_id=?`,
			cacheartifact.StatePurged, "permanently purged from quarantine after separate approval", artifactID); err != nil {
			return err
		}
		return finishCacheActionTx(tx, actionID, "purged", "quarantine-only purge completed", nowNs)
	})
}

func finishCacheActionTx(tx *Tx, actionID, result, reason string, nowNs int64) error {
	res, err := tx.tx.ExecContext(tx.ctx, `UPDATE cache_actions SET updated_ns=?, result=?, reason=? WHERE action_id=?`,
		nowNs, result, reason, actionID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows != 1 {
		return ErrNotFound
	}
	return nil
}
