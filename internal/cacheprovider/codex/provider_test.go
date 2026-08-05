package codex

import (
	"context"
	"strings"
	"testing"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
	"github.com/jamesonstone/ghostgc/internal/cachefs"
	"github.com/jamesonstone/ghostgc/internal/cacheprovider"
)

const (
	testThread  = "019fcde3-594a-7eb1-a102-ee8c7893c2dc"
	otherThread = "119fcde3-594a-7eb1-a102-ee8c7893c2dc"
)

func TestProviderAcceptsOnlyExactCompletedExclusiveSnapshot(t *testing.T) {
	root := "/tmp/codex/shell_snapshots"
	rootID := testIdentity(7, 10, "directory")
	entryID := testIdentity(7, 11, "regular")
	filesystem := cachefs.NewFake()
	filesystem.SetRoot(root, rootID)
	filesystem.Put(root, testThread+".42.sh", entryID)
	sessions := []cacheprovider.Session{{
		ID: "session-1", NativeID: testThread, Agent: "codex", State: "completed", CodexHome: "/tmp/codex", Confidence: 1,
	}}

	result, err := New(501, root).Observe(context.Background(), sessions, filesystem, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.Inspected != 1 || len(result.Artifacts) != 1 {
		t.Fatalf("observation = %#v", result)
	}
	artifact := result.Artifacts[0]
	if artifact.Lifecycle == cacheartifact.StateProtected || artifact.SessionID != "session-1" {
		t.Fatalf("exact completed snapshot unexpectedly protected: %#v", artifact)
	}
	if artifact.ID == "" || strings.Contains(artifact.ID, testThread) {
		t.Fatalf("artifact id must be opaque, got %q", artifact.ID)
	}
}

func TestProviderProtectsAmbiguousOrUnsafeEntries(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		entry   cacheartifact.Identity
		session cacheprovider.Session
		extra   *cacheprovider.Session
		reason  string
	}{
		{name: "unknown thread", file: otherThread + ".1.sh", entry: testIdentity(7, 11, "regular"), reason: "0 known"},
		{name: "malformed", file: testThread + ".sh", entry: testIdentity(7, 11, "regular"), reason: "does not match"},
		{name: "active session", file: testThread + ".1.sh", entry: testIdentity(7, 11, "regular"), session: testSession("running"), reason: "not completed"},
		{name: "live process", file: testThread + ".1.sh", entry: testIdentity(7, 11, "regular"), session: liveSession(), reason: "live process"},
		{name: "hard link", file: testThread + ".1.sh", entry: withLinks(testIdentity(7, 11, "regular"), 2), session: testSession("completed"), reason: "hard-link"},
		{name: "symlink", file: testThread + ".1.sh", entry: testIdentity(7, 11, "symlink"), session: testSession("completed"), reason: "not a regular"},
		{name: "cross device", file: testThread + ".1.sh", entry: testIdentity(8, 11, "regular"), session: testSession("completed"), reason: "crosses"},
		{name: "foreign uid", file: testThread + ".1.sh", entry: withUID(testIdentity(7, 11, "regular"), 777), session: testSession("completed"), reason: "another user"},
		{name: "weak ownership", file: testThread + ".1.sh", entry: testIdentity(7, 11, "regular"), session: weakSession(), reason: "below the exclusive"},
		{name: "ambiguous", file: testThread + ".1.sh", entry: testIdentity(7, 11, "regular"), session: testSession("completed"), extra: ptrSession(testSession("completed")), reason: "2 known"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filesystem := cachefs.NewFake()
			root := "/tmp/codex/shell_snapshots"
			filesystem.SetRoot(root, testIdentity(7, 10, "directory"))
			filesystem.Put(root, tt.file, tt.entry)
			session := tt.session
			if session.CodexHome == "" {
				session = testSession("completed")
			}
			sessions := []cacheprovider.Session{session}
			if tt.extra != nil {
				sessions = append(sessions, *tt.extra)
			}
			result, err := New(501, root).Observe(context.Background(), sessions, filesystem, 10)
			if err != nil {
				t.Fatal(err)
			}
			artifact := result.Artifacts[0]
			if artifact.Lifecycle != cacheartifact.StateProtected || !strings.Contains(strings.Join(artifact.Evidence, " "), tt.reason) {
				t.Fatalf("unsafe entry was not protected for %q: %#v", tt.reason, artifact)
			}
		})
	}
}

