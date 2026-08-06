package api

import (
	"encoding/json"
	"testing"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
)

func TestStatusResponseOmitsDevelopmentMetadata(t *testing.T) {
	payload, err := json.Marshal(StatusResponse{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	expected := map[string]bool{
		"health": true, "mode": true, "version": true, "platform": true,
		"pid": true, "started_ns": true, "uptime_seconds": true, "agents": true,
		"sessions_by_state": true, "classifications_by_state": true,
		"sessions": true, "cleanup_candidates": true, "candidate_diagnostics": true, "signalling_enabled": true,
		"manual_cleanup_enabled": true, "automatic_cleanup_enabled": true,
		"cache_enabled": true, "cache_mode": true, "cache_candidates": true,
		"cache_quarantined":  true,
		"worktrees_by_state": true, "stale_worktrees": true, "protected_worktrees": true,
	}
	if len(fields) != len(expected) {
		t.Fatalf("status response fields = %v, want %v", fields, expected)
	}
	for field := range fields {
		if !expected[field] {
			t.Fatalf("status response exposes unexpected metadata %q: %s", field, payload)
		}
	}
}

func TestCacheJSONUsesStableOpaqueContract(t *testing.T) {
	payload, err := json.Marshal(CachePreviewResponse{
		Action: "cleanup", Approval: "secret", Artifact: cacheartifact.Artifact{ID: "ca_opaque"},
		Destination: ".ghostgc-quarantine/ca_opaque", Revalidation: []string{"identity"}, Note: "preview",
	})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"action", "approval", "expires_ns", "artifact", "destination", "command", "revalidation", "note"} {
		if _, ok := fields[required]; !ok {
			t.Fatalf("cache preview JSON omitted %q: %s", required, payload)
		}
	}
	var artifact map[string]json.RawMessage
	if err := json.Unmarshal(fields["artifact"], &artifact); err != nil {
		t.Fatal(err)
	}
	if string(artifact["artifact_id"]) != `"ca_opaque"` {
		t.Fatalf("cache identity is not exposed through artifact_id: %s", fields["artifact"])
	}
	for _, forbidden := range []string{"path_glob", "automatic", "contents"} {
		if _, ok := fields[forbidden]; ok {
			t.Fatalf("cache JSON exposed forbidden authority %q", forbidden)
		}
	}
}
