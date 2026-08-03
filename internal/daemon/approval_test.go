package daemon

import (
	"testing"
	"time"
)

func TestApprovalExpiryAndSingleUse(t *testing.T) {
	d := &Daemon{approvals: map[string]*cleanupApproval{}}
	token, digest, err := newSecret(16)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	d.approvals[digest] = &cleanupApproval{expires: now.Add(-time.Second)}
	if approval, reason := d.consumeApproval(token, now); approval == nil || reason != "approval has expired" {
		t.Fatalf("expired approval = %+v, %q", approval, reason)
	}
	if _, reason := d.consumeApproval(token, now); reason != "approval has already been consumed" {
		t.Fatalf("replayed approval reason = %q", reason)
	}
}