func TestProviderFailsClosedWhenTraversalBoundIsExhausted(t *testing.T) {
	filesystem := cachefs.NewFake()
	root := "/tmp/codex/shell_snapshots"
	filesystem.SetRoot(root, testIdentity(7, 10, "directory"))
	filesystem.Put(root, testThread+".1.sh", testIdentity(7, 11, "regular"))
	filesystem.Put(root, testThread+".2.sh", testIdentity(7, 12, "regular"))

	result, err := New(501, root).Observe(context.Background(), []cacheprovider.Session{testSession("completed")}, filesystem, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete || len(result.Artifacts) != 1 || result.Artifacts[0].Lifecycle != cacheartifact.StateProtected {
		t.Fatalf("bounded scan must be incomplete and protected: %#v", result)
	}
}

func TestProviderBoundsSessionAndRootWork(t *testing.T) {
	filesystem := cachefs.NewFake()
	filesystem.SetRoot("/tmp/a/shell_snapshots", testIdentity(7, 10, "directory"))
	filesystem.SetRoot("/tmp/b/shell_snapshots", testIdentity(7, 20, "directory"))
	sessions := []cacheprovider.Session{
		{ID: "a", NativeID: testThread, Agent: "codex", State: "completed", CodexHome: "/tmp/a", Confidence: 1},
		{ID: "b", NativeID: otherThread, Agent: "codex", State: "completed", CodexHome: "/tmp/b", Confidence: 1},
	}
	result, err := New(501, "/tmp/a/shell_snapshots", "/tmp/b/shell_snapshots").Observe(context.Background(), sessions, filesystem, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete {
		t.Fatal("session/root truncation must make the provider result incomplete")
	}
}

func TestProviderIgnoresSessionRootsOutsideExplicitAllowlist(t *testing.T) {
	filesystem := cachefs.NewFake()
	root := "/tmp/codex/shell_snapshots"
	filesystem.SetRoot(root, testIdentity(7, 10, "directory"))
	filesystem.Put(root, testThread+".1.sh", testIdentity(7, 11, "regular"))
	result, err := New(501, "/tmp/other/shell_snapshots").Observe(
		context.Background(), []cacheprovider.Session{testSession("completed")}, filesystem, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 0 || result.Inspected != 0 {
		t.Fatalf("unapproved root was observed: %#v", result)
	}
}

func TestSnapshotFilenameContract(t *testing.T) {
	for _, name := range []string{testThread + ".0.sh", testThread + ".18446744073709551615.ps1"} {
		if got, ok := parseSnapshotName(name); !ok || got != testThread {
			t.Fatalf("valid name %q rejected", name)
		}
	}
	for _, name := range []string{testThread + ".-1.sh", testThread + ".tmp-1", strings.ToUpper(testThread) + ".1.sh", testThread + ".1.txt", "anything.sh"} {
		if _, ok := parseSnapshotName(name); ok {
			t.Fatalf("invalid name %q accepted", name)
		}
	}
}

func testIdentity(device, inode uint64, kind string) cacheartifact.Identity {
	return cacheartifact.Identity{UID: 501, Device: device, Inode: inode, Mode: 0o100600, Nlink: 1, Size: 12, MTimeNs: 1, CTimeNs: 2, ATimeNs: 3, EntryType: kind}
}

func testSession(state string) cacheprovider.Session {
	return cacheprovider.Session{ID: "session-1", NativeID: testThread, Agent: "codex", State: state, CodexHome: "/tmp/codex", Confidence: 1}
}

func liveSession() cacheprovider.Session {
	session := testSession("completed")
	session.LiveProcesses = 1
	return session
}

func weakSession() cacheprovider.Session {
	session := testSession("completed")
	session.Confidence = 0.9
	return session
}

func withLinks(identity cacheartifact.Identity, links uint64) cacheartifact.Identity {
	identity.Nlink = links
	return identity
}
func withUID(identity cacheartifact.Identity, uid uint32) cacheartifact.Identity {
	identity.UID = uid
	return identity
}
func ptrSession(session cacheprovider.Session) *cacheprovider.Session { return &session }
