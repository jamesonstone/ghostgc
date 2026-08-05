package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
)

// ListCacheActions returns durable cache action history.
func (s *Store) ListCacheActions(ctx context.Context, artifactID, kind, result string, limit int) ([]cacheartifact.Action, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1_000 {
		limit = 1_000
	}
	query := `SELECT id, action_id, artifact_id, kind, policy_id, requested_ns, updated_ns,
		result, reason, evidence FROM cache_actions WHERE 1=1`
	var args []any
	for _, filter := range []struct{ column, value string }{
		{"artifact_id", artifactID}, {"kind", kind}, {"result", result},
	} {
		if filter.value != "" {
			query += " AND " + filter.column + " = ?"
			args = append(args, filter.value)
		}
	}
	query += " ORDER BY requested_ns DESC"
	query += " LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: listing cache actions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []cacheartifact.Action
	for rows.Next() {
		var item cacheartifact.Action
		var evidenceJSON string
		if err := rows.Scan(&item.ID, &item.ActionID, &item.ArtifactID, &item.Kind, &item.PolicyID,
			&item.RequestedNs, &item.UpdatedNs, &item.Result, &item.Reason, &evidenceJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(evidenceJSON), &item.Evidence); err != nil {
			return nil, fmt.Errorf("storage: decoding cache action evidence: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
