package api

import (
	"encoding/json"
	"testing"
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
		"sessions": true, "cleanup_candidates": true, "signalling_enabled": true,
		"manual_cleanup_enabled": true, "automatic_cleanup_enabled": true,
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
