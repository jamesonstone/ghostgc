package main

import (
	"context"
	"testing"
)

func TestCacheCLIRefusesBroadOrUnconfirmedActions(t *testing.T) {
	tests := []struct {
		name   string
		action string
		args   []string
	}{
		{name: "cleanup lacks policy", action: "cleanup", args: []string{"--dry-run", "--artifact", "ca_test"}},
		{name: "purge lacks policy", action: "purge", args: []string{"--dry-run", "--artifact", "ca_test"}},
		{name: "apply lacks yes", action: "cleanup", args: []string{"--apply", "--approval", "token"}},
		{name: "purge lacks full confirmation", action: "purge", args: []string{"--apply", "--approval", "token", "--yes"}},
		{name: "apply widens artifact", action: "cleanup", args: []string{"--apply", "--approval", "token", "--yes", "--artifact", "ca_test"}},
		{name: "both modes", action: "cleanup", args: []string{"--dry-run", "--apply"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := cmdCacheAction(context.Background(), &env{}, tt.action, tt.args); err == nil {
				t.Fatal("unsafe cache command reached the API")
			}
		})
	}
}
