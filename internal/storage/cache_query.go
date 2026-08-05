package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
)

const cacheArtifactColumns = `artifact_id, provider, agent_id, session_id, artifact_kind,
	root_path, relative_path, identity_json, root_identity_json, identity_digest, manifest_digest,
	first_observed_ns, last_observed_ns, stable_since_ns, lifecycle, reason, evidence,
	configuration_digest, evaluation_id, policy_id, quarantine_path, quarantined_at_ns, quarantine_digest`

// ListCacheArtifacts returns current projections, newest first.
func (s *Store) ListCacheArtifacts(ctx context.Context, lifecycle string, currentOnly bool) ([]cacheartifact.Artifact, error) {
	query := `SELECT ` + cacheArtifactColumns + ` FROM cache_artifacts`
	var where []string
	var args []any
	if lifecycle != "" {
		where = append(where, "lifecycle = ?")
		args = append(args, lifecycle)
	}
	if currentOnly {
		where = append(where, "evaluation_id = (SELECT MAX(id) FROM cache_evaluations)")
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY last_observed_ns DESC, artifact_id"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: listing cache artifacts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []cacheartifact.Artifact
	for rows.Next() {
		artifact, err := scanCacheArtifact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, artifact)
	}
	return out, rows.Err()
}

// CacheArtifact returns one exact artifact projection.
func (s *Store) CacheArtifact(ctx context.Context, id string) (cacheartifact.Artifact, error) {
	artifact, err := scanCacheArtifact(s.db.QueryRowContext(ctx,
		`SELECT `+cacheArtifactColumns+` FROM cache_artifacts WHERE artifact_id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return cacheartifact.Artifact{}, ErrNotFound
	}
	return artifact, err
}

// CacheArtifactMap returns only the newest committed evaluation keyed by ID.
func (s *Store) CacheArtifactMap(ctx context.Context) (map[string]cacheartifact.Artifact, error) {
	artifacts, err := s.ListCacheArtifacts(ctx, "", true)
	if err != nil {
		return nil, err
	}
	out := make(map[string]cacheartifact.Artifact, len(artifacts))
	for _, artifact := range artifacts {
		out[artifact.ID] = artifact
	}
	return out, nil
}

func scanCacheArtifact(row rowScanner) (cacheartifact.Artifact, error) {
	var artifact cacheartifact.Artifact
	var identityJSON, rootIdentityJSON, evidenceJSON string
	err := row.Scan(&artifact.ID, &artifact.Provider, &artifact.Agent, &artifact.SessionID, &artifact.Kind,
		&artifact.RootPath, &artifact.RelativePath, &identityJSON, &rootIdentityJSON,
		&artifact.IdentityDigest, &artifact.ManifestDigest, &artifact.FirstObservedNs,
		&artifact.LastObservedNs, &artifact.StableSinceNs, &artifact.Lifecycle,
		&artifact.Reason, &evidenceJSON, &artifact.Configuration, &artifact.EvaluationID,
		&artifact.PolicyID, &artifact.QuarantinePath, &artifact.QuarantinedAtNs, &artifact.QuarantineDigest)
	if err != nil {
		return cacheartifact.Artifact{}, err
	}
	if err := json.Unmarshal([]byte(identityJSON), &artifact.Identity); err != nil {
		return cacheartifact.Artifact{}, fmt.Errorf("storage: decoding cache identity: %w", err)
	}
	if err := json.Unmarshal([]byte(rootIdentityJSON), &artifact.RootIdentity); err != nil {
		return cacheartifact.Artifact{}, fmt.Errorf("storage: decoding cache root identity: %w", err)
	}
	if err := json.Unmarshal([]byte(evidenceJSON), &artifact.Evidence); err != nil {
		return cacheartifact.Artifact{}, fmt.Errorf("storage: decoding cache evidence: %w", err)
	}
	return artifact, nil
}

// ListCacheQuarantines returns durable quarantine projections.
func (s *Store) ListCacheQuarantines(ctx context.Context, status string) ([]cacheartifact.Quarantine, error) {
	query := `SELECT artifact_id, root_path, original_path, quarantine_path, identity_json,
		manifest_digest, original_manifest_digest, quarantined_ns, grace_until_ns, status, updated_ns,
		configuration_digest FROM cache_quarantines`
	var args []any
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY updated_ns DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: listing cache quarantines: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []cacheartifact.Quarantine
	for rows.Next() {
		item, err := scanCacheQuarantine(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// CacheQuarantine returns one exact quarantine record.
func (s *Store) CacheQuarantine(ctx context.Context, id string) (cacheartifact.Quarantine, error) {
	item, err := scanCacheQuarantine(s.db.QueryRowContext(ctx, `SELECT artifact_id, root_path,
		original_path, quarantine_path, identity_json, manifest_digest, original_manifest_digest,
		quarantined_ns, grace_until_ns, status, updated_ns, configuration_digest
		FROM cache_quarantines WHERE artifact_id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return cacheartifact.Quarantine{}, ErrNotFound
	}
	return item, err
}

func scanCacheQuarantine(row rowScanner) (cacheartifact.Quarantine, error) {
	var item cacheartifact.Quarantine
	var identityJSON string
	err := row.Scan(&item.ArtifactID, &item.RootPath, &item.OriginalPath, &item.QuarantinePath,
		&identityJSON, &item.ManifestDigest, &item.OriginalManifest, &item.QuarantinedNs,
		&item.GraceUntilNs, &item.Status, &item.UpdatedNs, &item.Configuration)
	if err != nil {
		return cacheartifact.Quarantine{}, err
	}
	if err := json.Unmarshal([]byte(identityJSON), &item.Identity); err != nil {
		return cacheartifact.Quarantine{}, fmt.Errorf("storage: decoding quarantine identity: %w", err)
	}
	return item, nil
}
