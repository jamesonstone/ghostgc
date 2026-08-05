package main

import (
	"context"
	"testing"
)

func TestWorktreeCLIRequiresExactPurgeConfirmationOnly(t *testing.T) {
	for name, fixture := range map[string]struct {
		action string
		args   []string
	}{
		"purge lacks confirmation":     {"purge", []string{"--apply", "--approval", "token", "--yes"}},
		"restore rejects confirmation": {"restore", []string{"--apply", "--approval", "token", "--yes", "--confirm", "id"}},
		"retire rejects confirmation":  {"remove", []string{"--apply", "--approval", "token", "--yes", "--confirm", "id"}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := cmdWorktreeAction(context.Background(), &env{}, fixture.action, fixture.args); err == nil {
				t.Fatal("unsafe worktree command reached the API")
			}
		})
	}
}
