package worktree

import (
	"strings"
	"testing"
	"time"
)

func TestParseRegistrationsPreservesUnusualPaths(t *testing.T) {
	path := "/tmp/a path\nwith-tab\tand spaces"
	raw := []byte("worktree " + path + "\x00HEAD abc123\x00branch refs/heads/topic\x00\x00" +
		"worktree /tmp/detached\x00HEAD def456\x00detached\x00locked reason\x00\x00")
	records, err := ParseRegistrations(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Path != path || records[0].Branch != "topic" {
		t.Fatalf("registrations = %+v", records)
	}
	if !records[1].Detached || !records[1].Locked {
		t.Fatalf("detached registration = %+v", records[1])
	}
}

func TestParseStatusStoresCountsNotPaths(t *testing.T) {
	secretName := "private-customer-name.txt"
	raw := []byte("1 M. N... 100644 100644 100644 abc abc tracked\x00? " + secretName + "\x00! ignored\x00")
	status, err := ParseStatus(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status.Staged != 1 || status.Untracked != 1 || status.Ignored != 1 || status.Fingerprint == "" {
		t.Fatalf("status = %+v", status)
	}
	if strings.Contains(status.Fingerprint, secretName) {
		t.Fatal("fingerprint retained a filename")
	}
}

func TestParseStatusRejectsMalformedEntries(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte("1 X N... 100644 100644 100644 abc abc path\x00"),
		[]byte("2 R. N... 100644 100644 100644 abc abc R100 path\x00"),
	} {
		if _, err := ParseStatus(raw, nil); err == nil {
			t.Fatalf("expected malformed entry %q to be refused", raw)
		}
	}
}

func TestStableIDMoveAndRecreationContract(t *testing.T) {
	common := FileIdentity{Path: "/repo/.git", Device: 1, Inode: 10}
	admin := FileIdentity{Path: "/repo/.git/worktrees/lane", Device: 1, Inode: 20}
	stable := StableID(common, admin)
	if stable == "" {
		t.Fatal("stable identity was empty")
	}
	recreated := admin
	recreated.Inode++
	if stable == StableID(common, recreated) {
		t.Fatal("recreated administration reused identity")
	}
}

func TestContinuousInactivityStateMachine(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	daemonStart := now.Add(-8 * 24 * time.Hour)
	obs := Observation{ID: "id", Present: true, Canonical: true, Complete: true, HEAD: "a", Ref: "refs/heads/main", Status: StatusEvidence{Fingerprint: "clean"}}
	base := Record{ID: "id", State: StateObserving, HEAD: "a", Ref: obs.Ref,
		StatusFingerprint: "clean", LastSeen: now.Add(-5 * time.Minute),
		LastActivity: now.Add(-7 * 24 * time.Hour), InactiveSince: now.Add(-7 * 24 * time.Hour), DaemonStarted: daemonStart}
	got := Classify(base, obs, now, daemonStart, 7*24*time.Hour, 5*time.Minute, false, true)
	if got.State != StateStale {
		t.Fatalf("boundary state = %s", got.State)
	}
	checks := []struct {
		name             string
		previous         Record
		observation      Observation
		at, start        time.Time
		active, complete bool
		want             State
	}{
		{"activity", base, obs, now, daemonStart, true, true, StateActive},
		{"activity ended", func() Record { r := base; r.State = StateActive; r.InactiveSince = time.Time{}; return r }(), obs, now, daemonStart, false, true, StateObserving},
		{"restart", base, obs, now, now.Add(-time.Minute), false, true, StateObserving},
		{"scan gap", func() Record { r := base; r.LastSeen = now.Add(-11 * time.Minute); return r }(), obs, now, daemonStart, false, true, StateObserving},
		{"unknown", base, obs, now, daemonStart, false, false, StateUnknown},
		{"git change", base, func() Observation { o := obs; o.HEAD = "b"; return o }(), now, daemonStart, false, true, StateObserving},
		{"clock backwards", base, obs, base.LastSeen.Add(-time.Second), daemonStart, false, true, StateObserving},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			result := Classify(check.previous, check.observation, check.at, check.start, 7*24*time.Hour, 5*time.Minute, check.active, check.complete)
			if result.State != check.want {
				t.Fatalf("state = %s, want %s", result.State, check.want)
			}
			if check.want != StateStale && !result.InactiveSince.IsZero() && result.InactiveSince.Before(check.at) {
				t.Fatal("unsafe inactivity window survived reset")
			}
		})
	}
}
